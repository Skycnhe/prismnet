// Package config 读取 PrismNet 后端配置（/etc/prismnet/config.json）。
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
)

// Site 概览「站点延迟」卡片的一个站点。
type Site struct {
	Name string `json:"name"`
	URL  string `json:"url"`
	Icon string `json:"icon"`
}

// Mihomo 核心连接参数。
type Mihomo struct {
	Controller string `json:"controller"`  // 外部控制器地址，例如 http://127.0.0.1:9090
	Secret     string `json:"secret"`      // 外部控制器密钥
	MixedPort  int    `json:"mixed_port"`  // 本机 mixed 端口，站点延迟 / 代理出口 IP 经它测
	ConfigPath string `json:"config_path"` // 核心配置文件，广告拦截写入规则时修改它
	Service    string `json:"service"`     // 系统服务名（OpenRC / systemd）
}

// Watchdog 故障自动直连（看门狗）。
type Watchdog struct {
	Enabled       bool   `json:"enabled"`
	Interval      int    `json:"interval"`       // 检查间隔（秒）
	Timeout       int    `json:"timeout"`        // 单次检查超时（秒）
	FailThreshold int    `json:"fail_threshold"` // 连续失败几次后切换直连
	RestartCmd    string `json:"restart_cmd"`    // 为空时按 service 自动选择 rc-service / systemctl
	BypassCmd     string `json:"bypass_cmd"`     // 关闭透明代理（直连）命令
	RestoreCmd    string `json:"restore_cmd"`    // 恢复透明代理命令
}

// Update 在线更新。
type Update struct {
	Repo  string `json:"repo"`  // GitHub 仓库 owner/name
	Proxy bool   `json:"proxy"` // 经核心 mixed 端口下载
}

// Config 后端配置。
type Config struct {
	Listen   string   `json:"listen"`
	Secret   string   `json:"secret"` // 面板访问密钥（为空则不校验）
	DataDir  string   `json:"data_dir"`
	UIDir    string   `json:"ui_dir"`
	Mihomo   Mihomo   `json:"mihomo"`
	Watchdog Watchdog `json:"watchdog"`
	Sites    []Site   `json:"sites"`
	Update   Update   `json:"update"`

	path string
	mu   sync.Mutex
}

// DefaultSites 默认站点（与截图一致）。
func DefaultSites() []Site {
	return []Site{
		{Name: "Google", URL: "https://www.google.com/generate_204", Icon: "google"},
		{Name: "YouTube", URL: "https://www.youtube.com/generate_204", Icon: "youtube"},
		{Name: "Telegram", URL: "https://web.telegram.org/favicon.ico", Icon: "telegram"},
		{Name: "GitHub", URL: "https://github.com/favicon.ico", Icon: "github"},
		{Name: "百度", URL: "https://www.baidu.com/favicon.ico", Icon: "baidu"},
		{Name: "淘宝", URL: "https://www.taobao.com/favicon.ico", Icon: "taobao"},
	}
}

// Default 返回默认配置。
func Default() *Config {
	return &Config{
		Listen:  ":7880",
		DataDir: "/var/lib/prismnet",
		UIDir:   "/opt/prismnet/ui",
		Mihomo: Mihomo{
			Controller: "http://127.0.0.1:9090",
			MixedPort:  7890,
			ConfigPath: "/etc/mihomo/config.yaml",
			Service:    "mihomo",
		},
		Watchdog: Watchdog{Enabled: true, Interval: 30, Timeout: 8, FailThreshold: 2},
		Sites:    DefaultSites(),
		Update:   Update{Repo: "Skycnhe/prismnet"},
	}
}

// Load 读取配置文件；文件不存在时返回默认配置（并记住路径以便保存）。
func Load(path string) (*Config, error) {
	c := Default()
	c.path = path
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, c); err != nil {
		return nil, err
	}
	c.normalize()
	return c, nil
}

func (c *Config) normalize() {
	d := Default()
	if c.Listen == "" {
		c.Listen = d.Listen
	}
	if c.DataDir == "" {
		c.DataDir = d.DataDir
	}
	if c.UIDir == "" {
		c.UIDir = d.UIDir
	}
	if c.Mihomo.Controller == "" {
		c.Mihomo.Controller = d.Mihomo.Controller
	}
	if c.Mihomo.MixedPort <= 0 {
		c.Mihomo.MixedPort = d.Mihomo.MixedPort
	}
	if c.Mihomo.Service == "" {
		c.Mihomo.Service = d.Mihomo.Service
	}
	if c.Watchdog.Interval < 5 {
		c.Watchdog.Interval = d.Watchdog.Interval
	}
	if c.Watchdog.Timeout <= 0 {
		c.Watchdog.Timeout = d.Watchdog.Timeout
	}
	if c.Watchdog.FailThreshold <= 0 {
		c.Watchdog.FailThreshold = d.Watchdog.FailThreshold
	}
	if len(c.Sites) == 0 {
		c.Sites = d.Sites
	}
	if c.Update.Repo == "" {
		c.Update.Repo = d.Update.Repo
	}
}

// Path 配置文件路径。
func (c *Config) Path() string { return c.path }

// Save 原子写回配置文件。
func (c *Config) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.path == "" {
		return errors.New("config path unknown")
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, append(raw, '\n'), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}
