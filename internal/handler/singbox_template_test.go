package handler

import (
	"encoding/json"
	"strings"
	"testing"
)

// sbTestDoc 测试里只关心这几段。
type sbTestDoc struct {
	Outbounds   []map[string]any `json:"outbounds"`
	Endpoints   []map[string]any `json:"endpoints"`
	HTTPClients []map[string]any `json:"http_clients"`
	Route       struct {
		Final   string           `json:"final"`
		Rules   []map[string]any `json:"rules"`
		RuleSet []map[string]any `json:"rule_set"`
	} `json:"route"`
	DNS struct {
		Servers []map[string]any `json:"servers"`
		Rules   []map[string]any `json:"rules"`
	} `json:"dns"`
}

func parseSBTestDoc(t *testing.T, out string) sbTestDoc {
	t.Helper()
	var doc sbTestDoc
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("输出不是合法 JSON: %v\n%s", err, out)
	}
	return doc
}

func sbFind(doc sbTestDoc, tag string) map[string]any {
	for _, ob := range doc.Outbounds {
		if ob["tag"] == tag {
			return ob
		}
	}
	return nil
}

func sbMembers(ob map[string]any) []string {
	var out []string
	list, _ := ob["outbounds"].([]any)
	for _, m := range list {
		if s, ok := m.(string); ok {
			out = append(out, s)
		}
	}
	return out
}

func sbSS(name string) map[string]any {
	return map[string]any{"name": name, "type": "ss", "server": "1.1.1.1", "port": 443, "cipher": "aes-128-gcm", "password": "p"}
}

const sbTestTemplate = `{
  // 注释和末尾逗号都是 sing-box 自己容忍的写法
  "log": { "level": "info" },
  "outbounds": [
    { "type": "selector", "tag": "代理", "outbounds": ["自动", "{all}", "直连"], "default": "不存在的节点" },
    { "type": "urltest", "tag": "自动", "outbounds": ["{all}"], "url": "https://a.example/?x=1&y=2", "interval": "5m" },
    { "type": "selector", "tag": "香港", "outbounds": ["{all}"],
      "filter": [ { "action": "include", "keywords": ["HK|香港"] }, { "action": "exclude", "keywords": ["高倍"] } ] },
    { "type": "selector", "tag": "日本", "outbounds": ["{all}"], "filter": [ { "action": "include", "keywords": ["JP"] } ] },
    { "type": "direct", "tag": "直连" },
  ],
  "route": { "final": "代理", "rules": [ { "domain_suffix": ["jp"], "outbound": "日本" } ] },
}`

func TestInjectSingboxTemplate_ExpandsPlaceholdersAndFilters(t *testing.T) {
	proxies := []map[string]any{sbSS("HK-01"), sbSS("香港-高倍"), sbSS("US-01")}
	out, err := injectProxiesIntoSingboxTemplate(sbTestTemplate, proxies, nil)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)

	if got := strings.Join(sbMembers(sbFind(doc, "代理")), ","); got != "自动,HK-01,香港-高倍,US-01,直连" {
		t.Errorf("{all} 应在原位置展开成全部节点,实际 %s", got)
	}
	if got := strings.Join(sbMembers(sbFind(doc, "香港")), ","); got != "HK-01" {
		t.Errorf("filter 应先 include 再 exclude,实际 %s", got)
	}
	// 一个节点都没筛到的分组:路由规则还引用着它,不能删,也不能留空(sing-box 拒载)。
	if got := strings.Join(sbMembers(sbFind(doc, "日本")), ","); got != "直连" {
		t.Errorf("筛空的分组应只剩直连,实际 %s", got)
	}
	if _, has := sbFind(doc, "代理")["default"]; has {
		t.Errorf("default 指向不在分组里的出站时应去掉")
	}
	for _, ob := range doc.Outbounds {
		if _, has := ob["filter"]; has {
			t.Errorf("filter 不是 sing-box 的字段,输出里不能留: %v", ob["tag"])
		}
	}
	for _, name := range []string{"HK-01", "香港-高倍", "US-01"} {
		if ob := sbFind(doc, name); ob == nil || ob["type"] != "shadowsocks" {
			t.Errorf("节点 %s 应作为出站写进配置", name)
		}
	}
	if doc.Route.Final != "代理" {
		t.Errorf("模板的其它段应原样保留")
	}
	// 模板里的键顺序、地址里的 & 都不该被改写。
	if !strings.Contains(out, `"url": "https://a.example/?x=1&y=2"`) {
		t.Errorf("地址被转义了:\n%s", out)
	}
	if strings.Index(out, `"log"`) > strings.Index(out, `"outbounds"`) || strings.Index(out, `"outbounds"`) > strings.Index(out, `"route"`) {
		t.Errorf("顶层键顺序应与模板一致")
	}
}

// 分组没写 {all} 也放进全部节点:接在模板写的成员后面,filter 照样生效,已经写过的节点不重复。
func TestInjectSingboxTemplate_GroupsWithoutPlaceholderGetAllNodes(t *testing.T) {
	tpl := `{
  "outbounds": [
    { "type": "selector", "tag": "代理", "outbounds": ["自动", "直连"] },
    { "type": "urltest", "tag": "自动", "outbounds": [] },
    { "type": "selector", "tag": "香港", "outbounds": ["US-01"], "filter": [ { "action": "include", "keywords": ["HK"] } ] },
    { "type": "selector", "tag": "写了一个节点", "outbounds": ["HK-01"] },
    { "type": "selector", "tag": "只选分组", "outbounds": ["代理", "直连"], "include_all": false },
    { "type": "selector", "tag": "关了但写了占位符", "outbounds": ["{all}"], "include_all": false },
    { "type": "direct", "tag": "直连", "include_all": false }
  ]
}`
	proxies := []map[string]any{sbSS("HK-01"), sbSS("US-01")}
	out, err := injectProxiesIntoSingboxTemplate(tpl, proxies, nil)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)
	for group, want := range map[string]string{
		"代理":       "自动,直连,HK-01,US-01",
		"自动":       "HK-01,US-01",
		"香港":       "US-01,HK-01",
		"写了一个节点":   "HK-01,US-01",
		"只选分组":     "代理,直连",
		"关了但写了占位符": "HK-01,US-01",
	} {
		if got := strings.Join(sbMembers(sbFind(doc, group)), ","); got != want {
			t.Errorf("分组 %s 应为 %s,实际 %s", group, want, got)
		}
	}
	if strings.Contains(out, "include_all") {
		t.Errorf("include_all 不是 sing-box 的字段,输出里不能留:\n%s", out)
	}
}

// 中转组是动态生成的:节点的 detour 指向它,模板里没有,必须一起变成 urltest 出站。
func TestInjectSingboxTemplate_RelayGroupsAndDanglingDetour(t *testing.T) {
	landing := sbSS("落地")
	landing["dialer-proxy"] = "中转组"
	orphan := sbSS("孤儿")
	orphan["dialer-proxy"] = "没有成员的组"
	lost := sbSS("丢链")
	lost["dialer-proxy"] = "不存在的节点"
	proxies := []map[string]any{sbSS("入口A"), sbSS("入口B"), landing, orphan, lost}
	relayGroups := []map[string]any{
		{"name": "中转组", "type": "url-test", "proxies": []string{"入口A", "入口B", "已删除"}, "url": "http://www.gstatic.com/generate_204", "interval": 300},
		{"name": "没有成员的组", "type": "url-test", "proxies": []string{"已删除"}},
	}
	out, err := injectProxiesIntoSingboxTemplate(sbTestTemplate, proxies, relayGroups)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)

	group := sbFind(doc, "中转组")
	if group == nil || group["type"] != "urltest" {
		t.Fatalf("中转组应变成 urltest 出站:\n%s", out)
	}
	if got := strings.Join(sbMembers(group), ","); got != "入口A,入口B" {
		t.Errorf("中转组成员应剔掉不存在的,实际 %s", got)
	}
	if group["interval"] != "300s" {
		t.Errorf("间隔应带单位,实际 %v", group["interval"])
	}
	if ob := sbFind(doc, "落地"); ob == nil || ob["detour"] != "中转组" {
		t.Errorf("落地节点应保留指向中转组的 detour")
	}
	// detour 悬空的节点:留着 sing-box 整份拒载,摘掉 detour 则绕过入口直连落地 —— 两样都不行,只能不发。
	for _, name := range []string{"孤儿", "丢链", "没有成员的组"} {
		if sbFind(doc, name) != nil {
			t.Errorf("%s 不该出现在输出里", name)
		}
	}
	if got := strings.Join(sbMembers(sbFind(doc, "代理")), ","); got != "自动,入口A,入口B,落地,直连" {
		t.Errorf("分组里只该有真正下发的节点,实际 %s", got)
	}
	// 整份配置里不能有任何悬空引用。
	tags := map[string]bool{}
	for _, ob := range doc.Outbounds {
		tags[ob["tag"].(string)] = true
	}
	for _, ob := range doc.Outbounds {
		for _, m := range sbMembers(ob) {
			if !tags[m] {
				t.Errorf("分组 %v 引用了不存在的出站 %s", ob["tag"], m)
			}
		}
		if d, _ := ob["detour"].(string); d != "" && !tags[d] {
			t.Errorf("出站 %v 的 detour %s 不存在", ob["tag"], d)
		}
	}
}

func TestInjectSingboxTemplate_WireGuardGoesToEndpoints(t *testing.T) {
	wg := map[string]any{
		"name": "WG-01", "type": "wireguard", "server": "2.2.2.2", "port": 51820,
		"ip": "10.0.0.2", "private-key": "eCtXsJZ27+4PbhDkHnB923tkUn2Gj59wZw5wFA75MnU=",
		"public-key": "Cr8hWlKvtDt7nrvf+f0brNQQzabAqrjfBvas9pmowjo=", "udp": true,
	}
	out, err := injectProxiesIntoSingboxTemplate(sbTestTemplate, []map[string]any{sbSS("HK-01"), wg}, nil)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)
	if sbFind(doc, "WG-01") != nil {
		t.Errorf("WireGuard 在 sing-box 里是 endpoint,不能放进 outbounds")
	}
	if len(doc.Endpoints) != 1 || doc.Endpoints[0]["tag"] != "WG-01" {
		t.Fatalf("WireGuard 应写进 endpoints:\n%s", out)
	}
	if got := strings.Join(sbMembers(sbFind(doc, "代理")), ","); got != "自动,HK-01,WG-01,直连" {
		t.Errorf("endpoint 一样能被分组引用,实际 %s", got)
	}
}

func TestInjectSingboxTemplate_SkipsUnsupportedAndCollidingNodes(t *testing.T) {
	collide := sbSS("直连") // 与模板里的出站重名
	unsupported := map[string]any{"name": "SSR-01", "type": "ssr", "server": "3.3.3.3", "port": 1, "cipher": "aes-128-cfb", "password": "p", "protocol": "origin", "obfs": "plain"}
	out, err := injectProxiesIntoSingboxTemplate(sbTestTemplate, []map[string]any{collide, unsupported, sbSS("HK-01")}, nil)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)
	count := 0
	for _, ob := range doc.Outbounds {
		if ob["tag"] == "直连" {
			count++
			if ob["type"] != "direct" {
				t.Errorf("重名时应保留模板里的出站")
			}
		}
	}
	if count != 1 {
		t.Errorf("tag 不能重复,「直连」出现了 %d 次", count)
	}
	if sbFind(doc, "SSR-01") != nil {
		t.Errorf("sing-box 不支持的节点不该下发")
	}
	if got := strings.Join(sbMembers(sbFind(doc, "代理")), ","); got != "自动,HK-01,直连" {
		t.Errorf("分组成员不对,实际 %s", got)
	}
}

// 模板里的规则 / final / DNS detour 指向不存在的出站(面板里删了分组,或规则直指的节点这次没下发):
// sing-box 遇到是整份拒载,所以去掉这些引用,让流量落到默认出站。
func TestInjectSingboxTemplate_PrunesDanglingRefs(t *testing.T) {
	tpl := `{
  "dns": { "servers": [ { "type": "https", "tag": "a", "server": "1.1.1.1", "detour": "已删的组" }, { "type": "udp", "tag": "b", "server": "223.5.5.5", "detour": "代理" } ] },
  "http_clients": [ { "tag": "x", "detour": "已删的组" }, { "tag": "y", "detour": "代理" } ],
  "outbounds": [ { "type": "selector", "tag": "代理", "outbounds": ["{all}"] }, { "type": "direct", "tag": "直连" } ],
  "route": {
    "rules": [
      { "action": "sniff" },
      { "domain_suffix": ["a.com"], "outbound": "已删的组" },
      { "domain_suffix": ["b.com"], "outbound": "HK-01" },
      { "domain_suffix": ["c.com"], "outbound": "没下发的节点" },
      { "domain_suffix": ["d.com"], "action": "reject" }
    ],
    "final": "已删的组"
  }
}`
	out, err := injectProxiesIntoSingboxTemplate(tpl, []map[string]any{sbSS("HK-01")}, nil)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)
	if doc.Route.Final != "" {
		t.Errorf("final 指向不存在的出站时应去掉,实际 %q", doc.Route.Final)
	}
	if len(doc.Route.Rules) != 3 {
		t.Fatalf("应只去掉指向不存在出站的两条规则,剩 %d 条:\n%s", len(doc.Route.Rules), out)
	}
	if doc.Route.Rules[0]["action"] != "sniff" || doc.Route.Rules[1]["outbound"] != "HK-01" || doc.Route.Rules[2]["action"] != "reject" {
		t.Errorf("留下的规则不对: %v", doc.Route.Rules)
	}
	if _, has := doc.DNS.Servers[0]["detour"]; has {
		t.Errorf("DNS 服务器的悬空 detour 应去掉")
	}
	if doc.DNS.Servers[1]["detour"] != "代理" {
		t.Errorf("有效的 detour 不该动")
	}
	if _, has := doc.HTTPClients[0]["detour"]; has || doc.HTTPClients[1]["detour"] != "代理" {
		t.Errorf("HTTP 客户端的悬空 detour 应去掉、有效的保留: %v", doc.HTTPClients)
	}
}

// sing-box 解析配置时任何一个出站不合格都是整份拒载。两种不发,别的节点照常:
// snell(sing-box 客户端还不支持)、REALITY 公钥为空的节点(invalid public_key)。
func TestInjectSingboxTemplate_SkipsNodesThatBreakWholeConfig(t *testing.T) {
	reality := func(name, publicKey string) map[string]any {
		return map[string]any{
			"name": name, "type": "vless", "server": "4.4.4.4", "port": 443, "uuid": "11111111-2222-3333-4444-555555555555",
			"network": "tcp", "tls": true, "servername": "www.example.com", "flow": "xtls-rprx-vision",
			"client-fingerprint": "chrome", "reality-opts": map[string]any{"public-key": publicKey, "short-id": "01ab"},
		}
	}
	snell := map[string]any{"name": "SNELL-01", "type": "snell", "server": "5.5.5.5", "port": 1234, "psk": "p", "version": 4}
	good := reality("REALITY-好", "Cr8hWlKvtDt7nrvf-f0brNQQzabAqrjfBvas9pmowjo")
	out, err := injectProxiesIntoSingboxTemplate(sbTestTemplate, []map[string]any{snell, reality("REALITY-空公钥", ""), reality("REALITY-坏公钥", "abc"), good, sbSS("HK-01")}, nil)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)
	for _, name := range []string{"SNELL-01", "REALITY-空公钥", "REALITY-坏公钥"} {
		if sbFind(doc, name) != nil {
			t.Errorf("%s 会让 sing-box 拒载整份配置,不该下发", name)
		}
	}
	if ob := sbFind(doc, "REALITY-好"); ob == nil || ob["type"] != "vless" {
		t.Fatalf("公钥合法的 REALITY 节点应照常下发:\n%s", out)
	}
	if got := strings.Join(sbMembers(sbFind(doc, "代理")), ","); got != "自动,REALITY-好,HK-01,直连" {
		t.Errorf("分组里只该有真正下发的节点,实际 %s", got)
	}
}

// 模板里没有直连出站、分组又被筛空时,补一个直连兜底。
func TestInjectSingboxTemplate_AddsDirectWhenTemplateHasNone(t *testing.T) {
	tpl := `{"outbounds":[{"type":"selector","tag":"日本","outbounds":["{all}"],"filter":[{"action":"include","keywords":["JP"]}]}]}`
	out, err := injectProxiesIntoSingboxTemplate(tpl, []map[string]any{sbSS("HK-01")}, nil)
	if err != nil {
		t.Fatalf("注入失败: %v", err)
	}
	doc := parseSBTestDoc(t, out)
	if got := strings.Join(sbMembers(sbFind(doc, "日本")), ","); got != "direct" {
		t.Errorf("筛空的分组应指向补上的直连,实际 %s", got)
	}
	if ob := sbFind(doc, "direct"); ob == nil || ob["type"] != "direct" {
		t.Errorf("应补一个直连出站")
	}
}

// 社区模板的关键字是照 Python 正则写的。里面有一段 Go 不支持的写法(这里是前瞻)时,
// 退回「按 | 拆开逐个做子串匹配」:那一段匹配不上,但其余的照常生效,不能让整条筛选失效。
func TestApplySingboxFilters_FallsBackOnUnsupportedRegex(t *testing.T) {
	raw := json.RawMessage(`[{"action":"exclude","keywords":["(?!x)剩余", "官网|到期"]}]`)
	got := applySingboxFilters([]string{"HK-01", "到期时间", "官网地址"}, raw)
	if strings.Join(got, ",") != "HK-01" {
		t.Errorf("实际 %v", got)
	}
}

func TestValidateSingboxTemplate(t *testing.T) {
	cases := []struct {
		name    string
		content string
		wantErr string
	}{
		{"合格", sbTestTemplate, ""},
		{"空", "  ", "为空"},
		{"不是 JSON", "proxies:\n  - a", "JSON"},
		{"少逗号", `{"outbounds": [{"type":"direct" "tag":"d"}]}`, "行"},
		{"没有 outbounds", `{"route": {}}`, "outbounds"},
		{"outbounds 不是数组", `{"outbounds": {}}`, "数组"},
		{"出站没有 tag", `{"outbounds": [{"type":"direct"}]}`, "tag"},
		{"tag 重复", `{"outbounds": [{"type":"direct","tag":"a"},{"type":"block","tag":"a"}]}`, "重复"},
		{"成员不是字符串", `{"outbounds": [{"type":"selector","tag":"a","outbounds":[1]}]}`, "字符串数组"},
	}
	for _, c := range cases {
		err := validateSingboxTemplate(c.content)
		switch {
		case c.wantErr == "" && err != nil:
			t.Errorf("%s: 不该报错,实际 %v", c.name, err)
		case c.wantErr != "" && err == nil:
			t.Errorf("%s: 应该报错", c.name)
		case c.wantErr != "" && !strings.Contains(err.Error(), c.wantErr):
			t.Errorf("%s: 报错里应提到 %q,实际 %v", c.name, c.wantErr, err)
		}
	}
}

func TestLooksLikeSingboxTemplate(t *testing.T) {
	if !looksLikeSingboxTemplate(sbTestTemplate) {
		t.Errorf("sing-box 模板没认出来")
	}
	for name, content := range map[string]string{
		"Clash YAML":    "proxy-groups:\n  - name: a\n",
		"JSON 写的 Clash": `{"proxies": [], "proxy-groups": [], "outbounds": []}`,
		"Surge":         "[General]\n[Proxy]\n",
	} {
		if looksLikeSingboxTemplate(content) {
			t.Errorf("%s 被误认成 sing-box 模板", name)
		}
	}
}

// 模板预览 / 订阅生成器走 processV3Template:sing-box 模板要按 JSON 注入,不能落到 Clash 处理器。
func TestProcessV3TemplateRecognizesSingbox(t *testing.T) {
	h := &TemplateV3Handler{}
	out, err := h.processV3Template(sbTestTemplate, []map[string]any{sbSS("HK-01")})
	if err != nil {
		t.Fatal(err)
	}
	doc := parseSBTestDoc(t, out)
	if sbFind(doc, "HK-01") == nil || doc.Route.Final != "代理" {
		t.Fatalf("节点没写进 sing-box 模板:\n%s", out)
	}
}

// 模板种类必须按扩展名分对:.json 被判成 clash 的话,订阅生成时会走 Clash 处理器。
func TestTemplateKindOfFileRecognisesSingbox(t *testing.T) {
	if got := templateKindOfFile("default__singbox.json"); got != templateKindSingbox {
		t.Errorf("templateKindOfFile(.json) = %d,期望 templateKindSingbox", got)
	}
	if got := templateKindForClient("sing-box"); got != templateKindSingbox {
		t.Errorf("templateKindForClient(\"sing-box\") = %d,期望 templateKindSingbox", got)
	}
	for _, ct := range []string{"clash", "surge", "loon", "qx", ""} {
		if templateKindForClient(ct) == templateKindSingbox {
			t.Errorf("客户端 %q 被误判成 sing-box", ct)
		}
	}
}
