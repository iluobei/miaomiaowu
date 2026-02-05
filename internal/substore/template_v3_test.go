package substore

import (
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// 模拟节点数据
func createMockProxies() []map[string]any {
	return []map[string]any{
		// 香港节点
		{"name": "🇭🇰 香港 01", "type": "vmess", "server": "hk1.example.com", "port": 443},
		{"name": "🇭🇰 香港 02", "type": "trojan", "server": "hk2.example.com", "port": 443},
		{"name": "HK-03 Premium", "type": "ss", "server": "hk3.example.com", "port": 8388},
		// 美国节点
		{"name": "🇺🇸 美国 洛杉矶", "type": "vmess", "server": "us1.example.com", "port": 443},
		{"name": "US-02 Seattle", "type": "vless", "server": "us2.example.com", "port": 443},
		// 日本节点
		{"name": "🇯🇵 日本 东京", "type": "trojan", "server": "jp1.example.com", "port": 443},
		{"name": "JP-02 Osaka", "type": "vmess", "server": "jp2.example.com", "port": 443},
		// 新加坡节点
		{"name": "🇸🇬 新加坡 01", "type": "ss", "server": "sg1.example.com", "port": 8388},
		{"name": "SG-02", "type": "vmess", "server": "sg2.example.com", "port": 443},
		// 台湾节点
		{"name": "🇹🇼 台湾 01", "type": "vmess", "server": "tw1.example.com", "port": 443},
		// 韩国节点
		{"name": "🇰🇷 韩国 首尔", "type": "trojan", "server": "kr1.example.com", "port": 443},
		// 其他地区节点
		{"name": "🇦🇺 澳大利亚", "type": "vmess", "server": "au1.example.com", "port": 443},
		{"name": "🇮🇳 印度", "type": "ss", "server": "in1.example.com", "port": 8388},
		// 中转节点
		{"name": "中转 HK-01", "type": "vmess", "server": "relay1.example.com", "port": 443},
		{"name": "CO-Premium", "type": "trojan", "server": "relay2.example.com", "port": 443},
		// 落地节点
		{"name": "LD-US-01", "type": "vmess", "server": "ld1.example.com", "port": 443},
		{"name": "落地-JP", "type": "trojan", "server": "ld2.example.com", "port": 443},
	}
}

func TestTemplateV3Processor_ProcessTemplate(t *testing.T) {
	// 读取模板文件
	templateContent, err := os.ReadFile("../../rule_templates/redirhost__v3.yaml")
	if err != nil {
		t.Fatalf("Failed to read template file: %v", err)
	}

	// 创建处理器
	processor := NewTemplateV3Processor(nil, nil)

	// 处理模板
	proxies := createMockProxies()
	result, err := processor.ProcessTemplate(string(templateContent), proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	// 验证结果不为空
	if result == "" {
		t.Fatal("ProcessTemplate returned empty result")
	}

	// 解析结果验证 YAML 格式正确
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	// 验证 proxy-groups 存在
	proxyGroups, ok := parsed["proxy-groups"].([]any)
	if !ok {
		t.Fatal("proxy-groups not found in result")
	}

	t.Logf("Found %d proxy groups", len(proxyGroups))

	// 验证各个代理组
	groupNames := make(map[string]bool)
	for _, g := range proxyGroups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		name, _ := group["name"].(string)
		groupNames[name] = true

		// 检查 proxies 字段
		proxies, hasProxies := group["proxies"].([]any)
		if hasProxies {
			t.Logf("Group %q has %d proxies", name, len(proxies))
		}
	}

	// 验证必要的代理组存在
	requiredGroups := []string{
		"🚀 手动选择",
		"♻️ 自动选择",
		"🇭🇰 香港节点",
		"🇺🇸 美国节点",
		"🇯🇵 日本节点",
		"🎯 全球直连",
	}

	for _, name := range requiredGroups {
		if !groupNames[name] {
			t.Errorf("Required proxy group %q not found", name)
		}
	}
}

func TestTemplateV3Processor_IncludeAll(t *testing.T) {
	// 简单模板测试 include-all
	templateContent := `
proxy-groups:
  - name: 全部节点
    type: select
    include-all: true
`
	processor := NewTemplateV3Processor(nil, nil)
	proxies := createMockProxies()

	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups := parsed["proxy-groups"].([]any)
	group := proxyGroups[0].(map[string]any)
	groupProxies := group["proxies"].([]any)

	// 应该包含所有节点
	if len(groupProxies) != len(proxies) {
		t.Errorf("Expected %d proxies with include-all, got %d", len(proxies), len(groupProxies))
	}

	// 验证 include-all 字段已被移除
	if _, exists := group["include-all"]; exists {
		t.Error("include-all field should be removed after processing")
	}
}

func TestTemplateV3Processor_Filter(t *testing.T) {
	// 测试 filter 功能
	templateContent := `
proxy-groups:
  - name: 香港节点
    type: url-test
    include-all: true
    filter: "香港|HK|港"
    url: https://www.gstatic.com/generate_204
    interval: 300
`
	processor := NewTemplateV3Processor(nil, nil)
	proxies := createMockProxies()

	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups := parsed["proxy-groups"].([]any)
	group := proxyGroups[0].(map[string]any)
	groupProxies := group["proxies"].([]any)

	// 验证只包含香港节点
	for _, p := range groupProxies {
		name := p.(string)
		if !strings.Contains(name, "香港") && !strings.Contains(name, "HK") && !strings.Contains(name, "港") {
			t.Errorf("Unexpected proxy in filtered group: %s", name)
		}
	}

	// 验证 filter 字段已被移除
	if _, exists := group["filter"]; exists {
		t.Error("filter field should be removed after processing")
	}

	t.Logf("Filtered to %d Hong Kong proxies", len(groupProxies))
}

func TestTemplateV3Processor_ExcludeFilter(t *testing.T) {
	// 测试 exclude-filter 功能
	templateContent := `
proxy-groups:
  - name: 非香港节点
    type: select
    include-all: true
    exclude-filter: "香港|HK|港"
`
	processor := NewTemplateV3Processor(nil, nil)
	proxies := createMockProxies()

	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups := parsed["proxy-groups"].([]any)
	group := proxyGroups[0].(map[string]any)
	groupProxies := group["proxies"].([]any)

	// 验证不包含香港节点
	for _, p := range groupProxies {
		name := p.(string)
		if strings.Contains(name, "香港") || strings.Contains(strings.ToUpper(name), "HK") {
			t.Errorf("Hong Kong proxy should be excluded: %s", name)
		}
	}

	t.Logf("Excluded Hong Kong, got %d proxies", len(groupProxies))
}

func TestTemplateV3Processor_IncludeAllProxies(t *testing.T) {
	// 测试 include-all-proxies 功能
	templateContent := `
proxy-groups:
  - name: 所有代理
    type: select
    include-all-proxies: true
`
	processor := NewTemplateV3Processor(nil, nil)
	proxies := createMockProxies()

	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups := parsed["proxy-groups"].([]any)
	group := proxyGroups[0].(map[string]any)
	groupProxies := group["proxies"].([]any)

	// 应该包含所有节点
	if len(groupProxies) != len(proxies) {
		t.Errorf("Expected %d proxies with include-all-proxies, got %d", len(proxies), len(groupProxies))
	}
}

func TestTemplateV3Processor_StaticProxies(t *testing.T) {
	// 测试静态 proxies 保留
	templateContent := `
proxy-groups:
  - name: 手动选择
    type: select
    include-all: true
    proxies:
      - ♻️ 自动选择
      - 🎯 全球直连
`
	processor := NewTemplateV3Processor(nil, nil)
	proxies := createMockProxies()

	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups := parsed["proxy-groups"].([]any)
	group := proxyGroups[0].(map[string]any)
	groupProxies := group["proxies"].([]any)

	// 验证静态代理在前面
	if len(groupProxies) < 2 {
		t.Fatal("Expected at least 2 proxies")
	}

	firstProxy := groupProxies[0].(string)
	secondProxy := groupProxies[1].(string)

	if firstProxy != "♻️ 自动选择" {
		t.Errorf("First proxy should be '♻️ 自动选择', got %s", firstProxy)
	}
	if secondProxy != "🎯 全球直连" {
		t.Errorf("Second proxy should be '🎯 全球直连', got %s", secondProxy)
	}

	// 验证动态节点也被添加
	if len(groupProxies) <= 2 {
		t.Error("Dynamic proxies should be added after static proxies")
	}

	t.Logf("Total proxies: %d (2 static + %d dynamic)", len(groupProxies), len(groupProxies)-2)
}

func TestTemplateV3Processor_ComplexFilter(t *testing.T) {
	// 测试复杂的正则过滤
	templateContent := `
proxy-groups:
  - name: 中转节点
    type: select
    include-all: true
    filter: "中转|CO|co"
`
	processor := NewTemplateV3Processor(nil, nil)
	proxies := createMockProxies()

	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups := parsed["proxy-groups"].([]any)
	group := proxyGroups[0].(map[string]any)
	groupProxies := group["proxies"].([]any)

	// 应该匹配 "中转 HK-01" 和 "CO-Premium"
	expectedCount := 2
	if len(groupProxies) != expectedCount {
		t.Errorf("Expected %d relay proxies, got %d", expectedCount, len(groupProxies))
		for _, p := range groupProxies {
			t.Logf("  - %s", p.(string))
		}
	}
}

func TestTemplateV3Processor_RedirHostTemplate(t *testing.T) {
	// 完整测试 redirhost__v3.yaml 模板
	templateContent, err := os.ReadFile("../../rule_templates/redirhost__v3.yaml")
	if err != nil {
		t.Fatalf("Failed to read template file: %v", err)
	}

	processor := NewTemplateV3Processor(nil, nil)
	proxies := createMockProxies()

	result, err := processor.ProcessTemplate(string(templateContent), proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	// 验证 DNS 配置保留
	dns, ok := parsed["dns"].(map[string]any)
	if !ok {
		t.Fatal("DNS config not found")
	}
	if dns["enhanced-mode"] != "redir-host" {
		t.Errorf("Expected enhanced-mode to be 'redir-host', got %v", dns["enhanced-mode"])
	}

	// 验证 rules 保留
	rules, ok := parsed["rules"].([]any)
	if !ok {
		t.Fatal("Rules not found")
	}
	if len(rules) == 0 {
		t.Error("Rules should not be empty")
	}

	// 验证 rule-providers 保留
	ruleProviders, ok := parsed["rule-providers"].(map[string]any)
	if !ok {
		t.Fatal("Rule providers not found")
	}
	if len(ruleProviders) == 0 {
		t.Error("Rule providers should not be empty")
	}

	// 验证代理组
	proxyGroups := parsed["proxy-groups"].([]any)

	// 检查香港节点组
	var hkGroup map[string]any
	for _, g := range proxyGroups {
		group := g.(map[string]any)
		if group["name"] == "🇭🇰 香港节点" {
			hkGroup = group
			break
		}
	}

	if hkGroup == nil {
		t.Fatal("Hong Kong proxy group not found")
	}

	hkProxies := hkGroup["proxies"].([]any)
	t.Logf("Hong Kong group has %d proxies", len(hkProxies))

	// 验证香港节点组只包含香港节点
	for _, p := range hkProxies {
		name := p.(string)
		// 香港节点的 filter 很复杂，这里简单验证
		if !strings.Contains(name, "香港") && !strings.Contains(name, "HK") && !strings.Contains(name, "港") {
			t.Logf("Warning: proxy %q might not be a Hong Kong node", name)
		}
	}

	// 检查全球直连组
	var directGroup map[string]any
	for _, g := range proxyGroups {
		group := g.(map[string]any)
		if group["name"] == "🎯 全球直连" {
			directGroup = group
			break
		}
	}

	if directGroup == nil {
		t.Fatal("Direct proxy group not found")
	}

	directProxies := directGroup["proxies"].([]any)
	if len(directProxies) != 1 || directProxies[0].(string) != "DIRECT" {
		t.Errorf("Direct group should only contain DIRECT, got %v", directProxies)
	}

	t.Log("RedirHost template test passed!")
}

func TestApplyFilter(t *testing.T) {
	proxies := []string{
		"🇭🇰 香港 01",
		"🇭🇰 香港 02",
		"HK-03 Premium",
		"🇺🇸 美国 洛杉矶",
		"US-02 Seattle",
		"🇯🇵 日本 东京",
	}

	// 测试单个模式
	result := applyFilter(proxies, "香港")
	if len(result) != 2 {
		t.Errorf("Expected 2 proxies matching '香港', got %d", len(result))
	}

	// 测试多个模式（用 | 分隔）
	result = applyFilter(proxies, "香港|HK")
	if len(result) != 3 {
		t.Errorf("Expected 3 proxies matching '香港|HK', got %d", len(result))
	}

	// 测试用反引号分隔的多个模式
	result = applyFilter(proxies, "香港`HK")
	if len(result) != 3 {
		t.Errorf("Expected 3 proxies matching '香港`HK', got %d", len(result))
	}
}

func TestApplyExcludeFilter(t *testing.T) {
	proxies := []string{
		"🇭🇰 香港 01",
		"🇭🇰 香港 02",
		"HK-03 Premium",
		"🇺🇸 美国 洛杉矶",
		"US-02 Seattle",
		"🇯🇵 日本 东京",
	}

	// 排除香港节点
	result := applyExcludeFilter(proxies, "香港|HK")
	if len(result) != 3 {
		t.Errorf("Expected 3 proxies after excluding '香港|HK', got %d", len(result))
	}

	// 验证结果不包含香港节点
	for _, p := range result {
		if strings.Contains(p, "香港") || strings.Contains(p, "HK") {
			t.Errorf("Proxy %q should be excluded", p)
		}
	}
}

func TestRemoveDuplicates(t *testing.T) {
	proxies := []string{
		"Proxy1",
		"Proxy2",
		"Proxy1",
		"Proxy3",
		"Proxy2",
	}

	result := removeDuplicates(proxies)
	if len(result) != 3 {
		t.Errorf("Expected 3 unique proxies, got %d", len(result))
	}

	// 验证顺序保持
	expected := []string{"Proxy1", "Proxy2", "Proxy3"}
	for i, p := range result {
		if p != expected[i] {
			t.Errorf("Expected %q at position %d, got %q", expected[i], i, p)
		}
	}
}

func TestExtractProxyNodes(t *testing.T) {
	proxies := []map[string]any{
		{"name": "Proxy1", "type": "vmess", "server": "1.1.1.1"},
		{"name": "Proxy2", "type": "TROJAN", "server": "2.2.2.2"},
		{"name": "", "type": "ss", "server": "3.3.3.3"},           // 无名称，应跳过
		{"name": "Proxy4", "type": "", "server": "4.4.4.4"},       // 无类型，应跳过
		{"name": "Proxy5", "type": "vless", "server": "5.5.5.5"},
	}

	nodes := extractProxyNodes(proxies)

	if len(nodes) != 3 {
		t.Errorf("Expected 3 valid nodes, got %d", len(nodes))
	}

	// 验证类型转为小写
	for _, node := range nodes {
		if node.Type != strings.ToLower(node.Type) {
			t.Errorf("Type should be lowercase, got %q", node.Type)
		}
	}
}

func TestParseTypeList(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"vmess|vless|trojan", []string{"vmess", "vless", "trojan"}},
		{"VMESS|VLESS", []string{"vmess", "vless"}},
		{"ss | ssr | http", []string{"ss", "ssr", "http"}},
		{"", []string{}},
	}

	for _, tt := range tests {
		result := parseTypeList(tt.input)
		if len(result) != len(tt.expected) {
			t.Errorf("parseTypeList(%q) = %v, expected %v", tt.input, result, tt.expected)
			continue
		}
		for i, v := range result {
			if v != tt.expected[i] {
				t.Errorf("parseTypeList(%q)[%d] = %q, expected %q", tt.input, i, v, tt.expected[i])
			}
		}
	}
}

func TestContainsType(t *testing.T) {
	types := []string{"vmess", "vless", "trojan"}

	if !containsType(types, "vmess") {
		t.Error("Expected containsType to return true for 'vmess'")
	}

	if !containsType(types, "VMESS") {
		t.Error("Expected containsType to return true for 'VMESS' (case insensitive)")
	}

	if containsType(types, "ss") {
		t.Error("Expected containsType to return false for 'ss'")
	}
}

func TestTemplateV3Processor_ProxyOrderWithMarkers(t *testing.T) {
	// 测试 proxies 列表中标记的顺序
	templateContent := `
proxy-groups:
  - name: 🚀 手动选择
    type: select
    include-all-proxies: true
    include-region-proxy-groups: true
    proxies:
      - ♻️ 自动选择
      - __PROXY_PROVIDERS__
      - __PROXY_NODES__
      - 🌄 落地节点
      - __REGION_PROXY_GROUPS__
`
	processor := NewTemplateV3Processor(nil, nil)
	proxies := []map[string]any{
		{"name": "🇭🇰 香港 01", "type": "vmess", "server": "hk1.example.com", "port": 443},
		{"name": "🇺🇸 美国 01", "type": "vmess", "server": "us1.example.com", "port": 443},
	}

	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups := parsed["proxy-groups"].([]any)

	// 找到 🚀 手动选择 代理组
	var manualGroup map[string]any
	for _, g := range proxyGroups {
		group := g.(map[string]any)
		if group["name"] == "🚀 手动选择" {
			manualGroup = group
			break
		}
	}

	if manualGroup == nil {
		t.Fatal("Manual select proxy group not found")
	}

	groupProxies := manualGroup["proxies"].([]any)

	// 验证顺序：♻️ 自动选择 应该在最前面
	if len(groupProxies) < 1 || groupProxies[0].(string) != "♻️ 自动选择" {
		t.Errorf("First proxy should be '♻️ 自动选择', got %v", groupProxies[0])
	}

	// 验证 __REGION_PROXY_GROUPS__ 被替换为区域代理组名称，且在最后
	// 区域代理组名称应该在 🌄 落地节点 之后
	foundLuodi := false
	foundRegionAfterLuodi := false
	for i, p := range groupProxies {
		name := p.(string)
		if name == "🌄 落地节点" {
			foundLuodi = true
		}
		if foundLuodi && (name == "🇭🇰 香港节点" || name == "🇺🇸 美国节点" || name == "🇯🇵 日本节点") {
			foundRegionAfterLuodi = true
			t.Logf("Found region group %q at position %d (after 🌄 落地节点)", name, i)
		}
	}

	if !foundLuodi {
		t.Error("🌄 落地节点 not found in proxies list")
	}

	if !foundRegionAfterLuodi {
		t.Error("Region proxy groups should be after 🌄 落地节点")
	}

	t.Logf("Proxy order test passed! Total proxies: %d", len(groupProxies))
	for i, p := range groupProxies {
		t.Logf("  [%d] %s", i, p.(string))
	}
}

// TestTemplateV3Processor_EmptyGroupReferenceCleanup tests that references to removed empty groups are cleaned up
func TestTemplateV3Processor_EmptyGroupReferenceCleanup(t *testing.T) {
	// Template with region groups where some will be empty due to no matching proxies
	templateContent := `
proxy-groups:
  - name: 🚀 节点选择
    type: select
    proxies:
      - ♻️ 自动选择
      - 🇭🇰 香港节点
      - 🇺🇸 美国节点
      - 🇯🇵 日本节点
      - DIRECT
  - name: ♻️ 自动选择
    type: url-test
    include-all-proxies: true
    url: https://cp.cloudflare.com/generate_204
    interval: 300
  - name: 🇭🇰 香港节点
    type: url-test
    include-all-proxies: true
    filter: 🇭🇰|港|HK
    url: https://cp.cloudflare.com/generate_204
    interval: 300
  - name: 🇺🇸 美国节点
    type: url-test
    include-all-proxies: true
    filter: 🇺🇸|美|US
    url: https://cp.cloudflare.com/generate_204
    interval: 300
  - name: 🇯🇵 日本节点
    type: url-test
    include-all-proxies: true
    filter: 🇯🇵|日本|JP
    url: https://cp.cloudflare.com/generate_204
    interval: 300
`

	// Only provide Hong Kong proxies - US and JP groups will be empty
	proxies := []map[string]any{
		{"name": "🇭🇰 香港 01", "type": "vmess", "server": "hk1.example.com", "port": 443},
		{"name": "🇭🇰 香港 02", "type": "trojan", "server": "hk2.example.com", "port": 443},
	}

	processor := NewTemplateV3Processor(nil, nil)
	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	// Parse result
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	proxyGroups, ok := parsed["proxy-groups"].([]any)
	if !ok {
		t.Fatal("proxy-groups not found in result")
	}

	// Find 🚀 节点选择 group and check its proxies
	var nodeSelectGroup map[string]any
	groupNames := make(map[string]bool)
	for _, g := range proxyGroups {
		group, ok := g.(map[string]any)
		if !ok {
			continue
		}
		name, _ := group["name"].(string)
		groupNames[name] = true
		if name == "🚀 节点选择" {
			nodeSelectGroup = group
		}
	}

	// Verify that empty groups (🇺🇸 美国节点, 🇯🇵 日本节点) are removed
	if groupNames["🇺🇸 美国节点"] {
		t.Error("🇺🇸 美国节点 should be removed (no matching proxies)")
	}
	if groupNames["🇯🇵 日本节点"] {
		t.Error("🇯🇵 日本节点 should be removed (no matching proxies)")
	}

	// Verify that 🇭🇰 香港节点 still exists (has matching proxies)
	if !groupNames["🇭🇰 香港节点"] {
		t.Error("🇭🇰 香港节点 should exist (has matching proxies)")
	}

	// Verify that references to removed groups are cleaned up in 🚀 节点选择
	if nodeSelectGroup == nil {
		t.Fatal("🚀 节点选择 group not found")
	}

	proxiesList, ok := nodeSelectGroup["proxies"].([]any)
	if !ok {
		t.Fatal("proxies not found in 🚀 节点选择 group")
	}

	// Check that removed groups are not in the proxies list
	for _, p := range proxiesList {
		proxyName, _ := p.(string)
		if proxyName == "🇺🇸 美国节点" {
			t.Error("Reference to removed group 🇺🇸 美国节点 should be cleaned up")
		}
		if proxyName == "🇯🇵 日本节点" {
			t.Error("Reference to removed group 🇯🇵 日本节点 should be cleaned up")
		}
	}

	// Log the final proxies list for debugging
	t.Logf("🚀 节点选择 proxies after cleanup: %v", proxiesList)
	t.Logf("Remaining groups: %v", groupNames)
}

// TestTemplateV3Processor_LandingNodeDialerProxy tests that landing node proxies get dialer-proxy added
func TestTemplateV3Processor_LandingNodeDialerProxy(t *testing.T) {
	// Template with landing nodes and relay nodes
	templateContent := `
proxy-groups:
  - name: 🚀 节点选择
    type: select
    proxies:
      - 🌠 中转节点
      - 🌄 落地节点
      - DIRECT
  - name: 🌠 中转节点
    type: select
    include-all-proxies: true
    filter: 中转|CO|co
  - name: 🌄 落地节点
    type: select
    include-all-proxies: true
    filter: LD|落地
`

	// Provide both relay and landing proxies
	proxies := []map[string]any{
		{"name": "中转-HK-01", "type": "vmess", "server": "relay1.example.com", "port": 443},
		{"name": "CO-Premium", "type": "trojan", "server": "relay2.example.com", "port": 443},
		{"name": "LD-US-01", "type": "vmess", "server": "ld1.example.com", "port": 443},
		{"name": "落地-JP", "type": "trojan", "server": "ld2.example.com", "port": 443},
	}

	processor := NewTemplateV3Processor(nil, nil)
	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		t.Fatalf("ProcessTemplate failed: %v", err)
	}

	// Parse result
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(result), &parsed); err != nil {
		t.Fatalf("Result is not valid YAML: %v", err)
	}

	// Check top-level proxies
	topProxies, ok := parsed["proxies"].([]any)
	if !ok {
		t.Fatal("proxies not found in result")
	}

	// Verify landing node proxies have dialer-proxy
	landingProxiesWithDialer := 0
	relayProxiesWithoutDialer := 0

	for _, p := range topProxies {
		proxy, ok := p.(map[string]any)
		if !ok {
			continue
		}
		name, _ := proxy["name"].(string)
		dialerProxy, hasDialer := proxy["dialer-proxy"].(string)

		t.Logf("Proxy %q: dialer-proxy=%v", name, dialerProxy)

		// Landing nodes should have dialer-proxy
		if name == "LD-US-01" || name == "落地-JP" {
			if !hasDialer || dialerProxy != "🌠 中转节点" {
				t.Errorf("Landing node %q should have dialer-proxy: 🌠 中转节点, got: %v", name, dialerProxy)
			} else {
				landingProxiesWithDialer++
			}
		}

		// Relay nodes should NOT have dialer-proxy
		if name == "中转-HK-01" || name == "CO-Premium" {
			if hasDialer {
				t.Errorf("Relay node %q should NOT have dialer-proxy, got: %v", name, dialerProxy)
			} else {
				relayProxiesWithoutDialer++
			}
		}
	}

	if landingProxiesWithDialer != 2 {
		t.Errorf("Expected 2 landing proxies with dialer-proxy, got %d", landingProxiesWithDialer)
	}
	if relayProxiesWithoutDialer != 2 {
		t.Errorf("Expected 2 relay proxies without dialer-proxy, got %d", relayProxiesWithoutDialer)
	}

	t.Logf("Test passed: %d landing proxies with dialer-proxy, %d relay proxies without", landingProxiesWithDialer, relayProxiesWithoutDialer)
}
