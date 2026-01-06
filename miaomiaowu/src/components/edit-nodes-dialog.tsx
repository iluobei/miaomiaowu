import React, { useState, useMemo, memo, useContext, createContext } from 'react'
import { GripVertical, X, Plus, Check, Search, Settings2, Eye, EyeOff } from 'lucide-react'
import { Twemoji } from '@/components/twemoji'
import {
  DndContext,
  DragOverlay,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
  useDraggable,
  useDroppable,
  type DragStartEvent,
  type DragEndEvent,
  pointerWithin,
  closestCenter,
  type CollisionDetection
} from '@dnd-kit/core'
import { SortableContext, rectSortingStrategy, useSortable, arrayMove } from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from '@/components/ui/dialog'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { OUTBOUND_NAMES } from '@/lib/sublink/translations'

interface ProxyGroup {
  name: string
  type: string
  proxies: string[]
  use?: string[]  // 代理集合引用
  url?: string
  interval?: number
  strategy?: 'round-robin' | 'consistent-hashing' | 'sticky-sessions'
}

interface Node {
  node_name: string
  tag?: string
  [key: string]: any
}

// 拖拽类型定义
type DragItemType = 'available-node' | 'available-header' | 'group-node' | 'group-title' | 'group-card' | 'proxy-provider' | 'use-item'

interface DragItemData {
  type: DragItemType
  nodeName?: string
  nodeNames?: string[]
  groupName?: string
  index?: number
  providerName?: string  // 代理集合名称
}

interface ActiveDragItem {
  id: string
  data: DragItemData
}

// 特殊节点列表
const SPECIAL_NODES = ['♻️ 自动选择', '🚀 节点选择', 'DIRECT', 'REJECT']

// 拖拽状态 Context - 避免 isActiveDragging 导致全量重渲染
const DragStateContext = createContext<{ isActiveDragging: boolean }>({ isActiveDragging: false })

// 代理组类型选择器 - 提取到外部
interface ProxyTypeSelectorProps {
  group: ProxyGroup
  onChange: (updatedGroup: ProxyGroup) => void
}

const ProxyTypeSelector = memo(function ProxyTypeSelector({ group, onChange }: ProxyTypeSelectorProps) {
  const types = [
    { value: 'select', label: '手动选择', hasUrl: false, hasStrategy: false },
    { value: 'url-test', label: '自动选择', hasUrl: true, hasStrategy: false },
    { value: 'fallback', label: '自动回退', hasUrl: true, hasStrategy: false },
    { value: 'load-balance', label: '负载均衡', hasUrl: true, hasStrategy: true },
  ]

  const handleTypeSelect = (type: string) => {
    const typeConfig = types.find(t => t.value === type)
    const updatedGroup: ProxyGroup = {
      ...group,
      type,
    }

    if (typeConfig?.hasUrl) {
      updatedGroup.url = group.url || 'https://www.gstatic.com/generate_204'
      updatedGroup.interval = group.interval || 300
    } else {
      delete updatedGroup.url
      delete updatedGroup.interval
    }

    if (typeConfig?.hasStrategy) {
      updatedGroup.strategy = group.strategy || 'round-robin'
    } else {
      delete updatedGroup.strategy
    }

    onChange(updatedGroup)
  }

  return (
    <div className='space-y-1'>
      {types.map(({ value, label }) => (
        <Button
          key={value}
          variant={group.type === value ? 'default' : 'ghost'}
          size='sm'
          className='w-full justify-start'
          onClick={() => handleTypeSelect(value)}
        >
          {label}
        </Button>
      ))}

      {group.type === 'load-balance' && (
        <div className='pt-2 border-t'>
          <p className='text-xs text-muted-foreground mb-1'>策略</p>
          <Select
            value={group.strategy || 'round-robin'}
            onValueChange={(value) => onChange({ ...group, strategy: value as ProxyGroup['strategy'] })}
          >
            <SelectTrigger className='h-8 text-xs'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value='round-robin'>轮询</SelectItem>
              <SelectItem value='consistent-hashing'>一致性哈希</SelectItem>
              <SelectItem value='sticky-sessions'>粘性会话</SelectItem>
            </SelectContent>
          </Select>
        </div>
      )}
    </div>
  )
})

// 快捷拖放区（添加到所有代理组）- 提取到外部
const DroppableAllGroupsZone = memo(function DroppableAllGroupsZone() {
  const { setNodeRef, isOver } = useDroppable({
    id: 'all-groups-zone',
    data: { type: 'all-groups-zone' }
  })

  return (
    <div
      ref={setNodeRef}
      className={`w-40 h-20 border-2 rounded-lg flex items-center justify-center text-sm transition-all ${
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
})

// 快捷拖放区（从所有代理组移除）- 提取到外部
const DroppableRemoveFromAllZone = memo(function DroppableRemoveFromAllZone() {
  const { setNodeRef, isOver } = useDroppable({
    id: 'remove-from-all-zone',
    data: { type: 'remove-from-all-zone' }
  })

  return (
    <div
      ref={setNodeRef}
      className={`w-40 h-20 border-2 rounded-lg flex items-center justify-center text-sm transition-all ${
        isOver
          ? 'border-destructive bg-destructive/10 border-solid'
          : 'border-dashed border-muted-foreground/30 bg-muted/20'
      }`}
    >
      <span className={isOver ? 'text-destructive font-medium' : 'text-muted-foreground'}>
        从所有代理组移除
      </span>
    </div>
  )
})

// 可用节点区域（接收从代理组拖回的节点）- 提取到外部
interface DroppableAvailableZoneProps {
  children: React.ReactNode
}

const DroppableAvailableZone = memo(function DroppableAvailableZone({ children }: DroppableAvailableZoneProps) {
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
})

// 可拖动的代理组标题 - 提取到外部
interface DraggableGroupTitleProps {
  groupName: string
  isEditing: boolean
  editingValue: string
  onEditingValueChange: (value: string) => void
  onSubmitEdit: () => void
  onCancelEdit: () => void
  onStartEdit: (groupName: string) => void
}

const DraggableGroupTitle = memo(function DraggableGroupTitle({
  groupName,
  isEditing,
  editingValue,
  onEditingValueChange,
  onSubmitEdit,
  onCancelEdit,
  onStartEdit
}: DraggableGroupTitleProps) {
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

  return (
    <div ref={setNodeRef} style={style} className='flex items-center gap-2 group/title'>
      <div {...attributes} {...listeners} className='cursor-move' style={{ touchAction: 'none' }}>
        <GripVertical className='h-3 w-3 text-muted-foreground flex-shrink-0' />
      </div>
      {isEditing ? (
        <div className='flex items-center gap-1 flex-1 min-w-0'>
          <Input
            value={editingValue}
            onChange={(e) => onEditingValueChange(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === 'Enter') onSubmitEdit()
              else if (e.key === 'Escape') onCancelEdit()
            }}
            className='h-6 text-base flex-1 min-w-0'
            placeholder='输入新名称...'
            autoFocus
          />
          <Button size='sm' className='h-6 w-6 p-0' onClick={onSubmitEdit} variant='ghost'>
            <Check className='h-3 w-3 text-green-600' />
          </Button>
        </div>
      ) : (
        <CardTitle
          className='text-base truncate cursor-text hover:text-foreground/80 flex-1 min-w-0'
          onClick={() => onStartEdit(groupName)}
          title='点击编辑名称'
        >
          <Twemoji>{groupName}</Twemoji>
        </CardTitle>
      )}
    </div>
  )
})

// 可排序的代理组卡片 - 提取到外部
interface SortableCardProps {
  group: ProxyGroup
  isEditing: boolean
  editingValue: string
  onEditingValueChange: (value: string) => void
  onSubmitEdit: () => void
  onCancelEdit: () => void
  onStartEdit: (groupName: string) => void
  onGroupTypeChange: (groupName: string, updatedGroup: ProxyGroup) => void
  onRemoveGroup: (groupName: string) => void
  onRemoveNodeFromGroup: (groupName: string, nodeIndex: number) => void
  onRemoveUseItem: (groupName: string, index: number) => void
  mmwProviderNames: Set<string>
}

const SortableCard = memo(function SortableCard({
  group,
  isEditing,
  editingValue,
  onEditingValueChange,
  onSubmitEdit,
  onCancelEdit,
  onStartEdit,
  onGroupTypeChange,
  onRemoveGroup,
  onRemoveNodeFromGroup,
  onRemoveUseItem,
  mmwProviderNames
}: SortableCardProps) {
  // 从 context 获取拖拽状态
  const { isActiveDragging } = useContext(DragStateContext)
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

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition: isDragging ? 'none' : transition,
    opacity: isDragging ? 0.5 : 1,
    // 拖拽时禁用非拖拽卡片的指针事件，避免 hover 效果触发
    pointerEvents: isActiveDragging && !isDragging ? 'none' : 'auto',
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
            isEditing ? 'cursor-not-allowed opacity-50' : 'cursor-move'
          }`}
          style={isEditing ? {} : { touchAction: 'none' }}
          {...(isEditing ? {} : attributes)}
          {...(isEditing ? {} : listeners)}
        >
          <div className={`group/drag-handle rounded-md px-3 py-1 transition-colors ${
            isEditing ? 'opacity-50' : ''
          } ${!isActiveDragging ? 'hover:bg-accent' : ''}`}>
            <GripVertical className={`h-4 w-4 text-muted-foreground transition-colors ${!isActiveDragging ? 'group-hover/drag-handle:text-foreground' : ''}`} />
          </div>
        </div>

        <div className='flex items-start justify-between gap-2'>
          <div className='flex-1 min-w-0'>
            <DraggableGroupTitle
              groupName={group.name}
              isEditing={isEditing}
              editingValue={editingValue}
              onEditingValueChange={onEditingValueChange}
              onSubmitEdit={onSubmitEdit}
              onCancelEdit={onCancelEdit}
              onStartEdit={onStartEdit}
            />
            <CardDescription className='text-xs'>
              {group.type} ({(group.proxies || []).length} 个节点{(group.use || []).length > 0 ? `, ${(group.use || []).length} 个集合` : ''})
            </CardDescription>
          </div>
          {!isEditing && (
            <div className='flex items-center gap-1'>
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    variant='ghost'
                    size='sm'
                    className='h-8 w-8 p-0 flex-shrink-0'
                    title='切换代理组类型'
                  >
                    <Settings2 className='h-4 w-4 text-muted-foreground hover:text-foreground' />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className='w-48 p-2' align='end'>
                  <ProxyTypeSelector
                    group={group}
                    onChange={(updatedGroup) => onGroupTypeChange(group.name, updatedGroup)}
                  />
                </PopoverContent>
              </Popover>
              <Button
                variant='ghost'
                size='sm'
                className='h-8 w-8 p-0 flex-shrink-0'
                onClick={(e) => {
                  e.stopPropagation()
                  onRemoveGroup(group.name)
                }}
              >
                <X className='h-4 w-4 text-muted-foreground hover:text-destructive' />
              </Button>
            </div>
          )}
        </div>
      </CardHeader>
      <CardContent className='flex-1 space-y-1 min-h-[200px]' data-card-content>
        {/* 合并 proxies 和 use 到同一个 SortableContext，解决单个 use-item 无法拖动的问题 */}
        <SortableContext
          items={[
            ...(group.proxies || []).filter(p => p).map(p => `${group.name}-${p}`),
            ...(group.use || []).map(providerName => `use-${group.name}-${providerName}`)
          ]}
          strategy={rectSortingStrategy}
        >
          {/* 普通节点 */}
          {(group.proxies || []).map((proxy, idx) => (
            proxy && (
              <SortableProxy
                key={`${group.name}-${proxy}-${idx}`}
                proxy={proxy}
                groupName={group.name}
                index={idx}
                isMmwProvider={mmwProviderNames.has(proxy)}
                onRemove={onRemoveNodeFromGroup}
              />
            )
          ))}

          {/* 代理集合（use）显示 */}
          {(group.use || []).map((providerName, idx) => (
            <SortableUseItem
              key={`use-${group.name}-${providerName}`}
              providerName={providerName}
              groupName={group.name}
              index={idx}
              onRemove={() => onRemoveUseItem(group.name, idx)}
            />
          ))}
        </SortableContext>

        {(group.proxies || []).filter(p => p).length === 0 && (group.use || []).length === 0 && (
          <div className={`text-sm text-center py-8 transition-colors ${
            isOver ? 'text-primary font-medium' : 'text-muted-foreground'
          }`}>
            将节点拖拽到这里
          </div>
        )}
      </CardContent>
    </Card>
  )
})

// 可拖动的可用节点（提取到外部并 memoize）
interface DraggableAvailableNodeProps {
  proxy: string
  index: number
}

const DraggableAvailableNode = memo(function DraggableAvailableNode({ proxy, index }: DraggableAvailableNodeProps) {
  // 从 context 获取拖拽状态
  const { isActiveDragging } = useContext(DragStateContext)
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
    touchAction: 'none',
    // 拖拽时禁用非拖拽元素的指针事件，避免 hover 效果触发
    pointerEvents: isActiveDragging && !isDragging ? 'none' : 'auto',
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
      <span className='text-sm truncate flex-1'><Twemoji>{proxy}</Twemoji></span>
    </div>
  )
})

// 可拖动的代理集合（提取到外部并 memoize）
interface DraggableProxyProviderProps {
  name: string
}

const DraggableProxyProvider = memo(function DraggableProxyProvider({ name }: DraggableProxyProviderProps) {
  const { attributes, listeners, setNodeRef, transform, isDragging } = useDraggable({
    id: `proxy-provider-${name}`,
    data: {
      type: 'proxy-provider',
      providerName: name
    } as DragItemData
  })

  const style: React.CSSProperties = {
    transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
    opacity: isDragging ? 0.5 : 1,
    touchAction: 'none',
  }

  return (
    <div
      ref={setNodeRef}
      style={style}
      {...attributes}
      {...listeners}
      className='flex items-center gap-2 p-2 rounded border border-purple-200 dark:border-purple-800 hover:border-purple-400 hover:bg-purple-50 dark:hover:bg-purple-900/30 cursor-move transition-colors duration-75'
    >
      <GripVertical className='h-4 w-4 text-purple-500 flex-shrink-0' />
      <span className='text-sm truncate flex-1 text-purple-700 dark:text-purple-300'>📦 {name}</span>
    </div>
  )
})

// 可拖动的可用节点卡片标题（批量拖动）
interface DraggableAvailableHeaderProps {
  filteredNodes: string[]
  totalNodes: number
}

const DraggableAvailableHeader = memo(function DraggableAvailableHeader({ filteredNodes, totalNodes }: DraggableAvailableHeaderProps) {
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
    touchAction: 'none',
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
})

// 可排序的代理组内节点 - 提取到外部并 memoize
interface SortableProxyProps {
  proxy: string
  groupName: string
  index: number
  isMmwProvider: boolean
  onRemove: (groupName: string, index: number) => void
}

const SortableProxy = memo(function SortableProxy({
  proxy,
  groupName,
  index,
  isMmwProvider,
  onRemove
}: SortableProxyProps) {
  // 从 context 获取拖拽状态
  const { isActiveDragging } = useContext(DragStateContext)
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
    isOver,
  } = useSortable({
    id: `${groupName}-${proxy}`,
    transition: {
      duration: 150,
      easing: 'ease-out',
    },
    data: {
      type: isMmwProvider ? 'use-item' : 'group-node',
      groupName,
      nodeName: proxy,
      providerName: isMmwProvider ? proxy : undefined,
      index
    } as DragItemData,
  })

  // 判断是否显示插入指示器：有拖拽进行中 + 当前项被悬停 + 当前项不是正在拖拽的项
  const showDropIndicator = isActiveDragging && isOver && !isDragging

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition: transition || 'transform 150ms ease-out',
    opacity: isDragging ? 0.5 : 1,
    touchAction: 'none',
    // 拖拽时禁用非拖拽元素的指针事件，避免 hover 效果触发
    pointerEvents: isActiveDragging && !isDragging ? 'none' : 'auto',
  }

  // MMW 代理集合使用紫色样式
  if (isMmwProvider) {
    return (
      <div className='relative' style={{ pointerEvents: isActiveDragging && !isDragging ? 'none' : 'auto' }}>
        {showDropIndicator && (
          <div className='absolute -top-0.5 left-0 right-0 h-1 bg-blue-500 rounded-full z-10' />
        )}
        <div
          ref={setNodeRef}
          style={style}
          {...attributes}
          {...listeners}
          className={`flex items-center gap-2 p-2 rounded border border-purple-300 dark:border-purple-700 bg-purple-50 dark:bg-purple-950/20 cursor-move ${
            showDropIndicator ? 'border-blue-400 bg-blue-100 dark:bg-blue-950/30' : ''
          } ${isDragging ? 'shadow-lg' : ''}`}
          data-use-item
        >
          <GripVertical className='h-4 w-4 text-purple-500 flex-shrink-0' />
          <span className='text-sm truncate flex-1 text-purple-700 dark:text-purple-300'>📦 {proxy}</span>
          <Button
            variant='ghost'
            size='sm'
            className='h-6 w-6 p-0 flex-shrink-0'
            onPointerDown={(e) => e.stopPropagation()}
            onTouchStart={(e) => e.stopPropagation()}
            onClick={(e) => {
              e.stopPropagation()
              onRemove(groupName, index)
            }}
          >
            <X className='h-4 w-4 text-purple-400 hover:text-destructive' />
          </Button>
        </div>
      </div>
    )
  }

  return (
    <div className='relative' style={{ pointerEvents: isActiveDragging && !isDragging ? 'none' : 'auto' }}>
      {/* 顶部插入指示器 */}
      {showDropIndicator && (
        <div className='absolute -top-0.5 left-0 right-0 h-1 bg-blue-500 rounded-full z-10' />
      )}
      <div
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        className={`flex items-center gap-2 p-2 rounded border hover:border-border hover:bg-accent group/item cursor-move ${
          showDropIndicator ? 'border-blue-400 bg-blue-50 dark:bg-blue-950/30' : ''
        }`}
        data-proxy-item
      >
        <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
        <span className='text-sm truncate flex-1'><Twemoji>{proxy}</Twemoji></span>
        <Button
          variant='ghost'
          size='sm'
          className='h-6 w-6 p-0 flex-shrink-0'
          onPointerDown={(e) => e.stopPropagation()}
          onTouchStart={(e) => e.stopPropagation()}
          onClick={(e) => {
            e.stopPropagation()
            onRemove(groupName, index)
          }}
        >
          <X className='h-4 w-4 text-muted-foreground hover:text-destructive' />
        </Button>
      </div>
    </div>
  )
})

// 可排序的代理集合项 - 提取到外部并 memoize
interface SortableUseItemProps {
  providerName: string
  groupName: string
  index: number
  onRemove: () => void
}

const SortableUseItem = memo(function SortableUseItem({
  providerName,
  groupName,
  index,
  onRemove
}: SortableUseItemProps) {
  // 从 context 获取拖拽状态
  const { isActiveDragging } = useContext(DragStateContext)
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
    isOver
  } = useSortable({
    id: `use-${groupName}-${providerName}`,
    transition: {
      duration: 150,
      easing: 'ease-out',
    },
    data: {
      type: 'use-item',
      groupName,
      providerName,
      index
    } as DragItemData,
  })

  // 判断是否显示插入指示器
  const showDropIndicator = isActiveDragging && isOver && !isDragging

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition: transition || 'transform 150ms ease-out',
    opacity: isDragging ? 0.5 : 1,
    touchAction: 'none',
    // 拖拽时禁用非拖拽元素的指针事件，避免 hover 效果触发
    pointerEvents: isActiveDragging && !isDragging ? 'none' : 'auto',
  }

  return (
    <div className='relative' style={{ pointerEvents: isActiveDragging && !isDragging ? 'none' : 'auto' }}>
      {/* 顶部插入指示器 */}
      {showDropIndicator && (
        <div className='absolute -top-0.5 left-0 right-0 h-1 bg-blue-500 rounded-full z-10' />
      )}
      <div
        ref={setNodeRef}
        style={style}
        {...attributes}
        {...listeners}
        className={`flex items-center gap-2 p-2 rounded border border-purple-300 dark:border-purple-700 bg-purple-50 dark:bg-purple-950/20 cursor-move ${
          showDropIndicator ? 'border-blue-400 bg-blue-100 dark:bg-blue-950/30' : ''
        } ${isDragging ? 'shadow-lg' : ''}`}
        data-use-item
      >
        <GripVertical className='h-4 w-4 text-purple-500 flex-shrink-0' />
        <span className='text-sm truncate flex-1 text-purple-700 dark:text-purple-300'>📦 {providerName}</span>
        <Button
          variant='ghost'
          size='sm'
          className='h-6 w-6 p-0 flex-shrink-0'
          onPointerDown={(e) => e.stopPropagation()}
          onTouchStart={(e) => e.stopPropagation()}
          onClick={(e) => {
            e.stopPropagation()
            onRemove()
          }}
        >
          <X className='h-4 w-4 text-purple-400 hover:text-destructive' />
        </Button>
      </div>
    </div>
  )
})

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
  showSpecialNodesAtBottom?: boolean  // 是否在底部显示特殊节点
  proxyProviderConfigs?: Array<{ id: number; name: string; process_mode?: string }>  // 代理集合配置列表
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
  showSpecialNodesAtBottom = false,
  proxyProviderConfigs = [],
  onRemoveNodeFromGroup,
  onRemoveGroup,
  onRenameGroup
}: EditNodesDialogProps) {
  // 添加代理组对话框状态
  const [addGroupDialogOpen, setAddGroupDialogOpen] = useState(false)
  const [newGroupName, setNewGroupName] = useState('')

  // 检查新代理组名称是否与现有组冲突
  const isGroupNameDuplicate = useMemo(() => {
    const trimmedName = newGroupName.trim()
    if (!trimmedName) return false
    return proxyGroups.some(group => group.name === trimmedName)
  }, [newGroupName, proxyGroups])

  // 代理组改名状态
  const [editingGroupName, setEditingGroupName] = useState<string | null>(null)
  const [editingGroupValue, setEditingGroupValue] = useState('')

  // 节点筛选状态
  const [nodeNameFilter, setNodeNameFilter] = useState('')
  const [nodeTagFilter, setNodeTagFilter] = useState<string>('all')

  // 统一的拖拽状态
  const [activeDragItem, setActiveDragItem] = useState<ActiveDragItem | null>(null)

  // 保存滚动位置
  const scrollContainerRef = React.useRef<HTMLDivElement>(null)
  const availableNodesScrollRef = React.useRef<HTMLDivElement>(null)

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

  // MMW 模式代理集合名称集合（用于识别 proxies 中的代理集合引用）
  const mmwProviderNames = useMemo(() => {
    return new Set(
      proxyProviderConfigs
        .filter(c => c.process_mode === 'mmw')
        .map(c => c.name)
    )
  }, [proxyProviderConfigs])

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

  // 统一的传感器配置 - 同时支持鼠标和触摸
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: {
        distance: 5,
      },
    }),
    useSensor(TouchSensor, {
      activationConstraint: {
        delay: 100,
        tolerance: 5,
      },
    })
  )

  // 自定义碰撞检测 - 优先使用指针检测，然后使用最近中心点
  const customCollisionDetection: CollisionDetection = React.useCallback((args) => {
    // 先尝试指针检测
    const pointerCollisions = pointerWithin(args)
    if (pointerCollisions.length > 0) {
      return pointerCollisions
    }
    // 回退到最近中心点检测（比矩形相交更精确）
    return closestCenter(args)
  }, [])

  // 统一的拖拽开始处理
  const handleDragStart = (event: DragStartEvent) => {
    const { active } = event
    const data = active.data.current as DragItemData

    // 锁定 body 滚动，防止 iPad 上拖拽时背景滚动
    document.body.style.overflow = 'hidden'
    document.body.style.touchAction = 'none'

    setActiveDragItem({
      id: String(active.id),
      data
    })
  }

  // 统一的拖拽结束处理
  const handleDragEnd = (event: DragEndEvent) => {
    const { active, over } = event

    // 保存可用节点列表的滚动位置
    const availableNodesScrollTop = availableNodesScrollRef.current?.scrollTop ?? 0

    // 恢复 body 滚动
    document.body.style.overflow = ''
    document.body.style.touchAction = ''

    setActiveDragItem(null)

    // 恢复可用节点列表的滚动位置
    const restoreAvailableNodesScroll = () => {
      requestAnimationFrame(() => {
        if (availableNodesScrollRef.current) {
          availableNodesScrollRef.current.scrollTop = availableNodesScrollTop
        }
      })
    }

    if (!over) return

    const activeData = active.data.current as DragItemData
    const overId = String(over.id)
    const overData = over.data.current as DragItemData | { type?: string; groupName?: string } | undefined

    // 获取目标代理组名称
    const getTargetGroupName = (): string | null => {
      if (overId === 'all-groups-zone') return 'all-groups'
      if (overId === 'remove-from-all-zone') return 'remove-from-all'
      if (overId === 'available-zone') return 'available'
      if (overId.startsWith('drop-')) return overId.replace('drop-', '')
      // 优先从 overData 中获取 groupName（适用于 group-node、use-item 等）
      if (overData?.groupName) return overData.groupName
      // 检查是否放在了某个代理组的节点上（排除 available-node、group-title）
      if (overId.includes('-') && !overId.startsWith('available-node-') && !overId.startsWith('group-title-')) {
        // 找到对应的代理组
        const groupName = proxyGroups.find(g => overId.startsWith(`${g.name}-`))?.name
        if (groupName) return groupName
      }
      return null
    }

    // 计算在目标代理组中的插入位置
    const getInsertIndex = (group: ProxyGroup): number => {
      // 如果 overData 包含 index 信息（放在了某个节点或 use-item 上）
      if (overData && 'index' in overData && typeof overData.index === 'number' && overData.groupName === group.name) {
        // 如果是 use-item，index 已经是正确的位置（proxies.length + use 的 index）
        // 但我们需要将节点插入到 proxies 末尾
        if (overData.type === 'use-item') {
          return group.proxies.length
        }
        return overData.index
      }
      // 否则插入到末尾
      return group.proxies.length
    }

    // 计算在目标代理组 use 数组中的插入位置
    const getUseInsertIndex = (group: ProxyGroup): number => {
      const currentUse = group.use || []
      // 如果 overData 是 use-item 且在同一代理组
      if (overData && 'type' in overData && overData.type === 'use-item' &&
          'index' in overData && typeof overData.index === 'number' &&
          overData.groupName === group.name) {
        // use-item 的 index 已经是 use 数组内的索引
        return Math.max(0, Math.min(overData.index, currentUse.length))
      }
      // 如果 overData 是 group-node，插入到 use 数组末尾（紧跟在普通节点后面）
      if (overData && 'type' in overData && overData.type === 'group-node' &&
          overData.groupName === group.name) {
        return 0
      }
      // 否则插入到末尾
      return currentUse.length
    }

    switch (activeData.type) {
      case 'available-node': {
        // 从可用节点拖到代理组
        const targetGroup = getTargetGroupName()
        if (!targetGroup || targetGroup === 'available') return

        const nodeName = activeData.nodeName!

        if (targetGroup === 'remove-from-all') {
          // 从所有代理组移除该节点
          const updatedGroups = proxyGroups.map(group => {
            if (group.proxies.includes(nodeName)) {
              return { ...group, proxies: group.proxies.filter(p => p !== nodeName) }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        } else if (targetGroup === 'all-groups') {
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
              // 使用 getInsertIndex 计算插入位置
              const insertIndex = getInsertIndex(group)

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

        if (targetGroup === 'remove-from-all') {
          // 批量从所有代理组移除
          const nodeNamesToRemove = new Set(nodeNames)
          const updatedGroups = proxyGroups.map(group => {
            const newProxies = group.proxies.filter(p => !nodeNamesToRemove.has(p))
            if (newProxies.length !== group.proxies.length) {
              return { ...group, proxies: newProxies }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        } else if (targetGroup === 'all-groups') {
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
                // 使用 getInsertIndex 计算插入位置
                const insertIndex = getInsertIndex(group)
                const newProxies = [...group.proxies]
                newProxies.splice(insertIndex, 0, ...newNodes)
                return { ...group, proxies: newProxies }
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

        if (targetGroup === 'remove-from-all') {
          // 从所有代理组移除该节点
          const nodeName = activeData.nodeName!
          const updatedGroups = proxyGroups.map(group => {
            if (group.proxies.includes(nodeName)) {
              return { ...group, proxies: group.proxies.filter(p => p !== nodeName) }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
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
        } else if (targetGroup === 'all-groups') {
          // 添加到所有代理组
          const nodeName = activeData.nodeName!
          const updatedGroups = proxyGroups.map(group => {
            if (group.name !== nodeName && !group.proxies.includes(nodeName)) {
              return { ...group, proxies: [...group.proxies, nodeName] }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        } else {
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
              // 使用 getInsertIndex 计算插入位置
              const insertIndex = getInsertIndex(group)
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
              // 使用 getInsertIndex 计算插入位置
              const insertIndex = getInsertIndex(group)
              const newProxies = [...group.proxies]
              newProxies.splice(insertIndex, 0, sourceGroupName)
              return { ...group, proxies: newProxies }
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

      case 'proxy-provider': {
        // 代理集合拖到代理组
        const providerName = activeData.providerName!
        const targetGroup = getTargetGroupName()

        if (!targetGroup || targetGroup === 'available' || targetGroup === 'remove-from-all') return

        if (targetGroup === 'all-groups') {
          // 添加到所有代理组的 use 数组
          const updatedGroups = proxyGroups.map(group => {
            const currentUse = group.use || []
            if (!currentUse.includes(providerName)) {
              return { ...group, use: [...currentUse, providerName] }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        } else {
          // 添加到指定代理组的 use 数组
          const updatedGroups = proxyGroups.map(group => {
            if (group.name === targetGroup) {
              const currentUse = group.use || []
              if (!currentUse.includes(providerName)) {
                // 使用 getUseInsertIndex 计算插入位置
                const insertIndex = getUseInsertIndex(group)
                const newUse = [...currentUse]
                newUse.splice(insertIndex, 0, providerName)
                return { ...group, use: newUse }
              }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        }
        break
      }

      case 'use-item': {
        // use-item 拖放处理
        const sourceGroup = activeData.groupName!
        const sourceProviderName = activeData.providerName!
        const targetGroup = getTargetGroupName()

        if (!targetGroup) return

        // 同一代理组内排序
        if (targetGroup === sourceGroup && overData && 'type' in overData) {
          const group = proxyGroups.find(g => g.name === sourceGroup)
          if (!group?.use) break

          const oldIndex = group.use.indexOf(sourceProviderName)
          if (oldIndex === -1) break

          let newIndex: number
          if (overData.type === 'use-item' && 'providerName' in overData) {
            newIndex = group.use.indexOf((overData as any).providerName)
          } else if (overData.type === 'group-node' && 'index' in overData) {
            // 放在某个节点上，移动到 use 数组开头
            newIndex = 0
          } else {
            break
          }

          if (newIndex !== -1 && oldIndex !== newIndex) {
            const updatedGroups = proxyGroups.map(g => {
              if (g.name === sourceGroup && g.use) {
                return { ...g, use: arrayMove(g.use, oldIndex, newIndex) }
              }
              return g
            })
            onProxyGroupsChange(updatedGroups)
          }
        }
        // 跨代理组移动 use-item
        else if (targetGroup !== sourceGroup && targetGroup !== 'available' && targetGroup !== 'remove-from-all') {
          const updatedGroups = proxyGroups.map(group => {
            if (group.name === sourceGroup && group.use) {
              // 从源组移除
              return { ...group, use: group.use.filter(u => u !== sourceProviderName) }
            }
            if (group.name === targetGroup) {
              const currentUse = group.use || []
              if (!currentUse.includes(sourceProviderName)) {
                const insertIndex = getUseInsertIndex(group)
                const newUse = [...currentUse]
                newUse.splice(insertIndex, 0, sourceProviderName)
                return { ...group, use: newUse }
              }
            }
            return group
          })
          onProxyGroupsChange(updatedGroups)
        }
        break
      }
    }

    // 恢复可用节点列表的滚动位置
    restoreAvailableNodesScroll()
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

  // 代理组类型变更处理
  const handleGroupTypeChange = React.useCallback((groupName: string, updatedGroup: ProxyGroup) => {
    const updatedGroups = proxyGroups.map(g =>
      g.name === groupName ? updatedGroup : g
    )
    onProxyGroupsChange(updatedGroups)
  }, [proxyGroups, onProxyGroupsChange])

  // 移除 use-item 的回调
  const handleRemoveUseItem = React.useCallback((groupName: string, index: number) => {
    const updatedGroups = proxyGroups.map(g => {
      if (g.name === groupName) {
        const newUse = (g.use || []).filter((_, i) => i !== index)
        return { ...g, use: newUse.length > 0 ? newUse : undefined }
      }
      return g
    })
    onProxyGroupsChange(updatedGroups)
  }, [proxyGroups, onProxyGroupsChange])

  // Context 值 - 使用 useMemo 避免不必要的重渲染
  const dragStateValue = useMemo(() => ({
    isActiveDragging: !!activeDragItem
  }), [activeDragItem])

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
            <DragStateContext.Provider value={dragStateValue}>
            <DialogHeader>
              <div className='flex items-start justify-between gap-4'>
                <div className='flex-1'>
                  <DialogTitle>{title}</DialogTitle>
                  <DialogDescription>{description}</DialogDescription>
                  <p className='mt-2 text-sm text-primary flex flex-wrap items-center gap-1'>
                    <GripVertical className='h-4 w-4 inline' /> 为可拖动元素，
                    <Settings2 className='h-4 w-4 inline' /> 切换代理组类型、双击代理组标题编辑代理组名称，拖动可用节点标题时，代表拖动可用节点内的所有节点
                  </p>
                </div>
                {/* 快捷拖放区 */}
                <div className='flex gap-2 mr-9'>
                  <DroppableRemoveFromAllZone />
                  <DroppableAllGroupsZone />
                </div>
              </div>
            </DialogHeader>

            <div className='flex-1 flex gap-4 py-4 min-h-0'>
              {/* 左侧：代理组 */}
              <div ref={scrollContainerRef} className='flex-1 overflow-y-auto pr-2'>
                <SortableContext
                  items={proxyGroups.map(g => g.name)}
                  strategy={rectSortingStrategy}
                >
                  <div className='grid gap-4 pt-1' style={{ gridTemplateColumns: 'repeat(auto-fill, minmax(240px, 1fr))' }}>
                    {proxyGroups.map((group) => (
                      <SortableCard
                        key={group.name}
                        group={group}
                        isEditing={editingGroupName === group.name}
                        editingValue={editingGroupValue}
                        onEditingValueChange={setEditingGroupValue}
                        onSubmitEdit={submitEditingGroup}
                        onCancelEdit={cancelEditingGroup}
                        onStartEdit={startEditingGroup}
                        onGroupTypeChange={handleGroupTypeChange}
                        onRemoveGroup={wrappedRemoveGroup}
                        onRemoveNodeFromGroup={wrappedRemoveNodeFromGroup}
                        onRemoveUseItem={handleRemoveUseItem}
                        mmwProviderNames={mmwProviderNames}
                      />
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

                {/* 隐藏/显示已添加节点按钮 */}
                {showAllNodes !== undefined && onShowAllNodesChange && (
                  <div className='flex-shrink-0 mb-4'>
                    <Button
                      variant='outline'
                      className='w-full relative'
                      onClick={() => onShowAllNodesChange(!showAllNodes)}
                    >
                      {showAllNodes ? <Eye className='h-4 w-4 mr-2' /> : <EyeOff className='h-4 w-4 mr-2' />}
                      {showAllNodes ? '显示已添加节点' : '隐藏已添加节点'}
                      {!showAllNodes && (
                        <span className='absolute -top-1 -right-1 h-4 w-4 bg-green-500 rounded-full flex items-center justify-center'>
                          <Check className='h-3 w-3 text-white' />
                        </span>
                      )}
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

                  {(uniqueTags.length > 0 || showSpecialNodesAtBottom || proxyProviderConfigs.length > 0) && (
                    <Select value={nodeTagFilter} onValueChange={setNodeTagFilter}>
                      <SelectTrigger className='h-9 text-sm w-[120px]'>
                        <SelectValue placeholder='所有标签' />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='all'>所有</SelectItem>
                        {uniqueTags.map(tag => (
                          <SelectItem key={tag} value={tag}>
                            {tag}
                          </SelectItem>
                        ))}
                        {showSpecialNodesAtBottom && (
                          <SelectItem value='__special__'>特殊节点</SelectItem>
                        )}
                        {proxyProviderConfigs.length > 0 && (
                          <SelectItem value='__provider__'>代理集合</SelectItem>
                        )}
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
                  <CardContent ref={availableNodesScrollRef} className='flex-1 overflow-y-auto space-y-1 min-h-0'>
                    {/* 普通节点 - 仅在非特殊筛选时显示 */}
                    {nodeTagFilter !== '__special__' && nodeTagFilter !== '__provider__' && (
                      filteredAvailableNodes.map((proxy, idx) => (
                        <DraggableAvailableNode
                          key={`available-${proxy}-${idx}`}
                          proxy={proxy}
                          index={idx}
                        />
                      ))
                    )}

                    {/* 代理集合区块 */}
                    {proxyProviderConfigs.length > 0 && (nodeTagFilter === 'all' || nodeTagFilter === '__provider__') && (
                      <>
                        {nodeTagFilter === 'all' && (
                          <div className='pt-3 pb-1 border-t mt-3'>
                            <span className='text-xs text-purple-600 dark:text-purple-400 font-medium'>📦 代理集合</span>
                          </div>
                        )}
                        {proxyProviderConfigs.map((config) => (
                          <DraggableProxyProvider
                            key={`provider-${config.id}`}
                            name={config.name}
                          />
                        ))}
                      </>
                    )}

                    {/* 特殊节点区块 */}
                    {showSpecialNodesAtBottom && (nodeTagFilter === 'all' || nodeTagFilter === '__special__') && (
                      <>
                        {nodeTagFilter === 'all' && (
                          <div className='pt-3 pb-1 border-t mt-3'>
                            <span className='text-xs text-muted-foreground font-medium'>特殊节点</span>
                          </div>
                        )}
                        {SPECIAL_NODES.map((node, idx) => (
                          <DraggableAvailableNode
                            key={`special-${node}-${idx}`}
                            proxy={node}
                            index={availableNodes.length + idx}
                          />
                        ))}
                      </>
                    )}
                  </CardContent>
                </DroppableAvailableZone>
              </div>
            </div>

            {/* DragOverlay */}
            <DragOverlay dropAnimation={null} style={{ cursor: 'grabbing' }}>
              {activeDragItem?.data.type === 'available-node' && (
                <div className='flex items-center gap-2 p-2 rounded border bg-background shadow-2xl pointer-events-none'>
                  <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                  <span className='text-sm truncate'><Twemoji>{activeDragItem.data.nodeName}</Twemoji></span>
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
                  <span className='text-sm truncate'><Twemoji>{activeDragItem.data.nodeName}</Twemoji></span>
                </div>
              )}
              {activeDragItem?.data.type === 'group-title' && (
                <div className='flex items-center gap-2 p-2 rounded border bg-background shadow-2xl pointer-events-none'>
                  <GripVertical className='h-4 w-4 text-muted-foreground flex-shrink-0' />
                  <span className='text-sm truncate'><Twemoji>{activeDragItem.data.groupName}</Twemoji></span>
                </div>
              )}
              {activeDragItem?.data.type === 'proxy-provider' && (
                <div className='flex items-center gap-2 p-2 rounded border border-purple-400 bg-purple-50 dark:bg-purple-950/50 shadow-2xl pointer-events-none'>
                  <GripVertical className='h-4 w-4 text-purple-500 flex-shrink-0' />
                  <span className='text-sm truncate text-purple-700 dark:text-purple-300'>📦 {activeDragItem.data.providerName}</span>
                </div>
              )}
              {activeDragItem?.data.type === 'use-item' && (
                <div className='flex items-center gap-2 p-2 rounded border border-purple-400 bg-purple-50 dark:bg-purple-950/50 shadow-2xl pointer-events-none'>
                  <GripVertical className='h-4 w-4 text-purple-500 flex-shrink-0' />
                  <span className='text-sm truncate text-purple-700 dark:text-purple-300'>📦 {activeDragItem.data.providerName}</span>
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
                          <CardTitle className='text-base truncate'><Twemoji>{activeDragItem.data.groupName}</Twemoji></CardTitle>
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
                          <span className='text-sm truncate flex-1'><Twemoji>{proxy}</Twemoji></span>
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
            </DragOverlay>
            </DragStateContext.Provider>
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
                  if (e.key === 'Enter' && !isGroupNameDuplicate) handleAddGroup()
                }}
                className={isGroupNameDuplicate ? 'border-destructive' : ''}
              />
              {isGroupNameDuplicate && (
                <p className='text-sm text-destructive mt-1'>已存在同名代理组</p>
              )}
            </div>

            <div>
              <p className='text-sm text-muted-foreground mb-2'>快速选择：</p>
              <div className='grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 gap-2'>
                {Object.entries(OUTBOUND_NAMES).map(([key, value]) => {
                  const isDuplicate = proxyGroups.some(g => g.name === value)
                  return (
                    <Button
                      key={key}
                      variant='outline'
                      size='sm'
                      className={`justify-start text-left h-auto py-2 px-3 ${isDuplicate ? 'opacity-50' : ''}`}
                      onClick={() => handleQuickSelect(value)}
                      disabled={isDuplicate}
                    >
                      <span className='truncate'>{value}</span>
                    </Button>
                  )
                })}
              </div>
            </div>
          </div>

          <DialogFooter>
            <Button variant='outline' onClick={() => setAddGroupDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={handleAddGroup} disabled={!newGroupName.trim() || isGroupNameDuplicate}>
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}
