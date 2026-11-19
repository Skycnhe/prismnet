// Command prismnet 是 PrismNet 面板的后端：提供面板静态文件、mihomo 外部控制器反向代理，
// 以及站点延迟、IP 信息、广告拦截、DNS 统计、看门狗、在线更新等扩展 API。
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Skycnhe/prismnet/server/internal/adblock"
	"github.com/Skycnhe/prismnet/server/internal/api"
	"github.com/Skycnhe/prismnet/server/internal/config"
	"github.com/Skycnhe/prismnet/server/internal/ipinfo"
	"github.com/Skycnhe/prismnet/server/internal/logstat"
	"github.com/Skycnhe/prismnet/server/internal/mihomo"
	"github.com/Skycnhe/prismnet/server/internal/sites"
	"github.com/Skycnhe/prismnet/server/internal/update"
	"github.com/Skycnhe/prismnet/server/internal/watchdog"
)

// version 由构建时 -ldflags "-X main.version=vX.Y.Z" 写入。
var version = "dev"

func main() {
	cfgPath := flag.String("c", "/etc/prismnet/config.json", "配置文件路径")
	listen := flag.String("listen", "", "监听地址（覆盖配置文件）")
	uiDir := flag.String("ui", "", "面板静态文件目录（覆盖配置文件）")
	showVer := flag.Bool("version", false, "显示版本")
	initCfg := flag.Bool("init", false, "写出默认配置文件后退出（已存在则不覆盖）")
	flag.Parse()
	if *showVer {
		fmt.Println("prismnet", version)
		return
	}
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		log.Fatalf("读取配置失败：%v", err)
	}
	if *initCfg {
		if _, err := os.Stat(*cfgPath); err == nil {
			fmt.Println("配置已存在：", *cfgPath)
			return
		}
		if err := cfg.Save(); err != nil {
			log.Fatal(err)
		}
		fmt.Println("已写出默认配置：", *cfgPath)
		return
	}
	if *listen != "" {
		cfg.Listen = *listen
	}
	if *uiDir != "" {
		cfg.UIDir = *uiDir
	}
	if cfg.Watchdog.RestartCmd == "" {
		cfg.Watchdog.RestartCmd = watchdog.DefaultRestartCmd(cfg.Mihomo.Service)
	}
	if cfg.Watchdog.BypassCmd == "" {
		cfg.Watchdog.BypassCmd = stopCmd(cfg.Watchdog.RestartCmd)
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		log.Fatalf("创建数据目录失败：%v", err)
	}

	core, err := mihomo.New(cfg.Mihomo.Controller, cfg.Mihomo.Secret, cfg.Mihomo.MixedPort)
	if err != nil {
		log.Fatal(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	stats := &logstat.Stats{C: core, File: filepath.Join(cfg.DataDir, "stats.json")}
	stats.Load()
	go stats.Run(ctx)

	ab := &adblock.Manager{
		File:       filepath.Join(cfg.DataDir, "adblock.json"),
		CacheDir:   filepath.Join(cfg.DataDir, "adblock"),
		ConfigPath: cfg.Mihomo.ConfigPath,
		Core:       api.CoreOps{C: core},
		Fetch:      adblock.HTTPFetcher(mihomo.DirectHTTP(90*time.Second), core.ProxyHTTP(90*time.Second, false)),
	}
	ab.Load()
	go adblockDaily(ctx, ab)

	wd := watchdog.New(func(c context.Context) (string, error) { return core.Alive(c) },
		filepath.Join(cfg.DataDir, "watchdog.json"), watchdog.Options{
			Interval:   time.Duration(cfg.Watchdog.Interval) * time.Second,
			Timeout:    time.Duration(cfg.Watchdog.Timeout) * time.Second,
			Threshold:  cfg.Watchdog.FailThreshold,
			RestartCmd: cfg.Watchdog.RestartCmd,
			BypassCmd:  cfg.Watchdog.BypassCmd,
			RestoreCmd: cfg.Watchdog.RestoreCmd,
		}, cfg.Watchdog.Enabled)
	wd.Pause(20 * time.Second)
	go wd.Loop(ctx)

	up := &update.Updater{
		Repo: cfg.Update.Repo, Current: version, UIDir: cfg.UIDir, Mirror: cfg.Update.Mirror,
		WorkDir: filepath.Join(cfg.DataDir, "update"),
		Client: func() *http.Client {
			if cfg.Update.Proxy {
				return core.ProxyHTTP(5*time.Minute, false)
			}
			return &http.Client{Timeout: 5 * time.Minute}
		},
	}

	srv := &api.Server{
		Cfg: cfg, Version: version, Core: core,
		Sites:    &sites.Tester{P: core, Sites: func() []config.Site { return cfg.Sites }},
		IP:       &ipinfo.Service{C: core},
		Stats:    stats,
		Adblock:  ab,
		Watchdog: wd,
		Updater:  up,
		RunCmd:   watchdog.Shell,
		Restart:  func() { restartSelf(stats) },
	}
	hs := &http.Server{Addr: cfg.Listen, Handler: srv.Handler(), ReadHeaderTimeout: 15 * time.Second}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = hs.Shutdown(c)
	}()
	log.Printf("PrismNet %s 监听 %s，核心 %s，面板目录 %s", version, cfg.Listen, cfg.Mihomo.Controller, cfg.UIDir)
	if err := hs.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
	stats.Flush()
}

// stopCmd 由重启命令推出停止命令（看门狗切换直连时停止核心，TUN / 透明代理路由随之撤销）。
func stopCmd(restart string) string {
	if n := len(restart); n > 8 && restart[n-8:] == " restart" {
		return restart[:n-8] + " stop"
	}
	return ""
}

func adblockDaily(ctx context.Context, ab *adblock.Manager) {
	for {
		st := ab.State()
		wait := 24 * time.Hour
		if st.Updated == 0 {
			wait = 2 * time.Minute
		} else if d := time.Until(time.Unix(st.Updated, 0).Add(24 * time.Hour)); d > time.Minute {
			wait = d
		} else {
			wait = time.Minute
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if ab.State().Enabled || ab.State().Updated == 0 {
			c, cancel := context.WithTimeout(ctx, 5*time.Minute)
			if err := ab.Update(c); err != nil {
				log.Printf("广告拦截列表更新失败：%v", err)
			}
			cancel()
		}
	}
}

// restartSelf 在线更新完成后用新程序替换当前进程（PID 不变，服务管理器无感知）。
func restartSelf(stats *logstat.Stats) {
	stats.Flush()
	exe, err := os.Executable()
	if err != nil {
		log.Printf("重启失败：%v", err)
		os.Exit(1)
	}
	exe, _ = filepath.EvalSymlinks(exe)
	if err := syscall.Exec(exe, os.Args, os.Environ()); err != nil {
		log.Printf("重启失败：%v，退出交由服务管理器拉起", err)
		os.Exit(1)
	}
}
