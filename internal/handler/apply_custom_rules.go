package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"miaomiaowu/internal/logger"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/validator"

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

func applyCustomRulesToYamlFiltered(ctx context.Context, repo *storage.TrafficRepository, yamlData []byte, selectedIDs map[int64]bool) ([]byte, []string, error) {
	rules, err := repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get custom rules: %w", err)
	}

	if len(selectedIDs) > 0 {
		filtered := rules[:0]
		for _, r := range rules {
			if selectedIDs[r.ID] {
				filtered = append(filtered, r)
			}
		}
		rules = filtered
	}

	if len(rules) == 0 {
		return yamlData, nil, nil
	}

	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return nil, nil, fmt.Errorf("failed to parse YAML: %w", err)
	}
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return yamlData, nil, nil
	}
	docNode := rootNode.Content[0]
	if docNode.Kind != yaml.MappingNode {
		return yamlData, nil, nil
	}

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

	addedGroups := autoAddMissingProxyGroups(docNode)

	var configMap map[string]interface{}
	var tempBuf bytes.Buffer
	tempEncoder := yaml.NewEncoder(&tempBuf)
	tempEncoder.SetIndent(2)
	if err := tempEncoder.Encode(&rootNode); err != nil {
		return nil, nil, fmt.Errorf("编码配置用于校验失败: %w", err)
	}
	if err := yaml.Unmarshal(tempBuf.Bytes(), &configMap); err != nil {
		return nil, nil, fmt.Errorf("解析配置用于校验失败: %w", err)
	}

	validationResult := validator.ValidateClashConfig(configMap)
	if !validationResult.Valid {
		logger.Info("[应用自定义规则] [配置校验] 校验失败")
		var errorMessages []string
		for _, issue := range validationResult.Issues {
			if issue.Level == validator.ErrorLevel {
				errorMsg := issue.Message
				if issue.Location != "" {
					errorMsg = fmt.Sprintf("%s (位置: %s)", errorMsg, issue.Location)
				}
				errorMessages = append(errorMessages, errorMsg)
				logger.Info("[应用自定义规则] [配置校验] 错误", "message", errorMsg)
			}
		}
		return nil, nil, fmt.Errorf("配置校验失败: %s", strings.Join(errorMessages, "; "))
	}

	if validationResult.FixedConfig != nil {
		fixedYAML, err := yaml.Marshal(validationResult.FixedConfig)
		if err != nil {
			return nil, nil, fmt.Errorf("序列化修复配置失败: %w", err)
		}
		if err := yaml.Unmarshal(fixedYAML, &rootNode); err != nil {
			return nil, nil, fmt.Errorf("解析修复配置失败: %w", err)
		}
		for _, issue := range validationResult.Issues {
			if issue.Level == validator.WarningLevel && issue.AutoFixed {
				logger.Info("[应用自定义规则] [配置校验] 警告(已修复)", "message", issue.Message, "location", issue.Location)
			}
		}
	}

	fixShortIdStyleInNode(&rootNode)

	modifiedData, err := MarshalYAMLWithIndent(&rootNode)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal modified YAML: %w", err)
	}

	result := RemoveUnicodeEscapeQuotes(string(modifiedData))
	return []byte(result), addedGroups, nil
}

// applyCustomRulesToYaml applies enabled custom rules to the YAML data
func applyCustomRulesToYaml(ctx context.Context, repo *storage.TrafficRepository, yamlData []byte) ([]byte, []string, error) {
	return applyCustomRulesToYamlFiltered(ctx, repo, yamlData, nil)
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
				logger.Info("删除重复的MATCH规则", "rule", ruleStr)
				continue
			}

			// Only keep if not a duplicate (case-insensitive)
			if !newRulesSet[strings.ToLower(key)] {
				filtered = append(filtered, rule)
			} else {
				logger.Info("删除重复规则", "rule", ruleStr)
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
				filtered = append(filtered, node)
				continue
			}

			key := extractRuleKey(ruleStr)

			// If newRules contains MATCH rule, remove all MATCH rules from existing
			if hasMatchRule && isMatchRule(ruleStr) {
				logger.Info("删除重复的MATCH规则", "rule", ruleStr)
				continue
			}

			// Only keep if not a duplicate
			if !newRulesSet[strings.ToLower(key)] {
				filtered = append(filtered, node)
			} else {
				logger.Info("删除重复规则", "rule", ruleStr)
			}
		} else {
			// Keep non-scalar nodes as-is
			filtered = append(filtered, node)
		}
	}

	return filtered
}

// extractRuleSetRules extracts RULE-SET type rules from a rules node
func extractRuleSetRules(rulesNode *yaml.Node) []*yaml.Node {
	var ruleSetRules []*yaml.Node
	if rulesNode == nil || rulesNode.Kind != yaml.SequenceNode {
		return ruleSetRules
	}

	for _, node := range rulesNode.Content {
		if node.Kind == yaml.ScalarNode {
			trimmed := strings.TrimSpace(node.Value)
			if strings.HasPrefix(trimmed, "- ") {
				trimmed = strings.TrimSpace(trimmed[2:])
			}
			// Check if this is a RULE-SET rule (case-insensitive)
			if strings.HasPrefix(strings.ToUpper(trimmed), "RULE-SET") {
				ruleSetRules = append(ruleSetRules, node)
			}
		}
	}
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
				if policy != "DIRECT" && policy != "REJECT" && policy != "REJECT-DROP" && policy != "PROXY" && policy != "" {
					referencedGroups[policy] = true
				}
			} else if len(parts) == 2 {
				// MATCH,POLICY format
				policy := strings.TrimSpace(parts[1])
				if policy != "DIRECT" && policy != "REJECT" && policy != "REJECT-DROP" && policy != "PROXY" && policy != "" {
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
			logger.Info("自动添加缺失的代理组", "group_name", groupName)

			// Determine default proxies order based on group name
			// For domestic service group, DIRECT should be first
			var defaultProxies []*yaml.Node
			if groupName == "🔒 国内服务" {
				defaultProxies = []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "DIRECT"},
					{Kind: yaml.ScalarNode, Value: "🚀 节点选择"},
				}
			} else {
				defaultProxies = []*yaml.Node{
					{Kind: yaml.ScalarNode, Value: "🚀 节点选择"},
					{Kind: yaml.ScalarNode, Value: "DIRECT"},
				}
			}

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
						Kind:    yaml.SequenceNode,
						Content: defaultProxies,
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
			if policy != "DIRECT" && policy != "REJECT" && policy != "REJECT-DROP" && policy != "PROXY" && policy != "" {
				if !groupSet[policy] {
					groupSet[policy] = true
					groups = append(groups, policy)
				}
			}
		} else if len(parts) == 2 {
			policy := strings.TrimSpace(parts[1])
			if policy != "DIRECT" && policy != "REJECT" && policy != "REJECT-DROP" && policy != "PROXY" && policy != "" {
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
			setFieldNode(docNode, "rules", newRulesNode)
		} else {
			if newRulesNode.Kind == yaml.SequenceNode {
				filteredExisting := removeDuplicateNodesBasedOnNewRules(existingRulesNode.Content, newRulesNode.Content)

				// 找到 MATCH 规则的位置，将新规则插入到 MATCH 之前
				matchIdx := -1
				for i, node := range filteredExisting {
					if node.Kind == yaml.ScalarNode && strings.HasPrefix(strings.ToUpper(strings.TrimSpace(node.Value)), "MATCH,") {
						matchIdx = i
						break
					}
				}

				var content []*yaml.Node
				if matchIdx >= 0 {
					content = make([]*yaml.Node, 0, len(filteredExisting)+len(newRulesNode.Content))
					content = append(content, filteredExisting[:matchIdx]...)
					content = append(content, newRulesNode.Content...)
					content = append(content, filteredExisting[matchIdx:]...)
				} else {
					content = append(filteredExisting, newRulesNode.Content...)
				}

				combined := &yaml.Node{
					Kind:    yaml.SequenceNode,
					Style:   existingRulesNode.Style,
					Tag:     existingRulesNode.Tag,
					Content: content,
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

