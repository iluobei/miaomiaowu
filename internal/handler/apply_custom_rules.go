package handler

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	YamlContent      string   `json:"yaml_content"`
	AddedProxyGroups []string `json:"added_proxy_groups,omitempty"`
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
		modifiedYaml, addedGroups, err := applyCustomRulesToYaml(r.Context(), repo, []byte(payload.YamlContent))
		if err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("failed to apply custom rules: %w", err))
			return
		}


		resp := applyCustomRulesResponse{
			YamlContent:      string(modifiedYaml),
			AddedProxyGroups: addedGroups,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// applyCustomRulesToYaml applies enabled custom rules to the YAML data
// Returns modified YAML and list of added proxy groups
func applyCustomRulesToYaml(ctx context.Context, repo *storage.TrafficRepository, yamlData []byte) ([]byte, []string, error) {
	// Get enabled custom rules first
	rules, err := repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get custom rules: %w", err)
	}

	log.Printf("[applyCustomRulesToYaml] Found %d enabled custom rules", len(rules))
	if len(rules) == 0 {
		return yamlData, nil, nil
	}

	// Parse YAML data as Node to preserve structure and order
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Get the document node
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return yamlData, nil, nil
	}

	docNode := rootNode.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return yamlData, nil, nil
	}

	// Apply each rule based on its type using Node API
	for _, rule := range rules {
		log.Printf("[applyCustomRulesToYaml] Applying rule: ID=%d, Type=%s, Mode=%s, Name=%s", rule.ID, rule.Type, rule.Mode, rule.Name)
		switch rule.Type {
		case "dns":
			applyDNSRuleToNode(docNode, rule)
		case "rules":
			applyRulesRuleToNode(docNode, rule)
		case "rule-providers":
			applyRuleProvidersRuleToNode(docNode, rule)
		}
	}

	// Auto-add missing proxy groups referenced in rules
	addedGroups := autoAddMissingProxyGroups(docNode)

	// Fix short-id fields to use double quotes before marshaling
	fixShortIdStyleInNode(&rootNode)

	// Marshal the modified node (使用2空格缩进)
	modifiedData, err := MarshalYAMLWithIndent(&rootNode)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	// Post-process to remove quotes from strings with Unicode characters (emoji)
	result := RemoveUnicodeEscapeQuotes(string(modifiedData))

	return []byte(result), addedGroups, nil
}

// applyCustomRulesToYamlSmart applies custom rules with intelligent deduplication
// This function is used for auto-sync to avoid duplicate content in prepend mode
func applyCustomRulesToYamlSmart(ctx context.Context, repo *storage.TrafficRepository, yamlData []byte, subscribeFileID int64) ([]byte, []string, error) {
	// Get enabled custom rules
	rules, err := repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get custom rules: %w", err)
	}

	if len(rules) == 0 {
		return yamlData, nil, nil
	}

	// Get historical application records
	applications, err := repo.GetCustomRuleApplications(ctx, subscribeFileID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get custom rule applications: %w", err)
	}

	// Build a map of historical applications for quick lookup
	historyMap := make(map[string]*storage.CustomRuleApplication)
	for i := range applications {
		key := fmt.Sprintf("%d-%s", applications[i].CustomRuleID, applications[i].RuleType)
		historyMap[key] = &applications[i]
	}

	// Parse YAML using Node API to preserve order
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Get the document node
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return yamlData, nil, nil
	}

	docNode := rootNode.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return yamlData, nil, nil
	}

	// Apply each rule with deduplication using Node API
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
			applyDNSRuleToNodeSmart(docNode, rule, prevApp, ctx, repo, subscribeFileID, contentHash)

		case "rules":
			applyRulesRuleToNodeSmart(docNode, rule, prevApp, ctx, repo, subscribeFileID, contentHash)

		case "rule-providers":
			applyRuleProvidersRuleToNodeSmart(docNode, rule, prevApp, ctx, repo, subscribeFileID, contentHash)
		}
	}

	// Auto-add missing proxy groups referenced in rules
	addedGroups := autoAddMissingProxyGroups(docNode)

	// Fix short-id fields to use double quotes before marshaling
	fixShortIdStyleInNode(&rootNode)

	// Marshal the modified node (使用2空格缩进)
	modifiedData, err := MarshalYAMLWithIndent(&rootNode)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	// Post-process to remove quotes from strings with Unicode characters (emoji)
	result := RemoveUnicodeEscapeQuotes(string(modifiedData))

	return []byte(result), addedGroups, nil
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
	} else if rule.Mode == "append" {
		// Remove historical content if exists
		if prevApp != nil && prevApp.AppliedContent != "" {
			var historicalRules []interface{}
			if err := json.Unmarshal([]byte(prevApp.AppliedContent), &historicalRules); err == nil {
				existingRules = removeRulesFromList(existingRules, historicalRules)
			}
		}
		// Remove from existingRules any rules that match newRules (case-insensitive, based on text before second comma)
		existingRules = removeDuplicateRulesCaseInsensitive(existingRules, newRules)
		// Append new rules
		config["rules"] = append(existingRules, newRules...)
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

// removeDuplicateRulesCaseInsensitive removes rules from existing list that match newRules (case-insensitive)
func removeDuplicateRulesCaseInsensitive(existing []interface{}, newRules []interface{}) []interface{} {
	// Build a set of new rules in lowercase for O(n) lookup
	// Extract text before second comma for comparison
	newRulesSet := make(map[string]bool)
	hasMatchRule := false
	for _, rule := range newRules {
		if ruleStr, ok := rule.(string); ok {
			// Extract text before second comma
			key := extractRuleKey(ruleStr)
			newRulesSet[strings.ToLower(key)] = true

			// Check if there's a MATCH rule in newRules (handle YAML format with "- " prefix)
			if isMatchRule(ruleStr) {
				hasMatchRule = true
			}
		}
	}

	// Filter out existing rules that match new rules (case-insensitive)
	var filtered []interface{}
	for _, rule := range existing {
		if ruleStr, ok := rule.(string); ok {
			// Extract text before second comma for comparison
			key := extractRuleKey(ruleStr)

			// If newRules contains MATCH rule, remove all MATCH rules from existing
			if hasMatchRule && isMatchRule(ruleStr) {
				log.Printf("删除重复的MATCH规则: %s", ruleStr)
				continue
			}

			// Only keep if not a duplicate (case-insensitive)
			if !newRulesSet[strings.ToLower(key)] {
				filtered = append(filtered, rule)
			} else {
				log.Printf("删除重复规则: %s", ruleStr)
			}
		} else {
			// Keep non-string rules as-is
			filtered = append(filtered, rule)
		}
	}

	return filtered
}

// extractRuleKey extracts text before the second comma from a rule string
func extractRuleKey(ruleStr string) string {
	// Count commas and extract text before second comma
	commaCount := 0
	for i, ch := range ruleStr {
		if ch == ',' {
			commaCount++
			if commaCount == 2 {
				return ruleStr[:i]
			}
		}
	}
	// If less than 2 commas, return the whole string
	return ruleStr
}

// isMatchRule checks if a rule string is a MATCH rule (handles YAML format with "- " prefix)
func isMatchRule(ruleStr string) bool {
	// Trim whitespace and remove YAML list prefix "- " if present
	trimmed := strings.TrimSpace(ruleStr)
	if strings.HasPrefix(trimmed, "- ") {
		trimmed = strings.TrimSpace(trimmed[2:])
	}
	// Check if it starts with MATCH (case-insensitive)
	return strings.HasPrefix(strings.ToUpper(trimmed), "MATCH")
}

// removeDuplicateNodesBasedOnNewRules removes duplicate yaml nodes from existing based on newRules
// Uses the same logic as removeDuplicateRulesCaseInsensitive but works with yaml.Node
func removeDuplicateNodesBasedOnNewRules(existing []*yaml.Node, newRules []*yaml.Node) []*yaml.Node {
	// Build a set of new rules in lowercase for O(n) lookup
	newRulesSet := make(map[string]bool)
	hasMatchRule := false

	for _, node := range newRules {
		if node.Kind == yaml.ScalarNode {
			ruleStr := node.Value
			key := extractRuleKey(ruleStr)
			newRulesSet[strings.ToLower(key)] = true

			if isMatchRule(ruleStr) {
				hasMatchRule = true
			}
		}
	}

	// Filter out existing rules that match new rules
	var filtered []*yaml.Node

	for _, node := range existing {
		if node.Kind == yaml.ScalarNode {
			ruleStr := node.Value

			// Always preserve RULE-SET rules
			trimmed := strings.TrimSpace(ruleStr)
			if strings.HasPrefix(trimmed, "- ") {
				trimmed = strings.TrimSpace(trimmed[2:])
			}
			if strings.HasPrefix(strings.ToUpper(trimmed), "RULE-SET") {
				log.Printf("保留RULE-SET规则: %s", ruleStr)
				filtered = append(filtered, node)
				continue
			}

			key := extractRuleKey(ruleStr)

			// If newRules contains MATCH rule, remove all MATCH rules from existing
			if hasMatchRule && isMatchRule(ruleStr) {
				log.Printf("删除重复的MATCH规则: %s", ruleStr)
				continue
			}

			// Only keep if not a duplicate
			if !newRulesSet[strings.ToLower(key)] {
				filtered = append(filtered, node)
			} else {
				log.Printf("删除重复规则: %s", ruleStr)
			}
		} else {
			// Keep non-scalar nodes as-is
			filtered = append(filtered, node)
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

// extractRuleSetRules extracts RULE-SET type rules from a rules node
func extractRuleSetRules(rulesNode *yaml.Node) []*yaml.Node {
	var ruleSetRules []*yaml.Node
	if rulesNode == nil || rulesNode.Kind != yaml.SequenceNode {
		log.Printf("[extractRuleSetRules] rulesNode is nil or not a sequence")
		return ruleSetRules
	}

	log.Printf("[extractRuleSetRules] Scanning %d rules for RULE-SET entries", len(rulesNode.Content))
	for _, node := range rulesNode.Content {
		if node.Kind == yaml.ScalarNode {
			trimmed := strings.TrimSpace(node.Value)
			if strings.HasPrefix(trimmed, "- ") {
				trimmed = strings.TrimSpace(trimmed[2:])
			}
			// Check if this is a RULE-SET rule (case-insensitive)
			if strings.HasPrefix(strings.ToUpper(trimmed), "RULE-SET") {
				log.Printf("[extractRuleSetRules] Found RULE-SET rule: %s", node.Value)
				ruleSetRules = append(ruleSetRules, node)
			}
		}
	}
	log.Printf("[extractRuleSetRules] Total RULE-SET rules found: %d", len(ruleSetRules))
	return ruleSetRules
}

// autoAddMissingProxyGroups checks rules and auto-adds missing proxy groups
// Returns a list of added proxy group names
func autoAddMissingProxyGroups(docNode *yaml.Node) []string {
	// Get rules node
	rulesNode, _ := findFieldNode(docNode, "rules")
	if rulesNode == nil || rulesNode.Kind != yaml.SequenceNode {
		return []string{}
	}

	// Get proxy-groups node
	proxyGroupsNode, proxyGroupsIdx := findFieldNode(docNode, "proxy-groups")
	if proxyGroupsNode == nil || proxyGroupsNode.Kind != yaml.SequenceNode {
		return []string{}
	}

	// Collect existing proxy group names
	existingGroups := make(map[string]bool)
	for _, groupNode := range proxyGroupsNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			nameNode, _ := findFieldNode(groupNode, "name")
			if nameNode != nil && nameNode.Kind == yaml.ScalarNode {
				existingGroups[nameNode.Value] = true
			}
		}
	}

	// Collect proxy groups referenced in rules
	referencedGroups := make(map[string]bool)
	for _, ruleNode := range rulesNode.Content {
		if ruleNode.Kind == yaml.ScalarNode {
			// Parse rule: TYPE,PARAM,POLICY or TYPE,PARAM,POLICY,no-resolve
			parts := strings.Split(ruleNode.Value, ",")
			if len(parts) >= 3 {
				var policy string
				// Check if last part is "no-resolve"
				lastPart := strings.TrimSpace(parts[len(parts)-1])
				if lastPart == "no-resolve" && len(parts) >= 4 {
					// Policy is before "no-resolve": TYPE,PARAM,POLICY,no-resolve
					policy = strings.TrimSpace(parts[len(parts)-2])
				} else {
					// Policy is the last part: TYPE,PARAM,POLICY
					policy = lastPart
				}
				// Skip built-in policies
				if policy != "DIRECT" && policy != "REJECT" && policy != "PROXY" && policy != "" {
					referencedGroups[policy] = true
				}
			} else if len(parts) == 2 {
				// MATCH,POLICY format
				policy := strings.TrimSpace(parts[1])
				if policy != "DIRECT" && policy != "REJECT" && policy != "PROXY" && policy != "" {
					referencedGroups[policy] = true
				}
			}
		}
	}

	// Find missing groups
	var missingGroups []string
	for group := range referencedGroups {
		if !existingGroups[group] {
			missingGroups = append(missingGroups, group)
		}
	}

	// Add missing groups
	if len(missingGroups) > 0 {
		for _, groupName := range missingGroups {
			log.Printf("自动添加缺失的代理组: %s", groupName)

			// Create a new proxy group node
			newGroupNode := &yaml.Node{
				Kind: yaml.MappingNode,
				Content: []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "name"},
					{Kind: yaml.ScalarNode, Value: groupName},
					{Kind: yaml.ScalarNode, Value: "type"},
					{Kind: yaml.ScalarNode, Value: "select"},
					{Kind: yaml.ScalarNode, Value: "proxies"},
					{
						Kind: yaml.SequenceNode,
						Content: []*yaml.Node{
							{Kind: yaml.ScalarNode, Value: "🚀 节点选择"},
							{Kind: yaml.ScalarNode, Value: "DIRECT"},
						},
					},
				},
			}

			// Append to proxy-groups
			proxyGroupsNode.Content = append(proxyGroupsNode.Content, newGroupNode)
		}

		// Update the proxy-groups node in docNode
		if proxyGroupsIdx >= 0 {
			docNode.Content[proxyGroupsIdx] = proxyGroupsNode
		}
	}

	return missingGroups
}

// extractProxyGroupsFromRulesContent extracts proxy group names from rules content
func extractProxyGroupsFromRulesContent(content string) []string {
	var groups []string
	groupSet := make(map[string]bool)

	// Parse content as YAML to get rules list
	var rulesData interface{}
	if err := yaml.Unmarshal([]byte(content), &rulesData); err != nil {
		return groups
	}

	// Handle different formats
	var rulesList []string
	switch v := rulesData.(type) {
	case []interface{}:
		for _, rule := range v {
			if ruleStr, ok := rule.(string); ok {
				rulesList = append(rulesList, ruleStr)
			}
		}
	case map[string]interface{}:
		if rules, ok := v["rules"].([]interface{}); ok {
			for _, rule := range rules {
				if ruleStr, ok := rule.(string); ok {
					rulesList = append(rulesList, ruleStr)
				}
			}
		}
	}

	// Extract proxy groups from rules
	for _, ruleStr := range rulesList {
		parts := strings.Split(ruleStr, ",")
		if len(parts) >= 3 {
			var policy string
			lastPart := strings.TrimSpace(parts[len(parts)-1])
			if lastPart == "no-resolve" && len(parts) >= 4 {
				policy = strings.TrimSpace(parts[len(parts)-2])
			} else {
				policy = lastPart
			}
			// Skip built-in policies
			if policy != "DIRECT" && policy != "REJECT" && policy != "PROXY" && policy != "" {
				if !groupSet[policy] {
					groupSet[policy] = true
					groups = append(groups, policy)
				}
			}
		} else if len(parts) == 2 {
			policy := strings.TrimSpace(parts[1])
			if policy != "DIRECT" && policy != "REJECT" && policy != "PROXY" && policy != "" {
				if !groupSet[policy] {
					groupSet[policy] = true
					groups = append(groups, policy)
				}
			}
		}
	}

	return groups
}

// findFieldNode finds a field node by key in a mapping node
func findFieldNode(mappingNode *yaml.Node, key string) (*yaml.Node, int) {
	if mappingNode.Kind != yaml.MappingNode {
		return nil, -1
	}

	for i := 0; i < len(mappingNode.Content); i += 2 {
		keyNode := mappingNode.Content[i]
		if keyNode.Value == key {
			return mappingNode.Content[i+1], i + 1
		}
	}
	return nil, -1
}

// applyDNSRuleToNode applies DNS rule to the YAML node
func applyDNSRuleToNode(docNode *yaml.Node, rule storage.CustomRule) {
	var parsedContent yaml.Node
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
		return
	}

	// Check if parsed content is a document node
	var contentNode *yaml.Node
	if parsedContent.Kind == yaml.DocumentNode && len(parsedContent.Content) > 0 {
		contentNode = parsedContent.Content[0]
	} else {
		contentNode = &parsedContent
	}

	// Check if user input contains "dns:" key
	if dnsNode, _ := findFieldNode(contentNode, "dns"); dnsNode != nil {
		// Replace the entire dns block
		setFieldNode(docNode, "dns", dnsNode)
	} else {
		// Otherwise, replace with the entire content
		setFieldNode(docNode, "dns", contentNode)
	}
}

// applyRulesRuleToNode applies rules to the YAML node
func applyRulesRuleToNode(docNode *yaml.Node, rule storage.CustomRule) {
	var parsedContent yaml.Node
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
		return
	}

	// Get content node
	var contentNode *yaml.Node
	if parsedContent.Kind == yaml.DocumentNode && len(parsedContent.Content) > 0 {
		contentNode = parsedContent.Content[0]
	} else {
		contentNode = &parsedContent
	}

	// Check if it contains "rules:" key
	var newRulesNode *yaml.Node
	if contentNode.Kind == yaml.MappingNode {
		if rulesNode, _ := findFieldNode(contentNode, "rules"); rulesNode != nil {
			newRulesNode = rulesNode
		}
	}

	// If not found as mapping, treat the content as rules array
	if newRulesNode == nil {
		if contentNode.Kind == yaml.SequenceNode {
			newRulesNode = contentNode
		} else {
			return
		}
	}

	// Get existing rules node
	existingRulesNode, idx := findFieldNode(docNode, "rules")

	if rule.Mode == "replace" {
		// Extract RULE-SET rules from existing rules to preserve them
		ruleSetRules := extractRuleSetRules(existingRulesNode)
		log.Printf("[applyRulesRuleToNode] Replace mode: extracted %d RULE-SET rules from existing", len(ruleSetRules))

		// If we have RULE-SET rules, append them to new rules
		if len(ruleSetRules) > 0 && newRulesNode.Kind == yaml.SequenceNode {
			log.Printf("[applyRulesRuleToNode] Appending %d RULE-SET rules to end of new rules", len(ruleSetRules))
			combined := &yaml.Node{
				Kind:    yaml.SequenceNode,
				Style:   newRulesNode.Style,
				Tag:     newRulesNode.Tag,
				Content: append(newRulesNode.Content, ruleSetRules...),
			}
			if idx >= 0 {
				docNode.Content[idx] = combined
			} else {
				setFieldNode(docNode, "rules", combined)
			}
		} else {
			if idx >= 0 {
				docNode.Content[idx] = newRulesNode
			} else {
				setFieldNode(docNode, "rules", newRulesNode)
			}
		}
	} else if rule.Mode == "prepend" {
		if existingRulesNode == nil || existingRulesNode.Kind != yaml.SequenceNode {
			// No existing rules, just set the new ones
			setFieldNode(docNode, "rules", newRulesNode)
		} else {
			// Prepend new rules to existing rules with deduplication
			if newRulesNode.Kind == yaml.SequenceNode {
				// Remove duplicates from existing rules before prepending
				filteredExisting := removeDuplicateNodesBasedOnNewRules(existingRulesNode.Content, newRulesNode.Content)

				combined := &yaml.Node{
					Kind:    yaml.SequenceNode,
					Style:   existingRulesNode.Style,
					Tag:     existingRulesNode.Tag,
					Content: append(newRulesNode.Content, filteredExisting...),
				}
				docNode.Content[idx] = combined
			}
		}
	} else if rule.Mode == "append" {
		if existingRulesNode == nil || existingRulesNode.Kind != yaml.SequenceNode {
			// No existing rules, just set the new ones
			setFieldNode(docNode, "rules", newRulesNode)
		} else {
			// Append new rules to existing rules with deduplication
			if newRulesNode.Kind == yaml.SequenceNode {
				// Remove duplicates from existing rules before appending
				filteredExisting := removeDuplicateNodesBasedOnNewRules(existingRulesNode.Content, newRulesNode.Content)

				combined := &yaml.Node{
					Kind:    yaml.SequenceNode,
					Style:   existingRulesNode.Style,
					Tag:     existingRulesNode.Tag,
					Content: append(filteredExisting, newRulesNode.Content...),
				}
				docNode.Content[idx] = combined
			}
		}
	}
}

// applyRuleProvidersRuleToNode applies rule-providers to the YAML node
func applyRuleProvidersRuleToNode(docNode *yaml.Node, rule storage.CustomRule) {
	var parsedContent yaml.Node
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
		return
	}

	// Get content node
	var contentNode *yaml.Node
	if parsedContent.Kind == yaml.DocumentNode && len(parsedContent.Content) > 0 {
		contentNode = parsedContent.Content[0]
	} else {
		contentNode = &parsedContent
	}

	// Check if it contains "rule-providers:" key
	var newProvidersNode *yaml.Node
	if contentNode.Kind == yaml.MappingNode {
		if providersNode, _ := findFieldNode(contentNode, "rule-providers"); providersNode != nil {
			newProvidersNode = providersNode
		} else {
			newProvidersNode = contentNode
		}
	} else {
		return
	}

	// Get existing rule-providers node
	existingProvidersNode, idx := findFieldNode(docNode, "rule-providers")

	if rule.Mode == "replace" {
		// In replace mode, merge new providers with existing ones (new providers take precedence)
		if existingProvidersNode != nil && existingProvidersNode.Kind == yaml.MappingNode && newProvidersNode.Kind == yaml.MappingNode {
			log.Printf("[applyRuleProvidersRuleToNode] Replace mode: merging new rule-providers with existing ones")
			mergeMapNodes(existingProvidersNode, newProvidersNode)
		} else {
			// No existing providers or wrong type, just set the new ones
			if idx >= 0 {
				docNode.Content[idx] = newProvidersNode
			} else {
				setFieldNode(docNode, "rule-providers", newProvidersNode)
			}
		}
	} else if rule.Mode == "prepend" {
		if existingProvidersNode == nil || existingProvidersNode.Kind != yaml.MappingNode {
			// No existing providers, just set the new ones
			setFieldNode(docNode, "rule-providers", newProvidersNode)
		} else {
			// Merge: new providers take precedence
			if newProvidersNode.Kind == yaml.MappingNode {
				mergeMapNodes(existingProvidersNode, newProvidersNode)
			}
		}
	}
}

// setFieldNode sets or adds a field in a mapping node
func setFieldNode(mappingNode *yaml.Node, key string, valueNode *yaml.Node) {
	if mappingNode.Kind != yaml.MappingNode {
		return
	}

	// Check if key already exists
	for i := 0; i < len(mappingNode.Content); i += 2 {
		keyNode := mappingNode.Content[i]
		if keyNode.Value == key {
			// Replace value
			mappingNode.Content[i+1] = valueNode
			return
		}
	}

	// Add new key-value pair
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: key,
	}
	mappingNode.Content = append(mappingNode.Content, keyNode, valueNode)
}

// mergeMapNodes merges newNode into existingNode (new values take precedence)
func mergeMapNodes(existingNode *yaml.Node, newNode *yaml.Node) {
	if existingNode.Kind != yaml.MappingNode || newNode.Kind != yaml.MappingNode {
		return
	}

	// Iterate through new node's key-value pairs
	for i := 0; i < len(newNode.Content); i += 2 {
		newKeyNode := newNode.Content[i]
		newValueNode := newNode.Content[i+1]

		// Find if key exists in existing node
		found := false
		for j := 0; j < len(existingNode.Content); j += 2 {
			existingKeyNode := existingNode.Content[j]
			if existingKeyNode.Value == newKeyNode.Value {
				// Replace value
				existingNode.Content[j+1] = newValueNode
				found = true
				break
			}
		}

		// If not found, append
		if !found {
			existingNode.Content = append(existingNode.Content, newKeyNode, newValueNode)
		}
	}
}

// applyDNSRuleToNodeSmart applies DNS rule to YAML node (smart version for auto-sync)
func applyDNSRuleToNodeSmart(docNode *yaml.Node, rule storage.CustomRule, prevApp *storage.CustomRuleApplication, ctx context.Context, repo *storage.TrafficRepository, subscribeFileID int64, contentHash string) {
	// DNS rules always replace, no deduplication needed
	applyDNSRuleToNode(docNode, rule)

	// Record the application
	_ = recordApplication(ctx, repo, subscribeFileID, rule, "", contentHash)
}

// applyRulesRuleToNodeSmart applies rules to YAML node with deduplication (smart version for auto-sync)
func applyRulesRuleToNodeSmart(docNode *yaml.Node, rule storage.CustomRule, prevApp *storage.CustomRuleApplication, ctx context.Context, repo *storage.TrafficRepository, subscribeFileID int64, contentHash string) {
	var parsedContent yaml.Node
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
		return
	}

	// Get content node
	var contentNode *yaml.Node
	if parsedContent.Kind == yaml.DocumentNode && len(parsedContent.Content) > 0 {
		contentNode = parsedContent.Content[0]
	} else {
		contentNode = &parsedContent
	}

	// Check if it contains "rules:" key
	var newRulesNode *yaml.Node
	if contentNode.Kind == yaml.MappingNode {
		if rulesNode, _ := findFieldNode(contentNode, "rules"); rulesNode != nil {
			newRulesNode = rulesNode
		}
	}

	// If not found as mapping, treat the content as rules array
	if newRulesNode == nil {
		if contentNode.Kind == yaml.SequenceNode {
			newRulesNode = contentNode
		} else {
			return
		}
	}

	// Get existing rules node
	existingRulesNode, idx := findFieldNode(docNode, "rules")

	if rule.Mode == "replace" {
		// Extract RULE-SET rules from existing rules to preserve them
		ruleSetRules := extractRuleSetRules(existingRulesNode)

		// If we have RULE-SET rules, append them to new rules
		if len(ruleSetRules) > 0 && newRulesNode.Kind == yaml.SequenceNode {
			combined := &yaml.Node{
				Kind:    yaml.SequenceNode,
				Style:   newRulesNode.Style,
				Tag:     newRulesNode.Tag,
				Content: append(newRulesNode.Content, ruleSetRules...),
			}
			if idx >= 0 {
				docNode.Content[idx] = combined
			} else {
				setFieldNode(docNode, "rules", combined)
			}
		} else {
			if idx >= 0 {
				docNode.Content[idx] = newRulesNode
			} else {
				setFieldNode(docNode, "rules", newRulesNode)
			}
		}
	} else if rule.Mode == "prepend" {
		if existingRulesNode == nil || existingRulesNode.Kind != yaml.SequenceNode {
			// No existing rules, just set the new ones
			setFieldNode(docNode, "rules", newRulesNode)
		} else {
			// Remove historical content if exists
			if prevApp != nil && prevApp.AppliedContent != "" {
				var historicalRules []interface{}
				if err := json.Unmarshal([]byte(prevApp.AppliedContent), &historicalRules); err == nil {
					existingRulesNode.Content = removeNodesFromSequence(existingRulesNode.Content, historicalRules)
				}
			}
			// Prepend new rules to existing rules with deduplication
			if newRulesNode.Kind == yaml.SequenceNode {
				// Remove duplicates from existing rules before prepending
				filteredExisting := removeDuplicateNodesBasedOnNewRules(existingRulesNode.Content, newRulesNode.Content)

				combined := &yaml.Node{
					Kind:    yaml.SequenceNode,
					Style:   existingRulesNode.Style,
					Tag:     existingRulesNode.Tag,
					Content: append(newRulesNode.Content, filteredExisting...),
				}
				docNode.Content[idx] = combined
			}
		}
	} else if rule.Mode == "append" {
		if existingRulesNode == nil || existingRulesNode.Kind != yaml.SequenceNode {
			// No existing rules, just set the new ones
			setFieldNode(docNode, "rules", newRulesNode)
		} else {
			// Remove historical content if exists
			if prevApp != nil && prevApp.AppliedContent != "" {
				var historicalRules []interface{}
				if err := json.Unmarshal([]byte(prevApp.AppliedContent), &historicalRules); err == nil {
					existingRulesNode.Content = removeNodesFromSequence(existingRulesNode.Content, historicalRules)
				}
			}
			// Append new rules to existing rules
			if newRulesNode.Kind == yaml.SequenceNode {
				combined := &yaml.Node{
					Kind:    yaml.SequenceNode,
					Style:   existingRulesNode.Style,
					Tag:     existingRulesNode.Tag,
					Content: append(existingRulesNode.Content, newRulesNode.Content...),
				}
				docNode.Content[idx] = combined
			}
		}
	}

	// Serialize applied content for tracking (convert nodes to interface{} for JSON)
	var appliedRules []interface{}
	for _, node := range newRulesNode.Content {
		var val interface{}
		if err := node.Decode(&val); err == nil {
			appliedRules = append(appliedRules, val)
		}
	}
	appliedJSON, _ := json.Marshal(appliedRules)
	_ = recordApplication(ctx, repo, subscribeFileID, rule, string(appliedJSON), contentHash)
}

// applyRuleProvidersRuleToNodeSmart applies rule-providers to YAML node with deduplication (smart version for auto-sync)
func applyRuleProvidersRuleToNodeSmart(docNode *yaml.Node, rule storage.CustomRule, prevApp *storage.CustomRuleApplication, ctx context.Context, repo *storage.TrafficRepository, subscribeFileID int64, contentHash string) {
	var parsedContent yaml.Node
	if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err != nil {
		return
	}

	// Get content node
	var contentNode *yaml.Node
	if parsedContent.Kind == yaml.DocumentNode && len(parsedContent.Content) > 0 {
		contentNode = parsedContent.Content[0]
	} else {
		contentNode = &parsedContent
	}

	// Check if it contains "rule-providers:" key
	var newProvidersNode *yaml.Node
	if contentNode.Kind == yaml.MappingNode {
		if providersNode, _ := findFieldNode(contentNode, "rule-providers"); providersNode != nil {
			newProvidersNode = providersNode
		} else {
			newProvidersNode = contentNode
		}
	} else {
		return
	}

	// Get existing rule-providers node
	existingProvidersNode, idx := findFieldNode(docNode, "rule-providers")

	if rule.Mode == "replace" {
		if idx >= 0 {
			docNode.Content[idx] = newProvidersNode
		} else {
			setFieldNode(docNode, "rule-providers", newProvidersNode)
		}
	} else if rule.Mode == "prepend" {
		if existingProvidersNode == nil || existingProvidersNode.Kind != yaml.MappingNode {
			// No existing providers, just set the new ones
			setFieldNode(docNode, "rule-providers", newProvidersNode)
		} else {
			// Remove historical providers if exists
			if prevApp != nil && prevApp.AppliedContent != "" {
				var historicalProviders map[string]interface{}
				if err := json.Unmarshal([]byte(prevApp.AppliedContent), &historicalProviders); err == nil {
					removeKeysFromMapNode(existingProvidersNode, historicalProviders)
				}
			}
			// Merge: new providers take precedence
			if newProvidersNode.Kind == yaml.MappingNode {
				mergeMapNodes(existingProvidersNode, newProvidersNode)
			}
		}
	}

	// Serialize applied content for tracking
	var appliedProviders map[string]interface{}
	if err := newProvidersNode.Decode(&appliedProviders); err == nil {
		appliedJSON, _ := json.Marshal(appliedProviders)
		_ = recordApplication(ctx, repo, subscribeFileID, rule, string(appliedJSON), contentHash)
	}
}

// removeNodesFromSequence removes nodes from a sequence that match the given values
func removeNodesFromSequence(nodes []*yaml.Node, toRemove []interface{}) []*yaml.Node {
	// Build a set of values to remove
	removeSet := make(map[string]bool)
	for _, val := range toRemove {
		if str, ok := val.(string); ok {
			removeSet[str] = true
		}
	}

	// Filter out nodes that match
	var filtered []*yaml.Node
	for _, node := range nodes {
		var val interface{}
		if err := node.Decode(&val); err == nil {
			if str, ok := val.(string); ok {
				if !removeSet[str] {
					filtered = append(filtered, node)
				}
				continue
			}
		}
		// Keep non-string nodes
		filtered = append(filtered, node)
	}
	return filtered
}

// removeKeysFromMapNode removes keys from a map node
func removeKeysFromMapNode(mapNode *yaml.Node, keysToRemove map[string]interface{}) {
	if mapNode.Kind != yaml.MappingNode {
		return
	}

	// Create a new content slice without the keys to remove
	var newContent []*yaml.Node
	for i := 0; i < len(mapNode.Content); i += 2 {
		if i+1 < len(mapNode.Content) {
			keyNode := mapNode.Content[i]
			valueNode := mapNode.Content[i+1]

			// Check if this key should be removed
			if _, shouldRemove := keysToRemove[keyNode.Value]; !shouldRemove {
				newContent = append(newContent, keyNode, valueNode)
			}
		}
	}
	mapNode.Content = newContent
}
