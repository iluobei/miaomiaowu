package handler

import (
	"context"
	"errors"
	"fmt"
	"miaomiaowu/internal/logger"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
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

	filename := strings.TrimSpace(r.URL.Query().Get("filename"))
	var subscribeFile storage.SubscribeFile
	var displayName string
	var err error

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
			logger.Info("[Subscription] 用户启用强制同步", "user", username, "cache_expire_minutes", settings.CacheExpireMinutes)

			// Get external subscriptions referenced in current file
			usedExternalSubs, err := getExternalSubscriptionsFromFile(r.Context(), data, username, h.repo)
			if err != nil {
				logger.Info("[Subscription] 获取文件中的外部订阅失败", "error", err)
			} else if len(usedExternalSubs) > 0 {
				logger.Info("[Subscription] 找到当前文件引用的外部订阅", "count", len(usedExternalSubs))

				// Get user's external subscriptions to check cache and get URLs
				allExternalSubs, err := h.repo.ListExternalSubscriptions(r.Context(), username)
				if err != nil {
					logger.Info("[Subscription] 获取外部订阅列表失败", "error", err)
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

					logger.Info("[Subscription] 强制同步已启用，将同步引用的外部订阅", "sync_count", len(subsToSync), "total_count", len(allExternalSubs))

					// Check if we need to sync based on cache expiration
					shouldSync := false
					if settings.CacheExpireMinutes > 0 {
						// Check last sync time only for referenced subscriptions
						for _, sub := range subsToSync {
							if sub.LastSyncAt == nil {
								// Never synced before
								logger.Info("[Subscription] 订阅从未同步过，将进行同步", "name", sub.Name, "url", sub.URL)
								shouldSync = true
								break
							}

							// Calculate time difference in minutes
							elapsed := time.Since(*sub.LastSyncAt).Minutes()
							if elapsed >= float64(settings.CacheExpireMinutes) {
								// Cache expired
								logger.Info("[Subscription] 订阅缓存已过期，将进行同步", "name", sub.Name, "url", sub.URL, "elapsed_minutes", elapsed, "expire_minutes", settings.CacheExpireMinutes)
								shouldSync = true
								break
							}
						}
						if !shouldSync {
							logger.Info("[Subscription] All referenced subscriptions are within cache time, skipping sync")
						}
					} else {
						// Cache expire minutes is 0, always sync
						logger.Info("[Subscription] Cache expire minutes is 0, will always sync referenced subscriptions")
						shouldSync = true
					}

					if shouldSync {
						logger.Info("[Subscription] 开始同步用户的外部订阅(仅引用的订阅)", "user", username)
						// Sync only the referenced external subscriptions
						if err := syncReferencedExternalSubscriptions(r.Context(), h.repo, h.baseDir, username, subsToSync); err != nil {
							logger.Info("[Subscription] 同步外部订阅失败", "error", err)
							// Log error but don't fail the request
							// The sync is best-effort
						} else {
							logger.Info("[Subscription] External subscriptions sync completed successfully")

							// Re-read the subscription file after sync to get updated nodes
							updatedData, err := os.ReadFile(resolvedPath)
							if err != nil {
								logger.Info("[Subscription] 同步后重新读取订阅文件失败", "error", err)
							} else {
								data = updatedData
								logger.Info("[Subscription] 同步后重新读取订阅文件成功", "bytes", len(data))
							}
						}
					}
				}
			} else {
				logger.Info("[Subscription] No external subscriptions referenced in current file, skipping sync")
			}
		}
	}

	// 在转换订阅格式之前，先收集探针服务器和外部订阅流量信息
	// 这样可以确保无论订阅被转换成什么格式，都能正确收集信息
	externalTrafficLimit, externalTrafficUsed := int64(0), int64(0)
	usesProbeNodes := false                      // 是否使用了探针节点
	probeBindingEnabled := false                 // 是否开启了探针服务器绑定
	var usedProbeServers map[string]struct{}     // 订阅文件中使用的探针服务器列表

	if username != "" && h.repo != nil {
		settings, err := h.repo.GetUserSettings(r.Context(), username)
		if err == nil {
			probeBindingEnabled = settings.EnableProbeBinding

			// 如果开启了探针绑定或流量同步，需要解析 YAML 获取节点信息
			if probeBindingEnabled || settings.SyncTraffic {
				// 解析 YAML 文件，获取其中使用的节点名称
				var yamlConfig map[string]any
				if err := yaml.Unmarshal(data, &yamlConfig); err == nil {
					if proxies, ok := yamlConfig["proxies"].([]any); ok {
						logger.Info("[Subscription] 找到订阅YAML中的代理节点", "count", len(proxies))
						// 收集所有节点名称
						usedNodeNames := make(map[string]bool)
						for _, proxy := range proxies {
							if proxyMap, ok := proxy.(map[string]any); ok {
								if name, ok := proxyMap["name"].(string); ok && name != "" {
									usedNodeNames[name] = true
								}
							}
						}

						// 如果有节点名称，从数据库查询这些节点
						if len(usedNodeNames) > 0 {
							logger.Info("[Subscription] 查询数据库中的节点", "count", len(usedNodeNames))
							nodes, err := h.repo.ListNodes(r.Context(), username)
							if err == nil {
								// 收集使用到的外部订阅名称（通过 tag 识别）
								usedExternalSubs := make(map[string]bool)

								for _, node := range nodes {
									// 检查节点是否在订阅文件中
									if usedNodeNames[node.NodeName] {
										// 检测是否为探针节点（有绑定探针服务器）
										if probeBindingEnabled && node.ProbeServer != "" {
											usesProbeNodes = true
											// 收集订阅文件中使用的探针服务器
											if usedProbeServers == nil {
												usedProbeServers = make(map[string]struct{})
											}
											usedProbeServers[node.ProbeServer] = struct{}{}
											logger.Info("[Subscription] 检测到探针节点绑定服务器", "node_name", node.NodeName, "probe_server", node.ProbeServer)
										}

										// 如果开启了流量同步，收集外部订阅节点
										if settings.SyncTraffic {
											// 如果 tag 不是默认值，说明是外部订阅节点
											if node.Tag != "" && node.Tag != "手动输入" {
												usedExternalSubs[node.Tag] = true
												logger.Info("[Subscription] 节点来自外部订阅", "node_name", node.NodeName, "tag", node.Tag)
											}
										}
									}
								}

								// 如果开启了流量同步且有使用到外部订阅的节点，汇总这些订阅的流量
								if settings.SyncTraffic && len(usedExternalSubs) > 0 {
									logger.Info("[Subscription] 用户启用流量同步，找到使用中的外部订阅", "user", username, "count", len(usedExternalSubs), "tags", getKeys(usedExternalSubs))
									externalSubs, err := h.repo.ListExternalSubscriptions(r.Context(), username)
									if err == nil {
										now := time.Now()
										for _, sub := range externalSubs {
											// 只汇总使用到的外部订阅
											if usedExternalSubs[sub.Name] {
												// 如果有过期时间且已过期，则跳过
												// 如果过期时间为空，表示长期订阅，不跳过
												if sub.Expire != nil && sub.Expire.Before(now) {
													logger.Info("[Subscription] 跳过已过期的外部订阅", "name", sub.Name, "expire", sub.Expire.Format("2006-01-02 15:04:05"))
													continue
												}
												if sub.Expire == nil {
													logger.Info("[Subscription] 添加长期外部订阅流量", "name", sub.Name, "upload", sub.Upload, "download", sub.Download, "total", sub.Total, "mode", sub.TrafficMode)
												} else {
													logger.Info("[Subscription] 添加外部订阅流量", "name", sub.Name, "upload", sub.Upload, "download", sub.Download, "total", sub.Total, "mode", sub.TrafficMode, "expire", sub.Expire.Format("2006-01-02 15:04:05"))
												}
												externalTrafficLimit += sub.Total
												// 根据 TrafficMode 计算已用流量
												switch sub.TrafficMode {
												case "download":
													externalTrafficUsed += sub.Download
												case "upload":
													externalTrafficUsed += sub.Upload
												default: // "both" 或空
													externalTrafficUsed += sub.Upload + sub.Download
												}
											}
										}
										logger.Info("[Subscription] 外部订阅流量汇总", "limit_bytes", externalTrafficLimit, "limit_gb", float64(externalTrafficLimit)/(1024*1024*1024), "used_bytes", externalTrafficUsed, "used_gb", float64(externalTrafficUsed)/(1024*1024*1024))
									} else {
										logger.Info("[Subscription] 获取外部订阅列表失败", "error", err)
									}
								} else if settings.SyncTraffic {
									logger.Info("[Subscription] 用户启用流量同步但未找到使用中的外部订阅节点", "user", username)
								}
							} else {
								logger.Info("[Subscription] 获取节点列表失败", "error", err)
							}
						}
					}
				}
			}
		}
	}

	// 获取用户的节点排序配置，需要在转换之前使用
	var nodeOrder []int64
	if username != "" && h.repo != nil {
		settings, err := h.repo.GetUserSettings(r.Context(), username)
		if err == nil {
			nodeOrder = settings.NodeOrder
			logger.Info("[Subscription] 用户节点排序配置", "user", username, "node_count", len(nodeOrder))
		}
	}

	// 在转换之前根据节点排序配置调整原始 YAML
	// 这样转换后的任何格式都会保持正确的节点顺序
	if len(nodeOrder) > 0 && username != "" && h.repo != nil {
		var yamlNode yaml.Node
		if err := yaml.Unmarshal(data, &yamlNode); err == nil {
			shouldRewrite := false
			if len(yamlNode.Content) > 0 && yamlNode.Content[0].Kind == yaml.MappingNode {
				rootMap := yamlNode.Content[0]
				for i := 0; i < len(rootMap.Content); i += 2 {
					if rootMap.Content[i].Value == "proxies" {
						proxiesNode := rootMap.Content[i+1]
						if proxiesNode.Kind == yaml.SequenceNode {
							if err := sortProxiesByNodeOrder(r.Context(), h.repo, username, proxiesNode, nodeOrder); err != nil {
								logger.Info("[Subscription] 转换前按节点顺序排序失败", "error", err)
							} else {
								shouldRewrite = true
								logger.Info("[Subscription] Successfully sorted proxies by node order before conversion")
							}
						}
						break
					}
				}
			}

			// 如果排序成功，重新序列化YAML并替换data
			if shouldRewrite {
				if reorderedData, err := MarshalYAMLWithIndent(&yamlNode); err == nil {
					fixed := RemoveUnicodeEscapeQuotes(string(reorderedData))
					data = []byte(fixed)
					logger.Info("[Subscription] Rewrote YAML data with sorted proxies")
				}
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

	// 尝试获取流量信息，如果探针报错则跳过流量统计，不影响订阅输出
	// 如果开启了探针绑定，只统计订阅文件中使用的节点绑定的探针服务器流量
	totalLimit, _, totalUsed, err := h.summary.fetchTotals(r.Context(), username, usedProbeServers)
	hasTrafficInfo := err == nil

	// 使用订阅名称
	attachmentName := url.PathEscape(displayName)

	// 对于 YAML 格式的数据，重新排序以将 rule-providers 放在最后
	// 注意：节点排序已经在转换之前完成，这里只处理其他的YAML重排需求
	if contentType == "text/yaml; charset=utf-8" || contentType == "text/yaml; charset=utf-8; charset=UTF-8" {
		// 使用 yaml.Node 来保持原始类型信息（避免 563905e2 被解析为科学计数法）
		var yamlNode yaml.Node
		if err := yaml.Unmarshal(data, &yamlNode); err == nil {
			// 检查是否有 rule-providers 需要重新排序
			// yamlNode.Content[0] 是文档节点，yamlNode.Content[0].Content 是根映射的键值对
			if len(yamlNode.Content) > 0 && yamlNode.Content[0].Kind == yaml.MappingNode {
				rootMap := yamlNode.Content[0]

				// 注意：节点排序已经在转换之前完成，这里不再重复排序
				// 只处理 WireGuard 修复和字段重排

				// 重新排序 proxies 中每个节点的字段
				for i := 0; i < len(rootMap.Content); i += 2 {
					if rootMap.Content[i].Value == "proxies" {
						proxiesNode := rootMap.Content[i+1]
						if proxiesNode.Kind == yaml.SequenceNode {
							// 先修复 WireGuard 节点的 allowed-ips 字段
							fixWireGuardAllowedIPs(proxiesNode)
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
		var finalLimit, finalUsed int64

		// 判断是否需要包含探针流量：
		// 1. 探针服务器绑定关闭时，始终包含探针流量
		// 2. 探针服务器绑定开启时，只有使用了探针节点才包含探针流量
		includeProbeTraffic := !probeBindingEnabled || usesProbeNodes

		if includeProbeTraffic && hasTrafficInfo {
			finalLimit = totalLimit + externalTrafficLimit
			finalUsed = totalUsed + externalTrafficUsed
			logger.Info("[Subscription] 最终流量统计", "user", username)
			logger.Info("[Subscription] 探针流量", "limit_bytes", totalLimit, "limit_gb", float64(totalLimit)/(1024*1024*1024), "used_bytes", totalUsed, "used_gb", float64(totalUsed)/(1024*1024*1024))
		} else {
			// 仅统计外部订阅流量
			finalLimit = externalTrafficLimit
			finalUsed = externalTrafficUsed
			logger.Info("[Subscription] 最终流量统计(仅外部订阅)", "user", username)
			logger.Info("[Subscription] 探针流量未包含(探针绑定已开启但未使用探针节点)")
		}

		logger.Info("[Subscription] 外部订阅流量", "limit_bytes", externalTrafficLimit, "limit_gb", float64(externalTrafficLimit)/(1024*1024*1024), "used_bytes", externalTrafficUsed, "used_gb", float64(externalTrafficUsed)/(1024*1024*1024))
		logger.Info("[Subscription] 总流量", "limit_bytes", finalLimit, "limit_gb", float64(finalLimit)/(1024*1024*1024), "used_bytes", finalUsed, "used_gb", float64(finalUsed)/(1024*1024*1024))

		headerValue := buildSubscriptionHeader(finalLimit, finalUsed)
		w.Header().Set("subscription-userinfo", headerValue)
		logger.Info("[Subscription] 设置订阅用户信息头", "header", headerValue)
	}
	w.Header().Set("profile-update-interval", "24")
	// 只有非浏览器访问时才添加 content-disposition 头（避免浏览器直接下载）
	userAgent := r.Header.Get("User-Agent")
	isBrowser := strings.Contains(userAgent, "Mozilla") || strings.Contains(userAgent, "Chrome") || strings.Contains(userAgent, "Safari") || strings.Contains(userAgent, "Edge")
	if !isBrowser {
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
// Also checks proxy-providers for proxy provider configs that reference external subscriptions
func getExternalSubscriptionsFromFile(ctx context.Context, data []byte, username string, repo *storage.TrafficRepository) (map[string]bool, error) {
	usedURLs := make(map[string]bool)

	// Parse YAML content
	var yamlContent map[string]any
	if err := yaml.Unmarshal(data, &yamlContent); err != nil {
		return usedURLs, fmt.Errorf("failed to parse YAML: %w", err)
	}

	// Extract proxies and query database for their raw_url
	if proxies, ok := yamlContent["proxies"].([]any); ok {
		logger.Info("[Subscription] 找到订阅文件中的代理节点", "count", len(proxies))

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
			logger.Info("[Subscription] 查询数据库获取外部订阅URL", "proxy_count", len(proxyNames))

			// Query database for nodes with these names
			nodes, err := repo.ListNodes(ctx, username)
			if err != nil {
				logger.Info("[Subscription] 查询节点列表失败", "error", err)
				return usedURLs, fmt.Errorf("failed to list nodes: %w", err)
			}

			// Find matching nodes and collect their raw_url
			for _, node := range nodes {
				if proxyNames[node.NodeName] && node.RawURL != "" {
					usedURLs[node.RawURL] = true
					logger.Info("[Subscription] 从节点找到外部订阅URL", "node_name", node.NodeName, "url", node.RawURL)
				}
			}
		}
	}

	// Also check proxy-groups for 'use' field referencing proxy provider configs
	// This handles the case where proxy-providers + use is used instead of direct proxies
	if proxyGroups, ok := yamlContent["proxy-groups"].([]any); ok {
		providerNames := make(map[string]bool)
		for _, group := range proxyGroups {
			if groupMap, ok := group.(map[string]any); ok {
				if useList, ok := groupMap["use"].([]any); ok {
					for _, use := range useList {
						if useName, ok := use.(string); ok && useName != "" {
							providerNames[useName] = true
						}
					}
				}
			}
		}

		if len(providerNames) > 0 {
			logger.Info("[Subscription] 找到代理集合引用", "count", len(providerNames))

			// Get all proxy provider configs for this user
			configs, err := repo.ListProxyProviderConfigs(ctx, username)
			if err != nil {
				logger.Info("[Subscription] 查询代理集合配置失败", "error", err)
			} else {
				// Get external subscriptions to map config -> URL
				externalSubs, err := repo.ListExternalSubscriptions(ctx, username)
				if err != nil {
					logger.Info("[Subscription] 获取外部订阅列表失败", "error", err)
				} else {
					// Build external subscription ID -> URL map
					subIDToURL := make(map[int64]string)
					for _, sub := range externalSubs {
						subIDToURL[sub.ID] = sub.URL
					}

					// Find configs that match the provider names and get their external subscription URLs
					for _, config := range configs {
						if providerNames[config.Name] {
							if url, ok := subIDToURL[config.ExternalSubscriptionID]; ok {
								usedURLs[url] = true
								logger.Info("[Subscription] 从代理集合配置找到外部订阅URL", "config_name", config.Name, "url", url)
							}
						}
					}
				}
			}
		}
	}

	logger.Info("[Subscription] 找到当前文件引用的外部订阅URL", "count", len(usedURLs))
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

	logger.Info("[Subscription] 用户需要同步的外部订阅", "user", username, "count", len(subsToSync), "match_rule", userSettings.MatchRule)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track total nodes synced
	totalNodesSynced := 0

	for _, sub := range subsToSync {
		nodeCount, updatedSub, err := syncSingleExternalSubscription(ctx, client, repo, subscribeDir, username, sub, userSettings)
		if err != nil {
			logger.Info("[Subscription] 同步订阅失败", "name", sub.Name, "url", sub.URL, "error", err)
			continue
		}

		totalNodesSynced += nodeCount

		// Update last sync time and node count
		// Use updatedSub which contains traffic info from parseAndUpdateTrafficInfo
		now := time.Now()
		updatedSub.LastSyncAt = &now
		updatedSub.NodeCount = nodeCount
		if err := repo.UpdateExternalSubscription(ctx, updatedSub); err != nil {
			logger.Info("[Subscription] 更新订阅同步时间失败", "name", sub.Name, "error", err)
		}
	}

	logger.Info("[Subscription] 同步完成", "total_nodes", totalNodesSynced, "subscription_count", len(subsToSync))

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
			logger.Info("[Token Invalid] 转换失败", "client_type", clientType, "error", err)
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

	logger.Info("[Token Invalid] 返回Token失效响应", "client_type", clientType)
}

// convertSubscription converts a YAML subscription file to the specified client format
func (h *SubscriptionHandler) convertSubscription(yamlData []byte, clientType string) ([]byte, error) {
	// 使用 yaml.Node 解析, 解决值前导零的问题
	var rootNode yaml.Node
	if err := yaml.Unmarshal(yamlData, &rootNode); err != nil {
		return nil, fmt.Errorf("failed to parse YAML: %w", err)
	}

	config, err := yamlNodeToMap(&rootNode)
	if err != nil {
		return nil, fmt.Errorf("failed to convert YAML node: %w", err)
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

	// 调用Produce方法生成转换后的节点, 传入完整配置供需要的 Producer 使用（如 Stash）
	opts := &substore.ProduceOptions{
		FullConfig: config,
	}
	result, err := producer.Produce(proxies, "", opts)
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

// fixWireGuardAllowedIPs fixes allowed-ips field type for WireGuard nodes
func fixWireGuardAllowedIPs(proxiesNode *yaml.Node) {
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return
	}

	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}

		// Check if this is a WireGuard node
		isWireGuard := false
		for i := 0; i < len(proxyNode.Content); i += 2 {
			if i+1 >= len(proxyNode.Content) {
				break
			}
			if proxyNode.Content[i].Value == "type" && proxyNode.Content[i+1].Value == "wireguard" {
				isWireGuard = true
				break
			}
		}

		if !isWireGuard {
			continue
		}

		// Fix allowed-ips field
		for i := 0; i < len(proxyNode.Content); i += 2 {
			if i+1 >= len(proxyNode.Content) {
				break
			}
			keyNode := proxyNode.Content[i]
			valueNode := proxyNode.Content[i+1]

			if keyNode.Value == "allowed-ips" {
				// If it's already a sequence node, just clear any string tags
				if valueNode.Kind == yaml.SequenceNode {
					valueNode.Tag = ""
					valueNode.Style = 0
					// Also clear tags from child nodes
					for _, childNode := range valueNode.Content {
						if childNode.Tag == "!!str" {
							childNode.Tag = ""
						}
					}
				} else if valueNode.Kind == yaml.ScalarNode {
					// If it's a scalar with !!str tag or looks like a JSON array, clear the tag
					if valueNode.Tag == "!!str" || valueNode.Tag == "tag:yaml.org,2002:str" {
						valueNode.Tag = ""
						valueNode.Style = 0
					}
				}
				break
			}
		}
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

		// Special handling for allowed-ips field to ensure it's treated as an array
		if keyNode.Value == "allowed-ips" && valueNode.Kind == yaml.ScalarNode {
			// If it's a scalar string that looks like a JSON array, mark it explicitly
			if valueNode.Tag == "!!str" || (valueNode.Style == yaml.DoubleQuotedStyle &&
				len(valueNode.Value) > 0 && valueNode.Value[0] == '[') {
				// Remove the !!str tag and let YAML infer the type
				valueNode.Tag = ""
				valueNode.Style = 0
			}
		}

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

// sortProxiesByNodeOrder 根据用户配置的节点顺序对 proxies 进行排序
// nodeOrder 是节点 ID 的数组，proxiesNode 是 YAML 中的 proxies 序列节点
func sortProxiesByNodeOrder(ctx context.Context, repo *storage.TrafficRepository, username string, proxiesNode *yaml.Node, nodeOrder []int64) error {
	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return errors.New("invalid proxies node")
	}
	
	if len(nodeOrder) == 0 || len(proxiesNode.Content) == 0 {
		return nil
	}

	// 获取用户的所有节点信息
	nodes, err := repo.ListNodes(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to list nodes: %w", err)
	}

	// 创建节点名称 -> 节点ID 的映射
	nodeNameToID := make(map[string]int64)
	for _, node := range nodes {
		nodeNameToID[node.NodeName] = node.ID
	}

	// 创建节点 ID -> 排序位置的映射
	nodeIDToPosition := make(map[int64]int)
	for pos, nodeID := range nodeOrder {
		nodeIDToPosition[nodeID] = pos
	}

	// 创建 proxy 节点的排序信息
	type proxyWithOrder struct {
		node     *yaml.Node
		position int  // 在 nodeOrder 中的位置，-1 表示不在 nodeOrder 中
		name     string
	}

	proxiesWithOrder := make([]proxyWithOrder, 0, len(proxiesNode.Content))

	// 解析每个 proxy 节点，获取其名称和排序位置
	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}

		// 查找 proxy 的 name 字段
		var proxyName string
		for i := 0; i < len(proxyNode.Content); i += 2 {
			if proxyNode.Content[i].Value == "name" {
				if i+1 < len(proxyNode.Content) {
					proxyName = proxyNode.Content[i+1].Value
				}
				break
			}
		}

		if proxyName == "" {
			// 如果没有 name 字段，保持原位置（放在最后）
			proxiesWithOrder = append(proxiesWithOrder, proxyWithOrder{
				node:     proxyNode,
				position: -1,
				name:     "",
			})
			continue
		}

		// 查找该节点名称对应的节点 ID
		nodeID, exists := nodeNameToID[proxyName]
		position := -1
		if exists {
			// 查找该节点 ID 在 nodeOrder 中的位置
			if pos, found := nodeIDToPosition[nodeID]; found {
				position = pos
			}
		}

		proxiesWithOrder = append(proxiesWithOrder, proxyWithOrder{
			node:     proxyNode,
			position: position,
			name:     proxyName,
		})
	}

	// 排序：按 position 升序排序，-1 的放在最后
	// 对于 position 相同的节点，保持原有顺序（稳定排序）
	sort.SliceStable(proxiesWithOrder, func(i, j int) bool {
		posI := proxiesWithOrder[i].position
		posJ := proxiesWithOrder[j].position

		// 如果 i 不在 nodeOrder 中，i 应该在 j 之后
		if posI == -1 {
			return false
		}
		// 如果 j 不在 nodeOrder 中，i 应该在 j 之前
		if posJ == -1 {
			return true
		}
		// 都在 nodeOrder 中，按 position 排序
		return posI < posJ
	})

	// 更新 proxiesNode 的内容
	newContent := make([]*yaml.Node, 0, len(proxiesWithOrder))
	for _, p := range proxiesWithOrder {
		newContent = append(newContent, p.node)
	}
	proxiesNode.Content = newContent

	logger.Info("[Subscription] 按节点顺序排序完成", "count", len(proxiesWithOrder), "user", username)
	return nil
}
