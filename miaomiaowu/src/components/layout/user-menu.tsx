import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { LogOut, Settings2, ExternalLink, BookOpen, HardDrive, RefreshCw } from 'lucide-react'
import useDialogState from '@/hooks/use-dialog-state'
import { SignOutDialog } from '@/components/sign-out-dialog'
import { BackupDialog } from '@/components/backup-dialog'
import { UpdateDialog } from '@/components/update-dialog'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { profileQueryFn } from '@/lib/profile'
import { useAuthStore } from '@/stores/auth-store'
import { useVersionCheck } from '@/hooks/use-version-check'

export function UserMenu() {
  const [open, setOpen] = useDialogState<boolean>()
  const [backupDialogOpen, setBackupDialogOpen] = useState(false)
  const [updateDialogOpen, setUpdateDialogOpen] = useState(false)
  const { auth } = useAuthStore()
  const { currentVersion, hasUpdate, releaseUrl } = useVersionCheck()
  const { data: profile } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const displayName = profile?.nickname || profile?.username || '用户'
  const fallbackAvatar = profile?.is_admin ? '/images/admin-avatar.webp' : '/images/user-avatar.png'
  const avatarSrc = profile?.avatar_url?.trim() ? profile.avatar_url.trim() : fallbackAvatar
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
            className='h-9 min-w-0 justify-center gap-2 px-2 py-2 overflow-hidden sm:min-w-[120px] sm:gap-2 sm:px-3'
          >
            <span className='sr-only'>{`用户菜单: ${displayName}`}</span>
            <Avatar className='size-7 border-[1.5px] border-[color:rgba(241,140,110,0.45)] shadow-[2px_2px_0_rgba(0,0,0,0.2)]'>
              <AvatarImage src={avatarSrc} alt={displayName} />
              <AvatarFallback>{fallbackText || '用户'}</AvatarFallback>
            </Avatar>
            <div className='hidden sm:flex sm:flex-col sm:items-center sm:leading-tight'>
              <span className='text-sm font-semibold truncate max-w-[70px]'>{displayName}</span>
              <span className='text-xs uppercase tracking-[0.2em] text-muted-foreground'>
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
              <p className='text-sm font-semibold leading-tight'>{displayName}</p>
              <p className='text-xs text-muted-foreground'>{profile?.username || '未登录'}</p>
              {emailText ? (
                <p className='text-xs text-muted-foreground break-all'>{emailText}</p>
              ) : (
                <p className='text-xs text-muted-foreground'>未填写邮箱</p>
              )}
            </div>
          </div>
          <DropdownMenuSeparator />
          <DropdownMenuItem asChild className='cursor-pointer justify-center'>
            <Link to='/settings' className='flex items-center gap-2'>
              <Settings2 className='size-4' /> 个人设置
            </Link>
          </DropdownMenuItem>
          <DropdownMenuItem asChild className='cursor-pointer justify-center'>
            <Link to='/docs' className='flex items-center gap-2'>
              <BookOpen className='size-4' /> 使用帮助
            </Link>
          </DropdownMenuItem>
          {profile?.is_admin && (
            <DropdownMenuItem onClick={() => setBackupDialogOpen(true)} className='cursor-pointer justify-center'>
              <HardDrive className='size-4' /> 备份数据
            </DropdownMenuItem>
          )}
          {profile?.is_admin && (
            <DropdownMenuItem onClick={() => setUpdateDialogOpen(true)} className='cursor-pointer justify-center'>
              <RefreshCw className='size-4' />
              <span className='relative'>
                检查更新
                {hasUpdate && (
                  <span className='absolute mt-2 -right-1.5 -top-1.5 flex size-1.5'>
                    <span className='absolute inline-flex h-full w-full animate-ping rounded-full bg-primary opacity-75'></span>
                    <span className='relative inline-flex size-1.5 rounded-full bg-primary'></span>
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
          <DropdownMenuSeparator />
          <DropdownMenuItem onClick={() => setOpen(true)} className='cursor-pointer justify-center'>
            <LogOut className='size-4' /> 退出登录
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>

      <SignOutDialog open={Boolean(open)} onOpenChange={(value) => setOpen(value)} />
      <BackupDialog open={backupDialogOpen} onOpenChange={setBackupDialogOpen} />
      <UpdateDialog open={updateDialogOpen} onOpenChange={setUpdateDialogOpen} />
    </>
  )
}
