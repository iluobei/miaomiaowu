package handler

import (
	"strings"
	"testing"
)

// 模板预览 / 订阅生成器走 processV3Template:QX 模板要按 QX 的写法注入节点(#1198)。
func TestProcessV3TemplateRecognizesQX(t *testing.T) {
	tpl := "[general]\nserver_check_url = http://cp.cloudflare.com/generate_204\n\n[policy]\nstatic = PROXY, direct\n\n[server_local]\n# 节点由面板注入\n\n[filter_local]\nfinal, PROXY\n"
	if !looksLikeQXTemplate(tpl) {
		t.Fatal("带 [server_local] 的模板应认作 QX")
	}
	for _, other := range []string{
		"[General]\nloglevel = notify\n[Proxy]\n[Proxy Group]\n[Rule]\nFINAL,DIRECT\n",
		"proxies: []\nproxy-groups: []\nrules: []\n",
		`{"outbounds":[{"type":"selector","tag":"proxy","outbounds":["{all}"]}]}`,
	} {
		if looksLikeQXTemplate(other) {
			t.Fatalf("不该认作 QX: %q", other)
		}
	}
	h := &TemplateV3Handler{}
	out, err := h.processV3Template(tpl, []map[string]any{{"name": "n1", "type": "ss", "server": "1.1.1.1", "port": 443, "cipher": "aes-128-gcm", "password": "p"}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tag=n1") {
		t.Fatalf("节点没有写进 [server_local]:\n%s", out)
	}
	if !strings.Contains(out, "static = PROXY, direct") || !strings.Contains(out, "final, PROXY") {
		t.Fatalf("模板其余内容应原样保留:\n%s", out)
	}
}
