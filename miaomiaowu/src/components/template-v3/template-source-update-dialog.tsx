// 规则模板「从链接更新」:重新拉取来源链接覆盖模板内容。
// 从链接导入的模板记着来源,打开就填好;老模板没记过的,填个链接也能更新并记下。
import { useEffect, useState } from 'react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

export function TemplateSourceUpdateDialog({
  name,
  sourceUrl,
  onClose,
}: {
  // 为空 = 关闭
  name: string | null
  sourceUrl?: string
  onClose: () => void
}) {
  const queryClient = useQueryClient()
  const [url, setUrl] = useState('')
  useEffect(() => {
    if (name) setUrl(sourceUrl ?? '')
  }, [name, sourceUrl])

  const update = useMutation({
    mutationFn: async () => {
      const res = await api.post(
        '/api/admin/rule-templates/update-from-source',
        { name, url: url.trim() }
      )
      return res.data as { changed: boolean; bytes: number }
    },
    onSuccess: (data) => {
      toast.success(
        data.changed
          ? `已从链接更新「${name}」`
          : `链接内容与当前模板相同,「${name}」没有变化`
      )
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      onClose()
    },
    onError: (err: any) => {
      toast.error(err?.response?.data?.error || '从链接更新失败')
    },
  })

  return (
    <Dialog open={!!name} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className='sm:max-w-lg'>
        <DialogHeader>
          <DialogTitle>从链接更新</DialogTitle>
          <DialogDescription className='break-all'>{name}</DialogDescription>
        </DialogHeader>
        <div className='space-y-2'>
          <Label htmlFor='template-source-url'>来源链接</Label>
          <Input
            id='template-source-url'
            value={url}
            placeholder='https://…'
            onChange={(e) => setUrl(e.target.value)}
            className='font-mono text-xs'
          />
          {!sourceUrl && (
            <p className='text-muted-foreground text-xs'>
              这个模板没有记录来源链接。填上后会记下,下次直接点更新即可。
            </p>
          )}
          <div className='flex items-start gap-2 rounded-md border border-amber-300 bg-amber-50 p-2 text-xs text-amber-900 dark:border-amber-800 dark:bg-amber-950/40 dark:text-amber-200'>
            <AlertTriangle className='mt-0.5 size-3.5 shrink-0' />
            <span>
              会用链接里的内容整个覆盖这个模板,在面板里改过的地方会丢。
            </span>
          </div>
        </div>
        <DialogFooter>
          <Button variant='outline' onClick={onClose}>
            取消
          </Button>
          <Button
            onClick={() => update.mutate()}
            disabled={!url.trim() || update.isPending}
          >
            {update.isPending && (
              <Loader2 className='mr-1.5 h-4 w-4 animate-spin' />
            )}
            拉取并覆盖
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
