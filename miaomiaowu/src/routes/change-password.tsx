import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useEffect } from 'react'
import { useAuthStore } from '@/stores/auth-store'

export const Route = createFileRoute('/change-password')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: ChangePasswordRedirect,
})

function ChangePasswordRedirect() {
  const navigate = useNavigate()
  const { auth } = useAuthStore()

  useEffect(() => {
    if (!auth.accessToken) {
      navigate({ to: '/', replace: true })
      return
    }
    navigate({ to: '/settings', replace: true })
  }, [auth.accessToken, navigate])

  return null
}
