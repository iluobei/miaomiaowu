package handler

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type RuleEditorHandler struct {
	repo      *storage.TrafficRepository
	baseDir   string
	readLimit int64
}

func NewRuleEditorHandler(baseDir string, repo *storage.TrafficRepository) http.Handler {
	handler := &RuleEditorHandler{
		repo:      repo,
		baseDir:   baseDir,
		readLimit: 5 << 20, // 5MB per request
	}
	return handler
}

func (h *RuleEditorHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if h == nil {
		http.Error(w, "handler not initialized", http.StatusInternalServerError)
		return
	}

	path := strings.Trim(r.URL.Path, "/")
	if path == "" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		h.handleList(w, r)
		return
	}

	segments := strings.Split(path, "/")
	if len(segments) == 0 {
		http.NotFound(w, r)
		return
	}

	filename := segments[0]

	if len(segments) == 2 && segments[1] == "history" {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}
		h.handleHistory(w, r, filename)
		return
	}

	if len(segments) > 1 {
		http.NotFound(w, r)
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, filename)
	case http.MethodPut:
		h.handleUpdate(w, r, filename)
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}

func (h *RuleEditorHandler) handleList(w http.ResponseWriter, r *http.Request) {
	entries, err := os.ReadDir(h.baseDir)
	if err != nil {
		http.Error(w, "读取规则目录失败", http.StatusInternalServerError)
		return
	}

	type item struct {
		Name          string `json:"name"`
		Size          int64  `json:"size"`
		ModTime       int64  `json:"mod_time"`
		LatestVersion int64  `json:"latest_version"`
	}

	files := make([]item, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !isYAMLFile(name) {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		var latestVersion int64
		if h.repo != nil {
			if versions, err := h.repo.ListRuleVersions(r.Context(), name, 1); err == nil && len(versions) > 0 {
				latestVersion = versions[0].Version
			}
		}

		files = append(files, item{
			Name:          name,
			Size:          info.Size(),
			ModTime:       info.ModTime().Unix(),
			LatestVersion: latestVersion,
		})
	}

	respondJSON(w, http.StatusOK, map[string]any{"files": files})
}

func (h *RuleEditorHandler) handleGet(w http.ResponseWriter, r *http.Request, filename string) {
	resolved, err := h.resolveFilename(filename)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	content, err := os.ReadFile(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "读取规则文件失败", http.StatusInternalServerError)
		return
	}

	var latestVersion int64
	if h.repo != nil {
		if versions, err := h.repo.ListRuleVersions(r.Context(), filename, 1); err == nil && len(versions) > 0 {
			latestVersion = versions[0].Version
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"name":           filename,
		"content":        string(content),
		"latest_version": latestVersion,
	})
}

func (h *RuleEditorHandler) handleHistory(w http.ResponseWriter, r *http.Request, filename string) {
	resolved, err := h.resolveFilename(filename)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	if _, err := os.Stat(resolved); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "读取规则文件失败", http.StatusInternalServerError)
		return
	}

	if h.repo == nil {
		respondJSON(w, http.StatusOK, map[string]any{"history": []storage.RuleVersion{}})
		return
	}

	versions, err := h.repo.ListRuleVersions(r.Context(), filename, 20)
	if err != nil {
		http.Error(w, "获取历史版本失败", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{"history": versions})
}

func (h *RuleEditorHandler) handleUpdate(w http.ResponseWriter, r *http.Request, filename string) {
	resolved, err := h.resolveFilename(filename)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}

	if _, err := os.Stat(resolved); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			http.NotFound(w, r)
			return
		}
		http.Error(w, "读取规则文件失败", http.StatusInternalServerError)
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, h.readLimit))
	if err != nil {
		writeBadRequest(w, "读取请求体失败")
		return
	}

	var payload struct {
		Content string `json:"content"`
	}

	if err := json.Unmarshal(body, &payload); err != nil {
		writeBadRequest(w, "请求数据格式错误")
		return
	}

	if payload.Content == "" {
		writeBadRequest(w, "内容不能为空")
		return
	}

	var parsed any
	if err := yaml.Unmarshal([]byte(payload.Content), &parsed); err != nil {
		writeBadRequest(w, "YAML 解析失败: "+err.Error())
		return
	}

	if err := os.WriteFile(resolved, []byte(payload.Content), 0o644); err != nil {
		http.Error(w, "写入规则文件失败", http.StatusInternalServerError)
		return
	}

	username := auth.UsernameOrDefault(r.Context(), "unknown")

	var newVersion int64
	if h.repo != nil {
		v, saveErr := h.repo.SaveRuleVersion(r.Context(), filename, payload.Content, username)
		if saveErr != nil {
			http.Error(w, "保存历史版本失败", http.StatusInternalServerError)
			return
		}
		newVersion = v
	}

	respondJSON(w, http.StatusOK, map[string]any{"version": newVersion})
}

func (h *RuleEditorHandler) resolveFilename(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("文件名不能为空")
	}

	if !isYAMLFile(name) {
		return "", errors.New("仅支持 YAML 格式文件")
	}

	clean := filepath.Clean(name)
	if clean != name || clean == ".." || strings.Contains(clean, "../") || strings.Contains(clean, "..\\") {
		return "", errors.New("文件名不合法")
	}

	if filepath.Base(clean) != clean {
		return "", errors.New("不支持子目录访问")
	}

	return filepath.Join(h.baseDir, clean), nil
}

func isYAMLFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml")
}

func methodNotAllowed(w http.ResponseWriter, methods ...string) {
	w.Header().Set("Allow", strings.Join(methods, ", "))
	http.Error(w, "方法不被允许", http.StatusMethodNotAllowed)
}

func writeBadRequest(w http.ResponseWriter, message string) {
	respondJSON(w, http.StatusBadRequest, map[string]string{"error": message})
}

func respondJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
