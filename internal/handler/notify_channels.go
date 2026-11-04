package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"
	"strings"

	"miaomiaowu/internal/notify"
	"miaomiaowu/internal/storage"
)

// 推送渠道管理:/api/admin/notify-channels
//
//	GET    /api/admin/notify-channels          列表(含内置 Telegram,id=0)+ 事件分组 + 渠道类型
//	POST   /api/admin/notify-channels          新建
//	PUT    /api/admin/notify-channels/{id}     修改(id=0 只能改启用与事件订阅,token / chat_id 在通知设置里改)
//	DELETE /api/admin/notify-channels/{id}     删除(内置 Telegram 不能删)
//	POST   /api/admin/notify-channels/test     用提交的(可能未保存的)参数发一条测试消息
//
// 「这个事件要不要发」仍由通知设置里的全局开关决定;渠道只决定「发到哪」:events 为空 = 全部已开的事件。

type NotifyChannelsHandler struct {
	repo *storage.TrafficRepository
}

func NewNotifyChannelsHandler(repo *storage.TrafficRepository) *NotifyChannelsHandler {
	return &NotifyChannelsHandler{repo: repo}
}

type notifyChannelView struct {
	ID          int64             `json:"id"`
	Type        string            `json:"type"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Builtin     bool              `json:"builtin"`
	Configured  bool              `json:"configured"`
	Params      map[string]string `json:"params"`
	Events      []string          `json:"events"`
	LastError   string            `json:"last_error"`
	LastErrorAt int64             `json:"last_error_at"`
	LastOKAt    int64             `json:"last_ok_at"`
}

type notifyChannelRequest struct {
	ID      int64             `json:"id"`
	Type    string            `json:"type"`
	Name    string            `json:"name"`
	Enabled *bool             `json:"enabled"`
	Params  map[string]string `json:"params"`
	Events  []string          `json:"events"`
}

// notifyChannelTypes 前端可添加的渠道类型(顺序即展示顺序)。
var notifyChannelTypes = []string{
	string(notify.ChannelSMTP), string(notify.ChannelServerChan), string(notify.ChannelFeishu),
	string(notify.ChannelDingTalk), string(notify.ChannelBark), string(notify.ChannelTelegram),
}

func (h *NotifyChannelsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	rest := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/notify-channels"), "/")
	switch {
	case rest == "" && r.Method == http.MethodGet:
		h.handleList(w, r)
	case rest == "" && r.Method == http.MethodPost:
		h.handleCreate(w, r)
	case rest == "test" && r.Method == http.MethodPost:
		h.handleTest(w, r)
	case rest != "" && (r.Method == http.MethodPut || r.Method == http.MethodDelete):
		id, err := strconv.ParseInt(rest, 10, 64)
		if err != nil || id < 0 {
			writeError(w, http.StatusBadRequest, errors.New("渠道 ID 不对"))
			return
		}
		if r.Method == http.MethodPut {
			h.handleUpdate(w, r, id)
		} else {
			h.handleDelete(w, r, id)
		}
	default:
		writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
	}
}

func (h *NotifyChannelsHandler) builtinView(ctx context.Context) (notifyChannelView, error) {
	sysCfg, err := h.repo.GetSystemConfig(ctx)
	if err != nil {
		return notifyChannelView{}, err
	}
	cfg := BuildNotifyConfig(ctx, h.repo, sysCfg)
	ch := cfg.BuiltinTelegramChannel()
	v := notifyChannelView{
		ID: notify.BuiltinTelegramID, Type: string(notify.ChannelTelegram), Name: "Telegram", Builtin: true,
		Enabled:    !cfg.TelegramChannelDisabled,
		Configured: cfg.BotToken != "" && cfg.ChatID != "",
		Params:     notify.MaskParams(notify.ChannelTelegram, ch.Params),
		Events:     nonNilEvents(cfg.TelegramEvents),
	}
	v.LastError, _ = h.repo.GetSystemSetting(ctx, notifyTelegramLastErrKey)
	v.LastErrorAt = settingInt64(ctx, h.repo, notifyTelegramLastErrAtKey)
	v.LastOKAt = settingInt64(ctx, h.repo, notifyTelegramLastOKAtKey)
	return v, nil
}

func settingInt64(ctx context.Context, repo *storage.TrafficRepository, key string) int64 {
	v, _ := repo.GetSystemSetting(ctx, key)
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

func nonNilEvents(e []string) []string {
	if e == nil {
		return []string{}
	}
	return e
}

func channelView(c storage.NotifyChannel) notifyChannelView {
	t := notify.ChannelType(c.Type)
	return notifyChannelView{
		ID: c.ID, Type: c.Type, Name: c.Name, Enabled: c.Enabled,
		Configured: notify.ValidateChannel(notify.ChannelConfig{Type: t, Params: c.Params}) == nil,
		Params:     notify.MaskParams(t, c.Params),
		Events:     nonNilEvents(c.Events),
		LastError:  c.LastError, LastErrorAt: c.LastErrorAt, LastOKAt: c.LastOKAt,
	}
}

// mergeLiveStatus 内存里的状态比库里新(落库有节流),有就用内存的。
func mergeLiveStatus(v *notifyChannelView, live map[int64]notify.ChannelStatus) {
	st, ok := live[v.ID]
	if !ok {
		return
	}
	if st.LastOKAt > v.LastOKAt {
		v.LastOKAt = st.LastOKAt
	}
	if st.LastErrorAt > v.LastErrorAt {
		v.LastErrorAt = st.LastErrorAt
	}
	if st.LastErrorAt > 0 || st.LastOKAt > 0 {
		v.LastError = st.LastError
	}
}

func (h *NotifyChannelsHandler) handleList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	builtin, err := h.builtinView(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	rows, err := h.repo.ListNotifyChannels(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	var live map[int64]notify.ChannelStatus
	if n := GetNotifier(); n != nil {
		live = n.ChannelStatuses()
	}
	list := []notifyChannelView{builtin}
	for _, c := range rows {
		list = append(list, channelView(c))
	}
	for i := range list {
		mergeLiveStatus(&list[i], live)
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"channels":     list,
		"event_groups": notify.EventGroups,
		"types":        notifyChannelTypes,
	})
}

func decodeChannelRequest(r *http.Request) (notifyChannelRequest, error) {
	var req notifyChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, errors.New("invalid JSON")
	}
	req.Type = strings.TrimSpace(req.Type)
	req.Name = strings.TrimSpace(req.Name)
	if len([]rune(req.Name)) > 64 {
		return req, errors.New("名称不能超过 64 个字符")
	}
	return req, nil
}

// buildChannel 把请求合并到已保存的渠道上(新建时 saved 为空),并校验。
func buildChannel(req notifyChannelRequest, saved storage.NotifyChannel) (storage.NotifyChannel, error) {
	c := saved
	if req.Type != "" {
		c.Type = req.Type
	}
	t := notify.ChannelType(c.Type)
	if !notify.ValidChannelType(t) {
		return c, errors.New("不支持的推送渠道类型")
	}
	if req.Name != "" || c.Name == "" {
		c.Name = req.Name
	}
	if req.Enabled != nil {
		c.Enabled = *req.Enabled
	}
	if req.Params != nil {
		savedParams := saved.Params
		if saved.Type != c.Type {
			savedParams = nil // 换了类型,旧密钥不能沿用
		}
		c.Params = notify.MergeParams(t, req.Params, savedParams)
	}
	if req.Events != nil {
		c.Events = notify.NormalizeEventGroups(req.Events)
	}
	// 停用状态下允许先存半成品(参数没填全);启用时必须能用
	if c.Enabled {
		if err := notify.ValidateChannel(notify.ChannelConfig{Type: t, Enabled: c.Enabled, Params: c.Params}); err != nil {
			return c, err
		}
	}
	return c, nil
}

func (h *NotifyChannelsHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	req, err := decodeChannelRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, errors.New("请选择渠道类型"))
		return
	}
	c, err := buildChannel(req, storage.NotifyChannel{})
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	id, err := h.repo.CreateNotifyChannel(r.Context(), c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	h.reload(r.Context())
	saved, err := h.repo.GetNotifyChannel(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, channelView(saved))
}

func (h *NotifyChannelsHandler) handleUpdate(w http.ResponseWriter, r *http.Request, id int64) {
	req, err := decodeChannelRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	if id == notify.BuiltinTelegramID {
		h.updateBuiltin(w, r, req)
		return
	}
	saved, err := h.repo.GetNotifyChannel(ctx, id)
	if errors.Is(err, storage.ErrNotifyChannelNotFound) {
		writeError(w, http.StatusNotFound, err)
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	c, err := buildChannel(req, saved)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := h.repo.UpdateNotifyChannel(ctx, c); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	h.reload(ctx)
	saved, err = h.repo.GetNotifyChannel(ctx, id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, channelView(saved))
}

// updateBuiltin 内置 Telegram 只存「启用」与「事件订阅」两项;token / chat_id 仍在 notify-config 里改。
func (h *NotifyChannelsHandler) updateBuiltin(w http.ResponseWriter, r *http.Request, req notifyChannelRequest) {
	ctx := r.Context()
	values := map[string]string{}
	if req.Enabled != nil {
		values[notifyTelegramDisabledKey] = strconv.FormatBool(!*req.Enabled)
	}
	if req.Events != nil {
		raw, _ := json.Marshal(notify.NormalizeEventGroups(req.Events))
		values[notifyTelegramEventsKey] = string(raw)
	}
	if len(values) > 0 {
		if err := setSystemSettings(ctx, h.repo, values); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}
	sysCfg, err := h.repo.GetSystemConfig(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	if n := GetNotifier(); n != nil {
		n.UpdateConfig(BuildNotifyConfig(ctx, h.repo, sysCfg))
	}
	v, err := h.builtinView(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, v)
}

func (h *NotifyChannelsHandler) handleDelete(w http.ResponseWriter, r *http.Request, id int64) {
	if id == notify.BuiltinTelegramID {
		writeError(w, http.StatusBadRequest, errors.New("内置 Telegram 渠道不能删除,可以停用或清空 Bot Token"))
		return
	}
	if err := h.repo.DeleteNotifyChannel(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrNotifyChannelNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	h.reload(r.Context())
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// handleTest 用提交的参数发测试消息。带 id 时,掩码的密钥沿用该渠道已保存的值(编辑中还没保存也能测)。
func (h *NotifyChannelsHandler) handleTest(w http.ResponseWriter, r *http.Request) {
	req, err := decodeChannelRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	ctx := r.Context()
	var saved storage.NotifyChannel
	switch {
	case req.ID == notify.BuiltinTelegramID && (req.Type == "" || req.Type == string(notify.ChannelTelegram)):
		sysCfg, err := h.repo.GetSystemConfig(ctx)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		saved = storage.NotifyChannel{Type: string(notify.ChannelTelegram), Name: "Telegram",
			Params: map[string]string{"bot_token": sysCfg.TelegramBotToken, "chat_id": sysCfg.TelegramChatID}}
	case req.ID > 0:
		saved, err = h.repo.GetNotifyChannel(ctx, req.ID)
		if err != nil {
			writeError(w, http.StatusNotFound, err)
			return
		}
	}
	enabled := true
	req.Enabled = &enabled
	c, err := buildChannel(req, saved)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}
	if err := notify.TestChannel(ctx, notify.ChannelConfig{Type: notify.ChannelType(c.Type), Name: c.Name, Params: c.Params}); err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *NotifyChannelsHandler) reload(ctx context.Context) {
	if err := ReloadNotifyChannels(ctx, h.repo); err != nil {
		log.Printf("[Notify] 重新加载推送渠道失败: %v", err)
	}
}
