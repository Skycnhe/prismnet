package watchdog

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func TestWatchdogFlow(t *testing.T) {
	alive := true
	var cmds []string
	w := New(func(context.Context) (string, error) {
		if alive {
			return "v1", nil
		}
		return "", errors.New("timeout")
	}, filepath.Join(t.TempDir(), "wd.json"), Options{Interval: time.Second, Timeout: time.Second, Threshold: 2, RestartCmd: "restart", BypassCmd: "bypass", RestoreCmd: "restore"}, true)
	w.Run = func(_ context.Context, c string) error { cmds = append(cmds, c); return nil }
	ctx := context.Background()
	w.Tick(ctx)
	if w.Status().State != "ok" {
		t.Fatal(w.Status().State)
	}
	alive = false
	w.Tick(ctx) // 1st failure → restart
	w.Tick(ctx) // 2nd failure → bypass
	if st := w.Status(); st.State != "bypass" || st.Counts.Restart != 1 || st.Counts.Bypass != 1 {
		t.Fatalf("%+v", st)
	}
	alive = true
	w.Tick(ctx)
	st := w.Status()
	if st.State != "ok" || st.Counts.Recover != 1 {
		t.Fatalf("%+v", st)
	}
	want := []string{"restart", "bypass", "restore"}
	if len(cmds) != 3 || cmds[0] != want[0] || cmds[1] != want[1] || cmds[2] != want[2] {
		t.Fatal(cmds)
	}
}

func TestEventsCapped(t *testing.T) {
	w := New(func(context.Context) (string, error) { return "", errors.New("x") }, "", Options{Interval: time.Second, Timeout: time.Second, Threshold: 1000}, true)
	w.Run = func(context.Context, string) error { return nil }
	for i := 0; i < 30; i++ {
		w.Tick(context.Background())
	}
	if n := len(w.Status().Events); n != MaxEvents {
		t.Fatal(n)
	}
}
