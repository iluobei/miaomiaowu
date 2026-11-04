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
 * 正规 CA 证书节点不固定证书指纹(从妙妙屋X 移植,默认关)。
 * 开了之后,证书由公共 CA 签发的节点在订阅里不带证书指纹(HY2 的 pinSHA256 等)、
 * 也不跳过证书验证,客户端按域名正常校验,证书续期后不用重新拉订阅;自签证书照旧固定指纹。
 */
export function CACertNoPinCard() {
  const queryClient = useQueryClient()
  const queryKey = ['ca-cert-no-pin']
  const { data: enabled = false, isLoading } = useQuery({
    queryKey,
    queryFn: async () =>
      (await api.get('/api/admin/system-settings/ca-cert-no-pin')).data
        ?.enabled === true,
  })
  const save = useMutation({
    mutationFn: async (next: boolean) =>
      api.put('/api/admin/system-settings/ca-cert-no-pin', { enabled: next }),
    onSuccess: (_res, next) => {
      queryClient.setQueryData(queryKey, next)
      toast.success(
        next
          ? '已开启:正规 CA 证书节点不再固定证书指纹'
          : '已关闭:节点照旧固定证书指纹'
      )
    },
    onError: (e: { response?: { data?: { message?: string } } }) =>
      toast.error(e.response?.data?.message || '保存失败'),
  })

  return (
    <Card>
      <CardHeader className='pb-4'>
        <CardTitle>正规 CA 证书节点不固定证书指纹</CardTitle>
        <CardDescription>
          订阅默认会把节点的证书指纹写进去(如 HY2 链接的
          pinSHA256),客户端只认这一张证书 —— Let&apos;s Encrypt 等正规证书每
          60~90 天续期一次,续期后客户端会连不上,直到重新更新订阅。
        </CardDescription>
      </CardHeader>
      <CardContent>
        <div className='flex items-start justify-between gap-4 rounded-lg border p-3'>
          <div className='min-w-0 space-y-1'>
            <div className='text-sm font-medium'>
              正规 CA 证书节点不固定证书指纹
            </div>
            <p className='text-muted-foreground text-xs'>
              {enabled
                ? '已开启:证书由正规 CA 签发的节点,订阅里不再带证书指纹,并关闭「跳过证书验证」,客户端按域名正常校验,证书续期不受影响。自签证书的节点照旧固定指纹。'
                : '已关闭:所有 TLS 节点照旧输出证书指纹(与以前一致)。'}
            </p>
            <p className='text-muted-foreground text-xs'>
              判断依据:开启后立刻、之后每天一次,对 TCP TLS
              节点握手一次,看证书能否用系统根证书按域名校验通过(结果见日志管理的「节点证书
              CA 校验」任务)。HY2 / TUIC 等 QUIC 节点无法判断,照旧固定指纹。
            </p>
          </div>
          <Switch
            className='shrink-0'
            checked={enabled}
            disabled={isLoading || save.isPending}
            onCheckedChange={(on) => save.mutate(on)}
            aria-label='正规 CA 证书节点不固定证书指纹'
          />
        </div>
      </CardContent>
    </Card>
  )
}
