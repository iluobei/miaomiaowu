import { useEffect, useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { toast } from 'sonner'
import { Topbar } from '@/components/layout/topbar'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { CircleHelp, RefreshCw, Settings } from 'lucide-react'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'
import { useSyncProxyGroupCategories } from '@/hooks/use-proxy-groups'

interface NotifyConfig {
  notify_enabled: boolean
  telegram_bot_token: string
  telegram_chat_id: string
  notify_subscribe_fetch: boolean
  notify_login: boolean
  notify_ip_ban: boolean
  notify_silent_mode: boolean
  notify_daily_traffic: boolean
  notify_expiry: boolean
  notify_daily_traffic_time: string
}

interface UserConfig {
  force_sync_external: boolean
  match_rule: string
  sync_scope: string
  keep_node_name: boolean
  cache_expire_minutes: number
  sync_traffic: boolean
  enable_probe_binding: boolean
  enable_short_link: boolean
  template_version: string
  enable_proxy_provider: boolean
  proxy_groups_source_url: string
  client_compatibility_mode: boolean
  silent_mode: boolean
  silent_mode_timeout: number
  node_name_filter: string
  append_sub_info: boolean
  enable_sub_info_nodes: boolean
  sub_info_expire_prefix: string
  sub_info_traffic_prefix: string
  enable_sub_traffic_header: boolean
  enable_override_scripts: boolean
  subscription_output_format: string
  login_rate_max_attempts: number
  login_rate_window: number
  login_rate_lock_duration: number
  brute_force_enabled: boolean
  brute_force_max_failures: number
  brute_force_window: number
  brute_force_block_duration: number
  sub_rate_limit_enabled: boolean
  sub_rate_limit_max: number
  sub_rate_limit_window: number
  skip_local_ip: boolean
  block_unknown_subscription_ua: boolean
}

export const Route = createFileRoute('/system-settings')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: SystemSettingsPage,
})

function SystemSettingsPage() {
  const queryClient = useQueryClient()
  const { auth } = useAuthStore()
  const [forceSyncExternal, setForceSyncExternal] = useState(false)
  const [matchRule, setMatchRule] = useState<'node_name' | 'server_port' | 'type_server_port'>('node_name')
  const [syncScope, setSyncScope] = useState<'saved_only' | 'all'>('saved_only')
  const [keepNodeName, setKeepNodeName] = useState(true)
  const [cacheExpireMinutes, setCacheExpireMinutes] = useState(0)
  const [syncTraffic, setSyncTraffic] = useState(false)
  const [enableProbeBinding, setEnableProbeBinding] = useState(false)
  const [enableShortLink, setEnableShortLink] = useState(false)
  const [templateVersion, setTemplateVersion] = useState<'v1' | 'v2' | 'v3'>('v2')
  const [enableProxyProvider, setEnableProxyProvider] = useState(false)
  const [proxyGroupsSourceUrl, setProxyGroupsSourceUrl] = useState('')
  const [clientCompatibilityMode, setClientCompatibilityMode] = useState(false)
  const [silentMode, setSilentMode] = useState(false)
  const [silentModeTimeout, setSilentModeTimeout] = useState(15)
  const [nodeNameFilter, setNodeNameFilter] = useState('剩余|流量|到期|订阅|时间|重置')
  const [appendSubInfo, setAppendSubInfo] = useState(false)
  const [enableSubInfoNodes, setEnableSubInfoNodes] = useState(false)
  const [subInfoExpirePrefix, setSubInfoExpirePrefix] = useState('📅过期时间')
  const [subInfoTrafficPrefix, setSubInfoTrafficPrefix] = useState('⌛剩余流量')
  const [enableSubTrafficHeader, setEnableSubTrafficHeader] = useState(true)
  const [enableOverrideScripts, setEnableOverrideScripts] = useState(false)
  const [subscriptionOutputFormat, setSubscriptionOutputFormat] = useState('yaml')

  // 安全配置
  const [loginRateMaxAttempts, setLoginRateMaxAttempts] = useState(5)
  const [loginRateWindow, setLoginRateWindow] = useState(60)
  const [loginRateLockDuration, setLoginRateLockDuration] = useState(60)
  const [bruteForceEnabled, setBruteForceEnabled] = useState(true)
  const [bruteForceMaxFailures, setBruteForceMaxFailures] = useState(5)
  const [bruteForceWindow, setBruteForceWindow] = useState(1440)
  const [bruteForceBlockDuration, setBruteForceBlockDuration] = useState(1440)
  const [subRateLimitEnabled, setSubRateLimitEnabled] = useState(true)
  const [subRateLimitMax, setSubRateLimitMax] = useState(30)
  const [subRateLimitWindow, setSubRateLimitWindow] = useState(120)
  const [skipLocalIP, setSkipLocalIP] = useState(true)
  const [blockUnknownSubUA, setBlockUnknownSubUA] = useState(false)

  // Notification config state
  const [notifyConfig, setNotifyConfig] = useState<NotifyConfig>({
    notify_enabled: false,
    telegram_bot_token: '',
    telegram_chat_id: '',
    notify_subscribe_fetch: true,
    notify_login: true,
    notify_ip_ban: true,
    notify_silent_mode: true,
    notify_daily_traffic: false,
    notify_expiry: true,
    notify_daily_traffic_time: '08:00',
  })

  // Sync proxy group categories mutation
  const syncProxyGroupsMutation = useSyncProxyGroupCategories()

  // Notify config query
  const { data: notifyConfigData } = useQuery({
    queryKey: ['notify-config'],
    queryFn: async () => {
      const response = await api.get('/api/admin/notify-config')
      return response.data as NotifyConfig
    },
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (notifyConfigData) {
      setNotifyConfig(notifyConfigData)
    }
  }, [notifyConfigData])

  const updateNotifyMutation = useMutation({
    mutationFn: async (data: NotifyConfig) => {
      await api.put('/api/admin/notify-config', data)
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['notify-config'] })
      setNotifyConfig(variables)
      toast.success('通知配置已更新')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('更新通知配置失败')
    },
  })

  const testNotifyMutation = useMutation({
    mutationFn: async () => {
      await api.post('/api/admin/notify-config/test')
    },
    onSuccess: () => {
      toast.success('测试通知已发送')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('发送测试通知失败')
    },
  })

  const saveNotifyConfig = (updates: Partial<NotifyConfig>) => {
    const newConfig = { ...notifyConfig, ...updates }
    setNotifyConfig(newConfig)
    updateNotifyMutation.mutate(newConfig)
  }

  const { data: userConfig, isLoading: loadingConfig } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as UserConfig
    },
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (userConfig) {
      setForceSyncExternal(userConfig.force_sync_external)
      setMatchRule(userConfig.match_rule as 'node_name' | 'server_port' | 'type_server_port')
      setSyncScope((userConfig.sync_scope as 'saved_only' | 'all') || 'saved_only')
      setKeepNodeName(userConfig.keep_node_name !== false) // 默认为 true
      setCacheExpireMinutes(userConfig.cache_expire_minutes)
      setSyncTraffic(userConfig.sync_traffic)
      setEnableProbeBinding(userConfig.enable_probe_binding || false)
      setEnableShortLink(userConfig.enable_short_link || false)
      setTemplateVersion((userConfig.template_version as 'v1' | 'v2' | 'v3') || 'v2')
      setEnableProxyProvider(userConfig.enable_proxy_provider || false)
      setProxyGroupsSourceUrl(userConfig.proxy_groups_source_url || '')
      setClientCompatibilityMode(userConfig.client_compatibility_mode || false)
      setSilentMode(userConfig.silent_mode || false)
      setSilentModeTimeout(userConfig.silent_mode_timeout || 15)
      setNodeNameFilter(userConfig.node_name_filter ?? '剩余|流量|到期|订阅|时间|重置')
      setAppendSubInfo(userConfig.append_sub_info || false)
      setEnableSubInfoNodes(userConfig.enable_sub_info_nodes || false)
      setSubInfoExpirePrefix(userConfig.sub_info_expire_prefix || '📅过期时间')
      setSubInfoTrafficPrefix(userConfig.sub_info_traffic_prefix || '⌛剩余流量')
      setEnableSubTrafficHeader(userConfig.enable_sub_traffic_header !== false)
      setEnableOverrideScripts(userConfig.enable_override_scripts || false)
      setSubscriptionOutputFormat(userConfig.subscription_output_format || 'yaml')
      setLoginRateMaxAttempts(userConfig.login_rate_max_attempts || 5)
      setLoginRateWindow(userConfig.login_rate_window || 60)
      setLoginRateLockDuration(userConfig.login_rate_lock_duration || 60)
      setBruteForceEnabled(userConfig.brute_force_enabled !== false)
      setBruteForceMaxFailures(userConfig.brute_force_max_failures || 5)
      setBruteForceWindow(userConfig.brute_force_window || 1440)
      setBruteForceBlockDuration(userConfig.brute_force_block_duration || 1440)
      setSubRateLimitEnabled(userConfig.sub_rate_limit_enabled !== false)
      setSubRateLimitMax(userConfig.sub_rate_limit_max || 30)
      setSubRateLimitWindow(userConfig.sub_rate_limit_window || 120)
      setSkipLocalIP(userConfig.skip_local_ip !== false)
      setBlockUnknownSubUA(userConfig.block_unknown_subscription_ua === true)
    }
  }, [userConfig])

  const updateConfigMutation = useMutation({
    mutationFn: async (data: UserConfig) => {
      await api.put('/api/user/config', data)
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['user-config'] })
      // 当短链接开关状态改变时，刷新订阅列表以更新链接显示
      if (variables.enable_short_link !== enableShortLink) {
        queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
      }
      setForceSyncExternal(variables.force_sync_external)
      setMatchRule(variables.match_rule as 'node_name' | 'server_port' | 'type_server_port')
      setSyncScope(variables.sync_scope as 'saved_only' | 'all')
      setKeepNodeName(variables.keep_node_name)
      setCacheExpireMinutes(variables.cache_expire_minutes)
      setSyncTraffic(variables.sync_traffic)
      setEnableProbeBinding(variables.enable_probe_binding)
      setEnableShortLink(variables.enable_short_link)
      setTemplateVersion(variables.template_version as 'v1' | 'v2' | 'v3')
      setEnableProxyProvider(variables.enable_proxy_provider)
      setProxyGroupsSourceUrl(variables.proxy_groups_source_url || '')
      setClientCompatibilityMode(variables.client_compatibility_mode)
      setSilentMode(variables.silent_mode)
      setSilentModeTimeout(variables.silent_mode_timeout)
      setNodeNameFilter(variables.node_name_filter)
      setAppendSubInfo(variables.append_sub_info)
      setEnableSubInfoNodes(variables.enable_sub_info_nodes)
      setSubInfoExpirePrefix(variables.sub_info_expire_prefix)
      setSubInfoTrafficPrefix(variables.sub_info_traffic_prefix)
      setEnableSubTrafficHeader(variables.enable_sub_traffic_header)
      setEnableOverrideScripts(variables.enable_override_scripts)
      setSubscriptionOutputFormat(variables.subscription_output_format || 'yaml')
      setLoginRateMaxAttempts(variables.login_rate_max_attempts)
      setLoginRateWindow(variables.login_rate_window)
      setLoginRateLockDuration(variables.login_rate_lock_duration)
      setBruteForceEnabled(variables.brute_force_enabled)
      setBruteForceMaxFailures(variables.brute_force_max_failures)
      setBruteForceWindow(variables.brute_force_window)
      setBruteForceBlockDuration(variables.brute_force_block_duration)
      setSubRateLimitEnabled(variables.sub_rate_limit_enabled)
      setSubRateLimitMax(variables.sub_rate_limit_max)
      setSubRateLimitWindow(variables.sub_rate_limit_window)
      setSkipLocalIP(variables.skip_local_ip)
      setBlockUnknownSubUA(variables.block_unknown_subscription_ua)
      toast.success('设置已更新')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('更新设置失败')
    },
  })

  // 通用的更新配置方法
  const updateConfig = (updates: Partial<UserConfig>) => {
    updateConfigMutation.mutate({
      force_sync_external: forceSyncExternal,
      match_rule: matchRule,
      sync_scope: syncScope,
      keep_node_name: keepNodeName,
      cache_expire_minutes: cacheExpireMinutes,
      sync_traffic: syncTraffic,
      enable_probe_binding: enableProbeBinding,
      enable_short_link: enableShortLink,
      template_version: templateVersion,
      enable_proxy_provider: enableProxyProvider,
      proxy_groups_source_url: proxyGroupsSourceUrl,
      client_compatibility_mode: clientCompatibilityMode,
      silent_mode: silentMode,
      silent_mode_timeout: silentModeTimeout,
      node_name_filter: nodeNameFilter,
      append_sub_info: appendSubInfo,
      enable_sub_info_nodes: enableSubInfoNodes,
      sub_info_expire_prefix: subInfoExpirePrefix,
      sub_info_traffic_prefix: subInfoTrafficPrefix,
      enable_sub_traffic_header: enableSubTrafficHeader,
      enable_override_scripts: enableOverrideScripts,
      subscription_output_format: subscriptionOutputFormat,
      login_rate_max_attempts: loginRateMaxAttempts,
      login_rate_window: loginRateWindow,
      login_rate_lock_duration: loginRateLockDuration,
      brute_force_enabled: bruteForceEnabled,
      brute_force_max_failures: bruteForceMaxFailures,
      brute_force_window: bruteForceWindow,
      brute_force_block_duration: bruteForceBlockDuration,
      sub_rate_limit_enabled: subRateLimitEnabled,
      sub_rate_limit_max: subRateLimitMax,
      sub_rate_limit_window: subRateLimitWindow,
      skip_local_ip: skipLocalIP,
      block_unknown_subscription_ua: blockUnknownSubUA,
      ...updates,
    })
  }

  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <main className='mx-auto w-full max-w-4xl px-4 py-8 sm:px-6 pt-24'>
        <section className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>系统设置</h1>
          <p className='text-muted-foreground'>管理订阅同步和功能开关</p>
        </section>

        <div className='mt-8 space-y-6'>
          {/* 外部订阅同步设置 */}
          <Card>
            <CardHeader className='pb-4'>
              <CardTitle>外部订阅同步设置</CardTitle>
              <CardDescription>配置外部订阅的同步行为</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex items-center justify-between'>
                <div className='flex items-center gap-2'>
                  <Label htmlFor='sync-traffic' className='cursor-pointer'>
                    同步外部订阅流量信息
                  </Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                    </TooltipTrigger>
                    <TooltipContent side='right' className='max-w-xs'>
                      <p>开启后，流量信息数据包含外部订阅的流量信息</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Switch
                  id='sync-traffic'
                  checked={syncTraffic}
                  onCheckedChange={(checked) => updateConfig({ sync_traffic: checked })}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>

              <div className='flex items-center justify-between'>
                <div className='flex items-center gap-2'>
                  <Label htmlFor='append-sub-info' className='cursor-pointer'>
                    节点名称追加订阅信息
                  </Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                    </TooltipTrigger>
                    <TooltipContent side='right' className='max-w-xs'>
                      <p>开启后，同步外部订阅时在节点名称后追加剩余流量和剩余天数，例如：节点名 398.22GB📊 26Days⏳</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Switch
                  id='append-sub-info'
                  checked={appendSubInfo}
                  onCheckedChange={(checked) => updateConfig({ append_sub_info: checked })}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>

              <div className='space-y-2 pt-3 border-t'>
                <div className='flex items-center gap-2'>
                  <Label htmlFor='node-name-filter'>节点名称过滤</Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                    </TooltipTrigger>
                    <TooltipContent side='right' className='max-w-xs'>
                      <p>使用正则表达式过滤节点名称，匹配的节点将被排除。留空则不过滤。</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Input
                  id='node-name-filter'
                  value={nodeNameFilter}
                  onChange={(e) => setNodeNameFilter(e.target.value)}
                  onBlur={() => updateConfig({ node_name_filter: nodeNameFilter })}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                  placeholder='剩余|流量|到期|订阅|时间|重置'
                />
                <p className='text-xs text-muted-foreground'>正则表达式，匹配的节点将在同步时被过滤掉</p>
              </div>

              <div className='flex items-center justify-between pt-3 border-t'>
                <div className='flex items-center gap-2'>
                  <Label htmlFor='force-sync-external' className='cursor-pointer'>
                    外部订阅同步设置
                  </Label>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                    </TooltipTrigger>
                    <TooltipContent side='right' className='max-w-xs'>
                      <p>开启后，从订阅链接获取订阅时将重新获取外部订阅链接的最新节点</p>
                    </TooltipContent>
                  </Tooltip>
                </div>
                <Switch
                  id='force-sync-external'
                  checked={forceSyncExternal}
                  onCheckedChange={(checked) => updateConfig({ force_sync_external: checked })}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>

              {forceSyncExternal && (
                <div className='space-y-4 pt-3 border-t bg-muted/30 -mx-6 px-6 py-4 rounded-b-lg'>
                  <div className='space-y-2'>
                    <Label>匹配规则</Label>
                    <RadioGroup
                      value={matchRule}
                      onValueChange={(value: 'node_name' | 'server_port' | 'type_server_port') => {
                        setMatchRule(value)
                        updateConfig({ match_rule: value })
                      }}
                      disabled={loadingConfig || updateConfigMutation.isPending}
                      className='flex flex-wrap gap-4'
                    >
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='node_name' id='match-node-name' />
                        <Label htmlFor='match-node-name' className='font-normal cursor-pointer'>
                          节点名称
                        </Label>
                      </div>
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='server_port' id='match-server-port' />
                        <Label htmlFor='match-server-port' className='font-normal cursor-pointer'>
                          服务器:端口
                        </Label>
                      </div>
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='type_server_port' id='match-type-server-port' />
                        <Label htmlFor='match-type-server-port' className='font-normal cursor-pointer'>
                          类型:服务器:端口
                        </Label>
                      </div>
                    </RadioGroup>
                  </div>

                  <div className='space-y-2 pt-3 border-t border-border/50'>
                    <Label>同步范围</Label>
                    <RadioGroup
                      value={syncScope}
                      onValueChange={(value: 'saved_only' | 'all') => {
                        setSyncScope(value)
                        updateConfig({ sync_scope: value })
                      }}
                      disabled={loadingConfig || updateConfigMutation.isPending}
                      className='flex flex-wrap gap-4'
                    >
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='saved_only' id='sync-saved-only' />
                        <Label htmlFor='sync-saved-only' className='font-normal cursor-pointer'>
                          仅同步已保存节点
                        </Label>
                      </div>
                      <div className='flex items-center space-x-2'>
                        <RadioGroupItem value='all' id='sync-all' />
                        <Label htmlFor='sync-all' className='font-normal cursor-pointer'>
                          同步所有节点
                        </Label>
                      </div>
                    </RadioGroup>
                  </div>

                  <div className='flex items-center justify-between pt-3 border-t border-border/50'>
                    <div className='flex items-center gap-2'>
                      <Label htmlFor='keep-node-name' className='cursor-pointer'>
                        保留当前节点名称
                      </Label>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                        </TooltipTrigger>
                        <TooltipContent side='right' className='max-w-xs'>
                          <p>开启后，同步时保留数据库中的节点名称，不使用外部订阅的节点名称</p>
                        </TooltipContent>
                      </Tooltip>
                    </div>
                    <Switch
                      id='keep-node-name'
                      checked={keepNodeName}
                      onCheckedChange={(checked) => {
                        setKeepNodeName(checked)
                        updateConfig({ keep_node_name: checked })
                      }}
                      disabled={loadingConfig || updateConfigMutation.isPending}
                    />
                  </div>

                  <div className='space-y-2 pt-3 border-t border-border/50'>
                    <div className='flex items-center gap-2'>
                      <Label htmlFor='cache-expire-minutes'>缓存过期时间（分钟）</Label>
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                        </TooltipTrigger>
                        <TooltipContent side='right' className='max-w-xs'>
                          <p>设置为0表示每次获取订阅时都重新拉取。大于0时，只有超过设置的分钟数才会重新拉取</p>
                        </TooltipContent>
                      </Tooltip>
                    </div>
                    <Input
                      id='cache-expire-minutes'
                      type='number'
                      min='0'
                      value={cacheExpireMinutes}
                      onChange={(e) => setCacheExpireMinutes(parseInt(e.target.value) || 0)}
                      onBlur={() => updateConfig({ cache_expire_minutes: cacheExpireMinutes })}
                      disabled={loadingConfig || updateConfigMutation.isPending}
                      placeholder='0'
                      className='w-32'
                    />
                    <p className='text-xs text-destructive'>注意：每次都更新订阅会影响获取订阅接口的响应速度</p>
                  </div>
                </div>
              )}
            </CardContent>
          </Card>

          {/* 功能开关 */}
          <Card>
            <CardHeader className='pb-4'>
              <CardTitle>功能开关</CardTitle>
              <CardDescription>管理系统功能的启用状态</CardDescription>
            </CardHeader>
            <CardContent>
              <div className='grid grid-cols-1 sm:grid-cols-2 gap-4'>
                {/* 节点探针服务器绑定 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='enable-probe-binding' className='cursor-pointer'>
                      探针服务器绑定
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后，节点列表将显示探针按钮，可为节点绑定特定的探针服务器。流量统计将只汇总绑定节点的探针流量。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='enable-probe-binding'
                    checked={enableProbeBinding}
                    onCheckedChange={(checked) => updateConfig({ enable_probe_binding: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>

                {/* 短链接 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='enable-short-link' className='cursor-pointer'>
                      启用短链接
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后，订阅链接页面将显示6位字符的短链接。可在个人设置页面重置短链接。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='enable-short-link'
                    checked={enableShortLink}
                    onCheckedChange={(checked) => updateConfig({ enable_short_link: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>

                {/* 模板版本选择 */}
                <div className='rounded-lg border p-3'>
                  <div className='flex items-center gap-2 mb-3'>
                    <Label className='font-medium'>模板版本</Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>v1：使用 rule_templates 目录下的文件模板</p>
                        <p>v2：使用数据库模板（通用后端，支持网页端管理）</p>
                        <p>v3：使用新版模板系统（类 mihomo 配置，支持可视化编辑）</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <div className='flex gap-2'>
                    {[
                      { value: 'v1', label: '旧 (v1)' },
                      { value: 'v2', label: '通用后端 (v2)' },
                      { value: 'v3', label: '新 (v3)' },
                    ].map((option) => (
                      <button
                        key={option.value}
                        type='button'
                        onClick={() => updateConfig({ template_version: option.value })}
                        disabled={loadingConfig || updateConfigMutation.isPending}
                        className={`flex items-center gap-2 px-3 py-1.5 rounded-md border text-sm transition-colors ${
                          templateVersion === option.value
                            ? 'bg-primary text-primary-foreground border-primary'
                            : 'bg-background hover:bg-muted border-border'
                        } disabled:opacity-50 disabled:cursor-not-allowed`}
                      >
                        <span className={`w-3 h-3 rounded-full border-2 flex items-center justify-center ${
                          templateVersion === option.value
                            ? 'border-primary-foreground'
                            : 'border-muted-foreground'
                        }`}>
                          {templateVersion === option.value && (
                            <span className='w-1.5 h-1.5 rounded-full bg-primary-foreground' />
                          )}
                        </span>
                        {option.label}
                      </button>
                    ))}
                  </div>
                </div>

                {/* 代理集合 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='enable-proxy-provider' className='cursor-pointer'>
                      启用代理集合
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>代理集合（Proxy Provider）允许从外部订阅动态加载节点。开启后可在订阅文件页面配置代理集合，并在编辑代理组时将代理集合拖入代理组。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='enable-proxy-provider'
                    checked={enableProxyProvider}
                    onCheckedChange={(checked) => updateConfig({ enable_proxy_provider: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>

                {/* 客户端兼容模式 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='client-compatibility-mode' className='cursor-pointer'>
                      客户端兼容模式
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>自动过滤不兼容的节点（如 WireGuard），仅记录日志不报错。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='client-compatibility-mode'
                    checked={clientCompatibilityMode}
                    onCheckedChange={(checked) => updateConfig({ client_compatibility_mode: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>

                {/* 覆写脚本 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='enable-override-scripts' className='cursor-pointer'>
                      覆写脚本
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后，覆写管理页面将显示覆写脚本功能，可使用 JavaScript 脚本修改订阅配置或节点属性。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='enable-override-scripts'
                    checked={enableOverrideScripts}
                    onCheckedChange={(checked) => updateConfig({ enable_override_scripts: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>

                {/* 通知推送 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Popover>
                      <PopoverTrigger asChild>
                        <Button variant='outline' size='icon' className='h-7 w-7'>
                          <Settings className='h-3.5 w-3.5' />
                        </Button>
                      </PopoverTrigger>
                      <PopoverContent className='w-80' side='bottom' align='start'>
                        <div className='space-y-4'>
                          <div className='space-y-2'>
                            <Label htmlFor='telegram-bot-token'>Bot Token</Label>
                            <Input
                              id='telegram-bot-token'
                              value={notifyConfig.telegram_bot_token}
                              onChange={(e) => setNotifyConfig({ ...notifyConfig, telegram_bot_token: e.target.value })}
                              onBlur={() => saveNotifyConfig({ telegram_bot_token: notifyConfig.telegram_bot_token })}
                              placeholder='123456:ABC-DEF...'
                            />
                          </div>
                          <div className='space-y-2'>
                            <Label htmlFor='telegram-chat-id'>Chat ID</Label>
                            <Input
                              id='telegram-chat-id'
                              value={notifyConfig.telegram_chat_id}
                              onChange={(e) => setNotifyConfig({ ...notifyConfig, telegram_chat_id: e.target.value })}
                              onBlur={() => saveNotifyConfig({ telegram_chat_id: notifyConfig.telegram_chat_id })}
                              placeholder='-1001234567890'
                            />
                          </div>
                          <Button
                            variant='outline'
                            size='sm'
                            className='w-full'
                            onClick={() => testNotifyMutation.mutate()}
                            disabled={testNotifyMutation.isPending || !notifyConfig.telegram_bot_token || !notifyConfig.telegram_chat_id}
                          >
                            {testNotifyMutation.isPending ? '发送中...' : '发送测试通知'}
                          </Button>
                          <div className='border-t pt-3 space-y-2'>
                            <div className='flex items-center gap-2'>
                              <Checkbox
                                id='notify-subscribe-fetch'
                                checked={notifyConfig.notify_subscribe_fetch}
                                onCheckedChange={(checked) => saveNotifyConfig({ notify_subscribe_fetch: checked === true })}
                              />
                              <Label htmlFor='notify-subscribe-fetch' className='cursor-pointer text-sm'>订阅获取通知</Label>
                            </div>
                            <div className='flex items-center gap-2'>
                              <Checkbox
                                id='notify-login'
                                checked={notifyConfig.notify_login}
                                onCheckedChange={(checked) => saveNotifyConfig({ notify_login: checked === true })}
                              />
                              <Label htmlFor='notify-login' className='cursor-pointer text-sm'>登录通知</Label>
                            </div>
                            <div className='flex items-center gap-2'>
                              <Checkbox
                                id='notify-ip-ban'
                                checked={notifyConfig.notify_ip_ban}
                                onCheckedChange={(checked) => saveNotifyConfig({ notify_ip_ban: checked === true })}
                              />
                              <Label htmlFor='notify-ip-ban' className='cursor-pointer text-sm'>IP 封禁通知</Label>
                            </div>
                            <div className='flex items-center gap-2'>
                              <Checkbox
                                id='notify-silent-mode'
                                checked={notifyConfig.notify_silent_mode}
                                onCheckedChange={(checked) => saveNotifyConfig({ notify_silent_mode: checked === true })}
                              />
                              <Label htmlFor='notify-silent-mode' className='cursor-pointer text-sm'>静默模式通知</Label>
                            </div>
                            <div className='flex items-center gap-2'>
                              <Checkbox
                                id='notify-expiry'
                                checked={notifyConfig.notify_expiry}
                                onCheckedChange={(checked) => saveNotifyConfig({ notify_expiry: checked === true })}
                              />
                              <Label htmlFor='notify-expiry' className='cursor-pointer text-sm'>订阅到期通知</Label>
                            </div>
                            <div className='flex items-center gap-2'>
                              <Checkbox
                                id='notify-daily-traffic'
                                checked={notifyConfig.notify_daily_traffic}
                                onCheckedChange={(checked) => saveNotifyConfig({ notify_daily_traffic: checked === true })}
                              />
                              <Label htmlFor='notify-daily-traffic' className='cursor-pointer text-sm'>每日流量通知</Label>
                              {notifyConfig.notify_daily_traffic && (
                                <Input
                                  type='time'
                                  value={notifyConfig.notify_daily_traffic_time}
                                  onChange={(e) => setNotifyConfig({ ...notifyConfig, notify_daily_traffic_time: e.target.value })}
                                  onBlur={() => saveNotifyConfig({ notify_daily_traffic_time: notifyConfig.notify_daily_traffic_time })}
                                  className='h-7 w-24 text-xs'
                                />
                              )}
                            </div>
                          </div>
                        </div>
                      </PopoverContent>
                    </Popover>
                    <Label htmlFor='notify-enabled' className='cursor-pointer'>
                      通知推送
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后，系统会通过 Telegram 发送关键事件通知。点击配置按钮设置 Bot Token 和通知类型。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='notify-enabled'
                    checked={notifyConfig.notify_enabled}
                    onCheckedChange={(checked) => saveNotifyConfig({ notify_enabled: checked })}
                    disabled={updateNotifyMutation.isPending}
                  />
                </div>

                {/* 静默模式 */}
                <div className='flex items-center justify-between rounded-lg border border-orange-200 bg-orange-50 p-3 dark:border-orange-900 dark:bg-orange-950'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='silent-mode' className='cursor-pointer'>
                      静默模式
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后服务响应返回 404，获取一次订阅后恢复访问 {silentModeTimeout} 分钟。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='silent-mode'
                    checked={silentMode}
                    onCheckedChange={(checked) => updateConfig({ silent_mode: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>

                {/* 订阅响应头流量信息 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='enable-sub-traffic-header' className='cursor-pointer'>
                      订阅响应头流量信息
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后，获取订阅时读取探针和外部订阅流量数据，并在响应头中写入 subscription-userinfo 信息。关闭后跳过流量读取，不写入流量响应头。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='enable-sub-traffic-header'
                    checked={enableSubTrafficHeader}
                    onCheckedChange={(checked) => updateConfig({ enable_sub_traffic_header: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>

                {/* 订阅序列化格式 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label className='cursor-default'>订阅序列化格式</Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>选择 Clash 订阅的输出格式。默认 YAML，选择 JSON 后订阅将以 JSON 格式输出。仅影响 Clash 格式订阅，不影响其他客户端格式（Surge、Sing-Box 等）。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <div className='flex gap-1'>
                    {[
                      { value: 'yaml', label: 'YAML' },
                      { value: 'json', label: 'JSON' },
                    ].map((opt) => (
                      <button
                        key={opt.value}
                        type='button'
                        onClick={() => updateConfig({ subscription_output_format: opt.value })}
                        disabled={loadingConfig || updateConfigMutation.isPending}
                        className={`px-3 py-1 text-xs border rounded-md transition-colors ${
                          subscriptionOutputFormat === opt.value
                            ? 'bg-primary text-primary-foreground border-primary'
                            : 'bg-background hover:bg-muted border-border'
                        }`}
                      >
                        {opt.label}
                      </button>
                    ))}
                  </div>
                </div>
              </div>

              {/* 静默模式超时设置 */}
              {silentMode && (
                <div className='mt-4 space-y-2 rounded-lg border border-orange-200 bg-orange-50 p-3 dark:border-orange-900 dark:bg-orange-950'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='silent-mode-timeout'>恢复访问时长（分钟）</Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>用户获取订阅后，服务器恢复访问的时长。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Input
                    id='silent-mode-timeout'
                    type='number'
                    min={1}
                    max={1440}
                    value={silentModeTimeout}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                    onChange={(e) => setSilentModeTimeout(parseInt(e.target.value) || 15)}
                    onBlur={() => updateConfig({ silent_mode_timeout: silentModeTimeout })}
                    className='max-w-32'
                  />
                </div>
              )}

              {/* 订阅信息节点 */}
              <div className='mt-4 space-y-3 rounded-lg border p-4'>
                <div className='flex items-center justify-between'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='enable-sub-info-nodes' className='cursor-pointer font-medium'>
                      订阅信息节点
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后，订阅输出时在节点列表顶部添加过期时间和剩余流量信息节点。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='enable-sub-info-nodes'
                    checked={enableSubInfoNodes}
                    onCheckedChange={(checked) => updateConfig({ enable_sub_info_nodes: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
                </div>
                {enableSubInfoNodes && (
                  <div className='grid grid-cols-2 gap-3 pt-3 border-t'>
                    <div className='space-y-2'>
                      <Label htmlFor='sub-info-expire-prefix'>过期时间前缀</Label>
                      <Input
                        id='sub-info-expire-prefix'
                        value={subInfoExpirePrefix}
                        onChange={(e) => setSubInfoExpirePrefix(e.target.value)}
                        onBlur={() => updateConfig({ sub_info_expire_prefix: subInfoExpirePrefix })}
                        disabled={loadingConfig || updateConfigMutation.isPending}
                        placeholder='📅过期时间'
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor='sub-info-traffic-prefix'>剩余流量前缀</Label>
                      <Input
                        id='sub-info-traffic-prefix'
                        value={subInfoTrafficPrefix}
                        onChange={(e) => setSubInfoTrafficPrefix(e.target.value)}
                        onBlur={() => updateConfig({ sub_info_traffic_prefix: subInfoTrafficPrefix })}
                        disabled={loadingConfig || updateConfigMutation.isPending}
                        placeholder='⌛剩余流量'
                      />
                    </div>
                  </div>
                )}
              </div>
            </CardContent>
          </Card>

          {/* 安全配置 */}
          <Card>
            <CardHeader className='pb-4'>
              <CardTitle>安全配置</CardTitle>
              <CardDescription>配置登录保护、暴力探测封禁和订阅频率限制</CardDescription>
            </CardHeader>
            <CardContent className='space-y-6'>
              {/* 不封禁本地 IP */}
              <div className='flex items-start justify-between gap-4'>
                <div className='flex-1'>
                  <h4 className='text-sm font-medium'>不封禁本地 IP</h4>
                  <p className='text-xs text-muted-foreground mt-1'>
                    反代/Docker 场景下，若上游未传 X-Forwarded-For，主控可能将所有用户视作同一本机 IP — 一次封禁会让所有人连不上。开启后，loopback / 内网 / 私有网段 IP 跳过封禁与频率限制（登录账户维度仍生效）。
                  </p>
                </div>
                <Switch checked={skipLocalIP} onCheckedChange={(v) => { setSkipLocalIP(v); updateConfig({ skip_local_ip: v }) }} disabled={loadingConfig || updateConfigMutation.isPending} />
              </div>

              <hr className='border-border/50' />

              <div className='flex items-start justify-between gap-4'>
                <div className='flex-1'>
                  <h4 className='text-sm font-medium'>拦截未知订阅客户端</h4>
                  <p className='text-xs text-muted-foreground mt-1'>
                    仅允许 Clash、Stash、Loon、Quantumult X、Surge、sing-box、v2ray 等已识别客户端获取订阅，避免浏览器预览和爬虫误触。
                  </p>
                </div>
                <Switch checked={blockUnknownSubUA} onCheckedChange={(v) => { setBlockUnknownSubUA(v); updateConfig({ block_unknown_subscription_ua: v }) }} disabled={loadingConfig || updateConfigMutation.isPending} />
              </div>

              <hr className='border-border/50' />

              <TurnstileSettings />

              <hr className='border-border/50' />

              {/* 登录保护 */}
              <div className='space-y-3'>
                <h4 className='text-sm font-medium'>登录保护</h4>
                <p className='text-xs text-muted-foreground'>限制登录失败次数，防止暴力破解密码</p>
                <div className='grid grid-cols-1 sm:grid-cols-3 gap-3'>
                  <div className='space-y-1'>
                    <Label className='text-xs'>最大尝试次数</Label>
                    <Input type='number' min={1} value={loginRateMaxAttempts} onChange={(e) => setLoginRateMaxAttempts(Number(e.target.value) || 5)} disabled={loadingConfig || updateConfigMutation.isPending} />
                  </div>
                  <div className='space-y-1'>
                    <Label className='text-xs'>时间窗口 (分钟)</Label>
                    <Input type='number' min={1} value={loginRateWindow} onChange={(e) => setLoginRateWindow(Number(e.target.value) || 60)} disabled={loadingConfig || updateConfigMutation.isPending} />
                  </div>
                  <div className='space-y-1'>
                    <Label className='text-xs'>锁定时长 (分钟)</Label>
                    <Input type='number' min={1} value={loginRateLockDuration} onChange={(e) => setLoginRateLockDuration(Number(e.target.value) || 60)} disabled={loadingConfig || updateConfigMutation.isPending} />
                  </div>
                </div>
              </div>

              <hr className='border-border/50' />

              {/* 订阅暴力探测防护 */}
              <div className='space-y-3'>
                <div className='flex items-center justify-between'>
                  <div>
                    <h4 className='text-sm font-medium'>订阅暴力探测防护</h4>
                    <p className='text-xs text-muted-foreground'>访问不存在的订阅链接多次后封禁 IP</p>
                  </div>
                  <Switch checked={bruteForceEnabled} onCheckedChange={(v) => { setBruteForceEnabled(v); updateConfig({ brute_force_enabled: v }) }} disabled={loadingConfig || updateConfigMutation.isPending} />
                </div>
                {bruteForceEnabled && (
                  <div className='grid grid-cols-1 sm:grid-cols-3 gap-3'>
                    <div className='space-y-1'>
                      <Label className='text-xs'>最大失败次数</Label>
                      <Input type='number' min={1} value={bruteForceMaxFailures} onChange={(e) => setBruteForceMaxFailures(Number(e.target.value) || 5)} disabled={loadingConfig || updateConfigMutation.isPending} />
                    </div>
                    <div className='space-y-1'>
                      <Label className='text-xs'>统计窗口 (分钟)</Label>
                      <Input type='number' min={1} value={bruteForceWindow} onChange={(e) => setBruteForceWindow(Number(e.target.value) || 1440)} disabled={loadingConfig || updateConfigMutation.isPending} />
                    </div>
                    <div className='space-y-1'>
                      <Label className='text-xs'>封禁时长 (分钟)</Label>
                      <Input type='number' min={1} value={bruteForceBlockDuration} onChange={(e) => setBruteForceBlockDuration(Number(e.target.value) || 1440)} disabled={loadingConfig || updateConfigMutation.isPending} />
                    </div>
                  </div>
                )}
              </div>

              <hr className='border-border/50' />

              {/* 订阅频率限制 */}
              <div className='space-y-3'>
                <div className='flex items-center justify-between'>
                  <div>
                    <h4 className='text-sm font-medium'>订阅频率限制</h4>
                    <p className='text-xs text-muted-foreground'>限制每个 IP 获取订阅的频率，防止枚举和抓取</p>
                  </div>
                  <Switch checked={subRateLimitEnabled} onCheckedChange={(v) => { setSubRateLimitEnabled(v); updateConfig({ sub_rate_limit_enabled: v }) }} disabled={loadingConfig || updateConfigMutation.isPending} />
                </div>
                {subRateLimitEnabled && (
                  <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
                    <div className='space-y-1'>
                      <Label className='text-xs'>最大请求次数</Label>
                      <Input type='number' min={1} value={subRateLimitMax} onChange={(e) => setSubRateLimitMax(Number(e.target.value) || 30)} disabled={loadingConfig || updateConfigMutation.isPending} />
                    </div>
                    <div className='space-y-1'>
                      <Label className='text-xs'>时间窗口 (分钟)</Label>
                      <Input type='number' min={1} value={subRateLimitWindow} onChange={(e) => setSubRateLimitWindow(Number(e.target.value) || 120)} disabled={loadingConfig || updateConfigMutation.isPending} />
                    </div>
                  </div>
                )}
              </div>

              <Button
                className='w-full sm:w-auto'
                onClick={() => updateConfig({
                  login_rate_max_attempts: loginRateMaxAttempts,
                  login_rate_window: loginRateWindow,
                  login_rate_lock_duration: loginRateLockDuration,
                  brute_force_enabled: bruteForceEnabled,
                  brute_force_max_failures: bruteForceMaxFailures,
                  brute_force_window: bruteForceWindow,
                  brute_force_block_duration: bruteForceBlockDuration,
                  sub_rate_limit_enabled: subRateLimitEnabled,
                  sub_rate_limit_max: subRateLimitMax,
                  sub_rate_limit_window: subRateLimitWindow,
                  skip_local_ip: skipLocalIP,
                  block_unknown_subscription_ua: blockUnknownSubUA,
                })}
                disabled={loadingConfig || updateConfigMutation.isPending}
              >
                保存安全配置
              </Button>
            </CardContent>
          </Card>

          {/* 代理组配置同步 */}
          <Card>
            <CardHeader className='pb-4'>
              <CardTitle>代理组配置同步</CardTitle>
              <CardDescription>从远程同步最新的预设代理组配置</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex flex-col gap-3'>
                <p className='text-sm text-muted-foreground'>
                  代理组配置包含常用规则分类和对应的 rule-providers 设置。同步后将更新生成订阅页面的规则选择器和预置代理组。
                </p>
                <div className='space-y-2'>
                  <Label htmlFor='proxy-groups-source-url'>远程配置地址</Label>
                  <Input
                    id='proxy-groups-source-url'
                    value={proxyGroupsSourceUrl}
                    placeholder='https://raw.githubusercontent.com/iluobei/miaomiaowu/refs/heads/main/proxy_groups/proxy_groups.json'
                    disabled={loadingConfig || updateConfigMutation.isPending}
                    onChange={(e) => setProxyGroupsSourceUrl(e.target.value)}
                    onBlur={() => {
                      const trimmed = proxyGroupsSourceUrl.trim()
                      setProxyGroupsSourceUrl(trimmed)
                      updateConfig({ proxy_groups_source_url: trimmed })
                    }}
                  />
                  <p className='text-xs text-muted-foreground'>留空使用系统默认地址或环境变量配置</p>
                </div>
                <Button
                  onClick={() => {
                    const override = proxyGroupsSourceUrl.trim() || undefined
                    syncProxyGroupsMutation.mutate(override, {
                      onSuccess: (data) => {
                        toast.success(data.message || '代理组配置同步成功')
                      },
                      onError: (error) => {
                        handleServerError(error)
                      },
                    })
                  }}
                  disabled={syncProxyGroupsMutation.isPending}
                  className='w-full sm:w-auto'
                >
                  {syncProxyGroupsMutation.isPending ? (
                    <>
                      <RefreshCw className='mr-2 h-4 w-4 animate-spin' />
                      同步中...
                    </>
                  ) : (
                    <>
                      <RefreshCw className='mr-2 h-4 w-4' />
                      同步代理组配置
                    </>
                  )}
                </Button>
                {syncProxyGroupsMutation.isSuccess && (
                  <p className='text-sm text-green-600 dark:text-green-400'>
                    ✓ 同步成功，配置已更新
                  </p>
                )}
              </div>
            </CardContent>
          </Card>
        </div>
      </main>
    </div>
  )
}

function TurnstileSettings() {
  const [siteKey, setSiteKey] = useState('')
  const [secretKey, setSecretKey] = useState('')
  const settings = useQuery({
    queryKey: ['turnstile-settings'],
    queryFn: async () => (await api.get('/api/admin/security/turnstile')).data as {
      site_key: string
      secret_key: string
      enabled: boolean
    },
  })
  useEffect(() => {
    if (!settings.data) return
    setSiteKey(settings.data.site_key ?? '')
    setSecretKey(settings.data.secret_key ?? '')
  }, [settings.data])
  const save = useMutation({
    mutationFn: async () => api.put('/api/admin/security/turnstile', {
      site_key: siteKey.trim(),
      secret_key: secretKey.trim(),
    }),
    onSuccess: () => {
      settings.refetch()
      toast.success('Turnstile 设置已保存')
    },
    onError: handleServerError,
  })
  return (
    <div className='space-y-3'>
      <div className='flex items-center justify-between gap-3'>
        <div>
          <h4 className='text-sm font-medium'>Cloudflare Turnstile</h4>
          <p className='mt-1 text-xs text-muted-foreground'>Site Key 和 Secret Key 都填写后，登录页自动启用人机验证；两项留空即关闭。</p>
        </div>
        <span className={`text-xs ${settings.data?.enabled ? 'text-green-600' : 'text-muted-foreground'}`}>
          {settings.data?.enabled ? '已启用' : '未启用'}
        </span>
      </div>
      <div className='grid grid-cols-1 gap-3 sm:grid-cols-2'>
        <div className='space-y-1'>
          <Label htmlFor='turnstile-site-key' className='text-xs'>Site Key</Label>
          <Input id='turnstile-site-key' value={siteKey} onChange={(event) => setSiteKey(event.target.value)} placeholder='0x4AAAA...' />
        </div>
        <div className='space-y-1'>
          <Label htmlFor='turnstile-secret-key' className='text-xs'>Secret Key</Label>
          <Input id='turnstile-secret-key' type='password' value={secretKey} onChange={(event) => setSecretKey(event.target.value)} placeholder='0x4AAAA...' />
        </div>
      </div>
      <Button type='button' variant='outline' onClick={() => save.mutate()} disabled={settings.isLoading || save.isPending}>
        {save.isPending ? '保存中...' : '保存 Turnstile 设置'}
      </Button>
    </div>
  )
}
