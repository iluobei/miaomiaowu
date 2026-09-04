import { useState } from 'react'

interface ProxyGroup {
  name: string
  type: string
  proxies: string[]
}

interface DraggedNode {
  name: string
  fromGroup: string | null
  fromIndex: number
  filteredNodes?: string[]
}

interface UseNodeDragDropOptions {
  proxyGroups: ProxyGroup[]
  onProxyGroupsChange: (groups: ProxyGroup[]) => void
  specialNodesToFilter?: string[]  // 特殊节点列表，如 ['♻️ 自动选择', '🚀 节点选择', 'DIRECT', 'REJECT']
}

export function useNodeDragDrop({
  proxyGroups,
  onProxyGroupsChange,
  specialNodesToFilter = []
}: UseNodeDragDropOptions) {
  const [draggedNode, setDraggedNode] = useState<DraggedNode | null>(null)
  const [dragOverGroup, setDragOverGroup] = useState<string | null>(null)
  const [activeGroupTitle, setActiveGroupTitle] = useState<string | null>(null)

  const handleDragStart = (
    nodeName: string,
    fromGroup: string | null,
    fromIndex: number,
    filteredNodes?: string[]
  ) => {
    setDraggedNode({ name: nodeName, fromGroup, fromIndex, filteredNodes })
  }

  const handleDragEnd = () => {
    setDraggedNode(null)
    setDragOverGroup(null)
    setActiveGroupTitle(null)
  }

  const handleDragEnterGroup = (groupName: string) => {
    setDragOverGroup(groupName)
  }

  const handleDragLeaveGroup = () => {
    setDragOverGroup(null)
  }

  const handleDrop = (toGroup: string, targetIndex?: number) => {
    if (!draggedNode) return

    const updatedGroups = [...proxyGroups]

    // 特殊处理：添加到所有代理组
    if (toGroup === 'all-groups') {
      // 如果拖动的是"可用节点"标题，添加筛选后的可用节点到所有代理组
      if (draggedNode.name === '__AVAILABLE_NODES__') {
        const nodesToAdd = draggedNode.filteredNodes || []
        updatedGroups.forEach(group => {
          nodesToAdd.forEach(nodeName => {
            // 过滤掉特殊节点
            const shouldAdd =
              !group.proxies.includes(nodeName) &&
              !specialNodesToFilter.includes(nodeName)

            if (shouldAdd) {
              group.proxies.push(nodeName)
            }
          })
        })
      } else {
        // 否则，将单个节点添加到所有代理组
        updatedGroups.forEach(group => {
          // 防止代理组添加到自己内部，也防止重复添加
          if (draggedNode.name !== group.name && !group.proxies.includes(draggedNode.name)) {
            group.proxies.push(draggedNode.name)
          }
        })
      }
      onProxyGroupsChange(updatedGroups)
      handleDragEnd()
      return
    }

    // 从原来的位置移除（只有从代理组拖动时才移除，从可用节点拖动时不移除）
    if (
      draggedNode.fromGroup &&
      draggedNode.fromGroup !== 'available' &&
      draggedNode.name !== '__AVAILABLE_NODES__'
    ) {
      const fromGroupIndex = updatedGroups.findIndex(g => g.name === draggedNode.fromGroup)
      if (fromGroupIndex !== -1) {
        updatedGroups[fromGroupIndex].proxies = updatedGroups[fromGroupIndex].proxies.filter(
          (_, idx) => idx !== draggedNode.fromIndex
        )
      }
    }

    // 添加到新位置
    if (toGroup !== 'available') {
      const toGroupIndex = updatedGroups.findIndex(g => g.name === toGroup)
      if (toGroupIndex !== -1) {
        // 特殊处理：如果拖动的是"可用节点"标题，添加筛选后的可用节点
        if (draggedNode.name === '__AVAILABLE_NODES__') {
          const nodesToAdd = draggedNode.filteredNodes || []
          nodesToAdd.forEach(nodeName => {
            // 过滤掉特殊节点
            const shouldAdd =
              !updatedGroups[toGroupIndex].proxies.includes(nodeName) &&
              !specialNodesToFilter.includes(nodeName)

            if (shouldAdd) {
              updatedGroups[toGroupIndex].proxies.push(nodeName)
            }
          })
        } else {
          // 防止代理组添加到自己内部
          if (draggedNode.name === toGroup) {
            handleDragEnd()
            return
          }
          // 检查节点是否已存在于目标组中
          if (!updatedGroups[toGroupIndex].proxies.includes(draggedNode.name)) {
            if (targetIndex !== undefined) {
              // 插入到指定位置
              updatedGroups[toGroupIndex].proxies.splice(targetIndex, 0, draggedNode.name)
            } else {
              // 添加到末尾
              updatedGroups[toGroupIndex].proxies.push(draggedNode.name)
            }
          }
        }
      }
    }

    onProxyGroupsChange(updatedGroups)
    handleDragEnd()
  }

  const handleDropToAvailable = () => {
    if (!draggedNode || !draggedNode.fromGroup || draggedNode.fromGroup === 'available') {
      handleDragEnd()
      return
    }

    // 从代理组中移除节点
    const updatedGroups = [...proxyGroups]
    const fromGroupIndex = updatedGroups.findIndex(g => g.name === draggedNode.fromGroup)
    if (fromGroupIndex !== -1) {
      updatedGroups[fromGroupIndex].proxies = updatedGroups[fromGroupIndex].proxies.filter(
        (_, idx) => idx !== draggedNode.fromIndex
      )
    }

    onProxyGroupsChange(updatedGroups)
    handleDragEnd()
  }

  return {
    draggedNode,
    dragOverGroup,
    activeGroupTitle,
    setActiveGroupTitle,
    handleDragStart,
    handleDragEnd,
    handleDragEnterGroup,
    handleDragLeaveGroup,
    handleDrop,
    handleDropToAvailable
  }
}
