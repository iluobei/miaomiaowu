package storage

import (
	"context"
	"database/sql"
	"time"
)

// 规则集(clash rule-providers)托管。
//
// 为什么存库而不是落盘:内容要能进自动备份。落盘的话得再挂一个目录 —— Docker 那边
// 已经因为 subscribes/ 没挂而丢过用户文件,不想再添一个同类陷阱。规则集本来就是
// 几十到几千行文本,放 TEXT 列没有压力。
//
// source 只有两种:manual(面板里编辑,或从本地文件读进来的文本)和 remote
// (填远程地址,由主控定时抓取并缓存)。"上传文件"不单独算一种 —— 文件在浏览器端
// 读成文本就落进编辑器了,存的时候和手打的没有区别。

// RuleProvider 一个托管的规则集。
type RuleProvider struct {
	ID             int64      `json:"id"`
	Name           string     `json:"name"` // 公开路径用的文件名,唯一
	DisplayName    string     `json:"display_name"`
	Source         string     `json:"source"` // manual | remote
	RemoteURL      string     `json:"remote_url"`
	RefreshMinutes int        `json:"refresh_minutes"`
	Content        string     `json:"content,omitempty"`
	Size           int        `json:"size"`
	LastFetchAt    *time.Time `json:"last_fetch_at,omitempty"`
	LastFetchError string     `json:"last_fetch_error"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

const ruleProviderColumns = `id, name, display_name, source, remote_url, refresh_minutes,
	last_fetch_at, last_fetch_error, created_at, updated_at`

func scanRuleProvider(scan func(...any) error) (RuleProvider, error) {
	var p RuleProvider
	var fetchedAt sql.NullTime
	err := scan(&p.ID, &p.Name, &p.DisplayName, &p.Source, &p.RemoteURL,
		&p.RefreshMinutes, &fetchedAt, &p.LastFetchError, &p.CreatedAt, &p.UpdatedAt)
	if err != nil {
		return p, err
	}
	if fetchedAt.Valid {
		t := fetchedAt.Time
		p.LastFetchAt = &t
	}
	return p, nil
}

// ListRuleProviders 列表。**不带 content** —— 列表页只需要元信息,
// 十几个规则集各几百 KB 一起塞进响应没有意义(size 单独算给前端显示)。
func (r *TrafficRepository) ListRuleProviders(ctx context.Context) ([]RuleProvider, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+ruleProviderColumns+`, LENGTH(content) FROM rule_providers ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []RuleProvider{}
	for rows.Next() {
		var p RuleProvider
		var fetchedAt sql.NullTime
		var size sql.NullInt64
		if err := rows.Scan(&p.ID, &p.Name, &p.DisplayName, &p.Source, &p.RemoteURL,
			&p.RefreshMinutes, &fetchedAt, &p.LastFetchError, &p.CreatedAt, &p.UpdatedAt, &size); err != nil {
			return nil, err
		}
		if fetchedAt.Valid {
			t := fetchedAt.Time
			p.LastFetchAt = &t
		}
		p.Size = int(size.Int64)
		list = append(list, p)
	}
	return list, rows.Err()
}

// GetRuleProvider 按 id 取,带内容。
func (r *TrafficRepository) GetRuleProvider(ctx context.Context, id int64) (RuleProvider, error) {
	row := r.db.QueryRowContext(ctx,
		`SELECT `+ruleProviderColumns+`, content FROM rule_providers WHERE id = ?`, id)
	var p RuleProvider
	var fetchedAt sql.NullTime
	err := row.Scan(&p.ID, &p.Name, &p.DisplayName, &p.Source, &p.RemoteURL,
		&p.RefreshMinutes, &fetchedAt, &p.LastFetchError, &p.CreatedAt, &p.UpdatedAt, &p.Content)
	if err != nil {
		return p, err
	}
	if fetchedAt.Valid {
		t := fetchedAt.Time
		p.LastFetchAt = &t
	}
	p.Size = len(p.Content)
	return p, nil
}

// GetRuleProviderContentByName 公开下载走这条:只取内容和更新时间,不碰其它列。
func (r *TrafficRepository) GetRuleProviderContentByName(ctx context.Context, name string) (string, time.Time, error) {
	var content string
	var updated time.Time
	err := r.db.QueryRowContext(ctx,
		`SELECT content, updated_at FROM rule_providers WHERE name = ?`, name).Scan(&content, &updated)
	return content, updated, err
}

// CreateRuleProvider 新建,返回自增 id。
func (r *TrafficRepository) CreateRuleProvider(ctx context.Context, p RuleProvider) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`INSERT INTO rule_providers (name, display_name, source, remote_url, refresh_minutes, content)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		p.Name, p.DisplayName, p.Source, p.RemoteURL, p.RefreshMinutes, p.Content)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// UpdateRuleProvider 更新元信息与内容。
func (r *TrafficRepository) UpdateRuleProvider(ctx context.Context, p RuleProvider) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE rule_providers SET name = ?, display_name = ?, source = ?, remote_url = ?,
		 refresh_minutes = ?, content = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		p.Name, p.DisplayName, p.Source, p.RemoteURL, p.RefreshMinutes, p.Content, p.ID)
	return err
}

// DeleteRuleProvider 删除。
func (r *TrafficRepository) DeleteRuleProvider(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM rule_providers WHERE id = ?`, id)
	return err
}

// ListRuleProvidersDueForFetch 到期该抓的远程规则集。
//
// 判据写成「从没抓过 或 距上次抓取已超过刷新间隔」。时间比较交给数据库做,
// 两种方言都支持 —— 但 SQLite 没有 INTERVAL,所以改成在 Go 侧算截止时刻传进去。
func (r *TrafficRepository) ListRuleProvidersDueForFetch(ctx context.Context, now time.Time) ([]RuleProvider, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT `+ruleProviderColumns+` FROM rule_providers
		 WHERE source = 'remote' AND remote_url <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	due := []RuleProvider{}
	for rows.Next() {
		p, err := scanRuleProvider(rows.Scan)
		if err != nil {
			return nil, err
		}
		// 间隔判断放 Go 里:SQLite 与 Postgres 的时间加减语法不通用,
		// 而这张表最多几十行,全读回来再筛的代价可以忽略。
		if p.RefreshMinutes <= 0 {
			continue
		}
		if p.LastFetchAt == nil ||
			!p.LastFetchAt.Add(time.Duration(p.RefreshMinutes)*time.Minute).After(now.UTC()) {
			due = append(due, p)
		}
	}
	return due, rows.Err()
}

// MarkRuleProviderFetched 记录一次抓取结果。
//
// 失败时**不清空已有内容**:上次抓到的还能继续服务,总比订阅拿到空文件强。
func (r *TrafficRepository) MarkRuleProviderFetched(ctx context.Context, id int64, content string, fetchErr string) error {
	if fetchErr != "" {
		_, err := r.db.ExecContext(ctx,
			`UPDATE rule_providers SET last_fetch_at = CURRENT_TIMESTAMP, last_fetch_error = ? WHERE id = ?`,
			fetchErr, id)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE rule_providers SET content = ?, last_fetch_at = CURRENT_TIMESTAMP,
		 last_fetch_error = '', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		content, id)
	return err
}
