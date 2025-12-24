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
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { RadioGroup, RadioGroupItem } from '@/components/ui/radio-group'
import { Input } from '@/components/ui/input'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { useAuthStore } from '@/stores/auth-store'

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
      toast.success('设置已更新')
    },
    onError: (error) => {
      handleServerError(error)
      toast.error('更新设置失败')
    },
  })

  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <main className='mx-auto w-full max-w-4xl px-4 py-8 sm:px-6 pt-24'>
        <section className='space-y-2'>
          <h1 className='text-3xl font-semibold tracking-tight'>系统设置</h1>
          <p className='text-muted-foreground'>管理订阅同步和节点探针相关设置</p>
        </section>

        <div className='mt-8 space-y-6'>
          {/* 订阅同步设置 */}
          <Card>
            <CardHeader>
              <CardTitle>外部订阅同步设置</CardTitle>
              <CardDescription>配置外部订阅的同步行为</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex items-center justify-between space-x-2 pt-2'>
                <div className='flex-1 space-y-1'>
                  <Label htmlFor='sync-traffic' className='cursor-pointer'>
                    同步外部订阅流量信息
                  </Label>
                  <p className='text-sm text-muted-foreground'>
                    开启后，流量信息数据包含外部订阅的流量信息
                  </p>
                </div>
                <Switch
                  id='sync-traffic'
                  checked={syncTraffic}
                  onCheckedChange={(checked) => {
                    updateConfigMutation.mutate({
                      force_sync_external: forceSyncExternal,
                      match_rule: matchRule,
                      sync_scope: syncScope,
                      keep_node_name: keepNodeName,
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: checked,
                      enable_probe_binding: enableProbeBinding,
                      enable_short_link: enableShortLink,
                      use_new_template_system: useNewTemplateSystem,
                    })
                  }}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>
              <div className='flex items-center justify-between space-x-2 pt-2 border-t'>
                <div className='flex-1 space-y-1'>
                  <Label htmlFor='force-sync-external' className='cursor-pointer'>
                    外部订阅同步设置
                  </Label>
                  <p className='text-sm text-muted-foreground'>
                    开启后，从订阅链接获取订阅时将重新获取外部订阅链接的最新节点
                  </p>
                </div>
                <Switch
                  id='force-sync-external'
                  checked={forceSyncExternal}
                  onCheckedChange={(checked) => {
                    updateConfigMutation.mutate({
                      force_sync_external: checked,
                      match_rule: matchRule,
                      sync_scope: syncScope,
                      keep_node_name: keepNodeName,
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: syncTraffic,
                      enable_probe_binding: enableProbeBinding,
                      enable_short_link: enableShortLink,
                      use_new_template_system: useNewTemplateSystem,
                    })
                  }}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>

              {forceSyncExternal && (
                <>
                  <div className='space-y-3 pt-2 border-t'>
                    <div className='space-y-2'>
                      <Label>匹配规则</Label>
                      <RadioGroup
                        value={matchRule}
                        onValueChange={(value: 'node_name' | 'server_port' | 'type_server_port') => {
                          setMatchRule(value)
                          updateConfigMutation.mutate({
                            force_sync_external: forceSyncExternal,
                            match_rule: value,
                            sync_scope: syncScope,
                            keep_node_name: keepNodeName,
                            cache_expire_minutes: cacheExpireMinutes,
                            sync_traffic: syncTraffic,
                            enable_probe_binding: enableProbeBinding,
                            enable_short_link: enableShortLink,
                            use_new_template_system: useNewTemplateSystem,
                          })
                        }}
                        disabled={loadingConfig || updateConfigMutation.isPending}
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
                            服务器:端口 (server:port)
                          </Label>
                        </div>
                        <div className='flex items-center space-x-2'>
                          <RadioGroupItem value='type_server_port' id='match-type-server-port' />
                          <Label htmlFor='match-type-server-port' className='font-normal cursor-pointer'>
                            代理类型:服务器:端口 (type:server:port)
                          </Label>
                        </div>
                      </RadioGroup>
                      <p className='text-sm text-muted-foreground'>
                        {matchRule === 'node_name'
                          ? '根据节点名称匹配并更新节点信息'
                          : matchRule === 'server_port'
                            ? '根据服务器地址和端口匹配并更新节点信息，适用于节点名称会变更的情况'
                            : '根据代理类型、服务器地址和端口匹配并更新节点信息，适用于同一服务器有多种代理类型的情况'}
                      </p>
                    </div>

                    <div className='space-y-2 pt-2 border-t'>
                      <Label>同步范围</Label>
                      <RadioGroup
                        value={syncScope}
                        onValueChange={(value: 'saved_only' | 'all') => {
                          setSyncScope(value)
                          updateConfigMutation.mutate({
                            force_sync_external: forceSyncExternal,
                            match_rule: matchRule,
                            sync_scope: value,
                            keep_node_name: keepNodeName,
                            cache_expire_minutes: cacheExpireMinutes,
                            sync_traffic: syncTraffic,
                            enable_probe_binding: enableProbeBinding,
                            enable_short_link: enableShortLink,
                            use_new_template_system: useNewTemplateSystem,
                          })
                        }}
                        disabled={loadingConfig || updateConfigMutation.isPending}
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
                      <p className='text-sm text-muted-foreground'>
                        {syncScope === 'saved_only'
                          ? '只更新已保存到数据库的节点，新增节点不会自动保存'
                          : '同步外部订阅的所有节点，包括新增的节点'}
                      </p>
                    </div>

                    <div className='flex items-center justify-between space-x-2 pt-2 border-t'>
                      <div className='flex-1 space-y-1'>
                        <Label htmlFor='keep-node-name' className='cursor-pointer'>
                          保留当前节点名称
                        </Label>
                        <p className='text-sm text-muted-foreground'>
                          开启后，同步时保留数据库中的节点名称，不使用外部订阅的节点名称
                        </p>
                      </div>
                      <Switch
                        id='keep-node-name'
                        checked={keepNodeName}
                        onCheckedChange={(checked) => {
                          setKeepNodeName(checked)
                          updateConfigMutation.mutate({
                            force_sync_external: forceSyncExternal,
                            match_rule: matchRule,
                            sync_scope: syncScope,
                            keep_node_name: checked,
                            cache_expire_minutes: cacheExpireMinutes,
                            sync_traffic: syncTraffic,
                            enable_probe_binding: enableProbeBinding,
                            enable_short_link: enableShortLink,
                            use_new_template_system: useNewTemplateSystem,
                          })
                        }}
                        disabled={loadingConfig || updateConfigMutation.isPending}
                      />
                    </div>

                    <div className='space-y-2 pt-2 border-t'>
                      <Label htmlFor='cache-expire-minutes'>缓存过期时间（分钟）</Label>
                      <Input
                        id='cache-expire-minutes'
                        type='number'
                        min='0'
                        value={cacheExpireMinutes}
                        onChange={(e) => {
                          const value = parseInt(e.target.value) || 0
                          setCacheExpireMinutes(value)
                        }}
                        onBlur={() => {
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
                          })
                        }}
                        disabled={loadingConfig || updateConfigMutation.isPending}
                        placeholder='0'
                      />
                      <p className='text-sm text-muted-foreground'>
                        设置为0表示每次获取订阅时都重新拉取外部订阅节点。大于0时，只有距离上次同步时间超过设置的分钟数才会重新拉取
                      </p>
                      <p className='mt-2 text-sm font-semibold text-destructive'>注意!!! 每次都更新订阅会影响获取订阅接口的响应速度</p>
                    </div>
                  </div>
                </>
              )}
            </CardContent>
          </Card>

          {/* 节点探针设置 */}
          <Card>
            <CardHeader>
              <CardTitle>节点流量设置</CardTitle>
              <CardDescription>配置节点与探针服务器的绑定关系</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex items-center justify-between space-x-2 pt-2'>
                <div className='flex-1 space-y-1'>
                  <Label htmlFor='enable-probe-binding' className='cursor-pointer'>
                    节点探针服务器绑定
                  </Label>
                  <p className='text-sm text-muted-foreground'>
                    开启后，节点列表将显示探针按钮，可为节点绑定特定的探针服务器。流量统计将只汇总绑定节点的探针流量，适用与创建单个节点的订阅时，保证流量统计正确
                  </p>
                </div>
                <Switch
                  id='enable-probe-binding'
                  checked={enableProbeBinding}
                  onCheckedChange={(checked) => {
                    updateConfigMutation.mutate({
                      force_sync_external: forceSyncExternal,
                      match_rule: matchRule,
                      sync_scope: syncScope,
                      keep_node_name: keepNodeName,
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: syncTraffic,
                      enable_probe_binding: checked,
                      enable_short_link: enableShortLink,
                      use_new_template_system: useNewTemplateSystem,
                    })
                  }}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>
              {enableProbeBinding && (
                <div className='rounded-lg border bg-muted/40 p-4'>
                  <p className='text-sm text-muted-foreground'>
                    • 开启后，在节点列表的 IP 按钮旁会显示"探针"按钮
                    <br />
                    • 点击探针按钮可为节点选择绑定的探针服务器
                    <br />
                    • 流量统计只会汇总绑定了节点的探针服务器流量
                    <br />
                    • 关闭后，流量统计会汇总所有探针服务器的流量
                  </p>
                </div>
              )}
            </CardContent>
          </Card>

          {/* 短链接设置 */}
          <Card>
            <CardHeader>
              <CardTitle>短链接设置</CardTitle>
              <CardDescription>配置订阅链接的短链接功能</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex items-center justify-between space-x-2 pt-2'>
                <div className='flex-1 space-y-1'>
                  <Label htmlFor='enable-short-link' className='cursor-pointer'>
                    启用短链接
                  </Label>
                  <p className='text-sm text-muted-foreground'>
                    开启后，订阅链接页面将显示6位字符的短链接，响应与/api/clash/subscribe接口一致
                  </p>
                </div>
                <Switch
                  id='enable-short-link'
                  checked={enableShortLink}
                  onCheckedChange={(checked) => {
                    updateConfigMutation.mutate({
                      force_sync_external: forceSyncExternal,
                      match_rule: matchRule,
                      sync_scope: syncScope,
                      keep_node_name: keepNodeName,
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: syncTraffic,
                      enable_probe_binding: enableProbeBinding,
                      enable_short_link: checked,
                      use_new_template_system: useNewTemplateSystem,
                    })
                  }}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>
              {enableShortLink && (
                <div className='rounded-lg border bg-muted/40 p-4'>
                  <p className='text-sm text-muted-foreground'>
                    • 短链接格式：https://server:port/随机6位字符
                    <br />
                    • 可在个人设置页面重置短链接
                    <br />
                    • 重置Token时会同时重置短链接
                  </p>
                </div>
              )}
            </CardContent>
          </Card>

          {/* 模板系统设置 */}
          <Card>
            <CardHeader>
              <CardTitle>模板系统设置</CardTitle>
              <CardDescription>配置订阅生成使用的模板系统</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex items-center justify-between space-x-2 pt-2'>
                <div className='flex-1 space-y-1'>
                  <Label htmlFor='use-new-template-system' className='cursor-pointer'>
                    使用新模板系统
                  </Label>
                  <p className='text-sm text-muted-foreground'>
                    开启后使用数据库模板，关闭后使用 rule_templates 目录下的模板文件
                  </p>
                </div>
                <Switch
                  id='use-new-template-system'
                  checked={useNewTemplateSystem}
                  onCheckedChange={(checked) => {
                    updateConfigMutation.mutate({
                      force_sync_external: forceSyncExternal,
                      match_rule: matchRule,
                      sync_scope: syncScope,
                      keep_node_name: keepNodeName,
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: syncTraffic,
                      enable_probe_binding: enableProbeBinding,
                      enable_short_link: enableShortLink,
                      use_new_template_system: checked,
                    })
                  }}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>
              {!useNewTemplateSystem && (
                <div className='rounded-lg border bg-muted/40 p-4'>
                  <p className='text-sm text-muted-foreground'>
                    • 旧模板系统从 rule_templates 目录读取 YAML 模板文件
                    <br />
                    • 新模板系统使用数据库存储的模板，支持在网页端管理
                  </p>
                </div>
              )}
            </CardContent>
          </Card>
        </div>
      </main>
    </div>
  )
}
