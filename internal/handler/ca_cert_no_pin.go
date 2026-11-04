package handler

import (
	"context"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

// 「正规 CA 证书节点不固定证书指纹」(系统设置 → 订阅同步),默认关。从妙妙屋X 移植(#1391)。
//
//	GET /api/admin/system-settings/ca-cert-no-pin → {"enabled":true|false}
//	PUT /api/admin/system-settings/ca-cert-no-pin   {"enabled":true|false}
//
// 按分享链接导入的 TLS 节点带着证书 SHA-256(tls-fingerprint,HY2 链接的 pinSHA256、vless / trojan 链接的
// pcs 解析而来),CA 签发的也带。各格式的订阅都会把它写成证书钉:HY2 链接 pinSHA256、vless / trojan 链接 pcs、
// mihomo fingerprint、Surge server-cert-fingerprint-sha256、Loon tls-cert-sha256、sing-box certificate_sha256……
// 钉的是叶子证书,Let's Encrypt 60~90 天一续期,客户端就连不上,直到重新拉订阅。
//
// 开了之后,输出订阅时,证书由公共 CA 签发的节点去掉证书指纹、skip-cert-verify 写成 false,
// 客户端按 SNI 正常校验,续期不受影响。自签证书照旧钉。只改输出,不改库里的节点。
//
// 「是不是公共 CA 签发」不在订阅请求里握手判断,只看已有的数据(见 certTrustIndex.publicCA):
// 后台的证书校验任务(cert_probe_trust.go)对 TCP TLS 节点握手一次,记下证书链能不能用系统根证书按 SNI 校验过
// (持久化到 system_settings.cert_probe_trust)。妙妙屋没有面板证书库,妙妙屋X 里「证书库里的证书离线校验证书链」
// 这一路没有数据来源,未移植。
//
// HY2 / TUIC 等 QUIC 节点 TCP 握手取不到证书,判断不了,照旧钉。

const (
	settingCACertNoPin = "ca_cert_no_pin"
	// settingCertProbeTrust 证书校验任务握手的结论:{"server|port|sni": true(按 SNI 校验得过)}。
	settingCertProbeTrust = "cert_probe_trust"
	certTrustIndexTTL     = 5 * time.Minute
)

var (
	caCertNoPinEnabled atomic.Bool
	caCertNoPinRepo    atomic.Pointer[storage.TrafficRepository]

	certProbeTrustMu    sync.Mutex
	certProbeTrust      = map[string]bool{}
	certProbeTrustDirty bool

	certTrustCacheMu    sync.Mutex
	certTrustCache      *certTrustIndex
	certTrustCacheBuilt time.Time
)

// LoadCACertNoPin 启动时读开关与握手结论。没设过 = 关。
func LoadCACertNoPin(ctx context.Context, repo *storage.TrafficRepository) {
	if repo == nil {
		return
	}
	caCertNoPinRepo.Store(repo)
	v, _ := repo.GetSystemSetting(ctx, settingCACertNoPin)
	caCertNoPinEnabled.Store(v == "true")
	if raw, err := repo.GetSystemSetting(ctx, settingCertProbeTrust); err == nil && strings.TrimSpace(raw) != "" {
		var m map[string]bool
		if json.Unmarshal([]byte(raw), &m) == nil {
			certProbeTrustMu.Lock()
			for k, v := range m {
				certProbeTrust[k] = v
			}
			certProbeTrustMu.Unlock()
		}
	}
}

// NewCACertNoPinHandler 开关的读写接口。
func NewCACertNoPinHandler(repo *storage.TrafficRepository) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			respondJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": caCertNoPinEnabled.Load()})
		case http.MethodPut:
			var req struct {
				Enabled *bool `json:"enabled"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Enabled == nil {
				respondJSON(w, http.StatusBadRequest, map[string]any{"success": false, "message": "enabled 必须是 true 或 false"})
				return
			}
			value := "false"
			if *req.Enabled {
				value = "true"
			}
			if err := repo.SetSystemSetting(r.Context(), settingCACertNoPin, value); err != nil {
				respondJSON(w, http.StatusInternalServerError, map[string]any{"success": false, "message": err.Error()})
				return
			}
			caCertNoPinRepo.Store(repo)
			caCertNoPinEnabled.Store(*req.Enabled)
			invalidateCertTrustIndex()
			log.Printf("[证书指纹] 正规 CA 证书节点不固定证书指纹: %v", *req.Enabled)
			if *req.Enabled {
				// 刚打开时还没有握手结论,所有节点都照旧钉;马上在后台校验一轮,不用等到每天的那次。
				triggerCertProbeTrustRefresh(repo)
			}
			respondJSON(w, http.StatusOK, map[string]any{"success": true, "enabled": *req.Enabled})
		default:
			respondJSON(w, http.StatusMethodNotAllowed, map[string]any{"success": false, "message": "method not allowed"})
		}
	})
}

// certProbeKey 握手结论的键。host 是节点的 SNI(没写 SNI 时就是 server)。
func certProbeKey(server string, port int, host string) string {
	return strings.ToLower(strings.TrimSpace(server)) + "|" + strconv.Itoa(port) + "|" + strings.ToLower(strings.TrimSpace(host))
}

// recordCertProbeTrust 证书校验任务握手后记下这个端点的证书按 SNI 校验得过没有。
func recordCertProbeTrust(server string, port int, host string, trusted bool) {
	key := certProbeKey(server, port, host)
	certProbeTrustMu.Lock()
	defer certProbeTrustMu.Unlock()
	if old, ok := certProbeTrust[key]; ok && old == trusted {
		return
	}
	certProbeTrust[key] = trusted
	certProbeTrustDirty = true
}

// saveCertProbeTrust 有变化才落库。
func saveCertProbeTrust(ctx context.Context, repo *storage.TrafficRepository) {
	certProbeTrustMu.Lock()
	if !certProbeTrustDirty || repo == nil {
		certProbeTrustMu.Unlock()
		return
	}
	b, _ := json.Marshal(certProbeTrust)
	certProbeTrustDirty = false
	certProbeTrustMu.Unlock()
	if err := repo.SetSystemSetting(ctx, settingCertProbeTrust, string(b)); err != nil {
		log.Printf("[证书指纹] 保存握手结论失败: %v", err)
	}
	invalidateCertTrustIndex()
}

func invalidateCertTrustIndex() {
	certTrustCacheMu.Lock()
	certTrustCache = nil
	certTrustCacheMu.Unlock()
}

// currentCertTrustIndex 开关关着返回 nil(调用方据此什么都不做)。握手结论按 TTL 重新快照,订阅热路径上不每次拷贝。
func currentCertTrustIndex() *certTrustIndex {
	if !caCertNoPinEnabled.Load() {
		return nil
	}
	if caCertNoPinRepo.Load() == nil {
		return nil
	}
	certTrustCacheMu.Lock()
	defer certTrustCacheMu.Unlock()
	if certTrustCache != nil && time.Since(certTrustCacheBuilt) < certTrustIndexTTL {
		return certTrustCache
	}
	certProbeTrustMu.Lock()
	probe := make(map[string]bool, len(certProbeTrust))
	for k, v := range certProbeTrust {
		probe[k] = v
	}
	certProbeTrustMu.Unlock()
	certTrustCache = buildCertTrustIndex(probe)
	certTrustCacheBuilt = time.Now()
	return certTrustCache
}

// certTrustIndex 判断节点证书是否公共 CA 签发所需的全部数据,构建一次、按 TTL 复用。
type certTrustIndex struct {
	probe map[string]bool
}

func buildCertTrustIndex(probe map[string]bool) *certTrustIndex {
	return &certTrustIndex{probe: probe}
}

// publicCA 节点(server:port,SNI 为 sni)的证书是否公共 CA 签发、客户端按 SNI 校验得过。
// 拿不准一律 false(照旧钉证书 —— 不钉的后果是连不上,钉的后果只是续期后要重新拉订阅):
// 只认证书校验任务握手过的端点,用握手结论;没握手过的(如 HY2 / TUIC)照旧钉。
func (idx *certTrustIndex) publicCA(server string, port int, sni string) bool {
	if idx == nil {
		return false
	}
	host := strings.TrimSpace(sni)
	if host == "" {
		host = strings.TrimSpace(server)
	}
	if host == "" {
		return false
	}
	if trusted, ok := idx.probe[certProbeKey(server, port, host)]; ok {
		return trusted
	}
	return false
}

// pinnedCertFingerprint 节点记录的证书指纹:tls-fingerprint(Sub-Store 内部字段),或 mihomo 的 fingerprint
// (只认 64 位十六进制的证书指纹)。
func pinnedCertFingerprint(m map[string]any) string {
	if fp := normalizedCertFingerprint(stringValue(m["tls-fingerprint"])); fp != "" {
		return fp
	}
	return normalizedCertFingerprint(stringValue(m["fingerprint"]))
}

// unpinProxy 证书公共 CA 签发的节点:去掉证书指纹、skip-cert-verify 改成 false。改了返回 true。
// REALITY 不走证书校验,不动;既没钉证书也没跳过校验的,没什么可改。
func (idx *certTrustIndex) unpinProxy(m map[string]any) bool {
	if idx == nil || m == nil || isRealityProxy(m) {
		return false
	}
	fp := pinnedCertFingerprint(m)
	skip := isTruthySkipCert(m["skip-cert-verify"])
	_, hasTLSFP := m["tls-fingerprint"]
	if fp == "" && !skip && !hasTLSFP {
		return false
	}
	if !idx.publicCA(stringValue(m["server"]), intValue(m["port"]), proxySNI(m)) {
		return false
	}
	delete(m, "tls-fingerprint")
	if normalizedCertFingerprint(stringValue(m["fingerprint"])) != "" {
		delete(m, "fingerprint")
	}
	if skip {
		m["skip-cert-verify"] = false
	}
	return true
}

// unpinPublicCACertProxies 对一批要输出的节点套用设置(开关关着什么都不做)。
func unpinPublicCACertProxies(proxies []map[string]any) {
	idx := currentCertTrustIndex()
	if idx == nil {
		return
	}
	for _, p := range proxies {
		idx.unpinProxy(p)
	}
}

// unpinPublicCACertsInClashYAML 对 Clash YAML 里的节点套用设置。开关关着、不是 Clash YAML、没改到任何节点时原样返回
// (fail-open,不弄丢订阅)。
func unpinPublicCACertsInClashYAML(data []byte) []byte {
	idx := currentCertTrustIndex()
	if idx == nil {
		return data
	}
	var root yaml.Node
	if err := yaml.Unmarshal(data, &root); err != nil || len(root.Content) == 0 {
		return data
	}
	doc := root.Content[0]
	if doc.Kind != yaml.MappingNode {
		return data
	}
	rewritten := false
	for i := 0; i+1 < len(doc.Content); i += 2 {
		if doc.Content[i].Value != "proxies" {
			continue
		}
		seq := doc.Content[i+1]
		if seq.Kind != yaml.SequenceNode {
			return data
		}
		for _, item := range seq.Content {
			if idx.unpinYAMLNode(item) {
				rewritten = true
			}
		}
		break
	}
	if !rewritten {
		return data
	}
	out, err := MarshalYAMLWithIndent(doc)
	if err != nil {
		return data
	}
	return []byte(RemoveUnicodeEscapeQuotes(string(out)))
}

// unpinYAMLNode 同 unpinProxy,作用在 YAML 节点上(原样输出 Clash YAML 时不经 map,保留原有写法)。
func (idx *certTrustIndex) unpinYAMLNode(item *yaml.Node) bool {
	if idx == nil || item.Kind != yaml.MappingNode {
		return false
	}
	m := map[string]any{}
	for i := 0; i+1 < len(item.Content); i += 2 {
		k, v := item.Content[i].Value, item.Content[i+1]
		switch k {
		case "reality-opts":
			m[k] = map[string]any{}
		case "server", "port", "sni", "servername", "server-name", "tls-fingerprint", "fingerprint", "skip-cert-verify", "security":
			if v.Kind == yaml.ScalarNode {
				m[k] = v.Value
			}
		}
	}
	_, hadTLSFP := m["tls-fingerprint"]
	_, hadFP := m["fingerprint"]
	if !idx.unpinProxy(m) {
		return false
	}
	_, hasTLSFP := m["tls-fingerprint"]
	_, hasFP := m["fingerprint"]
	kept := item.Content[:0]
	for i := 0; i+1 < len(item.Content); i += 2 {
		k, v := item.Content[i], item.Content[i+1]
		if (k.Value == "tls-fingerprint" && hadTLSFP && !hasTLSFP) || (k.Value == "fingerprint" && hadFP && !hasFP) {
			continue
		}
		if k.Value == "skip-cert-verify" && m["skip-cert-verify"] == false {
			v.Kind, v.Tag, v.Value, v.Style = yaml.ScalarNode, "!!bool", "false", 0
		}
		kept = append(kept, k, v)
	}
	item.Content = kept
	return true
}

// ---- 节点字段读取(沿用妙妙屋X 指纹补全任务里的写法) ----

func isRealityProxy(cfg map[string]any) bool {
	if v, ok := cfg["reality-opts"].(map[string]any); ok && v != nil {
		return true
	}
	return strings.EqualFold(stringValue(cfg["security"]), "reality")
}

func proxySNI(cfg map[string]any) string {
	for _, key := range []string{"sni", "servername", "server-name"} {
		if value := strings.TrimSpace(stringValue(cfg[key])); value != "" {
			return value
		}
	}
	return ""
}

const sha256HexLength = 64

func normalizedCertFingerprint(value string) string {
	value = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), ":", ""))
	if len(value) != sha256HexLength {
		return ""
	}
	if _, err := hex.DecodeString(value); err != nil {
		return ""
	}
	return value
}

func stringValue(value any) string {
	s, _ := value.(string)
	return s
}

func intValue(value any) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := strconv.Atoi(v.String())
		return n
	case string:
		n, _ := strconv.Atoi(v)
		return n
	}
	return 0
}

// peerCertTrustRoots 校验用的根证书;nil = 系统根证书。测试里替换。
var peerCertTrustRoots *x509.CertPool
