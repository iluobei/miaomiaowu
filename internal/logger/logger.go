package logger

import (
	"fmt"
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/natefinch/lumberjack.v2"
)

// SystemLogPath 主日志文件路径（主控自身日志），系统日志查看与日志文件管理接口都以它为准。
const SystemLogPath = "data/logs/mmw.log"

// Logger 封装slog，支持debug文件输出
type Logger struct {
	*slog.Logger
	debugFile *os.File
	mu        sync.RWMutex
}

var (
	defaultLogger *Logger
	once          sync.Once
	// 主日志文件写入器（lumberjack 大小轮转：单文件 50MB，最多保留 4 个文件即当前+3备份，超出删最旧）
	fileWriter *lumberjack.Logger
)

// stdlogAdapter 把标准库 log 的裸文本行包装成与 slog 一致的 logfmt。
//
// 为什么需要它：NodeProbe / SpeedTester / DNSPatch 等处仍用 log.Printf，这些日志只进 stderr、
// 不落主日志文件。直接 log.SetOutput 又会让文件里混进标准库自带的 "2009/11/10 23:00:00"
// 时间戳格式，解析器得兼容两套 —— 故在此统一成 logfmt。
//
// 标准库 log 没有级别概念。历史调用点通常用 Failed/失败 等明确表达错误，
// 适配时识别这些稳定标记，避免错误被写成 INFO 而无法按级别筛选。
type stdlogAdapter struct{ w io.Writer }

func inferStdLogLevel(msg string) string {
	upper := strings.ToUpper(msg)
	lower := strings.ToLower(msg)
	if strings.Contains(upper, "ERROR:") ||
		strings.Contains(lower, "failed") ||
		strings.Contains(msg, "失败") {
		return "ERROR"
	}
	if strings.Contains(upper, "WARNING:") || strings.Contains(msg, "警告") {
		return "WARN "
	}
	return "INFO "
}

func (a stdlogAdapter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	line := fmt.Sprintf("time=%q level=%q msg=%q\n",
		time.Now().Format("2006-01-02 15:04:05"), inferStdLogLevel(msg), msg)
	if _, err := a.w.Write([]byte(line)); err != nil {
		return 0, err
	}
	// 必须返回入参长度而非改写后的长度：标准库 log 拿 n != len(p) 会当成写入不完整。
	return len(p), nil
}

// Init 初始化全局logger — 日志真实落地到 data/logs/mmw.log，同时写 stdout 供容器（docker logs）查看。
func Init() *Logger {
	once.Do(func() {
		var w io.Writer = os.Stdout
		if err := os.MkdirAll("data/logs", 0755); err == nil {
			fileWriter = &lumberjack.Logger{
				Filename:   SystemLogPath,
				MaxSize:    50, // MB
				MaxBackups: 3,
				MaxAge:     0, // 不按时间删，只看大小/数量
				Compress:   false,
			}
			w = io.MultiWriter(os.Stdout, fileWriter)
		}
		handler := newTextHandler(w, slog.LevelInfo)
		defaultLogger = &Logger{
			Logger: slog.New(handler),
		}
		// 把标准库 log 也接进同一个 sink。SetFlags(0) 去掉它自带的时间戳，改由 adapter 统一加。
		log.SetFlags(0)
		log.SetOutput(stdlogAdapter{w: w})
	})
	return defaultLogger
}

// GetLogger 获取全局logger实例
func GetLogger() *Logger {
	if defaultLogger == nil {
		return Init()
	}
	return defaultLogger
}

// newTextHandler 创建自定义文本handler（中文友好的格式）
func newTextHandler(w io.Writer, level slog.Level) slog.Handler {
	return slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			// 自定义时间格式（仅处理slog内部的TimeKey）
			if a.Key == slog.TimeKey && a.Value.Kind() == slog.KindTime {
				t := a.Value.Time()
				return slog.String("time", t.Format("2006-01-02 15:04:05"))
			}
			// 自定义级别显示
			if a.Key == slog.LevelKey {
				level := a.Value.Any().(slog.Level)
				levelStr := ""
				switch level {
				case slog.LevelDebug:
					levelStr = "DEBUG"
				case slog.LevelInfo:
					levelStr = "INFO "
				case slog.LevelWarn:
					levelStr = "WARN "
				case slog.LevelError:
					levelStr = "ERROR"
				}
				return slog.String("level", levelStr)
			}
			return a
		},
	})
}

// EnableDebugLog 开启debug日志文件
func (l *Logger) EnableDebugLog(filePath string) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	// 如果已经有文件打开，先关闭
	if l.debugFile != nil {
		l.debugFile.Close()
	}

	// 创建日志文件
	f, err := os.Create(filePath)
	if err != nil {
		return fmt.Errorf("创建日志文件失败: %w", err)
	}

	l.debugFile = f

	// 同时输出到控制台、主日志文件和临时 debug 文件
	writers := []io.Writer{os.Stdout, f}
	if fileWriter != nil {
		writers = append(writers, fileWriter)
	}
	handler := newTextHandler(io.MultiWriter(writers...), slog.LevelDebug)
	l.Logger = slog.New(handler)

	l.Info("Debug日志已开启", "file", filePath)

	return nil
}

// DisableDebugLog 关闭debug日志，返回文件路径
func (l *Logger) DisableDebugLog() string {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.debugFile == nil {
		return ""
	}

	filePath := l.debugFile.Name()

	l.Info("Debug日志即将关闭", "file", filePath)

	l.debugFile.Close()
	l.debugFile = nil

	// 恢复到控制台 + 主日志文件输出
	var w io.Writer = os.Stdout
	if fileWriter != nil {
		w = io.MultiWriter(os.Stdout, fileWriter)
	}
	handler := newTextHandler(w, slog.LevelInfo)
	l.Logger = slog.New(handler)

	return filePath
}

// IsDebugEnabled 检查debug模式是否开启
func (l *Logger) IsDebugEnabled() bool {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.debugFile != nil
}

// GetDebugFilePath 获取当前debug文件路径
func (l *Logger) GetDebugFilePath() string {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.debugFile != nil {
		return l.debugFile.Name()
	}
	return ""
}

// sanitizeArgs 脱敏敏感信息
func sanitizeArgs(args []any) []any {
	if len(args) == 0 {
		return args
	}

	result := make([]any, len(args))
	copy(result, args)

	for i := 0; i < len(result)-1; i += 2 {
		if keyStr, ok := result[i].(string); ok {
			keyLower := strings.ToLower(keyStr)
			if strings.Contains(keyLower, "password") ||
				strings.Contains(keyLower, "token") ||
				strings.Contains(keyLower, "secret") ||
				strings.Contains(keyLower, "key") && !strings.Contains(keyLower, "key=") {
				result[i+1] = "***"
			}
		}
	}

	return result
}

// 全局便捷方法
func Info(msg string, args ...any) {
	GetLogger().Info(msg, sanitizeArgs(args)...)
}

func Warn(msg string, args ...any) {
	GetLogger().Warn(msg, sanitizeArgs(args)...)
}

func Error(msg string, args ...any) {
	GetLogger().Error(msg, sanitizeArgs(args)...)
}

func Debug(msg string, args ...any) {
	GetLogger().Debug(msg, sanitizeArgs(args)...)
}

// EnableDebug 全局开启debug
func EnableDebug(filePath string) error {
	return GetLogger().EnableDebugLog(filePath)
}

// DisableDebug 全局关闭debug
func DisableDebug() string {
	return GetLogger().DisableDebugLog()
}

// IsDebugEnabled 全局检查debug状态
func IsDebugEnabled() bool {
	return GetLogger().IsDebugEnabled()
}
