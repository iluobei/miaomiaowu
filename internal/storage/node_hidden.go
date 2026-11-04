package storage

import (
	"context"
	"strings"
)

// 节点「隐藏」只是节点管理列表的显示开关(管理员整理列表用),不影响订阅与下发。
// 单独查询而不进 ListNodes 的列清单:列表接口照常返回全部节点,前端按这份 ID 过滤。

// ListHiddenNodeIDs 返回被隐藏的节点 ID。
func (r *TrafficRepository) ListHiddenNodeIDs(ctx context.Context) ([]int64, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT id FROM nodes WHERE COALESCE(hidden, 0) = 1 ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// SetNodesHidden 批量设置隐藏 / 取消隐藏,返回实际改动的行数。
func (r *TrafficRepository) SetNodesHidden(ctx context.Context, ids []int64, hidden bool) (int64, error) {
	if len(ids) == 0 {
		return 0, nil
	}
	val := 0
	if hidden {
		val = 1
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, val)
	ph := make([]string, len(ids))
	for i, id := range ids {
		ph[i] = "?"
		args = append(args, id)
	}
	res, err := r.db.ExecContext(ctx, `UPDATE nodes SET hidden = ? WHERE id IN (`+strings.Join(ph, ",")+`)`, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
