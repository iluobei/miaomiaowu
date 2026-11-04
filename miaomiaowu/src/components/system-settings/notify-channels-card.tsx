import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { BellRing, Pencil, Plus, Send, Trash2 } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { cn } from '@/lib/utils'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Checkbox } from '@/components/ui/checkbox'
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
import { Switch } from '@/components/ui/switch'
import { ConfirmDialog } from '@/components/confirm-dialog'

// 推送渠道(从妙妙屋X 移植)。「哪些事件要发」仍由「通知推送」里的事件开关决定(全局);
// 这里决定「发到哪」:每个渠道可只订阅部分事件,不选 = 全部已开启的事件。

type Channel = {
  id: number
  type: string
  name: string
  enabled: boolean
  builtin: boolean
  configured: boolean
  params: Record<string, string>
  events: string[]
  last_error: string
  last_error_at: number
  last_ok_at: number
}

type ChannelsResponse = {
  channels: Channel[]
  event_groups: string[]
  types: string[]
}

type ParamField = {
  key: string
  label: string
  secret?: boolean
  // 密码类密钥:密码框、编辑时不回显(留空 = 沿用已保存的值,后端同样处理)
  password?: boolean
  optional?: boolean
  placeholder?: string | ((params: Record<string, string>) => string)
  options?: { value: string; label: string }[]
}

const SMTP_DEFAULT_PORTS: Record<string, string> = {
  starttls: '587',
  tls: '465',
  none: '25',
}

const TYPE_LABELS: Record<string, string> = {
  telegram: 'Telegram',
  feishu: '飞书',
  dingtalk: '钉钉',
  serverchan: 'Server酱',
  bark: 'Bark',
  smtp: '邮件(SMTP)',
}

const TYPE_HINTS: Record<string, string> = {
  smtp: '用自己的邮箱发通知:填发件邮箱的 SMTP 服务器。QQ / 163 / Gmail 等要先在邮箱设置里开启 SMTP,「密码」填授权码(应用专用密码)而不是登录密码。端口与加密方式要对应:465 选 SSL/TLS,587 选 STARTTLS。邮箱服务商对短时间大量发信很敏感,建议只订阅离线、到期这类关键事件。',
  serverchan:
    '在 sct.ftqq.com 获取 SendKey(Turbo 版 SCT 开头,Server酱³ 为 sctp 开头)。注意:免费版每天只能推送 5 条,建议只订阅离线、到期这类关键事件。',
  feishu:
    '在飞书群里添加「自定义机器人」,复制 Webhook 地址;开了「签名校验」就把密钥也填上。飞书限制每个机器人每秒 5 条、每分钟 100 条,超出的消息会排队或被丢弃。',
  dingtalk:
    '在钉钉群里添加「自定义机器人」,安全设置建议选「加签」并填上 SEC 开头的密钥;若用「自定义关键词」,消息里必须含有该关键词。钉钉限制每个机器人每分钟 20 条,超出会被限流 10 分钟。',
  bark: '在 Bark App 里复制推送链接,直接粘贴到 Device Key 即可(会自动拆出服务器地址);自建服务器填自己的地址。',
  telegram: '再加一个 Telegram 机器人 / 群组(比如把告警单独发到另一个群)。',
}

const EVENT_LABELS: Record<string, string> = {
  subscribe_fetch: '订阅获取',
  login: '登录',
  ip_ban: 'IP 封禁',
  silent_mode: '静默模式',
  daily_traffic: '每日流量',
  expiry: '订阅到期',
  node_probe_offline: '节点探测离线',
  node_probe_online: '节点探测恢复',
}

// 每种渠道要填的参数。
const PARAM_FIELDS: Record<string, ParamField[]> = {
  smtp: [
    { key: 'host', label: 'SMTP 服务器', placeholder: 'smtp.qq.com' },
    {
      key: 'security',
      label: '加密方式',
      options: [
        { value: 'starttls', label: 'STARTTLS(一般 587 端口)' },
        { value: 'tls', label: 'SSL/TLS 直连(一般 465 端口)' },
        { value: 'none', label: '不加密(仅限内网中继)' },
      ],
    },
    {
      key: 'port',
      label: '端口',
      optional: true,
      placeholder: (p) => SMTP_DEFAULT_PORTS[p.security || 'starttls'] ?? '',
    },
    {
      key: 'username',
      label: '用户名',
      optional: true,
      placeholder: 'me@example.com',
    },
    { key: 'password', label: '密码 / 授权码', password: true, optional: true },
    {
      key: 'from',
      label: '发件人地址',
      optional: true,
      placeholder: 'me@example.com',
    },
    {
      key: 'from_name',
      label: '发件人名称',
      optional: true,
      placeholder: '妙妙屋',
    },
    { key: 'to', label: '收件人', placeholder: 'a@example.com, b@example.com' },
  ],
  serverchan: [
    {
      key: 'sendkey',
      label: 'SendKey',
      secret: true,
      placeholder: 'SCT… / sctp…',
    },
  ],
  feishu: [
    {
      key: 'webhook',
      label: 'Webhook 地址',
      secret: true,
      placeholder: 'https://open.feishu.cn/open-apis/bot/v2/hook/…',
    },
    { key: 'secret', label: '签名密钥', secret: true, optional: true },
  ],
  dingtalk: [
    {
      key: 'webhook',
      label: 'Webhook 地址',
      secret: true,
      placeholder: 'https://oapi.dingtalk.com/robot/send?access_token=…',
    },
    {
      key: 'secret',
      label: '加签密钥',
      secret: true,
      optional: true,
      placeholder: 'SEC…',
    },
  ],
  bark: [
    { key: 'device_key', label: 'Device Key', secret: true },
    {
      key: 'server',
      label: '服务器地址',
      optional: true,
      placeholder: 'https://api.day.app',
    },
    { key: 'group', label: '分组', optional: true, placeholder: '妙妙屋' },
    {
      key: 'level',
      label: '推送级别',
      optional: true,
      options: [
        { value: '', label: '默认' },
        { value: 'active', label: 'active' },
        { value: 'timeSensitive', label: 'timeSensitive' },
        { value: 'passive', label: 'passive' },
        { value: 'critical', label: 'critical' },
      ],
    },
  ],
  telegram: [
    {
      key: 'bot_token',
      label: 'Bot Token',
      secret: true,
      placeholder: '123456:ABC-DEF…',
    },
    { key: 'chat_id', label: 'Chat ID', placeholder: '-1001234567890' },
  ],
}

// 切换到该类型时预填的参数(下拉框没有「默认」项的要先给个值)
const DEFAULT_PARAMS: Record<string, Record<string, string>> = {
  smtp: { security: 'starttls' },
}

const ADDABLE_TYPES = [
  'smtp',
  'serverchan',
  'feishu',
  'dingtalk',
  'bark',
  'telegram',
]

type Draft = {
  id: number | null
  type: string
  name: string
  enabled: boolean
  params: Record<string, string>
  events: string[]
  // 已保存过的密码类参数(编辑时密码框是空的,用来提示「留空不修改」)
  savedPasswords: string[]
}

function formatTime(sec: number) {
  if (!sec) return ''
  return new Date(sec * 1000).toLocaleString()
}

const typeLabel = (type: string) => TYPE_LABELS[type] ?? type

export function NotifyChannelsCard({
  active,
  notifyEnabled,
}: {
  active: boolean
  notifyEnabled: boolean
}) {
  const queryClient = useQueryClient()
  const [draft, setDraft] = useState<Draft | null>(null)
  const [removing, setRemoving] = useState<Channel | null>(null)

  const query = useQuery({
    queryKey: ['notify-channels'],
    queryFn: async () =>
      (await api.get<ChannelsResponse>('/api/admin/notify-channels')).data,
    enabled: active,
  })
  const channels = query.data?.channels ?? []
  const eventGroups = query.data?.event_groups ?? []

  const refresh = () =>
    queryClient.invalidateQueries({ queryKey: ['notify-channels'] })

  const saveMutation = useMutation({
    mutationFn: async (d: Draft) => {
      const body = {
        type: d.type,
        name: d.name,
        enabled: d.enabled,
        params: d.params,
        events: d.events,
      }
      if (d.id === null) {
        await api.post('/api/admin/notify-channels', body)
      } else {
        await api.put(`/api/admin/notify-channels/${d.id}`, body)
      }
    },
    onSuccess: () => {
      setDraft(null)
      refresh()
      toast.success('推送渠道已保存')
    },
    onError: handleServerError,
  })

  // 列表里直接开关 / 改内置 Telegram 的事件订阅:只提交变化的字段
  const patchMutation = useMutation({
    mutationFn: async ({
      id,
      patch,
    }: {
      id: number
      patch: { enabled?: boolean; events?: string[] }
    }) => {
      await api.put(`/api/admin/notify-channels/${id}`, patch)
    },
    onSuccess: () => refresh(),
    onError: (e) => {
      refresh()
      handleServerError(e)
    },
  })

  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/api/admin/notify-channels/${id}`)
    },
    onSuccess: () => {
      setRemoving(null)
      refresh()
      toast.success('推送渠道已删除')
    },
    onError: handleServerError,
  })

  const testMutation = useMutation({
    mutationFn: async (d: {
      id: number | null
      type: string
      name: string
      params?: Record<string, string>
    }) => {
      await api.post('/api/admin/notify-channels/test', {
        id: d.id ?? 0,
        type: d.type,
        name: d.name,
        params: d.params,
      })
    },
    onSuccess: () => {
      toast.success('测试消息已发送')
      refresh()
    },
    onError: handleServerError,
  })

  const openCreate = () =>
    setDraft({
      id: null,
      type: 'smtp',
      name: '',
      enabled: true,
      params: { ...DEFAULT_PARAMS.smtp },
      events: [],
      savedPasswords: [],
    })

  const openEdit = (c: Channel) => {
    const params = { ...c.params }
    const savedPasswords: string[] = []
    for (const f of PARAM_FIELDS[c.type] ?? []) {
      if (!f.password) continue
      if (params[f.key]) savedPasswords.push(f.key)
      params[f.key] = ''
    }
    setDraft({
      id: c.id,
      type: c.type,
      name: c.name,
      enabled: c.enabled,
      params,
      events: [...c.events],
      savedPasswords,
    })
  }

  return (
    <Card>
      <CardHeader className='pb-4'>
        <div className='flex flex-wrap items-start justify-between gap-3'>
          <div>
            <CardTitle className='flex items-center gap-2'>
              <BellRing className='h-5 w-5' />
              推送渠道
            </CardTitle>
            <CardDescription className='mt-1.5'>
              「通知推送」里开启的事件会推送到下面所有已启用的渠道;每个渠道可以只订阅一部分事件。渠道之间互不影响,某个渠道发送失败会在这里显示原因。
            </CardDescription>
          </div>
          <Button size='sm' onClick={openCreate}>
            <Plus className='mr-1 h-4 w-4' />
            添加渠道
          </Button>
        </div>
      </CardHeader>
      <CardContent className='space-y-3'>
        {!notifyEnabled && (
          <p className='text-muted-foreground rounded-md border border-dashed p-2 text-xs'>
            通知总开关关着,任何渠道都不会推送。
          </p>
        )}
        {channels.map((c) => (
          <div key={c.id} className='space-y-2 rounded-lg border p-3'>
            <div className='flex flex-wrap items-center justify-between gap-2'>
              <div className='flex min-w-0 flex-wrap items-center gap-2'>
                <Badge variant='secondary'>{typeLabel(c.type)}</Badge>
                <span className='truncate text-sm font-medium'>
                  {c.builtin ? 'Telegram(内置)' : c.name || typeLabel(c.type)}
                </span>
                {!c.configured && (
                  <Badge variant='outline' className='text-amber-600'>
                    未配置
                  </Badge>
                )}
              </div>
              <div className='flex items-center gap-1'>
                <Button
                  variant='ghost'
                  size='sm'
                  className='h-8 px-2'
                  disabled={!c.configured || testMutation.isPending}
                  onClick={() =>
                    testMutation.mutate({
                      id: c.id,
                      type: c.type,
                      name: c.name,
                    })
                  }
                  title='发送测试'
                >
                  <Send className='h-4 w-4' />
                </Button>
                {!c.builtin && (
                  <>
                    <Button
                      variant='ghost'
                      size='sm'
                      className='h-8 px-2'
                      onClick={() => openEdit(c)}
                      title='编辑'
                    >
                      <Pencil className='h-4 w-4' />
                    </Button>
                    <Button
                      variant='ghost'
                      size='sm'
                      className='text-destructive h-8 px-2'
                      onClick={() => setRemoving(c)}
                      title='删除'
                    >
                      <Trash2 className='h-4 w-4' />
                    </Button>
                  </>
                )}
                <Switch
                  checked={c.enabled}
                  disabled={
                    patchMutation.isPending ||
                    (!c.builtin && !c.configured && !c.enabled)
                  }
                  onCheckedChange={(enabled) =>
                    patchMutation.mutate({ id: c.id, patch: { enabled } })
                  }
                />
              </div>
            </div>
            <div className='text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs'>
              <span>
                {c.events.length === 0
                  ? '全部已开启的事件'
                  : `订阅 ${c.events.length} 类事件`}
              </span>
              {c.last_ok_at > 0 && (
                <span>最近成功 {formatTime(c.last_ok_at)}</span>
              )}
              {c.builtin && (
                <span>Bot Token / Chat ID 在上方「通知推送」的配置里设置</span>
              )}
            </div>
            {c.last_error && (
              <p className='text-destructive text-xs break-all'>
                最近失败 {formatTime(c.last_error_at)}:{c.last_error}
              </p>
            )}
            {c.builtin && (
              <EventPicker
                groups={eventGroups}
                value={c.events}
                compact
                onChange={(events) =>
                  patchMutation.mutate({ id: c.id, patch: { events } })
                }
              />
            )}
          </div>
        ))}
      </CardContent>

      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title='删除推送渠道'
        desc={
          removing
            ? `确定删除「${removing.name || typeLabel(removing.type)}」吗?删除后不再向它推送。`
            : ''
        }
        destructive
        isLoading={deleteMutation.isPending}
        handleConfirm={() => removing && deleteMutation.mutate(removing.id)}
      />

      <Dialog open={draft !== null} onOpenChange={(o) => !o && setDraft(null)}>
        {draft && (
          <DialogContent className='max-h-[90vh] overflow-y-auto sm:max-w-lg'>
            <DialogHeader>
              <DialogTitle>
                {draft.id === null ? '添加推送渠道' : '编辑推送渠道'}
              </DialogTitle>
              <DialogDescription>
                {TYPE_HINTS[draft.type] ?? ''}
              </DialogDescription>
            </DialogHeader>
            <div className='space-y-4'>
              <div className='grid gap-4 sm:grid-cols-2'>
                <div className='space-y-2'>
                  <Label>渠道类型</Label>
                  <Select
                    value={draft.type}
                    disabled={draft.id !== null}
                    onValueChange={(type) =>
                      setDraft({
                        ...draft,
                        type,
                        params: { ...DEFAULT_PARAMS[type] },
                        savedPasswords: [],
                      })
                    }
                  >
                    <SelectTrigger className='w-full'>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {ADDABLE_TYPES.map((type) => (
                        <SelectItem key={type} value={type}>
                          {typeLabel(type)}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
                <div className='space-y-2'>
                  <Label htmlFor='notify-channel-name'>名称</Label>
                  <Input
                    id='notify-channel-name'
                    value={draft.name}
                    maxLength={64}
                    placeholder={typeLabel(draft.type)}
                    onChange={(e) =>
                      setDraft({ ...draft, name: e.target.value })
                    }
                  />
                </div>
              </div>
              {(PARAM_FIELDS[draft.type] ?? []).map((f) => (
                <div key={f.key} className='space-y-2'>
                  <Label htmlFor={`notify-param-${f.key}`}>
                    {f.label}
                    {f.optional && (
                      <span className='text-muted-foreground ml-1 text-xs font-normal'>
                        (可选)
                      </span>
                    )}
                  </Label>
                  {f.options ? (
                    <Select
                      value={draft.params[f.key] || '_'}
                      onValueChange={(v) =>
                        setDraft({
                          ...draft,
                          params: {
                            ...draft.params,
                            [f.key]: v === '_' ? '' : v,
                          },
                        })
                      }
                    >
                      <SelectTrigger
                        id={`notify-param-${f.key}`}
                        className='w-full'
                      >
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {f.options.map((o) => (
                          <SelectItem
                            key={o.value || '_'}
                            value={o.value || '_'}
                          >
                            {o.label}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  ) : (
                    <Input
                      id={`notify-param-${f.key}`}
                      type={f.password ? 'password' : 'text'}
                      value={draft.params[f.key] ?? ''}
                      placeholder={
                        f.password && draft.savedPasswords.includes(f.key)
                          ? '已保存,留空则不修改'
                          : typeof f.placeholder === 'function'
                            ? f.placeholder(draft.params)
                            : f.placeholder
                      }
                      autoComplete={f.password ? 'new-password' : 'off'}
                      spellCheck={false}
                      onChange={(e) =>
                        setDraft({
                          ...draft,
                          params: { ...draft.params, [f.key]: e.target.value },
                        })
                      }
                    />
                  )}
                </div>
              ))}
              {(PARAM_FIELDS[draft.type] ?? []).some((f) => f.password) ? (
                <p className='text-muted-foreground text-xs'>
                  密码只保存在服务器上,不会再显示;留空保存会沿用原密码。不需要登录时清空用户名即可。
                </p>
              ) : (
                draft.id !== null && (
                  <p className='text-muted-foreground text-xs'>
                    已保存的密钥只显示掩码,保持不变就不会覆盖。
                  </p>
                )
              )}
              <div className='space-y-2'>
                <Label>订阅的事件</Label>
                <EventPicker
                  groups={eventGroups}
                  value={draft.events}
                  onChange={(events) => setDraft({ ...draft, events })}
                />
              </div>
              <div className='flex items-center justify-between rounded-lg border p-3'>
                <Label htmlFor='notify-channel-enabled'>启用</Label>
                <Switch
                  id='notify-channel-enabled'
                  checked={draft.enabled}
                  onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
                />
              </div>
            </div>
            <DialogFooter className='gap-2'>
              <Button
                variant='outline'
                disabled={testMutation.isPending}
                onClick={() =>
                  testMutation.mutate({
                    id: draft.id,
                    type: draft.type,
                    name: draft.name,
                    params: draft.params,
                  })
                }
              >
                发送测试
              </Button>
              <Button
                disabled={saveMutation.isPending}
                onClick={() => saveMutation.mutate(draft)}
              >
                保存
              </Button>
            </DialogFooter>
          </DialogContent>
        )}
      </Dialog>
    </Card>
  )
}

// EventPicker 事件订阅多选。空数组 = 全部事件(勾上「全部」时清空)。
function EventPicker({
  groups,
  value,
  onChange,
  compact,
}: {
  groups: string[]
  value: string[]
  onChange: (next: string[]) => void
  compact?: boolean
}) {
  const [open, setOpen] = useState(!compact)
  const all = value.length === 0
  const toggle = (g: string, on: boolean) => {
    // 从「全部」切到具体勾选时,先展开成全部分组再去掉这一项
    const base = all ? groups : value
    const next = on ? [...base, g] : base.filter((x) => x !== g)
    onChange(
      next.length === groups.length
        ? []
        : groups.filter((x) => next.includes(x))
    )
  }
  if (compact && !open) {
    return (
      <button
        type='button'
        className='text-primary text-xs underline-offset-2 hover:underline'
        onClick={() => setOpen(true)}
      >
        选择订阅的事件
      </button>
    )
  }
  return (
    <div className={cn('space-y-2', compact && 'border-t pt-2')}>
      <div className='flex items-center gap-2'>
        <Checkbox
          id={`events-all-${compact ? 'c' : 'd'}`}
          checked={all}
          onCheckedChange={(c) => c === true && onChange([])}
        />
        <Label
          htmlFor={`events-all-${compact ? 'c' : 'd'}`}
          className='cursor-pointer text-sm'
        >
          全部已开启的事件
        </Label>
      </div>
      <div className='grid grid-cols-1 gap-1.5 sm:grid-cols-2'>
        {groups.map((g) => {
          const id = `event-${compact ? 'c' : 'd'}-${g}`
          return (
            <div key={g} className='flex items-center gap-2'>
              <Checkbox
                id={id}
                checked={all || value.includes(g)}
                onCheckedChange={(c) => toggle(g, c === true)}
              />
              <Label
                htmlFor={id}
                className='cursor-pointer text-xs font-normal'
              >
                {EVENT_LABELS[g] ?? g}
              </Label>
            </div>
          )
        })}
      </div>
    </div>
  )
}
