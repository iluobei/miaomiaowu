package handler

import (
	"fmt"
	"strings"
	"testing"
)

func qxTemplateFixture() string {
	return strings.Join([]string{
		"[general]",
		"server_check_url = http://cp.cloudflare.com/generate_204",
		"",
		"[policy]",
		"static = 🚀 节点选择, ♻️ 自动选择, direct",
		"",
		"[server_local]",
		"# 节点由面板注入",
		";shadowsocks=example.com:80, method=aes-128-gcm, password=x, tag=注释掉的示例",
		"shadowsocks=example.com:80, method=aes-128-gcm, password=x, tag=占位节点",
		"",
		"[filter_local]",
		"final, 🚀 节点选择",
	}, "\n")
}

func qxProxy(name string) map[string]any {
	return map[string]any{
		"name": name, "type": "ss", "server": "1.2.3.4", "port": 443,
		"cipher": "aes-128-gcm", "password": "pw",
	}
}

// .qx 是 QX 模板,.conf 归 Surge —— 两者共用后缀就只能靠猜内容,
// 猜错的后果是一份彻底错的配置(Loon 当初选 .lcf 也是这个理由)。
func TestQXTemplateFileDetection(t *testing.T) {
	for name, want := range map[string]bool{
		"starter__qx.qx":       true,
		"STARTER__QX.QX":       true,
		"surge_cn__surge.conf": false,
		"kelee.lcf":            false,
		"fake_ip__v3.yaml":     false,
	} {
		if got := isQXTemplateFile(name); got != want {
			t.Errorf("isQXTemplateFile(%q) = %v,期望 %v", name, got, want)
		}
	}
}

// 节点必须落进 [server_local],而且段内原有的**占位节点要被清掉** ——
// 不清的话模板里那条示例节点会跟着发到用户手上。注释要留着(它是使用说明)。
func TestInjectProxiesIntoQXTemplate(t *testing.T) {
	out, err := injectProxiesIntoQXTemplate(qxTemplateFixture(), []map[string]any{
		qxProxy("🇭🇰 香港01"), qxProxy("🇯🇵 日本01"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "tag=🇭🇰 香港01") || !strings.Contains(out, "tag=🇯🇵 日本01") {
		t.Fatalf("节点没注入:\n%s", out)
	}
	if strings.Contains(out, "tag=占位节点") {
		t.Fatalf("段内**未注释**的占位节点没被清掉 —— 它会跟着发到用户手上:\n%s", out)
	}
	// 注释要留着(它是给用户看的使用说明)。QX 官方 sample 用 ; 注掉示例行,
	// 两种注释符都得认 —— 只认 # 的话,; 开头的示例节点会被当成真节点留在配置里。
	if !strings.Contains(out, "# 节点由面板注入") {
		t.Fatal("# 注释被误删了")
	}
	if !strings.Contains(out, "tag=注释掉的示例") {
		t.Fatal("; 注释被误删了 —— QX 官方示例正是用 ; 注掉的")
	}
	// 其它段落必须原样保留:策略组和规则全由模板决定,后端不该碰。
	for _, keep := range []string{"[general]", "[policy]", "static = 🚀 节点选择", "final, 🚀 节点选择"} {
		if !strings.Contains(out, keep) {
			t.Fatalf("模板内容 %q 丢失:\n%s", keep, out)
		}
	}
	// 注入点必须在 [server_local] 段内,不能跑到别的段里。
	serverIdx := strings.Index(out, "[server_local]")
	filterIdx := strings.Index(out, "[filter_local]")
	nodeIdx := strings.Index(out, "tag=🇭🇰 香港01")
	if !(serverIdx < nodeIdx && nodeIdx < filterIdx) {
		t.Fatalf("节点没落在 [server_local] 段内(server=%d node=%d filter=%d)", serverIdx, nodeIdx, filterIdx)
	}
}

// 模板没有 [server_local] 段时要补一个,否则节点无处可去、订阅里一个节点都没有。
func TestInjectProxiesIntoQXTemplateAppendsMissingSection(t *testing.T) {
	out, err := injectProxiesIntoQXTemplate("[general]\nno-ipv6\n", []map[string]any{qxProxy("A")})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "[server_local]") || !strings.Contains(out, "tag=A") {
		t.Fatalf("缺段时没补上:\n%s", out)
	}
}

func TestQXTemplateOmitsEmptySections(t *testing.T) {
	for _, newline := range []string{"\n", "\r\n"} {
		t.Run(fmt.Sprintf("newline-%q", newline), func(t *testing.T) {
			template := strings.Join([]string{
				"# template preamble", "[general]", "server_check_url = https://example.com/",
				"", "[server_remote]", "  ", "[filter_remote]", "# example", "; example", "// example",
				"[server_local]", "// node explanation", "[filter_local]", "final, direct",
				"", "[rewrite_remote]", "https://example.com/rewrite.txt, tag=rules", "", "[mitm]",
			}, newline)
			for _, withNodes := range []bool{false, true} {
				var proxies []map[string]any
				if withNodes {
					proxies = []map[string]any{qxProxy("test")}
				}
				out, err := injectProxiesIntoQXTemplate(template, proxies)
				if err != nil {
					t.Fatal(err)
				}
				for _, section := range []string{"[server_remote]", "[filter_remote]", "[mitm]"} {
					if strings.Contains(out, section) {
						t.Fatalf("empty section %s retained:\n%s", section, out)
					}
				}
				for _, keep := range []string{"# template preamble", "[general]" + newline + "server_check_url = https://example.com/", "[filter_local]" + newline + "final, direct", "[rewrite_remote]" + newline + "https://example.com/rewrite.txt, tag=rules"} {
					if !strings.Contains(out, keep) {
						t.Fatalf("populated section or preamble changed: %q\n%s", keep, out)
					}
				}
				if strings.Contains(out, "[server_local]") != withNodes || strings.Contains(out, "// node explanation") != withNodes {
					t.Fatalf("server_local must follow injected nodes: withNodes=%v\n%s", withNodes, out)
				}
			}
		})
	}
	if out, err := injectProxiesIntoQXTemplate("# preamble\n", nil); err != nil || strings.Contains(out, "[server_local]") {
		t.Fatalf("must not append an empty node section: %q, %v", out, err)
	}
}

// 模板种类必须按扩展名分对:.qx 被判成 clash 的话,面板会拿 YAML 编辑器去编辑它、
// 订阅生成时也会走 Clash 处理器 —— 产出一份彻底错的配置(Loon 当初就是踩了这个)。
func TestTemplateKindOfFileRecognisesQX(t *testing.T) {
	for name, want := range map[string]templateKind{
		"starter__qx.qx":       templateKindQX,
		"surge_cn__surge.conf": templateKindSurge,
		"kelee.lcf":            templateKindLoon,
		"fake_ip__v3.yaml":     templateKindClash,
	} {
		if got := templateKindOfFile(name); got != want {
			t.Errorf("templateKindOfFile(%q) = %d,期望 %d", name, got, want)
		}
	}
	if got := templateKindForClient("qx"); got != templateKindQX {
		t.Errorf("templateKindForClient(\"qx\") = %d,期望 templateKindQX", got)
	}
	// 其它客户端不能被误判成 QX。
	for _, ct := range []string{"clash", "surge", "loon", ""} {
		if templateKindForClient(ct) == templateKindQX {
			t.Errorf("客户端 %q 被误判成 QX", ct)
		}
	}
}
