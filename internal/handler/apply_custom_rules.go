package handler

import (
	"context"
	"crypto/sha256"
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
	// Get enabled custom rules first
	rules, err := repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get custom rules: %w", err)
	}

	if len(rules) == 0 {
		return yamlData, nil
	}

	// Parse YAML data as yaml.Node to preserve types
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Also parse as map for easier manipulation
	var config map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
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

	// Update the rootNode with modified config values
	if len(rootNode.Content) > 0 {
		updateYAMLNodeFromMap(rootNode.Content[0], config)
		// Apply field ordering
		reorderYAMLFields(rootNode.Content[0])
	}

	// Marshal back to bytes
	modifiedData, err := yaml.Marshal(&rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	return modifiedData, nil
}

// updateYAMLNodeFromMap updates yaml.Node fields from map values while preserving node styles
func updateYAMLNodeFromMap(docNode *yaml.Node, config map[string]interface{}) {
	if docNode.Kind != yaml.MappingNode {
		return
	}

	// Build a map of existing fields
	existingFields := make(map[string]*yaml.Node) // fieldName -> valueNode
	for i := 0; i < len(docNode.Content); i += 2 {
		if i+1 >= len(docNode.Content) {
			break
		}
		keyNode := docNode.Content[i]
		valueNode := docNode.Content[i+1]
		existingFields[keyNode.Value] = valueNode
	}

	// Update existing fields and add new ones
	for key, value := range config {
		if valueNode, exists := existingFields[key]; exists {
			// Update existing value node
			updateYAMLValueNode(valueNode, value)
		} else {
			// Add new field at the end
			keyNode := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: key,
			}
			valueNode := encodeYAMLValue(value)
			docNode.Content = append(docNode.Content, keyNode, valueNode)
			existingFields[key] = valueNode
		}
	}
}

// updateYAMLValueNode updates a yaml.Node's value from an interface{} while preserving style
func updateYAMLValueNode(node *yaml.Node, newValue interface{}) {
	if node == nil {
		return
	}

	switch v := newValue.(type) {
	case string:
		if node.Kind == yaml.ScalarNode {
			node.Value = v
			// Clear !!str tag if the value is not empty, to prevent unnecessary quoting
			if v != "" && node.Tag == "!!str" {
				node.Tag = ""
			}
		} else {
			node.Kind = yaml.ScalarNode
			node.Value = v
		}
	case int:
		if node.Kind == yaml.ScalarNode {
			node.SetString(fmt.Sprintf("%d", v))
		} else {
			node.Kind = yaml.ScalarNode
			node.SetString(fmt.Sprintf("%d", v))
		}
	case int64:
		if node.Kind == yaml.ScalarNode {
			node.SetString(fmt.Sprintf("%d", v))
		} else {
			node.Kind = yaml.ScalarNode
			node.SetString(fmt.Sprintf("%d", v))
		}
	case float64:
		if node.Kind == yaml.ScalarNode {
			node.SetString(fmt.Sprintf("%v", v))
		} else {
			node.Kind = yaml.ScalarNode
			node.SetString(fmt.Sprintf("%v", v))
		}
	case bool:
		if node.Kind == yaml.ScalarNode {
			if v {
				node.Value = "true"
			} else {
				node.Value = "false"
			}
		} else {
			node.Kind = yaml.ScalarNode
			if v {
				node.Value = "true"
			} else {
				node.Value = "false"
			}
		}
	case []interface{}:
		// Rebuild array
		node.Kind = yaml.SequenceNode
		node.Content = nil
		for _, item := range v {
			node.Content = append(node.Content, encodeYAMLValue(item))
		}
	case map[string]interface{}:
		// Rebuild or update map
		if node.Kind == yaml.MappingNode {
			updateYAMLNodeFromMap(node, v)
		} else {
			node.Kind = yaml.MappingNode
			node.Content = nil
			for k, val := range v {
				keyNode := &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: k,
				}
				valueNode := encodeYAMLValue(val)
				node.Content = append(node.Content, keyNode, valueNode)
			}
		}
	}
}

// encodeYAMLValue converts a Go value to a yaml.Node
func encodeYAMLValue(value interface{}) *yaml.Node {
	node := &yaml.Node{}

	switch v := value.(type) {
	case string:
		node.Kind = yaml.ScalarNode
		if v == "" {
			node.Tag = "!!str"
		}
		node.Value = v
	case int:
		node.Kind = yaml.ScalarNode
		node.SetString(fmt.Sprintf("%d", v))
	case int64:
		node.Kind = yaml.ScalarNode
		node.SetString(fmt.Sprintf("%d", v))
	case float64:
		node.Kind = yaml.ScalarNode
		node.SetString(fmt.Sprintf("%v", v))
	case bool:
		node.Kind = yaml.ScalarNode
		if v {
			node.Value = "true"
		} else {
			node.Value = "false"
		}
	case []interface{}:
		node.Kind = yaml.SequenceNode
		for _, item := range v {
			node.Content = append(node.Content, encodeYAMLValue(item))
		}
	case map[string]interface{}:
		node.Kind = yaml.MappingNode
		for k, val := range v {
			keyNode := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: k,
			}
			node.Content = append(node.Content, keyNode)
			node.Content = append(node.Content, encodeYAMLValue(val))
		}
	default:
		node.Kind = yaml.ScalarNode
		node.SetString(fmt.Sprintf("%v", v))
	}

	return node
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

// applyCustomRulesToYamlSmart applies custom rules with intelligent deduplication
// This function is used for auto-sync to avoid duplicate content in prepend mode
func applyCustomRulesToYamlSmart(ctx context.Context, repo *storage.TrafficRepository, yamlData []byte, subscribeFileID int64) ([]byte, error) {
	// Get enabled custom rules
	rules, err := repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		return nil, fmt.Errorf("failed to get custom rules: %w", err)
	}

	if len(rules) == 0 {
		return yamlData, nil
	}

	// Get historical application records
	applications, err := repo.GetCustomRuleApplications(ctx, subscribeFileID)
	if err != nil {
		return nil, fmt.Errorf("failed to get custom rule applications: %w", err)
	}

	// Build a map of historical applications for quick lookup
	historyMap := make(map[string]*storage.CustomRuleApplication)
	for i := range applications {
		key := fmt.Sprintf("%d-%s", applications[i].CustomRuleID, applications[i].RuleType)
		historyMap[key] = &applications[i]
	}

	// Parse YAML
	var config map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Apply each rule with deduplication
	for _, rule := range rules {
		key := fmt.Sprintf("%d-%s", rule.ID, rule.Type)
		prevApp := historyMap[key]

		// Calculate content hash for change detection
		contentHash := fmt.Sprintf("%x", sha256.Sum256([]byte(rule.Content)))

		// If content hasn't changed and mode is replace, skip
		if prevApp != nil && prevApp.ContentHash == contentHash && rule.Mode == "replace" {
			continue
		}

		switch rule.Type {
		case "dns":
			if err := applyDNSRule(config, rule, prevApp); err != nil {
				continue
			}

		case "rules":
			appliedContent, err := applyRulesRule(config, rule, prevApp)
			if err != nil {
				continue
			}
			// Record what was applied
			if err := recordApplication(ctx, repo, subscribeFileID, rule, appliedContent, contentHash); err != nil {
				continue
			}

		case "rule-providers":
			appliedContent, err := applyRuleProvidersRule(config, rule, prevApp)
			if err != nil {
				continue
			}
			// Record what was applied
			if err := recordApplication(ctx, repo, subscribeFileID, rule, appliedContent, contentHash); err != nil {
				continue
			}
		}
	}

	// Marshal back to YAML with node-based approach for proper formatting
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err == nil && len(rootNode.Content) > 0 {
		updateYAMLNodeFromMap(rootNode.Content[0], config)
		reorderYAMLFields(rootNode.Content[0])
		return yaml.Marshal(&rootNode)
	}

	// Fallback to simple marshal
	return yaml.Marshal(config)
}

// applyDNSRule applies DNS custom rule
func applyDNSRule(config map[string]interface{}, rule storage.CustomRule, prevApp *storage.CustomRuleApplication) error {
	var parsedContent map[string]interface{}
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
		return err
	}

	if dnsValue, hasDnsKey := parsedContent["dns"]; hasDnsKey {
		config["dns"] = dnsValue
	} else {
		config["dns"] = parsedContent
	}

	return nil
}

// applyRulesRule applies rules custom rule with deduplication
func applyRulesRule(config map[string]interface{}, rule storage.CustomRule, prevApp *storage.CustomRuleApplication) (string, error) {
	// Parse rule content
	var newRules []interface{}

	// Try to parse as map first (with "rules:" key)
	var parsedAsMap map[string]interface{}
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedAsMap); err == nil {
		if rulesValue, hasRulesKey := parsedAsMap["rules"]; hasRulesKey {
			if rulesArray, ok := rulesValue.([]interface{}); ok {
				newRules = rulesArray
			}
		}
	}

	// Try to parse as YAML array
	if len(newRules) == 0 {
		if err := yaml.Unmarshal([]byte(rule.Content), &newRules); err != nil {
			// Parse as plain text
			lines := strings.Split(rule.Content, "\n")
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line != "" && !strings.HasPrefix(line, "#") {
					newRules = append(newRules, line)
				}
			}
		}
	}

	if len(newRules) == 0 {
		return "", errors.New("no rules parsed")
	}

	// Get existing rules
	existingRules, ok := config["rules"].([]interface{})
	if !ok {
		existingRules = []interface{}{}
	}

	if rule.Mode == "replace" {
		config["rules"] = newRules
	} else if rule.Mode == "prepend" {
		// Remove historical content if exists
		if prevApp != nil && prevApp.AppliedContent != "" {
			var historicalRules []interface{}
			if err := json.Unmarshal([]byte(prevApp.AppliedContent), &historicalRules); err == nil {
				existingRules = removeRulesFromList(existingRules, historicalRules)
			}
		}
		// Prepend new rules
		config["rules"] = append(newRules, existingRules...)
	}

	// Serialize applied content for tracking
	appliedJSON, _ := json.Marshal(newRules)
	return string(appliedJSON), nil
}

// applyRuleProvidersRule applies rule-providers custom rule with deduplication
func applyRuleProvidersRule(config map[string]interface{}, rule storage.CustomRule, prevApp *storage.CustomRuleApplication) (string, error) {
	var parsedContent map[string]interface{}
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
		return "", err
	}

	// Extract the providers map
	var providersMap map[string]interface{}
	if providersValue, hasProvidersKey := parsedContent["rule-providers"]; hasProvidersKey {
		var ok bool
		providersMap, ok = providersValue.(map[string]interface{})
		if !ok {
			return "", errors.New("invalid rule-providers format")
		}
	} else {
		providersMap = parsedContent
	}

	if len(providersMap) == 0 {
		return "", errors.New("no providers parsed")
	}

	// Get existing rule-providers
	existingProviders, ok := config["rule-providers"].(map[string]interface{})
	if !ok {
		existingProviders = make(map[string]interface{})
	}

	if rule.Mode == "replace" {
		config["rule-providers"] = providersMap
	} else if rule.Mode == "prepend" {
		// Remove historical providers if exists
		if prevApp != nil && prevApp.AppliedContent != "" {
			var historicalProviders map[string]interface{}
			if err := json.Unmarshal([]byte(prevApp.AppliedContent), &historicalProviders); err == nil {
				for key := range historicalProviders {
					delete(existingProviders, key)
				}
			}
		}
		// Merge new providers (new providers take precedence)
		for k, v := range providersMap {
			existingProviders[k] = v
		}
		config["rule-providers"] = existingProviders
	}

	// Serialize applied content for tracking
	appliedJSON, _ := json.Marshal(providersMap)
	return string(appliedJSON), nil
}

// removeRulesFromList removes rules from the list
func removeRulesFromList(existing []interface{}, toRemove []interface{}) []interface{} {
	// Build a set of rules to remove for O(n) lookup
	removeSet := make(map[string]bool)
	for _, rule := range toRemove {
		if ruleStr, ok := rule.(string); ok {
			removeSet[ruleStr] = true
		}
	}

	// Filter out rules that are in the remove set
	var filtered []interface{}
	for _, rule := range existing {
		if ruleStr, ok := rule.(string); ok {
			if !removeSet[ruleStr] {
				filtered = append(filtered, rule)
			}
		} else {
			// Keep non-string rules as-is
			filtered = append(filtered, rule)
		}
	}

	return filtered
}

// recordApplication records what was applied for future deduplication
func recordApplication(ctx context.Context, repo *storage.TrafficRepository, fileID int64, rule storage.CustomRule, appliedContent string, contentHash string) error {
	app := &storage.CustomRuleApplication{
		SubscribeFileID: fileID,
		CustomRuleID:    rule.ID,
		RuleType:        rule.Type,
		RuleMode:        rule.Mode,
		AppliedContent:  appliedContent,
		ContentHash:     contentHash,
	}

	return repo.UpsertCustomRuleApplication(ctx, app)
}
