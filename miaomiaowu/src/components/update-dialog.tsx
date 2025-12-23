import { useState, useRef, useCallback } from 'react'
import { useQuery } from '@tanstack/react-query'
import {
  RefreshCw,
  Download,
  CheckCircle,
  AlertTriangle,
  ExternalLink,
  Circle,
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
import { Progress } from '@/components/ui/progress'
import { useAuthStore } from '@/stores/auth-store'

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

interface UpdateProgress {
  step: 'checking' | 'downloading' | 'backing_up' | 'replacing' | 'restarting' | 'done' | 'error'
  progress: number
  message: string
}

const STEPS = [
  { key: 'checking', label: '检查版本' },
  { key: 'downloading', label: '下载更新' },
  { key: 'backing_up', label: '备份当前版本' },
  { key: 'replacing', label: '替换文件' },
  { key: 'restarting', label: '重启服务' },
] as const

export function UpdateDialog({ open, onOpenChange }: UpdateDialogProps) {
  const [isUpdating, setIsUpdating] = useState(false)
  const [updateProgress, setUpdateProgress] = useState<UpdateProgress | null>(null)
  const updateCompleteRef = useRef(false)
  const { auth } = useAuthStore()

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

  // Start update with SSE using fetch (more reliable than EventSource for auth)
  const startUpdate = useCallback(async () => {
    setIsUpdating(true)
    setUpdateProgress(null)
    updateCompleteRef.current = false

    try {
      const response = await fetch('/api/admin/update/apply-sse', {
        method: 'GET',
        headers: {
          'MM-Authorization': auth.accessToken || '',
        },
      })

      if (!response.ok) {
        throw new Error(`HTTP ${response.status}`)
      }

      const reader = response.body?.getReader()
      if (!reader) {
        throw new Error('无法读取响应流')
      }

      const decoder = new TextDecoder()
      let buffer = ''

      while (true) {
        const { done, value } = await reader.read()
        if (done) break

        buffer += decoder.decode(value, { stream: true })
        const lines = buffer.split('\n')
        buffer = lines.pop() || ''

        for (const line of lines) {
          if (line.startsWith('data: ')) {
            try {
              const progress = JSON.parse(line.slice(6)) as UpdateProgress
              setUpdateProgress(progress)

              if (progress.step === 'done') {
                updateCompleteRef.current = true
                toast.success('更新成功，页面将在 3 秒后刷新')
                setTimeout(() => {
                  window.location.reload()
                }, 3000)
                return
              } else if (progress.step === 'error') {
                updateCompleteRef.current = true
                setIsUpdating(false)
                toast.error(progress.message)
                return
              }
            } catch {
              // Ignore parse errors
            }
          }
        }
      }

      // Stream ended without done/error
      if (!updateCompleteRef.current) {
        setIsUpdating(false)
        toast.error('连接意外关闭')
      }
    } catch (error) {
      if (!updateCompleteRef.current) {
        setIsUpdating(false)
        toast.error(`更新失败: ${error instanceof Error ? error.message : '未知错误'}`)
      }
    }
  }, [auth.accessToken])

  // Cleanup on close
  const handleOpenChange = (newOpen: boolean) => {
    onOpenChange(newOpen)
  }

  const isCheckingOrRefetching = isLoading || isRefetching

  // Get current step index for UI
  const currentStepIndex = STEPS.findIndex(s => s.key === updateProgress?.step)

  return (
    <Dialog open={open} onOpenChange={handleOpenChange}>
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

              {updateInfo.release_notes && !isUpdating && (
                <div className='space-y-2 overflow-hidden'>
                  <p className='text-sm font-medium'>更新内容：</p>
                  <div className='bg-muted/30 rounded-lg p-3 max-h-40 overflow-y-auto overflow-x-hidden'>
                    <p className='text-sm text-muted-foreground whitespace-pre-wrap break-all'>
                      {updateInfo.release_notes}
                    </p>
                  </div>
                </div>
              )}

              {/* Progress UI */}
              {isUpdating && (
                <div className='space-y-4'>
                  <div className='space-y-2'>
                    {STEPS.map((step, index) => {
                      const isCompleted = index < currentStepIndex
                      const isCurrent = step.key === updateProgress?.step
                      const isPending = index > currentStepIndex || currentStepIndex === -1

                      return (
                        <div key={step.key} className='flex items-center gap-3'>
                          {isCompleted ? (
                            <CheckCircle className='size-5 text-green-500 shrink-0' />
                          ) : isCurrent ? (
                            <RefreshCw className='size-5 text-primary animate-spin shrink-0' />
                          ) : (
                            <Circle className='size-5 text-muted-foreground shrink-0' />
                          )}
                          <span
                            className={
                              isCurrent
                                ? 'font-medium'
                                : isPending
                                  ? 'text-muted-foreground'
                                  : ''
                            }
                          >
                            {step.label}
                          </span>
                          {isCurrent && step.key === 'downloading' && updateProgress && (
                            <span className='ml-auto text-sm font-mono'>
                              {updateProgress.progress}%
                            </span>
                          )}
                        </div>
                      )
                    })}
                  </div>

                  {/* Progress bar for downloading */}
                  {updateProgress?.step === 'downloading' && (
                    <Progress value={updateProgress.progress} className='h-2' />
                  )}

                  <div className='bg-blue-50 dark:bg-blue-950/30 rounded-lg p-3'>
                    <p className='text-sm text-blue-600 dark:text-blue-400'>
                      {updateProgress?.message || '正在准备更新...'}
                    </p>
                  </div>
                </div>
              )}

              {!isUpdating && (
                <div className='flex flex-col gap-2'>
                  <Button
                    onClick={startUpdate}
                    disabled={isUpdating || !updateInfo.download_url}
                    className='w-full'
                  >
                    <Download className='size-4 mr-2' />
                    立即更新
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
