package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"miaomiaowu/internal/storage"
)

type TurnstileSettingsHandler struct {
	repo *storage.TrafficRepository
}

func NewTurnstileSettingsHandler(repo *storage.TrafficRepository) *TurnstileSettingsHandler {
	return &TurnstileSettingsHandler{repo: repo}
}

func (h *TurnstileSettingsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		siteKey, err := h.repo.GetSystemSetting(r.Context(), "turnstile_site_key")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		secret, err := h.repo.GetSystemSetting(r.Context(), "turnstile_secret_key")
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		masked := ""
		if secret != "" {
			masked = "********"
		}
		respondJSON(w, http.StatusOK, map[string]any{"site_key": siteKey, "secret_key": masked, "enabled": siteKey != "" && secret != ""})
	case http.MethodPut:
		var body struct {
			SiteKey   string `json:"site_key"`
			SecretKey string `json:"secret_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			writeBadRequest(w, "invalid body")
			return
		}
		body.SiteKey = strings.TrimSpace(body.SiteKey)
		body.SecretKey = strings.TrimSpace(body.SecretKey)
		if body.SiteKey != "" && len(body.SiteKey) < 20 {
			writeBadRequest(w, "invalid Turnstile site key")
			return
		}
		if err := h.repo.SetSystemSetting(r.Context(), "turnstile_site_key", body.SiteKey); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if body.SecretKey != "********" {
			if err := h.repo.SetSystemSetting(r.Context(), "turnstile_secret_key", body.SecretKey); err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}
		respondJSON(w, http.StatusOK, map[string]bool{"success": true})
	default:
		methodNotAllowed(w, http.MethodGet, http.MethodPut)
	}
}
