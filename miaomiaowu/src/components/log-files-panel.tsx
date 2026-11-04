import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { HardDrive, RefreshCw, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/confirm-dialog'

type LogFile = {
  name: string
  size: number
  modified: string
  // 正在被写入的文件。删除它只会截断(不能 unlink,进程还持着 fd),UI 上要说清楚。
  active: boolean
}

type FilesResp = {
  files?: LogFile[]
  total_size?: number
  dir?: string
}

function formatBytes(n: number): string {
  if (!n) return '0 B'
  const units = ['B', 'KB', 'MB', 'GB']
  let v = n
  let i = 0
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i++
  }
  return `${v >= 10 || i === 0 ? Math.round(v) : v.toFixed(1)} ${units[i]}`
}

/**
 * LogFilesPanel 日志文件占用与清理(data/logs)。
 *
 * 后端对"当前正在写入的文件"执行截断而非删除 —— 那个文件被进程持有 fd,
 * unlink 后日志会继续写进已删除的 inode(空间不释放、页面也看不到新行)。
 * 所以这里对 active 文件把按钮文案写成"清空",避免用户以为删除失败。
 */
export function LogFilesPanel() {
  const queryClient = useQueryClient()
  const queryKey = ['admin-log-files']
  const [confirm, setConfirm] = useState<{
    name?: string
    all?: boolean
  } | null>(null)

  const filesQuery = useQuery({
    queryKey,
    queryFn: async () =>
      (await api.get('/api/admin/logs/files')).data as FilesResp,
  })

  const purgeMutation = useMutation({
    mutationFn: async (target: { name?: string; all?: boolean }) => {
      await api.delete('/api/admin/logs/files', {
        params: target.all ? { all: '1' } : { name: target.name ?? '' },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey })
      toast.success('已清理')
      setConfirm(null)
    },
    onError: () => {
      toast.error('清理失败')
      setConfirm(null)
    },
  })

  const data = filesQuery.data
  const files = data?.files ?? []

  return (
    <div className='border'>
      <div className='flex flex-wrap items-center gap-2 border-b px-3 py-2'>
        <HardDrive className='text-muted-foreground h-4 w-4' />
        <span className='text-sm font-medium'>日志文件</span>
        <span className='text-muted-foreground text-xs'>
          共 {files.length} 个 · {formatBytes(data?.total_size ?? 0)}
        </span>
        {data?.dir && (
          <span className='text-muted-foreground hidden text-xs sm:inline'>
            · {data.dir}
          </span>
        )}
        <div className='ml-auto flex items-center gap-2'>
          <Button
            variant='outline'
            size='sm'
            onClick={() => filesQuery.refetch()}
            disabled={filesQuery.isFetching}
          >
            <RefreshCw
              className={`h-4 w-4 ${filesQuery.isFetching ? 'animate-spin' : ''}`}
            />
          </Button>
          <Button
            variant='destructive'
            size='sm'
            onClick={() => setConfirm({ all: true })}
            disabled={files.length === 0 || purgeMutation.isPending}
          >
            清空全部
          </Button>
        </div>
      </div>

      {filesQuery.isLoading ? (
        <div className='text-muted-foreground p-3 text-sm'>加载中…</div>
      ) : files.length === 0 ? (
        <div className='text-muted-foreground p-3 text-sm'>暂无日志文件</div>
      ) : (
        <ul className='divide-y'>
          {files.map((f) => (
            <li
              key={f.name}
              className='flex flex-wrap items-center gap-2 px-3 py-2 text-sm'
            >
              <span className='truncate font-mono text-xs'>{f.name}</span>
              {f.active && (
                <Badge variant='secondary' className='text-[10px]'>
                  写入中
                </Badge>
              )}
              <span className='text-muted-foreground text-xs'>
                {formatBytes(f.size)}
              </span>
              <span className='text-muted-foreground hidden text-xs sm:inline'>
                {f.modified}
              </span>
              <Button
                variant='ghost'
                size='sm'
                className='text-muted-foreground hover:text-destructive ml-auto'
                onClick={() => setConfirm({ name: f.name })}
                disabled={purgeMutation.isPending}
                title={
                  f.active ? '清空该文件内容(文件本身会保留)' : '删除该文件'
                }
              >
                <Trash2 className='h-4 w-4' />
              </Button>
            </li>
          ))}
        </ul>
      )}

      <ConfirmDialog
        open={!!confirm}
        onOpenChange={(o) => !o && setConfirm(null)}
        title={confirm?.all ? '清空全部日志文件?' : '清理日志文件?'}
        desc={
          confirm?.all
            ? '历史日志文件将被删除,当前正在写入的文件会被清空(文件保留)。此操作不可撤销。'
            : files.find((f) => f.name === confirm?.name)?.active
              ? `「${confirm?.name}」正在写入中,将清空其内容(文件本身保留)。此操作不可撤销。`
              : `将删除「${confirm?.name}」。此操作不可撤销。`
        }
        confirmText='确定'
        cancelBtnText='取消'
        destructive
        isLoading={purgeMutation.isPending}
        handleConfirm={() => confirm && purgeMutation.mutate(confirm)}
      />
    </div>
  )
}
