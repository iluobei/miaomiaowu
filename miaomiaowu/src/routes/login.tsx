// @ts-nocheck
import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { Upload, AlertTriangle } from 'lucide-react'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { handleServerError } from '@/lib/handle-server-error'

export const Route = createFileRoute('/login')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (token) {
      throw redirect({ to: '/' })
    }
  },
  component: LoginPage,
})

type LoginFormValues = {
  username: string
  password: string
  remember_me: boolean
}

type SetupFormValues = {
  username: string
  password: string
  nickname: string
  email: string
  avatar_url: string
}

function LoginPage() {
  // Check if initial setup is needed
  const { data: setupStatus, isLoading: isCheckingSetup } = useQuery({
    queryKey: ['setup-status'],
    queryFn: async () => {
      const response = await api.get('/api/setup/status')
      return response.data as { needs_setup: boolean }
    },
    staleTime: Infinity,
  })

  if (isCheckingSetup) {
    return (
      <div className='flex min-h-svh items-center justify-center bg-background'>
        <Card className='w-full max-w-sm'>
          <CardHeader className='space-y-2 text-center'>
            <CardTitle>加载中...</CardTitle>
            <CardDescription>正在检查系统状态</CardDescription>
          </CardHeader>
        </Card>
      </div>
    )
  }

  if (setupStatus?.needs_setup) {
    return <InitialSetupView />
  }

  return <LoginView />
}

function LoginView() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { auth } = useAuthStore()
  const form = useForm<LoginFormValues>({
    defaultValues: {
      username: '',
      password: '',
      remember_me: false,
    },
  })

  const login = useMutation({
    mutationFn: async (values: LoginFormValues) => {
      const response = await api.post('/api/login', values)
      return response.data as {
        token: string
        expires_at: string
        username: string
        email: string
        nickname: string
        role: string
        is_admin: boolean
      }
    },
    onSuccess: (payload) => {
      auth.setAccessToken(payload.token)
      queryClient.invalidateQueries({ queryKey: ['traffic-summary'] })
      queryClient.setQueryData(['profile'], {
        username: payload.username,
        email: payload.email,
        nickname: payload.nickname,
        role: payload.role,
        is_admin: payload.is_admin,
      })
      toast.success('登录成功')
      form.reset()
      navigate({ to: '/' })
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('登录失败，请检查账号或密码')
    },
  })

  const onSubmit = form.handleSubmit((values) => {
    login.mutate(values)
  })

  return (
    <div className='flex min-h-svh items-center justify-center bg-[radial-gradient(circle_at_top,_var(--tw-gradient-stops))] from-background via-muted/40 to-muted/60 px-4 py-12'>
      <Card className='w-full max-w-sm shadow-lg'>
        <CardHeader className='space-y-2 text-center'>
          <CardTitle className='text-2xl font-semibold'>登录妙妙屋</CardTitle>
          <CardDescription>请输入管理员账号以访问控制台。</CardDescription>
        </CardHeader>
        <CardContent>
          <form className='space-y-6' onSubmit={onSubmit}>
            <div className='space-y-2'>
              <Label htmlFor='username'>用户名</Label>
              <Input
                id='username'
                name='username'
                type='text'
                autoCapitalize='none'
                autoComplete='username'
                autoFocus
                placeholder='请输入用户名'
                {...form.register('username', { required: true })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='password'>密码</Label>
              <Input
                id='password'
                name='password'
                type='password'
                autoComplete='current-password'
                placeholder='请输入密码'
                {...form.register('password', { required: true })}
              />
            </div>
            <div className='flex items-center space-x-2'>
              <Checkbox
                id='remember_me'
                checked={form.watch('remember_me')}
                onCheckedChange={(checked) => form.setValue('remember_me', checked === true)}
              />
              <Label htmlFor='remember_me' className='text-sm font-normal cursor-pointer'>
                记住我
              </Label>
            </div>
            <Button type='submit' className='w-full' disabled={login.isPending}>
              {login.isPending ? '登录中...' : '登录'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

function InitialSetupView() {
  const queryClient = useQueryClient()
  const [backupFile, setBackupFile] = useState<File | null>(null)
  const form = useForm<SetupFormValues>({
    defaultValues: {
      username: '',
      password: '',
      nickname: '',
      email: '',
      avatar_url: '',
    },
  })

  const setup = useMutation({
    mutationFn: async (values: SetupFormValues) => {
      const response = await api.post('/api/setup/init', values)
      return response.data as {
        username: string
        nickname: string
        email: string
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['setup-status'] })
      toast.success('首次初始化成功！请使用刚才创建的账号登录。')
      form.reset()
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('初始化失败，请重试')
    },
  })

  const restoreBackup = useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData()
      formData.append('backup', file)
      return api.post('/api/setup/restore-backup', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['setup-status'] })
      toast.success('备份恢复成功！请刷新页面后登录。')
      setBackupFile(null)
      // Reload page after a short delay
      setTimeout(() => {
        window.location.reload()
      }, 1500)
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('备份恢复失败')
    },
  })

  const onSubmit = form.handleSubmit((values) => {
    setup.mutate(values)
  })

  return (
    <div className='flex min-h-svh items-center justify-center bg-[radial-gradient(circle_at_top,_var(--tw-gradient-stops))] from-background via-muted/40 to-muted/60 px-4 py-12'>
      <Card className='w-full max-w-md shadow-lg'>
        <CardHeader className='space-y-2 text-center'>
          <CardTitle className='text-2xl font-semibold'>欢迎使用妙妙屋</CardTitle>
          <CardDescription>
            这是首次启动，请创建管理员账号。首次注册的用户将自动成为管理员。
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form className='space-y-4' onSubmit={onSubmit}>
            <div className='space-y-2'>
              <Label htmlFor='setup-username'>
                用户名 <span className='text-destructive'>*</span>
              </Label>
              <Input
                id='setup-username'
                name='username'
                type='text'
                autoCapitalize='none'
                autoComplete='username'
                autoFocus
                placeholder='请输入用户名'
                {...form.register('username', { required: true })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='setup-password'>
                密码 <span className='text-destructive'>*</span>
              </Label>
              <Input
                id='setup-password'
                name='password'
                type='password'
                autoComplete='new-password'
                placeholder='请输入密码'
                {...form.register('password', { required: true })}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='setup-nickname'>昵称</Label>
              <Input
                id='setup-nickname'
                name='nickname'
                type='text'
                autoComplete='name'
                placeholder='留空则使用用户名'
                {...form.register('nickname')}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='setup-email'>邮箱</Label>
              <Input
                id='setup-email'
                name='email'
                type='email'
                autoComplete='email'
                placeholder='可选'
                {...form.register('email')}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='setup-avatar'>头像地址</Label>
              <Input
                id='setup-avatar'
                name='avatar_url'
                type='url'
                autoComplete='url'
                placeholder='可选，填写头像图片URL'
                {...form.register('avatar_url')}
              />
            </div>
            <Button type='submit' className='w-full' disabled={setup.isPending}>
              {setup.isPending ? '创建中...' : '创建管理员账号'}
            </Button>
          </form>

          {/* Divider */}
          <div className='relative my-6'>
            <div className='absolute inset-0 flex items-center'>
              <span className='w-full border-t' />
            </div>
            <div className='relative flex justify-center text-xs uppercase'>
              <span className='bg-card px-2 text-muted-foreground'>或</span>
            </div>
          </div>

          {/* Restore from backup */}
          <div className='space-y-3'>
            <Label>从备份恢复</Label>
            <Input
              type='file'
              accept='.zip'
              onChange={(e) => setBackupFile(e.target.files?.[0] || null)}
              className='cursor-pointer'
            />
            <Button
              type='button'
              onClick={() => backupFile && restoreBackup.mutate(backupFile)}
              disabled={!backupFile || restoreBackup.isPending}
              variant='outline'
              className='w-full'
            >
              <Upload className='size-4 mr-2' />
              {restoreBackup.isPending ? '恢复中...' : '从备份恢复'}
            </Button>
            <div className='flex items-start gap-2 text-xs text-muted-foreground'>
              <AlertTriangle className='size-4 shrink-0 text-amber-500' />
              <span>如果您有之前的备份文件，可以在这里恢复数据</span>
            </div>
          </div>
        </CardContent>
      </Card>
    </div>
  )
}
