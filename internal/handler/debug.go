package handler

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
)

type debugHandler struct {
	repo       *storage.TrafficRepository
	logManager *logger.LogManager
}

// NewDebugHandler 创建debug日志handler
func NewDebugHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("debug handler requires repository")
	}

	return &debugHandler{
		repo:       repo,
		logManager: logger.NewLogManager("data/logs"),
	}
}

func (h *debugHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if strings.TrimSpace(username) == "" {
		writeError(w, http.StatusUnauthorized, errors.New("unauthorized"))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/api/user/debug")
	path = strings.Trim(path, "/")

	switch {
	case path == "enable" && r.Method == http.MethodPost:
		h.handleEnable(w, r, username)
	case path == "disable" && r.Method == http.MethodPost:
		h.handleDisable(w, r, username)
	case path == "status" && r.Method == http.MethodGet:
		h.handleStatus(w, r, username)
	case path == "download" && r.Method == http.MethodGet:
		h.handleDownload(w, r, username)
	default:
		allowed := []string{http.MethodGet, http.MethodPost}
		methodNotAllowed(w, allowed...)
	}
}

// handleEnable 开启debug日志
func (h *debugHandler) handleEnable(w http.ResponseWriter, r *http.Request, username string) {
	// 获取当前设置
	settings, err := h.repo.GetUserSettings(r.Context(), username)
	if err != nil {
		if errors.Is(err, storage.ErrUserSettingsNotFound) {
			// 创建默认设置
			settings = storage.UserSettings{
				Username: username,
			}
		} else {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	// 如果已经开启，直接返回
	if settings.DebugEnabled {
		respondJSON(w, http.StatusOK, map[string]any{
			"status":      "already_enabled",
			"log_path":    settings.DebugLogPath,
			"started_at":  settings.DebugStartedAt,
		})
		return
	}

	// 创建日志文件
	logPath, err := h.logManager.CreateLogFile()
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("创建日志文件失败: %w", err))
		return
	}

	// 开启debug日志
	if err := logger.EnableDebug(logPath); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("开启debug日志失败: %w", err))
		return
	}

	// 更新设置
	now := time.Now()
	settings.DebugEnabled = true
	settings.DebugLogPath = logPath
	settings.DebugStartedAt = &now

	if err := h.repo.UpsertUserSettings(r.Context(), settings); err != nil {
		// 如果数据库更新失败，关闭debug日志
		logger.DisableDebug()
		writeError(w, http.StatusInternalServerError, fmt.Errorf("更新用户设置失败: %w", err))
		return
	}

	logger.Info("[Debug日志] 已开启", "user", username, "log_path", logPath)

	respondJSON(w, http.StatusOK, map[string]any{
		"status":     "enabled",
		"log_path":   logPath,
		"started_at": now.Format(time.RFC3339),
	})
}

// handleDisable 关闭debug日志
func (h *debugHandler) handleDisable(w http.ResponseWriter, r *http.Request, username string) {
	// 获取当前设置
	settings, err := h.repo.GetUserSettings(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 如果没有开启，直接返回
	if !settings.DebugEnabled {
		respondJSON(w, http.StatusOK, map[string]any{
			"status": "already_disabled",
		})
		return
	}

	// 关闭debug日志
	logPath := logger.DisableDebug()

	// 更新设置
	settings.DebugEnabled = false
	settings.DebugStartedAt = nil
	// 保留log_path用于下载

	if err := h.repo.UpsertUserSettings(r.Context(), settings); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Errorf("更新用户设置失败: %w", err))
		return
	}

	logger.Info("[Debug日志] 已关闭", "user", username, "log_path", logPath)

	// 返回下载链接
	filename := filepath.Base(logPath)
	respondJSON(w, http.StatusOK, map[string]any{
		"status":       "disabled",
		"log_path":     logPath,
		"download_url": fmt.Sprintf("/api/user/debug/download?file=%s", filename),
	})
}

// handleStatus 获取debug状态
func (h *debugHandler) handleStatus(w http.ResponseWriter, r *http.Request, username string) {
	// 获取当前设置
	settings, err := h.repo.GetUserSettings(r.Context(), username)
	if err != nil {
		if errors.Is(err, storage.ErrUserSettingsNotFound) {
			respondJSON(w, http.StatusOK, map[string]any{
				"enabled": false,
			})
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	response := map[string]any{
		"enabled": settings.DebugEnabled,
	}

	if settings.DebugEnabled && settings.DebugLogPath != "" {
		response["log_path"] = settings.DebugLogPath
		response["started_at"] = settings.DebugStartedAt

		// 获取文件大小
		if size, err := h.logManager.GetLogFileSize(settings.DebugLogPath); err == nil {
			response["file_size"] = size
		}

		// 计算运行时长
		if settings.DebugStartedAt != nil {
			duration := time.Since(*settings.DebugStartedAt)
			response["duration_seconds"] = int(duration.Seconds())
		}
	}

	respondJSON(w, http.StatusOK, response)
}

// handleDownload 下载日志文件
func (h *debugHandler) handleDownload(w http.ResponseWriter, r *http.Request, username string) {
	// 获取文件名
	filename := r.URL.Query().Get("file")
	if filename == "" {
		writeError(w, http.StatusBadRequest, errors.New("文件名不能为空"))
		return
	}

	// 只允许下载log_开头的文件（安全性）
	if !strings.HasPrefix(filename, "log_") || !strings.HasSuffix(filename, ".txt") {
		writeError(w, http.StatusBadRequest, errors.New("无效的文件名"))
		return
	}

	// 获取当前设置（验证权限）
	settings, err := h.repo.GetUserSettings(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 验证文件是否属于该用户
	if settings.DebugLogPath != "" && filepath.Base(settings.DebugLogPath) != filename {
		writeError(w, http.StatusForbidden, errors.New("无权访问该文件"))
		return
	}

	// 构建文件路径
	filePath := filepath.Join(h.logManager.BaseDir, filename)

	// 检查文件是否存在
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeError(w, http.StatusNotFound, errors.New("文件不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// 打开文件
	file, err := os.Open(filePath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	defer file.Close()

	// 设置响应头
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", fileInfo.Size()))

	// 发送文件内容
	if _, err := io.Copy(w, file); err != nil {
		logger.Error("[Debug日志] 下载文件失败", "user", username, "file", filename, "error", err)
		return
	}

	logger.Info("[Debug日志] 文件已下载", "user", username, "file", filename, "size", fileInfo.Size())

	// 下载完成后删除文件
	go func() {
		time.Sleep(1 * time.Second) // 等待下载完成
		if err := h.logManager.DeleteLogFile(filename); err != nil {
			logger.Error("[Debug日志] 删除文件失败", "file", filename, "error", err)
		} else {
			logger.Info("[Debug日志] 文件已删除", "file", filename)
		}
	}()
}
