package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"miaomiaowu/internal/storage"
)

// issue #113:上游一个端口分配给多个用户(a、b 凭据不同),同一管理员用 a、b 两条外部订阅
// 各自生成订阅。按 type:server:port 匹配时,同步 a 会匹配到 b 的同端口节点并覆盖掉,
// 导致 b 的订阅节点信息(uuid)丢失。新增的 type_server_port_cred 把凭据纳入匹配,应能拦住。
func TestExternalSync_SharedPort_CredMatchProtectsOtherUser(t *testing.T) {
	nodeJSON := func(name, uuid string) string {
		b, _ := json.Marshal(map[string]any{
			"name": name, "type": "vless", "server": "1.2.3.4", "port": 443, "uuid": uuid,
		})
		return string(b)
	}

	// a 的外部订阅内容:同 server:port,uuid=AAAA,节点名更新为 A-new。
	aSub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/yaml")
		_, _ = w.Write([]byte("proxies:\n  - " + nodeJSON("A-new", "AAAA") + "\n"))
	}))
	defer aSub.Close()

	run := func(t *testing.T, matchRule string) ([]storage.Node, []externalSyncCandidate) {
		repo := relayTestRepo(t)
		ctx := context.Background()
		const user = "admin"

		// 真实时序:管理员先同步了 b 的外部订阅(库里只有 b 的节点),此时才第一次同步 a。
		// a 从未同步过,所以此刻库里没有 a 的旧节点 —— a 的新节点只能去和 b 的同端口节点比。
		if _, err := repo.CreateNode(ctx, storage.Node{
			Username: user, RawURL: "http://b.example/sub", NodeName: "B-node", Protocol: "vless",
			ClashConfig: nodeJSON("B-node", "BBBB"), Enabled: true,
		}); err != nil {
			t.Fatalf("seed B: %v", err)
		}

		settings := storage.UserSettings{Username: user, MatchRule: matchRule, SyncScope: "saved_only", TemplateVersion: "v3"}
		if err := repo.UpsertUserSettings(ctx, settings); err != nil {
			t.Fatalf("settings: %v", err)
		}

		sub := storage.ExternalSubscription{Username: user, Name: "A-sub", URL: aSub.URL}
		_, _, candidates, err := syncSingleExternalSubscriptionWithSelection(
			ctx, http.DefaultClient, repo, t.TempDir(), user, sub, settings, true,
		)
		if err != nil {
			t.Fatalf("sync(%s): %v", matchRule, err)
		}

		nodes, err := repo.ListNodes(ctx, user)
		if err != nil {
			t.Fatalf("ListNodes: %v", err)
		}
		return nodes, candidates
	}

	uuidOf := func(n storage.Node) string {
		var m map[string]any
		_ = json.Unmarshal([]byte(n.ClashConfig), &m)
		s, _ := m["uuid"].(string)
		return s
	}
	findByUUID := func(nodes []storage.Node, uuid string) *storage.Node {
		for i := range nodes {
			if uuidOf(nodes[i]) == uuid {
				return &nodes[i]
			}
		}
		return nil
	}

	// 新规则:同步 a 后,b 的节点(uuid=BBBB)必须原样保留;a 的新节点(uuid=AAAA)不匹配 b,
	// 于是作为「待确认的新节点」进入候选列表,而不是覆盖 b。
	t.Run("type_server_port_cred 保护 b", func(t *testing.T) {
		nodes, candidates := run(t, "type_server_port_cred")
		b := findByUUID(nodes, "BBBB")
		if b == nil {
			t.Fatalf("b 的节点(uuid=BBBB)丢失了;nodes=%+v", nodes)
		}
		if b.NodeName != "B-node" {
			t.Errorf("b 的节点被改写: name=%q(应为 B-node)", b.NodeName)
		}
		// a 的新节点应作为候选出现(未匹配到任何已存节点 → 新增而非覆盖)。
		aInCand := false
		for _, c := range candidates {
			if uuidOf(c.node) == "AAAA" {
				aInCand = true
			}
		}
		if !aInCand && findByUUID(nodes, "AAAA") == nil {
			t.Errorf("a 的新节点(uuid=AAAA)既不在候选也不在已存节点里;candidates=%+v", candidates)
		}
	})

	// 对照组:旧规则 type_server_port 下,a 的新节点会匹配到 b 的同端口节点并覆盖之,
	// b(uuid=BBBB)因此丢失 —— 复现原 bug,证明测试命中了脆弱路径。
	t.Run("type_server_port 复现原 bug(b 丢失)", func(t *testing.T) {
		nodes, _ := run(t, "type_server_port")
		if b := findByUUID(nodes, "BBBB"); b != nil {
			t.Errorf("预期复现 bug(b 被覆盖),但 b 仍在 name=%q —— 测试未命中脆弱路径", b.NodeName)
		}
	})
}
