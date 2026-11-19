package adblock

import (
	"bytes"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Inject 在 mihomo 配置里加入（on）或移除（!on）广告拦截的规则集与规则，尽量保留原有注释与顺序。
// 返回新内容与是否有改动。
func Inject(raw []byte, on bool) ([]byte, bool, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, false, fmt.Errorf("解析核心配置失败：%w", err)
	}
	if len(doc.Content) == 0 {
		doc = yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode}}}
	}
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return nil, false, fmt.Errorf("核心配置顶层不是映射")
	}
	changed := false

	providers := mapGet(root, "rule-providers")
	rules := mapGet(root, "rules")
	if on {
		if providers == nil {
			providers = &yaml.Node{Kind: yaml.MappingNode}
			mapSet(root, "rule-providers", providers)
			changed = true
		}
		for _, name := range []string{BlockSet, AllowSet} {
			want := providerNode(name)
			if cur := mapGet(providers, name); cur == nil || !sameNode(cur, want) {
				mapSet(providers, name, want)
				changed = true
			}
		}
		if rules == nil {
			rules = &yaml.Node{Kind: yaml.SequenceNode}
			mapSet(root, "rules", rules)
			changed = true
		}
		if rules.Kind != yaml.SequenceNode {
			return nil, false, fmt.Errorf("rules 不是列表")
		}
		if len(rules.Content) == 0 || strings.TrimSpace(rules.Content[0].Value) != Rule {
			removeRule(rules)
			rules.Content = append([]*yaml.Node{{Kind: yaml.ScalarNode, Value: Rule}}, rules.Content...)
			changed = true
		}
	} else {
		if providers != nil {
			for _, name := range []string{BlockSet, AllowSet} {
				if mapDel(providers, name) {
					changed = true
				}
			}
			if len(providers.Content) == 0 {
				mapDel(root, "rule-providers")
			}
		}
		if rules != nil && removeRule(rules) {
			changed = true
		}
	}
	if !changed {
		return raw, false, nil
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&doc); err != nil {
		return nil, false, err
	}
	_ = enc.Close()
	return buf.Bytes(), true, nil
}

func providerNode(name string) *yaml.Node {
	n := &yaml.Node{Kind: yaml.MappingNode}
	for _, kv := range [][2]string{{"type", "file"}, {"behavior", "domain"}, {"format", "text"}, {"path", "./prismnet/" + name + ".txt"}} {
		n.Content = append(n.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: kv[0]}, &yaml.Node{Kind: yaml.ScalarNode, Value: kv[1]})
	}
	return n
}

func sameNode(a, b *yaml.Node) bool {
	if a.Kind != yaml.MappingNode || len(a.Content) != len(b.Content) {
		return false
	}
	for i := 0; i+1 < len(b.Content); i += 2 {
		v := mapGet(a, b.Content[i].Value)
		if v == nil || v.Value != b.Content[i+1].Value {
			return false
		}
	}
	return true
}

func removeRule(rules *yaml.Node) bool {
	out := rules.Content[:0]
	removed := false
	for _, n := range rules.Content {
		v := strings.TrimSpace(n.Value)
		if v == Rule || strings.Contains(v, "RULE-SET,"+BlockSet) {
			removed = true
			continue
		}
		out = append(out, n)
	}
	rules.Content = out
	return removed
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return m.Content[i+1]
		}
	}
	return nil
}

func mapSet(m *yaml.Node, key string, v *yaml.Node) {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content[i+1] = v
			return
		}
	}
	m.Content = append(m.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: key}, v)
}

func mapDel(m *yaml.Node, key string) bool {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			m.Content = append(m.Content[:i], m.Content[i+2:]...)
			return true
		}
	}
	return false
}
