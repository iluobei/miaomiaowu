// 节点管理「隐藏节点」:勾选 = 隐藏。可按分组(主标签)或按服务器整组勾选。
// 隐藏只影响节点管理列表的显示,订阅与下发都不受影响。
// 从妙妙屋X 移植,去掉了 i18n(妙妙屋前端只有中文)。
import { useMemo, useState } from 'react'
import { ChevronDown, ChevronRight, Loader2, Search } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Twemoji } from '@/components/twemoji'

type HideNodeItem = {
  id: number
  name: string
  protocol: string
  group: string
  server: string
}

export function HideNodesDialog({
  open,
  onOpenChange,
  nodes,
  hiddenIds,
  onSave,
}: {
  open: boolean
  onOpenChange: (open: boolean) => void
  nodes: HideNodeItem[]
  hiddenIds: Set<number>
  onSave: (next: Set<number>) => Promise<void>
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='flex max-h-[90svh] flex-col gap-3 sm:max-w-2xl'>
        {/* 每次打开都从当前隐藏状态重新起草(内容随 open 挂载 / 卸载) */}
        {open && (
          <HideNodesBody
            nodes={nodes}
            hiddenIds={hiddenIds}
            onCancel={() => onOpenChange(false)}
            onSave={onSave}
          />
        )}
      </DialogContent>
    </Dialog>
  )
}

function HideNodesBody({
  nodes,
  hiddenIds,
  onCancel,
  onSave,
}: {
  nodes: HideNodeItem[]
  hiddenIds: Set<number>
  onCancel: () => void
  onSave: (next: Set<number>) => Promise<void>
}) {
  const [draft, setDraft] = useState(() => new Set(hiddenIds))
  const [by, setBy] = useState<'group' | 'server'>('group')
  const [q, setQ] = useState('')
  const [collapsed, setCollapsed] = useState<Set<string>>(new Set())
  const [saving, setSaving] = useState(false)

  const groups = useMemo(() => {
    const kw = q.trim().toLowerCase()
    const m = new Map<string, HideNodeItem[]>()
    for (const n of nodes) {
      if (
        kw &&
        !n.name.toLowerCase().includes(kw) &&
        !n.server.toLowerCase().includes(kw) &&
        !n.group.toLowerCase().includes(kw)
      )
        continue
      const key = (by === 'group' ? n.group : n.server) || ''
      const list = m.get(key) ?? []
      list.push(n)
      m.set(key, list)
    }
    return [...m.entries()].map(([key, list]) => ({ key, list }))
  }, [nodes, by, q])

  const setMany = (ids: number[], on: boolean) =>
    setDraft((prev) => {
      const next = new Set(prev)
      for (const id of ids) {
        if (on) next.add(id)
        else next.delete(id)
      }
      return next
    })
  const toggleCollapsed = (key: string) =>
    setCollapsed((prev) => {
      const next = new Set(prev)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })

  const changed =
    draft.size !== hiddenIds.size || [...draft].some((id) => !hiddenIds.has(id))

  return (
    <>
      <DialogHeader>
        <DialogTitle>隐藏节点</DialogTitle>
        <DialogDescription>
          勾选的节点不在节点管理列表里显示(可随时点「显示已隐藏」查看),订阅与下发都不受影响。
        </DialogDescription>
      </DialogHeader>

      <div className='flex flex-wrap items-center gap-2'>
        <div className='bg-muted inline-flex rounded-md p-0.5'>
          {(['group', 'server'] as const).map((k) => (
            <button
              key={k}
              type='button'
              aria-pressed={by === k}
              onClick={() => setBy(k)}
              className={cn(
                'h-8 rounded px-3 text-sm',
                by === k
                  ? 'bg-background font-medium shadow-sm'
                  : 'text-muted-foreground'
              )}
            >
              {k === 'group' ? '按分组' : '按服务器'}
            </button>
          ))}
        </div>
        <div className='relative min-w-0 flex-1'>
          <Search className='text-muted-foreground pointer-events-none absolute top-1/2 left-2.5 h-4 w-4 -translate-y-1/2' />
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder='搜索节点 / 服务器 / 分组'
            className='h-9 pl-8'
          />
        </div>
      </div>

      <div className='min-h-0 flex-1 overflow-y-auto rounded-md border'>
        {groups.length === 0 && (
          <div className='text-muted-foreground py-8 text-center text-sm'>
            没有匹配的节点
          </div>
        )}
        {groups.map(({ key, list }) => {
          const ids = list.map((n) => n.id)
          const on = ids.filter((id) => draft.has(id)).length
          const isCollapsed = collapsed.has(key)
          return (
            <div key={key} className='border-b last:border-b-0'>
              <div className='bg-muted/50 flex items-center gap-2 px-3 py-2'>
                <Checkbox
                  aria-label={key || '未分组'}
                  checked={
                    on === 0
                      ? false
                      : on === ids.length
                        ? true
                        : 'indeterminate'
                  }
                  onCheckedChange={() => setMany(ids, on !== ids.length)}
                />
                <button
                  type='button'
                  className='flex min-w-0 flex-1 items-center gap-1 text-left text-sm font-medium'
                  onClick={() => toggleCollapsed(key)}
                >
                  {isCollapsed ? (
                    <ChevronRight className='size-4 shrink-0' />
                  ) : (
                    <ChevronDown className='size-4 shrink-0' />
                  )}
                  <span className='truncate'>
                    <Twemoji>{key || '未分组'}</Twemoji>
                  </span>
                </button>
                <span className='text-muted-foreground shrink-0 text-xs tabular-nums'>
                  已隐藏 {on}/{ids.length}
                </span>
              </div>
              {!isCollapsed &&
                list.map((n) => (
                  <label
                    key={n.id}
                    className='hover:bg-muted/40 flex cursor-pointer items-center gap-2 px-3 py-1.5 pl-9 text-sm'
                  >
                    <Checkbox
                      checked={draft.has(n.id)}
                      onCheckedChange={(c) => setMany([n.id], c === true)}
                    />
                    <span className='text-muted-foreground w-16 shrink-0 truncate text-[11px] uppercase'>
                      {n.protocol}
                    </span>
                    <span className='min-w-0 flex-1 truncate'>
                      <Twemoji>{n.name}</Twemoji>
                    </span>
                    {by === 'group' && n.server && (
                      <span className='text-muted-foreground hidden max-w-[40%] shrink-0 truncate text-xs sm:inline'>
                        {n.server}
                      </span>
                    )}
                  </label>
                ))}
            </div>
          )
        })}
      </div>

      <DialogFooter className='flex-row items-center gap-2 sm:justify-between'>
        <span className='text-muted-foreground mr-auto text-sm'>
          共隐藏 {draft.size} 个
        </span>
        <Button variant='outline' onClick={onCancel} disabled={saving}>
          取消
        </Button>
        <Button
          disabled={!changed || saving}
          onClick={async () => {
            setSaving(true)
            try {
              await onSave(draft)
              onCancel()
            } finally {
              setSaving(false)
            }
          }}
        >
          {saving && <Loader2 className='mr-1 size-4 animate-spin' />}
          保存
        </Button>
      </DialogFooter>
    </>
  )
}
