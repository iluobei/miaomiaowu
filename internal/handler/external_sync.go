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

	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

// syncExternalSubscriptions fetches nodes from all external subscriptions and updates the node table
func syncExternalSubscriptions(ctx context.Context, repo *storage.TrafficRepository, subscribeDir, username string) error {
	if repo == nil || username == "" {
		return fmt.Errorf("invalid parameters")
	}

	// Get user settings to check match rule and ForceSyncExternal
	userSettings, err := repo.GetUserSettings(ctx, username)
	if err != nil {
		// If settings not found, use default match rule
		userSettings.MatchRule = "node_name"
		userSettings.ForceSyncExternal = false
	}

	// Get user's external subscriptions
	externalSubs, err := repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		return fmt.Errorf("list external subscriptions: %w", err)
	}

	if len(externalSubs) == 0 {
		// No external subscriptions, nothing to sync
		return nil
	}

	// If ForceSyncExternal is enabled, only sync subscriptions used in config files
	var subsToSync []storage.ExternalSubscription
	if userSettings.ForceSyncExternal {
		usedURLs, err := getUsedExternalSubscriptionURLs(ctx, repo, subscribeDir, username)
		if err != nil {
			log.Printf("[External Sync] Failed to get used subscription URLs: %v, falling back to sync all", err)
			subsToSync = externalSubs
		} else {
			// Filter subscriptions to only those used in config files
			for _, sub := range externalSubs {
				if _, used := usedURLs[sub.URL]; used {
					subsToSync = append(subsToSync, sub)
				}
			}
			log.Printf("[External Sync] ForceSyncExternal enabled: syncing %d/%d subscriptions actually used in config files", len(subsToSync), len(externalSubs))
		}
	} else {
		subsToSync = externalSubs
	}

	if len(subsToSync) == 0 {
		log.Printf("[External Sync] No subscriptions to sync for user %s", username)
		return nil
	}

	log.Printf("[External Sync] User %s has %d external subscriptions to sync (match rule: %s)", username, len(subsToSync), userSettings.MatchRule)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Track total nodes synced
	totalNodesSynced := 0

	for _, sub := range subsToSync {
		nodeCount, updatedSub, err := syncSingleExternalSubscription(ctx, client, repo, subscribeDir, username, sub, userSettings.MatchRule)
		if err != nil {
			log.Printf("[External Sync] Failed to sync subscription %s (%s): %v", sub.Name, sub.URL, err)
			continue
		}

		totalNodesSynced += nodeCount

		// Update last sync time and node count
		// Use updatedSub which contains traffic info from parseAndUpdateTrafficInfo
		now := time.Now()
		updatedSub.LastSyncAt = &now
		updatedSub.NodeCount = nodeCount
		if err := repo.UpdateExternalSubscription(ctx, updatedSub); err != nil {
			log.Printf("[External Sync] Failed to update sync time for subscription %s: %v", sub.Name, err)
		}
	}

	log.Printf("[External Sync] Completed: synced %d nodes total from %d subscriptions", totalNodesSynced, len(subsToSync))

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
func syncSingleExternalSubscription(ctx context.Context, client *http.Client, repo *storage.TrafficRepository, subscribeDir, username string, sub storage.ExternalSubscription, matchRule string) (int, storage.ExternalSubscription, error) {
	// Fetch subscription content
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, sub.URL, nil)
	if err != nil {
		return 0, sub, fmt.Errorf("create request: %w", err)
	}

	// 使用订阅保存的 User-Agent，如果为空则使用默认值
	userAgent := sub.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	if err != nil {
		return 0, sub, fmt.Errorf("fetch subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, sub, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	// Parse subscription-userinfo header if sync_traffic is enabled
	userSettings, err := repo.GetUserSettings(ctx, username)
	if err == nil && userSettings.SyncTraffic {
		if userInfo := resp.Header.Get("subscription-userinfo"); userInfo != "" {
			parseAndUpdateTrafficInfo(ctx, repo, &sub, userInfo)
		}
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, sub, fmt.Errorf("read response body: %w", err)
	}

	// Parse YAML content
	var yamlContent map[string]any
	if err := yaml.Unmarshal(body, &yamlContent); err != nil {
		return 0, sub, fmt.Errorf("parse yaml: %w", err)
	}

	// Extract proxies
	proxies, ok := yamlContent["proxies"].([]any)
	if !ok || len(proxies) == 0 {
		return 0, sub, fmt.Errorf("no proxies found in subscription")
	}

	log.Printf("[External Sync] Fetched %d proxies from %s", len(proxies), sub.Name)

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
		return 0, sub, fmt.Errorf("no valid nodes to sync")
	}

	// Sync nodes to database (replace nodes based on match rule)
	syncedCount := 0
	for _, node := range nodesToUpdate {
		// Get existing nodes
		existingNodes, err := repo.ListNodes(ctx, username)
		if err != nil {
			continue
		}

		var existingNode *storage.Node

		// Match based on rule
		if matchRule == "server_port" {
			// Match by server:port
			var newNodeClashConfig map[string]any
			if err := json.Unmarshal([]byte(node.ClashConfig), &newNodeClashConfig); err == nil {
				newServer, newServerOk := newNodeClashConfig["server"].(string)
				newPort, newPortOk := newNodeClashConfig["port"]

				if newServerOk && newPortOk {
					for i := range existingNodes {
						var existingClashConfig map[string]any
						if err := json.Unmarshal([]byte(existingNodes[i].ClashConfig), &existingClashConfig); err == nil {
							existingServer, existingServerOk := existingClashConfig["server"].(string)
							existingPort, existingPortOk := existingClashConfig["port"]

							if existingServerOk && existingPortOk {
								// Compare server:port
								if existingServer == newServer && fmt.Sprintf("%v", existingPort) == fmt.Sprintf("%v", newPort) {
									existingNode = &existingNodes[i]
									break
								}
							}
						}
					}
				}
			}
		} else {
			// Default: match by node name
			for i := range existingNodes {
				if existingNodes[i].NodeName == node.NodeName {
					existingNode = &existingNodes[i]
					break
				}
			}
		}

		if existingNode != nil {
			// Update existing node
			oldNodeName := existingNode.NodeName

			// Update all node fields from external subscription
			existingNode.RawURL = node.RawURL
			existingNode.NodeName = node.NodeName // Always update to new name
			existingNode.Protocol = node.Protocol
			existingNode.ParsedConfig = node.ParsedConfig
			existingNode.ClashConfig = node.ClashConfig
			existingNode.Enabled = node.Enabled
			existingNode.Tag = node.Tag

			_, err := repo.UpdateNode(ctx, *existingNode)
			if err != nil {
				log.Printf("[External Sync] Failed to update node %s: %v", node.NodeName, err)
				continue
			}

			// Sync to YAML files (handle name change if needed)
			if subscribeDir != "" {
				if err := syncNodeToYAMLFiles(subscribeDir, oldNodeName, existingNode.NodeName, existingNode.ClashConfig); err != nil {
					log.Printf("[External Sync] Failed to sync node %s to YAML: %v", node.NodeName, err)
				}
			}

			syncedCount++
		} else {
			// Create new node
			_, err := repo.CreateNode(ctx, node)
			if err != nil {
				log.Printf("[External Sync] Failed to create node %s: %v", node.NodeName, err)
				continue
			}

			syncedCount++
		}
	}

	log.Printf("[External Sync] Synced %d/%d nodes from subscription %s", syncedCount, len(nodesToUpdate), sub.Name)

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
