package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// ListNodes returns all nodes for a specific username.
func (r *TrafficRepository) ListNodes(ctx context.Context, username string) ([]Node, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	username = strings.TrimSpace(username)
	if username == "" {
		return nil, errors.New("username is required")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, COALESCE(tag, 'personal'), COALESCE(original_server, ''), COALESCE(probe_server, ''), created_at, updated_at FROM nodes WHERE username = ? ORDER BY created_at DESC`, username)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()

	var nodes []Node
	for rows.Next() {
		var node Node
		var enabled int
		if err := rows.Scan(&node.ID, &node.Username, &node.RawURL, &node.NodeName, &node.Protocol, &node.ParsedConfig, &node.ClashConfig, &enabled, &node.Tag, &node.OriginalServer, &node.ProbeServer, &node.CreatedAt, &node.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}
		node.Enabled = enabled != 0
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
	row := r.db.QueryRowContext(ctx, `SELECT id, username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, COALESCE(tag, 'personal'), COALESCE(original_server, ''), COALESCE(probe_server, ''), created_at, updated_at FROM nodes WHERE id = ? AND username = ? LIMIT 1`, id, username)
	if err := row.Scan(&node.ID, &node.Username, &node.RawURL, &node.NodeName, &node.Protocol, &node.ParsedConfig, &node.ClashConfig, &enabled, &node.Tag, &node.OriginalServer, &node.ProbeServer, &node.CreatedAt, &node.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return node, ErrNodeNotFound
		}
		return node, fmt.Errorf("get node: %w", err)
	}
	node.Enabled = enabled != 0

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

	enabled := 0
	if node.Enabled {
		enabled = 1
	}

	res, err := r.db.ExecContext(ctx, `INSERT INTO nodes (username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, tag, original_server) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, node.Username, node.RawURL, node.NodeName, node.Protocol, node.ParsedConfig, node.ClashConfig, enabled, node.Tag, node.OriginalServer)
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

	enabled := 0
	if node.Enabled {
		enabled = 1
	}

	res, err := r.db.ExecContext(ctx, `UPDATE nodes SET raw_url = ?, node_name = ?, protocol = ?, parsed_config = ?, clash_config = ?, enabled = ?, tag = ?, original_server = ?, probe_server = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND username = ?`, node.RawURL, node.NodeName, node.Protocol, node.ParsedConfig, node.ClashConfig, enabled, node.Tag, node.OriginalServer, node.ProbeServer, node.ID, node.Username)
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

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO nodes (username, raw_url, node_name, protocol, parsed_config, clash_config, enabled, tag, original_server) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return nil, fmt.Errorf("prepare insert node: %w", err)
	}
	defer stmt.Close()

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
		if node.Protocol == "" {
			return nil, fmt.Errorf("node %d: protocol is required", idx+1)
		}
		if node.Tag == "" {
			node.Tag = "手动输入"
		}

		enabled := 0
		if node.Enabled {
			enabled = 1
		}

		res, err := stmt.ExecContext(ctx, node.Username, node.RawURL, node.NodeName, node.Protocol, node.ParsedConfig, node.ClashConfig, enabled, node.Tag, node.OriginalServer)
		if err != nil {
			return nil, fmt.Errorf("insert node %d: %w", idx+1, err)
		}

		id, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("fetch node %d id: %w", idx+1, err)
		}

		createdIDs = append(createdIDs, id)
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
