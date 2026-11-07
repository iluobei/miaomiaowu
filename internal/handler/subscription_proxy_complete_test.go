package handler

import (
	"context"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"miaomiaowu/internal/storage"
)

func TestClashProxyMissingFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		proxy map[string]any
		want  []string
	}{
		{"完整节点", map[string]any{"name": "HK", "type": "vless", "server": "1.2.3.4", "port": float64(443)}, nil},
		{"port 是 int(代码里直接赋值的)", map[string]any{"name": "HK", "type": "vless", "server": "1.2.3.4", "port": 443}, nil},
		{"port 是字符串", map[string]any{"name": "HK", "type": "ss", "server": "1.2.3.4", "port": "8388"}, nil},
		{"hysteria2 端口跳跃只有 ports", map[string]any{"name": "HY", "type": "hysteria2", "server": "1.2.3.4", "ports": "20000-30000"}, nil},
		{"mieru 只有 port-range", map[string]any{"name": "MR", "type": "mieru", "server": "1.2.3.4", "port-range": "2090-2099"}, nil},
		{"只有 server 的半个节点", map[string]any{"server": "209.248.57.93", "uuid": "u"}, []string{"name", "type", "port"}},
		{"port 为 0", map[string]any{"name": "HK", "type": "vless", "server": "1.2.3.4", "port": float64(0)}, []string{"port"}},
		{"空白 name / server", map[string]any{"name": " ", "type": "vless", "server": "", "port": float64(443)}, []string{"name", "server"}},
		// #135:Tailscale 组网节点没有 server / port,不能当残缺剔掉
		{"tailscale 节点", map[string]any{"name": "Tailscale-unraid", "type": "tailscale", "auth-key": "tskey-auth-x", "hostname": "mihomo-node", "udp": true}, nil},
		{"tailscale 节点缺 name", map[string]any{"type": "tailscale", "auth-key": "tskey-auth-x"}, []string{"name"}},
		{"direct 出站", map[string]any{"name": "直连", "type": "direct", "udp": true}, nil},
	} {
		if got := clashProxyMissingFields(tc.proxy); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: 得到 %v,期望 %v", tc.name, got, tc.want)
		}
	}
}

// 解析订阅 YAML,返回 proxies 名字列表与各组成员。
func parseProxiesAndGroups(t *testing.T, data []byte) ([]string, map[string][]string) {
	t.Helper()
	var cfg struct {
		Proxies []map[string]any `yaml:"proxies"`
		Groups  []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("解析输出失败: %v\n%s", err, data)
	}
	names := make([]string, 0, len(cfg.Proxies))
	for _, p := range cfg.Proxies {
		names = append(names, p["name"].(string))
	}
	groups := make(map[string][]string, len(cfg.Groups))
	for _, g := range cfg.Groups {
		groups[g.Name] = g.Proxies
	}
	return names, groups
}

// 静态订阅文件:残缺节点从 proxies 与各组成员里一并摘掉;只引用了残缺节点的组补占位节点,
// 占位节点同时追加进 proxies —— mihomo 对空组、悬空引用都是整份配置拒绝加载。
func TestDropIncompleteProxiesFromStaticSubscription(t *testing.T) {
	src := []byte(`proxies:
  - name: HK
    type: ss
    server: 1.2.3.4
    port: 443
    cipher: aes-128-gcm
    password: a
  - name: Broken
    server: 5.6.7.8
  - name: JP
    type: vless
    server: 9.9.9.9
    port: 443
    uuid: u
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - HK
      - Broken
      - JP
  - name: ONLY-BROKEN
    type: select
    proxies:
      - Broken
rules:
  - MATCH,PROXY
`)
	out := dropIncompleteProxiesFromStaticSubscription(src)
	names, groups := parseProxiesAndGroups(t, out)
	if !reflect.DeepEqual(names, []string{"HK", "JP", noAvailableNodeName}) {
		t.Fatalf("proxies = %v", names)
	}
	if !reflect.DeepEqual(groups["PROXY"], []string{"HK", "JP"}) {
		t.Fatalf("PROXY 组 = %v", groups["PROXY"])
	}
	if !reflect.DeepEqual(groups["ONLY-BROKEN"], []string{noAvailableNodeName}) {
		t.Fatalf("被摘空的组应补占位节点,得到 %v", groups["ONLY-BROKEN"])
	}

	// 没有残缺节点:原字节原样返回,不重新序列化
	clean := []byte("proxies:\n  - name: HK\n    type: ss\n    server: 1.2.3.4\n    port: 443\n")
	if got := dropIncompleteProxiesFromStaticSubscription(clean); string(got) != string(clean) {
		t.Fatalf("完整订阅不该被改写:\n%s", got)
	}

	// 全部残缺:只剩占位节点
	allBroken := []byte("proxies:\n  - name: A\n    server: 1.2.3.4\n  - name: B\n    type: ss\n")
	names, _ = parseProxiesAndGroups(t, dropIncompleteProxiesFromStaticSubscription(allBroken))
	if !reflect.DeepEqual(names, []string{noAvailableNodeName}) {
		t.Fatalf("全部残缺时应只剩占位节点,得到 %v", names)
	}

	// 非 YAML 内容(如 Surge 原始文本)不动
	raw := []byte("[Proxy]\nHK = ss, 1.2.3.4, 443\n")
	if got := dropIncompleteProxiesFromStaticSubscription(raw); string(got) != string(raw) {
		t.Fatalf("非 YAML 内容不该被改写:\n%s", got)
	}
}

func newProxyCompleteTestRepo(t *testing.T) *storage.TrafficRepository {
	t.Helper()
	repo, err := storage.NewTrafficRepository(filepath.Join(t.TempDir(), "proxy-complete.db"))
	if err != nil {
		t.Fatalf("NewTrafficRepository: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })
	if err := repo.CreateUser(context.Background(), "admin", "", "admin", "hash", storage.RoleAdmin, ""); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return repo
}

func createProxyCompleteTestNode(t *testing.T, repo *storage.TrafficRepository, name, clashConfig string) {
	t.Helper()
	if _, err := repo.CreateNode(context.Background(), storage.Node{
		Username: "admin", NodeName: name, Protocol: "ss", Enabled: true,
		Tag: "test", Tags: []string{"test"},
		ClashConfig: clashConfig, ParsedConfig: "{}",
	}); err != nil {
		t.Fatalf("CreateNode %s: %v", name, err)
	}
}

// 聚合 / 标签动态订阅:残缺节点不发;一个都没有时下发占位节点而不是空配置。
func TestGenerateFromSelectedTagsDropsIncompleteAndFallsBackToPlaceholder(t *testing.T) {
	repo := newProxyCompleteTestRepo(t)
	h := NewSubscriptionHandlerConcrete(repo, t.TempDir())
	file := storage.SubscribeFile{Name: "agg", SelectedTags: []string{"test"}}

	createProxyCompleteTestNode(t, repo, "Broken", `{"server":"5.6.7.8"}`)
	out, err := h.generateFromSelectedTags(context.Background(), "admin", file)
	if err != nil {
		t.Fatalf("generateFromSelectedTags: %v", err)
	}
	names, groups := parseProxiesAndGroups(t, out)
	if !reflect.DeepEqual(names, []string{noAvailableNodeName}) {
		t.Fatalf("全是残缺节点时应只剩占位节点,得到 %v", names)
	}
	if !reflect.DeepEqual(groups["PROXY"], []string{noAvailableNodeName, "DIRECT"}) {
		t.Fatalf("PROXY 组 = %v", groups["PROXY"])
	}

	createProxyCompleteTestNode(t, repo, "HK", `{"name":"HK","type":"ss","server":"1.2.3.4","port":443,"cipher":"aes-128-gcm","password":"a"}`)
	out, err = h.generateFromSelectedTags(context.Background(), "admin", file)
	if err != nil {
		t.Fatalf("generateFromSelectedTags: %v", err)
	}
	names, _ = parseProxiesAndGroups(t, out)
	if !reflect.DeepEqual(names, []string{"HK"}) {
		t.Fatalf("有完整节点时残缺节点不发、也不该有占位节点,得到 %v", names)
	}
}

// 模板订阅:同一道闸。用仓库自带的 V3 模板跑一遍。
func TestGenerateFromTemplateDropsIncompleteAndFallsBackToPlaceholder(t *testing.T) {
	t.Chdir("../..")
	repo := newProxyCompleteTestRepo(t)
	h := NewSubscriptionHandlerConcrete(repo, t.TempDir())
	file := storage.SubscribeFile{Name: "tpl", TemplateFilename: "redirhost__v3.yaml"}

	createProxyCompleteTestNode(t, repo, "Broken", `{"server":"5.6.7.8"}`)
	out, err := h.generateFromTemplate(context.Background(), "admin", file)
	if err != nil {
		t.Fatalf("generateFromTemplate: %v", err)
	}
	names, _ := parseProxiesAndGroups(t, out)
	if !reflect.DeepEqual(names, []string{noAvailableNodeName}) {
		t.Fatalf("全是残缺节点时应只剩占位节点,得到 %v", names)
	}

	createProxyCompleteTestNode(t, repo, "HK", `{"name":"HK","type":"ss","server":"1.2.3.4","port":443,"cipher":"aes-128-gcm","password":"a"}`)
	out, err = h.generateFromTemplate(context.Background(), "admin", file)
	if err != nil {
		t.Fatalf("generateFromTemplate: %v", err)
	}
	names, _ = parseProxiesAndGroups(t, out)
	if !reflect.DeepEqual(names, []string{"HK"}) {
		t.Fatalf("有完整节点时残缺节点不发、也不该有占位节点,得到 %v", names)
	}
	if strings.Contains(string(out), "Broken") {
		t.Fatalf("残缺节点不该出现在任何地方:\n%s", out)
	}
}
