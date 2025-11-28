package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

const (
	SubscribeTypeCreate = "create"
	SubscribeTypeImport = "import"
	SubscribeTypeUpload = "upload"
)

// ListSubscribeFiles returns all subscribe files ordered by creation time.
func (r *TrafficRepository) ListSubscribeFiles(ctx context.Context) ([]SubscribeFile, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `SELECT id, name, COALESCE(description, ''), url, type, filename, COALESCE(file_short_code, ''), created_at, updated_at FROM subscribe_files ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list subscribe files: %w", err)
	}
	defer rows.Close()

	var files []SubscribeFile
	for rows.Next() {
		var file SubscribeFile
		if err := rows.Scan(&file.ID, &file.Name, &file.Description, &file.URL, &file.Type, &file.Filename, &file.FileShortCode, &file.CreatedAt, &file.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan subscribe file: %w", err)
		}
		files = append(files, file)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate subscribe files: %w", err)
	}

	return files, nil
}

// GetSubscribeFileByID retrieves a subscribe file by ID.
func (r *TrafficRepository) GetSubscribeFileByID(ctx context.Context, id int64) (SubscribeFile, error) {
	var file SubscribeFile
	if r == nil || r.db == nil {
		return file, errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return file, errors.New("subscribe file id is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, COALESCE(description, ''), url, type, filename, COALESCE(file_short_code, ''), created_at, updated_at FROM subscribe_files WHERE id = ? LIMIT 1`, id)
	if err := row.Scan(&file.ID, &file.Name, &file.Description, &file.URL, &file.Type, &file.Filename, &file.FileShortCode, &file.CreatedAt, &file.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return file, ErrSubscribeFileNotFound
		}
		return file, fmt.Errorf("get subscribe file: %w", err)
	}

	return file, nil
}

// GetSubscribeFileByName retrieves a subscribe file by name.
func (r *TrafficRepository) GetSubscribeFileByName(ctx context.Context, name string) (SubscribeFile, error) {
	var file SubscribeFile
	if r == nil || r.db == nil {
		return file, errors.New("traffic repository not initialized")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return file, errors.New("subscribe file name is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, COALESCE(description, ''), url, type, filename, COALESCE(file_short_code, ''), created_at, updated_at FROM subscribe_files WHERE name = ? LIMIT 1`, name)
	if err := row.Scan(&file.ID, &file.Name, &file.Description, &file.URL, &file.Type, &file.Filename, &file.FileShortCode, &file.CreatedAt, &file.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return file, ErrSubscribeFileNotFound
		}
		return file, fmt.Errorf("get subscribe file by name: %w", err)
	}

	return file, nil
}

// GetSubscribeFileByFilename retrieves a subscribe file by filename.
func (r *TrafficRepository) GetSubscribeFileByFilename(ctx context.Context, filename string) (SubscribeFile, error) {
	var file SubscribeFile
	if r == nil || r.db == nil {
		return file, errors.New("traffic repository not initialized")
	}

	filename = strings.TrimSpace(filename)
	if filename == "" {
		return file, errors.New("subscribe file filename is required")
	}

	row := r.db.QueryRowContext(ctx, `SELECT id, name, COALESCE(description, ''), url, type, filename, COALESCE(file_short_code, ''), created_at, updated_at FROM subscribe_files WHERE filename = ? LIMIT 1`, filename)
	if err := row.Scan(&file.ID, &file.Name, &file.Description, &file.URL, &file.Type, &file.Filename, &file.FileShortCode, &file.CreatedAt, &file.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return file, ErrSubscribeFileNotFound
		}
		return file, fmt.Errorf("get subscribe file by filename: %w", err)
	}

	return file, nil
}

// CreateSubscribeFile inserts a new subscribe file record.
func (r *TrafficRepository) CreateSubscribeFile(ctx context.Context, file SubscribeFile) (SubscribeFile, error) {
	if r == nil || r.db == nil {
		return SubscribeFile{}, errors.New("traffic repository not initialized")
	}

	file.Name = strings.TrimSpace(file.Name)
	file.Description = strings.TrimSpace(file.Description)
	file.URL = strings.TrimSpace(file.URL)
	file.Type = strings.ToLower(strings.TrimSpace(file.Type))
	file.Filename = strings.TrimSpace(file.Filename)

	if file.Name == "" {
		return SubscribeFile{}, errors.New("subscribe file name is required")
	}
	if file.Type != SubscribeTypeCreate && file.Type != SubscribeTypeImport && file.Type != SubscribeTypeUpload {
		return SubscribeFile{}, errors.New("invalid subscribe file type")
	}
	// URL只对import类型必填，upload类型可以为空
	if (file.Type == SubscribeTypeImport) && file.URL == "" {
		return SubscribeFile{}, errors.New("subscribe file url is required")
	}
	if file.Filename == "" {
		return SubscribeFile{}, errors.New("subscribe file filename is required")
	}

	// Generate file short code with retry logic for collision handling
	const maxRetries = 10
	for i := 0; i < maxRetries; i++ {
		newFileShortCode, err := generateFileShortCode()
		if err != nil {
			return SubscribeFile{}, fmt.Errorf("generate file short code: %w", err)
		}

		res, err := r.db.ExecContext(ctx, `INSERT INTO subscribe_files (name, description, url, type, filename, file_short_code) VALUES (?, ?, ?, ?, ?, ?)`,
			file.Name, file.Description, file.URL, file.Type, file.Filename, newFileShortCode)
		if err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "unique") && strings.Contains(strings.ToLower(err.Error()), "file_short_code") {
				// File short code collision, retry
				continue
			}
			if strings.Contains(strings.ToLower(err.Error()), "unique") {
				return SubscribeFile{}, ErrSubscribeFileExists
			}
			return SubscribeFile{}, fmt.Errorf("create subscribe file: %w", err)
		}

		id, err := res.LastInsertId()
		if err != nil {
			return SubscribeFile{}, fmt.Errorf("fetch subscribe file id: %w", err)
		}

		return r.GetSubscribeFileByID(ctx, id)
	}

	return SubscribeFile{}, errors.New("failed to generate unique file short code after retries")
}

// UpdateSubscribeFile updates an existing subscribe file record.
func (r *TrafficRepository) UpdateSubscribeFile(ctx context.Context, file SubscribeFile) (SubscribeFile, error) {
	if r == nil || r.db == nil {
		return SubscribeFile{}, errors.New("traffic repository not initialized")
	}

	if file.ID <= 0 {
		return SubscribeFile{}, errors.New("subscribe file id is required")
	}

	file.Name = strings.TrimSpace(file.Name)
	file.Description = strings.TrimSpace(file.Description)
	file.URL = strings.TrimSpace(file.URL)
	file.Type = strings.ToLower(strings.TrimSpace(file.Type))
	file.Filename = strings.TrimSpace(file.Filename)

	if file.Name == "" {
		return SubscribeFile{}, errors.New("subscribe file name is required")
	}
	if file.Type != SubscribeTypeCreate && file.Type != SubscribeTypeImport && file.Type != SubscribeTypeUpload {
		return SubscribeFile{}, errors.New("invalid subscribe file type")
	}
	// URL只对import类型必填，upload类型可以为空
	if (file.Type == SubscribeTypeImport) && file.URL == "" {
		return SubscribeFile{}, errors.New("subscribe file url is required")
	}
	if file.Filename == "" {
		return SubscribeFile{}, errors.New("subscribe file filename is required")
	}

	res, err := r.db.ExecContext(ctx, `UPDATE subscribe_files SET name = ?, description = ?, url = ?, type = ?, filename = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		file.Name, file.Description, file.URL, file.Type, file.Filename, file.ID)
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			return SubscribeFile{}, ErrSubscribeFileExists
		}
		return SubscribeFile{}, fmt.Errorf("update subscribe file: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return SubscribeFile{}, fmt.Errorf("subscribe file update rows affected: %w", err)
	}
	if affected == 0 {
		return SubscribeFile{}, ErrSubscribeFileNotFound
	}

	return r.GetSubscribeFileByID(ctx, file.ID)
}

// DeleteSubscribeFile removes a subscribe file record.
func (r *TrafficRepository) DeleteSubscribeFile(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return errors.New("subscribe file id is required")
	}

	res, err := r.db.ExecContext(ctx, `DELETE FROM subscribe_files WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete subscribe file: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("subscribe file delete rows affected: %w", err)
	}
	if affected == 0 {
		return ErrSubscribeFileNotFound
	}

	return nil
}
