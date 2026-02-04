package substore

import (
	"testing"
)

func TestParseACLConfig(t *testing.T) {
	// Sample ACL4SSR config content - using backtick as separator
	content := "; 节点组\n" +
		"[custom]\n" +
		";自动测速\n" +
		"custom_proxy_group=🚀 节点选择`select`[]🎯 全球直连`[]♻️ 自动选择`[]🇭🇰 香港节点`[]🇯🇵 日本节点`[]🇺🇸 美国节点\n" +
		"custom_proxy_group=♻️ 自动选择`url-test`.*`http://www.gstatic.com/generate_204`300,,50\n" +
		"custom_proxy_group=🇭🇰 香港节点`url-test`(港|HK|Hong Kong)`http://www.gstatic.com/generate_204`300,,50\n" +
		"custom_proxy_group=🇯🇵 日本节点`url-test`(日本|JP|Japan)`http://www.gstatic.com/generate_204`300,,50\n" +
		"custom_proxy_group=🇺🇸 美国节点`url-test`(美|US|USA)`http://www.gstatic.com/generate_204`300,,50\n" +
		"custom_proxy_group=🎯 全球直连`select`[]DIRECT\n" +
		"\n" +
		";规则集\n" +
		"ruleset=🎯 全球直连,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/LocalAreaNetwork.list\n" +
		"ruleset=🛑 广告拦截,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/BanAD.list\n" +
		"ruleset=🚀 节点选择,https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/ProxyMedia.list\n" +
		"ruleset=🎯 全球直连,[]GEOIP,CN\n" +
		"ruleset=🚀 节点选择,[]MATCH\n"

	rulesets, proxyGroups := ParseACLConfig(content)

	// Test proxy groups
	t.Logf("Found %d proxy groups", len(proxyGroups))
	for i, pg := range proxyGroups {
		t.Logf("Proxy Group[%d]: Name='%s', Type='%s', HasWildcard=%v, Proxies=%v",
			i, pg.Name, pg.Type, pg.HasWildcard, pg.Proxies)
	}

	if len(proxyGroups) != 6 {
		t.Errorf("Expected 6 proxy groups, got %d", len(proxyGroups))
	}

	// Test specific proxy groups
	expectedGroups := []struct {
		name        string
		groupType   string
		hasWildcard bool
	}{
		{"🚀 节点选择", "select", false},
		{"♻️ 自动选择", "url-test", true},
		{"🇭🇰 香港节点", "url-test", false},
		{"🇯🇵 日本节点", "url-test", false},
		{"🇺🇸 美国节点", "url-test", false},
		{"🎯 全球直连", "select", false},
	}

	for i, expected := range expectedGroups {
		if i >= len(proxyGroups) {
			t.Errorf("Missing proxy group at index %d", i)
			continue
		}
		pg := proxyGroups[i]
		if pg.Name != expected.name {
			t.Errorf("Proxy group[%d] name: expected '%s', got '%s'", i, expected.name, pg.Name)
		}
		if pg.Type != expected.groupType {
			t.Errorf("Proxy group[%d] type: expected '%s', got '%s'", i, expected.groupType, pg.Type)
		}
		if pg.HasWildcard != expected.hasWildcard {
			t.Errorf("Proxy group[%d] hasWildcard: expected %v, got %v", i, expected.hasWildcard, pg.HasWildcard)
		}
	}

	// Test rulesets
	t.Logf("Found %d rulesets", len(rulesets))
	for i, rs := range rulesets {
		t.Logf("Ruleset[%d]: Group='%s', URL='%s', Behavior='%s'", i, rs.Group, rs.RuleURL, rs.Behavior)
	}

	if len(rulesets) != 5 {
		t.Errorf("Expected 5 rulesets, got %d", len(rulesets))
	}
}

func TestConvertACLToV3(t *testing.T) {
	content := "custom_proxy_group=🚀 节点选择`select`[]🎯 全球直连`[]♻️ 自动选择\n" +
		"custom_proxy_group=♻️ 自动选择`url-test`.*`http://www.gstatic.com/generate_204`300,,50\n" +
		"custom_proxy_group=🇭🇰 香港节点`url-test`(港|HK|Hong Kong)`http://www.gstatic.com/generate_204`300,,50\n" +
		"custom_proxy_group=🎯 全球直连`select`[]DIRECT\n" +
		"\n" +
		"ruleset=🎯 全球直连,[]GEOIP,CN\n" +
		"ruleset=🚀 节点选择,[]MATCH\n"

	result, err := ConvertACLToV3(content)
	if err != nil {
		t.Fatalf("ConvertACLToV3 failed: %v", err)
	}

	t.Logf("Converted %d proxy groups, %d rules, %d rule providers",
		len(result.ProxyGroups), len(result.Rules), len(result.RuleProviders))

	for i, pg := range result.ProxyGroups {
		t.Logf("V3 Proxy Group[%d]: Name='%s', Type='%s', IncludeAll=%v, IncludeAllProxies=%v, Filter='%s', Proxies=%v",
			i, pg.Name, pg.Type, pg.IncludeAll, pg.IncludeAllProxies, pg.Filter, pg.Proxies)
	}

	// Verify proxy groups have valid names and types
	for i, pg := range result.ProxyGroups {
		if pg.Name == "" {
			t.Errorf("V3 Proxy group[%d] has empty name", i)
		}
		if pg.Type == "" {
			t.Errorf("V3 Proxy group[%d] '%s' has empty type", i, pg.Name)
		}
	}

	// Verify rules
	for i, rule := range result.Rules {
		t.Logf("Rule[%d]: %s", i, rule)
	}
}

func TestConvertRulesWithNoResolve(t *testing.T) {
	content := "custom_proxy_group=🚀 手动选择`select`[]DIRECT\n" +
		"custom_proxy_group=🎯 全球直连`select`[]DIRECT\n" +
		"\n" +
		"ruleset=🚀 手动选择,[]GEOSITE,gfw\n" +
		"ruleset=🚀 手动选择,[]GEOIP,telegram,no-resolve\n" +
		"ruleset=🚀 手动选择,[]GEOIP,facebook,no-resolve\n" +
		"ruleset=🚀 手动选择,[]GEOIP,twitter,no-resolve\n" +
		"ruleset=🎯 全球直连,[]FINAL\n"

	result, err := ConvertACLToV3(content)
	if err != nil {
		t.Fatalf("ConvertACLToV3 failed: %v", err)
	}

	expectedRules := []string{
		"GEOSITE,gfw,🚀 手动选择",
		"GEOIP,telegram,🚀 手动选择,no-resolve",
		"GEOIP,facebook,🚀 手动选择,no-resolve",
		"GEOIP,twitter,🚀 手动选择,no-resolve",
		"MATCH,🎯 全球直连",
	}

	if len(result.Rules) != len(expectedRules) {
		t.Errorf("Expected %d rules, got %d", len(expectedRules), len(result.Rules))
	}

	for i, expected := range expectedRules {
		if i >= len(result.Rules) {
			t.Errorf("Missing rule at index %d: expected '%s'", i, expected)
			continue
		}
		if result.Rules[i] != expected {
			t.Errorf("Rule[%d]: expected '%s', got '%s'", i, expected, result.Rules[i])
		}
	}

	// Log all rules for debugging
	t.Log("Generated rules:")
	for i, rule := range result.Rules {
		t.Logf("  [%d]: %s", i, rule)
	}
}
