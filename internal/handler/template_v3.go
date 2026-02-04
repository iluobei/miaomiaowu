package handler

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/substore"

	"gopkg.in/yaml.v3"
)

// TemplateV3Handler handles v3 template operations
type TemplateV3Handler struct {
	repo *storage.TrafficRepository
}

// NewTemplateV3Handler creates a new v3 template handler
func NewTemplateV3Handler(repo *storage.TrafficRepository) *TemplateV3Handler {
	return &TemplateV3Handler{repo: repo}
}

// ServeHTTP handles HTTP requests for v3 template operations
func (h *TemplateV3Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/template-v3")

	switch {
	case path == "/process" || path == "/process/":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handleProcessTemplate(w, r)
	case path == "/preview" || path == "/preview/":
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		h.handlePreviewTemplate(w, r)
	default:
		http.Error(w, "Not found", http.StatusNotFound)
	}
}

// processTemplateRequest represents the request body for processing a v3 template
type processTemplateRequest struct {
	TemplateName string           `json:"template_name"` // Name of template file in rule_templates/
	Proxies      []map[string]any `json:"proxies"`       // List of proxy nodes to inject
}

// previewTemplateRequest represents the request body for previewing a v3 template
type previewTemplateRequest struct {
	TemplateContent string           `json:"template_content"` // Raw template content
	Proxies         []map[string]any `json:"proxies"`          // List of proxy nodes to inject
}

// handleProcessTemplate processes a v3 template file with provided proxies
func (h *TemplateV3Handler) handleProcessTemplate(w http.ResponseWriter, r *http.Request) {
	var req processTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	templateName := strings.TrimSpace(req.TemplateName)
	if templateName == "" {
		writeJSONError(w, http.StatusBadRequest, "模板名称不能为空")
		return
	}

	// Security: Prevent directory traversal
	if strings.Contains(templateName, "..") || strings.Contains(templateName, "/") || strings.Contains(templateName, "\\") {
		writeJSONError(w, http.StatusBadRequest, "无效的模板名称")
		return
	}

	// Read template file
	templatesDir := "rule_templates"
	templatePath := filepath.Join(templatesDir, templateName)

	content, err := os.ReadFile(templatePath)
	if err != nil {
		if os.IsNotExist(err) {
			writeJSONError(w, http.StatusNotFound, "模板文件不存在")
		} else {
			writeJSONError(w, http.StatusInternalServerError, "读取模板文件失败")
		}
		return
	}

	// Process the template
	result, err := h.processV3Template(string(content), req.Proxies)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "处理模板失败: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"content": result,
	})
}

// handlePreviewTemplate previews a v3 template with provided content and proxies
func (h *TemplateV3Handler) handlePreviewTemplate(w http.ResponseWriter, r *http.Request) {
	var req previewTemplateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "无效的请求格式")
		return
	}

	if strings.TrimSpace(req.TemplateContent) == "" {
		writeJSONError(w, http.StatusBadRequest, "模板内容不能为空")
		return
	}

	// Process the template
	result, err := h.processV3Template(req.TemplateContent, req.Proxies)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "处理模板失败: "+err.Error())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{
		"content": result,
	})
}

// processV3Template processes a v3 template with the given proxies
func (h *TemplateV3Handler) processV3Template(templateContent string, proxies []map[string]any) (string, error) {
	// Create processor with empty providers (v3 doesn't use external providers)
	processor := substore.NewTemplateV3Processor(nil, nil)

	// Process the template
	result, err := processor.ProcessTemplate(templateContent, proxies)
	if err != nil {
		return "", err
	}

	// Inject proxies into the result
	result, err = injectProxiesIntoTemplate(result, proxies)
	if err != nil {
		return "", err
	}

	return result, nil
}

// injectProxiesIntoTemplate injects proxy nodes into the template's proxies section
func injectProxiesIntoTemplate(templateContent string, proxies []map[string]any) (string, error) {
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(templateContent), &root); err != nil {
		return "", err
	}

	if root.Kind != yaml.DocumentNode || len(root.Content) == 0 {
		return templateContent, nil
	}

	rootMap := root.Content[0]
	if rootMap.Kind != yaml.MappingNode {
		return templateContent, nil
	}

	// Find proxies key and inject nodes
	for i := 0; i < len(rootMap.Content); i += 2 {
		keyNode := rootMap.Content[i]
		if keyNode.Value == "proxies" {
			// Create new proxies sequence
			proxiesNode := &yaml.Node{
				Kind: yaml.SequenceNode,
				Tag:  "!!seq",
			}

			// Add each proxy as a mapping node
			for _, proxy := range proxies {
				proxyNode := mapToYAMLNode(proxy)
				proxiesNode.Content = append(proxiesNode.Content, proxyNode)
			}

			rootMap.Content[i+1] = proxiesNode
			break
		}
	}

	// Marshal back to YAML
	var buf strings.Builder
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&root); err != nil {
		return "", err
	}
	encoder.Close()

	// Post-process to remove quotes from emoji strings and convert Unicode escapes
	result := RemoveUnicodeEscapeQuotes(buf.String())
	return result, nil
}

// mapToYAMLNode converts a map to a YAML mapping node
func mapToYAMLNode(m map[string]any) *yaml.Node {
	node := &yaml.Node{
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}

	// Define preferred key order for proxy nodes
	keyOrder := []string{"name", "type", "server", "port", "password", "uuid", "alterId", "cipher", "udp", "tls", "skip-cert-verify", "sni", "servername", "network", "ws-opts", "grpc-opts", "reality-opts", "flow", "client-fingerprint"}

	// Add keys in preferred order first
	addedKeys := make(map[string]bool)
	for _, key := range keyOrder {
		if value, ok := m[key]; ok {
			addKeyValueToNode(node, key, value)
			addedKeys[key] = true
		}
	}

	// Add remaining keys
	for key, value := range m {
		if !addedKeys[key] {
			addKeyValueToNode(node, key, value)
		}
	}

	return node
}

// addKeyValueToNode adds a key-value pair to a YAML mapping node
func addKeyValueToNode(node *yaml.Node, key string, value any) {
	keyNode := &yaml.Node{
		Kind:  yaml.ScalarNode,
		Tag:   "!!str",
		Value: key,
	}

	valueNode := anyToYAMLNode(value)
	node.Content = append(node.Content, keyNode, valueNode)
}

// anyToYAMLNode converts any value to a YAML node
func anyToYAMLNode(v any) *yaml.Node {
	switch val := v.(type) {
	case string:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: val,
		}
	case int:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!int",
			Value: intToString(val),
		}
	case int64:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!int",
			Value: int64ToString(val),
		}
	case float64:
		// Check if it's actually an integer
		if val == float64(int(val)) {
			return &yaml.Node{
				Kind:  yaml.ScalarNode,
				Tag:   "!!int",
				Value: intToString(int(val)),
			}
		}
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!float",
			Value: floatToString(val),
		}
	case bool:
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!bool",
			Value: boolToString(val),
		}
	case []any:
		seqNode := &yaml.Node{
			Kind: yaml.SequenceNode,
			Tag:  "!!seq",
		}
		for _, item := range val {
			seqNode.Content = append(seqNode.Content, anyToYAMLNode(item))
		}
		return seqNode
	case map[string]any:
		return mapToYAMLNode(val)
	default:
		// Fallback: convert to string
		return &yaml.Node{
			Kind:  yaml.ScalarNode,
			Tag:   "!!str",
			Value: "",
		}
	}
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	var result []byte
	negative := n < 0
	if negative {
		n = -n
	}
	for n > 0 {
		result = append([]byte{byte('0' + n%10)}, result...)
		n /= 10
	}
	if negative {
		result = append([]byte{'-'}, result...)
	}
	return string(result)
}

func int64ToString(n int64) string {
	return intToString(int(n))
}

func floatToString(f float64) string {
	// Simple float to string conversion
	return strings.TrimRight(strings.TrimRight(
		strings.Replace(string(rune(int(f))), "", "", -1),
		"0"), ".")
}

func boolToString(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
