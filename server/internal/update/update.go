// Package update 在线更新：查询 GitHub 最新 Release，下载对应架构的发布包，
// 校验 SHA-256 后替换面板文件与后端程序，然后原地重启后端。
package update

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Release GitHub Release 的必要字段。
type Release struct {
	Tag       string  `json:"tag_name"`
	Name      string  `json:"name"`
	Body      string  `json:"body"`
	Published string  `json:"published_at"`
	HTMLURL   string  `json:"html_url"`
	Assets    []Asset `json:"assets"`
}

// Asset 发布附件。
type Asset struct {
	Name string `json:"name"`
	URL  string `json:"browser_download_url"`
	Size int64  `json:"size"`
}

// Check 检查结果。
type Check struct {
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	HasUpdate bool   `json:"has_update"`
	Notes     string `json:"notes"`
	URL       string `json:"url"`
	Published string `json:"published"`
	Asset     string `json:"asset"`
	Error     string `json:"error,omitempty"`
}

// Updater 更新器。
type Updater struct {
	Repo     string
	Current  string
	UIDir    string
	WorkDir  string
	Exe      string // 后端程序路径（空则取 os.Executable）
	Client   func() *http.Client
	APIBase  string // 测试用，默认 https://api.github.com
	Progress func(stage string)

	mu      sync.Mutex
	running bool
}

// AssetName 当前平台的发布包名。
func AssetName() string { return fmt.Sprintf("prismnet-linux-%s.tar.gz", runtime.GOARCH) }

func (u *Updater) http() *http.Client {
	if u.Client != nil {
		return u.Client()
	}
	return &http.Client{Timeout: 5 * time.Minute}
}

// Latest 读取最新 Release。
func (u *Updater) Latest(ctx context.Context) (*Release, error) {
	base := u.APIBase
	if base == "" {
		base = "https://api.github.com"
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/repos/"+u.Repo+"/releases/latest", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "PrismNet/"+u.Current)
	resp, err := u.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.New("仓库还没有发布版本")
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API HTTP %d", resp.StatusCode)
	}
	var r Release
	if err := json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(&r); err != nil {
		return nil, err
	}
	return &r, nil
}

// CheckUpdate 比较当前版本与最新版本。
func (u *Updater) CheckUpdate(ctx context.Context) Check {
	c := Check{Current: u.Current, Asset: AssetName()}
	r, err := u.Latest(ctx)
	if err != nil {
		c.Error = err.Error()
		return c
	}
	c.Latest, c.Notes, c.URL, c.Published = r.Tag, r.Body, r.HTMLURL, r.Published
	c.HasUpdate = Newer(r.Tag, u.Current)
	return c
}

// Newer a 是否比 b 新（语义化版本，忽略前缀 v；无法解析时按字符串不等处理）。
func Newer(a, b string) bool {
	pa, oka := parseVer(a)
	pb, okb := parseVer(b)
	if !oka || !okb {
		return strings.TrimPrefix(a, "v") != strings.TrimPrefix(b, "v") && a != ""
	}
	for i := 0; i < 3; i++ {
		if pa[i] != pb[i] {
			return pa[i] > pb[i]
		}
	}
	return false
}

func parseVer(s string) ([3]int, bool) {
	var v [3]int
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) == 0 || len(parts) > 3 {
		return v, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil {
			return v, false
		}
		v[i] = n
	}
	return v, true
}

// Apply 下载、校验并安装最新版本；成功后返回新版本号，调用方负责重启进程。
func (u *Updater) Apply(ctx context.Context) (string, error) {
	u.mu.Lock()
	if u.running {
		u.mu.Unlock()
		return "", errors.New("更新正在进行中")
	}
	u.running = true
	u.mu.Unlock()
	defer func() { u.mu.Lock(); u.running = false; u.mu.Unlock() }()

	u.step("检查最新版本")
	r, err := u.Latest(ctx)
	if err != nil {
		return "", err
	}
	var pkg, sums *Asset
	for i := range r.Assets {
		switch r.Assets[i].Name {
		case AssetName():
			pkg = &r.Assets[i]
		case "sha256sums.txt":
			sums = &r.Assets[i]
		}
	}
	if pkg == nil {
		return "", fmt.Errorf("发布 %s 中没有 %s", r.Tag, AssetName())
	}
	if sums == nil {
		return "", fmt.Errorf("发布 %s 中没有 sha256sums.txt，拒绝安装未校验的文件", r.Tag)
	}
	if err := os.MkdirAll(u.WorkDir, 0o755); err != nil {
		return "", err
	}
	u.step("下载校验文件")
	sumRaw, err := u.download(ctx, sums.URL, 1<<20)
	if err != nil {
		return "", err
	}
	want := findSum(string(sumRaw), pkg.Name)
	if want == "" {
		return "", fmt.Errorf("sha256sums.txt 中没有 %s", pkg.Name)
	}
	u.step("下载发布包")
	data, err := u.download(ctx, pkg.URL, 200<<20)
	if err != nil {
		return "", err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != want {
		return "", errors.New("SHA-256 校验失败，已放弃更新")
	}
	u.step("解压")
	stage := filepath.Join(u.WorkDir, "stage")
	_ = os.RemoveAll(stage)
	if err := Extract(data, stage); err != nil {
		return "", err
	}
	newBin := filepath.Join(stage, "prismnet")
	newUI := filepath.Join(stage, "ui")
	if fi, err := os.Stat(newBin); err != nil || fi.IsDir() {
		return "", errors.New("发布包缺少 prismnet 程序")
	}
	if _, err := os.Stat(filepath.Join(newUI, "index.html")); err != nil {
		return "", errors.New("发布包缺少 ui/index.html")
	}
	u.step("安装")
	if err := InstallUI(newUI, u.UIDir); err != nil {
		return "", err
	}
	exe := u.Exe
	if exe == "" {
		if exe, err = os.Executable(); err != nil {
			return "", err
		}
		exe, _ = filepath.EvalSymlinks(exe)
	}
	if err := InstallBinary(newBin, exe); err != nil {
		return "", err
	}
	_ = os.RemoveAll(stage)
	u.step("完成")
	return r.Tag, nil
}

func (u *Updater) step(s string) {
	if u.Progress != nil {
		u.Progress(s)
	}
}

func (u *Updater) download(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "PrismNet/"+u.Current)
	resp, err := u.http().Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("下载 %s：HTTP %d", url, resp.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(raw)) > limit {
		return nil, errors.New("文件过大")
	}
	return raw, nil
}

func findSum(sums, name string) string {
	sc := bufio.NewScanner(strings.NewReader(sums))
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name && len(f[0]) == 64 {
			return strings.ToLower(f[0])
		}
	}
	return ""
}

// Extract 解压 tar.gz 到 dst（拒绝绝对路径、.. 与链接）。
func Extract(data []byte, dst string) error {
	gz, err := gzip.NewReader(strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		name := filepath.Clean(strings.TrimPrefix(h.Name, "./"))
		if name == "." {
			continue
		}
		if filepath.IsAbs(name) || name == ".." || strings.HasPrefix(name, "../") {
			return fmt.Errorf("发布包含非法路径 %s", h.Name)
		}
		p := filepath.Join(dst, name)
		switch h.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(p, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o755|0o644)
			if err != nil {
				return err
			}
			if _, err := io.Copy(f, io.LimitReader(tr, 100<<20)); err != nil {
				f.Close()
				return err
			}
			if err := f.Close(); err != nil {
				return err
			}
		default: // 跳过链接等
		}
	}
}

// InstallUI 用 src 目录替换 dst（旧目录保留为 dst.bak）。
func InstallUI(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp := dst + ".new"
	_ = os.RemoveAll(tmp)
	if err := copyDir(src, tmp); err != nil {
		return err
	}
	bak := dst + ".bak"
	_ = os.RemoveAll(bak)
	if _, err := os.Stat(dst); err == nil {
		if err := os.Rename(dst, bak); err != nil {
			return err
		}
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Rename(bak, dst)
		return err
	}
	return nil
}

// InstallBinary 用 src 替换 dst（旧程序保留为 dst.bak）。
func InstallBinary(src, dst string) error {
	tmp := dst + ".new"
	if err := copyFile(src, tmp, 0o755); err != nil {
		return err
	}
	bak := dst + ".bak"
	_ = os.Remove(bak)
	if err := os.Link(dst, bak); err != nil {
		_ = copyFile(dst, bak, 0o755)
	}
	return os.Rename(tmp, dst)
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		t := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(t, 0o755)
		}
		if !fi.Mode().IsRegular() {
			return nil
		}
		return copyFile(p, t, 0o644)
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
