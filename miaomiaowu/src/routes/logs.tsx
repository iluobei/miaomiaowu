// @ts-nocheck
import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { RefreshCw, ShieldBan, ShieldCheck } from 'lucide-react'
import { toast } from 'sonner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { api } from '@/lib/api'
import { profileQueryFn } from '@/lib/profile'

export const Route = createFileRoute('/logs')({
  beforeLoad: async ({ context }) => {
    let profile
    try {
      profile = await context.queryClient.fetchQuery({ queryKey: ['profile'], queryFn: profileQueryFn })
    } catch {
      throw redirect({ to: '/login' })
    }
    if (!profile?.is_admin) throw redirect({ to: '/' })
  },
  component: LogsPage,
})

function LogsPage() {
  return (
    <div className='min-h-svh bg-background'>
      <main className='mx-auto max-w-6xl px-4 pb-10 pt-24'>
        <h1 className='text-3xl font-semibold'>日志管理</h1>
        <p className='mt-2 text-muted-foreground'>查看安全事件、当前封禁和后台任务执行结果。</p>
        <Tabs defaultValue='security' className='mt-6'>
          <TabsList><TabsTrigger value='security'>安全日志</TabsTrigger><TabsTrigger value='operations'>操作日志</TabsTrigger><TabsTrigger value='tasks'>任务日志</TabsTrigger></TabsList>
          <TabsContent value='security'><SecurityPanel /></TabsContent>
          <TabsContent value='operations'><OperationPanel /></TabsContent>
          <TabsContent value='tasks'><TaskPanel /></TabsContent>
        </Tabs>
      </main>
    </div>
  )
}

function SecurityPanel() {
  const client = useQueryClient()
  const [ip, setIP] = useState('')
  const bans = useQuery({ queryKey: ['security-bans'], queryFn: async () => (await api.get('/api/admin/security/bans')).data.bans ?? [], refetchInterval: 15000 })
  const events = useQuery({ queryKey: ['security-events'], queryFn: async () => (await api.get('/api/admin/security/events?limit=200')).data.events ?? [], refetchInterval: 15000 })
  const refresh = () => client.invalidateQueries({ queryKey: ['security-bans'] }).then(() => client.invalidateQueries({ queryKey: ['security-events'] }))
  const ban = useMutation({ mutationFn: async (permanent: boolean) => api.post('/api/admin/security/bans', { ip, permanent }), onSuccess: () => { setIP(''); toast.success('IP 已封禁'); refresh() } })
  const unban = useMutation({ mutationFn: async (value: string) => api.delete(`/api/admin/security/bans/${encodeURIComponent(value)}`), onSuccess: () => { toast.success('IP 已解封'); refresh() } })
  return <div className='mt-4 space-y-4'>
    <Card><CardHeader><CardTitle className='text-base'>手动封禁</CardTitle></CardHeader><CardContent className='flex flex-wrap gap-2'><Input className='max-w-xs' value={ip} onChange={(e) => setIP(e.target.value)} placeholder='IPv4 或 IPv6' /><Button disabled={!ip || ban.isPending} onClick={() => ban.mutate(false)}><ShieldBan className='mr-2 h-4 w-4' />临时封禁</Button><Button variant='destructive' disabled={!ip || ban.isPending} onClick={() => ban.mutate(true)}>永久封禁</Button></CardContent></Card>
    <Card><CardHeader className='flex-row items-center justify-between'><CardTitle className='text-base'>当前封禁（{bans.data?.length ?? 0}）</CardTitle><Button size='icon' variant='ghost' onClick={refresh}><RefreshCw className='h-4 w-4' /></Button></CardHeader><CardContent><div className='overflow-x-auto'><table className='w-full text-sm'><thead><tr className='border-b text-left'><th className='py-2'>IP</th><th>原因</th><th>到期时间</th><th>操作者</th><th /></tr></thead><tbody>{bans.data?.map((b) => <tr key={b.ip} className='border-b'><td className='py-2 font-mono'>{b.ip}</td><td>{b.reason}</td><td>{b.permanent ? <Badge>永久</Badge> : formatTime(b.expires_at)}</td><td>{b.actor || '-'}</td><td className='text-right'><Button size='sm' variant='outline' onClick={() => unban.mutate(b.ip)}><ShieldCheck className='mr-1 h-4 w-4' />解封</Button></td></tr>)}</tbody></table>{!bans.data?.length && <p className='py-6 text-center text-muted-foreground'>暂无活动封禁</p>}</div></CardContent></Card>
    <Card><CardHeader><CardTitle className='text-base'>安全事件</CardTitle></CardHeader><CardContent><div className='max-h-[420px] overflow-auto'><table className='w-full text-sm'><thead><tr className='border-b text-left'><th className='py-2'>时间</th><th>类型</th><th>IP</th><th>路径/详情</th></tr></thead><tbody>{events.data?.map((e) => <tr key={e.id} className='border-b'><td className='whitespace-nowrap py-2'>{formatTime(e.at)}</td><td><Badge variant='outline'>{e.kind}</Badge></td><td className='font-mono'>{e.ip}</td><td>{e.path || e.detail || '-'}</td></tr>)}</tbody></table></div></CardContent></Card>
  </div>
}

function TaskPanel() {
  const runs = useQuery({ queryKey: ['task-runs'], queryFn: async () => (await api.get('/api/admin/tasks/runs?limit=200')).data.runs ?? [], refetchInterval: 15000 })
  return <Card className='mt-4'><CardHeader className='flex-row items-center justify-between'><CardTitle className='text-base'>任务执行记录</CardTitle><Button size='icon' variant='ghost' onClick={() => runs.refetch()}><RefreshCw className={`h-4 w-4 ${runs.isFetching ? 'animate-spin' : ''}`} /></Button></CardHeader><CardContent><div className='overflow-x-auto'><table className='w-full text-sm'><thead><tr className='border-b text-left'><th className='py-2'>开始时间</th><th>任务</th><th>状态</th><th>耗时</th><th>详情</th></tr></thead><tbody>{runs.data?.map((run) => <tr key={run.id} className='border-b'><td className='whitespace-nowrap py-2'>{formatTime(run.started_at)}</td><td>{run.task_name}</td><td><Badge variant={run.status === 'error' ? 'destructive' : 'outline'}>{run.status}</Badge></td><td>{run.duration_ms} ms</td><td>{run.detail || '-'}</td></tr>)}</tbody></table>{!runs.data?.length && <p className='py-6 text-center text-muted-foreground'>暂无任务记录</p>}</div></CardContent></Card>
}

function OperationPanel() {
  const logs = useQuery({ queryKey: ['operation-logs'], queryFn: async () => (await api.get('/api/admin/operations?limit=200')).data.logs ?? [], refetchInterval: 15000 })
  return <Card className='mt-4'><CardHeader className='flex-row items-center justify-between'><CardTitle className='text-base'>管理员操作记录</CardTitle><Button size='icon' variant='ghost' onClick={() => logs.refetch()}><RefreshCw className={`h-4 w-4 ${logs.isFetching ? 'animate-spin' : ''}`} /></Button></CardHeader><CardContent><div className='overflow-x-auto'><table className='w-full text-sm'><thead><tr className='border-b text-left'><th className='py-2'>时间</th><th>操作者</th><th>操作</th><th>路径</th><th>状态</th><th>来源 IP</th></tr></thead><tbody>{logs.data?.map((log) => <tr key={log.id} className='border-b'><td className='whitespace-nowrap py-2'>{formatTime(log.at)}</td><td>{log.actor || '-'}</td><td><Badge variant='outline'>{log.method}</Badge></td><td className='font-mono'>{log.path}</td><td>{log.status}</td><td className='font-mono'>{log.ip}</td></tr>)}</tbody></table>{!logs.data?.length && <p className='py-6 text-center text-muted-foreground'>暂无操作记录</p>}</div></CardContent></Card>
}

function formatTime(value?: string) { return value ? new Date(value).toLocaleString() : '-' }
