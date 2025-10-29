package handler

import (
	"errors"
	"net"
	"net/http"
	"strings"
)

type dnsHandler struct{}

// NewDNSHandler returns a handler that provides DNS resolution services.
func NewDNSHandler() http.Handler {
	return &dnsHandler{}
}

func (h *dnsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/dns")
	path = strings.Trim(path, "/")

	switch {
	case path == "resolve" && r.Method == http.MethodGet:
		h.handleResolve(w, r)
	default:
		allowed := []string{http.MethodGet}
		methodNotAllowed(w, allowed...)
	}
}

func (h *dnsHandler) handleResolve(w http.ResponseWriter, r *http.Request) {
	hostname := r.URL.Query().Get("hostname")
	if hostname == "" {
		writeBadRequest(w, "hostname参数是必填项")
		return
	}

	// 去除端口号（如果有）
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}

	// 检查是否已经是IP地址
	if ip := net.ParseIP(hostname); ip != nil {
		respondJSON(w, http.StatusOK, map[string]any{
			"ips": []string{hostname},
		})
		return
	}

	// 解析域名
	ips, err := net.LookupIP(hostname)
	if err != nil {
		writeError(w, http.StatusBadRequest, errors.New("DNS解析失败: "+err.Error()))
		return
	}

	if len(ips) == 0 {
		respondJSON(w, http.StatusOK, map[string]any{
			"ips": []string{},
		})
		return
	}

	// 转换为字符串数组，优先返回IPv4，然后是IPv6
	var ipv4List []string
	var ipv6List []string

	for _, ip := range ips {
		if ipv4 := ip.To4(); ipv4 != nil {
			ipv4List = append(ipv4List, ip.String())
		} else {
			ipv6List = append(ipv6List, ip.String())
		}
	}

	// 合并列表，IPv4在前
	result := append(ipv4List, ipv6List...)

	respondJSON(w, http.StatusOK, map[string]any{
		"ips": result,
	})
}
