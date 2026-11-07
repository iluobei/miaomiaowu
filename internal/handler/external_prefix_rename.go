package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
)

// subscriptionPrefixGroup is one airport node (by original name) and the extra
// rows that are only another display form of it.
type subscriptionPrefixGroup struct {
	keeper    storage.Node
	newName   string
	deleteIDs []int64
}

// renameExternalSubscriptionNodePrefixes rewrites this subscription's managed
// node names from appliedPrefix to desiredPrefix immediately, without a sync.
// Custom names that are not the bare airport name or a managed display form are
// left alone. After every old display form is gone, the returned prefix is
// desiredPrefix so a later sync does not stack prefixes. A partial failure
// returns appliedPrefix so the next sync can still match the old names.
func renameExternalSubscriptionNodePrefixes(ctx context.Context, repo *storage.TrafficRepository, username, subURL, appliedPrefix, desiredPrefix, subscribeDir string) (string, error) {
	if appliedPrefix == desiredPrefix || strings.TrimSpace(subURL) == "" {
		return appliedPrefix, nil
	}
	if repo == nil {
		return appliedPrefix, errors.New("traffic repository is required")
	}

	nodes, err := repo.ListNodes(ctx, username)
	if err != nil {
		return appliedPrefix, err
	}

	groups := planExternalSubscriptionPrefixRenames(nodes, subURL, appliedPrefix, desiredPrefix)
	nodeByID := make(map[int64]storage.Node, len(nodes))
	for _, node := range nodes {
		nodeByID[node.ID] = node
	}

	failed := false
	keepYAML := make(map[string]bool)
	var yamlUpdates []NodeUpdate
	var yamlDeletes []string

	for _, group := range groups {
		oldName := group.keeper.NodeName
		if oldName != group.newName {
			node := group.keeper
			node.NodeName = group.newName
			node.ClashConfig = rewriteProxyConfigName(node.ClashConfig, group.newName)
			node.ParsedConfig = rewriteProxyConfigName(node.ParsedConfig, group.newName)
			if _, err := repo.UpdateNode(ctx, node); err != nil {
				logger.Info("[外部订阅前缀] 立即重命名节点失败", "node_name", oldName, "new_name", group.newName, "id", node.ID, "error", err)
				failed = true
				continue
			}
			// YAML 里现在还是旧名字，等批量改名；不要先把这条删掉。
			keepYAML[oldName] = true
			if node.ClashConfig != "" {
				yamlUpdates = append(yamlUpdates, NodeUpdate{
					OldName:         oldName,
					NewName:         group.newName,
					ClashConfigJSON: node.ClashConfig,
				})
			}
		} else {
			keepYAML[oldName] = true
		}

		for _, id := range group.deleteIDs {
			dup, ok := nodeByID[id]
			if !ok || id == 0 {
				continue
			}
			if err := repo.DeleteNodeForSync(ctx, id, username); err != nil {
				logger.Info("[外部订阅前缀] 删除重复展示名失败", "node_name", dup.NodeName, "id", id, "error", err)
				failed = true
				keepYAML[dup.NodeName] = true
				continue
			}
			if dup.NodeName != "" && !keepYAML[dup.NodeName] {
				yamlDeletes = append(yamlDeletes, dup.NodeName)
			}
		}
	}

	if subscribeDir != "" {
		seenDelete := make(map[string]bool, len(yamlDeletes))
		for _, name := range yamlDeletes {
			if keepYAML[name] || seenDelete[name] {
				continue
			}
			seenDelete[name] = true
			if err := deleteNodeFromYAMLFiles(subscribeDir, name); err != nil {
				logger.Info("[外部订阅前缀] 从 YAML 删除重复节点失败", "node_name", name, "error", err)
			}
		}
		if len(yamlUpdates) > 0 {
			if err := batchSyncNodesToYAMLFiles(subscribeDir, yamlUpdates); err != nil {
				logger.Info("[外部订阅前缀] 批量同步节点到 YAML 失败", "count", len(yamlUpdates), "error", err)
			}
		}
	}

	if failed {
		return appliedPrefix, nil
	}

	// 还有没改掉的旧前缀时先留着，下次同步仍能对上。
	refreshed, err := repo.ListNodes(ctx, username)
	if err != nil {
		logger.Info("[外部订阅前缀] 复核节点名失败，暂不推进已应用前缀", "error", err)
		return appliedPrefix, nil
	}
	probe := storage.ExternalSubscription{URL: subURL, AppliedNamePrefix: appliedPrefix, NamePrefix: desiredPrefix}
	if untouchedAppliedPrefixRemains(refreshed, probe, nil) {
		return appliedPrefix, nil
	}
	if appliedPrefix != desiredPrefix {
		logger.Info("[外部订阅前缀] 已立即更新节点展示名", "from", appliedPrefix, "to", desiredPrefix, "groups", len(groups))
	}
	return desiredPrefix, nil
}

// planExternalSubscriptionPrefixRenames groups one subscription's nodes by the
// airport original name recovered from the prefix already written on them.
// When nothing has been applied yet, the saved names are those originals.
// The lowest ID is kept and the other display forms are removed.
func planExternalSubscriptionPrefixRenames(nodes []storage.Node, subURL, appliedPrefix, desiredPrefix string) []subscriptionPrefixGroup {
	if appliedPrefix == desiredPrefix || strings.TrimSpace(subURL) == "" {
		return nil
	}
	scoped := make([]storage.Node, 0)
	for _, node := range nodes {
		if node.RawURL == subURL {
			scoped = append(scoped, node)
		}
	}
	originals := managedOriginalsForPrefixChange(scoped, appliedPrefix, desiredPrefix)
	if len(originals) == 0 {
		return nil
	}

	grouped := make(map[string][]int)
	for i, node := range scoped {
		original, ok := lookupManagedOriginal(node.NodeName, originals, appliedPrefix, desiredPrefix)
		if !ok {
			continue
		}
		grouped[original] = append(grouped[original], i)
	}

	plans := make([]subscriptionPrefixGroup, 0, len(grouped))
	for original, idxs := range grouped {
		best := idxs[0]
		for _, i := range idxs[1:] {
			if scoped[i].ID < scoped[best].ID {
				best = i
			}
		}
		group := subscriptionPrefixGroup{
			keeper:  scoped[best],
			newName: applyNamePrefix(desiredPrefix, original),
		}
		for _, i := range idxs {
			if i == best || scoped[i].ID == 0 {
				continue
			}
			group.deleteIDs = append(group.deleteIDs, scoped[i].ID)
		}
		plans = append(plans, group)
	}
	return plans
}

// managedOriginalsForPrefixChange recovers airport original names.
// A non-empty applied prefix is stripped off names that wear it. Those originals
// also identify the bare name and the new prefix form. Custom names do not wear
// the applied prefix, so they are not originals.
// When no prefix has been written, each saved name is the bare original. A name
// that is already desiredPrefix plus one of those names is the same node.
func managedOriginalsForPrefixChange(nodes []storage.Node, appliedPrefix, desiredPrefix string) map[string]struct{} {
	originals := make(map[string]struct{})
	if strings.TrimSpace(appliedPrefix) != "" {
		for _, node := range nodes {
			if original, ok := stripExactPrefix(node.NodeName, appliedPrefix); ok {
				originals[original] = struct{}{}
			}
		}
		return originals
	}

	desiredActive := strings.TrimSpace(desiredPrefix) != ""
	var alreadyDesired []string
	for _, node := range nodes {
		if node.NodeName == "" {
			continue
		}
		if desiredActive {
			if _, ok := stripExactPrefix(node.NodeName, desiredPrefix); ok {
				alreadyDesired = append(alreadyDesired, node.NodeName)
				continue
			}
		}
		originals[node.NodeName] = struct{}{}
	}
	for _, name := range alreadyDesired {
		original, _ := stripExactPrefix(name, desiredPrefix)
		if _, known := originals[original]; known {
			continue
		}
		// 整段名字本身就是原名（或已经带上目标前缀），避免再叠一次。
		originals[name] = struct{}{}
	}
	return originals
}

func stripExactPrefix(name, prefix string) (string, bool) {
	if strings.TrimSpace(prefix) == "" || name == "" {
		return "", false
	}
	if strings.HasPrefix(name, prefix) && len(name) > len(prefix) {
		return name[len(prefix):], true
	}
	return "", false
}

func lookupManagedOriginal(localName string, originals map[string]struct{}, appliedPrefix, desiredPrefix string) (string, bool) {
	best := ""
	found := false
	for original := range originals {
		if original == "" || !shouldRenameManagedPrefix(localName, original, appliedPrefix, desiredPrefix) {
			continue
		}
		if !found || len(original) > len(best) {
			best = original
			found = true
		}
	}
	return best, found
}

func rewriteProxyConfigName(raw, name string) string {
	if strings.TrimSpace(raw) == "" {
		return raw
	}
	var cfg map[string]any
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return raw
	}
	cfg["name"] = name
	updated, err := json.Marshal(cfg)
	if err != nil {
		return raw
	}
	return string(updated)
}
