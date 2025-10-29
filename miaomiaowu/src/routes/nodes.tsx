// @ts-nocheck
import { Outlet } from '@tanstack/react-router'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'

// @ts-ignore - simple route definition retained
export const Route = createFileRoute('/nodes')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: NodesShell,
})

function NodesShell() {
  return <Outlet />
}
