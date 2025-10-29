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
  const [matchRule, setMatchRule] = useState<'node_name' | 'server_port'>('node_name')
  const [cacheExpireMinutes, setCacheExpireMinutes] = useState(0)
  const [syncTraffic, setSyncTraffic] = useState(false)
  const [enableProbeBinding, setEnableProbeBinding] = useState(false)
  const [customRulesEnabled, setCustomRulesEnabled] = useState(false)

  const { data: userConfig, isLoading: loadingConfig } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as {
        force_sync_external: boolean
        match_rule: string
        cache_expire_minutes: number
        sync_traffic: boolean
        enable_probe_binding: boolean
        custom_rules_enabled: boolean
      }
    },
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  useEffect(() => {
    if (userConfig) {
      setForceSyncExternal(userConfig.force_sync_external)
      setMatchRule(userConfig.match_rule as 'node_name' | 'server_port')
      setCacheExpireMinutes(userConfig.cache_expire_minutes)
      setSyncTraffic(userConfig.sync_traffic)
      setEnableProbeBinding(userConfig.enable_probe_binding || false)
      setCustomRulesEnabled(userConfig.custom_rules_enabled || false)
    }
  }, [userConfig])

  const updateConfigMutation = useMutation({
    mutationFn: async (data: {
      force_sync_external: boolean
      match_rule: string
      cache_expire_minutes: number
      sync_traffic: boolean
      enable_probe_binding: boolean
      custom_rules_enabled: boolean
    }) => {
      await api.put('/api/user/config', data)
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['user-config'] })
      setForceSyncExternal(variables.force_sync_external)
      setMatchRule(variables.match_rule as 'node_name' | 'server_port')
      setCacheExpireMinutes(variables.cache_expire_minutes)
      setSyncTraffic(variables.sync_traffic)
      setEnableProbeBinding(variables.enable_probe_binding)
      setCustomRulesEnabled(variables.custom_rules_enabled)
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
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: checked,
                      enable_probe_binding: enableProbeBinding,
                      custom_rules_enabled: customRulesEnabled,
                    })
                  }}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>
              <div className='flex items-center justify-between space-x-2 pt-2 border-t'>
                <div className='flex-1 space-y-1'>
                  <Label htmlFor='force-sync-external' className='cursor-pointer'>
                    强制同步外部订阅
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
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: syncTraffic,
                      enable_probe_binding: enableProbeBinding,
                      custom_rules_enabled: customRulesEnabled,
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
                        onValueChange={(value: 'node_name' | 'server_port') => {
                          setMatchRule(value)
                          updateConfigMutation.mutate({
                            force_sync_external: forceSyncExternal,
                            match_rule: value,
                            cache_expire_minutes: cacheExpireMinutes,
                            sync_traffic: syncTraffic,
                            enable_probe_binding: enableProbeBinding,
                            custom_rules_enabled: customRulesEnabled,
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
                      </RadioGroup>
                      <p className='text-sm text-muted-foreground'>
                        {matchRule === 'node_name'
                          ? '根据节点名称匹配并更新节点信息'
                          : '根据服务器地址和端口匹配并更新节点信息，适用于节点名称会变更的情况'}
                      </p>
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
                            cache_expire_minutes: cacheExpireMinutes,
                            sync_traffic: syncTraffic,
                            enable_probe_binding: enableProbeBinding,
                            custom_rules_enabled: customRulesEnabled,
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
                    开启后，节点列表将显示探针按钮，可为节点绑定特定的探针服务器。流量统计将只汇总绑定节点的探针流量
                  </p>
                </div>
                <Switch
                  id='enable-probe-binding'
                  checked={enableProbeBinding}
                  onCheckedChange={(checked) => {
                    updateConfigMutation.mutate({
                      force_sync_external: forceSyncExternal,
                      match_rule: matchRule,
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: syncTraffic,
                      enable_probe_binding: checked,
                      custom_rules_enabled: customRulesEnabled,
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

          {/* 自定义规则设置 */}
          <Card>
            <CardHeader>
              <CardTitle>自定义规则设置</CardTitle>
              <CardDescription>配置自定义 DNS、规则和规则集功能</CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              <div className='flex items-center justify-between space-x-2 pt-2'>
                <div className='flex-1 space-y-1'>
                  <Label htmlFor='custom-rules-enabled' className='cursor-pointer'>
                    启用自定义规则
                  </Label>
                  <p className='text-sm text-muted-foreground'>
                    开启后，可在自定义规则页面配置 DNS、规则和规则集，应用于订阅生成时
                  </p>
                </div>
                <Switch
                  id='custom-rules-enabled'
                  checked={customRulesEnabled}
                  onCheckedChange={(checked) => {
                    updateConfigMutation.mutate({
                      force_sync_external: forceSyncExternal,
                      match_rule: matchRule,
                      cache_expire_minutes: cacheExpireMinutes,
                      sync_traffic: syncTraffic,
                      enable_probe_binding: enableProbeBinding,
                      custom_rules_enabled: checked,
                    })
                  }}
                  disabled={loadingConfig || updateConfigMutation.isPending}
                />
              </div>
              {customRulesEnabled && (
                <div className='rounded-lg border bg-muted/40 p-4'>
                  <p className='text-sm text-muted-foreground'>
                    • 开启后，导航栏将显示"自定义规则"菜单项
                    <br />
                    • 可以创建 DNS 配置、规则列表和规则集提供商
                    <br />
                    • 生成订阅时会自动应用已启用的自定义规则
                    <br />
                    • DNS 规则会替换默认的 DNS 配置
                    <br />
                    • 普通规则和规则集可选择"替换"或"添加至头部"模式
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
