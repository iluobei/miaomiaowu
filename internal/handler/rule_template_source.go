package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// 「从链接导入」的规则模板可以再从链接更新:导入时记下来源链接,之后一键重新拉取覆盖。

// validTemplateSourceURL 来源链接只认 http / https。
func validTemplateSourceURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return "", fmt.Errorf("链接无效,只支持 http / https")
	}
	return raw, nil
}

// isRuleTemplateFile 规则模板文件名后缀(与上传接口一致)。
func isRuleTemplateFile(name string) bool {
	lower := strings.ToLower(name)
	return strings.HasSuffix(lower, ".yaml") || strings.HasSuffix(lower, ".yml") ||
		strings.HasSuffix(lower, ".conf") || strings.HasSuffix(lower, ".lcf")
}

// fetchTemplateSource 经防 SSRF 的客户端拉取来源链接内容:这个接口普通用户也能对自己的模板调用,
// 不能让服务端被指去打内网 / 云元数据。
func fetchTemplateSource(src string, timeout time.Duration) (string, error) {
	resp, err := newSSRFSafeHTTPClient(timeout).Get(src)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBodyBytes+1))
	if err != nil {
		return "", err
	}
	if len(body) > maxFetchBodyBytes {
		return "", fmt.Errorf("模板内容过大,不能超过 %dMB", maxFetchBodyBytes>>20)
	}
	return string(body), nil
}

// handleUpdateFromSource POST /api/admin/rule-templates/update-from-source {name, url}
// 重新拉取来源链接覆盖模板内容。url 不填用记下的来源;填了就用它并记下(老模板可以补上来源)。
func (h *RuleTemplatesHandler) handleUpdateFromSource(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		URL  string `json:"url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式错误")
		return
	}
	name := req.Name
	if name == "" || strings.Contains(name, "..") || strings.ContainsAny(name, `/\`) || !isRuleTemplateFile(name) {
		writeBadRequest(w, "模板名无效")
		return
	}
	if !h.canModify(r, name) {
		http.Error(w, "无权修改该模板", http.StatusForbidden)
		return
	}
	templatePath := filepath.Join("rule_templates", name)
	if _, err := os.Stat(templatePath); err != nil {
		writeError(w, http.StatusNotFound, fmt.Errorf("模板文件不存在"))
		return
	}
	src := strings.TrimSpace(req.URL)
	if src == "" {
		sources, _ := h.repo.ListRuleTemplateSources(r.Context())
		src = sources[name]
	}
	if src == "" {
		writeBadRequest(w, "这个模板没有记录来源链接,请填写链接")
		return
	}
	src, err := validTemplateSourceURL(src)
	if err != nil {
		writeBadRequest(w, err.Error())
		return
	}
	content, err := fetchTemplateSource(src, 30*time.Second)
	if err != nil {
		writeError(w, http.StatusBadGateway, fmt.Errorf("拉取失败: %v", err))
		return
	}
	if strings.TrimSpace(content) == "" {
		writeError(w, http.StatusBadGateway, fmt.Errorf("链接返回的内容是空的"))
		return
	}
	old, _ := os.ReadFile(templatePath)
	changed := string(old) != content
	if changed {
		// 先写临时文件再改名:拉到一半失败或写到一半断电,都不会留下半个模板
		tmp := templatePath + ".tmp"
		if err := os.WriteFile(tmp, []byte(content), 0644); err != nil {
			writeError(w, http.StatusInternalServerError, fmt.Errorf("保存失败"))
			return
		}
		if err := os.Rename(tmp, templatePath); err != nil {
			_ = os.Remove(tmp)
			writeError(w, http.StatusInternalServerError, fmt.Errorf("保存失败"))
			return
		}
	}
	_ = h.repo.SetRuleTemplateSource(r.Context(), name, src)
	respondJSON(w, http.StatusOK, map[string]any{"success": true, "changed": changed, "bytes": len(content), "source_url": src})
}
