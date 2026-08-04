package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MMWOrg/mmwX-plugins/proxyparser/substore"
	"gopkg.in/yaml.v3"
)

func TestV2ProxyGroupsContainSelectedNodes(t *testing.T) {
	acl := "custom_proxy_group=所有节点`select`.*\ncustom_proxy_group=自动选择`url-test`.*`http://www.gstatic.com/generate_204`300\n"
	_, groups := substore.ParseACLConfig(acl)
	result := substore.GenerateClashProxyGroups(groups, []string{"香港 01", "美国 01"})
	for _, name := range []string{"香港 01", "美国 01"} {
		if !strings.Contains(result, name) {
			t.Fatalf("V2 proxy groups missing %q:\n%s", name, result)
		}
	}
}

func TestV2ConvertEndpointKeepsProxyGroupMembers(t *testing.T) {
	aclServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("ruleset=节点选择,[]FINAL\ncustom_proxy_group=节点选择`select`[]DIRECT`.*\ncustom_proxy_group=自动选择`url-test`.*`http://www.gstatic.com/generate_204`300\n"))
	}))
	defer aclServer.Close()

	body, _ := json.Marshal(convertRulesRequest{
		RuleSource:       aclServer.URL,
		Category:         "clash",
		EnableIncludeAll: true,
		ProxyNames:       []string{"香港 01", "美国 01"},
	})
	req := httptest.NewRequest(http.MethodPost, "/api/admin/templates/convert", bytes.NewReader(body))
	response := httptest.NewRecorder()
	NewTemplateConvertHandler().ServeHTTP(response, req)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var payload convertRulesResponse
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Groups []struct {
			Name    string   `yaml:"name"`
			Proxies []string `yaml:"proxies"`
		} `yaml:"proxy-groups"`
	}
	if err := yaml.Unmarshal([]byte(payload.Content), &config); err != nil {
		t.Fatal(err)
	}
	if len(config.Groups) == 0 {
		t.Fatalf("no proxy groups generated:\n%s", payload.Content)
	}
	for _, group := range config.Groups {
		if group.Name == "自动选择" && len(group.Proxies) != 2 {
			t.Fatalf("automatic group members=%v; content:\n%s", group.Proxies, payload.Content)
		}
	}
}

func TestEnsureV2ProxyGroupMembersFillsEmptyStaticGroup(t *testing.T) {
	got, err := ensureV2ProxyGroupMembers("proxy-groups:\n  - name: PROXY\n    type: select\n    proxies: []\n", []string{"香港 01", "美国 01"})
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"香港 01", "美国 01"} {
		if !strings.Contains(got, name) {
			t.Fatalf("filled group missing %q:\n%s", name, got)
		}
	}
}

func TestEnsureV2ProxyGroupMembersRejectsInvalidRuleSource(t *testing.T) {
	_, err := ensureV2ProxyGroupMembers("<!doctype html><title>login</title>", []string{"香港 01"})
	if err == nil || !strings.Contains(err.Error(), "HTML") {
		t.Fatalf("expected invalid rule source error, got %v", err)
	}
}
