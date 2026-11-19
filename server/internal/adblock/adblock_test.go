package adblock

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseFormats(t *testing.T) {
	in := `! comment
# comment
[Adblock Plus 2.0]
||ads.example.com^
||tracker.example.org^$important
||bad.example.net^$script
||path.example.com/ads.js
@@||good.example.com^
0.0.0.0 hosts.example.com
127.0.0.1 localhost
plain.example.com
+.clash.example.com
DOMAIN-SUFFIX,suffix.example.com
address=/dnsmasq.example.com/0.0.0.0
not a domain
`
	b, a := Parse(strings.NewReader(in))
	for _, d := range []string{"ads.example.com", "tracker.example.org", "hosts.example.com", "plain.example.com", "clash.example.com", "suffix.example.com", "dnsmasq.example.com"} {
		if !b[d] {
			t.Errorf("missing block %s", d)
		}
	}
	for _, d := range []string{"bad.example.net", "path.example.com", "localhost"} {
		if b[d] {
			t.Errorf("unexpected block %s", d)
		}
	}
	if !a["good.example.com"] {
		t.Error("missing allow")
	}
}

func TestCompact(t *testing.T) {
	got := Compact(map[string]bool{"a.com": true, "x.a.com": true, "b.org": true})
	if strings.Join(got, ",") != "a.com,b.org" {
		t.Fatal(got)
	}
}

func TestInjectRoundTrip(t *testing.T) {
	src := []byte("# my config\nmixed-port: 7890\nrules:\n  - GEOIP,CN,DIRECT\n  - MATCH,Proxy\n")
	out, changed, err := Inject(src, true)
	if err != nil || !changed {
		t.Fatal(err, changed)
	}
	s := string(out)
	if !strings.Contains(s, "# my config") || !strings.Contains(s, BlockSet+":") || !strings.Contains(s, "- "+Rule) {
		t.Fatal(s)
	}
	if strings.Index(s, Rule) > strings.Index(s, "GEOIP,CN,DIRECT") {
		t.Fatal("rule not first")
	}
	out2, changed2, _ := Inject(out, true)
	if changed2 || string(out2) != s {
		t.Fatal("second inject should be no-op")
	}
	back, changed3, _ := Inject(out, false)
	if !changed3 || strings.Contains(string(back), "prism-ad") {
		t.Fatal(string(back))
	}
}

type fakeCore struct{ reloads int }

func (f *fakeCore) ReloadConfig(context.Context, string) error        { f.reloads++; return nil }
func (f *fakeCore) RefreshRuleProvider(context.Context, string) error { return nil }

func TestManagerUpdateAndToggle(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, "mihomo", "config.yaml")
	_ = os.MkdirAll(filepath.Dir(cfg), 0o755)
	_ = os.WriteFile(cfg, []byte("rules:\n  - MATCH,DIRECT\n"), 0o644)
	fc := &fakeCore{}
	m := &Manager{File: filepath.Join(dir, "ab.json"), CacheDir: filepath.Join(dir, "cache"), ConfigPath: cfg, Core: fc,
		Fetch: func(context.Context, string) ([]byte, error) {
			return []byte("ads.a.com\nx.ads.a.com\nkeep.b.com\n"), nil
		}}
	m.Load()
	if err := m.SetLists([]List{{Name: "t", URL: "https://x/list", Enabled: true}}, []string{"keep.b.com"}, []string{"+.black.c.com"}); err != nil {
		t.Fatal(err)
	}
	if err := m.Update(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(m.RuleDir(), BlockSet+".txt"))
	if string(raw) != "+.ads.a.com\n+.black.c.com\n" {
		t.Fatalf("%q", raw)
	}
	if m.State().Rules != 2 {
		t.Fatal(m.State().Rules)
	}
	if err := m.SetEnabled(context.Background(), true); err != nil || fc.reloads != 1 {
		t.Fatal(err, fc.reloads)
	}
	c, _ := os.ReadFile(cfg)
	if !strings.Contains(string(c), Rule) {
		t.Fatal(string(c))
	}
	if err := m.SetEnabled(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	c, _ = os.ReadFile(cfg)
	if strings.Contains(string(c), "prism-ad") {
		t.Fatal(string(c))
	}
}
