package substore

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// AdapterType represents the type of proxy adapter (matching mihomo's definition)
type AdapterType string

const (
	AdapterDirect       AdapterType = "direct"
	AdapterReject       AdapterType = "reject"
	AdapterRejectDrop   AdapterType = "reject-drop"
	AdapterCompatible   AdapterType = "compatible"
	AdapterPass         AdapterType = "pass"
	AdapterDns          AdapterType = "dns"
	AdapterRelay        AdapterType = "relay"
	AdapterSelector     AdapterType = "select"
	AdapterFallback     AdapterType = "fallback"
	AdapterURLTest      AdapterType = "url-test"
	AdapterLoadBalance  AdapterType = "load-balance"
	AdapterShadowsocks  AdapterType = "ss"
	AdapterShadowsocksR AdapterType = "ssr"
	AdapterSnell        AdapterType = "snell"
	AdapterSocks5       AdapterType = "socks5"
	AdapterHttp         AdapterType = "http"
	AdapterVmess        AdapterType = "vmess"
	AdapterVless        AdapterType = "vless"
	AdapterTrojan       AdapterType = "trojan"
	AdapterHysteria     AdapterType = "hysteria"
	AdapterHysteria2    AdapterType = "hysteria2"
	AdapterWireGuard    AdapterType = "wireguard"
	AdapterTuic         AdapterType = "tuic"
	AdapterSsh          AdapterType = "ssh"
	AdapterAnytls       AdapterType = "anytls"
)

// ProxyGroupV3 represents a proxy group with mihomo-style include/filter options
type ProxyGroupV3 struct {
	Name                string   `yaml:"name"`
	Type                string   `yaml:"type"`
	Proxies             []string `yaml:"proxies,omitempty"`
	Use                 []string `yaml:"use,omitempty"`                   // 引入代理集合
	IncludeAll          bool     `yaml:"include-all,omitempty"`           // 引入所有出站代理和代理集合
	IncludeType         string   `yaml:"include-type,omitempty"`          // 根据节点类型引入节点
	IncludeAllProxies   bool     `yaml:"include-all-proxies,omitempty"`   // 引入所有出站代理
	IncludeAllProviders bool     `yaml:"include-all-providers,omitempty"` // 引入所有代理集合
	Filter              string   `yaml:"filter,omitempty"`                // 筛选节点的正则表达式
	ExcludeFilter       string   `yaml:"exclude-filter,omitempty"`        // 排除节点的正则表达式
	ExcludeType         string   `yaml:"exclude-type,omitempty"`          // 根据类型排除节点
	URL                 string   `yaml:"url,omitempty"`
	Interval            int      `yaml:"interval,omitempty"`
	Tolerance           int      `yaml:"tolerance,omitempty"`
	Lazy                bool     `yaml:"lazy,omitempty"`
	DisableUDP          bool     `yaml:"disable-udp,omitempty"`
	Strategy            string   `yaml:"strategy,omitempty"`
	InterfaceName       string   `yaml:"interface-name,omitempty"`
	RoutingMark         int      `yaml:"routing-mark,omitempty"`
}

// ProxyNode represents a proxy node with its type
type ProxyNode struct {
	Name string
	Type string
}

// TemplateV3Processor processes v3 templates with mihomo-style proxy group options
type TemplateV3Processor struct {
	allProxies    []ProxyNode          // All available proxy nodes
	proxyGroups   []string             // Names of proxy groups (for reference)
	providers     map[string][]string  // Provider name -> proxy names
}

// NewTemplateV3Processor creates a new v3 template processor
func NewTemplateV3Processor(proxies []ProxyNode, providers map[string][]string) *TemplateV3Processor {
	return &TemplateV3Processor{
		allProxies:  proxies,
		providers:   providers,
		proxyGroups: []string{},
	}
}

// ProcessTemplate processes a v3 template and expands proxy groups
func (p *TemplateV3Processor) ProcessTemplate(templateContent string, proxies []map[string]any) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(templateContent), &root); err != nil {
		return "", err
	}

	// Extract proxy nodes from the provided proxies
	p.allProxies = extractProxyNodes(proxies)

	// Find and process proxy-groups
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		rootMap := root.Content[0]
		if rootMap.Kind == yaml.MappingNode {
			for i := 0; i < len(rootMap.Content); i += 2 {
				keyNode := rootMap.Content[i]
				valueNode := rootMap.Content[i+1]

				if keyNode.Value == "proxy-groups" && valueNode.Kind == yaml.SequenceNode {
					// First pass: collect all proxy group names
					p.collectProxyGroupNames(valueNode)
					// Second pass: process each proxy group
					if err := p.processProxyGroups(valueNode); err != nil {
						return "", err
					}
				}
			}
		}
	}

	// Marshal back to YAML
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return "", err
	}
	encoder.Close()

	// Post-process to convert Unicode escape sequences back to original characters
	result := unescapeUnicode(buf.String())
	return result, nil
}

// collectProxyGroupNames collects all proxy group names for reference
func (p *TemplateV3Processor) collectProxyGroupNames(groupsNode *yaml.Node) {
	p.proxyGroups = []string{}
	for _, groupNode := range groupsNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			for i := 0; i < len(groupNode.Content); i += 2 {
				if groupNode.Content[i].Value == "name" {
					p.proxyGroups = append(p.proxyGroups, groupNode.Content[i+1].Value)
					break
				}
			}
		}
	}
}

// processProxyGroups processes all proxy groups in the template
func (p *TemplateV3Processor) processProxyGroups(groupsNode *yaml.Node) error {
	for _, groupNode := range groupsNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			if err := p.processProxyGroup(groupNode); err != nil {
				return err
			}
		}
	}
	return nil
}

// processProxyGroup processes a single proxy group
func (p *TemplateV3Processor) processProxyGroup(groupNode *yaml.Node) error {
	group := p.parseProxyGroup(groupNode)

	// Calculate the final proxy list
	finalProxies := p.calculateProxies(group)

	// Update the proxies field in the YAML node
	p.updateProxiesInNode(groupNode, finalProxies)

	// Remove mihomo-specific fields that we've processed
	p.removeMihomoFields(groupNode)

	return nil
}

// parseProxyGroup parses a proxy group from YAML node
func (p *TemplateV3Processor) parseProxyGroup(groupNode *yaml.Node) ProxyGroupV3 {
	var group ProxyGroupV3

	for i := 0; i < len(groupNode.Content); i += 2 {
		key := groupNode.Content[i].Value
		valueNode := groupNode.Content[i+1]

		switch key {
		case "name":
			group.Name = valueNode.Value
		case "type":
			group.Type = valueNode.Value
		case "proxies":
			if valueNode.Kind == yaml.SequenceNode {
				for _, item := range valueNode.Content {
					group.Proxies = append(group.Proxies, item.Value)
				}
			}
		case "use":
			if valueNode.Kind == yaml.SequenceNode {
				for _, item := range valueNode.Content {
					group.Use = append(group.Use, item.Value)
				}
			}
		case "include-all":
			group.IncludeAll = valueNode.Value == "true"
		case "include-type":
			group.IncludeType = valueNode.Value
		case "include-all-proxies":
			group.IncludeAllProxies = valueNode.Value == "true"
		case "include-all-providers":
			group.IncludeAllProviders = valueNode.Value == "true"
		case "filter":
			group.Filter = valueNode.Value
		case "exclude-filter":
			group.ExcludeFilter = valueNode.Value
		case "exclude-type":
			group.ExcludeType = valueNode.Value
		case "url":
			group.URL = valueNode.Value
		case "interval":
			// Parse int from string
			if valueNode.Tag == "!!int" {
				group.Interval = parseInt(valueNode.Value)
			}
		case "tolerance":
			if valueNode.Tag == "!!int" {
				group.Tolerance = parseInt(valueNode.Value)
			}
		}
	}

	return group
}

// calculateProxies calculates the final proxy list based on include/filter options
func (p *TemplateV3Processor) calculateProxies(group ProxyGroupV3) []string {
	var result []string

	// Start with explicitly defined proxies
	result = append(result, group.Proxies...)

	// Check if any include option is set
	hasIncludeOption := group.IncludeAll || group.IncludeAllProxies || group.IncludeAllProviders ||
		group.IncludeType != "" || len(group.Use) > 0

	// Handle include-all (includes both proxies and providers)
	if group.IncludeAll {
		// Add all proxy nodes
		for _, proxy := range p.allProxies {
			result = append(result, proxy.Name)
		}
		// Add all provider proxies (if not include-all-providers=false)
		if !group.IncludeAllProviders {
			for _, providerProxies := range p.providers {
				result = append(result, providerProxies...)
			}
		}
	} else {
		// Handle include-all-proxies
		if group.IncludeAllProxies {
			for _, proxy := range p.allProxies {
				result = append(result, proxy.Name)
			}
		}

		// Handle include-type (filter by proxy type)
		if group.IncludeType != "" {
			types := parseTypeList(group.IncludeType)
			for _, proxy := range p.allProxies {
				if containsType(types, proxy.Type) {
					result = append(result, proxy.Name)
				}
			}
		}

		// Handle use (specific providers)
		if len(group.Use) > 0 && !group.IncludeAllProviders {
			for _, providerName := range group.Use {
				if providerProxies, ok := p.providers[providerName]; ok {
					result = append(result, providerProxies...)
				}
			}
		}

		// Handle include-all-providers
		if group.IncludeAllProviders {
			for _, providerProxies := range p.providers {
				result = append(result, providerProxies...)
			}
		}

		// If no include option is set but filter/exclude-filter is present,
		// implicitly include all proxies (mihomo behavior)
		if !hasIncludeOption && (group.Filter != "" || group.ExcludeFilter != "") {
			for _, proxy := range p.allProxies {
				result = append(result, proxy.Name)
			}
		}
	}

	// Apply filter (include matching)
	if group.Filter != "" {
		result = applyFilter(result, group.Filter)
	}

	// Apply exclude-filter (exclude matching)
	if group.ExcludeFilter != "" {
		result = applyExcludeFilter(result, group.ExcludeFilter)
	}

	// Apply exclude-type
	if group.ExcludeType != "" {
		excludeTypes := parseTypeList(group.ExcludeType)
		result = p.excludeByType(result, excludeTypes)
	}

	// Remove duplicates while preserving order
	result = removeDuplicates(result)

	return result
}

// updateProxiesInNode updates the proxies field in the YAML node
func (p *TemplateV3Processor) updateProxiesInNode(groupNode *yaml.Node, proxies []string) {
	// Find or create proxies field
	var proxiesIndex int = -1
	for i := 0; i < len(groupNode.Content); i += 2 {
		if groupNode.Content[i].Value == "proxies" {
			proxiesIndex = i + 1
			break
		}
	}

	// Create new proxies sequence node
	proxiesNode := &yaml.Node{
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
	}
	for _, proxyName := range proxies {
		node := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: proxyName,
		}
		// Check if string contains non-ASCII characters (like emoji)
		hasNonASCII := false
		for _, r := range proxyName {
			if r > 127 {
				hasNonASCII = true
				break
			}
		}
		if !hasNonASCII {
			node.Tag = "!!str"
		}
		proxiesNode.Content = append(proxiesNode.Content, node)
	}

	if proxiesIndex >= 0 {
		// Replace existing proxies
		groupNode.Content[proxiesIndex] = proxiesNode
	} else {
		// Add proxies field after name and type
		insertIndex := 4 // After name and type (2 key-value pairs = 4 nodes)
		if insertIndex > len(groupNode.Content) {
			insertIndex = len(groupNode.Content)
		}

		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: "proxies",
		}

		// Insert key and value
		newContent := make([]*yaml.Node, 0, len(groupNode.Content)+2)
		newContent = append(newContent, groupNode.Content[:insertIndex]...)
		newContent = append(newContent, keyNode, proxiesNode)
		newContent = append(newContent, groupNode.Content[insertIndex:]...)
		groupNode.Content = newContent
	}
}

// removeMihomoFields removes mihomo-specific fields from the proxy group
func (p *TemplateV3Processor) removeMihomoFields(groupNode *yaml.Node) {
	fieldsToRemove := map[string]bool{
		"use":                   true,
		"include-all":          true,
		"include-type":         true,
		"include-all-proxies":  true,
		"include-all-providers": true,
		"filter":               true,
		"exclude-filter":       true,
		"exclude-type":         true,
	}

	newContent := make([]*yaml.Node, 0, len(groupNode.Content))
	for i := 0; i < len(groupNode.Content); i += 2 {
		key := groupNode.Content[i].Value
		if !fieldsToRemove[key] {
			newContent = append(newContent, groupNode.Content[i], groupNode.Content[i+1])
		}
	}
	groupNode.Content = newContent
}

// excludeByType excludes proxies by their type
func (p *TemplateV3Processor) excludeByType(proxies []string, excludeTypes []string) []string {
	proxyTypeMap := make(map[string]string)
	for _, proxy := range p.allProxies {
		proxyTypeMap[proxy.Name] = proxy.Type
	}

	var result []string
	for _, proxyName := range proxies {
		proxyType, ok := proxyTypeMap[proxyName]
		if !ok || !containsType(excludeTypes, proxyType) {
			result = append(result, proxyName)
		}
	}
	return result
}

// Helper functions

func extractProxyNodes(proxies []map[string]any) []ProxyNode {
	var nodes []ProxyNode
	for _, proxy := range proxies {
		name, _ := proxy["name"].(string)
		proxyType, _ := proxy["type"].(string)
		if name != "" && proxyType != "" {
			nodes = append(nodes, ProxyNode{Name: name, Type: strings.ToLower(proxyType)})
		}
	}
	return nodes
}

func parseTypeList(typeStr string) []string {
	parts := strings.Split(typeStr, "|")
	var result []string
	for _, part := range parts {
		trimmed := strings.TrimSpace(strings.ToLower(part))
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func containsType(types []string, proxyType string) bool {
	proxyType = strings.ToLower(proxyType)
	for _, t := range types {
		if t == proxyType {
			return true
		}
	}
	return false
}

func applyFilter(proxies []string, filterPattern string) []string {
	// Filter pattern can contain multiple patterns separated by backtick
	patterns := strings.Split(filterPattern, "`")

	var result []string
	for _, proxyName := range proxies {
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			matched, err := regexp.MatchString(pattern, proxyName)
			if err == nil && matched {
				result = append(result, proxyName)
				break
			}
		}
	}
	return result
}

func applyExcludeFilter(proxies []string, excludePattern string) []string {
	// Exclude pattern can contain multiple patterns separated by backtick
	patterns := strings.Split(excludePattern, "`")

	var result []string
	for _, proxyName := range proxies {
		excluded := false
		for _, pattern := range patterns {
			pattern = strings.TrimSpace(pattern)
			if pattern == "" {
				continue
			}
			matched, err := regexp.MatchString(pattern, proxyName)
			if err == nil && matched {
				excluded = true
				break
			}
		}
		if !excluded {
			result = append(result, proxyName)
		}
	}
	return result
}

func removeDuplicates(items []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func parseInt(s string) int {
	var result int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			result = result*10 + int(c-'0')
		}
	}
	return result
}

// unescapeUnicode converts Unicode escape sequences back to original characters
// Handles both \uXXXX (BMP) and \UXXXXXXXX (supplementary planes like emoji)
func unescapeUnicode(s string) string {
	var result strings.Builder
	i := 0
	for i < len(s) {
		if i+1 < len(s) && s[i] == '\\' {
			if s[i+1] == 'U' && i+10 <= len(s) {
				// \UXXXXXXXX format (8 hex digits)
				hexStr := s[i+2 : i+10]
				if codePoint, ok := parseHexString(hexStr); ok {
					result.WriteRune(rune(codePoint))
					i += 10
					continue
				}
			} else if s[i+1] == 'u' && i+6 <= len(s) {
				// \uXXXX format (4 hex digits)
				hexStr := s[i+2 : i+6]
				if codePoint, ok := parseHexString(hexStr); ok {
					result.WriteRune(rune(codePoint))
					i += 6
					continue
				}
			}
		}
		result.WriteByte(s[i])
		i++
	}
	return result.String()
}

// parseHexString parses a hex string to an integer
func parseHexString(s string) (int64, bool) {
	var result int64
	for _, c := range s {
		result *= 16
		if c >= '0' && c <= '9' {
			result += int64(c - '0')
		} else if c >= 'a' && c <= 'f' {
			result += int64(c - 'a' + 10)
		} else if c >= 'A' && c <= 'F' {
			result += int64(c - 'A' + 10)
		} else {
			return 0, false
		}
	}
	return result, true
}
