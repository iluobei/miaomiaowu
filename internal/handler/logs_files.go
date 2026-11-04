package handler

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"miaomiaowu/internal/logger"
)

// 日志文件管理:列出 data/logs 下的日志文件及占用空间,支持单个删除与一键清空。
//
// 两条必须守住的规则:
//
//  1. **路径穿越**:文件名来自前端。拒绝一切带路径分隔符 / `..` 的写法,
//     并在拼完之后再校验一次父目录 —— 见 safeLogPath。
//
//  2. **活跃文件只能截断,不能删除**:lumberjack 一直持有 mmw.log 的 fd。
//     unlink 之后进程会继续往那个已被删除的 inode 写,磁盘空间不释放、日志页
//     也再看不到新内容,直到下一次轮转才恢复 —— 表现为"清空后日志就不动了"。
//     所以活跃文件走 os.Truncate(size=0),fd 位置不变,写入立即可见。

var (
	logsDir = filepath.Dir(logger.SystemLogPath)
	// activeLogName 是 lumberjack 正在写的文件名。
	activeLogName = filepath.Base(logger.SystemLogPath)
)

type logFileInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`
	Modified string `json:"modified"`
	// Active 标记当前正在写入的文件。前端据此把"删除"显示成"清空",
	// 并说明它不会消失 —— 否则用户会以为删除失败。
	Active bool `json:"active"`
}

// NewLogFilesHandler 管理主控自身的日志文件(admin)。
func NewLogFilesHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			listLogFiles(w)
		case http.MethodDelete:
			deleteLogFiles(w, r)
		default:
			methodNotAllowed(w, http.MethodGet, http.MethodDelete)
		}
	})
}

func listLogFiles(w http.ResponseWriter) {
	files, total, err := collectLogFiles()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"success": true, "files": files, "total_size": total, "dir": logsDir,
	})
}

// collectLogFiles 读日志目录。目录不存在不算错误(还没写过日志),返回空列表。
func collectLogFiles() ([]logFileInfo, int64, error) {
	entries, err := os.ReadDir(logsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []logFileInfo{}, 0, nil
		}
		return nil, 0, err
	}
	out := make([]logFileInfo, 0, len(entries))
	var total int64
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			continue // 刚被轮转删掉,跳过即可
		}
		total += info.Size()
		out = append(out, logFileInfo{
			Name:     e.Name(),
			Size:     info.Size(),
			Modified: info.ModTime().Format("2006-01-02 15:04:05"),
			Active:   e.Name() == activeLogName,
		})
	}
	// 活跃文件置顶,其余按修改时间倒序 —— 最需要关注的永远在第一行。
	sort.Slice(out, func(i, j int) bool {
		if out[i].Active != out[j].Active {
			return out[i].Active
		}
		return out[i].Modified > out[j].Modified
	})
	return out, total, nil
}

// deleteLogFiles 删除单个日志文件(?name=),或清空全部(?all=1)。
func deleteLogFiles(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("all") == "1" {
		files, _, err := collectLogFiles()
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		removed, freed := 0, int64(0)
		for _, f := range files {
			if err := purgeLogFile(f.Name); err != nil {
				continue // 单个失败不中断整轮清空
			}
			removed++
			freed += f.Size
		}
		respondJSON(w, http.StatusOK, map[string]any{"success": true, "removed": removed, "freed": freed})
		return
	}

	name := strings.TrimSpace(r.URL.Query().Get("name"))
	if name == "" {
		writeBadRequest(w, "name 不能为空")
		return
	}
	if err := purgeLogFile(name); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

// purgeLogFile 清掉一个日志文件:活跃文件截断,其余删除。
func purgeLogFile(name string) error {
	path, err := safeLogPath(name)
	if err != nil {
		return err
	}
	if filepath.Base(path) == activeLogName {
		// 见文件头注释:活跃文件不能 unlink,只能截断。
		return os.Truncate(path, 0)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// safeLogPath 把前端传来的文件名解析成 logsDir 下的真实路径,拒绝一切带路径的写法。
//
// 刻意**拒绝**而不是 filepath.Base() 静默改写:Base("../traffic.db") 会得到 "traffic.db",
// 落点其实仍在 logsDir 内(穿不出去),但那意味着"请求删 ../traffic.db → 返回成功",
// 实际删的是另一个文件。合法日志名来自列表接口,本就是纯文件名,不含分隔符;
// 带路径的输入一定是手工构造的,直接打回比猜它想删谁更安全也更好审计。
func safeLogPath(name string) (string, error) {
	raw := strings.TrimSpace(name)
	if raw == "" || raw == "." || raw == ".." ||
		strings.ContainsAny(raw, `/\`) || strings.Contains(raw, "..") {
		return "", errors.New("非法的日志文件名")
	}
	path := filepath.Join(logsDir, raw)
	// 拼接后再验一次归属,不单靠上面的字符串判断。
	if filepath.Dir(path) != filepath.Clean(logsDir) {
		return "", errors.New("非法的日志文件名")
	}
	return path, nil
}
