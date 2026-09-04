package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/storage"
)

type userConfigRequest struct {
	ForceSyncExternal        bool    `json:"force_sync_external"`
	MatchRule                string  `json:"match_rule"`
	SyncScope                string  `json:"sync_scope"`
	KeepNodeName             bool    `json:"keep_node_name"`
	CacheExpireMinutes       int     `json:"cache_expire_minutes"`
	SyncTraffic              bool    `json:"sync_traffic"`
	EnableProbeBinding       bool    `json:"enable_probe_binding"`
	CustomRulesEnabled       bool    `json:"custom_rules_enabled"`
	EnableShortLink          bool    `json:"enable_short_link"`
	TemplateVersion          string  `json:"template_version"`
	EnableProxyProvider      bool    `json:"enable_proxy_provider"`
	NodeOrder                []int64 `json:"node_order"`
	NodeNameFilter           string  `json:"node_name_filter"`
	AppendSubInfo            bool    `json:"append_sub_info"`
	ProxyGroupsSourceURL     string  `json:"proxy_groups_source_url"`
	ClientCompatibilityMode  bool    `json:"client_compatibility_mode"`
	SilentMode               bool    `json:"silent_mode"`
	SilentModeTimeout        int     `json:"silent_mode_timeout"`
	EnableSubInfoNodes       bool    `json:"enable_sub_info_nodes"`
	SubInfoV2RayOnly         bool    `json:"sub_info_v2ray_only"`
	SubInfoExpirePrefix      string  `json:"sub_info_expire_prefix"`
	SubInfoTrafficPrefix     string  `json:"sub_info_traffic_prefix"`
	EnableSubTrafficHeader   bool    `json:"enable_sub_traffic_header"`
	EnableOverrideScripts    bool    `json:"enable_override_scripts"`
	SubscriptionOutputFormat string  `json:"subscription_output_format"`
	// 安全配置
	LoginRateMaxAttempts    int  `json:"login_rate_max_attempts"`
	LoginRateWindow         int  `json:"login_rate_window"`
	LoginRateLockDuration   int  `json:"login_rate_lock_duration"`
	BruteForceEnabled       bool `json:"brute_force_enabled"`
	BruteForceMaxFailures   int  `json:"brute_force_max_failures"`
	BruteForceWindow        int  `json:"brute_force_window"`
	BruteForceBlockDuration int  `json:"brute_force_block_duration"`
	SubRateLimitEnabled     bool `json:"sub_rate_limit_enabled"`
	SubRateLimitMax         int  `json:"sub_rate_limit_max"`
	SubRateLimitWindow      int  `json:"sub_rate_limit_window"`
	SkipLocalIP             bool `json:"skip_local_ip"`
	BlockUnknownSubUA       bool `json:"block_unknown_subscription_ua"`
}

type userConfigResponse struct {
	ForceSyncExternal        bool    `json:"force_sync_external"`
	MatchRule                string  `json:"match_rule"`
	SyncScope                string  `json:"sync_scope"`
	KeepNodeName             bool    `json:"keep_node_name"`
	CacheExpireMinutes       int     `json:"cache_expire_minutes"`
	SyncTraffic              bool    `json:"sync_traffic"`
	EnableProbeBinding       bool    `json:"enable_probe_binding"`
	CustomRulesEnabled       bool    `json:"custom_rules_enabled"`
	EnableShortLink          bool    `json:"enable_short_link"`
	TemplateVersion          string  `json:"template_version"`
	EnableProxyProvider      bool    `json:"enable_proxy_provider"`
	NodeOrder                []int64 `json:"node_order"`
	NodeNameFilter           string  `json:"node_name_filter"`
	AppendSubInfo            bool    `json:"append_sub_info"`
	ProxyGroupsSourceURL     string  `json:"proxy_groups_source_url"`
	ClientCompatibilityMode  bool    `json:"client_compatibility_mode"`
	SilentMode               bool    `json:"silent_mode"`
	SilentModeTimeout        int     `json:"silent_mode_timeout"`
	EnableSubInfoNodes       bool    `json:"enable_sub_info_nodes"`
	SubInfoV2RayOnly         bool    `json:"sub_info_v2ray_only"`
	SubInfoExpirePrefix      string  `json:"sub_info_expire_prefix"`
	SubInfoTrafficPrefix     string  `json:"sub_info_traffic_prefix"`
	EnableSubTrafficHeader   bool    `json:"enable_sub_traffic_header"`
	EnableOverrideScripts    bool    `json:"enable_override_scripts"`
	SubscriptionOutputFormat string  `json:"subscription_output_format"`
	// 安全配置
	LoginRateMaxAttempts    int  `json:"login_rate_max_attempts"`
	LoginRateWindow         int  `json:"login_rate_window"`
	LoginRateLockDuration   int  `json:"login_rate_lock_duration"`
	BruteForceEnabled       bool `json:"brute_force_enabled"`
	BruteForceMaxFailures   int  `json:"brute_force_max_failures"`
	BruteForceWindow        int  `json:"brute_force_window"`
	BruteForceBlockDuration int  `json:"brute_force_block_duration"`
	SubRateLimitEnabled     bool `json:"sub_rate_limit_enabled"`
	SubRateLimitMax         int  `json:"sub_rate_limit_max"`
	SubRateLimitWindow      int  `json:"sub_rate_limit_window"`
	SkipLocalIP             bool `json:"skip_local_ip"`
	BlockUnknownSubUA       bool `json:"block_unknown_subscription_ua"`
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
				ForceSyncExternal:        false,
				MatchRule:                "node_name",
				SyncScope:                "saved_only",
				KeepNodeName:             true,
				CacheExpireMinutes:       0,
				SyncTraffic:              false,
				EnableProbeBinding:       false,
				CustomRulesEnabled:       true, // 自定义规则始终启用
				EnableShortLink:          systemConfig.EnableShortLink,
				TemplateVersion:          "v2", // 默认使用v2模板系统
				EnableProxyProvider:      false,
				NodeOrder:                []int64{},
				NodeNameFilter:           "剩余|流量|到期|订阅|时间|重置",
				AppendSubInfo:            false,
				ProxyGroupsSourceURL:     systemConfig.ProxyGroupsSourceURL,
				ClientCompatibilityMode:  systemConfig.ClientCompatibilityMode,
				SilentMode:               systemConfig.SilentMode,
				SilentModeTimeout:        systemConfig.SilentModeTimeout,
				EnableSubInfoNodes:       systemConfig.EnableSubInfoNodes,
				SubInfoV2RayOnly:         systemConfig.SubInfoV2RayOnly,
				SubInfoExpirePrefix:      systemConfig.SubInfoExpirePrefix,
				SubInfoTrafficPrefix:     systemConfig.SubInfoTrafficPrefix,
				EnableSubTrafficHeader:   systemConfig.EnableSubTrafficHeader,
				EnableOverrideScripts:    systemConfig.EnableOverrideScripts,
				SubscriptionOutputFormat: systemConfig.SubscriptionOutputFormat,
				LoginRateMaxAttempts:     systemConfig.LoginRateMaxAttempts,
				LoginRateWindow:          systemConfig.LoginRateWindow,
				LoginRateLockDuration:    systemConfig.LoginRateLockDuration,
				BruteForceEnabled:        systemConfig.BruteForceEnabled,
				BruteForceMaxFailures:    systemConfig.BruteForceMaxFailures,
				BruteForceWindow:         systemConfig.BruteForceWindow,
				BruteForceBlockDuration:  systemConfig.BruteForceBlockDuration,
				SubRateLimitEnabled:      systemConfig.SubRateLimitEnabled,
				SubRateLimitMax:          systemConfig.SubRateLimitMax,
				SubRateLimitWindow:       systemConfig.SubRateLimitWindow,
				SkipLocalIP:              systemConfig.SkipLocalIP,
				BlockUnknownSubUA:        systemConfig.BlockUnknownSubUA,
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
		ForceSyncExternal:        settings.ForceSyncExternal,
		MatchRule:                settings.MatchRule,
		SyncScope:                settings.SyncScope,
		KeepNodeName:             settings.KeepNodeName,
		CacheExpireMinutes:       settings.CacheExpireMinutes,
		SyncTraffic:              settings.SyncTraffic,
		EnableProbeBinding:       settings.EnableProbeBinding,
		CustomRulesEnabled:       true, // 自定义规则始终启用
		EnableShortLink:          systemConfig.EnableShortLink,
		TemplateVersion:          settings.TemplateVersion,
		EnableProxyProvider:      settings.EnableProxyProvider,
		NodeOrder:                settings.NodeOrder,
		NodeNameFilter:           settings.NodeNameFilter,
		AppendSubInfo:            settings.AppendSubInfo,
		ProxyGroupsSourceURL:     systemConfig.ProxyGroupsSourceURL,
		ClientCompatibilityMode:  systemConfig.ClientCompatibilityMode,
		SilentMode:               systemConfig.SilentMode,
		SilentModeTimeout:        systemConfig.SilentModeTimeout,
		EnableSubInfoNodes:       systemConfig.EnableSubInfoNodes,
		SubInfoV2RayOnly:         systemConfig.SubInfoV2RayOnly,
		SubInfoExpirePrefix:      systemConfig.SubInfoExpirePrefix,
		SubInfoTrafficPrefix:     systemConfig.SubInfoTrafficPrefix,
		EnableSubTrafficHeader:   systemConfig.EnableSubTrafficHeader,
		EnableOverrideScripts:    systemConfig.EnableOverrideScripts,
		SubscriptionOutputFormat: systemConfig.SubscriptionOutputFormat,
		LoginRateMaxAttempts:     systemConfig.LoginRateMaxAttempts,
		LoginRateWindow:          systemConfig.LoginRateWindow,
		LoginRateLockDuration:    systemConfig.LoginRateLockDuration,
		BruteForceEnabled:        systemConfig.BruteForceEnabled,
		BruteForceMaxFailures:    systemConfig.BruteForceMaxFailures,
		BruteForceWindow:         systemConfig.BruteForceWindow,
		BruteForceBlockDuration:  systemConfig.BruteForceBlockDuration,
		SubRateLimitEnabled:      systemConfig.SubRateLimitEnabled,
		SubRateLimitMax:          systemConfig.SubRateLimitMax,
		SubRateLimitWindow:       systemConfig.SubRateLimitWindow,
		SkipLocalIP:              systemConfig.SkipLocalIP,
		BlockUnknownSubUA:        systemConfig.BlockUnknownSubUA,
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
	if matchRule != "node_name" && matchRule != "server_port" && matchRule != "type_server_port" && matchRule != "type_server_port_cred" {
		writeError(w, http.StatusBadRequest, errors.New("match_rule must be 'node_name', 'server_port', 'type_server_port', or 'type_server_port_cred'"))
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
		TemplateVersion:     templateVersion,
		EnableProxyProvider: payload.EnableProxyProvider,
		NodeOrder:           payload.NodeOrder,
		NodeNameFilter:      payload.NodeNameFilter,
		AppendSubInfo:       payload.AppendSubInfo,
	}

	if err := repo.UpsertUserSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// [安全] 全局系统配置(限流/防爆破/静默/代理组源等)只有管理员能改。
	// 这个 /api/user/config 接口任意登录用户可达 —— 若不拦,普通用户就能关掉全站防爆破/登录限流
	// 再爆破管理员,或把 proxy_groups_source_url 指向内网做 SSRF 跳板。
	// 对非管理员:把 payload 里的所有系统字段回填成当前 DB 值,下面的写入即成为无副作用的原样回写。
	callerIsAdmin := false
	if u, uerr := repo.GetUser(r.Context(), username); uerr == nil && u.Role == storage.RoleAdmin {
		callerIsAdmin = true
	}
	if !callerIsAdmin {
		cur, _ := repo.GetSystemConfig(r.Context())
		proxyGroupsSourceURL = cur.ProxyGroupsSourceURL
		payload.ProxyGroupsSourceURL = cur.ProxyGroupsSourceURL
		payload.ClientCompatibilityMode = cur.ClientCompatibilityMode
		payload.SilentMode = cur.SilentMode
		payload.SilentModeTimeout = cur.SilentModeTimeout
		payload.EnableSubInfoNodes = cur.EnableSubInfoNodes
		payload.SubInfoV2RayOnly = cur.SubInfoV2RayOnly
		payload.SubInfoExpirePrefix = cur.SubInfoExpirePrefix
		payload.SubInfoTrafficPrefix = cur.SubInfoTrafficPrefix
		payload.EnableShortLink = cur.EnableShortLink
		payload.EnableSubTrafficHeader = cur.EnableSubTrafficHeader
		payload.EnableOverrideScripts = cur.EnableOverrideScripts
		payload.SubscriptionOutputFormat = cur.SubscriptionOutputFormat
		payload.LoginRateMaxAttempts = cur.LoginRateMaxAttempts
		payload.LoginRateWindow = cur.LoginRateWindow
		payload.LoginRateLockDuration = cur.LoginRateLockDuration
		payload.BruteForceEnabled = cur.BruteForceEnabled
		payload.BruteForceMaxFailures = cur.BruteForceMaxFailures
		payload.BruteForceWindow = cur.BruteForceWindow
		payload.BruteForceBlockDuration = cur.BruteForceBlockDuration
		payload.SubRateLimitEnabled = cur.SubRateLimitEnabled
		payload.SubRateLimitMax = cur.SubRateLimitMax
		payload.SubRateLimitWindow = cur.SubRateLimitWindow
		payload.SkipLocalIP = cur.SkipLocalIP
		payload.BlockUnknownSubUA = cur.BlockUnknownSubUA
	}

	// Update system config with proxy groups source URL and silent mode
	silentModeTimeout := payload.SilentModeTimeout
	if silentModeTimeout <= 0 {
		silentModeTimeout = 15
	}
	subInfoExpirePrefix := payload.SubInfoExpirePrefix
	if subInfoExpirePrefix == "" {
		subInfoExpirePrefix = "📅过期时间"
	}
	subInfoTrafficPrefix := payload.SubInfoTrafficPrefix
	if subInfoTrafficPrefix == "" {
		subInfoTrafficPrefix = "⌛剩余流量"
	}
	oldSysCfg, _ := repo.GetSystemConfig(r.Context())

	subscriptionOutputFormat := strings.TrimSpace(payload.SubscriptionOutputFormat)
	if subscriptionOutputFormat == "" {
		subscriptionOutputFormat = "yaml"
	}
	if subscriptionOutputFormat != "yaml" && subscriptionOutputFormat != "json" {
		writeError(w, http.StatusBadRequest, errors.New("subscription_output_format must be 'yaml' or 'json'"))
		return
	}

	systemConfig := oldSysCfg
	systemConfig.ProxyGroupsSourceURL = proxyGroupsSourceURL
	systemConfig.ClientCompatibilityMode = payload.ClientCompatibilityMode
	systemConfig.SilentMode = payload.SilentMode
	systemConfig.SilentModeTimeout = silentModeTimeout
	systemConfig.EnableSubInfoNodes = payload.EnableSubInfoNodes
	systemConfig.SubInfoV2RayOnly = payload.SubInfoV2RayOnly
	systemConfig.SubInfoExpirePrefix = subInfoExpirePrefix
	systemConfig.SubInfoTrafficPrefix = subInfoTrafficPrefix
	systemConfig.EnableShortLink = payload.EnableShortLink
	systemConfig.EnableSubTrafficHeader = payload.EnableSubTrafficHeader
	systemConfig.EnableOverrideScripts = payload.EnableOverrideScripts
	systemConfig.SubscriptionOutputFormat = subscriptionOutputFormat
	// 安全配置
	systemConfig.LoginRateMaxAttempts = payload.LoginRateMaxAttempts
	systemConfig.LoginRateWindow = payload.LoginRateWindow
	systemConfig.LoginRateLockDuration = payload.LoginRateLockDuration
	systemConfig.BruteForceEnabled = payload.BruteForceEnabled
	systemConfig.BruteForceMaxFailures = payload.BruteForceMaxFailures
	systemConfig.BruteForceWindow = payload.BruteForceWindow
	systemConfig.BruteForceBlockDuration = payload.BruteForceBlockDuration
	systemConfig.SubRateLimitEnabled = payload.SubRateLimitEnabled
	systemConfig.SubRateLimitMax = payload.SubRateLimitMax
	systemConfig.SubRateLimitWindow = payload.SubRateLimitWindow
	systemConfig.SkipLocalIP = payload.SkipLocalIP
	systemConfig.BlockUnknownSubUA = payload.BlockUnknownSubUA
	if err := repo.UpdateSystemConfig(r.Context(), systemConfig); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("update system config: %w", err))
		return
	}

	// 热更新安全组件配置
	if rl := GetLoginRateLimiter(); rl != nil {
		rl.UpdateConfig(systemConfig.LoginRateMaxAttempts, systemConfig.LoginRateWindow, systemConfig.LoginRateLockDuration)
	}
	if bfp := GetBruteForceProtector(); bfp != nil {
		bfp.UpdateConfig(systemConfig.BruteForceEnabled, systemConfig.BruteForceMaxFailures, systemConfig.BruteForceWindow, systemConfig.BruteForceBlockDuration)
	}
	if srl := GetSubscriptionRateLimiter(); srl != nil {
		srl.UpdateConfig(systemConfig.SubRateLimitEnabled, systemConfig.SubRateLimitMax, systemConfig.SubRateLimitWindow)
	}
	if rl := GetLoginRateLimiter(); rl != nil {
		rl.SetSkipLocalIP(systemConfig.SkipLocalIP)
	}
	if bfp := GetBruteForceProtector(); bfp != nil {
		bfp.SetSkipLocalIP(systemConfig.SkipLocalIP)
	}
	if srl := GetSubscriptionRateLimiter(); srl != nil {
		srl.SetSkipLocalIP(systemConfig.SkipLocalIP)
	}
	SetBlockUnknownSubscriptionUA(systemConfig.BlockUnknownSubUA)

	if oldSysCfg.SilentMode != payload.SilentMode {
		if n := GetNotifier(); n != nil {
			status := "已关闭"
			if payload.SilentMode {
				status = "已开启"
			}
			go n.Send(context.Background(), notify.Event{
				Type:    notify.EventSilentMode,
				Title:   "静默模式变更",
				Message: fmt.Sprintf("静默模式 %s", status),
			})
		}
	}

	resp := userConfigResponse{
		ForceSyncExternal:        settings.ForceSyncExternal,
		MatchRule:                settings.MatchRule,
		SyncScope:                settings.SyncScope,
		KeepNodeName:             settings.KeepNodeName,
		CacheExpireMinutes:       settings.CacheExpireMinutes,
		SyncTraffic:              settings.SyncTraffic,
		EnableProbeBinding:       settings.EnableProbeBinding,
		CustomRulesEnabled:       true, // 自定义规则始终启用
		EnableShortLink:          payload.EnableShortLink,
		TemplateVersion:          settings.TemplateVersion,
		EnableProxyProvider:      settings.EnableProxyProvider,
		NodeOrder:                settings.NodeOrder,
		NodeNameFilter:           settings.NodeNameFilter,
		AppendSubInfo:            settings.AppendSubInfo,
		ProxyGroupsSourceURL:     proxyGroupsSourceURL,
		ClientCompatibilityMode:  payload.ClientCompatibilityMode,
		SilentMode:               payload.SilentMode,
		SilentModeTimeout:        silentModeTimeout,
		EnableSubInfoNodes:       payload.EnableSubInfoNodes,
		SubInfoV2RayOnly:         payload.SubInfoV2RayOnly,
		SubInfoExpirePrefix:      subInfoExpirePrefix,
		SubInfoTrafficPrefix:     subInfoTrafficPrefix,
		EnableSubTrafficHeader:   payload.EnableSubTrafficHeader,
		EnableOverrideScripts:    payload.EnableOverrideScripts,
		SubscriptionOutputFormat: subscriptionOutputFormat,
		LoginRateMaxAttempts:     systemConfig.LoginRateMaxAttempts,
		LoginRateWindow:          systemConfig.LoginRateWindow,
		LoginRateLockDuration:    systemConfig.LoginRateLockDuration,
		BruteForceEnabled:        systemConfig.BruteForceEnabled,
		BruteForceMaxFailures:    systemConfig.BruteForceMaxFailures,
		BruteForceWindow:         systemConfig.BruteForceWindow,
		BruteForceBlockDuration:  systemConfig.BruteForceBlockDuration,
		SubRateLimitEnabled:      systemConfig.SubRateLimitEnabled,
		SubRateLimitMax:          systemConfig.SubRateLimitMax,
		SubRateLimitWindow:       systemConfig.SubRateLimitWindow,
		SkipLocalIP:              systemConfig.SkipLocalIP,
		BlockUnknownSubUA:        systemConfig.BlockUnknownSubUA,
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
