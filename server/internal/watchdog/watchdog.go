// Package watchdog 故障自动直连（看门狗）：每隔一段时间检查核心控制接口；
// 无响应时自动重启核心，连续失败达到阈值后关闭透明代理让局域网直连，核心恢复后自动重新接管。
// 最近 20 条事件落盘保存（满了删最旧的）。
package watchdog

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Skycnhe/prismnet/server/internal/store"
)

// MaxEvents 事件最多保留条数。
const MaxEvents = 20

// Event 一条看门狗事件。
type Event struct {
	T    int64  `json:"t"`
	Kind string `json:"kind"` // restart / bypass / recover / error
	Msg  string `json:"msg"`
}

// Status 看门狗状态。
type Status struct {
	Enabled   bool    `json:"enabled"`
	State     string  `json:"state"` // ok / failing / bypass / disabled
	Fails     int     `json:"fails"`
	LastCheck int64   `json:"last_check"`
	Interval  int     `json:"interval"`
	Threshold int     `json:"threshold"`
	Version   string  `json:"version"`
	Events    []Event `json:"events"`
	Counts    struct {
		Restart int `json:"restart"`
		Bypass  int `json:"bypass"`
		Recover int `json:"recover"`
	} `json:"counts"`
}

// Checker 检查核心是否存活，返回版本号。
type Checker func(ctx context.Context) (string, error)

// Runner 执行一条 shell 命令。
type Runner func(ctx context.Context, cmd string) error

// Options 看门狗参数。
type Options struct {
	Interval   time.Duration
	Timeout    time.Duration
	Threshold  int
	RestartCmd string
	BypassCmd  string
	RestoreCmd string
}

// Watchdog 看门狗。
type Watchdog struct {
	Check Checker
	Run   Runner
	File  string
	Now   func() time.Time

	mu          sync.Mutex
	opt         Options
	enabled     bool
	st          Status
	bypass      bool
	bypassTicks int
	paused      time.Time // 面板主动重启核心时暂停判定到该时间
}

// New 创建看门狗。
func New(check Checker, file string, opt Options, enabled bool) *Watchdog {
	w := &Watchdog{Check: check, Run: Shell, File: file, opt: opt, enabled: enabled}
	var saved struct {
		Events []Event `json:"events"`
		Bypass bool    `json:"bypass"`
	}
	if file != "" && store.Read(file, &saved) {
		w.st.Events = saved.Events
		w.bypass = saved.Bypass
	}
	return w
}

// Shell 用 /bin/sh -c 执行命令。
func Shell(ctx context.Context, cmd string) error {
	if strings.TrimSpace(cmd) == "" {
		return nil
	}
	out, err := exec.CommandContext(ctx, "/bin/sh", "-c", cmd).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// DefaultRestartCmd 根据系统选择重启核心服务的命令。
func DefaultRestartCmd(service string) string {
	if _, err := os.Stat("/sbin/openrc-run"); err == nil {
		return "rc-service " + service + " restart"
	}
	if _, err := os.Stat("/sbin/rc-service"); err == nil {
		return "rc-service " + service + " restart"
	}
	return "systemctl restart " + service
}

func (w *Watchdog) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

// SetEnabled 开关看门狗。
func (w *Watchdog) SetEnabled(v bool) {
	w.mu.Lock()
	w.enabled = v
	w.mu.Unlock()
}

// Pause 在 d 时间内不判定失败（例如面板刚重载 / 重启核心）。
func (w *Watchdog) Pause(d time.Duration) {
	w.mu.Lock()
	w.paused = w.now().Add(d)
	w.mu.Unlock()
}

func (w *Watchdog) addEvent(kind, msg string) {
	w.st.Events = append(w.st.Events, Event{T: w.now().Unix(), Kind: kind, Msg: msg})
	if len(w.st.Events) > MaxEvents {
		w.st.Events = w.st.Events[len(w.st.Events)-MaxEvents:]
	}
	if w.File != "" {
		_ = store.Write(w.File, map[string]any{"events": w.st.Events, "bypass": w.bypass})
	}
}

// Tick 执行一次检查。
func (w *Watchdog) Tick(ctx context.Context) {
	w.mu.Lock()
	if !w.enabled || w.now().Before(w.paused) {
		w.mu.Unlock()
		return
	}
	opt := w.opt
	w.mu.Unlock()

	c2, cancel := context.WithTimeout(ctx, opt.Timeout)
	ver, err := w.Check(c2)
	cancel()

	w.mu.Lock()
	defer w.mu.Unlock()
	w.st.LastCheck = w.now().Unix()
	if err == nil {
		w.st.Version = ver
		if w.bypass {
			if e := w.runLocked(ctx, opt.RestoreCmd); e != nil {
				w.addEvent("error", "恢复透明代理失败："+e.Error())
				return
			}
			w.bypass = false
			w.addEvent("recover", "核心已恢复，透明代理重新接管")
		} else if w.st.Fails > 0 {
			w.addEvent("recover", "核心已恢复")
		}
		w.st.Fails = 0
		return
	}
	w.st.Fails++
	if w.bypass {
		// 已直连：保持核心停止（直连命令幂等），每 4 次检查尝试重启一次核心，下次检查应答即恢复接管
		w.bypassTicks++
		if w.bypassTicks%4 == 0 {
			msg := "直连中，尝试重启核心"
			if e := w.runLocked(ctx, opt.RestartCmd); e != nil {
				msg += "失败：" + e.Error()
			}
			w.addEvent("restart", msg)
		} else {
			_ = w.runLocked(ctx, opt.BypassCmd)
		}
		return
	}
	if w.st.Fails >= opt.Threshold {
		e := w.runLocked(ctx, opt.BypassCmd)
		if e != nil {
			w.addEvent("error", "切换直连失败："+e.Error())
		} else {
			w.bypass = true
			w.bypassTicks = 0
			w.addEvent("bypass", fmt.Sprintf("连续 %d 次无响应，已关闭透明代理，局域网直连", w.st.Fails))
		}
		return
	}
	msg := fmt.Sprintf("核心无响应（%s），自动重启", short(err))
	if e := w.runLocked(ctx, opt.RestartCmd); e != nil {
		msg += "失败：" + e.Error()
	}
	w.addEvent("restart", msg)
}

func (w *Watchdog) runLocked(ctx context.Context, cmd string) error {
	if w.Run == nil || strings.TrimSpace(cmd) == "" {
		return nil
	}
	c2, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	w.mu.Unlock()
	defer w.mu.Lock()
	return w.Run(c2, cmd)
}

func short(err error) string {
	s := err.Error()
	if strings.Contains(s, "deadline exceeded") || strings.Contains(s, "Timeout") {
		return "超时"
	}
	if strings.Contains(s, "connection refused") {
		return "连接被拒绝"
	}
	if len(s) > 60 {
		s = s[:60] + "…"
	}
	return s
}

// Status 当前状态（含事件计数）。
func (w *Watchdog) Status() Status {
	w.mu.Lock()
	defer w.mu.Unlock()
	st := w.st
	st.Enabled = w.enabled
	st.Interval = int(w.opt.Interval / time.Second)
	st.Threshold = w.opt.Threshold
	st.Events = append([]Event(nil), w.st.Events...)
	switch {
	case !w.enabled:
		st.State = "disabled"
	case w.bypass:
		st.State = "bypass"
	case w.st.Fails > 0:
		st.State = "failing"
	default:
		st.State = "ok"
	}
	for _, e := range st.Events {
		switch e.Kind {
		case "restart":
			st.Counts.Restart++
		case "bypass":
			st.Counts.Bypass++
		case "recover":
			st.Counts.Recover++
		}
	}
	return st
}

// Loop 按间隔循环检查直到 ctx 取消。
func (w *Watchdog) Loop(ctx context.Context) {
	for {
		w.mu.Lock()
		iv := w.opt.Interval
		w.mu.Unlock()
		select {
		case <-ctx.Done():
			return
		case <-time.After(iv):
		}
		w.Tick(ctx)
	}
}
