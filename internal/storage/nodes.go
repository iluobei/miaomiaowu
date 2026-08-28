package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// scanNodeTags deserializes JSON tags and syncs Tag field.
func scanNodeTags(node *Node, tagsJSON string) {
	if tagsJSON != "" && tagsJSON != "[]" {
		if err := json.Unmarshal([]byte(tagsJSON), &node.Tags); err != nil {
			node.Tags = nil
		}
	}
	if len(node.Tags) > 0 && node.Tag == "" {
		node.Tag = node.Tags[0]
	}
	if node.Tag != "" && len(node.Tags) == 0 {
		node.Tags = []string{node.Tag}
	}
}

// serializeNodeTags returns JSON string for tags and syncs Tag/Tags fields.
func serializeNodeTags(node *Node) string {
	if len(node.Tags) == 0 && node.Tag != "" {
		node.Tags = []string{node.Tag}
	}
	if len(node.Tags) > 0 {
		node.Tag = node.Tags[0]
	}
	if len(node.Tags) == 0 {
		return "[]"
	}
	b, err := json.Marshal(node.Tags)
	if err != nil {
		return "[]"
	}
	return string(b)
}

func scanRelayGroupNodeIDs(node *Node, idsJSON string) {
	if idsJSON != "" && idsJSON != "[]" {
		_ = json.Unmarshal([]byte(idsJSON), &node.RelayGroupNodeIDs)
	}
}

func serializeRelayGroupNodeIDs(ids []int64) string {
	if len(ids) == 0 {
		return "[]"
	}
	b, _ := json.Marshal(ids)
	return string(b)
}

// HasAnyTag returns true if the node has at least one tag in the given set.
func (n Node) HasAnyTag(tags map[string]bool) bool {
	for _, t := range n.Tags {
		if tags[t] {
			return true
		}
	}
	return false
}

// CheckNodeNameExists checks if a node name already exists for a user (excluding a specific node ID if provided).
func (r *TrafficRepository) CheckNodeNameExists(ctx context.Context, nodeName, username string, excludeID int64) (bool, error) {
	if r == nil || r.db == nil {
		return false, errors.New("traffic repository not initialized")
	}

	nodeName = strings.TrimSpace(nodeName)
	username = strings.TrimSpace(username)
	if nodeName == "" || username == "" {
		return false, errors.New("node name and username are required")
	}

	var count int
	query := `SELECT COUNT(*) FROM nodes WHERE node_name = ? AND username = ?`
	args := []interface{}{nodeName, username}

	if excludeID > 0 {
		query += ` AND id != ?`
		args = append(args, excludeID)
	}

	err := r.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("check node name exists: %w", err)
	}

	return count > 0, nil
}

// ListNodes returns all nodes for a specific username.
func (r *TrafficRepository) ListNodes(ctx context.Context, username string) ([]Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, COALESCE(tag, 'personal'), COALESCE(original_server, ''), COALESCE(probe_server, ''), COALESCE(tags, '[]'), chain_proxy_node_id, COALESCE(relay_group_name,''), COALESCE(relay_group_node_ids,'[]'), COALESCE(probe_enabled, 0), created_at, updated_at FROM nodes WHERE username = ? ORDER BY created_at DESC`, username)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var node Node
		var enabled int
		var probeEnabled int
		var tagsJSON, relayGroupNodeIDsJSON string
		if err := rows.Scan(&node.ID, &node.Username, &node.RawURL, &node.NodeName, &node.Protocol, &node.ParsedConfig, &node.ClashConfig, &enabled, &node.Tag, &node.OriginalServer, &node.ProbeServer, &tagsJSON, &node.ChainProxyNodeID, &node.RelayGroupName, &relayGroupNodeIDsJSON, &probeEnabled, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		node.Enabled = enabled != 0
		node.ProbeEnabled = probeEnabled != 0
		scanNodeTags(&node, tagsJSON)
		scanRelayGroupNodeIDs(&node, relayGroupNodeIDsJSON)
		nodes = append(nodes, node)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate nodes: %w", err)
	}

	return nodes, nil
}

// GetNode retrieves a single node by ID and username.
func (r *TrafficRepository) GetNode(ctx context.Context, id int64, username string) (Node, error) {
	var node Node
	if r == nil || r.db == nil {
		return node, errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return node, errors.New("node id is required")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return node, errors.New("username is required")
	}

	var enabled int
	var probeEnabled int
	var tagsJSON, relayGroupNodeIDsJSON string
	row := r.db.QueryRowContext(ctx, `SELECT id, username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, COALESCE(tag, 'personal'), COALESCE(original_server, ''), COALESCE(probe_server, ''), COALESCE(tags, '[]'), chain_proxy_node_id, COALESCE(relay_group_name,''), COALESCE(relay_group_node_ids,'[]'), COALESCE(probe_enabled, 0), created_at, updated_at FROM nodes WHERE id = ? AND username = ? LIMIT 1`, id, username)
	if err := row.Scan(&node.ID, &node.Username, &node.RawURL, &node.NodeName, &node.Protocol, &node.ParsedConfig, &node.ClashConfig, &enabled, &node.Tag, &node.OriginalServer, &node.ProbeServer, &tagsJSON, &node.ChainProxyNodeID, &node.RelayGroupName, &relayGroupNodeIDsJSON, &probeEnabled, &node.CreatedAt, &node.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return node, ErrNodeNotFound
		}
		return node, fmt.Errorf("get node: %w", err)
	}
	node.Enabled = enabled != 0
	node.ProbeEnabled = probeEnabled != 0
	scanNodeTags(&node, tagsJSON)
	scanRelayGroupNodeIDs(&node, relayGroupNodeIDsJSON)

	return node, nil
}

// CreateNode inserts a new proxy node.
func (r *TrafficRepository) CreateNode(ctx context.Context, node Node) (Node, error) {
	if r == nil || r.db == nil {
		return Node{}, errors.New("traffic repository not initialized")
	}

	node.Username = strings.TrimSpace(node.Username)
	node.RawURL = strings.TrimSpace(node.RawURL)
	node.NodeName = strings.TrimSpace(node.NodeName)
	node.Protocol = strings.ToLower(strings.TrimSpace(node.Protocol))
	node.Tag = strings.TrimSpace(node.Tag)

	if node.Username == "" {
		return Node{}, errors.New("username is required")
	}
	// RawURL 可以为空（Clash 订阅节点），但 ClashConfig 必须存在
	if node.RawURL == "" && node.ClashConfig == "" {
		return Node{}, errors.New("raw URL or clash config is required")
	}
	if node.NodeName == "" {
		return Node{}, errors.New("node name is required")
	}
	if node.Protocol == "" {
		return Node{}, errors.New("protocol is required")
	}
	if node.Tag == "" {
		node.Tag = "手动输入"
	}

	tagsJSON := serializeNodeTags(&node)

	enabled := 0
	if node.Enabled {
		enabled = 1
	}

	// 互斥：中转组和单链式代理不能同时存在
	if len(node.RelayGroupNodeIDs) > 0 {
		node.ChainProxyNodeID = nil
	}
	if node.ChainProxyNodeID != nil {
		node.RelayGroupName = ""
		node.RelayGroupNodeIDs = nil
	}
	relayGroupNodeIDsJSON := serializeRelayGroupNodeIDs(node.RelayGroupNodeIDs)

	res, err := r.db.ExecContext(ctx, `INSERT INTO nodes (username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, tag, tags, original_server, chain_proxy_node_id, relay_group_name, relay_group_node_ids) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, node.Username, node.RawURL, node.NodeName, node.Protocol, node.ParsedConfig, node.ClashConfig, enabled, node.Tag, tagsJSON, node.OriginalServer, node.ChainProxyNodeID, node.RelayGroupName, relayGroupNodeIDsJSON)
	if err != nil {
		return Node{}, fmt.Errorf("create node: %w", err)
	}

	id, err := res.LastInsertId()
	if err != nil {
		return Node{}, fmt.Errorf("fetch node id: %w", err)
	}

	return r.GetNode(ctx, id, node.Username)
}

// UpdateNode updates an existing proxy node.
func (r *TrafficRepository) UpdateNode(ctx context.Context, node Node) (Node, error) {
	if r == nil || r.db == nil {
		return Node{}, errors.New("traffic repository not initialized")
	}

	if node.ID <= 0 {
		return Node{}, errors.New("node id is required")
	}

	node.Username = strings.TrimSpace(node.Username)
	node.RawURL = strings.TrimSpace(node.RawURL)
	node.NodeName = strings.TrimSpace(node.NodeName)
	node.Protocol = strings.ToLower(strings.TrimSpace(node.Protocol))
	node.Tag = strings.TrimSpace(node.Tag)

	if node.Username == "" {
		return Node{}, errors.New("username is required")
	}
	// RawURL 可以为空（Clash 订阅节点），但 ClashConfig 必须存在
	if node.RawURL == "" && node.ClashConfig == "" {
		return Node{}, errors.New("raw URL or clash config is required")
	}
	if node.NodeName == "" {
		return Node{}, errors.New("node name is required")
	}
	if node.Protocol == "" {
		return Node{}, errors.New("protocol is required")
	}
	if node.Tag == "" {
		node.Tag = "手动输入"
	}

	tagsJSON := serializeNodeTags(&node)

	enabled := 0
	if node.Enabled {
		enabled = 1
	}

	// 互斥：中转组和单链式代理不能同时存在
	if len(node.RelayGroupNodeIDs) > 0 {
		node.ChainProxyNodeID = nil
	}
	if node.ChainProxyNodeID != nil {
		node.RelayGroupName = ""
		node.RelayGroupNodeIDs = nil
	}
	relayGroupNodeIDsJSON := serializeRelayGroupNodeIDs(node.RelayGroupNodeIDs)

	res, err := r.db.ExecContext(ctx, `UPDATE nodes SET raw_url = ?, node_name = ?, protocol = ?, parsed_config = ?, clash_config = ?, enabled = ?, tag = ?, tags = ?, original_server = ?, probe_server = ?, chain_proxy_node_id = ?, relay_group_name = ?, relay_group_node_ids = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`, node.RawURL, node.NodeName, node.Protocol, node.ParsedConfig, node.ClashConfig, enabled, node.Tag, tagsJSON, node.OriginalServer, node.ProbeServer, node.ChainProxyNodeID, node.RelayGroupName, relayGroupNodeIDsJSON, node.ID, node.Username)
	if err != nil {
		return Node{}, fmt.Errorf("update node: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return Node{}, fmt.Errorf("node update rows affected: %w", err)
	}
	if affected == 0 {
		return Node{}, ErrNodeNotFound
	}

	return r.GetNode(ctx, node.ID, node.Username)
}

// DeleteNode removes a proxy node.
func (r *TrafficRepository) DeleteNode(ctx context.Context, id int64, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return errors.New("node id is required")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	// 获取节点的 raw_url，用于后续检查外部订阅
	var rawURL string
	err := r.db.QueryRowContext(ctx, `SELECT raw_url FROM nodes WHERE id = ? AND username = ?`, id, username).Scan(&rawURL)
	if err != nil {
		if err == sql.ErrNoRows {
			return ErrNodeNotFound
		}
		return fmt.Errorf("get node raw_url: %w", err)
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ? AND username = ?`, id, username)
	if err != nil {
		return fmt.Errorf("delete node: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("node delete rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNodeNotFound
	}

	// 清除引用了该节点作为中转节点的 chain_proxy_node_id
	_, _ = r.db.ExecContext(ctx, `UPDATE nodes SET chain_proxy_node_id = NULL WHERE chain_proxy_node_id = ? AND username = ?`, id, username)

	// 从其他节点的中转组成员中移除该节点；若中转组因此为空，清除整个中转组配置
	r.pruneRelayGroupMember(ctx, id, username)

	// 检查该 raw_url 是否还有其他节点使用
	// 如果没有，则删除对应的外部订阅及其关联的代理集合配置
	if rawURL != "" {
		var count int
		err = r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM nodes WHERE username = ? AND raw_url = ?`, username, rawURL).Scan(&count)
		if err != nil {
			// 记录错误但不影响删除节点的操作
			return nil
		}

		// 如果没有节点使用该订阅链接，删除外部订阅
		if count == 0 {
			// 首先获取外部订阅的 ID
			var subID int64
			err = r.db.QueryRowContext(ctx, `SELECT id FROM external_subscriptions WHERE username = ? AND url = ?`, username, rawURL).Scan(&subID)
			if err == nil && subID > 0 {
				// 删除关联的代理集合配置
				_, _ = r.db.ExecContext(ctx, `DELETE FROM proxy_provider_configs WHERE external_subscription_id = ?`, subID)
			}
			// 删除外部订阅
			_, err = r.db.ExecContext(ctx, `DELETE FROM external_subscriptions WHERE username = ? AND url = ?`, username, rawURL)
			if err != nil {
				// 记录错误但不影响删除节点的操作
				// 可以在这里添加日志记录
			}
		}
	}

	return nil
}

// pruneRelayGroupMember 在删除节点后，从同一用户其他节点的中转组成员列表中移除该节点 ID。
// 若移除后某节点的中转组成员为空，则直接清除其整个中转组配置（relay_group_name + relay_group_node_ids），
// 使该节点在生成订阅时回退为普通节点，避免悬空的中转组引用。
func (r *TrafficRepository) pruneRelayGroupMember(ctx context.Context, deletedID int64, username string) {
	rows, err := r.db.QueryContext(ctx, `SELECT id, relay_group_node_ids FROM nodes WHERE username = ? AND relay_group_node_ids != '' AND relay_group_node_ids != '[]'`, username)
	if err != nil {
		return
	}
	type pending struct {
		id  int64
		ids []int64
	}
	var updates []pending
	for rows.Next() {
		var nid int64
		var idsJSON string
		if err := rows.Scan(&nid, &idsJSON); err != nil {
			continue
		}
		var ids []int64
		if err := json.Unmarshal([]byte(idsJSON), &ids); err != nil {
			continue
		}
		var filtered []int64
		removed := false
		for _, x := range ids {
			if x == deletedID {
				removed = true
				continue
			}
			filtered = append(filtered, x)
		}
		if removed {
			updates = append(updates, pending{id: nid, ids: filtered})
		}
	}
	rows.Close()

	for _, u := range updates {
		if len(u.ids) == 0 {
			// 中转组成员已全部删除：清除整个中转组配置
			_, _ = r.db.ExecContext(ctx, `UPDATE nodes SET relay_group_name = '', relay_group_node_ids = '[]', updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`, u.id, username)
		} else {
			_, _ = r.db.ExecContext(ctx, `UPDATE nodes SET relay_group_node_ids = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`, serializeRelayGroupNodeIDs(u.ids), u.id, username)
		}
	}
}

// DeleteNodeForSync removes a node without triggering external subscription cleanup.
// This is intended for internal sync workflows that need to prune nodes safely.
func (r *TrafficRepository) DeleteNodeForSync(ctx context.Context, id int64, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return errors.New("node id is required")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ? AND username = ?`, id, username)
	if err != nil {
		return fmt.Errorf("delete node for sync: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("node delete for sync rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNodeNotFound
	}

	_, _ = r.db.ExecContext(ctx, `UPDATE nodes SET chain_proxy_node_id = NULL WHERE chain_proxy_node_id = ? AND username = ?`, id, username)

	return nil
}

// BatchCreateNodes creates multiple nodes in a single transaction.
func (r *TrafficRepository) BatchCreateNodes(ctx context.Context, nodes []Node) ([]Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	if len(nodes) == 0 {
		return nil, errors.New("nodes list is empty")
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin batch create nodes tx: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO nodes (username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, tag, tags, original_server, chain_proxy_node_id, relay_group_name, relay_group_node_ids) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare insert node: %w", err)
	}
	defer stmt.Close()

	takenByUser := make(map[string]map[string]bool)
	for _, node := range nodes {
		username := strings.TrimSpace(node.Username)
		if username == "" || takenByUser[username] != nil {
			continue
		}
		taken := make(map[string]bool)
		rows, queryErr := tx.QueryContext(ctx, `SELECT node_name FROM nodes WHERE username = ?`, username)
		if queryErr != nil {
			return nil, fmt.Errorf("list existing node names: %w", queryErr)
		}
		for rows.Next() {
			var name string
			if scanErr := rows.Scan(&name); scanErr != nil {
				rows.Close()
				return nil, fmt.Errorf("scan existing node name: %w", scanErr)
			}
			taken[name] = true
		}
		if rowsErr := rows.Err(); rowsErr != nil {
			rows.Close()
			return nil, fmt.Errorf("list existing node names: %w", rowsErr)
		}
		rows.Close()
		takenByUser[username] = taken
	}

	var createdIDs []int64
	for idx, node := range nodes {
		node.Username = strings.TrimSpace(node.Username)
		node.RawURL = strings.TrimSpace(node.RawURL)
		node.NodeName = strings.TrimSpace(node.NodeName)
		node.Protocol = strings.ToLower(strings.TrimSpace(node.Protocol))
		node.Tag = strings.TrimSpace(node.Tag)

		if node.Username == "" {
			return nil, fmt.Errorf("node %d: username is required", idx+1)
		}
		// RawURL 可以为空（Clash 订阅节点），但 ClashConfig 必须存在
		if node.RawURL == "" && node.ClashConfig == "" {
			return nil, fmt.Errorf("node %d: raw URL or clash config is required", idx+1)
		}
		if node.NodeName == "" {
			return nil, fmt.Errorf("node %d: node name is required", idx+1)
		}
		taken := takenByUser[node.Username]
		originalName := node.NodeName
		node.NodeName = uniqueImportedNodeName(node.NodeName, taken)
		if node.NodeName != originalName {
			node.ClashConfig = replaceJSONNodeName(node.ClashConfig, node.NodeName)
			node.ParsedConfig = replaceJSONNodeName(node.ParsedConfig, node.NodeName)
		}
		taken[node.NodeName] = true
		if node.Protocol == "" {
			return nil, fmt.Errorf("node %d: protocol is required", idx+1)
		}
		if node.Tag == "" {
			node.Tag = "手动输入"
		}

		tagsJSON := serializeNodeTags(&node)

		enabled := 0
		if node.Enabled {
			enabled = 1
		}

		relayGroupNodeIDsJSON := serializeRelayGroupNodeIDs(node.RelayGroupNodeIDs)
		res, err := stmt.ExecContext(ctx, node.Username, node.RawURL, node.NodeName, node.Protocol, node.ParsedConfig, node.ClashConfig, enabled, node.Tag, tagsJSON, node.OriginalServer, node.ChainProxyNodeID, node.RelayGroupName, relayGroupNodeIDsJSON)
		if err != nil {
			return nil, fmt.Errorf("insert node %d: %w", idx+1, err)
		}

		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("fetch node %d id: %w", idx+1, err)
		}

		createdIDs = append(createdIDs, id)
		nodes[idx] = node
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit batch create nodes: %w", err)
	}

	// Fetch created nodes
	var created []Node
	for i, id := range createdIDs {
		node, err := r.GetNode(ctx, id, nodes[i].Username)
		if err != nil {
			return nil, fmt.Errorf("fetch created node %d: %w", i+1, err)
		}
		created = append(created, node)
	}

	return created, nil
}

func uniqueImportedNodeName(base string, taken map[string]bool) string {
	if !taken[base] {
		return base
	}
	for suffix := 2; ; suffix++ {
		candidate := fmt.Sprintf("%s-%d", base, suffix)
		if !taken[candidate] {
			return candidate
		}
	}
}

func replaceJSONNodeName(config, name string) string {
	if strings.TrimSpace(config) == "" {
		return config
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(config), &parsed); err != nil {
		return config
	}
	parsed["name"] = name
	updated, err := json.Marshal(parsed)
	if err != nil {
		return config
	}
	return string(updated)
}

// DeleteAllUserNodes removes all nodes for a specific user.
func (r *TrafficRepository) DeleteAllUserNodes(ctx context.Context, username string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	_, err := r.db.ExecContext(ctx, `DELETE FROM nodes WHERE username = ?`, username)
	if err != nil {
		return fmt.Errorf("delete all user nodes: %w", err)
	}

	return nil
}

// UpdateNodeProbeServer updates the probe server binding for a node.
func (r *TrafficRepository) UpdateNodeProbeServer(ctx context.Context, nodeID int64, username, probeServer string) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if nodeID <= 0 {
		return errors.New("node id is required")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return errors.New("username is required")
	}

	probeServer = strings.TrimSpace(probeServer)

	res, err := r.db.ExecContext(ctx, `UPDATE nodes SET probe_server = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`, probeServer, nodeID, username)
	if err != nil {
		return fmt.Errorf("update node probe server: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("node probe server update rows affected: %w", err)
	}
	if affected == 0 {
		return ErrNodeNotFound
	}

	return nil
}

// migrateChainProxyNodes migrates legacy chain proxy nodes that store dialer-proxy in clash_config.
// It resolves the target node by name, sets chain_proxy_node_id, and removes dialer-proxy from clash_config.
func (r *TrafficRepository) migrateChainProxyNodes() {
	rows, err := r.db.Query(`SELECT id, username, node_name, clash_config FROM nodes WHERE chain_proxy_node_id IS NULL AND clash_config LIKE '%dialer-proxy%'`)
	if err != nil {
		return
	}
	defer rows.Close()

	type migrationItem struct {
		id          int64
		username    string
		nodeName    string
		clashConfig string
	}

	var items []migrationItem
	for rows.Next() {
		var item migrationItem
		if err := rows.Scan(&item.id, &item.username, &item.nodeName, &item.clashConfig); err != nil {
			continue
		}
		items = append(items, item)
	}

	for _, item := range items {
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(item.clashConfig), &config); err != nil {
			continue
		}

		dialerProxy, ok := config["dialer-proxy"].(string)
		if !ok || dialerProxy == "" {
			continue
		}

		// Find target node by name within the same user
		var targetID int64
		err := r.db.QueryRow(`SELECT id FROM nodes WHERE username = ? AND node_name = ? AND id != ? LIMIT 1`, item.username, dialerProxy, item.id).Scan(&targetID)
		if err != nil {
			continue
		}

		// Remove dialer-proxy from clash_config
		delete(config, "dialer-proxy")
		newConfig, err := json.Marshal(config)
		if err != nil {
			continue
		}

		// Update the node
		r.db.Exec(`UPDATE nodes SET chain_proxy_node_id = ?, clash_config = ?, parsed_config = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, targetID, string(newConfig), string(newConfig), item.id)
	}
}
