package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"miaomiaowu/internal/storage"
)

// 菜单排序与显示/隐藏。
//
// 一份全局配置,管理员定,对所有人生效。它和现有的「用户页面权限」是两回事,叠加生效:
// 权限决定某个用户**能不能**看见某页,这里决定**顺序**和**要不要摆出来**。
// 隐藏只是不摆进导航 —— 路由本身还在,知道地址的人照样进得去,所以它是整理界面用的,
// 不是权限手段(前端文案里写清楚了)。
//
// 用路由路径(如 "/nodes")当标识而不是序号:菜单项增删时序号会整体错位,
// 而路径是稳定的。认不出的路径在前端直接忽略,不影响其余项。

// NavMenuConfigKey 存 system_config 的键。
const NavMenuConfigKey = "nav_menu_config"

var (
	errNavKeyTooLong   = errors.New("菜单标识过长")
	errNavTooManyItems = errors.New("菜单项过多")
)

const (
	navMenuMaxItems  = 64
	navMenuMaxKeyLen = 64
)

// NavMenuConfig 菜单配置。order 是完整顺序,hidden 是不摆进导航的那些。
type NavMenuConfig struct {
	Order  []string `json:"order"`
	Hidden []string `json:"hidden"`
}

// NavMenuHandler 读(所有登录用户)与写(管理员)。
type NavMenuHandler struct {
	repo  *storage.TrafficRepository
	admin bool // true = 写入端点
}

func NewNavMenuHandler(repo *storage.TrafficRepository, admin bool) *NavMenuHandler {
	return &NavMenuHandler{repo: repo, admin: admin}
}

// loadNavMenuConfig 读出配置。没配过或解析失败都返回空配置 ——
// 空配置的含义是「按代码里的默认顺序、全部显示」,任何情况下导航都不该消失。
func loadNavMenuConfig(h *NavMenuHandler, r *http.Request) NavMenuConfig {
	cfg := NavMenuConfig{Order: []string{}, Hidden: []string{}}
	raw, _ := h.repo.GetSystemSetting(r.Context(), NavMenuConfigKey)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return cfg
	}
	var parsed NavMenuConfig
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return cfg
	}
	if parsed.Order != nil {
		cfg.Order = parsed.Order
	}
	if parsed.Hidden != nil {
		cfg.Hidden = parsed.Hidden
	}
	return cfg
}

func (h *NavMenuHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		respondJSON(w, http.StatusOK, map[string]any{
			"success": true,
			"config":  loadNavMenuConfig(h, r),
		})
	case http.MethodPut:
		if !h.admin {
			http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
			return
		}
		h.handleSave(w, r)
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}

func (h *NavMenuHandler) handleSave(w http.ResponseWriter, r *http.Request) {
	var cfg NavMenuConfig
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 64<<10)).Decode(&cfg); err != nil {
		writeBadRequest(w, "请求格式错误")
		return
	}
	order, err := sanitizeNavKeys(cfg.Order)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	hidden, err := sanitizeNavKeys(cfg.Hidden)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	blob, err := json.Marshal(NavMenuConfig{Order: order, Hidden: hidden})
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if err := h.repo.SetSystemSetting(r.Context(), NavMenuConfigKey, string(blob)); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "message": "菜单设置已保存"})
}

// sanitizeNavKeys 去空白、去重、限长限量。
//
// 不校验「这个路径是不是真的存在」:菜单项由前端定义,后端不该持有那份清单
// (持有就得两边同步,又是一份会漂移的硬编码)。认不出的键前端忽略即可。
func sanitizeNavKeys(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if len(v) > navMenuMaxKeyLen {
			return nil, errNavKeyTooLong
		}
		if seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
		if len(out) > navMenuMaxItems {
			return nil, errNavTooManyItems
		}
	}
	return out, nil
}
