package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/util"
	"miaomiaowu/internal/validator"

	"gopkg.in/yaml.v3"
)

type proxyProviderConfigRequest struct {
	ExternalSubscriptionID int64  `json:"external_subscription_id"`
	Name                   string `json:"name"`
	Type                   string `json:"type"`
	Interval               int    `json:"interval"`
	Proxy                  string `json:"proxy"`
	SizeLimit              int    `json:"size_limit"`
	Header                 string `json:"header"` // JSON string

	HealthCheckEnabled        bool   `json:"health_check_enabled"`
	HealthCheckURL            string `json:"health_check_url"`
	HealthCheckInterval       int    `json:"health_check_interval"`
	HealthCheckTimeout        int    `json:"health_check_timeout"`
	HealthCheckLazy           bool   `json:"health_check_lazy"`
	HealthCheckExpectedStatus int    `json:"health_check_expected_status"`

	Filter        string `json:"filter"`
	ExcludeFilter string `json:"exclude_filter"`
	ExcludeType   string `json:"exclude_type"`
	GeoIPFilter   string `json:"geo_ip_filter"` // 国家代码，逗号分隔，如 "HK,TW"（仅 MMW 模式生效）
	Override      string `json:"override"`      // JSON string

	ProcessMode string `json:"process_mode"` // 'client' or 'mmw'
}

type proxyProviderConfigResponse struct {
	ID                        int64  `json:"id"`
	ExternalSubscriptionID    int64  `json:"external_subscription_id"`
	Name                      string `json:"name"`
	Type                      string `json:"type"`
	Interval                  int    `json:"interval"`
	Proxy                     string `json:"proxy"`
	SizeLimit                 int    `json:"size_limit"`
	Header                    string `json:"header"`
	HealthCheckEnabled        bool   `json:"health_check_enabled"`
	HealthCheckURL            string `json:"health_check_url"`
	HealthCheckInterval       int    `json:"health_check_interval"`
	HealthCheckTimeout        int    `json:"health_check_timeout"`
	HealthCheckLazy           bool   `json:"health_check_lazy"`
	HealthCheckExpectedStatus int    `json:"health_check_expected_status"`
	Filter                    string `json:"filter"`
	ExcludeFilter             string `json:"exclude_filter"`
	ExcludeType               string `json:"exclude_type"`
	GeoIPFilter               string `json:"geo_ip_filter"`
	Override                  string `json:"override"`
	ProcessMode               string `json:"process_mode"`
	CreatedAt                 string `json:"created_at"`
	UpdatedAt                 string `json:"updated_at"`
}

func NewProxyProviderConfigsHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("proxy provider configs handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleListProxyProviderConfigs(w, r, repo, username)
		case http.MethodPost:
			handleCreateProxyProviderConfig(w, r, repo, username)
		case http.MethodPut:
			handleUpdateProxyProviderConfig(w, r, repo, username)
		case http.MethodDelete:
			handleDeleteProxyProviderConfig(w, r, repo, username)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
		}
	})
}

func handleListProxyProviderConfigs(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	// Check if filtering by external_subscription_id
	externalSubIDStr := r.URL.Query().Get("external_subscription_id")

	var configs []storage.ProxyProviderConfig
	var err error

	if externalSubIDStr != "" {
		externalSubID, parseErr := strconv.ParseInt(externalSubIDStr, 10, 64)
		if parseErr != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid external_subscription_id"))
			return
		}
		configs, err = repo.ListProxyProviderConfigsBySubscription(r.Context(), externalSubID)
	} else {
		configs, err = repo.ListProxyProviderConfigs(r.Context(), username)
	}

	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	resp := make([]proxyProviderConfigResponse, 0, len(configs))
	for _, config := range configs {
		resp = append(resp, toProxyProviderConfigResponse(config))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

func handleCreateProxyProviderConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	var payload proxyProviderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("proxy provider name is required"))
		return
	}

	if payload.ExternalSubscriptionID <= 0 {
		writeError(w, http.StatusBadRequest, errors.New("external_subscription_id is required"))
		return
	}

	// Verify that external subscription exists and belongs to user
	sub, err := repo.GetExternalSubscription(r.Context(), payload.ExternalSubscriptionID, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if sub.ID == 0 {
		writeError(w, http.StatusNotFound, errors.New("external subscription not found"))
		return
	}

	// Set defaults
	configType := payload.Type
	if configType == "" {
		configType = "http"
	}
	interval := payload.Interval
	if interval <= 0 {
		interval = 3600
	}
	proxy := payload.Proxy
	if proxy == "" {
		proxy = "DIRECT"
	}
	healthCheckURL := payload.HealthCheckURL
	if healthCheckURL == "" {
		healthCheckURL = "https://www.gstatic.com/generate_204"
	}
	healthCheckInterval := payload.HealthCheckInterval
	if healthCheckInterval <= 0 {
		healthCheckInterval = 300
	}
	healthCheckTimeout := payload.HealthCheckTimeout
	if healthCheckTimeout <= 0 {
		healthCheckTimeout = 5000
	}
	healthCheckExpectedStatus := payload.HealthCheckExpectedStatus
	if healthCheckExpectedStatus <= 0 {
		healthCheckExpectedStatus = 204
	}
	processMode := payload.ProcessMode
	if processMode == "" {
		processMode = "client"
	}

	config := &storage.ProxyProviderConfig{
		Username:                  username,
		ExternalSubscriptionID:   payload.ExternalSubscriptionID,
		Name:                      name,
		Type:                      configType,
		Interval:                  interval,
		Proxy:                     proxy,
		SizeLimit:                 payload.SizeLimit,
		Header:                    payload.Header,
		HealthCheckEnabled:        payload.HealthCheckEnabled,
		HealthCheckURL:            healthCheckURL,
		HealthCheckInterval:       healthCheckInterval,
		HealthCheckTimeout:        healthCheckTimeout,
		HealthCheckLazy:           payload.HealthCheckLazy,
		HealthCheckExpectedStatus: healthCheckExpectedStatus,
		Filter:                    payload.Filter,
		ExcludeFilter:             payload.ExcludeFilter,
		ExcludeType:               payload.ExcludeType,
		GeoIPFilter:               payload.GeoIPFilter,
		Override:                  payload.Override,
		ProcessMode:               processMode,
	}

	id, err := repo.CreateProxyProviderConfig(r.Context(), config)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	config.ID = id
	config.CreatedAt = time.Now()
	config.UpdatedAt = time.Now()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(toProxyProviderConfigResponse(*config))
}

func handleUpdateProxyProviderConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	var payload proxyProviderConfigRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	name := strings.TrimSpace(payload.Name)
	if name == "" {
		writeError(w, http.StatusBadRequest, errors.New("proxy provider name is required"))
		return
	}

	// Verify that config exists and belongs to user
	existing, err := repo.GetProxyProviderConfig(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if existing == nil || existing.Username != username {
		writeError(w, http.StatusNotFound, errors.New("proxy provider config not found"))
		return
	}

	// Set defaults
	configType := payload.Type
	if configType == "" {
		configType = "http"
	}
	interval := payload.Interval
	if interval <= 0 {
		interval = 3600
	}
	proxy := payload.Proxy
	if proxy == "" {
		proxy = "DIRECT"
	}
	healthCheckURL := payload.HealthCheckURL
	if healthCheckURL == "" {
		healthCheckURL = "https://www.gstatic.com/generate_204"
	}
	healthCheckInterval := payload.HealthCheckInterval
	if healthCheckInterval <= 0 {
		healthCheckInterval = 300
	}
	healthCheckTimeout := payload.HealthCheckTimeout
	if healthCheckTimeout <= 0 {
		healthCheckTimeout = 5000
	}
	healthCheckExpectedStatus := payload.HealthCheckExpectedStatus
	if healthCheckExpectedStatus <= 0 {
		healthCheckExpectedStatus = 204
	}
	processMode := payload.ProcessMode
	if processMode == "" {
		processMode = "client"
	}

	config := &storage.ProxyProviderConfig{
		ID:                        id,
		Username:                  username,
		ExternalSubscriptionID:    existing.ExternalSubscriptionID,
		Name:                      name,
		Type:                      configType,
		Interval:                  interval,
		Proxy:                     proxy,
		SizeLimit:                 payload.SizeLimit,
		Header:                    payload.Header,
		HealthCheckEnabled:        payload.HealthCheckEnabled,
		HealthCheckURL:            healthCheckURL,
		HealthCheckInterval:       healthCheckInterval,
		HealthCheckTimeout:        healthCheckTimeout,
		HealthCheckLazy:           payload.HealthCheckLazy,
		HealthCheckExpectedStatus: healthCheckExpectedStatus,
		Filter:                    payload.Filter,
		ExcludeFilter:             payload.ExcludeFilter,
		ExcludeType:               payload.ExcludeType,
		GeoIPFilter:               payload.GeoIPFilter,
		Override:                  payload.Override,
		ProcessMode:               processMode,
	}

	if err := repo.UpdateProxyProviderConfig(r.Context(), config); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	config.CreatedAt = existing.CreatedAt
	config.UpdatedAt = time.Now()

	// 检测 ProcessMode 是否发生变化，如果变化则同步更新订阅文件
	if existing.ProcessMode != processMode {
		log.Printf("[代理集合] 处理模式切换: %s -> %s (代理集合: %s)", existing.ProcessMode, processMode, config.Name)
		go syncProxyProviderModeChange(repo, config, existing.ProcessMode, processMode)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(toProxyProviderConfigResponse(*config))
}

func handleDeleteProxyProviderConfig(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, username string) {
	idStr := r.URL.Query().Get("id")
	if idStr == "" {
		writeError(w, http.StatusBadRequest, errors.New("id is required"))
		return
	}

	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("invalid id"))
		return
	}

	if err := repo.DeleteProxyProviderConfig(r.Context(), id, username); err != nil {
		if err.Error() == "proxy provider config not found or not owned by user" {
			writeError(w, http.StatusNotFound, errors.New("proxy provider config not found"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func toProxyProviderConfigResponse(config storage.ProxyProviderConfig) proxyProviderConfigResponse {
	return proxyProviderConfigResponse{
		ID:                        config.ID,
		ExternalSubscriptionID:    config.ExternalSubscriptionID,
		Name:                      config.Name,
		Type:                      config.Type,
		Interval:                  config.Interval,
		Proxy:                     config.Proxy,
		SizeLimit:                 config.SizeLimit,
		Header:                    config.Header,
		HealthCheckEnabled:        config.HealthCheckEnabled,
		HealthCheckURL:            config.HealthCheckURL,
		HealthCheckInterval:       config.HealthCheckInterval,
		HealthCheckTimeout:        config.HealthCheckTimeout,
		HealthCheckLazy:           config.HealthCheckLazy,
		HealthCheckExpectedStatus: config.HealthCheckExpectedStatus,
		Filter:                    config.Filter,
		ExcludeFilter:             config.ExcludeFilter,
		ExcludeType:               config.ExcludeType,
		GeoIPFilter:               config.GeoIPFilter,
		Override:                  config.Override,
		ProcessMode:               config.ProcessMode,
		CreatedAt:                 config.CreatedAt.Format(time.RFC3339),
		UpdatedAt:                 config.UpdatedAt.Format(time.RFC3339),
	}
}

// NewProxyProviderCacheRefreshHandler 创建代理集合缓存刷新处理器
func NewProxyProviderCacheRefreshHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("proxy provider cache refresh handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		// 从 URL 中获取配置 ID
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			writeError(w, http.StatusBadRequest, errors.New("id is required"))
			return
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid id"))
			return
		}

		// 获取配置
		config, err := repo.GetProxyProviderConfig(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if config == nil || config.Username != username {
			writeError(w, http.StatusNotFound, errors.New("proxy provider config not found"))
			return
		}

		// 只有 MMW 模式才需要缓存
		if config.ProcessMode != "mmw" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"message":    "客户端处理模式无需缓存",
				"cached":     false,
				"node_count": 0,
			})
			return
		}

		// 获取外部订阅信息
		sub, err := repo.GetExternalSubscription(r.Context(), config.ExternalSubscriptionID, username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if sub.ID == 0 {
			writeError(w, http.StatusNotFound, errors.New("external subscription not found"))
			return
		}

		// 刷新缓存
		entry, err := RefreshProxyProviderCache(&sub, config)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"message":    "缓存刷新成功",
			"cached":     true,
			"node_count": entry.NodeCount,
			"fetched_at": entry.FetchedAt.Format(time.RFC3339),
		})
	})
}

// NewProxyProviderNodesHandler 获取代理集合的节点列表（用于前端应用分组时获取 MMW 节点）
func NewProxyProviderNodesHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("proxy provider nodes handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		// 从 URL 中获取配置 ID
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			writeError(w, http.StatusBadRequest, errors.New("id is required"))
			return
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid id"))
			return
		}

		// 获取配置
		config, err := repo.GetProxyProviderConfig(r.Context(), id)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if config == nil || config.Username != username {
			writeError(w, http.StatusNotFound, errors.New("proxy provider config not found"))
			return
		}

		// 非 MMW 模式返回空节点列表
		if config.ProcessMode != "mmw" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]any{
				"nodes":  []any{},
				"prefix": "",
			})
			return
		}

		// 从缓存获取节点
		cache := GetProxyProviderCache()
		entry, ok := cache.Get(id)
		if !ok || cache.IsExpired(entry) {
			// 缓存不存在或过期，尝试刷新
			sub, err := repo.GetExternalSubscription(r.Context(), config.ExternalSubscriptionID, username)
			if err != nil || sub.ID == 0 {
				writeError(w, http.StatusInternalServerError, errors.New("获取外部订阅失败"))
				return
			}
			entry, err = RefreshProxyProviderCache(&sub, config)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}

		// 计算前缀
		namePrefix := config.Name
		if idx := strings.Index(config.Name, "-"); idx > 0 {
			namePrefix = config.Name[:idx]
		}
		prefix := "〖" + namePrefix + "〗"

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]any{
			"nodes":  entry.Nodes,
			"prefix": prefix,
		})
	})
}

// NewProxyProviderCacheStatusHandler 获取代理集合缓存状态
func NewProxyProviderCacheStatusHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("proxy provider cache status handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		username := auth.UsernameFromContext(r.Context())
		if strings.TrimSpace(username) == "" {
			writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
			return
		}

		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		// 获取用户的所有代理集合配置
		configs, err := repo.ListProxyProviderConfigs(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		cache := GetProxyProviderCache()
		result := make(map[string]any)

		for _, config := range configs {
			if config.ProcessMode == "mmw" {
				status := cache.GetCacheStatus(config.ID)
				result[strconv.FormatInt(config.ID, 10)] = status
			}
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(result)
	})
}

// syncProxyProviderModeChange 同步代理集合处理模式切换到所有订阅文件
// oldMode -> newMode:
//   - client -> mmw: 为代理集合创建同名代理组，添加节点到 proxies，删除 proxy-providers 配置
//   - mmw -> client: 删除同名代理组，删除节点，恢复 proxy-providers 配置
func syncProxyProviderModeChange(repo *storage.TrafficRepository, config *storage.ProxyProviderConfig, oldMode, newMode string) {
	ctx := context.Background()
	subscribesDir := "subscribes"

	// 获取所有订阅文件
	files, err := repo.ListSubscribeFiles(ctx)
	if err != nil {
		log.Printf("[代理集合模式切换] 获取订阅文件列表失败: %v", err)
		return
	}

	// 计算前缀
	namePrefix := config.Name
	if idx := strings.Index(config.Name, "-"); idx > 0 {
		namePrefix = config.Name[:idx]
	}
	prefix := fmt.Sprintf("〖%s〗", namePrefix)

	syncedCount := 0
	for _, file := range files {
		filePath := fmt.Sprintf("%s/%s", subscribesDir, file.Filename)

		// 检查文件是否存在
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			continue
		}

		// 读取 YAML 文件
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("[代理集合模式切换] 读取文件 %s 失败: %v", file.Filename, err)
			continue
		}

		// 解析 YAML
		var rootNode yaml.Node
		if err := yaml.Unmarshal(content, &rootNode); err != nil {
			log.Printf("[代理集合模式切换] 解析文件 %s 失败: %v", file.Filename, err)
			continue
		}

		// 检查文件是否使用了该代理集合
		if !fileUsesProxyProvider(&rootNode, config.Name) {
			continue
		}

		log.Printf("[代理集合模式切换] 处理文件 %s (代理集合: %s, %s -> %s)", file.Filename, config.Name, oldMode, newMode)

		var modified bool
		if newMode == "mmw" {
			// client -> mmw: 添加节点和代理组
			modified, err = syncClientToMMW(ctx, repo, config, &rootNode, prefix)
		} else {
			// mmw -> client: 删除节点和代理组，恢复 proxy-providers
			// 从缓存获取该代理集合的节点名称列表（用于精确删除节点）
			var nodeNamesToRemove []string
			cache := GetProxyProviderCache()
			if entry, ok := cache.Get(config.ID); ok {
				// 缓存中的节点名称需要加上前缀
				for _, node := range entry.Nodes {
					if nodeMap, ok := node.(map[string]any); ok {
						if name, ok := nodeMap["name"].(string); ok {
							nodeNamesToRemove = append(nodeNamesToRemove, prefix+name)
						}
					}
				}
			}
			modified, err = syncMMWToClient(config, &rootNode, prefix, nodeNamesToRemove)
		}

		if err != nil {
			log.Printf("[代理集合模式切换] 处理文件 %s 失败: %v", file.Filename, err)
			continue
		}

		if !modified {
			continue
		}

		// 校验生成的配置
		var configMap map[string]interface{}
		var tempBuf bytes.Buffer
		tempEncoder := yaml.NewEncoder(&tempBuf)
		tempEncoder.SetIndent(2)
		if err := tempEncoder.Encode(&rootNode); err != nil {
			log.Printf("[代理集合模式切换] [配置校验] 编码配置失败: %v", err)
			continue
		}
		if err := yaml.Unmarshal(tempBuf.Bytes(), &configMap); err != nil {
			log.Printf("[代理集合模式切换] [配置校验] 解析配置失败: %v", err)
			continue
		}

		validationResult := validator.ValidateClashConfig(configMap)
		if !validationResult.Valid {
			log.Printf("[代理集合模式切换] [配置校验] 文件 %s 校验失败，跳过保存", file.Filename)
			for _, issue := range validationResult.Issues {
				if issue.Level == validator.ErrorLevel {
					log.Printf("[代理集合模式切换] [配置校验] 错误: %s - 位置: %s", issue.Message, issue.Location)
				}
			}
			continue
		}

		// 如果有自动修复，使用修复后的配置
		if validationResult.FixedConfig != nil {
			var fixedNode yaml.Node
			fixedYAML, err := yaml.Marshal(validationResult.FixedConfig)
			if err != nil {
				log.Printf("[代理集合模式切换] [配置校验] 序列化修复配置失败: %v", err)
				continue
			}
			if err := yaml.Unmarshal(fixedYAML, &fixedNode); err != nil {
				log.Printf("[代理集合模式切换] [配置校验] 解析修复配置失败: %v", err)
				continue
			}
			rootNode = fixedNode

			// 记录自动修复的警告
			for _, issue := range validationResult.Issues {
				if issue.Level == validator.WarningLevel && issue.AutoFixed {
					log.Printf("[代理集合模式切换] [配置校验] 警告(已修复): %s - 位置: %s", issue.Message, issue.Location)
				}
			}
		}

		// 保存文件
		var buf bytes.Buffer
		encoder := yaml.NewEncoder(&buf)
		encoder.SetIndent(2)
		if err := encoder.Encode(&rootNode); err != nil {
			log.Printf("[代理集合模式切换] 编码文件 %s 失败: %v", file.Filename, err)
			continue
		}

		// 处理 emoji 编码
		output := RemoveUnicodeEscapeQuotes(buf.String())
		if err := os.WriteFile(filePath, []byte(output), 0644); err != nil {
			log.Printf("[代理集合模式切换] 保存文件 %s 失败: %v", file.Filename, err)
			continue
		}

		syncedCount++
		log.Printf("[代理集合模式切换] 文件 %s 同步完成", file.Filename)
	}

	if syncedCount > 0 {
		log.Printf("[代理集合模式切换] 同步完成: 共更新 %d 个文件", syncedCount)
	}
}

// fileUsesProxyProvider 检查文件是否使用了指定的代理集合
func fileUsesProxyProvider(rootNode *yaml.Node, providerName string) bool {
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return false
	}

	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return false
	}

	// 遍历所有顶层节点
	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind != yaml.ScalarNode {
			continue
		}

		switch keyNode.Value {
		case "proxy-groups":
			if valueNode.Kind != yaml.SequenceNode {
				continue
			}
			// 遍历 proxy-groups
			for _, groupNode := range valueNode.Content {
				if groupNode.Kind != yaml.MappingNode {
					continue
				}

				// 检查是否存在同名代理组（MMW 模式创建的）
				name := util.GetNodeFieldValue(groupNode, "name")
				if name == providerName {
					return true
				}

				for j := 0; j < len(groupNode.Content)-1; j += 2 {
					gKeyNode := groupNode.Content[j]
					gValueNode := groupNode.Content[j+1]
					if gKeyNode.Kind != yaml.ScalarNode || gValueNode.Kind != yaml.SequenceNode {
						continue
					}

					// 检查 use 中是否包含此代理集合
					if gKeyNode.Value == "use" {
						for _, useItem := range gValueNode.Content {
							if useItem.Kind == yaml.ScalarNode && useItem.Value == providerName {
								return true
							}
						}
					}

					// 检查 proxies 中是否引用了同名代理组（表示之前是 MMW 模式）
					if gKeyNode.Value == "proxies" {
						for _, proxyItem := range gValueNode.Content {
							if proxyItem.Kind == yaml.ScalarNode && proxyItem.Value == providerName {
								return true
							}
						}
					}
				}
			}

		case "proxy-providers":
			// 检查 proxy-providers 中是否存在该代理集合配置
			if valueNode.Kind != yaml.MappingNode {
				continue
			}
			for j := 0; j < len(valueNode.Content)-1; j += 2 {
				providerKeyNode := valueNode.Content[j]
				if providerKeyNode.Kind == yaml.ScalarNode && providerKeyNode.Value == providerName {
					return true
				}
			}
		}
	}

	return false
}

// syncClientToMMW 从客户端模式切换到 MMW 模式
// 1. 从缓存获取节点
// 2. 创建同名代理组
// 3. 将 use 引用替换为代理组名称
// 4. 添加节点到 proxies
// 5. 删除 proxy-providers 中的配置
func syncClientToMMW(ctx context.Context, repo *storage.TrafficRepository, config *storage.ProxyProviderConfig, rootNode *yaml.Node, prefix string) (bool, error) {
	// 获取外部订阅
	sub, err := repo.GetExternalSubscription(ctx, config.ExternalSubscriptionID, config.Username)
	if err != nil || sub.ID == 0 {
		return false, fmt.Errorf("获取外部订阅失败: %v", err)
	}

	// 刷新缓存获取节点
	entry, err := RefreshProxyProviderCache(&sub, config)
	if err != nil {
		return false, fmt.Errorf("刷新缓存失败: %v", err)
	}

	if len(entry.Nodes) == 0 {
		return false, nil
	}

	// 准备节点数据
	proxiesRaw := make([]any, len(entry.Nodes))
	nodeNames := make([]string, 0, len(entry.Nodes))
	for i, node := range entry.Nodes {
		nodeCopy := copyMapForProvider(node.(map[string]any))
		if name, ok := nodeCopy["name"].(string); ok {
			newName := prefix + name
			nodeCopy["name"] = newName
			nodeNames = append(nodeNames, newName)
		}
		proxiesRaw[i] = nodeCopy
	}

	// 调用 updateYAMLFileWithProxyProviderNodes 相同的逻辑
	return updateYAMLNodeForMMW(rootNode, config.Name, prefix, proxiesRaw, nodeNames)
}

// syncMMWToClient 从 MMW 模式切换到客户端模式
// 1. 删除同名代理组
// 2. 删除代理集合的节点（精确匹配 nodeNamesToRemove 列表）
// 3. 将代理组的 proxies 中的代理组名称替换为 use 引用
// 4. 添加 proxy-providers 配置
func syncMMWToClient(config *storage.ProxyProviderConfig, rootNode *yaml.Node, prefix string, nodeNamesToRemove []string) (bool, error) {
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return false, nil
	}

	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return false, nil
	}

	modified := false

	// 查找各节点
	var proxyGroupsNode *yaml.Node
	var proxiesNode *yaml.Node
	var proxyProvidersNode *yaml.Node
	var proxyProvidersKeyIndex int = -1

	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode {
			switch keyNode.Value {
			case "proxy-groups":
				proxyGroupsNode = valueNode
			case "proxies":
				proxiesNode = valueNode
			case "proxy-providers":
				proxyProvidersNode = valueNode
				proxyProvidersKeyIndex = i
			}
		}
	}

	if proxyGroupsNode == nil || proxyGroupsNode.Kind != yaml.SequenceNode {
		return false, nil
	}

	// 1. 删除同名代理组
	newProxyGroups := make([]*yaml.Node, 0)
	for _, groupNode := range proxyGroupsNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			name := util.GetNodeFieldValue(groupNode, "name")
			if name == config.Name {
				modified = true
				log.Printf("[代理集合模式切换] 删除代理组: %s", config.Name)
				continue
			}
		}
		newProxyGroups = append(newProxyGroups, groupNode)
	}
	proxyGroupsNode.Content = newProxyGroups

	// 2. 遍历代理组，将 proxies 中的代理组名称替换为 use 引用
	for _, groupNode := range proxyGroupsNode.Content {
		if groupNode.Kind != yaml.MappingNode {
			continue
		}

		var groupProxiesNode *yaml.Node
		var useNode *yaml.Node
		var useKeyIndex int = -1

		for i := 0; i < len(groupNode.Content)-1; i += 2 {
			keyNode := groupNode.Content[i]
			valueNode := groupNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode {
				switch keyNode.Value {
				case "proxies":
					groupProxiesNode = valueNode
				case "use":
					useNode = valueNode
					useKeyIndex = i
				}
			}
		}

		if groupProxiesNode == nil || groupProxiesNode.Kind != yaml.SequenceNode {
			continue
		}

		// 检查是否引用了代理集合名称
		foundProvider := false
		newProxiesContent := make([]*yaml.Node, 0)
		for _, p := range groupProxiesNode.Content {
			if p.Kind == yaml.ScalarNode && p.Value == config.Name {
				foundProvider = true
				continue
			}
			newProxiesContent = append(newProxiesContent, p)
		}

		if !foundProvider {
			continue
		}

		modified = true
		groupProxiesNode.Content = newProxiesContent

		// 添加 use 引用
		if useNode == nil {
			// 创建 use 字段
			useNode = &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0)}
			groupNode.Content = append(groupNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "use"},
				useNode,
			)
		} else if useKeyIndex == -1 {
			// use 节点存在但索引未记录，重新查找
			for i := 0; i < len(groupNode.Content)-1; i += 2 {
				if groupNode.Content[i].Kind == yaml.ScalarNode && groupNode.Content[i].Value == "use" {
					useNode = groupNode.Content[i+1]
					break
				}
			}
		}

		// 添加代理集合到 use
		alreadyInUse := false
		for _, u := range useNode.Content {
			if u.Kind == yaml.ScalarNode && u.Value == config.Name {
				alreadyInUse = true
				break
			}
		}
		if !alreadyInUse {
			useNode.Content = append(useNode.Content, &yaml.Node{Kind: yaml.ScalarNode, Value: config.Name})
		}

		log.Printf("[代理集合模式切换] 代理组恢复 use 引用: %s", config.Name)
	}

	// 3. 删除代理集合的节点（精确匹配 nodeNamesToRemove 列表）
	if proxiesNode != nil && proxiesNode.Kind == yaml.SequenceNode && len(nodeNamesToRemove) > 0 {
		// 构建需要删除的节点名称集合
		nodeNamesToRemoveSet := make(map[string]bool)
		for _, name := range nodeNamesToRemove {
			nodeNamesToRemoveSet[name] = true
		}

		newProxiesContent := make([]*yaml.Node, 0)
		removedCount := 0
		for _, p := range proxiesNode.Content {
			if p.Kind == yaml.MappingNode {
				name := util.GetNodeFieldValue(p, "name")
				if nodeNamesToRemoveSet[name] {
					removedCount++
					continue
				}
			}
			newProxiesContent = append(newProxiesContent, p)
		}
		if removedCount > 0 {
			modified = true
			proxiesNode.Content = newProxiesContent
			log.Printf("[代理集合模式切换] 删除 %d 个节点 (代理集合: %s)", removedCount, config.Name)
		}
	}

	// 4. 添加或更新 proxy-providers 配置
	if modified {
		providerConfig := createProxyProviderYAMLNode(config)

		if proxyProvidersNode == nil {
			// 创建 proxy-providers
			proxyProvidersNode = &yaml.Node{Kind: yaml.MappingNode, Content: make([]*yaml.Node, 0)}
			docContent.Content = append(docContent.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "proxy-providers"},
				proxyProvidersNode,
			)
		}

		// 检查是否已存在
		found := false
		for i := 0; i < len(proxyProvidersNode.Content)-1; i += 2 {
			if proxyProvidersNode.Content[i].Kind == yaml.ScalarNode && proxyProvidersNode.Content[i].Value == config.Name {
				proxyProvidersNode.Content[i+1] = providerConfig
				found = true
				break
			}
		}
		if !found {
			proxyProvidersNode.Content = append(proxyProvidersNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: config.Name},
				providerConfig,
			)
		}
		log.Printf("[代理集合模式切换] 添加 proxy-providers 配置: %s", config.Name)

		// 如果是新创建的，需要重新设置索引
		if proxyProvidersKeyIndex == -1 {
			for i := 0; i < len(docContent.Content)-1; i += 2 {
				if docContent.Content[i].Kind == yaml.ScalarNode && docContent.Content[i].Value == "proxy-providers" {
					proxyProvidersKeyIndex = i
					break
				}
			}
		}
	}

	return modified, nil
}

// updateYAMLNodeForMMW 更新 YAML 节点为 MMW 模式（与 updateYAMLFileWithProxyProviderNodes 逻辑相同）
func updateYAMLNodeForMMW(rootNode *yaml.Node, providerName, prefix string, proxies []any, nodeNames []string) (bool, error) {
	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return false, nil
	}

	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return false, nil
	}

	modified := false

	// 查找各节点
	var proxyGroupsNode *yaml.Node
	var proxiesNode *yaml.Node
	var proxyProvidersNode *yaml.Node
	var proxyProvidersKeyIndex int = -1

	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode {
			switch keyNode.Value {
			case "proxy-groups":
				proxyGroupsNode = valueNode
			case "proxies":
				proxiesNode = valueNode
			case "proxy-providers":
				proxyProvidersNode = valueNode
				proxyProvidersKeyIndex = i
			}
		}
	}

	if proxyGroupsNode == nil || proxyGroupsNode.Kind != yaml.SequenceNode {
		return false, nil
	}

	needCreateNewGroup := false

	// 遍历 proxy-groups，处理 use 引用
	for _, groupNode := range proxyGroupsNode.Content {
		if groupNode.Kind != yaml.MappingNode {
			continue
		}

		var useNode *yaml.Node
		var useKeyIndex int = -1
		var groupProxiesNode *yaml.Node
		var groupName string

		for i := 0; i < len(groupNode.Content)-1; i += 2 {
			keyNode := groupNode.Content[i]
			valueNode := groupNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode {
				switch keyNode.Value {
				case "use":
					useNode = valueNode
					useKeyIndex = i
				case "proxies":
					groupProxiesNode = valueNode
				case "name":
					if valueNode.Kind == yaml.ScalarNode {
						groupName = valueNode.Value
					}
				}
			}
		}

		if useNode == nil || useNode.Kind != yaml.SequenceNode {
			continue
		}

		// 检查是否包含此代理集合
		foundProvider := false
		newUseContent := make([]*yaml.Node, 0)
		for _, useItem := range useNode.Content {
			if useItem.Kind == yaml.ScalarNode && useItem.Value == providerName {
				foundProvider = true
			} else {
				newUseContent = append(newUseContent, useItem)
			}
		}

		if !foundProvider {
			continue
		}

		modified = true
		needCreateNewGroup = true
		log.Printf("[代理集合模式切换] 在代理组 %s 中找到代理集合 %s 的引用", groupName, providerName)

		// 确保 proxies 节点存在
		if groupProxiesNode == nil {
			groupProxiesNode = &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0)}
			groupNode.Content = append(groupNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "proxies"},
				groupProxiesNode,
			)
		}

		// 移除旧的代理组名称引用，添加新的
		newProxiesContent := make([]*yaml.Node, 0)
		for _, p := range groupProxiesNode.Content {
			if p.Kind == yaml.ScalarNode {
				if strings.HasPrefix(p.Value, prefix) || p.Value == providerName {
					continue
				}
			}
			newProxiesContent = append(newProxiesContent, p)
		}
		newProxiesContent = append(newProxiesContent, &yaml.Node{Kind: yaml.ScalarNode, Value: providerName})
		groupProxiesNode.Content = newProxiesContent

		// 更新 use 字段
		if len(newUseContent) == 0 && useKeyIndex >= 0 {
			groupNode.Content = append(groupNode.Content[:useKeyIndex], groupNode.Content[useKeyIndex+2:]...)
		} else {
			useNode.Content = newUseContent
		}
	}

	// 创建或更新同名代理组
	if needCreateNewGroup {
		existingGroupNode := (*yaml.Node)(nil)
		for _, groupNode := range proxyGroupsNode.Content {
			if groupNode.Kind == yaml.MappingNode {
				name := util.GetNodeFieldValue(groupNode, "name")
				if name == providerName {
					existingGroupNode = groupNode
					break
				}
			}
		}

		if existingGroupNode != nil {
			// 更新已存在的代理组
			var existingProxiesNode *yaml.Node
			for i := 0; i < len(existingGroupNode.Content)-1; i += 2 {
				keyNode := existingGroupNode.Content[i]
				valueNode := existingGroupNode.Content[i+1]
				if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "proxies" {
					existingProxiesNode = valueNode
					break
				}
			}

			if existingProxiesNode == nil {
				existingProxiesNode = &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0)}
				existingGroupNode.Content = append(existingGroupNode.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: "proxies"},
					existingProxiesNode,
				)
			}

			// 构建精确匹配集合
			nodeNamesSet := make(map[string]bool)
			for _, name := range nodeNames {
				nodeNamesSet[name] = true
			}

			newContent := make([]*yaml.Node, 0)
			for _, p := range existingProxiesNode.Content {
				if p.Kind == yaml.ScalarNode && nodeNamesSet[p.Value] {
					continue
				}
				newContent = append(newContent, p)
			}
			for _, nodeName := range nodeNames {
				newContent = append(newContent, &yaml.Node{Kind: yaml.ScalarNode, Value: nodeName})
			}
			existingProxiesNode.Content = newContent
		} else {
			// 创建新代理组
			newGroupNode := &yaml.Node{Kind: yaml.MappingNode}
			newGroupNode.Content = append(newGroupNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "name"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: providerName},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "type"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "url-test"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "url"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "http://www.gstatic.com/generate_204"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "interval"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "300"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: "tolerance"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: "50"},
			)

			newGroupProxies := &yaml.Node{Kind: yaml.SequenceNode}
			for _, nodeName := range nodeNames {
				newGroupProxies.Content = append(newGroupProxies.Content,
					&yaml.Node{Kind: yaml.ScalarNode, Value: nodeName})
			}
			newGroupNode.Content = append(newGroupNode.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "proxies"},
				newGroupProxies,
			)

			proxyGroupsNode.Content = append(proxyGroupsNode.Content, newGroupNode)
		}
	}

	if !modified {
		return false, nil
	}

	// 确保 proxies 节点存在
	if proxiesNode == nil {
		proxiesNode = &yaml.Node{Kind: yaml.SequenceNode, Content: make([]*yaml.Node, 0)}
		docContent.Content = append([]*yaml.Node{
			{Kind: yaml.ScalarNode, Value: "proxies"},
			proxiesNode,
		}, docContent.Content...)
	}

	// 移除旧的代理集合节点，添加新节点（精确匹配 nodeNames 列表）
	nodeNamesSet := make(map[string]bool)
	for _, name := range nodeNames {
		nodeNamesSet[name] = true
	}

	newProxiesContent := make([]*yaml.Node, 0)
	for _, p := range proxiesNode.Content {
		if p.Kind == yaml.MappingNode {
			name := util.GetNodeFieldValue(p, "name")
			if nodeNamesSet[name] {
				continue
			}
		}
		newProxiesContent = append(newProxiesContent, p)
	}

	for _, proxy := range proxies {
		if proxyMap, ok := proxy.(map[string]any); ok {
			proxyNode := util.ReorderProxyFieldsToNode(proxyMap)
			newProxiesContent = append(newProxiesContent, proxyNode)
		}
	}
	proxiesNode.Content = newProxiesContent

	// 清理 proxy-providers
	if proxyProvidersNode != nil && proxyProvidersNode.Kind == yaml.MappingNode && proxyProvidersKeyIndex >= 0 {
		newProvidersContent := make([]*yaml.Node, 0)
		for i := 0; i < len(proxyProvidersNode.Content)-1; i += 2 {
			keyNode := proxyProvidersNode.Content[i]
			valueNode := proxyProvidersNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == providerName {
				continue
			}
			newProvidersContent = append(newProvidersContent, keyNode, valueNode)
		}

		if len(newProvidersContent) == 0 {
			docContent.Content = append(docContent.Content[:proxyProvidersKeyIndex], docContent.Content[proxyProvidersKeyIndex+2:]...)
		} else {
			proxyProvidersNode.Content = newProvidersContent
		}
	}

	return true, nil
}

// createProxyProviderYAMLNode 创建 proxy-provider 的 YAML 配置节点
func createProxyProviderYAMLNode(config *storage.ProxyProviderConfig) *yaml.Node {
	node := &yaml.Node{Kind: yaml.MappingNode}

	// type
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "type"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: config.Type},
	)

	// path
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "path"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("./proxy_providers/%s.yaml", config.Name)},
	)

	// url (使用相对路径，实际 URL 需要前端填充)
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "url"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: fmt.Sprintf("/api/proxy-provider/%d", config.ID)},
	)

	// interval
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "interval"},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(config.Interval)},
	)

	// health-check
	if config.HealthCheckEnabled {
		healthCheck := &yaml.Node{Kind: yaml.MappingNode}
		healthCheck.Content = append(healthCheck.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "enable"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: "true"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: "url"},
			&yaml.Node{Kind: yaml.ScalarNode, Value: config.HealthCheckURL},
			&yaml.Node{Kind: yaml.ScalarNode, Value: "interval"},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(config.HealthCheckInterval)},
		)
		node.Content = append(node.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "health-check"},
			healthCheck,
		)
	}

	return node
}

// copyMapForProvider 深拷贝 map（用于代理节点）
func copyMapForProvider(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		switch vv := v.(type) {
		case map[string]any:
			result[k] = copyMapForProvider(vv)
		case []any:
			newSlice := make([]any, len(vv))
			copy(newSlice, vv)
			result[k] = newSlice
		default:
			result[k] = v
		}
	}
	return result
}
