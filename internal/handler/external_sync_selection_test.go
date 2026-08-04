package handler

import (
	"testing"
	"time"

	"miaomiaowu/internal/storage"
)

func TestStoreExternalSyncSelectionUsesOpaqueIDs(t *testing.T) {
	username := "selection-test"
	sessionID, err := storeExternalSyncSelection(username, []externalSyncCandidate{{
		SubscriptionName: "sub", Name: "node", Protocol: "vless",
		node: storage.Node{Username: username, NodeName: "node", Protocol: "vless"},
	}})
	if err != nil || sessionID == "" {
		t.Fatalf("store selection: id=%q err=%v", sessionID, err)
	}
	externalSyncSelections.Lock()
	session, ok := externalSyncSelections.sessions[sessionID]
	delete(externalSyncSelections.sessions, sessionID)
	externalSyncSelections.Unlock()
	if !ok || session.Username != username || len(session.Candidates) != 1 {
		t.Fatalf("invalid stored session: %#v", session)
	}
	if !session.ExpiresAt.After(time.Now()) {
		t.Fatal("selection session is already expired")
	}
	for id := range session.Candidates {
		if id == "" || id == "node" {
			t.Fatalf("candidate ID is not opaque: %q", id)
		}
	}
}
