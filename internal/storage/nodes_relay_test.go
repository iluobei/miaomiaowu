package storage

import (
	"context"
	"path/filepath"
	"testing"
)

// newTestRepo creates a TrafficRepository backed by a fresh temp SQLite DB.
func newTestRepo(t *testing.T) *TrafficRepository {
	t.Helper()
	repo, err := NewTrafficRepository(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewTrafficRepository: %v", err)
	}
	return repo
}

func mustCreateNode(t *testing.T, repo *TrafficRepository, n Node) Node {
	t.Helper()
	created, err := repo.CreateNode(context.Background(), n)
	if err != nil {
		t.Fatalf("CreateNode(%s): %v", n.NodeName, err)
	}
	return created
}

// TestDeleteNode_PrunesRelayGroupMember verifies that deleting a relay-group
// member removes it from the source node's relay_group_node_ids, and that once
// all members are gone the source node's relay-group config is fully cleared.
func TestDeleteNode_PrunesRelayGroupMember(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	const user = "admin"

	memberA := mustCreateNode(t, repo, Node{
		Username: user, NodeName: "Member-A", Protocol: "trojan",
		ClashConfig: `{"name":"Member-A","type":"trojan","server":"a.example.com","port":443}`,
		Enabled:     true,
	})
	memberB := mustCreateNode(t, repo, Node{
		Username: user, NodeName: "Member-B", Protocol: "trojan",
		ClashConfig: `{"name":"Member-B","type":"trojan","server":"b.example.com","port":443}`,
		Enabled:     true,
	})
	source := mustCreateNode(t, repo, Node{
		Username: user, NodeName: "Source", Protocol: "vmess",
		ClashConfig:       `{"name":"Source","type":"vmess","server":"s.example.com","port":443}`,
		Enabled:           true,
		RelayGroupName:    "中转HK",
		RelayGroupNodeIDs: []int64{memberA.ID, memberB.ID},
	})

	// Delete one member → source keeps the group, member removed from the list.
	if err := repo.DeleteNode(ctx, memberA.ID, user); err != nil {
		t.Fatalf("DeleteNode(memberA): %v", err)
	}
	got, err := repo.GetNode(ctx, source.ID, user)
	if err != nil {
		t.Fatalf("GetNode(source): %v", err)
	}
	if got.RelayGroupName != "中转HK" {
		t.Errorf("after deleting one member, RelayGroupName = %q, want %q", got.RelayGroupName, "中转HK")
	}
	if len(got.RelayGroupNodeIDs) != 1 || got.RelayGroupNodeIDs[0] != memberB.ID {
		t.Errorf("RelayGroupNodeIDs = %v, want [%d]", got.RelayGroupNodeIDs, memberB.ID)
	}

	// Delete the last member → source's relay-group config is fully cleared.
	if err := repo.DeleteNode(ctx, memberB.ID, user); err != nil {
		t.Fatalf("DeleteNode(memberB): %v", err)
	}
	got, err = repo.GetNode(ctx, source.ID, user)
	if err != nil {
		t.Fatalf("GetNode(source): %v", err)
	}
	if got.RelayGroupName != "" {
		t.Errorf("after deleting all members, RelayGroupName = %q, want empty", got.RelayGroupName)
	}
	if len(got.RelayGroupNodeIDs) != 0 {
		t.Errorf("after deleting all members, RelayGroupNodeIDs = %v, want empty", got.RelayGroupNodeIDs)
	}
}

func TestDeleteNodeForSync_PrunesRelayGroupMember(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	const user = "admin"

	memberA := mustCreateNode(t, repo, Node{
		Username: user, NodeName: "Member-A", Protocol: "trojan",
		ClashConfig: `{"name":"Member-A","type":"trojan","server":"a.example.com","port":443}`,
		Enabled:     true,
	})
	memberB := mustCreateNode(t, repo, Node{
		Username: user, NodeName: "Member-B", Protocol: "trojan",
		ClashConfig: `{"name":"Member-B","type":"trojan","server":"b.example.com","port":443}`,
		Enabled:     true,
	})
	source := mustCreateNode(t, repo, Node{
		Username: user, NodeName: "Source", Protocol: "vmess",
		ClashConfig:       `{"name":"Source","type":"vmess","server":"s.example.com","port":443}`,
		Enabled:           true,
		RelayGroupName:    "中转HK",
		RelayGroupNodeIDs: []int64{memberA.ID, memberB.ID},
		ChainProxyNodeID:  &memberA.ID,
	})

	if err := repo.DeleteNodeForSync(ctx, memberA.ID, user); err != nil {
		t.Fatalf("DeleteNodeForSync(memberA): %v", err)
	}
	got, err := repo.GetNode(ctx, source.ID, user)
	if err != nil {
		t.Fatalf("GetNode(source): %v", err)
	}
	if got.ChainProxyNodeID != nil {
		t.Errorf("ChainProxyNodeID = %v, want nil", got.ChainProxyNodeID)
	}
	if got.RelayGroupName != "中转HK" {
		t.Errorf("RelayGroupName = %q, want 中转HK", got.RelayGroupName)
	}
	if len(got.RelayGroupNodeIDs) != 1 || got.RelayGroupNodeIDs[0] != memberB.ID {
		t.Errorf("RelayGroupNodeIDs = %v, want [%d]", got.RelayGroupNodeIDs, memberB.ID)
	}
}
