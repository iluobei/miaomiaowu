import { useState } from 'react'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import {
  LogOut,
  Settings2,
  ExternalLink,
  BookOpen,
  HardDrive,
  RefreshCw,
  Bug,
  Palette,
  Sparkles,
} from 'lucide-react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { getCookie, setCookie } from '@/lib/cookies'
import { handleServerError } from '@/lib/handle-server-error'
import { profileQueryFn } from '@/lib/profile'
import useDialogState from '@/hooks/use-dialog-state'
import { useVersionCheck } from '@/hooks/use-version-check'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Switch } from '@/components/ui/switch'
import { BackupDialog } from '@/components/backup-dialog'
import { MmwxDialog } from '@/components/mmwx-dialog'
import { SignOutDialog } from '@/components/sign-out-dialog'
import { UpdateDialog } from '@/components/update-dialog'

export function UserMenu() {
  const [open, setOpen] = useDialogState<boolean>()
  const [backupDialogOpen, setBackupDialogOpen] = useState(false)
  const [updateDialogOpen, setUpdateDialogOpen] = useState(false)
  const [mmwxDialogOpen, setMmwxDialogOpen] = useState(false)

  const { auth } = useAuthStore()
  const { currentVersion, hasUpdate, releaseUrl } = useVersionCheck()
  const queryClient = useQueryClient()

  const { data: profile } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  // Debug日志状态
  const { data: debugStatus } = useQuery({
    queryKey: ['debug-status'],
    queryFn: async () => {
      const response = await api.get('/api/user/debug/status')
      return response.data as {
        enabled: boolean
        log_path?: string
        started_at?: string
        file_size?: string
        duration?: string
      }
    },
    enabled: Boolean(auth.accessToken),
    refetchInterval: (query) => {
      return query.state.data?.enabled ? 5000 : false
    },
  })

  // 开启Debug日志
  const enableDebugMutation = useMutation({
    mutationFn: async () => {
      const response = await api.post('/api/user/debug/enable')
      return response.data
    },
    onSuccess: () => {
      toast.success('Debug日志已开启')
      queryClient.invalidateQueries({ queryKey: ['debug-status'] })
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('开启Debug日志失败')
    },
  })

  // 关闭Debug日志
  const disableDebugMutation = useMutation({
    mutationFn: async () => {
      const response = await api.post('/api/user/debug/disable')
      return response.data
    },
    onSuccess: () => {
      toast.success('Debug日志已关闭')
      queryClient.invalidateQueries({ queryKey: ['debug-status'] })
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('关闭Debug日志失败')
    },
  })

  const handleDebugToggle = (checked: boolean) => {
    if (checked) {
      enableDebugMutation.mutate()
    } else {
      disableDebugMutation.mutate()
    }
  }

  const displayName = profile?.nickname || profile?.username || '用户'
  const fallbackAvatar = profile?.is_admin
    ? `${import.meta.env.BASE_URL}images/admin-avatar.webp`
    : `${import.meta.env.BASE_URL}images/user-avatar.png`
  const avatarSrc = profile?.avatar_url?.trim()
    ? profile.avatar_url.trim()
    : fallbackAvatar
  const fallbackText = displayName.slice(0, 2)
  const emailText = profile?.email?.trim()
  const levelText = profile?.role ? profile.role.toUpperCase() : 'LV.0'

  return (
    <>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant='outline'
            size='sm'
            aria-label={`用户菜单: ${displayName}`}
            className='user-menu-trigger pixel-button h-9 min-w-0 justify-center gap-2 overflow-hidden px-2 py-2 sm:min-w-[120px] sm:gap-2 sm:px-3'
          >
            <span className='sr-only'>{`用户菜单: ${displayName}`}</span>
            <Avatar className='um-avatar size-7 border-[1.5px] border-[color:rgba(241,140,110,0.45)] shadow-[2px_2px_0_rgba(0,0,0,0.2)]'>
              <AvatarImage src={avatarSrc} alt={displayName} />
              <AvatarFallback>{fallbackText || '用户'}</AvatarFallback>
            </Avatar>
            <div className='hidden sm:flex sm:flex-col sm:items-center sm:leading-tight'>
              <span className='um-name max-w-[70px] truncate text-sm font-semibold'>
                {displayName}
              </span>
              <span className='um-level text-muted-foreground text-xs tracking-[0.2em] uppercase'>
                {levelText}
              </span>
            </div>
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align='end' className='w-56 space-y-3 p-4'>
          <div className='flex flex-col items-center gap-2 text-center'>
            <Avatar className='size-12'>
              <AvatarImage src={avatarSrc} alt={displayName} />
              <AvatarFallback>{fallbackText || '用户'}</AvatarFallback>
            </Avatar>
            <div className='space-y-1'>
              <p className='text-sm leading-tight font-semibold'>
                {displayName}
              </p>
              <p className='text-muted-foreground text-xs'>
                {profile?.username || '未登录'}
              </p>
              {emailText ? (
                <p className='text-muted-foreground text-xs break-all'>
                  {emailText}
                </p>
              ) : (
                <p className='text-muted-foreground text-xs'>未填写邮箱</p>
              )}
            </div>
          </div>
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild className='cursor-pointer justify-center'>
            <Link to='/settings' className='flex items-center gap-2'>
              <Settings2 className='size-4' /> 个人设置
            </Link>
          </DropdownMenuItem>

          {/* Debug日志开关 */}
          <DropdownMenuItem
            className='cursor-pointer justify-between px-2'
            onSelect={(e) => e.preventDefault()}
          >
            <div className='flex items-center gap-2'>
              <Bug className='size-4' />
              <span className='text-sm'>Debug 日志</span>
            </div>
            <Switch
              checked={debugStatus?.enabled || false}
              onCheckedChange={handleDebugToggle}
              disabled={
                enableDebugMutation.isPending || disableDebugMutation.isPending
              }
              onClick={(e) => e.stopPropagation()}
            />
          </DropdownMenuItem>

          {/* 界面风格切换 */}
          <DropdownMenuItem
            className='cursor-pointer px-2'
            onSelect={(e) => e.preventDefault()}
          >
            <Palette className='size-4 shrink-0' />
            <div className='flex flex-1 gap-1'>
              {[
                { value: 'miaomiaowu', label: '妙妙屋' },
                { value: 'flat', label: '扁平' },
                { value: 'anime', label: '二次元' },
                { value: 'glass', label: '液态玻璃' },
              ].map((opt) => (
                <button
                  key={opt.value}
                  type='button'
                  onClick={(e) => {
                    e.stopPropagation()
                    const current = getCookie('mmw-theme-style') || 'miaomiaowu'
                    if (current !== opt.value) {
                      setCookie(
                        'mmw-theme-style',
                        opt.value,
                        60 * 60 * 24 * 365
                      )
                      window.location.reload()
                    }
                  }}
                  className={`flex-1 border px-2 py-0.5 text-xs transition-colors ${
                    (getCookie('mmw-theme-style') || 'miaomiaowu') === opt.value
                      ? 'bg-primary text-primary-foreground border-primary'
                      : 'bg-background hover:bg-muted border-border'
                  }`}
                >
                  {opt.label}
                </button>
              ))}
            </div>
          </DropdownMenuItem>

          <DropdownMenuItem asChild className='cursor-pointer justify-center'>
            <a
              href='https://docs.miaomiaowu.net'
              target='_blank'
              rel='noopener noreferrer'
              className='flex items-center gap-2'
            >
              <BookOpen className='size-4' /> 使用帮助
            </a>
          </DropdownMenuItem>
          {profile?.is_admin && (
            <DropdownMenuItem
              onClick={() => setBackupDialogOpen(true)}
              className='cursor-pointer justify-center'
            >
              <HardDrive className='size-4' /> 备份数据
            </DropdownMenuItem>
          )}
          {profile?.is_admin && (
            <DropdownMenuItem
              onClick={() => setUpdateDialogOpen(true)}
              className='cursor-pointer justify-center'
            >
              <RefreshCw className='size-4' />
              <span className='relative'>
                检查更新
                {hasUpdate && (
                  <span className='absolute -top-1.5 -right-1.5 mt-2 flex size-1.5'>
                    <span className='bg-primary absolute inline-flex h-full w-full animate-ping rounded-full opacity-75'></span>
                    <span className='bg-primary relative inline-flex size-1.5 rounded-full'></span>
                  </span>
                )}
              </span>
            </DropdownMenuItem>
          )}
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild className='cursor-pointer justify-center'>
            <a
              href={releaseUrl}
              target='_blank'
              rel='noopener noreferrer'
              className='flex items-center gap-2'
            >
              <ExternalLink className='size-4' />
              版本 v{currentVersion}
            </a>
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => setMmwxDialogOpen(true)}
            className='cursor-pointer justify-center'
          >
            <Sparkles className='size-4' /> 妙妙屋X
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <DropdownMenuItem
            onClick={() => setOpen(true)}
            className='cursor-pointer justify-center'
          >
            <LogOut className='size-4' /> 退出登录
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <SignOutDialog
        open={Boolean(open)}
        onOpenChange={(value) => setOpen(value)}
      />
      <BackupDialog
        open={backupDialogOpen}
        onOpenChange={setBackupDialogOpen}
      />
      <UpdateDialog
        open={updateDialogOpen}
        onOpenChange={setUpdateDialogOpen}
      />
      <MmwxDialog open={mmwxDialogOpen} onOpenChange={setMmwxDialogOpen} />
    </>
  )
}
