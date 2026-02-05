package substore

import (
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// RegionProxyGroup defines a predefined region proxy group
type RegionProxyGroup struct {
	Name   string
	Filter string
}

// Predefined region proxy groups
var RegionProxyGroups = []RegionProxyGroup{
	{Name: "🇭🇰 香港节点", Filter: `🇭🇰|港|\bHK(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|hk|Hong Kong|HongKong|hongkong|HONG KONG|HONGKONG|深港|HKG|九龙|Kowloon|新界|沙田|荃湾|葵涌`},
	{Name: "🇺🇸 美国节点", Filter: `🇺🇸|美|波特兰|达拉斯|俄勒冈|凤凰城|费利蒙|硅谷|拉斯维加斯|洛杉矶|圣何塞|圣克拉拉|西雅图|芝加哥|纽约|纽纽|亚特兰大|迈阿密|华盛顿|\bUS(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|United States|UnitedStates|UNITED STATES|USA|America|AMERICA|JFK|EWR|IAD|ATL|ORD|MIA|NYC|LAX|SFO|SEA|DFW|SJC`},
	{Name: "🇯🇵 日本节点", Filter: `🇯🇵|日本|川日|东京|大阪|泉日|埼玉|沪日|深日|(?<!尼|-)日|\bJP(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Japan|JAPAN|JPN|NRT|HND|KIX|TYO|OSA|关西|Kansai|KANSAI`},
	{Name: "🇸🇬 新加坡节点", Filter: `🇸🇬|新加坡|坡|狮城|\bSG(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Singapore|SINGAPORE|SIN`},
	{Name: "🇼🇸 台湾节点", Filter: `🇹🇼|🇼🇸|台|新北|彰化|\bTW(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Taiwan|TAIWAN|TWN|TPE|ROC`},
	{Name: "🇰🇷 韩国节点", Filter: `🇰🇷|\bKR(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Korea|KOREA|KOR|首尔|韩|韓|春川|Chuncheon|ICN`},
	{Name: "🇨🇦 加拿大节点", Filter: `🇨🇦|加拿大|\bCA(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Canada|CANADA|CAN|渥太华|温哥华|卡尔加里|蒙特利尔|Montreal|YVR|YYZ|YUL`},
	{Name: "🇬🇧 英国节点", Filter: `🇬🇧|英国|Britain|United Kingdom|UNITED KINGDOM|England|伦敦|曼彻斯特|Manchester|\bUK(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|GBR|LHR|MAN`},
	{Name: "🇫🇷 法国节点", Filter: `🇫🇷|法国|\bFR(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|France|FRANCE|FRA|巴黎|马赛|Marseille|CDG|MRS`},
	{Name: "🇩🇪 德国节点", Filter: `🇩🇪|德国|Germany|GERMANY|\bDE(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|DEU|柏林|法兰克福|慕尼黑|Munich|MUC`},
	{Name: "🇳🇱 荷兰节点", Filter: `🇳🇱|荷兰|Netherlands|NETHERLANDS|\bNL(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|NLD|阿姆斯特丹|AMS`},
	{Name: "🇹🇷 土耳其节点", Filter: `🇹🇷|土耳其|Turkey|TURKEY|Türkiye|\bTR(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|TUR|IST|ANK`},
	{Name: "🌐 其他地区", Filter: `(^(?!.*(港|\bHK(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|hk|Hong Kong|HongKong|hongkong|HONG KONG|HONGKONG|深港|HKG|🇭🇰|九龙|Kowloon|新界|沙田|荃湾|葵涌|美|波特兰|达拉斯|俄勒冈|凤凰城|费利蒙|硅谷|拉斯维加斯|洛杉矶|圣何塞|圣克拉拉|西雅图|芝加哥|纽约|纽纽|亚特兰大|迈阿密|华盛顿|\bUS(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|United States|UnitedStates|UNITED STATES|USA|America|AMERICA|JFK|EWR|IAD|ATL|ORD|MIA|NYC|LAX|SFO|SEA|DFW|SJC|🇺🇸|日本|川日|东京|大阪|泉日|埼玉|沪日|深日|(?<!尼|-)日|\bJP(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Japan|JAPAN|JPN|NRT|HND|KIX|TYO|OSA|🇯🇵|关西|Kansai|KANSAI|新加坡|坡|狮城|\bSG(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Singapore|SINGAPORE|SIN|🇸🇬|台|新北|彰化|\bTW(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Taiwan|TAIWAN|TWN|TPE|ROC|🇹🇼|\bKR(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Korea|KOREA|KOR|首尔|韩|韓|春川|Chuncheon|ICN|🇰🇷|加拿大|\bCA(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|Canada|CANADA|CAN|渥太华|温哥华|卡尔加里|蒙特利尔|Montreal|YVR|YYZ|YUL|🇨🇦|英国|Britain|United Kingdom|UNITED KINGDOM|England|伦敦|曼彻斯特|Manchester|\bUK(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|GBR|LHR|MAN|🇬🇧|法国|\bFR(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|France|FRANCE|FRA|巴黎|马赛|Marseille|CDG|MRS|🇫🇷|德国|Germany|GERMANY|\bDE(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|DEU|柏林|法兰克福|慕尼黑|Munich|MUC|🇩🇪|荷兰|Netherlands|NETHERLANDS|\bNL(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|NLD|阿姆斯特丹|AMS|🇳🇱|土耳其|Turkey|TURKEY|Türkiye|\bTR(?:[-_ ]?\d+(?:[-_ ]?[A-Za-z]{2,})?)?\b|TUR|IST|ANK|🇹🇷)).*)`},
}

// Special markers for proxy order
const (
	ProxyNodesMarker        = "__PROXY_NODES__"
	ProxyProvidersMarker    = "__PROXY_PROVIDERS__"
	RegionProxyGroupsMarker = "__REGION_PROXY_GROUPS__"
)

// GetOtherRegionsExcludeFilter returns the exclude filter for "Other regions" group
func GetOtherRegionsExcludeFilter() string {
	var filters []string
	for _, r := range RegionProxyGroups {
		filters = append(filters, r.Filter)
	}
	return strings.Join(filters, "|")
}

// GetRegionProxyGroupNames returns all region proxy group names including "Other regions"
func GetRegionProxyGroupNames() []string {
	names := make([]string, 0, len(RegionProxyGroups)+1)
	for _, r := range RegionProxyGroups {
		names = append(names, r.Name)
	}
	names = append(names, "🌐 其他地区")
	return names
}

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
	Name                     string   `yaml:"name"`
	Type                     string   `yaml:"type"`
	Proxies                  []string `yaml:"proxies,omitempty"`
	Use                      []string `yaml:"use,omitempty"`                         // 引入代理集合
	IncludeAll               bool     `yaml:"include-all,omitempty"`                 // 引入所有出站代理和代理集合
	IncludeType              string   `yaml:"include-type,omitempty"`                // 根据节点类型引入节点
	IncludeAllProxies        bool     `yaml:"include-all-proxies,omitempty"`         // 引入所有出站代理
	IncludeAllProviders      bool     `yaml:"include-all-providers,omitempty"`       // 引入所有代理集合
	IncludeRegionProxyGroups bool     `yaml:"include-region-proxy-groups,omitempty"` // 引入地区代理组
	Filter                   string   `yaml:"filter,omitempty"`                      // 筛选节点的正则表达式
	ExcludeFilter            string   `yaml:"exclude-filter,omitempty"`              // 排除节点的正则表达式
	ExcludeType              string   `yaml:"exclude-type,omitempty"`                // 根据类型排除节点
	URL                      string   `yaml:"url,omitempty"`
	Interval                 int      `yaml:"interval,omitempty"`
	Tolerance                int      `yaml:"tolerance,omitempty"`
	Lazy                     bool     `yaml:"lazy,omitempty"`
	DisableUDP               bool     `yaml:"disable-udp,omitempty"`
	Strategy                 string   `yaml:"strategy,omitempty"`
	InterfaceName            string   `yaml:"interface-name,omitempty"`
	RoutingMark              int      `yaml:"routing-mark,omitempty"`
}

// ProxyNode represents a proxy node with its type
type ProxyNode struct {
	Name string
	Type string
}

// TemplateV3Processor processes v3 templates with mihomo-style proxy group options
type TemplateV3Processor struct {
	allProxies        []ProxyNode         // All available proxy nodes
	proxyGroups       []string            // Names of proxy groups (for reference)
	providers         map[string][]string // Provider name -> proxy names
	regionGroupsAdded bool                // Whether region proxy groups have been added
	regionGroupNames  []string            // Names of region proxy groups
}

// NewTemplateV3Processor creates a new v3 template processor
func NewTemplateV3Processor(proxies []ProxyNode, providers map[string][]string) *TemplateV3Processor {
	return &TemplateV3Processor{
		allProxies:        proxies,
		providers:         providers,
		proxyGroups:       []string{},
		regionGroupsAdded: false,
		regionGroupNames:  GetRegionProxyGroupNames(),
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

	// Track which proxy nodes are used (for adding to top-level proxies)
	usedProxyNames := make(map[string]bool)

	// Find and process proxy-groups
	if root.Kind == yaml.DocumentNode && len(root.Content) > 0 {
		rootMap := root.Content[0]
		if rootMap.Kind == yaml.MappingNode {
			var proxyGroupsIndex int = -1
			var addRegionProxyGroups bool = false

			// First pass: find proxy-groups and check for add-region-proxy-groups
			for i := 0; i < len(rootMap.Content); i += 2 {
				keyNode := rootMap.Content[i]
				valueNode := rootMap.Content[i+1]

				if keyNode.Value == "add-region-proxy-groups" {
					addRegionProxyGroups = valueNode.Value == "true"
				}
				if keyNode.Value == "proxy-groups" && valueNode.Kind == yaml.SequenceNode {
					proxyGroupsIndex = i + 1
				}
			}

			// Process proxy-groups if found
			if proxyGroupsIndex >= 0 {
				valueNode := rootMap.Content[proxyGroupsIndex]

				// Check if any proxy group has include-region-proxy-groups: true or __REGION_PROXY_GROUPS__ marker
				if !addRegionProxyGroups {
					addRegionProxyGroups = p.hasIncludeRegionProxyGroups(valueNode) || p.hasRegionProxyGroupsMarker(valueNode)
				}

				// If add-region-proxy-groups is true or any group has include-region-proxy-groups, insert region groups
				if addRegionProxyGroups {
					p.insertRegionProxyGroups(valueNode)
				}

				// Collect all proxy group names (including newly added region groups)
				p.collectProxyGroupNames(valueNode)

				// Process each proxy group
				if err := p.processProxyGroups(valueNode); err != nil {
					return "", err
				}

				// Collect used proxy names from processed proxy-groups
				usedProxyNames = p.collectUsedProxyNames(valueNode)

				// Collect proxies that belong to specific groups (for dialer-proxy)
				landingNodeProxies := p.collectProxiesByGroupName(valueNode, "🌄 落地节点")

				// Add or update top-level proxies with used proxy configs
				p.updateTopLevelProxies(rootMap, proxies, usedProxyNames, landingNodeProxies)
			}

			// Remove add-region-proxy-groups from output
			p.removeGlobalConfig(rootMap, "add-region-proxy-groups")
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

// hasIncludeRegionProxyGroups checks if any proxy group has include-region-proxy-groups: true
func (p *TemplateV3Processor) hasIncludeRegionProxyGroups(groupsNode *yaml.Node) bool {
	for _, groupNode := range groupsNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			for i := 0; i < len(groupNode.Content); i += 2 {
				if groupNode.Content[i].Value == "include-region-proxy-groups" {
					if groupNode.Content[i+1].Value == "true" {
						return true
					}
				}
			}
		}
	}
	return false
}

// hasRegionProxyGroupsMarker checks if any proxy group's proxies list contains __REGION_PROXY_GROUPS__ marker
func (p *TemplateV3Processor) hasRegionProxyGroupsMarker(groupsNode *yaml.Node) bool {
	for _, groupNode := range groupsNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			for i := 0; i < len(groupNode.Content); i += 2 {
				if groupNode.Content[i].Value == "proxies" {
					proxiesNode := groupNode.Content[i+1]
					if proxiesNode.Kind == yaml.SequenceNode {
						for _, item := range proxiesNode.Content {
							if item.Value == RegionProxyGroupsMarker {
								return true
							}
						}
					}
				}
			}
		}
	}
	return false
}

// insertRegionProxyGroups inserts predefined region proxy groups at the beginning
func (p *TemplateV3Processor) insertRegionProxyGroups(groupsNode *yaml.Node) {
	if p.regionGroupsAdded {
		return
	}

	var newGroups []*yaml.Node

	// Create region proxy groups
	for _, region := range RegionProxyGroups {
		groupNode := p.createRegionGroupNode(region.Name, region.Filter, "")
		newGroups = append(newGroups, groupNode)
	}

	// Create "Other regions" group with exclude filter
	otherRegionNode := p.createRegionGroupNode("🌐 其他地区", "", GetOtherRegionsExcludeFilter())
	newGroups = append(newGroups, otherRegionNode)

	// Prepend new groups to existing groups
	groupsNode.Content = append(newGroups, groupsNode.Content...)
	p.regionGroupsAdded = true
}

// createRegionGroupNode creates a YAML node for a region proxy group
func (p *TemplateV3Processor) createRegionGroupNode(name, filter, excludeFilter string) *yaml.Node {
	groupNode := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}

	// Add name
	groupNode.Content = append(groupNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "name"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: name},
	)

	// Add type (url-test)
	groupNode.Content = append(groupNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "type"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "url-test"},
	)

	// Add include-all-proxies
	groupNode.Content = append(groupNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "include-all-proxies"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
	)

	// Add filter or exclude-filter
	if filter != "" {
		groupNode.Content = append(groupNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "filter"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: filter},
		)
	}
	if excludeFilter != "" {
		groupNode.Content = append(groupNode.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "exclude-filter"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: excludeFilter},
		)
	}

	// Add url-test options
	groupNode.Content = append(groupNode.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "url"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "https://cp.cloudflare.com/generate_204"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "interval"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "300"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "tolerance"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "50"},
	)

	return groupNode
}

// removeGlobalConfig removes a global config key from the root map
func (p *TemplateV3Processor) removeGlobalConfig(rootMap *yaml.Node, key string) {
	newContent := make([]*yaml.Node, 0, len(rootMap.Content))
	for i := 0; i < len(rootMap.Content); i += 2 {
		if rootMap.Content[i].Value != key {
			newContent = append(newContent, rootMap.Content[i], rootMap.Content[i+1])
		}
	}
	rootMap.Content = newContent
}

// processProxyGroups processes all proxy groups in the template
func (p *TemplateV3Processor) processProxyGroups(groupsNode *yaml.Node) error {
	var newContent []*yaml.Node
	removedGroups := make(map[string]bool)

	// First pass: process each proxy group and identify empty ones
	for _, groupNode := range groupsNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			if err := p.processProxyGroup(groupNode); err != nil {
				return err
			}
			// Check if proxies is empty after processing
			if p.hasEmptyProxies(groupNode) {
				// Record the removed group name
				for i := 0; i < len(groupNode.Content); i += 2 {
					if groupNode.Content[i].Value == "name" {
						removedGroups[groupNode.Content[i+1].Value] = true
						break
					}
				}
			} else {
				newContent = append(newContent, groupNode)
			}
		}
	}

	// Second pass: remove references to removed groups from remaining groups
	if len(removedGroups) > 0 {
		for _, groupNode := range newContent {
			p.removeGroupReferences(groupNode, removedGroups)
		}
		// Update proxy group names list
		p.proxyGroups = p.filterProxyGroupNames(p.proxyGroups, removedGroups)
	}

	groupsNode.Content = newContent
	return nil
}

// removeGroupReferences removes references to removed groups from a proxy group's proxies list
func (p *TemplateV3Processor) removeGroupReferences(groupNode *yaml.Node, removedGroups map[string]bool) {
	for i := 0; i < len(groupNode.Content); i += 2 {
		if groupNode.Content[i].Value == "proxies" {
			proxiesNode := groupNode.Content[i+1]
			if proxiesNode.Kind == yaml.SequenceNode {
				var newProxies []*yaml.Node
				for _, proxyNode := range proxiesNode.Content {
					if !removedGroups[proxyNode.Value] {
						newProxies = append(newProxies, proxyNode)
					}
				}
				proxiesNode.Content = newProxies
			}
			break
		}
	}
}

// filterProxyGroupNames filters out removed group names from the list
func (p *TemplateV3Processor) filterProxyGroupNames(names []string, removedGroups map[string]bool) []string {
	var result []string
	for _, name := range names {
		if !removedGroups[name] {
			result = append(result, name)
		}
	}
	return result
}

// hasEmptyProxies checks if a proxy group has empty or no proxies
func (p *TemplateV3Processor) hasEmptyProxies(groupNode *yaml.Node) bool {
	for i := 0; i < len(groupNode.Content); i += 2 {
		if groupNode.Content[i].Value == "proxies" {
			valueNode := groupNode.Content[i+1]
			return valueNode.Kind == yaml.SequenceNode && len(valueNode.Content) == 0
		}
	}
	// No proxies field found, treat as empty
	return true
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
		case "include-region-proxy-groups":
			group.IncludeRegionProxyGroups = valueNode.Value == "true"
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

	// Calculate proxy nodes (from include-all-proxies, include-type, filter)
	proxyNodes := p.calculateProxyNodes(group)

	// Calculate proxy providers (from use, include-all-providers)
	proxyProviders := p.calculateProxyProviders(group)

	// Check if proxies list contains markers
	hasNodesMarker := false
	hasProvidersMarker := false
	hasRegionGroupsMarker := false
	for _, proxy := range group.Proxies {
		if proxy == ProxyNodesMarker {
			hasNodesMarker = true
		}
		if proxy == ProxyProvidersMarker {
			hasProvidersMarker = true
		}
		if proxy == RegionProxyGroupsMarker {
			hasRegionGroupsMarker = true
		}
	}

	// If markers are present, use them to determine order
	if hasNodesMarker || hasProvidersMarker || hasRegionGroupsMarker {
		for _, proxy := range group.Proxies {
			if proxy == ProxyNodesMarker {
				result = append(result, proxyNodes...)
			} else if proxy == ProxyProvidersMarker {
				result = append(result, proxyProviders...)
			} else if proxy == RegionProxyGroupsMarker {
				result = append(result, p.regionGroupNames...)
			} else {
				result = append(result, proxy)
			}
		}
	} else {
		// No markers, use default order: region groups (if enabled), proxies, then nodes, then providers
		if group.IncludeRegionProxyGroups {
			result = append(result, p.regionGroupNames...)
		}
		result = append(result, group.Proxies...)
		result = append(result, proxyNodes...)
		result = append(result, proxyProviders...)
	}

	// Apply filter (include matching) - only to proxy nodes, not to proxy groups
	if group.Filter != "" {
		result = applyFilterPreservingGroups(result, group.Filter, p.proxyGroups)
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

// calculateProxyNodes calculates proxy nodes from include options
func (p *TemplateV3Processor) calculateProxyNodes(group ProxyGroupV3) []string {
	var nodes []string

	// Check if explicit include option is set (not counting filter as include)
	hasExplicitInclude := group.IncludeAll || group.IncludeAllProxies || group.IncludeType != ""

	if group.IncludeAll || group.IncludeAllProxies {
		for _, proxy := range p.allProxies {
			nodes = append(nodes, proxy.Name)
		}
	} else if group.IncludeType != "" {
		types := parseTypeList(group.IncludeType)
		for _, proxy := range p.allProxies {
			if containsType(types, proxy.Type) {
				nodes = append(nodes, proxy.Name)
			}
		}
	} else if !hasExplicitInclude && (group.Filter != "" || group.ExcludeFilter != "") {
		// If no explicit include option is set but filter/exclude-filter is present,
		// implicitly include all proxies (mihomo behavior)
		for _, proxy := range p.allProxies {
			nodes = append(nodes, proxy.Name)
		}
	}

	return nodes
}

// calculateProxyProviders calculates proxy providers from include options
func (p *TemplateV3Processor) calculateProxyProviders(group ProxyGroupV3) []string {
	var providers []string

	if group.IncludeAll || group.IncludeAllProviders {
		for _, providerProxies := range p.providers {
			providers = append(providers, providerProxies...)
		}
	} else if len(group.Use) > 0 {
		for _, providerName := range group.Use {
			if providerProxies, ok := p.providers[providerName]; ok {
				providers = append(providers, providerProxies...)
			}
		}
	}

	return providers
}

// applyFilterPreservingGroups applies filter but preserves proxy group names
func applyFilterPreservingGroups(proxies []string, filterPattern string, proxyGroups []string) []string {
	patterns := strings.Split(filterPattern, "`")
	groupSet := make(map[string]bool)
	for _, g := range proxyGroups {
		groupSet[g] = true
	}

	var result []string
	for _, proxyName := range proxies {
		// Always keep proxy groups
		if groupSet[proxyName] {
			result = append(result, proxyName)
			continue
		}

		// Apply filter to non-group proxies
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
		"use":                         true,
		"include-all":                 true,
		"include-type":                true,
		"include-all-proxies":         true,
		"include-all-providers":       true,
		"include-region-proxy-groups": true,
		"filter":                      true,
		"exclude-filter":              true,
		"exclude-type":                true,
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

// collectUsedProxyNames collects all proxy names used in processed proxy-groups
func (p *TemplateV3Processor) collectUsedProxyNames(groupsNode *yaml.Node) map[string]bool {
	usedNames := make(map[string]bool)
	proxyGroupNames := make(map[string]bool)

	// First, collect all proxy group names
	for _, name := range p.proxyGroups {
		proxyGroupNames[name] = true
	}
	for _, name := range p.regionGroupNames {
		proxyGroupNames[name] = true
	}

	// Collect proxy names from each group (excluding group names and built-in keywords)
	for _, groupNode := range groupsNode.Content {
		if groupNode.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i < len(groupNode.Content); i += 2 {
			if groupNode.Content[i].Value == "proxies" {
				proxiesNode := groupNode.Content[i+1]
				if proxiesNode.Kind == yaml.SequenceNode {
					for _, proxyNode := range proxiesNode.Content {
						name := proxyNode.Value
						// Exclude group names and built-in keywords
						if !proxyGroupNames[name] && name != "DIRECT" && name != "REJECT" && name != "PASS" {
							usedNames[name] = true
						}
					}
				}
				break
			}
		}
	}

	return usedNames
}

// collectProxiesByGroupName collects proxy names that belong to a specific group
func (p *TemplateV3Processor) collectProxiesByGroupName(groupsNode *yaml.Node, targetGroupName string) map[string]bool {
	result := make(map[string]bool)
	proxyGroupNames := make(map[string]bool)

	// First, collect all proxy group names
	for _, name := range p.proxyGroups {
		proxyGroupNames[name] = true
	}
	for _, name := range p.regionGroupNames {
		proxyGroupNames[name] = true
	}

	// Find the target group and collect its proxy names
	for _, groupNode := range groupsNode.Content {
		if groupNode.Kind != yaml.MappingNode {
			continue
		}

		var groupName string
		var proxiesNode *yaml.Node

		for i := 0; i < len(groupNode.Content); i += 2 {
			key := groupNode.Content[i].Value
			if key == "name" {
				groupName = groupNode.Content[i+1].Value
			} else if key == "proxies" {
				proxiesNode = groupNode.Content[i+1]
			}
		}

		if groupName == targetGroupName && proxiesNode != nil && proxiesNode.Kind == yaml.SequenceNode {
			for _, proxyNode := range proxiesNode.Content {
				name := proxyNode.Value
				// Exclude group names and built-in keywords
				if !proxyGroupNames[name] && name != "DIRECT" && name != "REJECT" && name != "PASS" {
					result[name] = true
				}
			}
			break
		}
	}

	return result
}

// updateTopLevelProxies adds or updates the top-level proxies list with used proxy configs
func (p *TemplateV3Processor) updateTopLevelProxies(rootMap *yaml.Node, proxies []map[string]any, usedProxyNames map[string]bool, landingNodeProxies map[string]bool) {
	if len(usedProxyNames) == 0 {
		return
	}

	// Build a map of proxy name to config
	proxyConfigMap := make(map[string]map[string]any)
	for _, proxy := range proxies {
		if name, ok := proxy["name"].(string); ok {
			proxyConfigMap[name] = proxy
		}
	}

	// Create ordered list of proxies (maintain order from p.allProxies)
	// Add dialer-proxy to landing node proxies
	var orderedProxies []map[string]any
	for _, proxyNode := range p.allProxies {
		if usedProxyNames[proxyNode.Name] {
			if config, exists := proxyConfigMap[proxyNode.Name]; exists {
				// If this proxy belongs to landing nodes group, add dialer-proxy
				// Modify the original map so that injectProxiesIntoTemplate will also have the change
				if landingNodeProxies[proxyNode.Name] {
					config["dialer-proxy"] = "🌠 中转节点"
				}
				orderedProxies = append(orderedProxies, config)
			}
		}
	}

	if len(orderedProxies) == 0 {
		return
	}

	// Find existing proxies index or create new one
	var proxiesIndex int = -1
	for i := 0; i < len(rootMap.Content); i += 2 {
		if rootMap.Content[i].Value == "proxies" {
			proxiesIndex = i + 1
			break
		}
	}

	// Convert proxies to YAML nodes
	proxiesYAML, err := yaml.Marshal(orderedProxies)
	if err != nil {
		return
	}

	var proxiesNode yaml.Node
	if err := yaml.Unmarshal(proxiesYAML, &proxiesNode); err != nil {
		return
	}

	// proxiesNode is a DocumentNode containing the actual sequence
	if proxiesNode.Kind != yaml.DocumentNode || len(proxiesNode.Content) == 0 {
		return
	}
	actualProxiesNode := proxiesNode.Content[0]

	if proxiesIndex >= 0 {
		// Replace existing proxies
		rootMap.Content[proxiesIndex] = actualProxiesNode
	} else {
		// Insert proxies after dns (or at the beginning if dns not found)
		insertIndex := 0
		for i := 0; i < len(rootMap.Content); i += 2 {
			key := rootMap.Content[i].Value
			// Insert after common config keys
			if key == "dns" || key == "mode" || key == "log-level" || key == "allow-lan" || key == "port" || key == "socks-port" {
				insertIndex = i + 2
			}
		}

		// Create key node
		keyNode := &yaml.Node{
			Kind:  yaml.ScalarNode,
			Value: "proxies",
		}

		// Insert at the calculated position
		newContent := make([]*yaml.Node, 0, len(rootMap.Content)+2)
		newContent = append(newContent, rootMap.Content[:insertIndex]...)
		newContent = append(newContent, keyNode, actualProxiesNode)
		newContent = append(newContent, rootMap.Content[insertIndex:]...)
		rootMap.Content = newContent
	}
}
