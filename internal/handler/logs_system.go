package handler

import (
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"miaomiaowu/internal/logger"
)

// SystemLogHandler 提供主控自身日志（data/logs/mmw.log）的只读查询，admin 专用。
//
// 为什么不复用 /api/user/debug/：那套是 per-user token 鉴权、且是「全局单例伪装成 per-user」
// （debugUsername/autoCloseTimer 是单槽字段，第二个用户 enable 会冲掉第一个）。系统日志是
// admin 级只读，不涉及会话状态，独立成一个干净的 handler。
type SystemLogHandler struct{}

func NewSystemLogHandler() *SystemLogHandler {
	return &SystemLogHandler{}
}

// logRow 是解析后的一行结构化日志。解析失败时 Time/Level 为空、Raw 保留原文（不吞行）。
type logRow struct {
	Time   string `json:"time"`
	Level  string `json:"level"`
	Msg    string `json:"msg"`
	Fields string `json:"fields,omitempty"` // msg 之后的结构化 k=v(如 nodes=... filtered_count=...),前端拼在 msg 后展示
	Raw    string `json:"raw,omitempty"`
}

func (h *SystemLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w, http.MethodGet)
		return
	}

	lines := 500
	if v := r.URL.Query().Get("lines"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 2000 {
			lines = n
		}
	}
	levelFilter := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("level")))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	// tailFile 读取窗口:
	//   - 无筛选:多读一点(lines*4)给"空行/非目标行"留余量,最终仍只返回最后 lines 行。
	//   - 有 level/q 筛选:必须在**大得多**的窗口里搜,否则高频日志会把想找的行刷出 lines*4 窗口
	//     —— 表现为"服务器 grep 有、页面看不到"。放大到 50000 行,反向 8KB 分块对本地文件仍是毫秒级。
	scanLines := lines * 4
	if levelFilter != "" || q != "" {
		scanLines = 50000
	}
	rawTail, err := tailFile(logger.SystemLogPath, scanLines)
	if err != nil {
		abs, _ := filepath.Abs(logger.SystemLogPath)
		if os.IsNotExist(err) {
			// 文件不存在（全新部署还没写日志）不算错误，返回空列表 —— 但带上绝对路径,
			// 便于排查"服务器有日志但页面空"(通常是 systemd WorkingDirectory
			// 与代码相对路径 data/logs 不一致,导致接口读的根本不是 logger 写的那个文件)。
			respondJSON(w, http.StatusOK, map[string]any{"rows": []logRow{}, "log_path": abs, "note": "日志文件不存在"})
			return
		}
		// 其它错误(权限/IO)不再静默吞成空列表 —— 直接把路径和错误返回,让前端能看到真因。
		respondJSON(w, http.StatusOK, map[string]any{"rows": []logRow{}, "log_path": abs, "error": err.Error()})
		return
	}

	rows := make([]logRow, 0, lines)
	for _, line := range strings.Split(rawTail, "\n") {
		if line == "" {
			continue
		}
		row := parseLogfmtLine(line)
		if levelFilter != "" && strings.TrimSpace(row.Level) != levelFilter {
			continue
		}
		if q != "" && !strings.Contains(line, q) {
			continue
		}
		rows = append(rows, row)
	}
	// 只保留最后 lines 行（过滤后可能仍超量）
	if len(rows) > lines {
		rows = rows[len(rows)-lines:]
	}

	respondJSON(w, http.StatusOK, map[string]any{"rows": rows})
}

// parseLogfmtLine 解析一行 logfmt 日志，抽出 time / level / msg。纯函数，可单测。
//
// 格式来自 logger.newTextHandler：time="2006-01-02 15:04:05" level="INFO " msg=... 后跟任意 k=v。
// msg 只在含空格时才带引号（slog logfmt 规则），故不能按空格 split。这里做最小化解析：
// 只认 time= / level= / msg= 三个前缀键，取到 msg= 后把剩余整段当消息（含后续 k=v，
// 对展示够用，避免实现完整 logfmt 状态机）。
//
// 解析不出 time/level（如接管前的历史行、或非本格式的行）→ Level/Time 空、Raw=整行原文，
// 让前端原样显示，绝不吞行。
func parseLogfmtLine(line string) logRow {
	time, rest, okT := cutLogfmtValue(line, "time=")
	level, rest2, okL := cutLogfmtValue(rest, "level=")
	if !okT || !okL {
		return logRow{Raw: line}
	}
	// msg 走同一套引号配对逻辑（cutLogfmtValue）：带引号时正确识别右引号边界，
	// 不能简单「取到行尾再 unquote」—— 那样 `msg="x" k=v` 会把整段当引号值。
	// rest3 = msg 之后剩下的结构化字段(如 filtered_count=8 nodes="🇭🇰..."),必须保留 ——
	// 否则「被过滤的节点名」这类关键信息在页面上完全看不到。
	msg, rest3, _ := cutLogfmtValue(rest2, "msg=")
	return logRow{Time: time, Level: strings.TrimSpace(level), Msg: msg, Fields: strings.TrimSpace(rest3)}
}

// cutLogfmtValue 从 s 中定位 key（形如 "time="），取其后的一个 logfmt 值，
// 返回 (值, 该值之后的剩余串, 是否找到)。值可能带引号也可能是裸 token。
func cutLogfmtValue(s, key string) (string, string, bool) {
	idx := strings.Index(s, key)
	if idx < 0 {
		return "", s, false
	}
	after := s[idx+len(key):]
	if strings.HasPrefix(after, `"`) {
		// 引号值：找配对的未转义右引号
		end := 1
		for end < len(after) {
			if after[end] == '\\' {
				end += 2
				continue
			}
			if after[end] == '"' {
				break
			}
			end++
		}
		// end 可能因引号未闭合(tailFile 切出的半行常见)或反斜杠结尾而 >= len(after);
		// 必须夹到 len(after),否则 after[:end+1] 越界 panic。
		val := unquoteLogfmt(after[:min(end+1, len(after))])
		return val, after[min(end+1, len(after)):], true
	}
	// 裸值：到下一个空格
	sp := strings.IndexByte(after, ' ')
	if sp < 0 {
		return after, "", true
	}
	return after[:sp], after[sp:], true
}

// unquoteLogfmt 去掉外层引号并还原 \" \\ 转义。非引号串原样返回。
func unquoteLogfmt(s string) string {
	s = strings.TrimSpace(s)
	if len(s) < 2 || s[0] != '"' || s[len(s)-1] != '"' {
		return s
	}
	inner := s[1 : len(s)-1]
	inner = strings.ReplaceAll(inner, `\"`, `"`)
	inner = strings.ReplaceAll(inner, `\\`, `\`)
	return inner
}
