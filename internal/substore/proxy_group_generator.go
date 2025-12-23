package substore

import (
	"fmt"
	"regexp"
	"strings"
)

// filterProxyNamesByRegex filters proxy names using regex patterns
// Returns matched proxy names, or all names if no patterns or regex error
func filterProxyNamesByRegex(allNames []string, regexFilters []string) []string {
	if len(regexFilters) == 0 || len(allNames) == 0 {
		return allNames
	}

	// Merge all regex patterns into one
	pattern := MergeRegexFilters(regexFilters)
	re, err := regexp.Compile(pattern)
	if err != nil {
		// Invalid regex, return all names
		return allNames
	}

	var matched []string
	for _, name := range allNames {
		if re.MatchString(name) {
			matched = append(matched, name)
		}
	}
	return matched
}

// GenerateClashProxyGroups generates Clash format proxy groups
// When allProxyNames is provided, outputs explicit proxies list instead of include-all/filter
// The decision to include all proxies is based on g.HasWildcard (from .* in ACL config)
func GenerateClashProxyGroups(groups []ACLProxyGroup, allProxyNames []string) string {
	var lines []string
	lines = append(lines, "proxy-groups:")

	for _, g := range groups {
		lines = append(lines, fmt.Sprintf("  - name: %s", g.Name))
		lines = append(lines, fmt.Sprintf("    type: %s", g.Type))

		if g.Type == "url-test" || g.Type == "fallback" {
			url := g.URL
			if url == "" {
				url = "http://www.gstatic.com/generate_204"
			}
			lines = append(lines, fmt.Sprintf("    url: %s", url))

			interval := g.Interval
			if interval <= 0 {
				interval = 300
			}
			lines = append(lines, fmt.Sprintf("    interval: %d", interval))

			tolerance := g.Tolerance
			if tolerance <= 0 {
				tolerance = 150
			}
			lines = append(lines, fmt.Sprintf("    tolerance: %d", tolerance))
		}

		// Separate regex patterns and normal proxy references
		var regexFilters []string
		var normalProxies []string
		for _, proxy := range g.Proxies {
			if IsRegexProxyPattern(proxy) {
				regexFilters = append(regexFilters, proxy)
			} else {
				normalProxies = append(normalProxies, proxy)
			}
		}

		// Determine which proxies to include
		var proxiesToOutput []string

		// For select type with policy references (normalProxies), .* wildcard should NOT add actual nodes
		// because the nodes are accessed through the referenced policy groups
		// Only url-test/fallback/load-balance types need actual nodes
		shouldAddActualNodes := len(normalProxies) == 0 || g.Type == "url-test" || g.Type == "fallback" || g.Type == "load-balance"

		if len(allProxyNames) > 0 && shouldAddActualNodes {
			// Explicit mode: use provided proxy names with regex filtering
			if len(regexFilters) > 0 {
				// Apply regex filter to get matching proxies
				proxiesToOutput = filterProxyNamesByRegex(allProxyNames, regexFilters)
			} else if g.HasWildcard {
				// Has .* wildcard: include all provided proxies
				proxiesToOutput = allProxyNames
			}
		} else if len(allProxyNames) == 0 {
			// Legacy mode: use include-all and filter fields
			if len(regexFilters) > 0 {
				lines = append(lines, "    include-all: true")
				lines = append(lines, fmt.Sprintf("    filter: %s", MergeRegexFilters(regexFilters)))
			} else if g.HasWildcard && shouldAddActualNodes {
				lines = append(lines, "    include-all: true")
			}
		}

		// Output proxies list
		// Combine: explicit proxies from filter + normal policy references (DIRECT, other groups)
		allProxiesToOutput := append(proxiesToOutput, normalProxies...)
		if len(allProxiesToOutput) > 0 {
			lines = append(lines, "    proxies:")
			for _, proxy := range allProxiesToOutput {
				lines = append(lines, fmt.Sprintf("      - %s", proxy))
			}
		}
	}

	return strings.Join(lines, "\n")
}

// GenerateSurgeProxyGroups generates Surge format proxy groups
// Supports policy-regex-filter + include-all-proxies
func GenerateSurgeProxyGroups(groups []ACLProxyGroup, enableIncludeAll bool) string {
	var lines []string
	lines = append(lines, "[Proxy Group]")

	for _, g := range groups {
		// Separate regex patterns and normal proxy references
		var regexFilters []string
		var normalProxies []string
		for _, proxy := range g.Proxies {
			if IsRegexProxyPattern(proxy) {
				regexFilters = append(regexFilters, proxy)
			} else {
				normalProxies = append(normalProxies, proxy)
			}
		}

		var line string

		if g.Type == "url-test" || g.Type == "fallback" {
			url := g.URL
			if url == "" {
				url = "http://www.gstatic.com/generate_204"
			}
			interval := g.Interval
			if interval <= 0 {
				interval = 300
			}
			tolerance := g.Tolerance
			if tolerance <= 0 {
				tolerance = 150
			}

			// When regex patterns exist, force include-all-proxies (policy-regex-filter depends on it)
			if len(regexFilters) > 0 {
				filter := ExtractSurgeRegexFilter(regexFilters)
				if len(normalProxies) > 0 {
					line = fmt.Sprintf("%s = %s, %s, url=%s, interval=%d, timeout=5, tolerance=%d, policy-regex-filter=%s, include-all-proxies=1",
						g.Name, g.Type, strings.Join(normalProxies, ", "), url, interval, tolerance, filter)
				} else {
					line = fmt.Sprintf("%s = %s, url=%s, interval=%d, timeout=5, tolerance=%d, policy-regex-filter=%s, include-all-proxies=1",
						g.Name, g.Type, url, interval, tolerance, filter)
				}
			} else if enableIncludeAll {
				// User enabled include-all mode
				proxies := normalProxies
				if len(proxies) == 0 {
					proxies = []string{"DIRECT"}
				}
				line = fmt.Sprintf("%s = %s, %s, url=%s, interval=%d, timeout=5, tolerance=%d, include-all-proxies=1",
					g.Name, g.Type, strings.Join(proxies, ", "), url, interval, tolerance)
			} else {
				// Normal mode without include-all-proxies
				proxies := normalProxies
				if len(proxies) == 0 {
					proxies = []string{"DIRECT"}
				}
				line = fmt.Sprintf("%s = %s, %s, url=%s, interval=%d, timeout=5, tolerance=%d",
					g.Name, g.Type, strings.Join(proxies, ", "), url, interval, tolerance)
			}
		} else {
			// select, load-balance and other types
			if len(regexFilters) > 0 {
				// When regex patterns exist, force include-all-proxies
				filter := ExtractSurgeRegexFilter(regexFilters)
				if len(normalProxies) > 0 {
					line = fmt.Sprintf("%s = %s, %s, policy-regex-filter=%s, include-all-proxies=1",
						g.Name, g.Type, strings.Join(normalProxies, ", "), filter)
				} else {
					line = fmt.Sprintf("%s = %s, policy-regex-filter=%s, include-all-proxies=1",
						g.Name, g.Type, filter)
				}
			} else if enableIncludeAll {
				// User enabled include-all mode
				proxies := normalProxies
				if len(proxies) == 0 {
					proxies = []string{"DIRECT"}
				}
				line = fmt.Sprintf("%s = %s, %s, include-all-proxies=1", g.Name, g.Type, strings.Join(proxies, ", "))
			} else {
				// Normal mode without include-all-proxies
				proxies := normalProxies
				if len(proxies) == 0 {
					proxies = []string{"DIRECT"}
				}
				line = fmt.Sprintf("%s = %s, %s", g.Name, g.Type, strings.Join(proxies, ", "))
			}
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}
