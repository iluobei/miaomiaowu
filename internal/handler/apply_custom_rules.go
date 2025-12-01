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

	// Parse YAML data as Node to preserve structure and order
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Get the document node
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return yamlData, nil
	}

	docNode := rootNode.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return yamlData, nil
	}

	// Apply each rule based on its type using Node API
	for _, rule := range rules {
		switch rule.Type {
		case "dns":
			applyDNSRuleToNode(docNode, rule)
		case "rules":
			applyRulesRuleToNode(docNode, rule)
		case "rule-providers":
			applyRuleProvidersRuleToNode(docNode, rule)
		}
	}

	// Fix short-id fields to use double quotes before marshaling
	fixShortIdStyleInNode(&rootNode)

	// Marshal the modified node (使用2空格缩进)
	modifiedData, err := MarshalYAMLWithIndent(&rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	// Post-process to remove quotes from strings with Unicode characters (emoji)
	result := RemoveUnicodeEscapeQuotes(string(modifiedData))

	return []byte(result), nil
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

	// Parse YAML using Node API to preserve order
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Get the document node
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return yamlData, nil
	}

	docNode := rootNode.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return yamlData, nil
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

	// Fix short-id fields to use double quotes before marshaling
	fixShortIdStyleInNode(&rootNode)

	// Marshal the modified node (使用2空格缩进)
	modifiedData, err := MarshalYAMLWithIndent(&rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	// Post-process to remove quotes from strings with Unicode characters (emoji)
	result := RemoveUnicodeEscapeQuotes(string(modifiedData))

	return []byte(result), nil
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
		// Remove case-insensitive duplicates before appending
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
	newRulesSet := make(map[string]bool)
	for _, rule := range newRules {
		if ruleStr, ok := rule.(string); ok {
			newRulesSet[strings.ToLower(ruleStr)] = true
		}
	}

	// Filter out existing rules that match new rules (case-insensitive)
	var filtered []interface{}
	for _, rule := range existing {
		if ruleStr, ok := rule.(string); ok {
			// Only keep if not a duplicate (case-insensitive)
			if !newRulesSet[strings.ToLower(ruleStr)] {
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
		if idx >= 0 {
			docNode.Content[idx] = newRulesNode
		} else {
			setFieldNode(docNode, "rules", newRulesNode)
		}
	} else if rule.Mode == "prepend" {
		if existingRulesNode == nil || existingRulesNode.Kind != yaml.SequenceNode {
			// No existing rules, just set the new ones
			setFieldNode(docNode, "rules", newRulesNode)
		} else {
			// Prepend new rules to existing rules
			if newRulesNode.Kind == yaml.SequenceNode {
				combined := &yaml.Node{
					Kind:    yaml.SequenceNode,
					Style:   existingRulesNode.Style,
					Tag:     existingRulesNode.Tag,
					Content: append(newRulesNode.Content, existingRulesNode.Content...),
				}
				docNode.Content[idx] = combined
			}
		}
	} else if rule.Mode == "append" {
		if existingRulesNode == nil || existingRulesNode.Kind != yaml.SequenceNode {
			// No existing rules, just set the new ones
			setFieldNode(docNode, "rules", newRulesNode)
		} else {
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
		if idx >= 0 {
			docNode.Content[idx] = newRulesNode
		} else {
			setFieldNode(docNode, "rules", newRulesNode)
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
			// Prepend new rules to existing rules
			if newRulesNode.Kind == yaml.SequenceNode {
				combined := &yaml.Node{
					Kind:    yaml.SequenceNode,
					Style:   existingRulesNode.Style,
					Tag:     existingRulesNode.Tag,
					Content: append(newRulesNode.Content, existingRulesNode.Content...),
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
