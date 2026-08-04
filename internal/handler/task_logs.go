package handler

import (
	"net/http"
	"strings"

	"miaomiaowu/internal/storage"
)

// TaskLogHandler 提供定时任务运行记录查询，admin 专用。
//
//	GET /api/admin/tasks/runs?task=&status=&limit=&offset=  运行记录（后端分页）
//	GET /api/admin/tasks/types                              任务类型清单（下拉筛选用）
type TaskLogHandler struct {
	repo *storage.TrafficRepository
}

func NewTaskLogHandler(repo *storage.TrafficRepository) *TaskLogHandler {
	return &TaskLogHandler{repo: repo}
}

// taskType 是一个任务的机器名 + 中文显示名。前端筛选下拉的唯一数据源，避免前端硬编码漂移。
type taskType struct {
	Name  string `json:"name"`
	Label string `json:"label"`
}

// 与各任务 taskrun.Record 里传的机器名一一对应。
var taskTypes = []taskType{
	{"wal_checkpoint", "数据库 WAL 巡检"},
	{"traffic_collector", "流量采集"},
	{"speed_collector", "测速采集"},
	{"traffic_enforcer", "流量限制执行"},
	{"daily_snapshot", "每日快照"},
	{"orphan_xray_cleaner", "孤儿客户端清理"},
	{"notify_daily_traffic", "每日流量推送"},
	{"ddns_reconciler", "DDNS 重试"},
	{"cert_renewal", "证书续期"},
	{"node_tls_fingerprint_backfill", "节点证书指纹补全"},
	{"probe_quality_alert", "探针质量告警"},
}

func (h *TaskLogHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/tasks/")
	switch path {
	case "types":
		respondJSON(w, http.StatusOK, map[string]any{"types": taskTypes})
	case "runs":
		task := strings.TrimSpace(r.URL.Query().Get("task"))
		status := strings.TrimSpace(r.URL.Query().Get("status"))
		limit := atoiDefault(r.URL.Query().Get("limit"), 200)
		offset := atoiDefault(r.URL.Query().Get("offset"), 0)
		runs, err := h.repo.ListTaskRuns(r.Context(), task, status, limit, offset)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if runs == nil {
			runs = []storage.TaskRun{}
		}
		respondJSON(w, http.StatusOK, map[string]any{"runs": runs})
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}
