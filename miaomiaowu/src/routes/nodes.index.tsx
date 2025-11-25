// @ts-nocheck
import { useState, useMemo, useCallback } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Topbar } from '@/components/layout/topbar'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '@/components/ui/alert-dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { parseProxyUrl, toClashProxy, type ProxyNode, type ClashProxy } from '@/lib/proxy-parser'
import { Check, Pencil, X, Undo2, Activity, Eye, Copy } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import IpIcon from '@/assets/icons/ip.svg'
import ExchangeIcon from '@/assets/icons/exchange.svg'
import URI_Producer from '@/lib/substore/producers/uri'

// @ts-ignore - retained simple route definition
export const Route = createFileRoute('/nodes/')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: NodesPage,
})

type ParsedNode = {
  id: number
  raw_url: string
  node_name: string
  protocol: string
  parsed_config: string
  clash_config: string
  enabled: boolean
  tag: string
  original_server: string
  probe_server: string
  created_at: string
  updated_at: string
}

type TempNode = {
  id: string
  rawUrl: string
  name: string
  parsed: ProxyNode | null
  clash: ClashProxy | null
  enabled: boolean
  originalServer?: string // 保存原始服务器地址，用于回退
  tag?: string
  isSaved?: boolean
  dbId?: number
  dbNode?: ParsedNode
}

const PROTOCOL_COLORS: Record<string, string> = {
  vmess: 'bg-blue-500/10 text-blue-700 dark:text-blue-400',
  vless: 'bg-purple-500/10 text-purple-700 dark:text-purple-400',
  trojan: 'bg-red-500/10 text-red-700 dark:text-red-400',
  ss: 'bg-green-500/10 text-green-700 dark:text-green-400',
  socks5: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400',
  hysteria: 'bg-pink-500/10 text-pink-700 dark:text-pink-400',
  hysteria2: 'bg-indigo-500/10 text-indigo-700 dark:text-indigo-400',
  tuic: 'bg-cyan-500/10 text-cyan-700 dark:text-cyan-400',
  anytls: 'bg-teal-500/10 text-teal-700 dark:text-teal-400',
}

const PROTOCOLS = ['vmess', 'vless', 'trojan', 'ss', 'socks5', 'hysteria', 'hysteria2', 'tuic', 'anytls']

// 检查是否是IP地址（IPv4或IPv6）
function isIpAddress(hostname: string): boolean {
  if (!hostname) return false

  // 去除IPv6地址的方括号（如 [2a03:4000:6:d221::1]）
  const cleanHostname = hostname.replace(/^\[|\]$/g, '')

  // IPv4正则
  const ipv4Regex = /^(\d{1,3}\.){3}\d{1,3}$/
  // IPv6正则（简化版，匹配标准IPv6格式）
  const ipv6Regex = /^([0-9a-fA-F]{0,4}:){2,7}[0-9a-fA-F]{0,4}$/

  return ipv4Regex.test(cleanHostname) || ipv6Regex.test(cleanHostname)
}

// 重新排序代理配置对象，确保 name, type, server, port 在最前面
function reorderProxyConfig(config: ClashProxy): ClashProxy {
  if (!config || typeof config !== 'object') return config

  const ordered: any = {}
  const priorityKeys = ['name', 'type', 'server', 'port']

  // 先添加优先字段
  for (const key of priorityKeys) {
    if (key in config) {
      ordered[key] = config[key]
    }
  }

  // 再添加其他字段
  for (const [key, value] of Object.entries(config)) {
    if (!priorityKeys.includes(key)) {
      ordered[key] = value
    }
  }

  return ordered as ClashProxy
}

function NodesPage() {
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()
  const [input, setInput] = useState('')
  const [subscriptionUrl, setSubscriptionUrl] = useState('')
  const [userAgent, setUserAgent] = useState<string>('clash.meta')
  const [customUserAgent, setCustomUserAgent] = useState<string>('')
  const [tempNodes, setTempNodes] = useState<TempNode[]>([])
  const [selectedProtocol, setSelectedProtocol] = useState<string>('all')
  const [currentTag, setCurrentTag] = useState<string>('manual') // 'manual' 或 'subscription'
  const [tagFilter, setTagFilter] = useState<string>('all')
  const [editingNode, setEditingNode] = useState<{ id: string; value: string } | null>(null)
  const [resolvingIpFor, setResolvingIpFor] = useState<string | null>(null) // 正在解析IP的节点ID
  const [ipMenuState, setIpMenuState] = useState<{ nodeId: string; ips: string[] } | null>(null) // IP选择菜单状态
  const [probeBindingDialogOpen, setProbeBindingDialogOpen] = useState(false)
  const [selectedNodeForProbe, setSelectedNodeForProbe] = useState<ParsedNode | null>(null)
  const [exchangeDialogOpen, setExchangeDialogOpen] = useState(false)
  const [sourceNodeForExchange, setSourceNodeForExchange] = useState<ParsedNode | null>(null)
  const [exchangeFilterText, setExchangeFilterText] = useState<string>('')

  // 自定义标签状态
  const [manualTag, setManualTag] = useState<string>('手动输入')
  const [subscriptionTag, setSubscriptionTag] = useState<string>('')

  // 批量操作状态
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<number>>(new Set())
  const [batchTagDialogOpen, setBatchTagDialogOpen] = useState(false)
  const [batchTag, setBatchTag] = useState<string>('')

  // Clash 配置编辑状态
  const [clashDialogOpen, setClashDialogOpen] = useState(false)
  const [editingClashConfig, setEditingClashConfig] = useState<{ nodeId: number; config: string } | null>(null)
  const [clashConfigError, setClashConfigError] = useState<string>('')
  const [jsonErrorLines, setJsonErrorLines] = useState<number[]>([])

  // URI 复制状态
  const [uriDialogOpen, setUriDialogOpen] = useState(false)
  const [uriContent, setUriContent] = useState<string>('')

  // 优化的回调函数
  const handleUserAgentChange = useCallback((value: string) => {
    setUserAgent(value)
  }, [])

  const handleCustomUserAgentChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setCustomUserAgent(e.target.value)
  }, [])

  const handleSubscriptionUrlChange = useCallback((e: React.ChangeEvent<HTMLInputElement>) => {
    setSubscriptionUrl(e.target.value)
  }, [])

  // 获取用户配置
  const { data: userConfig } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as {
        force_sync_external: boolean
        match_rule: string
        cache_expire_minutes: number
        sync_traffic: boolean
        enable_probe_binding: boolean
      }
    },
    enabled: Boolean(auth.accessToken),
  })

  // 获取探针服务器列表
  const { data: probeConfigResponse, refetch: refetchProbeConfig } = useQuery({
    queryKey: ['probe-config'],
    queryFn: async () => {
      const response = await api.get('/api/admin/probe-config')
      return response.data as {
        config: {
          probe_type: string
          address: string
          servers: Array<{ id: number; name: string; server_id: string }>
        }
      }
    },
    enabled: false, // 手动触发，不自动执行
  })

  const probeConfig = probeConfigResponse?.config

  // 获取已保存的节点
  const { data: nodesData } = useQuery({
    queryKey: ['nodes'],
    queryFn: async () => {
      const response = await api.get('/api/admin/nodes')
      return response.data as { nodes: ParsedNode[] }
    },
    enabled: Boolean(auth.accessToken),
  })

  const savedNodes = useMemo(() => nodesData?.nodes ?? [], [nodesData?.nodes])

  const updateConfigName = (config, name) => {
    if (!config) return config
    try {
      const parsed = JSON.parse(config)
      if (parsed && typeof parsed === 'object') {
        parsed.name = name
      }
      return JSON.stringify(parsed)
    } catch (error) {
      return config
    }
  }

  const cloneProxyWithName = (proxy, name) => {
    if (!proxy || typeof proxy !== 'object') {
      return proxy
    }
    return {
      ...proxy,
      name,
    }
  }

  const updateNodeNameMutation = useMutation({
    mutationFn: async ({ id, name }: { id: number; name: string }) => {
      const target = savedNodes.find(n => n.id === id)
      if (!target) {
        throw new Error('未找到节点?')
      }
      const updatedParsedConfig = updateConfigName(target.parsed_config, name)
      const updatedClashConfig = updateConfigName(target.clash_config, name)
      const response = await api.put(`/api/admin/nodes/${id}`, {
        raw_url: target.raw_url,
        node_name: name,
        protocol: target.protocol,
        parsed_config: updatedParsedConfig,
        clash_config: updatedClashConfig,
        enabled: target.enabled,
        tag: target.tag,
      })
      return response.data
    },
    onSuccess: () => {
      toast.success('节点名称已更新')
      setEditingNode(null)
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '节点名称更新失败')
    },
  })

  // DNS解析IP地址
  const resolveIpMutation = useMutation({
    mutationFn: async (hostname: string) => {
      const response = await api.get(`/api/dns/resolve?hostname=${encodeURIComponent(hostname)}`)
      return response.data as { ips: string[] }
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'IP解析失败')
      setResolvingIpFor(null)
    },
  })

  // 更新节点服务器地址
  const updateNodeServerMutation = useMutation({
    mutationFn: async (payload: { nodeId: number; server: string }) => {
      const response = await api.put(`/api/admin/nodes/${payload.nodeId}/server`, { server: payload.server })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('服务器地址已更新')
      setResolvingIpFor(null)
      setIpMenuState(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '服务器地址更新失败')
      setResolvingIpFor(null)
    },
  })

  // 恢复节点原始域名
  const restoreNodeServerMutation = useMutation({
    mutationFn: async (nodeId: number) => {
      const response = await api.put(`/api/admin/nodes/${nodeId}/restore-server`)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('已恢复原始域名')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '恢复原始域名失败')
    },
  })

  // 更新节点 Clash 配置
  const updateClashConfigMutation = useMutation({
    mutationFn: async (payload: { nodeId: number; clashConfig: string }) => {
      const response = await api.put(`/api/admin/nodes/${payload.nodeId}/config`, {
        clash_config: payload.clashConfig
      })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('Clash 配置已更新')
      setClashDialogOpen(false)
      // 状态清理会在 onOpenChange 中自动处理
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || 'Clash 配置更新失败')
    },
  })

  // 更新节点探针绑定
  const updateProbeBindingMutation = useMutation({
    mutationFn: async (payload: { nodeId: number; probeServer: string }) => {
      const response = await api.put(`/api/admin/nodes/${payload.nodeId}/probe-binding`, {
        probe_server: payload.probeServer
      })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('探针绑定已更新')
      setProbeBindingDialogOpen(false)
      setSelectedNodeForProbe(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '探针绑定更新失败')
    },
  })

  // 处理 Clash 配置编辑（支持已保存节点和临时节点）
  const handleEditClashConfig = (node: ParsedNode | TempNode) => {
    // 对于已保存节点，使用 clash_config 字段
    // 对于临时节点，使用 clash 对象
    const clashConfig = 'clash_config' in node
      ? node.clash_config
      : (node.clash ? JSON.stringify(node.clash) : null)

    if (!clashConfig) return

    // 格式化 JSON 以便编辑
    try {
      const parsed = JSON.parse(clashConfig)
      const formatted = JSON.stringify(parsed, null, 2)
      setEditingClashConfig({
        nodeId: 'id' in node && typeof node.id === 'number' ? node.id : -1, // 临时节点使用 -1
        config: formatted
      })
    } catch {
      // 如果解析失败，使用原始字符串
      setEditingClashConfig({
        nodeId: 'id' in node && typeof node.id === 'number' ? node.id : -1,
        config: clashConfig
      })
    }
    setClashConfigError('')
    setJsonErrorLines([])
    setClashDialogOpen(true)
  }

  // 验证并保存 Clash 配置
  const handleSaveClashConfig = () => {
    if (!editingClashConfig) return

    try {
      // 验证 JSON 格式
      const parsedConfig = JSON.parse(editingClashConfig.config)

      // 检查必需字段
      if (!parsedConfig.name || !parsedConfig.type || !parsedConfig.server || !parsedConfig.port) {
        setClashConfigError('配置缺少必需字段: name, type, server, port')
        return
      }

      // 保存配置（压缩格式，不带空格和换行）
      updateClashConfigMutation.mutate({
        nodeId: editingClashConfig.nodeId,
        clashConfig: JSON.stringify(parsedConfig)
      })
    } catch (error) {
      setClashConfigError(`JSON 格式错误: ${error instanceof Error ? error.message : String(error)}`)
    }
  }

  // 处理配置文本变化，实时验证
  const handleClashConfigChange = (value: string) => {
    if (!editingClashConfig) return

    setEditingClashConfig({
      ...editingClashConfig,
      config: value
    })

    // 实时验证 JSON 格式
    try {
      JSON.parse(value)
      setClashConfigError('')
      setJsonErrorLines([])
    } catch (error) {
      const errorMsg = error instanceof Error ? error.message : String(error)
      setClashConfigError(`JSON 格式错误: ${errorMsg}`)

      // 尝试提取错误行号
      // JSON.parse 错误信息格式通常是 "Unexpected token ... in JSON at position ..."
      // 我们需要根据position计算行号
      if (error instanceof SyntaxError && errorMsg.includes('position')) {
        const match = errorMsg.match(/position (\d+)/)
        if (match) {
          const position = parseInt(match[1], 10)
          const lines = value.substring(0, position).split('\n')
          const errorLine = lines.length

          // 只有当错误是 "Expected ',' or '}'" 时，才同时标记错误行和上一行
          // 因为这种错误通常是上一行缺少逗号导致的
          const isMissingCommaError = errorMsg.includes("Expected ',' or '}'")
          const errorLines = isMissingCommaError && errorLine > 1
            ? [errorLine - 1, errorLine]
            : [errorLine]
          setJsonErrorLines(errorLines)
        }
      } else {
        setJsonErrorLines([])
      }
    }
  }

  // 复制 URI 到剪贴板
  const handleCopyUri = async (node: ParsedNode) => {
    if (!node.clash_config) return

    try {
      // 解析 Clash 配置
      const clashConfig = JSON.parse(node.clash_config)

      // 使用 URI producer 转换为 URI
      const producer = URI_Producer()
      const uri = producer.produce(clashConfig)

      // 尝试复制到剪贴板
      try {
        await navigator.clipboard.writeText(uri)
        toast.success('URI 已复制到剪贴板')
      } catch (clipboardError) {
        // 复制失败，显示手动复制对话框
        setUriContent(uri)
        setUriDialogOpen(true)
      }
    } catch (error) {
      toast.error('生成 URI 失败: ' + (error instanceof Error ? error.message : String(error)))
    }
  }

  // 处理IP解析
  const handleResolveIp = async (node: TempNode) => {
    if (!node.parsed?.server) return

    const nodeKey = node.isSaved ? String(node.dbId) : node.id
    setResolvingIpFor(nodeKey)

    try {
      const result = await resolveIpMutation.mutateAsync(node.parsed.server)

      if (result.ips.length === 0) {
        toast.error('未解析到IP地址')
        setResolvingIpFor(null)
        return
      }

      if (result.ips.length === 1) {
        // 只有一个IP，直接更新
        if (node.isSaved && node.dbId) {
          // 已保存的节点，调用API更新
          updateNodeServerMutation.mutate({
            nodeId: node.dbId,
            server: result.ips[0],
          })
        } else {
          // 未保存的节点，更新临时节点列表
          updateTempNodeServer(node.id, result.ips[0])
          setResolvingIpFor(null)
        }
      } else {
        // 多个IP，显示菜单让用户选择
        setIpMenuState({ nodeId: nodeKey, ips: result.ips })
        setResolvingIpFor(null)
      }
    } catch (error) {
      // Error already handled by mutation
    }
  }

  // 更新临时节点的服务器地址
  const updateTempNodeServer = (nodeId: string, server: string) => {
    setTempNodes(prev => prev.map(n => {
      if (n.id !== nodeId) return n

      // 如果还没有保存原始服务器地址，则保存当前的
      const originalServer = n.originalServer || n.parsed?.server

      // 更新 parsed 配置
      const updatedParsed = n.parsed ? { ...n.parsed, server } : n.parsed

      // 更新 clash 配置
      const updatedClash = n.clash ? { ...n.clash, server } : n.clash

      return {
        ...n,
        parsed: updatedParsed,
        clash: updatedClash,
        originalServer,
      }
    }))
    toast.success('服务器地址已更新')
  }

  // 恢复临时节点的原始服务器地址
  const restoreTempNodeServer = (nodeId: string) => {
    setTempNodes(prev => prev.map(n => {
      if (n.id !== nodeId || !n.originalServer) return n

      // 恢复到原始服务器地址
      const updatedParsed = n.parsed ? { ...n.parsed, server: n.originalServer } : n.parsed
      const updatedClash = n.clash ? { ...n.clash, server: n.originalServer } : n.clash

      return {
        ...n,
        parsed: updatedParsed,
        clash: updatedClash,
        originalServer: undefined, // 清除原始服务器地址标记
      }
    }))
    toast.success('已恢复原始服务器地址')
  }

  // 批量创建节点
  const batchCreateMutation = useMutation({
    mutationFn: async (nodes: TempNode[]) => {
      // 根据当前标签类型使用对应的自定义标签
      const tag = currentTag === 'manual'
        ? (manualTag.trim() || '手动输入')
        : (subscriptionTag.trim() || '订阅导入')

      const payload = nodes.map(n => ({
        raw_url: n.rawUrl,
        node_name: n.name || '未知',
        protocol: n.parsed?.type || 'unknown',
        parsed_config: n.parsed ? JSON.stringify(cloneProxyWithName(n.parsed, n.name)) : '',
        clash_config: n.clash ? JSON.stringify(cloneProxyWithName(n.clash, n.name)) : '',
        enabled: n.enabled,
        tag: tag,
      }))

      const response = await api.post('/api/admin/nodes/batch', { nodes: payload })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('节点保存成功')
      setInput('')
      setTempNodes([])
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '保存失败')
    },
  })

  // 切换节点启用状态
  const toggleMutation = useMutation({
    mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) => {
      const node = savedNodes.find(n => n.id === id)
      if (!node) return

      const response = await api.put(`/api/admin/nodes/${id}`, {
        raw_url: node.raw_url,
        node_name: node.node_name,
        protocol: node.protocol,
        parsed_config: node.parsed_config,
        clash_config: node.clash_config,
        enabled,
      })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '更新失败')
    },
  })

  // 删除节点
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/api/admin/nodes/${id}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('节点已删除')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '删除失败')
    },
  })

  // 清空所有节点
  const clearAllMutation = useMutation({
    mutationFn: async () => {
      await api.post('/api/admin/nodes/clear')
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('所有节点已清空')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '清空失败')
    },
  })

  // 批量更新节点标签
  const batchUpdateTagMutation = useMutation({
    mutationFn: async ({ nodeIds, tag }: { nodeIds: number[]; tag: string }) => {
      const promises = nodeIds.map((id) => {
        const node = savedNodes.find(n => n.id === id)
        if (!node) return Promise.resolve()

        return api.put(`/api/admin/nodes/${id}`, {
          raw_url: node.raw_url,
          node_name: node.node_name,
          protocol: node.protocol,
          parsed_config: node.parsed_config,
          clash_config: node.clash_config,
          enabled: node.enabled,
          tag: tag,
        })
      })
      await Promise.all(promises)
    },
    onSuccess: (_data, variables) => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success(`成功更新 ${variables.nodeIds.length} 个节点的标签`)
      setBatchTagDialogOpen(false)
      setSelectedNodeIds(new Set())
      setBatchTag('')
      setTagFilter('all') // 切换到全部标签
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '批量更新标签失败')
    },
  })

  // 创建链式代理节点
  const createRelayNodeMutation = useMutation({
    mutationFn: async ({ sourceNode, targetNode }: { sourceNode: ParsedNode; targetNode: ParsedNode }) => {
      // 解析源节点的 clash 配置
      let sourceClashConfig: ClashProxy
      try {
        sourceClashConfig = JSON.parse(sourceNode.clash_config)
      } catch (e) {
        throw new Error('源节点配置解析失败')
      }

      // 创建新的节点名称：源节点名称⇋目标节点名称
      const newNodeName = `${sourceNode.node_name}⇋${targetNode.node_name}`

      // 添加 dialer-proxy 属性
      const newClashConfig = {
        ...sourceClashConfig,
        name: newNodeName,
        'dialer-proxy': targetNode.node_name,
      }

      // 创建新节点
      const response = await api.post('/api/admin/nodes', {
        raw_url: sourceNode.raw_url, // 使用源节点的原始URL
        node_name: newNodeName,
        protocol: `${sourceNode.protocol}⇋${targetNode.protocol}`,
        parsed_config: JSON.stringify(newClashConfig), // 使用clash配置作为parsed配置
        clash_config: JSON.stringify(newClashConfig),
        enabled: true,
        tag: '链式代理',
        original_server: sourceNode.original_server,
        probe_server: sourceNode.probe_server || '',
      })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('链式代理节点创建成功')
      setExchangeDialogOpen(false)
      setSourceNodeForExchange(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '创建链式代理节点失败')
    },
  })

  // 从订阅获取节点
  const fetchSubscriptionMutation = useMutation({
    mutationFn: async ({ url, userAgent }: { url: string; userAgent: string }) => {
      const response = await api.post('/api/admin/nodes/fetch-subscription', {
        url,
        user_agent: userAgent
      })
      return response.data as { proxies: ClashProxy[]; count: number }
    },
    onSuccess: async (data, variables) => {
      // 将Clash节点转换为TempNode格式
      const parsed: TempNode[] = data.proxies.map((clashNode) => {
        // Clash节点已经是标准格式，直接作为ProxyNode和ClashProxy使用
        const proxyNode: ProxyNode = {
          name: clashNode.name || '未知',
          type: clashNode.type || 'unknown',
          server: clashNode.server || '',
          port: clashNode.port || 0,
          ...clashNode,
        }
        const name = proxyNode.name || '未知'
        const parsedProxy = cloneProxyWithName(proxyNode, name)
        const clashProxy = cloneProxyWithName(clashNode, name)

        // 提取服务器名称用于标签
        let defaultTag = '外部订阅'
        try {
          const urlObj = new URL(variables.url)
          defaultTag = urlObj.hostname || '外部订阅'
        } catch {
          // URL解析失败时使用默认标签
        }

        return {
          id: Math.random().toString(36).substring(7),
          rawUrl: variables.url, // 使用订阅链接地址
          name,
          parsed: parsedProxy,
          clash: clashProxy,
          enabled: true,
          tag: subscriptionTag.trim() || defaultTag, // 添加标签信息
        }
      })

      setTempNodes(parsed)
      setCurrentTag('subscription') // 订阅导入

      // 如果用户没有设置标签，自动使用服务器地址作为标签
      if (!subscriptionTag.trim()) {
        let serverName = '外部订阅'
        try {
          const urlObj = new URL(variables.url)
          serverName = urlObj.hostname || '外部订阅'
        } catch {
          // URL解析失败时使用默认标签
        }
        setSubscriptionTag(serverName)
      }

      toast.success(`成功导入 ${data.count} 个节点`)

      // 保存外部订阅链接
      try {
        // 从 URL 中提取名称（使用域名或者最后一部分）
        const urlObj = new URL(variables.url)
        const name = urlObj.hostname || '外部订阅'

        await api.post('/api/user/external-subscriptions', {
          name: name,
          url: variables.url,
          user_agent: variables.userAgent, // 保存 User-Agent
        })
      } catch (error) {
        // 如果保存失败（比如已经存在），忽略错误
        console.log('保存外部订阅链接失败（可能已存在）:', error)
      }
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '获取订阅失败')
    },
  })

  const handleParse = () => {
    const lines = input.split('\n').filter(line => line.trim())
    const parsed: TempNode[] = []

    for (const line of lines) {
      const trimmed = line.trim()
      if (!trimmed || !trimmed.includes('://')) continue
      const parsedNode = parseProxyUrl(trimmed)
      const clashNode = parsedNode ? toClashProxy(parsedNode) : null
      const name = parsedNode?.name || clashNode?.name || '未知'
      const normalizedParsed = cloneProxyWithName(parsedNode, name)
      const normalizedClash = cloneProxyWithName(clashNode, name)

      parsed.push({
        id: Math.random().toString(36).substring(7),
        rawUrl: trimmed,
        name,
        parsed: normalizedParsed,
        clash: normalizedClash,
        enabled: true,
        tag: manualTag.trim() || '手动输入', // 添加标签信息
      })
    }

    setTempNodes(parsed)
    setCurrentTag('manual') // 手动输入
  }

  const handleSave = () => {
    if (tempNodes.length === 0) {
      toast.error('没有可保存的节点')
      return
    }
    batchCreateMutation.mutate(tempNodes)
  }

  const handleToggle = (id: number) => {
    const node = savedNodes.find(n => n.id === id)
    if (node) {
      toggleMutation.mutate({ id, enabled: !node.enabled })
    }
  }

  const handleDelete = (id: number) => {
    deleteMutation.mutate(id)
  }

  const handleDeleteTemp = (id: string) => {
    setTempNodes(prev => prev.filter(node => node.id !== id))
    toast.success('已移除临时节点')
  }

  const handleNameEditStart = (node) => {
    setEditingNode({ id: node.id, value: node.name })
  }

  const handleNameEditChange = (value: string) => {
    setEditingNode(prev => (prev ? { ...prev, value } : prev))
  }

  const handleNameEditCancel = () => {
    setEditingNode(null)
  }

  const handleNameEditSubmit = (node) => {
    if (!editingNode) return
    const trimmed = editingNode.value.trim()
    if (!trimmed) {
      toast.error('节点名称不能为空')
      return
    }
    if (trimmed === node.name) {
      setEditingNode(null)
      return
    }

    if (node.isSaved) {
      updateNodeNameMutation.mutate({ id: node.dbId, name: trimmed })
      return
    }

    setTempNodes(prev =>
      prev.map(item => {
        if (item.id !== node.id) return item
        return {
          ...item,
          name: trimmed,
          parsed: cloneProxyWithName(item.parsed, trimmed),
          clash: cloneProxyWithName(item.clash, trimmed),
        }
      }),
    )
    toast.success('已更新临时节点名称')
    setEditingNode(null)
  }

  const handleClearAll = () => {
    clearAllMutation.mutate()
  }

  const handleFetchSubscription = () => {
    if (!subscriptionUrl.trim()) {
      toast.error('请输入订阅链接')
      return
    }

    // 确定使用哪个 User-Agent
    const finalUserAgent = userAgent === '手动输入' ? customUserAgent : userAgent

    if (userAgent === '手动输入' && !customUserAgent.trim()) {
      toast.error('请输入自定义 User-Agent')
      return
    }

    fetchSubscriptionMutation.mutate({
      url: subscriptionUrl,
      userAgent: finalUserAgent
    })
  }

  // 合并保存的节点和临时节点用于显示
  const displayNodes = useMemo(() => {
    // 将保存的节点转换为显示格式
    const saved = savedNodes.map(n => {
      let parsed: ProxyNode | null = null
      let clash: ClashProxy | null = null
      try {
        if (n.parsed_config) parsed = JSON.parse(n.parsed_config)
        if (n.clash_config) clash = JSON.parse(n.clash_config)
      } catch (e) {
        // 解析失败，保持 null
      }
      const displayName = (n.node_name && n.node_name.trim()) || parsed?.name || '未知'
      const parsedWithName = cloneProxyWithName(parsed, displayName)
      const clashWithName = cloneProxyWithName(clash, displayName)
      return {
        id: n.id.toString(),
        rawUrl: n.raw_url,
        name: displayName,
        parsed: parsedWithName,
        clash: clashWithName,
        enabled: n.enabled,
        tag: n.tag || '手动输入',
        isSaved: true,
        dbId: n.id,
        dbNode: n,
      }
    })

    // 临时节点
    const temp = tempNodes.map(n => ({
      ...n,
      parsed: cloneProxyWithName(n.parsed, n.name),
      clash: cloneProxyWithName(n.clash, n.name),
      isSaved: false,
      dbId: 0,
    }))

    return [...temp, ...saved]
  }, [savedNodes, tempNodes])

  const filteredNodes = useMemo(() => {
    let nodes = displayNodes

    // 按协议筛选
    if (selectedProtocol !== 'all') {
      nodes = nodes.filter(node => node.parsed?.type === selectedProtocol)
    }

    // 按标签筛选
    if (tagFilter !== 'all') {
      nodes = nodes.filter(node => node.tag === tagFilter)
    }

    return nodes
  }, [displayNodes, selectedProtocol, tagFilter])

  const protocolCounts = useMemo(() => {
    const counts: Record<string, number> = { all: displayNodes.length }
    for (const protocol of PROTOCOLS) {
      counts[protocol] = displayNodes.filter(n => n.parsed?.type === protocol).length
    }
    return counts
  }, [displayNodes])

  const tagCounts = useMemo(() => {
    const counts: Record<string, number> = { all: displayNodes.length }
    const tags = new Set<string>()
    displayNodes.forEach(node => {
      if (node.tag) {
        tags.add(node.tag)
        counts[node.tag] = (counts[node.tag] || 0) + 1
      }
    })
    return counts
  }, [displayNodes])

  // 提取所有唯一的标签
  const allUniqueTags = useMemo(() => {
    const tags = new Set<string>()
    savedNodes.forEach(node => {
      if (node.tag && node.tag.trim()) {
        tags.add(node.tag.trim())
      }
    })
    return Array.from(tags).sort()
  }, [savedNodes])

  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <main className='mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24'>
        <section className='space-y-4'>
          <div>
            <h1 className='text-3xl font-semibold tracking-tight'>节点管理</h1>
            <p className='text-muted-foreground mt-2'>
              输入代理节点信息，每行一个节点，支持 VMess、VLESS、Trojan、Shadowsocks、Hysteria、Socks、TUIC、AnyTLS 协议。
            </p>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>节点输入</CardTitle>
            </CardHeader>
            <CardContent>
              <Tabs defaultValue='manual' className='w-full'>
                <TabsList className='grid w-full grid-cols-2'>
                  <TabsTrigger value='manual'>手动输入</TabsTrigger>
                  <TabsTrigger value='subscription'>订阅导入</TabsTrigger>
                </TabsList>

                <TabsContent value='manual' className='space-y-4 mt-4'>
                  <Textarea
                    placeholder={`vmess://eyJwcyI6IuWPsOa5vualviIsImFkZCI6ImV4YW1wbGUuY29tIiwicG9ydCI6IjQ0MyIsImlkIjoidXVpZCIsImFpZCI6IjAiLCJzY3kiOiJhdXRvIiwibmV0Ijoid3MiLCJ0bHMiOiJ0bHMifQ==
vless://uuid@example.com:443?type=ws&security=tls&path=/websocket#VLESS节点
trojan://password@example.com:443?sni=example.com#Trojan节点
anytls://password@example.com:443/?sni=example.com&fp=chrome&alpn=h2#AnyTLS节点`}
                    value={input}
                    onChange={(e) => setInput(e.target.value)}
                    className='min-h-[200px] font-mono text-sm'
                  />
                  <div className='space-y-2'>
                    <Label htmlFor='manual-tag' className='text-sm font-medium'>
                      节点标签
                    </Label>
                    <Input
                      id='manual-tag'
                      placeholder='手动输入'
                      value={manualTag}
                      onChange={(e) => setManualTag(e.target.value)}
                      className='font-mono text-sm'
                    />
                    <p className='text-xs text-muted-foreground'>
                      为这些节点设置标签，用于节点管理中的分类和筛选
                    </p>
                  </div>
                  <div className='flex justify-end gap-2'>
                    <Button onClick={handleParse} disabled={!input.trim()} variant='outline'>
                      解析节点
                    </Button>
                    <Button
                      onClick={handleSave}
                      disabled={tempNodes.length === 0 || batchCreateMutation.isPending}
                    >
                      {batchCreateMutation.isPending ? '保存中...' : '保存节点'}
                    </Button>
                  </div>
                </TabsContent>

                <TabsContent value='subscription' className='space-y-4 mt-4'>
                  <div className='space-y-2'>
                    <Input
                      placeholder='https://example.com/api/clash/subscribe?token=xxx'
                      value={subscriptionUrl}
                      onChange={handleSubscriptionUrlChange}
                      className='font-mono text-sm'
                    />
                    <p className='text-xs text-muted-foreground'>
                      请输入 Clash 订阅链接，系统将自动获取并解析节点
                    </p>
                  </div>
                  <div className='flex items-center gap-2'>
                    <Label htmlFor='user-agent' className='whitespace-nowrap'>User-Agent:</Label>
                    <Select value={userAgent} onValueChange={handleUserAgentChange}>
                      <SelectTrigger id='user-agent' className='w-[200px]'>
                        <SelectValue placeholder='选择 User-Agent' />
                      </SelectTrigger>
                      <SelectContent>
                        <SelectItem value='clash.meta'>clash.meta</SelectItem>
                        <SelectItem value='clash-verge/v1.5.1'>clash-verge/v1.5.1</SelectItem>
                        <SelectItem value='Clash'>Clash</SelectItem>
                        <SelectItem value='手动输入'>手动输入</SelectItem>
                      </SelectContent>
                    </Select>
                    {userAgent === '手动输入' && (
                      <Input
                        placeholder='输入自定义 User-Agent'
                        value={customUserAgent}
                        onChange={handleCustomUserAgentChange}
                        className='font-mono text-sm flex-1'
                      />
                    )}
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='subscription-tag' className='text-sm font-medium'>
                      节点标签
                    </Label>
                    <Input
                      id='subscription-tag'
                      placeholder='默认使用服务器地址作为标签'
                      value={subscriptionTag}
                      onChange={(e) => setSubscriptionTag(e.target.value)}
                      className='font-mono text-sm'
                    />
                    <p className='text-xs text-muted-foreground'>
                      为订阅导入的节点设置标签，留空将使用服务器地址作为标签
                    </p>
                  </div>
                  <div className='flex justify-end gap-2'>
                    <Button
                      onClick={handleFetchSubscription}
                      disabled={!subscriptionUrl.trim() || fetchSubscriptionMutation.isPending}
                      variant='outline'
                    >
                      {fetchSubscriptionMutation.isPending ? '导入中...' : '导入节点'}
                    </Button>
                    <Button
                      onClick={handleSave}
                      disabled={tempNodes.length === 0 || batchCreateMutation.isPending}
                    >
                      {batchCreateMutation.isPending ? '保存中...' : '保存节点'}
                    </Button>
                  </div>
                </TabsContent>
              </Tabs>
            </CardContent>
          </Card>

          {displayNodes.length > 0 && (
            <Card>
              <CardHeader>
                <div className='flex items-center justify-between'>
                  <div>
                    <CardTitle>节点列表 ({filteredNodes.length})</CardTitle>
                    <p className='mt-2 text-sm font-semibold text-destructive'>注意!!! 节点的修改与删除均会同步更新所有订阅 </p>
                  </div>
                  <div className='flex gap-2'>
                    {selectedNodeIds.size > 0 && (
                      <>
                        <Button
                          variant='default'
                          size='sm'
                          onClick={() => setBatchTagDialogOpen(true)}
                        >
                          批量修改标签 ({selectedNodeIds.size})
                        </Button>
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button
                              variant='destructive'
                              size='sm'
                            >
                              批量删除 ({selectedNodeIds.size})
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>确认批量删除节点</AlertDialogTitle>
                              <AlertDialogDescription>
                                确定要删除选中的 {selectedNodeIds.size} 个节点吗？此操作不可撤销。
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>取消</AlertDialogCancel>
                              <AlertDialogAction
                                onClick={() => {
                                  // 使用批量删除 API
                                  const ids = Array.from(selectedNodeIds)
                                  api.post('/api/admin/nodes/batch-delete', { node_ids: ids })
                                    .then((response) => {
                                      queryClient.invalidateQueries({ queryKey: ['nodes'] })
                                      setSelectedNodeIds(new Set())
                                      const { deleted, total } = response.data
                                      if (deleted === total) {
                                        toast.success(`成功删除 ${deleted} 个节点`)
                                      } else {
                                        toast.success(`成功删除 ${deleted}/${total} 个节点`)
                                      }
                                    })
                                    .catch((error) => {
                                      toast.error(error.response?.data?.error || '批量删除失败')
                                    })
                                }}
                              >
                                确认删除
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      </>
                    )}
                    {savedNodes.length > 0 && (
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            variant='destructive'
                            size='sm'
                            disabled={clearAllMutation.isPending}
                          >
                            {clearAllMutation.isPending ? '清空中...' : '清空所有'}
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>确认清空所有节点</AlertDialogTitle>
                            <AlertDialogDescription>
                              确定要清空所有已保存的节点吗？此操作不可撤销，将删除 {savedNodes.length} 个节点。
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>取消</AlertDialogCancel>
                            <AlertDialogAction onClick={handleClearAll}>
                              清空所有
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    )}
                  </div>
                </div>
              </CardHeader>
              <CardContent className='space-y-4'>
                {/* 协议筛选按钮 */}
                <div className='space-y-3'>
                  <div>
                    <div className='text-sm font-medium mb-2'>按协议筛选</div>
                    <div className='flex flex-wrap gap-2'>
                      <Button
                        size='sm'
                        variant={selectedProtocol === 'all' ? 'default' : 'outline'}
                        onClick={() => setSelectedProtocol('all')}
                      >
                        全部 ({protocolCounts.all})
                      </Button>
                      {PROTOCOLS.map(protocol => {
                        const count = protocolCounts[protocol] || 0
                        if (count === 0) return null
                        return (
                          <Button
                            key={protocol}
                            size='sm'
                            variant={selectedProtocol === protocol ? 'default' : 'outline'}
                            onClick={() => setSelectedProtocol(protocol)}
                          >
                            {protocol.toUpperCase()} ({count})
                          </Button>
                        )
                      })}
                    </div>
                  </div>

                  {/* 标签筛选按钮 */}
                  <div>
                    <div className='text-sm font-medium mb-2'>按标签筛选</div>
                    <div className='flex flex-wrap gap-2'>
                      <Button
                        size='sm'
                        variant={tagFilter === 'all' ? 'default' : 'outline'}
                        onClick={() => {
                          setTagFilter('all')
                          // 计算应该选中的节点
                          const nodesToSelect = displayNodes
                            .filter(n => n.isSaved && n.dbId)
                            .filter(n => selectedProtocol === 'all' || n.dbNode?.protocol?.toLowerCase() === selectedProtocol)
                          const nodeIdsToSelect = new Set(nodesToSelect.map(n => n.dbId!))

                          // 如果当前选中的节点和应该选中的节点完全一致，则取消选中
                          const currentIds = Array.from(selectedNodeIds).sort()
                          const targetIds = Array.from(nodeIdsToSelect).sort()
                          if (tagFilter === 'all' && currentIds.length === targetIds.length &&
                              currentIds.every((id, i) => id === targetIds[i])) {
                            setSelectedNodeIds(new Set())
                          } else {
                            setSelectedNodeIds(nodeIdsToSelect)
                          }
                        }}
                      >
                        全部 ({tagCounts.all})
                      </Button>
                      {Object.keys(tagCounts).filter(tag => tag !== 'all' && tagCounts[tag] > 0).map(tag => (
                        <Button
                          key={tag}
                          size='sm'
                          variant={tagFilter === tag ? 'default' : 'outline'}
                          onClick={() => {
                            setTagFilter(tag)
                            // 计算应该选中的节点
                            const nodesToSelect = displayNodes
                              .filter(n => n.isSaved && n.dbId && n.dbNode?.tag === tag)
                              .filter(n => selectedProtocol === 'all' || n.dbNode?.protocol?.toLowerCase() === selectedProtocol)
                            const nodeIdsToSelect = new Set(nodesToSelect.map(n => n.dbId!))

                            // 如果当前选中的节点和应该选中的节点完全一致，则取消选中
                            const currentIds = Array.from(selectedNodeIds).sort()
                            const targetIds = Array.from(nodeIdsToSelect).sort()
                            if (tagFilter === tag && currentIds.length === targetIds.length &&
                                currentIds.every((id, i) => id === targetIds[i])) {
                              setSelectedNodeIds(new Set())
                            } else {
                              setSelectedNodeIds(nodeIdsToSelect)
                            }
                          }}
                        >
                          {tag} ({tagCounts[tag]})
                        </Button>
                      ))}
                    </div>
                  </div>
                </div>

                {/* 节点表格 */}
                <div className='rounded-md border overflow-auto'>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className='w-[50px]'>
                          <Checkbox
                            checked={
                              filteredNodes.filter(n => n.isSaved && n.dbId).length > 0 &&
                              filteredNodes.filter(n => n.isSaved && n.dbId).every(n => selectedNodeIds.has(n.dbId!))
                            }
                            onCheckedChange={(checked) => {
                              const savedNodes = filteredNodes.filter(n => n.isSaved && n.dbId)
                              if (checked) {
                                setSelectedNodeIds(new Set(savedNodes.map(n => n.dbId!)))
                              } else {
                                setSelectedNodeIds(new Set())
                              }
                            }}
                          />
                        </TableHead>
                        <TableHead className='w-[100px]'>协议</TableHead>
                        <TableHead className='min-w-[150px]'>节点名称</TableHead>
                        <TableHead className='w-[100px]'>标签</TableHead>
                        <TableHead className='min-w-[200px]'>服务器地址</TableHead>
                        <TableHead className='w-[80px] text-center'>配置</TableHead>
                        <TableHead className='w-[100px] text-center'>操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredNodes.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={7} className='text-center text-muted-foreground py-8'>
                            没有找到匹配的节点
                          </TableCell>
                        </TableRow>
                      ) : (
                        filteredNodes.map(node => (
                          <TableRow key={node.id}>
                            <TableCell>
                              {node.isSaved && node.dbId && (
                                <Checkbox
                                  checked={selectedNodeIds.has(node.dbId)}
                                  onCheckedChange={(checked) => {
                                    const newSet = new Set(selectedNodeIds)
                                    if (checked) {
                                      newSet.add(node.dbId!)
                                    } else {
                                      newSet.delete(node.dbId!)
                                    }
                                    setSelectedNodeIds(newSet)
                                  }}
                                />
                              )}
                            </TableCell>
                            <TableCell>
                              {node.parsed ? (
                                <Badge
                                  variant='outline'
                                  className={
                                    node.dbNode?.protocol?.includes('⇋')
                                      ? 'bg-pink-500/10 text-pink-700 border-pink-200 dark:text-pink-300 dark:border-pink-800'
                                      : PROTOCOL_COLORS[node.parsed.type] || 'bg-gray-500/10'
                                  }
                                >
                                  {node.dbNode?.protocol?.includes('⇋')
                                    ? node.dbNode.protocol.toUpperCase()
                                    : node.parsed.type.toUpperCase()}
                                </Badge>
                              ) : (
                                <Badge variant='destructive'>解析失败</Badge>
                              )}
                            </TableCell>
                            <TableCell className='font-medium'>
                              {editingNode?.id === node.id ? (
                                <div className='flex items-center gap-2'>
                                  <Input
                                    value={editingNode.value}
                                    onChange={(event) => handleNameEditChange(event.target.value)}
                                    onKeyDown={(event) => {
                                      if (event.key === 'Enter') {
                                        event.preventDefault()
                                        handleNameEditSubmit(node)
                                      } else if (event.key === 'Escape') {
                                        event.preventDefault()
                                        handleNameEditCancel()
                                      }
                                    }}
                                    className='h-8 w-48'
                                    autoFocus
                                  />
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-8 text-emerald-600'
                                    onClick={() => handleNameEditSubmit(node)}
                                    disabled={node.isSaved ? updateNodeNameMutation.isPending : false}
                                  >
                                    <Check className='size-4' />
                                  </Button>
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-8 text-muted-foreground'
                                    onClick={handleNameEditCancel}
                                  >
                                    <X className='size-4' />
                                  </Button>
                                  {node.isSaved && (
                                    <Badge variant='secondary' className='text-xs'>已保存</Badge>
                                  )}
                                </div>
                              ) : (
                                <div className='flex items-center gap-2'>
                                  <span className='truncate max-w-[200px]'>{node.name || '未知'}</span>
                                  {node.isSaved && (
                                    <Badge variant='secondary' className='text-xs'>已保存</Badge>
                                  )}
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-[#d97757] hover:text-[#c66647]'
                                    onClick={() => handleNameEditStart(node)}
                                    disabled={node.isSaved ? updateNodeNameMutation.isPending : false}
                                  >
                                    <Pencil className='size-4' />
                                  </Button>
                                  {node.isSaved && node.dbNode && !node.dbNode.protocol.includes('⇋') && (
                                    <Button
                                      variant='ghost'
                                      size='icon'
                                      className='size-7 text-muted-foreground hover:text-foreground'
                                      onClick={() => {
                                        setSourceNodeForExchange(node.dbNode)
                                        setExchangeDialogOpen(true)
                                      }}
                                    >
                                      <img
                                        src={ExchangeIcon}
                                        alt='交换'
                                        className='size-4 [filter:invert(63%)_sepia(45%)_saturate(1068%)_hue-rotate(327deg)_brightness(95%)_contrast(88%)]'
                                      />
                                    </Button>
                                  )}
                                </div>
                              )}
                            </TableCell>
                            <TableCell>
                              <div className='flex flex-wrap gap-1'>
                                <Badge
                                  variant='secondary'
                                  className='text-xs'
                                >
                                  {node.dbNode?.tag || node.tag || (currentTag === 'manual' ? manualTag.trim() || '手动输入' : currentTag === 'subscription' ? subscriptionTag.trim() || '订阅导入' : '未知')}
                                </Badge>
                                {node.isSaved && node.dbNode?.probe_server && (
                                  <Badge variant='secondary' className='text-xs flex items-center gap-1'>
                                    <Activity className='size-3' />
                                    {node.dbNode.probe_server}
                                  </Badge>
                                )}
                              </div>
                            </TableCell>
                            <TableCell>
                              <div className='text-sm text-muted-foreground'>
                                {node.parsed ? (
                                  <div className='flex items-center gap-2'>
                                    <div>
                                      <div className='font-mono'>{node.parsed.server}:{node.parsed.port}</div>
                                      {node.parsed.network && node.parsed.network !== 'tcp' && (
                                        <div className='text-xs mt-1'>
                                          <Badge variant='outline' className='text-xs'>
                                            {node.parsed.network}
                                          </Badge>
                                        </div>
                                      )}
                                    </div>
                                    {node.parsed?.server && (
                                      (() => {
                                        const nodeKey = node.isSaved ? String(node.dbId) : node.id
                                        const serverIsIp = isIpAddress(node.parsed.server)
                                        const hasOriginalServer = !node.isSaved && node.originalServer

                                        // 已保存的节点且服务器地址已经是IP，不显示按钮
                                        if (node.isSaved && serverIsIp) {
                                          return null
                                        }

                                        // 未保存的节点且有原始服务器地址，显示回退按钮
                                        if (hasOriginalServer) {
                                          return (
                                            <Button
                                              variant='ghost'
                                              size='sm'
                                              className='size-6 p-0 border border-orange-500/50 hover:border-orange-500'
                                              title='恢复原始域名'
                                              onClick={() => restoreTempNodeServer(node.id)}
                                            >
                                              <Undo2 className='size-4 text-orange-500' />
                                            </Button>
                                          )
                                        }

                                        // 显示IP解析菜单或按钮
                                        return ipMenuState?.nodeId === nodeKey ? (
                                          <DropdownMenu open={true} onOpenChange={(open) => !open && setIpMenuState(null)}>
                                            <DropdownMenuTrigger asChild>
                                              <Button
                                                variant='ghost'
                                                size='sm'
                                                className='size-6 p-0 border border-primary/50 hover:border-primary'
                                                title='选择IP地址'
                                              >
                                                <img
                                                  src={IpIcon}
                                                  alt='IP'
                                                  className='size-4 [filter:invert(63%)_sepia(45%)_saturate(1068%)_hue-rotate(327deg)_brightness(95%)_contrast(88%)]'
                                                />
                                              </Button>
                                            </DropdownMenuTrigger>
                                            <DropdownMenuContent align='start'>
                                              {ipMenuState.ips.map((ip) => (
                                                <DropdownMenuItem
                                                  key={ip}
                                                  onClick={() => {
                                                    if (node.isSaved && node.dbId) {
                                                      updateNodeServerMutation.mutate({
                                                        nodeId: node.dbId,
                                                        server: ip,
                                                      })
                                                    } else {
                                                      updateTempNodeServer(node.id, ip)
                                                      setIpMenuState(null)
                                                    }
                                                  }}
                                                >
                                                  <span className='font-mono'>{ip}</span>
                                                </DropdownMenuItem>
                                              ))}
                                            </DropdownMenuContent>
                                          </DropdownMenu>
                                        ) : (
                                          <Button
                                            variant='ghost'
                                            size='sm'
                                            className='size-6 p-0 border border-primary/50 hover:border-primary'
                                            title='解析IP地址'
                                            disabled={resolvingIpFor === nodeKey}
                                            onClick={() => handleResolveIp(node)}
                                          >
                                            <img
                                              src={IpIcon}
                                              alt='IP'
                                              className='size-4 [filter:invert(63%)_sepia(45%)_saturate(1068%)_hue-rotate(327deg)_brightness(95%)_contrast(88%)]'
                                            />
                                          </Button>
                                        )
                                      })()
                                    )}
                                    {node.isSaved && node.dbNode?.original_server && (
                                      <Button
                                        variant='ghost'
                                        size='sm'
                                        className='size-6 p-0 border border-primary/50 hover:border-primary ml-1'
                                        title='恢复原始域名'
                                        disabled={restoreNodeServerMutation.isPending}
                                        onClick={() => restoreNodeServerMutation.mutate(node.dbId)}
                                      >
                                        <Undo2 className='size-3' />
                                      </Button>
                                    )}
                                    {userConfig?.enable_probe_binding && node.isSaved && node.dbNode && (
                                      <Button
                                        variant='ghost'
                                        size='sm'
                                        className='size-6 p-0 border border-primary/50 hover:border-primary ml-1'
                                        title={node.dbNode.probe_server ? `当前绑定: ${node.dbNode.probe_server}` : '绑定探针服务器'}
                                        onClick={() => {
                                          setSelectedNodeForProbe(node.dbNode!)
                                          setProbeBindingDialogOpen(true)
                                          refetchProbeConfig() // 打开对话框时查询探针配置
                                        }}
                                      >
                                        <Activity className={`size-4 ${node.dbNode.probe_server ? 'text-green-600' : 'text-[#d97757]'}`} />
                                      </Button>
                                    )}
                                  </div>
                                ) : (
                                  '-'
                                )}
                              </div>
                            </TableCell>
                            <TableCell className='text-center'>
                              {node.clash ? (
                                <div className='flex gap-1 justify-center'>
                                  <Dialog
                                    open={clashDialogOpen && (
                                      (node.isSaved && editingClashConfig?.nodeId === node.dbNode?.id) ||
                                      (!node.isSaved && editingClashConfig?.nodeId === -1)
                                    )}
                                    onOpenChange={(open) => {
                                      setClashDialogOpen(open)
                                      if (!open) {
                                        // Dialog关闭后清理状态
                                        setTimeout(() => {
                                          setEditingClashConfig(null)
                                          setClashConfigError('')
                                          setJsonErrorLines([])
                                        }, 150) // 等待关闭动画完成
                                      }
                                    }}
                                  >
                                    <DialogTrigger asChild>
                                      <Button
                                        variant='ghost'
                                        size='icon'
                                        className='h-8 w-8'
                                        onClick={() => {
                                          if (node.isSaved && node.dbNode) {
                                            handleEditClashConfig(node.dbNode)
                                          } else if (!node.isSaved) {
                                            handleEditClashConfig(node)
                                          }
                                        }}
                                      >
                                        <Eye className='h-4 w-4' />
                                      </Button>
                                    </DialogTrigger>
                                    <DialogContent className='max-w-4xl sm:max-w-4xl max-h-[80vh] flex flex-col'>
                                    <DialogHeader>
                                      <DialogTitle>
                                        Clash 配置详情{editingClashConfig?.nodeId === -1 ? '（仅查看）' : ''}
                                      </DialogTitle>
                                      <DialogDescription>
                                        {node.name || '未知'}
                                        {editingClashConfig?.nodeId === -1 && ' - 保存节点后可编辑配置'}
                                      </DialogDescription>
                                    </DialogHeader>
                                    <div className='mt-4 flex-1 flex flex-col gap-3 min-h-0'>
                                      <div className='flex-1 flex border rounded overflow-hidden bg-muted'>
                                        {/* 行号列 */}
                                        <div className='flex flex-col bg-muted-foreground/10 text-muted-foreground text-xs font-mono select-none py-3 px-2 text-right'>
                                          {editingClashConfig?.config.split('\n').map((_, i) => {
                                            const lineNum = i + 1
                                            const isErrorLine = jsonErrorLines.includes(lineNum)
                                            return (
                                              <div
                                                key={i}
                                                className={`leading-5 h-5 ${isErrorLine ? 'bg-destructive/20 text-destructive font-bold' : ''}`}
                                              >
                                                {lineNum}
                                              </div>
                                            )
                                          })}
                                        </div>
                                        {/* 文本编辑区 */}
                                        <Textarea
                                          value={editingClashConfig?.config || ''}
                                          onChange={(e) => handleClashConfigChange(e.target.value)}
                                          className='font-mono text-xs flex-1 min-h-[400px] resize-none border-0 rounded-none focus-visible:ring-0 leading-5'
                                          placeholder='输入 JSON 配置...'
                                          readOnly={editingClashConfig?.nodeId === -1}
                                        />
                                      </div>
                                      {clashConfigError && (
                                        <div className='text-xs text-destructive bg-destructive/10 p-2 rounded'>
                                          {clashConfigError}
                                        </div>
                                      )}
                                      <div className='flex gap-2 justify-end'>
                                        <Button
                                          variant='outline'
                                          size='sm'
                                          onClick={() => setClashDialogOpen(false)}
                                        >
                                          {editingClashConfig?.nodeId === -1 ? '关闭' : '取消'}
                                        </Button>
                                        {editingClashConfig?.nodeId !== -1 && (
                                          <Button
                                            size='sm'
                                            onClick={handleSaveClashConfig}
                                            disabled={!!clashConfigError || updateClashConfigMutation.isPending}
                                          >
                                            {updateClashConfigMutation.isPending ? '保存中...' : '保存'}
                                          </Button>
                                        )}
                                      </div>
                                    </div>
                                  </DialogContent>
                                </Dialog>
                                <Button
                                  variant='ghost'
                                  size='icon'
                                  className='h-8 w-8'
                                  title='复制 URI'
                                  onClick={() => node.isSaved && handleCopyUri(node.dbNode!)}
                                >
                                  <Copy className='h-4 w-4' />
                                </Button>
                              </div>
                              ) : (
                                <span className='text-xs text-muted-foreground'>-</span>
                              )}
                            </TableCell>
                            <TableCell className='text-center'>
                              <AlertDialog>
                                <AlertDialogTrigger asChild>
                                  <Button
                                    variant='ghost'
                                    size='sm'
                                    disabled={node.isSaved && deleteMutation.isPending}
                                  >
                                    删除
                                  </Button>
                                </AlertDialogTrigger>
                                <AlertDialogContent>
                                  <AlertDialogHeader>
                                    <AlertDialogTitle>确认删除</AlertDialogTitle>
                                    <AlertDialogDescription>
                                      确定要删除节点 "{node.name || '未知'}" 吗？
                                      {node.isSaved && '此操作不可撤销。'}
                                    </AlertDialogDescription>
                                  </AlertDialogHeader>
                                  <AlertDialogFooter>
                                    <AlertDialogCancel>取消</AlertDialogCancel>
                                    <AlertDialogAction
                                      onClick={() => node.isSaved ? handleDelete(node.dbId) : handleDeleteTemp(node.id)}
                                    >
                                      删除
                                    </AlertDialogAction>
                                  </AlertDialogFooter>
                                </AlertDialogContent>
                              </AlertDialog>
                            </TableCell>
                          </TableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                </div>
              </CardContent>
            </Card>
          )}
        </section>
      </main>

      {/* 探针绑定对话框 */}
      <Dialog open={probeBindingDialogOpen} onOpenChange={setProbeBindingDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>绑定探针服务器</DialogTitle>
            <DialogDescription>
              为节点 "{selectedNodeForProbe?.node_name}" 选择要绑定的探针服务器
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            {probeConfig?.servers && probeConfig.servers.length > 0 ? (
              <div className='space-y-2'>
                {probeConfig.servers.map((server) => (
                  <Button
                    key={server.id}
                    variant={selectedNodeForProbe?.probe_server === server.name ? 'default' : 'outline'}
                    className='w-full justify-start'
                    onClick={() => {
                      if (selectedNodeForProbe) {
                        updateProbeBindingMutation.mutate({
                          nodeId: selectedNodeForProbe.id,
                          probeServer: server.name
                        })
                      }
                    }}
                    disabled={updateProbeBindingMutation.isPending}
                  >
                    <div className='flex items-center gap-2'>
                      <Activity className='size-4' />
                      <div className='text-left'>
                        <div className='font-medium'>{server.name}</div>
                        <div className='text-xs text-muted-foreground'>ID: {server.server_id}</div>
                      </div>
                    </div>
                  </Button>
                ))}
                {selectedNodeForProbe?.probe_server && (
                  <Button
                    variant='ghost'
                    className='w-full'
                    onClick={() => {
                      if (selectedNodeForProbe) {
                        updateProbeBindingMutation.mutate({
                          nodeId: selectedNodeForProbe.id,
                          probeServer: ''
                        })
                      }
                    }}
                    disabled={updateProbeBindingMutation.isPending}
                  >
                    <X className='size-4 mr-2' />
                    取消绑定
                  </Button>
                )}
              </div>
            ) : (
              <div className='text-center text-sm text-muted-foreground py-8'>
                暂无可用的探针服务器
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* URI 手动复制对话框 */}
      <Dialog open={uriDialogOpen} onOpenChange={setUriDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>手动复制 URI</DialogTitle>
            <DialogDescription>
              自动复制失败，请手动复制下方的 URI
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='p-3 bg-muted rounded-md'>
              <code className='text-xs break-all'>{uriContent}</code>
            </div>
            <div className='flex justify-end gap-2'>
              <Button
                variant='outline'
                onClick={() => setUriDialogOpen(false)}
              >
                关闭
              </Button>
              <Button
                onClick={() => {
                  navigator.clipboard.writeText(uriContent).then(() => {
                    toast.success('URI 已复制到剪贴板')
                    setUriDialogOpen(false)
                  }).catch(() => {
                    toast.error('复制失败，请手动选择文本复制')
                  })
                }}
              >
                再试一次
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* 节点交换对话框 */}
      <Dialog open={exchangeDialogOpen} onOpenChange={(open) => {
        setExchangeDialogOpen(open)
        if (!open) {
          setExchangeFilterText('') // 关闭对话框时清空筛选
        }
      }}>
        <DialogContent className='max-w-2xl max-h-[80vh] overflow-y-auto'>
          <DialogHeader>
            <DialogTitle>创建链式代理节点</DialogTitle>
            <DialogDescription>
              选择目标节点与 "{sourceNodeForExchange?.node_name}" 创建链式代理
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            {/* 筛选输入框 */}
            <div className='space-y-2'>
              <Input
                placeholder='搜索节点名称、协议或标签...'
                value={exchangeFilterText}
                onChange={(e) => setExchangeFilterText(e.target.value)}
                className='text-sm'
              />
              <p className='text-xs text-muted-foreground'>
                自动排除链式代理节点
              </p>
            </div>

            {(() => {
              // 筛选逻辑
              const filteredNodes = savedNodes
                .filter(node => node.id !== sourceNodeForExchange?.id) // 排除源节点自己
                .filter(node => !node.protocol.includes('⇋')) // 排除链式代理节点（协议包含⇋）
                .filter(node => {
                  if (!exchangeFilterText.trim()) return true
                  const searchText = exchangeFilterText.toLowerCase()
                  return (
                    node.node_name.toLowerCase().includes(searchText) ||
                    node.protocol.toLowerCase().includes(searchText) ||
                    (node.tag && node.tag.toLowerCase().includes(searchText))
                  )
                })

              return filteredNodes.length > 0 ? (
                <div className='space-y-2'>
                  {filteredNodes.map((node) => (
                    <Button
                      key={node.id}
                      variant='outline'
                      className='w-full justify-start text-left h-auto py-3'
                      onClick={() => {
                        if (sourceNodeForExchange) {
                          createRelayNodeMutation.mutate({
                            sourceNode: sourceNodeForExchange,
                            targetNode: node
                          })
                        }
                      }}
                      disabled={createRelayNodeMutation.isPending}
                    >
                      <div className='flex flex-col gap-2 w-full items-start'>
                        <div className='flex items-center gap-2 w-full flex-wrap'>
                          <span className='font-medium'>{node.node_name}</span>
                          <span className='text-xs text-muted-foreground'>
                            {node.protocol} - {node.original_server}
                          </span>
                        </div>
                        {node.tag && (
                          <Badge variant='secondary' className='text-xs'>
                            {node.tag}
                          </Badge>
                        )}
                      </div>
                    </Button>
                  ))}
                </div>
              ) : (
                <div className='text-center text-sm text-muted-foreground py-8'>
                  {exchangeFilterText.trim() ? '未找到匹配的节点' : '暂无可用的节点'}
                </div>
              )
            })()}
          </div>
        </DialogContent>
      </Dialog>

      {/* 批量修改标签对话框 */}
      <Dialog open={batchTagDialogOpen} onOpenChange={setBatchTagDialogOpen}>
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>批量修改标签</DialogTitle>
            <DialogDescription>
              将为选中的 {selectedNodeIds.size} 个节点修改标签
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            {allUniqueTags.length > 0 && (
              <div className='space-y-2'>
                <Label className='text-sm font-medium'>快速选择标签</Label>
                <div className='flex flex-wrap gap-2'>
                  {allUniqueTags.map((tag) => (
                    <Badge
                      key={tag}
                      variant='outline'
                      className='cursor-pointer hover:bg-primary hover:text-primary-foreground transition-colors'
                      onClick={() => setBatchTag(tag)}
                    >
                      {tag}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
            <div className='space-y-2'>
              <Label htmlFor='batch-tag-input' className='text-sm font-medium'>
                标签名称
              </Label>
              <Input
                id='batch-tag-input'
                placeholder='输入标签名称'
                value={batchTag}
                onChange={(e) => setBatchTag(e.target.value)}
                className='font-mono text-sm'
              />
            </div>
            <div className='flex justify-end gap-2 pt-2'>
              <Button
                variant='outline'
                onClick={() => {
                  setBatchTagDialogOpen(false)
                  setBatchTag('')
                }}
                disabled={batchUpdateTagMutation.isPending}
              >
                取消
              </Button>
              <Button
                onClick={() => {
                  if (!batchTag.trim()) {
                    toast.error('请输入标签名称')
                    return
                  }
                  const nodeIds = Array.from(selectedNodeIds)
                  batchUpdateTagMutation.mutate({
                    nodeIds,
                    tag: batchTag.trim(),
                  })
                }}
                disabled={batchUpdateTagMutation.isPending || !batchTag.trim()}
              >
                {batchUpdateTagMutation.isPending ? '保存中...' : '保存'}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
