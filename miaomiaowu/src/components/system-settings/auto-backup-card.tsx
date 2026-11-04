// 自动备份:定时把完整备份投递到 WebDAV / S3 兼容 / Google Drive,并按份数清理旧的。
//
// 凭据在后端用密钥种子派生的 key 加密入库,读接口一律返回占位符 ********;
// 保存时把占位符原样回传 = 不修改该项。所以这里**不要**把占位符当成真值去做校验。
//
// 从妙妙屋X 迁移而来,去掉了「包含数据库」(那是 PostgreSQL 部署用的,妙妙屋只有 SQLite,库本来就在 data/ 里)。
import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CloudUpload, Loader2 } from 'lucide-react'
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
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'

type Kind = 'webdav' | 's3' | 'gdrive'

interface AutoBackupConfig {
  enabled: boolean
  kind: Kind | ''
  interval_hours: number
  // 每天固定备份整点(0-23);null/缺省 = 按间隔。设了就每天该点跑一次,忽略间隔。
  preferred_hour?: number | null
  keep: number
  webdav: { URL: string; Username: string; Password: string }
  s3: {
    Endpoint: string
    Region: string
    Bucket: string
    Prefix: string
    AccessKey: string
    SecretKey: string
    PathStyle: boolean
  }
  gdrive: {
    ClientID: string
    ClientSecret: string
    RefreshToken: string
    FolderID: string
  }
}

const empty: AutoBackupConfig = {
  enabled: false,
  kind: '',
  interval_hours: 24,
  preferred_hour: null,
  keep: 7,
  webdav: { URL: '', Username: '', Password: '' },
  s3: {
    Endpoint: '',
    Region: 'auto',
    Bucket: '',
    Prefix: '',
    AccessKey: '',
    SecretKey: '',
    PathStyle: false,
  },
  gdrive: { ClientID: '', ClientSecret: '', RefreshToken: '', FolderID: '' },
}

export function AutoBackupCard() {
  const queryClient = useQueryClient()
  const [cfg, setCfg] = useState<AutoBackupConfig>(empty)
  const [dirty, setDirty] = useState(false)

  const { data } = useQuery({
    queryKey: ['auto-backup-config'],
    queryFn: async () =>
      (await api.get('/api/admin/backup/auto')).data as {
        config: AutoBackupConfig
        last_run: string
      },
    staleTime: 60 * 1000,
  })

  useEffect(() => {
    // 只在没改过时回填,免得后台刷新冲掉正在编辑的内容。
    if (data?.config && !dirty) setCfg({ ...empty, ...data.config })
  }, [data, dirty])

  const set = (patch: Partial<AutoBackupConfig>) => {
    setCfg((p) => ({ ...p, ...patch }))
    setDirty(true)
  }
  const setSub = <K extends 'webdav' | 's3' | 'gdrive'>(
    key: K,
    patch: Partial<AutoBackupConfig[K]>
  ) => {
    setCfg((p) => ({ ...p, [key]: { ...p[key], ...patch } }))
    setDirty(true)
  }

  const saveMutation = useMutation({
    mutationFn: async () => (await api.put('/api/admin/backup/auto', cfg)).data,
    onSuccess: () => {
      setDirty(false)
      queryClient.invalidateQueries({ queryKey: ['auto-backup-config'] })
      toast.success('自动备份设置已保存')
    },
    onError: (e: any) => toast.error(e?.response?.data?.error || '保存失败'),
  })

  const testMutation = useMutation({
    mutationFn: async () =>
      (await api.post('/api/admin/backup/auto/test', cfg)).data as {
        success: boolean
        message?: string
        error?: string
      },
    onSuccess: (r) =>
      r.success
        ? toast.success(r.message || '连接正常')
        : toast.error(r.error || '连接失败'),
    onError: (e: any) => toast.error(e?.response?.data?.error || '测试失败'),
  })

  const runMutation = useMutation({
    mutationFn: async () =>
      (await api.post('/api/admin/backup/auto/run', {})).data,
    onSuccess: (r: any) => toast.success(r?.message || '已开始备份'),
    onError: (e: any) => toast.error(e?.response?.data?.error || '触发失败'),
  })

  return (
    <Card>
      <CardHeader className='pb-4'>
        <CardTitle className='flex items-center gap-2'>
          <CloudUpload className='size-5' /> 自动备份
        </CardTitle>
        <CardDescription>
          定时打一份完整备份(数据库、订阅文件)并上传到远端,按份数保留。
          凭据加密存储,保存后不再回显。
          {data?.last_run && (
            <span className='ml-1'>上次成功:{data.last_run}</span>
          )}
        </CardDescription>
      </CardHeader>
      <CardContent className='space-y-4'>
        <div className='flex items-center justify-between'>
          <Label htmlFor='auto-backup-enabled'>启用定时备份</Label>
          <Switch
            id='auto-backup-enabled'
            checked={cfg.enabled}
            onCheckedChange={(v) => set({ enabled: v })}
          />
        </div>

        <div className='grid gap-3 sm:grid-cols-3'>
          <div className='space-y-1.5'>
            <Label>备份目标</Label>
            <Select
              value={cfg.kind || undefined}
              onValueChange={(v) => set({ kind: v as Kind })}
            >
              <SelectTrigger>
                <SelectValue placeholder='选择' />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='webdav'>WebDAV</SelectItem>
                <SelectItem value='s3'>
                  S3 兼容(R2 / B2 / MinIO / OSS)
                </SelectItem>
                <SelectItem value='gdrive'>Google Drive</SelectItem>
              </SelectContent>
            </Select>
          </div>
          <div className='space-y-1.5'>
            <Label>间隔(小时)</Label>
            <Input
              type='number'
              min={1}
              value={cfg.interval_hours}
              disabled={cfg.preferred_hour != null}
              onChange={(e) =>
                set({ interval_hours: Number(e.target.value) || 24 })
              }
            />
            {cfg.preferred_hour != null && (
              <p className='text-muted-foreground text-xs'>
                已设固定时间,间隔忽略
              </p>
            )}
          </div>
          <div className='space-y-1.5'>
            <Label>每天固定时间(点)</Label>
            <Input
              type='number'
              min={0}
              max={23}
              placeholder='留空=按间隔'
              value={cfg.preferred_hour ?? ''}
              onChange={(e) => {
                const v = e.target.value.trim()
                if (v === '') {
                  set({ preferred_hour: null })
                  return
                }
                const n = Math.floor(Number(v))
                set({
                  preferred_hour: Number.isFinite(n)
                    ? Math.min(23, Math.max(0, n))
                    : null,
                })
              }}
            />
            <p className='text-muted-foreground text-xs'>
              如 3 = 每天凌晨3点备份;留空则按上面的间隔
            </p>
          </div>
          <div className='space-y-1.5'>
            <Label>远端保留份数</Label>
            <Input
              type='number'
              min={0}
              value={cfg.keep}
              onChange={(e) => set({ keep: Number(e.target.value) || 0 })}
            />
            <p className='text-muted-foreground text-xs'>0 = 不自动清理</p>
          </div>
        </div>

        {cfg.kind === 'webdav' && (
          <div className='grid gap-3 sm:grid-cols-3'>
            <div className='space-y-1.5 sm:col-span-3'>
              <Label>WebDAV 目录地址</Label>
              <Input
                value={cfg.webdav.URL}
                onChange={(e) => setSub('webdav', { URL: e.target.value })}
                placeholder='https://dav.example.com/remote.php/dav/files/me/miaomiaowu'
              />
            </div>
            <div className='space-y-1.5'>
              <Label>用户名</Label>
              <Input
                value={cfg.webdav.Username}
                onChange={(e) => setSub('webdav', { Username: e.target.value })}
              />
            </div>
            <div className='space-y-1.5'>
              <Label>密码</Label>
              <Input
                type='password'
                value={cfg.webdav.Password}
                onChange={(e) => setSub('webdav', { Password: e.target.value })}
              />
            </div>
          </div>
        )}

        {cfg.kind === 's3' && (
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-1.5'>
              <Label>Endpoint</Label>
              <Input
                value={cfg.s3.Endpoint}
                onChange={(e) => setSub('s3', { Endpoint: e.target.value })}
                placeholder='https://xxx.r2.cloudflarestorage.com(AWS 可留空)'
              />
            </div>
            <div className='space-y-1.5'>
              <Label>Region</Label>
              <Input
                value={cfg.s3.Region}
                onChange={(e) => setSub('s3', { Region: e.target.value })}
                placeholder='R2 填 auto'
              />
            </div>
            <div className='space-y-1.5'>
              <Label>Bucket</Label>
              <Input
                value={cfg.s3.Bucket}
                onChange={(e) => setSub('s3', { Bucket: e.target.value })}
              />
            </div>
            <div className='space-y-1.5'>
              <Label>目录前缀(可空)</Label>
              <Input
                value={cfg.s3.Prefix}
                onChange={(e) => setSub('s3', { Prefix: e.target.value })}
              />
            </div>
            <div className='space-y-1.5'>
              <Label>Access Key</Label>
              <Input
                value={cfg.s3.AccessKey}
                onChange={(e) => setSub('s3', { AccessKey: e.target.value })}
              />
            </div>
            <div className='space-y-1.5'>
              <Label>Secret Key</Label>
              <Input
                type='password'
                value={cfg.s3.SecretKey}
                onChange={(e) => setSub('s3', { SecretKey: e.target.value })}
              />
            </div>
            <label className='flex cursor-pointer items-center gap-2 sm:col-span-2'>
              <Checkbox
                checked={cfg.s3.PathStyle}
                onCheckedChange={(v) => setSub('s3', { PathStyle: v === true })}
              />
              <span className='text-sm'>
                路径风格(MinIO / 自建常用;R2、AWS 不用勾)
              </span>
            </label>
          </div>
        )}

        {cfg.kind === 'gdrive' && (
          <div className='grid gap-3 sm:grid-cols-2'>
            <div className='space-y-1.5'>
              <Label>Client ID</Label>
              <Input
                value={cfg.gdrive.ClientID}
                onChange={(e) => setSub('gdrive', { ClientID: e.target.value })}
              />
            </div>
            <div className='space-y-1.5'>
              <Label>Client Secret</Label>
              <Input
                type='password'
                value={cfg.gdrive.ClientSecret}
                onChange={(e) =>
                  setSub('gdrive', { ClientSecret: e.target.value })
                }
              />
            </div>
            <div className='space-y-1.5'>
              <Label>Refresh Token</Label>
              <Input
                type='password'
                value={cfg.gdrive.RefreshToken}
                onChange={(e) =>
                  setSub('gdrive', { RefreshToken: e.target.value })
                }
              />
            </div>
            <div className='space-y-1.5'>
              <Label>文件夹 ID(可空)</Label>
              <Input
                value={cfg.gdrive.FolderID}
                onChange={(e) => setSub('gdrive', { FolderID: e.target.value })}
                placeholder='网盘地址栏 folders/ 后面那段'
              />
            </div>
            <p className='text-muted-foreground text-xs sm:col-span-2'>
              用个人账号的 refresh token,不要用服务账号 ——
              服务账号的存储配额几乎为 0,大备份必然上传失败。
            </p>
          </div>
        )}

        <div className='flex flex-wrap gap-2'>
          <Button
            onClick={() => saveMutation.mutate()}
            disabled={saveMutation.isPending}
          >
            {saveMutation.isPending && (
              <Loader2 className='mr-2 size-4 animate-spin' />
            )}
            保存
          </Button>
          <Button
            variant='outline'
            onClick={() => testMutation.mutate()}
            disabled={testMutation.isPending || !cfg.kind}
          >
            {testMutation.isPending && (
              <Loader2 className='mr-2 size-4 animate-spin' />
            )}
            测试连接
          </Button>
          <Button
            variant='outline'
            onClick={() => runMutation.mutate()}
            disabled={runMutation.isPending || !cfg.kind}
          >
            立即备份一次
          </Button>
        </div>
        <p className='text-muted-foreground text-xs'>
          「测试连接」会上传一个小文件、列一次目录再删掉 ——
          只测能否连上是不够的,最常见的配错是能连但没有写权限。
        </p>
      </CardContent>
    </Card>
  )
}
