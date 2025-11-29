package handler

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/substore"

	"gopkg.in/yaml.v3"
)

const subscriptionDefaultType = "clash"

// Token失效时返回的YAML内容
const tokenInvalidYAML = `allow-lan: false
dns:
  enable: true
  enhanced-mode: fake-ip
  ipv6: true
  nameserver:
    - https://120.53.53.53/dns-query
    - https://223.5.5.5/dns-query
  nameserver-policy:
    geosite:cn,private:
      - https://120.53.53.53/dns-query
      - https://223.5.5.5/dns-query
    geosite:geolocation-!cn:
      - https://dns.cloudflare.com/dns-query
      - https://dns.google/dns-query
  proxy-server-nameserver:
    - https://120.53.53.53/dns-query
    - https://223.5.5.5/dns-query
  respect-rules: true
geo-auto-update: true
geo-update-interval: 24
geodata-loader: standard
geodata-mode: true
geox-url:
  asn: https://github.com/xishang0128/geoip/releases/download/latest/GeoLite2-ASN.mmdb
  geoip: https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geoip.dat
  geosite: https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/geosite.dat
  mmdb: https://testingcf.jsdelivr.net/gh/MetaCubeX/meta-rules-dat@release/country.mmdb
log-level: info
mode: rule
port: 7890
proxies:
  - name: ⚠️ Token已过期
    type: ss
    server: test.example.com.cn
    port: 443
    password: password123
    cipher: 2022-blake3-chacha20-poly1305
  - name: ⚠️ 请联系管理员
    type: ss
    server: test.example.com.cn
    port: 443
    password: password123
    cipher: 2022-blake3-chacha20-poly1305
proxy-groups:
  - name: 🚀 节点选择
    type: select
    proxies:
      - Token已过期
      - 请联系管理员
rules:
  - MATCH,DIRECT
socks-port: 7891
`

// Context key for token invalid flag
type ContextKey string

const TokenInvalidKey ContextKey = "token_invalid"

type SubscriptionHandler struct {
	summary  *TrafficSummaryHandler
	repo     *storage.TrafficRepository
	baseDir  string
	fallback string
}

type subscriptionEndpoint struct {
	tokens *auth.TokenStore
	repo   *storage.TrafficRepository
	inner  *SubscriptionHandler
}

func NewSubscriptionHandler(repo *storage.TrafficRepository, baseDir string) http.Handler {
	if repo == nil {
		panic("subscription handler requires repository")
	}

	summary := NewTrafficSummaryHandler(repo)
	return newSubscriptionHandler(summary, repo, baseDir, subscriptionDefaultType)
}

// NewSubscriptionHandlerConcrete creates a subscription handler and returns the concrete type.
// This is used when other handlers need direct access to the SubscriptionHandler.
func NewSubscriptionHandlerConcrete(repo *storage.TrafficRepository, baseDir string) *SubscriptionHandler {
	if repo == nil {
		panic("subscription handler requires repository")
	}

	summary := NewTrafficSummaryHandler(repo)
	return newSubscriptionHandler(summary, repo, baseDir, subscriptionDefaultType)
}

// NewSubscriptionEndpoint returns a handler that serves subscription files, allowing either session tokens or user tokens via query parameter.
func NewSubscriptionEndpoint(tokens *auth.TokenStore, repo *storage.TrafficRepository, baseDir string) http.Handler {
	if tokens == nil {
		panic("subscription endpoint requires token store")
	}
	if repo == nil {
		panic("subscription endpoint requires repository")
	}

	inner := newSubscriptionHandler(nil, repo, baseDir, subscriptionDefaultType)
	return &subscriptionEndpoint{tokens: tokens, repo: repo, inner: inner}
}

func newSubscriptionHandler(summary *TrafficSummaryHandler, repo *storage.TrafficRepository, baseDir, fallback string) *SubscriptionHandler {
	if summary == nil {
		if repo == nil {
			panic("subscription handler requires repository")
		}
		summary = NewTrafficSummaryHandler(repo)
	}

	if repo == nil {
		panic("subscription handler requires repository")
	}

	if baseDir == "" {
		baseDir = filepath.FromSlash("subscribes")
	}

	cleanedBase := filepath.Clean(baseDir)
	if fallback == "" {
		fallback = subscriptionDefaultType
	}

	return &SubscriptionHandler{summary: summary, repo: repo, baseDir: cleanedBase, fallback: fallback}
}

func (s *subscriptionEndpoint) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	request, ok := s.authorizeRequest(w, r)
	if !ok {
		return
	}

	s.inner.ServeHTTP(w, request)
}

func (s *subscriptionEndpoint) authorizeRequest(w http.ResponseWriter, r *http.Request) (*http.Request, bool) {
	if r.Method != http.MethodGet {
		// allow handler to respond with method restrictions
		return r, true
	}

	// Check for username parameter (from composite short link - already authenticated by short link handler)
	queryUsername := strings.TrimSpace(r.URL.Query().Get("username"))
	if queryUsername != "" {
		ctx := auth.ContextWithUsername(r.Context(), queryUsername)
		return r.WithContext(ctx), true
	}

	// Check for token parameter (legacy/direct access)
	queryToken := strings.TrimSpace(r.URL.Query().Get("token"))
	if queryToken != "" && s.repo != nil {
		username, err := s.repo.ValidateUserToken(r.Context(), queryToken)
		if err == nil {
			ctx := auth.ContextWithUsername(r.Context(), username)
			return r.WithContext(ctx), true
		}
		if !errors.Is(err, storage.ErrTokenNotFound) {
			writeError(w, http.StatusInternalServerError, err)
			return nil, false
		}
	}

	// Check for header token (session-based access)
	headerToken := strings.TrimSpace(r.Header.Get(auth.AuthHeader))
	username, ok := s.tokens.Lookup(headerToken)
	if ok {
		ctx := auth.ContextWithUsername(r.Context(), username)
		return r.WithContext(ctx), true
	}

	// 所有认证方式都失败，设置token失效标记
	ctx := context.WithValue(r.Context(), TokenInvalidKey, true)
	return r.WithContext(ctx), true
}

func (h *SubscriptionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
		return
	}

	// 检查是否是token失效场景
	if tokenInvalid, ok := r.Context().Value(TokenInvalidKey).(bool); ok && tokenInvalid {
		h.serveTokenInvalidResponse(w, r)
		return
	}

	// Get username from context
	username := auth.UsernameFromContext(r.Context())

	// 尝试获取流量信息，如果探针未配置则跳过流量统计
	totalLimit, _, totalUsed, err := h.summary.fetchTotals(r.Context(), username)
	hasTrafficInfo := err == nil
	// 如果是探针未配置的错误，不返回错误，继续处理
	if err != nil && !errors.Is(err, storage.ErrProbeConfigNotFound) {
		// 其他错误才返回
		writeError(w, http.StatusBadGateway, err)
		return
	}

	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	var subscribeFile storage.SubscribeFile
	var displayName string

	if filename != "" {
		subscribeFile, err = h.repo.GetSubscribeFileByFilename(r.Context(), filename)
		if err != nil {
			if errors.Is(err, storage.ErrSubscribeFileNotFound) {
				writeError(w, http.StatusNotFound, errors.New("not found"))
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		displayName = subscribeFile.Name
	} else {
		// TODO: 订阅链接已经配置到客户端，管理员修改文件名后，原订阅链接无法使用
		// 1.0 版本时改为与表里的ID关联，暂时先不改
		legacyName := strings.TrimSpace(r.URL.Query().Get("t"))
		link, err := h.resolveSubscription(r.Context(), legacyName)
		if err != nil {
			if errors.Is(err, storage.ErrSubscriptionNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		filename = link.RuleFilename
		displayName = link.Name
	}

	cleanedName := filepath.Clean(filename)
	if strings.HasPrefix(cleanedName, "..") {
		writeError(w, http.StatusBadRequest, errors.New("invalid rule filename"))
		return
	}

	resolvedPath := filepath.Join(h.baseDir, cleanedName)

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	// Check if force sync external subscriptions is enabled and sync only referenced subscriptions
	if username != "" && h.repo != nil {
		settings, err := h.repo.GetUserSettings(r.Context(), username)
		if err == nil && settings.ForceSyncExternal {
			log.Printf("[Subscription] User %s has force sync enabled (cache_expire_minutes: %d)", username, settings.CacheExpireMinutes)

			// Get external subscriptions referenced in current file
			usedExternalSubs, err := getExternalSubscriptionsFromFile(r.Context(), data, username, h.repo)
			if err != nil {
				log.Printf("[Subscription] Failed to get external subscriptions from file: %v", err)
			} else if len(usedExternalSubs) > 0 {
				log.Printf("[Subscription] Found %d external subscriptions referenced in current file", len(usedExternalSubs))

				// Get user's external subscriptions to check cache and get URLs
				allExternalSubs, err := h.repo.ListExternalSubscriptions(r.Context(), username)
				if err != nil {
					log.Printf("[Subscription] Failed to list external subscriptions: %v", err)
				} else {
					// Filter to only sync subscriptions that are referenced in the current file
					var subsToSync []storage.ExternalSubscription
					subURLMap := make(map[string]string) // URL -> name mapping

					for _, sub := range allExternalSubs {
						subURLMap[sub.URL] = sub.Name
						if _, used := usedExternalSubs[sub.URL]; used {
							subsToSync = append(subsToSync, sub)
						}
					}

					log.Printf("[Subscription] ForceSyncExternal enabled: will sync %d/%d external subscriptions referenced in current file", len(subsToSync), len(allExternalSubs))

					// Check if we need to sync based on cache expiration
					shouldSync := false
					if settings.CacheExpireMinutes > 0 {
						// Check last sync time only for referenced subscriptions
						for _, sub := range subsToSync {
							if sub.LastSyncAt == nil {
								// Never synced before
								log.Printf("[Subscription] Subscription %s (%s) never synced, will sync", sub.Name, sub.URL)
								shouldSync = true
								break
							}

							// Calculate time difference in minutes
							elapsed := time.Since(*sub.LastSyncAt).Minutes()
							if elapsed >= float64(settings.CacheExpireMinutes) {
								// Cache expired
								log.Printf("[Subscription] Subscription %s (%s) cache expired (%.2f >= %d minutes), will sync", sub.Name, sub.URL, elapsed, settings.CacheExpireMinutes)
								shouldSync = true
								break
							}
						}
						if !shouldSync {
							log.Printf("[Subscription] All referenced subscriptions are within cache time, skipping sync")
						}
					} else {
						// Cache expire minutes is 0, always sync
						log.Printf("[Subscription] Cache expire minutes is 0, will always sync referenced subscriptions")
						shouldSync = true
					}

					if shouldSync {
						log.Printf("[Subscription] Starting external subscriptions sync for user %s (only referenced subscriptions)", username)
						// Sync only the referenced external subscriptions
						if err := syncReferencedExternalSubscriptions(r.Context(), h.repo, h.baseDir, username, subsToSync); err != nil {
							log.Printf("[Subscription] Failed to sync external subscriptions: %v", err)
							// Log error but don't fail the request
							// The sync is best-effort
						} else {
							log.Printf("[Subscription] External subscriptions sync completed successfully")

							// Re-read the subscription file after sync to get updated nodes
							updatedData, err := os.ReadFile(resolvedPath)
							if err != nil {
								log.Printf("[Subscription] Failed to re-read subscription file after sync: %v", err)
							} else {
								data = updatedData
								log.Printf("[Subscription] Re-read subscription file after sync, got %d bytes", len(data))
							}
						}
					}
				}
			} else {
				log.Printf("[Subscription] No external subscriptions referenced in current file, skipping sync")
			}
		}
	}

	// 根据参数t的类型调用substore的转换代码
	clientType := strings.TrimSpace(r.URL.Query().Get("t"))
	// 默认浏览器打开时直接输入文本, 不再下载问卷
	contentType := "text/yaml; charset=utf-8; charset=UTF-8"
	ext := filepath.Ext(filename)
	if ext == "" {
		ext = ".yaml"
	}

	// clash 和 clashmeta 类型直接输出源文件, 不需要转换
	if clientType != "" && clientType != "clash" && clientType != "clashmeta" {
		// Convert subscription using substore producers
		convertedData, err := h.convertSubscription(data, clientType)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Errorf("failed to convert subscription for client %s: %w", clientType, err))
			return
		}
		data = convertedData

		// Set content type and extension based on client type
		switch clientType {
		case "surge", "surgemac", "loon", "qx", "surfboard", "shadowrocket":
			// Text-based formats
			contentType = "text/plain; charset=utf-8"
			ext = ".txt"
		case "sing-box":
			// JSON format
			contentType = "application/json; charset=utf-8"
			ext = ".json"
		case "v2ray":
			// Base64 format
			contentType = "text/plain; charset=utf-8"
			ext = ".txt"
		case "uri":
			// URI format
			contentType = "text/plain; charset=utf-8"
			ext = ".txt"
		default:
			// YAML-based formats (clash, clashmeta, stash, shadowrocket, egern)
			contentType = "text/yaml; charset=utf-8"
			ext = ".yaml"
		}
	}

	// 检查是否需要汇总外部订阅的流量信息
	externalTrafficLimit, externalTrafficUsed := int64(0), int64(0)
	if username != "" && h.repo != nil {
		settings, err := h.repo.GetUserSettings(r.Context(), username)
		if err == nil && settings.SyncTraffic {
			log.Printf("[Subscription] User %s has SyncTraffic enabled, checking for external subscription nodes", username)
			// 解析 YAML 文件，获取其中使用的节点名称
			var yamlConfig map[string]any
			if err := yaml.Unmarshal(data, &yamlConfig); err == nil {
				if proxies, ok := yamlConfig["proxies"].([]any); ok {
					log.Printf("[Subscription] Found %d proxies in subscription YAML", len(proxies))
					// 收集所有节点名称
					usedNodeNames := make(map[string]bool)
					for _, proxy := range proxies {
						if proxyMap, ok := proxy.(map[string]any); ok {
							if name, ok := proxyMap["name"].(string); ok && name != "" {
								usedNodeNames[name] = true
							}
						}
					}

					// 如果有节点名称，从数据库查询这些节点的 tag
					if len(usedNodeNames) > 0 {
						log.Printf("[Subscription] Querying database for %d nodes", len(usedNodeNames))
						nodes, err := h.repo.ListNodes(r.Context(), username)
						if err == nil {
							// 收集使用到的外部订阅名称（通过 tag 识别）
							usedExternalSubs := make(map[string]bool)
							for _, node := range nodes {
								// 检查节点是否在订阅文件中
								if usedNodeNames[node.NodeName] {
									// 如果 tag 不是默认值，说明是外部订阅节点
									if node.Tag != "" && node.Tag != "手动输入" {
										usedExternalSubs[node.Tag] = true
										log.Printf("[Subscription] Node '%s' is from external subscription '%s'", node.NodeName, node.Tag)
									}
								}
							}

							// 如果有使用到外部订阅的节点，汇总这些订阅的流量
							if len(usedExternalSubs) > 0 {
								log.Printf("[Subscription] Found %d external subscriptions in use: %v", len(usedExternalSubs), getKeys(usedExternalSubs))
								externalSubs, err := h.repo.ListExternalSubscriptions(r.Context(), username)
								if err == nil {
									now := time.Now()
									for _, sub := range externalSubs {
										// 只汇总使用到的外部订阅
										if usedExternalSubs[sub.Name] {
											// 如果有过期时间且已过期，则跳过
											// 如果过期时间为空，表示长期订阅，不跳过
											if sub.Expire != nil && sub.Expire.Before(now) {
												log.Printf("[Subscription] Skipping expired external subscription '%s' (expired at %s)", sub.Name, sub.Expire.Format("2006-01-02 15:04:05"))
												continue
											}
											if sub.Expire == nil {
												log.Printf("[Subscription] Adding traffic from long-term external subscription '%s': upload=%d, download=%d, total=%d",
													sub.Name, sub.Upload, sub.Download, sub.Total)
											} else {
												log.Printf("[Subscription] Adding traffic from external subscription '%s': upload=%d, download=%d, total=%d (expires at %s)",
													sub.Name, sub.Upload, sub.Download, sub.Total, sub.Expire.Format("2006-01-02 15:04:05"))
											}
											externalTrafficLimit += sub.Total
											externalTrafficUsed += sub.Upload + sub.Download
										}
									}
									log.Printf("[Subscription] External subscriptions traffic total: limit=%d bytes (%.2f GB), used=%d bytes (%.2f GB)",
										externalTrafficLimit, float64(externalTrafficLimit)/(1024*1024*1024),
										externalTrafficUsed, float64(externalTrafficUsed)/(1024*1024*1024))
								} else {
									log.Printf("[Subscription] Failed to list external subscriptions: %v", err)
								}
							} else {
								log.Printf("[Subscription] No external subscription nodes found in use")
							}
						} else {
							log.Printf("[Subscription] Failed to list nodes: %v", err)
						}
					}
				}
			}
		}
	}

	attachmentName := url.PathEscape("妙妙屋-" + displayName + ext)

	// 对于 YAML 格式的数据，重新排序以将 rule-providers 放在最后
	if contentType == "text/yaml; charset=utf-8" || contentType == "text/yaml; charset=utf-8; charset=UTF-8" {
		// 使用 yaml.Node 来保持原始类型信息（避免 563905e2 被解析为科学计数法）
		var yamlNode yaml.Node
		if err := yaml.Unmarshal(data, &yamlNode); err == nil {
			// 检查是否有 rule-providers 需要重新排序
			// yamlNode.Content[0] 是文档节点，yamlNode.Content[0].Content 是根映射的键值对
			if len(yamlNode.Content) > 0 && yamlNode.Content[0].Kind == yaml.MappingNode {
				rootMap := yamlNode.Content[0]

				// 重新排序 proxies 中每个节点的字段
				for i := 0; i < len(rootMap.Content); i += 2 {
					if rootMap.Content[i].Value == "proxies" {
						proxiesNode := rootMap.Content[i+1]
						if proxiesNode.Kind == yaml.SequenceNode {
							reorderProxies(proxiesNode)
						}
						break
					}
				}

				// 重新排序 proxy-groups 中每个代理组的字段
				for i := 0; i < len(rootMap.Content); i += 2 {
					if rootMap.Content[i].Value == "proxy-groups" {
						proxyGroupsNode := rootMap.Content[i+1]
						if proxyGroupsNode.Kind == yaml.SequenceNode {
							reorderProxyGroups(proxyGroupsNode)
						}
						break
					}
				}

				// 查找 rule-providers 的位置
				ruleProvidersIdx := -1
				for i := 0; i < len(rootMap.Content); i += 2 {
					if rootMap.Content[i].Value == "rule-providers" {
						ruleProvidersIdx = i
						break
					}
				}

				// 如果找到 rule-providers 且不在最后，则移动到最后
				if ruleProvidersIdx >= 0 && ruleProvidersIdx < len(rootMap.Content)-2 {
					// 提取 rule-providers 的键和值
					keyNode := rootMap.Content[ruleProvidersIdx]
					valueNode := rootMap.Content[ruleProvidersIdx+1]

					// 从原位置删除
					rootMap.Content = append(rootMap.Content[:ruleProvidersIdx], rootMap.Content[ruleProvidersIdx+2:]...)

					// 添加到最后
					rootMap.Content = append(rootMap.Content, keyNode, valueNode)
				}
			}

			// 重新序列化为 YAML (使用2空格缩进)
			if reorderedData, err := MarshalYAMLWithIndent(&yamlNode); err == nil {
				// Fix emoji escapes and quoted numbers
				fixed := RemoveUnicodeEscapeQuotes(string(reorderedData))
				data = []byte(fixed)
			}
		}
	}

	w.Header().Set("Content-Type", contentType)
	// 只有在有流量信息时才添加 subscription-userinfo 头
	if hasTrafficInfo || externalTrafficLimit > 0 {
		// 汇总探针流量和外部订阅流量
		finalLimit := totalLimit + externalTrafficLimit
		finalUsed := totalUsed + externalTrafficUsed

		log.Printf("[Subscription] Final traffic summary for user %s:", username)
		log.Printf("[Subscription]   Probe traffic:    limit=%d bytes (%.2f GB), used=%d bytes (%.2f GB)",
			totalLimit, float64(totalLimit)/(1024*1024*1024),
			totalUsed, float64(totalUsed)/(1024*1024*1024))
		log.Printf("[Subscription]   External traffic: limit=%d bytes (%.2f GB), used=%d bytes (%.2f GB)",
			externalTrafficLimit, float64(externalTrafficLimit)/(1024*1024*1024),
			externalTrafficUsed, float64(externalTrafficUsed)/(1024*1024*1024))
		log.Printf("[Subscription]   Total traffic:    limit=%d bytes (%.2f GB), used=%d bytes (%.2f GB)",
			finalLimit, float64(finalLimit)/(1024*1024*1024),
			finalUsed, float64(finalUsed)/(1024*1024*1024))

		headerValue := buildSubscriptionHeader(finalLimit, finalUsed)
		w.Header().Set("subscription-userinfo", headerValue)
		log.Printf("[Subscription] Setting subscription-userinfo header: %s", headerValue)
	}
	w.Header().Set("profile-update-interval", "24")
	if clientType == "" {
		w.Header().Set("content-disposition", "attachment;filename*=UTF-8''"+attachmentName)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)
}

func (h *SubscriptionHandler) resolveSubscription(ctx context.Context, name string) (storage.SubscriptionLink, error) {
	if h == nil {
		return storage.SubscriptionLink{}, errors.New("subscription handler not initialized")
	}

	if h.repo == nil {
		return storage.SubscriptionLink{}, errors.New("subscription repository not configured")
	}

	trimmed := strings.TrimSpace(name)
	if trimmed != "" {
		return h.repo.GetSubscriptionByName(ctx, trimmed)
	}

	if h.fallback != "" {
		link, err := h.repo.GetSubscriptionByName(ctx, h.fallback)
		if err == nil {
			return link, nil
		}
		if !errors.Is(err, storage.ErrSubscriptionNotFound) {
			return storage.SubscriptionLink{}, err
		}
	}

	return h.repo.GetFirstSubscriptionLink(ctx)
}

func buildSubscriptionHeader(totalLimit, totalUsed int64) string {
	download := strconv.FormatInt(totalUsed, 10)
	total := strconv.FormatInt(totalLimit, 10)
	return "upload=0; download=" + download + "; total=" + total + "; expire="
}

// getKeys returns the keys of a map as a slice
func getKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getExternalSubscriptionsFromFile extracts external subscription URLs from YAML file content
// by analyzing proxies and querying the database for their raw_url (external subscription links)
func getExternalSubscriptionsFromFile(ctx context.Context, data []byte, username string, repo *storage.TrafficRepository) (map[string]bool, error) {
	usedURLs := make(map[string]bool)

	// Parse YAML content
	var yamlContent map[string]any
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		return usedURLs, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Extract proxies and query database for their raw_url
	if proxies, ok := yamlContent["proxies"].([]any); ok {
		log.Printf("[Subscription] Found %d proxies in subscription file", len(proxies))

		// Collect all proxy names
		proxyNames := make(map[string]bool)
		for _, proxy := range proxies {
			if proxyMap, ok := proxy.(map[string]any); ok {
				if name, ok := proxyMap["name"].(string); ok && name != "" {
					proxyNames[name] = true
				}
			}
		}

		if len(proxyNames) > 0 {
			log.Printf("[Subscription] Querying database for %d proxies to find external subscription URLs", len(proxyNames))

			// Query database for nodes with these names
			nodes, err := repo.ListNodes(ctx, username)
			if err != nil {
				log.Printf("[Subscription] Failed to list nodes from database: %v", err)
				return usedURLs, fmt.Errorf("failed to list nodes: %w", err)
			}

			// Find matching nodes and collect their raw_url
			for _, node := range nodes {
				if proxyNames[node.NodeName] && node.RawURL != "" {
					usedURLs[node.RawURL] = true
					log.Printf("[Subscription] Found external subscription URL from node '%s': %s", node.NodeName, node.RawURL)
				}
			}
		}
	}

	log.Printf("[Subscription] Found %d unique external subscription URLs referenced in current file", len(usedURLs))
	return usedURLs, nil
}

// syncReferencedExternalSubscriptions syncs only the specified external subscriptions
func syncReferencedExternalSubscriptions(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string, subsToSync []storage.ExternalSubscription) error {
	if repo == nil || username == "" || len(subsToSync) == 0 {
		return fmt.Errorf("invalid parameters")
	}

	// Get user settings to check match rule
	userSettings, err := repo.GetUserSettings(ctx, username)
	if err != nil {
		// If settings not found, use default match rule
		userSettings.MatchRule = "node_name"
	}

	log.Printf("[Subscription] User %s has %d referenced external subscriptions to sync (match rule: %s)", username, len(subsToSync), userSettings.MatchRule)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track total nodes synced
	totalNodesSynced := 0

	for _, sub := range subsToSync {
		nodeCount, updatedSub, err := syncSingleExternalSubscription(ctx, client, repo, subscribeDir, username, sub, userSettings.MatchRule)
		if err != nil {
			log.Printf("[Subscription] Failed to sync subscription %s (%s): %v", sub.Name, sub.URL, err)
			continue
		}

		totalNodesSynced += nodeCount

		// Update last sync time and node count
		// Use updatedSub which contains traffic info from parseAndUpdateTrafficInfo
		now := time.Now()
		updatedSub.LastSyncAt = &now
		updatedSub.NodeCount = nodeCount
		if err := repo.UpdateExternalSubscription(ctx, updatedSub); err != nil {
			log.Printf("[Subscription] Failed to update sync time for subscription %s: %v", sub.Name, err)
		}
	}

	log.Printf("[Subscription] Completed: synced %d nodes total from %d referenced subscriptions", totalNodesSynced, len(subsToSync))

	return nil
}

// serveTokenInvalidResponse serves the token invalid YAML content with client type conversion
func (h *SubscriptionHandler) serveTokenInvalidResponse(w http.ResponseWriter, r *http.Request) {
	data := []byte(tokenInvalidYAML)

	// 根据参数t的类型调用substore的转换代码
	clientType := strings.TrimSpace(r.URL.Query().Get("t"))
	contentType := "text/yaml; charset=utf-8"
	ext := ".yaml"

	// 如果指定了客户端类型且不是clash/clashmeta，进行转换
	if clientType != "" && clientType != "clash" && clientType != "clashmeta" {
		convertedData, err := h.convertSubscription(data, clientType)
		if err != nil {
			// 转换失败，记录日志但继续返回YAML
			log.Printf("[Token Invalid] Failed to convert for client %s: %v", clientType, err)
		} else {
			data = convertedData

			// 根据客户端类型设置content type和扩展名
			switch clientType {
			case "surge", "surgemac", "loon", "qx", "surfboard", "shadowrocket":
				contentType = "text/plain; charset=utf-8"
				ext = ".txt"
			case "sing-box":
				contentType = "application/json; charset=utf-8"
				ext = ".json"
			case "v2ray", "uri":
				contentType = "text/plain; charset=utf-8"
				ext = ".txt"
			default:
				contentType = "text/yaml; charset=utf-8"
				ext = ".yaml"
			}
		}
	}

	attachmentName := url.PathEscape("Token已失效" + ext)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("profile-update-interval", "24")
	if clientType == "" {
		w.Header().Set("content-disposition", "attachment;filename*=UTF-8''"+attachmentName)
	}
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(data)

	log.Printf("[Token Invalid] Served token invalid response, client type: %s", clientType)
}

// convertSubscription converts a YAML subscription file to the specified client format
func (h *SubscriptionHandler) convertSubscription(yamlData []byte, clientType string) ([]byte, error) {
	// 读取yaml
	var config map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// 读取yaml中proxies属性的节点列表
	proxiesRaw, ok := config["proxies"]
	if !ok {
		return nil, errors.New("no 'proxies' field found in YAML")
	}

	proxiesArray, ok := proxiesRaw.([]interface{})
	if !ok {
		return nil, errors.New("'proxies' field is not an array")
	}

	// 转换成substore的Proxy结构
	var proxies []substore.Proxy
	for _, p := range proxiesArray {
		proxyMap, ok := p.(map[string]interface{})
		if !ok {
			continue
		}
		proxies = append(proxies, substore.Proxy(proxyMap))
	}

	if len(proxies) == 0 {
		return nil, errors.New("no valid proxies found in YAML")
	}

	factory := substore.GetDefaultFactory()

	// 根据客户端类型获取Producer
	producer, err := factory.GetProducer(clientType)
	if err != nil {
		return nil, fmt.Errorf("unsupported client type '%s': %w", clientType, err)
	}

	// 调用Produce方法生成转换后的节点, 这里不处理原substore的internal模式与额外菜蔬
	result, err := producer.Produce(proxies, "", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to produce subscription: %w", err)
	}
	switch v := result.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	default:
		return nil, fmt.Errorf("unexpected result type from producer: %T, expected string or []byte", result)
	}
}

// reorderProxies reorders each proxy's fields in the sequence node
func reorderProxies(seqNode *yaml.Node) {
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		return
	}

	// Process each proxy in the sequence
	for _, proxyNode := range seqNode.Content {
		if proxyNode.Kind == yaml.MappingNode {
			reorderProxyNode(proxyNode)
		}
	}
}

// reorderProxyNode reorders proxy configuration fields
// Priority order: name, type, server, port, then all other fields
func reorderProxyNode(proxyNode *yaml.Node) {
	if proxyNode == nil || proxyNode.Kind != yaml.MappingNode {
		return
	}

	// Priority fields in desired order
	priorityFields := []string{"name", "type", "server", "port"}

	// Create a map of existing fields
	fieldMap := make(map[string]*yaml.Node)
	fieldKeyNodes := make(map[string]*yaml.Node) // Store original key nodes to preserve style
	remainingFields := []*yaml.Node{}

	// Parse existing fields
	for i := 0; i < len(proxyNode.Content); i += 2 {
		if i+1 >= len(proxyNode.Content) {
			break
		}
		keyNode := proxyNode.Content[i]
		valueNode := proxyNode.Content[i+1]

		// Check if this is a priority field
		isPriority := false
		for _, pf := range priorityFields {
			if keyNode.Value == pf {
				fieldMap[pf] = valueNode
				fieldKeyNodes[pf] = keyNode
				isPriority = true
				break
			}
		}

		// If not a priority field, save both key and value for later
		if !isPriority {
			remainingFields = append(remainingFields, keyNode, valueNode)
		}
	}

	// Rebuild the Content with ordered fields
	newContent := []*yaml.Node{}

	// Add priority fields first (in order)
	for _, fieldName := range priorityFields {
		if valueNode, exists := fieldMap[fieldName]; exists {
			// Use original key node if available, otherwise create new one
			keyNode := fieldKeyNodes[fieldName]
			if keyNode == nil {
				keyNode = &yaml.Node{
					Kind:  yaml.ScalarNode,
					Value: fieldName,
				}
			}
			newContent = append(newContent, keyNode, valueNode)
		}
	}

	// Add remaining fields
	newContent = append(newContent, remainingFields...)

	// Replace the original content
	proxyNode.Content = newContent
}

// reorderProxyGroups reorders each proxy group's fields in the sequence node
func reorderProxyGroups(seqNode *yaml.Node) {
	if seqNode == nil || seqNode.Kind != yaml.SequenceNode {
		return
	}

	// Process each proxy group in the sequence
	for _, groupNode := range seqNode.Content {
		if groupNode.Kind == yaml.MappingNode {
			reorderProxyGroupFields(groupNode)
		}
	}
}

// reorderProxyGroupFields reorders proxy group configuration fields
// Priority order: name, type, strategy, proxies, url, interval, tolerance, lazy, hidden
func reorderProxyGroupFields(groupNode *yaml.Node) {
	if groupNode == nil || groupNode.Kind != yaml.MappingNode {
		return
	}

	// Priority fields in desired order
	priorityFields := []string{"name", "type", "strategy", "proxies", "url", "interval", "tolerance", "lazy", "hidden"}

	// Create a map of existing fields
	fieldMap := make(map[string]*yaml.Node)
	remainingFields := []*yaml.Node{}

	// Parse existing fields
	for i := 0; i < len(groupNode.Content); i += 2 {
		if i+1 >= len(groupNode.Content) {
			break
		}
		keyNode := groupNode.Content[i]
		valueNode := groupNode.Content[i+1]

		// Check if this is a priority field
		isPriority := false
		for _, pf := range priorityFields {
			if keyNode.Value == pf {
				fieldMap[pf] = valueNode
				isPriority = true
				break
			}
		}

		// If not a priority field, save both key and value for later
		if !isPriority {
			remainingFields = append(remainingFields, keyNode, valueNode)
		}
	}

	// Rebuild the Content with ordered fields
	newContent := []*yaml.Node{}

	// Add priority fields first (in order)
	for _, fieldName := range priorityFields {
		if valueNode, exists := fieldMap[fieldName]; exists {
			keyNode := &yaml.Node{
				Kind:  yaml.ScalarNode,
				Value: fieldName,
			}
			newContent = append(newContent, keyNode, valueNode)
		}
	}

	// Add remaining fields
	newContent = append(newContent, remainingFields...)

	// Replace the original content
	groupNode.Content = newContent
}
