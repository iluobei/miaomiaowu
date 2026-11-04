package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// 推送渠道:飞书 / 钉钉 / Server酱 / Bark / 邮件 / 额外的 Telegram 等。
//
// 用独立表而不是一项 JSON 设置:发送结果(last_error / last_ok_at)由各渠道的发送 goroutine
// 各自回写,JSON 整份读改写会互相覆盖;一行一渠道就只改自己那行。
// 内置 Telegram(system_config 里的 token / chat_id)不进这张表,原配置原样生效。

type NotifyChannel struct {
	ID          int64             `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Params      map[string]string `json:"params"`
	Events      []string          `json:"events"`
	LastError   string            `json:"last_error"`
	LastErrorAt int64             `json:"last_error_at"`
	LastOKAt    int64             `json:"last_ok_at"`
	CreatedAt   time.Time         `json:"created_at"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

const notifyChannelsSchema = `
CREATE TABLE IF NOT EXISTS notify_channels (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    type TEXT NOT NULL,
    name TEXT NOT NULL DEFAULT '',
    enabled INTEGER NOT NULL DEFAULT 0,
    params TEXT NOT NULL DEFAULT '{}',
    events TEXT NOT NULL DEFAULT '[]',
    last_error TEXT NOT NULL DEFAULT '',
    last_error_at INTEGER NOT NULL DEFAULT 0,
    last_ok_at INTEGER NOT NULL DEFAULT 0,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP
);`

func (r *TrafficRepository) ensureNotifyChannelsTable(ctx context.Context) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}
	_, err := r.db.ExecContext(ctx, notifyChannelsSchema)
	return err
}

const notifyChannelColumns = `id, type, name, enabled, params, events, last_error, last_error_at, last_ok_at, created_at, updated_at`

func scanNotifyChannel(row interface{ Scan(...any) error }) (NotifyChannel, error) {
	var c NotifyChannel
	var enabled int
	var params, events string
	if err := row.Scan(&c.ID, &c.Type, &c.Name, &enabled, &params, &events,
		&c.LastError, &c.LastErrorAt, &c.LastOKAt, &c.CreatedAt, &c.UpdatedAt); err != nil {
		return c, err
	}
	c.Enabled = enabled != 0
	c.Params = map[string]string{}
	c.Events = []string{}
	_ = json.Unmarshal([]byte(params), &c.Params)
	_ = json.Unmarshal([]byte(events), &c.Events)
	return c, nil
}

func (r *TrafficRepository) ListNotifyChannels(ctx context.Context) ([]NotifyChannel, error) {
	if err := r.ensureNotifyChannelsTable(ctx); err != nil {
		return nil, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT `+notifyChannelColumns+` FROM notify_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []NotifyChannel{}
	for rows.Next() {
		c, err := scanNotifyChannel(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// ErrNotifyChannelNotFound 渠道不存在。
var ErrNotifyChannelNotFound = errors.New("推送渠道不存在")

func (r *TrafficRepository) GetNotifyChannel(ctx context.Context, id int64) (NotifyChannel, error) {
	if err := r.ensureNotifyChannelsTable(ctx); err != nil {
		return NotifyChannel{}, err
	}
	c, err := scanNotifyChannel(r.db.QueryRowContext(ctx, `SELECT `+notifyChannelColumns+` FROM notify_channels WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, ErrNotifyChannelNotFound
	}
	return c, err
}

func encodeNotifyChannel(c NotifyChannel) (params, events string, enabled int) {
	if c.Params == nil {
		c.Params = map[string]string{}
	}
	if c.Events == nil {
		c.Events = []string{}
	}
	p, _ := json.Marshal(c.Params)
	e, _ := json.Marshal(c.Events)
	if c.Enabled {
		enabled = 1
	}
	return string(p), string(e), enabled
}

func (r *TrafficRepository) CreateNotifyChannel(ctx context.Context, c NotifyChannel) (int64, error) {
	if err := r.ensureNotifyChannelsTable(ctx); err != nil {
		return 0, err
	}
	params, events, enabled := encodeNotifyChannel(c)
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO notify_channels (type, name, enabled, params, events, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		c.Type, c.Name, enabled, params, events)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateNotifyChannel 改配置(类型 / 名称 / 启用 / 参数 / 事件),不动发送状态。
func (r *TrafficRepository) UpdateNotifyChannel(ctx context.Context, c NotifyChannel) error {
	if err := r.ensureNotifyChannelsTable(ctx); err != nil {
		return err
	}
	params, events, enabled := encodeNotifyChannel(c)
	res, err := r.db.ExecContext(ctx,
		`UPDATE notify_channels SET type = ?, name = ?, enabled = ?, params = ?, events = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		c.Type, c.Name, enabled, params, events, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotifyChannelNotFound
	}
	return nil
}

func (r *TrafficRepository) DeleteNotifyChannel(ctx context.Context, id int64) error {
	if err := r.ensureNotifyChannelsTable(ctx); err != nil {
		return err
	}
	res, err := r.db.ExecContext(ctx, `DELETE FROM notify_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotifyChannelNotFound
	}
	return nil
}

// UpdateNotifyChannelStatus 回写最近一次发送结果。渠道已删除时静默忽略。
func (r *TrafficRepository) UpdateNotifyChannelStatus(ctx context.Context, id int64, lastError string, lastErrorAt, lastOKAt int64) error {
	if err := r.ensureNotifyChannelsTable(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE notify_channels SET last_error = ?, last_error_at = ?, last_ok_at = ? WHERE id = ?`,
		lastError, lastErrorAt, lastOKAt, id)
	return err
}
