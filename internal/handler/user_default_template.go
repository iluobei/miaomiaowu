package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type userDefaultTemplateHandler struct{ repo *storage.TrafficRepository }

func NewUserDefaultTemplateHandler(repo *storage.TrafficRepository) http.Handler {
	return &userDefaultTemplateHandler{repo: repo}
}

func (h *userDefaultTemplateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}
	settings, err := h.repo.GetUserSettings(r.Context(), username)
	if err != nil && !errors.Is(err, storage.ErrUserSettingsNotFound) {
		writeError(w, 500, err)
		return
	}
	if errors.Is(err, storage.ErrUserSettingsNotFound) {
		settings = storage.UserSettings{Username: username, MatchRule: "node_name", SyncScope: "saved_only", KeepNodeName: true, TemplateVersion: "v2"}
	}
	if r.Method == http.MethodGet {
		respondJSON(w, 200, map[string]string{"default_template_filename": settings.DefaultTemplateFilename, "default_surge_template_filename": settings.DefaultSurgeTemplateFilename, "default_loon_template_filename": settings.DefaultLoonTemplateFilename})
		return
	}
	if r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", 405)
		return
	}
	var body struct {
		DefaultTemplateFilename      string `json:"default_template_filename"`
		DefaultSurgeTemplateFilename string `json:"default_surge_template_filename"`
		DefaultLoonTemplateFilename  string `json:"default_loon_template_filename"`
	}
	if json.NewDecoder(r.Body).Decode(&body) != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}
	for filename, suffix := range map[string]string{strings.TrimSpace(body.DefaultTemplateFilename): ".yaml", strings.TrimSpace(body.DefaultSurgeTemplateFilename): ".conf", strings.TrimSpace(body.DefaultLoonTemplateFilename): ".lcf"} {
		if filename == "" {
			continue
		}
		lower := strings.ToLower(filename)
		validSuffix := strings.HasSuffix(lower, suffix) || (suffix == ".yaml" && strings.HasSuffix(lower, ".yml"))
		if filepath.Base(filename) != filename || !validSuffix {
			writeBadRequest(w, "模板文件名或类型不正确")
			return
		}
		if _, err := os.Stat(filepath.Join("rule_templates", filename)); err != nil {
			writeBadRequest(w, "模板不存在")
			return
		}
		owner, _ := h.repo.GetRuleTemplateOwner(r.Context(), filename)
		user, _ := h.repo.GetUser(r.Context(), username)
		if user.Role != storage.RoleAdmin && owner != username && !h.repo.IsRuleTemplatePublic(r.Context(), filename) && owner != "" {
			writeError(w, http.StatusForbidden, errors.New("无权使用该模板"))
			return
		}
	}
	settings.DefaultTemplateFilename = strings.TrimSpace(body.DefaultTemplateFilename)
	settings.DefaultSurgeTemplateFilename = strings.TrimSpace(body.DefaultSurgeTemplateFilename)
	settings.DefaultLoonTemplateFilename = strings.TrimSpace(body.DefaultLoonTemplateFilename)
	if err := h.repo.UpsertUserSettings(r.Context(), settings); err != nil {
		writeError(w, 500, err)
		return
	}
	respondJSON(w, 200, body)
}
