package handler

import (
	"net/http"
	"strings"
)

type clientUARule struct {
	keywords []string
	client   string
}

var clientUARules = []clientUARule{
	{[]string{"stash"}, "stash"},
	{[]string{"shadowrocket"}, "shadowrocket"},
	{[]string{"surge mac", "surgemac"}, "surgemac"},
	{[]string{"surge"}, "surge"},
	{[]string{"loon"}, "loon"},
	{[]string{"quantumult%20x", "quantumult x", "quantumultx"}, "qx"},
	{[]string{"egern"}, "egern"},
	{[]string{"surfboard"}, "surfboard"},
	{[]string{"sing-box", "sfi/", "sfa/", "sfm/", "sft/"}, "sing-box"},
	{[]string{"v2rayn", "v2rayng", "v2box"}, "v2ray"},
	{[]string{"mihomo", "clash"}, "clash"},
}

func detectClientTypeFromUA(ua string) string {
	ua = strings.ToLower(strings.TrimSpace(ua))
	for _, rule := range clientUARules {
		for _, keyword := range rule.keywords {
			if strings.Contains(ua, keyword) {
				return rule.client
			}
		}
	}
	return ""
}

// resolveClientType translates ?t=auto to the format implied by User-Agent.
// Unknown clients intentionally fall back to the default Clash YAML output.
func resolveClientType(r *http.Request) string {
	clientType := strings.TrimSpace(r.URL.Query().Get("t"))
	if !strings.EqualFold(clientType, "auto") {
		return clientType
	}
	return detectClientTypeFromUA(r.Header.Get("User-Agent"))
}
