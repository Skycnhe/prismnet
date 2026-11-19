// Package ipinfo 查询本地公网 IP 与代理出口 IP（含归属地 / 运营商）以及出口所走的节点链。
package ipinfo

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Skycnhe/prismnet/server/internal/mihomo"
)

// Side 一侧（本地 / 代理）的 IP 信息。
type Side struct {
	IP       string `json:"ip"`
	Country  string `json:"country"`
	Region   string `json:"region"`
	City     string `json:"city"`
	ISP      string `json:"isp"`
	Location string `json:"location"` // 已拼好的展示文本，例如「中国 重庆 电信」
	Error    string `json:"error,omitempty"`
}

// Info 查询结果。
type Info struct {
	Local  Side     `json:"local"`
	Proxy  Side     `json:"proxy"`
	Chains []string `json:"chains"` // 代理出口连接的节点链（mihomo 顺序：节点在前，策略组在后）
	Rule   string   `json:"rule"`
	At     int64    `json:"at"`
}

// Service 带缓存的 IP 信息查询。
type Service struct {
	C      *mihomo.Client
	MaxAge time.Duration

	mu   sync.Mutex
	last *Info
	at   time.Time
	busy sync.Mutex
}

// Get 返回缓存（未过期）或重新查询。
func (s *Service) Get(ctx context.Context, force bool) *Info {
	maxAge := s.MaxAge
	if maxAge <= 0 {
		maxAge = 10 * time.Minute
	}
	s.mu.Lock()
	if !force && s.last != nil && time.Since(s.at) < maxAge {
		v := s.last
		s.mu.Unlock()
		return v
	}
	s.mu.Unlock()
	s.busy.Lock()
	defer s.busy.Unlock()
	info := s.query(ctx)
	s.mu.Lock()
	s.last, s.at = info, time.Now()
	s.mu.Unlock()
	return info
}

func (s *Service) query(ctx context.Context) *Info {
	info := &Info{At: time.Now().Unix()}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		info.Local = s.local(ctx)
	}()
	go func() {
		defer wg.Done()
		info.Proxy, info.Chains, info.Rule = s.proxy(ctx)
	}()
	wg.Wait()
	return info
}

// local 本地公网 IP：优先直连 myip.ipip.net（国内服务，自带归属地与运营商），失败再经代理（规则模式下国内域名走直连）。
func (s *Service) local(ctx context.Context) Side {
	for _, cl := range []*http.Client{mihomo.DirectHTTP(8 * time.Second), s.C.ProxyHTTP(8*time.Second, false)} {
		var v struct {
			Ret  string `json:"ret"`
			Data struct {
				IP       string   `json:"ip"`
				Location []string `json:"location"`
			} `json:"data"`
		}
		if err := getJSON(ctx, cl, "https://myip.ipip.net/json", &v); err != nil || v.Data.IP == "" {
			continue
		}
		sd := Side{IP: v.Data.IP}
		loc := v.Data.Location
		pick := func(i int) string {
			if i < len(loc) {
				return strings.TrimSpace(loc[i])
			}
			return ""
		}
		sd.Country, sd.Region, sd.City, sd.ISP = pick(0), pick(1), pick(2), pick(4)
		sd.Location = joinLoc(sd.Country, sd.Region, sd.City, sd.ISP)
		return sd
	}
	// 兜底：Cloudflare trace（只有 IP）+ ip-api 归属地
	ip, err := cfTrace(ctx, mihomo.DirectHTTP(8*time.Second))
	if err != nil {
		return Side{Error: err.Error()}
	}
	sd := geo(ctx, mihomo.DirectHTTP(8*time.Second), ip)
	sd.IP = ip
	return sd
}

// proxy 代理出口：经 mixed 端口取出口 IP，连接未关闭时从核心 /connections 找到这条连接的节点链。
func (s *Service) proxy(ctx context.Context) (Side, []string, string) {
	cl := s.C.ProxyHTTP(10*time.Second, true)
	defer cl.CloseIdleConnections()
	var v struct {
		IP string `json:"ip"`
	}
	if err := getJSON(ctx, cl, "https://api.ipify.org?format=json", &v); err != nil || v.IP == "" {
		ip, err2 := cfTrace(ctx, cl)
		if err2 != nil {
			if err == nil {
				err = err2
			}
			return Side{Error: err.Error()}, nil, ""
		}
		v.IP = ip
	}
	chains, rule := s.findChain(ctx, "api.ipify.org", "1.1.1.1")
	sd := geo(ctx, cl, v.IP)
	if sd.Error != "" {
		sd = geo(ctx, mihomo.DirectHTTP(8*time.Second), v.IP)
	}
	sd.IP = v.IP
	return sd, chains, rule
}

func (s *Service) findChain(ctx context.Context, hosts ...string) ([]string, string) {
	var snap struct {
		Connections []struct {
			Chains   []string `json:"chains"`
			Rule     string   `json:"rule"`
			Metadata struct {
				Host          string `json:"host"`
				DestinationIP string `json:"destinationIP"`
			} `json:"metadata"`
		} `json:"connections"`
	}
	c2, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	if err := s.C.GetJSON(c2, "/connections", &snap); err != nil {
		return nil, ""
	}
	for i := len(snap.Connections) - 1; i >= 0; i-- {
		cn := snap.Connections[i]
		for _, h := range hosts {
			if cn.Metadata.Host == h || cn.Metadata.DestinationIP == h {
				return cn.Chains, cn.Rule
			}
		}
	}
	return nil, ""
}

func geo(ctx context.Context, cl *http.Client, ip string) Side {
	var v struct {
		Status     string `json:"status"`
		Country    string `json:"country"`
		RegionName string `json:"regionName"`
		City       string `json:"city"`
		ISP        string `json:"isp"`
		Org        string `json:"org"`
	}
	err := getJSON(ctx, cl, fmt.Sprintf("http://ip-api.com/json/%s?lang=zh-CN&fields=status,country,regionName,city,isp,org", ip), &v)
	if err == nil && v.Status == "success" {
		isp := v.ISP
		if isp == "" {
			isp = v.Org
		}
		return Side{Country: v.Country, Region: v.RegionName, City: v.City, ISP: isp, Location: joinLoc(v.Country, v.RegionName, v.City, isp)}
	}
	var w struct {
		Success    bool   `json:"success"`
		Country    string `json:"country"`
		Region     string `json:"region"`
		City       string `json:"city"`
		Connection struct {
			ISP string `json:"isp"`
		} `json:"connection"`
	}
	if err2 := getJSON(ctx, cl, "https://ipwho.is/"+ip+"?lang=zh-CN", &w); err2 == nil && w.Success {
		return Side{Country: w.Country, Region: w.Region, City: w.City, ISP: w.Connection.ISP, Location: joinLoc(w.Country, w.Region, w.City, w.Connection.ISP)}
	}
	if err == nil {
		err = fmt.Errorf("geo lookup failed")
	}
	return Side{Error: err.Error()}
}

func cfTrace(ctx context.Context, cl *http.Client) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://1.1.1.1/cdn-cgi/trace", nil)
	if err != nil {
		return "", err
	}
	resp, err := cl.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	for _, ln := range strings.Split(string(raw), "\n") {
		if strings.HasPrefix(ln, "ip=") {
			return strings.TrimSpace(ln[3:]), nil
		}
	}
	return "", fmt.Errorf("no ip in trace")
}

func getJSON(ctx context.Context, cl *http.Client, url string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "curl/8")
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(v)
}

// joinLoc 拼接归属地，去掉相邻重复（如「重庆 重庆」）。
func joinLoc(parts ...string) string {
	var out []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" || (len(out) > 0 && out[len(out)-1] == p) {
			continue
		}
		out = append(out, p)
	}
	return strings.Join(out, " ")
}
