// @ts-nocheck
import { useEffect, useMemo, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { Plus, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { Topbar } from '@/components/layout/topbar'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { profileQueryFn } from '@/lib/profile'
import { useAuthStore } from '@/stores/auth-store'
import { DataTable } from '@/components/data-table'
import type { DataTableColumn } from '@/components/data-table'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'

type ServerForm = {
  key: string
  id?: number
  server_id: string
  name: string
  traffic_method: string
  monthly_traffic_gb: number | string | null
}

type ProbeConfigResponse = {
  probe_type: string
  address: string
  servers: Array<{
    id: number
    server_id: string
    name: string
    traffic_method: string
    monthly_traffic_gb: number
    position: number
    monthly_traffic_bytes: number
  }>
  created_at?: string
  updated_at?: string
}

const PROBE_TYPES = [
  { value: 'nezha', label: '哪吒面板' },
  { value: 'nezhav0', label: '哪吒 V0' },
  { value: 'dstatus', label: 'DStatus' },
  { value: 'komari', label: 'Komari' },
]

const TRAFFIC_METHODS = [
  { value: 'both', label: '上下行总和' },
  { value: 'up', label: '仅上行' },
  { value: 'down', label: '仅下行' },
]

// @ts-ignore - simple route definition retained
export const Route = createFileRoute('/probe')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: ProbeManagePage,
})

function ProbeManagePage() {
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()

  const [formState, setFormState] = useState({
    probeType: '',
    address: '',
    servers: [] as ServerForm[],
  })
  const [syncLoading, setSyncLoading] = useState(false)

  const generateKey = () =>
    typeof crypto !== 'undefined' && crypto?.randomUUID
      ? crypto.randomUUID()
      : Math.random().toString(36).slice(2, 10)

  const { data: profile, isLoading: profileLoading } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const isAdmin = Boolean(profile?.is_admin)

  const { data: configData, isLoading: configLoading } = useQuery({
    queryKey: ['probe-config'],
    queryFn: async () => {
      const response = await api.get('/api/admin/probe-config')
      return response.data as { config: ProbeConfigResponse }
    },
    enabled: Boolean(auth.accessToken && isAdmin),
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    const config = configData?.config
    if (!config) {
      return
    }

    const normalizedType = (config.probe_type ?? '').toLowerCase().trim()
    const fallbackType = PROBE_TYPES[0]?.value ?? 'nezha'
    const matchedType = PROBE_TYPES.some((item) => item.value === normalizedType)
      ? normalizedType
      : fallbackType

    setFormState({
      probeType: matchedType,
      address: config.address?.trim() ?? '',
      servers: (config.servers ?? []).map((server) => ({
        key: `${server.id ?? server.server_id}-${server.position}`,
        id: server.id,
        server_id: server.server_id,
        name: server.name,
        traffic_method: server.traffic_method,
        monthly_traffic_gb: Number.isFinite(server.monthly_traffic_gb)
          ? Number(server.monthly_traffic_gb)
          : 0,
      })),
    })
  }, [configData])

  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat('zh-CN', {
        dateStyle: 'medium',
        timeStyle: 'short',
        hour12: false,
      }),
    []
  )

  const lastUpdated = useMemo(() => {
    const updatedAt = configData?.config?.updated_at
    if (!updatedAt || updatedAt === '' || updatedAt.startsWith('0001-01-01')) {
      return null
    }
    const date = new Date(updatedAt)
    if (isNaN(date.getTime())) {
      return null
    }
    return dateFormatter.format(date)
  }, [configData, dateFormatter])

  const mutation = useMutation({
    mutationFn: async () => {
      const payload = {
        probe_type: formState.probeType,
        address: formState.address.trim(),
        servers: formState.servers.map((server) => ({
          server_id: server.server_id.trim(),
          name: server.name.trim(),
          traffic_method: server.traffic_method,
          monthly_traffic_gb: (() => {
            if (typeof server.monthly_traffic_gb === 'number') {
              return server.monthly_traffic_gb
            }
            if (typeof server.monthly_traffic_gb === 'string') {
              const trimmed = server.monthly_traffic_gb.trim()
              return trimmed ? Number(trimmed) : 0
            }
            return 0
          })(),
        })),
      }

      const response = await api.put('/api/admin/probe-config', payload)
      return response.data as { config: ProbeConfigResponse }
    },
    onSuccess: (data) => {
      toast.success('探针配置已保存')
      queryClient.setQueryData(['probe-config'], { config: data.config })
      // 使流量汇总数据缓存失效，以便重新获取最新数据
      queryClient.invalidateQueries({ queryKey: ['traffic-summary'] })
    },
    onError: handleServerError,
  })

  const handleServerChange = (
    index: number,
    field: keyof ServerForm,
    value: string | number | null
  ) => {
    setFormState((prev) => {
      const servers = [...prev.servers]
      const raw = value
      let nextValue: string | number | null = raw

      if (field === 'monthly_traffic_gb') {
        if (raw === null) {
          nextValue = null
        } else if (typeof raw === 'string') {
          const trimmed = raw.trim()
          nextValue = trimmed === '' ? null : trimmed
        } else {
          nextValue = raw
        }
      }

      const target = { ...servers[index], [field]: nextValue as any }
      servers.splice(index, 1, target)
      return { ...prev, servers }
    })
  }

  const handleAddServer = () => {
    setFormState((prev) => ({
      ...prev,
      servers: [
        ...prev.servers,
        {
          key: generateKey(),
          server_id: '',
          name: '',
          traffic_method: 'both',
          monthly_traffic_gb: null,
        },
      ],
    }))
  }

  const handleRemoveServer = (index: number) => {
    setFormState((prev) => {
      const servers = [...prev.servers]
      servers.splice(index, 1)
      return { ...prev, servers }
    })
  }

  const trimAddress = () => formState.address.trim().replace(/\/$/, '')

  const fetchDstatusServers = async (baseURL: string): Promise<ServerForm[]> => {
    const response = await api.post('/api/admin/probe-sync', {
      probe_type: 'dstatus',
      address: baseURL,
    })

    const servers: Array<any> = response.data?.servers ?? []
    return servers.map((server, index) => ({
      key: `${server.server_id || 'server'}-${index}-${generateKey()}`,
      server_id: server.server_id ?? '',
      name: server.name ?? `服务器 ${index + 1}`,
      traffic_method: server.traffic_method ?? 'both',
      monthly_traffic_gb: server.monthly_traffic_gb ?? 0,
    }))
  }

  const fetchNezhaServers = async (baseURL: string): Promise<ServerForm[]> => {
    const response = await api.post('/api/admin/probe-sync', {
      probe_type: 'nezha',
      address: baseURL,
    })

    const servers: Array<any> = response.data?.servers ?? []
    return servers.map((server, index) => ({
      key: `${server.server_id || 'server'}-${index}-${generateKey()}`,
      server_id: server.server_id ?? '',
      name: server.name ?? `服务器 ${index + 1}`,
      traffic_method: server.traffic_method ?? 'both',
      monthly_traffic_gb: server.monthly_traffic_gb ?? null,
    }))
  }

  const fetchKomariServers = async (baseURL: string): Promise<ServerForm[]> => {
    const response = await api.post('/api/admin/probe-sync', {
      probe_type: 'komari',
      address: baseURL,
    })

    const servers: Array<any> = response.data?.servers ?? []
    return servers.map((server, index) => ({
      key: `${server.server_id || 'server'}-${index}-${generateKey()}`,
      server_id: server.server_id ?? '',
      name: server.name ?? `服务器 ${index + 1}`,
      traffic_method: server.traffic_method ?? 'both',
      monthly_traffic_gb: server.monthly_traffic_gb ?? null,
    }))
  }

  const fetchNezhaV0Servers = async (baseURL: string): Promise<ServerForm[]> => {
    const response = await api.post('/api/admin/probe-sync', {
      probe_type: 'nezhav0',
      address: baseURL,
    })

    const servers: Array<any> = response.data?.servers ?? []
    return servers.map((server, index) => ({
      key: `${server.server_id || 'server'}-${index}-${generateKey()}`,
      server_id: server.server_id ?? '',
      name: server.name ?? `服务器 ${index + 1}`,
      traffic_method: server.traffic_method ?? 'both',
      monthly_traffic_gb: 0,
    }))
  }

  const handleSyncServers = async () => {
    if (!formState.address.trim()) {
      toast.error('请先填写探针面板地址')
      return
    }

    setSyncLoading(true)
    try {
      const baseURL = trimAddress()
      let mapped: ServerForm[] = []

      if (formState.probeType === 'dstatus') {
        mapped = await fetchDstatusServers(baseURL)
      } else if (formState.probeType === 'nezha') {
        mapped = await fetchNezhaServers(baseURL)
      } else if (formState.probeType === 'nezhav0') {
        mapped = await fetchNezhaV0Servers(baseURL)
      } else if (formState.probeType === 'komari') {
        mapped = await fetchKomariServers(baseURL)
      } else {
        toast.error('当前探针类型暂不支持自动同步')
        return
      }

      if (mapped.length === 0) {
        toast.error('未从面板获取到服务器列表')
        return
      }

      setFormState((prev) => ({
        ...prev,
        servers: mapped,
      }))
      toast.success('已从面板同步服务器列表')
    } catch (error) {
      console.error(error)
      toast.error(
        error instanceof Error ? error.message : '同步服务器失败，请检查面板地址或网络'
      )
    } finally {
      setSyncLoading(false)
    }
  }

  const handleSubmit = (event: React.FormEvent<HTMLFormElement>) => {
    event.preventDefault()

    if (!formState.address.trim()) {
      toast.error('请输入探针地址')
      return
    }

    if (!formState.probeType) {
      toast.error('请选择探针类型')
      return
    }

    if (formState.servers.length === 0) {
      toast.error('请至少添加一个服务器节点')
      return
    }

    for (const server of formState.servers) {
      if (!server.server_id.trim()) {
        toast.error('服务器 ID 不能为空')
        return
      }
      if (!server.name.trim()) {
        toast.error('服务器名称不能为空')
        return
      }
      if (!server.traffic_method) {
        toast.error('请选择流量计算方式')
        return
      }
      const monthlyValue = (() => {
        if (typeof server.monthly_traffic_gb === 'number') {
          return server.monthly_traffic_gb
        }
        if (typeof server.monthly_traffic_gb === 'string') {
          const trimmed = server.monthly_traffic_gb.trim()
          return trimmed ? Number(trimmed) : NaN
        }
        return NaN
      })()
      if (!Number.isFinite(monthlyValue) || monthlyValue <= 0) {
        toast.error('请填写服务器月流量（GB）')
        return
      }
    }

    mutation.mutate()
  }

  if (profileLoading || (isAdmin && configLoading)) {
    return (
      <div className='min-h-svh bg-background'>
        <Topbar />
        <main className='mx-auto w-full max-w-5xl px-4 py-8 sm:px-6 pt-24'>
          <Card className='border-dashed shadow-none'>
            <CardHeader>
              <CardTitle>加载中…</CardTitle>
              <CardDescription>正在读取探针配置，请稍候。</CardDescription>
            </CardHeader>
            <CardContent>
              <div className='space-y-3'>
                <div className='h-10 w-full animate-pulse rounded-md bg-muted' />
                <div className='h-10 w-full animate-pulse rounded-md bg-muted' />
                <div className='h-10 w-full animate-pulse rounded-md bg-muted' />
              </div>
            </CardContent>
          </Card>
        </main>
      </div>
    )
  }

  if (!isAdmin) {
    return (
      <div className='min-h-svh bg-background'>
        <Topbar />
        <main className='mx-auto flex w-full max-w-3xl flex-col items-center justify-center gap-4 px-4 py-20 text-center sm:px-6 pt-24'>
          <Card className='w-full border-dashed shadow-none'>
            <CardHeader>
              <CardTitle>权限不足</CardTitle>
              <CardDescription>仅管理员可管理探针数据源配置。</CardDescription>
            </CardHeader>
          </Card>
        </main>
      </div>
    )
  }

  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <main className='mx-auto w-full max-w-5xl px-4 py-8 sm:px-6 pt-24'>
        <form className='space-y-8' onSubmit={handleSubmit}>
          <section className='space-y-2'>
            <div className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <h1 className='text-3xl font-semibold tracking-tight'>探针数据源</h1>
                <p className='text-muted-foreground'>配置 traffic 接口所使用的探针类型、面板地址以及参与汇总的服务器列表。</p>
              </div>
              <Button type='submit' disabled={mutation.isPending}>
                {mutation.isPending ? '保存中…' : '保存配置'}
              </Button>
            </div>
            {lastUpdated ? (
              <p className='text-sm text-muted-foreground'>最近保存：{lastUpdated}</p>
            ) : null}
          </section>

          <Card>
            <CardHeader>
              <CardTitle>探针信息</CardTitle>
              <CardDescription>支持哪吒、DStatus、Komari 三种探针类型。</CardDescription>
            </CardHeader>
            <CardContent className='space-y-6'>
              <div className='grid gap-6 sm:grid-cols-2'>
                <div className='space-y-2'>
                  <Label htmlFor='probe-type'>探针类型</Label>
                  <Select
                    key={formState.probeType}
                    value={formState.probeType}
                    onValueChange={(value) =>
                      setFormState((prev) => ({ ...prev, probeType: value }))
                    }
                  >
                    <SelectTrigger id='probe-type'>
                      <SelectValue placeholder='选择探针类型' />
                    </SelectTrigger>
                    <SelectContent>
                      {PROBE_TYPES.map((item) => (
                        <SelectItem key={item.value} value={item.value}>
                          {item.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className='space-y-2'>
                  <Label htmlFor='probe-address'>面板地址</Label>
                  <Input
                    id='probe-address'
                    value={formState.address}
                    onChange={(event) =>
                      setFormState((prev) => ({ ...prev, address: event.target.value }))
                    }
                    placeholder='例如：https://panel.example.com'
                  />
                </div>
              </div>
            </CardContent>
          </Card>

          <Card>
            <CardHeader className='flex flex-col gap-2 sm:flex-row sm:items-center sm:justify-between'>
              <div>
                <CardTitle>服务器列表</CardTitle>
                <CardDescription>根据探针类型填写对应的服务器 ID、名称和流量统计方式。</CardDescription>
                <p className='mt-2 text-sm font-semibold text-destructive'>请为每个服务器填写月流量（GB），该字段为必填项。</p>
              </div>
              <div className='flex flex-wrap gap-2'>
                {['dstatus', 'nezha', 'nezhav0', 'komari'].includes(formState.probeType) ? (
                  <Button
                    type='button'
                    variant='secondary'
                    size='sm'
                    onClick={handleSyncServers}
                    disabled={syncLoading}
                  >
                    {syncLoading ? '同步中…' : '从面板同步'}
                  </Button>
                ) : null}
                <Button type='button' variant='outline' size='sm' onClick={handleAddServer}>
                  <Plus className='mr-2 size-4' />新增服务器
                </Button>
              </div>
            </CardHeader>
            <CardContent>
              {/* 移动端卡片视图 */}
              <div className='md:hidden space-y-3'>
                {formState.servers.length === 0 ? (
                  <Card>
                    <CardContent className='text-center text-muted-foreground py-8'>
                      尚未配置服务器，请点击右上角按钮添加。
                    </CardContent>
                  </Card>
                ) : (
                  formState.servers.map((server, index) => {
                    const numericMonthly =
                      typeof server.monthly_traffic_gb === 'number'
                        ? server.monthly_traffic_gb
                        : typeof server.monthly_traffic_gb === 'string'
                          ? Number(server.monthly_traffic_gb)
                          : NaN
                    const monthlyInvalid = !Number.isFinite(numericMonthly) || numericMonthly <= 0

                    return (
                      <Card key={server.key} className='overflow-hidden'>
                        <CardContent className='p-3 space-y-2'>
                          {/* 头部：显示名称和删除按钮 */}
                          <div className='flex items-center justify-between'>
                            <div className='font-medium truncate flex-1 mr-2'>{server.name || '未命名服务器'}</div>
                            <Button
                              type='button'
                              variant='outline'
                              size='icon'
                              className='size-8 text-destructive hover:text-destructive hover:bg-destructive/10 shrink-0'
                              onClick={() => handleRemoveServer(index)}
                            >
                              <Trash2 className='size-4' />
                            </Button>
                          </div>

                          {/* 只读信息 - 紧凑单行显示 */}
                          <div className='text-xs'>
                            <div className='flex items-start gap-2'>
                              <span className='text-muted-foreground shrink-0 min-w-[60px]'>服务器ID:</span>
                              <span className='flex-1 min-w-0 font-mono break-all'>{server.server_id}</span>
                            </div>
                          </div>

                          {/* 可编辑字段 - 流量统计和月流量在同一行 */}
                          <div className='grid grid-cols-2 gap-2 pt-1'>
                            <div className='space-y-1.5'>
                              <Label className='text-xs text-muted-foreground'>流量统计</Label>
                              <Select
                                value={server.traffic_method}
                                onValueChange={(value) =>
                                  handleServerChange(index, 'traffic_method', value)
                                }
                              >
                                <SelectTrigger className='h-9'>
                                  <SelectValue />
                                </SelectTrigger>
                                <SelectContent>
                                  {TRAFFIC_METHODS.map((item) => (
                                    <SelectItem key={item.value} value={item.value}>
                                      {item.label}
                                    </SelectItem>
                                  ))}
                                </SelectContent>
                              </Select>
                            </div>

                            <div className='space-y-1.5'>
                              <Label className='text-xs text-muted-foreground'>
                                月流量 (GB) <span className='text-destructive'>*</span>
                              </Label>
                              <Input
                                type='number'
                                inputMode='decimal'
                                min={0}
                                step='0.01'
                                value={
                                  typeof server.monthly_traffic_gb === 'number'
                                    ? server.monthly_traffic_gb
                                    : server.monthly_traffic_gb ?? ''
                                }
                                onChange={(event) =>
                                  handleServerChange(
                                    index,
                                    'monthly_traffic_gb',
                                    event.target.value
                                  )
                                }
                                className={
                                  monthlyInvalid
                                    ? 'h-9 border-destructive focus-visible:ring-destructive/80 focus-visible:ring-2'
                                    : 'h-9'
                                }
                                aria-invalid={monthlyInvalid}
                                placeholder='必填'
                                required
                                title='请填写服务器的月流量（GB）'
                              />
                            </div>
                          </div>
                          {monthlyInvalid && (
                            <p className='text-xs font-semibold text-destructive'>请输入该服务器的月流量（GB）</p>
                          )}
                        </CardContent>
                      </Card>
                    )
                  })
                )}
              </div>

              {/* 桌面端表格视图 */}
              <div className='hidden md:block overflow-x-auto max-h-[600px] overflow-y-auto'>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className='w-[200px]'>服务器 ID</TableHead>
                      <TableHead className='w-[200px]'>显示名称</TableHead>
                      <TableHead className='w-[160px]'>流量统计</TableHead>
                      <TableHead className='w-[160px]'>
                        <span className='flex items-center gap-1'>
                          月流量 (GB)
                          <span className='text-destructive'>*</span>
                        </span>
                      </TableHead>
                      <TableHead className='w-[80px]' />
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {formState.servers.map((server, index) => {
                      const numericMonthly =
                        typeof server.monthly_traffic_gb === 'number'
                          ? server.monthly_traffic_gb
                          : typeof server.monthly_traffic_gb === 'string'
                            ? Number(server.monthly_traffic_gb)
                            : NaN
                      const monthlyInvalid = !Number.isFinite(numericMonthly) || numericMonthly <= 0

                      return (
                        <TableRow key={server.key}>
                        <TableCell>
                          <Input
                            value={server.server_id}
                            readOnly
                            disabled
                            placeholder='服务器唯一 ID'
                            className='bg-muted/60 text-muted-foreground'
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            value={server.name}
                            readOnly
                            disabled
                            placeholder='用于展示的名称'
                            className='bg-muted/60 text-muted-foreground'
                          />
                        </TableCell>
                        <TableCell>
                          <Select
                            value={server.traffic_method}
                            onValueChange={(value) =>
                              handleServerChange(index, 'traffic_method', value)
                            }
                          >
                            <SelectTrigger>
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              {TRAFFIC_METHODS.map((item) => (
                                <SelectItem key={item.value} value={item.value}>
                                  {item.label}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </TableCell>
                        <TableCell>
                          <Input
                            type='number'
                            inputMode='decimal'
                            min={0}
                            step='0.01'
                            value={
                              typeof server.monthly_traffic_gb === 'number'
                                ? server.monthly_traffic_gb
                                : server.monthly_traffic_gb ?? ''
                            }
                            onChange={(event) =>
                              handleServerChange(
                                index,
                                'monthly_traffic_gb',
                                event.target.value
                              )
                            }
                               className={
                                 monthlyInvalid
                                   ? 'border-destructive focus-visible:ring-destructive/80 focus-visible:ring-2'
                                   : undefined
                               }
                               aria-invalid={monthlyInvalid}
                               placeholder='必填，单位 GB'
                               required
                               title='请填写服务器的月流量（GB）'
                             />
                             {monthlyInvalid ? (
                               <p className='mt-1 text-xs font-semibold text-destructive'>请输入该服务器的月流量（GB）</p>
                             ) : (
                               <p className='mt-1 text-xs text-muted-foreground'>单位：GB</p>
                             )}
                           </TableCell>
                           <TableCell className='text-right'>
                             <Button
                               type='button'
                               variant='outline'
                               size='icon'
                               className='text-destructive hover:text-destructive hover:bg-destructive/10'
                               onClick={() => handleRemoveServer(index)}
                             >
                               <Trash2 className='size-4' />
                             </Button>
                           </TableCell>
                        </TableRow>
                      )
                    })}
                    {formState.servers.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={5} className='py-10 text-center text-muted-foreground'>
                          尚未配置服务器，请点击右上角按钮添加。
                        </TableCell>
                      </TableRow>
                    ) : null}
                  </TableBody>
                </Table>
              </div>
            </CardContent>
          </Card>
        </form>
      </main>
    </div>
  )
}
