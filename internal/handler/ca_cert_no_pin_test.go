package handler

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"miaomiaowu/internal/storage"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
	"gopkg.in/yaml.v3"
)

// testCA 一个测试用根证书,放进 peerCertTrustRoots 模拟「系统信任的公共 CA」。
type testCA struct {
	cert *x509.Certificate
	key  *ecdsa.PrivateKey
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "Test Public CA"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	cert, _ := x509.ParseCertificate(der)
	old := peerCertTrustRoots
	t.Cleanup(func() { peerCertTrustRoots = old })
	pool := x509.NewCertPool()
	pool.AddCert(cert)
	peerCertTrustRoots = pool
	return &testCA{cert: cert, key: key}
}

// leafTLSCert 签一张叶子证书;ca 为 nil 时自签。
func leafTLSCert(t *testing.T, ca *testCA, dnsNames ...string) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject:      pkix.Name{CommonName: dnsNames[0]},
		DNSNames:     dnsNames,
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	parent, signer := tmpl, key
	if ca != nil {
		parent, signer = ca.cert, ca.key
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, parent, &key.PublicKey, signer)
	if err != nil {
		t.Fatal(err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

// startTLSServer 起一个用给定证书的 TLS 服务,返回地址与端口。
func startTLSServer(t *testing.T, cert tls.Certificate) (string, int) {
	t.Helper()
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	host, portText, _ := net.SplitHostPort(strings.TrimPrefix(srv.URL, "https://"))
	port, _ := strconv.Atoi(portText)
	return host, port
}

// 握手判断:CA 签发且覆盖 SNI 的证书校验得过;自签的不过;CA 签发但 SNI 不在证书里也不过。
func TestProbePeerCertTrust(t *testing.T) {
	ca := newTestCA(t)
	caHost, caPort := startTLSServer(t, leafTLSCert(t, ca, "ca.example.com"))
	selfHost, selfPort := startTLSServer(t, leafTLSCert(t, nil, "self.example.com"))

	if ok, err := probePeerCertTrust(t.Context(), caHost, caPort, "ca.example.com", ""); err != nil || !ok {
		t.Fatalf("CA-signed cert: trusted=%v err=%v, want true", ok, err)
	}
	if ok, err := probePeerCertTrust(t.Context(), caHost, caPort, "other.example.com", ""); err != nil || ok {
		t.Fatalf("CA-signed cert with wrong SNI: trusted=%v err=%v, want false", ok, err)
	}
	if ok, err := probePeerCertTrust(t.Context(), selfHost, selfPort, "self.example.com", ""); err != nil || ok {
		t.Fatalf("self-signed cert: trusted=%v err=%v, want false", ok, err)
	}
}

type certTrustFixture struct {
	idx *certTrustIndex
}

// newCertTrustFixture 模拟证书校验任务握手过的端点:1.1.1.1:443(CA 签发)、1.1.1.1:8443(自签)。
func newCertTrustFixture(t *testing.T) certTrustFixture {
	t.Helper()
	probe := map[string]bool{
		certProbeKey("1.1.1.1", 443, "ca.example.com"):        true,
		certProbeKey("1.1.1.1", 8443, "self.example.com"):     false,
		certProbeKey("ca.example.com", 443, "ca.example.com"): true,
	}
	return certTrustFixture{idx: buildCertTrustIndex(probe)}
}

func TestCertTrustIndexPublicCA(t *testing.T) {
	f := newCertTrustFixture(t)
	cases := []struct {
		name   string
		server string
		port   int
		sni    string
		want   bool
	}{
		{"握手结论:CA", "1.1.1.1", 443, "ca.example.com", true},
		{"握手结论:自签", "1.1.1.1", 8443, "self.example.com", false},
		{"握手结论按端点,换端口不算", "1.1.1.1", 9443, "ca.example.com", false},
		{"握手结论按端点,换 SNI 不算", "1.1.1.1", 443, "other.example.com", false},
		{"没写 SNI 用 server", "ca.example.com", 443, "", true},
		{"大小写不敏感", "1.1.1.1", 443, "CA.Example.com", true},
		{"没握手过(如非面板证书的 HY2)拿不准", "1.2.3.4", 443, "www.bing.com", false},
		{"server 与 SNI 都空", "", 443, "", false},
	}
	for _, c := range cases {
		if got := f.idx.publicCA(c.server, c.port, c.sni); got != c.want {
			t.Errorf("%s: publicCA(%s,%d,%s) = %v, want %v", c.name, c.server, c.port, c.sni, got, c.want)
		}
	}
	var nilIdx *certTrustIndex
	if nilIdx.publicCA("1.1.1.1", 443, "ca.example.com") {
		t.Fatal("nil index (setting off) must never report CA")
	}
}

const (
	testCAFP   = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testSelfFP = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

func TestUnpinProxyHY2URI(t *testing.T) {
	f := newCertTrustFixture(t)
	hy2 := func(port float64, sni, fp string) map[string]any {
		return map[string]any{
			"name": "hy2", "type": "hysteria2", "server": "1.1.1.1", "port": port,
			"password": "pw", "sni": sni, "skip-cert-verify": true, "tls-fingerprint": fp,
		}
	}
	uri := func(m map[string]any) string {
		s, err := substore.NewURIProducer().ProduceOne(substore.Proxy(m))
		if err != nil {
			t.Fatal(err)
		}
		return s
	}

	caNode := hy2(443, "ca.example.com", testCAFP)
	if before := uri(hy2(443, "ca.example.com", testCAFP)); !strings.Contains(before, "pinSHA256=") || !strings.Contains(before, "insecure=1") {
		t.Fatalf("precondition: CA node URI without the setting should pin: %s", before)
	}
	if !f.idx.unpinProxy(caNode) {
		t.Fatal("CA node should be unpinned")
	}
	if got := uri(caNode); strings.Contains(got, "pinSHA256") || strings.Contains(got, "insecure") || !strings.Contains(got, "sni=ca.example.com") {
		t.Fatalf("CA node URI = %s, want no pinSHA256 / insecure", got)
	}
	if caNode["skip-cert-verify"] != false {
		t.Fatalf("skip-cert-verify = %v, want false", caNode["skip-cert-verify"])
	}

	selfNode := hy2(8443, "self.example.com", testSelfFP)
	if f.idx.unpinProxy(selfNode) {
		t.Fatal("self-signed node must keep its pin")
	}
	if got := uri(selfNode); !strings.Contains(got, "pinSHA256="+testSelfFP) || !strings.Contains(got, "insecure=1") {
		t.Fatalf("self-signed node URI = %s, want pinSHA256 + insecure=1", got)
	}
}

func TestUnpinProxyOtherFormats(t *testing.T) {
	f := newCertTrustFixture(t)
	vless := map[string]any{
		"name": "v", "type": "vless", "server": "1.1.1.1", "port": float64(443), "uuid": "11111111-1111-1111-1111-111111111111",
		"tls": true, "servername": "ca.example.com", "tls-fingerprint": testCAFP, "client-fingerprint": "chrome",
	}
	if !f.idx.unpinProxy(vless) {
		t.Fatal("CA vless node should be unpinned")
	}
	if _, has := vless["tls-fingerprint"]; has || vless["client-fingerprint"] != "chrome" {
		t.Fatalf("vless after unpin = %v (uTLS client-fingerprint must stay)", vless)
	}
	if _, has := vless["skip-cert-verify"]; has {
		t.Fatalf("skip-cert-verify should not be added when absent: %v", vless)
	}
	out, err := substore.GetDefaultFactory().GetProducer("clashmeta")
	if err != nil {
		t.Fatal(err)
	}
	produced, err := out.Produce([]substore.Proxy{substore.Proxy(vless)}, "", &substore.ProduceOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if s := fmt.Sprint(produced); strings.Contains(s, testCAFP) {
		t.Fatalf("clashmeta output still pins the CA leaf:\n%s", s)
	}

	// mihomo 的 fingerprint(证书指纹)也算钉证书;REALITY 不动
	mihomo := map[string]any{"type": "trojan", "server": "1.1.1.1", "port": 443, "sni": "ca.example.com", "fingerprint": testCAFP}
	if !f.idx.unpinProxy(mihomo) {
		t.Fatal("mihomo fingerprint pin should be removed")
	}
	if _, has := mihomo["fingerprint"]; has {
		t.Fatalf("fingerprint kept: %v", mihomo)
	}
	reality := map[string]any{"type": "vless", "server": "1.1.1.1", "port": 443, "servername": "ca.example.com",
		"skip-cert-verify": true, "reality-opts": map[string]any{"public-key": "x"}}
	if f.idx.unpinProxy(reality) {
		t.Fatal("REALITY nodes must not be touched")
	}
}

func TestUnpinYAMLNodeKeepsSelfSigned(t *testing.T) {
	f := newCertTrustFixture(t)
	in := `proxies:
  - name: CA
    type: hysteria2
    server: 1.1.1.1
    port: 443
    password: pw
    sni: ca.example.com
    skip-cert-verify: true
    tls-fingerprint: ` + testCAFP + `
  - name: SELF
    type: hysteria2
    server: 1.1.1.1
    port: 8443
    password: pw
    sni: self.example.com
    skip-cert-verify: true
    tls-fingerprint: ` + testSelfFP + `
proxy-groups:
  - name: G
    type: select
    proxies: [CA, SELF]
`
	var root yaml.Node
	if err := yaml.Unmarshal([]byte(in), &root); err != nil {
		t.Fatal(err)
	}
	seq := root.Content[0].Content[1]
	if !f.idx.unpinYAMLNode(seq.Content[0]) {
		t.Fatal("CA node should be rewritten")
	}
	if f.idx.unpinYAMLNode(seq.Content[1]) {
		t.Fatal("self-signed node must not be rewritten")
	}
	out, err := MarshalYAMLWithIndent(root.Content[0])
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Proxies []map[string]any `yaml:"proxies"`
	}
	if err := yaml.Unmarshal(out, &doc); err != nil {
		t.Fatalf("output is not YAML: %v\n%s", err, out)
	}
	if len(doc.Proxies) != 2 {
		t.Fatalf("proxies = %v", doc.Proxies)
	}
	ca, self := doc.Proxies[0], doc.Proxies[1]
	if ca["skip-cert-verify"] != false || ca["tls-fingerprint"] != nil || ca["fingerprint"] != nil {
		t.Fatalf("CA proxy = %v, want skip-cert-verify false and no fingerprint", ca)
	}
	if self["skip-cert-verify"] != true || self["tls-fingerprint"] != testSelfFP {
		t.Fatalf("self-signed proxy = %v, want still pinned", self)
	}
}

// 整条链路:开关存库 → 握手结论 → 订阅 YAML / 节点列表都按设置输出;关掉后恢复原样。
func TestCACertNoPinSettingEndToEnd(t *testing.T) {
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "canopin.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	certProbeTrustMu.Lock()
	saved := certProbeTrust
	certProbeTrust = map[string]bool{}
	certProbeTrustMu.Unlock()
	t.Cleanup(func() {
		caCertNoPinEnabled.Store(false)
		certProbeTrustMu.Lock()
		certProbeTrust = saved
		certProbeTrustMu.Unlock()
		invalidateCertTrustIndex()
	})
	recordCertProbeTrust("1.1.1.1", 443, "ca.example.com", true)
	saveCertProbeTrust(context.Background(), repo)
	LoadCACertNoPin(context.Background(), repo)
	h := NewCACertNoPinHandler(repo)

	put := func(body string) {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodPut, "/api/admin/system-settings/ca-cert-no-pin", strings.NewReader(body)))
		if rec.Code != http.StatusOK {
			t.Fatalf("PUT %s = %d %s", body, rec.Code, rec.Body.String())
		}
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/admin/system-settings/ca-cert-no-pin", nil))
	if !strings.Contains(rec.Body.String(), `"enabled":false`) {
		t.Fatalf("default should be off: %s", rec.Body.String())
	}

	sub := "proxies:\n  - {name: CA, type: hysteria2, server: 1.1.1.1, port: 443, password: pw, sni: ca.example.com, skip-cert-verify: true, tls-fingerprint: " + testCAFP + "}\n"
	if got := unpinPublicCACertsInClashYAML([]byte(sub)); string(got) != sub {
		t.Fatalf("setting off must not touch output:\n%s", got)
	}

	put(`{"enabled":true}`)
	if v, _ := repo.GetSystemSetting(context.Background(), settingCACertNoPin); v != "true" {
		t.Fatalf("stored setting = %q", v)
	}
	got := string(unpinPublicCACertsInClashYAML([]byte(sub)))
	if strings.Contains(got, testCAFP) || !strings.Contains(got, "skip-cert-verify: false") {
		t.Fatalf("setting on: output still pins:\n%s", got)
	}
	proxies := []map[string]any{{"type": "hysteria2", "server": "1.1.1.1", "port": float64(443), "sni": "ca.example.com", "skip-cert-verify": true, "tls-fingerprint": testCAFP}}
	unpinPublicCACertProxies(proxies)
	if _, has := proxies[0]["tls-fingerprint"]; has {
		t.Fatalf("proxies not unpinned: %v", proxies[0])
	}

	put(`{"enabled":false}`)
	if got := unpinPublicCACertsInClashYAML([]byte(sub)); string(got) != sub {
		t.Fatalf("after turning off, output must be unchanged:\n%s", got)
	}
}

func TestCertProbeTrustPersists(t *testing.T) {
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "probe.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	certProbeTrustMu.Lock()
	saved := certProbeTrust
	certProbeTrust = map[string]bool{}
	certProbeTrustMu.Unlock()
	t.Cleanup(func() {
		certProbeTrustMu.Lock()
		certProbeTrust = saved
		certProbeTrustMu.Unlock()
	})

	recordCertProbeTrust("Node.Example.com", 443, "Node.Example.com", true)
	saveCertProbeTrust(context.Background(), repo)
	certProbeTrustMu.Lock()
	certProbeTrust = map[string]bool{}
	certProbeTrustMu.Unlock()
	LoadCACertNoPin(context.Background(), repo)
	certProbeTrustMu.Lock()
	v, ok := certProbeTrust[certProbeKey("node.example.com", 443, "node.example.com")]
	certProbeTrustMu.Unlock()
	if !ok || !v {
		t.Fatalf("probe result not restored after reload: %v %v", v, ok)
	}
}

// 证书校验任务整轮:库里有 CA 签发与自签两个节点,跑一轮后结论落库;REALITY / HY2 跳过不握手。
func TestRefreshCertProbeTrust(t *testing.T) {
	ca := newTestCA(t)
	caHost, caPort := startTLSServer(t, leafTLSCert(t, ca, "ca.example.com"))
	selfHost, selfPort := startTLSServer(t, leafTLSCert(t, nil, "self.example.com"))

	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "refresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	certProbeTrustMu.Lock()
	saved := certProbeTrust
	certProbeTrust = map[string]bool{}
	certProbeTrustMu.Unlock()
	t.Cleanup(func() {
		certProbeTrustMu.Lock()
		certProbeTrust = saved
		certProbeTrustMu.Unlock()
		invalidateCertTrustIndex()
	})
	ctx := context.Background()
	if err := repo.CreateUser(ctx, "admin", "", "", "hash", storage.RoleAdmin, ""); err != nil {
		t.Fatal(err)
	}
	mk := func(name, protocol, cfg string) {
		if _, err := repo.CreateNode(ctx, storage.Node{Username: "admin", NodeName: name, Protocol: protocol, ClashConfig: cfg, Enabled: true}); err != nil {
			t.Fatal(err)
		}
	}
	mk("ca", "trojan", fmt.Sprintf(`{"type":"trojan","server":"%s","port":%d,"sni":"ca.example.com","password":"pw"}`, caHost, caPort))
	mk("self", "trojan", fmt.Sprintf(`{"type":"trojan","server":"%s","port":%d,"sni":"self.example.com","password":"pw"}`, selfHost, selfPort))
	mk("reality", "vless", `{"type":"vless","server":"9.9.9.9","port":443,"tls":true,"servername":"ca.example.com","reality-opts":{"public-key":"x"}}`)
	mk("hy2", "hysteria2", `{"type":"hysteria2","server":"9.9.9.9","port":443,"sni":"ca.example.com","password":"pw"}`)
	mk("tuic", "tuic", `{"type":"tuic","server":"9.9.9.9","port":443,"sni":"ca.example.com","uuid":"u","password":"pw"}`)

	detail, err := refreshCertProbeTrust(ctx, repo)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(detail, "CA签发=1") || !strings.Contains(detail, "自签/校验不过=1") || !strings.Contains(detail, "跳过=3") || !strings.Contains(detail, "失败=0") {
		t.Fatalf("detail = %s", detail)
	}
	raw, _ := repo.GetSystemSetting(ctx, settingCertProbeTrust)
	if !strings.Contains(raw, certProbeKey(caHost, caPort, "ca.example.com")+`":true`) || !strings.Contains(raw, certProbeKey(selfHost, selfPort, "self.example.com")+`":false`) {
		t.Fatalf("persisted probe trust = %s", raw)
	}
	if strings.Contains(raw, "9.9.9.9") {
		t.Fatalf("REALITY / HY2 / TUIC nodes must not be probed: %s", raw)
	}
}
