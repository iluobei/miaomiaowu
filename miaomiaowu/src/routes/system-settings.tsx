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
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { CircleHelp, RefreshCw } from 'lucide-react'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'
import { useSyncProxyGroupCategories } from '@/hooks/use-proxy-groups'

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
  const [useNewTemplateSystem, setUseNewTemplateSystem] = useState(true)
  const [enableProxyProvider, setEnableProxyProvider] = useState(false)
  const [proxyGroupsSourceUrl, setProxyGroupsSourceUrl] = useState('')
  const [clientCompatibilityMode, setClientCompatibilityMode] = useState(false)
  const [silentMode, setSilentMode] = useState(false)
  const [silentModeTimeout, setSilentModeTimeout] = useState(15)

  // Sync proxy group categories mutation
  const syncProxyGroupsMutation = useSyncProxyGroupCategories()

  const { data: userConfig, isLoading: loadingConfig } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as {
        force_sync_external: boolean
        match_rule: string
        sync_scope: string
        keep_node_name: boolean
        cache_expire_minutes: number
        sync_traffic: boolean
        enable_probe_binding: boolean
        enable_short_link: boolean
        use_new_template_system: boolean
        enable_proxy_provider: boolean
        proxy_groups_source_url: string
        client_compatibility_mode: boolean
        silent_mode: boolean
        silent_mode_timeout: number
      }
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
      setUseNewTemplateSystem(userConfig.use_new_template_system !== false) // 默认为 true
      setEnableProxyProvider(userConfig.enable_proxy_provider || false)
      setProxyGroupsSourceUrl(userConfig.proxy_groups_source_url || '')
      setClientCompatibilityMode(userConfig.client_compatibility_mode || false)
      setSilentMode(userConfig.silent_mode || false)
      setSilentModeTimeout(userConfig.silent_mode_timeout || 15)
    }
  }, [userConfig])

  const updateConfigMutation = useMutation({
    mutationFn: async (data: {
      force_sync_external: boolean
      match_rule: string
      sync_scope: string
      keep_node_name: boolean
      cache_expire_minutes: number
      sync_traffic: boolean
      enable_probe_binding: boolean
      enable_short_link: boolean
      use_new_template_system: boolean
      enable_proxy_provider: boolean
      proxy_groups_source_url: string
      client_compatibility_mode: boolean
      silent_mode: boolean
      silent_mode_timeout: number
    }) => {
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
      setUseNewTemplateSystem(variables.use_new_template_system)
      setEnableProxyProvider(variables.enable_proxy_provider)
      setProxyGroupsSourceUrl(variables.proxy_groups_source_url || '')
      setClientCompatibilityMode(variables.client_compatibility_mode)
      setSilentMode(variables.silent_mode)
      setSilentModeTimeout(variables.silent_mode_timeout)
      toast.success('设置已更新')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('更新设置失败')
    },
  })

  // 通用的更新配置方法
  const updateConfig = (updates: Partial<{
    force_sync_external: boolean
    match_rule: string
    sync_scope: string
    keep_node_name: boolean
    cache_expire_minutes: number
    sync_traffic: boolean
    enable_probe_binding: boolean
    enable_short_link: boolean
    use_new_template_system: boolean
    enable_proxy_provider: boolean
    proxy_groups_source_url: string
    client_compatibility_mode: boolean
    silent_mode: boolean
    silent_mode_timeout: number
  }>) => {
    updateConfigMutation.mutate({
      force_sync_external: forceSyncExternal,
      match_rule: matchRule,
      sync_scope: syncScope,
      keep_node_name: keepNodeName,
      cache_expire_minutes: cacheExpireMinutes,
      sync_traffic: syncTraffic,
      enable_probe_binding: enableProbeBinding,
      enable_short_link: enableShortLink,
      use_new_template_system: useNewTemplateSystem,
      enable_proxy_provider: enableProxyProvider,
      proxy_groups_source_url: proxyGroupsSourceUrl,
      client_compatibility_mode: clientCompatibilityMode,
      silent_mode: silentMode,
      silent_mode_timeout: silentModeTimeout,
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

                {/* 新模板系统 */}
                <div className='flex items-center justify-between rounded-lg border p-3'>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='use-new-template-system' className='cursor-pointer'>
                      新模板系统
                    </Label>
                    <Tooltip>
                      <TooltipTrigger asChild>
                        <CircleHelp className='h-4 w-4 text-muted-foreground cursor-help' />
                      </TooltipTrigger>
                      <TooltipContent side='top' className='max-w-xs'>
                        <p>开启后使用数据库模板（支持网页端管理），关闭后使用 rule_templates 目录下的模板文件。</p>
                      </TooltipContent>
                    </Tooltip>
                  </div>
                  <Switch
                    id='use-new-template-system'
                    checked={useNewTemplateSystem}
                    onCheckedChange={(checked) => updateConfig({ use_new_template_system: checked })}
                    disabled={loadingConfig || updateConfigMutation.isPending}
                  />
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
                    placeholder='https://example.com/proxy-groups.json'
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
