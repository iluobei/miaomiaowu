package substore

// Proxy represents a generic proxy configuration
// This is a flexible map structure that can hold different proxy types
type Proxy map[string]interface{}

// ProduceOptions contains options for proxy production
type ProduceOptions struct {
	IncludeUnsupportedProxy bool
	ClientCompatibilityMode bool // Auto-filter incompatible nodes for clients
	UseMihomoExternal       bool
	LocalPort               int
	DefaultNameserver       []string
	Nameserver              []string
	// FullConfig contains the complete original config for producers that need to output full config (e.g., Stash)
	FullConfig map[string]interface{}
}

// Producer is the interface for all proxy format producers
type Producer interface {
	// Produce converts proxies to the target format
	// Returns either a slice of Proxy maps or a string representation
	Produce(proxies []Proxy, outputType string, opts *ProduceOptions) (interface{}, error)

	// GetType returns the producer type (e.g., "clash", "surge", etc.)
	GetType() string
}

// Common proxy type constants
const (
	ProxyTypeShadowsocks  = "ss"
	ProxyTypeShadowsocksR = "ssr"
	ProxyTypeVMess        = "vmess"
	ProxyTypeVLess        = "vless"
	ProxyTypeTrojan       = "trojan"
	ProxyTypeHysteria     = "hysteria"
	ProxyTypeHysteria2    = "hysteria2"
	ProxyTypeTUIC         = "tuic"
	ProxyTypeWireGuard    = "wireguard"
	ProxyTypeHTTP         = "http"
	ProxyTypeSOCKS5       = "socks5"
	ProxyTypeSnell        = "snell"
)

// Common network types
const (
	NetworkTCP  = "tcp"
	NetworkWS   = "ws"
	NetworkGRPC = "grpc"
	NetworkH2   = "h2"
	NetworkHTTP = "http"
	NetworkQUIC = "quic"
)

// ProxyHelper provides common proxy manipulation functions
type ProxyHelper struct{}

// NewProxyHelper creates a new ProxyHelper
func NewProxyHelper() *ProxyHelper {
	return &ProxyHelper{}
}

// GetProxyType returns the proxy type
func (h *ProxyHelper) GetProxyType(proxy Proxy) string {
	return GetString(proxy, "type")
}

// GetProxyName returns the proxy name
func (h *ProxyHelper) GetProxyName(proxy Proxy) string {
	return GetString(proxy, "name")
}

// GetProxyServer returns the proxy server address
func (h *ProxyHelper) GetProxyServer(proxy Proxy) string {
	return GetString(proxy, "server")
}

// GetProxyPort returns the proxy port
func (h *ProxyHelper) GetProxyPort(proxy Proxy) int {
	return GetInt(proxy, "port")
}

// CloneProxy creates a deep copy of a proxy
func (h *ProxyHelper) CloneProxy(proxy Proxy) Proxy {
	clone := make(Proxy)
	for k, v := range proxy {
		clone[k] = v
	}
	return clone
}

// FilterProxies filters proxies by type
func (h *ProxyHelper) FilterProxies(proxies []Proxy, proxyTypes ...string) []Proxy {
	if len(proxyTypes) == 0 {
		return proxies
	}

	typeMap := make(map[string]bool)
	for _, t := range proxyTypes {
		typeMap[t] = true
	}

	filtered := make([]Proxy, 0)
	for _, proxy := range proxies {
		if typeMap[h.GetProxyType(proxy)] {
			filtered = append(filtered, proxy)
		}
	}
	return filtered
}

// RemoveProxyFields removes specified fields from a proxy
func (h *ProxyHelper) RemoveProxyFields(proxy Proxy, fields ...string) {
	for _, field := range fields {
		delete(proxy, field)
	}
}
