package handler

import (
	"fmt"
	"strings"

	"miaomiaowu/internal/logger"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
)

// Loon 模板。机制与 Surge 模板同构:模板是一份 INI 风格分段文本,后端只把节点写进
// [Proxy] 段,策略组和规则完全由模板决定。
//
// 与 Surge 的两点不同:
//   - **扩展名用 .lcf**。Loon 自己导出的配置也叫 .conf,而 .conf 在这里已经代表
//     Surge 模板了 —— 靠扩展名区分是现成机制(isSurgeTemplateFile),再让两种模板
//     共用一个后缀就只能去猜内容,那才是真的脆。.lcf 是 Loon 社区通用的后缀,
//     本仓库内置的 kelee 模板用的也是它。
//   - 多一个 [Proxy Chain] 段:clash 的 dialer-proxy 要落成链,否则流量会绕过入口。

// isLoonTemplateFile 按扩展名识别 Loon 模板。
func isLoonTemplateFile(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".lcf")
}

// isLoonTemplateClientType 判断这次请求**能不能套用户的 Loon 模板**。
//
// 与已有的 isLoonClientType(loon_hysteria2_compat.go)区别只在 kelee:
// 那个判「是不是 Loon 系」用于节点字段兼容处理,kelee 当然算;
// 而 kelee 本身就是一份**固定模板**的产物,再套一份用户模板只会互相打架。
func isLoonTemplateClientType(clientType string) bool {
	switch strings.ToLower(strings.TrimSpace(clientType)) {
	case "loon", "clash-to-loon":
		return true
	}
	return false
}

// loonTemplatePolicyNames 扫出模板 [Proxy Group] 段里定义的组名。
//
// 用来校验链的首跳:模板自带的策略组名这边本来看不到,不扫的话,
// 指向它们的链会被当成悬空引用丢掉 —— 而模板场景下这恰恰是最常见的一种。
func loonTemplatePolicyNames(templateContent string) []string {
	var names []string
	inGroups := false
	for _, line := range strings.Split(templateContent, "\n") {
		t := strings.TrimSpace(line)
		if strings.HasPrefix(t, "[") && strings.HasSuffix(t, "]") {
			inGroups = strings.EqualFold(t, "[Proxy Group]")
			continue
		}
		if !inGroups || t == "" || strings.HasPrefix(t, "#") {
			continue
		}
		if i := strings.Index(t, "="); i > 0 {
			if n := strings.TrimSpace(t[:i]); n != "" {
				names = append(names, n)
			}
		}
	}
	return names
}

// injectProxiesIntoLoonTemplate 把节点写进模板的 [Proxy] 段,并按需补 [Proxy Chain] 段。
//
// 与 Surge 版同样的取舍:段内保留注释(占位说明),丢弃其它内容,避免残留占位节点。
func injectProxiesIntoLoonTemplate(templateContent string, proxies []map[string]any) (string, error) {
	loonProxies := make([]substore.Proxy, 0, len(proxies))
	for _, p := range proxies {
		loonProxies = append(loonProxies, substore.Proxy(p))
	}
	proxyLines, chainLines, err := substore.BuildLoonProxySections(
		loonProxies, loonTemplatePolicyNames(templateContent))
	if err != nil {
		return "", fmt.Errorf("build loon proxy sections: %w", err)
	}
	proxyLines = strings.TrimRight(proxyLines, "\n")

	// 生成器对不支持的节点类型是静默跳过的。逐个探一遍并记日志,
	// 免得用户只看到"订阅里少了几个节点"却无从查起(照搬 Surge 那边的做法)。
	producer := substore.NewLoonProducer()
	var filtered []string
	for _, p := range loonProxies {
		if line, perr := producer.ProduceOne(p, "", &substore.ProduceOptions{}); perr != nil || line == "" {
			name, _ := p["name"].(string)
			typ, _ := p["type"].(string)
			filtered = append(filtered, fmt.Sprintf("%s(%s)", name, typ))
		}
	}
	if len(filtered) > 0 {
		logger.Info("[Loon模板] 部分节点因类型不受 Loon 支持被过滤",
			"filtered_count", len(filtered), "total", len(loonProxies), "nodes", strings.Join(filtered, ", "))
	}

	out, injectedProxy, hadChainSection := injectIntoLoonSections(templateContent, proxyLines, chainLines)
	if !injectedProxy {
		out = append(out, "", "[Proxy]")
		if proxyLines != "" {
			out = append(out, proxyLines)
		}
	}
	// 模板没有 [Proxy Chain] 段但这次有链 → 追加一段,否则链就丢了。
	if !hadChainSection && chainLines != "" {
		out = append(out, "", "[Proxy Chain]", chainLines)
	}
	return strings.Join(out, "\n"), nil
}

// injectIntoLoonSections 遍历模板,替换 [Proxy] 与 [Proxy Chain] 两段的内容。
func injectIntoLoonSections(templateContent, proxyLines, chainLines string) (out []string, injectedProxy, hadChain bool) {
	section := ""
	for _, line := range strings.Split(templateContent, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			switch {
			case strings.EqualFold(trimmed, "[Proxy]"):
				section = "proxy"
			case strings.EqualFold(trimmed, "[Proxy Chain]"):
				section = "chain"
			default:
				section = ""
			}
			out = append(out, line)
			switch section {
			case "proxy":
				if proxyLines != "" {
					out = append(out, proxyLines)
				}
				injectedProxy = true
			case "chain":
				if chainLines != "" {
					out = append(out, chainLines)
				}
				hadChain = true
			}
			continue
		}
		if section != "" {
			if strings.HasPrefix(trimmed, "#") || trimmed == "" {
				out = append(out, line)
			}
			continue
		}
		out = append(out, line)
	}
	return out, injectedProxy, hadChain
}

// templateKind 模板种类。靠**扩展名**区分:.yaml/.yml = Clash、.conf = Surge、.lcf = Loon。
//
// 加 Loon 之前这里只有一个 bool(wantSurge),三种就表达不了了 ——
// 而 `isSurgeTemplateFile(name) != wantSurge` 这种写法一旦有第三种,
// Loon 模板会被当成 Clash 模板选中,产出一份彻底错的配置。
type templateKind int

const (
	templateKindClash templateKind = iota
	templateKindSurge
	templateKindLoon
)

func templateKindOfFile(name string) templateKind {
	switch {
	case isSurgeTemplateFile(name):
		return templateKindSurge
	case isLoonTemplateFile(name):
		return templateKindLoon
	default:
		return templateKindClash
	}
}

// templateKindForClient 这次订阅请求该用哪一种模板。
func templateKindForClient(clientType string) templateKind {
	switch {
	case isSurgeClientType(clientType):
		return templateKindSurge
	case isLoonTemplateClientType(clientType):
		return templateKindLoon
	default:
		return templateKindClash
	}
}
