package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
)

type RuleTemplatesHandler struct{}

func NewRuleTemplatesHandler() *RuleTemplatesHandler {
	return &RuleTemplatesHandler{}
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
	default:
		// Get specific template content
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Extract template name from path (remove leading slash)
		templateName := strings.TrimPrefix(path, "/")
		h.handleGetTemplate(w, r, templateName)
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

	// Filter YAML files
	var templates []string
	for _, entry := range entries {
		if !entry.IsDir() && (strings.HasSuffix(entry.Name(), ".yaml") || strings.HasSuffix(entry.Name(), ".yml")) {
			templates = append(templates, entry.Name())
		}
	}

	// Return JSON response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"templates": templates,
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
	if !strings.HasSuffix(filename, ".yaml") && !strings.HasSuffix(filename, ".yml") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": "只支持 .yaml 或 .yml 文件",
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

	// Return success response with filename
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"filename": filename,
		"message":  "模板上传成功",
	})
}
