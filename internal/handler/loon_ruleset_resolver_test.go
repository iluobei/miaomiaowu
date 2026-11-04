package handler

import (
	"strings"
	"testing"
)

// clash 的 YAML 规则集按 behavior 转成 Loon 规则行。
func TestConvertRuleSetPayloadByBehavior(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		behavior string
		want     []string
	}{
		{
			name:     "domain",
			body:     "payload:\n  - '+.example.com'\n  - exact.com\n  - '*.wild.com'\n",
			behavior: "domain",
			want: []string{
				"DOMAIN-SUFFIX,example.com,PROXY",
				"DOMAIN,exact.com,PROXY",
				"DOMAIN-SUFFIX,wild.com,PROXY",
			},
		},
		{
			name:     "ipcidr 分 v4/v6",
			body:     "payload:\n  - 1.2.3.0/24\n  - '2001:db8::/32'\n",
			behavior: "ipcidr",
			want: []string{
				"IP-CIDR,1.2.3.0/24,PROXY",
				"IP-CIDR6,2001:db8::/32,PROXY",
			},
		},
		{
			// classical 条目本身就是完整规则,策略要**插在选项之前**
			name:     "classical 保留 no-resolve",
			body:     "payload:\n  - DOMAIN-SUFFIX,a.com\n  - 'IP-CIDR,10.0.0.0/8,no-resolve'\n",
			behavior: "classical",
			want: []string{
				"DOMAIN-SUFFIX,a.com,PROXY",
				"IP-CIDR,10.0.0.0/8,PROXY,no-resolve",
			},
		},
		{
			name:     "纯文本清单(非 YAML)",
			body:     "# 注释\nDOMAIN-SUFFIX,b.com\n\nDOMAIN,c.com\n",
			behavior: "classical",
			want:     []string{"DOMAIN-SUFFIX,b.com,PROXY", "DOMAIN,c.com,PROXY"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := convertRuleSetPayload([]byte(c.body), c.behavior, "PROXY")
			if !ok {
				t.Fatalf("应能解析:%s", c.body)
			}
			if strings.Join(got, "\n") != strings.Join(c.want, "\n") {
				t.Errorf("得到:\n%s\n期望:\n%s", strings.Join(got, "\n"), strings.Join(c.want, "\n"))
			}
		})
	}
}

// 中间带通配的域名没有对等写法,丢掉好过写错。
func TestRuleEntryDropsMidWildcard(t *testing.T) {
	if got := ruleEntryToLoon("a*b.com", "domain", "P"); got != "" {
		t.Errorf("中间通配应丢弃,得到 %q", got)
	}
}

// 空内容 / 超大规则集都当解析不了 —— 让这条规则不生效,而不是产出巨物或空段。
func TestConvertRuleSetPayloadRejects(t *testing.T) {
	if _, ok := convertRuleSetPayload([]byte("payload: []"), "domain", "P"); ok {
		t.Error("空 payload 不该判为可解析")
	}
	var b strings.Builder
	b.WriteString("payload:\n")
	for i := 0; i < loonRuleSetMaxLines+1; i++ {
		b.WriteString("  - a.com\n")
	}
	if _, ok := convertRuleSetPayload([]byte(b.String()), "domain", "P"); ok {
		t.Error("超过上限不该判为可解析")
	}
}
