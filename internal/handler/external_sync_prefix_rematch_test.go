package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"testing"

	"miaomiaowu/internal/storage"
)

func proxyJSON(name, server string, port int) string {
	b, _ := json.Marshal(map[string]any{
		"name": name, "type": "ss", "server": server, "port": port,
		"cipher": "aes-256-gcm", "password": "pw",
	})
	return string(b)
}

func clashFeed(t *testing.T, proxies ...string) *httptest.Server {
	t.Helper()
	body := "proxies:\n"
	for _, p := range proxies {
		body += "  - " + p + "\n"
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func seedProxyNode(t *testing.T, repo *storage.TrafficRepository, user, rawURL, name, server string, port int) storage.Node {
	t.Helper()
	cfg := proxyJSON(name, server, port)
	return mustNode(t, repo, storage.Node{
		Username: user, RawURL: rawURL, NodeName: name, Protocol: "ss",
		ClashConfig: cfg, ParsedConfig: cfg, Enabled: true, Tag: "机场",
	})
}

func listUserNodes(t *testing.T, repo *storage.TrafficRepository, user string) []storage.Node {
	t.Helper()
	nodes, err := repo.ListNodes(context.Background(), user)
	if err != nil {
		t.Fatalf("list nodes: %v", err)
	}
	return nodes
}

func sortedNames(nodes []storage.Node) []string {
	out := make([]string, len(nodes))
	for i, node := range nodes {
		out[i] = node.NodeName
	}
	sort.Strings(out)
	return out
}

func clashNameOf(t *testing.T, node storage.Node) string {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.ClashConfig), &cfg); err != nil {
		t.Fatalf("clash config: %v", err)
	}
	name, _ := cfg["name"].(string)
	return name
}

func runPrefixSync(t *testing.T, repo *storage.TrafficRepository, user string, sub storage.ExternalSubscription, settings storage.UserSettings, deferNew bool) (storage.ExternalSubscription, []externalSyncCandidate) {
	t.Helper()
	_, updated, candidates, err := syncSingleExternalSubscriptionWithSelection(
		context.Background(), http.DefaultClient, repo, t.TempDir(), user, sub, settings, deferNew,
	)
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	return updated, candidates
}

// 前缀只是展示层改名。匹配看机场原始名是否完全相同：空→前缀、A→B、A→空都更新原来那一条，不另插一条。
func TestExternalSync_PrefixTransitionRematch(t *testing.T) {
	const user = "prefix-user"
	tests := []struct {
		name     string
		applied  string
		desired  string
		existing []string
		want     []string
	}{
		{
			name:     "unprefixed plus new prefix renames",
			applied:  "",
			desired:  "〖机场A〗",
			existing: []string{"香港", "日本"},
			want:     []string{"〖机场A〗日本", "〖机场A〗香港"},
		},
		{
			name:     "prefix A to prefix B",
			applied:  "【aa】",
			desired:  "[bb]",
			existing: []string{"【aa】香港", "【aa】日本"},
			want:     []string{"[bb]日本", "[bb]香港"},
		},
		{
			name:     "prefix to empty",
			applied:  "【aa】",
			desired:  "",
			existing: []string{"【aa】香港", "【aa】日本"},
			want:     []string{"日本", "香港"},
		},
		{
			name:     "same prefix is idempotent",
			applied:  "〖机场A〗",
			desired:  "〖机场A〗",
			existing: []string{"〖机场A〗香港", "〖机场A〗日本"},
			want:     []string{"〖机场A〗日本", "〖机场A〗香港"},
		},
		{
			name:     "empty prefix leaves names unchanged",
			applied:  "",
			desired:  "",
			existing: []string{"香港", "日本"},
			want:     []string{"日本", "香港"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := relayTestRepo(t)
			feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443), proxyJSON("日本", "2.2.2.2", 8443))
			seeded := make([]storage.Node, 0, len(tt.existing))
			for i, name := range tt.existing {
				server, port := "1.1.1.1", 443
				if i == 1 {
					server, port = "2.2.2.2", 8443
				}
				seeded = append(seeded, seedProxyNode(t, repo, user, feed.URL, name, server, port))
			}

			settings := storage.UserSettings{
				Username: user, MatchRule: "node_name", SyncScope: "saved_only", KeepNodeName: true,
			}
			sub := storage.ExternalSubscription{
				Username: user, Name: "机场A", URL: feed.URL,
				NamePrefix: tt.desired, AppliedNamePrefix: tt.applied,
			}
			// 手动同步会把「没对上的节点」放进待确认列表；对上了就不能再出现一条。
			updated, candidates := runPrefixSync(t, repo, user, sub, settings, true)
			if len(candidates) != 0 {
				t.Fatalf("candidates = %+v, want none", candidates)
			}
			if updated.AppliedNamePrefix != tt.desired {
				t.Fatalf("applied prefix = %q, want %q", updated.AppliedNamePrefix, tt.desired)
			}

			nodes := listUserNodes(t, repo, user)
			if got := sortedNames(nodes); len(got) != len(tt.want) || !sameStrings(got, tt.want) {
				t.Fatalf("names = %q, want %q", got, tt.want)
			}
			ids := map[int64]bool{}
			for _, node := range nodes {
				ids[node.ID] = true
				if clashNameOf(t, node) != node.NodeName {
					t.Fatalf("clash name %q != node name %q", clashNameOf(t, node), node.NodeName)
				}
			}
			for _, seed := range seeded {
				if !ids[seed.ID] {
					t.Fatalf("node %d (%s) was replaced instead of updated", seed.ID, seed.NodeName)
				}
			}

			// 再用同步返回的已应用前缀跑一遍，名字不能叠成前缀前缀。
			updated, candidates = runPrefixSync(t, repo, user, updated, settings, true)
			if len(candidates) != 0 {
				t.Fatalf("second sync candidates = %+v", candidates)
			}
			nodes = listUserNodes(t, repo, user)
			if got := sortedNames(nodes); !sameStrings(got, tt.want) {
				t.Fatalf("second sync names = %q, want %q", got, tt.want)
			}
			if updated.AppliedNamePrefix != tt.desired {
				t.Fatalf("second sync applied prefix = %q", updated.AppliedNamePrefix)
			}
		})
	}
}

func TestExternalSync_PrefixRematchDoesNotCollideSimilarNames(t *testing.T) {
	const user = "prefix-user"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443), proxyJSON("香港1", "2.2.2.2", 8443))
	seedProxyNode(t, repo, user, feed.URL, "香港", "1.1.1.1", 443)
	seedProxyNode(t, repo, user, feed.URL, "香港1", "2.2.2.2", 8443)

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "saved_only", KeepNodeName: true}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL, NamePrefix: "〖机场A〗"}
	updated, candidates := runPrefixSync(t, repo, user, sub, settings, true)
	if len(candidates) != 0 {
		t.Fatalf("candidates = %+v", candidates)
	}
	nodes := listUserNodes(t, repo, user)
	want := []string{"〖机场A〗香港", "〖机场A〗香港1"}
	if got := sortedNames(nodes); !sameStrings(got, want) {
		t.Fatalf("names = %q, want %q", got, want)
	}
	if updated.AppliedNamePrefix != "〖机场A〗" {
		t.Fatalf("applied = %q", updated.AppliedNamePrefix)
	}
}

func TestExternalSync_PrefixDuplicateKeepsOlderNode(t *testing.T) {
	const user = "prefix-user"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
	older := seedProxyNode(t, repo, user, feed.URL, "香港", "1.1.1.1", 443)
	newer := seedProxyNode(t, repo, user, feed.URL, "〖机场A〗香港", "1.1.1.1", 443)

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "saved_only", KeepNodeName: true}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL, NamePrefix: "〖机场A〗"}
	if _, candidates := runPrefixSync(t, repo, user, sub, settings, true); len(candidates) != 0 {
		t.Fatalf("candidates = %+v", candidates)
	}
	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 1 {
		t.Fatalf("nodes = %q, want one", sortedNames(nodes))
	}
	if nodes[0].ID != older.ID {
		t.Fatalf("kept id %d, want older %d (newer was %d)", nodes[0].ID, older.ID, newer.ID)
	}
	if nodes[0].NodeName != "〖机场A〗香港" {
		t.Fatalf("name = %q", nodes[0].NodeName)
	}
}

func TestExternalSync_PrefixRematchIgnoresOtherSubscription(t *testing.T) {
	const user = "prefix-user"
	repo := relayTestRepo(t)
	feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
	ours := seedProxyNode(t, repo, user, feed.URL, "香港", "1.1.1.1", 443)
	other := seedProxyNode(t, repo, user, "https://other.example/sub", "香港", "9.9.9.9", 443)

	settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "all", KeepNodeName: true}
	sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL, NamePrefix: "〖机场A〗"}
	if _, candidates := runPrefixSync(t, repo, user, sub, settings, false); len(candidates) != 0 {
		t.Fatalf("candidates = %+v", candidates)
	}
	nodes := listUserNodes(t, repo, user)
	if len(nodes) != 2 {
		t.Fatalf("nodes = %+v", nodes)
	}
	for _, node := range nodes {
		switch node.ID {
		case ours.ID:
			if node.NodeName != "〖机场A〗香港" {
				t.Fatalf("our node name = %q", node.NodeName)
			}
		case other.ID:
			if node.NodeName != "香港" {
				t.Fatalf("other subscription node renamed to %q", node.NodeName)
			}
		default:
			t.Fatalf("unexpected node %d %q", node.ID, node.NodeName)
		}
	}
}

func TestExternalSync_KeepNodeNameAndPrefix(t *testing.T) {
	const user = "prefix-user"

	t.Run("server port keeps a truly custom name", func(t *testing.T) {
		repo := relayTestRepo(t)
		feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
		custom := seedProxyNode(t, repo, user, feed.URL, "我的香港", "1.1.1.1", 443)
		settings := storage.UserSettings{Username: user, MatchRule: "server_port", SyncScope: "all", KeepNodeName: true}
		sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL, NamePrefix: "〖机场A〗"}
		if _, candidates := runPrefixSync(t, repo, user, sub, settings, false); len(candidates) != 0 {
			t.Fatalf("candidates = %+v", candidates)
		}
		nodes := listUserNodes(t, repo, user)
		if len(nodes) != 1 || nodes[0].ID != custom.ID || nodes[0].NodeName != "我的香港" {
			t.Fatalf("nodes = %+v", nodes)
		}
		if clashNameOf(t, nodes[0]) != "我的香港" {
			t.Fatalf("clash name = %q", clashNameOf(t, nodes[0]))
		}
	})

	t.Run("server port without KeepNodeName adopts the prefix", func(t *testing.T) {
		repo := relayTestRepo(t)
		feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
		custom := seedProxyNode(t, repo, user, feed.URL, "我的香港", "1.1.1.1", 443)
		settings := storage.UserSettings{Username: user, MatchRule: "server_port", SyncScope: "all", KeepNodeName: false}
		sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL, NamePrefix: "〖机场A〗"}
		runPrefixSync(t, repo, user, sub, settings, false)
		nodes := listUserNodes(t, repo, user)
		if len(nodes) != 1 || nodes[0].ID != custom.ID || nodes[0].NodeName != "〖机场A〗香港" {
			t.Fatalf("nodes = %+v", nodes)
		}
	})

	t.Run("node name match does not treat a custom name as the same node", func(t *testing.T) {
		repo := relayTestRepo(t)
		feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443))
		custom := seedProxyNode(t, repo, user, feed.URL, "我的香港", "1.1.1.1", 443)
		settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "saved_only", KeepNodeName: true}
		sub := storage.ExternalSubscription{Username: user, Name: "机场A", URL: feed.URL, NamePrefix: "〖机场A〗"}
		_, candidates := runPrefixSync(t, repo, user, sub, settings, true)
		nodes := listUserNodes(t, repo, user)
		if len(nodes) != 1 || nodes[0].ID != custom.ID || nodes[0].NodeName != "我的香港" {
			t.Fatalf("nodes = %+v", nodes)
		}
		if len(candidates) != 1 || candidates[0].Name != "〖机场A〗香港" {
			t.Fatalf("candidates = %+v", candidates)
		}
	})
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
