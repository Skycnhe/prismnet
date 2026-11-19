// Package adblock 广告拦截：下载过滤列表 → 合成 mihomo 规则集文件（text / domain），
// 开启时在核心配置里加入规则集与一条 REJECT 规则并热重载，关闭时移除。
package adblock

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Skycnhe/prismnet/server/internal/store"
)

// 规则集名称与规则。放行集合用逻辑规则排除：命中拦截集且不在放行集时 REJECT。
const (
	BlockSet = "prism-adblock"
	AllowSet = "prism-adallow"
	Rule     = "AND,((RULE-SET," + BlockSet + "),(NOT,((RULE-SET," + AllowSet + ")))),REJECT"
	// 规则集文件不能为空，放一个不会被访问的占位域名
	placeholder = "prismnet-placeholder.invalid"
)

// List 一个过滤列表。
type List struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Enabled bool   `json:"enabled"`
	Count   int    `json:"count"`
	Updated int64  `json:"updated"`
	Error   string `json:"error,omitempty"`
}

// State 广告拦截设置。
type State struct {
	Enabled  bool     `json:"enabled"`
	Lists    []List   `json:"lists"`
	White    []string `json:"white"`
	Black    []string `json:"black"`
	Rules    int      `json:"rules"`   // 合成后的规则条数
	Updated  int64    `json:"updated"` // 最近一次合成时间
	Injected bool     `json:"injected"`
	Error    string   `json:"error,omitempty"`
}

// DefaultLists 默认列表。
func DefaultLists() []List {
	return []List{
		{ID: "anti-ad", Name: "anti-AD", URL: "https://anti-ad.net/domains.txt", Enabled: true},
		{ID: "adguard-dns", Name: "AdGuard DNS filter", URL: "https://adguardteam.github.io/AdGuardSDNSFilter/Filters/filter.txt", Enabled: false},
	}
}

// Core 合成后需要对核心做的操作（便于测试替换）。
type Core interface {
	ReloadConfig(ctx context.Context, path string) error
	RefreshRuleProvider(ctx context.Context, name string) error
}

// Manager 广告拦截管理器。
type Manager struct {
	File       string // 设置文件
	CacheDir   string // 列表缓存目录
	ConfigPath string // mihomo 配置文件
	Core       Core
	Fetch      func(ctx context.Context, url string) ([]byte, error)

	mu sync.Mutex
	st State
}

// Load 读取设置。
func (m *Manager) Load() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !store.Read(m.File, &m.st) {
		m.st = State{Lists: DefaultLists()}
	}
	if m.st.White == nil {
		m.st.White = []string{}
	}
	if m.st.Black == nil {
		m.st.Black = []string{}
	}
}

// State 当前设置副本。
func (m *Manager) State() State {
	m.mu.Lock()
	defer m.mu.Unlock()
	st := m.st
	st.Lists = append([]List(nil), m.st.Lists...)
	st.White = append([]string{}, m.st.White...)
	st.Black = append([]string{}, m.st.Black...)
	return st
}

func (m *Manager) saveLocked() { _ = store.Write(m.File, m.st) }

// RuleDir 规则集文件目录（核心配置目录下的 prismnet/）。
func (m *Manager) RuleDir() string { return filepath.Join(filepath.Dir(m.ConfigPath), "prismnet") }

func listID(url string) string {
	h := sha1.Sum([]byte(url))
	return hex.EncodeToString(h[:4])
}

// SetLists 替换列表与自定义黑白名单（不立即下载）。
func (m *Manager) SetLists(lists []List, white, black []string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	seen := map[string]bool{}
	var out []List
	for _, l := range lists {
		l.URL = strings.TrimSpace(l.URL)
		if !strings.HasPrefix(l.URL, "http://") && !strings.HasPrefix(l.URL, "https://") {
			return fmt.Errorf("列表地址无效：%s", l.URL)
		}
		if l.ID == "" {
			l.ID = listID(l.URL)
		}
		if seen[l.ID] {
			continue
		}
		seen[l.ID] = true
		if l.Name == "" {
			l.Name = l.URL
		}
		for _, old := range m.st.Lists {
			if old.ID == l.ID {
				l.Count, l.Updated, l.Error = old.Count, old.Updated, old.Error
			}
		}
		out = append(out, l)
	}
	clean := func(in []string) []string {
		res := []string{}
		for _, s := range in {
			if d := NormDomain(strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(s), "+."), "*.")); d != "" {
				res = append(res, d)
			}
		}
		return res
	}
	m.st.Lists, m.st.White, m.st.Black = out, clean(white), clean(black)
	m.saveLocked()
	return nil
}

// Update 下载所有启用的列表（失败的沿用缓存），合成规则集；已开启时通知核心重新加载。
func (m *Manager) Update(ctx context.Context) error {
	st := m.State()
	for i, l := range st.Lists {
		if !l.Enabled {
			continue
		}
		raw, err := m.Fetch(ctx, l.URL)
		if err == nil && len(bytes.TrimSpace(raw)) == 0 {
			err = fmt.Errorf("列表为空")
		}
		if err != nil {
			st.Lists[i].Error = err.Error()
			continue
		}
		if err := os.MkdirAll(m.CacheDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(m.CacheDir, l.ID+".txt"), raw, 0o644); err != nil {
			return err
		}
		b, _ := Parse(bytes.NewReader(raw))
		st.Lists[i].Count, st.Lists[i].Updated, st.Lists[i].Error = len(b), time.Now().Unix(), ""
	}
	m.mu.Lock()
	for _, l := range st.Lists {
		for j := range m.st.Lists {
			if m.st.Lists[j].ID == l.ID {
				m.st.Lists[j].Count, m.st.Lists[j].Updated, m.st.Lists[j].Error = l.Count, l.Updated, l.Error
			}
		}
	}
	m.saveLocked()
	m.mu.Unlock()
	return m.Compose(ctx)
}

// Compose 根据缓存的列表与自定义黑白名单写出规则集文件。
func (m *Manager) Compose(ctx context.Context) error {
	st := m.State()
	block, allow := map[string]bool{}, map[string]bool{}
	for _, l := range st.Lists {
		if !l.Enabled {
			continue
		}
		f, err := os.Open(filepath.Join(m.CacheDir, l.ID+".txt"))
		if err != nil {
			continue
		}
		b, a := Parse(f)
		f.Close()
		for d := range b {
			block[d] = true
		}
		for d := range a {
			allow[d] = true
		}
	}
	for _, d := range st.Black {
		block[d] = true
	}
	for _, d := range st.White {
		allow[d] = true
	}
	for d := range allow { // 完全相同的放行直接从拦截集合去掉
		delete(block, d)
	}
	bl, al := Compact(block), Compact(allow)
	if err := writeSet(filepath.Join(m.RuleDir(), BlockSet+".txt"), bl); err != nil {
		return err
	}
	if err := writeSet(filepath.Join(m.RuleDir(), AllowSet+".txt"), al); err != nil {
		return err
	}
	m.mu.Lock()
	m.st.Rules, m.st.Updated = len(bl), time.Now().Unix()
	enabled, injected := m.st.Enabled, m.st.Injected
	m.saveLocked()
	m.mu.Unlock()
	if enabled && injected && m.Core != nil {
		_ = m.Core.RefreshRuleProvider(ctx, BlockSet)
		_ = m.Core.RefreshRuleProvider(ctx, AllowSet)
	}
	return nil
}

func writeSet(path string, list []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	if len(list) == 0 {
		buf.WriteString(placeholder + "\n")
	}
	for _, d := range list {
		buf.WriteString("+.")
		buf.WriteString(d)
		buf.WriteByte('\n')
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SetEnabled 开关广告拦截：修改核心配置（加入 / 移除规则集与规则）并热重载。
func (m *Manager) SetEnabled(ctx context.Context, on bool) error {
	if on {
		if _, err := os.Stat(filepath.Join(m.RuleDir(), BlockSet+".txt")); err != nil {
			if err := m.Compose(ctx); err != nil {
				return err
			}
		}
	}
	raw, err := os.ReadFile(m.ConfigPath)
	if err != nil {
		return fmt.Errorf("读取核心配置失败：%w", err)
	}
	out, changed, err := Inject(raw, on)
	if err != nil {
		return err
	}
	if changed {
		if err := os.WriteFile(m.ConfigPath+".prismnet.bak", raw, 0o600); err != nil {
			return err
		}
		if err := writeFileKeepMode(m.ConfigPath, out); err != nil {
			return err
		}
		if m.Core != nil {
			if err := m.Core.ReloadConfig(ctx, m.ConfigPath); err != nil {
				_ = writeFileKeepMode(m.ConfigPath, raw) // 重载失败：还原配置
				_ = m.Core.ReloadConfig(ctx, m.ConfigPath)
				return fmt.Errorf("核心重载失败，已还原配置：%w", err)
			}
		}
	}
	m.mu.Lock()
	m.st.Enabled, m.st.Injected = on, on
	m.saveLocked()
	m.mu.Unlock()
	return nil
}

func writeFileKeepMode(path string, data []byte) error {
	mode := os.FileMode(0o644)
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := path + ".prismnet.tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// HTTPFetcher 依次尝试给定的客户端下载 url。
func HTTPFetcher(clients ...*http.Client) func(ctx context.Context, url string) ([]byte, error) {
	return func(ctx context.Context, url string) ([]byte, error) {
		var last error
		for _, cl := range clients {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
			if err != nil {
				return nil, err
			}
			req.Header.Set("User-Agent", "PrismNet")
			resp, err := cl.Do(req)
			if err != nil {
				last = err
				continue
			}
			raw, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
			resp.Body.Close()
			if err == nil && resp.StatusCode != http.StatusOK {
				err = fmt.Errorf("HTTP %d", resp.StatusCode)
			}
			if err != nil {
				last = err
				continue
			}
			return raw, nil
		}
		return nil, last
	}
}
