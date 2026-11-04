import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Switch } from '@/components/ui/switch'

/**
 * PWA 开关(全站生效,默认开,从妙妙屋X 移植)。
 * 值随首屏 HTML 的 <meta name="mmw-pwa"> 下发,改了之后各页面下次打开时生效 ——
 * 关掉时浏览器里已注册的 SW 会自己注销并清缓存(lib/pwa.ts + 服务端 /sw.js 两边兜)。
 */
export function PWASettingCard() {
  const queryClient = useQueryClient()
  const queryKey = ['pwa-setting']
  const { data: enabled = true, isLoading } = useQuery({
    queryKey,
    queryFn: async () =>
      (await api.get('/api/admin/system-settings/pwa')).data?.enabled !== false,
  })
  const save = useMutation({
    mutationFn: async (next: boolean) =>
      api.put('/api/admin/system-settings/pwa', { enabled: next }),
    onSuccess: (_res, next) => {
      queryClient.setQueryData(queryKey, next)
      toast.success(next ? '已启用 PWA' : '已关闭 PWA')
    },
    onError: (e: { response?: { data?: { message?: string } } }) =>
      toast.error(e.response?.data?.message || '保存失败'),
  })

  return (
    <Card>
      <CardHeader className='pb-4'>
        <CardTitle>安装到桌面(PWA)</CardTitle>
        <CardDescription>
          允许把面板装到电脑桌面或手机主屏幕,像 App
          一样在独立窗口打开;静态资源会缓存在浏览器里,再次打开更快。接口与订阅链接从不缓存。升级后,已打开的页面会提示刷新。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='flex items-start justify-between gap-4 rounded-lg border p-3'>
          <div className='min-w-0'>
            <div className='text-sm font-medium'>启用 PWA</div>
            <p className='text-muted-foreground text-xs'>
              {enabled
                ? '浏览器会注册 Service Worker 并缓存静态资源。'
                : '关闭后,已注册的 Service Worker 会在用户下次打开面板时自动注销并清除缓存。'}
            </p>
          </div>
          <Switch
            className='shrink-0'
            checked={enabled}
            disabled={isLoading || save.isPending}
            onCheckedChange={(on) => save.mutate(on)}
            aria-label='启用 PWA'
          />
        </div>
      </CardContent>
    </Card>
  )
}
