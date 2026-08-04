package storage

import (
	"context"
	"strings"
	"testing"
)

func TestBatchCreateNodesAddsSuffixForDuplicateNames(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	mustCreateNode(t, repo, Node{
		Username: "alice", NodeName: "香港", Protocol: "ss",
		ClashConfig: `{"name":"香港","type":"ss"}`, Enabled: true,
	})

	created, err := repo.BatchCreateNodes(ctx, []Node{
		{Username: "alice", NodeName: "香港", Protocol: "vmess", ClashConfig: `{"name":"香港","type":"vmess"}`, Enabled: true},
		{Username: "alice", NodeName: "香港", Protocol: "trojan", ClashConfig: `{"name":"香港","type":"trojan"}`, Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if created[0].NodeName != "香港-2" || created[1].NodeName != "香港-3" {
		t.Fatalf("unexpected names: %q, %q", created[0].NodeName, created[1].NodeName)
	}
	if !strings.Contains(created[0].ClashConfig, `"name":"香港-2"`) || !strings.Contains(created[1].ClashConfig, `"name":"香港-3"`) {
		t.Fatalf("config names were not updated: %s; %s", created[0].ClashConfig, created[1].ClashConfig)
	}
}
