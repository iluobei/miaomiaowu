package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type userConfigRequest struct {
	ForceSyncExternal  bool   `json:"force_sync_external"`
	MatchRule          string `json:"match_rule"`
	CacheExpireMinutes int    `json:"cache_expire_minutes"`
	SyncTraffic        bool   `json:"sync_traffic"`
	EnableProbeBinding bool   `json:"enable_probe_binding"`
	CustomRulesEnabled bool   `json:"custom_rules_enabled"`
	EnableShortLink    bool   `json:"enable_short_link"`
}

type userConfigResponse struct {
	ForceSyncExternal  bool   `json:"force_sync_external"`
	MatchRule          string `json:"match_rule"`
	CacheExpireMinutes int    `json:"cache_expire_minutes"`
	SyncTraffic        bool   `json:"sync_traffic"`
	EnableProbeBinding bool   `json:"enable_probe_binding"`
	CustomRulesEnabled bool   `json:"custom_rules_enabled"`
	EnableShortLink    bool   `json:"enable_short_link"`
}

func NewUserConfigHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("user config handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleGetUserConfig(w, r, repo, username)
		case http.MethodPut:
			handleUpdateUserConfig(w, r, repo, username)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET and PUT are supported"))
		}
	})
}

func handleGetUserConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	settings, err := repo.GetUserSettings(r.Context(), username)
	if err != nil {
		if errors.Is(err, storage.ErrUserSettingsNotFound) {
			// Return default settings if not found
			resp := userConfigResponse{
				ForceSyncExternal:  false,
				MatchRule:          "node_name",
				CacheExpireMinutes: 0,
				SyncTraffic:        false,
				EnableProbeBinding: false,
				CustomRulesEnabled: false,
				EnableShortLink:    false,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(resp)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := userConfigResponse{
		ForceSyncExternal:  settings.ForceSyncExternal,
		MatchRule:          settings.MatchRule,
		CacheExpireMinutes: settings.CacheExpireMinutes,
		SyncTraffic:        settings.SyncTraffic,
		EnableProbeBinding: settings.EnableProbeBinding,
		CustomRulesEnabled: settings.CustomRulesEnabled,
		EnableShortLink:    settings.EnableShortLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleUpdateUserConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	var payload userConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Validate match rule
	matchRule := strings.TrimSpace(payload.MatchRule)
	if matchRule == "" {
		matchRule = "node_name"
	}
	if matchRule != "node_name" && matchRule != "server_port" {
		writeError(w, http.StatusBadRequest, errors.New("match_rule must be 'node_name' or 'server_port'"))
		return
	}

	// Validate cache expire minutes
	cacheExpireMinutes := payload.CacheExpireMinutes
	if cacheExpireMinutes < 0 {
		cacheExpireMinutes = 0
	}

	settings := storage.UserSettings{
		Username:           username,
		ForceSyncExternal:  payload.ForceSyncExternal,
		MatchRule:          matchRule,
		CacheExpireMinutes: cacheExpireMinutes,
		SyncTraffic:        payload.SyncTraffic,
		EnableProbeBinding: payload.EnableProbeBinding,
		CustomRulesEnabled: payload.CustomRulesEnabled,
		EnableShortLink:    payload.EnableShortLink,
	}

	if err := repo.UpsertUserSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := userConfigResponse{
		ForceSyncExternal:  settings.ForceSyncExternal,
		MatchRule:          settings.MatchRule,
		CacheExpireMinutes: settings.CacheExpireMinutes,
		SyncTraffic:        settings.SyncTraffic,
		EnableProbeBinding: settings.EnableProbeBinding,
		CustomRulesEnabled: settings.CustomRulesEnabled,
		EnableShortLink:    settings.EnableShortLink,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
