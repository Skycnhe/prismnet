package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{{"v1.2.0", "v1.1.9", true}, {"v1.2.0", "1.2.0", false}, {"v0.1.0", "v0.2.0", false}, {"v1.0.0", "dev", true}}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%s,%s)=%v", c.a, c.b, got)
		}
	}
}

func tgz(files map[string]string) []byte {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for n, c := range files {
		_ = tw.WriteHeader(&tar.Header{Name: n, Mode: 0o755, Size: int64(len(c)), Typeflag: tar.TypeReg})
		_, _ = tw.Write([]byte(c))
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestExtractRejectsTraversal(t *testing.T) {
	if err := Extract(tgz(map[string]string{"../evil": "x"}), t.TempDir()); err == nil {
		t.Fatal("expected error")
	}
}

func TestApply(t *testing.T) {
	pkg := tgz(map[string]string{"prismnet": "NEWBIN", "ui/index.html": "<html>new</html>"})
	sum := sha256.Sum256(pkg)
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/releases/latest":
			_ = json.NewEncoder(w).Encode(Release{Tag: "v9.0.0", Assets: []Asset{
				{Name: AssetName(), URL: srv.URL + "/pkg"},
				{Name: "sha256sums.txt", URL: srv.URL + "/sums"},
			}})
		case "/pkg":
			_, _ = w.Write(pkg)
		case "/sums":
			fmt.Fprintf(w, "%s  %s\n", hex.EncodeToString(sum[:]), AssetName())
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	exe := filepath.Join(dir, "bin", "prismnet")
	_ = os.MkdirAll(filepath.Dir(exe), 0o755)
	_ = os.WriteFile(exe, []byte("OLDBIN"), 0o755)
	ui := filepath.Join(dir, "ui")
	_ = os.MkdirAll(ui, 0o755)
	_ = os.WriteFile(filepath.Join(ui, "index.html"), []byte("old"), 0o644)
	u := &Updater{Repo: "o/r", Current: "v1.0.0", UIDir: ui, WorkDir: filepath.Join(dir, "work"), Exe: exe, APIBase: srv.URL}
	if c := u.CheckUpdate(context.Background()); !c.HasUpdate || c.Latest != "v9.0.0" {
		t.Fatalf("%+v", c)
	}
	tag, err := u.Apply(context.Background())
	if err != nil || tag != "v9.0.0" {
		t.Fatal(tag, err)
	}
	if b, _ := os.ReadFile(exe); string(b) != "NEWBIN" {
		t.Fatal(string(b))
	}
	if b, _ := os.ReadFile(exe + ".bak"); string(b) != "OLDBIN" {
		t.Fatal(string(b))
	}
	if b, _ := os.ReadFile(filepath.Join(ui, "index.html")); string(b) != "<html>new</html>" {
		t.Fatal(string(b))
	}
}
