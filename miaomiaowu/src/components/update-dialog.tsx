import { useState } from 'react'
import { useQuery, useMutation } from '@tanstack/react-query'
import {
  RefreshCw,
  Download,
  CheckCircle,
  AlertTriangle,
  ExternalLink,
} from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'

interface UpdateDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

interface UpdateInfo {
  current_version: string
  latest_version: string
  has_update: boolean
  release_url: string
  download_url: string
  release_notes: string
}

export function UpdateDialog({ open, onOpenChange }: UpdateDialogProps) {
  const [isUpdating, setIsUpdating] = useState(false)

  // Check for updates
  const {
    data: updateInfo,
    isLoading,
    refetch,
    isRefetching,
  } = useQuery({
    queryKey: ['update-check'],
    queryFn: async () => {
      const response = await api.get('/api/admin/update/check')
      return response.data as UpdateInfo
    },
    enabled: open,
    staleTime: 0,
    retry: 1,
  })

  // Apply update
  const applyUpdate = useMutation({
    mutationFn: async () => {
      setIsUpdating(true)
      return api.post('/api/admin/update/apply')
    },
    onSuccess: () => {
      toast.success('更新成功，页面将在 3 秒后刷新')
      setTimeout(() => {
        window.location.reload()
      }, 3000)
    },
    onError: (error: Error) => {
      setIsUpdating(false)
      toast.error(`更新失败: ${error.message}`)
    },
  })

  const isCheckingOrRefetching = isLoading || isRefetching

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md overflow-hidden'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <RefreshCw className='size-5' /> 检查更新
          </DialogTitle>
          <DialogDescription>检查是否有新版本可用</DialogDescription>
        </DialogHeader>

        <div className='space-y-4'>
          {isCheckingOrRefetching ? (
            <div className='text-center py-8'>
              <RefreshCw className='size-8 animate-spin mx-auto mb-3 text-primary' />
              <p className='text-sm text-muted-foreground'>正在检查更新...</p>
            </div>
          ) : updateInfo?.has_update ? (
            <div className='space-y-4'>
              <div className='flex items-center gap-2 text-amber-500'>
                <AlertTriangle className='size-5' />
                <span className='font-medium'>发现新版本！</span>
              </div>

              <div className='bg-muted/50 rounded-lg p-3 space-y-2'>
                <div className='flex justify-between text-sm'>
                  <span className='text-muted-foreground'>当前版本</span>
                  <span className='font-mono'>v{updateInfo.current_version}</span>
                </div>
                <div className='flex justify-between text-sm'>
                  <span className='text-muted-foreground'>最新版本</span>
                  <span className='font-mono text-green-600'>
                    v{updateInfo.latest_version}
                  </span>
                </div>
              </div>

              {updateInfo.release_notes && (
                <div className='space-y-2 overflow-hidden'>
                  <p className='text-sm font-medium'>更新内容：</p>
                  <div className='bg-muted/30 rounded-lg p-3 max-h-40 overflow-y-auto overflow-x-hidden'>
                    <p className='text-sm text-muted-foreground whitespace-pre-wrap break-all'>
                      {updateInfo.release_notes}
                    </p>
                  </div>
                </div>
              )}

              <div className='flex flex-col gap-2'>
                <Button
                  onClick={() => applyUpdate.mutate()}
                  disabled={isUpdating || !updateInfo.download_url}
                  className='w-full'
                >
                  <Download className='size-4 mr-2' />
                  {isUpdating ? '更新中，请稍候...' : '立即更新'}
                </Button>

                {!updateInfo.download_url && (
                  <p className='text-xs text-destructive text-center'>
                    未找到适合当前系统的下载文件
                  </p>
                )}

                {updateInfo.release_url && (
                  <Button
                    variant='outline'
                    className='w-full'
                    onClick={() => window.open(updateInfo.release_url, '_blank')}
                  >
                    <ExternalLink className='size-4 mr-2' />
                    查看 GitHub Release
                  </Button>
                )}
              </div>

              {isUpdating && (
                <div className='bg-blue-50 dark:bg-blue-950/30 rounded-lg p-3'>
                  <p className='text-sm text-blue-600 dark:text-blue-400'>
                    正在下载并安装更新，完成后将自动重启服务。请勿关闭此页面。
                  </p>
                </div>
              )}
            </div>
          ) : (
            <div className='text-center py-8'>
              <CheckCircle className='size-12 text-green-500 mx-auto mb-3' />
              <p className='font-medium text-lg'>已是最新版本</p>
              <p className='text-sm text-muted-foreground mt-1'>
                当前版本：v{updateInfo?.current_version}
              </p>
            </div>
          )}

          <Button
            variant='outline'
            onClick={() => refetch()}
            disabled={isCheckingOrRefetching || isUpdating}
            className='w-full'
          >
            <RefreshCw
              className={`size-4 mr-2 ${isCheckingOrRefetching ? 'animate-spin' : ''}`}
            />
            重新检查
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  )
}
