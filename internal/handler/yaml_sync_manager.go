package handler

import (
	"log"
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

	log.Printf("[YAML同步] 开始同步节点: %s -> %s", oldNodeName, newNodeName)
	err := syncNodeToYAMLFiles(m.subscribeDir, oldNodeName, newNodeName, clashConfigJSON)
	if err != nil {
		log.Printf("[YAML同步] 节点同步失败: %s, 错误: %v", oldNodeName, err)
	} else {
		log.Printf("[YAML同步] 节点同步成功: %s", newNodeName)
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

	log.Printf("[YAML同步] 开始删除节点: %s", nodeName)
	affectedFiles, err := deleteNodeFromYAMLFilesWithLog(m.subscribeDir, nodeName)
	if err != nil {
		log.Printf("[YAML同步] 节点删除失败: %s, 错误: %v", nodeName, err)
	} else if len(affectedFiles) > 0 {
		log.Printf("[YAML同步] 节点删除成功: %s, 影响了 %d 个订阅文件: %v", nodeName, len(affectedFiles), affectedFiles)
	} else {
		log.Printf("[YAML同步] 节点 %s 未在任何订阅文件中找到", nodeName)
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

	log.Printf("[YAML同步] 开始批量删除 %d 个节点", len(nodeNames))

	totalAffectedFiles := make(map[string]int) // 文件名 -> 删除的节点数
	successCount := 0
	failCount := 0

	// Delete all nodes in a single locked operation
	for _, nodeName := range nodeNames {
		affectedFiles, err := deleteNodeFromYAMLFilesWithLog(m.subscribeDir, nodeName)
		if err != nil {
			log.Printf("[YAML同步] 批量删除中节点失败: %s, 错误: %v", nodeName, err)
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
		log.Printf("[YAML同步] 批量删除完成: 成功 %d 个, 失败 %d 个, 共影响 %d 个订阅文件",
			successCount, failCount, len(totalAffectedFiles))
		for fileName, count := range totalAffectedFiles {
			log.Printf("[YAML同步]   - %s: 删除了 %d 个节点", fileName, count)
		}
	} else {
		log.Printf("[YAML同步] 批量删除完成: %d 个节点未在任何订阅文件中找到", len(nodeNames))
	}

	return nil
}
