package handler

import (
	"strings"

	"gopkg.in/yaml.v3"

	"miaomiaowu/internal/logger"
)

// clashProxyMissingFields 返回 clash proxy 缺的必填字段(name/type/server/port),齐全返回 nil。
//
// 订阅里只要有一个这样的条目,mihomo 就整份配置拒绝加载(「proxy N: missing type」),
// 用户一个节点都用不了 —— 所以发出前一律拦下,宁可少这一个节点。
//
// 实际来源:库里 clash_config 本身就是残缺的(例如导入时解析失败只剩 `{}`、或手动编辑配置时删掉了
// 必填字段),订阅生成再往里写节点名,就得到只有 name 的「半个节点」。
//
// 端口跳跃的节点可以没有 port:hysteria2 用 ports、mieru 用 port-range。
// Tailscale 组网节点(auth-key / hostname)和 direct 出站本来就没有 server / port,只要求 name(#135)。
func clashProxyMissingFields(proxy map[string]any) []string {
	var missing []string
	if isServerlessProxyType(proxy) {
		if !clashFieldPresent(proxy["name"]) {
			missing = append(missing, "name")
		}
		return missing
	}
	for _, k := range []string{"name", "type", "server"} {
		if !clashFieldPresent(proxy[k]) {
			missing = append(missing, k)
		}
	}
	if !clashFieldPresent(proxy["port"]) && !clashFieldPresent(proxy["ports"]) && !clashFieldPresent(proxy["port-range"]) {
		missing = append(missing, "port")
	}
	return missing
}

// isServerlessProxyType mihomo 里不需要 server / port 的节点类型。
func isServerlessProxyType(proxy map[string]any) bool {
	typ, _ := proxy["type"].(string)
	switch strings.ToLower(strings.TrimSpace(typ)) {
	case "tailscale", "direct":
		return true
	}
	return false
}

func clashFieldPresent(v any) bool {
	switch x := v.(type) {
	case nil:
		return false
	case string:
		return strings.TrimSpace(x) != ""
	case float64:
		return x != 0
	case int:
		return x != 0
	default:
		return true
	}
}

// dropIncompleteProxiesFromYAML 静态订阅文件那条路的同一道闸:从 YAML 根节点的 proxies 里剔掉残缺节点,
// 并把它们从各代理组的成员里摘掉。组被摘空时补占位节点(补 DIRECT 的话客户端就悄悄直连了);
// 剔完一个都不剩、或有组被补过,就把占位节点追加进 proxies,用户在客户端里看得到原因(见 no_available_node.go)。
// 返回剔掉的个数;没有残缺节点时不动 YAML。
//
// 模板订阅 / 聚合订阅是从节点表动态生成的,在 buildProxyConfig 那一层就拦下了,不走这里。
func dropIncompleteProxiesFromYAML(rootMap *yaml.Node) int {
	if rootMap == nil || rootMap.Kind != yaml.MappingNode {
		return 0
	}
	var proxiesNode, groupsNode *yaml.Node
	for i := 0; i+1 < len(rootMap.Content); i += 2 {
		switch rootMap.Content[i].Value {
		case "proxies":
			proxiesNode = rootMap.Content[i+1]
		case "proxy-groups":
			groupsNode = rootMap.Content[i+1]
		}
	}
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return 0
	}

	removed := make(map[string]bool)
	kept := make([]*yaml.Node, 0, len(proxiesNode.Content))
	dropped := 0
	for _, item := range proxiesNode.Content {
		var proxy map[string]any
		if item.Kind == yaml.MappingNode && item.Decode(&proxy) == nil && proxy != nil {
			if missing := clashProxyMissingFields(proxy); len(missing) > 0 {
				name := yamlMapGet(item, "name")
				logger.Warn("订阅(静态文件):节点 clash 配置残缺,本次不下发", "node", name, "missing", missing)
				if name != "" {
					removed[name] = true
				}
				dropped++
				continue
			}
		}
		kept = append(kept, item)
	}
	if dropped == 0 {
		return 0
	}

	filled := false
	if groupsNode != nil && groupsNode.Kind == yaml.SequenceNode {
		for _, g := range groupsNode.Content {
			members := yamlMapSeq(g, "proxies")
			if members == nil {
				continue
			}
			refs := make([]*yaml.Node, 0, len(members.Content))
			for _, m := range members.Content {
				if m.Kind == yaml.ScalarNode && removed[m.Value] {
					continue
				}
				refs = append(refs, m)
			}
			if len(refs) == 0 && len(members.Content) > 0 {
				refs = append(refs, &yaml.Node{Kind: yaml.ScalarNode, Value: noAvailableNodeName})
				filled = true
			}
			members.Content = refs
		}
	}
	if filled || len(kept) == 0 {
		var placeholder yaml.Node
		if err := placeholder.Encode(noAvailableNodeProxy()); err == nil {
			kept = append(kept, &placeholder)
		}
	}
	proxiesNode.Content = kept
	return dropped
}

// dropIncompleteProxiesFromStaticSubscription 对静态订阅文件的原始 YAML 跑一遍 dropIncompleteProxiesFromYAML。
// 没剔掉任何节点、或解析不了(非 YAML 内容)时原样返回,不重新序列化。
func dropIncompleteProxiesFromStaticSubscription(data []byte) []byte {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return data
	}
	n := dropIncompleteProxiesFromYAML(doc.Content[0])
	if n == 0 {
		return data
	}
	logger.Warn("订阅(静态文件):已剔除残缺节点", "count", n)
	out, err := MarshalYAMLWithIndent(&doc)
	if err != nil {
		return data
	}
	return []byte(RemoveUnicodeEscapeQuotes(string(out)))
}
