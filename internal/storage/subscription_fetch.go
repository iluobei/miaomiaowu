package storage

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// 订阅获取开关(users.subscription_fetch)。关掉后该用户的所有订阅链接一律拒绝,节点凭据不动。
//
//	on  —— 默认,允许拉订阅
//	off —— 管理员关的,拉订阅一律 403
//
// 限时打开(#1497):users.subscription_fetch_until 存到期时刻(unix 秒,0 = 不限时),只在 on 时有意义。
// 到期后回到打开前的关闭状态 —— 妙妙屋只有管理员一条控制路径、只有 off 一种关闭状态,
// 所以不论打开前是 off 还是永久开,到期一律回到 off(与 X 里「管理员发起 → admin_off」同义)。
//
// 不跑定时任务,读的时候判:on 且已过期 = off(见 Effective),由 handler 顺手把库里也改掉。
// 手动关闭、永久打开都会把 until 清掉。
const (
	SubscriptionFetchOn  = "on"
	SubscriptionFetchOff = "off"
)

// SubscriptionFetchStatus 开关状态。Until 只在 State 为 on 且限时打开时非零,为到期时刻(unix 秒)。
type SubscriptionFetchStatus struct {
	State string
	Until int64
}

// Effective 按 now 算实际生效的状态:限时打开已到期的回到 off。
func (s SubscriptionFetchStatus) Effective(now time.Time) SubscriptionFetchStatus {
	if s.State == SubscriptionFetchOn && s.Until > 0 && s.Until <= now.Unix() {
		return SubscriptionFetchStatus{State: SubscriptionFetchOff}
	}
	return s
}

// normalizeSubscriptionFetch 不认识的值一律当开:列里出了脏数据不该把人的订阅断掉。
func normalizeSubscriptionFetch(v string) string {
	if strings.TrimSpace(v) == SubscriptionFetchOff {
		return SubscriptionFetchOff
	}
	return SubscriptionFetchOn
}

func newSubscriptionFetchStatus(state sql.NullString, until int64) SubscriptionFetchStatus {
	s := SubscriptionFetchStatus{State: normalizeSubscriptionFetch(state.String)}
	if s.State == SubscriptionFetchOn && until > 0 {
		s.Until = until
	}
	return s
}

const subscriptionFetchColumns = `subscription_fetch, COALESCE(subscription_fetch_until, 0)`

// GetUserSubscriptionFetch 读一个用户的订阅获取开关(库里的原值,不判到期)。用户不存在回 ErrUserNotFound。
func (r *TrafficRepository) GetUserSubscriptionFetch(ctx context.Context, username string) (SubscriptionFetchStatus, error) {
	var state sql.NullString
	var until int64
	err := r.db.QueryRowContext(ctx,
		`SELECT `+subscriptionFetchColumns+` FROM users WHERE username = ?`, username).Scan(&state, &until)
	if errors.Is(err, sql.ErrNoRows) {
		return SubscriptionFetchStatus{}, ErrUserNotFound
	}
	if err != nil {
		return SubscriptionFetchStatus{}, err
	}
	return newSubscriptionFetchStatus(state, until), nil
}

// ListUserSubscriptionFetch 全部用户的订阅获取开关(用户列表用,避免 N+1;库里的原值,不判到期)。
func (r *TrafficRepository) ListUserSubscriptionFetch(ctx context.Context) (map[string]SubscriptionFetchStatus, error) {
	out := make(map[string]SubscriptionFetchStatus)
	rows, err := r.db.QueryContext(ctx, `SELECT username, `+subscriptionFetchColumns+` FROM users`)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var username string
		var state sql.NullString
		var until int64
		if err := rows.Scan(&username, &state, &until); err != nil {
			return out, err
		}
		out[username] = newSubscriptionFetchStatus(state, until)
	}
	return out, rows.Err()
}

// ExpireUserSubscriptionFetch 限时打开到期后把库里改成 off。
// 只在仍是「on + 同一个 until」时改,免得把这期间刚被重新打开 / 续期的覆盖掉。
func (r *TrafficRepository) ExpireUserSubscriptionFetch(ctx context.Context, username string, until int64) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE users SET subscription_fetch = ?, subscription_fetch_until = 0
		 WHERE username = ? AND subscription_fetch = ? AND subscription_fetch_until = ?`,
		SubscriptionFetchOff, username, SubscriptionFetchOn, until)
	return err
}

// SetUserSubscriptionFetch 管理员开关:
//   - 关闭 = off,清 until
//   - 永久打开(until 0)= on,清 until
//   - 限时打开(until > 0)= on + until,到期回到 off;限时期间再限时(续期)只是换个 until
func (r *TrafficRepository) SetUserSubscriptionFetch(ctx context.Context, username string, enabled bool, until int64) (SubscriptionFetchStatus, error) {
	state := SubscriptionFetchOff
	if enabled {
		state = SubscriptionFetchOn
	}
	if !enabled || until < 0 {
		until = 0
	}
	res, err := r.db.ExecContext(ctx,
		`UPDATE users SET subscription_fetch = ?, subscription_fetch_until = ? WHERE username = ?`,
		state, until, username)
	if err != nil {
		return SubscriptionFetchStatus{}, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return SubscriptionFetchStatus{}, ErrUserNotFound
	}
	return r.GetUserSubscriptionFetch(ctx, username)
}
