// Package api 组装 PrismNet 后端的 HTTP 路由：面板静态文件、核心 API 反向代理、扩展 API。
package api

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Skycnhe/prismnet/server/internal/adblock"
	"github.com/Skycnhe/prismnet/server/internal/config"
	"github.com/Skycnhe/prismnet/server/internal/ipinfo"
	"github.com/Skycnhe/prismnet/server/internal/logstat"
	"github.com/Skycnhe/prismnet/server/internal/mihomo"
	"github.com/Skycnhe/prismnet/server/internal/sites"
	"github.com/Skycnhe/prismnet/server/internal/update"
	"github.com/Skycnhe/prismnet/server/internal/watchdog"
)

// CorePrefix 核心 API 的反向代理前缀；扩展 API 挂在 CorePrefix+"/prism" 下，
// 这样面板只需知道一个后端地址（同一个密钥）就能同时访问两者。
const CorePrefix = "/mihomo"

// Server 后端各组件。
type Server struct {
	Cfg      *config.Config
	Version  string
	Core     *mihomo.Client
	Sites    *sites.Tester
	IP       *ipinfo.Service
	Stats    *logstat.Stats
	Adblock  *adblock.Manager
	Watchdog *watchdog.Watchdog
	Updater  *update.Updater
	Restart  func() // 更新完成后重启自身
	RunCmd   func(ctx context.Context, cmd string) error
}

// Handler 返回完整路由。
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	prism := http.NewServeMux()
	s.routes(prism)
	core := s.Core.ReverseProxy(CorePrefix)

	mux.Handle(CorePrefix+"/prism/", s.auth(http.StripPrefix(CorePrefix, prism)))
	mux.Handle("/api/prism/", s.auth(http.StripPrefix("/api", prism)))
	mux.Handle(CorePrefix+"/", s.auth(core))
	mux.Handle(CorePrefix, s.auth(core))
	mux.HandleFunc("/config.js", s.configJS)
	mux.Handle("/", s.static())
	return mux
}

func (s *Server) routes(m *http.ServeMux) {
	m.HandleFunc("GET /prism/info", s.info)
	m.HandleFunc("GET /prism/sites", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.Sites.Get(r.Context(), r.URL.Query().Get("force") == "1"))
	})
	m.HandleFunc("GET /prism/ipinfo", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, s.IP.Get(r.Context(), r.URL.Query().Get("force") == "1"))
	})
	m.HandleFunc("GET /prism/stats", func(w http.ResponseWriter, r *http.Request) {
		st := s.Adblock.State()
		writeJSON(w, map[string]any{
			"stats":   s.Stats.Report(),
			"adblock": map[string]any{"enabled": st.Enabled, "rules": st.Rules, "updated": st.Updated},
		})
	})
	m.HandleFunc("GET /prism/adblock", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Adblock.State()) })
	m.HandleFunc("PUT /prism/adblock", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Lists []adblock.List `json:"lists"`
			White []string       `json:"white"`
			Black []string       `json:"black"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		if err := s.Adblock.SetLists(b.Lists, b.White, b.Black); err != nil {
			writeErr(w, http.StatusBadRequest, err)
			return
		}
		if err := s.Adblock.Compose(r.Context()); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, s.Adblock.State())
	})
	m.HandleFunc("POST /prism/adblock/update", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		if err := s.Adblock.Update(ctx); err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, s.Adblock.State())
	})
	m.HandleFunc("POST /prism/adblock/toggle", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Enabled bool `json:"enabled"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		s.Watchdog.Pause(30 * time.Second)
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Minute)
		defer cancel()
		if b.Enabled && s.Adblock.State().Rules == 0 {
			_ = s.Adblock.Update(ctx)
		}
		if err := s.Adblock.SetEnabled(ctx, b.Enabled); err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, s.Adblock.State())
	})
	m.HandleFunc("GET /prism/watchdog", func(w http.ResponseWriter, r *http.Request) { writeJSON(w, s.Watchdog.Status()) })
	m.HandleFunc("POST /prism/watchdog", func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Enabled bool `json:"enabled"`
		}
		if !readJSON(w, r, &b) {
			return
		}
		s.Watchdog.SetEnabled(b.Enabled)
		s.Cfg.Watchdog.Enabled = b.Enabled
		_ = s.Cfg.Save()
		writeJSON(w, s.Watchdog.Status())
	})
	m.HandleFunc("POST /prism/core/restart", func(w http.ResponseWriter, r *http.Request) {
		s.Watchdog.Pause(45 * time.Second)
		ctx, cancel := context.WithTimeout(r.Context(), time.Minute)
		defer cancel()
		if err := s.RunCmd(ctx, s.Cfg.Watchdog.RestartCmd); err != nil {
			writeErr(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
	})
	m.HandleFunc("GET /prism/update/check", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()
		writeJSON(w, s.Updater.CheckUpdate(ctx))
	})
	m.HandleFunc("POST /prism/update/apply", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
		defer cancel()
		tag, err := s.Updater.Apply(ctx)
		if err != nil {
			writeErr(w, http.StatusBadGateway, err)
			return
		}
		writeJSON(w, map[string]any{"ok": true, "version": tag, "restarting": true})
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		if s.Restart != nil {
			go func() {
				time.Sleep(800 * time.Millisecond)
				s.Restart()
			}()
		}
	})
}

func (s *Server) info(w http.ResponseWriter, r *http.Request) {
	ver, err := s.Core.Alive(r.Context())
	core := map[string]any{"alive": err == nil, "version": ver, "service": s.Cfg.Mihomo.Service, "config_path": s.Cfg.Mihomo.ConfigPath}
	if err != nil {
		core["error"] = err.Error()
	}
	writeJSON(w, map[string]any{
		"name":     "PrismNet",
		"version":  s.Version,
		"repo":     s.Cfg.Update.Repo,
		"core":     core,
		"features": []string{"sites", "ipinfo", "stats", "adblock", "watchdog", "update", "core-restart"},
	})
}

// auth 面板密钥校验（Bearer 头或 ?token=，与 mihomo 外部控制器相同的方式）；未设置密钥时放行。
func (s *Server) auth(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.Cfg.Secret != "" {
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if tok == "" || tok == r.Header.Get("Authorization") {
				tok = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(tok), []byte(s.Cfg.Secret)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = io.WriteString(w, `{"message":"Unauthorized"}`)
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// configJS 让面板默认连接本后端的核心代理地址（同源），首次打开无需手动填写。
func (s *Server) configJS(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, "window.__METACUBEXD_CONFIG__ = { defaultBackendURL: window.location.origin + '"+CorePrefix+"', githubToken: '', prismnet: { version: "+jsString(s.Version)+", secretRequired: "+boolJS(s.Cfg.Secret != "")+" } }\n")
}

func jsString(v string) string { b, _ := json.Marshal(v); return string(b) }

func boolJS(b bool) string {
	if b {
		return "true"
	}
	return "false"
}

// static 面板静态文件；找不到的路径回退到 index.html（hash 路由下极少用到）。
func (s *Server) static() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		dir := s.Cfg.UIDir
		p := filepath.Clean("/" + r.URL.Path)
		if strings.HasPrefix(p, "/api/") { // 未知 API（如 metacubexd 的 /api/control 探测）不回退到 index.html
			http.NotFound(w, r)
			return
		}
		full := filepath.Join(dir, p)
		fi, err := os.Stat(full)
		if err == nil && fi.IsDir() {
			full = filepath.Join(full, "index.html")
			fi, err = os.Stat(full)
		}
		if err != nil {
			if filepath.Ext(p) != "" && p != "/index.html" {
				http.NotFound(w, r)
				return
			}
			full = filepath.Join(dir, "index.html")
			if fi, err = os.Stat(full); err != nil {
				http.Error(w, "面板文件不存在："+dir, http.StatusNotFound)
				return
			}
		}
		switch {
		case strings.Contains(p, "/_nuxt/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case strings.HasSuffix(full, ".html"), strings.HasSuffix(full, "sw.js"), strings.HasSuffix(full, ".webmanifest"):
			w.Header().Set("Cache-Control", "no-cache")
		}
		f, err := os.Open(full)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer f.Close()
		http.ServeContent(w, r, fi.Name(), fi.ModTime(), f)
	})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, err error) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"message": err.Error()})
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(io.LimitReader(r.Body, 4<<20)).Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, err)
		return false
	}
	return true
}

// CoreOps 用核心 API 实现 adblock.Core。
type CoreOps struct{ C *mihomo.Client }

// ReloadConfig 让核心按配置文件路径热重载。
func (o CoreOps) ReloadConfig(ctx context.Context, path string) error {
	code, raw, err := o.C.Do(ctx, http.MethodPut, "/configs?force=true", map[string]string{"path": path, "payload": ""})
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return &httpErr{code, string(raw)}
	}
	return nil
}

// RefreshRuleProvider 让核心重新读取规则集文件。
func (o CoreOps) RefreshRuleProvider(ctx context.Context, name string) error {
	code, raw, err := o.C.Do(ctx, http.MethodPut, "/providers/rules/"+url.PathEscape(name), nil)
	if err != nil {
		return err
	}
	if code/100 != 2 {
		return &httpErr{code, string(raw)}
	}
	return nil
}

type httpErr struct {
	code int
	body string
}

func (e *httpErr) Error() string {
	b := strings.TrimSpace(e.body)
	if len(b) > 200 {
		b = b[:200]
	}
	return "HTTP " + http.StatusText(e.code) + " " + b
}
