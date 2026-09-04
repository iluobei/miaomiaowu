package handler

import (
	"testing"

	"gopkg.in/yaml.v3"
)

// issue #111:规则里用 REJECT-DROP 作分流策略时,不能被当成「缺失的代理组」自动创建。
func TestRejectDropNotTreatedAsMissingGroup(t *testing.T) {
	const cfg = `
proxy-groups:
  - name: PROXY
    type: select
    proxies: [DIRECT]
rules:
  - DOMAIN-SUFFIX,ads.example.com,REJECT-DROP
  - DOMAIN-SUFFIX,work.example.com,业务分组
  - MATCH,PROXY
`
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(cfg), &doc); err != nil {
		t.Fatalf("parse cfg: %v", err)
	}
	// yaml.Unmarshal 顶层是 DocumentNode,autoAddMissingProxyGroups 要的是根 MappingNode。
	if len(doc.Content) == 0 {
		t.Fatal("empty doc")
	}
	added := autoAddMissingProxyGroups(doc.Content[0])

	for _, g := range added {
		if g == "REJECT-DROP" {
			t.Errorf("REJECT-DROP 被错误地当成缺失代理组自动创建了;added=%v", added)
		}
	}
	// 真正缺失的分组仍应被补上,证明逻辑没被改坏。
	foundReal := false
	for _, g := range added {
		if g == "业务分组" {
			foundReal = true
		}
	}
	if !foundReal {
		t.Errorf("真正缺失的「业务分组」未被自动补上;added=%v", added)
	}
}

// 规则内容里提取代理组时,REJECT-DROP 不应作为组名被提取(extractProxyGroupsFromRulesContent 路径)。
func TestExtractProxyGroupsSkipsRejectDrop(t *testing.T) {
	const content = `- DOMAIN-SUFFIX,ads.example.com,REJECT-DROP
- DOMAIN-SUFFIX,work.example.com,业务分组
- MATCH,REJECT-DROP`
	groups := extractProxyGroupsFromRulesContent(content)
	for _, g := range groups {
		if g == "REJECT-DROP" {
			t.Errorf("REJECT-DROP 被当成代理组提取;groups=%v", groups)
		}
	}
}
