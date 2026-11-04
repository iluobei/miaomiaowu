package handler

import (
	"net/url"
	"strings"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
)

// normalizeLoonHysteria2Passwords works around Loon treating percent escapes in
// a Hysteria2 password as literal password bytes. Imported URI credentials can
// survive in Clash YAML as e.g. "secret%3D"; Loon's native configuration wants
// the decoded credential instead. PathUnescape is intentional: unlike query
// decoding it preserves a literal '+'.
func normalizeLoonHysteria2Passwords(proxies []substore.Proxy) {
	for _, proxy := range proxies {
		proxyType := strings.ToLower(strings.TrimSpace(substore.GetString(proxy, "type")))
		if proxyType != "hysteria2" && proxyType != "hy2" {
			continue
		}

		password, ok := proxy["password"].(string)
		if !ok || !strings.Contains(password, "%") {
			continue
		}
		decoded, err := url.PathUnescape(password)
		if err == nil {
			proxy["password"] = decoded
		}
	}
}

func isLoonClientType(clientType string) bool {
	switch strings.ToLower(strings.TrimSpace(clientType)) {
	case "loon", "clash-to-loon", "clash-to-loon-kelee":
		return true
	default:
		return false
	}
}
