import { useEffect, useState } from 'react'
import { useForm } from 'react-hook-form'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { Download } from 'lucide-react'
import { QRCodeSVG } from 'qrcode.react'
import { toast } from 'sonner'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { getCookie, setCookie } from '@/lib/cookies'
import { handleServerError } from '@/lib/handle-server-error'
import { profileQueryFn } from '@/lib/profile'
import { Avatar, AvatarFallback, AvatarImage } from '@/components/ui/avatar'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import {
  InputOTP,
  InputOTPGroup,
  InputOTPSlot,
} from '@/components/ui/input-otp'
import { Label } from '@/components/ui/label'
import { Topbar } from '@/components/layout/topbar'

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

  const resetShortLinkMutation = useMutation({
    mutationFn: async () => {
      const response = await api.post('/api/user/short-link')
      return response.data as { message: string }
    },
    onSuccess: () => {
      // Invalidate subscriptions to refresh short URLs
      queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
      toast.success('所有订阅的短链接已重置')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('重置短链接失败')
    },
  })

  const { data: shortCodeData } = useQuery({
    queryKey: ['user-custom-short-code'],
    queryFn: async () => {
      const response = await api.get('/api/user/custom-short-code')
      return response.data as { custom_short_code: string }
    },
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const [shortCodeInput, setShortCodeInput] = useState('')
  useEffect(() => {
    if (shortCodeData?.custom_short_code !== undefined) {
      setShortCodeInput(shortCodeData.custom_short_code)
    }
  }, [shortCodeData])

  const updateShortCodeMutation = useMutation({
    mutationFn: async (code: string) => {
      const response = await api.post('/api/user/custom-short-code', {
        custom_short_code: code.trim(),
      })
      return response.data
    },
    onSuccess: () => {
      toast.success('自定义连接已更新')
      queryClient.invalidateQueries({ queryKey: ['user-custom-short-code'] })
    },
    onError: (error) => {
      handleServerError(error)
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
  const fallbackAvatar = profile?.is_admin
    ? `${import.meta.env.BASE_URL}images/admin-avatar.webp`
    : `${import.meta.env.BASE_URL}images/user-avatar.png`
  const avatarSrc = profile?.avatar_url?.trim()
    ? profile.avatar_url.trim()
    : fallbackAvatar
  const avatarFallback = displayName.slice(0, 2) || '用户'
  const tokenValue = tokenData?.token ?? ''

  return (
    <div className='bg-background min-h-svh'>
      <Topbar />
      <main className='mx-auto w-full max-w-4xl px-4 py-8 pt-24 sm:px-6'>
        <section className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>个人设置</h1>
        </section>

        <div className='mt-8 grid gap-6 lg:grid-cols-2'>
          {/* 左侧：个人资料 */}
          <div className='space-y-6'>
            <Card>
              <CardHeader>
                <CardTitle>个人资料</CardTitle>
                <CardDescription>
                  修改用户名、昵称、邮箱和头像链接。
                </CardDescription>
              </CardHeader>
              <CardContent>
                <form className='space-y-5' onSubmit={submitProfile}>
                  <div className='flex items-center gap-4'>
                    <Avatar className='size-12'>
                      <AvatarImage src={avatarSrc} alt={displayName} />
                      <AvatarFallback>{avatarFallback}</AvatarFallback>
                    </Avatar>
                    <div className='text-muted-foreground text-sm'>
                      {profile?.is_admin
                        ? '管理员头像默认根据角色区分，设置自定义链接将覆盖默认头像。'
                        : '支持使用任意公开可访问的图片链接。'}
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
                      <p className='text-muted-foreground text-xs'>
                        管理员用户名暂不支持修改。
                      </p>
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

                  <Button
                    type='submit'
                    className='w-full'
                    disabled={updateProfileMutation.isPending}
                  >
                    {updateProfileMutation.isPending ? '保存中…' : '保存变更'}
                  </Button>
                </form>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>界面风格</CardTitle>
                <CardDescription>
                  选择界面显示风格，切换后页面将刷新
                </CardDescription>
              </CardHeader>
              <CardContent>
                <div className='flex gap-2'>
                  {[
                    { value: 'miaomiaowu', label: '妙妙屋' },
                    { value: 'flat', label: '扁平' },
                    { value: 'anime', label: '二次元' },
                  ].map((opt) => (
                    <button
                      key={opt.value}
                      type='button'
                      onClick={() => {
                        const current =
                          getCookie('mmw-theme-style') || 'miaomiaowu'
                        if (current !== opt.value) {
                          setCookie(
                            'mmw-theme-style',
                            opt.value,
                            60 * 60 * 24 * 365
                          )
                          window.location.reload()
                        }
                      }}
                      className={`flex items-center gap-2 rounded-md border px-3 py-1.5 text-sm transition-colors ${
                        (getCookie('mmw-theme-style') || 'miaomiaowu') ===
                        opt.value
                          ? 'bg-primary text-primary-foreground border-primary'
                          : 'bg-background hover:bg-muted border-border'
                      }`}
                    >
                      {opt.label}
                    </button>
                  ))}
                </div>
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle>自定义订阅连接</CardTitle>
                <CardDescription>
                  设置后短链接中将使用自定义连接替代系统随机生成的部分。只允许字母和数字，留空则使用系统默认。
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-3'>
                <div className='space-y-2'>
                  <Label htmlFor='custom_short_code'>自定义连接</Label>
                  <Input
                    id='custom_short_code'
                    placeholder='字母和数字'
                    value={shortCodeInput}
                    onChange={(e) => setShortCodeInput(e.target.value)}
                  />
                </div>
                <Button
                  className='w-full'
                  disabled={updateShortCodeMutation.isPending}
                  onClick={() => updateShortCodeMutation.mutate(shortCodeInput)}
                >
                  {updateShortCodeMutation.isPending ? '保存中…' : '保存'}
                </Button>
              </CardContent>
            </Card>
          </div>

          {/* 右侧：修改密码和订阅Token */}
          <div className='space-y-6'>
            <Card>
              <CardHeader>
                <CardTitle>修改密码</CardTitle>
                <CardDescription>
                  修改后需要使用新密码重新登录系统。
                </CardDescription>
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
                      {...passwordForm.register('current_password', {
                        required: true,
                      })}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='new_password'>新密码</Label>
                    <Input
                      id='new_password'
                      type='password'
                      autoComplete='new-password'
                      placeholder='至少 8 位，建议包含符号'
                      {...passwordForm.register('new_password', {
                        required: true,
                      })}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='confirm_password'>确认新密码</Label>
                    <Input
                      id='confirm_password'
                      type='password'
                      autoComplete='new-password'
                      placeholder='再次输入新密码'
                      {...passwordForm.register('confirm_password', {
                        required: true,
                      })}
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
                <CardDescription>
                  <p className='text-destructive mt-2 text-sm font-semibold'>
                    token用于客户端订阅，发生泄露后重置token只会影响更新订阅，为防止盗用，还需要修改服务器各个节点的鉴权凭证。
                  </p>
                </CardDescription>
              </CardHeader>
              <CardContent className='space-y-4'>
                <div className='bg-muted/40 rounded-md border p-3 font-mono text-xs break-all shadow-inner sm:text-sm'>
                  {loadingToken ? '加载中…' : tokenValue || '尚未生成'}
                </div>
                <div className='flex flex-wrap gap-2'>
                  <Button
                    size='sm'
                    variant='secondary'
                    disabled={!tokenValue || resetTokenMutation.isPending}
                    onClick={async () => {
                      if (!tokenValue) return
                      if (
                        typeof navigator !== 'undefined' &&
                        navigator.clipboard?.writeText
                      ) {
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

                <div className='space-y-2 border-t pt-4'>
                  <Label>订阅短链接</Label>
                  <p className='text-muted-foreground text-xs'>
                    重置所有订阅的短链接。短链接在订阅链接页面显示。
                  </p>
                  <Button
                    size='sm'
                    variant='outline'
                    disabled={resetShortLinkMutation.isPending}
                    onClick={() => resetShortLinkMutation.mutate()}
                    className='w-full'
                  >
                    {resetShortLinkMutation.isPending
                      ? '重置中…'
                      : '重置所有订阅短链接'}
                  </Button>
                </div>
              </CardContent>
            </Card>

            <TwoFactorCard />
          </div>
        </div>
      </main>
    </div>
  )
}

function TwoFactorCard() {
  const queryClient = useQueryClient()
  const { data: profile } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    staleTime: 5 * 60 * 1000,
  })
  const [setupOpen, setSetupOpen] = useState(false)
  const [disableOpen, setDisableOpen] = useState(false)
  const [setupStep, setSetupStep] = useState<
    'password' | 'qr' | 'verify' | 'recovery'
  >('password')
  const [setupPassword, setSetupPassword] = useState('')
  const [totpUrl, setTotpUrl] = useState('')
  const [totpSecret, setTotpSecret] = useState('')
  const [verifyCode, setVerifyCode] = useState('')
  const [recoveryCodes, setRecoveryCodes] = useState<string[]>([])
  const [disableCode, setDisableCode] = useState('')

  const { data: tfStatus } = useQuery({
    queryKey: ['2fa-status'],
    queryFn: async () => {
      const res = await api.get('/api/user/2fa/status')
      return res.data as { enabled: boolean }
    },
    staleTime: 30_000,
  })

  const setupMutation = useMutation({
    mutationFn: async (password: string) => {
      const res = await api.post('/api/user/2fa/setup', { password })
      return res.data as { secret: string; url: string }
    },
    onSuccess: (data) => {
      setTotpSecret(data.secret)
      setTotpUrl(data.url)
      setSetupStep('qr')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('密码验证失败')
    },
  })

  const verifySetupMutation = useMutation({
    mutationFn: async (code: string) => {
      const res = await api.post('/api/user/2fa/verify-setup', { code })
      return res.data as { recovery_codes: string[] }
    },
    onSuccess: (data) => {
      setRecoveryCodes(data.recovery_codes)
      setSetupStep('recovery')
      queryClient.invalidateQueries({ queryKey: ['2fa-status'] })
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('验证码无效')
      setVerifyCode('')
    },
  })

  const disableMutation = useMutation({
    mutationFn: async (code: string) => {
      await api.post('/api/user/2fa/disable', { code })
    },
    onSuccess: () => {
      toast.success('两步验证已禁用')
      setDisableOpen(false)
      setDisableCode('')
      queryClient.invalidateQueries({ queryKey: ['2fa-status'] })
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('验证码无效')
      setDisableCode('')
    },
  })

  const resetSetup = () => {
    setSetupStep('password')
    setSetupPassword('')
    setTotpUrl('')
    setTotpSecret('')
    setVerifyCode('')
    setRecoveryCodes([])
  }

  return (
    <>
      <Card>
        <CardHeader>
          <CardTitle>两步验证</CardTitle>
          <CardDescription>
            {tfStatus?.enabled
              ? '两步验证已启用，每次登录需要输入验证码。'
              : '启用后每次登录需要输入验证器应用中的验证码。'}
          </CardDescription>
        </CardHeader>
        <CardContent>
          {tfStatus?.enabled ? (
            <Button
              variant='destructive'
              className='w-full'
              onClick={() => setDisableOpen(true)}
            >
              禁用两步验证
            </Button>
          ) : (
            <Button
              className='w-full'
              onClick={() => {
                resetSetup()
                setSetupOpen(true)
              }}
            >
              启用两步验证
            </Button>
          )}
        </CardContent>
      </Card>

      <Dialog
        open={setupOpen}
        onOpenChange={(open) => {
          if (!open && setupStep !== 'recovery') {
            setSetupOpen(false)
            resetSetup()
          }
        }}
      >
        <DialogContent
          className='sm:max-w-md'
          onInteractOutside={(e) => {
            if (setupStep === 'recovery') e.preventDefault()
          }}
        >
          <DialogHeader>
            <DialogTitle>
              {setupStep === 'password' && '验证密码'}
              {setupStep === 'qr' && '扫描二维码'}
              {setupStep === 'verify' && '验证设置'}
              {setupStep === 'recovery' && '保存恢复码'}
            </DialogTitle>
            <DialogDescription>
              {setupStep === 'password' && '请输入当前密码以开始设置两步验证。'}
              {setupStep === 'qr' && '使用验证器应用扫描下方二维码。'}
              {setupStep === 'verify' && '输入验证器应用显示的 6 位验证码。'}
              {setupStep === 'recovery' &&
                '请妥善保存以下恢复码，用于在无法访问验证器时登录。'}
            </DialogDescription>
          </DialogHeader>

          {setupStep === 'password' && (
            <div className='space-y-4'>
              <Input
                type='password'
                placeholder='输入当前密码'
                value={setupPassword}
                onChange={(e) => setSetupPassword(e.target.value)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' && setupPassword)
                    setupMutation.mutate(setupPassword)
                }}
                autoFocus
              />
              <Button
                className='w-full'
                disabled={!setupPassword || setupMutation.isPending}
                onClick={() => setupMutation.mutate(setupPassword)}
              >
                {setupMutation.isPending ? '验证中...' : '下一步'}
              </Button>
            </div>
          )}

          {setupStep === 'qr' && (
            <div className='space-y-4'>
              <div className='flex justify-center rounded-lg border bg-white p-4'>
                <QRCodeSVG value={totpUrl} size={200} />
              </div>
              <div className='space-y-1'>
                <Label className='text-muted-foreground text-xs'>
                  手动输入密钥
                </Label>
                <div className='bg-muted/40 rounded-md border p-2 font-mono text-xs break-all select-all'>
                  {totpSecret}
                </div>
              </div>
              <Button className='w-full' onClick={() => setSetupStep('verify')}>
                下一步
              </Button>
            </div>
          )}

          {setupStep === 'verify' && (
            <div className='space-y-4'>
              <div className='flex justify-center'>
                <InputOTP
                  maxLength={6}
                  value={verifyCode}
                  onChange={setVerifyCode}
                  onComplete={(code) => verifySetupMutation.mutate(code)}
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
                disabled={
                  verifyCode.length !== 6 || verifySetupMutation.isPending
                }
                onClick={() => verifySetupMutation.mutate(verifyCode)}
              >
                {verifySetupMutation.isPending ? '验证中...' : '验证并启用'}
              </Button>
            </div>
          )}

          {setupStep === 'recovery' && (
            <div className='space-y-4'>
              <div className='bg-muted/40 grid grid-cols-2 gap-2 rounded-lg border p-3'>
                {recoveryCodes.map((code) => (
                  <div key={code} className='text-center font-mono text-sm'>
                    {code}
                  </div>
                ))}
              </div>
              <div className='grid grid-cols-2 gap-2'>
                <Button
                  variant='outline'
                  onClick={async () => {
                    try {
                      await navigator.clipboard.writeText(
                        recoveryCodes.join('\n')
                      )
                      toast.success('恢复码已复制')
                    } catch {
                      toast.error('复制失败，请手动复制')
                    }
                  }}
                >
                  复制恢复码
                </Button>
                <Button
                  variant='outline'
                  onClick={() => {
                    const text = recoveryCodes.join('\n')
                    const blob = new Blob([text], { type: 'text/plain' })
                    const url = URL.createObjectURL(blob)
                    const a = document.createElement('a')
                    a.href = url
                    a.download = `妙妙屋-${profile?.username || 'user'}.txt`
                    a.click()
                    URL.revokeObjectURL(url)
                  }}
                >
                  <Download className='mr-1 size-4' />
                  下载恢复码
                </Button>
              </div>
              <Button
                className='w-full'
                onClick={() => {
                  setSetupOpen(false)
                  resetSetup()
                }}
              >
                我已保存恢复码
              </Button>
            </div>
          )}
        </DialogContent>
      </Dialog>

      <Dialog
        open={disableOpen}
        onOpenChange={(open) => {
          if (!open) {
            setDisableOpen(false)
            setDisableCode('')
          }
        }}
      >
        <DialogContent className='sm:max-w-md'>
          <DialogHeader>
            <DialogTitle>禁用两步验证</DialogTitle>
            <DialogDescription>
              请输入验证器应用中的 6 位验证码以禁用两步验证。
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            <div className='flex justify-center'>
              <InputOTP
                maxLength={6}
                value={disableCode}
                onChange={setDisableCode}
                onComplete={(code) => disableMutation.mutate(code)}
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
              variant='destructive'
              className='w-full'
              disabled={disableCode.length !== 6 || disableMutation.isPending}
              onClick={() => disableMutation.mutate(disableCode)}
            >
              {disableMutation.isPending ? '禁用中...' : '确认禁用'}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </>
  )
}
