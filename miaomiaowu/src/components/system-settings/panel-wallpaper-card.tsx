// 面板壁纸 —— 液态玻璃主题的背景层。
//
// 它不是「换张图」这么简单:玻璃需要有东西可以折射,没有背景的玻璃只是一块灰。
// 所以壁纸和主题是一起设计的,而且默认就带一张内置极光(纯 CSS,不依赖图片)。
//
// 设置只对液态玻璃主题有视觉意义 —— 其他主题是实色背景,壁纸透不出来。卡片里写清了
// 这一点,免得有人在扁平主题下设了图然后以为坏了。
//
// 从妙妙屋X 迁移而来,去掉了「跟随登录页壁纸」(miaomiaowu 面板壁纸经首屏注入对登录页
// 与面板一体生效)与 license 门控(miaomiaowu 无授权系统,液态玻璃免费)。
import { useEffect, useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Loader2, Upload, Wallpaper } from 'lucide-react'
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
import { Switch } from '@/components/ui/switch'

// 与后端 safeGlassTone / glass-theme.css 的四个预设 + custom 对应。
const GLASS_TONES = ['sea', 'amber', 'ice', 'graphite', 'custom'] as const

interface PanelWallpaperConfig {
  panel_wallpaper: string
  panel_wallpaper_dim: number
  panel_wallpaper_tone: string
  panel_wallpaper_tone_a: string
  panel_wallpaper_tone_b: string
  reduce_transparency: boolean
}

const EMPTY: PanelWallpaperConfig = {
  panel_wallpaper: '',
  panel_wallpaper_dim: 22,
  panel_wallpaper_tone: 'sea',
  panel_wallpaper_tone_a: '#4cb8f5',
  panel_wallpaper_tone_b: '#f0a35b',
  reduce_transparency: false,
}

const TONE_LABELS: Record<string, string> = {
  sea: '海玻璃',
  amber: '琥珀',
  ice: '冰蓝',
  graphite: '石墨',
  custom: '自定义',
}

// 与 glass-theme.css 里的四个色调对应,只用于预览色块。
const TONE_PREVIEW: Record<string, string> = {
  sea: 'radial-gradient(circle at 30% 30%, #4cb8f5, #0b1220)',
  amber: 'radial-gradient(circle at 30% 30%, #f0a35b, #1a1208)',
  ice: 'radial-gradient(circle at 30% 30%, #c9d6ff, #0b1220)',
  graphite: 'radial-gradient(circle at 30% 30%, #6b7785, #0b0e14)',
}

// 自定义那一格的预览用当前两色现算,不能进上面那张静态表。
const customPreview = (a: string, b: string) =>
  `radial-gradient(circle at 28% 28%, ${a}, ${b} 62%, #0b1220)`

export function PanelWallpaperCard() {
  const queryClient = useQueryClient()
  const [cfg, setCfg] = useState<PanelWallpaperConfig>(EMPTY)
  const [dirty, setDirty] = useState(false)

  const { data } = useQuery({
    queryKey: ['panel-wallpaper'],
    queryFn: async () =>
      (await api.get('/api/admin/system-settings/panel-wallpaper')).data as {
        config: PanelWallpaperConfig
      },
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    // 只在没改过时回填,免得后台刷新冲掉正在编辑的内容。
    if (data?.config && !dirty) setCfg({ ...EMPTY, ...data.config })
  }, [data, dirty])

  const set = (patch: Partial<PanelWallpaperConfig>) => {
    setCfg((p) => ({ ...p, ...patch }))
    setDirty(true)
  }

  const saveMutation = useMutation({
    mutationFn: async () =>
      (await api.put('/api/admin/system-settings/panel-wallpaper', cfg)).data,
    onSuccess: () => {
      setDirty(false)
      queryClient.invalidateQueries({ queryKey: ['panel-wallpaper'] })
      // 背景参数是首屏注入的,当前这一页仍是旧的 —— 直接刷新比让用户自己猜要好。
      toast.success('面板外观已保存,正在刷新以套用')
      setTimeout(() => window.location.reload(), 600)
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '保存失败'),
  })

  const fileRef = useRef<HTMLInputElement>(null)
  // 上传是**独立于保存**的一条路:服务端写完盘就把 panel_wallpaper 指过去并触发
  // 首屏同步,所以这里直接刷新页面,不需要用户回去再点一次保存。
  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      const form = new FormData()
      form.append('file', file)
      return (
        await api.post(
          '/api/admin/system-settings/panel-wallpaper/upload',
          form,
          { headers: { 'Content-Type': 'multipart/form-data' } }
        )
      ).data as { panel_wallpaper: string }
    },
    onSuccess: () => {
      setDirty(false)
      toast.success('壁纸已上传并应用,正在刷新')
      setTimeout(() => window.location.reload(), 600)
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '上传失败'),
  })

  return (
    <Card>
      <CardHeader className='pb-4'>
        <CardTitle className='flex items-center gap-2'>
          <Wallpaper className='size-5' /> 面板壁纸
        </CardTitle>
        <CardDescription>
          液态玻璃主题的背景层。留空则使用内置极光(纯 CSS,不需要图片)。
          其他主题是实色背景,设了图也透不出来。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='space-y-4'>
          <div className='space-y-1.5'>
            <Label>背景图片</Label>
            <div className='flex gap-2'>
              <Input
                value={cfg.panel_wallpaper}
                onChange={(e) => set({ panel_wallpaper: e.target.value })}
                placeholder='留空 = 内置极光,或填 https://…'
              />
              <input
                ref={fileRef}
                type='file'
                accept='image/jpeg,image/png,image/webp,image/gif'
                className='hidden'
                onChange={(e) => {
                  const f = e.target.files?.[0]
                  // 清空 value,否则同一个文件选第二次不触发 change
                  e.target.value = ''
                  if (f) uploadMutation.mutate(f)
                }}
              />
              <Button
                type='button'
                variant='outline'
                className='shrink-0'
                disabled={uploadMutation.isPending}
                onClick={() => fileRef.current?.click()}
              >
                {uploadMutation.isPending ? (
                  <Loader2 className='mr-2 size-4 animate-spin' />
                ) : (
                  <Upload className='mr-2 size-4' />
                )}
                上传
              </Button>
            </div>
            <p className='text-muted-foreground text-xs'>
              上传后立即生效,无需再点保存。支持 JPG / PNG / WebP / GIF, 上限
              8MB。也可以直接填外链。
            </p>
          </div>

          <div className='space-y-1.5'>
            <Label htmlFor='pw-dim'>
              遮罩强度
              <span className='text-muted-foreground ml-1 text-xs font-normal'>
                · 让文字压得住花哨的照片
              </span>
            </Label>
            <div className='flex items-center gap-3'>
              <input
                id='pw-dim'
                type='range'
                min={0}
                max={60}
                value={cfg.panel_wallpaper_dim}
                onChange={(e) =>
                  set({ panel_wallpaper_dim: Number(e.target.value) })
                }
                className='accent-primary w-full'
              />
              <span className='w-10 text-right text-sm tabular-nums'>
                {cfg.panel_wallpaper_dim}%
              </span>
            </div>
          </div>

          <div className='space-y-1.5'>
            <Label>内置极光色调</Label>
            <div className='flex flex-wrap gap-2'>
              {GLASS_TONES.map((tone) => (
                <button
                  key={tone}
                  type='button'
                  aria-pressed={cfg.panel_wallpaper_tone === tone}
                  onClick={() => set({ panel_wallpaper_tone: tone })}
                  title={TONE_LABELS[tone]}
                  className={`h-9 w-16 rounded-md border-2 transition-colors ${
                    cfg.panel_wallpaper_tone === tone
                      ? 'border-primary'
                      : 'border-transparent'
                  }`}
                  style={{
                    background:
                      tone === 'custom'
                        ? customPreview(
                            cfg.panel_wallpaper_tone_a,
                            cfg.panel_wallpaper_tone_b
                          )
                        : TONE_PREVIEW[tone],
                  }}
                >
                  <span className='sr-only'>{TONE_LABELS[tone]}</span>
                </button>
              ))}
            </div>
            {cfg.panel_wallpaper_tone === 'custom' && (
              <div className='flex flex-wrap items-center gap-4 pt-1'>
                {(
                  [
                    ['panel_wallpaper_tone_a', '主光斑'],
                    ['panel_wallpaper_tone_b', '副光斑'],
                  ] as const
                ).map(([key, label]) => (
                  <div key={key} className='flex items-center gap-2'>
                    <Label htmlFor={key} className='text-xs font-normal'>
                      {label}
                    </Label>
                    <input
                      id={key}
                      type='color'
                      value={cfg[key]}
                      onChange={(e) => set({ [key]: e.target.value })}
                      className='h-8 w-12 cursor-pointer rounded border bg-transparent p-0.5'
                    />
                    <code className='text-muted-foreground font-mono text-xs'>
                      {cfg[key]}
                    </code>
                  </div>
                ))}
              </div>
            )}
            <p className='text-muted-foreground text-xs'>
              只在没设背景图片时生效。
            </p>
          </div>

          <div className='flex items-center justify-between border-t pt-4'>
            <div className='space-y-0.5'>
              <Label htmlFor='pw-reduce'>降低透明度</Label>
              <p className='text-muted-foreground max-w-prose text-xs'>
                玻璃退化成实色面板。眩光敏感时可开;集显设备上也建议开 ——
                一屏几十张卡各带一层模糊会掉帧。
              </p>
            </div>
            <Switch
              id='pw-reduce'
              checked={cfg.reduce_transparency}
              onCheckedChange={(v) => set({ reduce_transparency: v })}
            />
          </div>

          <Button
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
          >
            {saveMutation.isPending && (
              <Loader2 className='mr-2 size-4 animate-spin' />
            )}
            保存
          </Button>
        </div>
      </CardContent>
    </Card>
  )
}
