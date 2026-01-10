package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// LogManager 日志文件管理器
type LogManager struct {
	BaseDir string
	maxSize int64 // 最大文件大小（字节）
	maxAge  int   // 最大保留天数
}

// NewLogManager 创建日志管理器
func NewLogManager(baseDir string) *LogManager {
	return &LogManager{
		BaseDir: baseDir,
		maxSize: 100 * 1024 * 1024, // 100MB
		maxAge:  7,                  // 7天
	}
}

// CreateLogFile 创建新的日志文件
func (m *LogManager) CreateLogFile() (string, error) {
	// 确保目录存在
	if err := os.MkdirAll(m.BaseDir, 0755); err != nil {
		return "", fmt.Errorf("创建日志目录失败: %w", err)
	}

	// 生成文件名：log_20260110_143015.txt
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("log_%s.txt", timestamp)
	path := filepath.Join(m.BaseDir, filename)

	return path, nil
}

// CleanupOldLogs 清理过期日志文件
func (m *LogManager) CleanupOldLogs() error {
	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 目录不存在，无需清理
		}
		return fmt.Errorf("读取日志目录失败: %w", err)
	}

	cutoff := time.Now().AddDate(0, 0, -m.maxAge)
	cleanedCount := 0

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		// 只处理log_开头的文件
		if !strings.HasPrefix(entry.Name(), "log_") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(cutoff) {
			path := filepath.Join(m.BaseDir, entry.Name())
			if err := os.Remove(path); err != nil {
				Debug("删除过期日志失败", "file", entry.Name(), "error", err)
			} else {
				cleanedCount++
				Debug("删除过期日志", "file", entry.Name(), "age_days", int(time.Since(info.ModTime()).Hours()/24))
			}
		}
	}

	if cleanedCount > 0 {
		Info("清理过期日志完成", "count", cleanedCount)
	}

	return nil
}

// GetLogFileSize 获取日志文件大小
func (m *LogManager) GetLogFileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// CheckRotation 检查是否需要轮转（文件过大）
func (m *LogManager) CheckRotation(currentPath string) (needRotate bool, newPath string, err error) {
	size, err := m.GetLogFileSize(currentPath)
	if err != nil {
		return false, "", err
	}

	if size > m.maxSize {
		newPath, err := m.CreateLogFile()
		if err != nil {
			return false, "", err
		}
		return true, newPath, nil
	}

	return false, "", nil
}

// DeleteLogFile 删除指定日志文件
func (m *LogManager) DeleteLogFile(filename string) error {
	path := filepath.Join(m.BaseDir, filename)
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("删除日志文件失败: %w", err)
	}
	return nil
}

// ListLogFiles 列出所有日志文件
func (m *LogManager) ListLogFiles() ([]LogFileInfo, error) {
	entries, err := os.ReadDir(m.BaseDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []LogFileInfo{}, nil
		}
		return nil, fmt.Errorf("读取日志目录失败: %w", err)
	}

	var files []LogFileInfo
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasPrefix(entry.Name(), "log_") {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, LogFileInfo{
			Name:     entry.Name(),
			Size:     info.Size(),
			ModTime:  info.ModTime(),
			FullPath: filepath.Join(m.BaseDir, entry.Name()),
		})
	}

	return files, nil
}

// LogFileInfo 日志文件信息
type LogFileInfo struct {
	Name     string
	Size     int64
	ModTime  time.Time
	FullPath string
}

// FormatSize 格式化文件大小
func (f LogFileInfo) FormatSize() string {
	const unit = 1024
	if f.Size < unit {
		return fmt.Sprintf("%d B", f.Size)
	}
	div, exp := int64(unit), 0
	for n := f.Size / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(f.Size)/float64(div), "KMGTPE"[exp])
}

// Age 获取文件年龄
func (f LogFileInfo) Age() time.Duration {
	return time.Since(f.ModTime)
}
