import { useMemo } from 'react'

import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'

export interface ExternalSyncCandidate {
  id: string
  subscription_name: string
  name: string
  protocol: string
  server: string
  port?: string | number
}

export interface ExternalSyncSelection {
  sessionId: string
  nodes: ExternalSyncCandidate[]
  selectedIds: Set<string>
}

export function ExternalSyncNodeDialog({ selection, saving, onSelectionChange, onCancel, onConfirm }: {
  selection: ExternalSyncSelection | null
  saving: boolean
  onSelectionChange: (ids: Set<string>) => void
  onCancel: () => void
  onConfirm: () => void
}) {
  const groups = useMemo(() => {
    const grouped = new Map<string, ExternalSyncCandidate[]>()
    for (const node of selection?.nodes ?? []) grouped.set(node.subscription_name, [...(grouped.get(node.subscription_name) ?? []), node])
    return [...grouped.entries()]
  }, [selection?.nodes])
  const allSelected = Boolean(selection?.nodes.length) && selection?.selectedIds.size === selection?.nodes.length
  const toggle = (id: string, checked: boolean) => {
    if (!selection) return
    const next = new Set(selection.selectedIds)
    if (checked) next.add(id)
    else next.delete(id)
    onSelectionChange(next)
  }
  return <Dialog open={Boolean(selection)} onOpenChange={(open) => !open && !saving && onCancel()}>
    <DialogContent className='flex max-h-[85vh] max-w-3xl flex-col overflow-hidden'>
      <DialogHeader><DialogTitle>选择要保存的新增节点</DialogTitle><DialogDescription>已有节点已经更新；本次发现 {selection?.nodes.length ?? 0} 个新增节点。</DialogDescription></DialogHeader>
      <div className='flex items-center justify-between border-b pb-3'>
        <label className='flex items-center gap-2 text-sm'><Checkbox checked={allSelected} onCheckedChange={() => selection && onSelectionChange(allSelected ? new Set() : new Set(selection.nodes.map(n => n.id)))} />全选</label>
        <span className='text-sm text-muted-foreground'>已选 {selection?.selectedIds.size ?? 0} 个</span>
      </div>
      <div className='min-h-0 flex-1 space-y-4 overflow-y-auto'>
        {groups.map(([name, nodes]) => <section key={name} className='space-y-2'><div className='sticky top-0 rounded bg-muted px-2 py-1 text-sm font-medium'>{name}</div>
          {nodes.map(node => <label key={node.id} className='flex cursor-pointer items-center gap-3 rounded border p-3'><Checkbox checked={selection?.selectedIds.has(node.id)} onCheckedChange={v => toggle(node.id, v === true)} /><div className='min-w-0 flex-1'><div className='truncate text-sm font-medium'>{node.name}</div><div className='truncate text-xs text-muted-foreground'>{node.server}{node.port !== undefined ? `:${node.port}` : ''}</div></div><Badge variant='secondary'>{node.protocol}</Badge></label>)}
        </section>)}
      </div>
      <DialogFooter><Button variant='outline' disabled={saving} onClick={onCancel}>取消</Button><Button disabled={saving} onClick={onConfirm}>{saving ? '保存中…' : `保存 ${selection?.selectedIds.size ?? 0} 个节点`}</Button></DialogFooter>
    </DialogContent>
  </Dialog>
}
