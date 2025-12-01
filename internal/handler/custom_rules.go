package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
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
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Type      string `json:"type"`
	Mode      string `json:"mode"`
	Content   string `json:"content"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
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
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(response)

	// Trigger auto-sync for subscribe files with auto-sync enabled
	go triggerAutoSync(repo, rule.ID)
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

	// Trigger auto-sync for subscribe files with auto-sync enabled
	go triggerAutoSync(repo, rule.ID)
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
func triggerAutoSync(repo *storage.TrafficRepository, ruleID int64) {
	ctx := context.Background()

	// Get all subscribe files with auto-sync enabled
	files, err := repo.GetSubscribeFilesWithAutoSync(ctx)
	if err != nil {
		log.Printf("[AutoSync] Failed to get subscribe files with auto-sync: %v", err)
		return
	}

	if len(files) == 0 {
		return
	}

	log.Printf("[AutoSync] Syncing custom rule %d to %d subscribe files", ruleID, len(files))

	// Sync to each file
	for _, file := range files {
		if err := syncCustomRulesToFile(ctx, repo, file); err != nil {
			log.Printf("[AutoSync] Failed to sync to file %s (ID: %d): %v", file.Filename, file.ID, err)
		} else {
			log.Printf("[AutoSync] Successfully synced to file %s (ID: %d)", file.Filename, file.ID)
		}
	}
}

// syncCustomRulesToFile synchronizes all custom rules to a specific subscribe file
func syncCustomRulesToFile(ctx context.Context, repo *storage.TrafficRepository, file storage.SubscribeFile) error {
	// Read the subscribe file
	filePath := filepath.Join("subscribes", file.Filename)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("read file: %w", err)
	}

	// Apply custom rules using the smart algorithm
	modified, err := applyCustomRulesToYamlSmart(ctx, repo, data, file.ID)
	if err != nil {
		return fmt.Errorf("apply custom rules: %w", err)
	}

	// Write back to file
	if err := os.WriteFile(filePath, modified, 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}
