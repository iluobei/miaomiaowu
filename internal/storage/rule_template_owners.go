package storage

import "context"

// rule_template_owners 记录 rule_templates 目录下每个模板文件的创建者,
// 用于普通用户"只能删除/修改自己上传的模板"。文件系统本身无归属信息,故用此表追踪。

func (r *TrafficRepository) ensureRuleTemplateOwnersTable(ctx context.Context) error {
	if r == nil || r.db == nil {
		return nil
	}
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS rule_template_owners (
		filename TEXT PRIMARY KEY,
		created_by TEXT NOT NULL DEFAULT '',
		is_public INTEGER NOT NULL DEFAULT 0
	)`)
	if err == nil {
		_, _ = r.db.ExecContext(ctx, `ALTER TABLE rule_template_owners ADD COLUMN is_public INTEGER NOT NULL DEFAULT 0`)
	}
	return err
}

func (r *TrafficRepository) SetRuleTemplatePublic(ctx context.Context, filename string, public bool) error {
	if err := r.ensureRuleTemplateOwnersTable(ctx); err != nil {
		return err
	}
	value := 0
	if public {
		value = 1
	}
	_, err := r.db.ExecContext(ctx, `UPDATE rule_template_owners SET is_public = ? WHERE filename = ?`, value, filename)
	return err
}

func (r *TrafficRepository) IsRuleTemplatePublic(ctx context.Context, filename string) bool {
	if r.ensureRuleTemplateOwnersTable(ctx) != nil {
		return false
	}
	var value int
	return r.db.QueryRowContext(ctx, `SELECT is_public FROM rule_template_owners WHERE filename = ?`, filename).Scan(&value) == nil && value != 0
}

// SetRuleTemplateOwner 记录/更新模板文件归属。
func (r *TrafficRepository) SetRuleTemplateOwner(ctx context.Context, filename, createdBy string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if err := r.ensureRuleTemplateOwnersTable(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO rule_template_owners (filename, created_by) VALUES (?, ?)
		 ON CONFLICT(filename) DO UPDATE SET created_by = excluded.created_by`,
		filename, createdBy)
	return err
}

// GetRuleTemplateOwner 返回模板归属者,无记录返回空串(视为历史/管理员模板)。
func (r *TrafficRepository) GetRuleTemplateOwner(ctx context.Context, filename string) (string, error) {
	if r == nil || r.db == nil {
		return "", nil
	}
	if err := r.ensureRuleTemplateOwnersTable(ctx); err != nil {
		return "", err
	}
	var owner string
	err := r.db.QueryRowContext(ctx, `SELECT created_by FROM rule_template_owners WHERE filename = ?`, filename).Scan(&owner)
	if err != nil {
		return "", nil // 无记录 → 空
	}
	return owner, nil
}

// RenameRuleTemplateOwner 模板重命名时迁移归属记录。
func (r *TrafficRepository) RenameRuleTemplateOwner(ctx context.Context, oldName, newName string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if err := r.ensureRuleTemplateOwnersTable(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `UPDATE rule_template_owners SET filename = ? WHERE filename = ?`, newName, oldName)
	return err
}

// DeleteRuleTemplateOwner 删除模板时清理归属记录。
func (r *TrafficRepository) DeleteRuleTemplateOwner(ctx context.Context, filename string) error {
	if r == nil || r.db == nil {
		return nil
	}
	if err := r.ensureRuleTemplateOwnersTable(ctx); err != nil {
		return err
	}
	_, err := r.db.ExecContext(ctx, `DELETE FROM rule_template_owners WHERE filename = ?`, filename)
	return err
}

// CountUserRuleTemplates 统计某用户拥有的规则模板数(供 per-user 配额校验)。
func (r *TrafficRepository) CountUserRuleTemplates(ctx context.Context, username string) (int, error) {
	if r == nil || r.db == nil {
		return 0, nil
	}
	if err := r.ensureRuleTemplateOwnersTable(ctx); err != nil {
		return 0, err
	}
	var n int
	err := r.db.QueryRowContext(ctx, `SELECT COUNT(1) FROM rule_template_owners WHERE created_by = ?`, username).Scan(&n)
	return n, err
}

// ListRuleTemplateOwners 返回 filename→created_by 映射(供列表标注归属)。
func (r *TrafficRepository) ListRuleTemplateOwners(ctx context.Context) (map[string]string, error) {
	result := make(map[string]string)
	if r == nil || r.db == nil {
		return result, nil
	}
	if err := r.ensureRuleTemplateOwnersTable(ctx); err != nil {
		return result, err
	}
	rows, err := r.db.QueryContext(ctx, `SELECT filename, created_by FROM rule_template_owners`)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var f, c string
		if err := rows.Scan(&f, &c); err != nil {
			return result, err
		}
		result[f] = c
	}
	return result, rows.Err()
}
