package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/logger"
	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/taskrun"
)

// 规则集(clash rule-providers)托管。
//
// 目的很直接:订阅里的 rule-providers 需要一个能被客户端直接拉取的地址。以前只能
// 指向第三方仓库 —— 更新不可控,墙内还常常拉不动。放到主控上之后,订阅引用的就是
// 自己这台机器的地址。
//
// 因此这个下载端点**必须公开**:拉它的是 mihomo/clash 客户端,没有登录态,
// 也不可能带 securechan 信封。它挂在 /rules/ 而不是 /api/ 下,
// requiresSecureChannel 对非 /api/ 路径本来就放行(与壁纸文件同理)。
//
// 内容不是秘密(就是一堆域名/IP 规则),但名字由管理员自己起 —— 不想被人猜到就
// 起个带随机后缀的名字。这一点在前端文案里写清楚了。

const (
	// RuleProviderURLPrefix 公开下载路径前缀。
	// 用 /ruleset/ 而不是 /rules/:后者会和前端已有的 /rules SPA 路由撞车 ——
	// Go 的 ServeMux 对注册了 /rules/ 子树的请求会把 /rules 自动 301 到 /rules/,
	// 从而把那个页面顶掉。/ruleset 前端没有同名路由,安全。
	RuleProviderURLPrefix = "/ruleset/"
	// 4MiB:够放 geosite 级别的大表,又不至于让人把整个 DB 塞成规则集。
	ruleProviderMaxBytes = 4 << 20
	// 远程抓取的最小间隔。允许 1 分钟的话,几十个规则集能把上游打成 DDoS。
	ruleProviderMinRefreshMinutes = 5
	ruleProviderMaxNameLen        = 64
)

// ruleProviderNameRe 公开路径里的文件名。必须以字母数字开头 —— 这样连 ".." 都构造不出,
// 也不会出现以点开头的隐藏名。
var ruleProviderNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// RuleProvidersHandler 管理端 CRUD。
type RuleProvidersHandler struct {
	repo *storage.TrafficRepository
}

func NewRuleProvidersHandler(repo *storage.TrafficRepository) *RuleProvidersHandler {
	return &RuleProvidersHandler{repo: repo}
}

func (h *RuleProvidersHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/admin/rule-providers"), "/")
	switch {
	case path == "" && r.Method == http.MethodGet:
		h.handleList(w, r)
	case path == "" && r.Method == http.MethodPost:
		h.handleCreate(w, r)
	case strings.HasSuffix(path, "/refresh") && r.Method == http.MethodPost:
		h.handleRefresh(w, r, strings.TrimSuffix(path, "/refresh"))
	case path != "" && r.Method == http.MethodGet:
		h.handleGet(w, r, path)
	case path != "" && r.Method == http.MethodPut:
		h.handleUpdate(w, r, path)
	case path != "" && r.Method == http.MethodDelete:
		h.handleDelete(w, r, path)
	default:
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
	}
}

func (h *RuleProvidersHandler) handleList(w http.ResponseWriter, r *http.Request) {
	list, err := h.repo.ListRuleProviders(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{
		"success":    true,
		"providers":  list,
		"url_prefix": RuleProviderURLPrefix,
	})
}

func (h *RuleProvidersHandler) handleGet(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeBadRequest(w, "id 无效")
		return
	}
	p, err := h.repo.GetRuleProvider(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("规则集不存在"))
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "provider": p})
}

// ruleProviderPayload 新建/更新共用的请求体。
type ruleProviderPayload struct {
	Name           string `json:"name"`
	DisplayName    string `json:"display_name"`
	Source         string `json:"source"`
	RemoteURL      string `json:"remote_url"`
	RefreshMinutes int    `json:"refresh_minutes"`
	Content        string `json:"content"`
}

// normalize 校验并规整。返回的 error 直接给用户看。
func (p *ruleProviderPayload) normalize() error {
	p.Name = strings.TrimSpace(p.Name)
	if !ruleProviderNameRe.MatchString(p.Name) {
		return fmt.Errorf("文件名只能用字母、数字、点、下划线和减号,以字母或数字开头,最长 %d 位", ruleProviderMaxNameLen)
	}
	p.DisplayName = strings.TrimSpace(p.DisplayName)
	p.Source = strings.TrimSpace(p.Source)
	if p.Source != "remote" {
		p.Source = "manual"
	}
	p.RemoteURL = strings.TrimSpace(p.RemoteURL)
	if p.Source == "remote" {
		low := strings.ToLower(p.RemoteURL)
		if !strings.HasPrefix(low, "http://") && !strings.HasPrefix(low, "https://") {
			return errors.New("远程地址必须以 http:// 或 https:// 开头")
		}
		if p.RefreshMinutes < ruleProviderMinRefreshMinutes {
			p.RefreshMinutes = ruleProviderMinRefreshMinutes
		}
	} else {
		// 手动维护的不需要抓取周期,清零免得被调度器捡走。
		p.RemoteURL = ""
		p.RefreshMinutes = 0
	}
	if len(p.Content) > ruleProviderMaxBytes {
		return fmt.Errorf("内容过大(上限 %d MB)", ruleProviderMaxBytes>>20)
	}
	return nil
}

func decodeRuleProviderPayload(r *http.Request) (ruleProviderPayload, error) {
	var p ruleProviderPayload
	if err := json.NewDecoder(io.LimitReader(r.Body, ruleProviderMaxBytes+(1<<20))).Decode(&p); err != nil {
		return p, errors.New("请求格式错误")
	}
	return p, p.normalize()
}

func (h *RuleProvidersHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	p, err := decodeRuleProviderPayload(r)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	id, err := h.repo.CreateRuleProvider(r.Context(), storage.RuleProvider{
		Name: p.Name, DisplayName: p.DisplayName, Source: p.Source,
		RemoteURL: p.RemoteURL, RefreshMinutes: p.RefreshMinutes, Content: p.Content,
	})
	if err != nil {
		// UNIQUE(name) 撞了是最常见的失败,单独说清楚 —— 否则用户只看到一串 SQL 错误。
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeBadRequest(w, "文件名已被占用")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	// 远程源建好就先抓一次,不然要等下一个周期才有内容。
	if p.Source == "remote" {
		h.fetchInto(r.Context(), id, p.RemoteURL)
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "id": id})
}

func (h *RuleProvidersHandler) handleUpdate(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeBadRequest(w, "id 无效")
		return
	}
	p, err := decodeRuleProviderPayload(r)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	err = h.repo.UpdateRuleProvider(r.Context(), storage.RuleProvider{
		ID: id, Name: p.Name, DisplayName: p.DisplayName, Source: p.Source,
		RemoteURL: p.RemoteURL, RefreshMinutes: p.RefreshMinutes, Content: p.Content,
	})
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			writeBadRequest(w, "文件名已被占用")
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

func (h *RuleProvidersHandler) handleDelete(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeBadRequest(w, "id 无效")
		return
	}
	if err := h.repo.DeleteRuleProvider(r.Context(), id); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

// handleRefresh 立刻抓一次远程源。
func (h *RuleProvidersHandler) handleRefresh(w http.ResponseWriter, r *http.Request, idStr string) {
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		writeBadRequest(w, "id 无效")
		return
	}
	p, err := h.repo.GetRuleProvider(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusNotFound, errors.New("规则集不存在"))
		return
	}
	if p.Source != "remote" || p.RemoteURL == "" {
		writeBadRequest(w, "只有远程来源的规则集才能抓取")
		return
	}
	if err := h.fetchInto(r.Context(), id, p.RemoteURL); err != nil {
		writeBadRequest(w, "抓取失败:"+err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true})
}

// fetchInto 抓一次并写库。抓取失败只记错误、**不动已有内容** ——
// 上次抓到的还能继续服务,总比让订阅拿到空文件强。
func (h *RuleProvidersHandler) fetchInto(ctx context.Context, id int64, url string) error {
	content, err := fetchRuleProviderContent(ctx, url)
	if err != nil {
		_ = h.repo.MarkRuleProviderFetched(ctx, id, "", err.Error())
		return err
	}
	return h.repo.MarkRuleProviderFetched(ctx, id, content, "")
}

// fetchRuleProviderContent 抓远程规则集。
//
// 走 SSRF 安全客户端:地址是管理员填的,但「管理员填的」不等于「安全」——
// 主控能访问的内网和云元数据接口,不该因为一个输入框就暴露出去(订阅导入踩过)。
func fetchRuleProviderContent(ctx context.Context, url string) (string, error) {
	client := newSSRFSafeHTTPClient(30 * time.Second)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "mmwx-rule-provider/1.0")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, ruleProviderMaxBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > ruleProviderMaxBytes {
		return "", fmt.Errorf("内容过大(上限 %d MB)", ruleProviderMaxBytes>>20)
	}
	if len(body) == 0 {
		return "", errors.New("远程返回空内容")
	}
	return string(body), nil
}

// ServeRuleProviderFile GET /rules/{name} —— 免鉴权。
//
// 拉它的是 mihomo/clash 客户端,没有登录态。用 http.ServeContent 而不是直接写:
// 它会处理 Last-Modified / If-Modified-Since,客户端按自己的 interval 轮询时
// 多数请求能落到 304,不必每次传整份规则表。
func (h *RuleProvidersHandler) ServeRuleProviderFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, RuleProviderURLPrefix)
	if !ruleProviderNameRe.MatchString(name) {
		http.NotFound(w, r)
		return
	}
	content, updated, err := h.repo.GetRuleProviderContentByName(r.Context(), name)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	ct := "text/yaml; charset=utf-8"
	low := strings.ToLower(name)
	if strings.HasSuffix(low, ".txt") || strings.HasSuffix(low, ".list") {
		ct = "text/plain; charset=utf-8"
	}
	w.Header().Set("Content-Type", ct)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, updated, strings.NewReader(content))
}

// StartRuleProviderRefresher 远程规则集的定时抓取。
//
// 每分钟看一眼有没有到期的,而不是给每个规则集起一个 ticker:规则集数量是用户
// 定的,几十个 goroutine 各自计时既难观测也难停。到期判断在仓库层做。
func StartRuleProviderRefresher(ctx context.Context, repo *storage.TrafficRepository) {
	h := NewRuleProvidersHandler(repo)
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				taskrun.Record(ctx, "rule_provider_refresh", func() (string, error) {
					due, err := repo.ListRuleProvidersDueForFetch(ctx, time.Now())
					if err != nil {
						return "", err
					}
					if len(due) == 0 {
						return "", nil
					}
					ok, failed := 0, 0
					for _, p := range due {
						if err := h.fetchInto(ctx, p.ID, p.RemoteURL); err != nil {
							failed++
							logger.Warn("规则集抓取失败", "name", p.Name, "err", err)
							continue
						}
						ok++
					}
					return fmt.Sprintf("成功 %d,失败 %d", ok, failed), nil
				})
			}
		}
	}()
}
