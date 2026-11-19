package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Skycnhe/prismnet/server/internal/adblock"
	"github.com/Skycnhe/prismnet/server/internal/config"
	"github.com/Skycnhe/prismnet/server/internal/logstat"
	"github.com/Skycnhe/prismnet/server/internal/mihomo"
	"github.com/Skycnhe/prismnet/server/internal/update"
	"github.com/Skycnhe/prismnet/server/internal/watchdog"
)

func newTestServer(t *testing.T, secret string) (*httptest.Server, *string) {
	var gotAuth string
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		if r.URL.Path == "/version" {
			_, _ = io.WriteString(w, `{"version":"v1.19.0","meta":true}`)
			return
		}
		_, _ = io.WriteString(w, `{"path":"`+r.URL.Path+`"}`)
	}))
	t.Cleanup(core.Close)
	dir := t.TempDir()
	ui := filepath.Join(dir, "ui")
	_ = os.MkdirAll(ui, 0o755)
	_ = os.WriteFile(filepath.Join(ui, "index.html"), []byte("<html>panel</html>"), 0o644)
	cfg := config.Default()
	cfg.UIDir, cfg.Secret, cfg.Mihomo.Controller, cfg.Mihomo.Secret = ui, secret, core.URL, "coresecret"
	c, _ := mihomo.New(core.URL, "coresecret", 7890)
	ab := &adblock.Manager{File: filepath.Join(dir, "ab.json")}
	ab.Load()
	s := &Server{Cfg: cfg, Version: "v0.0.1", Core: c, Stats: &logstat.Stats{}, Adblock: ab,
		Watchdog: watchdog.New(func(context.Context) (string, error) { return "", nil }, "", watchdog.Options{}, true),
		Updater:  &update.Updater{}}
	s.Stats.Load()
	srv := httptest.NewServer(s.Handler())
	t.Cleanup(srv.Close)
	return srv, &gotAuth
}

func get(t *testing.T, url, tok string) (int, string) {
	req, _ := http.NewRequest("GET", url, nil)
	if tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func TestRoutes(t *testing.T) {
	srv, gotAuth := newTestServer(t, "panel")
	if code, body := get(t, srv.URL+"/", ""); code != 200 || !strings.Contains(body, "panel") {
		t.Fatal(code, body)
	}
	if code, _ := get(t, srv.URL+"/api/control/info", ""); code != 404 {
		t.Fatal(code)
	}
	if code, body := get(t, srv.URL+"/config.js", ""); code != 200 || !strings.Contains(body, "/mihomo") {
		t.Fatal(code, body)
	}
	if code, _ := get(t, srv.URL+"/mihomo/proxies", ""); code != 401 {
		t.Fatal("expected 401", code)
	}
	code, body := get(t, srv.URL+"/mihomo/proxies", "panel")
	if code != 200 || !strings.Contains(body, `"/proxies"`) || *gotAuth != "Bearer coresecret" {
		t.Fatal(code, body, *gotAuth)
	}
	if code, body := get(t, srv.URL+"/mihomo/prism/info", "panel"); code != 200 || !strings.Contains(body, `"alive":true`) {
		t.Fatal(code, body)
	}
	if code, body := get(t, srv.URL+"/mihomo/prism/stats?token=panel", ""); code != 200 || !strings.Contains(body, `"adblock"`) {
		t.Fatal(code, body)
	}
}
