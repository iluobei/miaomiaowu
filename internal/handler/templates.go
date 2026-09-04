package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
	"gopkg.in/yaml.v3"
	"miaomiaowu/internal/storage"
)

type templateRequest struct {
	Name             string `json:"name"`
	Category         string `json:"category"`
	TemplateURL      string `json:"template_url"`
	RuleSource       string `json:"rule_source"`
	UseProxy         bool   `json:"use_proxy"`
	EnableIncludeAll bool   `json:"enable_include_all"`
}

type templateResponse struct {
	ID               int64  `json:"id"`
	Name             string `json:"name"`
	Category         string `json:"category"`
	TemplateURL      string `json:"template_url"`
	RuleSource       string `json:"rule_source"`
	UseProxy         bool   `json:"use_proxy"`
	EnableIncludeAll bool   `json:"enable_include_all"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

type convertRulesRequest struct {
	TemplateURL      string   `json:"template_url"`
	RuleSource       string   `json:"rule_source"`
	Category         string   `json:"category"`
	UseProxy         bool     `json:"use_proxy"`
	EnableIncludeAll bool     `json:"enable_include_all"`
	ProxyNames       []string `json:"proxy_names"` // 节点名称列表，用于显式填充 proxies 字段
}

type convertRulesResponse struct {
	Content string `json:"content"`
}

// NewTemplatesHandler handles template list and create operations
func NewTemplatesHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("templates handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleListTemplates(w, r, repo)
		case http.MethodPost:
			handleCreateTemplate(w, r, repo)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET and POST are supported"))
		}
	})
}

// NewTemplateHandler handles single template operations (GET, PUT, DELETE)
func NewTemplateHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("template handler requires repository")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Extract template ID from URL path
		path := strings.TrimPrefix(r.URL.Path, "/api/admin/templates/")
		idStr := strings.TrimSpace(path)
		if idStr == "" {
			writeError(w, http.StatusBadRequest, errors.New("template id is required"))
			return
		}

		id, err := strconv.ParseInt(idStr, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid template id"))
			return
		}

		switch r.Method {
		case http.MethodGet:
			handleGetTemplate(w, r, repo, id)
		case http.MethodPut:
			handleUpdateTemplate(w, r, repo, id)
		case http.MethodDelete:
			handleDeleteTemplate(w, r, repo, id)
		default:
			writeError(w, http.StatusMethodNotAllowed, errors.New("only GET, PUT and DELETE are supported"))
		}
	})
}

// NewTemplateConvertHandler handles rule conversion
func NewTemplateConvertHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("only POST is supported"))
			return
		}

		var req convertRulesRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if req.RuleSource == "" {
			writeError(w, http.StatusBadRequest, errors.New("rule_source is required"))
			return
		}

		if req.Category == "" {
			req.Category = "clash"
		}

		// Fetch template content from URL
		var templateContent string
		if req.TemplateURL != "" {
			content, err := fetchRemoteContent(req.TemplateURL, 30*time.Second)
			if err != nil {
				writeError(w, http.StatusBadRequest, errors.New("failed to fetch template: "+err.Error()))
				return
			}
			templateContent = content
		}

		// Detect template type and validate
		detectedType := substore.DetectTemplateType(templateContent)
		if detectedType != "" && detectedType != req.Category {
			writeError(w, http.StatusBadRequest, errors.New("template type mismatch: detected "+detectedType+" but requested "+req.Category))
			return
		}

		// Use default template if empty
		if strings.TrimSpace(templateContent) == "" {
			if req.Category == "surge" {
				templateContent = substore.GetDefaultSurgeTemplate()
			} else {
				templateContent = substore.GetDefaultClashTemplate()
			}
		}

		// Fetch ACL configuration
		aclContent, err := fetchRemoteContent(req.RuleSource, 30*time.Second)
		if err != nil {
			writeError(w, http.StatusBadRequest, errors.New("failed to fetch rule source: "+err.Error()))
			return
		}

		// Parse ACL configuration
		rulesets, proxyGroups := substore.ParseACLConfig(aclContent)

		// 处理req.ProxyNames里的特殊字符
		// 对包含特殊字符的节点名称加上引号，避免 YAML 解析错误
		for i, name := range req.ProxyNames {
			needsQuote := strings.ContainsAny(name, ":#[],") || strings.HasPrefix(name, "@")
			if needsQuote {
				req.ProxyNames[i] = `"` + name + `"`
			}
		}

		// Generate proxy groups and rules based on category
		var finalContent string
		if req.Category == "surge" {
			proxyGroupsStr := substore.GenerateSurgeProxyGroups(proxyGroups, req.EnableIncludeAll)
			rulesStr, err := substore.GenerateSurgeRules(rulesets)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			finalContent = substore.MergeToSurgeTemplate(templateContent, proxyGroupsStr, rulesStr)
		} else {
			proxyGroupsStr := substore.GenerateClashProxyGroups(proxyGroups, req.ProxyNames)
			rulesStr, providersStr, err := substore.GenerateClashRules(rulesets)
			if err != nil {
				writeError(w, http.StatusInternalServerError, err)
				return
			}
			finalContent = substore.MergeToClashTemplate(templateContent, proxyGroupsStr, rulesStr, providersStr)
			finalContent, err = ensureV2ProxyGroupMembers(finalContent, req.ProxyNames)
			if err != nil {
				writeError(w, http.StatusBadRequest, err)
				return
			}
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(convertRulesResponse{Content: finalContent})
	})
}

// ensureV2ProxyGroupMembers prevents a malformed or expired ACL source from
// silently producing a Clash config whose groups cannot select any node.
func ensureV2ProxyGroupMembers(content string, proxyNames []string) (string, error) {
	trimmed := strings.TrimSpace(strings.ToLower(content))
	if strings.HasPrefix(trimmed, "<!doctype html") || strings.HasPrefix(trimmed, "<html") {
		return "", errors.New("规则源返回了 HTML 页面而不是 ACL 配置，请检查模板 URL 是否需要登录或已经失效")
	}
	var config map[string]any
	if err := yaml.Unmarshal([]byte(content), &config); err != nil {
		return "", fmt.Errorf("V2 模板生成了无效 YAML: %w", err)
	}
	groups, ok := config["proxy-groups"].([]any)
	if !ok || len(groups) == 0 {
		return "", errors.New("规则源中没有有效的 custom_proxy_group，请检查模板 URL 是否返回了 HTML、登录页或已失效")
	}
	if len(proxyNames) == 0 {
		return "", errors.New("没有可注入代理组的节点")
	}

	changed := false
	for _, raw := range groups {
		group, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		proxies, hasProxies := group["proxies"].([]any)
		filter, _ := group["filter"].(string)
		hasDynamicSource := group["include-all"] == true || strings.TrimSpace(filter) != "" || nonEmptyList(group["use"])
		if (!hasProxies || len(proxies) == 0) && !hasDynamicSource {
			members := make([]any, 0, len(proxyNames))
			for _, name := range proxyNames {
				members = append(members, strings.Trim(name, `"`))
			}
			group["proxies"] = members
			changed = true
		}
	}
	if !changed {
		return content, nil
	}
	result, err := yaml.Marshal(config)
	if err != nil {
		return "", fmt.Errorf("序列化 V2 模板失败: %w", err)
	}
	return string(result), nil
}

func nonEmptyList(value any) bool {
	items, ok := value.([]any)
	return ok && len(items) > 0
}

func handleListTemplates(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository) {
	templates, err := repo.ListTemplates(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	response := make([]templateResponse, 0, len(templates))
	for _, t := range templates {
		response = append(response, templateToResponse(t))
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"templates": response})
}

func handleGetTemplate(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, id int64) {
	t, err := repo.GetTemplateByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, storage.ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(templateToResponse(t))
}

func handleCreateTemplate(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository) {
	var req templateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}

	t := storage.Template{
		Name:             req.Name,
		Category:         req.Category,
		TemplateURL:      req.TemplateURL,
		RuleSource:       req.RuleSource,
		UseProxy:         req.UseProxy,
		EnableIncludeAll: req.EnableIncludeAll,
	}

	id, err := repo.CreateTemplate(r.Context(), t)
	if err != nil {
		if errors.Is(err, storage.ErrTemplateExists) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	created, _ := repo.GetTemplateByID(r.Context(), id)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(templateToResponse(created))
}

func handleUpdateTemplate(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, id int64) {
	var req templateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	if strings.TrimSpace(req.Name) == "" {
		writeError(w, http.StatusBadRequest, errors.New("name is required"))
		return
	}

	t := storage.Template{
		ID:               id,
		Name:             req.Name,
		Category:         req.Category,
		TemplateURL:      req.TemplateURL,
		RuleSource:       req.RuleSource,
		UseProxy:         req.UseProxy,
		EnableIncludeAll: req.EnableIncludeAll,
	}

	if err := repo.UpdateTemplate(r.Context(), t); err != nil {
		if errors.Is(err, storage.ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, storage.ErrTemplateExists) {
			writeError(w, http.StatusConflict, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	updated, _ := repo.GetTemplateByID(r.Context(), id)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(templateToResponse(updated))
}

func handleDeleteTemplate(w http.ResponseWriter, r *http.Request, repo *storage.TrafficRepository, id int64) {
	if err := repo.DeleteTemplate(r.Context(), id); err != nil {
		if errors.Is(err, storage.ErrTemplateNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func templateToResponse(t storage.Template) templateResponse {
	return templateResponse{
		ID:               t.ID,
		Name:             t.Name,
		Category:         t.Category,
		TemplateURL:      t.TemplateURL,
		RuleSource:       t.RuleSource,
		UseProxy:         t.UseProxy,
		EnableIncludeAll: t.EnableIncludeAll,
		CreatedAt:        t.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:        t.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func fetchRemoteContent(url string, timeout time.Duration) (string, error) {
	// 注:此处为管理员专用(RequireAdmin)的规则源抓取,允许指向 LAN/自建源(自托管常见需求),
	// 故不套 SSRF 客户端。SSRF 防护只加在非管理员可达 / 用户可控 URL 的抓取路径上。
	client := &http.Client{Timeout: timeout}
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", errors.New("HTTP " + resp.Status)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	return string(body), nil
}

type fetchSourceRequest struct {
	URL      string `json:"url"`
	UseProxy bool   `json:"use_proxy"`
}

type fetchSourceResponse struct {
	Content string `json:"content"`
}

// NewTemplateFetchSourceHandler handles fetching template source file content
func NewTemplateFetchSourceHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeError(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
			return
		}

		var req fetchSourceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, err)
			return
		}

		if req.URL == "" {
			writeError(w, http.StatusBadRequest, errors.New("url is required"))
			return
		}

		// 如果启用代理，通过 1ms.cc 代理获取
		fetchURL := req.URL
		if req.UseProxy && !strings.HasPrefix(req.URL, "https://1ms.cc/") {
			fetchURL = "https://1ms.cc/" + req.URL
		}

		content, err := fetchRemoteContent(fetchURL, 30*time.Second)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(fetchSourceResponse{Content: content})
	})
}
