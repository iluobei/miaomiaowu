package handler

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"miaomiaowu/internal/logger"
)

// TCPingRequest represents a TCP ping request
type TCPingRequest struct {
	Host    string `json:"host"`
	Port    int    `json:"port"`
	Timeout int    `json:"timeout"` // timeout in milliseconds, default 5000
}

// TCPingResponse represents a TCP ping response
type TCPingResponse struct {
	Success bool    `json:"success"`
	Latency float64 `json:"latency"` // latency in milliseconds
	Error   string  `json:"error,omitempty"`
}

// NewTCPingHandler returns a handler that performs TCP ping
func NewTCPingHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "only POST is supported")
			return
		}

		var req TCPingRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if req.Host == "" {
			writeJSONError(w, http.StatusBadRequest, "host is required")
			return
		}

		if req.Port <= 0 || req.Port > 65535 {
			writeJSONError(w, http.StatusBadRequest, "invalid port")
			return
		}

		timeout := req.Timeout
		if timeout <= 0 {
			timeout = 5000
		}
		if timeout > 30000 {
			timeout = 30000
		}

		address := net.JoinHostPort(req.Host, fmt.Sprintf("%d", req.Port))
		timeoutDuration := time.Duration(timeout) * time.Millisecond

		logger.Debug("[TCPing] 开始测试", "address", address, "timeout", timeout)

		start := time.Now()
		conn, err := net.DialTimeout("tcp", address, timeoutDuration)
		latency := float64(time.Since(start).Microseconds()) / 1000.0

		resp := TCPingResponse{}

		if err != nil {
			logger.Debug("[TCPing] 连接失败", "address", address, "error", err)
			resp.Success = false
			resp.Error = err.Error()
		} else {
			conn.Close()
			logger.Debug("[TCPing] 连接成功", "address", address, "latency", latency)
			resp.Success = true
			resp.Latency = latency
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(resp)
	})
}

// NewTCPingBatchHandler returns a handler that performs batch TCP ping
func NewTCPingBatchHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "only POST is supported")
			return
		}

		var requests []TCPingRequest
		if err := json.NewDecoder(r.Body).Decode(&requests); err != nil {
			writeJSONError(w, http.StatusBadRequest, "invalid request body")
			return
		}

		if len(requests) == 0 {
			writeJSONError(w, http.StatusBadRequest, "empty request list")
			return
		}

		if len(requests) > 50 {
			writeJSONError(w, http.StatusBadRequest, "too many requests (max 50)")
			return
		}

		results := make([]TCPingResponse, len(requests))
		done := make(chan struct{}, len(requests))

		for i, req := range requests {
			go func(idx int, r TCPingRequest) {
				defer func() { done <- struct{}{} }()

				if r.Host == "" || r.Port <= 0 || r.Port > 65535 {
					results[idx] = TCPingResponse{Success: false, Error: "invalid host or port"}
					return
				}

				timeout := r.Timeout
				if timeout <= 0 {
					timeout = 5000
				}
				if timeout > 30000 {
					timeout = 30000
				}

				address := net.JoinHostPort(r.Host, fmt.Sprintf("%d", r.Port))
				timeoutDuration := time.Duration(timeout) * time.Millisecond

				start := time.Now()
				conn, err := net.DialTimeout("tcp", address, timeoutDuration)
				latency := float64(time.Since(start).Microseconds()) / 1000.0

				if err != nil {
					results[idx] = TCPingResponse{Success: false, Error: err.Error()}
				} else {
					conn.Close()
					results[idx] = TCPingResponse{Success: true, Latency: latency}
				}
			}(i, req)
		}

		for range requests {
			<-done
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(results)
	})
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}
