package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

// 自定义 CSS 的多套配置与「从链接更新」。
//
// 生效的那一套照旧写在 branding_custom_css —— 注入页面那条路(EffectiveCustomCSS)一点不动;
// 这里只是在它前面加一层「多套里选一套」。只发 custom_css 时,改的是当前生效那套。

const brandingCustomCSSProfilesKey = "branding_custom_css_profiles"

// customCSSMaxProfiles 每套都跟着设置一起存,不设上限就能把一行设置撑成几 MB。
const customCSSMaxProfiles = 20

type customCSSProfile struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	CSS       string `json:"css"`
	SourceURL string `json:"source_url,omitempty"`
}

type customCSSProfiles struct {
	Active   string             `json:"active"`
	Profiles []customCSSProfile `json:"profiles"`
}

// loadCustomCSSProfiles 读多套配置;从没存过时用当前生效的 CSS 合成一套「默认」(不落库)。
func (h *BrandingHandler) loadCustomCSSProfiles(ctx context.Context) customCSSProfiles {
	var p customCSSProfiles
	if raw, _ := h.repo.GetSystemSetting(ctx, brandingCustomCSSProfilesKey); strings.TrimSpace(raw) != "" {
		_ = json.Unmarshal([]byte(raw), &p)
	}
	if len(p.Profiles) == 0 {
		css, _ := h.repo.GetSystemSetting(ctx, brandingCustomCSSKey)
		p = customCSSProfiles{Active: "default", Profiles: []customCSSProfile{{ID: "default", Name: "默认", CSS: css}}}
	}
	if p.activeIndex() < 0 {
		p.Active = p.Profiles[0].ID
	}
	return p
}

func (p customCSSProfiles) activeIndex() int {
	for i, pr := range p.Profiles {
		if pr.ID == p.Active {
			return i
		}
	}
	return -1
}

// normalizeCustomCSSProfiles 校验前端提交的多套配置;返回规整后的配置与生效的那一套 CSS。
func normalizeCustomCSSProfiles(p customCSSProfiles) (customCSSProfiles, string, error) {
	if len(p.Profiles) == 0 {
		return p, "", errors.New("至少要保留一套配置")
	}
	if len(p.Profiles) > customCSSMaxProfiles {
		return p, "", fmt.Errorf("最多 %d 套配置", customCSSMaxProfiles)
	}
	seen := map[string]bool{}
	for i := range p.Profiles {
		pr := &p.Profiles[i]
		pr.ID = strings.TrimSpace(pr.ID)
		pr.Name = strings.TrimSpace(pr.Name)
		if pr.ID == "" || seen[pr.ID] {
			return p, "", fmt.Errorf("第 %d 套配置的 ID 为空或重复", i+1)
		}
		seen[pr.ID] = true
		if pr.Name == "" {
			pr.Name = fmt.Sprintf("配置 %d", i+1)
		}
		if utf8.RuneCountInString(pr.Name) > 40 {
			return p, "", fmt.Errorf("配置名「%s」太长", pr.Name)
		}
		css, err := sanitizeCustomCSS(pr.CSS)
		if err != nil {
			return p, "", fmt.Errorf("「%s」:%v", pr.Name, err)
		}
		pr.CSS = css
		pr.SourceURL = strings.TrimSpace(pr.SourceURL)
		if pr.SourceURL != "" {
			if err := validateFetchURL(pr.SourceURL); err != nil {
				return p, "", fmt.Errorf("「%s」的来源链接:%v", pr.Name, err)
			}
		}
	}
	i := p.activeIndex()
	if i < 0 {
		return p, "", errors.New("生效的配置不存在")
	}
	return p, p.Profiles[i].CSS, nil
}

// ---- 从链接拉取 ----

var (
	githubBlobURL = regexp.MustCompile(`^https?://github\.com/([^/]+)/([^/]+)/blob/(.+)$`)
	githubGistURL = regexp.MustCompile(`^https?://gist\.github\.com/([^/]+)/([0-9a-fA-F]+)/?$`)
)

// rawCSSURL 把 GitHub 的网页地址换成原始文件地址:用户从浏览器地址栏复制的多半是 blob / gist 页面,
// 抓回来是一整页 HTML。其它地址原样返回。
func rawCSSURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if m := githubBlobURL.FindStringSubmatch(raw); m != nil {
		return fmt.Sprintf("https://raw.githubusercontent.com/%s/%s/%s", m[1], m[2], m[3])
	}
	if m := githubGistURL.FindStringSubmatch(raw); m != nil {
		return fmt.Sprintf("https://gist.githubusercontent.com/%s/%s/raw", m[1], m[2])
	}
	return raw
}

// fetchCustomCSS 经 SSRF 防护的客户端拉一份 CSS,与手填走同一套校验(大小、</style)。
var fetchCustomCSS = func(ctx context.Context, rawURL string) (string, error) {
	target := rawCSSURL(rawURL)
	if err := validateFetchURL(target); err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return "", err
	}
	resp, err := newSSRFSafeHTTPClient(20 * time.Second).Do(req)
	if err != nil {
		return "", fmt.Errorf("下载失败:%v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载失败:%s 返回 %d", target, resp.StatusCode)
	}
	if ct := strings.ToLower(resp.Header.Get("Content-Type")); strings.Contains(ct, "text/html") {
		return "", fmt.Errorf("%s 返回的是网页而不是 CSS,请填原始文件(Raw)地址", target)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, customCSSMaxBytes+1))
	if err != nil {
		return "", fmt.Errorf("读取失败:%v", err)
	}
	if !utf8.Valid(body) {
		return "", errors.New("内容不是 UTF-8 文本,不像是 CSS")
	}
	return sanitizeCustomCSS(string(body))
}

// FetchCustomCSS POST /api/admin/system-settings/custom-css/fetch {url} → {css}
// 只拉取、不保存:内容先回到编辑框让管理员看过再存,远端被改坏也不会直接打到所有页面上。
func (h *BrandingHandler) FetchCustomCSS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "方法不允许", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		URL string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.URL) == "" {
		writeBadRequest(w, "请填写 CSS 链接")
		return
	}
	if u, err := url.Parse(strings.TrimSpace(req.URL)); err != nil || u.Host == "" {
		writeBadRequest(w, "链接格式不对")
		return
	}
	css, err := fetchCustomCSS(r.Context(), req.URL)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "css": css, "url": rawCSSURL(req.URL)})
}
