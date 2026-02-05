package handler

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/substore"
	"miaomiaowu/internal/validator"

	"gopkg.in/yaml.v3"
)

type subscribeFilesHandler struct {
	repo *storage.TrafficRepository
}

// NewSubscribeFilesHandler returns an admin-only handler for managing subscribe files.
func NewSubscribeFilesHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("subscribe files handler requires repository")
	}

	return &subscribeFilesHandler{
		repo: repo,
	}
}

func (h *subscribeFilesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/subscribe-files")
	path = strings.Trim(path, "/")

	switch {
	case path == "" && r.Method == http.MethodGet:
		h.handleList(w, r)
	case path == "" && r.Method == http.MethodPost:
		h.handleCreate(w, r)
	case path == "import" && r.Method == http.MethodPost:
		h.handleImport(w, r)
	case path == "upload" && r.Method == http.MethodPost:
		h.handleUpload(w, r)
	case path == "create-from-config" && r.Method == http.MethodPost:
		h.handleCreateFromConfig(w, r)
	case strings.HasSuffix(path, "/content") && r.Method == http.MethodGet:
		// GET /api/admin/subscribe-files/{filename}/content
		filename := strings.TrimSuffix(path, "/content")
		h.handleGetContent(w, r, filename)
	case strings.HasSuffix(path, "/content") && r.Method == http.MethodPut:
		// PUT /api/admin/subscribe-files/{filename}/content
		filename := strings.TrimSuffix(path, "/content")
		h.handleUpdateContent(w, r, filename)
	case path != "" && path != "import" && path != "upload" && path != "create-from-config" && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		h.handleUpdate(w, r, path)
	case path != "" && path != "import" && path != "upload" && path != "create-from-config" && r.Method == http.MethodDelete:
		h.handleDelete(w, r, path)
	default:
		allowed := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
		methodNotAllowed(w, allowed...)
	}
}

func (h *subscribeFilesHandler) handleList(w http.ResponseWriter, r *http.Request) {
	files, err := h.repo.ListSubscribeFiles(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"files": h.convertSubscribeFilesWithVersions(r.Context(), files),
	})
}

func (h *subscribeFilesHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req subscribeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.Name == "" {
		writeBadRequest(w, "订阅名称是必填项")
		return
	}
	if req.URL == "" {
		writeBadRequest(w, "链接地址是必填项")
		return
	}
	if req.Type == "" {
		writeBadRequest(w, "类型是必填项")
		return
	}
	if req.Filename == "" {
		writeBadRequest(w, "文件名是必填项")
		return
	}

	file := storage.SubscribeFile{
		Name:        req.Name,
		Description: req.Description,
		URL:         req.URL,
		Type:        req.Type,
		Filename:    req.Filename,
	}

	expireAt, err := parseExpireAt(req.ExpireAt)
	if err != nil {
		writeBadRequest(w, "过期时间格式不正确，需为 RFC3339")
		return
	}
	file.ExpireAt = expireAt

	created, err := h.repo.CreateSubscribeFile(r.Context(), file)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Don't auto-apply custom rules for URL-based subscriptions
	// They will be applied when the subscription is first fetched

	respondJSON(w, http.StatusCreated, map[string]any{
		"file": convertSubscribeFile(created),
	})
}

func (h *subscribeFilesHandler) handleImport(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		URL         string `json:"url"`
		Filename    string `json:"filename"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.URL == "" {
		writeBadRequest(w, "订阅URL是必填项")
		return
	}
	if req.Name == "" {
		writeBadRequest(w, "订阅名称是必填项")
		return
	}

	// 创建HTTP客户端并获取订阅内容
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	httpReq, err := http.NewRequest("GET", req.URL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("无效的订阅URL"))
		return
	}

	// 添加User-Agent头
	httpReq.Header.Set("User-Agent", "clash-meta/2.4.0")

	resp, err := client.Do(httpReq)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("无法获取订阅内容: "+err.Error()))
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		writeError(w, http.StatusBadRequest, errors.New("订阅服务器返回错误状态"))
		return
	}

	// 读取响应内容
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("读取订阅内容失败"))
		return
	}

	// 验证YAML格式
	var yamlCheck map[string]any
	if err := yaml.Unmarshal(body, &yamlCheck); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("订阅内容不是有效的YAML格式"))
		return
	}

	// 从content-disposition获取文件名
	filename := req.Filename
	if filename == "" {
		contentDisposition := resp.Header.Get("Content-Disposition")
		if contentDisposition != "" {
			filename = parseFilenameFromContentDisposition(contentDisposition)
		}
		if filename == "" {
			filename = fmt.Sprintf("subscription_%d.yaml", time.Now().Unix())
		}
	}

	// 确保文件名有.yaml或.yml扩展名
	ext := filepath.Ext(filename)
	if ext != ".yaml" && ext != ".yml" {
		filename = filename + ".yaml"
	}

	// 保存文件到subscribes目录
	subscribesDir := "subscribes"
	if err := os.MkdirAll(subscribesDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("创建订阅目录失败"))
		return
	}

	filePath := filepath.Join(subscribesDir, filename)
	if err := os.WriteFile(filePath, body, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("保存订阅文件失败"))
		return
	}

	// 保存到数据库
	file := storage.SubscribeFile{
		Name:        req.Name,
		Description: req.Description,
		URL:         req.URL,
		Type:        storage.SubscribeTypeImport,
		Filename:    filename,
	}

	created, err := h.repo.CreateSubscribeFile(r.Context(), file)
	if err != nil {
		// 如果数据库保存失败，删除已保存的文件
		_ = os.Remove(filePath)
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Don't auto-apply custom rules for imported files
	// Users can manually enable auto-sync if needed

	respondJSON(w, http.StatusCreated, map[string]any{
		"file": convertSubscribeFile(created),
	})
}

func (h *subscribeFilesHandler) handleUpload(w http.ResponseWriter, r *http.Request) {
	// 解析multipart form
	if err := r.ParseMultipartForm(10 << 20); err != nil { // 10MB
		writeBadRequest(w, "解析表单失败")
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeBadRequest(w, "文件上传失败")
		return
	}
	defer file.Close()

	name := r.FormValue("name")
	if name == "" {
		name = strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
	}

	description := r.FormValue("description")
	filename := r.FormValue("filename")
	if filename == "" {
		filename = header.Filename
	}

	// 确保文件名有.yaml或.yml扩展名
	ext := filepath.Ext(filename)
	if ext != ".yaml" && ext != ".yml" {
		filename = filename + ".yaml"
	}

	// 读取并验证YAML格式
	content, err := io.ReadAll(file)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("读取文件失败"))
		return
	}

	var yamlCheck map[string]any
	if err := yaml.Unmarshal(content, &yamlCheck); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("文件不是有效的YAML格式"))
		return
	}

	// 保存文件到subscribes目录
	subscribesDir := "subscribes"
	if err := os.MkdirAll(subscribesDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("创建订阅目录失败"))
		return
	}

	filePath := filepath.Join(subscribesDir, filename)
	if err := os.WriteFile(filePath, content, 0644); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("保存订阅文件失败"))
		return
	}

	// 保存到数据库
	subscribeFile := storage.SubscribeFile{
		Name:        name,
		Description: description,
		URL:         "", // 上传的文件没有URL
		Type:        storage.SubscribeTypeUpload,
		Filename:    filename,
	}

	created, err := h.repo.CreateSubscribeFile(r.Context(), subscribeFile)
	if err != nil {
		// 如果数据库保存失败，删除已保存的文件
		_ = os.Remove(filePath)
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Don't auto-apply custom rules for uploaded files
	// Users can manually enable auto-sync if needed

	respondJSON(w, http.StatusCreated, map[string]any{
		"file": convertSubscribeFile(created),
	})
}

func (h *subscribeFilesHandler) handleUpdate(w http.ResponseWriter, r *http.Request, idSegment string) {
	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的订阅ID")
		return
	}

	existing, err := h.repo.GetSubscribeFileByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	var req subscribeFileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	// 更新字段
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.URL != "" {
		existing.URL = req.URL
	}
	if req.Type != "" {
		existing.Type = req.Type
	}
	// Update auto_sync_custom_rules if provided
	wasAutoSyncEnabled := existing.AutoSyncCustomRules
	if req.AutoSyncCustomRules != nil {
		existing.AutoSyncCustomRules = *req.AutoSyncCustomRules
	}
	// 更新模板绑定（绑定模板后禁用规则同步）
	templateJustBound := false
	if req.TemplateFilename != nil {
		existing.TemplateFilename = *req.TemplateFilename
		// 绑定模板后自动禁用规则同步，因为配置将由模板生成
		if *req.TemplateFilename != "" {
			existing.AutoSyncCustomRules = false
			templateJustBound = true
			logger.Info("[订阅更新] 绑定模板，已禁用规则同步", "subscribe_id", existing.ID, "template", *req.TemplateFilename)
		}
	}
	if req.ExpireAt != nil {
		expireAt, parseErr := parseExpireAt(req.ExpireAt)
		if parseErr != nil {
			writeBadRequest(w, "过期时间格式不正确，需为 RFC3339")
			return
		}
		existing.ExpireAt = expireAt
	} else {
		// 为空时清除过期时间
		existing.ExpireAt = nil
	}

	// 处理文件名更新
	oldFilename := existing.Filename
	needRenameFile := false
	if req.Filename != "" && req.Filename != existing.Filename {
		// 验证新文件名
		ext := filepath.Ext(req.Filename)
		if ext != ".yaml" && ext != ".yml" {
			writeError(w, http.StatusBadRequest, errors.New("文件名必须以 .yaml 或 .yml 结尾"))
			return
		}

		// 检查新文件名是否已被其他订阅使用
		if existingFile, err := h.repo.GetSubscribeFileByFilename(r.Context(), req.Filename); err == nil && existingFile.ID != id {
			writeError(w, http.StatusConflict, errors.New("文件名已被其他订阅使用"))
			return
		}

		existing.Filename = req.Filename
		needRenameFile = true
	}

	updated, err := h.repo.UpdateSubscribeFile(r.Context(), existing)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// 如果文件名发生变化，重命名物理文件
	if needRenameFile {
		oldPath := filepath.Join("subscribes", oldFilename)
		newPath := filepath.Join("subscribes", req.Filename)

		// 检查旧文件是否存在
		if _, err := os.Stat(oldPath); err == nil {
			// 重命名文件
			if err := os.Rename(oldPath, newPath); err != nil {
				// 重命名失败，回滚数据库更新
				existing.Filename = oldFilename
				_, _ = h.repo.UpdateSubscribeFile(r.Context(), existing)
				writeError(w, http.StatusInternalServerError, errors.New("重命名文件失败: "+err.Error()))
				return
			}
		}
		// 如果旧文件不存在，只更新数据库记录，不报错
	}

	// If auto_sync was just enabled (changed from false to true), trigger immediate sync
	if !wasAutoSyncEnabled && updated.AutoSyncCustomRules {
		go func() {
			addedGroups, err := syncCustomRulesToFile(context.Background(), h.repo, updated)
			if err != nil {
				logger.Info("[AutoSync] 同步自定义规则失败", "filename", updated.Filename, "id", updated.ID, "error", err)
			} else {
				logger.Info("[AutoSync] 同步自定义规则成功", "filename", updated.Filename, "id", updated.ID)
				if len(addedGroups) > 0 {
					logger.Info("[AutoSync] 添加的代理组", "groups", addedGroups)
				}
			}
		}()
	}

	// 如果绑定了V3模板，从模板重新生成订阅文件
	if templateJustBound && updated.TemplateFilename != "" {
		go func() {
			ctx := context.Background()
			username := auth.UsernameFromContext(r.Context())
			if username == "" {
				logger.Info("[模板生成] 无法获取用户名，跳过模板生成", "subscribe_id", updated.ID)
				return
			}
			if err := h.regenerateFromTemplate(ctx, username, updated); err != nil {
				logger.Info("[模板生成] 生成失败", "subscribe_id", updated.ID, "template", updated.TemplateFilename, "error", err)
			} else {
				logger.Info("[模板生成] 生成成功", "subscribe_id", updated.ID, "template", updated.TemplateFilename)
			}
		}()
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"file": convertSubscribeFile(updated),
	})
}

func (h *subscribeFilesHandler) handleDelete(w http.ResponseWriter, r *http.Request, idSegment string) {
	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的订阅ID")
		return
	}

	// 获取文件信息以便删除物理文件
	file, err := h.repo.GetSubscribeFileByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 删除数据库记录
	if err := h.repo.DeleteSubscribeFile(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 删除物理文件
	filePath := filepath.Join("subscribes", file.Filename)
	_ = os.Remove(filePath) // 忽略错误，即使文件不存在也继续

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// parseFilenameFromContentDisposition 从Content-Disposition头解析文件名
// 支持格式: attachment;filename*=UTF-8”%E6%B3%A1%E6%B3%A1Dog
func parseFilenameFromContentDisposition(header string) string {
	// 查找 filename*= 部分
	if idx := strings.Index(header, "filename*="); idx != -1 {
		// 提取等号后的内容
		value := header[idx+10:]
		// 查找两个单引号后的内容
		if idx2 := strings.LastIndex(value, "''"); idx2 != -1 {
			encoded := value[idx2+2:]
			// URL解码
			if decoded, err := url.QueryUnescape(encoded); err == nil {
				return decoded
			}
		}
	}

	// 如果没有filename*=，尝试filename=
	if idx := strings.Index(header, "filename="); idx != -1 {
		value := header[idx+9:]
		value = strings.Trim(value, `"`)
		if idx2 := strings.IndexAny(value, ";,"); idx2 != -1 {
			value = value[:idx2]
		}
		return strings.TrimSpace(value)
	}

	return ""
}

type subscribeFileRequest struct {
	Name                string  `json:"name"`
	Description         string  `json:"description"`
	URL                 string  `json:"url"`
	Type                string  `json:"type"`
	Filename            string  `json:"filename"`
	AutoSyncCustomRules *bool   `json:"auto_sync_custom_rules,omitempty"` // Pointer to distinguish between false and not provided
	TemplateFilename    *string `json:"template_filename,omitempty"`      // 绑定的 V3 模板文件名
	ExpireAt            *string `json:"expire_at,omitempty"`
}

type subscribeFileDTO struct {
	ID                  int64      `json:"id"`
	Name                string     `json:"name"`
	Description         string     `json:"description"`
	Type                string     `json:"type"`
	Filename            string     `json:"filename"`
	ExpireAt            *time.Time `json:"expire_at,omitempty"`
	AutoSyncCustomRules bool       `json:"auto_sync_custom_rules"`
	TemplateFilename    string     `json:"template_filename"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
	LatestVersion       int64      `json:"latest_version,omitempty"`
}

func convertSubscribeFile(file storage.SubscribeFile) subscribeFileDTO {
	return subscribeFileDTO{
		ID:                  file.ID,
		Name:                file.Name,
		Description:         file.Description,
		Type:                file.Type,
		Filename:            file.Filename,
		ExpireAt:            file.ExpireAt,
		AutoSyncCustomRules: file.AutoSyncCustomRules,
		TemplateFilename:    file.TemplateFilename,
		CreatedAt:           file.CreatedAt,
		UpdatedAt:           file.UpdatedAt,
	}
}

func convertSubscribeFiles(files []storage.SubscribeFile) []subscribeFileDTO {
	result := make([]subscribeFileDTO, 0, len(files))
	for _, file := range files {
		result = append(result, convertSubscribeFile(file))
	}
	return result
}

func (h *subscribeFilesHandler) convertSubscribeFilesWithVersions(ctx context.Context, files []storage.SubscribeFile) []subscribeFileDTO {
	result := make([]subscribeFileDTO, 0, len(files))
	for _, file := range files {
		dto := convertSubscribeFile(file)

		// 获取最新版本号
		if versions, err := h.repo.ListRuleVersions(ctx, file.Filename, 1); err == nil && len(versions) > 0 {
			dto.LatestVersion = versions[0].Version
		}

		result = append(result, dto)
	}
	return result
}

func parseExpireAt(raw *string) (*time.Time, error) {
	if raw == nil {
		return nil, nil
	}
	value := strings.TrimSpace(*raw)
	if value == "" {
		return nil, nil
	}
	// Try RFC3339 first (without milliseconds)
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		// Fallback to RFC3339Nano (with milliseconds/nanoseconds)
		parsed, err = time.Parse(time.RFC3339Nano, value)
		if err != nil {
			return nil, err
		}
	}
	return &parsed, nil
}

// handleCreateFromConfig 保存生成的配置为订阅文件
func (h *subscribeFilesHandler) handleCreateFromConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Filename    string `json:"filename"`
		Content     string `json:"content"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.Name == "" {
		writeBadRequest(w, "订阅名称是必填项")
		return
	}
	if req.Content == "" {
		writeBadRequest(w, "配置内容不能为空")
		return
	}

	// 获取当前用户名和设置，判断是否需要校验
	username := auth.UsernameFromContext(r.Context())
	shouldValidate := true // 默认进行校验
	if username != "" {
		// 获取用户设置
		settings, err := h.repo.GetUserSettings(r.Context(), username)
		if err == nil {
			// 只有在使用v2模板系统时才进行校验
			shouldValidate = settings.TemplateVersion == "v2"
			logger.Info("[创建订阅文件] 用户设置", "username", username, "template_version", settings.TemplateVersion, "should_validate", shouldValidate)
		} else if !errors.Is(err, storage.ErrUserSettingsNotFound) {
			logger.Info("[创建订阅文件] 获取用户设置失败，使用默认行为(进行校验)", "username", username, "error", err)
		}
	}

	// 设置默认文件名
	filename := req.Filename
	if filename == "" {
		filename = req.Name
	}

	// 确保文件名有.yaml或.yml扩展名
	ext := filepath.Ext(filename)
	if ext != ".yaml" && ext != ".yml" {
		filename = filename + ".yaml"
	}

	// 验证YAML格式，使用Node API保持顺序和格式
	var rootNode yaml.Node
	if err := yaml.Unmarshal([]byte(req.Content), &rootNode); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("配置内容不是有效的YAML格式"))
		return
	}

	// 只有在使用新模板系统时才进行配置校验
	if shouldValidate {
		// 校验配置内容
		var configMap map[string]interface{}
		var tempBuf bytes.Buffer
		tempEncoder := yaml.NewEncoder(&tempBuf)
		tempEncoder.SetIndent(2)
		if err := tempEncoder.Encode(&rootNode); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("编码配置用于校验失败"))
			return
		}
		if err := yaml.Unmarshal(tempBuf.Bytes(), &configMap); err != nil {
			writeError(w, http.StatusInternalServerError, errors.New("解析配置用于校验失败"))
			return
		}

		validationResult := validator.ValidateClashConfig(configMap)
		if !validationResult.Valid {
			logger.Info("[创建订阅文件] [配置校验] 校验失败", "filename", filename)
			var errorMessages []string
			for _, issue := range validationResult.Issues {
				if issue.Level == validator.ErrorLevel {
					errorMsg := issue.Message
					if issue.Location != "" {
						errorMsg = fmt.Sprintf("%s (位置: %s)", errorMsg, issue.Location)
					}
					errorMessages = append(errorMessages, errorMsg)
					logger.Info("[创建订阅文件] [配置校验] 错误", "message", errorMsg)
				}
			}
			writeError(w, http.StatusBadRequest, errors.New("配置校验失败: "+strings.Join(errorMessages, "; ")))
			return
		}

		// 如果有自动修复，使用修复后的配置
		if validationResult.FixedConfig != nil {
			fixedYAML, err := yaml.Marshal(validationResult.FixedConfig)
			if err != nil {
				writeError(w, http.StatusInternalServerError, errors.New("序列化修复配置失败"))
				return
			}
			if err := yaml.Unmarshal(fixedYAML, &rootNode); err != nil {
				writeError(w, http.StatusInternalServerError, errors.New("解析修复配置失败"))
				return
			}

			// 记录自动修复的警告
			for _, issue := range validationResult.Issues {
				if issue.Level == validator.WarningLevel && issue.AutoFixed {
					logger.Info("[创建订阅文件] [配置校验] 警告(已修复)", "message", issue.Message, "location", issue.Location)
				}
			}
		}
	} else {
		logger.Info("[创建订阅文件] 使用旧模板系统，跳过配置校验", "filename", filename)
	}

	// 修复short-id字段，确保使用双引号
	// fixShortIdStyleInNode(&rootNode)

	// 重新序列化YAML，保持原有顺序和格式
	reserializedContent, err := MarshalYAMLWithIndent(&rootNode)
	if err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("处理YAML内容失败"))
		return
	}

	// Fix emoji/backslash escapes
	fixedContent := RemoveUnicodeEscapeQuotes(string(reserializedContent))

	// 保存文件到subscribes目录
	subscribesDir := "subscribes"
	if err := os.MkdirAll(subscribesDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("创建订阅目录失败"))
		return
	}

	filePath := filepath.Join(subscribesDir, filename)
	if err := os.WriteFile(filePath, []byte(fixedContent), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("保存订阅文件失败"))
		return
	}

	// 保存到数据库
	file := storage.SubscribeFile{
		Name:        req.Name,
		Description: req.Description,
		URL:         "",
		Type:        storage.SubscribeTypeCreate,
		Filename:    filename,
	}

	created, err := h.repo.CreateSubscribeFile(r.Context(), file)
	if err != nil {
		// 如果数据库保存失败，删除已保存的文件
		_ = os.Remove(filePath)
		if errors.Is(err, storage.ErrSubscribeFileExists) {
			writeError(w, http.StatusConflict, errors.New("订阅名称已存在"))
			return
		}
		writeError(w, http.StatusBadRequest, err)
		return
	}

	// Initialize custom rule application records to prevent duplicates on first modification
	h.initializeCustomRuleApplications(r.Context(), created.ID)

	// 同步 MMW 模式代理集合的节点到配置文件
	// 使用 goroutine 异步执行，不阻塞响应
	go h.syncMMWProxyProvidersToFile(subscribesDir, filename)

	respondJSON(w, http.StatusCreated, map[string]any{
		"file": convertSubscribeFile(created),
	})
}

// handleGetContent 获取订阅文件内容
func (h *subscribeFilesHandler) handleGetContent(w http.ResponseWriter, r *http.Request, filename string) {
	if filename == "" {
		writeBadRequest(w, "文件名不能为空")
		return
	}

	// 验证文件名
	filename, err := url.QueryUnescape(filename)
	if err != nil {
		writeBadRequest(w, "无效的文件名")
		return
	}

	// 检查文件是否存在于数据库
	_, err = h.repo.GetSubscribeFileByFilename(r.Context(), filename)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, errors.New("订阅文件不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 读取文件内容
	filePath := filepath.Join("subscribes", filename)
	content, err := os.ReadFile(filePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			writeError(w, http.StatusNotFound, errors.New("文件不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, errors.New("读取文件失败"))
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"content": string(content),
	})
}

// handleUpdateContent 更新订阅文件内容
func (h *subscribeFilesHandler) handleUpdateContent(w http.ResponseWriter, r *http.Request, filename string) {
	if filename == "" {
		writeBadRequest(w, "文件名不能为空")
		return
	}

	// 验证文件名
	filename, err := url.QueryUnescape(filename)
	if err != nil {
		writeBadRequest(w, "无效的文件名")
		return
	}

	// 检查文件是否存在于数据库
	subscribeFile, err := h.repo.GetSubscribeFileByFilename(r.Context(), filename)
	if err != nil {
		if errors.Is(err, storage.ErrSubscribeFileNotFound) {
			writeError(w, http.StatusNotFound, errors.New("订阅文件不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 解析请求体
	var req struct {
		Content string `json:"content"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.Content == "" {
		writeBadRequest(w, "内容不能为空")
		return
	}

	// 验证YAML格式，使用 Node API 保持顺序
	var rootNode yaml.Node
	if err := yaml.Unmarshal([]byte(req.Content), &rootNode); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("内容不是有效的YAML格式: "+err.Error()))
		return
	}

	// 转换为 map 进行基本校验（只检查错误，不做修复）
	var yamlCheck map[string]any
	if err := yaml.Unmarshal([]byte(req.Content), &yamlCheck); err != nil {
		writeError(w, http.StatusBadRequest, errors.New("内容不是有效的YAML格式: "+err.Error()))
		return
	}

	// 校验配置内容
	validationResult := validator.ValidateClashConfig(yamlCheck)
	if !validationResult.Valid {
		logger.Info("[更新订阅文件] [配置校验] 校验失败", "filename", filename)
		var errorMessages []string
		for _, issue := range validationResult.Issues {
			if issue.Level == validator.ErrorLevel {
				errorMsg := issue.Message
				if issue.Location != "" {
					errorMsg = fmt.Sprintf("%s (位��: %s)", errorMsg, issue.Location)
				}
				errorMessages = append(errorMessages, errorMsg)
				logger.Info("[更新订阅文件] [配置校验] 错误", "message", errorMsg)
			}
		}
		writeError(w, http.StatusBadRequest, errors.New("配置校验失败: "+strings.Join(errorMessages, "; ")))
		return
	}

	// 直接保存前端发送的内容（已经过前端修复，保持字段顺序）
	contentToSave := RemoveUnicodeEscapeQuotes(req.Content)

	// 记录警告信息（如果有）
	for _, issue := range validationResult.Issues {
		if issue.Level == validator.WarningLevel {
			logger.Info("[更新订阅文件] [配置校验] 警告(前端已修复)", "message", issue.Message, "location", issue.Location)
		}
	}

	// 保存文件
	filePath := filepath.Join("subscribes", filename)
	if err := os.WriteFile(filePath, []byte(contentToSave), 0644); err != nil {
		writeError(w, http.StatusInternalServerError, errors.New("保存文件失败"))
		return
	}

	// 保存版本记录
	version, err := h.repo.SaveRuleVersion(r.Context(), filename, contentToSave, "admin")
	if err != nil {
		// 版本保存失败不影响文件保存，只记录错误
		writeError(w, http.StatusInternalServerError, errors.New("保存版本记录失败"))
		return
	}

	// 更新数据库中的updated_at字段
	subscribeFile.UpdatedAt = time.Now()
	_, err = h.repo.UpdateSubscribeFile(r.Context(), subscribeFile)
	if err != nil {
		// 更新时间戳失败不影响文件保存，只记录错误
		writeError(w, http.StatusInternalServerError, errors.New("更新订阅信息失败"))
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "updated",
		"version": version,
	})
}

// initializeCustomRuleApplications records the initial custom rule application state for a newly created subscribe file.
// This is called when a file is created from the generator page where custom rules are already included in the content.
// We only record the application state, not re-apply the rules (which would duplicate them).
func (h *subscribeFilesHandler) initializeCustomRuleApplications(ctx context.Context, fileID int64) {
	// Get all enabled custom rules to record their current state
	rules, err := h.repo.ListEnabledCustomRules(ctx, "")
	if err != nil {
		logger.Info("[Subscribe] 获取自定义规则失败", "error", err)
		return
	}

	if len(rules) == 0 {
		return
	}

	// Record each rule's current state without modifying the file
	for _, rule := range rules {
		// Calculate content hash for tracking future changes
		hash := sha256.Sum256([]byte(rule.Content))
		contentHash := hex.EncodeToString(hash[:])

		// Parse the rule content to extract the actual rules/providers that were applied
		// This must match the format used in applyRulesRule and applyRuleProvidersRule
		var appliedContent string
		if rule.Type == "rules" {
			// Parse rule content to get the array of rules
			var newRules []interface{}

			// Try to parse as map first (with "rules:" key)
			var parsedAsMap map[string]interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &parsedAsMap); err == nil {
				if rulesValue, hasRulesKey := parsedAsMap["rules"]; hasRulesKey {
					if rulesArray, ok := rulesValue.([]interface{}); ok {
						newRules = rulesArray
					}
				}
			}

			// Try to parse as YAML array
			if len(newRules) == 0 {
				if err := yaml.Unmarshal([]byte(rule.Content), &newRules); err != nil {
					// Parse as plain text
					lines := strings.Split(rule.Content, "\n")
					for _, line := range lines {
						line = strings.TrimSpace(line)
						if line != "" && !strings.HasPrefix(line, "#") {
							newRules = append(newRules, line)
						}
					}
				}
			}

			// Serialize to JSON format (same as applyRulesRule does)
			if len(newRules) > 0 {
				appliedJSON, _ := json.Marshal(newRules)
				appliedContent = string(appliedJSON)
			}
		} else if rule.Type == "rule-providers" {
			// Parse rule-providers content
			var parsedContent map[string]interface{}
			if err := yaml.Unmarshal([]byte(rule.Content), &parsedContent); err == nil {
				var providersMap map[string]interface{}
				if providersValue, hasProvidersKey := parsedContent["rule-providers"]; hasProvidersKey {
					if pm, ok := providersValue.(map[string]interface{}); ok {
						providersMap = pm
					}
				} else {
					providersMap = parsedContent
				}

				// Serialize to JSON format
				if len(providersMap) > 0 {
					appliedJSON, _ := json.Marshal(providersMap)
					appliedContent = string(appliedJSON)
				}
			}
		} else if rule.Type == "dns" {
			// For DNS rules, we don't track applied content
			appliedContent = ""
		}

		app := &storage.CustomRuleApplication{
			SubscribeFileID: fileID,
			CustomRuleID:    rule.ID,
			RuleType:        rule.Type,
			RuleMode:        rule.Mode,
			AppliedContent:  appliedContent,
			ContentHash:     contentHash,
		}

		if err := h.repo.UpsertCustomRuleApplication(ctx, app); err != nil {
			logger.Info("[Subscribe] 记录自定义规则应用失败", "rule_id", rule.ID, "error", err)
		}
	}

	logger.Info("[Subscribe] 记录自定义规则应用状态完成", "rule_count", len(rules), "file_id", fileID)
}

// syncMMWProxyProvidersToFile 同步 MMW 模式代理集合的节点到指定文件
// 保存配置文件后调用，将 proxy-groups 中 use 引用的 MMW 模式代理集合节点直接写入配置
func (h *subscribeFilesHandler) syncMMWProxyProvidersToFile(subscribeDir, filename string) {
	SyncMMWProxyProvidersToFile(h.repo, subscribeDir, filename)
}

// SyncMMWProxyProvidersToFile 同步 MMW 模式代理集合的节点到指定文件（公共版本）
// 可由 subscription.go 调用，确保获取订阅时包含最新的代理集合节点
func SyncMMWProxyProvidersToFile(repo *storage.TrafficRepository, subscribeDir, filename string) {
	filePath := filepath.Join(subscribeDir, filename)

	// 1. 读取刚保存的 YAML 文件
	content, err := os.ReadFile(filePath)
	if err != nil {
		logger.Info("[MMW同步] 读取文件失败", "error", err)
		return
	}

	// 2. 解析 YAML
	var rootNode yaml.Node
	if err := yaml.Unmarshal(content, &rootNode); err != nil {
		logger.Info("[MMW同步] 解析YAML失败", "error", err)
		return
	}

	// 3. 查找 proxy-groups，收集 use 引用的代理集合名称
	providerNames := collectUsedProviderNames(&rootNode)
	if len(providerNames) == 0 {
		return
	}

	// 获取现有节点数量用于比较
	existingNodes := collectExistingProxyNodes(&rootNode)
	logger.Info("[MMW同步] 文件使用代理集合", "filename", filename, "count", len(providerNames), "providers", providerNames, "existing_nodes", len(existingNodes))

	ctx := context.Background()
	syncedCount := 0

	// 4. 根据名称查找代理集合配置，筛选 MMW 模式
	for _, providerName := range providerNames {
		config, err := repo.GetProxyProviderConfigByName(ctx, providerName)
		if err != nil {
			logger.Info("[MMW同步] 查询代理集合配置失败", "provider_name", providerName, "error", err)
			continue
		}
		if config == nil {
			continue
		}
		if config.ProcessMode != "mmw" {
			continue
		}

		// 5. 从缓存获取节点数据
		cache := GetProxyProviderCache()
		entry, ok := cache.Get(config.ID)
		if !ok || cache.IsExpired(entry) {
			// 缓存不存在或过期，尝试刷新
			sub, err := repo.GetExternalSubscription(ctx, config.ExternalSubscriptionID, config.Username)
			if err != nil || sub.ID == 0 {
				logger.Info("[MMW同步] 获取代理集合的外部订阅失败", "provider_name", providerName, "error", err)
				continue
			}
			entry, err = RefreshProxyProviderCache(&sub, config)
			if err != nil {
				logger.Info("[MMW同步] 刷新代理集合缓存失败", "provider_name", providerName, "error", err)
				continue
			}
		}

		if len(entry.Nodes) == 0 {
			logger.Info("[MMW同步] 代理集合没有节点", "provider_name", providerName)
			continue
		}

		// 6. 为节点添加前缀（使用名称前缀，即第一个 - 之前的部分）
		namePrefix := config.Name
		if idx := strings.Index(config.Name, "-"); idx > 0 {
			namePrefix = config.Name[:idx]
		}
		prefix := fmt.Sprintf("〖%s〗", namePrefix)

		// 复制节点并添加前缀
		proxiesRaw := make([]any, len(entry.Nodes))
		nodeNames := make([]string, 0, len(entry.Nodes))
		for i, node := range entry.Nodes {
			nodeCopy := copyMap(node.(map[string]any))
			if name, ok := nodeCopy["name"].(string); ok {
				newName := prefix + name
				nodeCopy["name"] = newName
				nodeNames = append(nodeNames, newName)
			}
			proxiesRaw[i] = nodeCopy
		}

		// 7. 调用已有的同步函数写入节点
		if err := updateYAMLFileWithProxyProviderNodes(subscribeDir, filename, config.Name, prefix, proxiesRaw, nodeNames); err != nil {
			logger.Info("[MMW同步] 更新文件失败", "filename", filename, "error", err)
			continue
		}

		// 记录同步完成（详细的 old_count/new_count 日志已在 external_sync.go 中输出）
		logger.Info("[MMW同步] 代理集合同步完成", "provider_name", providerName, "node_count", len(nodeNames))

		syncedCount++
	}

	if syncedCount > 0 {
		logger.Info("[MMW同步] 文件同步完成", "filename", filename, "synced_count", syncedCount)
	}
}

// collectExistingProxyNodes 从 YAML 中收集现有的 proxies 节点名称
func collectExistingProxyNodes(rootNode *yaml.Node) []string {
	nodeNames := make([]string, 0)

	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return nodeNames
	}

	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return nodeNames
	}

	// 查找 proxies 节点
	var proxiesNode *yaml.Node
	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "proxies" {
			proxiesNode = valueNode
			break
		}
	}

	if proxiesNode == nil || proxiesNode.Kind != yaml.SequenceNode {
		return nodeNames
	}

	// 遍历 proxies，收集 name 字段
	for _, proxyNode := range proxiesNode.Content {
		if proxyNode.Kind != yaml.MappingNode {
			continue
		}
		for i := 0; i < len(proxyNode.Content)-1; i += 2 {
			keyNode := proxyNode.Content[i]
			valueNode := proxyNode.Content[i+1]
			if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "name" && valueNode.Kind == yaml.ScalarNode {
				nodeNames = append(nodeNames, valueNode.Value)
				break
			}
		}
	}

	return nodeNames
}

// collectUsedProviderNames 从 YAML 中收集所有 proxy-groups 的代理集合引用
// 支持两种模式：
// 1. 客户端模式：从 use 字段收集 provider 名称
// 2. 妙妙屋模式：从 proxy-group 的 name 字段收集（MMW模式下 proxy-group 名称与代理集合名称相同）
func collectUsedProviderNames(rootNode *yaml.Node) []string {
	providerNames := make([]string, 0)
	seen := make(map[string]bool)

	if rootNode.Kind != yaml.DocumentNode || len(rootNode.Content) == 0 {
		return providerNames
	}

	docContent := rootNode.Content[0]
	if docContent.Kind != yaml.MappingNode {
		return providerNames
	}

	// 查找 proxy-groups 节点
	var proxyGroupsNode *yaml.Node
	for i := 0; i < len(docContent.Content)-1; i += 2 {
		keyNode := docContent.Content[i]
		valueNode := docContent.Content[i+1]
		if keyNode.Kind == yaml.ScalarNode && keyNode.Value == "proxy-groups" {
			proxyGroupsNode = valueNode
			break
		}
	}

	if proxyGroupsNode == nil || proxyGroupsNode.Kind != yaml.SequenceNode {
		return providerNames
	}

	// 遍历 proxy-groups
	for _, groupNode := range proxyGroupsNode.Content {
		if groupNode.Kind != yaml.MappingNode {
			continue
		}

		var groupName string
		var hasUse bool

		for i := 0; i < len(groupNode.Content)-1; i += 2 {
			keyNode := groupNode.Content[i]
			valueNode := groupNode.Content[i+1]

			if keyNode.Kind == yaml.ScalarNode {
				switch keyNode.Value {
				case "name":
					if valueNode.Kind == yaml.ScalarNode {
						groupName = valueNode.Value
					}
				case "use":
					hasUse = true
					// 客户端模式：收集 use 字段的值
					if valueNode.Kind == yaml.SequenceNode {
						for _, useItem := range valueNode.Content {
							if useItem.Kind == yaml.ScalarNode && useItem.Value != "" {
								if !seen[useItem.Value] {
									seen[useItem.Value] = true
									providerNames = append(providerNames, useItem.Value)
								}
							}
						}
					}
				}
			}
		}

		// 妙妙屋模式：如果没有 use 字段，使用 proxy-group 的 name
		if !hasUse && groupName != "" && !seen[groupName] {
			seen[groupName] = true
			providerNames = append(providerNames, groupName)
		}
	}

	return providerNames
}

// copyMap 深拷贝 map
func copyMap(m map[string]any) map[string]any {
	result := make(map[string]any)
	for k, v := range m {
		switch vv := v.(type) {
		case map[string]any:
			result[k] = copyMap(vv)
		case []any:
			newSlice := make([]any, len(vv))
			copy(newSlice, vv)
			result[k] = newSlice
		default:
			result[k] = v
		}
	}
	return result
}

// regenerateFromTemplate 从V3模板重新生成订阅文件
func (h *subscribeFilesHandler) regenerateFromTemplate(ctx context.Context, username string, subscribeFile storage.SubscribeFile) error {
	if subscribeFile.TemplateFilename == "" {
		return errors.New("订阅未绑定模板")
	}

	// 1. 读取模板文件
	templatePath := filepath.Join("rule_templates", subscribeFile.TemplateFilename)
	templateContent, err := os.ReadFile(templatePath)
	if err != nil {
		return fmt.Errorf("读取模板文件失败: %w", err)
	}
	logger.Info("[模板生成] 读取模板文件", "template", subscribeFile.TemplateFilename, "bytes", len(templateContent))

	// 2. 从节点表获取用户的所有代理节点
	nodes, err := h.repo.ListNodes(ctx, username)
	if err != nil {
		return fmt.Errorf("获取节点列表失败: %w", err)
	}

	// 将节点转换为 proxies 格式（[]map[string]any）
	var proxies []map[string]any
	for _, node := range nodes {
		if !node.Enabled {
			continue // 跳过禁用的节点
		}
		// ClashConfig 是 JSON 格式的字符串，需要解析
		var proxyConfig map[string]any
		if err := json.Unmarshal([]byte(node.ClashConfig), &proxyConfig); err != nil {
			logger.Info("[模板生成] 解析节点配置失败，跳过", "node", node.NodeName, "error", err)
			continue
		}
		// 确保节点名称正确（使用数据库中的名称）
		proxyConfig["name"] = node.NodeName
		proxies = append(proxies, proxyConfig)
	}
	logger.Info("[模板生成] 从节点表获取代理节点", "total", len(nodes), "enabled", len(proxies))

	// 3. 从代理集合表获取用户的代理集合配置（用于 proxy-providers）
	providerConfigs, err := h.repo.ListProxyProviderConfigs(ctx, username)
	if err != nil {
		logger.Info("[模板生成] 获取代理集合配置失败", "error", err)
		// 不是致命错误，继续处理
	}

	// 构建 providers map：provider name -> proxy names
	providers := make(map[string][]string)
	for _, config := range providerConfigs {
		// 获取该代理集合下的所有节点名称
		// 通过 tag 字段匹配：节点的 tag 等于代理集合的名称
		var providerProxyNames []string
		for _, node := range nodes {
			if node.Enabled && node.Tag == config.Name {
				providerProxyNames = append(providerProxyNames, node.NodeName)
			}
		}
		if len(providerProxyNames) > 0 {
			providers[config.Name] = providerProxyNames
		}
	}
	logger.Info("[模板生成] 从代理集合表获取代理集合", "count", len(providerConfigs), "with_nodes", len(providers))

	// 4. 使用 TemplateV3Processor 处理模板
	processor := substore.NewTemplateV3Processor(nil, providers)
	result, err := processor.ProcessTemplate(string(templateContent), proxies)
	if err != nil {
		return fmt.Errorf("处理模板失败: %w", err)
	}

	// 5. 注入代理节点到proxies字段（与预览保持一致）
	result, err = injectProxiesIntoTemplate(result, proxies)
	if err != nil {
		return fmt.Errorf("注入代理节点失败: %w", err)
	}

	// 6. 写入订阅文件
	subscribePath := filepath.Join("subscribes", subscribeFile.Filename)
	if err := os.WriteFile(subscribePath, []byte(result), 0644); err != nil {
		return fmt.Errorf("写入订阅文件失败: %w", err)
	}

	logger.Info("[模板生成] 模板处理完成", "subscribe", subscribeFile.Name, "template", subscribeFile.TemplateFilename, "result_bytes", len(result))
	return nil
}

// RefreshAllTemplateSubscriptions 刷新所有绑定了模板的订阅
// 当节点发生变化（新增、删除、修改）时调用此函数
func RefreshAllTemplateSubscriptions(repo *storage.TrafficRepository, username string) {
	ctx := context.Background()

	// 获取所有绑定了模板的订阅
	files, err := repo.GetSubscribeFilesWithTemplate(ctx)
	if err != nil {
		logger.Info("[模板刷新] 获取绑定模板的订阅失败", "error", err)
		return
	}

	if len(files) == 0 {
		logger.Info("[模板刷新] 没有绑定模板的订阅需要刷新")
		return
	}

	logger.Info("[模板刷新] 开始刷新绑定模板的订阅", "count", len(files))

	// 创建临时 handler 用于调用 regenerateFromTemplate
	h := &subscribeFilesHandler{repo: repo}

	successCount := 0
	for _, file := range files {
		if err := h.regenerateFromTemplate(ctx, username, file); err != nil {
			logger.Info("[模板刷新] 刷新订阅失败", "subscribe", file.Name, "template", file.TemplateFilename, "error", err)
		} else {
			logger.Info("[模板刷新] 刷新订阅成功", "subscribe", file.Name, "template", file.TemplateFilename)
			successCount++
		}
	}

	logger.Info("[模板刷新] 刷新完成", "total", len(files), "success", successCount)
}
