package storage

import (
	"context"
	"errors"
	"fmt"
)

// 外部节点连通性探测的数据层。
//
// 探测本身很重 —— 每个节点要起一个 mihomo 进程真连一次 —— 所以不对全部导入节点跑,
// 由用户在节点列表逐个勾选(probe_enabled)。这里只提供「取待探测清单」和「改开关」,
// 探测结果不落库:默认存内存 ring(见 handler 侧),避免重蹈 forward_hop_metrics 的覆辙
// (分钟级时序把 SQLite 的 WAL 顶到几十 GB,而主库才十几 MB)。

// ProbeTargetNode 一个待探测节点的最小信息。
// 刻意不返回整个 Node:探测只需要这几样,而 clash_config 可能很大,
// 一轮探测几百个节点时没必要把无关字段全读进内存。
type ProbeTargetNode struct {
	ID       int64
	NodeName string
	Protocol string
	// Username 节点归属用户。「掉线自动重新同步外部订阅」按用户维度同步
	// (syncExternalSubscriptions 就是按 username 拉该用户的全部外部订阅)。
	Username    string
	ClashConfig string
}

// ListProbeEnabledNodes 取所有勾选了探测的节点。
//
// 不按 original_server 过滤 —— 妙妙屋X 那边用 original_server=” 筛「外部节点」,
// 因为它那里该列存的是自建服务器名。**本项目里 original_server 语义完全不同**:
// 它是「手动改节点服务器地址时备份的旧地址」(见 handler/nodes.go handleUpdateServer),
// 照抄过来会把所有手动改过地址的节点莫名排除掉。
//
// 而且本项目没有「自建服务器」这个概念,节点全都是导入来的,本来就都该可探测。
// 真正的开关是用户逐个勾选的 probe_enabled。
func (r *TrafficRepository) ListProbeEnabledNodes(ctx context.Context) ([]ProbeTargetNode, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, node_name, protocol, username, clash_config
		FROM nodes
		WHERE COALESCE(probe_enabled, 0) = 1
		  AND enabled = 1
		ORDER BY id ASC`)
	if err != nil {
		return nil, fmt.Errorf("list probe-enabled nodes: %w", err)
	}
	defer rows.Close()

	var out []ProbeTargetNode
	for rows.Next() {
		var n ProbeTargetNode
		if err := rows.Scan(&n.ID, &n.NodeName, &n.Protocol, &n.Username, &n.ClashConfig); err != nil {
			return nil, fmt.Errorf("scan probe target: %w", err)
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// SetNodeProbeEnabled 开关某个节点的探测。
func (r *TrafficRepository) SetNodeProbeEnabled(ctx context.Context, nodeID int64, enabled bool) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	v := 0
	if enabled {
		v = 1
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE nodes SET probe_enabled = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`, v, nodeID)
	if err != nil {
		return fmt.Errorf("set node probe_enabled: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return fmt.Errorf("节点 %d 不存在", nodeID)
	}
	return nil
}

// CountProbeEnabledNodes 供前端展示"已开启探测 N 个",也用于调度器空转时提前返回。
func (r *TrafficRepository) CountProbeEnabledNodes(ctx context.Context) (int, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}
	var n int
	err := r.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM nodes
		WHERE COALESCE(probe_enabled, 0) = 1 AND enabled = 1
`).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("count probe-enabled nodes: %w", err)
	}
	return n, nil
}
