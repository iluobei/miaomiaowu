package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"miaomiaowu/internal/storage"
)

func clashOf(t *testing.T, node storage.Node) map[string]any {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.ClashConfig), &cfg); err != nil {
		t.Fatalf("clash config: %v", err)
	}
	return cfg
}

func nodesByName(t *testing.T, repo *storage.TrafficRepository, user string) map[string]storage.Node {
	t.Helper()
	got := map[string]storage.Node{}
	for _, n := range listUserNodes(t, repo, user) {
		got[n.NodeName] = n
	}
	return got
}

func TestStripSubInfoSuffix(t *testing.T) {
	tests := map[string]string{
		"香港":                 "香港",
		"香港 12.34GB📊":        "香港",
		"香港 12.34GB📊 5Days⏳": "香港",
		"〖机场A〗香港 398.00GB📊 26Days⏳": "〖机场A〗香港",
		"日本 12Days⏳":                "日本",
		"美国 512MB📊":                 "美国",
	}
	for in, want := range tests {
		if got := stripSubInfoSuffix(in); got != want {
			t.Errorf("stripSubInfoSuffix(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMatchRule_NodeName_DoesNotOverwriteOtherSubscription(t *testing.T) {
	const user = "match-cross-name"
	repo := relayTestRepo(t)
	otherURL := "https://other.example/sub"
	other := seedTaggedNode(t, repo, user, otherURL, "香港", "9.9.9.9", 443, "机场B")
	mustCreateExternalSub(t, repo, user, "机场B", otherURL)

	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all"}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2 isolated rows: %v", len(nodes), sortedNames(nodes))
	}
	got := nodesByName(t, repo, user)
	if got["香港"].ID == other.ID {
		// both named 香港 — identify by raw_url
	}
	var ours, theirs storage.Node
	for _, n := range nodes {
		if n.ID == other.ID {
			theirs = n
		} else {
			ours = n
		}
	}
	if theirs.RawURL != otherURL || clashOf(t, theirs)["server"] != "9.9.9.9" || theirs.Tag != "机场B" {
		t.Fatalf("other subscription was overwritten: %+v", theirs)
	}
	if ours.RawURL != feed.URL || clashOf(t, ours)["server"] != "1.1.1.1" || ours.Tag != "机场A" {
		t.Fatalf("new node not created for current sub: %+v", ours)
	}
}

func TestMatchRule_ServerPort_DoesNotOverwriteOtherSubscription(t *testing.T) {
	const user = "match-cross-sp"
	repo := relayTestRepo(t)
	otherURL := "https://other.example/sub"
	other := seedTaggedNode(t, repo, user, otherURL, "B-香港", "1.1.1.1", 443, "机场B")
	mustCreateExternalSub(t, repo, user, "机场B", otherURL)

	feed := clashFeed(t, proxyJSON("A-香港", "1.1.1.1", 443))
	settings := storage.UserSettings{Username: user, MatchRule: "server_port", SyncScope: "all"}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2: %v", len(nodes), sortedNames(nodes))
	}
	for _, n := range nodes {
		if n.ID == other.ID {
			if n.NodeName != "B-香港" || n.RawURL != otherURL || n.Tag != "机场B" {
				t.Fatalf("other sub overwritten: %+v", n)
			}
		} else if n.NodeName != "A-香港" || n.RawURL != feed.URL {
			t.Fatalf("current sub node unexpected: %+v", n)
		}
	}
}

func TestMatchRule_NodeName_PrefixedSameNameDoesNotOverwriteOtherSub(t *testing.T) {
	const user = "match-prefix-cross"
	repo := relayTestRepo(t)
	otherURL := "https://other.example/sub"
	other := seedTaggedNode(t, repo, user, otherURL, "【A】香港", "9.9.9.9", 443, "机场B")
	mustCreateExternalSub(t, repo, user, "机场B", otherURL)

	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all"}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL, NamePrefix: "【A】"}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 2 {
		t.Fatalf("got %d nodes, want 2: %v", len(nodes), sortedNames(nodes))
	}
	for _, n := range nodes {
		if n.ID == other.ID {
			if n.RawURL != otherURL || clashOf(t, n)["server"] != "9.9.9.9" {
				t.Fatalf("other sub overwritten: %+v", n)
			}
		} else if n.RawURL != feed.URL || n.NodeName != "【A】香港" {
			t.Fatalf("current sub node unexpected: %+v", n)
		}
	}
}

func TestMatchRule_NodeName_AirportRenameKeepsID(t *testing.T) {
	const user = "match-rename"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港2", "1.1.1.1", 443))
	old := seedProxyNode(t, repo, user, feed.URL, "香港", "1.1.1.1", 443)

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all", KeepNodeName: false}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1: %v", len(nodes), sortedNames(nodes))
	}
	if nodes[0].ID != old.ID {
		t.Fatalf("id=%d, want kept %d", nodes[0].ID, old.ID)
	}
	if nodes[0].NodeName != "香港2" {
		t.Fatalf("name=%q, want 香港2", nodes[0].NodeName)
	}
}

func TestMatchRule_NodeName_CustomNameKept(t *testing.T) {
	const user = "match-keep"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
	custom := seedProxyNode(t, repo, user, feed.URL, "我的香港", "1.1.1.1", 443)

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all", KeepNodeName: true}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes: %v", len(nodes), sortedNames(nodes))
	}
	if nodes[0].ID != custom.ID || nodes[0].NodeName != "我的香港" {
		t.Fatalf("custom name/id lost: id=%d name=%q", nodes[0].ID, nodes[0].NodeName)
	}
}

func TestMatchRule_NodeName_DuplicateNamesDedupedInOneRound(t *testing.T) {
	const user = "match-dup"
	repo := relayTestRepo(t)
	feed := clashFeed(t,
		proxyJSON("香港", "1.1.1.1", 443),
		proxyJSON("香港", "2.2.2.2", 443),
	)
	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all"}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 deduped row: %v", len(nodes), sortedNames(nodes))
	}
	server, _ := clashOf(t, nodes[0])["server"].(string)
	if server != "1.1.1.1" {
		t.Fatalf("surviving server=%q, want first-write 1.1.1.1", server)
	}
}

func TestMatchRule_NodeName_MatchesDespiteTrafficSuffix(t *testing.T) {
	const user = "match-suffix"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
	old := seedProxyNode(t, repo, user, feed.URL, "香港 12.34GB📊 5Days⏳", "1.1.1.1", 443)

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all", KeepNodeName: false}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes, want 1 rematched row: %v", len(nodes), sortedNames(nodes))
	}
	if nodes[0].ID != old.ID {
		t.Fatalf("id=%d, want %d (suffix should not force recreate)", nodes[0].ID, old.ID)
	}
}

func mustBg() context.Context { return context.Background() }

func TestMatchRule_PreservesDisabledAndCustomTags(t *testing.T) {
	const user = "match-enabled"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 8443))
	node := seedTaggedNode(t, repo, user, feed.URL, "香港", "1.1.1.1", 443, "机场A")
	node.Enabled = false
	node.Tag = "我的分组"
	node.Tags = []string{"我的分组"}
	if _, err := repo.UpdateNode(mustBg(), node); err != nil {
		t.Fatalf("disable node: %v", err)
	}

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all", KeepNodeName: false}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 1 {
		t.Fatalf("got %d nodes: %v", len(nodes), sortedNames(nodes))
	}
	if nodes[0].Enabled {
		t.Fatal("disabled node was re-enabled")
	}
	if nodes[0].Tag != "我的分组" || len(nodes[0].Tags) != 1 || nodes[0].Tags[0] != "我的分组" {
		t.Fatalf("custom tags overwritten: tag=%q tags=%v", nodes[0].Tag, nodes[0].Tags)
	}
	if fmt.Sprintf("%v", clashOf(t, nodes[0])["port"]) != "8443" {
		t.Fatalf("config not updated, port=%v", clashOf(t, nodes[0])["port"])
	}
}

func TestMatchRule_SavedOnlyDoesNotCreateOrDelete(t *testing.T) {
	const user = "match-saved-only"
	repo := relayTestRepo(t)
	oldFeed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443), proxyJSON("日本", "2.2.2.2", 8443))
	newFeed := clashFeed(t, proxyJSON("香港", "9.9.9.9", 443), proxyJSON("新加坡", "8.8.8.8", 443))
	hk := seedProxyNode(t, repo, user, oldFeed.URL, "香港", "1.1.1.1", 443)
	jp := seedProxyNode(t, repo, user, oldFeed.URL, "日本", "2.2.2.2", 8443)

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "saved_only"}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: newFeed.URL}
	// 模拟已改挂到新地址
	if _, err := repo.RebindNodesRawURL(mustBg(), user, oldFeed.URL, newFeed.URL); err != nil {
		t.Fatalf("rebind: %v", err)
	}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	names := sortedNames(nodes)
	if len(names) != 2 || names[0] != "日本" || names[1] != "香港" {
		t.Fatalf("nodes = %v, want [日本 香港]", names)
	}
	var gotHK, gotJP storage.Node
	for _, n := range nodes {
		if n.ID == hk.ID {
			gotHK = n
		}
		if n.ID == jp.ID {
			gotJP = n
		}
	}
	if clashOf(t, gotHK)["server"] != "9.9.9.9" {
		t.Fatalf("香港 not updated: %v", clashOf(t, gotHK)["server"])
	}
	if gotJP.ID != jp.ID {
		t.Fatal("日本 should remain under saved_only")
	}
}

func TestMatchRule_URLChangeCleansStaleAndKeepsOtherSub(t *testing.T) {
	const user = "url-change-user"
	repo := relayTestRepo(t)
	oldFeed := clashFeed(t,
		proxyJSON("香港", "1.1.1.1", 443),
		proxyJSON("日本", "2.2.2.2", 8443),
		proxyJSON("美国", "3.3.3.3", 443),
	)
	newFeed := clashFeed(t,
		proxyJSON("香港", "9.9.9.9", 443),
		proxyJSON("新加坡", "8.8.8.8", 443),
	)
	sub := mustCreateExternalSub(t, repo, user, "千速喵", oldFeed.URL)
	seedTaggedNode(t, repo, user, oldFeed.URL, "香港", "1.1.1.1", 443, "千速喵")
	seedTaggedNode(t, repo, user, oldFeed.URL, "日本", "2.2.2.2", 8443, "千速喵")
	seedTaggedNode(t, repo, user, oldFeed.URL, "美国", "3.3.3.3", 443, "千速喵")
	other := seedTaggedNode(t, repo, user, "https://other.example/sub", "备用", "4.4.4.4", 443, "其他机场")
	mustCreateExternalSub(t, repo, user, "其他机场", "https://other.example/sub")

	sub.URL = newFeed.URL
	if err := repo.UpdateExternalSubscription(mustBg(), sub); err != nil {
		t.Fatalf("update subscription url: %v", err)
	}

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all"}
	runPrefixSync(t, repo, user, sub, settings, false)

	got := nodesByName(t, repo, user)
	if _, ok := got["日本"]; ok {
		t.Fatalf("old-only node 日本 still present")
	}
	if _, ok := got["美国"]; ok {
		t.Fatalf("old-only node 美国 still present")
	}
	if _, ok := got["香港"]; !ok {
		t.Fatal("expected rematched 香港")
	}
	if _, ok := got["新加坡"]; !ok {
		t.Fatal("expected new 新加坡")
	}
	if got["备用"].ID != other.ID {
		t.Fatal("unrelated node 备用 was changed")
	}
}

func TestMatchRule_SyncDeletePrunesRelayGroup(t *testing.T) {
	const user = "match-relay"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443)) // 日本 will disappear
	hk := seedProxyNode(t, repo, user, feed.URL, "香港", "1.1.1.1", 443)
	jp := seedProxyNode(t, repo, user, feed.URL, "日本", "2.2.2.2", 8443)
	source := mustNode(t, repo, storage.Node{
		Username: user, RawURL: "", NodeName: "入口", Protocol: "ss",
		ClashConfig:  proxyJSON("入口", "3.3.3.3", 443),
		ParsedConfig: proxyJSON("入口", "3.3.3.3", 443),
		Enabled:      true, Tag: "手动",
		RelayGroupName:    "中转HK",
		RelayGroupNodeIDs: []int64{hk.ID, jp.ID},
		ChainProxyNodeID:  &jp.ID,
	})

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all"}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL}
	runPrefixSync(t, repo, user, sub, settings, false)

	got, err := repo.GetNode(mustBg(), source.ID, user)
	if err != nil {
		t.Fatalf("get source: %v", err)
	}
	for _, id := range got.RelayGroupNodeIDs {
		if id == jp.ID {
			t.Fatalf("deleted member still in relay group: %v", got.RelayGroupNodeIDs)
		}
	}
	if got.ChainProxyNodeID != nil && *got.ChainProxyNodeID == jp.ID {
		t.Fatal("chain_proxy_node_id still points at deleted node")
	}
}

func TestMatchRule_DefaultTagIsRefreshedCustomIsKept(t *testing.T) {
	if !shouldRefreshSubscriptionTags(storage.Node{}, "机场A") {
		t.Fatal("empty tags should refresh")
	}
	if !shouldRefreshSubscriptionTags(storage.Node{Tag: "机场A", Tags: []string{"机场A"}}, "机场A") {
		t.Fatal("default sub tag should refresh")
	}
	if shouldRefreshSubscriptionTags(storage.Node{Tag: "我的分组", Tags: []string{"我的分组"}}, "机场A") {
		t.Fatal("custom tag should be kept")
	}
}
