package handler

import (
	"sync"
)

// YAMLSyncManager manages concurrent access to YAML subscription files
type YAMLSyncManager struct {
	mu           sync.Mutex
	subscribeDir string
}

// NewYAMLSyncManager creates a new YAML sync manager
func NewYAMLSyncManager(subscribeDir string) *YAMLSyncManager {
	return &YAMLSyncManager{
		subscribeDir: subscribeDir,
	}
}

// SyncNode synchronizes a node update to YAML files with proper locking
func (m *YAMLSyncManager) SyncNode(oldNodeName, newNodeName string, clashConfigJSON string) error {
	if m.subscribeDir == "" {
		return nil // No-op if subscribe directory is not configured
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return syncNodeToYAMLFiles(m.subscribeDir, oldNodeName, newNodeName, clashConfigJSON)
}

// DeleteNode deletes a node from YAML files with proper locking
func (m *YAMLSyncManager) DeleteNode(nodeName string) error {
	if m.subscribeDir == "" {
		return nil // No-op if subscribe directory is not configured
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	return deleteNodeFromYAMLFiles(m.subscribeDir, nodeName)
}

// BatchDeleteNodes efficiently deletes multiple nodes with a single lock
func (m *YAMLSyncManager) BatchDeleteNodes(nodeNames []string) error {
	if m.subscribeDir == "" || len(nodeNames) == 0 {
		return nil // No-op if subscribe directory is not configured or no nodes to delete
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Delete all nodes in a single locked operation
	for _, nodeName := range nodeNames {
		if err := deleteNodeFromYAMLFiles(m.subscribeDir, nodeName); err != nil {
			// Log error but continue with other deletions
			// The error is not critical for the operation
			continue
		}
	}

	return nil
}
