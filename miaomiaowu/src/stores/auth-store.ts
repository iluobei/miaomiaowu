import { create } from 'zustand'
import { getCookie, setCookie, removeCookie } from '@/lib/cookies'

const TOKEN_COOKIE = 'traffic_info_access_token'

interface AuthState {
  auth: {
    accessToken: string
    setAccessToken: (accessToken: string) => void
    reset: () => void
  }
}

export const useAuthStore = create<AuthState>()((set) => {
  const cookieState = getCookie(TOKEN_COOKIE)
  const initToken = cookieState ? JSON.parse(cookieState) : ''

  return {
    auth: {
      accessToken: initToken,
      setAccessToken: (accessToken) =>
        set((state) => {
          if (accessToken) {
            setCookie(TOKEN_COOKIE, JSON.stringify(accessToken))
          } else {
            removeCookie(TOKEN_COOKIE)
          }
          return { ...state, auth: { ...state.auth, accessToken } }
        }),
      reset: () =>
        set((state) => {
          removeCookie(TOKEN_COOKIE)
          return {
            ...state,
            auth: { ...state.auth, accessToken: '' },
          }
        }),
    },
  }
})
