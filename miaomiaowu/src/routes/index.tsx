// @ts-nocheck
import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import {
  Area,
  AreaChart,
  CartesianGrid,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from 'recharts'
import { Activity, HardDrive, PieChart, TrendingUp } from 'lucide-react'
import { Topbar } from '@/components/layout/topbar'
import { api } from '@/lib/api'
import { useAuthStore } from '@/stores/auth-store'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'

// @ts-ignore - retained simple route definition
export const Route = createFileRoute('/')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/login' })
    }
  },
  component: DashboardPage,
})

function DashboardPage() {
  const { auth } = useAuthStore()

  const numberFormatter = useMemo(
    () =>
      new Intl.NumberFormat('zh-CN', {
        maximumFractionDigits: 2,
        minimumFractionDigits: 0,
      }),
    []
  )

  const { data, isLoading, isError } = useQuery({
    queryKey: ['traffic-summary'],
    queryFn: async () => {
      const response = await api.get('/api/traffic/summary')
      return response.data
    },
    staleTime: 5 * 60 * 1000,
    refetchInterval: 5 * 60 * 1000,
    enabled: Boolean(auth.accessToken),
  })

  const metrics = useMemo(() => data?.metrics ?? {}, [data?.metrics])

  const cards = useMemo(
    () => [
      {
        title: '总流量配额',
        description: '全部服务器的可用总配额',
        value: formatMetric(metrics.total_limit_gb, numberFormatter),
        icon: TrendingUp,
      },
      {
        title: '已用流量',
        description: '截止今日的累计消耗',
        value: formatMetric(metrics.total_used_gb, numberFormatter),
        icon: Activity,
      },
      {
        title: '剩余流量',
        description: '仍可分配的余量',
        value: formatMetric(metrics.total_remaining_gb, numberFormatter),
        icon: HardDrive,
      },
      {
        title: '使用率',
        description: '累计使用占比',
        value: formatPercentage(metrics.usage_percentage, numberFormatter),
        progress: Number(metrics.usage_percentage ?? 0),
        icon: PieChart,
      },
    ],
    [metrics, numberFormatter]
  )

  const chartData = useMemo(() => {
    return (data?.history ?? []).map((item: any) => ({
      date: item.date,
      label: item.date.slice(5),
      used: Number(item.used_gb ?? 0),
    }))
  }, [data])

  const hasHistory = chartData.length > 0

  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <main className='mx-auto w-full max-w-5xl px-4 py-8 sm:px-6 pt-24'>

        <section className=' grid gap-4 sm:grid-cols-2 lg:grid-cols-4'>
          {isLoading
            ? Array.from({ length: 4 }).map((_, index) => (
                <Card key={index}>
                  <CardHeader className='space-y-2'>
                    <CardTitle className='flex flex-row items-center justify-between text-base'>
                      <Skeleton className='h-5 w-24' />
                      <Skeleton className='h-10 w-10 rounded-full' />
                    </CardTitle>
                    <CardDescription>
                      <Skeleton className='h-4 w-32' />
                    </CardDescription>
                  </CardHeader>
                  <CardContent>
                    <Skeleton className='h-9 w-28' />
                  </CardContent>
                </Card>
              ))
            : cards.map(({ title, description, value, icon: Icon, progress }) => (
                <Card key={title}>
                  <CardHeader className='space-y-2'>
                    <CardTitle className='flex flex-row items-center justify-between text-base'>
                      {title}
                      <Icon className='size-8 text-primary' />
                    </CardTitle>
                    <CardDescription>{description}</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <div className='text-3xl font-semibold'>{value}</div>
                    {typeof progress === 'number' && !Number.isNaN(progress) ? (
                      <div className='mt-4 space-y-2'>
                        <Progress value={Math.min(Math.max(progress, 0), 100)} max={100} />
                        <div className='text-xs text-muted-foreground'>
                          已使用 {numberFormatter.format(progress)}%
                        </div>
                      </div>
                    ) : null}
                  </CardContent>
                </Card>
              ))}
        </section>

        <Card className='mt-8'>
          <CardHeader>
            <CardTitle>每日流量消耗</CardTitle>
            <CardDescription>最近记录的日度流量趋势</CardDescription>
          </CardHeader>
          <CardContent className='pt-0'>
            <div className='h-80'>
              {isLoading ? (
                <div className='flex h-full items-center justify-center'>
                  <Skeleton className='h-32 w-full max-w-3xl' />
                </div>
              ) : !hasHistory ? (
                <div className='flex h-full items-center justify-center text-sm text-muted-foreground'>
                  {isError ? '数据加载失败，请稍后重试。' : '暂无历史记录。'}
                </div>
              ) : (
                <ResponsiveContainer width='100%' height='100%'>
                  <AreaChart data={chartData} margin={{ left: 16, right: 16, top: 24, bottom: 8 }}>
                    <defs>
                      <linearGradient id='dailyUsageGradient' x1='0' y1='0' x2='0' y2='1'>
                        <stop offset='0%' stopColor='#d97757' stopOpacity={0.7} />
                        <stop offset='100%' stopColor='#d97757' stopOpacity={0.2} />
                      </linearGradient>
                      <filter id='shadow' x='-50%' y='-50%' width='200%' height='200%'>
                        <feDropShadow dx='0' dy='2' stdDeviation='3' floodColor='#d97757' floodOpacity='0.3'/>
                      </filter>
                    </defs>
                    <XAxis
                      dataKey='label'
                      tickLine={false}
                      axisLine={false}
                      splitLine={false}
                      className='fill-foreground'
                      stroke='#a1a1aa'
                    />
                    <YAxis
                      tickLine={false}
                      axisLine={false}
                      splitLine={false}
                      tickFormatter={(value: number) => `${numberFormatter.format(value)}`}
                      className='fill-foreground'
                      stroke='#a1a1aa'
                    />
                    <Tooltip
                      cursor={{ stroke: '#d97757', strokeWidth: 2 }}
                      labelFormatter={(label: string) => `日期：${chartData.find((item) => item.label === label)?.date ?? label}`}
                      formatter={(value: number) => [`${numberFormatter.format(value)} GB`, '日消耗']}
                      contentStyle={{
                        backgroundColor: 'hsl(var(--popover))',
                        border: '1px solid hsl(var(--border))',
                        borderRadius: 'var(--radius)',
                      }}
                      labelStyle={{ color: 'hsl(var(--foreground))' }}
                    />
                    <Area
                      type='monotone'
                      dataKey='used'
                      stroke='#d97757'
                      fill='url(#dailyUsageGradient)'
                      strokeWidth={3}
                      name='日消耗'
                      filter='url(#shadow)'
                    />
                  </AreaChart>
                </ResponsiveContainer>
              )}
            </div>
          </CardContent>
        </Card>
      </main>
    </div>
  )
}

function formatMetric(value: number | undefined, formatter: Intl.NumberFormat) {
  if (value === undefined || value === null) return '--'
  let unit = 'GB'
  let displayValue = value

  if (value >= 1024) {
    displayValue = value / 1024
    unit = 'TB'
  }

  return `${formatter.format(displayValue)} ${unit}`
}

function formatPercentage(value: number | undefined, formatter: Intl.NumberFormat) {
  if (value === undefined || value === null) return '--'
  return `${formatter.format(value)} %`
}
