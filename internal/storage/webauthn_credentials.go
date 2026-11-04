package storage

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// WebAuthnCredential 一把 passkey。
//
// 与 2FA 不同,这里不能像 totp_secret 那样往 users 加列 —— 一个用户可以注册多把
// passkey(手机一把、电脑一把、硬件密钥一把),必须独立成表。
//
// Credential 列存 webauthn.Credential 的完整 JSON,而不是逐字段建列:该结构的字段随库
// 版本增删(v0.18 就比早期版本多出 AttestationFormat / Extensions),逐字段建列意味着每次
// 升级库都要改 schema 并写迁移。整体存 JSON 天然向前兼容,反正除 credential_id 外没有
// 任何字段需要参与查询。
//
// CredentialID 单独提一列是必须的:passkey 登录(discoverable login)时浏览器只回传
// credential id,没有用户名,得靠它反查是谁。
type WebAuthnCredential struct {
	ID           int64      `json:"id"`
	Username     string     `json:"username"`
	CredentialID string     `json:"credential_id"`
	Credential   string     `json:"-"`
	Name         string     `json:"name"`
	AAGUID       string     `json:"aaguid"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at,omitempty"`
}

const webauthnCredentialSelect = `SELECT id, username, credential_id, credential,
 COALESCE(name, ''), COALESCE(aaguid, ''), created_at, last_used_at FROM webauthn_credentials`

func scanWebAuthnCredential(row interface{ Scan(...any) error }) (*WebAuthnCredential, error) {
	var c WebAuthnCredential
	var lastUsed sql.NullTime
	if err := row.Scan(&c.ID, &c.Username, &c.CredentialID, &c.Credential,
		&c.Name, &c.AAGUID, &c.CreatedAt, &lastUsed); err != nil {
		return nil, err
	}
	if lastUsed.Valid {
		t := lastUsed.Time
		c.LastUsedAt = &t
	}
	return &c, nil
}

// ListWebAuthnCredentials 列出某用户的全部 passkey(管理界面 + 登录时取候选凭据都用它)。
func (r *TrafficRepository) ListWebAuthnCredentials(ctx context.Context, username string) ([]WebAuthnCredential, error) {
	rows, err := r.db.QueryContext(ctx, webauthnCredentialSelect+` WHERE username = ? ORDER BY id ASC`, username)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := make([]WebAuthnCredential, 0)
	for rows.Next() {
		c, err := scanWebAuthnCredential(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, *c)
	}
	return list, rows.Err()
}

// GetWebAuthnCredentialByID 按 credential id 反查。passkey 登录只有这一个已知量。
func (r *TrafficRepository) GetWebAuthnCredentialByID(ctx context.Context, credentialID string) (*WebAuthnCredential, error) {
	c, err := scanWebAuthnCredential(r.db.QueryRowContext(ctx, webauthnCredentialSelect+` WHERE credential_id = ?`, credentialID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return c, err
}

// CreateWebAuthnCredential 落一把新注册的 passkey。credential_id 上的唯一约束会挡住
// 同一把钥匙重复注册(浏览器也会靠 excludeCredentials 提前拦一道)。
func (r *TrafficRepository) CreateWebAuthnCredential(ctx context.Context, c WebAuthnCredential) error {
	_, err := r.db.ExecContext(ctx, `INSERT INTO webauthn_credentials
		(username, credential_id, credential, name, aaguid) VALUES (?, ?, ?, ?, ?)`,
		c.Username, c.CredentialID, c.Credential, c.Name, c.AAGUID)
	return err
}

// UpdateWebAuthnCredentialAfterLogin 登录成功后回写:sign count 等状态在 credential JSON
// 里,连同 last_used_at 一起更新。sign count 是克隆检测的依据,不回写等于关掉该保护。
func (r *TrafficRepository) UpdateWebAuthnCredentialAfterLogin(ctx context.Context, credentialID, credential string) error {
	_, err := r.db.ExecContext(ctx, `UPDATE webauthn_credentials
		SET credential = ?, last_used_at = CURRENT_TIMESTAMP WHERE credential_id = ?`, credential, credentialID)
	return err
}

// RenameWebAuthnCredential 改显示名。限定 username 防越权改他人的。
func (r *TrafficRepository) RenameWebAuthnCredential(ctx context.Context, username string, id int64, name string) error {
	res, err := r.db.ExecContext(ctx, `UPDATE webauthn_credentials SET name = ? WHERE id = ? AND username = ?`, name, id, username)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return errors.New("passkey 不存在")
	}
	return nil
}

// DeleteWebAuthnCredential 删除。限定 username 防越权删他人的。
func (r *TrafficRepository) DeleteWebAuthnCredential(ctx context.Context, username string, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM webauthn_credentials WHERE id = ? AND username = ?`, id, username)
	if err != nil {
		return err
	}
	if affected, _ := res.RowsAffected(); affected == 0 {
		return errors.New("passkey 不存在")
	}
	return nil
}
