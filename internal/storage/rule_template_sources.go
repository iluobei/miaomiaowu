package storage

import "context"

// rule_template_sources 记录「从链接导入」的规则模板的来源链接,供「从链接更新」再拉一次。
// 不放进 rule_template_owners:那张表有行就算「有归属」,给内置模板记来源会让普通用户看不到它。

func (r *TrafficRepository) ensureRuleTemplateSourcesTable(ctx context.Context) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS rule_template_sources (
		filename   TEXT PRIMARY KEY,
		source_url TEXT NOT NULL DEFAULT ''
	)`)
	return err
}

// SetRuleTemplateSource 记录模板的来源链接;url 为空时删掉记录。
func (r *TrafficRepository) SetRuleTemplateSource(ctx context.Context, filename, url string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if err := r.ensureRuleTemplateSourcesTable(ctx); err != nil {
		return err
	}
	if url == "" {
		_, err := r.db.ExecContext(ctx, `DELETE FROM rule_template_sources WHERE filename = ?`, filename)
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO rule_template_sources (filename, source_url) VALUES (?, ?)
		 ON CONFLICT(filename) DO UPDATE SET source_url = excluded.source_url`,
		filename, url)
	return err
}

// ListRuleTemplateSources 全部模板的来源链接:文件名 → 链接。
func (r *TrafficRepository) ListRuleTemplateSources(ctx context.Context) (map[string]string, error) {
	result := map[string]string{}
	if r == nil || r.db == nil {
		return result, nil
	}
	if err := r.ensureRuleTemplateSourcesTable(ctx); err != nil {
		return result, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT filename, source_url FROM rule_template_sources`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var f, u string
		if err := rows.Scan(&f, &u); err != nil {
			return result, err
		}
		result[f] = u
	}
	return result, rows.Err()
}

// RenameRuleTemplateSource 模板改名时跟着迁移来源记录。
func (r *TrafficRepository) RenameRuleTemplateSource(ctx context.Context, oldName, newName string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if err := r.ensureRuleTemplateSourcesTable(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE rule_template_sources SET filename = ? WHERE filename = ?`, newName, oldName)
	return err
}
