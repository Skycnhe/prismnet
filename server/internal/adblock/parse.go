package adblock

import (
	"bufio"
	"io"
	"regexp"
	"sort"
	"strings"
)

var rxDomain = regexp.MustCompile(`^(?:[a-z0-9_](?:[a-z0-9_-]{0,61}[a-z0-9_])?\.)+[a-z0-9][a-z0-9-]{0,61}[a-z0-9]$|^(?:[a-z0-9_](?:[a-z0-9_-]{0,61}[a-z0-9_])?\.)+xn--[a-z0-9-]+$`)

// NormDomain 规范化并校验域名；无效返回空串。
func NormDomain(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.TrimSuffix(s, ".")
	if len(s) > 253 || strings.Count(s, ".") == 0 || !rxDomain.MatchString(s) {
		return ""
	}
	return s
}

// allowedOpts AdGuard 规则里不影响「整个域名」语义的选项；带其它选项（如 $script、$domain=）的规则跳过。
var allowedOpts = map[string]bool{"": true, "important": true, "all": true, "document": true, "doc": true, "third-party": true, "3p": true, "popup": true}

// Parse 解析一个过滤列表：支持 hosts（0.0.0.0 / 127.0.0.1 开头）、纯域名、AdGuard / ABP 的 ||domain^ 与 @@||domain^、
// Clash 规则集的 +.domain / DOMAIN-SUFFIX,domain / DOMAIN,domain 写法。返回拦截与放行域名集合。
func Parse(r io.Reader) (block, allow map[string]bool) {
	block, allow = map[string]bool{}, map[string]bool{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		ln := strings.TrimSpace(sc.Text())
		if ln == "" || ln[0] == '!' || ln[0] == '#' || ln[0] == '[' {
			continue
		}
		if i := strings.Index(ln, " #"); i > 0 {
			ln = strings.TrimSpace(ln[:i])
		}
		target := block
		if strings.HasPrefix(ln, "@@") {
			target, ln = allow, ln[2:]
		}
		var d string
		switch {
		case strings.HasPrefix(ln, "||"):
			body := ln[2:]
			opts := ""
			if i := strings.IndexByte(body, '$'); i >= 0 {
				body, opts = body[:i], body[i+1:]
			}
			ok := true
			for _, o := range strings.Split(opts, ",") {
				if !allowedOpts[strings.TrimSpace(o)] {
					ok = false
				}
			}
			if !ok {
				continue
			}
			body = strings.TrimSuffix(body, "|")
			body = strings.TrimSuffix(body, "^")
			if strings.ContainsAny(body, "/*^|") {
				continue
			}
			d = body
		case strings.HasPrefix(ln, "0.0.0.0 ") || strings.HasPrefix(ln, "127.0.0.1 ") || strings.HasPrefix(ln, ":: ") || strings.HasPrefix(ln, "::1 "):
			f := strings.Fields(ln)
			if len(f) < 2 || f[1] == "localhost" || f[1] == "0.0.0.0" {
				continue
			}
			d = f[1]
		case strings.HasPrefix(ln, "+.") || strings.HasPrefix(ln, "*."):
			d = ln[2:]
		case strings.HasPrefix(ln, "."):
			d = ln[1:]
		case strings.HasPrefix(strings.ToUpper(ln), "DOMAIN-SUFFIX,") || strings.HasPrefix(strings.ToUpper(ln), "DOMAIN,"):
			f := strings.Split(ln, ",")
			d = f[1]
		case strings.HasPrefix(ln, "address=/"): // dnsmasq：address=/domain/0.0.0.0
			f := strings.Split(ln, "/")
			if len(f) >= 3 {
				d = f[1]
			}
		default:
			if strings.ContainsAny(ln, " \t/$^|*=,") {
				continue
			}
			d = ln
		}
		if d = NormDomain(d); d != "" {
			target[d] = true
		}
	}
	return block, allow
}

// Compact 去掉已被父域名覆盖的子域名（+.parent 已匹配全部子域），返回排序后的列表。
func Compact(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for d := range set {
		covered := false
		for p := d; ; {
			i := strings.IndexByte(p, '.')
			if i < 0 {
				break
			}
			p = p[i+1:]
			if set[p] {
				covered = true
				break
			}
		}
		if !covered {
			out = append(out, d)
		}
	}
	sort.Strings(out)
	return out
}
