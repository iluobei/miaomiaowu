// 菜单排序与显示/隐藏 —— 管理员统一设定,对所有人生效。
//
// 隐藏只影响导航,路由本身还在 —— 知道地址的人照样进得去,所以它是整理界面用的,
// 不是权限手段。卡片文案里说明了。
//
// 从妙妙屋X 迁移而来,菜单清单换成妙妙屋自己的(见 lib/nav-links.ts)。
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  DndContext,
  PointerSensor,
  closestCenter,
  useSensor,
  useSensors,
  type DragEndEvent,
} from '@dnd-kit/core'
import {
  SortableContext,
  arrayMove,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'
import { CSS } from '@dnd-kit/utilities'
import {
  ChevronDown,
  ChevronUp,
  GripVertical,
  Loader2,
  Menu,
  RotateCcw,
} from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import {
  allNavLinksForConfig,
  navMenuQueryFn,
  type NavLink,
} from '@/lib/nav-links'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'

/** 把 list[index] 与相邻项交换(dir=-1 上移,1 下移),越界返回原数组。 */
function moveItem<T>(list: T[], index: number, dir: -1 | 1): T[] {
  const to = index + dir
  if (index < 0 || to < 0 || to >= list.length) return list
  const next = list.slice()
  ;[next[index], next[to]] = [next[to], next[index]]
  return next
}

function SortableRow({
  link,
  hidden,
  onToggle,
  onMove,
  isFirst,
  isLast,
}: {
  link: NavLink
  hidden: boolean
  onToggle: (to: string, visible: boolean) => void
  onMove: (dir: -1 | 1) => void
  isFirst: boolean
  isLast: boolean
}) {
  const {
    attributes,
    listeners,
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({ id: link.to })
  const Icon = link.icon
  return (
    <div
      ref={setNodeRef}
      style={{
        transform: CSS.Transform.toString(transform),
        transition,
        opacity: isDragging ? 0.6 : 1,
      }}
      className='bg-card flex items-center gap-3 border px-3 py-2'
    >
      <button
        type='button'
        // 手机上把手柄的点按区域放大(图标不变)
        className='text-muted-foreground cursor-grab touch-none active:cursor-grabbing max-md:-m-2 max-md:p-2'
        aria-label='拖动排序'
        {...attributes}
        {...listeners}
      >
        <GripVertical className='size-4' />
      </button>
      <Icon className='text-muted-foreground size-4 shrink-0' />
      <span className='min-w-0 flex-1 truncate text-sm'>{link.title}</span>
      <code className='text-muted-foreground hidden font-mono text-xs sm:block'>
        {link.to}
      </code>
      <Switch
        checked={!hidden}
        onCheckedChange={(v) => onToggle(link.to, v)}
        aria-label='显示这个菜单'
      />
      {/* 触屏上拖动常和页面滚动抢手势,补一对上移/下移按钮 */}
      <div className='-mr-1 flex shrink-0 items-center md:hidden'>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          className='size-8'
          disabled={isFirst}
          aria-label='上移'
          onClick={() => onMove(-1)}
        >
          <ChevronUp className='size-4' />
        </Button>
        <Button
          type='button'
          variant='ghost'
          size='icon'
          className='size-8'
          disabled={isLast}
          aria-label='下移'
          onClick={() => onMove(1)}
        >
          <ChevronDown className='size-4' />
        </Button>
      </div>
    </div>
  )
}

export function NavMenuCard() {
  const queryClient = useQueryClient()
  const allLinks = allNavLinksForConfig()
  const [order, setOrder] = useState<string[]>([])
  const [hidden, setHidden] = useState<string[]>([])
  const [dirty, setDirty] = useState(false)

  const { data } = useQuery({
    queryKey: ['nav-menu'],
    queryFn: navMenuQueryFn,
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (!data || dirty) return
    // 配置里没提到的菜单项排在最后、保持默认相对顺序 —— 新版本加了菜单而管理员的
    // 旧配置里没有它时,不该悄悄消失,也不该跳到最前面。
    const known = new Set(allLinks.map((l) => l.to))
    const fromCfg = (data.order ?? []).filter((to) => known.has(to))
    const rest = allLinks.map((l) => l.to).filter((to) => !fromCfg.includes(to))
    setOrder([...fromCfg, ...rest])
    setHidden(data.hidden ?? [])
    // allLinks 每次渲染都是新数组,放进依赖会无限循环;它的内容是编译期固定的。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [data, dirty])

  const sensors = useSensors(
    // 拖动 8px 才算开始:否则点开关和抓手柄会互相抢事件。
    useSensor(PointerSensor, { activationConstraint: { distance: 8 } })
  )

  const byTo = new Map(allLinks.map((l) => [l.to, l]))
  const rows = order.map((to) => byTo.get(to)).filter(Boolean) as NavLink[]

  const onDragEnd = (e: DragEndEvent) => {
    const { active, over } = e
    if (!over || active.id === over.id) return
    const from = order.indexOf(String(active.id))
    const to = order.indexOf(String(over.id))
    if (from < 0 || to < 0) return
    setOrder(arrayMove(order, from, to))
    setDirty(true)
  }

  const saveMutation = useMutation({
    mutationFn: async () =>
      (await api.put('/api/admin/system-settings/nav-menu', { order, hidden }))
        .data,
    onSuccess: () => {
      setDirty(false)
      queryClient.invalidateQueries({ queryKey: ['nav-menu'] })
      toast.success('菜单设置已保存')
    },
    onError: handleServerError,
  })

  return (
    <Card>
      <CardHeader className='pb-4'>
        <CardTitle className='flex items-center gap-2'>
          <Menu className='size-5' /> 菜单排序
        </CardTitle>
        <CardDescription>
          拖动调整顺序,开关控制是否摆进导航。对所有人生效。 隐藏只是不显示入口
          —— 路由还在,知道地址仍能进入,所以它用来整理界面,不能当权限用。
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-3'>
        <DndContext
          sensors={sensors}
          collisionDetection={closestCenter}
          onDragEnd={onDragEnd}
        >
          <SortableContext items={order} strategy={verticalListSortingStrategy}>
            <div className='space-y-2'>
              {rows.map((link, i) => (
                <SortableRow
                  key={link.to}
                  link={link}
                  hidden={hidden.includes(link.to)}
                  isFirst={i === 0}
                  isLast={i === rows.length - 1}
                  onMove={(dir) => {
                    setOrder(moveItem(order, order.indexOf(link.to), dir))
                    setDirty(true)
                  }}
                  onToggle={(to, visible) => {
                    setHidden((h) =>
                      visible ? h.filter((x) => x !== to) : [...h, to]
                    )
                    setDirty(true)
                  }}
                />
              ))}
            </div>
          </SortableContext>
        </DndContext>

        <div className='flex gap-2 pt-1'>
          <Button
            disabled={!dirty || saveMutation.isPending}
            onClick={() => saveMutation.mutate()}
          >
            {saveMutation.isPending && (
              <Loader2 className='mr-2 size-4 animate-spin' />
            )}
            保存
          </Button>
          <Button
            variant='outline'
            disabled={saveMutation.isPending}
            onClick={() => {
              setOrder(allLinks.map((l) => l.to))
              setHidden([])
              setDirty(true)
            }}
          >
            <RotateCcw className='mr-2 size-4' /> 恢复默认
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
