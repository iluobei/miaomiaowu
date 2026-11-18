package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"testing"

	"miaomiaowu/internal/storage"
)

func seedTaggedNode(t *testing.T, repo *storage.TrafficRepository, user, rawURL, name, server string, port int, tag string) storage.Node {
	t.Helper()
	node := seedProxyNode(t, repo, user, rawURL, name, server, port)
	node.Tag = tag
	node.Tags = []string{tag}
	updated, err := repo.UpdateNode(context.Background(), node)
	if err != nil {
		t.Fatalf("tag node %s: %v", name, err)
	}
	return updated
}

func mustCreateExternalSub(t *testing.T, repo *storage.TrafficRepository, user, name, rawURL string) storage.ExternalSubscription {
	t.Helper()
	id, err := repo.CreateExternalSubscription(context.Background(), storage.ExternalSubscription{
		Username: user, Name: name, URL: rawURL,
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	sub, err := repo.GetExternalSubscription(context.Background(), id, user)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	return sub
}

// 复现 issue #136：订阅已改成新地址，但节点还挂在旧 raw_url 上（当前线上用户的数据形态）。
// 全量同步应删除旧源独有节点、更新同名节点、创建新节点，且不影响其它订阅。
func TestExternalSync_URLChangeRemovesStaleNodes(t *testing.T) {
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
	seedTaggedNode(t, repo, user, "https://other.example/sub", "备用", "4.4.4.4", 443, "其他机场")
	mustCreateExternalSub(t, repo, user, "其他机场", "https://other.example/sub")

	// 模拟用户已经在 UI 里改掉了订阅 URL，但还没把节点 raw_url 改过去。
	sub.URL = newFeed.URL
	if err := repo.UpdateExternalSubscription(context.Background(), sub); err != nil {
		t.Fatalf("update subscription url: %v", err)
	}

	settings := storage.UserSettings{
		Username: user, MatchRule: "node_name", SyncScope: "all",
	}
	runPrefixSync(t, repo, user, sub, settings, false)

	nodes := listUserNodes(t, repo, user)
	got := map[string]storage.Node{}
	for _, n := range nodes {
		got[n.NodeName] = n
	}

	if _, ok := got["日本"]; ok {
		t.Fatalf("old-only node 日本 still present: %+v", got["日本"])
	}
	if _, ok := got["美国"]; ok {
		t.Fatalf("old-only node 美国 still present: %+v", got["美国"])
	}
	hk, ok := got["香港"]
	if !ok {
		t.Fatal("expected rematched node 香港")
	}
	if hk.RawURL != newFeed.URL {
		t.Fatalf("香港 raw_url = %q, want new feed %q", hk.RawURL, newFeed.URL)
	}
	sg, ok := got["新加坡"]
	if !ok {
		t.Fatal("expected newly created node 新加坡")
	}
	if sg.RawURL != newFeed.URL {
		t.Fatalf("新加坡 raw_url = %q, want new feed %q", sg.RawURL, newFeed.URL)
	}
	if _, ok := got["备用"]; !ok {
		t.Fatal("unrelated subscription node 备用 was deleted")
	}
	if len(got) != 3 {
		t.Fatalf("nodes = %v, want 香港, 新加坡, 备用", sortedNames(nodes))
	}
}

func TestExternalSync_URLChangeSavedOnlyKeepsUnmatched(t *testing.T) {
	const user = "url-change-saved-only"
	repo := relayTestRepo(t)
	oldFeed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443), proxyJSON("日本", "2.2.2.2", 8443))
	newFeed := clashFeed(t, proxyJSON("香港", "9.9.9.9", 443), proxyJSON("新加坡", "8.8.8.8", 443))

	seedProxyNode(t, repo, user, oldFeed.URL, "香港", "1.1.1.1", 443)
	seedProxyNode(t, repo, user, oldFeed.URL, "日本", "2.2.2.2", 8443)

	settings := storage.UserSettings{
		Username: user, MatchRule: "node_name", SyncScope: "saved_only",
	}
	sub := storage.ExternalSubscription{
		Username: user, Name: "千速喵", URL: newFeed.URL,
	}
	runPrefixSync(t, repo, user, sub, settings, false)

	names := sortedNames(listUserNodes(t, repo, user))
	// saved_only：不新建、不删除，同名节点可以改写 raw_url。
	if len(names) != 2 || names[0] != "日本" || names[1] != "香港" {
		t.Fatalf("nodes = %v, want [日本 香港]", names)
	}
}

func TestUpdateExternalSubscriptionRebindsNodeRawURL(t *testing.T) {
	const user = "rebind-user"
	repo := relayTestRepo(t)
	ctx := context.Background()
	if err := repo.CreateUser(ctx, user, "", "", "x", "user", ""); err != nil {
		t.Fatalf("create user: %v", err)
	}

	oldURL := "http://127.0.0.1:1/old"
	newURL := "http://127.0.0.1:1/new"
	sub := mustCreateExternalSub(t, repo, user, "千速喵", oldURL)
	seedTaggedNode(t, repo, user, oldURL, "香港", "1.1.1.1", 443, "千速喵")
	seedTaggedNode(t, repo, user, "https://other.example/sub", "备用", "4.4.4.4", 443, "其他机场")

	h := NewExternalSubscriptionsHandler(repo, t.TempDir())
	body, _ := json.Marshal(map[string]any{"name": "千速喵", "url": newURL})
	rec := serveExternalSub(t, h, http.MethodPut, "/api/user/external-subscriptions?id="+strconv.FormatInt(sub.ID, 10), string(body), user)
	if rec.Code != http.StatusOK {
		t.Fatalf("update status %d body %s", rec.Code, rec.Body.String())
	}

	nodes := listUserNodes(t, repo, user)
	got := map[string]storage.Node{}
	for _, n := range nodes {
		got[n.NodeName] = n
	}
	if got["香港"].RawURL != newURL {
		t.Fatalf("香港 raw_url = %q, want %q", got["香港"].RawURL, newURL)
	}
	if got["备用"].RawURL != "https://other.example/sub" {
		t.Fatalf("备用 raw_url was changed to %q", got["备用"].RawURL)
	}
}
