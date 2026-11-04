package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"miaomiaowu/internal/storage"

	"gopkg.in/yaml.v3"
)

func TestPlanExternalSubscriptionPrefixRenames(t *testing.T) {
	const subURL = "https://airport.example/sub"

	t.Run("prefix A to B keeps custom and similar names apart", func(t *testing.T) {
		nodes := []storage.Node{
			{ID: 1, RawURL: subURL, NodeName: "【aa】香港"},
			{ID: 2, RawURL: subURL, NodeName: "【aa】香港1"},
			{ID: 3, RawURL: subURL, NodeName: "我的香港"},
			{ID: 4, RawURL: "https://other.example/sub", NodeName: "【aa】日本"},
		}
		groups := planExternalSubscriptionPrefixRenames(nodes, subURL, "【aa】", "[bb]")
		got := map[int64]string{}
		for _, group := range groups {
			got[group.keeper.ID] = group.newName
			if len(group.deleteIDs) != 0 {
				t.Fatalf("group %d deletes %+v", group.keeper.ID, group.deleteIDs)
			}
		}
		if len(got) != 2 || got[1] != "[bb]香港" || got[2] != "[bb]香港1" {
			t.Fatalf("groups = %+v", got)
		}
	})

	t.Run("clear collapses bare and prefixed forms onto the older node", func(t *testing.T) {
		nodes := []storage.Node{
			{ID: 5, RawURL: subURL, NodeName: "【aa】香港"},
			{ID: 2, RawURL: subURL, NodeName: "香港"},
			{ID: 9, RawURL: subURL, NodeName: "我的香港"},
		}
		groups := planExternalSubscriptionPrefixRenames(nodes, subURL, "【aa】", "")
		if len(groups) != 1 {
			t.Fatalf("groups = %+v", groups)
		}
		if groups[0].keeper.ID != 2 || groups[0].newName != "香港" || len(groups[0].deleteIDs) != 1 || groups[0].deleteIDs[0] != 5 {
			t.Fatalf("group = %+v", groups[0])
		}
	})

	t.Run("empty applied prefix renames bare names without stacking", func(t *testing.T) {
		nodes := []storage.Node{
			{ID: 1, RawURL: subURL, NodeName: "香港"},
			{ID: 4, RawURL: subURL, NodeName: "〖机场A〗香港"},
			{ID: 3, RawURL: subURL, NodeName: "日本"},
		}
		groups := planExternalSubscriptionPrefixRenames(nodes, subURL, "", "〖机场A〗")
		byID := map[int64]subscriptionPrefixGroup{}
		for _, group := range groups {
			byID[group.keeper.ID] = group
		}
		hk, ok := byID[1]
		if !ok || hk.newName != "〖机场A〗香港" || len(hk.deleteIDs) != 1 || hk.deleteIDs[0] != 4 {
			t.Fatalf("hong kong group = %+v", hk)
		}
		jp, ok := byID[3]
		if !ok || jp.newName != "〖机场A〗日本" || len(jp.deleteIDs) != 0 {
			t.Fatalf("japan group = %+v", jp)
		}
		if len(byID) != 2 {
			t.Fatalf("groups = %+v", byID)
		}
	})

	t.Run("unchanged prefix is a no-op", func(t *testing.T) {
		nodes := []storage.Node{{ID: 1, RawURL: subURL, NodeName: "【aa】香港"}}
		if groups := planExternalSubscriptionPrefixRenames(nodes, subURL, "【aa】", "【aa】"); len(groups) != 0 {
			t.Fatalf("groups = %+v", groups)
		}
	})
}

func TestSaveExternalSubscriptionPrefixRenamesNodes(t *testing.T) {
	const user = "prefix-save"

	t.Run("A to B renames immediately and sync does not stack", func(t *testing.T) {
		repo := relayTestRepo(t)
		if err := repo.CreateUser(context.Background(), user, "", "", "x", "user", ""); err != nil {
			t.Fatalf("create user: %v", err)
		}
		feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443), proxyJSON("日本", "2.2.2.2", 8443))
		sub := mustExternalSub(t, repo, user, feed.URL, "【aa】", "【aa】")
		hk := seedProxyNode(t, repo, user, feed.URL, "【aa】香港", "1.1.1.1", 443)
		jp := seedProxyNode(t, repo, user, feed.URL, "【aa】日本", "2.2.2.2", 8443)
		custom := seedProxyNode(t, repo, user, feed.URL, "我的香港", "3.3.3.3", 1000)
		other := seedProxyNode(t, repo, user, "https://other.example/sub", "【aa】新加坡", "9.9.9.9", 443)

		h := NewExternalSubscriptionsHandler(repo, "")
		putExternalPrefix(t, h, sub.ID, feed.URL, "[bb]")

		stored := mustGetExternalSub(t, repo, sub.ID, user)
		if stored.NamePrefix != "[bb]" || stored.AppliedNamePrefix != "[bb]" {
			t.Fatalf("prefix name %q applied %q", stored.NamePrefix, stored.AppliedNamePrefix)
		}
		nodes := listUserNodes(t, repo, user)
		byID := indexNodes(nodes)
		assertNodeName(t, byID, hk.ID, "[bb]香港")
		assertNodeName(t, byID, jp.ID, "[bb]日本")
		assertNodeName(t, byID, custom.ID, "我的香港")
		assertNodeName(t, byID, other.ID, "【aa】新加坡")
		if clashNameOf(t, byID[hk.ID]) != "[bb]香港" || parsedNameOf(t, byID[hk.ID]) != "[bb]香港" {
			t.Fatalf("config name clash %q parsed %q", clashNameOf(t, byID[hk.ID]), parsedNameOf(t, byID[hk.ID]))
		}

		settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "saved_only", KeepNodeName: true}
		updated, candidates := runPrefixSync(t, repo, user, stored, settings, true)
		if len(candidates) != 0 {
			t.Fatalf("candidates = %+v", candidates)
		}
		if updated.AppliedNamePrefix != "[bb]" {
			t.Fatalf("sync applied = %q", updated.AppliedNamePrefix)
		}
		if got := sortedNames(listUserNodes(t, repo, user)); !sameStrings(got, []string{"[bb]日本", "[bb]香港", "【aa】新加坡", "我的香港"}) {
			t.Fatalf("after sync names = %q", got)
		}
		updated, candidates = runPrefixSync(t, repo, user, updated, settings, true)
		if len(candidates) != 0 || updated.AppliedNamePrefix != "[bb]" {
			t.Fatalf("second sync applied %q candidates %+v", updated.AppliedNamePrefix, candidates)
		}
		if got := sortedNames(listUserNodes(t, repo, user)); !sameStrings(got, []string{"[bb]日本", "[bb]香港", "【aa】新加坡", "我的香港"}) {
			t.Fatalf("second sync names = %q", got)
		}
	})

	t.Run("empty prefix clears managed names and duplicates", func(t *testing.T) {
		repo := relayTestRepo(t)
		if err := repo.CreateUser(context.Background(), user, "", "", "x", "user", ""); err != nil {
			t.Fatalf("create user: %v", err)
		}
		const subURL = "https://airport.example/a"
		sub := mustExternalSub(t, repo, user, subURL, "【aa】", "【aa】")
		bare := seedProxyNode(t, repo, user, subURL, "香港", "1.1.1.1", 443)
		prefixed := seedProxyNode(t, repo, user, subURL, "【aa】香港", "1.1.1.1", 443)
		jp := seedProxyNode(t, repo, user, subURL, "【aa】日本", "2.2.2.2", 8443)
		custom := seedProxyNode(t, repo, user, subURL, "我的香港", "3.3.3.3", 1000)

		h := NewExternalSubscriptionsHandler(repo, "")
		putExternalPrefix(t, h, sub.ID, subURL, "")

		stored := mustGetExternalSub(t, repo, sub.ID, user)
		if stored.NamePrefix != "" || stored.AppliedNamePrefix != "" {
			t.Fatalf("prefix name %q applied %q", stored.NamePrefix, stored.AppliedNamePrefix)
		}
		nodes := listUserNodes(t, repo, user)
		if got := sortedNames(nodes); !sameStrings(got, []string{"我的香港", "日本", "香港"}) {
			t.Fatalf("names = %q", got)
		}
		byID := indexNodes(nodes)
		if _, ok := byID[prefixed.ID]; ok {
			t.Fatalf("prefixed duplicate %d still exists", prefixed.ID)
		}
		assertNodeName(t, byID, bare.ID, "香港")
		assertNodeName(t, byID, jp.ID, "日本")
		assertNodeName(t, byID, custom.ID, "我的香港")
		if clashNameOf(t, byID[jp.ID]) != "日本" {
			t.Fatalf("clash name = %q", clashNameOf(t, byID[jp.ID]))
		}
	})

	t.Run("empty to prefix renames bare names and sync does not stack", func(t *testing.T) {
		repo := relayTestRepo(t)
		if err := repo.CreateUser(context.Background(), user, "", "", "x", "user", ""); err != nil {
			t.Fatalf("create user: %v", err)
		}
		feed := clashFeed(t, proxyJSON("香港", "1.1.1.1", 443), proxyJSON("日本", "2.2.2.2", 8443))
		sub := mustExternalSub(t, repo, user, feed.URL, "", "")
		hk := seedProxyNode(t, repo, user, feed.URL, "香港", "1.1.1.1", 443)
		jp := seedProxyNode(t, repo, user, feed.URL, "日本", "2.2.2.2", 8443)

		h := NewExternalSubscriptionsHandler(repo, "")
		putExternalPrefix(t, h, sub.ID, feed.URL, "〖机场A〗")

		stored := mustGetExternalSub(t, repo, sub.ID, user)
		if stored.AppliedNamePrefix != "〖机场A〗" || stored.NamePrefix != "〖机场A〗" {
			t.Fatalf("prefix name %q applied %q", stored.NamePrefix, stored.AppliedNamePrefix)
		}
		byID := indexNodes(listUserNodes(t, repo, user))
		assertNodeName(t, byID, hk.ID, "〖机场A〗香港")
		assertNodeName(t, byID, jp.ID, "〖机场A〗日本")

		settings := storage.UserSettings{Username: user, MatchRule: "node_name", SyncScope: "saved_only", KeepNodeName: true}
		updated, candidates := runPrefixSync(t, repo, user, stored, settings, true)
		if len(candidates) != 0 || updated.AppliedNamePrefix != "〖机场A〗" {
			t.Fatalf("sync applied %q candidates %+v", updated.AppliedNamePrefix, candidates)
		}
		byID = indexNodes(listUserNodes(t, repo, user))
		assertNodeName(t, byID, hk.ID, "〖机场A〗香港")
		assertNodeName(t, byID, jp.ID, "〖机场A〗日本")
	})

	t.Run("unchanged or omitted prefix does not rename", func(t *testing.T) {
		repo := relayTestRepo(t)
		if err := repo.CreateUser(context.Background(), user, "", "", "x", "user", ""); err != nil {
			t.Fatalf("create user: %v", err)
		}
		const subURL = "https://airport.example/a"
		sub := mustExternalSub(t, repo, user, subURL, "[bb]", "【aa】")
		node := seedProxyNode(t, repo, user, subURL, "【aa】香港", "1.1.1.1", 443)
		h := NewExternalSubscriptionsHandler(repo, "")

		putExternalPrefix(t, h, sub.ID, subURL, "[bb]")
		stored := mustGetExternalSub(t, repo, sub.ID, user)
		if stored.AppliedNamePrefix != "【aa】" {
			t.Fatalf("applied changed on no-op save: %q", stored.AppliedNamePrefix)
		}
		assertNodeName(t, indexNodes(listUserNodes(t, repo, user)), node.ID, "【aa】香港")

		body, err := json.Marshal(map[string]any{"name": "机场A", "url": subURL, "traffic_mode": "download"})
		if err != nil {
			t.Fatal(err)
		}
		rec := serveExternalSub(t, h, http.MethodPut, externalSubURL(sub.ID), string(body), user)
		if rec.Code != http.StatusOK {
			t.Fatalf("omit status %d body %s", rec.Code, rec.Body.String())
		}
		stored = mustGetExternalSub(t, repo, sub.ID, user)
		if stored.NamePrefix != "[bb]" || stored.AppliedNamePrefix != "【aa】" {
			t.Fatalf("omit prefix name %q applied %q", stored.NamePrefix, stored.AppliedNamePrefix)
		}
		assertNodeName(t, indexNodes(listUserNodes(t, repo, user)), node.ID, "【aa】香港")
	})

	t.Run("yaml display names follow the saved prefix", func(t *testing.T) {
		repo := relayTestRepo(t)
		if err := repo.CreateUser(context.Background(), user, "", "", "x", "user", ""); err != nil {
			t.Fatalf("create user: %v", err)
		}
		const subURL = "https://airport.example/a"
		sub := mustExternalSub(t, repo, user, subURL, "【aa】", "【aa】")
		seedProxyNode(t, repo, user, subURL, "【aa】香港", "1.1.1.1", 443)

		dir := t.TempDir()
		path := filepath.Join(dir, "sub.yaml")
		const fixture = `proxies:
  - name: 【aa】香港
    type: ss
    server: 1.1.1.1
    port: 443
    cipher: aes-256-gcm
    password: pw
proxy-groups:
  - name: PROXY
    type: select
    proxies:
      - 【aa】香港
`
		if err := os.WriteFile(path, []byte(fixture), 0o644); err != nil {
			t.Fatal(err)
		}

		h := NewExternalSubscriptionsHandler(repo, dir)
		putExternalPrefix(t, h, sub.ID, subURL, "[bb]")

		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var doc struct {
			Proxies []struct {
				Name string `yaml:"name"`
			} `yaml:"proxies"`
			Groups []struct {
				Proxies []string `yaml:"proxies"`
			} `yaml:"proxy-groups"`
		}
		if err := yaml.Unmarshal(data, &doc); err != nil {
			t.Fatalf("yaml: %v\n%s", err, data)
		}
		if len(doc.Proxies) != 1 || doc.Proxies[0].Name != "[bb]香港" {
			t.Fatalf("proxies = %+v\n%s", doc.Proxies, data)
		}
		if len(doc.Groups) != 1 || len(doc.Groups[0].Proxies) != 1 || doc.Groups[0].Proxies[0] != "[bb]香港" {
			t.Fatalf("groups = %+v\n%s", doc.Groups, data)
		}
		if stored := mustGetExternalSub(t, repo, sub.ID, user); stored.AppliedNamePrefix != "[bb]" {
			t.Fatalf("applied = %q", stored.AppliedNamePrefix)
		}
	})
}

func mustExternalSub(t *testing.T, repo *storage.TrafficRepository, user, subURL, namePrefix, applied string) storage.ExternalSubscription {
	t.Helper()
	id, err := repo.CreateExternalSubscription(context.Background(), storage.ExternalSubscription{
		Username: user, Name: "机场A", URL: subURL,
		NamePrefix: namePrefix, AppliedNamePrefix: applied,
	})
	if err != nil {
		t.Fatalf("create subscription: %v", err)
	}
	return mustGetExternalSub(t, repo, id, user)
}

func mustGetExternalSub(t *testing.T, repo *storage.TrafficRepository, id int64, user string) storage.ExternalSubscription {
	t.Helper()
	got, err := repo.GetExternalSubscription(context.Background(), id, user)
	if err != nil {
		t.Fatalf("get subscription: %v", err)
	}
	return got
}

func putExternalPrefix(t *testing.T, h http.Handler, id int64, subURL, prefix string) {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"name": "机场A", "url": subURL, "name_prefix": prefix,
	})
	if err != nil {
		t.Fatal(err)
	}
	rec := serveExternalSub(t, h, http.MethodPut, externalSubURL(id), string(body), "prefix-save")
	if rec.Code != http.StatusOK {
		t.Fatalf("update status %d body %s", rec.Code, rec.Body.String())
	}
}

func externalSubURL(id int64) string {
	return "/api/user/external-subscriptions?id=" + strconv.FormatInt(id, 10)
}

func indexNodes(nodes []storage.Node) map[int64]storage.Node {
	out := make(map[int64]storage.Node, len(nodes))
	for _, node := range nodes {
		out[node.ID] = node
	}
	return out
}

func assertNodeName(t *testing.T, byID map[int64]storage.Node, id int64, want string) {
	t.Helper()
	node, ok := byID[id]
	if !ok {
		t.Fatalf("node %d missing", id)
	}
	if node.NodeName != want {
		t.Fatalf("node %d name = %q, want %q", id, node.NodeName, want)
	}
}

func parsedNameOf(t *testing.T, node storage.Node) string {
	t.Helper()
	var cfg map[string]any
	if err := json.Unmarshal([]byte(node.ParsedConfig), &cfg); err != nil {
		t.Fatalf("parsed config: %v", err)
	}
	name, _ := cfg["name"].(string)
	return name
}
