import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Activity, Loader2 } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Switch } from '@/components/ui/switch'

/**
 * 节点探测面板。
 *
 * 和「TCPing 测试」不是一回事:TCPing 只拨 server:port,握手失败、证书不对、密码错、
 * 被 QoS 掐了一概看不出来。这里是用 mihomo 走完整协议真连一次,拨的是节点本身能不能用。
 * 代价是每个节点要起一个进程,所以只跑用户勾选的那些。
 *
 * 列出全部节点由用户勾选:本项目的节点都是导入来的,没有「自建服务器」那一类。
 */

type ProbeSample = { at: number; latency_ms: number; ok: boolean }
type ProbeState = {
  node_id: number
  fail_streak: number
  announced: boolean
  samples: ProbeSample[] | null
  source: string
}
type ProbeStatus = {
  enabled: boolean
  tester_id: number
  enabled_count: number
  resync_minutes: number
  interval_sec: number
  // 后端 NodeProbeStore.All() 返回 map[int64]NodeProbeState → JSON 里是**对象**(键为节点 ID),
  // 不是数组。按数组解会直接 "object is not iterable" 把整页打崩。
  states: Record<string, ProbeState> | null
}
type ProbeNode = {
  id: number
  node_name: string
  protocol: string
  enabled: boolean
  probe_enabled: boolean
}

/** 最近 N 次采样的可用率,用于一眼看出「偶发抖动」还是「真的挂了」。 */
function availability(samples: ProbeSample[]): number | null {
  if (samples.length === 0) return null
  const ok = samples.filter((s) => s.ok).length
  return Math.round((ok / samples.length) * 100)
}

function latencyTone(ms: number): string {
  if (ms <= 0) return 'text-muted-foreground'
  if (ms < 200) return 'text-emerald-600 dark:text-emerald-400'
  if (ms < 500) return 'text-amber-600 dark:text-amber-400'
  return 'text-rose-600 dark:text-rose-400'
}

/**
 * 迷你趋势条。刻意不引图表库:每个节点一张图会拖慢长列表,而这里只需要看出
 * 「一直绿 / 偶尔红 / 最近全红」。失败点画成满高红条,不然失败(延迟 0)会是空白,
 * 反而看着像没数据。
 */
function Sparkline({ samples }: { samples: ProbeSample[] }) {
  const recent = samples.slice(-40)
  const max = Math.max(...recent.map((s) => (s.ok ? s.latency_ms : 0)), 1)
  return (
    <div className='flex h-6 items-end gap-px' aria-hidden>
      {recent.map((s, i) => (
        <div
          key={`${s.at}-${i}`}
          className={cn(
            'w-1 rounded-sm',
            s.ok ? 'bg-emerald-500/70' : 'bg-rose-500'
          )}
          style={{
            height: s.ok
              ? `${Math.max(12, (s.latency_ms / max) * 100)}%`
              : '100%',
          }}
        />
      ))}
    </div>
  )
}

/**
 * 面板内容。系统设置(订阅选项卡)直接嵌它,节点页用 NodeProbeDialog 包一层弹窗 ——
 * 两处共用同一个组件、同一组接口,不会出现"两个地方的开关对不上"。
 *
 * active 控制是否发请求:嵌在系统设置里时选项卡没切过来就不该轮询。
 */
export function NodeProbePanel({ active = true }: { active?: boolean }) {
  const queryClient = useQueryClient()

  const { data: status, isLoading: statusLoading } = useQuery({
    queryKey: ['node-probe'],
    queryFn: async () =>
      (await api.get('/api/admin/node-probe')).data as ProbeStatus,
    enabled: active,
    // 探测本身 5 分钟一轮,面板开着时跟着刷新即可
    refetchInterval: active ? 30_000 : false,
  })

  const { data: nodesData } = useQuery({
    queryKey: ['nodes'],
    queryFn: async () =>
      (await api.get('/api/admin/nodes')).data as { nodes: ProbeNode[] },
    enabled: active,
  })

  const { data: testersData } = useQuery({
    queryKey: ['speed-testers'],
    queryFn: async () =>
      (await api.get('/api/admin/speedtest/testers')).data as {
        testers: { id: number; name: string; online: boolean }[]
      },
    enabled: active,
    staleTime: 10_000,
  })
  const testers = testersData?.testers ?? []

  // 全部节点都可探测:本项目没有「自建服务器」概念,节点都是导入来的。
  // 注意**不能**按 original_server 过滤 —— 这一列在本项目里是「手动改地址前的
  // 旧地址备份」,拿来当外部节点判据会把改过地址的节点莫名藏起来。
  const externalNodes = useMemo(() => nodesData?.nodes ?? [], [nodesData])

  const stateByNode = useMemo(() => {
    const m = new Map<number, ProbeState>()
    for (const s of Object.values(status?.states ?? {})) m.set(s.node_id, s)
    return m
  }, [status])

  const settingsMutation = useMutation({
    mutationFn: async (patch: {
      enabled?: boolean
      tester_id?: number
      resync_minutes?: number
    }) => (await api.put('/api/admin/node-probe/settings', patch)).data,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['node-probe'] })
    },
    onError: (e: any) => {
      toast.error(e.response?.data?.error || '保存失败')
    },
  })

  const toggleMutation = useMutation({
    mutationFn: async (v: { node_id: number; enabled: boolean }) =>
      (await api.post('/api/admin/node-probe/toggle', v)).data,
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['node-probe'] })
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
    },
    onError: (e: any) => {
      toast.error(e.response?.data?.error || '操作失败')
    },
  })

  const intervalMin = Math.round((status?.interval_sec ?? 300) / 60)

  // 数字输入用本地草稿 + 失焦提交:直接在 onChange 里提交会把「30」的输入过程
  // 变成 3 → 30 两次请求,中间那次还会真的把阈值改成 3 分钟。
  const [resyncDraft, setResyncDraft] = useState('')
  useEffect(() => {
    if (status) setResyncDraft(String(status.resync_minutes ?? 0))
  }, [status])
  const commitResync = () => {
    const n = Number(resyncDraft)
    const safe =
      Number.isFinite(n) && n >= 0 ? Math.min(Math.floor(n), 1440) : 0
    setResyncDraft(String(safe))
    if (safe !== (status?.resync_minutes ?? 0)) {
      settingsMutation.mutate({ resync_minutes: safe })
    }
  }

  return (
    <div className='space-y-4'>
      <p className='text-muted-foreground text-sm'>
        {`用 mihomo 走完整协议定时真连一次,测出连通性与真实延迟。每 ${intervalMin} 分钟一轮,只跑勾选的节点。`}
      </p>
      <div className='space-y-4'>
        <div className='bg-muted/40 space-y-3 rounded-lg border p-3'>
          <div className='flex items-center justify-between gap-4'>
            <div className='space-y-0.5'>
              <Label htmlFor='probe-enabled'>启用定时探测</Label>
              <p className='text-muted-foreground text-xs'>
                关闭后已勾选的节点会保留,只是不再执行探测
              </p>
            </div>
            <Switch
              id='probe-enabled'
              checked={Boolean(status?.enabled)}
              disabled={statusLoading || settingsMutation.isPending}
              onCheckedChange={(v) => settingsMutation.mutate({ enabled: v })}
            />
          </div>

          <div className='space-y-2'>
            <Label>探测源</Label>
            <div className='flex flex-wrap gap-2'>
              <Button
                size='sm'
                variant={
                  !status?.tester_id || status.tester_id <= 0
                    ? 'default'
                    : 'outline'
                }
                onClick={() => settingsMutation.mutate({ tester_id: 0 })}
              >
                主控本机
              </Button>
              {testers.map((x) => (
                <Button
                  key={x.id}
                  size='sm'
                  variant={status?.tester_id === x.id ? 'default' : 'outline'}
                  className={x.online ? '' : 'opacity-60'}
                  title={
                    x.online ? '' : '该测速端当前离线,探测会自动回退到主控'
                  }
                  onClick={() => settingsMutation.mutate({ tester_id: x.id })}
                >
                  {x.name}
                  {!x.online && '(离线)'}
                </Button>
              ))}
            </div>
            <p className='text-muted-foreground text-xs'>
              选家宽测速端能测出更贴近用户的延迟;测速端不可用时自动回退到主控本机。
            </p>
          </div>

          <div className='space-y-2'>
            <Label htmlFor='probe-resync'>掉线自动重新同步外部订阅</Label>
            <div className='flex items-center gap-2'>
              <Input
                id='probe-resync'
                type='number'
                min={0}
                max={1440}
                className='w-28'
                value={resyncDraft}
                onChange={(e) => setResyncDraft(e.target.value)}
                onBlur={commitResync}
                disabled={settingsMutation.isPending}
              />
              <span className='text-muted-foreground text-sm'>
                分钟(0 = 关闭)
              </span>
            </div>
            <p className='text-muted-foreground text-xs'>
              节点连续掉线满设定分钟数后,自动重新拉取该节点所属用户的外部订阅。机场换服务器时订阅里的地址会变,重新同步一次往往就自愈了。同一用户最短
              15 分钟才会重同步一次,避免频繁请求机场接口。
            </p>
          </div>
        </div>

        <div className='flex items-center justify-between'>
          <p className='text-sm font-medium'>
            可探测的节点
            <span className='text-muted-foreground ml-2 text-xs font-normal'>
              {`已勾选 ${status?.enabled_count ?? 0} / ${externalNodes.length}`}
            </span>
          </p>
        </div>

        <ScrollArea className='h-[45vh] rounded-md border'>
          {externalNodes.length === 0 ? (
            <p className='text-muted-foreground p-6 text-center text-sm'>
              还没有节点。先在节点管理里导入或手动添加,再回来勾选要探测的。
            </p>
          ) : (
            <div className='divide-y'>
              {externalNodes.map((node) => {
                const st = stateByNode.get(node.id)
                const samples = st?.samples ?? []
                const last = samples[samples.length - 1]
                const avail = availability(samples)
                const down = (st?.fail_streak ?? 0) >= 2
                return (
                  <div
                    key={node.id}
                    className='hover:bg-muted/40 flex items-center gap-3 p-3'
                  >
                    <Checkbox
                      checked={node.probe_enabled}
                      disabled={toggleMutation.isPending}
                      onCheckedChange={(v) =>
                        toggleMutation.mutate({
                          node_id: node.id,
                          enabled: v === true,
                        })
                      }
                    />
                    <div className='min-w-0 flex-1'>
                      <div className='flex items-center gap-2'>
                        <span className='truncate text-sm font-medium'>
                          {node.node_name}
                        </span>
                        <Badge variant='outline' className='shrink-0 text-xs'>
                          {node.protocol}
                        </Badge>
                        {!node.enabled && (
                          <Badge variant='secondary' className='shrink-0'>
                            已禁用
                          </Badge>
                        )}
                        {down && (
                          <Badge variant='destructive' className='shrink-0'>
                            不可用
                          </Badge>
                        )}
                      </div>
                      {node.probe_enabled && st?.source && (
                        <p className='text-muted-foreground mt-0.5 text-xs'>
                          {`探测源 ${st.source}`}
                          {avail !== null && ` · ${`可用率 ${avail}%`}`}
                        </p>
                      )}
                    </div>

                    {node.probe_enabled ? (
                      samples.length === 0 ? (
                        <span className='text-muted-foreground flex items-center gap-1 text-xs'>
                          <Loader2 className='h-3 w-3 animate-spin' />
                          等待首次探测
                        </span>
                      ) : (
                        <div className='flex items-center gap-3'>
                          <Sparkline samples={samples} />
                          <span
                            className={cn(
                              'w-16 text-right text-sm font-semibold tabular-nums',
                              last?.ok
                                ? latencyTone(last.latency_ms)
                                : 'text-rose-600 dark:text-rose-400'
                            )}
                          >
                            {last?.ok ? `${last.latency_ms} ms` : '超时'}
                          </span>
                        </div>
                      )
                    ) : (
                      <span className='text-muted-foreground text-xs'>
                        未探测
                      </span>
                    )}
                  </div>
                )
              })}
            </div>
          )}
        </ScrollArea>
      </div>
    </div>
  )
}

/** 节点页用的弹窗外壳:标题 + 面板内容。设置项本身在系统设置的订阅选项卡里也有一份。 */
export function NodeProbeDialog({
  open,
  onOpenChange,
}: {
  open: boolean
  onOpenChange: (v: boolean) => void
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-h-[85vh] max-w-3xl overflow-y-auto'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <Activity className='h-5 w-5' />
            节点探测
          </DialogTitle>
        </DialogHeader>
        <NodeProbePanel active={open} />
      </DialogContent>
    </Dialog>
  )
}
