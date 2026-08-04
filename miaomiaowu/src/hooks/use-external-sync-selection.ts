import { useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'

import type { ExternalSyncCandidate, ExternalSyncSelection } from '@/components/external-sync-node-dialog'
import { api } from '@/lib/api'

export function useExternalSyncSelection() {
  const queryClient = useQueryClient()
  const [selection, setSelection] = useState<ExternalSyncSelection | null>(null)
  const present = (data: { session_id?: string; new_nodes?: ExternalSyncCandidate[] }) => {
    const nodes = data.new_nodes ?? []
    if (!data.session_id || nodes.length === 0) return false
    setSelection({ sessionId: data.session_id, nodes, selectedIds: new Set(nodes.map(node => node.id)) })
    return true
  }
  const confirm = useMutation({
    mutationFn: async () => selection && (await api.post('/api/admin/sync-external-subscriptions/confirm', { session_id: selection.sessionId, candidate_ids: [...selection.selectedIds] })).data,
    onSuccess: data => {
      setSelection(null)
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      queryClient.invalidateQueries({ queryKey: ['external-subscriptions'] })
      if (data?.message) toast.success(data.message)
    },
    onError: (error: any) => toast.error(error.response?.data?.error || '保存新增节点失败'),
  })
  return { selection, present, setSelectedIds: (ids: Set<string>) => setSelection(current => current ? { ...current, selectedIds: ids } : current), cancel: () => setSelection(null), confirm: () => confirm.mutate(), confirming: confirm.isPending }
}
