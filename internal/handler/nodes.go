package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"miaomiaowu/internal/logger"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

// convertNilToEmptyStringInMap recursively converts nil values to empty strings in a map
func convertNilToEmptyStringInMap(m map[string]any) {
	for k, v := range m {
		if v == nil {
			m[k] = ""
		} else if subMap, ok := v.(map[string]any); ok {
			convertNilToEmptyStringInMap(subMap)
		} else if slice, ok := v.([]any); ok {
			for i, item := range slice {
				if item == nil {
					slice[i] = ""
				} else if itemMap, ok := item.(map[string]any); ok {
					convertNilToEmptyStringInMap(itemMap)
				}
			}
		}
	}
}

// safeURLDecode 安全地进行 URL 解码，解码失败时返回原字符串
func safeURLDecode(s string) string {
	if s == "" {
		return s
	}
	decoded, err := url.QueryUnescape(s)
	if err != nil {
		return s
	}
	return decoded
}

// decodeProxyURLFields 对代理节点中可能包含 URL 编码的字段进行解码
// 主要处理 path、host 等字段，支持 ws-opts、h2-opts、grpc-opts 等传输层配置
func decodeProxyURLFields(proxy map[string]any) {
	// 处理 ws-opts
	if wsOpts, ok := proxy["ws-opts"].(map[string]any); ok {
		if path, ok := wsOpts["path"].(string); ok {
			wsOpts["path"] = safeURLDecode(path)
		}
		if headers, ok := wsOpts["headers"].(map[string]any); ok {
			if host, ok := headers["Host"].(string); ok {
				headers["Host"] = safeURLDecode(host)
			}
		}
	}

	// 处理 h2-opts
	if h2Opts, ok := proxy["h2-opts"].(map[string]any); ok {
		if path, ok := h2Opts["path"].(string); ok {
			h2Opts["path"] = safeURLDecode(path)
		}
		if host, ok := h2Opts["host"].(string); ok {
			h2Opts["host"] = safeURLDecode(host)
		}
		// host 也可能是数组
		if hosts, ok := h2Opts["host"].([]any); ok {
			for i, h := range hosts {
				if hs, ok := h.(string); ok {
					hosts[i] = safeURLDecode(hs)
				}
			}
		}
	}

	// 处理 grpc-opts
	if grpcOpts, ok := proxy["grpc-opts"].(map[string]any); ok {
		if serviceName, ok := grpcOpts["grpc-service-name"].(string); ok {
			grpcOpts["grpc-service-name"] = safeURLDecode(serviceName)
		}
	}

	// 处理顶层的 path 和 host 字段（某些协议可能直接放在顶层）
	if path, ok := proxy["path"].(string); ok {
		proxy["path"] = safeURLDecode(path)
	}
	if host, ok := proxy["host"].(string); ok {
		proxy["host"] = safeURLDecode(host)
	}

	// 处理 sni 和 servername 字段（TLS 相关）
	if sni, ok := proxy["sni"].(string); ok {
		proxy["sni"] = safeURLDecode(sni)
	}
	if servername, ok := proxy["servername"].(string); ok {
		proxy["servername"] = safeURLDecode(servername)
	}
}

type nodesHandler struct {
	repo            *storage.TrafficRepository
	subscribeDir    string
	yamlSyncManager *YAMLSyncManager
}

// NewNodesHandler returns an admin-only handler that manages proxy nodes.
func NewNodesHandler(repo *storage.TrafficRepository, subscribeDir string) http.Handler {
	if repo == nil {
		panic("nodes handler requires repository")
	}

	return &nodesHandler{
		repo:            repo,
		subscribeDir:    subscribeDir,
		yamlSyncManager: NewYAMLSyncManager(subscribeDir),
	}
}

func (h *nodesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/admin/nodes")
	path = strings.Trim(path, "/")

	switch {
	case path == "" && r.Method == http.MethodGet:
		h.handleList(w, r)
	case path == "" && r.Method == http.MethodPost:
		h.handleCreate(w, r)
	case path == "batch" && r.Method == http.MethodPost:
		h.handleBatchCreate(w, r)
	case path == "fetch-subscription" && r.Method == http.MethodPost:
		h.handleFetchSubscription(w, r)
	case strings.HasSuffix(path, "/probe-binding") && r.Method == http.MethodPut:
		idSegment := strings.TrimSuffix(path, "/probe-binding")
		h.handleUpdateProbeBinding(w, r, idSegment)
	case strings.HasSuffix(path, "/server") && r.Method == http.MethodPut:
		idSegment := strings.TrimSuffix(path, "/server")
		h.handleUpdateServer(w, r, idSegment)
	case strings.HasSuffix(path, "/restore-server") && r.Method == http.MethodPut:
		idSegment := strings.TrimSuffix(path, "/restore-server")
		h.handleRestoreServer(w, r, idSegment)
	case strings.HasSuffix(path, "/config") && r.Method == http.MethodPut:
		idSegment := strings.TrimSuffix(path, "/config")
		h.handleUpdateConfig(w, r, idSegment)
	case path != "" && path != "batch" && path != "fetch-subscription" && !strings.HasSuffix(path, "/probe-binding") && !strings.HasSuffix(path, "/server") && !strings.HasSuffix(path, "/restore-server") && !strings.HasSuffix(path, "/config") && (r.Method == http.MethodPut || r.Method == http.MethodPatch):
		h.handleUpdate(w, r, path)
	case path != "" && path != "batch" && path != "fetch-subscription" && r.Method == http.MethodDelete:
		h.handleDelete(w, r, path)
	case path == "clear" && r.Method == http.MethodPost:
		h.handleClearAll(w, r)
	case path == "batch-delete" && r.Method == http.MethodPost:
		h.handleBatchDelete(w, r)
	case path == "batch-rename" && r.Method == http.MethodPost:
		h.handleBatchRename(w, r)
	default:
		allowed := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete}
		methodNotAllowed(w, allowed...)
	}
}

func (h *nodesHandler) handleList(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	nodes, err := h.repo.ListNodes(r.Context(), username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"nodes": convertNodes(nodes),
	})
}

func (h *nodesHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	var req nodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	// 校验节点名称不为空
	if strings.TrimSpace(req.NodeName) == "" {
		logger.Info("[节点创建] 节点名称为空")
		writeBadRequest(w, "节点名称不能为空")
		return
	}

	// 校验节点名称是否重复（数据库层面）
	exists, err := h.repo.CheckNodeNameExists(r.Context(), req.NodeName, username, 0)
	if err != nil {
		logger.Info("[节点创建] 检查节点名称重复失败", "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("服务器错误"))
		return
	}
	if exists {
		logger.Info("[节点创建] 节点名称重复", "node_name", req.NodeName)
		writeBadRequest(w, fmt.Sprintf("节点名称 \"%s\" 已存在，请使用其他名称", req.NodeName))
		return
	}

	// 校验Clash配置格式
	if req.ClashConfig != "" {
		var clashConfig map[string]interface{}
		if err := json.Unmarshal([]byte(req.ClashConfig), &clashConfig); err != nil {
			logger.Info("[节点创建] Clash配置格式错误", "error", err)
			writeBadRequest(w, "Clash配置格式错误")
			return
		}

		// 确保配置中的name与节点名称一致
		if configName, ok := clashConfig["name"].(string); !ok || configName != req.NodeName {
			logger.Info("[节点创建] 配置name不匹配: 节点名=, 配置名", "node_name", req.NodeName, "param", clashConfig["name"])
			writeBadRequest(w, "Clash配置中的name字段必须与节点名称一致")
			return
		}
	}

	logger.Info("[节点创建] 校验通过 - 节点名称, 用户", "node_name", req.NodeName, "user", username)

	node := storage.Node{
		Username:     username,
		RawURL:       req.RawURL,
		NodeName:     req.NodeName,
		Protocol:     req.Protocol,
		ParsedConfig: req.ParsedConfig,
		ClashConfig:  req.ClashConfig,
		Enabled:      req.Enabled,
		Tag:          req.Tag,
	}

	created, err := h.repo.CreateNode(r.Context(), node)
	if err != nil {
		logger.Info("[节点创建] 数据库创建失败", "error", err)
		writeError(w, http.StatusBadRequest, err)
		return
	}

	logger.Info("[节点创建] 成功 - ID, 节点名称", "id", created.ID, "node_name", created.NodeName)

	respondJSON(w, http.StatusCreated, map[string]any{
		"node": convertNode(created),
	})
}

func (h *nodesHandler) handleBatchCreate(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	var req struct {
		Nodes []nodeRequest `json:"nodes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if len(req.Nodes) == 0 {
		writeBadRequest(w, "节点列表不能为空")
		return
	}

	nodes := make([]storage.Node, 0, len(req.Nodes))
	for _, n := range req.Nodes {
		// 允许 Clash 订阅节点没有 RawURL，但必须有 NodeName 和 ClashConfig
		if n.NodeName == "" || n.ClashConfig == "" {
			continue
		}
		nodes = append(nodes, storage.Node{
			Username:     username,
			RawURL:       n.RawURL, // 可以为空（Clash 订阅节点）
			NodeName:     n.NodeName,
			Protocol:     n.Protocol,
			ParsedConfig: n.ParsedConfig,
			ClashConfig:  n.ClashConfig,
			Enabled:      n.Enabled,
			Tag:          n.Tag,
		})
	}

	if len(nodes) == 0 {
		writeBadRequest(w, "没有有效的节点可以保存")
		return
	}

	created, err := h.repo.BatchCreateNodes(r.Context(), nodes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]any{
		"nodes": convertNodes(created),
	})
}

func (h *nodesHandler) handleUpdate(w http.ResponseWriter, r *http.Request, idSegment string) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的节点标识")
		return
	}

	existing, err := h.repo.GetNode(r.Context(), id, username)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	// Save old node name for YAML sync
	oldNodeName := existing.NodeName

	var req nodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	// 如果节点名称被修改，需要校验新名称
	if req.NodeName != "" && req.NodeName != oldNodeName {
		// 校验节点名称不为空
		if strings.TrimSpace(req.NodeName) == "" {
			logger.Info("[节点更新] 节点名称为空")
			writeBadRequest(w, "节点名称不能为空")
			return
		}

		// 校验节点名称是否重复（数据库层面）
		exists, err := h.repo.CheckNodeNameExists(r.Context(), req.NodeName, username, id)
		if err != nil {
			logger.Info("[节点更新] 检查节点名称重复失败", "error", err)
			writeError(w, http.StatusInternalServerError, errors.New("服务器错误"))
			return
		}
		if exists {
			logger.Info("[节点更新] 节点名称重复", "node_name", req.NodeName)
			writeBadRequest(w, fmt.Sprintf("节点名称 \"%s\" 已存在，请使用其他名称", req.NodeName))
			return
		}
	}

	// 如果Clash配置被修改，需要校验格式
	if req.ClashConfig != "" {
		var clashConfig map[string]interface{}
		if err := json.Unmarshal([]byte(req.ClashConfig), &clashConfig); err != nil {
			logger.Info("[节点更新] Clash配置格式错误", "error", err)
			writeBadRequest(w, "Clash配置格式错误")
			return
		}

		// 确保配置中的name与节点名称一致
		newNodeName := req.NodeName
		if newNodeName == "" {
			newNodeName = oldNodeName
		}
		if configName, ok := clashConfig["name"].(string); !ok || configName != newNodeName {
			logger.Info("[节点更新] 配置name不匹配: 节点名=, 配置名", "value", newNodeName, "param", clashConfig["name"])
			writeBadRequest(w, "Clash配置中的name字段必须与节点名称一致")
			return
		}
	}

	logger.Info("[节点更新] 校验通过 - 节点ID, 旧名称, 新名称", "value", id, "param", oldNodeName, "node_name", req.NodeName)

	// Update fields
	if req.RawURL != "" {
		existing.RawURL = req.RawURL
	}
	if req.NodeName != "" {
		existing.NodeName = req.NodeName
	}
	if req.Protocol != "" {
		existing.Protocol = req.Protocol
	}
	if req.ParsedConfig != "" {
		existing.ParsedConfig = req.ParsedConfig
	}
	if req.ClashConfig != "" {
		existing.ClashConfig = req.ClashConfig
	}
	if req.Tag != "" {
		existing.Tag = req.Tag
	}
	existing.Enabled = req.Enabled

	updated, err := h.repo.UpdateNode(r.Context(), existing)
	if err != nil {
		logger.Info("[节点更新] 数据库更新失败", "error", err)
		status := http.StatusBadRequest
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	logger.Info("[节点更新] 数据库更新成功 - 节点ID, 节点名称", "id", updated.ID, "node_name", updated.NodeName)

	// Sync node changes to YAML files using the sync manager
	if updated.ClashConfig != "" {
		newNodeName := updated.NodeName
		if err := h.yamlSyncManager.SyncNode(oldNodeName, newNodeName, updated.ClashConfig); err != nil {
			// Log error but don't fail the request
			// The node update was successful, YAML sync is best-effort
			// You could add logging here if needed
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"node": convertNode(updated),
	})
}

func (h *nodesHandler) handleUpdateServer(w http.ResponseWriter, r *http.Request, idSegment string) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的节点标识")
		return
	}

	existing, err := h.repo.GetNode(r.Context(), id, username)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	var req struct {
		Server string `json:"server"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.Server == "" {
		writeBadRequest(w, "服务器地址不能为空")
		return
	}

	// Save original server before updating (only if not already saved)
	if existing.OriginalServer == "" {
		var currentClashConfig map[string]any
		if err := json.Unmarshal([]byte(existing.ClashConfig), &currentClashConfig); err == nil {
			if currentServer, ok := currentClashConfig["server"].(string); ok && currentServer != "" {
				existing.OriginalServer = currentServer
			}
		}
	}

	// 更新 ParsedConfig 中的 server 字段
	var parsedConfig map[string]any
	if err := json.Unmarshal([]byte(existing.ParsedConfig), &parsedConfig); err == nil {
		parsedConfig["server"] = req.Server
		if updatedParsed, err := json.Marshal(parsedConfig); err == nil {
			existing.ParsedConfig = string(updatedParsed)
		}
	}

	// 更新 ClashConfig 中的 server 字段
	var clashConfig map[string]any
	if err := json.Unmarshal([]byte(existing.ClashConfig), &clashConfig); err == nil {
		clashConfig["server"] = req.Server
		if updatedClash, err := json.Marshal(clashConfig); err == nil {
			existing.ClashConfig = string(updatedClash)
		}
	}

	updated, err := h.repo.UpdateNode(r.Context(), existing)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	// Sync node changes to YAML files (server address update) using the sync manager
	if updated.ClashConfig != "" {
		nodeName := updated.NodeName
		if err := h.yamlSyncManager.SyncNode(nodeName, nodeName, updated.ClashConfig); err != nil {
			// Log error but don't fail the request
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"node": convertNode(updated),
	})
}

func (h *nodesHandler) handleRestoreServer(w http.ResponseWriter, r *http.Request, idSegment string) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的节点标识")
		return
	}

	existing, err := h.repo.GetNode(r.Context(), id, username)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	// Check if original server exists
	if existing.OriginalServer == "" {
		writeBadRequest(w, "节点没有保存原始域名")
		return
	}

	// Restore server address from original_server
	originalServer := existing.OriginalServer

	// 更新 ParsedConfig 中的 server 字段
	var parsedConfig map[string]any
	if err := json.Unmarshal([]byte(existing.ParsedConfig), &parsedConfig); err == nil {
		parsedConfig["server"] = originalServer
		if updatedParsed, err := json.Marshal(parsedConfig); err == nil {
			existing.ParsedConfig = string(updatedParsed)
		}
	}

	// 更新 ClashConfig 中的 server 字段
	var clashConfig map[string]any
	if err := json.Unmarshal([]byte(existing.ClashConfig), &clashConfig); err == nil {
		clashConfig["server"] = originalServer
		if updatedClash, err := json.Marshal(clashConfig); err == nil {
			existing.ClashConfig = string(updatedClash)
		}
	}

	// Clear original_server after restoring
	existing.OriginalServer = ""

	updated, err := h.repo.UpdateNode(r.Context(), existing)
	if err != nil {
		status := http.StatusBadRequest
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	// Sync node changes to YAML files (restore server address) using the sync manager
	if updated.ClashConfig != "" {
		nodeName := updated.NodeName
		if err := h.yamlSyncManager.SyncNode(nodeName, nodeName, updated.ClashConfig); err != nil {
			// Log error but don't fail the request
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"node": convertNode(updated),
	})
}

func (h *nodesHandler) handleUpdateConfig(w http.ResponseWriter, r *http.Request, idSegment string) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的节点标识")
		return
	}

	var req struct {
		ClashConfig string `json:"clash_config"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	// Validate JSON format
	var clashConfigMap map[string]interface{}
	if err := json.Unmarshal([]byte(req.ClashConfig), &clashConfigMap); err != nil {
		writeBadRequest(w, "Clash 配置格式不正确: "+err.Error())
		return
	}

	// Validate required fields
	requiredFields := []string{"name", "type", "server", "port"}
	for _, field := range requiredFields {
		if _, ok := clashConfigMap[field]; !ok {
			writeBadRequest(w, fmt.Sprintf("配置缺少必需字段: %s", field))
			return
		}
	}

	// Get existing node
	node, err := h.repo.GetNode(r.Context(), id, username)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	oldNodeName := node.NodeName

	// Update node's ClashConfig and ParsedConfig
	node.ClashConfig = req.ClashConfig
	node.ParsedConfig = req.ClashConfig

	// Update node name from the config if changed
	if nameValue, ok := clashConfigMap["name"]; ok {
		if newName, ok := nameValue.(string); ok && newName != "" {
			node.NodeName = newName
		}
	}

	// Update node in database
	updated, err := h.repo.UpdateNode(r.Context(), node)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	// Sync to YAML subscription files using the sync manager
	if updated.ClashConfig != "" {
		// If node name changed, update old name to new name in YAML files
		newNodeName := updated.NodeName
		if err := h.yamlSyncManager.SyncNode(oldNodeName, newNodeName, updated.ClashConfig); err != nil {
			// Log error but don't fail the request
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"node": convertNode(updated),
	})
}

func (h *nodesHandler) handleDelete(w http.ResponseWriter, r *http.Request, idSegment string) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	id, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || id <= 0 {
		writeBadRequest(w, "无效的节点标识")
		return
	}

	// Get node name before deletion for YAML sync
	node, err := h.repo.GetNode(r.Context(), id, username)
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	if err := h.repo.DeleteNode(r.Context(), id, username); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, storage.ErrNodeNotFound) {
			status = http.StatusNotFound
		}
		writeError(w, status, err)
		return
	}

	// Sync deletion to YAML files using the sync manager
	if node.NodeName != "" {
		if err := h.yamlSyncManager.DeleteNode(node.NodeName); err != nil {
			// Log error but don't fail the request
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *nodesHandler) handleClearAll(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	if err := h.repo.DeleteAllUserNodes(r.Context(), username); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"status": "cleared"})
}

func (h *nodesHandler) handleBatchDelete(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	var req struct {
		NodeIDs []int64 `json:"node_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if len(req.NodeIDs) == 0 {
		writeBadRequest(w, "节点ID列表不能为空")
		return
	}

	// Get all node names before deletion for YAML sync
	nodeNames := make([]string, 0, len(req.NodeIDs))
	for _, id := range req.NodeIDs {
		node, err := h.repo.GetNode(r.Context(), id, username)
		if err != nil {
			// Skip nodes that don't exist or can't be accessed
			continue
		}
		if node.NodeName != "" {
			nodeNames = append(nodeNames, node.NodeName)
		}
	}

	// Delete nodes from database
	deletedCount := 0
	for _, id := range req.NodeIDs {
		if err := h.repo.DeleteNode(r.Context(), id, username); err != nil {
			// Continue with other deletions even if one fails
			continue
		}
		deletedCount++
	}

	// Batch sync deletion to YAML files using the sync manager
	// This is done in a single locked operation for efficiency
	if len(nodeNames) > 0 {
		if err := h.yamlSyncManager.BatchDeleteNodes(nodeNames); err != nil {
			// Log error but don't fail the request
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "deleted",
		"deleted": deletedCount,
		"total":   len(req.NodeIDs),
	})
}

func (h *nodesHandler) handleBatchRename(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	var req struct {
		Updates []struct {
			NodeID  int64  `json:"node_id"`
			NewName string `json:"new_name"`
		} `json:"updates"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if len(req.Updates) == 0 {
		writeBadRequest(w, "更新列表不能为空")
		return
	}

	successCount := 0
	failCount := 0
	var updatedNodes []nodeDTO
	var yamlUpdates []NodeUpdate // 收集 YAML 同步更新

	for _, update := range req.Updates {
		if update.NewName == "" {
			failCount++
			continue
		}

		// Get existing node
		node, err := h.repo.GetNode(r.Context(), update.NodeID, username)
		if err != nil {
			failCount++
			continue
		}

		// Save old name for YAML sync
		oldNodeName := node.NodeName

		// Update node name
		node.NodeName = update.NewName

		// Update name in ClashConfig JSON
		var clashConfig map[string]any
		if err := json.Unmarshal([]byte(node.ClashConfig), &clashConfig); err == nil {
			clashConfig["name"] = update.NewName
			if updatedClash, err := json.Marshal(clashConfig); err == nil {
				node.ClashConfig = string(updatedClash)
			}
		}

		// Update name in ParsedConfig JSON
		var parsedConfig map[string]any
		if err := json.Unmarshal([]byte(node.ParsedConfig), &parsedConfig); err == nil {
			parsedConfig["name"] = update.NewName
			if updatedParsed, err := json.Marshal(parsedConfig); err == nil {
				node.ParsedConfig = string(updatedParsed)
			}
		}

		// Save to database
		updated, err := h.repo.UpdateNode(r.Context(), node)
		if err != nil {
			failCount++
			continue
		}

		// 收集 YAML 同步更新（不立即同步）
		if updated.ClashConfig != "" {
			yamlUpdates = append(yamlUpdates, NodeUpdate{
				OldName:         oldNodeName,
				NewName:         update.NewName,
				ClashConfigJSON: updated.ClashConfig,
			})
		}

		successCount++
		updatedNodes = append(updatedNodes, convertNode(updated))
	}

	// 批量同步到 YAML 文件（只读写文件一次）
	if len(yamlUpdates) > 0 {
		if err := h.yamlSyncManager.BatchSyncNodes(yamlUpdates); err != nil {
			// Log error but don't fail the request
			logger.Info("[批量重命名] YAML 同步失败", "error", err)
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"status":  "renamed",
		"success": successCount,
		"failed":  failCount,
		"total":   len(req.Updates),
		"nodes":   updatedNodes,
	})
}

type nodeRequest struct {
	RawURL       string `json:"raw_url"`
	NodeName     string `json:"node_name"`
	Protocol     string `json:"protocol"`
	ParsedConfig string `json:"parsed_config"`
	ClashConfig  string `json:"clash_config"`
	Enabled      bool   `json:"enabled"`
	Tag          string `json:"tag"`
}

type nodeDTO struct {
	ID             int64     `json:"id"`
	RawURL         string    `json:"raw_url"`
	NodeName       string    `json:"node_name"`
	Protocol       string    `json:"protocol"`
	ParsedConfig   string    `json:"parsed_config"`
	ClashConfig    string    `json:"clash_config"`
	Enabled        bool      `json:"enabled"`
	Tag            string    `json:"tag"`
	OriginalServer string    `json:"original_server"`
	ProbeServer    string    `json:"probe_server"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

func convertNode(node storage.Node) nodeDTO {
	return nodeDTO{
		ID:             node.ID,
		RawURL:         node.RawURL,
		NodeName:       node.NodeName,
		Protocol:       node.Protocol,
		ParsedConfig:   node.ParsedConfig,
		ClashConfig:    node.ClashConfig,
		Enabled:        node.Enabled,
		Tag:            node.Tag,
		OriginalServer: node.OriginalServer,
		ProbeServer:    node.ProbeServer,
		CreatedAt:      node.CreatedAt,
		UpdatedAt:      node.UpdatedAt,
	}
}

func convertNodes(nodes []storage.Node) []nodeDTO {
	result := make([]nodeDTO, 0, len(nodes))
	for _, node := range nodes {
		result = append(result, convertNode(node))
	}
	return result
}

func (h *nodesHandler) handleFetchSubscription(w http.ResponseWriter, r *http.Request) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	var req struct {
		URL       string `json:"url"`
		UserAgent string `json:"user_agent"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if req.URL == "" {
		writeBadRequest(w, "订阅URL是必填项")
		return
	}

	// 如果没有提供 User-Agent，使用默认值
	userAgent := req.UserAgent
	if userAgent == "" {
		userAgent = "clash-meta/2.4.0"
	}

	// 创建HTTP客户端并获取订阅内容
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	httpReq, err := http.NewRequest("GET", req.URL, nil)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("无效的订阅URL"))
		return
	}

	// 添加User-Agent头
	httpReq.Header.Set("User-Agent", userAgent)

	logger.Info("[订阅获取] 开始请求外部订阅", "url", req.URL, "user_agent", userAgent)

	resp, err := client.Do(httpReq)
	if err != nil {
		logger.Info("[订阅获取] 请求失败", "url", req.URL, "error", err)
		writeError(w, http.StatusBadRequest, errors.New("无法获取订阅内容: "+err.Error()))
		return
	}
	defer resp.Body.Close()

	logger.Info("[订阅获取] 收到响应",
		"url", req.URL,
		"status_code", resp.StatusCode,
		"status", resp.Status,
		"content_type", resp.Header.Get("Content-Type"),
		"content_length", resp.ContentLength)

	// 读取响应内容（无论成功还是失败都需要读取以便记录日志）
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		logger.Info("[订阅获取] 读取响应体失败", "url", req.URL, "error", err)
		writeError(w, http.StatusInternalServerError, errors.New("读取订阅内容失败"))
		return
	}

	logger.Info("[订阅获取] 响应体大小", "url", req.URL, "size", len(body))

	if resp.StatusCode != http.StatusOK {
		// 记录详细的错误响应内容
		bodyPreview := string(body)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500] + "...(截断)"
		}
		logger.Info("[订阅获取] 服务器返回错误状态",
			"url", req.URL,
			"status_code", resp.StatusCode,
			"status", resp.Status,
			"response_preview", bodyPreview)
		writeError(w, http.StatusBadRequest, fmt.Errorf("订阅服务器返回错误状态: %d %s", resp.StatusCode, resp.Status))
		return
	}

	// 解析YAML
	var clashConfig struct {
		Proxies []map[string]any `yaml:"proxies"`
	}

	if err := yaml.Unmarshal(body, &clashConfig); err != nil {
		// 记录解析失败时的内容预览
		bodyPreview := string(body)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500] + "...(截断)"
		}
		logger.Info("[订阅获取] YAML解析失败", "url", req.URL, "error", err, "content_preview", bodyPreview)
		writeError(w, http.StatusBadRequest, errors.New("解析订阅内容失败: "+err.Error()))
		return
	}

	if len(clashConfig.Proxies) == 0 {
		// 记录没有找到节点时的内容预览
		bodyPreview := string(body)
		if len(bodyPreview) > 500 {
			bodyPreview = bodyPreview[:500] + "...(截断)"
		}
		logger.Info("[订阅获取] 订阅中没有找到代理节点", "url", req.URL, "content_preview", bodyPreview)
		writeError(w, http.StatusBadRequest, errors.New("订阅中没有找到代理节点"))
		return
	}

	logger.Info("[订阅获取] 成功解析订阅", "url", req.URL, "node_count", len(clashConfig.Proxies))

	// Convert nil values to empty strings and decode URL-encoded fields in all proxies
	for _, proxy := range clashConfig.Proxies {
		convertNilToEmptyStringInMap(proxy)
		decodeProxyURLFields(proxy)
	}

	// 从 Content-Disposition 头中提取订阅名称作为建议的标签
	suggestedTag := ""
	contentDisposition := resp.Header.Get("Content-Disposition")
	if contentDisposition != "" {
		suggestedTag = parseFilenameFromContentDisposition(contentDisposition)
		// 移除文件扩展名
		if suggestedTag != "" {
			suggestedTag = strings.TrimSuffix(suggestedTag, ".yaml")
			suggestedTag = strings.TrimSuffix(suggestedTag, ".yml")
			suggestedTag = strings.TrimSuffix(suggestedTag, ".txt")
		}
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"proxies":       clashConfig.Proxies,
		"count":         len(clashConfig.Proxies),
		"suggested_tag": suggestedTag,
	})
}

// handleUpdateProbeBinding updates the probe server binding for a node.
func (h *nodesHandler) handleUpdateProbeBinding(w http.ResponseWriter, r *http.Request, idSegment string) {
	username := auth.UsernameFromContext(r.Context())
	if username == "" {
		writeError(w, http.StatusUnauthorized, errors.New("用户未认证"))
		return
	}

	nodeID, err := strconv.ParseInt(idSegment, 10, 64)
	if err != nil || nodeID <= 0 {
		writeBadRequest(w, "无效的节点ID")
		return
	}

	var req struct {
		ProbeServer string `json:"probe_server"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeBadRequest(w, "请求格式不正确")
		return
	}

	if err := h.repo.UpdateNodeProbeServer(r.Context(), nodeID, username, req.ProbeServer); err != nil {
		if errors.Is(err, storage.ErrNodeNotFound) {
			writeError(w, http.StatusNotFound, errors.New("节点不存在"))
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	node, err := h.repo.GetNode(r.Context(), nodeID, username)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	respondJSON(w, http.StatusOK, map[string]any{
		"node": convertNode(node),
	})
}
