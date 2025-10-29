import { api } from '@/lib/api'

export interface ProfileResponse {
  username: string
  email: string
  nickname: string
  avatar_url: string
  role: string
  is_admin: boolean
}

export const profileQueryFn = async (): Promise<ProfileResponse> => {
  const response = await api.get('/api/user/profile')
  return response.data as ProfileResponse
}
