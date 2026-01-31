package substore

import (
	"bufio"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ACLRuleset represents a ruleset definition in ACL4SSR config
type ACLRuleset struct {
	Group    string // Target proxy group
	RuleURL  string // Rule URL or inline rule (e.g., []GEOIP,CN)
	Behavior string // classical, domain, ipcidr (for Clash rule-providers)
	Interval int    // Update interval in seconds
}

// ACLProxyGroup represents a proxy group definition in ACL4SSR config
type ACLProxyGroup struct {
	Name        string   // Group name
	Type        string   // select, url-test, fallback, load-balance
	Proxies     []string // Proxy list (may include regex patterns like (香港|HK))
	HasWildcard bool     // Whether the group contains .* wildcard (include all proxies)
	URL         string   // Health check URL
	Interval    int      // Health check interval
	Tolerance   int      // Tolerance for url-test
}

// ParseACLConfig parses ACL4SSR format configuration content
// Extracts ruleset= and custom_proxy_group= definitions
// For duplicate proxy group names, the later one overrides the earlier one
func ParseACLConfig(content string) ([]ACLRuleset, []ACLProxyGroup) {
	var rulesets []ACLRuleset
	var proxyGroups []ACLProxyGroup

	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, ";") || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse ruleset=
		if strings.HasPrefix(line, "ruleset=") {
			parts := strings.SplitN(line[8:], ",", 2)
			if len(parts) == 2 {
				rs := parseRuleset(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
				rulesets = append(rulesets, rs)
			}
		}

		// Parse custom_proxy_group=
		if strings.HasPrefix(line, "custom_proxy_group=") {
			pg := parseProxyGroup(line[19:])
			if pg.Name != "" {
				proxyGroups = append(proxyGroups, pg)
			}
		}
	}

	// Deduplicate proxy groups: later ones override earlier ones
	proxyGroups = deduplicateProxyGroups(proxyGroups)

	return rulesets, proxyGroups
}

// deduplicateProxyGroups removes duplicate proxy groups by name
// Later occurrences override earlier ones, preserving the order of last occurrence
func deduplicateProxyGroups(groups []ACLProxyGroup) []ACLProxyGroup {
	if len(groups) <= 1 {
		return groups
	}

	// Use a map to track the last occurrence of each group name
	lastIndex := make(map[string]int)
	for i, g := range groups {
		lastIndex[g.Name] = i
	}

	// Build result preserving order of last occurrences
	seen := make(map[string]bool)
	var result []ACLProxyGroup
	for i, g := range groups {
		if lastIndex[g.Name] == i && !seen[g.Name] {
			result = append(result, g)
			seen[g.Name] = true
		}
	}

	return result
}

// parseRuleset parses a ruleset definition with support for various formats
// Formats supported:
// - clash-classic:https://...yaml,28800 (with type prefix and interval)
// - clash-domain:https://...yaml (with type prefix)
// - https://...list (plain URL)
// - rules/ACL4SSR/Clash/xxx.list (relative path for ACL4SSR)
// - []GEOIP,CN (inline rule)
func parseRuleset(group, ruleSpec string) ACLRuleset {
	rs := ACLRuleset{
		Group:    group,
		Behavior: "classical",
		Interval: 86400,
	}

	// Parse interval parameter (trailing ,number)
	if idx := strings.LastIndex(ruleSpec, ","); idx > 0 {
		suffix := ruleSpec[idx+1:]
		if interval, err := strconv.Atoi(suffix); err == nil {
			rs.Interval = interval
			ruleSpec = ruleSpec[:idx]
		}
	}

	// Parse type prefix
	switch {
	case strings.HasPrefix(ruleSpec, "clash-classic:"):
		rs.Behavior = "classical"
		rs.RuleURL = ruleSpec[14:]
	case strings.HasPrefix(ruleSpec, "clash-domain:"):
		rs.Behavior = "domain"
		rs.RuleURL = ruleSpec[13:]
	case strings.HasPrefix(ruleSpec, "clash-ipcidr:"):
		rs.Behavior = "ipcidr"
		rs.RuleURL = ruleSpec[13:]
	case strings.HasPrefix(ruleSpec, "[]"):
		// Inline rule
		rs.RuleURL = ruleSpec
	case strings.HasPrefix(ruleSpec, "http"):
		// Full URL
		rs.RuleURL = ruleSpec
	case strings.HasPrefix(ruleSpec, "rules/ACL4SSR/"):
		// ACL4SSR relative path → convert to full URL
		rs.RuleURL = "https://testingcf.jsdelivr.net/gh/ACL4SSR/ACL4SSR@master/" +
			strings.TrimPrefix(ruleSpec, "rules/ACL4SSR/")
	default:
		rs.RuleURL = ruleSpec
	}

	return rs
}

// parseProxyGroup parses a proxy group definition
// Format: name`type`proxy1`proxy2`...`url`interval,,tolerance
func parseProxyGroup(line string) ACLProxyGroup {
	parts := strings.Split(line, "`")
	if len(parts) < 2 {
		return ACLProxyGroup{}
	}

	pg := ACLProxyGroup{
		Name:    parts[0],
		Type:    parts[1],
		Proxies: make([]string, 0),
	}

	for i := 2; i < len(parts); i++ {
		part := parts[i]

		// Detect health check URL
		if strings.HasPrefix(part, "http://") || strings.HasPrefix(part, "https://") {
			pg.URL = part
			continue
		}

		// Detect numeric format: interval,,tolerance or just interval
		if matched, _ := regexp.MatchString(`^\d+`, part); matched {
			// Check for ,, separator (interval,,tolerance)
			if strings.Contains(part, ",") {
				numParts := strings.Split(part, ",")
				if len(numParts) >= 1 && numParts[0] != "" {
					fmt.Sscanf(numParts[0], "%d", &pg.Interval)
				}
				// Tolerance is in the last non-empty element
				for j := len(numParts) - 1; j >= 0; j-- {
					if numParts[j] != "" && j > 0 {
						fmt.Sscanf(numParts[j], "%d", &pg.Tolerance)
						break
					}
				}
			} else {
				fmt.Sscanf(part, "%d", &pg.Interval)
			}
			continue
		}

		// Proxy name, remove [] prefix if present
		proxyName := part
		if strings.HasPrefix(part, "[]") {
			proxyName = part[2:]
		}

		// Skip empty names
		if proxyName == "" {
			continue
		}

		// Detect .* wildcard (include all proxies)
		if proxyName == ".*" {
			pg.HasWildcard = true
			continue
		}

		pg.Proxies = append(pg.Proxies, proxyName)
	}

	return pg
}

// IsRegexProxyPattern checks if a proxy name is a regex pattern
// Format: (option1|option2|option3)
func IsRegexProxyPattern(proxy string) bool {
	proxy = strings.TrimSpace(proxy)
	if len(proxy) < 3 {
		return false
	}
	return strings.HasPrefix(proxy, "(") && strings.HasSuffix(proxy, ")") && strings.Contains(proxy, "|")
}

// MergeRegexFilters merges multiple regex filters into one
// Input: ["(香港|HK)", "(日本|JP)"]
// Output: "(香港|HK|日本|JP)"
func MergeRegexFilters(filters []string) string {
	if len(filters) == 1 {
		return filters[0]
	}
	var allOptions []string
	for _, f := range filters {
		// Remove outer parentheses and extract inner options
		inner := strings.TrimPrefix(strings.TrimSuffix(f, ")"), "(")
		allOptions = append(allOptions, inner)
	}
	return "(" + strings.Join(allOptions, "|") + ")"
}

// ExtractSurgeRegexFilter extracts regex filter for Surge format
// Input: ["(香港|HK)", "(日本|JP)"]
// Output: "香港|HK|日本|JP"
func ExtractSurgeRegexFilter(filters []string) string {
	var allOptions []string
	for _, f := range filters {
		// Remove outer parentheses and extract inner options
		inner := strings.TrimPrefix(strings.TrimSuffix(f, ")"), "(")
		allOptions = append(allOptions, inner)
	}
	return strings.Join(allOptions, "|")
}
