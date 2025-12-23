package handler

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"miaomiaowu/internal/storage"
)

// NewBackupDownloadHandler returns a handler that creates and downloads a backup zip file
// This handler requires admin authentication
func NewBackupDownloadHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("backup download handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeBackupError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
			return
		}

		// Checkpoint WAL to ensure all data is written to the main database file
		if err := repo.Checkpoint(); err != nil {
			writeBackupError(w, http.StatusInternalServerError, fmt.Errorf("failed to checkpoint database: %w", err))
			return
		}

		// Create zip file
		filename := fmt.Sprintf("miaomiaowu-backup-%s.zip", time.Now().Format("20060102-150405"))
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

		zipWriter := zip.NewWriter(w)
		defer zipWriter.Close()

		// Add data directory
		if err := addDirToZip(zipWriter, "data", "data"); err != nil {
			// Can't write error response after starting zip, just log
			return
		}

		// Add subscribes directory
		if err := addDirToZip(zipWriter, "subscribes", "subscribes"); err != nil {
			return
		}
	})
}

// NewBackupRestoreHandler returns a handler that restores from a backup zip file
// This handler requires admin authentication
func NewBackupRestoreHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("backup restore handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeBackupError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// Limit upload size to 100MB
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20)

		file, _, err := r.FormFile("backup")
		if err != nil {
			writeBackupError(w, http.StatusBadRequest, fmt.Errorf("failed to read backup file: %w", err))
			return
		}
		defer file.Close()

		// Save uploaded file to temp location
		tempFile, err := os.CreateTemp("", "backup-*.zip")
		if err != nil {
			writeBackupError(w, http.StatusInternalServerError, fmt.Errorf("failed to create temp file: %w", err))
			return
		}
		tempPath := tempFile.Name()
		defer os.Remove(tempPath)

		if _, err := io.Copy(tempFile, file); err != nil {
			tempFile.Close()
			writeBackupError(w, http.StatusInternalServerError, fmt.Errorf("failed to save backup file: %w", err))
			return
		}
		tempFile.Close()

		// Extract backup
		if err := extractBackup(tempPath); err != nil {
			writeBackupError(w, http.StatusInternalServerError, fmt.Errorf("failed to extract backup: %w", err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "备份恢复成功，请重启服务或刷新页面",
		})
	})
}

// NewSetupRestoreBackupHandler returns a handler for restoring backup during initial setup
// This handler does NOT require authentication but checks if setup is needed
func NewSetupRestoreBackupHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("setup restore backup handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeBackupError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		// CRITICAL SECURITY CHECK: Only allow if no users exist
		users, err := repo.ListUsers(r.Context(), 1)
		if err != nil {
			writeBackupError(w, http.StatusInternalServerError, err)
			return
		}

		if len(users) > 0 {
			writeBackupError(w, http.StatusForbidden, errors.New("系统已初始化，无法使用此接口恢复备份"))
			return
		}

		// Limit upload size to 100MB
		r.Body = http.MaxBytesReader(w, r.Body, 100<<20)

		file, _, err := r.FormFile("backup")
		if err != nil {
			writeBackupError(w, http.StatusBadRequest, fmt.Errorf("failed to read backup file: %w", err))
			return
		}
		defer file.Close()

		// Save uploaded file to temp location
		tempFile, err := os.CreateTemp("", "backup-*.zip")
		if err != nil {
			writeBackupError(w, http.StatusInternalServerError, fmt.Errorf("failed to create temp file: %w", err))
			return
		}
		tempPath := tempFile.Name()
		defer os.Remove(tempPath)

		if _, err := io.Copy(tempFile, file); err != nil {
			tempFile.Close()
			writeBackupError(w, http.StatusInternalServerError, fmt.Errorf("failed to save backup file: %w", err))
			return
		}
		tempFile.Close()

		// Extract backup
		if err := extractBackup(tempPath); err != nil {
			writeBackupError(w, http.StatusInternalServerError, fmt.Errorf("failed to extract backup: %w", err))
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"message": "备份恢复成功，请刷新页面后登录",
		})
	})
}

// addDirToZip recursively adds a directory to a zip writer
func addDirToZip(zipWriter *zip.Writer, srcDir, baseInZip string) error {
	return filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories (they're created implicitly)
		if info.IsDir() {
			return nil
		}

		// Skip hidden files and special files
		if strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		zipPath := filepath.Join(baseInZip, relPath)

		// Create file header with proper modification time
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}
		header.Name = zipPath
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()

		_, err = io.Copy(writer, file)
		return err
	})
}

// extractBackup extracts a backup zip file to the appropriate directories
func extractBackup(zipPath string) error {
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	// Validate zip contents first
	hasData := false
	hasSubscribes := false
	for _, f := range reader.File {
		if strings.HasPrefix(f.Name, "data/") {
			hasData = true
		}
		if strings.HasPrefix(f.Name, "subscribes/") {
			hasSubscribes = true
		}
	}

	if !hasData && !hasSubscribes {
		return errors.New("备份文件格式无效：缺少 data 或 subscribes 目录")
	}

	// Extract files
	for _, f := range reader.File {
		// Security check: prevent path traversal
		if strings.Contains(f.Name, "..") {
			continue
		}

		// Only extract data/ and subscribes/ directories
		if !strings.HasPrefix(f.Name, "data/") && !strings.HasPrefix(f.Name, "subscribes/") {
			continue
		}

		destPath := f.Name

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", destPath, err)
			}
			continue
		}

		// Ensure parent directory exists
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return fmt.Errorf("failed to create parent directory for %s: %w", destPath, err)
		}

		// Extract file
		srcFile, err := f.Open()
		if err != nil {
			return fmt.Errorf("failed to open zip file %s: %w", f.Name, err)
		}

		destFile, err := os.Create(destPath)
		if err != nil {
			srcFile.Close()
			return fmt.Errorf("failed to create file %s: %w", destPath, err)
		}

		_, err = io.Copy(destFile, srcFile)
		srcFile.Close()
		destFile.Close()

		if err != nil {
			return fmt.Errorf("failed to extract file %s: %w", f.Name, err)
		}
	}

	return nil
}

func writeBackupError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
}
