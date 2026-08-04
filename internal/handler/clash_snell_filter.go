package handler

import (
	"strconv"

	"gopkg.in/yaml.v3"
)

// filterSnellV6FromClashYAML 从 Clash YAML 中移除 snell v6 节点及其在 proxy-groups 里的名字引用。
//
// 背景:mihomo 上游只实现 snell v1–v5,遇到 version>=6 的节点是**整份配置 fatal 拒载**
// ("Parse config error: proxy N: snell version error: 6"),保留会炸掉 mihomo 用户的整个订阅。
// t=clashmeta/stash 的 producer 路径已各自过滤(clashmeta 跳 version>5,stash 跳 v4+),
// 但 t 为空 / clash / clashmeta 的**原样 YAML 输出**不经 producer —— 本函数补上这条缝。
// Surge / sing-box 等支持 v6 的格式不走本函数(它们经 convertSubscription 从含 v6 的原 YAML 转换)。
//
// 组引用清理:模板注入会把节点名塞进 proxy-groups,只删 proxies 不删引用同样让 mihomo 拒载
// ("proxy 'X' not found");组被删空时补 DIRECT(空 proxies 的组也是 fatal)。
// 解析失败或无 v6 节点时原样返回(fail-open,不弄丢订阅)。
func filterSnellV6FromClashYAML(data []byte) []byte {
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil || len(root.Content) == 0 {
		return data
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return data
	}

	removed := make(map[string]bool)
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value != "proxies" {
			continue
		}
		seq := doc.Content[i+1]
		if seq.Kind != yaml.SequenceNode {
			return data
		}
		kept := seq.Content[:0:0]
		for _, item := range seq.Content {
			if isSnellV6ProxyNode(item) {
				if name := yamlMapScalar(item, "name"); name != "" {
					removed[name] = true
				}
				continue
			}
			kept = append(kept, item)
		}
		seq.Content = kept
		break
	}
	if len(removed) == 0 {
		return data
	}

	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value != "proxy-groups" {
			continue
		}
		groups := doc.Content[i+1]
		if groups.Kind != yaml.SequenceNode {
			break
		}
		for _, g := range groups.Content {
			if g.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j+1 < len(g.Content); j += 2 {
				if g.Content[j].Value != "proxies" {
					continue
				}
				lst := g.Content[j+1]
				if lst.Kind != yaml.SequenceNode {
					break
				}
				keptRefs := lst.Content[:0:0]
				for _, ref := range lst.Content {
					if ref.Kind == yaml.ScalarNode && removed[ref.Value] {
						continue
					}
					keptRefs = append(keptRefs, ref)
				}
				if len(keptRefs) == 0 {
					keptRefs = append(keptRefs, &yaml.Node{Kind: yaml.ScalarNode, Value: "DIRECT"})
				}
				lst.Content = keptRefs
				break
			}
		}
		break
	}

	out, err := MarshalYAMLWithIndent(doc)
	if err != nil {
		return data
	}
	return []byte(RemoveUnicodeEscapeQuotes(string(out)))
}

// isSnellV6ProxyNode 判断 proxies 序列里的一项是否 type=snell 且 version>=6。
func isSnellV6ProxyNode(item *yaml.Node) bool {
	if item.Kind != yaml.MappingNode {
		return false
	}
	if yamlMapScalar(item, "type") != "snell" {
		return false
	}
	v, err := strconv.Atoi(yamlMapScalar(item, "version"))
	return err == nil && v >= 6
}

// yamlMapScalar 取 mapping node 里 key 对应的标量值;不存在/非标量返回 ""。
func yamlMapScalar(m *yaml.Node, key string) string {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key && m.Content[i+1].Kind == yaml.ScalarNode {
			return m.Content[i+1].Value
		}
	}
	return ""
}
