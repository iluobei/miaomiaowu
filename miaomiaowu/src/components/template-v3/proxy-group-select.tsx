import { useState, useEffect, useRef } from 'react'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { Command, CommandEmpty, CommandGroup, CommandInput, CommandItem, CommandList } from '@/components/ui/command'
import { Check, ChevronsUpDown, GripVertical, X, Server, Database } from 'lucide-react'
import { cn } from '@/lib/utils'
import { PROXY_NODES_MARKER, PROXY_PROVIDERS_MARKER } from '@/lib/template-v3-utils'
import {
  DndContext,
  DragOverlay,
  closestCenter,
  KeyboardSensor,
  PointerSensor,
  useSensor,
  useSensors,
  type DragEndEvent,
  type DragStartEvent,
  type DragOverEvent,
} from '@dnd-kit/core'
import {
  arrayMove,
  SortableContext,
  sortableKeyboardCoordinates,
  useSortable,
  rectSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'

interface ProxyGroupSelectProps {
  label: string
  value: string[]
  onChange: (value: string[]) => void
  availableGroups: string[]
  showNodesMarker?: boolean
  showProvidersMarker?: boolean
  placeholder?: string
}

interface SortableItemProps {
  id: string
  onRemove?: (id: string) => void
}

function SortableItem({ id, onRemove }: SortableItemProps) {
  const { attributes, listeners, setNodeRef, transform, transition, isDragging } = useSortable({ id })
  const isMarker = id === PROXY_NODES_MARKER || id === PROXY_PROVIDERS_MARKER

  const style: React.CSSProperties = {
    transform: CSS.Transform.toString(transform),
    transition: transition || 'transform 200ms ease',
    opacity: isDragging ? 0.4 : 1,
    zIndex: isDragging ? 1 : 0,
  }

  const getDisplayName = () => {
    if (id === PROXY_NODES_MARKER) return '代理节点'
    if (id === PROXY_PROVIDERS_MARKER) return '代理集合'
    return id
  }

  const getIcon = () => {
    if (id === PROXY_NODES_MARKER) return <Server className="h-3 w-3 mr-1" />
    if (id === PROXY_PROVIDERS_MARKER) return <Database className="h-3 w-3 mr-1" />
    return null
  }

  const bgClass = isMarker
    ? id === PROXY_NODES_MARKER
      ? 'bg-blue-100 dark:bg-blue-900/30 border border-blue-300 dark:border-blue-700'
      : 'bg-green-100 dark:bg-green-900/30 border border-green-300 dark:border-green-700'
    : 'bg-secondary'

  return (
    <div
      ref={setNodeRef}
      style={style}
      className={cn('flex items-center gap-1 rounded-md px-2 py-1', bgClass)}
    >
      <GripVertical className="h-3 w-3 cursor-grab text-muted-foreground" {...attributes} {...listeners} />
      {getIcon()}
      <span className="text-sm">{getDisplayName()}</span>
      {!isMarker && onRemove && (
        <Button
          variant="ghost"
          size="icon"
          className="h-4 w-4 p-0 hover:bg-transparent"
          onClick={() => onRemove(id)}
        >
          <X className="h-3 w-3" />
        </Button>
      )}
    </div>
  )
}

function DragOverlayItem({ id }: { id: string }) {
  const isMarker = id === PROXY_NODES_MARKER || id === PROXY_PROVIDERS_MARKER

  const getDisplayName = () => {
    if (id === PROXY_NODES_MARKER) return '代理节点'
    if (id === PROXY_PROVIDERS_MARKER) return '代理集合'
    return id
  }

  const getIcon = () => {
    if (id === PROXY_NODES_MARKER) return <Server className="h-3 w-3 mr-1" />
    if (id === PROXY_PROVIDERS_MARKER) return <Database className="h-3 w-3 mr-1" />
    return null
  }

  const bgClass = isMarker
    ? id === PROXY_NODES_MARKER
      ? 'bg-blue-100 dark:bg-blue-900/30 border border-blue-300 dark:border-blue-700'
      : 'bg-green-100 dark:bg-green-900/30 border border-green-300 dark:border-green-700'
    : 'bg-secondary'

  return (
    <div className={cn('flex items-center gap-1 rounded-md px-2 py-1 shadow-lg', bgClass)}>
      <GripVertical className="h-3 w-3 cursor-grab text-muted-foreground" />
      {getIcon()}
      <span className="text-sm">{getDisplayName()}</span>
    </div>
  )
}

export function ProxyGroupSelect({
  label,
  value,
  onChange,
  availableGroups,
  showNodesMarker = false,
  showProvidersMarker = false,
  placeholder = '选择代理组',
}: ProxyGroupSelectProps) {
  const [open, setOpen] = useState(false)
  const [activeId, setActiveId] = useState<string | null>(null)
  // Internal state for drag operations to avoid frequent parent re-renders
  const [internalOrder, setInternalOrder] = useState<string[]>(value)
  const isDraggingRef = useRef(false)

  // Sync internal state with external value when not dragging
  useEffect(() => {
    if (!isDraggingRef.current) {
      setInternalOrder(value)
    }
  }, [value])

  const sensors = useSensors(
    useSensor(PointerSensor),
    useSensor(KeyboardSensor, {
      coordinateGetter: sortableKeyboardCoordinates,
    })
  )

  const handleDragStart = (event: DragStartEvent) => {
    isDraggingRef.current = true
    setActiveId(event.active.id as string)
  }

  const handleDragOver = (event: DragOverEvent) => {
    const { active, over } = event
    if (over && active.id !== over.id) {
      const oldIndex = internalOrder.indexOf(active.id as string)
      const newIndex = internalOrder.indexOf(over.id as string)
      if (oldIndex !== -1 && newIndex !== -1 && oldIndex !== newIndex) {
        setInternalOrder(arrayMove(internalOrder, oldIndex, newIndex))
      }
    }
  }

  const handleDragEnd = (_event: DragEndEvent) => {
    isDraggingRef.current = false
    setActiveId(null)
    // Only sync to parent if order actually changed
    if (JSON.stringify(internalOrder) !== JSON.stringify(value)) {
      onChange(internalOrder)
    }
  }

  const handleSelect = (groupName: string) => {
    if (value.includes(groupName)) {
      onChange(value.filter(v => v !== groupName))
    } else {
      onChange([...value, groupName])
    }
  }

  const handleRemove = (groupName: string) => {
    onChange(value.filter(v => v !== groupName))
  }

  // Build display items: include markers if they should be shown and are in internalOrder
  const displayItems = internalOrder.filter(item => {
    if (item === PROXY_NODES_MARKER) return showNodesMarker
    if (item === PROXY_PROVIDERS_MARKER) return showProvidersMarker
    return true
  })

  // Ensure markers are in value if they should be shown
  const ensureMarkers = (newValue: string[]) => {
    let result = [...newValue]
    if (showNodesMarker && !result.includes(PROXY_NODES_MARKER)) {
      result.push(PROXY_NODES_MARKER)
    }
    if (showProvidersMarker && !result.includes(PROXY_PROVIDERS_MARKER)) {
      result.push(PROXY_PROVIDERS_MARKER)
    }
    // Remove markers that shouldn't be shown
    if (!showNodesMarker) {
      result = result.filter(v => v !== PROXY_NODES_MARKER)
    }
    if (!showProvidersMarker) {
      result = result.filter(v => v !== PROXY_PROVIDERS_MARKER)
    }
    return result
  }

  // Effect: ensure markers are present when props change
  const effectiveValue = ensureMarkers(value)
  if (JSON.stringify(effectiveValue) !== JSON.stringify(value)) {
    // Schedule update for next tick to avoid render loop
    setTimeout(() => onChange(effectiveValue), 0)
  }

  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <div className="flex flex-col gap-2">
        {displayItems.length > 0 && (
          <DndContext sensors={sensors} collisionDetection={closestCenter} onDragStart={handleDragStart} onDragOver={handleDragOver} onDragEnd={handleDragEnd}>
            <SortableContext items={displayItems} strategy={rectSortingStrategy}>
              <div className="flex flex-wrap gap-2">
                {displayItems.map(item => (
                  <SortableItem key={item} id={item} onRemove={handleRemove} />
                ))}
              </div>
            </SortableContext>
            <DragOverlay>
              {activeId ? <DragOverlayItem id={activeId} /> : null}
            </DragOverlay>
          </DndContext>
        )}
        <Popover open={open} onOpenChange={setOpen}>
          <PopoverTrigger asChild>
            <Button variant="outline" role="combobox" aria-expanded={open} className="justify-between">
              {placeholder}
              <ChevronsUpDown className="ml-2 h-4 w-4 shrink-0 opacity-50" />
            </Button>
          </PopoverTrigger>
          <PopoverContent className="w-[300px] p-0">
            <Command>
              <CommandInput placeholder="搜索代理组..." />
              <CommandList>
                <CommandEmpty>没有找到代理组</CommandEmpty>
                <CommandGroup>
                  {availableGroups.map(groupName => (
                    <CommandItem key={groupName} value={groupName} onSelect={() => handleSelect(groupName)}>
                      <Check className={cn('mr-2 h-4 w-4', value.includes(groupName) ? 'opacity-100' : 'opacity-0')} />
                      {groupName}
                    </CommandItem>
                  ))}
                </CommandGroup>
              </CommandList>
            </Command>
          </PopoverContent>
        </Popover>
      </div>
    </div>
  )
}
