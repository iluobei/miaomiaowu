import React, { useState, useMemo } from 'react'
import { createPortal } from 'react-dom'
import { GripVertical, X, Plus, Edit2, Check, Search } from 'lucide-react'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  useSensor,
  useSensors,
  useDraggable,
  useDroppable,
  type DragStartEvent,
  type DragEndEvent,
  pointerWithin,
  rectIntersection,
  type CollisionDetection
} from '@dnd-kit/core'
import { SortableContext, rectSortingStrategy, useSortable, arrayMove } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { OUTBOUND_NAMES } from '@/lib/sublink/translations'

interface ProxyGroup {
  name: string
  type: string
  proxies: string[]
}

interface Node {
  node_name: string
  tag?: string
  [key: string]: any
}

// 拖拽类型定义
type DragItemType = 'available-node' | 'available-header' | 'group-node' | 'group-title' | 'group-card'

interface DragItemData {
  type: DragItemType
  nodeName?: string
  nodeNames?: string[]
  groupName?: string
  index?: number
}

interface ActiveDragItem {
  id: string
  data: DragItemData
}

interface EditNodesDialogProps {
  allNodes?: Node[]
  open: boolean
  onOpenChange: (open: boolean) => void
  title: string
  description?: string
  proxyGroups: ProxyGroup[]
  availableNodes: string[]
  onProxyGroupsChange: (groups: ProxyGroup[]) => void
  onSave: () => void
  isSaving?: boolean
  showAllNodes?: boolean
  onShowAllNodesChange?: (show: boolean) => void
  onConfigureChainProxy?: () => void
  cancelButtonText?: string
  saveButtonText?: string
  // 保留旧的 props 以保持向后兼容，但不再使用
  draggedNode?: any
  onDragStart?: any
  onDragEnd?: any
  dragOverGroup?: any
  onDragEnterGroup?: any
  onDragLeaveGroup?: any
  onDrop?: any
  onDropToAvailable?: any
  onRemoveNodeFromGroup?: (groupName: string, nodeIndex: number) => void
  onRemoveGroup?: (groupName: string) => void
  onRenameGroup?: (oldName: string, newName: string) => void
  handleCardDragStart?: any
  handleCardDragEnd?: any
  handleNodeDragEnd?: any
  activeGroupTitle?: any
  activeCard?: any
}

export function EditNodesDialog({
  allNodes = [],
  open,
  onOpenChange,
  title,
  description = '拖拽节点到不同的代理组，自定义每个组的节点列表',
  proxyGroups,
  availableNodes,
  onProxyGroupsChange,
  onSave,
  isSaving = false,
  showAllNodes,
  onShowAllNodesChange,
  onConfigureChainProxy,
  cancelButtonText: _cancelButtonText = '取消',
  saveButtonText = '确定',
  onRemoveNodeFromGroup,
  onRemoveGroup,
  onRenameGroup
}: EditNodesDialogProps) {
  // 添加代理组对话框状态
  const [addGroupDialogOpen, setAddGroupDialogOpen] = useState(false)
  const [newGroupName, setNewGroupName] = useState('')

  // 代理组改名状态
  const [editingGroupName, setEditingGroupName] = useState<string | null>(null)
  const [editingGroupValue, setEditingGroupValue] = useState('')

  // 节点筛选状态
  const [nodeNameFilter, setNodeNameFilter] = useState('')
  const [nodeTagFilter, setNodeTagFilter] = useState<string>('')

  // 统一的拖拽状态
  const [activeDragItem, setActiveDragItem] = useState<ActiveDragItem | null>(null)

  // 保存滚动位置
  const scrollContainerRef = React.useRef<HTMLDivElement>(null)

  // 提取唯一标签列表
  const uniqueTags = useMemo(() => {
    const tags = new Set<string>()
    allNodes.forEach(node => {
      if (node.tag && node.tag.trim()) {
        tags.add(node.tag.trim())
      }
    })
    return Array.from(tags).sort()
  }, [allNodes])

  // 创建节点名称到标签的映射
  const nodeTagMap = useMemo(() => {
    const map = new Map<string, string>()
    allNodes.forEach(node => {
      map.set(node.node_name, node.tag || '')
    })
    return map
  }, [allNodes])

  // 筛选可用节点
  const filteredAvailableNodes = useMemo(() => {
    let filtered = availableNodes

    // 按名称筛选
    if (nodeNameFilter.trim()) {
      const filterLower = nodeNameFilter.toLowerCase().trim()
      filtered = filtered.filter(nodeName =>
        nodeName.toLowerCase().includes(filterLower)
      )
    }

    // 按标签筛选
    if (nodeTagFilter && nodeTagFilter !== 'all') {
      filtered = filtered.filter(nodeName => {
        const tag = nodeTagMap.get(nodeName) || ''
        return tag === nodeTagFilter
      })
    }

    return filtered
  }, [availableNodes, nodeNameFilter, nodeTagFilter, nodeTagMap])

  // 统一的传感器配置
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  )

  // 自定义碰撞检测 - 优先使用指针检测，然后使用矩形相交
  const customCollisionDetection: CollisionDetection = React.useCallback((args) => {
    // 先尝试指针检测
    const pointerCollisions = pointerWithin(args)
    if (pointerCollisions.length > 0) {
      return pointerCollisions
    }
    // 回退到矩形相交检测
    return rectIntersection(args)
  }, [])

  // 统一的拖拽开始处理
  const handleDragStart = (event: DragStartEvent) => {
    const { active } = event
    const data = active.data.current as DragItemData

    setActiveDragItem({
      id: String(active.id),
      data
    })
  }

  // 统一的拖拽结束处理
  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event
    setActiveDragItem(null)

    if (!over) return

    const activeData = active.data.current as DragItemData
    const overId = String(over.id)
    const overData = over.data.current as { type?: string; groupName?: string } | undefined

    // 获取目标代理组名称
    const getTargetGroupName = (): string | null => {
      if (overId === 'all-groups-zone') return 'all-groups'
      if (overId === 'available-zone') return 'available'
      if (overId.startsWith('drop-')) return overId.replace('drop-', '')
      if (overData?.groupName) return overData.groupName
      // 检查是否放在了某个代理组的节点上
      if (overId.includes('-') && !overId.startsWith('available-node-') && !overId.startsWith('group-title-')) {
        // 找到对应的代理组
        const groupName = proxyGroups.find(g => overId.startsWith(`${g.name}-`))?.name
        if (groupName) return groupName
      }
      return null
    }

    switch (activeData.type) {
      case 'available-node': {
        // 从可用节点拖到代理组
        const targetGroup = getTargetGroupName()
        if (!targetGroup || targetGroup === 'available') return

        const nodeName = activeData.nodeName!

        if (targetGroup === 'all-groups') {
          // 添加到所有代理组（跳过与节点同名的代理组，防止代理组添加到自己内部）
          const updatedGroups = proxyGroups.map(group => {
            if (group.name !== nodeName && !group.proxies.includes(nodeName)) {
              return { ...group, proxies: [...group.proxies, nodeName] }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        } else {
          // 阻止将代理组添加到自己内部
          if (nodeName === targetGroup) return

          // 添加到指定代理组
          const updatedGroups = proxyGroups.map(group => {
            if (group.name === targetGroup && !group.proxies.includes(nodeName)) {
              // 计算插入位置
              let insertIndex = group.proxies.length

              // 如果放置目标是代理组内的节点，计算插入位置
              if (overId.startsWith(`${targetGroup}-`)) {
                const targetNodeName = overId.replace(`${targetGroup}-`, '')
                const targetIdx = group.proxies.indexOf(targetNodeName)
                if (targetIdx !== -1) {
                  insertIndex = targetIdx
                }
              }

              const newProxies = [...group.proxies]
              newProxies.splice(insertIndex, 0, nodeName)
              return { ...group, proxies: newProxies }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        }
        break
      }

      case 'available-header': {
        // 批量添加筛选后的节点
        const targetGroup = getTargetGroupName()
        if (!targetGroup || targetGroup === 'available') return

        const nodeNames = activeData.nodeNames || []

        if (targetGroup === 'all-groups') {
          // 添加到所有代理组（过滤掉与代理组同名的节点，防止代理组添加到自己内部）
          const updatedGroups = proxyGroups.map(group => {
            const existingNodes = new Set(group.proxies)
            // 过滤掉已存在的节点和与当前代理组同名的节点
            const newNodes = nodeNames.filter(name => !existingNodes.has(name) && name !== group.name)
            if (newNodes.length > 0) {
              return { ...group, proxies: [...group.proxies, ...newNodes] }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        } else {
          // 添加到指定代理组
          const updatedGroups = proxyGroups.map(group => {
            if (group.name === targetGroup) {
              const existingNodes = new Set(group.proxies)
              // 过滤掉已存在的节点和与当前代理组同名的节点
              const newNodes = nodeNames.filter(name => !existingNodes.has(name) && name !== group.name)
              if (newNodes.length > 0) {
                return { ...group, proxies: [...group.proxies, ...newNodes] }
              }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        }
        break
      }

      case 'group-node': {
        // 代理组内节点拖拽
        const sourceGroup = activeData.groupName!
        const targetGroup = getTargetGroupName()

        if (!targetGroup) return

        if (targetGroup === 'available') {
          // 从代理组移除节点（拖回可用节点区域）
          if (onRemoveNodeFromGroup && activeData.index !== undefined) {
            onRemoveNodeFromGroup(sourceGroup, activeData.index)
          }
          return
        }

        if (sourceGroup === targetGroup) {
          // 同一代理组内排序
          const group = proxyGroups.find(g => g.name === sourceGroup)
          if (!group) return

          const oldIndex = activeData.index!
          const nodeId = overId
          const targetNodeName = nodeId.replace(`${sourceGroup}-`, '')
          const newIndex = group.proxies.indexOf(targetNodeName)

          if (newIndex !== -1 && oldIndex !== newIndex) {
            const updatedGroups = proxyGroups.map(g => {
              if (g.name === sourceGroup) {
                return { ...g, proxies: arrayMove(g.proxies, oldIndex, newIndex) }
              }
              return g
            })
            onProxyGroupsChange(updatedGroups)
          }
        } else if (targetGroup !== 'all-groups') {
          // 跨代理组移动节点
          const nodeName = activeData.nodeName!

          // 阻止将代理组添加到自己内部（代理组名称不能作为节点添加到同名代理组）
          if (nodeName === targetGroup) return

          const updatedGroups = proxyGroups.map(group => {
            if (group.name === sourceGroup) {
              // 从源组移除
              return { ...group, proxies: group.proxies.filter((_, i) => i !== activeData.index) }
            }
            if (group.name === targetGroup && !group.proxies.includes(nodeName)) {
              // 添加到目标组
              return { ...group, proxies: [...group.proxies, nodeName] }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        }
        break
      }

      case 'group-title': {
        // 代理组标题拖到其他代理组（作为节点添加）
        const sourceGroupName = activeData.groupName!
        const targetGroup = getTargetGroupName()

        if (!targetGroup || targetGroup === sourceGroupName || targetGroup === 'available') return

        if (targetGroup === 'all-groups') {
          // 添加到所有代理组
          const updatedGroups = proxyGroups.map(group => {
            if (group.name !== sourceGroupName && !group.proxies.includes(sourceGroupName)) {
              return { ...group, proxies: [...group.proxies, sourceGroupName] }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        } else {
          // 添加到指定代理组
          const updatedGroups = proxyGroups.map(group => {
            if (group.name === targetGroup && !group.proxies.includes(sourceGroupName)) {
              return { ...group, proxies: [...group.proxies, sourceGroupName] }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        }
        break
      }

      case 'group-card': {
        // 代理组卡片排序
        if (active.id === over.id) return

        const oldIndex = proxyGroups.findIndex(g => g.name === active.id)
        const newIndex = proxyGroups.findIndex(g => g.name === over.id)

        if (oldIndex !== -1 && newIndex !== -1) {
          onProxyGroupsChange(arrayMove(proxyGroups, oldIndex, newIndex))
        }
        break
      }
    }
  }

  // 保存滚动位置的包装函数
  const withScrollPreservation = <T extends (...args: any[]) => void>(fn: T) => {
    return (...args: Parameters<T>) => {
      const scrollTop = scrollContainerRef.current?.scrollTop ?? 0
      fn(...args)
      requestAnimationFrame(() => {
        if (scrollContainerRef.current) {
          scrollContainerRef.current.scrollTop = scrollTop
        }
      })
    }
  }

  // 包装删除节点函数
  const wrappedRemoveNodeFromGroup = React.useCallback(
    withScrollPreservation((groupName: string, nodeIndex: number) => {
      if (onRemoveNodeFromGroup) {
        onRemoveNodeFromGroup(groupName, nodeIndex)
      }
    }),
    [onRemoveNodeFromGroup]
  )

  // 包装删除代理组函数
  const wrappedRemoveGroup = React.useCallback(
    withScrollPreservation((groupName: string) => {
      if (onRemoveGroup) {
        onRemoveGroup(groupName)
      }
    }),
    [onRemoveGroup]
  )

  // 处理代理组改名
  const handleRenameGroupInternal = (oldName: string, newName: string) => {
    const trimmedName = newName.trim()
    if (!trimmedName || trimmedName === oldName) {
      setEditingGroupName(null)
      setEditingGroupValue('')
      return
    }

    const existingGroup = proxyGroups.find(group => group.name === trimmedName && group.name !== oldName)
    if (existingGroup) {
      return
    }

    if (onRenameGroup) {
      onRenameGroup(oldName, trimmedName)
    }
    setEditingGroupName(null)
    setEditingGroupValue('')
  }

  const startEditingGroup = (groupName: string) => {
    setEditingGroupName(groupName)
    setEditingGroupValue(groupName)
  }

  const cancelEditingGroup = () => {
    setEditingGroupName(null)
    setEditingGroupValue('')
  }

  const submitEditingGroup = () => {
    if (editingGroupName && editingGroupValue) {
      handleRenameGroupInternal(editingGroupName, editingGroupValue)
    }
  }

  // 添加新代理组
  const handleAddGroup = () => {
    if (!newGroupName.trim()) return

    const newGroup: ProxyGroup = {
      name: newGroupName.trim(),
      type: 'select',
      proxies: []
    }

    onProxyGroupsChange([newGroup, ...proxyGroups])
    setNewGroupName('')
    setAddGroupDialogOpen(false)
  }

  const handleQuickSelect = (name: string) => {
    setNewGroupName(name)
  }

  // ================== 组件定义 ==================

  // 可拖动的可用节点
  interface DraggableAvailableNodeProps {
    proxy: string
    index: number
  }

  const DraggableAvailableNode = ({ proxy, index }: DraggableAvailableNodeProps) => {
    const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
      id: `available-node-${proxy}-${index}`,
      data: {
        type: 'available-node',
        nodeName: proxy,
        index
      } as DragItemData
    })

    const style: React.CSSProperties = {
      transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
      opacity: isDragging ? 0.5 : 1,
    }

    return (
      <div
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        className='flex items-center gap-2 p-2 rounded border hover:border-border hover:bg-accent cursor-move transition-colors duration-75'
      >
        <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
        <span className='text-sm truncate flex-1'>{proxy}</span>
      </div>
    )
  }

  // 可拖动的可用节点卡片标题（批量拖动）
  interface DraggableAvailableHeaderProps {
    filteredNodes: string[]
    totalNodes: number
  }

  const DraggableAvailableHeader = ({ filteredNodes, totalNodes }: DraggableAvailableHeaderProps) => {
    const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
      id: 'available-header',
      data: {
        type: 'available-header',
        nodeNames: filteredNodes
      } as DragItemData
    })

    const style: React.CSSProperties = {
      transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
      opacity: isDragging ? 0.5 : 1,
    }

    return (
      <div
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        className='flex items-center gap-2 cursor-move rounded-md px-2 py-1 hover:bg-accent transition-colors'
      >
        <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
        <div>
          <CardTitle className='text-base'>可用节点</CardTitle>
          <CardDescription className='text-xs'>
            {filteredNodes.length} / {totalNodes} 个节点
          </CardDescription>
        </div>
      </div>
    )
  }

  // 快捷拖放区（添加到所有代理组）
  const DroppableAllGroupsZone = () => {
    const { setNodeRef, isOver } = useDroppable({
      id: 'all-groups-zone',
      data: { type: 'all-groups-zone' }
    })

    return (
      <div
        ref={setNodeRef}
        className={`w-48 h-20 mr-9 border-2 rounded-lg flex items-center justify-center text-sm transition-all ${
          isOver
            ? 'border-primary bg-primary/10 border-solid'
            : 'border-dashed border-muted-foreground/30 bg-muted/20'
        }`}
      >
        <span className={isOver ? 'text-primary font-medium' : 'text-muted-foreground'}>
          添加到所有代理组
        </span>
      </div>
    )
  }

  // 可用节点区域（接收从代理组拖回的节点）
  interface DroppableAvailableZoneProps {
    children: React.ReactNode
  }

  const DroppableAvailableZone = ({ children }: DroppableAvailableZoneProps) => {
    const { setNodeRef, isOver } = useDroppable({
      id: 'available-zone',
      data: { type: 'available-zone' }
    })

    return (
      <Card
        ref={setNodeRef}
        className={`flex flex-col flex-1 transition-all duration-75 ${
          isOver ? 'ring-2 ring-primary shadow-lg scale-[1.02]' : ''
        }`}
      >
        {children}
      </Card>
    )
  }

  // 可拖动的代理组标题
  interface DraggableGroupTitleProps {
    groupName: string
  }

  const DraggableGroupTitle = ({ groupName }: DraggableGroupTitleProps) => {
    const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
      id: `group-title-${groupName}`,
      data: {
        type: 'group-title',
        groupName
      } as DragItemData
    })

    const style: React.CSSProperties = {
      transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
      opacity: isDragging ? 0.5 : 1,
    }

    const isEditing = editingGroupName === groupName

    return (
      <div ref={setNodeRef} style={style} className='flex items-center gap-2 group/title'>
        <div {...attributes} {...listeners} className='cursor-move'>
          <GripVertical className='h-3 w-3 text-muted-foreground flex-shrink-0' />
        </div>
        {isEditing ? (
          <div className='flex items-center gap-1 flex-1 min-w-0'>
            <Input
              value={editingGroupValue}
              onChange={(e) => setEditingGroupValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') submitEditingGroup()
                else if (e.key === 'Escape') cancelEditingGroup()
              }}
              className='h-6 text-base flex-1 min-w-0'
              placeholder='输入新名称...'
              autoFocus
            />
            <Button size='sm' className='h-6 w-6 p-0' onClick={submitEditingGroup} variant='ghost'>
              <Check className='h-3 w-3 text-green-600' />
            </Button>
          </div>
        ) : (
          <div className='flex items-center gap-1 flex-1 min-w-0'>
            <CardTitle
              className='text-base truncate cursor-text hover:text-foreground/80 flex-1 min-w-0'
              onClick={() => startEditingGroup(groupName)}
              title='点击编辑名称'
            >
              {groupName}
            </CardTitle>
            <Button
              size='sm'
              variant='ghost'
              className='h-5 w-5 p-0 flex-shrink-0 opacity-0 group-hover/title:opacity-100 transition-opacity'
              onClick={() => startEditingGroup(groupName)}
              title='编辑名称'
            >
              <Edit2 className='h-3 w-3 text-muted-foreground hover:text-foreground' />
            </Button>
          </div>
        )}
      </div>
    )
  }

  // 可排序的代理组内节点
  interface SortableProxyProps {
    proxy: string
    groupName: string
    index: number
  }

  const SortableProxy = ({ proxy, groupName, index }: SortableProxyProps) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({
      id: `${groupName}-${proxy}`,
      transition: {
        duration: 200,
        easing: 'cubic-bezier(0.25, 1, 0.5, 1)',
      },
      data: {
        type: 'group-node',
        groupName,
        nodeName: proxy,
        index
      } as DragItemData,
    })

    const style = {
      transform: CSS.Transform.toString(transform),
      transition: transition || 'transform 200ms cubic-bezier(0.25, 1, 0.5, 1)',
      opacity: isDragging ? 0.5 : 1,
    }

    return (
      <div
        ref={setNodeRef}
        style={style}
        className='flex items-center gap-2 p-2 rounded border hover:border-border hover:bg-accent group/item'
        data-proxy-item
      >
        <div {...attributes} {...listeners} className='cursor-move touch-none'>
          <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
        </div>
        <span className='text-sm truncate flex-1'>{proxy}</span>
        <Button
          variant='ghost'
          size='sm'
          className='h-6 w-6 p-0 flex-shrink-0'
          onClick={(e) => {
            e.stopPropagation()
            wrappedRemoveNodeFromGroup(groupName, index)
          }}
        >
          <X className='h-4 w-4 text-muted-foreground hover:text-destructive' />
        </Button>
      </div>
    )
  }

  // 可排序的代理组卡片
  interface SortableCardProps {
    group: ProxyGroup
  }

  const SortableCard = ({ group }: SortableCardProps) => {
    const isEditing = editingGroupName === group.name

    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      transition,
      isDragging,
    } = useSortable({
      id: group.name,
      data: {
        type: 'group-card',
        groupName: group.name,
      } as DragItemData,
      disabled: isEditing,
    })

    const { setNodeRef: setDropRef, isOver } = useDroppable({
      id: `drop-${group.name}`,
      data: {
        type: 'proxy-group',
        groupName: group.name,
      },
    })

    const style = {
      transform: CSS.Transform.toString(transform),
      transition: isDragging ? 'none' : transition,
      opacity: isDragging ? 0.5 : 1,
    }

    return (
      <Card
        ref={(node) => {
          setNodeRef(node)
          setDropRef(node)
        }}
        style={style}
        className={`flex flex-col transition-all ${
          isOver ? 'ring-2 ring-primary shadow-lg scale-[1.02]' : ''
        }`}
      >
        <CardHeader className='pb-3'>
          {/* 顶部居中拖动按钮 */}
          <div
            className={`flex justify-center -mt-2 mb-2 ${
              isEditing ? 'cursor-not-allowed opacity-50' : 'cursor-move touch-none'
            }`}
            {...(isEditing ? {} : attributes)}
            {...(isEditing ? {} : listeners)}
          >
            <div className={`group/drag-handle hover:bg-accent rounded-md px-3 py-1 transition-colors ${
              isEditing ? 'opacity-50' : ''
            }`}>
              <GripVertical className='h-4 w-4 text-muted-foreground group-hover/drag-handle:text-foreground transition-colors' />
            </div>
          </div>

          <div className='flex items-start justify-between gap-2'>
            <div className='flex-1 min-w-0'>
              <DraggableGroupTitle groupName={group.name} />
              <CardDescription className='text-xs'>
                {group.type} ({(group.proxies || []).length} 个节点)
              </CardDescription>
            </div>
            <Button
              variant='ghost'
              size='sm'
              className='h-6 w-6 p-0 flex-shrink-0'
              onClick={(e) => {
                e.stopPropagation()
                wrappedRemoveGroup(group.name)
              }}
            >
              <X className='h-4 w-4 text-muted-foreground hover:text-destructive' />
            </Button>
          </div>
        </CardHeader>
        <CardContent className='flex-1 space-y-1 min-h-[200px]' data-card-content>
          <SortableContext
            items={(group.proxies || []).filter(p => p).map(p => `${group.name}-${p}`)}
          >
            {(group.proxies || []).map((proxy, idx) => (
              proxy && (
                <SortableProxy
                  key={`${group.name}-${proxy}-${idx}`}
                  proxy={proxy}
                  groupName={group.name}
                  index={idx}
                />
              )
            ))}
          </SortableContext>
          {(group.proxies || []).filter(p => p).length === 0 && (
            <div className={`text-sm text-center py-8 transition-colors ${
              isOver ? 'text-primary font-medium' : 'text-muted-foreground'
            }`}>
              将节点拖拽到这里
            </div>
          )}
        </CardContent>
      </Card>
    )
  }

  // ================== 渲染 ==================

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className='!max-w-[95vw] w-[95vw] max-h-[90vh] flex flex-col' style={{ maxWidth: '95vw', width: '95vw' }}>
          <DndContext
            sensors={sensors}
            collisionDetection={customCollisionDetection}
            onDragStart={handleDragStart}
            onDragEnd={handleDragEnd}
          >
            <DialogHeader>
              <div className='flex items-start justify-between gap-4'>
                <div className='flex-1'>
                  <DialogTitle>{title}</DialogTitle>
                  <DialogDescription>{description}</DialogDescription>
                </div>
                {/* 快捷拖放区 */}
                <DroppableAllGroupsZone />
              </div>
            </DialogHeader>

            <div className='flex-1 flex gap-4 py-4 min-h-0'>
              {/* 左侧：代理组 */}
              <div ref={scrollContainerRef} className='flex-1 overflow-y-auto pr-2'>
                <SortableContext
                  items={proxyGroups.map(g => g.name)}
                  strategy={rectSortingStrategy}
                >
                  <div className='grid gap-4' style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}>
                    {proxyGroups.map((group) => (
                      <SortableCard key={group.name} group={group} />
                    ))}
                  </div>
                </SortableContext>
              </div>

              {/* 分割线 */}
              <div className='w-1 bg-border flex-shrink-0'></div>

              {/* 右侧：可用节点 */}
              <div className='w-64 flex-shrink-0 flex flex-col'>
                {/* 操作按钮 */}
                <div className='flex-shrink-0 mb-4'>
                  <div className='flex gap-2'>
                    <Button
                      variant='outline'
                      onClick={() => setAddGroupDialogOpen(true)}
                      className='flex-1'
                    >
                      <Plus className='h-4 w-4 mr-1' />
                      添加代理组
                    </Button>
                    <Button onClick={onSave} disabled={isSaving} className='flex-1'>
                      {isSaving ? '保存中...' : saveButtonText}
                    </Button>
                  </div>
                </div>

                {/* 显示/隐藏已添加节点按钮 */}
                {showAllNodes !== undefined && onShowAllNodesChange && (
                  <div className='flex-shrink-0 mb-4'>
                    <Button
                      variant='outline'
                      className='w-full'
                      onClick={() => onShowAllNodesChange(!showAllNodes)}
                    >
                      {showAllNodes ? '隐藏已添加节点' : '显示已添加节点'}
                    </Button>
                  </div>
                )}

                {/* 配置链式代理按钮 */}
                {onConfigureChainProxy && (
                  <div className='flex-shrink-0 mb-4'>
                    <Button
                      variant='outline'
                      className='w-full'
                      onClick={onConfigureChainProxy}
                    >
                      配置链式代理
                    </Button>
                  </div>
                )}

                {/* 筛选控件 */}
                <div className='flex-shrink-0 mb-4 flex gap-2 items-center'>
                  <div className='relative flex-1'>
                    <Search className='absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground' />
                    <Input
                      placeholder='按名称筛选...'
                      value={nodeNameFilter}
                      onChange={(e) => setNodeNameFilter(e.target.value)}
                      className='pl-8 h-9 text-sm'
                    />
                  </div>

                  {uniqueTags.length > 0 && (
                    <Select value={nodeTagFilter} onValueChange={setNodeTagFilter}>
                      <SelectTrigger className='h-9 text-sm w-[120px]'>
                        <SelectValue placeholder='所有标签' />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='all'>所有标签</SelectItem>
                        {uniqueTags.map(tag => (
                          <SelectItem key={tag} value={tag}>
                            {tag}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  )}
                </div>

                {/* 可用节点卡片 */}
                <DroppableAvailableZone>
                  <CardHeader className='pb-3 flex-shrink-0'>
                    <DraggableAvailableHeader
                      filteredNodes={filteredAvailableNodes}
                      totalNodes={availableNodes.length}
                    />
                  </CardHeader>
                  <CardContent className='flex-1 overflow-y-auto space-y-1 min-h-0'>
                    {filteredAvailableNodes.map((proxy, idx) => (
                      <DraggableAvailableNode
                        key={`available-${proxy}-${idx}`}
                        proxy={proxy}
                        index={idx}
                      />
                    ))}
                  </CardContent>
                </DroppableAvailableZone>
              </div>
            </div>

            {/* DragOverlay - 通过 Portal 渲染到 body 以避免 Dialog transform 影响定位 */}
            {typeof document !== 'undefined' && createPortal(
              <DragOverlay dropAnimation={null} style={{ cursor: 'grabbing' }}>
                {activeDragItem?.data.type === 'available-node' && (
                  <div className='flex items-center gap-2 p-2 rounded border bg-background shadow-2xl pointer-events-none'>
                    <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                    <span className='text-sm truncate'>{activeDragItem.data.nodeName}</span>
                  </div>
                )}
                {activeDragItem?.data.type === 'available-header' && (
                  <div className='flex items-center gap-2 p-2 rounded border bg-background shadow-2xl pointer-events-none'>
                    <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                    <span className='text-sm'>
                      批量添加 {activeDragItem.data.nodeNames?.length || 0} 个节点
                    </span>
                  </div>
                )}
                {activeDragItem?.data.type === 'group-node' && (
                  <div className='flex items-center gap-2 p-2 rounded border bg-background shadow-2xl pointer-events-none'>
                    <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                    <span className='text-sm truncate'>{activeDragItem.data.nodeName}</span>
                  </div>
                )}
                {activeDragItem?.data.type === 'group-title' && (
                  <div className='flex items-center gap-2 p-2 rounded border bg-background shadow-2xl pointer-events-none'>
                    <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                    <span className='text-sm truncate'>{activeDragItem.data.groupName}</span>
                  </div>
                )}
                {activeDragItem?.data.type === 'group-card' && (() => {
                  const group = proxyGroups.find(g => g.name === activeDragItem.data.groupName)
                  return (
                    <Card className='w-[240px] shadow-2xl opacity-95 pointer-events-none max-h-[400px] overflow-hidden'>
                      <CardHeader className='pb-3'>
                        <div className='flex justify-center -mt-2 mb-2'>
                          <div className='bg-accent rounded-md px-3 py-1'>
                            <GripVertical className='h-4 w-4 text-foreground' />
                          </div>
                        </div>
                        <div className='flex items-start justify-between gap-2'>
                          <div className='flex-1 min-w-0'>
                            <CardTitle className='text-base truncate'>{activeDragItem.data.groupName}</CardTitle>
                            <CardDescription className='text-xs'>
                              {group?.type || 'select'} ({group?.proxies.length || 0} 个节点)
                            </CardDescription>
                          </div>
                        </div>
                      </CardHeader>
                      <CardContent className='space-y-1 max-h-[280px] overflow-hidden'>
                        {group?.proxies.slice(0, 8).map((proxy, idx) => (
                          <div
                            key={`overlay-${proxy}-${idx}`}
                            className='flex items-center gap-2 p-2 rounded border bg-background'
                          >
                            <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                            <span className='text-sm truncate flex-1'>{proxy}</span>
                          </div>
                        ))}
                        {(group?.proxies.length || 0) > 8 && (
                          <div className='text-xs text-center text-muted-foreground py-1'>
                            还有 {(group?.proxies.length || 0) - 8} 个节点...
                          </div>
                        )}
                        {(group?.proxies.length || 0) === 0 && (
                          <div className='text-sm text-center py-4 text-muted-foreground'>
                            暂无节点
                          </div>
                        )}
                      </CardContent>
                    </Card>
                  )
                })()}
              </DragOverlay>,
              document.body
            )}
          </DndContext>
        </DialogContent>
      </Dialog>

      {/* 添加代理组对话框 */}
      <Dialog open={addGroupDialogOpen} onOpenChange={setAddGroupDialogOpen}>
        <DialogContent className='max-w-2xl'>
          <DialogHeader>
            <DialogTitle>添加代理组</DialogTitle>
            <DialogDescription>
              输入自定义名称或从预定义选项中快速选择
            </DialogDescription>
          </DialogHeader>

          <div className='space-y-4'>
            <div>
              <Input
                placeholder='输入代理组名称...'
                value={newGroupName}
                onChange={(e) => setNewGroupName(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter') handleAddGroup()
                }}
              />
            </div>

            <div>
              <p className='text-sm text-muted-foreground mb-2'>快速选择：</p>
              <div className='grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2'>
                {Object.entries(OUTBOUND_NAMES).map(([key, value]) => (
                  <Button
                    key={key}
                    variant='outline'
                    size='sm'
                    className='justify-start text-left h-auto py-2 px-3'
                    onClick={() => handleQuickSelect(value)}
                  >
                    <span className='truncate'>{value}</span>
                  </Button>
                ))}
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button variant='outline' onClick={() => setAddGroupDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={handleAddGroup} disabled={!newGroupName.trim()}>
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
