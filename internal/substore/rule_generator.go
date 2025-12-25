package substore

import (
	"fmt"
	"strings"
)

// extractNoResolve extracts the no-resolve suffix from a rule
// Returns: rule content (without no-resolve) and suffix (",no-resolve" or empty string)
func extractNoResolve(rule string) (string, string) {
	if strings.HasSuffix(rule, ",no-resolve") {
		return strings.TrimSuffix(rule, ",no-resolve"), ",no-resolve"
	}
	return rule, ""
}

// GenerateClashRules generates Clash format rules (RULE-SET mode)
// Returns rules string and rule-providers string
func GenerateClashRules(rulesets []ACLRuleset) (rulesStr string, providersStr string, err error) {
	var rules []string
	var providers []string
	providerIndex := make(map[string]bool)

	for _, rs := range rulesets {
		if strings.HasPrefix(rs.RuleURL, "[]") {
			// Inline rule
			rule := rs.RuleURL[2:] // Remove []
			if rule == "GEOIP,CN" {
				rules = append(rules, fmt.Sprintf("GEOIP,CN,%s", rs.Group))
			} else if rule == "FINAL" {
				rules = append(rules, fmt.Sprintf("MATCH,%s", rs.Group))
			} else if strings.HasPrefix(rule, "GEOIP,") {
				geo := strings.TrimPrefix(rule, "GEOIP,")
				geoContent, noResolve := extractNoResolve(geo)
				rules = append(rules, fmt.Sprintf("GEOIP,%s,%s%s", geoContent, rs.Group, noResolve))
			} else {
				ruleContent, noResolve := extractNoResolve(rule)
				rules = append(rules, fmt.Sprintf("%s,%s%s", ruleContent, rs.Group, noResolve))
			}
		} else if strings.HasPrefix(rs.RuleURL, "http") {
			// Remote rule, extract name
			providerName := extractProviderName(rs.RuleURL)

			// Use behavior and interval from parsed ruleset
			behavior := rs.Behavior
			if behavior == "" {
				behavior = "classical"
			}
			interval := rs.Interval
			if interval <= 0 {
				interval = 86400
			}

			// Add RULE-SET reference
			rules = append(rules, fmt.Sprintf("RULE-SET,%s,%s", providerName, rs.Group))

			// Add provider definition (avoid duplicates)
			if !providerIndex[providerName] {
				providerIndex[providerName] = true
				providers = append(providers, generateProviderWithInterval(providerName, rs.RuleURL, behavior, interval))
			}
		}
	}

	// Generate rules section
	var rulesLines []string
	rulesLines = append(rulesLines, "rules:")
	for _, rule := range rules {
		rulesLines = append(rulesLines, fmt.Sprintf("  - %s", rule))
	}
	rulesStr = strings.Join(rulesLines, "\n")

	// Generate rule-providers section if any
	if len(providers) > 0 {
		var providerLines []string
		providerLines = append(providerLines, "rule-providers:")
		for _, p := range providers {
			providerLines = append(providerLines, p)
		}
		providersStr = strings.Join(providerLines, "\n")
	}

	return rulesStr, providersStr, nil
}

// GenerateSurgeRules generates Surge format rules
// Surge uses URL directly as RULE-SET
func GenerateSurgeRules(rulesets []ACLRuleset) (string, error) {
	var lines []string
	lines = append(lines, "[Rule]")

	for _, rs := range rulesets {
		if strings.HasPrefix(rs.RuleURL, "[]") {
			rule := rs.RuleURL[2:]
			if rule == "GEOIP,CN" {
				lines = append(lines, fmt.Sprintf("GEOIP,CN,%s", rs.Group))
			} else if rule == "FINAL" {
				lines = append(lines, fmt.Sprintf("FINAL,%s", rs.Group))
			} else {
				lines = append(lines, fmt.Sprintf("%s,%s", rule, rs.Group))
			}
		} else if strings.HasPrefix(rs.RuleURL, "http") {
			lines = append(lines, fmt.Sprintf("RULE-SET,%s,%s,update-interval=86400", rs.RuleURL, rs.Group))
		}
	}

	return strings.Join(lines, "\n"), nil
}

// extractProviderName extracts provider name from URL
func extractProviderName(url string) string {
	// Extract filename from URL
	parts := strings.Split(url, "/")
	filename := parts[len(parts)-1]

	// Remove extensions (.list, .yaml, .yml)
	name := filename
	name = strings.TrimSuffix(name, ".list")
	name = strings.TrimSuffix(name, ".yaml")
	name = strings.TrimSuffix(name, ".yml")

	return name
}

// generateProviderWithInterval generates YAML for a single rule-provider with custom interval
func generateProviderWithInterval(name, url, behavior string, interval int) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("  %s:", name))
	lines = append(lines, "    type: http")
	lines = append(lines, fmt.Sprintf("    behavior: %s", behavior))
	lines = append(lines, fmt.Sprintf("    url: %s", url))

	// Determine path extension and format based on URL
	if strings.HasSuffix(url, ".list") {
		lines = append(lines, "    format: text")
		lines = append(lines, "    path: ./providers/"+strings.ReplaceAll(name, " ", "_")+".txt")
	} else {
		// .yaml or .yml files don't need format: text
		lines = append(lines, "    path: ./providers/"+strings.ReplaceAll(name, " ", "_")+".yaml")
	}

	lines = append(lines, fmt.Sprintf("    interval: %d", interval))
	return strings.Join(lines, "\n")
}
