package handler

import (
	"net/url"
	"strings"
)

// mierus:// 是 Mieru 官方客户端导出的分享链接,和 proxyparser 认识的 mieru:// 结构不同:
//   - 端口放在 query 的 port= 里,而不是 host:port
//   - 传输协议在 protocol=,mieru:// 里叫 transport
//   - 多出 handshake-mode / profile 等 mihomo 没有的字段
// 所以直接把 scheme 换成 mieru:// 会解析出 port=0(见 GitHub issue #115)。
// 这里把 mierus:// 规整成 proxyparser 能吃的 mieru://user:pass@host:port?transport=...#name。
// proxyparser 将来若原生支持 mierus:// 可移除本 shim。

// normalizeMieruShareURI 把单条 mierus:// 链接规整成 mieru://。非 mierus:// 原样返回。
func normalizeMieruShareURI(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "mierus://") {
		return raw
	}
	content := strings.TrimPrefix(trimmed, "mierus://")

	name := ""
	if i := strings.LastIndex(content, "#"); i != -1 {
		name = content[i+1:]
		content = content[:i]
	}

	query := ""
	if i := strings.Index(content, "?"); i != -1 {
		query = content[i+1:]
		content = content[:i]
	}
	content = strings.TrimSuffix(content, "/")

	at := strings.LastIndex(content, "@")
	if at == -1 {
		return raw // 结构不符合预期,原样交给上层报错
	}
	auth := content[:at]
	host := content[at+1:]

	q, _ := url.ParseQuery(query)
	// 端口在 query 里;host 自身没带端口时才补上(避免覆盖 host:port 或误伤 IPv6)。
	if port := q.Get("port"); port != "" && !strings.Contains(host, ":") {
		host = host + ":" + port
	}

	newQ := url.Values{}
	if p := q.Get("protocol"); p != "" {
		newQ.Set("transport", p)
	}
	if m := q.Get("multiplexing"); m != "" {
		newQ.Set("multiplexing", m)
	}
	if mtu := q.Get("mtu"); mtu != "" {
		newQ.Set("mtu", mtu)
	}

	out := "mieru://" + auth + "@" + host
	if enc := newQ.Encode(); enc != "" {
		out += "?" + enc
	}
	if name != "" {
		out += "#" + name
	}
	return out
}

// rewriteMierusInContent 把订阅内容里逐行的 mierus:// 规整成 mieru://。
// 只处理明文内容(用户直接粘贴分享链接的场景);base64 订阅里的 mierus:// 由上游解码后
// 走单条解析路径,这里看不到也不需要动。
func rewriteMierusInContent(content string) string {
	if !strings.Contains(content, "mierus://") {
		return content
	}
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.Contains(line, "mierus://") {
			lines[i] = normalizeMieruShareURI(line)
		}
	}
	return strings.Join(lines, "\n")
}
