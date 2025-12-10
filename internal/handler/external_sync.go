package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

// syncExternalSubscriptionsManual is for manual sync triggered by user - syncs ALL external subscriptions regardless of ForceSyncExternal setting
func syncExternalSubscriptionsManual(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string) error {
	if repo == nil || username == "" {
		return fmt.Errorf("invalid parameters")
	}

	log.Printf("[外部订阅同步-手动] 用户 %s 开始手动同步外部订阅", username)

	// Get user settings to check match rule (but ignore ForceSyncExternal for manual sync)
	userSettings, err := repo.GetUserSettings(ctx, username)
	if err != nil {
		log.Printf("[外部订阅同步-手动] 获取用户设置失败，使用默认设置: %v", err)
		userSettings.MatchRule = "node_name"
		userSettings.SyncScope = "saved_only"
		userSettings.KeepNodeName = true
	}

	matchRuleDesc := map[string]string{
		"node_name":        "节点名称",
		"server_port":      "服务器:端口",
		"type_server_port": "类型:服务器:端口",
	}
	syncScopeDesc := map[string]string{
		"saved_only": "仅同步已保存节点",
		"all":        "同步所有节点",
	}

	log.Printf("[外部订阅同步-手动] 同步配置: 匹配规则=%s(%s), 同步范围=%s(%s), 保留节点名称=%v",
		userSettings.MatchRule, matchRuleDesc[userSettings.MatchRule],
		userSettings.SyncScope, syncScopeDesc[userSettings.SyncScope],
		userSettings.KeepNodeName)

	// Get user's external subscriptions
	externalSubs, err := repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		log.Printf("[外部订阅同步-手动] 获取外部订阅列表失败: %v", err)
		return fmt.Errorf("list external subscriptions: %w", err)
	}

	if len(externalSubs) == 0 {
		log.Printf("[外部订阅同步-手动] 用户 %s 没有配置外部订阅，跳过同步", username)
		return nil
	}

	log.Printf("[外部订阅同步-手动] 用户 %s 共有 %d 个外部订阅需要同步", username, len(externalSubs))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track total nodes synced
	totalNodesSynced := 0

	for i, sub := range externalSubs {
		log.Printf("[外部订阅同步-手动] [%d/%d] 开始同步订阅: %s", i+1, len(externalSubs), sub.Name)
		nodeCount, updatedSub, err := syncSingleExternalSubscription(ctx, client, repo, subscribeDir, username, sub, userSettings)
		if err != nil {
			log.Printf("[外部订阅同步-手动] [%d/%d] 同步订阅 %s 失败: %v", i+1, len(externalSubs), sub.Name, err)
			continue
		}

		totalNodesSynced += nodeCount

		// Update last sync time and node count
		now := time.Now()
		updatedSub.LastSyncAt = &now
		updatedSub.NodeCount = nodeCount
		if err := repo.UpdateExternalSubscription(ctx, updatedSub); err != nil {
			log.Printf("[外部订阅同步-手动] 更新订阅 %s 的同步时间失败: %v", sub.Name, err)
		}
		log.Printf("[外部订阅同步-手动] [%d/%d] 订阅 %s 同步完成，同步了 %d 个节点", i+1, len(externalSubs), sub.Name, nodeCount)
	}

	log.Printf("[外部订阅同步-手动] 用户 %s 同步完成: 共从 %d 个订阅同步了 %d 个节点", username, len(externalSubs), totalNodesSynced)

	return nil
}

// syncExternalSubscriptions fetches nodes from all external subscriptions and updates the node table
func syncExternalSubscriptions(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string) error {
	if repo == nil || username == "" {
		return fmt.Errorf("invalid parameters")
	}

	log.Printf("[外部订阅同步-自动] 用户 %s 开始自动同步外部订阅", username)

	// Get user settings to check match rule and ForceSyncExternal
	userSettings, err := repo.GetUserSettings(ctx, username)
	if err != nil {
		log.Printf("[外部订阅同步-自动] 获取用户设置失败，使用默认设置: %v", err)
		userSettings.MatchRule = "node_name"
		userSettings.SyncScope = "saved_only"
		userSettings.KeepNodeName = true
		userSettings.ForceSyncExternal = false
	}

	matchRuleDesc := map[string]string{
		"node_name":        "节点名称",
		"server_port":      "服务器:端口",
		"type_server_port": "类型:服务器:端口",
	}
	syncScopeDesc := map[string]string{
		"saved_only": "仅同步已保存节点",
		"all":        "同步所有节点",
	}

	log.Printf("[外部订阅同步-自动] 同步配置: 匹配规则=%s(%s), 同步范围=%s(%s), 保留节点名称=%v",
		userSettings.MatchRule, matchRuleDesc[userSettings.MatchRule],
		userSettings.SyncScope, syncScopeDesc[userSettings.SyncScope],
		userSettings.KeepNodeName)

	// Get user's external subscriptions
	externalSubs, err := repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		log.Printf("[外部订阅同步-自动] 获取外部订阅列表失败: %v", err)
		return fmt.Errorf("list external subscriptions: %w", err)
	}

	if len(externalSubs) == 0 {
		log.Printf("[外部订阅同步-自动] 用户 %s 没有配置外部订阅，跳过同步", username)
		return nil
	}

	// If ForceSyncExternal is enabled, only sync subscriptions used in config files
	var subsToSync []storage.ExternalSubscription
	if userSettings.ForceSyncExternal {
		log.Printf("[外部订阅同步-自动] 强制同步已开启，正在筛选配置文件中使用的订阅...")
		usedURLs, err := getUsedExternalSubscriptionURLs(ctx, repo, subscribeDir, username)
		if err != nil {
			log.Printf("[外部订阅同步-自动] 获取配置文件中使用的订阅URL失败: %v，将同步所有订阅", err)
			subsToSync = externalSubs
		} else {
			// Filter subscriptions to only those used in config files
			for _, sub := range externalSubs {
				if _, used := usedURLs[sub.URL]; used {
					subsToSync = append(subsToSync, sub)
					log.Printf("[外部订阅同步-自动] 订阅 %s 在配置文件中被使用，将进行同步", sub.Name)
				} else {
					log.Printf("[外部订阅同步-自动] 订阅 %s 未在配置文件中使用，跳过同步", sub.Name)
				}
			}
			log.Printf("[外部订阅同步-自动] 筛选完成: %d/%d 个订阅需要同步", len(subsToSync), len(externalSubs))
		}
	} else {
		subsToSync = externalSubs
	}

	if len(subsToSync) == 0 {
		log.Printf("[外部订阅同步-自动] 用户 %s 没有需要同步的订阅", username)
		return nil
	}

	log.Printf("[外部订阅同步-自动] 用户 %s 共有 %d 个外部订阅需要同步", username, len(subsToSync))

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track total nodes synced
	totalNodesSynced := 0

	for i, sub := range subsToSync {
		log.Printf("[外部订阅同步-自动] [%d/%d] 开始同步订阅: %s", i+1, len(subsToSync), sub.Name)
		nodeCount, updatedSub, err := syncSingleExternalSubscription(ctx, client, repo, subscribeDir, username, sub, userSettings)
		if err != nil {
			log.Printf("[外部订阅同步-自动] [%d/%d] 同步订阅 %s 失败: %v", i+1, len(subsToSync), sub.Name, err)
			continue
		}

		totalNodesSynced += nodeCount

		// Update last sync time and node count
		// Use updatedSub which contains traffic info from parseAndUpdateTrafficInfo
		now := time.Now()
		updatedSub.LastSyncAt = &now
		updatedSub.NodeCount = nodeCount
		if err := repo.UpdateExternalSubscription(ctx, updatedSub); err != nil {
			log.Printf("[外部订阅同步-自动] 更新订阅 %s 的同步时间失败: %v", sub.Name, err)
		}
		log.Printf("[外部订阅同步-自动] [%d/%d] 订阅 %s 同步完成，同步了 %d 个节点", i+1, len(subsToSync), sub.Name, nodeCount)
	}

	log.Printf("[外部订阅同步-自动] 用户 %s 同步完成: 共从 %d 个订阅同步了 %d 个节点", username, len(subsToSync), totalNodesSynced)

	return nil
}

// getUsedExternalSubscriptionURLs extracts all external subscription URLs used in user's subscribe files
func getUsedExternalSubscriptionURLs(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string) (map[string]bool, error) {
	usedURLs := make(map[string]bool)

	if subscribeDir == "" {
		return usedURLs, fmt.Errorf("subscribe directory not configured")
	}

	// Get all subscribe files for the user
	allFiles, err := repo.ListSubscribeFiles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list subscribe files: %w", err)
	}

	// Read each YAML file from the subscribe directory
	for _, file := range allFiles {
		// Read the YAML file from disk
		filePath := fmt.Sprintf("%s/%s", subscribeDir, file.Filename)
		content, err := os.ReadFile(filePath)
		if err != nil {
			log.Printf("[External Sync] Failed to read file %s: %v", filePath, err)
			continue
		}

		// Parse YAML content
		var yamlContent map[string]any
		if err := yaml.Unmarshal(content, &yamlContent); err != nil {
			log.Printf("[External Sync] Failed to parse YAML for file %s: %v", file.Name, err)
			continue
		}

		// Extract proxy-providers URLs
		if proxyProviders, ok := yamlContent["proxy-providers"].(map[string]any); ok {
			for _, provider := range proxyProviders {
				if providerMap, ok := provider.(map[string]any); ok {
					if url, ok := providerMap["url"].(string); ok && url != "" {
						usedURLs[url] = true
						log.Printf("[External Sync] Found used subscription URL in file %s: %s", file.Name, url)
					}
				}
			}
		}
	}

	return usedURLs, nil
}

// syncSingleExternalSubscription fetches and syncs nodes from a single external subscription
// Returns: node count, updated subscription info, error
func syncSingleExternalSubscription(ctx context.Context, client *http.Client, repo *storage.TrafficRepository, subscribeDir, username string, sub storage.ExternalSubscription, settings storage.UserSettings) (int, storage.ExternalSubscription, error) {
	matchRule := settings.MatchRule
	syncScope := settings.SyncScope
	keepNodeName := settings.KeepNodeName

	log.Printf("[外部订阅同步] 开始获取订阅 %s 的内容, URL: %s", sub.Name, sub.URL)

	// Fetch subscription content
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.URL, nil)
	if err != nil {
		log.Printf("[外部订阅同步] 创建HTTP请求失败: %v", err)
		return 0, sub, fmt.Errorf("create request: %w", err)
	}

	// 使用订阅保存的 User-Agent，如果为空则使用默认值
	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	req.Header.Set("User-Agent", userAgent)
	log.Printf("[外部订阅同步] 使用 User-Agent: %s", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[外部订阅同步] 请求订阅URL失败: %v", err)
		return 0, sub, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	log.Printf("[外部订阅同步] HTTP响应状态码: %d", resp.StatusCode)

	if resp.StatusCode != http.StatusOK {
		log.Printf("[外部订阅同步] 订阅返回非200状态码: %d", resp.StatusCode)
		return 0, sub, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse subscription-userinfo header if sync_traffic is enabled
	if settings.SyncTraffic {
		if userInfo := resp.Header.Get("subscription-userinfo"); userInfo != "" {
			log.Printf("[外部订阅同步] 发现流量信息头，开始解析...")
			parseAndUpdateTrafficInfo(ctx, repo, &sub, userInfo)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[外部订阅同步] 读取响应内容失败: %v", err)
		return 0, sub, fmt.Errorf("read response body: %w", err)
	}

	log.Printf("[外部订阅同步] 成功获取订阅内容，大小: %d 字节", len(body))

	// Parse YAML content
	var yamlContent map[string]any
	if err := yaml.Unmarshal(body, &yamlContent); err != nil {
		log.Printf("[外部订阅同步] 解析YAML内容失败: %v", err)
		return 0, sub, fmt.Errorf("parse yaml: %w", err)
	}

	// Extract proxies
	proxies, ok := yamlContent["proxies"].([]any)
	if !ok || len(proxies) == 0 {
		log.Printf("[外部订阅同步] 订阅中未找到节点(proxies)数据")
		return 0, sub, fmt.Errorf("no proxies found in subscription")
	}

	log.Printf("[外部订阅同步] 从订阅 %s 解析到 %d 个节点", sub.Name, len(proxies))

	// Convert to storage.Node format
	nodesToUpdate := make([]storage.Node, 0, len(proxies))

	for _, proxy := range proxies {
		proxyMap, ok := proxy.(map[string]any)
		if !ok {
			continue
		}

		proxyName, ok := proxyMap["name"].(string)
		if !ok || proxyName == "" {
			continue
		}

		// Marshal proxy to JSON for storage
		clashConfigBytes, err := json.Marshal(proxyMap)
		if err != nil {
			continue
		}

		// Use clash config as parsed config as well
		parsedConfigBytes := clashConfigBytes

		// Determine protocol type
		protocol := "unknown"
		if proxyType, ok := proxyMap["type"].(string); ok {
			protocol = proxyType
		}

		node := storage.Node{
			Username:     username,
			RawURL:       sub.URL, // Save external subscription URL for tracking
			NodeName:     proxyName,
			Protocol:     protocol,
			ParsedConfig: string(parsedConfigBytes),
			ClashConfig:  string(clashConfigBytes),
			Enabled:      true,
			Tag:          sub.Name, // Use external subscription name as tag
		}

		nodesToUpdate = append(nodesToUpdate, node)
	}

	if len(nodesToUpdate) == 0 {
		log.Printf("[外部订阅同步] 没有有效的节点可以同步")
		return 0, sub, fmt.Errorf("no valid nodes to sync")
	}

	log.Printf("[外部订阅同步] 准备同步 %d 个节点", len(nodesToUpdate))

	// Get existing nodes once
	existingNodes, err := repo.ListNodes(ctx, username)
	if err != nil {
		log.Printf("[外部订阅同步] 获取已保存节点列表失败: %v", err)
		return 0, sub, fmt.Errorf("list existing nodes: %w", err)
	}

	log.Printf("[外部订阅同步] 数据库中已有 %d 个节点", len(existingNodes))

	// Sync nodes to database (replace nodes based on match rule)
	syncedCount := 0
	updatedCount := 0
	createdCount := 0
	skippedCount := 0

	for _, node := range nodesToUpdate {
		var existingNode *storage.Node

		// Parse new node's clash config for matching
		var newNodeClashConfig map[string]any
		if err := json.Unmarshal([]byte(node.ClashConfig), &newNodeClashConfig); err != nil {
			continue
		}

		newServer, _ := newNodeClashConfig["server"].(string)
		newPort := newNodeClashConfig["port"]
		newType, _ := newNodeClashConfig["type"].(string)

		// Match based on rule
		switch matchRule {
		case "type_server_port":
			// Match by type:server:port
			matchKey := fmt.Sprintf("%s:%s:%v", newType, newServer, newPort)
			if newServer != "" && newPort != nil && newType != "" {
				for i := range existingNodes {
					var existingClashConfig map[string]any
					if err := json.Unmarshal([]byte(existingNodes[i].ClashConfig), &existingClashConfig); err == nil {
						existingServer, _ := existingClashConfig["server"].(string)
						existingPort := existingClashConfig["port"]
						existingType, _ := existingClashConfig["type"].(string)

						// Compare type:server:port
						if existingType == newType && existingServer == newServer && fmt.Sprintf("%v", existingPort) == fmt.Sprintf("%v", newPort) {
							existingNode = &existingNodes[i]
							log.Printf("[外部订阅同步] 节点 %s 按 type:server:port 匹配成功: %s -> 已有节点 %s", node.NodeName, matchKey, existingNode.NodeName)
							break
						}
					}
				}
				if existingNode == nil {
					log.Printf("[外部订阅同步] 节点 %s 按 type:server:port 未找到匹配: %s", node.NodeName, matchKey)
				}
			}
		case "server_port":
			// Match by server:port
			matchKey := fmt.Sprintf("%s:%v", newServer, newPort)
			if newServer != "" && newPort != nil {
				for i := range existingNodes {
					var existingClashConfig map[string]any
					if err := json.Unmarshal([]byte(existingNodes[i].ClashConfig), &existingClashConfig); err == nil {
						existingServer, _ := existingClashConfig["server"].(string)
						existingPort := existingClashConfig["port"]

						// Compare server:port
						if existingServer == newServer && fmt.Sprintf("%v", existingPort) == fmt.Sprintf("%v", newPort) {
							existingNode = &existingNodes[i]
							log.Printf("[外部订阅同步] 节点 %s 按 server:port 匹配成功: %s -> 已有节点 %s", node.NodeName, matchKey, existingNode.NodeName)
							break
						}
					}
				}
				if existingNode == nil {
					log.Printf("[外部订阅同步] 节点 %s 按 server:port 未找到匹配: %s", node.NodeName, matchKey)
				}
			}
		default:
			// Default: match by node name
			for i := range existingNodes {
				if existingNodes[i].NodeName == node.NodeName {
					existingNode = &existingNodes[i]
					log.Printf("[外部订阅同步] 节点 %s 按名称匹配成功", node.NodeName)
					break
				}
			}
			if existingNode == nil {
				log.Printf("[外部订阅同步] 节点 %s 按名称未找到匹配", node.NodeName)
			}
		}

		if existingNode != nil {
			// Update existing node
			oldNodeName := existingNode.NodeName

			// Update node fields from external subscription
			existingNode.RawURL = node.RawURL
			existingNode.Protocol = node.Protocol
			existingNode.ParsedConfig = node.ParsedConfig
			existingNode.ClashConfig = node.ClashConfig
			existingNode.Enabled = node.Enabled
			existingNode.Tag = node.Tag

			// Handle node name based on keepNodeName setting
			if !keepNodeName {
				existingNode.NodeName = node.NodeName // Update to new name from external subscription
				if oldNodeName != node.NodeName {
					log.Printf("[外部订阅同步] 更新节点名称: %s -> %s", oldNodeName, node.NodeName)
				}
			} else {
				log.Printf("[外部订阅同步] 保留原节点名称: %s (外部订阅名称: %s)", oldNodeName, node.NodeName)
			}

			_, err := repo.UpdateNode(ctx, *existingNode)
			if err != nil {
				log.Printf("[外部订阅同步] 更新节点 %s 失败: %v", existingNode.NodeName, err)
				continue
			}

			log.Printf("[外部订阅同步] 成功更新节点: %s (ID: %d)", existingNode.NodeName, existingNode.ID)

			// Sync to YAML files (handle name change if needed)
			if subscribeDir != "" {
				if err := syncNodeToYAMLFiles(subscribeDir, oldNodeName, existingNode.NodeName, existingNode.ClashConfig); err != nil {
					log.Printf("[外部订阅同步] 同步节点 %s 到YAML文件失败: %v", existingNode.NodeName, err)
				}
			}

			syncedCount++
			updatedCount++
		} else {
			// New node not found in existing nodes
			// Check sync scope: only create new nodes if syncScope is "all"
			if syncScope == "all" {
				_, err := repo.CreateNode(ctx, node)
				if err != nil {
					log.Printf("[外部订阅同步] 创建新节点 %s 失败: %v", node.NodeName, err)
					continue
				}
				log.Printf("[外部订阅同步] 成功创建新节点: %s", node.NodeName)
				syncedCount++
				createdCount++
			} else {
				log.Printf("[外部订阅同步] 跳过新节点 %s (同步范围: 仅已保存节点)", node.NodeName)
				skippedCount++
			}
		}
	}

	log.Printf("[外部订阅同步] 订阅 %s 同步完成: 总计 %d/%d 个节点 (更新: %d, 新增: %d, 跳过: %d)",
		sub.Name, syncedCount, len(nodesToUpdate), updatedCount, createdCount, skippedCount)

	return syncedCount, sub, nil
}

// parseAndUpdateTrafficInfo parses subscription-userinfo header and updates traffic info
// Format: upload=0; download=685404160; total=1073741824; expire=1705276800
func parseAndUpdateTrafficInfo(ctx context.Context, repo *storage.TrafficRepository, sub *storage.ExternalSubscription, userInfo string) {
	log.Printf("[External Sync] Parsing traffic info for subscription %s (%s)", sub.Name, sub.URL)
	log.Printf("[External Sync] Raw subscription-userinfo: %s", userInfo)

	// Parse subscription-userinfo
	// Example: upload=0; download=685404160; total=1073741824; expire=1705276800
	parts := strings.Split(userInfo, ";")

	for _, part := range parts {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}

		key := strings.TrimSpace(kv[0])
		value := strings.TrimSpace(kv[1])

		switch key {
		case "upload":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				sub.Upload = v
				log.Printf("[External Sync] Parsed upload: %d bytes (%.2f MB)", v, float64(v)/(1024*1024))
			} else {
				log.Printf("[External Sync] Failed to parse upload value '%s': %v", value, err)
			}
		case "download":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				sub.Download = v
				log.Printf("[External Sync] Parsed download: %d bytes (%.2f MB)", v, float64(v)/(1024*1024))
			} else {
				log.Printf("[External Sync] Failed to parse download value '%s': %v", value, err)
			}
		case "total":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				sub.Total = v
				log.Printf("[External Sync] Parsed total: %d bytes (%.2f GB)", v, float64(v)/(1024*1024*1024))
			} else {
				log.Printf("[External Sync] Failed to parse total value '%s': %v", value, err)
			}
		case "expire":
			if v, err := strconv.ParseInt(value, 10, 64); err == nil {
				expireTime := time.Unix(v, 0)
				sub.Expire = &expireTime
				log.Printf("[External Sync] Parsed expire: %s", expireTime.Format("2006-01-02 15:04:05"))
			} else {
				log.Printf("[External Sync] Failed to parse expire value '%s': %v", value, err)
			}
		}
	}

	// Update subscription in database
	if err := repo.UpdateExternalSubscription(ctx, *sub); err != nil {
		log.Printf("[External Sync] Failed to update traffic info for subscription %s: %v", sub.Name, err)
	} else {
		log.Printf("[External Sync] Successfully updated traffic info for subscription %s", sub.Name)
		log.Printf("[External Sync]   Upload: %d bytes (%.2f MB)", sub.Upload, float64(sub.Upload)/(1024*1024))
		log.Printf("[External Sync]   Download: %d bytes (%.2f MB)", sub.Download, float64(sub.Download)/(1024*1024))
		log.Printf("[External Sync]   Total: %d bytes (%.2f GB)", sub.Total, float64(sub.Total)/(1024*1024*1024))
		log.Printf("[External Sync]   Used: %d bytes (%.2f GB)", sub.Upload+sub.Download, float64(sub.Upload+sub.Download)/(1024*1024*1024))
		if sub.Expire != nil {
			log.Printf("[External Sync]   Expire: %s", sub.Expire.Format("2006-01-02 15:04:05"))
		}
	}
}

// SyncExternalSubscriptionsHandler is an HTTP handler for manually triggering external subscription sync
type SyncExternalSubscriptionsHandler struct {
	repo         *storage.TrafficRepository
	subscribeDir string
}

// NewSyncExternalSubscriptionsHandler creates a new handler for manual sync
func NewSyncExternalSubscriptionsHandler(repo *storage.TrafficRepository, subscribeDir string) http.Handler {
	return &SyncExternalSubscriptionsHandler{
		repo:         repo,
		subscribeDir: subscribeDir,
	}
}

func (h *SyncExternalSubscriptionsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get username from context (set by auth middleware)
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	log.Printf("[Sync API] Manual sync triggered by user: %s", username)

	// Use manual sync function which ignores ForceSyncExternal setting
	if err := syncExternalSubscriptionsManual(r.Context(), h.repo, h.subscribeDir, username); err != nil {
		log.Printf("[Sync API] Failed to sync external subscriptions for user %s: %v", username, err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{
			"error": fmt.Sprintf("同步失败: %v", err),
		})
		return
	}

	log.Printf("[Sync API] Successfully synced external subscriptions for user: %s", username)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "外部订阅同步成功",
	})
}
