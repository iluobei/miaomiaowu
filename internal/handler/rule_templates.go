package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type RuleTemplatesHandler struct {
	repo *storage.TrafficRepository
}

func NewRuleTemplatesHandler(repo *storage.TrafficRepository) *RuleTemplatesHandler {
	return &RuleTemplatesHandler{repo: repo}
}

func (h *RuleTemplatesHandler) isAdmin(r *http.Request) bool {
	username := auth.UsernameFromContext(r.Context())
	user, err := h.repo.GetUser(r.Context(), username)
	return err == nil && user.Role == storage.RoleAdmin
}

func (h *RuleTemplatesHandler) canView(r *http.Request, filename string) bool {
	if h.isAdmin(r) {
		return true
	}
	owner, _ := h.repo.GetRuleTemplateOwner(r.Context(), filename)
	return owner == "" || owner == auth.UsernameFromContext(r.Context()) || h.repo.IsRuleTemplatePublic(r.Context(), filename)
}

func (h *RuleTemplatesHandler) canModify(r *http.Request, filename string) bool {
	if h.isAdmin(r) {
		return true
	}
	owner, _ := h.repo.GetRuleTemplateOwner(r.Context(), filename)
	return owner != "" && owner == auth.UsernameFromContext(r.Context())
}

func (h *RuleTemplatesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Remove /api/rule-templates prefix
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/rule-templates")

	switch {
	case path == "" || path == "/":
		// List templates
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleListTemplates(w, r)
	case path == "/upload":
		// Upload template
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleUploadTemplate(w, r)
	case path == "/rename":
		// Rename template
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleRenameTemplate(w, r)
	case path == "/visibility":
		h.handleVisibility(w, r)
	default:
		// Extract template name from path (remove leading slash)
		templateName := strings.TrimPrefix(path, "/")

		switch r.Method {
		case http.MethodGet:
			if !h.canView(r, templateName) {
				http.Error(w, "无权查看该模板", http.StatusForbidden)
				return
			}
			// Get specific template content
			h.handleGetTemplate(w, r, templateName)
		case http.MethodPut:
			if !h.canModify(r, templateName) {
				http.Error(w, "无权修改该模板", http.StatusForbidden)
				return
			}
			// Update template content
			h.handleUpdateTemplate(w, r, templateName)
		case http.MethodDelete:
			if !h.canModify(r, templateName) {
				http.Error(w, "无权删除该模板", http.StatusForbidden)
				return
			}
			// Delete template
			h.handleDeleteTemplate(w, r, templateName)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (h *RuleTemplatesHandler) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	templatesDir := "rule_templates"

	// Read directory
	entries, err := os.ReadDir(templatesDir)
	if err != nil {
		http.Error(w, "Failed to read templates directory", http.StatusInternalServerError)
		return
	}

	// Clash templates use YAML; Surge templates use .conf.
	var templates []string
	for _, entry := range entries {
		name := strings.ToLower(entry.Name())
		if !entry.IsDir() && h.canView(r, entry.Name()) && (strings.HasSuffix(name, ".yaml") || strings.HasSuffix(name, ".yml") || strings.HasSuffix(name, ".conf") || strings.HasSuffix(name, ".lcf")) {
			templates = append(templates, entry.Name())
		}
	}

	// Return JSON response
	visibility := make(map[string]bool, len(templates))
	owners, _ := h.repo.ListRuleTemplateOwners(r.Context())
	for _, filename := range templates {
		visibility[filename] = owners[filename] == "" || h.repo.IsRuleTemplatePublic(r.Context(), filename)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates, "visibility": visibility, "owners": owners,
	})
}

func (h *RuleTemplatesHandler) handleGetTemplate(w http.ResponseWriter, r *http.Request, templateName string) {
	// Security: Prevent directory traversal
	if strings.Contains(templateName, "..") || strings.Contains(templateName, "/") || strings.Contains(templateName, "\\") {
		http.Error(w, "Invalid template name", http.StatusBadRequest)
		return
	}

	templatesDir := "rule_templates"
	templatePath := filepath.Join(templatesDir, templateName)

	// Check if file exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		http.Error(w, "Template not found", http.StatusNotFound)
		return
	}

	// Read file content
	content, err := os.ReadFile(templatePath)
	if err != nil {
		http.Error(w, "Failed to read template", http.StatusInternalServerError)
		return
	}

	// Return JSON response with content
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"content": string(content),
	})
}

func (h *RuleTemplatesHandler) handleUpdateTemplate(w http.ResponseWriter, r *http.Request, templateName string) {
	// Security: Prevent directory traversal
	if strings.Contains(templateName, "..") || strings.Contains(templateName, "/") || strings.Contains(templateName, "\\") {
		http.Error(w, "Invalid template name", http.StatusBadRequest)
		return
	}

	templatesDir := "rule_templates"
	templatePath := filepath.Join(templatesDir, templateName)

	// Check if file exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "模板文件不存在",
		})
		return
	}

	// Parse request body
	var payload struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Write content to file
	if err := os.WriteFile(templatePath, []byte(payload.Content), 0644); err != nil {
		http.Error(w, "Failed to save template", http.StatusInternalServerError)
		return
	}

	// 异步刷新绑定了此模板的订阅
	if h.repo != nil {
		username := auth.UsernameFromContext(r.Context())
		go RefreshSubscriptionsByTemplate(h.repo, username, templateName)
	}

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "模板保存成功",
	})
}

func (h *RuleTemplatesHandler) handleDeleteTemplate(w http.ResponseWriter, r *http.Request, templateName string) {
	// Security: Prevent directory traversal
	if strings.Contains(templateName, "..") || strings.Contains(templateName, "/") || strings.Contains(templateName, "\\") {
		http.Error(w, "Invalid template name", http.StatusBadRequest)
		return
	}

	templatesDir := "rule_templates"
	templatePath := filepath.Join(templatesDir, templateName)

	// Check if file exists
	if _, err := os.Stat(templatePath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "模板文件不存在",
		})
		return
	}

	// Delete the file
	if err := os.Remove(templatePath); err != nil {
		http.Error(w, "Failed to delete template", http.StatusInternalServerError)
		return
	}
	_ = h.repo.DeleteRuleTemplateOwner(r.Context(), templateName)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "模板删除成功",
	})
}

func (h *RuleTemplatesHandler) handleRenameTemplate(w http.ResponseWriter, r *http.Request) {
	// Parse request body
	var payload struct {
		OldName string `json:"old_name"`
		NewName string `json:"new_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	oldName := strings.TrimSpace(payload.OldName)
	newName := strings.TrimSpace(payload.NewName)
	if !h.canModify(r, oldName) {
		http.Error(w, "无权重命名该模板", http.StatusForbidden)
		return
	}

	// Validate names
	if oldName == "" || newName == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "文件名不能为空",
		})
		return
	}

	// Security: Prevent directory traversal
	if strings.Contains(oldName, "..") || strings.Contains(oldName, "/") || strings.Contains(oldName, "\\") ||
		strings.Contains(newName, "..") || strings.Contains(newName, "/") || strings.Contains(newName, "\\") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// Preserve the template kind when the user omits an extension.
	lowerNewName := strings.ToLower(newName)
	if !strings.HasSuffix(lowerNewName, ".yaml") && !strings.HasSuffix(lowerNewName, ".yml") && !strings.HasSuffix(lowerNewName, ".conf") && !strings.HasSuffix(lowerNewName, ".lcf") {
		lowerOld := strings.ToLower(oldName)
		if strings.HasSuffix(lowerOld, ".conf") {
			newName += ".conf"
		} else if strings.HasSuffix(lowerOld, ".lcf") {
			newName += ".lcf"
		} else {
			newName += ".yaml"
		}
	}

	templatesDir := "rule_templates"
	oldPath := filepath.Join(templatesDir, oldName)
	newPath := filepath.Join(templatesDir, newName)

	// Check if old file exists
	if _, err := os.Stat(oldPath); os.IsNotExist(err) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "原文件不存在",
		})
		return
	}

	// Check if new file already exists
	if _, err := os.Stat(newPath); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "目标文件名已存在",
		})
		return
	}

	// Rename the file
	if err := os.Rename(oldPath, newPath); err != nil {
		http.Error(w, "Failed to rename template", http.StatusInternalServerError)
		return
	}
	_ = h.repo.RenameRuleTemplateOwner(r.Context(), oldName, newName)

	// Return success response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":  "模板重命名成功",
		"filename": newName,
	})
}

func (h *RuleTemplatesHandler) handleUploadTemplate(w http.ResponseWriter, r *http.Request) {
	// Parse multipart form (limit to 10MB)
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		http.Error(w, "Failed to parse form data", http.StatusBadRequest)
		return
	}

	// Get the file from form
	file, header, err := r.FormFile("template")
	if err != nil {
		http.Error(w, "Failed to get file from request", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Validate file extension
	filename := header.Filename
	lowerFilename := strings.ToLower(filename)
	if !strings.HasSuffix(lowerFilename, ".yaml") && !strings.HasSuffix(lowerFilename, ".yml") && !strings.HasSuffix(lowerFilename, ".conf") && !strings.HasSuffix(lowerFilename, ".lcf") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "只支持 .yaml、.yml、Surge .conf 或 Loon .lcf 文件",
		})
		return
	}

	// Security: Sanitize filename
	filename = filepath.Base(filename)
	if strings.Contains(filename, "..") {
		http.Error(w, "Invalid filename", http.StatusBadRequest)
		return
	}

	// Create templates directory if it doesn't exist
	templatesDir := "rule_templates"
	if err := os.MkdirAll(templatesDir, 0755); err != nil {
		http.Error(w, "Failed to create templates directory", http.StatusInternalServerError)
		return
	}

	// Create destination file
	templatePath := filepath.Join(templatesDir, filename)

	// Check if file already exists
	if _, err := os.Stat(templatePath); err == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": fmt.Sprintf("模板文件 %s 已存在", filename),
		})
		return
	}

	dst, err := os.Create(templatePath)
	if err != nil {
		http.Error(w, "Failed to create template file", http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	// Copy file content
	if _, err := io.Copy(dst, file); err != nil {
		// Clean up on error
		os.Remove(templatePath)
		http.Error(w, "Failed to save template file", http.StatusInternalServerError)
		return
	}
	_ = h.repo.SetRuleTemplateOwner(r.Context(), filename, auth.UsernameFromContext(r.Context()))

	// Return success response with filename
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filename": filename,
		"message":  "模板上传成功",
	})
}

func (h *RuleTemplatesHandler) handleVisibility(w http.ResponseWriter, r *http.Request) {
	if !h.isAdmin(r) {
		http.Error(w, "仅管理员可设置模板可见性", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var payload struct {
		Filename string `json:"filename"`
		Public   bool   `json:"public"`
	}
	if json.NewDecoder(r.Body).Decode(&payload) != nil || filepath.Base(payload.Filename) != payload.Filename {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	if err := h.repo.SetRuleTemplatePublic(r.Context(), payload.Filename, payload.Public); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"filename": payload.Filename, "public": payload.Public})
}
