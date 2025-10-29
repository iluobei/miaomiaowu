import { useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useAuthStore } from '@/stores/auth-store'
import { ConfirmDialog } from '@/components/confirm-dialog'

interface SignOutDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function SignOutDialog({ open, onOpenChange }: SignOutDialogProps) {
  const navigate = useNavigate()
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()

  const handleSignOut = () => {
    auth.reset()
    queryClient.removeQueries({ queryKey: ['traffic-summary'] })
    queryClient.removeQueries({ queryKey: ['user-token'] })
    queryClient.removeQueries({ queryKey: ['profile'] })
    navigate({
      to: '/',
      replace: true,
    })
  }

  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title='退出登录'
      desc='确定要退出登录吗？退出后需要重新登录才能访问控制台。'
      confirmText='确认退出'
      cancelBtnText='取消'
      handleConfirm={handleSignOut}
      className='sm:max-w-sm'
    />
  )
}
