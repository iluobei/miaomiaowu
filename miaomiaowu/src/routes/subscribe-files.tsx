import { createFileRoute, redirect, Outlet } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { Topbar } from '@/components/layout/topbar'

export const Route = createFileRoute('/subscribe-files')({
  beforeLoad: async () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: SubscribeFilesLayout,
})

function SubscribeFilesLayout() {
  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <Outlet />
    </div>
  )
}
