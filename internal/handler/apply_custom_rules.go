package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

type applyCustomRulesRequest struct {
	YamlContent string `json:"yaml_content"`
}

type applyCustomRulesResponse struct {
	YamlContent string `json:"yaml_content"`
}

// NewApplyCustomRulesHandler returns a handler that applies custom rules to YAML content
func NewApplyCustomRulesHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("apply custom rules handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		// Check if custom rules are enabled
		settings, err := repo.GetUserSettings(r.Context(), username)
		if err != nil || !settings.CustomRulesEnabled {
			// If not enabled, just return the original YAML
			var payload applyCustomRulesRequest
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}

			resp := applyCustomRulesResponse{
				YamlContent: payload.YamlContent,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}

		var payload applyCustomRulesRequest
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if strings.TrimSpace(payload.YamlContent) == "" {
			writeError(w, http.StatusBadRequest, errors.New("yaml_content is required"))
			return
		}

		// Apply custom rules
		modifiedYaml, err := applyCustomRulesToYaml(r.Context(), repo, []byte(payload.YamlContent))
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to apply custom rules: %w", err))
			return
		}

		resp := applyCustomRulesResponse{
			YamlContent: string(modifiedYaml),
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// applyCustomRulesToYaml applies enabled custom rules to the YAML data
func applyCustomRulesToYaml(ctx context.Context, repo *storage.TrafficRepository, yamlData []byte) ([]byte, error) {
	// Parse YAML data
	var config map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Get enabled custom rules
	rules, err := repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get custom rules: %w", err)
	}

	if len(rules) == 0 {
		return yamlData, nil
	}

	// Apply each rule based on its type
	for _, rule := range rules {
		switch rule.Type {
		case "dns":
			// Check if user input contains "dns:" key
			var parsedContent map[string]interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
				continue
			}

			// If user input contains "dns:" key, replace the entire dns block
			if dnsValue, hasDnsKey := parsedContent["dns"]; hasDnsKey {
				config["dns"] = dnsValue
			} else {
				// Otherwise, replace only the dns content
				config["dns"] = parsedContent
			}

		case "rules":
			// First try to parse as a map to check if it contains "rules:" key
			var parsedAsMap map[string]interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &parsedAsMap); err == nil {
				// Check if it contains "rules:" key
				if rulesValue, hasRulesKey := parsedAsMap["rules"]; hasRulesKey {
					// User provided "rules:" key, use it directly
					if rule.Mode == "replace" {
						config["rules"] = rulesValue
					} else if rule.Mode == "prepend" {
						// Get existing rules
						existingRules, ok := config["rules"].([]interface{})
						if !ok {
							existingRules = []interface{}{}
						}
						// Prepend new rules
						newRules, ok := rulesValue.([]interface{})
						if ok {
							config["rules"] = append(newRules, existingRules...)
						}
					}
					continue
				}
			}

			// Try to parse as YAML array (e.g., "- DOMAIN,xxx,xx")
			var rulesConfig []interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &rulesConfig); err == nil {
				// Successfully parsed as YAML array
				if rule.Mode == "replace" {
					config["rules"] = rulesConfig
				} else if rule.Mode == "prepend" {
					// Get existing rules
					existingRules, ok := config["rules"].([]interface{})
					if !ok {
						existingRules = []interface{}{}
					}
					// Prepend new rules
					config["rules"] = append(rulesConfig, existingRules...)
				}
				continue
			}

			// If YAML parsing failed, treat as plain text (each line is a rule)
			// Split by newlines and convert to array
			lines := strings.Split(rule.Content, "\n")
			var plainRules []interface{}
			for _, line := range lines {
				line = strings.TrimSpace(line)
				// Skip empty lines and comments
				if line == "" || strings.HasPrefix(line, "#") {
					continue
				}
				plainRules = append(plainRules, line)
			}

			if len(plainRules) > 0 {
				if rule.Mode == "replace" {
					config["rules"] = plainRules
				} else if rule.Mode == "prepend" {
					// Get existing rules
					existingRules, ok := config["rules"].([]interface{})
					if !ok {
						existingRules = []interface{}{}
					}
					// Prepend new rules
					config["rules"] = append(plainRules, existingRules...)
				}
			}

		case "rule-providers":
			// Parse as map
			var parsedContent map[string]interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
				continue
			}

			// If user input contains "rule-providers:" key, use it directly
			if providersValue, hasProvidersKey := parsedContent["rule-providers"]; hasProvidersKey {
				providersMap, ok := providersValue.(map[string]interface{})
				if !ok {
					continue
				}

				if rule.Mode == "replace" {
					config["rule-providers"] = providersMap
				} else if rule.Mode == "prepend" {
					// Get existing rule-providers
					existingProviders, ok := config["rule-providers"].(map[string]interface{})
					if !ok {
						existingProviders = make(map[string]interface{})
					}
					// Merge new providers (new providers take precedence)
					for k, v := range providersMap {
						existingProviders[k] = v
					}
					config["rule-providers"] = existingProviders
				}
			} else {
				// Otherwise, treat parsedContent as the rule-providers content
				if rule.Mode == "replace" {
					config["rule-providers"] = parsedContent
				} else if rule.Mode == "prepend" {
					// Get existing rule-providers
					existingProviders, ok := config["rule-providers"].(map[string]interface{})
					if !ok {
						existingProviders = make(map[string]interface{})
					}
					// Merge new providers (new providers take precedence)
					for k, v := range parsedContent {
						existingProviders[k] = v
					}
					config["rule-providers"] = existingProviders
				}
			}
		}
	}

	// Marshal back to YAML with proper field ordering
	modifiedData, err := marshalWithFieldOrder(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	return modifiedData, nil
}

// marshalWithFieldOrder marshals the config to YAML with fields in the correct order
func marshalWithFieldOrder(config map[string]interface{}) ([]byte, error) {
	// First marshal to get a proper YAML structure
	tempData, err := yaml.Marshal(config)
	if err != nil {
		return nil, err
	}

	// Parse into yaml.Node to manipulate field order
	var rootNode yaml.Node
	if err := yaml.Unmarshal(tempData, &rootNode); err != nil {
		return nil, err
	}

	// Apply field ordering
	if len(rootNode.Content) > 0 {
		reorderYAMLFields(rootNode.Content[0])
	}

	// Marshal back to bytes
	return yaml.Marshal(&rootNode)
}

// reorderYAMLFields reorders YAML fields according to priorityFields
func reorderYAMLFields(docNode *yaml.Node) {
	if docNode.Kind != yaml.MappingNode {
		return
	}

	// Define field pair structure
	type fieldPair struct {
		key   *yaml.Node
		value *yaml.Node
	}

	// yaml属性指定排序 (same as yaml_sync.go)
	priorityFields := []string{
		"port",
		"socks-port",
		"allow-lan",
		"mode",
		"log-level",
		"geodata-mode",
		"geo-auto-update",
		"geodata-loader",
		"geo-update-interval",
		"geox-url",
		"dns",
		"proxies",
		"proxy-groups",
		"rule-providers",
		"rules",
	}

	// Create a map to store all key-value pairs
	fieldMap := make(map[string]*fieldPair)
	var otherFields []*fieldPair

	// Extract all fields
	for i := 0; i < len(docNode.Content); i += 2 {
		if i+1 >= len(docNode.Content) {
			break
		}
		keyNode := docNode.Content[i]
		valueNode := docNode.Content[i+1]

		pair := &fieldPair{key: keyNode, value: valueNode}

		// Check if this is a priority field
		isPriority := false
		for _, pf := range priorityFields {
			if keyNode.Value == pf {
				fieldMap[pf] = pair
				isPriority = true
				break
			}
		}

		if !isPriority {
			otherFields = append(otherFields, pair)
		}
	}

	// Rebuild Content with priority fields first
	newContent := make([]*yaml.Node, 0, len(docNode.Content))

	// Add priority fields in order
	for _, fieldName := range priorityFields {
		if pair, ok := fieldMap[fieldName]; ok {
			// Special handling for proxies field: reorder fields inside each proxy
			if fieldName == "proxies" && pair.value.Kind == yaml.SequenceNode {
				reorderProxiesArrayFields(pair.value)
			}
			newContent = append(newContent, pair.key, pair.value)
		}
	}

	// Add remaining fields in their original order
	for _, pair := range otherFields {
		// Also handle proxies if they are not in priority fields
		if pair.key.Value == "proxies" && pair.value.Kind == yaml.SequenceNode {
			reorderProxiesArrayFields(pair.value)
		}
		newContent = append(newContent, pair.key, pair.value)
	}

	// Replace the content
	docNode.Content = newContent
}

// 对proxies的children属性重排序，避免某些客户端导入失败
func reorderProxiesArrayFields(proxiesNode *yaml.Node) {
	if proxiesNode.Kind != yaml.SequenceNode {
		return
	}

	// 保证首个属性为name
	proxyPriorityFields := []string{
		"name",
		"type",
		"server",
		"port",
	}

	for i := 0; i < len(proxiesNode.Content); i++ {
		proxyNode := proxiesNode.Content[i]
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}

		type proxyFieldPair struct {
			key   *yaml.Node
			value *yaml.Node
		}

		proxyFieldMap := make(map[string]*proxyFieldPair)
		var proxyOtherFields []*proxyFieldPair

		for j := 0; j < len(proxyNode.Content); j += 2 {
			if j+1 >= len(proxyNode.Content) {
				break
			}
			keyNode := proxyNode.Content[j]
			valueNode := proxyNode.Content[j+1]

			pair := &proxyFieldPair{key: keyNode, value: valueNode}

			isPriority := false
			for _, pf := range proxyPriorityFields {
				if keyNode.Value == pf {
					proxyFieldMap[pf] = pair
					isPriority = true
					break
				}
			}

			if !isPriority {
				proxyOtherFields = append(proxyOtherFields, pair)
			}
		}

		newProxyContent := make([]*yaml.Node, 0, len(proxyNode.Content))

		for _, fieldName := range proxyPriorityFields {
			if pair, ok := proxyFieldMap[fieldName]; ok {
				newProxyContent = append(newProxyContent, pair.key, pair.value)
			}
		}

		for _, pair := range proxyOtherFields {
			newProxyContent = append(newProxyContent, pair.key, pair.value)
		}

		proxyNode.Content = newProxyContent
	}
}
