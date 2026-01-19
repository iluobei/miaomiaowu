package substore

import (
	"strings"
	"testing"
)

func TestConvertLogLevel(t *testing.T) {
	tests := []struct {
		clashLevel string
		expected   string
	}{
		{"silent", "notify"},
		{"error", "error"},
		{"warning", "warning"},
		{"warn", "warning"},
		{"info", "notify"},
		{"debug", "verbose"},
		{"unknown", "notify"},
		{"", "notify"},
	}

	for _, tt := range tests {
		result := convertLogLevel(tt.clashLevel)
		if result != tt.expected {
			t.Errorf("convertLogLevel(%q) = %q, expected %q", tt.clashLevel, result, tt.expected)
		}
	}
}

func TestConvertDNSServer(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"223.5.5.5", "223.5.5.5"},
		{"https://223.5.5.5/dns-query", "223.5.5.5"},
		{"https://dns.google/dns-query", "dns.google"},
		{"tls://223.5.5.5", "223.5.5.5"},
		{"quic://223.5.5.5", "223.5.5.5"},
		{"1.1.1.1", "1.1.1.1"},
	}

	for _, tt := range tests {
		result := convertDNSServer(tt.input)
		if result != tt.expected {
			t.Errorf("convertDNSServer(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestConvertExternalController(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"0.0.0.0:9090", "password@0.0.0.0:9090"},
		{"127.0.0.1:9090", "password@127.0.0.1:9090"},
		{"secret@0.0.0.0:9090", "secret@0.0.0.0:9090"},
	}

	for _, tt := range tests {
		result := convertExternalController(tt.input)
		if result != tt.expected {
			t.Errorf("convertExternalController(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}

func TestConvertProxyGroupType(t *testing.T) {
	tests := []struct {
		clashType string
		expected  string
	}{
		{"select", "select"},
		{"url-test", "url-test"},
		{"fallback", "fallback"},
		{"load-balance", "load-balance"},
		{"relay", "select"},
		{"unknown", "select"},
	}

	for _, tt := range tests {
		result := convertProxyGroupType(tt.clashType)
		if result != tt.expected {
			t.Errorf("convertProxyGroupType(%q) = %q, expected %q", tt.clashType, result, tt.expected)
		}
	}
}

func TestConvertClashDNSToSurge(t *testing.T) {
	dns := ClashDNS{
		Enable: true,
		DefaultNameserver: []string{
			"tls://223.5.5.5",
		},
		Nameserver: []string{
			"https://223.5.5.5/dns-query",
			"https://119.29.29.29/dns-query",
		},
	}

	opts := &SurgeTemplateConfig{}
	result := convertClashDNSToSurge(dns, opts)

	if len(result) == 0 {
		t.Error("Expected non-empty DNS server list")
	}

	// Should contain extracted IPs
	expectedServers := map[string]bool{
		"223.5.5.5":   true,
		"119.29.29.29": true,
	}

	for _, server := range result {
		if !expectedServers[server] {
			t.Errorf("Unexpected DNS server: %s", server)
		}
	}
}

func TestBuildSurgeGeneral(t *testing.T) {
	clashConfig := &ClashConfig{
		Port:               7890,
		SocksPort:          7891,
		LogLevel:           "info",
		ExternalController: "0.0.0.0:9090",
		DNS: ClashDNS{
			Enable: true,
			Nameserver: []string{
				"https://223.5.5.5/dns-query",
			},
		},
	}

	opts := &SurgeTemplateConfig{}
	applyDefaultSurgeConfig(opts)

	result := buildSurgeGeneral(clashConfig, opts)

	// Check for required sections
	requiredStrings := []string{
		"[General]",
		"loglevel = notify",
		"skip-proxy =",
		"dns-server =",
		"http-listen = 0.0.0.0:7890",
		"socks5-listen = 0.0.0.0:7891",
	}

	for _, req := range requiredStrings {
		if !strings.Contains(result, req) {
			t.Errorf("Expected result to contain %q, got:\n%s", req, result)
		}
	}
}

func TestConvertClashRulesToSurgeFormat(t *testing.T) {
	rules := []string{
		"DOMAIN-SUFFIX,google.com,Proxy",
		"DOMAIN,example.com,DIRECT",
		"IP-CIDR,192.168.0.0/16,DIRECT",
		"GEOIP,CN,DIRECT",
		"MATCH,Proxy",
		"RULE-SET,telegram,Proxy",
	}

	ruleProviders := map[string]ClashRuleProvider{
		"telegram": {
			Type:     "http",
			Behavior: "classical",
			URL:      "https://example.com/telegram.list",
		},
	}

	result, err := ConvertClashRulesToSurgeFormat(rules, ruleProviders)
	if err != nil {
		t.Fatalf("ConvertClashRulesToSurgeFormat failed: %v", err)
	}

	// Check conversions
	expectedRules := []string{
		"DOMAIN-SUFFIX,google.com,Proxy",
		"DOMAIN,example.com,DIRECT",
		"IP-CIDR,192.168.0.0/16,DIRECT",
		"GEOIP,CN,DIRECT",
		"FINAL,Proxy", // MATCH -> FINAL
		"RULE-SET,https://example.com/telegram.list,Proxy",
	}

	if len(result) != len(expectedRules) {
		t.Errorf("Expected %d rules, got %d", len(expectedRules), len(result))
	}

	for i, expected := range expectedRules {
		if i >= len(result) {
			break
		}
		if result[i] != expected {
			t.Errorf("Rule %d: expected %q, got %q", i, expected, result[i])
		}
	}
}

func TestConvertClashProxyGroupsToSurge(t *testing.T) {
	groups := []ClashProxyGroup{
		{
			Name:    "Auto",
			Type:    "url-test",
			Proxies: []string{"Proxy1", "Proxy2"},
			URL:     "http://www.gstatic.com/generate_204",
			Interval: 300,
		},
		{
			Name:    "Select",
			Type:    "select",
			Proxies: []string{"Auto", "DIRECT"},
		},
		{
			Name:    "Relay",
			Type:    "relay",
			Proxies: []string{"Proxy1", "Proxy2"},
		},
	}

	result := ConvertClashProxyGroupsToSurge(groups)

	if len(result) != 3 {
		t.Fatalf("Expected 3 groups, got %d", len(result))
	}

	// Check Auto group
	if result[0].Name != "Auto" || result[0].Type != "url-test" {
		t.Errorf("Auto group conversion failed")
	}

	// Check Relay -> Select conversion
	if result[2].Name != "Relay" || result[2].Type != "select" {
		t.Errorf("Relay group should be converted to select, got: %s", result[2].Type)
	}
}

func TestBuildCompleteSurgeConfig(t *testing.T) {
	clashConfig := &ClashConfig{
		Port:      7890,
		SocksPort: 7891,
		LogLevel:  "info",
		DNS: ClashDNS{
			Enable: true,
			Nameserver: []string{
				"https://223.5.5.5/dns-query",
			},
		},
		ProxyGroups: []ClashProxyGroup{
			{
				Name:    "Proxy",
				Type:    "select",
				Proxies: []string{"DIRECT"},
			},
		},
		Rules: []string{
			"DOMAIN-SUFFIX,google.com,Proxy",
			"GEOIP,CN,DIRECT",
			"MATCH,Proxy",
		},
		RuleProviders: map[string]ClashRuleProvider{},
	}

	proxies := []Proxy{
		{
			"name":   "TestProxy",
			"type":   "ss",
			"server": "1.2.3.4",
			"port":   8388,
			"cipher": "aes-256-gcm",
			"password": "password",
		},
	}

	templateOpts := &SurgeTemplateConfig{}
	applyDefaultSurgeConfig(templateOpts)

	result, err := BuildCompleteSurgeConfig(clashConfig, proxies, templateOpts, false)
	if err != nil {
		t.Fatalf("BuildCompleteSurgeConfig failed: %v", err)
	}

	// Check for all sections
	requiredSections := []string{
		"[General]",
		"[Proxy]",
		"[Proxy Group]",
		"[Rule]",
	}

	for _, section := range requiredSections {
		if !strings.Contains(result, section) {
			t.Errorf("Expected result to contain section %q", section)
		}
	}

	// Check for DIRECT proxy
	if !strings.Contains(result, "DIRECT = direct") {
		t.Error("Expected DIRECT proxy in result")
	}

	// Check for TestProxy
	if !strings.Contains(result, "TestProxy=ss") {
		t.Error("Expected TestProxy in result")
	}

	// Check for FINAL rule (converted from MATCH)
	if !strings.Contains(result, "FINAL,Proxy") {
		t.Error("Expected FINAL rule in result")
	}
}

func TestApplyDefaultSurgeConfig(t *testing.T) {
	opts := &SurgeTemplateConfig{}
	applyDefaultSurgeConfig(opts)

	if opts.LogLevel != "notify" {
		t.Errorf("Expected default LogLevel to be 'notify', got %q", opts.LogLevel)
	}

	if opts.TestTimeout != 5 {
		t.Errorf("Expected default TestTimeout to be 5, got %d", opts.TestTimeout)
	}

	if !opts.BypassSystem {
		t.Error("Expected BypassSystem to be true")
	}

	if !opts.HTTPAPIWebDashboard {
		t.Error("Expected HTTPAPIWebDashboard to be true")
	}
}

func TestContains(t *testing.T) {
	slice := []string{"apple", "banana", "cherry"}

	if !contains(slice, "banana") {
		t.Error("Expected contains to return true for 'banana'")
	}

	if contains(slice, "grape") {
		t.Error("Expected contains to return false for 'grape'")
	}
}
