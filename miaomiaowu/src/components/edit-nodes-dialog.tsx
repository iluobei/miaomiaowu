import React, { useState, useMemo } from 'react'
import { GripVertical, X, Plus, Edit2, Check, Search } from 'lucide-react'
import { DndContext, DragOverlay, PointerSensor, closestCenter, useSensor, useSensors, useDraggable, useDroppable } from '@dnd-kit/core'
import { SortableContext, rectSortingStrategy, useSortable } from '@dnd-kit/sortable'
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
  draggedNode: { name: string; fromGroup: string | null; fromIndex: number; filteredNodes?: string[] } | null
  onDragStart: (nodeName: string, fromGroup: string | null, fromIndex: number, filteredNodes?: string[]) => void
  onDragEnd: () => void
  dragOverGroup: string | null
  onDragEnterGroup: (groupName: string) => void
  onDragLeaveGroup: () => void
  onDrop: (toGroup: string, targetIndex?: number) => void
  onDropToAvailable: () => void
  onRemoveNodeFromGroup: (groupName: string, nodeIndex: number) => void
  onRemoveGroup: (groupName: string) => void
  onRenameGroup: (oldName: string, newName: string) => void
  handleCardDragStart: (event: any) => void
  handleCardDragEnd: (event: any) => void
  handleNodeDragEnd: (groupName: string) => (event: any) => void
  activeGroupTitle: string | null
  activeCard: ProxyGroup | null
  onConfigureChainProxy?: () => void
  cancelButtonText?: string
  saveButtonText?: string
}

export function EditNodesDialog({
  allNodes = [],
  open,
  onOpenChange,
  title,
  description = '拖拽节点到不同的代理组，自定义每个组的节点列表',
  proxyGroups,
  availableNodes,
  onProxyGroupsChange: _onProxyGroupsChange,
  onSave,
  isSaving = false,
  showAllNodes,
  onShowAllNodesChange,
  draggedNode: _draggedNode,
  onDragStart,
  onDragEnd,
  dragOverGroup,
  onDragEnterGroup,
  onDragLeaveGroup,
  onDrop,
  onDropToAvailable,
  onRemoveNodeFromGroup,
  onRemoveGroup,
  onRenameGroup,
  handleCardDragStart,
  handleCardDragEnd,
  handleNodeDragEnd,
  activeGroupTitle,
  activeCard,
  onConfigureChainProxy,
  cancelButtonText: _cancelButtonText = '取消',
  saveButtonText = '确定'
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

  // 保存滚动位置
  const scrollContainerRef = React.useRef<HTMLDivElement>(null)

  // 拖拽区域的ref，用于防止闪烁
  const allGroupsDropZoneRef = React.useRef<HTMLDivElement>(null)
  const availableNodesCardRef = React.useRef<HTMLDivElement>(null)

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

  // 包装 handleNodeDragEnd 以保存和恢复滚动位置
  const wrappedHandleNodeDragEnd = React.useCallback(
    (groupName: string) => (event: any) => {
      // 保存当前滚动位置
      const scrollTop = scrollContainerRef.current?.scrollTop ?? 0

      // 调用原始的 handleNodeDragEnd
      handleNodeDragEnd(groupName)(event)

      // 在下一帧恢复滚动位置
      requestAnimationFrame(() => {
        if (scrollContainerRef.current) {
          scrollContainerRef.current.scrollTop = scrollTop
        }
      })
    },
    [handleNodeDragEnd]
  )

  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  )

  // 添加新代理组
  const handleAddGroup = () => {
    if (!newGroupName.trim()) return

    const newGroup: ProxyGroup = {
      name: newGroupName.trim(),
      type: 'select',
      proxies: []
    }

    // 添加到首位
    _onProxyGroupsChange([newGroup, ...proxyGroups])

    // 重置并关闭对话框
    setNewGroupName('')
    setAddGroupDialogOpen(false)
  }

  // 快速选择预定义名称
  const handleQuickSelect = (name: string) => {
    setNewGroupName(name)
  }

  // 包装删除节点函数，保存和恢复滚动位置
  const wrappedRemoveNodeFromGroup = React.useCallback(
    (groupName: string, nodeIndex: number) => {
      const scrollTop = scrollContainerRef.current?.scrollTop ?? 0

      onRemoveNodeFromGroup(groupName, nodeIndex)

      requestAnimationFrame(() => {
        if (scrollContainerRef.current) {
          scrollContainerRef.current.scrollTop = scrollTop
        }
      })
    },
    [onRemoveNodeFromGroup]
  )

  // 包装删除代理组函数，保存和恢复滚动位置
  const wrappedRemoveGroup = React.useCallback(
    (groupName: string) => {
      const scrollTop = scrollContainerRef.current?.scrollTop ?? 0

      onRemoveGroup(groupName)

      requestAnimationFrame(() => {
        if (scrollContainerRef.current) {
          scrollContainerRef.current.scrollTop = scrollTop
        }
      })
    },
    [onRemoveGroup]
  )

  // 处理代理组改名
  const handleRenameGroup = (oldName: string, newName: string) => {
    const trimmedName = newName.trim()

    // 如果名称为空，不执行任何操作
    if (!trimmedName) {
      return
    }

    // 如果名称没有改变，直接退出编辑模式
    if (trimmedName === oldName) {
      setEditingGroupName(null)
      setEditingGroupValue('')
      return
    }

    // 检查是否与现有组名重复
    const existingGroup = proxyGroups.find(group => group.name === trimmedName && group.name !== oldName)
    if (existingGroup) {
      // 这里可以添加错误提示
      return
    }

    onRenameGroup(oldName, trimmedName)
    setEditingGroupName(null)
    setEditingGroupValue('')
  }

  // 开始编辑代理组名称
  const startEditingGroup = (groupName: string) => {
    setEditingGroupName(groupName)
    setEditingGroupValue(groupName)
  }

  // 取消编辑
  const cancelEditingGroup = () => {
    setEditingGroupName(null)
    setEditingGroupValue('')
  }

  // 提交编辑
  const submitEditingGroup = () => {
    if (editingGroupName && editingGroupValue) {
      handleRenameGroup(editingGroupName, editingGroupValue)
    }
  }

  // 可排序的节点组件
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
        type: 'proxy',
        groupName,
      },
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

  // 可拖动的代理组标题组件
  interface DraggableGroupTitleProps {
    groupName: string
  }

  const DraggableGroupTitle = ({ groupName }: DraggableGroupTitleProps) => {
    const {
      attributes,
      listeners,
      setNodeRef,
      transform,
      isDragging,
    } = useDraggable({
      id: `group-title-${groupName}`,
      data: {
        type: 'group-title',
        groupName: groupName,
      },
    })

    const style: React.CSSProperties = {
      transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
      opacity: isDragging ? 0.5 : 1,
    }

    const isEditing = editingGroupName === groupName

    return (
      <div ref={setNodeRef} style={style} className='flex items-center gap-2 group/title'>
        <div
          {...attributes}
          {...listeners}
          className='cursor-move'
        >
          <GripVertical className='h-3 w-3 text-muted-foreground flex-shrink-0' />
        </div>
        {isEditing ? (
          <div className='flex items-center gap-1 flex-1 min-w-0'>
            <Input
              value={editingGroupValue}
              onChange={(e) => setEditingGroupValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  submitEditingGroup()
                } else if (e.key === 'Escape') {
                  cancelEditingGroup()
                }
              }}
              className='h-6 text-base flex-1 min-w-0'
              placeholder='输入新名称...'
              autoFocus
            />
            <Button
              size='sm'
              className='h-6 w-6 p-0'
              onClick={submitEditingGroup}
              variant='ghost'
            >
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
              className='h-5 w-5 p-0 flex-shrink-0 opacity-0 group/title:hover:opacity-100 transition-opacity'
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

  // 可排序的卡片组件
  interface SortableCardProps {
    group: ProxyGroup
  }

  const SortableCard = ({ group }: SortableCardProps) => {
    const isEditing = editingGroupName === group.name
    const cardRef = React.useRef<HTMLDivElement>(null)

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
      },
      disabled: isEditing, // 编辑状态下禁用拖拽
    })

    const { setNodeRef: setDropRef, isOver } = useDroppable({
      id: `drop-${group.name}`,
      data: {
        type: 'group',
        groupName: group.name,
      },
    })

    const style = {
      transform: CSS.Transform.toString(transform),
      transition: isDragging ? 'none' : transition,
      opacity: isDragging ? 0.5 : 1,
    }

    // 处理拖放到代理组，计算插入位置
    const handleDropWithPosition = (e: React.DragEvent) => {
      // 检查是否有正在拖动的节点（_draggedNode）
      // 如果没有，说明这是 @dnd-kit 的内部拖放事件，不处理
      if (!_draggedNode) {
        return
      }

      e.preventDefault()

      // 获取代理组内容区域
      const cardContent = cardRef.current?.querySelector('[data-card-content]') as HTMLElement
      if (!cardContent) {
        onDrop(group.name)
        return
      }

      // 获取所有节点元素
      const proxyElements = Array.from(cardContent.querySelectorAll('[data-proxy-item]'))

      if (proxyElements.length === 0) {
        // 如果没有节点，插入到开头
        onDrop(group.name, 0)
        return
      }

      // 计算应该插入的位置
      const mouseY = e.clientY
      let insertIndex = proxyElements.length // 默认插入到末尾

      for (let i = 0; i < proxyElements.length; i++) {
        const element = proxyElements[i] as HTMLElement
        const rect = element.getBoundingClientRect()
        const elementMiddle = rect.top + rect.height / 2

        if (mouseY < elementMiddle) {
          insertIndex = i
          break
        }
      }

      onDrop(group.name, insertIndex)
    }

    return (
      <Card
        ref={(node) => {
          setNodeRef(node)
          setDropRef(node)
          cardRef.current = node
        }}
        style={style}
        className={`flex flex-col transition-all ${
          isOver
            ? 'ring-2 ring-primary shadow-lg scale-[1.02]'
            : ''
        }`}
        onDragOver={(e) => e.preventDefault()}
        onDragEnter={() => onDragEnterGroup(group.name)}
        onDragLeave={(e) => {
          // 只有当鼠标真正离开Card容器时才清除状态
          // 检查relatedTarget是否在Card内部
          if (cardRef.current && !cardRef.current.contains(e.relatedTarget as HTMLElement)) {
            onDragLeaveGroup()
          }
        }}
        onDrop={handleDropWithPosition}
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
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={wrappedHandleNodeDragEnd(group.name)}
          >
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
          </DndContext>
          {(group.proxies || []).filter(p => p).length === 0 && (
            <div className={`text-sm text-center py-8 transition-colors ${
              isOver
                ? 'text-primary font-medium'
                : 'text-muted-foreground'
            }`}>
              将节点拖拽到这里
            </div>
          )}
        </CardContent>
      </Card>
    )
  }

  return (
    <>
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='!max-w-[95vw] w-[95vw] max-h-[90vh] flex flex-col' style={{ maxWidth: '95vw', width: '95vw' }}>
        <DialogHeader>
          <div className='flex items-start justify-between gap-4'>
            <div className='flex-1'>
              <DialogTitle>{title}</DialogTitle>
              <DialogDescription>
                {description}
              </DialogDescription>
            </div>
            {/* 拖放到所有代理组的区域 */}
            <div
              ref={allGroupsDropZoneRef}
              className={`w-48 h-20 mr-9 border-2 rounded-lg flex items-center justify-center text-sm transition-all ${
                dragOverGroup === 'all-groups'
                  ? 'border-primary bg-primary/10 border-solid'
                  : 'border-dashed border-muted-foreground/30 bg-muted/20'
              }`}
              onDragOver={(e) => e.preventDefault()}
              onDragEnter={() => onDragEnterGroup('all-groups')}
              onDragLeave={(e) => {
                // 只有当鼠标真正离开区域时才清除状态
                if (allGroupsDropZoneRef.current && !allGroupsDropZoneRef.current.contains(e.relatedTarget as HTMLElement)) {
                  onDragLeaveGroup()
                }
              }}
              onDrop={() => onDrop('all-groups')}
            >
              <span className={dragOverGroup === 'all-groups' ? 'text-primary font-medium' : 'text-muted-foreground'}>
                添加到所有代理组
              </span>
            </div>
          </div>
        </DialogHeader>
        <div className='flex-1 flex gap-4 py-4 min-h-0'>
          {/* 左侧：代理组 - 使用 DND Kit 实现排序 */}
          <div ref={scrollContainerRef} className='flex-1 overflow-y-auto pr-2'>
            <DndContext
              sensors={sensors}
              collisionDetection={closestCenter}
              onDragStart={handleCardDragStart}
              onDragEnd={handleCardDragEnd}
            >
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
              <DragOverlay dropAnimation={null} style={{ cursor: 'grabbing' }}>
                {activeCard ? (
                  <Card className='w-[240px] shadow-2xl opacity-90 pointer-events-none'>
                    <CardHeader className='pb-3'>
                      <div className='flex justify-center -mt-2 mb-2'>
                        <div className='group/drag-handle bg-accent rounded-md px-3 py-1'>
                          <GripVertical className='h-4 w-4 text-foreground' />
                        </div>
                      </div>
                      <div className='flex items-start justify-between gap-2'>
                        <div className='flex-1 min-w-0'>
                          <CardTitle className='text-base truncate'>{activeCard.name}</CardTitle>
                          <CardDescription className='text-xs'>
                            {activeCard.type} ({(activeCard.proxies || []).length} 个节点)
                          </CardDescription>
                        </div>
                      </div>
                    </CardHeader>
                    <CardContent className='space-y-1'>
                      {(activeCard.proxies || []).slice(0, 3).map((proxy, idx) => (
                        proxy && (
                          <div
                            key={`overlay-${proxy}-${idx}`}
                            className='flex items-center gap-2 p-2 rounded border bg-background'
                          >
                            <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                            <span className='text-sm truncate flex-1'>{proxy}</span>
                          </div>
                        )
                      ))}
                      {(activeCard.proxies || []).length > 3 && (
                        <div className='text-xs text-center text-muted-foreground py-1'>
                          还有 {(activeCard.proxies || []).length - 3} 个节点...
                        </div>
                      )}
                    </CardContent>
                  </Card>
                ) : activeGroupTitle ? (
                  <div
                    className='flex items-center gap-2 p-2 rounded border bg-background shadow-2xl pointer-events-none'
                    style={{
                      transform: 'translate(-50%, -150%)',
                      transformOrigin: 'top left'
                    }}
                  >
                    <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                    <span className='text-sm truncate'>{activeGroupTitle}</span>
                  </div>
                ) : null}
              </DragOverlay>
            </DndContext>
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

            {/* 显示/隐藏已添加节点按钮 (可选) */}
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

            {/* 配置链式代理按钮 (可选) */}
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
              {/* 名称筛选 */}
              <div className='relative flex-1'>
                <Search className='absolute left-2 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground' />
                <Input
                  placeholder='按名称筛选...'
                  value={nodeNameFilter}
                  onChange={(e) => setNodeNameFilter(e.target.value)}
                  className='pl-8 h-9 text-sm'
                />
              </div>

              {/* 标签筛选 */}
              {uniqueTags.length > 0 && (
                <Select
                  value={nodeTagFilter}
                  onValueChange={setNodeTagFilter}
                >
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

            <Card
              ref={availableNodesCardRef}
              className={`flex flex-col flex-1 transition-all duration-75 ${
                dragOverGroup === 'available'
                  ? 'ring-2 ring-primary shadow-lg scale-[1.02]'
                  : ''
              }`}
              onDragOver={(e) => e.preventDefault()}
              onDragEnter={() => onDragEnterGroup('available')}
              onDragLeave={(e) => {
                // 只有当鼠标真正离开Card容器时才清除状态
                if (availableNodesCardRef.current && !availableNodesCardRef.current.contains(e.relatedTarget as HTMLElement)) {
                  onDragLeaveGroup()
                }
              }}
              onDrop={onDropToAvailable}
            >
              <CardHeader className='pb-3 flex-shrink-0'>
                <div
                  draggable
                  onDragStart={() => onDragStart('__AVAILABLE_NODES__', 'available', -1, filteredAvailableNodes)}
                  onDragEnd={onDragEnd}
                  className='flex items-center gap-2 cursor-move rounded-md px-2 py-1 hover:bg-accent transition-colors'
                >
                  <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                  <div>
                    <CardTitle className='text-base'>可用节点</CardTitle>
                    <CardDescription className='text-xs'>
                      {filteredAvailableNodes.length} / {availableNodes.length} 个节点
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className='flex-1 overflow-y-auto space-y-1 min-h-0'>
                {filteredAvailableNodes.map((proxy, idx) => (
                  <div
                    key={`available-${proxy}-${idx}`}
                    draggable
                    onDragStart={() => onDragStart(proxy, 'available', idx)}
                    onDragEnd={onDragEnd}
                    className='flex items-center gap-2 p-2 rounded border hover:border-border hover:bg-accent cursor-move transition-colors duration-75'
                  >
                    <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                    <span className='text-sm truncate flex-1'>{proxy}</span>
                  </div>
                ))}
              </CardContent>
            </Card>
          </div>
        </div>
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
          {/* 输入框 */}
          <div>
            <Input
              placeholder='输入代理组名称...'
              value={newGroupName}
              onChange={(e) => setNewGroupName(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  handleAddGroup()
                }
              }}
            />
          </div>

          {/* 预定义选项 */}
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
