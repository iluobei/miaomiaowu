package handler

import (
	"encoding/json"
	"net/http"

	"miaomiaowu/internal/storage"
)

// 仅本机访问(GitHub issue #106)。
//
// 开关存在 system_settings KV 的 master_local_only 键,由 cmd/server 的 getAddr 在
// 启动时读取:开启则把 HTTP 服务绑到 127.0.0.1,只能本机 / 隧道访问。
// 改开关需要重启才生效 —— 监听地址在进程启动时就定死了,没法热切换,所以 UI 要提示。
// Docker 环境下该开关被忽略(容器内 127.0.0.1 收不到宿主转发进来的端口,一开就自锁)。

// MasterLocalOnlyKey system_settings 里存「仅本机访问」开关的键。
const MasterLocalOnlyKey = "master_local_only"

// AccessControlHandler 读写「仅本机访问」开关。
type AccessControlHandler struct {
	repo *storage.TrafficRepository
}

func NewAccessControlHandler(repo *storage.TrafficRepository) *AccessControlHandler {
	return &AccessControlHandler{repo: repo}
}

func (h *AccessControlHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		v, _ := h.repo.GetSystemSetting(r.Context(), MasterLocalOnlyKey)
		respondJSON(w, http.StatusOK, map[string]any{
			"success":    true,
			"local_only": v == "1",
			"is_docker":  IsDockerEnvironment(),
		})
	case http.MethodPut:
		var req struct {
			LocalOnly *bool `json:"local_only"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.LocalOnly == nil {
			writeBadRequest(w, "请求格式错误")
			return
		}
		if err := h.repo.SetSystemSetting(r.Context(), MasterLocalOnlyKey, boolSetting(*req.LocalOnly)); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		respondJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"message": "已保存,重启后生效",
		})
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}
