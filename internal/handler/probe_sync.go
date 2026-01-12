package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"miaomiaowu/internal/logger"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"miaomiaowu/internal/storage"
)

type probeSyncHandler struct {
	client *http.Client
	repo   *storage.TrafficRepository
}

type probeSyncRequest struct {
	ProbeType string `json:"probe_type"`
	Address   string `json:"address"`
}

type probeSyncServer struct {
	ServerID         string  `json:"server_id"`
	Name             string  `json:"name"`
	TrafficMethod    string  `json:"traffic_method"`
	MonthlyTrafficGB float64 `json:"monthly_traffic_gb"`
}

type probeSyncResponse struct {
	Servers []probeSyncServer `json:"servers"`
}

func NewProbeSyncHandler(repo *storage.TrafficRepository) http.Handler {
	if repo == nil {
		panic("probe sync handler requires repository")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	return &probeSyncHandler{client: client, repo: repo}
}

func (h *probeSyncHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w, http.MethodPost)
		return
	}

	var payload probeSyncRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&payload); err != nil {
		writeBadRequest(w, "请求数据格式错误")
		return
	}

	probeType := strings.ToLower(strings.TrimSpace(payload.ProbeType))
	// 浏览器地址栏粘贴过来的url带有/后缀, 这里去除防止接口报错
	address := strings.TrimRight(strings.TrimSpace(payload.Address), "/")

	if address == "" {
		writeBadRequest(w, "探针地址不能为空")
		return
	}

	var servers []probeSyncServer
	var err error

	logger.Info("[探针同步] 开始获取探针信息: 类型=%s, 地址=%s", probeType, address)

	switch probeType {
	case storage.ProbeTypeNezha:
		servers, err = h.fetchNezhaServers(r.Context(), address)
	case storage.ProbeTypeNezhaV0:
		servers, err = h.fetchNezhaV0Servers(r.Context(), address)
	case storage.ProbeTypeDstatus:
		servers, err = h.fetchDstatusServers(r.Context(), address)
	case storage.ProbeTypeKomari:
		servers, err = h.fetchKomariServers(r.Context(), address)
	default:
		logger.Info("[探针同步] 不支持的探针类型: %s", probeType)
		writeBadRequest(w, "不支持的探针类型")
		return
	}

	if err != nil {
		logger.Info("[探针同步] 获取探针信息失败: 类型=%s, 地址=%s, 错误=%v", probeType, address, err)
		writeError(w, http.StatusBadGateway, err)
		return
	}

	logger.Info("[探针同步] 成功获取探针信息: 类型=%s, 地址=%s, 服务器数量=%d", probeType, address, len(servers))
	respondJSON(w, http.StatusOK, probeSyncResponse{Servers: servers})
}

func (h *probeSyncHandler) fetchNezhaServers(ctx context.Context, address string) ([]probeSyncServer, error) {
	logger.Info("[探针同步-Nezha] 开始解析地址: %s", address)

	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		logger.Info("[探针同步-Nezha] 地址解析失败: %s, 错误=%v", address, err)
		return nil, fmt.Errorf("invalid probe address: %w", err)
	}

	switch strings.ToLower(base.Scheme) {
	case "", "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	case "ws", "wss":
		// keep as is
	default:
		base.Scheme = "wss"
	}

	endpoint := &url.URL{Path: "/api/v1/ws/server"}
	target := base.ResolveReference(endpoint)

	logger.Info("[探针同步-Nezha] 连接 WebSocket: %s", target.String())

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.DefaultDialer.DialContext(dialCtx, target.String(), nil)
	if err != nil {
		var respInfo string
		if resp != nil {
			bodyBytes, _ := io.ReadAll(resp.Body)
			respInfo = fmt.Sprintf("状态码=%d, 响应=%s", resp.StatusCode, string(bodyBytes))
			resp.Body.Close()
		} else {
			respInfo = "无响应"
		}
		logger.Info("[探针同步-Nezha] WebSocket 连接失败: URL=%s, 错误=%v, 响应信息=%s", target.String(), err, respInfo)
		return nil, fmt.Errorf("无法连接到 WebSocket 接口: %w", err)
	}
	defer conn.Close()

	logger.Info("[探针同步-Nezha] WebSocket 连接成功，等待数据...")

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		logger.Info("[探针同步-Nezha] 设置读取超时失败: %v", err)
		return nil, fmt.Errorf("set websocket deadline: %w", err)
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		logger.Info("[探针同步-Nezha] 读取 WebSocket 消息失败: %v", err)
		return nil, fmt.Errorf("未在期望时间内收到服务器数据: %w", err)
	}

	logger.Info("[探针同步-Nezha] 收到消息，长度=%d 字节", len(message))

	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		logger.Info("[探针同步-Nezha] 收到空消息")
		return nil, errors.New("empty probe websocket payload")
	}

	type nezhaServer struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	}

	type nezhaSnapshot struct {
		Servers []nezhaServer `json:"servers"`
	}

	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.UseNumber()

	var snapshot nezhaSnapshot

	// 记录消息预览（最多500字节）
	msgPreview := string(message)
	if len(msgPreview) > 500 {
		msgPreview = msgPreview[:500] + "...(截断)"
	}

	if message[0] == '[' {
		var frames []nezhaSnapshot
		if err := decoder.Decode(&frames); err != nil {
			logger.Info("[探针同步-Nezha] JSON数组解析失败: 错误=%v, 内容预览=%s", err, msgPreview)
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
		if len(frames) == 0 {
			logger.Info("[探针同步-Nezha] 返回的数组为空")
			return nil, errors.New("探针未返回任何服务器数据")
		}
		snapshot = frames[len(frames)-1]
		logger.Info("[探针同步-Nezha] 解析到 %d 个数据帧，使用最后一帧", len(frames))
	} else {
		if err := decoder.Decode(&snapshot); err != nil {
			logger.Info("[探针同步-Nezha] JSON对象解析失败: 错误=%v, 内容预览=%s", err, msgPreview)
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
	}

	if len(snapshot.Servers) == 0 {
		logger.Info("[探针同步-Nezha] 服务器列表为空, 内容预览=%s", msgPreview)
		return nil, errors.New("探针未返回任何服务器数据")
	}

	logger.Info("[探针同步-Nezha] 成功解析到 %d 个服务器", len(snapshot.Servers))

	servers := make([]probeSyncServer, 0, len(snapshot.Servers))
	for i, srv := range snapshot.Servers {
		var id string
		if v, err := srv.ID.Int64(); err == nil {
			id = strconv.FormatInt(v, 10)
		} else {
			raw := strings.TrimSpace(srv.ID.String())
			if raw != "" {
				if strings.ContainsAny(raw, ".eE") {
					if f, err := srv.ID.Float64(); err == nil {
						id = strconv.FormatInt(int64(math.Round(f)), 10)
					} else {
						id = raw
					}
				} else {
					id = raw
				}
			} else if f, err := srv.ID.Float64(); err == nil {
				id = strconv.FormatInt(int64(math.Round(f)), 10)
			}
		}

		id = strings.TrimSpace(id)
		name := strings.TrimSpace(srv.Name)
		if name == "" {
			name = fmt.Sprintf("服务器 %d", i+1)
		}

		servers = append(servers, probeSyncServer{
			ServerID:         id,
			Name:             name,
			TrafficMethod:    "both",
			MonthlyTrafficGB: 0,
		})
	}

	return servers, nil
}

func (h *probeSyncHandler) fetchDstatusServers(ctx context.Context, address string) ([]probeSyncServer, error) {
	logger.Info("[探针同步-Dstatus] 开始解析地址: %s", address)

	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		logger.Info("[探针同步-Dstatus] 地址解析失败: %s, 错误=%v", address, err)
		return nil, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/servers"}
	target := base.ResolveReference(endpoint)

	logger.Info("[探针同步-Dstatus] 请求服务器列表: %s", target.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		logger.Info("[探针同步-Dstatus] 创建请求失败: %v", err)
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		logger.Info("[探针同步-Dstatus] HTTP请求失败: URL=%s, 错误=%v", target.String(), err)
		return nil, fmt.Errorf("服务器接口返回异常: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体以便记录日志
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyPreview := string(bodyBytes)
	if len(bodyPreview) > 500 {
		bodyPreview = bodyPreview[:500] + "...(截断)"
	}

	logger.Info("[探针同步-Dstatus] 收到响应: 状态码=%d, Content-Type=%s, 内容长度=%d",
		resp.StatusCode, resp.Header.Get("Content-Type"), len(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		logger.Info("[探针同步-Dstatus] 服务器返回错误状态: 状态码=%d, 响应内容=%s", resp.StatusCode, bodyPreview)
		return nil, fmt.Errorf("服务器接口返回异常: 状态码=%d", resp.StatusCode)
	}

	var serversResp struct {
		Data []struct {
			ID   interface{} `json:"id"`
			Name string      `json:"name"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()

	if err := decoder.Decode(&serversResp); err != nil {
		logger.Info("[探针同步-Dstatus] JSON解析失败: 错误=%v, 响应内容=%s", err, bodyPreview)
		return nil, fmt.Errorf("parse servers response: %w", err)
	}

	if len(serversResp.Data) == 0 {
		logger.Info("[探针同步-Dstatus] 服务器列表为空, 响应内容=%s", bodyPreview)
		return nil, errors.New("未从面板获取到服务器列表")
	}

	logger.Info("[探针同步-Dstatus] 成功获取 %d 个服务器", len(serversResp.Data))

	serverIDs := make([]string, 0, len(serversResp.Data))
	serverMap := make(map[string]string)

	for _, item := range serversResp.Data {
		var id string
		switch v := item.ID.(type) {
		case string:
			id = strings.TrimSpace(v)
		case json.Number:
			id = strings.TrimSpace(v.String())
		case float64:
			id = strconv.FormatInt(int64(v), 10)
		case int:
			id = strconv.Itoa(v)
		case int64:
			id = strconv.FormatInt(v, 10)
		default:
			id = fmt.Sprintf("%v", v)
		}

		if id == "" {
			continue
		}
		serverIDs = append(serverIDs, id)
		serverMap[id] = strings.TrimSpace(item.Name)
	}

	// Fetch traffic limits
	usageMap := make(map[string]float64)
	if len(serverIDs) > 0 {
		statsEndpoint := &url.URL{Path: "/stats/batch-traffic"}
		statsTarget := base.ResolveReference(statsEndpoint)

		payload, _ := json.Marshal(map[string][]string{"serverIds": serverIDs})
		statsReq, err := http.NewRequestWithContext(ctx, http.MethodPost, statsTarget.String(), bytes.NewReader(payload))
		if err == nil {
			statsReq.Header.Set("Content-Type", "application/json")
			statsReq.Header.Set("Accept", "application/json")

			statsResp, err := h.client.Do(statsReq)
			if err == nil {
				defer statsResp.Body.Close()

				if statsResp.StatusCode == http.StatusOK {
					var statsData struct {
						Data map[string]struct {
							Monthly struct {
								Limit json.Number `json:"limit"`
							} `json:"monthly"`
						} `json:"data"`
					}

					statsDecoder := json.NewDecoder(statsResp.Body)
					statsDecoder.UseNumber()

					if err := statsDecoder.Decode(&statsData); err == nil {
						for id, entry := range statsData.Data {
							if limitBytes, err := entry.Monthly.Limit.Int64(); err == nil && limitBytes > 0 {
								limitGB := float64(limitBytes) / bytesPerGigabyte
								usageMap[id] = math.Round(limitGB*100) / 100
							}
						}
					}
				}
			}
		}
	}

	servers := make([]probeSyncServer, 0, len(serverIDs))
	for i, id := range serverIDs {
		name := serverMap[id]
		if name == "" {
			name = fmt.Sprintf("服务器 %d", i+1)
		}

		monthlyGB := usageMap[id]

		servers = append(servers, probeSyncServer{
			ServerID:         id,
			Name:             name,
			TrafficMethod:    "both",
			MonthlyTrafficGB: monthlyGB,
		})
	}

	return servers, nil
}

func (h *probeSyncHandler) fetchNezhaV0Servers(ctx context.Context, address string) ([]probeSyncServer, error) {
	logger.Info("[探针同步-NezhaV0] 开始解析地址: %s", address)

	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		logger.Info("[探针同步-NezhaV0] 地址解析失败: %s, 错误=%v", address, err)
		return nil, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/server"}
	target := base.ResolveReference(endpoint)

	logger.Info("[探针同步-NezhaV0] 请求HTTP接口: %s", target.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		logger.Info("[探针同步-NezhaV0] 创建请求失败: %v", err)
		return nil, err
	}

	resp, err := h.client.Do(req)
	var serverResp struct {
		Result []struct {
			ID   json.Number `json:"id"`
			Name string      `json:"name"`
		} `json:"result"`
	}

	httpSuccess := false
	var httpBodyPreview string
	if err == nil {
		defer resp.Body.Close()

		// 读取响应体以便记录日志
		bodyBytes, _ := io.ReadAll(resp.Body)
		httpBodyPreview = string(bodyBytes)
		if len(httpBodyPreview) > 500 {
			httpBodyPreview = httpBodyPreview[:500] + "...(截断)"
		}

		logger.Info("[探针同步-NezhaV0] HTTP响应: 状态码=%d, 内容长度=%d", resp.StatusCode, len(bodyBytes))

		if resp.StatusCode == http.StatusOK {
			decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
			decoder.UseNumber()

			if decodeErr := decoder.Decode(&serverResp); decodeErr == nil && len(serverResp.Result) > 0 {
				httpSuccess = true
				logger.Info("[探针同步-NezhaV0] HTTP接口成功，获取到 %d 个服务器", len(serverResp.Result))
			} else if decodeErr != nil {
				logger.Info("[探针同步-NezhaV0] HTTP响应JSON解析失败: 错误=%v, 内容=%s", decodeErr, httpBodyPreview)
			} else {
				logger.Info("[探针同步-NezhaV0] HTTP响应中服务器列表为空, 内容=%s", httpBodyPreview)
			}
		} else {
			logger.Info("[探针同步-NezhaV0] HTTP返回非200状态码: %d, 内容=%s", resp.StatusCode, httpBodyPreview)
		}
	} else {
		logger.Info("[探针同步-NezhaV0] HTTP请求失败: %v", err)
	}

	// 如果 HTTP 接口失败或没有数据，尝试使用 WebSocket
	if !httpSuccess {
		logger.Info("[探针同步-NezhaV0] HTTP接口失败，尝试使用 WebSocket 接口...")
		wsServers, wsErr := h.fetchNezhaV0ServersViaWebSocket(ctx, base)
		if wsErr != nil {
			// WebSocket 也失败了，返回综合错误信息
			logger.Info("[探针同步-NezhaV0] WebSocket 接口也失败: %v", wsErr)
			if err != nil {
				return nil, fmt.Errorf("HTTP 接口失败: %w; WebSocket 接口也失败: %v", err, wsErr)
			}
			return nil, fmt.Errorf("HTTP 接口未获取到数据; WebSocket 接口也失败: %v", wsErr)
		}
		logger.Info("[探针同步-NezhaV0] WebSocket 接口成功，获取到 %d 个服务器", len(wsServers))
		return wsServers, nil
	}

	if len(serverResp.Result) == 0 {
		logger.Info("[探针同步-NezhaV0] 服务器列表为空")
		return nil, errors.New("未从面板获取到服务器列表")
	}

	servers := make([]probeSyncServer, 0, len(serverResp.Result))
	for i, item := range serverResp.Result {
		var id string
		if v, err := item.ID.Int64(); err == nil {
			id = strconv.FormatInt(v, 10)
		} else {
			raw := strings.TrimSpace(item.ID.String())
			if raw != "" {
				if strings.ContainsAny(raw, ".eE") {
					if f, err := item.ID.Float64(); err == nil {
						id = strconv.FormatInt(int64(math.Round(f)), 10)
					} else {
						id = raw
					}
				} else {
					id = raw
				}
			} else if f, err := item.ID.Float64(); err == nil {
				id = strconv.FormatInt(int64(math.Round(f)), 10)
			}
		}

		id = strings.TrimSpace(id)
		name := strings.TrimSpace(item.Name)
		if name == "" {
			name = fmt.Sprintf("服务器 %d", i+1)
		}

		servers = append(servers, probeSyncServer{
			ServerID:         id,
			Name:             name,
			TrafficMethod:    "both",
			MonthlyTrafficGB: 0,
		})
	}

	return servers, nil
}

func (h *probeSyncHandler) fetchKomariServers(ctx context.Context, address string) ([]probeSyncServer, error) {
	logger.Info("[探针同步-Komari] 开始解析地址: %s", address)

	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		logger.Info("[探针同步-Komari] 地址解析失败: %s, 错误=%v", address, err)
		return nil, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/nodes"}
	target := base.ResolveReference(endpoint)

	logger.Info("[探针同步-Komari] 请求服务器列表: %s", target.String())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		logger.Info("[探针同步-Komari] 创建请求失败: %v", err)
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		logger.Info("[探针同步-Komari] HTTP请求失败: URL=%s, 错误=%v", target.String(), err)
		return nil, fmt.Errorf("服务器接口返回异常: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体以便记录日志
	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyPreview := string(bodyBytes)
	if len(bodyPreview) > 500 {
		bodyPreview = bodyPreview[:500] + "...(截断)"
	}

	logger.Info("[探针同步-Komari] 收到响应: 状态码=%d, Content-Type=%s, 内容长度=%d",
		resp.StatusCode, resp.Header.Get("Content-Type"), len(bodyBytes))

	if resp.StatusCode != http.StatusOK {
		logger.Info("[探针同步-Komari] 服务器返回错误状态: 状态码=%d, 响应内容=%s", resp.StatusCode, bodyPreview)
		return nil, fmt.Errorf("服务器接口返回异常: 状态码=%d", resp.StatusCode)
	}

	var nodesResp struct {
		Data []struct {
			UUID         string      `json:"uuid"`
			Name         string      `json:"name"`
			TrafficLimit json.Number `json:"traffic_limit"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(bytes.NewReader(bodyBytes))
	decoder.UseNumber()

	if err := decoder.Decode(&nodesResp); err != nil {
		logger.Info("[探针同步-Komari] JSON解析失败: 错误=%v, 响应内容=%s", err, bodyPreview)
		return nil, fmt.Errorf("parse nodes response: %w", err)
	}

	if len(nodesResp.Data) == 0 {
		logger.Info("[探针同步-Komari] 服务器列表为空, 响应内容=%s", bodyPreview)
		return nil, errors.New("未从面板获取到服务器列表")
	}

	logger.Info("[探针同步-Komari] 成功获取 %d 个服务器", len(nodesResp.Data))

	servers := make([]probeSyncServer, 0, len(nodesResp.Data))
	for i, node := range nodesResp.Data {
		id := strings.TrimSpace(node.UUID)
		name := strings.TrimSpace(node.Name)
		if name == "" {
			name = fmt.Sprintf("服务器 %d", i+1)
		}

		// komari traffic_limit / 1024 / 1024 /1024
		var monthlyGB float64
		if limitVal, err := node.TrafficLimit.Float64(); err == nil && limitVal > 0 {
			monthlyGB = limitVal / bytesPerGigabyte
		}

		servers = append(servers, probeSyncServer{
			ServerID:         id,
			Name:             name,
			TrafficMethod:    "both",
			MonthlyTrafficGB: monthlyGB,
		})
	}

	return servers, nil
}

func (h *probeSyncHandler) fetchNezhaV0ServersViaWebSocket(ctx context.Context, base *url.URL) ([]probeSyncServer, error) {
	// 转换 scheme 为 WebSocket
	switch strings.ToLower(base.Scheme) {
	case "", "http":
		base.Scheme = "ws"
	case "https":
		base.Scheme = "wss"
	case "ws", "wss":
		// keep as is
	default:
		base.Scheme = "wss"
	}

	endpoint := &url.URL{Path: "/ws"}
	target := base.ResolveReference(endpoint)

	logger.Info("[探针同步-NezhaV0-WS] 开始连接WebSocket: %s", target.String())

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.DefaultDialer.DialContext(dialCtx, target.String(), nil)
	if err != nil {
		if resp != nil {
			logger.Info("[探针同步-NezhaV0-WS] WebSocket连接失败，HTTP响应状态码: %d", resp.StatusCode)
			resp.Body.Close()
		}
		logger.Info("[探针同步-NezhaV0-WS] WebSocket连接失败: %v", err)
		return nil, fmt.Errorf("无法连接到 WebSocket 接口: %w", err)
	}
	defer conn.Close()
	logger.Info("[探针同步-NezhaV0-WS] WebSocket连接成功")

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, fmt.Errorf("set websocket deadline: %w", err)
	}

	logger.Info("[探针同步-NezhaV0-WS] 等待接收WebSocket消息...")
	_, message, err := conn.ReadMessage()
	if err != nil {
		logger.Info("[探针同步-NezhaV0-WS] 读取WebSocket消息失败: %v", err)
		return nil, fmt.Errorf("未在期望时间内收到服务器数据: %w", err)
	}
	logger.Info("[探针同步-NezhaV0-WS] 收到WebSocket消息，长度: %d 字节", len(message))

	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		logger.Info("[探针同步-NezhaV0-WS] WebSocket消息为空")
		return nil, errors.New("empty probe websocket payload")
	}

	// 记录消息内容预览
	msgPreview := string(message)
	if len(msgPreview) > 500 {
		msgPreview = msgPreview[:500] + "..."
	}
	logger.Info("[探针同步-NezhaV0-WS] 消息内容预览: %s", msgPreview)

	type nezhaServer struct {
		ID   json.Number `json:"id"`
		Name string      `json:"name"`
	}

	type nezhaSnapshot struct {
		Servers []nezhaServer `json:"servers"`
	}

	decoder := json.NewDecoder(bytes.NewReader(message))
	decoder.UseNumber()

	var snapshot nezhaSnapshot

	if message[0] == '[' {
		logger.Info("[探针同步-NezhaV0-WS] 检测到数组格式，解析多帧数据")
		var frames []nezhaSnapshot
		if err := decoder.Decode(&frames); err != nil {
			logger.Info("[探针同步-NezhaV0-WS] 解析数组格式数据失败: %v", err)
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
		logger.Info("[探针同步-NezhaV0-WS] 解析到 %d 个帧", len(frames))
		if len(frames) == 0 {
			logger.Info("[探针同步-NezhaV0-WS] 数组为空，没有数据帧")
			return nil, errors.New("探针未返回任何服务器数据")
		}
		snapshot = frames[len(frames)-1]
	} else {
		logger.Info("[探针同步-NezhaV0-WS] 检测到对象格式，解析单个快照")
		if err := decoder.Decode(&snapshot); err != nil {
			logger.Info("[探针同步-NezhaV0-WS] 解析对象格式数据失败: %v", err)
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
	}

	logger.Info("[探针同步-NezhaV0-WS] 快照中包含 %d 个服务器", len(snapshot.Servers))
	if len(snapshot.Servers) == 0 {
		logger.Info("[探针同步-NezhaV0-WS] 服务器列表为空")
		return nil, errors.New("探针未返回任何服务器数据")
	}

	servers := make([]probeSyncServer, 0, len(snapshot.Servers))
	for i, srv := range snapshot.Servers {
		var id string
		if v, err := srv.ID.Int64(); err == nil {
			id = strconv.FormatInt(v, 10)
		} else {
			raw := strings.TrimSpace(srv.ID.String())
			if raw != "" {
				if strings.ContainsAny(raw, ".eE") {
					if f, err := srv.ID.Float64(); err == nil {
						id = strconv.FormatInt(int64(math.Round(f)), 10)
					} else {
						id = raw
					}
				} else {
					id = raw
				}
			} else if f, err := srv.ID.Float64(); err == nil {
				id = strconv.FormatInt(int64(math.Round(f)), 10)
			}
		}

		id = strings.TrimSpace(id)
		name := strings.TrimSpace(srv.Name)
		if name == "" {
			name = fmt.Sprintf("服务器 %d", i+1)
		}

		servers = append(servers, probeSyncServer{
			ServerID:         id,
			Name:             name,
			TrafficMethod:    "both",
			MonthlyTrafficGB: 0,
		})
	}

	logger.Info("[探针同步-NezhaV0-WS] 成功解析 %d 个服务器", len(servers))
	return servers, nil
}
