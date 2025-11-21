import { useEffect } from 'react'
import { useForm } from 'react-hook-form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { Topbar } from '@/components/layout/topbar'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { profileQueryFn } from '@/lib/profile'
import { useAuthStore } from '@/stores/auth-store'

type ProfileFormValues = {
  username: string
  nickname: string
  email: string
  avatar_url: string
}

type PasswordFormValues = {
  current_password: string
  new_password: string
  confirm_password: string
}

export const Route = createFileRoute('/settings')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: SettingsPage,
})

function SettingsPage() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { auth } = useAuthStore()

  const { data: profile, isLoading: loadingProfile } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const { data: tokenData, isLoading: loadingToken } = useQuery({
    queryKey: ['user-token'],
    queryFn: async () => {
      const response = await api.get('/api/user/token')
      return response.data as { token: string }
    },
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const profileForm = useForm<ProfileFormValues>({
    defaultValues: {
      username: '',
      nickname: '',
      email: '',
      avatar_url: '',
    },
  })

  useEffect(() => {
    if (profile) {
      profileForm.reset({
        username: profile.username,
        nickname: profile.nickname,
        email: profile.email,
        avatar_url: profile.avatar_url,
      })
    }
  }, [profile, profileForm])

  const updateProfileMutation = useMutation({
    mutationFn: async (values: ProfileFormValues) => {
      const payload = {
        username: values.username.trim(),
        nickname: values.nickname.trim(),
        email: values.email.trim(),
        avatar_url: values.avatar_url.trim(),
      }
      const response = await api.put('/api/user/settings', payload)
      return response.data as { profile: ProfileFormValues }
    },
    onSuccess: () => {
      toast.success('个人信息已更新')
      queryClient.invalidateQueries({ queryKey: ['profile'] })
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('更新个人信息失败')
    },
  })

  const resetTokenMutation = useMutation({
    mutationFn: async () => {
      const response = await api.post('/api/user/token')
      return response.data as { token: string }
    },
    onSuccess: (payload) => {
      queryClient.setQueryData(['user-token'], payload)
      toast.success('Token 已重置')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('重置 Token 失败')
    },
  })

  const passwordForm = useForm<PasswordFormValues>({
    defaultValues: {
      current_password: '',
      new_password: '',
      confirm_password: '',
    },
  })

  const changePasswordMutation = useMutation({
    mutationFn: async (values: PasswordFormValues) => {
      const response = await api.post('/api/user/password', {
        current_password: values.current_password,
        new_password: values.new_password,
      })
      return response.data
    },
    onSuccess: () => {
      toast.success('密码已更新，请重新登录')
      passwordForm.reset()
      auth.reset()
      navigate({ to: '/', replace: true })
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('修改密码失败')
    },
  })

  const submitProfile = profileForm.handleSubmit((values) => {
    if (!values.username.trim()) {
      toast.error('用户名不能为空')
      return
    }

    if (profile?.is_admin && values.username.trim() !== profile.username) {
      toast.error('管理员用户名不可修改')
      return
    }

    updateProfileMutation.mutate(values)
  })

  const submitPassword = passwordForm.handleSubmit((values) => {
    if (values.new_password.trim().length < 8) {
      toast.error('新密码至少 8 位')
      return
    }

    if (values.new_password !== values.confirm_password) {
      toast.error('两次输入的新密码不一致')
      return
    }

    changePasswordMutation.mutate(values)
  })

  const displayName = profile?.nickname || profile?.username || '用户'
  const fallbackAvatar = profile?.is_admin ? '/images/admin-avatar.webp' : '/images/user-avatar.png'
  const avatarSrc = profile?.avatar_url?.trim() ? profile.avatar_url.trim() : fallbackAvatar
  const avatarFallback = displayName.slice(0, 2) || '用户'
  const tokenValue = tokenData?.token ?? ''

  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <main className='mx-auto w-full max-w-4xl px-4 py-8 sm:px-6 pt-24'>
        <section className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>个人设置</h1>
        </section>

        <div className='mt-8 grid gap-6 lg:grid-cols-2'>
          {/* 左侧：个人资料 */}
          <div className='space-y-6'>
            <Card>
              <CardHeader>
                <CardTitle>个人资料</CardTitle>
                <CardDescription>修改用户名、昵称、邮箱和头像链接。</CardDescription>
              </CardHeader>
              <CardContent>
                <form className='space-y-5' onSubmit={submitProfile}>
                  <div className='flex items-center gap-4'>
                    <Avatar className='size-12'>
                      <AvatarImage src={avatarSrc} alt={displayName} />
                      <AvatarFallback>{avatarFallback}</AvatarFallback>
                    </Avatar>
                    <div className='text-sm text-muted-foreground'>
                      {profile?.is_admin ? '管理员头像默认根据角色区分，设置自定义链接将覆盖默认头像。' : '支持使用任意公开可访问的图片链接。'}
                    </div>
                  </div>

                  <div className='space-y-2'>
                    <Label htmlFor='username'>用户名</Label>
                    <Input
                      id='username'
                      placeholder='用于登录的用户名'
                      disabled={loadingProfile || profile?.is_admin}
                      {...profileForm.register('username', { required: true })}
                    />
                    {profile?.is_admin ? (
                      <p className='text-xs text-muted-foreground'>管理员用户名暂不支持修改。</p>
                    ) : null}
                  </div>

                  <div className='space-y-2'>
                    <Label htmlFor='nickname'>昵称</Label>
                    <Input
                      id='nickname'
                      placeholder='用于展示的昵称'
                      disabled={loadingProfile}
                      {...profileForm.register('nickname')}
                    />
                  </div>

                  <div className='space-y-2'>
                    <Label htmlFor='email'>邮箱 (暂不可用)</Label>
                    <Input
                      id='email'
                      type='email'
                      placeholder='用于接收通知 (可选)'
                      disabled={loadingProfile}
                      {...profileForm.register('email')}
                    />
                  </div>

                  <div className='space-y-2'>
                    <Label htmlFor='avatar_url'>头像链接</Label>
                    <Input
                      id='avatar_url'
                      placeholder='https://example.com/avatar.png'
                      disabled={loadingProfile}
                      {...profileForm.register('avatar_url')}
                    />
                  </div>

                  <Button type='submit' className='w-full' disabled={updateProfileMutation.isPending}>
                    {updateProfileMutation.isPending ? '保存中…' : '保存变更'}
                  </Button>
                </form>
              </CardContent>
            </Card>
          </div>

          {/* 右侧：修改密码和订阅Token */}
          <div className='space-y-6'>
            <Card>
              <CardHeader>
                <CardTitle>修改密码</CardTitle>
                <CardDescription>修改后需要使用新密码重新登录系统。</CardDescription>
              </CardHeader>
              <CardContent>
                <form className='space-y-4' onSubmit={submitPassword}>
                  <div className='space-y-2'>
                    <Label htmlFor='current_password'>当前密码</Label>
                    <Input
                      id='current_password'
                      type='password'
                      autoComplete='current-password'
                      placeholder='请输入当前密码'
                      {...passwordForm.register('current_password', { required: true })}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='new_password'>新密码</Label>
                    <Input
                      id='new_password'
                      type='password'
                      autoComplete='new-password'
                      placeholder='至少 8 位，建议包含符号'
                      {...passwordForm.register('new_password', { required: true })}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='confirm_password'>确认新密码</Label>
                    <Input
                      id='confirm_password'
                      type='password'
                      autoComplete='new-password'
                      placeholder='再次输入新密码'
                      {...passwordForm.register('confirm_password', { required: true })}
                    />
                  </div>
                  <Button
                    type='submit'
                    className='w-full'
                    disabled={changePasswordMutation.isPending}
                  >
                    {changePasswordMutation.isPending ? '修改中…' : '更新密码'}
                  </Button>
                </form>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>订阅 Token</CardTitle>
                <CardDescription><p className='mt-2 text-sm font-semibold text-destructive'>token用于客户端订阅，发生泄露后重置token只会影响更新订阅，为防止盗用，还需要修改服务器各个节点的鉴权凭证。</p></CardDescription>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='font-mono text-xs sm:text-sm break-all rounded-md border bg-muted/40 p-3 shadow-inner'>
                  {loadingToken ? '加载中…' : tokenValue || '尚未生成'}
                </div>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    size='sm'
                    variant='secondary'
                    disabled={!tokenValue || resetTokenMutation.isPending}
                    onClick={async () => {
                      if (!tokenValue) return
                      if (typeof navigator !== 'undefined' && navigator.clipboard?.writeText) {
                        try {
                          await navigator.clipboard.writeText(tokenValue)
                          toast.success('Token 已复制')
                          return
                        } catch (error) {
                          console.error('copy token failed', error)
                        }
                      }
                      toast.error('复制失败(需要https)，请手动复制')
                    }}
                  >
                    复制 Token
                  </Button>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={resetTokenMutation.isPending}
                    onClick={() => resetTokenMutation.mutate()}
                  >
                    {resetTokenMutation.isPending ? '重置中…' : '重置 Token'}
                  </Button>
                </div>
              </CardContent>
            </Card>
          </div>
        </div>
      </main>
    </div>
  )
}
