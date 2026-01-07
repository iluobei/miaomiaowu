package substore

import (
	"strings"
)

// MergeToClashTemplate merges generated proxy groups and rules into a Clash template
// Uses string replacement to avoid yaml.Marshal escaping emoji
func MergeToClashTemplate(template, proxyGroups, rules, ruleProviders string) string {
	if strings.TrimSpace(template) == "" {
		// Template is empty, return generated content directly
		result := proxyGroups + "\n\n" + rules
		if ruleProviders != "" {
			result += "\n\n" + ruleProviders
		}
		return result
	}

	lines := strings.Split(template, "\n")
	var result []string
	skipSection := ""
	sectionsToReplace := map[string]bool{
		"proxy-groups:":   true,
		"rules:":          true,
		"rule-providers:": true,
	}

	for i, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Check if entering a section to replace
		if sectionsToReplace[trimmedLine] {
			skipSection = trimmedLine
			continue
		}

		// If currently in a section to skip
		if skipSection != "" {
			// Check if we've reached a new top-level key (doesn't start with space and ends with :)
			if trimmedLine != "" && !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
				// Check if next line is a list or nested content
				if strings.HasSuffix(trimmedLine, ":") || (i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "-")) {
					skipSection = ""
					result = append(result, line)
					continue
				}
				skipSection = ""
				result = append(result, line)
				continue
			}
			// Still in section to skip, skip this line
			continue
		}

		result = append(result, line)
	}

	// Combine results
	resultStr := strings.Join(result, "\n")
	resultStr = strings.TrimRight(resultStr, "\n")

	// Add generated proxy groups and rules
	resultStr += "\n\n" + proxyGroups + "\n\n" + rules
	if ruleProviders != "" {
		resultStr += "\n\n" + ruleProviders
	}

	return resultStr
}

// MergeToSurgeTemplate merges generated proxy groups and rules into a Surge template
func MergeToSurgeTemplate(template, proxyGroups, rules string) string {
	lines := strings.Split(template, "\n")
	var result []string

	skipSection := ""
	sectionsToReplace := map[string]bool{
		"[Proxy Group]": true,
		"[Rule]":        true,
	}

	for _, line := range lines {
		trimmedLine := strings.TrimSpace(line)

		// Check if entering a section to replace
		if strings.HasPrefix(trimmedLine, "[") && strings.HasSuffix(trimmedLine, "]") {
			if sectionsToReplace[trimmedLine] {
				skipSection = trimmedLine
				continue
			} else {
				skipSection = ""
			}
		}

		// Skip content of sections to replace
		if skipSection != "" {
			continue
		}

		result = append(result, line)
	}

	// Add generated content
	resultStr := strings.Join(result, "\n")
	resultStr = strings.TrimRight(resultStr, "\n")
	resultStr += "\n\n" + proxyGroups + "\n\n" + rules

	return resultStr
}

// DetectTemplateType detects template type (clash/surge)
func DetectTemplateType(template string) string {
	if strings.TrimSpace(template) == "" {
		return ""
	}

	// Surge characteristics: [General], [Proxy], [Proxy Group], [Rule] sections
	surgePatterns := []string{"[General]", "[Proxy]", "[Proxy Group]", "[Rule]"}
	for _, pattern := range surgePatterns {
		if strings.Contains(template, pattern) {
			return "surge"
		}
	}

	// Clash characteristics: YAML format with port:, proxies:, proxy-groups:, rules:
	clashPatterns := []string{"port:", "proxies:", "proxy-groups:", "rules:", "socks-port:", "dns:", "mode:"}
	for _, pattern := range clashPatterns {
		if strings.Contains(template, pattern) {
			return "clash"
		}
	}

	return ""
}

// GetDefaultClashTemplate returns a default Clash template
func GetDefaultClashTemplate() string {
	return `port: 7890
socks-port: 7891
allow-lan: true
mode: Rule
log-level: info
dns:
  enable: true
  ipv6: true
  respect-rules: true
  enhanced-mode: fake-ip
  nameserver:
    - "https://120.53.53.53/dns-query"
    - "https://223.5.5.5/dns-query"
  default-nameserver:
    - tls://223.5.5.5
  proxy-server-nameserver:
    - "https://120.53.53.53/dns-query"
    - "https://223.5.5.5/dns-query"
  nameserver-policy:
    geosite:cn,private:
      - "https://120.53.53.53/dns-query"
      - "https://223.5.5.5/dns-query"
    geosite:geolocation-!cn:
      - "https://dns.cloudflare.com/dns-query"
      - "https://dns.google/dns-query"
proxies: ~

`
}

// GetDefaultSurgeTemplate returns a default Surge template
func GetDefaultSurgeTemplate() string {
	return `[General]
loglevel = notify
bypass-system = true
skip-proxy = 127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,100.64.0.0/10,localhost,*.local,e.crashlytics.com,captive.apple.com,::ffff:0:0:0:0/1,::ffff:128:0:0:0/1
bypass-tun = 192.168.0.0/16,10.0.0.0/8,172.16.0.0/12
dns-server = 119.29.29.29,223.5.5.5,218.30.19.40,61.134.1.4
external-controller-access = password@0.0.0.0:6170
http-api = password@0.0.0.0:6171
test-timeout = 5
http-api-web-dashboard = true
exclude-simple-hostnames = true
allow-wifi-access = true
http-listen = 0.0.0.0:6152
socks5-listen = 0.0.0.0:6153
wifi-access-http-port = 6152
wifi-access-socks5-port = 6153

[Proxy]
DIRECT = direct

`
}
