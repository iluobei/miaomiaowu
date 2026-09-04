// 规则集托管 —— 把 clash rule-providers 的规则文件放在主控上,由订阅直接引用。
//
// 以前订阅里的 rule-providers 只能指向第三方仓库:更新不可控,墙内还常常拉不动。
// 放到自己这台机器上之后,订阅引用的就是本机地址。
//
// 三种来源合成两种存法:
//   手动 —— 面板里直接编辑,或从本地文件读进编辑器(文件在浏览器端读成文本,
//           不走上传接口:规则集本来就是文本,省掉一整条 multipart 链路和它
//           要登记的几份清单)。
//   远程 —— 填地址,由主控定时抓取并缓存,客户端拉的始终是本机。
import { useRef, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import {
  Copy,
  FileUp,
  Loader2,
  Pencil,
  Plus,
  RefreshCw,
  Trash2,
} from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { profileQueryFn } from '@/lib/profile'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent } from '@/components/ui/card'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Textarea } from '@/components/ui/textarea'
import { ConfirmDialog } from '@/components/confirm-dialog'

export const Route = createFileRoute('/rule-providers')({
  beforeLoad: async ({ context }) => {
    let profile: { is_admin?: boolean } | undefined
    try {
      profile = await (
        context as {
          queryClient: {
            fetchQuery: (o: unknown) => Promise<{ is_admin?: boolean }>
          }
        }
      ).queryClient.fetchQuery({
        queryKey: ['profile'],
        queryFn: profileQueryFn,
        staleTime: 5 * 60 * 1000,
      })
    } catch {
      throw redirect({ to: '/login' })
    }
    if (!profile?.is_admin) {
      throw redirect({ to: '/' })
    }
  },
  component: RuleProvidersPage,
})

interface RuleProvider {
  id: number
  name: string
  display_name: string
  source: 'manual' | 'remote'
  remote_url: string
  refresh_minutes: number
  content?: string
  size: number
  last_fetch_at?: string
  last_fetch_error: string
  updated_at: string
}

const EMPTY_DRAFT = {
  id: 0,
  name: '',
  display_name: '',
  source: 'manual' as 'manual' | 'remote',
  remote_url: '',
  refresh_minutes: 1440,
  content: '',
}
type Draft = typeof EMPTY_DRAFT

function formatSize(n: number) {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / 1024 / 1024).toFixed(2)} MB`
}

function RuleProvidersPage() {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [removing, setRemoving] = useState<RuleProvider | null>(null)
  const fileRef = useRef<HTMLInputElement>(null)

  const { data, isLoading } = useQuery({
    queryKey: ['rule-providers'],
    queryFn: async () =>
      (await api.get('/api/admin/rule-providers')).data as {
        providers: RuleProvider[]
        url_prefix: string
      },
  })
  const providers = data?.providers ?? []
  const prefix = data?.url_prefix ?? '/ruleset/'

  const invalidate = () =>
    queryClient.invalidateQueries({ queryKey: ['rule-providers'] })

  const saveMutation = useMutation({
    mutationFn: async (d: Draft) =>
      d.id
        ? (await api.put(`/api/admin/rule-providers/${d.id}`, d)).data
        : (await api.post('/api/admin/rule-providers', d)).data,
    onSuccess: () => {
      setDraft(null)
      invalidate()
      toast.success('已保存')
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '保存失败'),
  })

  const refreshMutation = useMutation({
    mutationFn: async (id: number) =>
      (await api.post(`/api/admin/rule-providers/${id}/refresh`)).data,
    onSuccess: () => {
      invalidate()
      toast.success('已抓取')
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '抓取失败'),
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) =>
      (await api.delete(`/api/admin/rule-providers/${id}`)).data,
    onSuccess: () => {
      setRemoving(null)
      invalidate()
      toast.success('已删除')
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '删除失败'),
  })

  // 编辑时才去取内容:列表接口刻意不带 content,十几个规则集各几百 KB
  // 一起塞进列表响应没有意义。
  const openEdit = async (p: RuleProvider) => {
    try {
      const full = (await api.get(`/api/admin/rule-providers/${p.id}`)).data
        .provider as RuleProvider
      setDraft({
        id: full.id,
        name: full.name,
        display_name: full.display_name,
        source: full.source,
        remote_url: full.remote_url,
        refresh_minutes: full.refresh_minutes || 1440,
        content: full.content ?? '',
      })
    } catch {
      toast.error('读取失败')
    }
  }

  const publicURL = (name: string) =>
    `${window.location.origin}${prefix}${name}`

  return (
    <div className='bg-background min-h-svh'>
      <main className='mx-auto w-full max-w-6xl px-4 py-8 pt-24 sm:px-6'>
        <section className='flex flex-wrap items-start justify-between gap-3'>
          <div className='space-y-2'>
            <h1 className='text-3xl font-semibold tracking-tight'>规则集</h1>
            <p className='text-muted-foreground max-w-prose'>
              把 rule-providers 的规则文件放在主控上,订阅直接引用本机地址 ——
              不再依赖第三方仓库的可用性。下载地址是公开的(客户端拉取时没有登录态),
              所以不想被人猜到就给文件名加个随机后缀。
            </p>
          </div>
          <Button onClick={() => setDraft({ ...EMPTY_DRAFT })}>
            <Plus className='mr-2 size-4' /> 新建规则集
          </Button>
        </section>

        <div className='mt-6 space-y-3'>
          {isLoading && (
            <p className='text-muted-foreground text-sm'>加载中…</p>
          )}
          {!isLoading && providers.length === 0 && (
            <Card>
              <CardContent className='text-muted-foreground py-10 text-center text-sm'>
                还没有规则集。新建一个,然后在订阅模板的 rule-providers
                里引用它的地址。
              </CardContent>
            </Card>
          )}
          {providers.map((p) => (
            <Card key={p.id}>
              <CardContent className='flex flex-wrap items-center gap-3 py-4'>
                <div className='min-w-0 flex-1 space-y-1'>
                  <div className='flex flex-wrap items-center gap-2'>
                    <span className='font-medium'>
                      {p.display_name || p.name}
                    </span>
                    <Badge variant='outline'>
                      {p.source === 'remote' ? '远程抓取' : '手动维护'}
                    </Badge>
                    <span className='text-muted-foreground text-xs tabular-nums'>
                      {formatSize(p.size)}
                    </span>
                  </div>
                  <code className='text-muted-foreground block truncate font-mono text-xs'>
                    {publicURL(p.name)}
                  </code>
                  {p.last_fetch_error && (
                    <p className='text-destructive text-xs'>
                      上次抓取失败:{p.last_fetch_error}
                    </p>
                  )}
                </div>
                <div className='flex shrink-0 items-center gap-1'>
                  <Button
                    variant='ghost'
                    size='icon'
                    title='复制订阅里要填的地址'
                    onClick={() => {
                      navigator.clipboard
                        .writeText(publicURL(p.name))
                        .then(() => toast.success('地址已复制'))
                        .catch(() => toast.error('复制失败'))
                    }}
                  >
                    <Copy className='size-4' />
                  </Button>
                  {p.source === 'remote' && (
                    <Button
                      variant='ghost'
                      size='icon'
                      title='立刻抓取一次'
                      disabled={refreshMutation.isPending}
                      onClick={() => refreshMutation.mutate(p.id)}
                    >
                      <RefreshCw className='size-4' />
                    </Button>
                  )}
                  <Button
                    variant='ghost'
                    size='icon'
                    title='编辑'
                    onClick={() => openEdit(p)}
                  >
                    <Pencil className='size-4' />
                  </Button>
                  <Button
                    variant='ghost'
                    size='icon'
                    title='删除'
                    onClick={() => setRemoving(p)}
                  >
                    <Trash2 className='text-destructive size-4' />
                  </Button>
                </div>
              </CardContent>
            </Card>
          ))}
        </div>
      </main>

      <Dialog open={!!draft} onOpenChange={(o) => !o && setDraft(null)}>
        <DialogContent className='max-w-2xl'>
          <DialogHeader>
            <DialogTitle>{draft?.id ? '编辑规则集' : '新建规则集'}</DialogTitle>
            <DialogDescription>
              文件名就是公开地址的最后一段,建议带上扩展名(如{' '}
              <code className='font-mono'>ads.yaml</code>)。
            </DialogDescription>
          </DialogHeader>
          {draft && (
            <div className='space-y-4'>
              <div className='grid gap-3 sm:grid-cols-2'>
                <div className='space-y-1.5'>
                  <Label htmlFor='rp-name'>文件名</Label>
                  <Input
                    id='rp-name'
                    value={draft.name}
                    placeholder='ads.yaml'
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                </div>
                <div className='space-y-1.5'>
                  <Label htmlFor='rp-display'>备注名(可选)</Label>
                  <Input
                    id='rp-display'
                    value={draft.display_name}
                    placeholder='广告拦截'
                    onChange={(e) =>
                      setDraft({ ...draft, display_name: e.target.value })
                    }
                  />
                </div>
              </div>

              <div className='space-y-1.5'>
                <Label>来源</Label>
                <Select
                  value={draft.source}
                  onValueChange={(v) =>
                    setDraft({ ...draft, source: v as Draft['source'] })
                  }
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='manual'>手动维护</SelectItem>
                    <SelectItem value='remote'>远程抓取并缓存</SelectItem>
                  </SelectContent>
                </Select>
              </div>

              {draft.source === 'remote' ? (
                <div className='grid gap-3 sm:grid-cols-[1fr_auto]'>
                  <div className='space-y-1.5'>
                    <Label htmlFor='rp-url'>远程地址</Label>
                    <Input
                      id='rp-url'
                      value={draft.remote_url}
                      placeholder='https://…/ads.yaml'
                      onChange={(e) =>
                        setDraft({ ...draft, remote_url: e.target.value })
                      }
                    />
                  </div>
                  <div className='space-y-1.5'>
                    <Label htmlFor='rp-interval'>间隔(分钟)</Label>
                    <Input
                      id='rp-interval'
                      type='number'
                      min={5}
                      className='w-32'
                      value={draft.refresh_minutes}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          refresh_minutes: Number(e.target.value),
                        })
                      }
                    />
                  </div>
                </div>
              ) : (
                <div className='space-y-1.5'>
                  <div className='flex items-center justify-between'>
                    <Label htmlFor='rp-content'>内容</Label>
                    <input
                      ref={fileRef}
                      type='file'
                      accept='.yaml,.yml,.txt,.list,text/plain'
                      className='hidden'
                      onChange={(e) => {
                        const f = e.target.files?.[0]
                        e.target.value = ''
                        if (!f) return
                        // 文件在浏览器端读成文本就落进编辑器 —— 不走上传接口。
                        f.text()
                          .then((t) =>
                            setDraft((d) => d && { ...d, content: t })
                          )
                          .catch(() => toast.error('读取文件失败'))
                      }}
                    />
                    <Button
                      type='button'
                      variant='outline'
                      size='sm'
                      onClick={() => fileRef.current?.click()}
                    >
                      <FileUp className='mr-2 size-4' /> 从文件导入
                    </Button>
                  </div>
                  <Textarea
                    id='rp-content'
                    rows={14}
                    className='font-mono text-xs'
                    value={draft.content}
                    placeholder={'payload:\n  - DOMAIN-SUFFIX,example.com'}
                    onChange={(e) =>
                      setDraft({ ...draft, content: e.target.value })
                    }
                  />
                </div>
              )}
            </div>
          )}
          <DialogFooter>
            <Button variant='outline' onClick={() => setDraft(null)}>
              取消
            </Button>
            <Button
              disabled={saveMutation.isPending}
              onClick={() => draft && saveMutation.mutate(draft)}
            >
              {saveMutation.isPending && (
                <Loader2 className='mr-2 size-4 animate-spin' />
              )}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title='删除规则集'
        desc={
          removing
            ? `删除「${removing.display_name || removing.name}」后,引用它的订阅会拉不到规则文件。`
            : ''
        }
        destructive
        handleConfirm={() => removing && deleteMutation.mutate(removing.id)}
      />
    </div>
  )
}
