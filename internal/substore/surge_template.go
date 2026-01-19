package substore

import (
	"fmt"
	"strings"
)

// ClashConfig represents the structure of a Clash configuration
type ClashConfig struct {
	Port                int                       `yaml:"port"`
	SocksPort           int                       `yaml:"socks-port"`
	AllowLan            bool                      `yaml:"allow-lan"`
	Mode                string                    `yaml:"mode"`
	LogLevel            string                    `yaml:"log-level"`
	ExternalController  string                    `yaml:"external-controller"`
	DNS                 ClashDNS                  `yaml:"dns"`
	Proxies             []map[string]any          `yaml:"proxies"`
	ProxyGroups         []ClashProxyGroup         `yaml:"proxy-groups"`
	Rules               []string                  `yaml:"rules"`
	RuleProviders       map[string]ClashRuleProvider `yaml:"rule-providers"`
}

// ClashDNS represents Clash DNS configuration
type ClashDNS struct {
	Enable                 bool                       `yaml:"enable"`
	IPv6                   bool                       `yaml:"ipv6"`
	EnhancedMode           string                     `yaml:"enhanced-mode"`
	FakeIPRange            string                     `yaml:"fake-ip-range"`
	FakeIPFilter           []string                   `yaml:"fake-ip-filter"`
	DefaultNameserver      []string                   `yaml:"default-nameserver"`
	Nameserver             []string                   `yaml:"nameserver"`
	NameserverPolicy       map[string]any             `yaml:"nameserver-policy"`
	ProxyServerNameserver  []string                   `yaml:"proxy-server-nameserver"`
	RespectRules           bool                       `yaml:"respect-rules"`
}

// ClashProxyGroup represents a Clash proxy group
type ClashProxyGroup struct {
	Name      string   `yaml:"name"`
	Type      string   `yaml:"type"`
	Proxies   []string `yaml:"proxies"`
	URL       string   `yaml:"url"`
	Interval  int      `yaml:"interval"`
	Tolerance int      `yaml:"tolerance"`
	Strategy  string   `yaml:"strategy"`
	Lazy      bool     `yaml:"lazy"`
}

// ClashRuleProvider represents a Clash rule provider
type ClashRuleProvider struct {
	Type     string `yaml:"type"`
	Behavior string `yaml:"behavior"`
	URL      string `yaml:"url"`
	Path     string `yaml:"path"`
	Interval int    `yaml:"interval"`
	Format   string `yaml:"format"`
}

// SurgeTemplateConfig represents configuration for Surge template conversion
type SurgeTemplateConfig struct {
	// General settings
	LogLevel              string
	BypassSystem          bool
	SkipProxy             string
	BypassTun             string
	DNSServer             []string
	ExternalController    string
	HTTPAPIPort           string
	TestTimeout           int
	HTTPListenPort        int
	Socks5ListenPort      int

	// Advanced settings
	ExcludeSimpleHostnames bool
	AllowWiFiAccess        bool
	HTTPAPIWebDashboard    bool
}

// ConvertClashToSurgeConfig converts a Clash configuration to Surge format
// This handles the template parts (General, DNS) while proxies are handled by surge.go
func ConvertClashToSurgeConfig(clashConfig *ClashConfig, opts *SurgeTemplateConfig) (string, error) {
	if opts == nil {
		opts = &SurgeTemplateConfig{}
		applyDefaultSurgeConfig(opts)
	}

	var sections []string

	// [General] section
	general := buildSurgeGeneral(clashConfig, opts)
	sections = append(sections, general)

	// Note: [Proxy] and [Proxy Group] sections should be added by caller
	// as they use surge.go for proxy conversion

	return strings.Join(sections, "\n\n"), nil
}

// buildSurgeGeneral builds the [General] section from Clash config
func buildSurgeGeneral(clashConfig *ClashConfig, opts *SurgeTemplateConfig) string {
	var lines []string
	lines = append(lines, "[General]")

	// Log level
	logLevel := convertLogLevel(clashConfig.LogLevel)
	if opts.LogLevel != "" {
		logLevel = opts.LogLevel
	}
	lines = append(lines, fmt.Sprintf("loglevel = %s", logLevel))

	// Bypass system
	if opts.BypassSystem {
		lines = append(lines, "bypass-system = true")
	}

	// Skip proxy (bypass rules for simple domains)
	skipProxy := opts.SkipProxy
	if skipProxy == "" {
		skipProxy = "127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,100.64.0.0/10,localhost,*.local"
	}
	lines = append(lines, fmt.Sprintf("skip-proxy = %s", skipProxy))

	// Bypass TUN
	if opts.BypassTun != "" {
		lines = append(lines, fmt.Sprintf("bypass-tun = %s", opts.BypassTun))
	}

	// DNS servers
	dnsServers := convertClashDNSToSurge(clashConfig.DNS, opts)
	if len(dnsServers) > 0 {
		lines = append(lines, fmt.Sprintf("dns-server = %s", strings.Join(dnsServers, ",")))
	}

	// External controller
	if clashConfig.ExternalController != "" {
		externalCtrl := convertExternalController(clashConfig.ExternalController)
		lines = append(lines, fmt.Sprintf("external-controller-access = %s", externalCtrl))
	} else if opts.ExternalController != "" {
		lines = append(lines, fmt.Sprintf("external-controller-access = %s", opts.ExternalController))
	}

	// HTTP API
	if opts.HTTPAPIPort != "" {
		lines = append(lines, fmt.Sprintf("http-api = %s", opts.HTTPAPIPort))
	}

	// Test timeout
	testTimeout := opts.TestTimeout
	if testTimeout <= 0 {
		testTimeout = 5
	}
	lines = append(lines, fmt.Sprintf("test-timeout = %d", testTimeout))

	// HTTP API Web Dashboard
	if opts.HTTPAPIWebDashboard {
		lines = append(lines, "http-api-web-dashboard = true")
	}

	// Exclude simple hostnames
	if opts.ExcludeSimpleHostnames {
		lines = append(lines, "exclude-simple-hostnames = true")
	}

	// Allow WiFi access
	if opts.AllowWiFiAccess {
		lines = append(lines, "allow-wifi-access = true")
	}

	// HTTP and SOCKS5 listen ports
	if clashConfig.Port > 0 || opts.HTTPListenPort > 0 {
		httpPort := clashConfig.Port
		if opts.HTTPListenPort > 0 {
			httpPort = opts.HTTPListenPort
		}
		lines = append(lines, fmt.Sprintf("http-listen = 0.0.0.0:%d", httpPort))
		lines = append(lines, fmt.Sprintf("wifi-access-http-port = %d", httpPort))
	}

	if clashConfig.SocksPort > 0 || opts.Socks5ListenPort > 0 {
		socksPort := clashConfig.SocksPort
		if opts.Socks5ListenPort > 0 {
			socksPort = opts.Socks5ListenPort
		}
		lines = append(lines, fmt.Sprintf("socks5-listen = 0.0.0.0:%d", socksPort))
		lines = append(lines, fmt.Sprintf("wifi-access-socks5-port = %d", socksPort))
	}

	return strings.Join(lines, "\n")
}

// convertClashDNSToSurge converts Clash DNS configuration to Surge DNS servers
func convertClashDNSToSurge(dns ClashDNS, opts *SurgeTemplateConfig) []string {
	// If user provided custom DNS, use it
	if len(opts.DNSServer) > 0 {
		return opts.DNSServer
	}

	var dnsServers []string

	// Process default nameservers first (these are used for resolving DoH/DoT servers)
	for _, ns := range dns.DefaultNameserver {
		converted := convertDNSServer(ns)
		if converted != "" {
			dnsServers = append(dnsServers, converted)
		}
	}

	// Process main nameservers
	for _, ns := range dns.Nameserver {
		converted := convertDNSServer(ns)
		if converted != "" {
			// Avoid duplicates
			if !contains(dnsServers, converted) {
				dnsServers = append(dnsServers, converted)
			}
		}
	}

	// If no DNS servers found, use defaults
	if len(dnsServers) == 0 {
		dnsServers = []string{"223.5.5.5", "119.29.29.29"}
	}

	return dnsServers
}

// convertDNSServer converts various DNS server formats
func convertDNSServer(server string) string {
	server = strings.TrimSpace(server)

	// Remove DoH/DoT/DoQ prefixes for Surge (Surge uses plain DNS in dns-server)
	// Surge supports DoH/DoT in encrypted-dns-server, but for simplicity, extract IP
	if after, found := strings.CutPrefix(server, "https://"); found {
		// Extract IP from DoH URL
		// e.g., "https://223.5.5.5/dns-query" -> "223.5.5.5"
		server = after
		if idx := strings.Index(server, "/"); idx > 0 {
			server = server[:idx]
		}
	} else if after, found := strings.CutPrefix(server, "tls://"); found {
		// e.g., "tls://223.5.5.5" -> "223.5.5.5"
		server = after
	} else if after, found := strings.CutPrefix(server, "quic://"); found {
		server = after
	}

	return server
}

// convertLogLevel converts Clash log level to Surge log level
func convertLogLevel(clashLevel string) string {
	switch strings.ToLower(clashLevel) {
	case "silent":
		return "notify"
	case "error":
		return "error"
	case "warning", "warn":
		return "warning"
	case "info":
		return "notify"
	case "debug":
		return "verbose"
	default:
		return "notify"
	}
}

// convertExternalController converts Clash external-controller to Surge format
func convertExternalController(controller string) string {
	// Clash format: "0.0.0.0:9090" or "127.0.0.1:9090"
	// Surge format: "password@0.0.0.0:6170"

	// If already has password, return as-is
	if strings.Contains(controller, "@") {
		return controller
	}

	// Add default password
	return "password@" + controller
}

// ConvertClashRulesToSurgeFormat converts Clash rules array to Surge rules
func ConvertClashRulesToSurgeFormat(rules []string, ruleProviders map[string]ClashRuleProvider) ([]string, error) {
	var surgeRules []string

	for _, rule := range rules {
		parts := strings.Split(rule, ",")
		if len(parts) < 2 {
			continue
		}

		ruleType := strings.TrimSpace(parts[0])

		switch ruleType {
		case "DOMAIN", "DOMAIN-SUFFIX", "DOMAIN-KEYWORD", "IP-CIDR", "IP-CIDR6", "GEOIP", "SRC-IP-CIDR", "SRC-PORT", "DST-PORT", "PROCESS-NAME":
			// These rules are compatible between Clash and Surge
			surgeRules = append(surgeRules, rule)

		case "MATCH":
			// Clash MATCH -> Surge FINAL
			if len(parts) >= 2 {
				surgeRules = append(surgeRules, fmt.Sprintf("FINAL,%s", strings.TrimSpace(parts[1])))
			}

		case "RULE-SET":
			// Convert RULE-SET reference
			if len(parts) >= 3 {
				ruleSetName := strings.TrimSpace(parts[1])
				policy := strings.TrimSpace(parts[2])

				// Look up rule provider
				if provider, ok := ruleProviders[ruleSetName]; ok {
					// Use the URL from rule provider
					noResolve := ""
					if len(parts) >= 4 && strings.TrimSpace(parts[3]) == "no-resolve" {
						noResolve = ",no-resolve"
					}
					surgeRules = append(surgeRules, fmt.Sprintf("RULE-SET,%s,%s%s", provider.URL, policy, noResolve))
				}
			}

		default:
			// Other rule types, keep as-is
			surgeRules = append(surgeRules, rule)
		}
	}

	return surgeRules, nil
}

// ConvertClashProxyGroupsToSurge converts Clash proxy groups to Surge format
func ConvertClashProxyGroupsToSurge(groups []ClashProxyGroup) []ACLProxyGroup {
	var aclGroups []ACLProxyGroup

	for _, g := range groups {
		aclGroup := ACLProxyGroup{
			Name:      g.Name,
			Type:      convertProxyGroupType(g.Type),
			Proxies:   g.Proxies,
			URL:       g.URL,
			Interval:  g.Interval,
			Tolerance: g.Tolerance,
		}

		// Check if has .* wildcard
		for _, proxy := range g.Proxies {
			if proxy == ".*" || strings.Contains(proxy, ".*") {
				aclGroup.HasWildcard = true
				break
			}
		}

		aclGroups = append(aclGroups, aclGroup)
	}

	return aclGroups
}

// convertProxyGroupType converts Clash proxy group type to Surge type
func convertProxyGroupType(clashType string) string {
	switch strings.ToLower(clashType) {
	case "select":
		return "select"
	case "url-test":
		return "url-test"
	case "fallback":
		return "fallback"
	case "load-balance":
		return "load-balance"
	case "relay":
		// Surge doesn't support relay, convert to select
		return "select"
	default:
		return "select"
	}
}

// applyDefaultSurgeConfig applies default values to Surge config
func applyDefaultSurgeConfig(opts *SurgeTemplateConfig) {
	if opts.LogLevel == "" {
		opts.LogLevel = "notify"
	}
	if opts.SkipProxy == "" {
		opts.SkipProxy = "127.0.0.1,192.168.0.0/16,10.0.0.0/8,172.16.0.0/12,100.64.0.0/10,localhost,*.local"
	}
	if opts.BypassTun == "" {
		opts.BypassTun = "192.168.0.0/16,10.0.0.0/8,172.16.0.0/12"
	}
	if opts.TestTimeout <= 0 {
		opts.TestTimeout = 5
	}
	if opts.ExternalController == "" {
		opts.ExternalController = "password@0.0.0.0:6170"
	}
	if opts.HTTPAPIPort == "" {
		opts.HTTPAPIPort = "password@0.0.0.0:6171"
	}

	opts.BypassSystem = true
	opts.ExcludeSimpleHostnames = true
	opts.AllowWiFiAccess = true
	opts.HTTPAPIWebDashboard = true
}

// contains checks if a string slice contains a value
func contains(slice []string, val string) bool {
	for _, item := range slice {
		if item == val {
			return true
		}
	}
	return false
}

// Note: We could use slices.Contains in Go 1.21+, but keeping this for compatibility

// BuildCompleteSurgeConfig builds a complete Surge configuration from Clash config
// This is a high-level function that combines all parts
func BuildCompleteSurgeConfig(
	clashConfig *ClashConfig,
	proxies []Proxy,
	templateOpts *SurgeTemplateConfig,
	includeUnsupported bool,
) (string, error) {

	var sections []string

	// 1. Build General section
	generalSection, err := ConvertClashToSurgeConfig(clashConfig, templateOpts)
	if err != nil {
		return "", fmt.Errorf("failed to convert general config: %w", err)
	}
	sections = append(sections, generalSection)

	// 2. Build Proxy section
	var proxyBuilder strings.Builder
	proxyBuilder.WriteString("[Proxy]")
	proxyBuilder.WriteString("\nDIRECT = direct")

	// Convert proxies using surge.go
	surgeProducer := NewSurgeProducer()
	opts := &ProduceOptions{
		IncludeUnsupportedProxy: includeUnsupported,
	}

	for _, proxy := range proxies {
		line, err := surgeProducer.ProduceOne(proxy, "", opts)
		if err != nil {
			// Skip unsupported proxies
			continue
		}
		if line != "" {
			proxyBuilder.WriteString("\n")
			proxyBuilder.WriteString(line)
		}
	}
	sections = append(sections, proxyBuilder.String())

	// 3. Build Proxy Group section
	aclGroups := ConvertClashProxyGroupsToSurge(clashConfig.ProxyGroups)
	proxyGroupSection := GenerateSurgeProxyGroups(aclGroups, false)
	sections = append(sections, proxyGroupSection)

	// 4. Build Rule section
	surgeRules, err := ConvertClashRulesToSurgeFormat(clashConfig.Rules, clashConfig.RuleProviders)
	if err != nil {
		return "", fmt.Errorf("failed to convert rules: %w", err)
	}

	var ruleBuilder strings.Builder
	ruleBuilder.WriteString("[Rule]")
	for _, rule := range surgeRules {
		ruleBuilder.WriteString("\n")
		ruleBuilder.WriteString(rule)
	}
	sections = append(sections, ruleBuilder.String())

	return strings.Join(sections, "\n\n"), nil
}
