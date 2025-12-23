package storage

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Template represents an ACL4SSR rule template configuration
type Template struct {
	ID               int64
	Name             string
	Category         string // clash or surge
	TemplateURL      string // GitHub template URL
	RuleSource       string // ACL configuration URL
	UseProxy         bool   // Whether to use proxy for downloading
	EnableIncludeAll bool   // Whether to enable include-all mode
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

var (
	ErrTemplateNotFound = errors.New("template not found")
	ErrTemplateExists   = errors.New("template already exists")
)

// ListTemplates returns all templates ordered by creation time
func (r *TrafficRepository) ListTemplates(ctx context.Context) ([]Template, error) {
	if r == nil || r.db == nil {
		return nil, errors.New("traffic repository not initialized")
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id, name, category, template_url, rule_source, use_proxy, enable_include_all, created_at, updated_at
		FROM templates
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("list templates: %w", err)
	}
	defer rows.Close()

	var templates []Template
	for rows.Next() {
		t, err := scanTemplate(rows)
		if err != nil {
			return nil, fmt.Errorf("scan template: %w", err)
		}
		templates = append(templates, t)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate templates: %w", err)
	}

	return templates, nil
}

// GetTemplateByID retrieves a template by its ID
func (r *TrafficRepository) GetTemplateByID(ctx context.Context, id int64) (Template, error) {
	var t Template
	if r == nil || r.db == nil {
		return t, errors.New("traffic repository not initialized")
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, category, template_url, rule_source, use_proxy, enable_include_all, created_at, updated_at
		FROM templates
		WHERE id = ?
	`, id)

	t, err := scanTemplate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrTemplateNotFound
		}
		return t, fmt.Errorf("get template by id: %w", err)
	}

	return t, nil
}

// GetTemplateByName retrieves a template by its name
func (r *TrafficRepository) GetTemplateByName(ctx context.Context, name string) (Template, error) {
	var t Template
	if r == nil || r.db == nil {
		return t, errors.New("traffic repository not initialized")
	}

	name = strings.TrimSpace(name)
	if name == "" {
		return t, errors.New("template name is required")
	}

	row := r.db.QueryRowContext(ctx, `
		SELECT id, name, category, template_url, rule_source, use_proxy, enable_include_all, created_at, updated_at
		FROM templates
		WHERE name = ?
	`, name)

	t, err := scanTemplate(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return t, ErrTemplateNotFound
		}
		return t, fmt.Errorf("get template by name: %w", err)
	}

	return t, nil
}

// CreateTemplate creates a new template
func (r *TrafficRepository) CreateTemplate(ctx context.Context, t Template) (int64, error) {
	if r == nil || r.db == nil {
		return 0, errors.New("traffic repository not initialized")
	}

	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return 0, errors.New("template name is required")
	}

	// Default category
	if t.Category == "" {
		t.Category = "clash"
	}

	// Check for duplicate name
	_, err := r.GetTemplateByName(ctx, t.Name)
	if err == nil {
		return 0, ErrTemplateExists
	} else if !errors.Is(err, ErrTemplateNotFound) {
		return 0, err
	}

	result, err := r.db.ExecContext(ctx, `
		INSERT INTO templates (name, category, template_url, rule_source, use_proxy, enable_include_all, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
	`, t.Name, t.Category, t.TemplateURL, t.RuleSource, boolToInt(t.UseProxy), boolToInt(t.EnableIncludeAll))
	if err != nil {
		return 0, fmt.Errorf("create template: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("get template id: %w", err)
	}

	return id, nil
}

// UpdateTemplate updates an existing template
func (r *TrafficRepository) UpdateTemplate(ctx context.Context, t Template) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if t.ID <= 0 {
		return errors.New("template id is required")
	}

	t.Name = strings.TrimSpace(t.Name)
	if t.Name == "" {
		return errors.New("template name is required")
	}

	// Default category
	if t.Category == "" {
		t.Category = "clash"
	}

	// Check if template exists
	existing, err := r.GetTemplateByID(ctx, t.ID)
	if err != nil {
		return err
	}

	// Check for duplicate name (if name changed)
	if t.Name != existing.Name {
		_, err := r.GetTemplateByName(ctx, t.Name)
		if err == nil {
			return ErrTemplateExists
		} else if !errors.Is(err, ErrTemplateNotFound) {
			return err
		}
	}

	_, err = r.db.ExecContext(ctx, `
		UPDATE templates
		SET name = ?, category = ?, template_url = ?, rule_source = ?, use_proxy = ?, enable_include_all = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, t.Name, t.Category, t.TemplateURL, t.RuleSource, boolToInt(t.UseProxy), boolToInt(t.EnableIncludeAll), t.ID)
	if err != nil {
		return fmt.Errorf("update template: %w", err)
	}

	return nil
}

// DeleteTemplate deletes a template by ID
func (r *TrafficRepository) DeleteTemplate(ctx context.Context, id int64) error {
	if r == nil || r.db == nil {
		return errors.New("traffic repository not initialized")
	}

	if id <= 0 {
		return errors.New("template id is required")
	}

	result, err := r.db.ExecContext(ctx, `DELETE FROM templates WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete template: %w", err)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("get affected rows: %w", err)
	}

	if affected == 0 {
		return ErrTemplateNotFound
	}

	return nil
}

func scanTemplate(scanner rowScanner) (Template, error) {
	var t Template
	var useProxy, enableIncludeAll int

	if err := scanner.Scan(&t.ID, &t.Name, &t.Category, &t.TemplateURL, &t.RuleSource, &useProxy, &enableIncludeAll, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return Template{}, err
	}

	t.UseProxy = useProxy != 0
	t.EnableIncludeAll = enableIncludeAll != 0

	return t, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
