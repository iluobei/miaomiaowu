// 自定义品牌:站点标题 / 左上角标题 / logo / 浏览器图标。
//
// 从妙妙屋X 迁移而来,去掉了 license 门控(miaomiaowu 无授权系统,对所有管理员直接可用)
// 与站点描述。
import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Upload } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
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

interface AdminBranding {
  branding: {
    site_title: string
    brand_title: string
    logo_url: string
    icon_url: string
  }
}

const IMAGE_ACCEPT =
  'image/png,image/jpeg,image/webp,image/gif,image/svg+xml,image/x-icon'

export function BrandingCard() {
  const queryClient = useQueryClient()
  const [siteTitle, setSiteTitle] = useState('')
  const [brandTitle, setBrandTitle] = useState('')
  const [logoUrl, setLogoUrl] = useState('')
  const [iconUrl, setIconUrl] = useState('')
  const logoFileRef = useRef<HTMLInputElement>(null)
  const iconFileRef = useRef<HTMLInputElement>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['admin-branding'],
    queryFn: async () =>
      (await api.get('/api/admin/system-settings/branding'))
        .data as AdminBranding,
  })

  useEffect(() => {
    if (data?.branding) {
      setSiteTitle(data.branding.site_title || '')
      setBrandTitle(data.branding.brand_title || '')
      setLogoUrl(data.branding.logo_url || '')
      setIconUrl(data.branding.icon_url || '')
    }
  }, [data])

  const refresh = () => {
    queryClient.invalidateQueries({ queryKey: ['admin-branding'] })
    queryClient.invalidateQueries({ queryKey: ['branding'] }) // 顶栏 / 标签页标题实时刷新
  }

  const saveMutation = useMutation({
    mutationFn: async () =>
      api.post('/api/admin/system-settings/branding', {
        site_title: siteTitle,
        brand_title: brandTitle,
        logo_url: logoUrl,
        icon_url: iconUrl,
      }),
    onSuccess: () => {
      toast.success('品牌设置已保存')
      refresh()
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '保存失败'),
  })

  const uploadLogoMutation = useMutation({
    mutationFn: async (file: File) => {
      const fd = new FormData()
      fd.append('logo', file)
      const res = await api.post(
        '/api/admin/system-settings/branding/logo',
        fd,
        { headers: { 'Content-Type': 'multipart/form-data' } }
      )
      return res.data as { logo_url: string }
    },
    onSuccess: (res) => {
      setLogoUrl(res.logo_url)
      toast.success('logo 已上传')
      refresh()
    },
    onError: (e: any) =>
      toast.error(e?.response?.data?.error || 'logo 上传失败'),
  })

  const uploadIconMutation = useMutation({
    mutationFn: async (file: File) => {
      const fd = new FormData()
      fd.append('icon', file)
      const res = await api.post(
        '/api/admin/system-settings/branding/icon',
        fd,
        { headers: { 'Content-Type': 'multipart/form-data' } }
      )
      return res.data as { icon_url: string }
    },
    onSuccess: (res) => {
      setIconUrl(res.icon_url)
      toast.success('浏览器图标已上传')
      refresh()
    },
    onError: (e: any) =>
      toast.error(e?.response?.data?.error || '浏览器图标上传失败'),
  })

  return (
    <Card>
      <CardHeader className='pb-4'>
        <CardTitle>自定义品牌</CardTitle>
        <CardDescription>
          自定义站点标题、左上角标题、Logo 与浏览器图标。标题与图标会注入首屏,
          登录页也一并生效。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='space-y-4'>
          <div className='space-y-1.5'>
            <Label>站点标题(浏览器标签页)</Label>
            <Input
              value={siteTitle}
              onChange={(e) => setSiteTitle(e.target.value)}
              placeholder='留空用默认'
            />
          </div>
          <div className='space-y-1.5'>
            <Label>左上角标题</Label>
            <Input
              value={brandTitle}
              onChange={(e) => setBrandTitle(e.target.value)}
              placeholder='留空用默认'
            />
          </div>
          <div className='space-y-1.5'>
            <Label>Logo(填图片 URL,或上传)</Label>
            <div className='flex gap-2'>
              <Input
                value={logoUrl}
                onChange={(e) => setLogoUrl(e.target.value)}
                placeholder='https://.../logo.png,留空用默认'
                className='flex-1'
              />
              <input
                ref={logoFileRef}
                type='file'
                accept={IMAGE_ACCEPT}
                className='hidden'
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) uploadLogoMutation.mutate(f)
                  e.target.value = ''
                }}
              />
              <Button
                type='button'
                variant='outline'
                onClick={() => logoFileRef.current?.click()}
                disabled={uploadLogoMutation.isPending}
              >
                {uploadLogoMutation.isPending ? (
                  <Loader2 className='h-4 w-4 animate-spin' />
                ) : (
                  <Upload className='h-4 w-4' />
                )}
                <span className='ml-1'>上传</span>
              </Button>
            </div>
            {logoUrl && (
              <img
                src={logoUrl}
                alt='logo 预览'
                className='mt-1 h-12 w-12 border object-contain'
              />
            )}
          </div>
          <div className='space-y-1.5'>
            <Label>浏览器图标(填图片 URL,或上传)</Label>
            <div className='flex gap-2'>
              <Input
                value={iconUrl}
                onChange={(e) => setIconUrl(e.target.value)}
                placeholder='https://.../favicon.ico,留空用默认'
                className='flex-1'
              />
              <input
                ref={iconFileRef}
                type='file'
                accept={IMAGE_ACCEPT}
                className='hidden'
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  if (f) uploadIconMutation.mutate(f)
                  e.target.value = ''
                }}
              />
              <Button
                type='button'
                variant='outline'
                onClick={() => iconFileRef.current?.click()}
                disabled={uploadIconMutation.isPending}
              >
                {uploadIconMutation.isPending ? (
                  <Loader2 className='h-4 w-4 animate-spin' />
                ) : (
                  <Upload className='h-4 w-4' />
                )}
                <span className='ml-1'>上传</span>
              </Button>
            </div>
            {iconUrl && (
              <img
                src={iconUrl}
                alt='浏览器图标预览'
                className='mt-1 h-8 w-8 border object-contain'
              />
            )}
          </div>
          <Button
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending || isLoading}
          >
            {saveMutation.isPending && (
              <Loader2 className='mr-1 h-4 w-4 animate-spin' />
            )}
            保存
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
