package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type userConfigRequest struct {
	ForceSyncExternal       bool    `json:"force_sync_external"`
	MatchRule               string  `json:"match_rule"`
	SyncScope               string  `json:"sync_scope"`
	KeepNodeName            bool    `json:"keep_node_name"`
	CacheExpireMinutes      int     `json:"cache_expire_minutes"`
	SyncTraffic             bool    `json:"sync_traffic"`
	EnableProbeBinding      bool    `json:"enable_probe_binding"`
	CustomRulesEnabled      bool    `json:"custom_rules_enabled"`
	EnableShortLink         bool    `json:"enable_short_link"`
	TemplateVersion         string  `json:"template_version"` // "v1", "v2", or "v3"
	EnableProxyProvider     bool    `json:"enable_proxy_provider"`
	NodeOrder               []int64 `json:"node_order"` // Node display order (array of node IDs)
	ProxyGroupsSourceURL    string  `json:"proxy_groups_source_url"`
	ClientCompatibilityMode bool    `json:"client_compatibility_mode"` // Auto-filter incompatible nodes for clients
	SilentMode              bool    `json:"silent_mode"`               // Silent mode: return 404 for all requests except subscription
	SilentModeTimeout       int     `json:"silent_mode_timeout"`       // Minutes to allow access after subscription fetch
}

type userConfigResponse struct {
	ForceSyncExternal       bool    `json:"force_sync_external"`
	MatchRule               string  `json:"match_rule"`
	SyncScope               string  `json:"sync_scope"`
	KeepNodeName            bool    `json:"keep_node_name"`
	CacheExpireMinutes      int     `json:"cache_expire_minutes"`
	SyncTraffic             bool    `json:"sync_traffic"`
	EnableProbeBinding      bool    `json:"enable_probe_binding"`
	CustomRulesEnabled      bool    `json:"custom_rules_enabled"`
	EnableShortLink         bool    `json:"enable_short_link"`
	TemplateVersion         string  `json:"template_version"` // "v1", "v2", or "v3"
	EnableProxyProvider     bool    `json:"enable_proxy_provider"`
	NodeOrder               []int64 `json:"node_order"` // Node display order (array of node IDs)
	ProxyGroupsSourceURL    string  `json:"proxy_groups_source_url"`
	ClientCompatibilityMode bool    `json:"client_compatibility_mode"` // Auto-filter incompatible nodes for clients
	SilentMode              bool    `json:"silent_mode"`               // Silent mode: return 404 for all requests except subscription
	SilentModeTimeout       int     `json:"silent_mode_timeout"`       // Minutes to allow access after subscription fetch
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
	// 获取系统配置
	systemConfig, err := repo.GetSystemConfig(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("get system config: %w", err))
		return
	}

	settings, err := repo.GetUserSettings(r.Context(), username)
	if err != nil {
		if errors.Is(err, storage.ErrUserSettingsNotFound) {
			// Return default settings if not found
			resp := userConfigResponse{
				ForceSyncExternal:       false,
				MatchRule:               "node_name",
				SyncScope:               "saved_only",
				KeepNodeName:            true,
				CacheExpireMinutes:      0,
				SyncTraffic:             false,
				EnableProbeBinding:      false,
				CustomRulesEnabled:      true,  // 自定义规则始终启用
				EnableShortLink:         false,
				TemplateVersion:         "v2",  // 默认使用v2模板系统
				EnableProxyProvider:     false,
				NodeOrder:               []int64{},
				ProxyGroupsSourceURL:    systemConfig.ProxyGroupsSourceURL,
				ClientCompatibilityMode: systemConfig.ClientCompatibilityMode,
				SilentMode:              systemConfig.SilentMode,
				SilentModeTimeout:       systemConfig.SilentModeTimeout,
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
		ForceSyncExternal:       settings.ForceSyncExternal,
		MatchRule:               settings.MatchRule,
		SyncScope:               settings.SyncScope,
		KeepNodeName:            settings.KeepNodeName,
		CacheExpireMinutes:      settings.CacheExpireMinutes,
		SyncTraffic:             settings.SyncTraffic,
		EnableProbeBinding:      settings.EnableProbeBinding,
		CustomRulesEnabled:      true, // 自定义规则始终启用
		EnableShortLink:         settings.EnableShortLink,
		TemplateVersion:         settings.TemplateVersion,
		EnableProxyProvider:     settings.EnableProxyProvider,
		NodeOrder:               settings.NodeOrder,
		ProxyGroupsSourceURL:    systemConfig.ProxyGroupsSourceURL,
		ClientCompatibilityMode: systemConfig.ClientCompatibilityMode,
		SilentMode:              systemConfig.SilentMode,
		SilentModeTimeout:       systemConfig.SilentModeTimeout,
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

	// Handle template_version, default to "v2" if not provided
	templateVersion := strings.TrimSpace(payload.TemplateVersion)
	if templateVersion == "" {
		templateVersion = "v2"
	}
	if templateVersion != "v1" && templateVersion != "v2" && templateVersion != "v3" {
		writeError(w, http.StatusBadRequest, errors.New("template_version must be 'v1', 'v2', or 'v3'"))
		return
	}

	// Validate and sanitize proxy groups source URL
	proxyGroupsSourceURL := strings.TrimSpace(payload.ProxyGroupsSourceURL)
	if err := validateProxyGroupsSourceURL(proxyGroupsSourceURL); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	settings := storage.UserSettings{
		Username:            username,
		ForceSyncExternal:   payload.ForceSyncExternal,
		MatchRule:           matchRule,
		SyncScope:           syncScope,
		KeepNodeName:        payload.KeepNodeName,
		CacheExpireMinutes:  cacheExpireMinutes,
		SyncTraffic:         payload.SyncTraffic,
		EnableProbeBinding:  payload.EnableProbeBinding,
		CustomRulesEnabled:  true, // 自定义规则始终启用
		EnableShortLink:     payload.EnableShortLink,
		TemplateVersion:     templateVersion,
		EnableProxyProvider: payload.EnableProxyProvider,
		NodeOrder:           payload.NodeOrder,
	}

	if err := repo.UpsertUserSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Update system config with proxy groups source URL and silent mode
	silentModeTimeout := payload.SilentModeTimeout
	if silentModeTimeout <= 0 {
		silentModeTimeout = 15
	}
	systemConfig := storage.SystemConfig{
		ProxyGroupsSourceURL:    proxyGroupsSourceURL,
		ClientCompatibilityMode: payload.ClientCompatibilityMode,
		SilentMode:              payload.SilentMode,
		SilentModeTimeout:       silentModeTimeout,
	}
	if err := repo.UpdateSystemConfig(r.Context(), systemConfig); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("update system config: %w", err))
		return
	}

	resp := userConfigResponse{
		ForceSyncExternal:       settings.ForceSyncExternal,
		MatchRule:               settings.MatchRule,
		SyncScope:               settings.SyncScope,
		KeepNodeName:            settings.KeepNodeName,
		CacheExpireMinutes:      settings.CacheExpireMinutes,
		SyncTraffic:             settings.SyncTraffic,
		EnableProbeBinding:      settings.EnableProbeBinding,
		CustomRulesEnabled:      true, // 自定义规则始终启用
		EnableShortLink:         settings.EnableShortLink,
		TemplateVersion:         settings.TemplateVersion,
		EnableProxyProvider:     settings.EnableProxyProvider,
		NodeOrder:               settings.NodeOrder,
		ProxyGroupsSourceURL:    proxyGroupsSourceURL,
		ClientCompatibilityMode: payload.ClientCompatibilityMode,
		SilentMode:              payload.SilentMode,
		SilentModeTimeout:       silentModeTimeout,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// validateProxyGroupsSourceURL 验证代理组远程地址的合法性
// 空字符串是合法的(表示使用默认或环境变量配置)
func validateProxyGroupsSourceURL(rawURL string) error {
	if rawURL == "" {
		return nil
	}

	parsedURL, err := url.ParseRequestURI(rawURL)
	if err != nil {
		return fmt.Errorf("proxy_groups_source_url 格式无效: %w", err)
	}

	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return errors.New("proxy_groups_source_url 仅支持 http 或 https 协议")
	}

	return nil
}
