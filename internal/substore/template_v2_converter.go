package substore

import (
	"log"
	"regexp"
	"strings"
)

// TemplateV3Content represents the converted v3 template content
type TemplateV3Content struct {
	ProxyGroups   []ProxyGroupV3Config          `yaml:"proxy-groups,omitempty"`
	Rules         []string                      `yaml:"rules,omitempty"`
	RuleProviders map[string]RuleProviderConfig `yaml:"rule-providers,omitempty"`
}

// ProxyGroupV3Config represents a proxy group in v3 format
type ProxyGroupV3Config struct {
	Name              string   `yaml:"name" json:"name"`
	Type              string   `yaml:"type" json:"type"`
	Proxies           []string `yaml:"proxies,omitempty" json:"proxies,omitempty"`
	IncludeAll        bool     `yaml:"include-all,omitempty" json:"include-all,omitempty"`
	IncludeAllProxies bool     `yaml:"include-all-proxies,omitempty" json:"include-all-proxies,omitempty"`
	Filter            string   `yaml:"filter,omitempty" json:"filter,omitempty"`
	ExcludeFilter     string   `yaml:"exclude-filter,omitempty" json:"exclude-filter,omitempty"`
	URL               string   `yaml:"url,omitempty" json:"url,omitempty"`
	Interval          int      `yaml:"interval,omitempty" json:"interval,omitempty"`
	Tolerance         int      `yaml:"tolerance,omitempty" json:"tolerance,omitempty"`
}

// RuleProviderConfig represents a rule provider configuration
type RuleProviderConfig struct {
	Type     string `yaml:"type" json:"type"`
	Behavior string `yaml:"behavior" json:"behavior"`
	URL      string `yaml:"url" json:"url"`
	Path     string `yaml:"path" json:"path"`
	Interval int    `yaml:"interval" json:"interval"`
}

// ConvertACLToV3 converts ACL4SSR format to v3 template format
func ConvertACLToV3(content string) (*TemplateV3Content, error) {
	log.Printf("[V2转V3] 开始转换，输入内容长度: %d 字节", len(content))

	// 打印前500个字符用于调试
	previewLen := 500
	if len(content) < previewLen {
		previewLen = len(content)
	}
	log.Printf("[V2转V3] 输入内容预览:\n%s", content[:previewLen])

	rulesets, proxyGroups := ParseACLConfig(content)

	log.Printf("[V2转V3] 解析完成: 找到 %d 个ruleset, %d 个proxy-group", len(rulesets), len(proxyGroups))

	// 打印每个解析到的代理组
	for i, pg := range proxyGroups {
		log.Printf("[V2转V3] 代理组[%d]: 名称='%s', 类型='%s', 代理数=%d, 通配符=%v, URL='%s', 间隔=%d, 容差=%d",
			i, pg.Name, pg.Type, len(pg.Proxies), pg.HasWildcard, pg.URL, pg.Interval, pg.Tolerance)
		if len(pg.Proxies) > 0 {
			log.Printf("[V2转V3] 代理组[%d] 代理列表: %v", i, pg.Proxies)
		}
	}

	// 打印每个解析到的规则集
	for i, rs := range rulesets {
		log.Printf("[V2转V3] 规则集[%d]: 组='%s', URL='%s', 行为='%s', 间隔=%d",
			i, rs.Group, rs.RuleURL, rs.Behavior, rs.Interval)
	}

	result := &TemplateV3Content{
		ProxyGroups:   make([]ProxyGroupV3Config, 0, len(proxyGroups)),
		Rules:         make([]string, 0),
		RuleProviders: make(map[string]RuleProviderConfig),
	}

	// Convert proxy groups
	for i, pg := range proxyGroups {
		log.Printf("[V2转V3] 转换代理组[%d]: '%s'", i, pg.Name)
		v3Group := convertProxyGroup(pg)
		log.Printf("[V2转V3] 转换结果[%d]: 名称='%s', 类型='%s', include-all=%v, include-all-proxies=%v, filter='%s', 代理数=%d",
			i, v3Group.Name, v3Group.Type, v3Group.IncludeAll, v3Group.IncludeAllProxies, v3Group.Filter, len(v3Group.Proxies))
		result.ProxyGroups = append(result.ProxyGroups, v3Group)
	}

	// Convert rulesets to rules and rule-providers
	convertRulesets(rulesets, result)

	log.Printf("[V2转V3] 转换完成: %d 个代理组, %d 条规则, %d 个规则提供者",
		len(result.ProxyGroups), len(result.Rules), len(result.RuleProviders))

	return result, nil
}

// convertProxyGroup converts an ACL proxy group to v3 format
func convertProxyGroup(pg ACLProxyGroup) ProxyGroupV3Config {
	log.Printf("[转换代理组] 开始转换: 名称='%s', 类型='%s'", pg.Name, pg.Type)

	v3 := ProxyGroupV3Config{
		Name: pg.Name,
		Type: pg.Type,
	}

	// 检查名称和类型是否为空
	if pg.Name == "" {
		log.Printf("[转换代理组] 警告: 代理组名称为空!")
	}
	if pg.Type == "" {
		log.Printf("[转换代理组] 警告: 代理组类型为空!")
	}

	// Separate regex filters from proxy references
	var regexFilters []string
	var proxyRefs []string

	for _, proxy := range pg.Proxies {
		if IsRegexProxyPattern(proxy) {
			log.Printf("[转换代理组] 识别为正则过滤器: '%s'", proxy)
			regexFilters = append(regexFilters, proxy)
		} else {
			log.Printf("[转换代理组] 识别为代理引用: '%s'", proxy)
			proxyRefs = append(proxyRefs, proxy)
		}
	}

	// Handle wildcard (.*) - means include all proxies
	if pg.HasWildcard {
		log.Printf("[转换代理组] 检测到通配符 .*, 设置 include-all=true")
		v3.IncludeAll = true
	}

	// Handle regex filters - convert to filter field
	if len(regexFilters) > 0 {
		// Merge all regex filters into one
		filter := MergeRegexFilters(regexFilters)
		log.Printf("[转换代理组] 合并正则过滤器: '%s'", filter)
		// Remove outer parentheses for v3 format
		filter = strings.TrimPrefix(strings.TrimSuffix(filter, ")"), "(")
		v3.Filter = filter
		log.Printf("[转换代理组] 最终过滤器: '%s'", v3.Filter)
		// If has filter but no include-all, set include-all-proxies
		if !v3.IncludeAll {
			log.Printf("[转换代理组] 设置 include-all-proxies=true (有过滤器但无通配符)")
			v3.IncludeAllProxies = true
		}
	}

	// Add proxy references (other groups)
	if len(proxyRefs) > 0 {
		v3.Proxies = proxyRefs
		log.Printf("[转换代理组] 添加代理引用: %v", proxyRefs)
	}

	// Add __PROXY_NODES__ marker if include-all or has filter
	if v3.IncludeAll || v3.IncludeAllProxies || v3.Filter != "" {
		if v3.Proxies == nil {
			v3.Proxies = []string{}
		}
		v3.Proxies = append(v3.Proxies, ProxyNodesMarker)
		log.Printf("[转换代理组] 添加代理节点标记: %s", ProxyNodesMarker)
	}

	// URL test options
	if pg.Type == "url-test" || pg.Type == "fallback" || pg.Type == "load-balance" {
		if pg.URL != "" {
			v3.URL = pg.URL
		} else {
			v3.URL = "https://cp.cloudflare.com/generate_204"
		}
		if pg.Interval > 0 {
			v3.Interval = pg.Interval
		} else {
			v3.Interval = 300
		}
		if pg.Tolerance > 0 && pg.Type != "load-balance" {
			v3.Tolerance = pg.Tolerance
		} else if pg.Type != "load-balance" {
			v3.Tolerance = 50
		}
		log.Printf("[转换代理组] URL测试参数: url='%s', interval=%d, tolerance=%d", v3.URL, v3.Interval, v3.Tolerance)
	}

	log.Printf("[转换代理组] 完成: 名称='%s', 类型='%s'", v3.Name, v3.Type)
	return v3
}

// convertRulesets converts ACL rulesets to v3 rules and rule-providers
func convertRulesets(rulesets []ACLRuleset, result *TemplateV3Content) {
	log.Printf("[转换规则集] 开始转换 %d 个规则集", len(rulesets))
	providerIndex := 0

	for i, rs := range rulesets {
		log.Printf("[转换规则集] 处理规则集[%d]: 组='%s', URL='%s'", i, rs.Group, rs.RuleURL)

		if strings.HasPrefix(rs.RuleURL, "[]") {
			// Inline rule: []GEOIP,CN or []GEOSITE,cn or []MATCH or []FINAL
			inlineRule := rs.RuleURL[2:] // Remove [] prefix

			// Handle special MATCH and FINAL rules
			upperRule := strings.ToUpper(inlineRule)
			if upperRule == "MATCH" || upperRule == "FINAL" {
				rule := "MATCH," + rs.Group
				result.Rules = append(result.Rules, rule)
				log.Printf("[转换规则集] MATCH 规则: %s (原始: %s)", rule, inlineRule)
				continue
			}

			// Parse rule parts: GEOIP,telegram,no-resolve -> [GEOIP, telegram, no-resolve]
			parts := strings.Split(inlineRule, ",")
			if len(parts) >= 2 {
				ruleType := strings.ToUpper(parts[0])
				ruleValue := parts[1]

				// Check for no-resolve suffix (usually the last part)
				var suffix string
				if len(parts) >= 3 {
					lastPart := strings.ToLower(parts[len(parts)-1])
					if lastPart == "no-resolve" {
						suffix = ",no-resolve"
					}
				}

				// Format: GEOIP,telegram,GroupName,no-resolve
				rule := ruleType + "," + ruleValue + "," + rs.Group + suffix
				result.Rules = append(result.Rules, rule)
				log.Printf("[转换规则集] 内联规则: %s", rule)
			} else {
				log.Printf("[转换规则集] 警告: 无法解析内联规则 '%s'", rs.RuleURL)
			}
		} else if strings.HasPrefix(rs.RuleURL, "http") {
			// External rule provider
			providerName := generateProviderName(rs.RuleURL, providerIndex)
			providerIndex++

			result.RuleProviders[providerName] = RuleProviderConfig{
				Type:     "http",
				Behavior: rs.Behavior,
				URL:      rs.RuleURL,
				Path:     "./providers/" + providerName + ".yaml",
				Interval: rs.Interval,
			}
			log.Printf("[转换规则集] 规则提供者: 名称='%s', URL='%s'", providerName, rs.RuleURL)

			// Add RULE-SET reference
			rule := "RULE-SET," + providerName + "," + rs.Group
			result.Rules = append(result.Rules, rule)
			log.Printf("[转换规则集] RULE-SET: %s", rule)
		} else {
			log.Printf("[转换规则集] 警告: 未知规则格式 '%s'", rs.RuleURL)
		}
	}

	// Add MATCH rule at the end if not present
	hasMatch := false
	for _, rule := range result.Rules {
		if strings.HasPrefix(rule, "MATCH,") {
			hasMatch = true
			break
		}
	}
	if !hasMatch && len(result.Rules) > 0 {
		// Find a suitable fallback group
		for _, pg := range result.ProxyGroups {
			if strings.Contains(pg.Name, "漏网") || strings.Contains(pg.Name, "其他") {
				result.Rules = append(result.Rules, "MATCH,"+pg.Name)
				log.Printf("[转换规则集] 添加默认 MATCH 规则: MATCH,%s", pg.Name)
				break
			}
		}
	}

	log.Printf("[转换规则集] 完成: %d 条规则, %d 个规则提供者", len(result.Rules), len(result.RuleProviders))
}

// generateProviderName generates a unique provider name from URL
func generateProviderName(url string, index int) string {
	// Extract filename from URL
	parts := strings.Split(url, "/")
	if len(parts) > 0 {
		filename := parts[len(parts)-1]
		// Remove extension
		filename = strings.TrimSuffix(filename, ".yaml")
		filename = strings.TrimSuffix(filename, ".list")
		// Clean up special characters
		re := regexp.MustCompile(`[^a-zA-Z0-9_-]`)
		filename = re.ReplaceAllString(filename, "_")
		if filename != "" {
			return filename
		}
	}
	return "provider_" + string(rune('0'+index))
}
