// Package logstat 订阅 mihomo 日志流（debug 级别），统计：
//   - 每小时的连接请求数、广告拦截数（REJECT 命中广告规则）、被拦截域名排行；
//   - 最近 100 条 DNS 查询的来源（缓存命中 / 上游应答 / 其它）；
//   - 定时经核心解析器查询随机子域名（绕过缓存）得到的平均解析耗时。
package logstat

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Skycnhe/prismnet/server/internal/mihomo"
	"github.com/Skycnhe/prismnet/server/internal/store"
)

// DNSRecent 最近多少条 DNS 查询参与「DNS 查询结果」统计。
const DNSRecent = 100

var (
	rxConn   = regexp.MustCompile(`--> (\S+?):\d+ (?:match|doesn't match)`)
	rxReject = regexp.MustCompile(`--> (\S+?):\d+ match (.*?) using (REJECT(?:-DROP)?)\b`)
	rxAdRule = regexp.MustCompile(`(?i)(prism-ad|adblock|anti-?ad|category-ads|reject|\bads?\b|广告)`)
	fakeNets = mustNets("198.18.0.0/15", "fc00::/18")
)

func mustNets(cidrs ...string) []*net.IPNet {
	var out []*net.IPNet
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}

// Hour 一个小时段的统计。
type Hour struct {
	Req   int     `json:"req"`
	Blk   int     `json:"blk"`
	DNSms float64 `json:"dns_ms"`
	DNSn  int     `json:"dns_n"`
}

// Day 一天的拦截统计。
type Day struct {
	Count int            `json:"count"`
	Top   map[string]int `json:"top"`
}

type persisted struct {
	Hours map[string]*Hour      `json:"hours"` // "2006-01-02 15"
	Days  map[string]*Day       `json:"days"`  // "2006-01-02"
	DNS5  map[string][2]float64 `json:"dns5"`  // "2006-01-02 15:04"（5 分钟段）→ [总耗时, 次数]
}

// Stats 日志统计器。
type Stats struct {
	C    *mihomo.Client
	File string
	Now  func() time.Time

	mu        sync.Mutex
	data      persisted
	dnsKinds  []string // 最近 DNS 查询的来源：cache / upstream / other
	dnsRecent []float64
	saved     time.Time
	connected bool
}

func (s *Stats) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Load 读取落盘数据。
func (s *Stats) Load() {
	s.mu.Lock()
	defer s.mu.Unlock()
	store.Read(s.File, &s.data)
	if s.data.Hours == nil {
		s.data.Hours = map[string]*Hour{}
	}
	if s.data.Days == nil {
		s.data.Days = map[string]*Day{}
	}
	if s.data.DNS5 == nil {
		s.data.DNS5 = map[string][2]float64{}
	}
}

func (s *Stats) hourLocked() *Hour {
	k := s.now().Format("2006-01-02 15")
	h := s.data.Hours[k]
	if h == nil {
		h = &Hour{}
		s.data.Hours[k] = h
	}
	return h
}

func (s *Stats) dayLocked() *Day {
	k := s.now().Format("2006-01-02")
	d := s.data.Days[k]
	if d == nil {
		d = &Day{Top: map[string]int{}}
		s.data.Days[k] = d
	}
	if d.Top == nil {
		d.Top = map[string]int{}
	}
	return d
}

func isFakeOrIP(host string) bool {
	ip := net.ParseIP(strings.Trim(host, "[]"))
	if ip == nil {
		return false
	}
	for _, n := range fakeNets {
		if n.Contains(ip) {
			return true
		}
	}
	return true // 纯 IP 不进域名排行
}

// Feed 处理一行日志（mihomo 日志流的一行 JSON：{"type":"info","payload":"..."}）。
func (s *Stats) Feed(line []byte) {
	var m struct {
		Type    string `json:"type"`
		Payload string `json:"payload"`
	}
	if json.Unmarshal(line, &m) != nil {
		return
	}
	s.FeedPayload(m.Payload)
}

// FeedPayload 处理一条日志正文。
func (s *Stats) FeedPayload(p string) {
	switch {
	case strings.Contains(p, "[DNS]"):
		kind := ""
		switch {
		case strings.Contains(p, "cache hit"):
			kind = "cache"
		case strings.Contains(p, "-->"):
			kind = "upstream"
		case strings.Contains(p, "hosts") || strings.Contains(p, "fake"):
			kind = "other"
		}
		if kind == "" {
			return
		}
		s.mu.Lock()
		s.dnsKinds = append(s.dnsKinds, kind)
		if len(s.dnsKinds) > DNSRecent {
			s.dnsKinds = s.dnsKinds[len(s.dnsKinds)-DNSRecent:]
		}
		s.mu.Unlock()
	case strings.Contains(p, "-->"):
		if !rxConn.MatchString(p) {
			return
		}
		s.mu.Lock()
		s.hourLocked().Req++
		if strings.Contains(p, "using REJECT") {
			if m := rxReject.FindStringSubmatch(p); m != nil && rxAdRule.MatchString(m[2]) {
				s.hourLocked().Blk++
				d := s.dayLocked()
				d.Count++
				host := strings.Trim(m[1], "[]")
				if !isFakeOrIP(host) {
					d.Top[host]++
					if len(d.Top) > 5000 {
						trimTop(d.Top, 2000)
					}
				}
			}
		}
		s.mu.Unlock()
	}
	if time.Since(s.saved) > time.Minute {
		s.Flush()
	}
}

func trimTop(m map[string]int, keep int) {
	type kv struct {
		k string
		v int
	}
	all := make([]kv, 0, len(m))
	for k, v := range m {
		all = append(all, kv{k, v})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].v > all[j].v })
	for i := keep; i < len(all); i++ {
		delete(m, all[i].k)
	}
}

// DNSSample 记录一次解析耗时（毫秒）。
func (s *Stats) DNSSample(ms float64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	h := s.hourLocked()
	h.DNSms += ms
	h.DNSn++
	t := s.now()
	k := t.Format("2006-01-02 15:") + twoDigit(t.Minute()/5*5)
	b := s.data.DNS5[k]
	b[0] += ms
	b[1]++
	s.data.DNS5[k] = b
	s.dnsRecent = append(s.dnsRecent, ms)
	if len(s.dnsRecent) > 60 {
		s.dnsRecent = s.dnsRecent[len(s.dnsRecent)-60:]
	}
}

func twoDigit(n int) string {
	if n < 10 {
		return "0" + string(rune('0'+n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

// Flush 修剪并落盘。
func (s *Stats) Flush() {
	s.mu.Lock()
	pruneKeys(s.data.Hours, 48)
	pruneKeys(s.data.Days, 14)
	pruneKeys(s.data.DNS5, 288)
	for _, d := range s.data.Days {
		if len(d.Top) > 500 {
			trimTop(d.Top, 500)
		}
	}
	raw, _ := json.Marshal(s.data)
	s.saved = time.Now()
	s.mu.Unlock()
	var cp persisted
	if json.Unmarshal(raw, &cp) == nil && s.File != "" {
		_ = store.Write(s.File, cp)
	}
}

func pruneKeys[V any](m map[string]V, keep int) {
	if len(m) <= keep {
		return
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys[:len(keys)-keep] {
		delete(m, k)
	}
}

// HourPoint 报表里的一个小时点。
type HourPoint struct {
	H   string   `json:"h"`
	Req int      `json:"req"`
	Blk int      `json:"blk"`
	DNS *float64 `json:"dns"`
}

// TopItem 被拦截域名排行的一项。
type TopItem struct {
	Domain string `json:"domain"`
	Count  int    `json:"count"`
}

// Report 「域名过滤」与「DNS 查询结果」卡片的数据。
type Report struct {
	Connected bool        `json:"connected"`
	Today     int         `json:"today"`
	Req24     int         `json:"req24"`
	Blk24     int         `json:"blk24"`
	Hours     []HourPoint `json:"hours"`
	Top       []TopItem   `json:"top"`
	Days      []struct {
		Date  string `json:"date"`
		Count int    `json:"count"`
	} `json:"days"`
	DNS struct {
		Avg     *float64   `json:"avg"`
		Samples int        `json:"samples"`
		Series  []*float64 `json:"series"`
	} `json:"dns"`
	DNSResult struct {
		Total    int `json:"total"`
		Cache    int `json:"cache"`
		Upstream int `json:"upstream"`
		Other    int `json:"other"`
	} `json:"dns_result"`
}

// Report 生成报表。
func (s *Stats) Report() Report {
	s.mu.Lock()
	defer s.mu.Unlock()
	var r Report
	r.Connected = s.connected
	now := s.now()
	day := s.data.Days[now.Format("2006-01-02")]
	if day != nil {
		r.Today = day.Count
		for k, v := range day.Top {
			r.Top = append(r.Top, TopItem{k, v})
		}
		sort.Slice(r.Top, func(i, j int) bool {
			if r.Top[i].Count != r.Top[j].Count {
				return r.Top[i].Count > r.Top[j].Count
			}
			return r.Top[i].Domain < r.Top[j].Domain
		})
		if len(r.Top) > 20 {
			r.Top = r.Top[:20]
		}
	}
	if r.Top == nil {
		r.Top = []TopItem{}
	}
	base := now.Truncate(time.Hour)
	var dnsT float64
	var dnsN int
	for i := 23; i >= 0; i-- {
		t := base.Add(-time.Duration(i) * time.Hour)
		h := s.data.Hours[t.Format("2006-01-02 15")]
		p := HourPoint{H: t.Format("15")}
		if h != nil {
			p.Req, p.Blk = h.Req, h.Blk
			if h.DNSn > 0 {
				v := round1(h.DNSms / float64(h.DNSn))
				p.DNS = &v
				dnsT += h.DNSms
				dnsN += h.DNSn
			}
		}
		r.Req24 += p.Req
		r.Blk24 += p.Blk
		r.Hours = append(r.Hours, p)
	}
	keys := make([]string, 0, len(s.data.Days))
	for k := range s.data.Days {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		r.Days = append(r.Days, struct {
			Date  string `json:"date"`
			Count int    `json:"count"`
		}{k, s.data.Days[k].Count})
	}
	if dnsN > 0 {
		v := round1(dnsT / float64(dnsN))
		r.DNS.Avg = &v
	}
	r.DNS.Samples = dnsN
	seg := now.Truncate(5 * time.Minute)
	valid := 0
	for i := 287; i >= 0; i-- {
		t := seg.Add(-time.Duration(i) * 5 * time.Minute)
		b, ok := s.data.DNS5[t.Format("2006-01-02 15:")+twoDigit(t.Minute()/5*5)]
		if ok && b[1] > 0 {
			v := round1(b[0] / b[1])
			r.DNS.Series = append(r.DNS.Series, &v)
			valid++
		} else {
			r.DNS.Series = append(r.DNS.Series, nil)
		}
	}
	if valid < 2 { // 刚启动：用最近的原始采样画曲线
		r.DNS.Series = nil
		for _, v := range s.dnsRecent {
			v := v
			r.DNS.Series = append(r.DNS.Series, &v)
		}
	}
	for _, k := range s.dnsKinds {
		switch k {
		case "cache":
			r.DNSResult.Cache++
		case "upstream":
			r.DNSResult.Upstream++
		default:
			r.DNSResult.Other++
		}
	}
	r.DNSResult.Total = len(s.dnsKinds)
	return r
}

func round1(v float64) float64 { return float64(int(v*10+0.5)) / 10 }

// Run 持续订阅日志流（断线 3 秒后重连）并定时做 DNS 探测，直到 ctx 取消。
func (s *Stats) Run(ctx context.Context) {
	go s.dnsLoop(ctx)
	for ctx.Err() == nil {
		s.setConnected(true)
		_ = s.C.Stream(ctx, "/logs?level=debug", s.Feed)
		s.setConnected(false)
		select {
		case <-ctx.Done():
		case <-time.After(3 * time.Second):
		}
	}
	s.Flush()
}

func (s *Stats) setConnected(v bool) {
	s.mu.Lock()
	s.connected = v
	s.mu.Unlock()
}

func (s *Stats) dnsLoop(ctx context.Context) {
	n := 0
	for {
		wait := 60 * time.Second
		if n < 3 {
			wait = 10 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		n++
		s.dnsProbe(ctx)
	}
}

// dnsProbe 经核心 /dns/query 解析随机子域名（国内 / 国外各一个，绕过缓存），取平均耗时。
func (s *Stats) dnsProbe(ctx context.Context) {
	var sum float64
	var cnt int
	for _, base := range []string{"baidu.com", "google.com"} {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		name := "prism-" + hex.EncodeToString(b) + "." + base
		c2, cancel := context.WithTimeout(ctx, 8*time.Second)
		st := time.Now()
		code, _, err := s.C.Do(c2, "GET", "/dns/query?name="+url.QueryEscape(name)+"&type=A", nil)
		cancel()
		if err == nil && code == 200 {
			sum += float64(max(1, time.Since(st).Milliseconds()))
			cnt++
		}
	}
	if cnt > 0 {
		s.DNSSample(round1(sum / float64(cnt)))
	}
}
