package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

type subscriptionAdminHandler struct {
	repo    *storage.TrafficRepository
	baseDir string
}

// NewSubscriptionAdminHandler returns an admin-only handler that manages subscription links.
func NewSubscriptionAdminHandler(baseDir string, repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("subscription admin handler requires repository")
	}

	if baseDir == "" {
		baseDir = filepath.FromSlash("subscribes")
	}

	return &subscriptionAdminHandler{
		repo:    repo,
		baseDir: filepath.Clean(baseDir),
	}
}

func (h *subscriptionAdminHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/subscriptions")
	path = strings.Trim(path, "/")

	switch {
	case path == "" && r.Method == http.MethodGet:
		h.handleList(w, r)
	case path == "" && r.Method == http.MethodPost:
		h.handleCreate(w, r)
	case path != "" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		h.handleUpdate(w, r, path)
	case path != "" && r.Method == http.MethodDelete:
		h.handleDelete(w, r, path)
	default:
		allowed := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
		methodNotAllowed(w, allowed...)
	}
}

func (h *subscriptionAdminHandler) handleList(w http.ResponseWriter, r *http.Request) {
	links, err := h.repo.ListSubscriptionLinks(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"subscriptions": convertSubscriptions(links),
	})
}

func (h *subscriptionAdminHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeBadRequest(w, "上传格式不正确")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	description := strings.TrimSpace(r.FormValue("description"))
	typ := strings.TrimSpace(r.FormValue("type"))
	buttons := r.MultipartForm.Value["buttons"]

	file, header, err := r.FormFile("rule_file")
	if err != nil {
		writeBadRequest(w, "规则文件是必填项")
		return
	}
	defer file.Close()

	filename, err := h.persistRuleFile(name, header, file, "")
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	link := storage.SubscriptionLink{
		Name:         name,
		Type:         typ,
		Description:  description,
		Buttons:      buttons,
		RuleFilename: filename,
	}

	created, err := h.repo.CreateSubscriptionLink(r.Context(), link)
	if err != nil {
		switch {
		case errors.Is(err, storage.ErrSubscriptionExists):
			writeError(w, http.StatusConflict, err)
		default:
			writeError(w, http.StatusBadRequest, err)
		}
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"subscription": convertSubscription(created),
	})
}

func (h *subscriptionAdminHandler) handleUpdate(w http.ResponseWriter, r *http.Request, idSegment string) {
	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的订阅标识")
		return
	}

	existing, err := h.repo.GetSubscriptionByID(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	if err := r.ParseMultipartForm(10 << 20); err != nil {
		writeBadRequest(w, "上传格式不正确")
		return
	}

	name := strings.TrimSpace(firstValue(r.MultipartForm.Value["name"], existing.Name))
	description := strings.TrimSpace(firstValue(r.MultipartForm.Value["description"], existing.Description))
	typ := strings.TrimSpace(firstValue(r.MultipartForm.Value["type"], existing.Type))
	buttons := r.MultipartForm.Value["buttons"]
	if len(buttons) == 0 {
		buttons = existing.Buttons
	}

	var filename = existing.RuleFilename
	var uploadedNewFile bool
	if header, err := fileHeader(r.MultipartForm.File["rule_file"]); err == nil {
		file, openErr := header.Open()
		if openErr != nil {
			writeError(w, http.StatusBadRequest, openErr)
			return
		}
		defer file.Close()

		persisted, persistErr := h.persistRuleFile(name, header, file, existing.RuleFilename)
		if persistErr != nil {
			writeError(w, http.StatusBadRequest, persistErr)
			return
		}
		filename = persisted
		uploadedNewFile = true
	}

	updated, err := h.repo.UpdateSubscriptionLink(r.Context(), storage.SubscriptionLink{
		ID:           existing.ID,
		Name:         name,
		Type:         typ,
		Description:  description,
		Buttons:      buttons,
		RuleFilename: filename,
	})
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			status = http.StatusNotFound
		} else if errors.Is(err, storage.ErrSubscriptionExists) {
			status = http.StatusConflict
		}
		writeError(w, status, err)
		return
	}

	if uploadedNewFile && filename != existing.RuleFilename {
		h.cleanupRuleFile(r.Context(), existing.RuleFilename)
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"subscription": convertSubscription(updated),
	})
}

func (h *subscriptionAdminHandler) handleDelete(w http.ResponseWriter, r *http.Request, idSegment string) {
	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的订阅标识")
		return
	}

	existing, err := h.repo.GetSubscriptionByID(r.Context(), id)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	if err := h.repo.DeleteSubscriptionLink(r.Context(), id); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrSubscriptionNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	h.cleanupRuleFile(r.Context(), existing.RuleFilename)

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *subscriptionAdminHandler) persistRuleFile(name string, header *multipart.FileHeader, src multipart.File, fallback string) (string, error) {
	if header == nil {
		return fallback, nil
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))
	if ext != ".yaml" && ext != ".yml" {
		return "", errors.New("仅支持 YAML 规则文件")
	}
	if ext == ".yml" {
		ext = ".yaml"
	}

	if header.Size > 10<<20 { // 10MB
		return "", errors.New("规则文件大小不可超过 10MB")
	}

	filename := buildRuleFilename(name, ext)
	if err := os.MkdirAll(h.baseDir, 0o755); err != nil {
		return "", fmt.Errorf("创建规则目录失败: %w", err)
	}

	destination := filepath.Join(h.baseDir, filename)
	if err := writeToFile(destination, src); err != nil {
		return "", err
	}

	return filename, nil
}

func (h *subscriptionAdminHandler) cleanupRuleFile(ctx context.Context, filename string) {
	filename = strings.TrimSpace(filename)
	if filename == "" {
		return
	}

	count, err := h.repo.CountSubscriptionsByFilename(ctx, filename)
	if err != nil || count > 0 {
		return
	}

	path := filepath.Join(h.baseDir, filename)
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
}

func writeToFile(path string, src multipart.File) error {
	out, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("保存规则文件失败: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, src); err != nil {
		return fmt.Errorf("写入规则文件失败: %w", err)
	}

	return nil
}

func buildRuleFilename(name, ext string) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "subscription"
	}
	runes := make([]rune, 0, len(base))
	for _, r := range base {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			runes = append(runes, r)
		case unicode.IsSpace(r):
			runes = append(runes, '-')
		case r == '-' || r == '_':
			runes = append(runes, r)
		}
	}
	if len(runes) == 0 {
		runes = []rune("subscription")
	}
	base = strings.Trim(strings.ToLower(string(runes)), "-")
	if base == "" {
		base = "subscription"
	}

	timestamp := time.Now().UnixNano()
	return fmt.Sprintf("%s-%d%s", base, timestamp, ext)
}

func fileHeader(headers []*multipart.FileHeader) (*multipart.FileHeader, error) {
	if len(headers) == 0 {
		return nil, errors.New("no file")
	}
	return headers[0], nil
}

func firstValue(values []string, fallback string) string {
	if len(values) == 0 {
		return fallback
	}
	return values[0]
}

type subscriptionDTO struct {
	ID           int64     `json:"id"`
	Name         string    `json:"name"`
	Type         string    `json:"type"`
	Description  string    `json:"description"`
	RuleFilename string    `json:"rule_filename"`
	Buttons      []string  `json:"buttons"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

func convertSubscription(link storage.SubscriptionLink) subscriptionDTO {
	return subscriptionDTO{
		ID:           link.ID,
		Name:         link.Name,
		Type:         link.Type,
		Description:  link.Description,
		RuleFilename: link.RuleFilename,
		Buttons:      append([]string(nil), link.Buttons...),
		CreatedAt:    link.CreatedAt,
		UpdatedAt:    link.UpdatedAt,
	}
}

func convertSubscriptions(links []storage.SubscriptionLink) []subscriptionDTO {
	result := make([]subscriptionDTO, 0, len(links))
	for _, link := range links {
		result = append(result, convertSubscription(link))
	}
	return result
}

// NewSubscriptionListHandler returns publicly accessible subscription metadata for authenticated users.
// For admin users, returns all subscriptions. For regular users, returns only subscriptions assigned to them.
func NewSubscriptionListHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("subscription list handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			methodNotAllowed(w, http.MethodGet)
			return
		}

		// Get username from context
		username := auth.UsernameFromContext(r.Context())
		if username == "" {
			writeError(w, http.StatusUnauthorized, errors.New("username not found in context"))
			return
		}

		// Get user to check role
		user, err := repo.GetUser(r.Context(), username)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		var files []storage.SubscribeFile

		// Admin users can see all subscriptions
		if user.Role == storage.RoleAdmin {
			files, err = repo.ListSubscribeFiles(r.Context())
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		} else {
			// Regular users can only see subscriptions assigned to them
			files, err = repo.GetUserSubscriptions(r.Context(), username)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
		}

		type item struct {
			ID            int64     `json:"id"`
			Name          string    `json:"name"`
			Description   string    `json:"description"`
			Filename      string    `json:"filename"`
			Type          string    `json:"type"`
			UpdatedAt     time.Time `json:"updated_at"`
			LatestVersion int64     `json:"latest_version,omitempty"`
		}

		payload := make([]item, 0, len(files))
		for _, file := range files {
			// Get latest version for this file
			var latestVersion int64
			if versions, err := repo.ListRuleVersions(r.Context(), file.Filename, 1); err == nil && len(versions) > 0 {
				latestVersion = versions[0].Version
			}

			payload = append(payload, item{
				ID:            file.ID,
				Name:          file.Name,
				Description:   file.Description,
				Filename:      file.Filename,
				Type:          file.Type,
				UpdatedAt:     file.UpdatedAt,
				LatestVersion: latestVersion,
			})
		}

		respondJSON(w, http.StatusOK, map[string]any{"subscriptions": payload})
	})
}
