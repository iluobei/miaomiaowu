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
	ForceSyncExternal    bool    `json:"force_sync_external"`
	MatchRule            string  `json:"match_rule"`
	SyncScope            string  `json:"sync_scope"`
	KeepNodeName         bool    `json:"keep_node_name"`
	CacheExpireMinutes   int     `json:"cache_expire_minutes"`
	SyncTraffic          bool    `json:"sync_traffic"`
	EnableProbeBinding   bool    `json:"enable_probe_binding"`
	CustomRulesEnabled   bool    `json:"custom_rules_enabled"`
	EnableShortLink      bool    `json:"enable_short_link"`
	UseNewTemplateSystem *bool   `json:"use_new_template_system"` // nil means not provided, default true
	EnableProxyProvider  bool    `json:"enable_proxy_provider"`
	NodeOrder            []int64 `json:"node_order"` // Node display order (array of node IDs)
}

type userConfigResponse struct {
	ForceSyncExternal    bool    `json:"force_sync_external"`
	MatchRule            string  `json:"match_rule"`
	SyncScope            string  `json:"sync_scope"`
	KeepNodeName         bool    `json:"keep_node_name"`
	CacheExpireMinutes   int     `json:"cache_expire_minutes"`
	SyncTraffic          bool    `json:"sync_traffic"`
	EnableProbeBinding   bool    `json:"enable_probe_binding"`
	CustomRulesEnabled   bool    `json:"custom_rules_enabled"`
	EnableShortLink      bool    `json:"enable_short_link"`
	UseNewTemplateSystem bool    `json:"use_new_template_system"`
	EnableProxyProvider  bool    `json:"enable_proxy_provider"`
	NodeOrder            []int64 `json:"node_order"` // Node display order (array of node IDs)
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
				ForceSyncExternal:    false,
				MatchRule:            "node_name",
				SyncScope:            "saved_only",
				KeepNodeName:         true,
				CacheExpireMinutes:   0,
				SyncTraffic:          false,
				EnableProbeBinding:   false,
				CustomRulesEnabled:   true,  // 自定义规则始终启用
				EnableShortLink:      false,
				UseNewTemplateSystem: true,  // 默认使用新模板系统
				EnableProxyProvider:  false,
				NodeOrder:            []int64{},
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
		ForceSyncExternal:    settings.ForceSyncExternal,
		MatchRule:            settings.MatchRule,
		SyncScope:            settings.SyncScope,
		KeepNodeName:         settings.KeepNodeName,
		CacheExpireMinutes:   settings.CacheExpireMinutes,
		SyncTraffic:          settings.SyncTraffic,
		EnableProbeBinding:   settings.EnableProbeBinding,
		CustomRulesEnabled:   true, // 自定义规则始终启用
		EnableShortLink:      settings.EnableShortLink,
		UseNewTemplateSystem: settings.UseNewTemplateSystem,
		EnableProxyProvider:  settings.EnableProxyProvider,
		NodeOrder:            settings.NodeOrder,
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
	if matchRule != "node_name" && matchRule != "server_port" && matchRule != "type_server_port" {
		writeError(w, http.StatusBadRequest, errors.New("match_rule must be 'node_name', 'server_port', or 'type_server_port'"))
		return
	}

	// Validate sync scope
	syncScope := strings.TrimSpace(payload.SyncScope)
	if syncScope == "" {
		syncScope = "saved_only"
	}
	if syncScope != "saved_only" && syncScope != "all" {
		writeError(w, http.StatusBadRequest, errors.New("sync_scope must be 'saved_only' or 'all'"))
		return
	}

	// Validate cache expire minutes
	cacheExpireMinutes := payload.CacheExpireMinutes
	if cacheExpireMinutes < 0 {
		cacheExpireMinutes = 0
	}

	// Handle use_new_template_system, default to true if not provided
	useNewTemplateSystem := true
	if payload.UseNewTemplateSystem != nil {
		useNewTemplateSystem = *payload.UseNewTemplateSystem
	}

	settings := storage.UserSettings{
		Username:             username,
		ForceSyncExternal:    payload.ForceSyncExternal,
		MatchRule:            matchRule,
		SyncScope:            syncScope,
		KeepNodeName:         payload.KeepNodeName,
		CacheExpireMinutes:   cacheExpireMinutes,
		SyncTraffic:          payload.SyncTraffic,
		EnableProbeBinding:   payload.EnableProbeBinding,
		CustomRulesEnabled:   true, // 自定义规则始终启用
		EnableShortLink:      payload.EnableShortLink,
		UseNewTemplateSystem: useNewTemplateSystem,
		EnableProxyProvider:  payload.EnableProxyProvider,
		NodeOrder:            payload.NodeOrder,
	}

	if err := repo.UpsertUserSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := userConfigResponse{
		ForceSyncExternal:    settings.ForceSyncExternal,
		MatchRule:            settings.MatchRule,
		SyncScope:            settings.SyncScope,
		KeepNodeName:         settings.KeepNodeName,
		CacheExpireMinutes:   settings.CacheExpireMinutes,
		SyncTraffic:          settings.SyncTraffic,
		EnableProbeBinding:   settings.EnableProbeBinding,
		CustomRulesEnabled:   true, // 自定义规则始终启用
		EnableShortLink:      settings.EnableShortLink,
		UseNewTemplateSystem: settings.UseNewTemplateSystem,
		EnableProxyProvider:  settings.EnableProxyProvider,
		NodeOrder:            settings.NodeOrder,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}
