// 自定义 CSS:管理员往每个页面注入一段样式。
//
// 可以存多套、随时切换生效的那一套;每套可填一个来源链接(GitHub 等),一键拉最新内容。
// 从妙妙屋X 迁移而来,去掉了 license 门控、按主题绑定与社区主题。
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { AlertTriangle, Download, Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'

interface CSSProfile {
  id: string
  name: string
  css: string
  source_url?: string
}

interface CustomCSSResponse {
  success: boolean
  custom_css: string
  profiles?: CSSProfile[]
  active?: string
  disabled_by_env: boolean
  max_bytes: number
}

// 注入点由服务端写进 index.html 的 <body> 开头。保存后直接改它的内容,
// 管理员当场就能看到效果,不用刷新页面。
const INJECT_POINT_ID = 'mmw-custom-css'

function applyLive(css: string) {
  const el = document.getElementById(INJECT_POINT_ID)
  if (el) el.textContent = css
}

function byteLength(s: string) {
  return new TextEncoder().encode(s).length
}

function newProfileID() {
  return `p${Date.now().toString(36)}${Math.random().toString(36).slice(2, 6)}`
}

export function CustomCSSCard() {
  const queryClient = useQueryClient()
  const [profiles, setProfiles] = useState<CSSProfile[]>([])
  const [active, setActive] = useState('')
  const [editing, setEditing] = useState('')
  const [dirty, setDirty] = useState(false)

  const { data } = useQuery({
    queryKey: ['custom-css-settings'],
    queryFn: async () =>
      (await api.get('/api/admin/system-settings/custom-css'))
        .data as CustomCSSResponse,
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    // 只在没改过的时候回填,免得后台刷新把管理员正在编辑的内容冲掉。
    if (!data || dirty) return
    const list = data.profiles?.length
      ? data.profiles
      : [{ id: 'default', name: '默认', css: data.custom_css || '' }]
    setProfiles(list)
    setActive(data.active || list[0].id)
    setEditing((cur) =>
      list.some((p) => p.id === cur) ? cur : data.active || list[0].id
    )
  }, [data, dirty])

  const current = profiles.find((p) => p.id === editing)
  const patchCurrent = (patch: Partial<CSSProfile>) => {
    setProfiles((list) =>
      list.map((p) => (p.id === editing ? { ...p, ...patch } : p))
    )
    setDirty(true)
  }

  // nextActive:「设为生效」直接带上要生效的那套一起保存 —— 只改本地状态的话
  // 还得再点下方「保存」才写入,看起来像没反应。
  const saveMutation = useMutation({
    mutationFn: async (nextActive?: string) => {
      const act = nextActive ?? active
      const res = await api.put('/api/admin/system-settings/custom-css', {
        profiles,
        active: act,
      })
      const css: string =
        res.data?.custom_css ?? profiles.find((p) => p.id === act)?.css ?? ''
      return { act, css }
    },
    onSuccess: ({ act, css }, nextActive) => {
      setActive(act)
      applyLive(css)
      // 先把缓存换成刚保存的内容再清 dirty:否则 dirty 一清,上面的回填会先用
      // 保存前的旧数据把列表盖掉,新拉取的配置看起来像被清空了。
      queryClient.setQueryData<CustomCSSResponse>(
        ['custom-css-settings'],
        (old) =>
          old ? { ...old, profiles, active: act, custom_css: css } : old
      )
      setDirty(false)
      queryClient.invalidateQueries({ queryKey: ['custom-css-settings'] })
      toast.success(nextActive ? '已设为生效' : '自定义 CSS 已保存')
    },
    onError: (e: any) =>
      toast.error(e?.response?.data?.error || '自定义 CSS 保存失败'),
  })

  const fetchMutation = useMutation({
    mutationFn: async (url: string) =>
      (
        await api.post('/api/admin/system-settings/custom-css/fetch', {
          url,
        })
      ).data as { css: string; url: string },
    onSuccess: (res) => {
      patchCurrent({ css: res.css })
      toast.success('已从链接拉取最新内容,确认无误后点「保存」生效')
    },
    onError: (e: any) =>
      toast.error(e?.response?.data?.error || '从链接拉取失败'),
  })

  const maxBytes = data?.max_bytes ?? 64 * 1024
  const used = byteLength(current?.css ?? '')
  const tooLong = profiles.some((p) => byteLength(p.css) > maxBytes)

  return (
    <Card>
      <CardHeader className='pb-4'>
        <CardTitle>自定义 CSS</CardTitle>
        <CardDescription>
          往每个页面注入一段样式,用来改配色、换字体、藏掉不需要的元素。样式会随页面
          一起下发(登录页也算),保存后立即生效。可以存多套随时切换,也可以填
          GitHub 等链接一键更新。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='space-y-3'>
          {data?.disabled_by_env && (
            <div className='flex items-start gap-2 border border-amber-500/40 bg-amber-500/10 p-3 text-sm'>
              <AlertTriangle className='mt-0.5 size-4 shrink-0 text-amber-600' />
              <div>
                当前设置了环境变量{' '}
                <code className='font-mono'>MMW_DISABLE_CUSTOM_CSS</code>
                ,自定义 CSS <strong>暂不生效</strong>
                。内容仍会正常保存,去掉该环境变量并重启即可恢复。
              </div>
            </div>
          )}

          <div className='flex flex-wrap items-center gap-2'>
            <Select value={editing} onValueChange={setEditing}>
              <SelectTrigger className='w-56'>
                <SelectValue placeholder='选择配置' />
              </SelectTrigger>
              <SelectContent>
                {profiles.map((p) => (
                  <SelectItem key={p.id} value={p.id}>
                    {p.name}
                    {p.id === active ? '(生效中)' : ''}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            {editing === active ? (
              <Badge>生效中</Badge>
            ) : (
              <Button
                type='button'
                variant='outline'
                size='sm'
                disabled={saveMutation.isPending || tooLong}
                title={dirty ? '会连同其它未保存的修改一起保存' : undefined}
                onClick={() => saveMutation.mutate(editing)}
              >
                设为生效
              </Button>
            )}
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={profiles.length >= 20}
              onClick={() => {
                const id = newProfileID()
                setProfiles((list) => [
                  ...list,
                  { id, name: `配置 ${list.length + 1}`, css: '' },
                ])
                setEditing(id)
                setDirty(true)
              }}
            >
              <Plus className='mr-1 size-4' />
              新建
            </Button>
            <Button
              type='button'
              variant='outline'
              size='sm'
              disabled={profiles.length <= 1 || editing === active}
              title={
                editing === active
                  ? '生效中的配置不能删除,先把别的配置设为生效'
                  : undefined
              }
              onClick={() => {
                setProfiles((list) => list.filter((p) => p.id !== editing))
                setEditing(active)
                setDirty(true)
              }}
            >
              <Trash2 className='mr-1 size-4' />
              删除
            </Button>
          </div>
          {/* 上面按钮的说明桌面端在 title 里;触屏没有悬停,直接写出来 */}
          {editing === active && profiles.length > 1 ? (
            <p className='text-muted-foreground -mt-2 text-xs md:hidden'>
              生效中的配置不能删除,先把别的配置设为生效
            </p>
          ) : editing !== active && dirty ? (
            <p className='text-muted-foreground -mt-2 text-xs md:hidden'>
              「设为生效」会连同其它未保存的修改一起保存
            </p>
          ) : null}

          {current && (
            <>
              <div className='grid gap-2 sm:grid-cols-[12rem_1fr_auto]'>
                <Input
                  value={current.name}
                  onChange={(e) => patchCurrent({ name: e.target.value })}
                  placeholder='配置名'
                  maxLength={40}
                />
                <Input
                  value={current.source_url ?? ''}
                  onChange={(e) => patchCurrent({ source_url: e.target.value })}
                  placeholder='来源链接(可选),如 https://github.com/用户/仓库/blob/main/theme.css'
                />
                <Button
                  type='button'
                  variant='outline'
                  disabled={
                    !current.source_url?.trim() || fetchMutation.isPending
                  }
                  onClick={() =>
                    fetchMutation.mutate(current.source_url!.trim())
                  }
                >
                  <Download className='mr-1 size-4' />
                  {fetchMutation.isPending ? '拉取中…' : '从链接更新'}
                </Button>
              </div>
              <Textarea
                value={current.css}
                onChange={(e) => patchCurrent({ css: e.target.value })}
                spellCheck={false}
                rows={14}
                className='font-mono text-xs'
                placeholder={`/* 例:把顶栏调矮一点 */\nheader { height: 3rem; }`}
              />
            </>
          )}

          <div className='flex flex-wrap items-center justify-between gap-2'>
            <span
              className={`text-xs tabular-nums ${used > maxBytes ? 'text-destructive' : 'text-muted-foreground'}`}
            >
              {used.toLocaleString()} / {maxBytes.toLocaleString()} 字节
              {dirty && ' · 有未保存的修改'}
            </span>
            <div className='flex gap-2'>
              <Button
                type='button'
                variant='outline'
                disabled={saveMutation.isPending || !current?.css}
                onClick={() => patchCurrent({ css: '' })}
              >
                清空当前配置
              </Button>
              <Button
                type='button'
                disabled={saveMutation.isPending || tooLong || !dirty}
                onClick={() => saveMutation.mutate(undefined)}
              >
                {saveMutation.isPending ? '保存中…' : '保存'}
              </Button>
            </div>
          </div>
          <p className='text-muted-foreground text-xs'>
            「从链接更新」只把内容拉回编辑框,点「保存」或「设为生效」才会写入。写坏了打不开面板?设环境变量{' '}
            <code className='font-mono'>MMW_DISABLE_CUSTOM_CSS=1</code>{' '}
            重启即可跳过注入,内容不会丢。
          </p>
        </div>
      </CardContent>
    </Card>
  )
}
