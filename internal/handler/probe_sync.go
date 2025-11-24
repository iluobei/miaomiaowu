package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
		writeBadRequest(w, "不支持的探针类型")
		return
	}

	if err != nil {
		writeError(w, http.StatusBadGateway, err)
		return
	}

	respondJSON(w, http.StatusOK, probeSyncResponse{Servers: servers})
}

func (h *probeSyncHandler) fetchNezhaServers(ctx context.Context, address string) ([]probeSyncServer, error) {
	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
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

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.DefaultDialer.DialContext(dialCtx, target.String(), nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf("无法连接到 WebSocket 接口: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, fmt.Errorf("set websocket deadline: %w", err)
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("未在期望时间内收到服务器数据: %w", err)
	}

	message = bytes.TrimSpace(message)
	if len(message) == 0 {
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

	if message[0] == '[' {
		var frames []nezhaSnapshot
		if err := decoder.Decode(&frames); err != nil {
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
		if len(frames) == 0 {
			return nil, errors.New("探针未返回任何服务器数据")
		}
		snapshot = frames[len(frames)-1]
	} else {
		if err := decoder.Decode(&snapshot); err != nil {
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
	}

	if len(snapshot.Servers) == 0 {
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

	return servers, nil
}

func (h *probeSyncHandler) fetchDstatusServers(ctx context.Context, address string) ([]probeSyncServer, error) {
	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return nil, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/servers"}
	target := base.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("服务器接口返回异常: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("服务器接口返回异常")
	}

	var serversResp struct {
		Data []struct {
			ID   interface{} `json:"id"`
			Name string      `json:"name"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	if err := decoder.Decode(&serversResp); err != nil {
		return nil, fmt.Errorf("parse servers response: %w", err)
	}

	if len(serversResp.Data) == 0 {
		return nil, errors.New("未从面板获取到服务器列表")
	}

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
	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return nil, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/server"}
	target := base.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
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
	if err == nil {
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			decoder := json.NewDecoder(resp.Body)
			decoder.UseNumber()

			if err := decoder.Decode(&serverResp); err == nil && len(serverResp.Result) > 0 {
				httpSuccess = true
			}
		}
	}

	// 如果 HTTP 接口失败或没有数据，尝试使用 WebSocket
	if !httpSuccess {
		wsServers, wsErr := h.fetchNezhaV0ServersViaWebSocket(ctx, base)
		if wsErr != nil {
			// WebSocket 也失败了，返回综合错误信息
			if err != nil {
				return nil, fmt.Errorf("HTTP 接口失败: %w; WebSocket 接口也失败: %v", err, wsErr)
			}
			return nil, fmt.Errorf("HTTP 接口未获取到数据; WebSocket 接口也失败: %v", wsErr)
		}
		return wsServers, nil
	}

	if len(serverResp.Result) == 0 {
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
	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return nil, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/nodes"}
	target := base.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return nil, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("服务器接口返回异常: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("服务器接口返回异常")
	}

	var nodesResp struct {
		Data []struct {
			UUID         string      `json:"uuid"`
			Name         string      `json:"name"`
			TrafficLimit json.Number `json:"traffic_limit"`
		} `json:"data"`
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	if err := decoder.Decode(&nodesResp); err != nil {
		return nil, fmt.Errorf("parse nodes response: %w", err)
	}

	if len(nodesResp.Data) == 0 {
		return nil, errors.New("未从面板获取到服务器列表")
	}

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

	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, resp, err := websocket.DefaultDialer.DialContext(dialCtx, target.String(), nil)
	if err != nil {
		if resp != nil {
			resp.Body.Close()
		}
		return nil, fmt.Errorf("无法连接到 WebSocket 接口: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return nil, fmt.Errorf("set websocket deadline: %w", err)
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("未在期望时间内收到服务器数据: %w", err)
	}

	message = bytes.TrimSpace(message)
	if len(message) == 0 {
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

	if message[0] == '[' {
		var frames []nezhaSnapshot
		if err := decoder.Decode(&frames); err != nil {
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
		if len(frames) == 0 {
			return nil, errors.New("探针未返回任何服务器数据")
		}
		snapshot = frames[len(frames)-1]
	} else {
		if err := decoder.Decode(&snapshot); err != nil {
			return nil, fmt.Errorf("解析探针返回数据失败: %w", err)
		}
	}

	if len(snapshot.Servers) == 0 {
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

	return servers, nil
}
