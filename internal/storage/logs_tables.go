package storage

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// 日志管理功能的持久化层。三张表都是全新表，故 CREATE TABLE + CREATE INDEX 同批 Exec 是安全的
// （历史教训「内联 CREATE INDEX 会炸」只针对给既有表 ALTER 加列后再对新列建索引的场景）。

// SecurityEvent 是一条安全事件（探测/封禁/解封/登录失败等），只增不改。
type SecurityEvent struct {
	ID       int64     `json:"id"`
	At       time.Time `json:"at"`
	IP       string    `json:"ip"`
	Kind     string    `json:"kind"` // probe | ban | unban | ban_manual | login_fail | login_locked
	Path     string    `json:"path"`
	Username string    `json:"username"`
	Detail   string    `json:"detail"`
	Actor    string    `json:"actor"`
}

// IPBan 是当前封禁态，一 IP 一行。
type IPBan struct {
	IP         string     `json:"ip"`
	Reason     string     `json:"reason"`
	BannedAt   time.Time  `json:"banned_at"`
	ExpiresAt  *time.Time `json:"expires_at"` // 永久封禁时为 nil
	Permanent  bool       `json:"permanent"`
	FailCount  int        `json:"fail_count"`
	ReleasedAt *time.Time `json:"released_at"` // 非 nil = 已解封（留痕）
	Actor      string     `json:"actor"`
}

// TaskRun 是一次定时任务运行记录（P3 用，此处一并定义建表）。
type TaskRun struct {
	ID         int64     `json:"id"`
	TaskName   string    `json:"task_name"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs int64     `json:"duration_ms"`
	Status     string    `json:"status"` // ok | error
	Detail     string    `json:"detail"`
}

type OperationLog struct {
	ID     int64     `json:"id"`
	At     time.Time `json:"at"`
	Actor  string    `json:"actor"`
	Method string    `json:"method"`
	Path   string    `json:"path"`
	Status int       `json:"status"`
	IP     string    `json:"ip"`
}

func (r *TrafficRepository) migrateLogTables() error {
	const schema = `
CREATE TABLE IF NOT EXISTS security_events (
    id       INTEGER PRIMARY KEY AUTOINCREMENT,
    at       TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ip       TEXT NOT NULL,
    kind     TEXT NOT NULL,
    path     TEXT NOT NULL DEFAULT '',
    username TEXT NOT NULL DEFAULT '',
    detail   TEXT NOT NULL DEFAULT '',
    actor    TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_sec_events_at ON security_events(at DESC);
CREATE INDEX IF NOT EXISTS idx_sec_events_ip ON security_events(ip);
CREATE INDEX IF NOT EXISTS idx_sec_events_kind_at ON security_events(kind, at DESC);

CREATE TABLE IF NOT EXISTS ip_bans (
    ip          TEXT PRIMARY KEY,
    reason      TEXT NOT NULL DEFAULT '',
    banned_at   TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at  TIMESTAMP,
    permanent   INTEGER NOT NULL DEFAULT 0,
    fail_count  INTEGER NOT NULL DEFAULT 0,
    released_at TIMESTAMP,
    actor       TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_ip_bans_active ON ip_bans(released_at, expires_at);

CREATE TABLE IF NOT EXISTS task_runs (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    task_name   TEXT NOT NULL,
    started_at  TIMESTAMP NOT NULL,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL,
    detail      TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_task_runs_name_started ON task_runs(task_name, started_at DESC);
CREATE INDEX IF NOT EXISTS idx_task_runs_started ON task_runs(started_at DESC);

CREATE TABLE IF NOT EXISTS operation_logs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    actor TEXT NOT NULL DEFAULT '',
    method TEXT NOT NULL,
    path TEXT NOT NULL,
    status INTEGER NOT NULL,
    ip TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_operation_logs_at ON operation_logs(at DESC);
CREATE INDEX IF NOT EXISTS idx_operation_logs_actor_at ON operation_logs(actor, at DESC);
`
	if _, err := r.db.Exec(schema); err != nil {
		return fmt.Errorf("migrate log tables: %w", err)
	}
	return nil
}

func (r *TrafficRepository) InsertOperationLog(ctx context.Context, log OperationLog) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO operation_logs (at, actor, method, path, status, ip) VALUES (?, ?, ?, ?, ?, ?)`, time.Now(), log.Actor, log.Method, log.Path, log.Status, log.IP)
	return err
}

func (r *TrafficRepository) ListOperationLogs(ctx context.Context, limit, offset int) ([]OperationLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, at, actor, method, path, status, ip FROM operation_logs ORDER BY at DESC, id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var logs []OperationLog
	for rows.Next() {
		var log OperationLog
		if err := rows.Scan(&log.ID, &log.At, &log.Actor, &log.Method, &log.Path, &log.Status, &log.IP); err != nil {
			return nil, err
		}
		logs = append(logs, log)
	}
	return logs, rows.Err()
}

func (r *TrafficRepository) DeleteOldOperationLogs(ctx context.Context, cutoff time.Time) (int64, error) {
	result, err := r.db.ExecContext(ctx, `DELETE FROM operation_logs WHERE at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ---- security_events ----

// InsertSecurityEvent 记一条安全事件。best-effort：调用方通常忽略 error（日志失败不该影响主流程）。
func (r *TrafficRepository) InsertSecurityEvent(ctx context.Context, e SecurityEvent) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO security_events (at, ip, kind, path, username, detail, actor)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		time.Now(), e.IP, e.Kind, e.Path, e.Username, e.Detail, e.Actor)
	return err
}

// ListSecurityEvents 查询事件流。kind/ip 为空则不过滤。后端分页（limit/offset）。
func (r *TrafficRepository) ListSecurityEvents(ctx context.Context, kind, ip string, limit, offset int) ([]SecurityEvent, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id, at, ip, kind, path, username, detail, actor FROM security_events WHERE 1=1`
	args := []any{}
	if kind != "" {
		q += ` AND kind = ?`
		args = append(args, kind)
	}
	if ip != "" {
		q += ` AND ip = ?`
		args = append(args, ip)
	}
	q += ` ORDER BY at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []SecurityEvent
	for rows.Next() {
		var e SecurityEvent
		if err := rows.Scan(&e.ID, &e.At, &e.IP, &e.Kind, &e.Path, &e.Username, &e.Detail, &e.Actor); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// DeleteOldSecurityEvents 删除早于 cutoff 的事件，返回删除行数。retention 清理用。
func (r *TrafficRepository) DeleteOldSecurityEvents(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM security_events WHERE at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}

// ---- ip_bans ----

// UpsertIPBan 写入/更新一条封禁。同 IP 已存在则覆盖（用于「提升为永久」/续封）。
func (r *TrafficRepository) UpsertIPBan(ctx context.Context, b IPBan) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO ip_bans (ip, reason, banned_at, expires_at, permanent, fail_count, released_at, actor)
		 VALUES (?, ?, ?, ?, ?, ?, NULL, ?)
		 ON CONFLICT(ip) DO UPDATE SET
		   reason=excluded.reason, banned_at=excluded.banned_at, expires_at=excluded.expires_at,
		   permanent=excluded.permanent, fail_count=excluded.fail_count, released_at=NULL, actor=excluded.actor`,
		b.IP, b.Reason, b.BannedAt, b.ExpiresAt, boolToInt(b.Permanent), b.FailCount, b.Actor)
	return err
}

// ReleaseIPBan 标记解封（留痕，不删行）。
func (r *TrafficRepository) ReleaseIPBan(ctx context.Context, ip, actor string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE ip_bans SET released_at = ?, actor = ? WHERE ip = ? AND released_at IS NULL`,
		time.Now(), actor, ip)
	return err
}

// ListActiveIPBans 返回当前仍生效的封禁（未解封，且永久或未过期）。
func (r *TrafficRepository) ListActiveIPBans(ctx context.Context) ([]IPBan, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT ip, reason, banned_at, expires_at, permanent, fail_count, released_at, actor
		 FROM ip_bans
		 WHERE released_at IS NULL AND (permanent = 1 OR expires_at > ?)
		 ORDER BY banned_at DESC`, time.Now())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanIPBans(rows)
}

// ListRestorableIPBans 启动回填用：所有仍生效的封禁（同 ListActiveIPBans 的条件）。
// 单独留一个语义名，方便回填逻辑读起来清楚“这是要灌回内存的集合”。
func (r *TrafficRepository) ListRestorableIPBans(ctx context.Context) ([]IPBan, error) {
	return r.ListActiveIPBans(ctx)
}

func scanIPBans(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]IPBan, error) {
	var out []IPBan
	for rows.Next() {
		var b IPBan
		var perm int
		if err := rows.Scan(&b.IP, &b.Reason, &b.BannedAt, &b.ExpiresAt, &perm, &b.FailCount, &b.ReleasedAt, &b.Actor); err != nil {
			return nil, err
		}
		b.Permanent = perm != 0
		out = append(out, b)
	}
	return out, rows.Err()
}

// ---- task_runs（P3 用，方法一并放这里） ----

// InsertTaskRun 记一次任务运行。
func (r *TrafficRepository) InsertTaskRun(ctx context.Context, taskName string, startedAt time.Time, durationMs int64, status, detail string) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO task_runs (task_name, started_at, duration_ms, status, detail) VALUES (?, ?, ?, ?, ?)`,
		taskName, startedAt, durationMs, status, detail)
	return err
}

// StartTaskRun 插入一条运行中记录并返回 ID，供耗时任务在开始时立即出现在日志页面。
func (r *TrafficRepository) StartTaskRun(ctx context.Context, taskName, detail string) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO task_runs (task_name, started_at, duration_ms, status, detail) VALUES (?, ?, 0, 'running', ?)`,
		taskName, time.Now(), detail)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// FinishTaskRun 原地完成一条运行中记录。
func (r *TrafficRepository) FinishTaskRun(ctx context.Context, id int64, durationMs int64, status, detail string) error {
	if r == nil || r.db == nil || id <= 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE task_runs SET duration_ms = ?, status = ?, detail = ? WHERE id = ?`,
		durationMs, status, detail, id)
	return err
}

// ListTaskRuns 查询运行记录。task/status 为空则不过滤。后端分页。
func (r *TrafficRepository) ListTaskRuns(ctx context.Context, task, status string, limit, offset int) ([]TaskRun, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}
	q := `SELECT id, task_name, started_at, duration_ms, status, detail FROM task_runs WHERE 1=1`
	args := []any{}
	if task != "" {
		q += ` AND task_name = ?`
		args = append(args, task)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += ` ORDER BY started_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []TaskRun
	for rows.Next() {
		var t TaskRun
		if err := rows.Scan(&t.ID, &t.TaskName, &t.StartedAt, &t.DurationMs, &t.Status, &t.Detail); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// DeleteOldTaskRuns 删除早于 cutoff 的运行记录，返回删除行数。
func (r *TrafficRepository) DeleteOldTaskRuns(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx, `DELETE FROM task_runs WHERE started_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
