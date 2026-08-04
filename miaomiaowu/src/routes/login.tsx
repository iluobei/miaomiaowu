// @ts-nocheck
import { useEffect, useRef, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { toast } from 'sonner'
import { Upload, AlertTriangle, ArrowLeft } from 'lucide-react'
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
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from '@/components/ui/input-otp'
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
      <div className='login-pixel-bg flex min-h-svh items-center justify-center'>
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

type LoginResponse = {
  token: string
  expires_at: string
  username: string
  email: string
  nickname: string
  role: string
  is_admin: boolean
}

function handleLoginSuccess(
  payload: LoginResponse,
  auth: ReturnType<typeof useAuthStore>['auth'],
  queryClient: ReturnType<typeof useQueryClient>,
  navigate: ReturnType<typeof useNavigate>,
) {
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
  navigate({ to: '/' })
}

function LoginView() {
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const { auth } = useAuthStore()
  const [twoFactorToken, setTwoFactorToken] = useState<string | null>(null)
  const [turnstileToken, setTurnstileToken] = useState('')
  const { data: captchaConfig } = useQuery({
    queryKey: ['captcha-config'],
    queryFn: async () => (await api.get('/api/captcha/config')).data as { enabled: boolean; site_key: string },
  })
  const form = useForm<LoginFormValues>({
    defaultValues: {
      username: '',
      password: '',
      remember_me: false,
    },
  })

  const login = useMutation({
    mutationFn: async (values: LoginFormValues) => {
      const response = await api.post('/api/login', { ...values, turnstile_token: turnstileToken })
      return response.data as (LoginResponse & { requires_2fa?: boolean; two_factor_token?: string })
    },
    onSuccess: (payload) => {
      if (payload.requires_2fa && payload.two_factor_token) {
        setTwoFactorToken(payload.two_factor_token)
        return
      }
      form.reset()
      handleLoginSuccess(payload, auth, queryClient, navigate)
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('登录失败，请检查账号或密码')
    },
  })

  const onSubmit = form.handleSubmit((values) => {
	if (captchaConfig?.enabled && !turnstileToken) {
	  toast.error('请先完成人机验证')
	  return
	}
    login.mutate(values)
  })

  if (twoFactorToken) {
    return (
      <TwoFactorStep
        twoFactorToken={twoFactorToken}
        onBack={() => setTwoFactorToken(null)}
        onSuccess={(payload) => handleLoginSuccess(payload, auth, queryClient, navigate)}
      />
    )
  }

  return (
    <div className='login-pixel-bg flex min-h-svh items-center justify-center px-4 py-12'>
      <Card className='w-full max-w-sm shadow-lg'>
        <CardHeader className='space-y-2 text-center'>
          <CardTitle className='text-2xl font-semibold'>登录妙妙屋</CardTitle>
          <CardDescription>请输入用户账号以访问妙妙屋</CardDescription>
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
            {captchaConfig?.enabled && captchaConfig.site_key && (
              <TurnstileWidget siteKey={captchaConfig.site_key} onToken={setTurnstileToken} />
            )}
            <Button type='submit' className='w-full' disabled={login.isPending}>
              {login.isPending ? '登录中...' : '登录'}
            </Button>
          </form>
        </CardContent>
      </Card>
    </div>
  )
}

function TurnstileWidget({ siteKey, onToken }: { siteKey: string; onToken: (token: string) => void }) {
  const containerRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    const render = () => {
      const turnstile = (window as any).turnstile
      if (turnstile && containerRef.current) {
        containerRef.current.innerHTML = ''
        turnstile.render(containerRef.current, {
          sitekey: siteKey,
          callback: onToken,
          'expired-callback': () => onToken(''),
          'error-callback': () => onToken(''),
        })
      }
    }
    const existing = document.querySelector('script[data-turnstile]') as HTMLScriptElement | null
    if (existing) {
      if ((window as any).turnstile) render()
      else existing.addEventListener('load', render, { once: true })
    } else {
      const script = document.createElement('script')
      script.src = 'https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit'
      script.async = true
      script.defer = true
      script.dataset.turnstile = 'true'
      script.addEventListener('load', render, { once: true })
      document.head.appendChild(script)
    }
    return () => onToken('')
  }, [siteKey, onToken])
  return <div ref={containerRef} className='flex min-h-[65px] justify-center' />
}

function TwoFactorStep({
  twoFactorToken,
  onBack,
  onSuccess,
}: {
  twoFactorToken: string
  onBack: () => void
  onSuccess: (payload: LoginResponse) => void
}) {
  const [otpCode, setOtpCode] = useState('')
  const [useRecovery, setUseRecovery] = useState(false)
  const [recoveryCode, setRecoveryCode] = useState('')

  const verify2FA = useMutation({
    mutationFn: async (code: string) => {
      const response = await api.post('/api/login/2fa', {
        two_factor_token: twoFactorToken,
        code,
      })
      return response.data as LoginResponse
    },
    onSuccess: (payload) => onSuccess(payload),
    onError: (error) => {
      handleServerError(error)
      toast.error('验证码无效')
      setOtpCode('')
    },
  })

  const verifyRecovery = useMutation({
    mutationFn: async (code: string) => {
      const response = await api.post('/api/login/recovery', {
        two_factor_token: twoFactorToken,
        recovery_code: code,
      })
      return response.data as LoginResponse
    },
    onSuccess: (payload) => {
      toast.success('恢复码验证成功，两步验证已重设')
      onSuccess(payload)
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('恢复码无效')
    },
  })

  return (
    <div className='login-pixel-bg flex min-h-svh items-center justify-center px-4 py-12'>
      <Card className='w-full max-w-sm shadow-lg'>
        <CardHeader className='space-y-2 text-center'>
          <CardTitle className='text-2xl font-semibold'>两步验证</CardTitle>
          <CardDescription>
            {useRecovery ? '请输入恢复码' : '请输入验证器应用中的 6 位验证码'}
          </CardDescription>
        </CardHeader>
        <CardContent className='space-y-6'>
          {useRecovery ? (
            <div className='space-y-4'>
              <Input
                value={recoveryCode}
                onChange={(e) => setRecoveryCode(e.target.value)}
                placeholder='输入 8 位恢复码'
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && recoveryCode.trim()) {
                    verifyRecovery.mutate(recoveryCode.trim())
                  }
                }}
              />
              <Button
                className='w-full'
                onClick={() => verifyRecovery.mutate(recoveryCode.trim())}
                disabled={!recoveryCode.trim() || verifyRecovery.isPending}
              >
                {verifyRecovery.isPending ? '验证中...' : '使用恢复码登录'}
              </Button>
            </div>
          ) : (
            <div className='space-y-4'>
              <div className='flex justify-center'>
                <InputOTP
                  maxLength={6}
                  value={otpCode}
                  onChange={setOtpCode}
                  onComplete={(code) => verify2FA.mutate(code)}
                  autoFocus
                >
                  <InputOTPGroup>
                    <InputOTPSlot index={0} />
                    <InputOTPSlot index={1} />
                    <InputOTPSlot index={2} />
                  </InputOTPGroup>
                  <InputOTPGroup>
                    <InputOTPSlot index={3} />
                    <InputOTPSlot index={4} />
                    <InputOTPSlot index={5} />
                  </InputOTPGroup>
                </InputOTP>
              </div>
              <Button
                className='w-full'
                onClick={() => verify2FA.mutate(otpCode)}
                disabled={otpCode.length !== 6 || verify2FA.isPending}
              >
                {verify2FA.isPending ? '验证中...' : '验证'}
              </Button>
            </div>
          )}
          <div className='flex items-center justify-between text-sm'>
            <button
              type='button'
              className='text-muted-foreground hover:text-foreground transition-colors flex items-center gap-1'
              onClick={onBack}
            >
              <ArrowLeft className='size-3' />
              返回
            </button>
            <button
              type='button'
              className='text-muted-foreground hover:text-foreground transition-colors'
              onClick={() => {
                setUseRecovery(!useRecovery)
                setOtpCode('')
                setRecoveryCode('')
              }}
            >
              {useRecovery ? '使用验证码' : '使用恢复码'}
            </button>
          </div>
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
    <div className='login-pixel-bg flex min-h-svh items-center justify-center px-4 py-12'>
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
