package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gorilla/websocket"

	"miaomiaowu/internal/auth"
	"miaomiaowu/internal/storage"
)

const bytesPerGigabyte = 1073741824.0

type TrafficSummaryHandler struct {
	client *http.Client
	repo   *storage.TrafficRepository
}

type trafficSummaryResponse struct {
	Metrics trafficSummaryMetrics `json:"metrics"`
	History []trafficDailyUsage   `json:"history"`
}

type trafficSummaryMetrics struct {
	TotalLimitGB     float64 `json:"total_limit_gb"`
	TotalUsedGB      float64 `json:"total_used_gb"`
	TotalRemainingGB float64 `json:"total_remaining_gb"`
	UsagePercentage  float64 `json:"usage_percentage"`
}

type trafficDailyUsage struct {
	Date   string  `json:"date"`
	UsedGB float64 `json:"used_gb"`
}

type batchTrafficResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    map[string]struct {
		Monthly struct {
			Limit     json.Number `json:"limit"`
			Remaining json.Number `json:"remaining"`
			Used      json.Number `json:"used"`
		} `json:"monthly"`
	} `json:"data"`
}

func NewTrafficSummaryHandler(repo *storage.TrafficRepository) *TrafficSummaryHandler {
	if repo == nil {
		panic("traffic summary handler requires repository")
	}

	client := &http.Client{Timeout: 15 * time.Second}
	return newTrafficSummaryHandler(client, repo)
}

func newTrafficSummaryHandler(client *http.Client, repo *storage.TrafficRepository) *TrafficSummaryHandler {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}

	return &TrafficSummaryHandler{client: client, repo: repo}
}

func (h *TrafficSummaryHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, errors.New("only GET is supported"))
		return
	}

	ctx := r.Context()
	username := auth.UsernameFromContext(ctx)

	totalLimit, totalRemaining, totalUsed, err := h.fetchTotals(ctx, username)
	if err != nil {
		// Return empty metrics if probe is not configured yet
		if errors.Is(err, storage.ErrProbeConfigNotFound) {
			response := trafficSummaryResponse{
				Metrics: trafficSummaryMetrics{
					TotalLimitGB:     0,
					TotalUsedGB:      0,
					TotalRemainingGB: 0,
					UsagePercentage:  0,
				},
				History: []trafficDailyUsage{},
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(response)
			return
		}

		writeError(w, http.StatusBadGateway, err)
		return
	}

	// Add external subscription traffic if sync_traffic is enabled
	if username != "" {
		externalLimit, externalUsed := h.fetchExternalSubscriptionTraffic(ctx, username)
		totalLimit += externalLimit
		totalUsed += externalUsed
		// Recalculate remaining
		totalRemaining = totalLimit - totalUsed
	}

	if err := h.recordSnapshot(ctx, totalLimit, totalUsed, totalRemaining); err != nil {
		log.Printf("record traffic snapshot failed: %v", err)
	}

	history, err := h.loadHistory(ctx, 30)
	if err != nil {
		log.Printf("load traffic history failed: %v", err)
	}

	metrics := trafficSummaryMetrics{
		TotalLimitGB:     roundUpTwoDecimals(bytesToGigabytes(totalLimit)),
		TotalUsedGB:      roundUpTwoDecimals(bytesToGigabytes(totalUsed)),
		TotalRemainingGB: roundUpTwoDecimals(bytesToGigabytes(totalRemaining)),
		UsagePercentage:  roundUpTwoDecimals(usagePercentage(totalUsed, totalLimit)),
	}

	response := trafficSummaryResponse{
		Metrics: metrics,
		History: history,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// RecordDailyUsage fetches the latest traffic summary and persists the snapshot.
func (h *TrafficSummaryHandler) RecordDailyUsage(ctx context.Context) error {
	totalLimit, totalRemaining, totalUsed, err := h.fetchTotals(ctx, "")
	if err != nil {
		log.Printf("[Traffic Record] Failed to fetch traffic data: %v", err)
		return err
	}

	// Log fetched data
	limitGB := roundUpTwoDecimals(bytesToGigabytes(totalLimit))
	usedGB := roundUpTwoDecimals(bytesToGigabytes(totalUsed))
	remainingGB := roundUpTwoDecimals(bytesToGigabytes(totalRemaining))
	usagePercent := roundUpTwoDecimals(usagePercentage(totalUsed, totalLimit))

	log.Printf("[Traffic Record] Fetched from probe - Limit: %.2f GB, Used: %.2f GB, Remaining: %.2f GB, Usage: %.2f%%",
		limitGB, usedGB, remainingGB, usagePercent)

	if err := h.recordSnapshot(ctx, totalLimit, totalUsed, totalRemaining); err != nil {
		log.Printf("[Traffic Record] Failed to save snapshot to database: %v", err)
		return err
	}

	log.Printf("[Traffic Record] Successfully saved snapshot to database")
	return nil
}

func (h *TrafficSummaryHandler) fetchTotals(ctx context.Context, username string) (int64, int64, int64, error) {
	if h.repo == nil {
		return 0, 0, 0, errors.New("traffic repository not configured")
	}

	cfg, err := h.repo.GetProbeConfig(ctx)
	if err != nil {
		return 0, 0, 0, err
	}

	if len(cfg.Servers) == 0 {
		return 0, 0, 0, errors.New("no probe servers configured")
	}

	// Get user settings to check if probe binding is enabled
	if username != "" {
		userSettings, err := h.repo.GetUserSettings(ctx, username)
		if err == nil && userSettings.EnableProbeBinding {
			// Get all nodes for this user
			nodes, err := h.repo.ListNodes(ctx, username)
			if err == nil {
				// Collect unique probe server names that are bound to nodes
				boundProbeServers := make(map[string]bool)
				for _, node := range nodes {
					if node.ProbeServer != "" {
						boundProbeServers[node.ProbeServer] = true
					}
				}

				if len(boundProbeServers) > 0 {
					// Filter servers to only include bound ones
					filteredServers := make([]storage.ProbeServer, 0)
					for _, srv := range cfg.Servers {
						name := strings.TrimSpace(srv.Name)
						if boundProbeServers[name] {
							filteredServers = append(filteredServers, srv)
						}
					}
					cfg.Servers = filteredServers
					log.Printf("[Traffic Fetch] Probe binding enabled, filtered to %d bound servers", len(cfg.Servers))
				} else {
					log.Printf("[Traffic Fetch] Probe binding enabled but no nodes have bound servers, returning zero traffic")
					return 0, 0, 0, nil
				}
			}
		}
	}

	serverIDs := make([]string, 0, len(cfg.Servers))
	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}
		serverIDs = append(serverIDs, id)
	}

	if len(serverIDs) == 0 {
		return 0, 0, 0, errors.New("no server ids configured")
	}

	log.Printf("[Traffic Fetch] Probe type: %s, Address: %s, Server count: %d, Server IDs: %v",
		cfg.ProbeType, cfg.Address, len(cfg.Servers), serverIDs)

	switch cfg.ProbeType {
	case storage.ProbeTypeNezha:
		return h.fetchNezhaTotals(ctx, cfg)
	case storage.ProbeTypeNezhaV0:
		return h.fetchNezhaV0Totals(ctx, cfg)
	case storage.ProbeTypeDstatus:
		return h.fetchBatchSummary(ctx, cfg.Address, serverIDs)
	case storage.ProbeTypeKomari:
		return h.fetchKomariTotals(ctx, cfg)
	default:
		return 0, 0, 0, fmt.Errorf("unsupported probe type: %s", cfg.ProbeType)
	}
}

func (h *TrafficSummaryHandler) fetchNezhaTotals(ctx context.Context, cfg storage.ProbeConfig) (int64, int64, int64, error) {
	baseAddress := strings.TrimSpace(cfg.Address)
	if baseAddress == "" {
		return 0, 0, 0, errors.New("invalid probe address")
	}

	base, err := url.Parse(baseAddress)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
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
		return 0, 0, 0, fmt.Errorf("connect probe websocket: %w", err)
	}
	defer conn.Close()

	if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return 0, 0, 0, fmt.Errorf("set websocket deadline: %w", err)
	}

	_, message, err := conn.ReadMessage()
	if err != nil {
		return 0, 0, 0, fmt.Errorf("read probe websocket: %w", err)
	}
	message = bytes.TrimSpace(message)
	if len(message) == 0 {
		return 0, 0, 0, errors.New("empty probe websocket payload")
	}

	type nezhaServer struct {
		ID    json.Number `json:"id"`
		State struct {
			NetInTransfer  json.Number `json:"net_in_transfer"`
			NetOutTransfer json.Number `json:"net_out_transfer"`
		} `json:"state"`
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
			return 0, 0, 0, fmt.Errorf("parse probe websocket payload: %w", err)
		}
		if len(frames) == 0 {
			return 0, 0, 0, errors.New("probe websocket payload missing frames")
		}
		snapshot = frames[len(frames)-1]
	} else {
		if err := decoder.Decode(&snapshot); err != nil {
			return 0, 0, 0, fmt.Errorf("parse probe websocket payload: %w", err)
		}
	}

	observed := make(map[string]struct {
		NetIn  int64
		NetOut int64
	})
	for _, entry := range snapshot.Servers {
		var id string
		if v, err := entry.ID.Int64(); err == nil {
			id = strconv.FormatInt(v, 10)
		} else {
			raw := strings.TrimSpace(entry.ID.String())
			if raw != "" {
				if strings.ContainsAny(raw, ".eE") {
					if f, err := entry.ID.Float64(); err == nil {
						id = strconv.FormatInt(int64(math.Round(f)), 10)
					} else {
						id = raw
					}
				} else {
					id = raw
				}
			} else if f, err := entry.ID.Float64(); err == nil {
				id = strconv.FormatInt(int64(math.Round(f)), 10)
			}
		}

		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		netIn := jsonNumberToInt64(entry.State.NetInTransfer)
		netOut := jsonNumberToInt64(entry.State.NetOutTransfer)
		observed[id] = struct {
			NetIn  int64
			NetOut int64
		}{NetIn: netIn, NetOut: netOut}
	}

	var totalLimit int64
	var totalUsed int64

	log.Printf("[Nezha] Processing %d servers from WebSocket data", len(cfg.Servers))

	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}

		totalLimit += srv.MonthlyTrafficBytes

		wsEntry, ok := observed[id]
		if !ok {
			log.Printf("[Nezha] Server ID %s not found in probe data", id)
			continue
		}

		var used int64
		switch strings.ToLower(strings.TrimSpace(srv.TrafficMethod)) {
		case storage.TrafficMethodUp:
			used = wsEntry.NetOut
		case storage.TrafficMethodDown:
			used = wsEntry.NetIn
		default:
			used = wsEntry.NetIn + wsEntry.NetOut
		}

		if used < 0 {
			used = 0
		}
		if srv.MonthlyTrafficBytes > 0 && used > srv.MonthlyTrafficBytes {
			used = srv.MonthlyTrafficBytes
		}

		log.Printf("[Nezha] Server ID %s - NetIn: %.2f GB, NetOut: %.2f GB, Method: %s, Used: %.2f GB, Limit: %.2f GB",
			id, bytesToGigabytes(wsEntry.NetIn), bytesToGigabytes(wsEntry.NetOut),
			srv.TrafficMethod, bytesToGigabytes(used), bytesToGigabytes(srv.MonthlyTrafficBytes))

		totalUsed += used
	}

	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	log.Printf("[Nezha] Total - Limit: %.2f GB, Used: %.2f GB, Remaining: %.2f GB",
		bytesToGigabytes(totalLimit), bytesToGigabytes(totalUsed), bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func (h *TrafficSummaryHandler) fetchNezhaV0Totals(ctx context.Context, cfg storage.ProbeConfig) (int64, int64, int64, error) {
	baseAddress := strings.TrimSpace(cfg.Address)
	if baseAddress == "" {
		return 0, 0, 0, errors.New("invalid probe address")
	}

	base, err := url.Parse(baseAddress)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/server"}
	target := base.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
	if err != nil {
		return 0, 0, 0, err
	}

	resp, err := h.client.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("nezha v0 request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, fmt.Errorf("nezha v0 request failed with status %s", resp.Status)
	}

	type nezhaV0Server struct {
		ID     json.Number `json:"id"`
		Status struct {
			NetInTransfer  json.Number `json:"NetInTransfer"`
			NetOutTransfer json.Number `json:"NetOutTransfer"`
		} `json:"status"`
	}

	type nezhaV0Response struct {
		Result []nezhaV0Server `json:"result"`
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	var payload nezhaV0Response
	if err := decoder.Decode(&payload); err != nil {
		return 0, 0, 0, fmt.Errorf("parse nezha v0 response: %w", err)
	}

	observed := make(map[string]struct {
		NetIn  int64
		NetOut int64
	})
	for _, entry := range payload.Result {
		var id string
		if v, err := entry.ID.Int64(); err == nil {
			id = strconv.FormatInt(v, 10)
		} else {
			raw := strings.TrimSpace(entry.ID.String())
			if raw != "" {
				if strings.ContainsAny(raw, ".eE") {
					if f, err := entry.ID.Float64(); err == nil {
						id = strconv.FormatInt(int64(math.Round(f)), 10)
					} else {
						id = raw
					}
				} else {
					id = raw
				}
			} else if f, err := entry.ID.Float64(); err == nil {
				id = strconv.FormatInt(int64(math.Round(f)), 10)
			}
		}

		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}

		netIn := jsonNumberToInt64(entry.Status.NetInTransfer)
		netOut := jsonNumberToInt64(entry.Status.NetOutTransfer)
		observed[id] = struct {
			NetIn  int64
			NetOut int64
		}{NetIn: netIn, NetOut: netOut}
	}

	var totalLimit int64
	var totalUsed int64

	log.Printf("[Nezha V0] Processing %d servers from HTTP API data", len(cfg.Servers))

	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}

		totalLimit += srv.MonthlyTrafficBytes

		entry, ok := observed[id]
		if !ok {
			log.Printf("[Nezha V0] Server ID %s not found in probe data", id)
			continue
		}

		var used int64
		switch strings.ToLower(strings.TrimSpace(srv.TrafficMethod)) {
		case storage.TrafficMethodUp:
			used = entry.NetOut
		case storage.TrafficMethodDown:
			used = entry.NetIn
		default:
			used = entry.NetIn + entry.NetOut
		}

		if used < 0 {
			used = 0
		}
		if srv.MonthlyTrafficBytes > 0 && used > srv.MonthlyTrafficBytes {
			used = srv.MonthlyTrafficBytes
		}

		log.Printf("[Nezha V0] Server ID %s - NetIn: %.2f GB, NetOut: %.2f GB, Method: %s, Used: %.2f GB, Limit: %.2f GB",
			id, bytesToGigabytes(entry.NetIn), bytesToGigabytes(entry.NetOut),
			srv.TrafficMethod, bytesToGigabytes(used), bytesToGigabytes(srv.MonthlyTrafficBytes))

		totalUsed += used
	}

	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	log.Printf("[Nezha V0] Total - Limit: %.2f GB, Used: %.2f GB, Remaining: %.2f GB",
		bytesToGigabytes(totalLimit), bytesToGigabytes(totalUsed), bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func (h *TrafficSummaryHandler) fetchBatchSummary(ctx context.Context, address string, serverIDs []string) (int64, int64, int64, error) {
	base, err := url.Parse(strings.TrimSpace(address))
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
	}

	return h.fetchBatchTraffic(ctx, base, serverIDs)
}

func (h *TrafficSummaryHandler) fetchKomariTotals(ctx context.Context, cfg storage.ProbeConfig) (int64, int64, int64, error) {
	baseAddress := strings.TrimSpace(cfg.Address)
	if baseAddress == "" {
		return 0, 0, 0, errors.New("invalid probe address")
	}

	base, err := url.Parse(baseAddress)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("invalid probe address: %w", err)
	}

	endpoint := &url.URL{Path: "/api/rpc2"}
	target := base.ResolveReference(endpoint)

	// Prepare JSON-RPC request
	rpcRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "common:getNodesLatestStatus",
		"id":      3,
	}

	requestBody, err := json.Marshal(rpcRequest)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("marshal komari request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(requestBody))
	if err != nil {
		return 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := h.client.Do(req)
	if err != nil {
		return 0, 0, 0, fmt.Errorf("komari request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, fmt.Errorf("komari request failed with status %s", resp.Status)
	}

	type komariResponse struct {
		Result map[string]struct {
			NetTotalUp   json.Number `json:"net_total_up"`
			NetTotalDown json.Number `json:"net_total_down"`
		} `json:"result"`
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	var payload komariResponse
	if err := decoder.Decode(&payload); err != nil {
		return 0, 0, 0, fmt.Errorf("parse komari response: %w", err)
	}

	observed := make(map[string]struct {
		Up   int64
		Down int64
	})
	for id, info := range payload.Result {
		cleanID := strings.TrimSpace(id)
		if cleanID == "" {
			continue
		}

		up := jsonNumberToInt64(info.NetTotalUp)
		if up < 0 {
			up = 0
		}
		down := jsonNumberToInt64(info.NetTotalDown)
		if down < 0 {
			down = 0
		}

		observed[cleanID] = struct {
			Up   int64
			Down int64
		}{Up: up, Down: down}
	}

	var totalLimit int64
	var totalUsed int64

	log.Printf("[Komari] Processing %d servers from JSON-RPC data", len(cfg.Servers))

	for _, srv := range cfg.Servers {
		id := strings.TrimSpace(srv.ServerID)
		if id == "" {
			continue
		}

		totalLimit += srv.MonthlyTrafficBytes

		usage, ok := observed[id]
		if !ok {
			log.Printf("[Komari] Server ID %s not found in probe data", id)
			continue
		}

		var used int64
		switch strings.ToLower(strings.TrimSpace(srv.TrafficMethod)) {
		case storage.TrafficMethodUp:
			used = usage.Up
		case storage.TrafficMethodDown:
			used = usage.Down
		default:
			used = usage.Up + usage.Down
		}

		if used < 0 {
			used = 0
		}
		if srv.MonthlyTrafficBytes > 0 && used > srv.MonthlyTrafficBytes {
			used = srv.MonthlyTrafficBytes
		}

		log.Printf("[Komari] Server ID %s - Up: %.2f GB, Down: %.2f GB, Method: %s, Used: %.2f GB, Limit: %.2f GB",
			id, bytesToGigabytes(usage.Up), bytesToGigabytes(usage.Down),
			srv.TrafficMethod, bytesToGigabytes(used), bytesToGigabytes(srv.MonthlyTrafficBytes))

		totalUsed += used
	}

	totalRemaining := totalLimit - totalUsed
	if totalRemaining < 0 {
		totalRemaining = 0
	}

	log.Printf("[Komari] Total - Limit: %.2f GB, Used: %.2f GB, Remaining: %.2f GB",
		bytesToGigabytes(totalLimit), bytesToGigabytes(totalUsed), bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func (h *TrafficSummaryHandler) fetchBatchTraffic(ctx context.Context, base *url.URL, serverIDs []string) (int64, int64, int64, error) {
	payload, err := json.Marshal(map[string][]string{"serverIds": serverIDs})
	if err != nil {
		return 0, 0, 0, err
	}

	endpoint := &url.URL{Path: "/stats/batch-traffic"}
	target := base.ResolveReference(endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), bytes.NewReader(payload))
	if err != nil {
		return 0, 0, 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "miaomiaowu/0.1")

	resp, err := h.client.Do(req)
	if err != nil {
		return 0, 0, 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, 0, 0, errors.New("batch traffic request failed with status " + resp.Status)
	}

	decoder := json.NewDecoder(resp.Body)
	decoder.UseNumber()

	var payloadResp batchTrafficResponse
	if err := decoder.Decode(&payloadResp); err != nil {
		return 0, 0, 0, err
	}

	if !payloadResp.Success {
		if payloadResp.Message != "" {
			return 0, 0, 0, errors.New(payloadResp.Message)
		}
		return 0, 0, 0, errors.New("batch traffic request unsuccessful")
	}

	var totalLimit int64
	var totalRemaining int64
	var totalUsed int64

	log.Printf("[Dstatus] Processing %d servers from batch traffic API", len(payloadResp.Data))

	for serverID, entry := range payloadResp.Data {
		limit := jsonNumberToInt64(entry.Monthly.Limit)
		used := jsonNumberToInt64(entry.Monthly.Used)
		remaining := jsonNumberToInt64(entry.Monthly.Remaining)

		log.Printf("[Dstatus] Server ID %s - Limit: %.2f GB, Used: %.2f GB, Remaining: %.2f GB",
			serverID, bytesToGigabytes(limit), bytesToGigabytes(used), bytesToGigabytes(remaining))

		totalLimit += limit
		totalRemaining += remaining
		totalUsed += used
	}

	log.Printf("[Dstatus] Total - Limit: %.2f GB, Used: %.2f GB, Remaining: %.2f GB",
		bytesToGigabytes(totalLimit), bytesToGigabytes(totalUsed), bytesToGigabytes(totalRemaining))

	return totalLimit, totalRemaining, totalUsed, nil
}

func jsonNumberToInt64(n json.Number) int64 {
	if n == "" {
		return 0
	}
	if v, err := n.Int64(); err == nil {
		return v
	}
	if f, err := n.Float64(); err == nil {
		if f < 0 {
			return int64(f - 0.5)
		}
		return int64(f + 0.5)
	}
	return 0
}

func roundUpTwoDecimals(value float64) float64 {
	return math.Ceil(value*100) / 100
}

func bytesToGigabytes(total int64) float64 {
	if total <= 0 {
		return 0
	}

	return float64(total) / bytesPerGigabyte
}

func usagePercentage(used, limit int64) float64 {
	if limit <= 0 {
		return 0
	}

	return (float64(used) / float64(limit)) * 100
}

func (h *TrafficSummaryHandler) recordSnapshot(ctx context.Context, totalLimit, totalUsed, totalRemaining int64) error {
	if h.repo == nil {
		return nil
	}

	return h.repo.RecordDaily(ctx, time.Now(), totalLimit, totalUsed, totalRemaining)
}

func (h *TrafficSummaryHandler) loadHistory(ctx context.Context, days int) ([]trafficDailyUsage, error) {
	if h.repo == nil {
		return nil, nil
	}

	records, err := h.repo.ListRecent(ctx, days)
	if err != nil {
		return nil, err
	}

	if len(records) == 0 {
		return nil, nil
	}

	sort.SliceStable(records, func(i, j int) bool {
		return records[i].Date.Before(records[j].Date)
	})

	usages := make([]trafficDailyUsage, 0, len(records))
	var prevUsed int64
	var hasPrev bool

	for _, record := range records {
		delta := record.TotalUsed
		if hasPrev {
			delta = record.TotalUsed - prevUsed
			if delta < 0 {
				delta = 0
			}
		}

		prevUsed = record.TotalUsed
		hasPrev = true

		usages = append(usages, trafficDailyUsage{
			Date:   record.Date.Format("2006-01-02"),
			UsedGB: roundUpTwoDecimals(bytesToGigabytes(delta)),
		})
	}

	return usages, nil
}

// fetchExternalSubscriptionTraffic fetches traffic from external subscriptions
// Returns totalLimit and totalUsed from non-expired subscriptions (or long-term subscriptions without expire date)
func (h *TrafficSummaryHandler) fetchExternalSubscriptionTraffic(ctx context.Context, username string) (int64, int64) {
	// Check if sync_traffic is enabled
	settings, err := h.repo.GetUserSettings(ctx, username)
	if err != nil || !settings.SyncTraffic {
		return 0, 0
	}

	// Get all external subscriptions
	subs, err := h.repo.ListExternalSubscriptions(ctx, username)
	if err != nil {
		log.Printf("[Traffic] Failed to fetch external subscriptions: %v", err)
		return 0, 0
	}

	var totalLimit int64
	var totalUsed int64
	now := time.Now()

	for _, sub := range subs {
		// Skip if subscription is expired
		// If Expire is nil, it's a long-term subscription and should not be skipped
		if sub.Expire != nil && sub.Expire.Before(now) {
			log.Printf("[Traffic] Skipping expired subscription: %s (expired at %s)", sub.Name, sub.Expire.Format("2006-01-02 15:04:05"))
			continue
		}

		// Add traffic from this subscription
		totalLimit += sub.Total
		totalUsed += sub.Upload + sub.Download

		if sub.Expire == nil {
			log.Printf("[Traffic] Adding long-term subscription traffic: %s (limit=%d, used=%d)", sub.Name, sub.Total, sub.Upload+sub.Download)
		} else {
			log.Printf("[Traffic] Adding subscription traffic: %s (limit=%d, used=%d, expires=%s)",
				sub.Name, sub.Total, sub.Upload+sub.Download, sub.Expire.Format("2006-01-02 15:04:05"))
		}
	}

	log.Printf("[Traffic] Total external subscription traffic: limit=%d, used=%d", totalLimit, totalUsed)
	return totalLimit, totalUsed
}

func writeError(w http.ResponseWriter, status int, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": err.Error(),
	})
}
