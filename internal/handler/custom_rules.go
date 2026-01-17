package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"miaomiaowu/internal/logger"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

type customRuleRequest struct {
	Name    string `json:"name"`
	Type    string `json:"type"` // "dns", "rules", "rule-providers"
	Mode    string `json:"mode"` // "replace", "prepend", "append" (append only for rules type)
	Content string `json:"content"`
	Enabled bool   `json:"enabled"`
}

type customRuleResponse struct {
	ID              int64    `json:"id"`
	Name            string   `json:"name"`
	Type            string   `json:"type"`
	Mode            string   `json:"mode"`
	Content         string   `json:"content"`
	Enabled         bool     `json:"enabled"`
	CreatedAt       string   `json:"created_at"`
	UpdatedAt       string   `json:"updated_at"`
	AddedProxyGroups []string `json:"added_proxy_groups,omitempty"`
}

func NewCustomRulesHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("custom rules handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		// Check if user is admin
		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if user.Role != storage.RoleAdmin {
			writeError(w, http.StatusForbidden, errors.New("only admin can manage custom rules"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleListCustomRules(w, r, repo)
		case http.MethodPost:
			handleCreateCustomRule(w, r, repo)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET and POST are supported"))
		}
	})
}

func NewCustomRuleHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("custom rule handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		// Check if user is admin
		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		if user.Role != storage.RoleAdmin {
			writeError(w, http.StatusForbidden, errors.New("only admin can manage custom rules"))
			return
		}

		// Extract rule ID from URL path
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/custom-rules/")
		idStr := strings.TrimSpace(path)
		if idStr == "" {
			writeError(w, http.StatusBadRequest, errors.New("rule id is required"))
			return
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid rule id"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleGetCustomRule(w, r, repo, id)
		case http.MethodPut:
			handleUpdateCustomRule(w, r, repo, id)
		case http.MethodDelete:
			handleDeleteCustomRule(w, r, repo, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET, PUT and DELETE are supported"))
		}
	})
}

func handleListCustomRules(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository) {
	ruleType := strings.TrimSpace(r.URL.Query().Get("type"))

	rules, err := repo.ListCustomRules(r.Context(), ruleType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	response := make([]customRuleResponse, 0, len(rules))
	for _, rule := range rules {
		response = append(response, customRuleResponse{
			ID:        rule.ID,
			Name:      rule.Name,
			Type:      rule.Type,
			Mode:      rule.Mode,
			Content:   rule.Content,
			Enabled:   rule.Enabled,
			CreatedAt: rule.CreatedAt.Format("2006-01-02 15:04:05"),
			UpdatedAt: rule.UpdatedAt.Format("2006-01-02 15:04:05"),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func handleGetCustomRule(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, id int64) {
	rule, err := repo.GetCustomRule(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrCustomRuleNotFound) {
			writeError(w, http.StatusNotFound, errors.New("custom rule not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	response := customRuleResponse{
		ID:        rule.ID,
		Name:      rule.Name,
		Type:      rule.Type,
		Mode:      rule.Mode,
		Content:   rule.Content,
		Enabled:   rule.Enabled,
		CreatedAt: rule.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt: rule.UpdatedAt.Format("2006-01-02 15:04:05"),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func handleCreateCustomRule(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository) {
	var payload customRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Validate YAML format if type is DNS or rule-providers
	if payload.Type == "dns" || payload.Type == "rule-providers" {
		var yamlData interface{}
		if err := yaml.Unmarshal([]byte(payload.Content), &yamlData); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid YAML format: "+err.Error()))
			return
		}
	}

	// Validate rules format (should be valid YAML array or string lines)
	if payload.Type == "rules" {
		// Check if it's valid YAML
		var yamlData interface{}
		if err := yaml.Unmarshal([]byte(payload.Content), &yamlData); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid YAML format: "+err.Error()))
			return
		}
	}

	rule := &storage.CustomRule{
		Name:    payload.Name,
		Type:    payload.Type,
		Mode:    payload.Mode,
		Content: payload.Content,
		Enabled: payload.Enabled,
	}

	if err := repo.CreateCustomRule(r.Context(), rule); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Trigger auto-sync for subscribe files with auto-sync enabled (synchronously to collect added groups)
	addedGroups := triggerAutoSync(repo, rule.ID)
	logger.Info("[CreateCustomRule] 为规则添加代理组", "name", rule.Name, "added_groups", addedGroups, "count", len(addedGroups))

	response := customRuleResponse{
		ID:              rule.ID,
		Name:            rule.Name,
		Type:            rule.Type,
		Mode:            rule.Mode,
		Content:         rule.Content,
		Enabled:         rule.Enabled,
		CreatedAt:       rule.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:       rule.UpdatedAt.Format("2006-01-02 15:04:05"),
		AddedProxyGroups: addedGroups,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)
}

func handleUpdateCustomRule(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, id int64) {
	var payload customRuleRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Validate YAML format if type is DNS or rule-providers
	if payload.Type == "dns" || payload.Type == "rule-providers" {
		var yamlData interface{}
		if err := yaml.Unmarshal([]byte(payload.Content), &yamlData); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid YAML format: "+err.Error()))
			return
		}
	}

	// Validate rules format
	if payload.Type == "rules" {
		var yamlData interface{}
		if err := yaml.Unmarshal([]byte(payload.Content), &yamlData); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid YAML format: "+err.Error()))
			return
		}
	}

	rule := &storage.CustomRule{
		ID:      id,
		Name:    payload.Name,
		Type:    payload.Type,
		Mode:    payload.Mode,
		Content: payload.Content,
		Enabled: payload.Enabled,
	}

	if err := repo.UpdateCustomRule(r.Context(), rule); err != nil {
		if errors.Is(err, storage.ErrCustomRuleNotFound) {
			writeError(w, http.StatusNotFound, errors.New("custom rule not found"))
			return
		}
		if strings.Contains(err.Error(), "already exists") {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Trigger auto-sync for subscribe files with auto-sync enabled (synchronously to collect added groups)
	addedGroups := triggerAutoSync(repo, rule.ID)
	logger.Info("[UpdateCustomRule] 为规则添加代理组", "name", rule.Name, "added_groups", addedGroups, "count", len(addedGroups))

	response := customRuleResponse{
		ID:              rule.ID,
		Name:            rule.Name,
		Type:            rule.Type,
		Mode:            rule.Mode,
		Content:         rule.Content,
		Enabled:         rule.Enabled,
		CreatedAt:       rule.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:       rule.UpdatedAt.Format("2006-01-02 15:04:05"),
		AddedProxyGroups: addedGroups,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

func handleDeleteCustomRule(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, id int64) {
	if err := repo.DeleteCustomRule(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrCustomRuleNotFound) {
			writeError(w, http.StatusNotFound, errors.New("custom rule not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// triggerAutoSync triggers automatic synchronization of custom rules to subscribe files with auto-sync enabled
// Returns list of all proxy groups that were added across all files
func triggerAutoSync(repo *storage.TrafficRepository, ruleID int64) []string {
	ctx := context.Background()

	// Get all subscribe files with auto-sync enabled
	files, err := repo.GetSubscribeFilesWithAutoSync(ctx)
	if err != nil {
		logger.Info("[AutoSync] Failed to get subscribe files with auto-sync", "error", err)
		return nil
	}

	if len(files) == 0 {
		return nil
	}

	logger.Info("[AutoSync] 同步自定义规则到订阅文件", "rule_id", ruleID, "file_count", len(files))

	// Collect all added groups
	allAddedGroups := make(map[string]bool)

	// Sync to each file
	for _, file := range files {
		addedGroups, err := syncCustomRulesToFile(ctx, repo, file)
		if err != nil {
			logger.Info("[AutoSync] Failed to sync to file (ID)", "filename", file.Filename, "id", file.ID, "error", err)
		} else {
			logger.Info("[AutoSync] Successfully synced to file (ID)", "filename", file.Filename, "id", file.ID)
			// Collect added groups
			for _, group := range addedGroups {
				allAddedGroups[group] = true
			}
		}
	}

	// Convert map to slice
	var result []string
	for group := range allAddedGroups {
		result = append(result, group)
	}

	return result
}

// syncCustomRulesToFile synchronizes all custom rules to a specific subscribe file
// Returns list of added proxy groups
func syncCustomRulesToFile(ctx context.Context, repo *storage.TrafficRepository, file storage.SubscribeFile) ([]string, error) {
	// Read the subscribe file
	filePath := filepath.Join("subscribes", file.Filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("read file: %w", err)
	}

	// Apply custom rules using the smart algorithm
	modified, addedGroups, err := applyCustomRulesToYamlSmart(ctx, repo, data, file.ID)
	if err != nil {
		return nil, fmt.Errorf("apply custom rules: %w", err)
	}

	// Write back to file
	if err := os.WriteFile(filePath, modified, 0644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}

	return addedGroups, nil
}

// checkAndAddMissingProxyGroupsForRule checks if a rules-type custom rule references missing proxy groups
// and adds them to all subscribe files
func checkAndAddMissingProxyGroupsForRule(ctx context.Context, repo *storage.TrafficRepository, rule *storage.CustomRule) ([]string, error) {
	if rule.Type != "rules" {
		return nil, nil
	}

	// Extract proxy groups from rule content
	referencedGroups := extractProxyGroupsFromRulesContent(rule.Content)
	if len(referencedGroups) == 0 {
		return nil, nil
	}

	// Get all subscribe files
	files, err := repo.ListSubscribeFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscribe files: %w", err)
	}

	addedGroups := make(map[string]bool)

	// Process each file
	for _, file := range files {
		filePath := filepath.Join("data", "subscriptions", file.FileShortCode+".yaml")
		data, err := os.ReadFile(filePath)
		if err != nil {
			logger.Info("Warning: failed to read file", "value", filePath, "error", err)
			continue
		}

		// Parse YAML
		var rootNode yaml.Node
		if err := yaml.Unmarshal(data, &rootNode); err != nil {
			logger.Info("Warning: failed to parse YAML for file", "value", filePath, "error", err)
			continue
		}

		if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
			continue
		}

		docNode := rootNode.Content[0]
		if docNode.Kind != yaml.MappingNode {
			continue
		}

		// Get proxy-groups node
		proxyGroupsNode, proxyGroupsIdx := findFieldNode(docNode, "proxy-groups")
		if proxyGroupsNode == nil || proxyGroupsNode.Kind != yaml.SequenceNode {
			continue
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

		// Find and add missing groups
		needsUpdate := false
		for _, groupName := range referencedGroups {
			if !existingGroups[groupName] {
				logger.Info("为订阅文件 自动添加代理组", "name", file.Name, "param", groupName)
				addedGroups[groupName] = true
				needsUpdate = true

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

				// Create new proxy group node
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

				proxyGroupsNode.Content = append(proxyGroupsNode.Content, newGroupNode)
			}
		}

		// If we added groups, save the file
		if needsUpdate {
			docNode.Content[proxyGroupsIdx] = proxyGroupsNode

			// Marshal back to YAML
			modifiedData, err := MarshalYAMLWithIndent(&rootNode)
			if err != nil {
				logger.Info("Warning: failed to marshal YAML for file", "value", filePath, "error", err)
				continue
			}

			result := RemoveUnicodeEscapeQuotes(string(modifiedData))
			if err := os.WriteFile(filePath, []byte(result), 0644); err != nil {
				logger.Info("Warning: failed to write file", "value", filePath, "error", err)
				continue
			}
		}
	}

	// Convert map to slice
	var result []string
	for group := range addedGroups {
		result = append(result, group)
	}

	return result, nil
}
