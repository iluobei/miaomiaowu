import React from 'react'
import { GripVertical, X } from 'lucide-react'
import { DndContext, DragOverlay, PointerSensor, closestCenter, useSensor, useSensors, useDraggable, useDroppable } from '@dnd-kit/core'
import { SortableContext, rectSortingStrategy, useSortable } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from '@/components/ui/dialog'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'

interface ProxyGroup {
  name: string
  type: string
  proxies: string[]
}

interface EditNodesDialogProps {
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
  draggedNode: { name: string; fromGroup: string | null; fromIndex: number } | null
  onDragStart: (nodeName: string, fromGroup: string | null, fromIndex: number) => void
  onDragEnd: () => void
  dragOverGroup: string | null
  onDragEnterGroup: (groupName: string) => void
  onDragLeaveGroup: () => void
  onDrop: (toGroup: string) => void
  onDropToAvailable: () => void
  onRemoveNodeFromGroup: (groupName: string, nodeIndex: number) => void
  onRemoveGroup: (groupName: string) => void
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
  handleCardDragStart,
  handleCardDragEnd,
  handleNodeDragEnd,
  activeGroupTitle,
  activeCard,
  onConfigureChainProxy,
  cancelButtonText = '取消',
  saveButtonText = '保存'
}: EditNodesDialogProps) {
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 8,
      },
    })
  )

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
            onRemoveNodeFromGroup(groupName, index)
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
      opacity: isDragging ? 0 : 1,
    }

    return (
      <div
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        className='flex items-center gap-2 cursor-move group/title'
      >
        <GripVertical className='h-3 w-3 text-muted-foreground flex-shrink-0' />
        <CardTitle className='text-base truncate'>{groupName}</CardTitle>
      </div>
    )
  }

  // 可排序的卡片组件
  interface SortableCardProps {
    group: ProxyGroup
  }

  const SortableCard = ({ group }: SortableCardProps) => {
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

    return (
      <Card
        ref={(node) => {
          setNodeRef(node)
          setDropRef(node)
        }}
        style={style}
        className={`flex flex-col transition-all ${
          isOver
            ? 'ring-2 ring-primary shadow-lg scale-[1.02]'
            : ''
        }`}
        onDragOver={(e) => e.preventDefault()}
        onDragEnter={() => onDragEnterGroup(group.name)}
        onDragLeave={onDragLeaveGroup}
        onDrop={() => onDrop(group.name)}
      >
        <CardHeader className='pb-3' {...attributes} {...listeners}>
          {/* 顶部居中拖动按钮 */}
          <div className='flex justify-center -mt-2 mb-2 cursor-move touch-none'>
            <div className='group/drag-handle hover:bg-accent rounded-md px-3 py-1 transition-colors'>
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
                onRemoveGroup(group.name)
              }}
            >
              <X className='h-4 w-4 text-muted-foreground hover:text-destructive' />
            </Button>
          </div>
        </CardHeader>
        <CardContent className='flex-1 space-y-1 min-h-[200px]'>
          <DndContext
            sensors={sensors}
            collisionDetection={closestCenter}
            onDragEnd={handleNodeDragEnd(group.name)}
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
              className={`w-48 h-20 mr-9 border-2 rounded-lg flex items-center justify-center text-sm transition-all ${
                dragOverGroup === 'all-groups'
                  ? 'border-primary bg-primary/10 border-solid'
                  : 'border-dashed border-muted-foreground/30 bg-muted/20'
              }`}
              onDragOver={(e) => e.preventDefault()}
              onDragEnter={() => onDragEnterGroup('all-groups')}
              onDragLeave={onDragLeaveGroup}
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
          <div className='flex-1 overflow-y-auto pr-2'>
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
                <Button variant='outline' onClick={() => onOpenChange(false)} className='flex-1'>
                  {cancelButtonText}
                </Button>
                <Button onClick={onSave} className='flex-1' disabled={isSaving}>
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

            <Card
              className={`flex flex-col flex-1 transition-all duration-75 ${
                dragOverGroup === 'available'
                  ? 'ring-2 ring-primary shadow-lg scale-[1.02]'
                  : ''
              }`}
              onDragOver={(e) => e.preventDefault()}
              onDragEnter={() => onDragEnterGroup('available')}
              onDragLeave={onDragLeaveGroup}
              onDrop={onDropToAvailable}
            >
              <CardHeader className='pb-3 flex-shrink-0'>
                <div
                  draggable
                  onDragStart={() => onDragStart('__AVAILABLE_NODES__', 'available', -1)}
                  onDragEnd={onDragEnd}
                  className='flex items-center gap-2 cursor-move rounded-md px-2 py-1 hover:bg-accent transition-colors'
                >
                  <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                  <div>
                    <CardTitle className='text-base'>可用节点</CardTitle>
                    <CardDescription className='text-xs'>
                      {availableNodes.length} 个节点
                    </CardDescription>
                  </div>
                </div>
              </CardHeader>
              <CardContent className='flex-1 overflow-y-auto space-y-1 min-h-0'>
                {availableNodes.map((proxy, idx) => (
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
  )
}
