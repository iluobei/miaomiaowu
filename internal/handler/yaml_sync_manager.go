package handler

import (
	"miaomiaowu/internal/logger"
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

	logger.Info("[YAML同步] 开始同步节点", "old_name", oldNodeName, "new_name", newNodeName)
	err := syncNodeToYAMLFiles(m.subscribeDir, oldNodeName, newNodeName, clashConfigJSON)
	if err != nil {
		logger.Info("[YAML同步] 节点同步失败", "node_name", oldNodeName, "error", err)
	} else {
		logger.Info("[YAML同步] 节点同步成功", "node_name", newNodeName)
	}
	return err
}

// DeleteNode deletes a node from YAML files with proper locking
func (m *YAMLSyncManager) DeleteNode(nodeName string) error {
	if m.subscribeDir == "" {
		return nil // No-op if subscribe directory is not configured
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logger.Info("[YAML同步] 开始删除节点", "node_name", nodeName)
	affectedFiles, err := deleteNodeFromYAMLFilesWithLog(m.subscribeDir, nodeName)
	if err != nil {
		logger.Info("[YAML同步] 节点删除失败", "node_name", nodeName, "error", err)
	} else if len(affectedFiles) > 0 {
		logger.Info("[YAML同步] 节点删除成功", "node_name", nodeName, "affected_count", len(affectedFiles), "files", affectedFiles)
	} else {
		logger.Info("[YAML同步] 节点未在任何订阅文件中找到", "node_name", nodeName)
	}
	return err
}

// BatchDeleteNodes efficiently deletes multiple nodes with a single lock
func (m *YAMLSyncManager) BatchDeleteNodes(nodeNames []string) error {
	if m.subscribeDir == "" || len(nodeNames) == 0 {
		return nil // No-op if subscribe directory is not configured or no nodes to delete
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logger.Info("[YAML同步] 开始批量删除节点", "count", len(nodeNames))

	totalAffectedFiles := make(map[string]int) // 文件名 -> 删除的节点数
	successCount := 0
	failCount := 0

	// Delete all nodes in a single locked operation
	for _, nodeName := range nodeNames {
		affectedFiles, err := deleteNodeFromYAMLFilesWithLog(m.subscribeDir, nodeName)
		if err != nil {
			logger.Info("[YAML同步] 批量删除中节点失败", "node_name", nodeName, "error", err)
			failCount++
			continue
		}

		if len(affectedFiles) > 0 {
			successCount++
			for _, fileName := range affectedFiles {
				totalAffectedFiles[fileName]++
			}
		}
	}

	// 输出批量删除摘要
	if len(totalAffectedFiles) > 0 {
		logger.Info("[YAML同步] 批量删除完成", "success_count", successCount, "fail_count", failCount, "affected_files", len(totalAffectedFiles))
		for fileName, count := range totalAffectedFiles {
			logger.Info("[YAML同步] 文件删除统计", "filename", fileName, "deleted_count", count)
		}
	} else {
		logger.Info("[YAML同步] 批量删除完成但未找到节点", "count", len(nodeNames))
	}

	return nil
}

// NodeUpdate 表示单个节点的更新信息
type NodeUpdate struct {
	OldName         string
	NewName         string
	ClashConfigJSON string
}

// BatchSyncNodes efficiently syncs multiple node updates with a single lock
// 批量同步多个节点更新，只读写 YAML 文件一次
func (m *YAMLSyncManager) BatchSyncNodes(updates []NodeUpdate) error {
	if m.subscribeDir == "" || len(updates) == 0 {
		return nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	logger.Info("[YAML同步] 开始批量同步节点", "count", len(updates))

	err := batchSyncNodesToYAMLFiles(m.subscribeDir, updates)
	if err != nil {
		logger.Info("[YAML同步] 批量同步失败", "error", err)
		return err
	}

	logger.Info("[YAML同步] 批量同步完成")
	return nil
}
