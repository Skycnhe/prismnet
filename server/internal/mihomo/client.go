// Package mihomo 封装 mihomo 外部控制器（Clash API）的访问：普通请求、流式日志、反向代理，
// 以及经核心 mixed 端口出站的 HTTP 客户端。
package mihomo

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

// Client 访问 mihomo 外部控制器。
type Client struct {
	Base      *url.URL
	Secret    string
	MixedPort int
	http      *http.Client
}

// New 创建客户端；controller 形如 http://127.0.0.1:9090。
func New(controller, secret string, mixedPort int) (*Client, error) {
	u, err := url.Parse(strings.TrimRight(controller, "/"))
	if err != nil {
		return nil, err
	}
	if u.Scheme == "" || u.Host == "" {
		return nil, fmt.Errorf("invalid controller address %q", controller)
	}
	tr := &http.Transport{Proxy: nil, MaxIdleConns: 8, IdleConnTimeout: 60 * time.Second}
	return &Client{Base: u, Secret: secret, MixedPort: mixedPort, http: &http.Client{Transport: tr}}, nil
}

func (c *Client) newReq(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, method, c.Base.String()+path, body)
	if err != nil {
		return nil, err
	}
	if c.Secret != "" {
		req.Header.Set("Authorization", "Bearer "+c.Secret)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// Do 发送请求并返回状态码与响应体。
func (c *Client) Do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := c.newReq(ctx, method, path, rd)
	if err != nil {
		return 0, nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	return resp.StatusCode, raw, err
}

// GetJSON GET 并解析 JSON。
func (c *Client) GetJSON(ctx context.Context, path string, v any) error {
	code, raw, err := c.Do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if code != http.StatusOK {
		return fmt.Errorf("GET %s: HTTP %d", path, code)
	}
	return json.Unmarshal(raw, v)
}

// Alive 核心控制接口是否应答（GET /version）。
func (c *Client) Alive(ctx context.Context) (string, error) {
	var v struct {
		Version string `json:"version"`
		Meta    bool   `json:"meta"`
	}
	if err := c.GetJSON(ctx, "/version", &v); err != nil {
		return "", err
	}
	return v.Version, nil
}

// Stream 以普通 HTTP 流读取 /logs、/traffic 等接口（mihomo 不升级 WebSocket 时按行输出 JSON），
// 每行回调一次，直到 ctx 取消或连接断开。
func (c *Client) Stream(ctx context.Context, path string, fn func(line []byte)) error {
	req, err := c.newReq(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	cl := &http.Client{Transport: c.http.Transport}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("stream %s: HTTP %d", path, resp.StatusCode)
	}
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		fn(sc.Bytes())
	}
	if err := sc.Err(); err != nil && ctx.Err() == nil {
		return err
	}
	return ctx.Err()
}

// ProxyHTTP 返回经核心 mixed 端口出站的 HTTP 客户端（keepAlive 时复用连接）。
func (c *Client) ProxyHTTP(timeout time.Duration, keepAlive bool) *http.Client {
	pu, _ := url.Parse(fmt.Sprintf("http://127.0.0.1:%d", c.MixedPort))
	tr := &http.Transport{
		Proxy:               http.ProxyURL(pu),
		DisableKeepAlives:   !keepAlive,
		MaxIdleConnsPerHost: 2,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: timeout,
		ForceAttemptHTTP2:   false,
	}
	return &http.Client{Transport: tr, Timeout: timeout}
}

// DirectHTTP 返回不经代理的 HTTP 客户端。
func DirectHTTP(timeout time.Duration) *http.Client {
	return &http.Client{Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, Timeout: timeout}
}

// ReverseProxy 把 prefix 下的请求转发到核心外部控制器（含 WebSocket），并替换为核心密钥。
func (c *Client) ReverseProxy(prefix string) http.Handler {
	rp := &httputil.ReverseProxy{
		Rewrite: func(pr *httputil.ProxyRequest) {
			pr.SetURL(c.Base)
			p := strings.TrimPrefix(pr.In.URL.Path, prefix)
			if p == "" {
				p = "/"
			}
			pr.Out.URL.Path = strings.TrimRight(c.Base.Path, "/") + p
			pr.Out.URL.RawPath = ""
			q := pr.Out.URL.Query()
			if q.Has("token") {
				q.Set("token", c.Secret)
				pr.Out.URL.RawQuery = q.Encode()
			}
			pr.Out.Header.Del("Authorization")
			if c.Secret != "" {
				pr.Out.Header.Set("Authorization", "Bearer "+c.Secret)
			}
			pr.Out.Host = c.Base.Host
		},
		FlushInterval: -1,
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			http.Error(w, `{"message":"mihomo unreachable: `+strings.ReplaceAll(err.Error(), `"`, `'`)+`"}`, http.StatusBadGateway)
		},
	}
	return rp
}
