import axios, { AxiosError } from 'axios'
import { useAuthStore } from '@/stores/auth-store'

const AUTH_HEADER = 'MM-Authorization'
const rawConfiguredBaseURL = (import.meta.env.VITE_API_BASE_URL ?? '').trim()
const configuredBaseURL =
  import.meta.env.PROD && rawConfiguredBaseURL === 'http://localhost:8080'
    ? ''
    : rawConfiguredBaseURL

export const api = axios.create({
  baseURL: configuredBaseURL || undefined,
  withCredentials: false,
})

if (!api.defaults.baseURL && typeof window !== 'undefined' && window.location) {
  const { protocol, host, hostname } = window.location
  if (hostname === 'localhost' || hostname === '127.0.0.1' || hostname === '::1') {
    api.defaults.baseURL = `${protocol}//${hostname}:8080`
  } else {
    api.defaults.baseURL = `${protocol}//${host}`
  }
}

api.interceptors.request.use((config) => {
  const token = useAuthStore.getState().auth.accessToken
  if (token) {
    config.headers = config.headers ?? {}
    config.headers[AUTH_HEADER] = token
  }
  return config
})

api.interceptors.response.use(
  (response) => response,
  (error) => {
    if (error instanceof AxiosError) {
      if (error.response?.status === 401) {
        useAuthStore.getState().auth.reset()
        if (typeof window !== 'undefined' && window.location.pathname !== '/login') {
          window.location.href = '/login'
        }
      }
      // 静默模式返回 404 时跳转到 404 页面
      if (error.response?.status === 404 && error.response?.headers?.['x-silent-mode'] === 'true') {
        if (typeof window !== 'undefined' && window.location.pathname !== '/404') {
          window.location.href = '/404'
        }
      }
    }
    return Promise.reject(error)
  }
)
