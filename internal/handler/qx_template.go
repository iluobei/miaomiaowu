package handler

import (
	"fmt"
	"strings"

	"miaomiaowu/internal/logger"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
)

// Quantumult X 模板(#835)。机制与 Surge / Loon 模板同构:模板是一份 INI 风格分段文本,
// 后端只把节点写进节点段,策略组和规则完全由模板决定。
//
// 三点与 Surge 不同:
//   - 节点段叫 **[server_local]**,不是 [Proxy];
//   - **扩展名用 .qx**。QX 自己的配置文件也叫 .conf,而 .conf 在这里已经代表
//     Surge 模板了(isSurgeTemplateFile 就是「以 .conf 结尾」)—— 两种模板共用一个
//     后缀就只能去猜内容。Loon 当初为同一个理由选了 .lcf,这里照做。
//   - 地区分组靠 **server-tag-regex**(QX 官方 sample.conf 的写法),
//     对应 Surge 的 policy-regex-filter。

// isQXTemplateFile 按扩展名识别 Quantumult X 模板。
func isQXTemplateFile(filename string) bool {
	return strings.HasSuffix(strings.ToLower(filename), ".qx")
}

// isQXTemplateClientType 判断这次请求能不能套用户的 QX 模板。
func isQXTemplateClientType(clientType string) bool {
	return strings.EqualFold(strings.TrimSpace(clientType), "qx")
}

// injectProxiesIntoQXTemplate 把节点写进模板的 [server_local] 段。
//
// 与 Surge 版同样的取舍:段内保留注释(占位说明),丢弃其它内容 —— 否则模板里
// 示例用的占位节点会跟着发到用户手上。
func injectProxiesIntoQXTemplate(templateContent string, proxies []map[string]any) (string, error) {
	qxProxies := make([]substore.Proxy, 0, len(proxies))
	for _, p := range proxies {
		qxProxies = append(qxProxies, substore.Proxy(p))
	}

	producer := substore.NewQXProducer()
	produced, err := producer.Produce(qxProxies, "", &substore.ProduceOptions{})
	if err != nil {
		return "", err
	}
	proxyLines, ok := produced.(string)
	if !ok {
		return "", fmt.Errorf("unexpected qx producer result type: %T", produced)
	}
	proxyLines = strings.TrimRight(proxyLines, "\n")

	// Produce 会静默丢弃 QX 不支持的节点类型。Surge 分支能逐个 ProduceOne 探出是哪几个,
	// QX producer 的 produceOne 没导出,这里只能按行数差报个数量 —— 说不出名字,
	// 但至少让「订阅里少了几个节点」这件事在日志里留下痕迹。
	if produced := countNonEmptyLines(proxyLines); produced < len(qxProxies) {
		logger.Info("[QX模板] 部分节点因类型不受 Quantumult X 支持被过滤",
			"filtered_count", len(qxProxies)-produced, "total", len(qxProxies))
	}

	lines := strings.Split(templateContent, "\n")
	out := make([]string, 0, len(lines)+2)
	inServerSection := false
	injected := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			inServerSection = strings.EqualFold(trimmed, "[server_local]")
			out = append(out, line)
			if inServerSection {
				if proxyLines != "" {
					out = append(out, proxyLines)
				}
				injected = true
			}
			continue
		}
		if inServerSection {
			if trimmed == "" || isQXComment(trimmed) {
				out = append(out, line)
			}
			continue
		}
		out = append(out, line)
	}

	if !injected {
		out = append(out, "", "[server_local]")
		if proxyLines != "" {
			out = append(out, proxyLines)
		}
	}

	return omitEmptyQXSections(strings.Join(out, "\n")), nil
}

func isQXComment(line string) bool {
	return strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "//")
}

// omitEmptyQXSections runs after node injection so a populated server_local is
// retained. Blank lines and commented examples do not make a section active.
// Preserve the preamble and populated sections verbatim.
func omitEmptyQXSections(content string) string {
	lines := strings.Split(content, "\n")
	out := make([]string, 0, len(lines))
	start := 0
	inSection, hasContent := false, false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if !inSection || hasContent {
				out = append(out, lines[start:i]...)
			}
			start, inSection, hasContent = i, true, false
		} else if trimmed != "" && !isQXComment(trimmed) {
			hasContent = true
		}
	}
	if !inSection || hasContent {
		out = append(out, lines[start:]...)
	}
	return strings.Join(out, "\n")
}

// countNonEmptyLines 数非空行。producer 每个节点输出一行,用它反推有多少节点被丢弃。
func countNonEmptyLines(s string) int {
	if strings.TrimSpace(s) == "" {
		return 0
	}
	n := 0
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			n++
		}
	}
	return n
}
