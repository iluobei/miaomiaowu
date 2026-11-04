package handler

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"miaomiaowu/internal/storage"
	"miaomiaowu/internal/taskrun"
)

// 节点证书 CA 校验任务:给「正规 CA 证书节点不固定证书指纹」(ca_cert_no_pin.go)提供握手结论。
//
// 妙妙屋X 里这一步是每天的节点证书指纹补全任务顺手做的(它本来就要握手取指纹);妙妙屋没有那个任务,
// 这里只保留握手判断这一件事:对每个 TCP TLS 节点握手一次,看证书链能不能用系统根证书按 SNI 校验过,
// 按 server|port|SNI 记下结论。不改库里的节点。
//
// 开关关着不跑(没人用这些结论);打开开关时立刻跑一轮,之后每天一轮(证书可能从自签换成 CA 签发,或反过来)。
// REALITY 不走证书校验、HY2 / TUIC 是 QUIC(TCP 握手取不到证书),都跳过 —— 没有结论就照旧钉。

const (
	certProbeTrustTaskName     = "cert_probe_trust"
	certProbeTrustInterval     = 24 * time.Hour
	certProbeTrustInitialDelay = time.Minute
	certProbeTrustWorkers      = 8
	certProbeTrustDialTimeout  = 12 * time.Second
)

// certProbeTrustRunning 同一时刻只跑一轮(开关连点、定时与手动触发撞上时后来者直接放弃)。
var certProbeTrustRunning atomic.Bool

// StartCertProbeTrustRefresher 启动后延迟一会儿跑一轮(开关开着才真跑),之后每天一轮。
func StartCertProbeTrustRefresher(ctx context.Context, repo *storage.TrafficRepository) {
	if repo == nil {
		return
	}
	timer := time.NewTimer(certProbeTrustInitialDelay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		refreshCertProbeTrustIfEnabled(ctx, repo)
	}
	ticker := time.NewTicker(certProbeTrustInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCertProbeTrustIfEnabled(ctx, repo)
		}
	}
}

// triggerCertProbeTrustRefresh 开关打开时在后台立刻跑一轮。
func triggerCertProbeTrustRefresh(repo *storage.TrafficRepository) {
	go refreshCertProbeTrustIfEnabled(context.Background(), repo)
}

func refreshCertProbeTrustIfEnabled(ctx context.Context, repo *storage.TrafficRepository) {
	if !caCertNoPinEnabled.Load() {
		return
	}
	if !certProbeTrustRunning.CompareAndSwap(false, true) {
		return
	}
	defer certProbeTrustRunning.Store(false)
	taskrun.Record(ctx, certProbeTrustTaskName, func() (string, error) {
		return refreshCertProbeTrust(ctx, repo)
	})
}

// refreshCertProbeTrust 对所有启用的 TCP TLS 节点握手一轮并保存结论。
func refreshCertProbeTrust(ctx context.Context, repo *storage.TrafficRepository) (string, error) {
	users, err := repo.ListUsers(ctx, 100000)
	if err != nil {
		return "", err
	}
	var nodes []storage.Node
	for _, user := range users {
		userNodes, err := repo.ListNodes(ctx, user.Username)
		if err != nil {
			return "", fmt.Errorf("list nodes for %s: %w", user.Username, err)
		}
		nodes = append(nodes, userNodes...)
	}

	// 网络握手可能一直等到超时,用固定大小的 worker pool,别让节点多时串行跑几分钟。
	// 同一端点(server|port|SNI)只握手一次。
	type target struct {
		server, sni, alpn string
		port              int
	}
	seen := map[string]bool{}
	var targets []target
	skipped := 0
	for _, node := range nodes {
		if !node.Enabled {
			continue
		}
		cfg, ok := decodeProxyConfig(node.ClashConfig)
		// 以 Clash 配置里的 type 为准(订阅就是按它输出的),没有才看节点表的 Protocol。
		protocol := stringValue(cfg["type"])
		if protocol == "" {
			protocol = node.Protocol
		}
		if !ok || !isTLSProxy(cfg, protocol) || isRealityProxy(cfg) || isHysteriaProxy(cfg, protocol) || isQUICProxy(cfg, protocol) {
			skipped++
			continue
		}
		server := strings.TrimSpace(stringValue(cfg["server"]))
		port := intValue(cfg["port"])
		sni := proxySNI(cfg)
		if sni == "" {
			sni = server
		}
		if server == "" || port == 0 {
			skipped++
			continue
		}
		key := certProbeKey(server, port, sni)
		if seen[key] {
			continue
		}
		seen[key] = true
		targets = append(targets, target{server: server, port: port, sni: sni, alpn: proxyALPN(cfg)})
	}

	var mu sync.Mutex
	var trusted, untrusted, failed int
	var failures []string
	jobs := make(chan target)
	var wg sync.WaitGroup
	for range certProbeTrustWorkers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range jobs {
				dialCtx, cancel := context.WithTimeout(ctx, certProbeTrustDialTimeout)
				ok, err := probePeerCertTrust(dialCtx, t.server, t.port, t.sni, t.alpn)
				cancel()
				mu.Lock()
				if err != nil {
					failed++
					if len(failures) < 10 {
						failures = append(failures, fmt.Sprintf("%s:%d: %v", t.server, t.port, err))
					}
				} else {
					if ok {
						trusted++
					} else {
						untrusted++
					}
					recordCertProbeTrust(t.server, t.port, t.sni, ok)
				}
				mu.Unlock()
			}
		}()
	}
	for _, t := range targets {
		select {
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return "", ctx.Err()
		case jobs <- t:
		}
	}
	close(jobs)
	wg.Wait()
	saveCertProbeTrust(ctx, repo)

	detail := fmt.Sprintf("扫描=%d 端点=%d CA签发=%d 自签/校验不过=%d 跳过=%d 失败=%d", len(nodes), len(targets), trusted, untrusted, skipped, failed)
	if len(failures) > 0 {
		detail += ";握手失败:" + strings.Join(failures, ";")
	}
	log.Printf("[证书指纹] 节点证书 CA 校验完成: %s", detail)
	return detail, nil
}

// probePeerCertTrust 握手一次,看对端证书链能不能用根证书(peerCertTrustRoots,nil = 系统根证书)按 SNI 校验过。
// 没写 SNI 时用地址当 SNI 并按它校验,与客户端一致。
func probePeerCertTrust(ctx context.Context, server string, port int, sni, alpn string) (bool, error) {
	name := strings.TrimSpace(sni)
	if name == "" {
		name = strings.TrimSpace(server)
	}
	certs, err := fetchPeerCertChain(ctx, server, port, name, alpn)
	if err != nil {
		return false, err
	}
	opts := x509.VerifyOptions{Roots: peerCertTrustRoots, DNSName: name, Intermediates: x509.NewCertPool()}
	for _, c := range certs[1:] {
		opts.Intermediates.AddCert(c)
	}
	_, verr := certs[0].Verify(opts)
	return verr == nil, nil
}

func decodeProxyConfig(raw string) (map[string]any, bool) {
	var cfg map[string]any
	if strings.TrimSpace(raw) == "" || json.Unmarshal([]byte(raw), &cfg) != nil || cfg == nil {
		return nil, false
	}
	return cfg, true
}

func isTLSProxy(cfg map[string]any, protocol string) bool {
	p := strings.ToLower(strings.TrimSpace(protocol))
	if p == "trojan" || p == "hysteria" || p == "hysteria2" || p == "hy2" || p == "tuic" {
		return true
	}
	switch v := cfg["tls"].(type) {
	case bool:
		return v
	case string:
		return strings.EqualFold(v, "true") || strings.EqualFold(v, "tls") || v == "1"
	}
	return false
}

func isHysteriaProxy(cfg map[string]any, protocol string) bool {
	p := strings.ToLower(strings.TrimSpace(protocol))
	return p == "hysteria" || p == "hysteria2" || p == "hy2" || strings.EqualFold(stringValue(cfg["network"]), "hysteria")
}

// isQUICProxy TUIC 也是 QUIC,TCP 握手取不到证书。
func isQUICProxy(cfg map[string]any, protocol string) bool {
	p := strings.ToLower(strings.TrimSpace(protocol))
	return p == "tuic" || strings.EqualFold(stringValue(cfg["type"]), "tuic")
}

func proxyALPN(cfg map[string]any) string {
	switch values := cfg["alpn"].(type) {
	case []any:
		parts := make([]string, 0, len(values))
		for _, value := range values {
			if s := strings.TrimSpace(stringValue(value)); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ",")
	case []string:
		return strings.Join(values, ",")
	case string:
		return values
	}
	return ""
}
