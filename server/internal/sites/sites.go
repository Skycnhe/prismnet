// Package sites 测量概览「站点延迟」：经核心 mixed 端口先建立连接并完成首个请求
// （DNS、TCP、TLS 握手不计时），再在同一连接上请求一次，只计这一次的往返——
// 与 mihomo unified-delay 的口径一致，也接近浏览器复用连接时的实际体验。
package sites

import (
	"context"
	"io"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/Skycnhe/prismnet/server/internal/config"
)

// HistLen 每个站点保留的历史条数。
const HistLen = 30

// Result 单个站点的结果；Ms < 0 表示超时 / 失败。
type Result struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Icon string `json:"icon"`
	Ms   int    `json:"ms"`
	Hist []int  `json:"hist"`
}

// Prober 抽象「经代理出站」的客户端工厂，便于测试。
type Prober interface {
	ProxyHTTP(timeout time.Duration, keepAlive bool) *http.Client
}

// Tester 站点延迟测试器（带缓存与历史）。
type Tester struct {
	P       Prober
	Sites   func() []config.Site
	Timeout time.Duration
	MaxAge  time.Duration

	mu      sync.Mutex
	hist    map[string][]int
	last    []Result
	lastAt  time.Time
	running sync.Mutex
}

// Get 返回最近结果；超过 MaxAge 或 force 时重新测量。并发调用共享同一次测量。
func (t *Tester) Get(ctx context.Context, force bool) []Result {
	if r, ok := t.cached(force); ok {
		return r
	}
	start := time.Now()
	t.running.Lock()
	defer t.running.Unlock()
	// 等锁期间别的请求刚测完：直接复用（force 时只复用等锁之后才产生的结果）
	t.mu.Lock()
	fresh := t.last != nil && t.lastAt.After(start)
	r := t.last
	t.mu.Unlock()
	if fresh {
		return r
	}
	if r, ok := t.cached(force); ok {
		return r
	}
	out := t.measure(ctx)
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.hist == nil {
		t.hist = map[string][]int{}
	}
	names := map[string]bool{}
	for i := range out {
		names[out[i].Name] = true
		h := append(t.hist[out[i].Name], out[i].Ms)
		if len(h) > HistLen {
			h = h[len(h)-HistLen:]
		}
		t.hist[out[i].Name] = h
		out[i].Hist = append([]int(nil), h...)
	}
	for k := range t.hist {
		if !names[k] {
			delete(t.hist, k)
		}
	}
	t.last, t.lastAt = out, time.Now()
	return out
}

func (t *Tester) cached(force bool) ([]Result, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	maxAge := t.MaxAge
	if maxAge <= 0 {
		maxAge = 20 * time.Second
	}
	if !force && t.last != nil && time.Since(t.lastAt) < maxAge {
		return t.last, true
	}
	return nil, false
}

type warm struct {
	cl    *http.Client
	ok    bool
	first int
}

// measure 建连与首个请求并发进行；计时的第二次请求逐个进行，避免其它连接的 TLS 握手占满 CPU 拖慢计时。
func (t *Tester) measure(ctx context.Context) []Result {
	list := t.Sites()
	to := t.Timeout
	if to <= 0 {
		to = 6 * time.Second
	}
	ws := make([]warm, len(list))
	var wg sync.WaitGroup
	for i, s := range list {
		wg.Add(1)
		go func(i int, s config.Site) {
			defer wg.Done()
			cl := t.P.ProxyHTTP(to, true)
			st := time.Now()
			if err := head(ctx, cl, s.URL, nil); err != nil {
				cl.CloseIdleConnections()
				return
			}
			ws[i] = warm{cl: cl, ok: true, first: int(time.Since(st).Milliseconds())}
		}(i, s)
	}
	wg.Wait()
	out := make([]Result, len(list))
	for i, s := range list {
		out[i] = Result{Name: s.Name, URL: s.URL, Icon: s.Icon, Ms: -1}
		if !ws[i].ok {
			continue
		}
		reused := false
		trace := &httptrace.ClientTrace{GotConn: func(ci httptrace.GotConnInfo) { reused = ci.Reused }}
		st := time.Now()
		err := head(ctx, ws[i].cl, s.URL, trace)
		ms := int(time.Since(st).Milliseconds())
		ws[i].cl.CloseIdleConnections()
		switch {
		case err == nil && reused:
			out[i].Ms = max(1, ms)
		case err == nil:
			out[i].Ms = max(1, ms) // 连接未复用：新连接整体耗时
		default:
			out[i].Ms = max(1, ws[i].first) // 第二次失败时退回第一次的总耗时
		}
	}
	return out
}

func head(ctx context.Context, cl *http.Client, url string, trace *httptrace.ClientTrace) error {
	if trace != nil {
		ctx = httptrace.WithClientTrace(ctx, trace)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "curl/8")
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}
