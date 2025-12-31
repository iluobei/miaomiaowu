// @ts-nocheck
import { useState, useMemo, useCallback, useEffect } from 'react'
import { createPortal } from 'react-dom'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Topbar } from '@/components/layout/topbar'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { cn } from '@/lib/utils'
import { Button } from '@/components/ui/button'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@/components/ui/table'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '@/components/ui/alert-dialog'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { parseProxyUrl, toClashProxy, type ProxyNode, type ClashProxy } from '@/lib/proxy-parser'
import { Check, Pencil, X, Undo2, Activity, Eye, Copy, ChevronDown, Link2, Flag, GripVertical } from 'lucide-react'
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from '@/components/ui/dropdown-menu'
import IpIcon from '@/assets/icons/ip.svg'
import ExchangeIcon from '@/assets/icons/exchange.svg'
import URI_Producer from '@/lib/substore/producers/uri'
import { countryCodeToFlag, hasEmojiPrefix, getGeoIPInfo } from '@/lib/country-flag'
import { Twemoji } from '@/components/twemoji'
import { useMediaQuery } from '@/hooks/use-media-query'
import {
  DndContext,
  closestCenter,
  DragEndEvent,
  DragStartEvent,
  DragOverlay,
  PointerSensor,
  TouchSensor,
  useSensor,
  useSensors,
} from '@dnd-kit/core'
import {
  arrayMove,
  SortableContext,
  useSortable,
  verticalListSortingStrategy,
} from '@dnd-kit/sortable'

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
  wireguard: 'bg-orange-500/10 text-orange-700 dark:text-orange-400',
}

const PROTOCOLS = ['vmess', 'vless', 'trojan', 'ss', 'socks5', 'hysteria', 'hysteria2', 'tuic', 'anytls', 'wireguard']

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

// 拖拽把手组件
function DragHandle({ id, size = 'default' }: { id: string; size?: 'default' | 'large' }) {
  const { attributes, listeners } = useSortable({ id })

  return (
    <div
      {...attributes}
      {...listeners}
      data-drag-handle
      className={cn(
        'cursor-grab active:cursor-grabbing touch-none rounded-md',
        size === 'large'
          ? 'p-2 hover:bg-accent/80'
          : 'p-1'
      )}
    >
      <GripVertical className={cn(
        'text-muted-foreground',
        size === 'large' ? 'h-5 w-5' : 'h-4 w-4'
      )} />
    </div>
  )
}

// 可拖拽排序的表格行组件
interface SortableTableRowProps {
  id: string
  isSaved: boolean
  dbId?: number
  batchDraggingIds: Set<number>
  isSelected?: boolean
  onClick?: (e: React.MouseEvent) => void
  children: React.ReactNode
}

function SortableTableRow({ id, isSaved, dbId, batchDraggingIds, isSelected, onClick, children }: SortableTableRowProps) {
  const {
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id,
    disabled: !isSaved, // 只有已保存的节点可以拖拽
    animateLayoutChanges: () => false,
  })

  // 检查是否是批量拖动中的节点（但不是被直接拖动的那个）
  const isBatchDragging = dbId && batchDraggingIds.size > 0 && batchDraggingIds.has(dbId) && !isDragging

  const style: React.CSSProperties = {
    transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
    transition: isDragging ? undefined : transition,
    opacity: isDragging ? 0.5 : 1,
    touchAction: 'none',
  }

  return (
    <TableRow
      ref={setNodeRef}
      style={style}
      onClick={onClick}
      className={cn(
        'cursor-pointer group/row',
        isDragging
          ? 'opacity-0'  // 被直接拖动的节点完全隐藏，由 DragOverlay 显示
          : isBatchDragging
            ? 'opacity-30 bg-primary/10'  // 其他选中节点也变淡
            : '',
        isSelected && !isDragging && !isBatchDragging && 'bg-primary/15 ring-2 ring-inset ring-primary/50 hover:bg-primary/20'
      )}
    >
      {children}
    </TableRow>
  )
}

// 可拖拽排序的移动端卡片组件
interface SortableCardProps {
  id: string
  isSaved: boolean
  dbId?: number
  batchDraggingIds: Set<number>
  isSelected?: boolean
  onClick?: (e: React.MouseEvent) => void
  children: React.ReactNode
}

function SortableCard({ id, isSaved, dbId, batchDraggingIds, isSelected, onClick, children }: SortableCardProps) {
  const {
    setNodeRef,
    transform,
    transition,
    isDragging,
  } = useSortable({
    id,
    disabled: !isSaved,
    animateLayoutChanges: () => false,
  })

  const isBatchDragging = dbId && batchDraggingIds.size > 0 && batchDraggingIds.has(dbId) && !isDragging

  const style: React.CSSProperties = {
    transform: transform ? `translate3d(${transform.x}px, ${transform.y}px, 0)` : undefined,
    transition: isDragging ? undefined : transition,
    opacity: isDragging ? 0.5 : 1,
    touchAction: 'none',
  }

  return (
    <Card
      ref={setNodeRef}
      style={style}
      onClick={onClick}
      className={cn(
        'overflow-hidden cursor-pointer',
        isDragging
          ? 'opacity-0'
          : isBatchDragging
            ? 'opacity-30 bg-primary/10'
            : '',
        isSelected && !isDragging && !isBatchDragging && 'bg-accent'
      )}
    >
      {children}
    </Card>
  )
}

// DragOverlay 内容组件
function DragOverlayContent({ nodes, protocolColors }: { nodes: TempNode[]; protocolColors: Record<string, string> }) {
  if (nodes.length === 0) return null

  if (nodes.length === 1) {
    // 单节点：显示简单的节点卡片
    const node = nodes[0]
    return (
      <div className='bg-background border rounded-md shadow-lg p-3 min-w-[200px] max-w-[300px]'>
        <div className='flex items-center gap-2'>
          <Badge variant='secondary' className={protocolColors[node.parsed?.type || ''] || ''}>
            {node.parsed?.type?.toUpperCase() || 'UNKNOWN'}
          </Badge>
          <span className='font-medium truncate'>{node.name}</span>
        </div>
      </div>
    )
  }

  // 多节点：显示堆叠效果 + 数量标记
  const firstNode = nodes[0]
  return (
    <div className='relative'>
      {/* 底部堆叠效果 */}
      {nodes.length > 2 && (
        <div className='absolute top-2 left-2 bg-muted border rounded-md shadow p-3 min-w-[200px] max-w-[300px] h-[48px] opacity-60' />
      )}
      <div className='absolute top-1 left-1 bg-muted border rounded-md shadow p-3 min-w-[200px] max-w-[300px] h-[48px] opacity-80' />

      {/* 主卡片 */}
      <div className='relative bg-background border rounded-md shadow-lg p-3 min-w-[200px] max-w-[300px]'>
        <div className='flex items-center gap-2'>
          <Badge variant='secondary' className={protocolColors[firstNode.parsed?.type || ''] || ''}>
            {firstNode.parsed?.type?.toUpperCase() || 'UNKNOWN'}
          </Badge>
          <span className='font-medium truncate'>{firstNode.name}</span>
        </div>

        {/* 数量标记 */}
        <Badge className='absolute -top-2 -right-2 bg-primary text-primary-foreground'>
          {nodes.length} 个节点
        </Badge>
      </div>
    </div>
  )
}

function NodesPage() {
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()

  // 视口宽度判断 - 用于条件渲染 SortableContext，避免重复注册导致拖动偏移
  const isDesktop = useMediaQuery('(min-width: 1024px)')
  const isTablet = useMediaQuery('(min-width: 768px)')

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

  // 导入节点卡片折叠状态 - 默认折叠
  const [isInputCardExpanded, setIsInputCardExpanded] = useState(false)

  // 批量操作状态
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<number>>(new Set())
  const [batchTagDialogOpen, setBatchTagDialogOpen] = useState(false)
  const [batchTag, setBatchTag] = useState<string>('')
  const [batchRenameDialogOpen, setBatchRenameDialogOpen] = useState(false)
  const [batchRenameText, setBatchRenameText] = useState<string>('')
  const [findText, setFindText] = useState<string>('')
  const [replaceText, setReplaceText] = useState<string>('')
  const [prefixText, setPrefixText] = useState<string>('')
  const [suffixText, setSuffixText] = useState<string>('')

  // Clash 配置编辑状态
  const [clashDialogOpen, setClashDialogOpen] = useState(false)
  const [editingClashConfig, setEditingClashConfig] = useState<{ nodeId: number; config: string } | null>(null)
  const [clashConfigError, setClashConfigError] = useState<string>('')
  const [jsonErrorLines, setJsonErrorLines] = useState<number[]>([])

  // URI 复制状态
  const [uriDialogOpen, setUriDialogOpen] = useState(false)
  const [uriContent, setUriContent] = useState<string>('')

  // 临时订阅状态
  const [tempSubDialogOpen, setTempSubDialogOpen] = useState(false)
  const [tempSubMaxAccess, setTempSubMaxAccess] = useState<number>(1)
  const [tempSubExpireSeconds, setTempSubExpireSeconds] = useState<number>(60)
  const [tempSubUrl, setTempSubUrl] = useState<string>('')
  const [tempSubGenerating, setTempSubGenerating] = useState(false)
  const [tempSubSingleNodeId, setTempSubSingleNodeId] = useState<number | null>(null) // 单个节点模式

  // 添加地区 emoji 状态
  const [addingRegionEmoji, setAddingRegionEmoji] = useState(false)
  const [addingEmojiForNode, setAddingEmojiForNode] = useState<number | null>(null)

  // 删除重复节点状态
  const [duplicateDialogOpen, setDuplicateDialogOpen] = useState(false)
  const [duplicateGroups, setDuplicateGroups] = useState<Array<{ config: string; nodes: ParsedNode[] }>>([])
  const [deletingDuplicates, setDeletingDuplicates] = useState(false)

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

  // 节点排序状态
  const [nodeOrder, setNodeOrder] = useState<number[]>([])
  // 批量拖动状态：当拖动选中的节点时，记录正在批量拖动的节点ID集合
  const [batchDraggingIds, setBatchDraggingIds] = useState<Set<number>>(new Set())
  // 当前正在拖动的节点ID（用于 DragOverlay）
  const [activeId, setActiveId] = useState<string | null>(null)
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
        node_order: number[]
      }
    },
    enabled: Boolean(auth.accessToken),
  })

  // 同步 nodeOrder 状态
  useEffect(() => {
    if (userConfig?.node_order) {
      setNodeOrder(userConfig.node_order)
    }
  }, [userConfig?.node_order])

  // dnd-kit sensors
  // 移动端需要更长的 delay 以允许正常滚动，只有长按才触发拖拽
  const sensors = useSensors(
    useSensor(PointerSensor, {
      activationConstraint: { distance: 8 },
    }),
    useSensor(TouchSensor, {
      activationConstraint: { delay: 500, tolerance: 8 },
    })
  )

  // 更新节点排序
  const updateNodeOrderMutation = useMutation({
    mutationFn: async (newOrder: number[]) => {
      await api.put('/api/user/config', {
        ...userConfig,
        node_order: newOrder
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['user-config'] })
    },
    onError: (error: any) => {
      toast.error('保存排序失败: ' + (error.response?.data?.error || error.message))
    }
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

  // 批量修改节点名称
  const batchRenameMutation = useMutation({
    mutationFn: async (updates: Array<{ node_id: number; new_name: string }>) => {
      const response = await api.post('/api/admin/nodes/batch-rename', { updates })
      return response.data
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success(`成功修改 ${data.success} 个节点名称`)
      setBatchRenameDialogOpen(false)
      setSelectedNodeIds(new Set())
      setBatchRenameText('')
      setFindText('')
      setReplaceText('')
      setPrefixText('')
      setSuffixText('')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '批量修改名称失败')
    },
  })

  // 批量添加地区 emoji
  const handleAddRegionEmoji = useCallback(async () => {
    const nodeIds = Array.from(selectedNodeIds)
    if (nodeIds.length === 0) {
      toast.error('请先选择节点')
      return
    }

    setAddingRegionEmoji(true)
    let successCount = 0
    let skipCount = 0
    let failCount = 0

    try {
      for (const nodeId of nodeIds) {
        const node = savedNodes.find(n => n.id === nodeId)
        if (!node) continue

        // 检查节点名称是否已有 emoji 前缀
        if (hasEmojiPrefix(node.node_name)) {
          skipCount++
          continue
        }

        try {
          // 获取 server 地址
          let parsedConfig
          try {
            parsedConfig = JSON.parse(node.parsed_config)
          } catch {
            failCount++
            continue
          }

          const server = parsedConfig?.server
          if (!server) {
            failCount++
            continue
          }

          let ip = server

          // 如果是域名，先解析为 IP（优先 IPv4）
          if (!isIpAddress(server)) {
            try {
              const dnsResult = await api.get(`/api/dns/resolve?hostname=${encodeURIComponent(server)}`)
              const ips = dnsResult.data?.ips || []
              if (ips.length === 0) {
                failCount++
                continue
              }
              // 优先使用 IPv4（DNS 接口已经排序好）
              ip = ips[0]
            } catch {
              failCount++
              continue
            }
          }

          // 获取 IP 地理位置
          const geoInfo = await getGeoIPInfo(ip)
          if (!geoInfo.country_code) {
            failCount++
            continue
          }

          // 转换为旗帜 emoji
          const flag = countryCodeToFlag(geoInfo.country_code)
          if (!flag) {
            failCount++
            continue
          }

          // 更新节点名称
          const newName = `${flag} ${node.node_name}`
          const updatedParsedConfig = updateConfigName(node.parsed_config, newName)
          const updatedClashConfig = updateConfigName(node.clash_config, newName)

          await api.put(`/api/admin/nodes/${nodeId}`, {
            raw_url: node.raw_url,
            node_name: newName,
            protocol: node.protocol,
            parsed_config: updatedParsedConfig,
            clash_config: updatedClashConfig,
            enabled: node.enabled,
            tag: node.tag,
          })

          successCount++
        } catch (error) {
          console.error(`Failed to add emoji for node ${nodeId}:`, error)
          failCount++
        }
      }

      // 刷新节点列表
      queryClient.invalidateQueries({ queryKey: ['nodes'] })

      // 显示结果
      if (successCount > 0 && failCount === 0 && skipCount === 0) {
        toast.success(`成功为 ${successCount} 个节点添加地区 emoji`)
      } else {
        const parts = []
        if (successCount > 0) parts.push(`成功 ${successCount}`)
        if (skipCount > 0) parts.push(`跳过 ${skipCount} (已有emoji)`)
        if (failCount > 0) parts.push(`失败 ${failCount}`)
        toast.info(parts.join('，'))
      }
    } finally {
      setAddingRegionEmoji(false)
    }
  }, [selectedNodeIds, savedNodes, queryClient])

  // 为单个节点添加地区 emoji
  const handleAddSingleNodeEmoji = useCallback(async (nodeId: number) => {
    const node = savedNodes.find(n => n.id === nodeId)
    if (!node) return

    // 检查节点名称是否已有 emoji 前缀
    if (hasEmojiPrefix(node.node_name)) {
      toast.info('该节点已有 emoji 前缀')
      return
    }

    setAddingEmojiForNode(nodeId)

    try {
      // 获取 server 地址
      let parsedConfig
      try {
        parsedConfig = JSON.parse(node.parsed_config)
      } catch {
        toast.error('无法解析节点配置')
        return
      }

      const server = parsedConfig?.server
      if (!server) {
        toast.error('节点配置中没有 server 地址')
        return
      }

      let ip = server

      // 如果是域名，先解析为 IP（优先 IPv4）
      if (!isIpAddress(server)) {
        try {
          const dnsResult = await api.get(`/api/dns/resolve?hostname=${encodeURIComponent(server)}`)
          const ips = dnsResult.data?.ips || []
          if (ips.length === 0) {
            toast.error('DNS 解析失败')
            return
          }
          ip = ips[0]
        } catch {
          toast.error('DNS 解析失败')
          return
        }
      }

      // 获取 IP 地理位置
      const geoInfo = await getGeoIPInfo(ip)
      if (!geoInfo.country_code) {
        toast.error('获取地理位置失败')
        return
      }

      // 转换为旗帜 emoji
      const flag = countryCodeToFlag(geoInfo.country_code)
      if (!flag) {
        toast.error('无法生成旗帜 emoji')
        return
      }

      // 更新节点名称
      const newName = `${flag} ${node.node_name}`
      const updatedParsedConfig = updateConfigName(node.parsed_config, newName)
      const updatedClashConfig = updateConfigName(node.clash_config, newName)

      await api.put(`/api/admin/nodes/${nodeId}`, {
        raw_url: node.raw_url,
        node_name: newName,
        protocol: node.protocol,
        parsed_config: updatedParsedConfig,
        clash_config: updatedClashConfig,
        enabled: node.enabled,
        tag: node.tag,
      })

      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success('已添加地区 emoji')
    } catch (error) {
      console.error('Failed to add emoji:', error)
      toast.error('添加 emoji 失败')
    } finally {
      setAddingEmojiForNode(null)
    }
  }, [savedNodes, queryClient])

  // 查找重复节点
  const findDuplicateNodes = useCallback(() => {
    if (savedNodes.length === 0) {
      toast.info('没有节点')
      return
    }

    // 按 clash_config + node_name 分组（只有连接配置和名称都相同才算重复）
    const configGroups = new Map<string, ParsedNode[]>()

    for (const node of savedNodes) {
      try {
        // 解析配置并按 key 排序，同时加上 node_name 作为唯一标识的一部分
        const config = JSON.parse(node.clash_config)
        // 使用数据库中的 node_name（用户可能修改过）而不是配置中的 name
        const configKey = JSON.stringify({
          ...config,
          __node_name__: node.node_name // 使用特殊 key 避免与配置字段冲突
        }, Object.keys({ ...config, __node_name__: node.node_name }).sort())

        if (!configGroups.has(configKey)) {
          configGroups.set(configKey, [])
        }
        configGroups.get(configKey)!.push(node)
      } catch {
        // 无法解析的配置，使用原始字符串 + node_name
        const configKey = node.clash_config + '|' + node.node_name
        if (!configGroups.has(configKey)) {
          configGroups.set(configKey, [])
        }
        configGroups.get(configKey)!.push(node)
      }
    }

    // 过滤出有重复的组
    const duplicates: Array<{ config: string; nodes: ParsedNode[] }> = []
    for (const [config, nodes] of configGroups) {
      if (nodes.length > 1) {
        duplicates.push({ config, nodes })
      }
    }

    if (duplicates.length === 0) {
      toast.success('没有发现重复节点')
      return
    }

    setDuplicateGroups(duplicates)
    setDuplicateDialogOpen(true)
  }, [savedNodes])

  // 删除重复节点（保留每组的第一个）
  const handleDeleteDuplicates = useCallback(async () => {
    if (duplicateGroups.length === 0) return

    // 收集所有要删除的节点 ID（每组保留第一个，删除其余）
    const nodeIdsToDelete: number[] = []
    for (const group of duplicateGroups) {
      // 按创建时间排序，保留最早创建的
      const sortedNodes = [...group.nodes].sort(
        (a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
      )
      // 跳过第一个，删除其余
      for (let i = 1; i < sortedNodes.length; i++) {
        nodeIdsToDelete.push(sortedNodes[i].id)
      }
    }

    if (nodeIdsToDelete.length === 0) {
      toast.info('没有需要删除的节点')
      return
    }

    setDeletingDuplicates(true)
    try {
      await api.post('/api/admin/nodes/batch-delete', { node_ids: nodeIdsToDelete })
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      toast.success(`成功删除 ${nodeIdsToDelete.length} 个重复节点`)
      setDuplicateDialogOpen(false)
      setDuplicateGroups([])
    } catch (error: any) {
      toast.error(error.response?.data?.error || '删除失败')
    } finally {
      setDeletingDuplicates(false)
    }
  }, [duplicateGroups, queryClient])

  // 生成临时订阅 (支持单个节点或批量模式)
  const generateTempSubscription = useCallback(async (singleNodeId?: number) => {
    const nodeIds = singleNodeId !== undefined ? [singleNodeId] : Array.from(selectedNodeIds)
    if (nodeIds.length === 0) {
      toast.error('请先选择节点')
      return
    }

    setTempSubGenerating(true)
    try {
      // 获取节点的 clash 配置
      const nodesData = savedNodes.filter(n => nodeIds.includes(n.id))
      const proxies = nodesData.map(node => {
        try {
          return JSON.parse(node.clash_config)
        } catch {
          return null
        }
      }).filter(Boolean)

      if (proxies.length === 0) {
        toast.error('无法解析节点的配置')
        return
      }

      const response = await api.post('/api/admin/temp-subscription', {
        proxies,
        max_access: tempSubMaxAccess,
        expire_seconds: tempSubExpireSeconds,
      })

      const fullUrl = `${window.location.origin}${response.data.url}`
      setTempSubUrl(fullUrl)
    } catch (error: any) {
      toast.error(error.response?.data?.error || '生成临时订阅失败')
    } finally {
      setTempSubGenerating(false)
    }
  }, [selectedNodeIds, savedNodes, tempSubMaxAccess, tempSubExpireSeconds])

  // 自动生成临时订阅：Dialog 打开时或参数变化时自动生成
  useEffect(() => {
    if (tempSubDialogOpen) {
      // 使用 setTimeout 来 debounce，避免频繁请求
      const timer = setTimeout(() => {
        generateTempSubscription(tempSubSingleNodeId ?? undefined)
      }, 300)
      return () => clearTimeout(timer)
    }
  }, [tempSubDialogOpen, tempSubMaxAccess, tempSubExpireSeconds, tempSubSingleNodeId])

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
      return response.data as { proxies: ClashProxy[]; count: number; suggested_tag?: string }
    },
    onSuccess: async (data, variables) => {
      // 优先使用后端返回的 suggested_tag（从 Content-Disposition 提取）
      // 其次使用 URL hostname
      let defaultTag = data.suggested_tag || ''
      if (!defaultTag) {
        try {
          const urlObj = new URL(variables.url)
          defaultTag = urlObj.hostname || '外部订阅'
        } catch {
          defaultTag = '外部订阅'
        }
      }

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

      // 如果用户没有设置标签，自动使用 suggested_tag 或服务器地址作为标签
      if (!subscriptionTag.trim()) {
        setSubscriptionTag(defaultTag)
      }

      toast.success(`成功导入 ${data.count} 个节点`)

      // 保存外部订阅链接
      try {
        // 使用 defaultTag（优先从 Content-Disposition 提取，其次使用域名）
        await api.post('/api/user/external-subscriptions', {
          name: defaultTag,
          url: variables.url,
          user_agent: variables.userAgent, // 保存 User-Agent
        })
        // 刷新外部订阅列表和流量数据
        queryClient.invalidateQueries({ queryKey: ['external-subscriptions'] })
        queryClient.invalidateQueries({ queryKey: ['traffic-summary'] })
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

    // 按 nodeOrder 排序已保存的节点
    const orderMap = new Map<number, number>()
    nodeOrder.forEach((id, index) => orderMap.set(id, index))

    const sortedSaved = [...saved].sort((a, b) => {
      const aOrder = orderMap.get(a.dbId) ?? Infinity
      const bOrder = orderMap.get(b.dbId) ?? Infinity
      return aOrder - bOrder
    })

    // 临时节点在前，已保存节点按排序顺序在后
    return [...temp, ...sortedSaved]
  }, [savedNodes, tempNodes, nodeOrder])

  // 拖拽开始处理：检测是否批量拖动
  const handleDragStart = useCallback((event: DragStartEvent) => {
    // 锁定 body 滚动
    document.body.style.overflow = 'hidden'
    document.body.style.touchAction = 'none'

    const { active } = event
    setActiveId(active.id as string)

    const savedDisplayNodes = displayNodes.filter(n => n.isSaved && n.dbId)
    const activeNode = savedDisplayNodes.find(n => n.id === active.id)

    // 如果拖动的节点在选中集合中，且选中了多个节点，则是批量拖动
    if (activeNode?.dbId && selectedNodeIds.has(activeNode.dbId) && selectedNodeIds.size > 1) {
      setBatchDraggingIds(new Set(selectedNodeIds))
    } else {
      setBatchDraggingIds(new Set())
    }
  }, [displayNodes, selectedNodeIds])

  // 拖拽结束处理（支持批量拖动）
  const handleDragEnd = useCallback((event: DragEndEvent) => {
    // 恢复 body 滚动
    document.body.style.overflow = ''
    document.body.style.touchAction = ''

    const { active, over } = event

    // 清除拖动状态（无论结果如何都要清除）
    setActiveId(null)
    setBatchDraggingIds(new Set())

    if (!over || active.id === over.id) return

    // 获取当前显示的已保存节点（按当前顺序）
    const savedDisplayNodes = displayNodes.filter(n => n.isSaved && n.dbId)
    const activeNode = savedDisplayNodes.find(n => n.id === active.id)
    if (!activeNode) return

    const overIndex = savedDisplayNodes.findIndex(n => n.id === over.id)
    if (overIndex === -1) return

    // 判断是否批量拖动：拖拽的节点在选中集合中，且选中了多个节点
    const isDraggingSelected = activeNode.dbId && selectedNodeIds.has(activeNode.dbId)

    if (isDraggingSelected && selectedNodeIds.size > 1) {
      // 批量拖动逻辑
      const targetNode = savedDisplayNodes[overIndex]

      // 如果目标也是选中的节点，忽略操作
      if (targetNode.dbId && selectedNodeIds.has(targetNode.dbId)) return

      // 获取选中节点的ID（保持当前显示顺序）
      const selectedIds = savedDisplayNodes
        .filter(n => n.dbId && selectedNodeIds.has(n.dbId))
        .map(n => n.dbId!)

      // 获取未选中的节点
      const unselectedNodes = savedDisplayNodes.filter(n => !n.dbId || !selectedNodeIds.has(n.dbId))

      // 计算在目标位置之前还是之后插入
      const activeIndex = savedDisplayNodes.findIndex(n => n.id === active.id)
      const insertAfter = activeIndex < overIndex

      // 重新排列：将选中的节点作为整体插入到目标位置
      const newOrder: number[] = []
      for (const node of unselectedNodes) {
        if (node.dbId === targetNode.dbId && !insertAfter) {
          // 在目标之前插入
          newOrder.push(...selectedIds)
        }
        newOrder.push(node.dbId!)
        if (node.dbId === targetNode.dbId && insertAfter) {
          // 在目标之后插入
          newOrder.push(...selectedIds)
        }
      }

      setNodeOrder(newOrder)
      updateNodeOrderMutation.mutate(newOrder)
    } else {
      // 单节点拖动（保持原有逻辑）
      const activeIndex = savedDisplayNodes.findIndex(n => n.id === active.id)
      if (activeIndex === -1) return

      const currentIds = savedDisplayNodes.map(n => n.dbId!)
      const newOrderIds = arrayMove(currentIds, activeIndex, overIndex)

      setNodeOrder(newOrderIds)
      updateNodeOrderMutation.mutate(newOrderIds)
    }
  }, [displayNodes, selectedNodeIds, updateNodeOrderMutation])

  // 拖拽取消处理
  const handleDragCancel = useCallback(() => {
    setActiveId(null)
    setBatchDraggingIds(new Set())
  }, [])

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

  // 获取要在 DragOverlay 中显示的节点
  const dragOverlayNodes = useMemo(() => {
    if (!activeId) return []

    const activeNode = filteredNodes.find(n => n.id === activeId)
    if (!activeNode) return []

    // 如果是批量拖动，返回所有选中的节点
    if (activeNode.dbId && selectedNodeIds.has(activeNode.dbId) && selectedNodeIds.size > 1) {
      return filteredNodes.filter(n => n.dbId && selectedNodeIds.has(n.dbId))
    }

    // 单节点拖动
    return [activeNode]
  }, [activeId, filteredNodes, selectedNodeIds])

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

  // 当选中的筛选器对应的节点都被删除时，自动重置为 'all'
  useEffect(() => {
    // 检查 tagFilter
    if (tagFilter !== 'all' && (!tagCounts[tagFilter] || tagCounts[tagFilter] === 0)) {
      setTagFilter('all')
    }
    // 检查 selectedProtocol
    if (selectedProtocol !== 'all' && (!protocolCounts[selectedProtocol] || protocolCounts[selectedProtocol] === 0)) {
      setSelectedProtocol('all')
    }
  }, [tagCounts, protocolCounts, tagFilter, selectedProtocol])

  return (
    <div className='min-h-svh bg-background'>
      <Topbar />
      <main className='mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24'>
        <section className='space-y-4'>
          <div>
            <h1 className='text-3xl font-semibold tracking-tight'>节点管理</h1>
            <p className='text-muted-foreground mt-2'>
              输入代理节点信息，每行一个节点，支持 VMess、VLESS、Trojan、Shadowsocks、Hysteria、Socks、TUIC、AnyTLS、WireGuard 协议。
            </p>
          </div>

          <Collapsible open={isInputCardExpanded} onOpenChange={setIsInputCardExpanded}>
            <Card>
              <CollapsibleTrigger asChild>
                <CardHeader className='cursor-pointer hover:bg-muted/50 transition-colors rounded-t-lg'>
                  <div className='flex items-center justify-between'>
                    <CardTitle>导入节点</CardTitle>
                    <div className='p-1.5 transition-all duration-200'>
                      <ChevronDown className={cn(
                        'h-5 w-5 transition-transform duration-200',
                        isInputCardExpanded ? 'rotate-180' : 'animate-bounce'
                      )} />
                    </div>
                  </div>
                </CardHeader>
              </CollapsibleTrigger>
              <CollapsibleContent className='CollapsibleContent'>
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
              </CollapsibleContent>
            </Card>
          </Collapsible>

          {displayNodes.length > 0 && (
            <Card>
              <CardHeader>
                <div className='flex flex-col gap-4 md:flex-row md:items-center md:justify-between'>
                  <div>
                    <CardTitle>节点列表 ({filteredNodes.length})</CardTitle>
                    <p className='mt-2 text-sm font-semibold text-destructive'>注意!!! 节点的修改与删除均会同步更新所有订阅 </p>
                    <p className='mt-2 text-xs text-primary flex flex-wrap items-center gap-1'>
                      <Pencil className='h-4 w-4 inline' /> 编辑节点名称，
                      <img src={ExchangeIcon} alt='链式代理' className='h-4 w-4 inline [filter:invert(63%)_sepia(45%)_saturate(1068%)_hue-rotate(327deg)_brightness(95%)_contrast(88%)]' /> 创建链式代理，
                      <Activity className='h-4 w-4 inline' /> 绑定探针，
                      <Flag className='h-4 w-4 inline' /> 添加地区emoji，
                      <img src={IpIcon} alt='解析IP地址' className='h-4 w-4 inline [filter:invert(63%)_sepia(45%)_saturate(1068%)_hue-rotate(327deg)_brightness(95%)_contrast(88%)]' /> 解析IP地址，
                      <Undo2 className='h-4 w-4 inline' /> 恢复原始域名，
                      <Eye className='h-4 w-4 inline' /> 查看修改配置，
                      <Copy className='h-4 w-4 inline' /> 复制URI，
                      <Link2 className='h-4 w-4 inline' /> 生成临时订阅
                    </p>
                  </div>
                  <div className='flex flex-wrap gap-2 justify-end'>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => {
                        toast.promise(
                          api.post('/api/admin/sync-external-subscriptions'),
                          {
                            loading: '正在同步外部订阅...',
                            success: (response) => {
                              queryClient.invalidateQueries({ queryKey: ['nodes'] })
                              return response.data.message || '外部订阅同步成功'
                            },
                            error: (error) => error.response?.data?.error || '同步失败'
                          }
                        )
                      }}
                    >
                      同步外部订阅
                    </Button>
                    {selectedNodeIds.size > 0 && (
                      <>
                        <Button
                          variant='default'
                          size='sm'
                          onClick={handleAddRegionEmoji}
                          disabled={addingRegionEmoji}
                        >
                          {addingRegionEmoji ? '添加中...' : `添加emoji (${selectedNodeIds.size})`}
                        </Button>
                        <Button
                          variant='default'
                          size='sm'
                          onClick={() => {
                            // 获取选中节点的名称
                            const selectedNodes = savedNodes.filter(n => selectedNodeIds.has(n.id))
                            const names = selectedNodes.map(n => n.node_name).join('\n')
                            setBatchRenameText(names)
                            setBatchRenameDialogOpen(true)
                          }}
                        >
                          修改名称 ({selectedNodeIds.size})
                        </Button>
                        <Button
                          variant='default'
                          size='sm'
                          onClick={() => setBatchTagDialogOpen(true)}
                        >
                          修改标签 ({selectedNodeIds.size})
                        </Button>
                        <Button
                          variant='secondary'
                          size='sm'
                          onClick={() => {
                            setTempSubSingleNodeId(null) // 批量模式
                            setTempSubUrl('')
                            setTempSubDialogOpen(true)
                          }}
                        >
                          生成临时订阅 ({selectedNodeIds.size})
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
                    {savedNodes.length > 0 && (
                      <Button
                        variant='outline'
                        size='sm'
                        onClick={findDuplicateNodes}
                      >
                        删除重复
                      </Button>
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

                {/* 移动端卡片视图 (<768px) */}
                {!isTablet && (
                <DndContext
                  sensors={sensors}
                  collisionDetection={closestCenter}
                  onDragStart={handleDragStart}
                  onDragEnd={handleDragEnd}
                  onDragCancel={handleDragCancel}
                >
                  <SortableContext
                    items={filteredNodes.map(n => n.id)}
                    strategy={verticalListSortingStrategy}
                  >
                    <div className='space-y-3'>
                      {filteredNodes.length === 0 ? (
                        <Card>
                          <CardContent className='text-center text-muted-foreground py-8'>
                            没有找到匹配的节点
                          </CardContent>
                        </Card>
                      ) : (
                        filteredNodes.map(node => (
                          <SortableCard
                            key={node.id}
                            id={node.id}
                            isSaved={node.isSaved}
                            dbId={node.dbId}
                            batchDraggingIds={batchDraggingIds}
                            isSelected={node.isSaved && node.dbId ? selectedNodeIds.has(node.dbId) : false}
                            onClick={() => {
                              if (node.isSaved && node.dbId) {
                                const newSet = new Set(selectedNodeIds)
                                if (selectedNodeIds.has(node.dbId)) {
                                  newSet.delete(node.dbId)
                                } else {
                                  newSet.add(node.dbId)
                                }
                                setSelectedNodeIds(newSet)
                              }
                            }}
                          >
                            <CardContent className='p-3 space-y-2'>
                              {/* 头部：协议、节点名称、已保存标签 */}
                              <div className='flex items-start justify-between gap-2'>
                                <div className='flex-1 min-w-0'>
                                  <div className='flex items-center gap-2 mb-1'>
                                    {node.isSaved && (
                                      <DragHandle id={node.id} size='large' />
                                    )}
                                    {node.isSaved && node.dbId && (
                                      <Checkbox
                                        className='hidden sm:flex'
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
                                {node.isSaved && (
                                  <Check className='size-4 text-green-600' />
                                )}
                              </div>
                              {/* 节点名称 */}
                              {editingNode?.id === node.id ? (
                                <div className='flex items-center gap-1' onClick={(e) => e.stopPropagation()}>
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
                                    className='h-7 flex-1 min-w-0'
                                    autoFocus
                                  />
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-emerald-600 shrink-0'
                                    onClick={() => handleNameEditSubmit(node)}
                                    disabled={node.isSaved ? updateNodeNameMutation.isPending : false}
                                  >
                                    <Check className='size-3.5' />
                                  </Button>
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-muted-foreground shrink-0'
                                    onClick={handleNameEditCancel}
                                  >
                                    <X className='size-3.5' />
                                  </Button>
                                </div>
                              ) : (
                                <div className='font-medium text-sm break-all line-clamp-2'><Twemoji>{node.name || '未知'}</Twemoji></div>
                              )}
                            </div>
                            {/* 编辑、交换和探针绑定按钮 */}
                            {editingNode?.id !== node.id && (
                              <div className='flex items-center gap-1 shrink-0' onClick={(e) => e.stopPropagation()}>
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
                                    className='size-7 text-[#d97757] hover:text-[#c66647]'
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
                                {userConfig?.enable_probe_binding && node.isSaved && node.dbNode && (
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-[#d97757] hover:text-[#c66647]'
                                    onClick={() => {
                                      setSelectedNodeForProbe(node.dbNode!)
                                      setProbeBindingDialogOpen(true)
                                      refetchProbeConfig()
                                    }}
                                  >
                                    <Activity className={`size-4 ${node.dbNode.probe_server ? 'text-green-600' : ''}`} />
                                  </Button>
                                )}
                                {node.isSaved && node.dbNode && !hasEmojiPrefix(node.name) && (
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <Button
                                        variant='ghost'
                                        size='icon'
                                        className='size-7 text-[#d97757] hover:text-[#c66647]'
                                        onClick={() => handleAddSingleNodeEmoji(node.dbNode!.id)}
                                        disabled={addingEmojiForNode === node.dbNode!.id}
                                      >
                                        <Flag className={`size-4 ${addingEmojiForNode === node.dbNode!.id ? 'animate-pulse' : ''}`} />
                                      </Button>
                                    </TooltipTrigger>
                                    <TooltipContent>添加地区 emoji</TooltipContent>
                                  </Tooltip>
                                )}
                                {node.isSaved && node.dbId && (
                                  <Tooltip>
                                    <TooltipTrigger asChild>
                                      <Button
                                        variant='ghost'
                                        size='icon'
                                        className='size-7 text-[#d97757] hover:text-[#c66647]'
                                        onClick={() => {
                                          setTempSubSingleNodeId(node.dbId!)
                                          setTempSubUrl('')
                                          setTempSubDialogOpen(true)
                                        }}
                                      >
                                        <Link2 className='size-4' />
                                      </Button>
                                    </TooltipTrigger>
                                    <TooltipContent>生成临时订阅</TooltipContent>
                                  </Tooltip>
                                )}
                              </div>
                            )}
                          </div>

                          {/* 服务器地址和标签 */}
                          <div className='space-y-1.5'>
                            {node.parsed && (
                              <div className='flex items-center gap-2 flex-wrap text-xs'>
                                <span className='text-muted-foreground shrink-0'>地址:</span>
                                <span className='font-mono break-all'>{node.parsed.server}:{node.parsed.port}</span>
                                {node.parsed.network && node.parsed.network !== 'tcp' && (
                                  <Badge variant='outline' className='text-xs'>
                                    {node.parsed.network}
                                  </Badge>
                                )}
                              </div>
                            )}
                            <div className='flex items-center gap-2 flex-wrap text-xs'>
                              <span className='text-muted-foreground shrink-0'>标签:</span>
                              <Badge variant='secondary' className='text-xs'>
                                {node.dbNode?.tag || node.tag || (currentTag === 'manual' ? manualTag.trim() || '手动输入' : currentTag === 'subscription' ? subscriptionTag.trim() || '订阅导入' : '未知')}
                              </Badge>
                              {node.isSaved && node.dbNode?.probe_server && (
                                <Badge variant='secondary' className='text-xs flex items-center gap-1'>
                                  <Activity className='size-3' />
                                  {node.dbNode.probe_server}
                                </Badge>
                              )}
                            </div>
                          </div>

                          {/* 操作按钮组 */}
                          <div className='flex items-center justify-center gap-2 pt-2 border-t' onClick={(e) => e.stopPropagation()}>
                            {node.clash && (
                              <Button
                                variant='outline'
                                size='sm'
                                className='flex-1'
                                onClick={() => {
                                  if (node.isSaved && node.dbNode) {
                                    handleEditClashConfig(node.dbNode)
                                  } else if (!node.isSaved) {
                                    handleEditClashConfig(node)
                                  }
                                }}
                              >
                                <Eye className='size-4 mr-1' />
                                配置
                              </Button>
                            )}
                            {node.clash && node.isSaved && (
                              <Button
                                variant='outline'
                                size='sm'
                                className='flex-1'
                                onClick={() => node.isSaved && handleCopyUri(node.dbNode!)}
                              >
                                <Copy className='size-4 mr-1' />
                                复制
                              </Button>
                            )}
                            <AlertDialog>
                              <AlertDialogTrigger asChild>
                                <Button
                                  variant='outline'
                                  size='sm'
                                  className='flex-1 text-destructive hover:text-destructive hover:bg-destructive/10'
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
                          </div>
                        </CardContent>
                      </SortableCard>
                    ))
                  )}
                    </div>
                  </SortableContext>
                  {createPortal(
                    <DragOverlay dropAnimation={null}>
                      {activeId && (
                        <DragOverlayContent nodes={dragOverlayNodes} protocolColors={PROTOCOL_COLORS} />
                      )}
                    </DragOverlay>,
                    document.body
                  )}
                </DndContext>
                )}

                {/* 平板端和桌面端共享 DndContext */}
                <DndContext
                  sensors={sensors}
                  collisionDetection={closestCenter}
                  onDragStart={handleDragStart}
                  onDragEnd={handleDragEnd}
                  onDragCancel={handleDragCancel}
                >
                  {/* 平板端表格视图 (768-1024px) - 和桌面一致，但服务器地址显示在节点名称下方 */}
                  {isTablet && !isDesktop && (
                  <div className='rounded-md border'>
                    <SortableContext
                      items={filteredNodes.map(n => n.id)}
                      strategy={verticalListSortingStrategy}
                    >
                    <Table className='w-full'>
                      <TableHeader>
                        <TableRow>
                          <TableHead style={{ width: '36px' }}></TableHead>
                          <TableHead style={{ width: '60px' }}>协议</TableHead>
                          <TableHead>节点名称</TableHead>
                          <TableHead style={{ width: '100px' }}>标签</TableHead>
                          <TableHead style={{ width: '70px' }} className='text-center'>配置</TableHead>
                        <TableHead style={{ width: '70px' }} className='text-center'>操作</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredNodes.length === 0 ? (
                        <TableRow>
                          <TableCell colSpan={6} className='text-center text-muted-foreground py-8'>
                            没有找到匹配的节点
                          </TableCell>
                        </TableRow>
                      ) : (
                        filteredNodes.map(node => (
                          <SortableTableRow
                            key={node.id}
                            id={node.id}
                            isSaved={node.isSaved}
                            dbId={node.dbId}
                            batchDraggingIds={batchDraggingIds}
                            isSelected={node.isSaved && node.dbId ? selectedNodeIds.has(node.dbId) : false}
                            onClick={(e) => {
                              const target = e.target as HTMLElement
                              if (target.closest('button, input, [role="checkbox"], [data-drag-handle]')) {
                                return
                              }
                              if (node.isSaved && node.dbId) {
                                const newSet = new Set(selectedNodeIds)
                                if (newSet.has(node.dbId)) {
                                  newSet.delete(node.dbId)
                                } else {
                                  newSet.add(node.dbId)
                                }
                                setSelectedNodeIds(newSet)
                              }
                            }}
                          >
                            <TableCell className='w-9 px-2'>
                              {node.isSaved && (
                                <DragHandle id={node.id} />
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
                            <TableCell className='font-medium min-w-[200px] max-w-[300px]'>
                              {editingNode?.id === node.id ? (
                                <div className='min-w-0'>
                                  <div className='flex items-center gap-1'>
                                    <Input
                                      value={editingNode.value}
                                      onChange={(e) => handleNameEditChange(e.target.value)}
                                      onKeyDown={(e) => {
                                        if (e.key === 'Enter') {
                                          e.preventDefault()
                                          handleNameEditSubmit(node)
                                        } else if (e.key === 'Escape') {
                                          e.preventDefault()
                                          handleNameEditCancel()
                                        }
                                      }}
                                      className='h-7 flex-1 min-w-0'
                                      autoFocus
                                      onClick={(e) => e.stopPropagation()}
                                    />
                                    <Button
                                      variant='ghost'
                                      size='icon'
                                      className='size-7 text-emerald-600 shrink-0'
                                      onClick={() => handleNameEditSubmit(node)}
                                      disabled={node.isSaved ? updateNodeNameMutation.isPending : false}
                                    >
                                      <Check className='size-3.5' />
                                    </Button>
                                    <Button
                                      variant='ghost'
                                      size='icon'
                                      className='size-7 text-muted-foreground shrink-0'
                                      onClick={handleNameEditCancel}
                                    >
                                      <X className='size-3.5' />
                                    </Button>
                                  </div>
                                  {/* 编辑时也保留服务器地址显示，避免行高变化 */}
                                  {node.parsed && (
                                    <div className='flex items-center gap-1 mt-0.5 text-xs text-muted-foreground'>
                                      <span className='font-mono truncate'>{node.parsed.server}:{node.parsed.port}</span>
                                      {node.parsed.network && node.parsed.network !== 'tcp' && (
                                        <Badge variant='outline' className='text-xs shrink-0'>
                                          {node.parsed.network}
                                        </Badge>
                                      )}
                                    </div>
                                  )}
                                </div>
                              ) : (
                                <div className='flex items-center gap-2 min-w-0'>
                                  <div className='flex-1 min-w-0'>
                                    <div className='flex items-center gap-1'>
                                      <span className='truncate'><Twemoji>{node.name || '未知'}</Twemoji></span>
                                      {node.isSaved && (
                                        <Check className='size-4 text-green-600 shrink-0' />
                                      )}
                                    </div>
                                    {/* 服务器地址显示在节点名称下方 */}
                                    {node.parsed && (
                                      <div className='flex items-center gap-1 mt-0.5 text-xs text-muted-foreground'>
                                        <span className='font-mono truncate'>{node.parsed.server}:{node.parsed.port}</span>
                                        {node.parsed.network && node.parsed.network !== 'tcp' && (
                                          <Badge variant='outline' className='text-xs shrink-0'>
                                            {node.parsed.network}
                                          </Badge>
                                        )}
                                      </div>
                                    )}
                                  </div>
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-[#d97757] hover:text-[#c66647] shrink-0'
                                    onClick={() => handleNameEditStart(node)}
                                    disabled={node.isSaved ? updateNodeNameMutation.isPending : false}
                                  >
                                    <Pencil className='size-4' />
                                  </Button>
                                  {node.isSaved && node.dbNode && !node.dbNode.protocol.includes('⇋') && (
                                    <Button
                                      variant='ghost'
                                      size='icon'
                                      className='size-7 text-[#d97757] hover:text-[#c66647] shrink-0'
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
                                  {node.isSaved && node.dbNode && !hasEmojiPrefix(node.name) && (
                                    <Tooltip>
                                      <TooltipTrigger asChild>
                                        <Button
                                          variant='ghost'
                                          size='icon'
                                          className='size-7 text-[#d97757] hover:text-[#c66647] shrink-0'
                                          onClick={() => handleAddSingleNodeEmoji(node.dbNode!.id)}
                                          disabled={addingEmojiForNode === node.dbNode!.id}
                                        >
                                          <Flag className={`size-4 ${addingEmojiForNode === node.dbNode!.id ? 'animate-pulse' : ''}`} />
                                        </Button>
                                      </TooltipTrigger>
                                      <TooltipContent>添加地区 emoji</TooltipContent>
                                    </Tooltip>
                                  )}
                                </div>
                              )}
                            </TableCell>
                            <TableCell>
                              <div className='flex flex-wrap gap-1'>
                                <Badge variant='secondary' className='text-xs max-w-[90px] truncate'>
                                  {node.dbNode?.tag || node.tag || (currentTag === 'manual' ? manualTag.trim() || '手动输入' : currentTag === 'subscription' ? subscriptionTag.trim() || '订阅导入' : '未知')}
                                </Badge>
                                {node.isSaved && node.dbNode?.probe_server && (
                                  <Badge variant='secondary' className='text-xs flex items-center gap-1'>
                                    <Activity className='size-3' />
                                  </Badge>
                                )}
                              </div>
                            </TableCell>
                            <TableCell className='text-center'>
                              {node.clash ? (
                                <div className='flex gap-1 justify-center'>
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='h-7 w-7'
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
                                  {node.isSaved && (
                                    <Button
                                      variant='ghost'
                                      size='icon'
                                      className='h-7 w-7'
                                      title='复制 URI'
                                      onClick={() => handleCopyUri(node.dbNode!)}
                                    >
                                      <Copy className='h-4 w-4' />
                                    </Button>
                                  )}
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
                                    className='h-7 text-xs'
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
                          </SortableTableRow>
                        ))
                      )}
                    </TableBody>
                  </Table>
                  </SortableContext>
                </div>
                  )}

                  {/* 桌面端表格视图 (>=1024px) */}
                  {isDesktop && (
                  <div className='rounded-md border'>
                    <SortableContext
                      items={filteredNodes.map(n => n.id)}
                      strategy={verticalListSortingStrategy}
                    >
                      <Table className='w-full'>
                        <TableHeader>
                          <TableRow>
                            <TableHead style={{ width: '36px' }}></TableHead>
                            <TableHead style={{ width: '90px' }}>协议</TableHead>
                            <TableHead>节点名称</TableHead>
                            <TableHead style={{ width: '120px' }}>标签</TableHead>
                            <TableHead>服务器地址</TableHead>
                            <TableHead style={{ width: '80px' }} className='text-center'>配置</TableHead>
                            <TableHead style={{ width: '80px' }} className='text-center'>操作</TableHead>
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
                              <SortableTableRow
                                key={node.id}
                                id={node.id}
                                isSaved={node.isSaved}
                                dbId={node.dbId}
                                batchDraggingIds={batchDraggingIds}
                                isSelected={node.isSaved && node.dbId ? selectedNodeIds.has(node.dbId) : false}
                                onClick={(e) => {
                                  // 如果点击的是按钮、输入框等交互元素，不触发选中
                                  const target = e.target as HTMLElement
                                  if (target.closest('button, input, [role="checkbox"], [data-drag-handle]')) {
                                    return
                                  }
                                  if (node.isSaved && node.dbId) {
                                    const newSet = new Set(selectedNodeIds)
                                    if (newSet.has(node.dbId)) {
                                      newSet.delete(node.dbId)
                                    } else {
                                      newSet.add(node.dbId)
                                    }
                                    setSelectedNodeIds(newSet)
                                  }
                                }}
                              >
                                <TableCell className='w-9 px-2'>
                                  {node.isSaved && (
                                    <DragHandle id={node.id} />
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
                            <TableCell className='font-medium min-w-[200px] max-w-[300px]'>
                              {editingNode?.id === node.id ? (
                                <div className='flex items-center gap-1'>
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
                                    className='h-7 flex-1 min-w-0'
                                    autoFocus
                                  />
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-emerald-600 shrink-0'
                                    onClick={() => handleNameEditSubmit(node)}
                                    disabled={node.isSaved ? updateNodeNameMutation.isPending : false}
                                  >
                                    <Check className='size-3.5' />
                                  </Button>
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-muted-foreground shrink-0'
                                    onClick={handleNameEditCancel}
                                  >
                                    <X className='size-3.5' />
                                  </Button>
                                </div>
                              ) : (
                                <div className='flex items-center gap-2 min-w-0'>
                                  <span className='truncate flex-1 min-w-0' title={node.name || '未知'}><Twemoji>{node.name || '未知'}</Twemoji></span>
                                  {node.isSaved && (
                                    <Check className='size-4 text-green-600 shrink-0' />
                                  )}
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='size-7 text-[#d97757] hover:text-[#c66647] shrink-0'
                                    onClick={() => handleNameEditStart(node)}
                                    disabled={node.isSaved ? updateNodeNameMutation.isPending : false}
                                  >
                                    <Pencil className='size-4' />
                                  </Button>
                                  {node.isSaved && node.dbNode && !node.dbNode.protocol.includes('⇋') && (
                                    <Button
                                      variant='ghost'
                                      size='icon'
                                      className='size-7 text-muted-foreground hover:text-foreground shrink-0'
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
                                  {node.isSaved && node.dbNode && !hasEmojiPrefix(node.name) && (
                                    <Tooltip>
                                      <TooltipTrigger asChild>
                                        <Button
                                          variant='ghost'
                                          size='icon'
                                          className='size-7 text-[#d97757] hover:text-[#c66647] shrink-0'
                                          onClick={() => handleAddSingleNodeEmoji(node.dbNode!.id)}
                                          disabled={addingEmojiForNode === node.dbNode!.id}
                                        >
                                          <Flag className={`size-4 ${addingEmojiForNode === node.dbNode!.id ? 'animate-pulse' : ''}`} />
                                        </Button>
                                      </TooltipTrigger>
                                      <TooltipContent>添加地区 emoji</TooltipContent>
                                    </Tooltip>
                                  )}
                                </div>
                              )}
                            </TableCell>
                            <TableCell>
                              <div className='flex flex-wrap gap-1'>
                                <Badge
                                  variant='secondary'
                                  className='text-xs max-w-[120px] truncate'
                                  title={node.dbNode?.tag || node.tag || (currentTag === 'manual' ? manualTag.trim() || '手动输入' : currentTag === 'subscription' ? subscriptionTag.trim() || '订阅导入' : '未知')}
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
                                  <div className='flex items-center gap-2 min-w-0'>
                                    <div className='min-w-0 flex-1'>
                                      <div className='font-mono truncate' title={`${node.parsed.server}:${node.parsed.port}`}>{node.parsed.server}:{node.parsed.port}</div>
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
                                              className='size-6 p-0 border border-orange-500/50 hover:border-orange-500 shrink-0'
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
                                                className='size-6 p-0 border border-primary/50 hover:border-primary shrink-0'
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
                                            className='size-6 p-0 border border-primary/50 hover:border-primary shrink-0'
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
                                        className='size-6 p-0 border border-primary/50 hover:border-primary ml-1 shrink-0'
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
                                        className='size-6 p-0 border border-primary/50 hover:border-primary ml-1 shrink-0'
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
                                        <Twemoji>{node.name || '未知'}</Twemoji>
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
                                <Button
                                  variant='ghost'
                                  size='icon'
                                  className='h-8 w-8'
                                  title='生成临时订阅'
                                  onClick={() => {
                                    if (node.isSaved && node.dbId) {
                                      setTempSubSingleNodeId(node.dbId)
                                      setTempSubUrl('')
                                      setTempSubDialogOpen(true)
                                    }
                                  }}
                                >
                                  <Link2 className='h-4 w-4' />
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
                              </SortableTableRow>
                            ))
                          )}
                        </TableBody>
                      </Table>
                    </SortableContext>
                  </div>
                  )}

                  {createPortal(
                    <DragOverlay dropAnimation={null}>
                      {activeId && (
                        <DragOverlayContent nodes={dragOverlayNodes} protocolColors={PROTOCOL_COLORS} />
                      )}
                    </DragOverlay>,
                    document.body
                  )}
                </DndContext>
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
            <DialogTitle>选择中转节点</DialogTitle>
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
                          <span className='font-medium'><Twemoji>{node.node_name}</Twemoji></span>
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

      {/* 批量修改名称对话框 */}
      <Dialog open={batchRenameDialogOpen} onOpenChange={setBatchRenameDialogOpen}>
        <DialogContent className='max-w-3xl max-h-[80vh] flex flex-col'>
          <DialogHeader>
            <DialogTitle>批量修改节点名称</DialogTitle>
            <DialogDescription>
              修改选中的 {selectedNodeIds.size} 个节点名称
            </DialogDescription>
          </DialogHeader>
          <div className='flex-1 space-y-4 py-4 min-h-0 flex flex-col'>
            {/* 搜索替换工具 */}
            <div className='grid grid-cols-3 gap-2 grid-cols-[1fr_1fr_auto] items-end'>
              <div className='space-y-2'>
                <Label htmlFor='find-text' className='text-sm font-medium'>
                  查找内容
                </Label>
                <Input
                  id='find-text'
                  placeholder='输入要查找的文本'
                  value={findText}
                  onChange={(e) => setFindText(e.target.value)}
                  className='text-sm'
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='replace-text' className='text-sm font-medium'>
                  替换为
                </Label>
                <div className='flex gap-2'>
                  <Input
                    id='replace-text'
                    placeholder='输入替换后的文本'
                    value={replaceText}
                    onChange={(e) => setReplaceText(e.target.value)}
                    className='text-sm'
                  />
                </div>
              </div>
              <Button
                size='sm'
                variant='outline'
                onClick={() => {
                  if (!findText) {
                    toast.error('请输入要查找的内容')
                    return
                  }
                  const replaced = batchRenameText.split('\n').map(line =>
                    line.replace(new RegExp(findText.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'g'), replaceText)
                  ).join('\n')
                  setBatchRenameText(replaced)
                  toast.success('替换完成')
                }}
                >
                替换
              </Button>
            </div>

            {/* 前缀后缀工具 */}
            <div className='grid grid-cols-3 gap-2 grid-cols-[1fr_1fr_auto] items-end'>
              <div className='space-y-2'>
                <Label htmlFor='prefix-text' className='text-sm font-medium'>
                  前缀
                </Label>
                <Input
                  id='prefix-text'
                  placeholder='添加到名称前面'
                  value={prefixText}
                  onChange={(e) => setPrefixText(e.target.value)}
                  className='text-sm'
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='suffix-text' className='text-sm font-medium'>
                  后缀
                </Label>
                <Input
                  id='suffix-text'
                  placeholder='添加到名称后面'
                  value={suffixText}
                  onChange={(e) => setSuffixText(e.target.value)}
                  className='text-sm'
                />
              </div>
              <Button
                size='sm'
                variant='outline'
                onClick={() => {
                  if (!prefixText && !suffixText) {
                    toast.error('请输入前缀或后缀')
                    return
                  }
                  const updated = batchRenameText.split('\n').map(line =>
                    line ? `${prefixText}${line}${suffixText}` : line
                  ).join('\n')
                  setBatchRenameText(updated)
                  setPrefixText('')
                  setSuffixText('')
                  toast.success('应用完成')
                }}
              >
                应用
              </Button>
            </div>

            {/* 名称编辑区 */}
            <div className='flex-1 space-y-2 min-h-0 flex flex-col'>
              <Label htmlFor='batch-rename-text' className='text-sm font-medium'>
                节点名称 (每行一个，共 {batchRenameText.split('\n').length} 行)
              </Label>
              <Textarea
                id='batch-rename-text'
                value={batchRenameText}
                onChange={(e) => setBatchRenameText(e.target.value)}
                className='font-mono text-sm flex-1 min-h-[300px] resize-none'
                placeholder='每行一个节点名称'
              />
              {/* <p className='text-xs text-muted-foreground'>
                支持多行编辑，使用上方的查找替换功能批量修改文本
              </p> */}
            </div>

            {/* 操作按钮 */}
            <div className='flex justify-end gap-2 pt-2'>
              <Button
                variant='outline'
                onClick={() => {
                  setBatchRenameDialogOpen(false)
                  setBatchRenameText('')
                  setFindText('')
                  setReplaceText('')
                  setPrefixText('')
                  setSuffixText('')
                }}
                disabled={batchRenameMutation.isPending}
              >
                取消
              </Button>
              <Button
                onClick={() => {
                  const newNames = batchRenameText.split('\n').map(line => line.trim()).filter(line => line)
                  const nodeIds = Array.from(selectedNodeIds)

                  if (newNames.length === 0) {
                    toast.error('请输入节点名称')
                    return
                  }

                  if (newNames.length !== nodeIds.length) {
                    toast.error(`名称数量 (${newNames.length}) 与选中节点数量 (${nodeIds.length}) 不匹配`)
                    return
                  }

                  // 构建更新请求
                  const updates = nodeIds.map((nodeId, index) => ({
                    node_id: nodeId,
                    new_name: newNames[index]
                  }))

                  batchRenameMutation.mutate(updates)
                }}
                disabled={batchRenameMutation.isPending || !batchRenameText.trim()}
              >
                {batchRenameMutation.isPending ? '保存中...' : '确认修改'}
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* 删除重复节点对话框 */}
      <Dialog open={duplicateDialogOpen} onOpenChange={setDuplicateDialogOpen}>
        <DialogContent className='max-w-2xl max-h-[80vh] flex flex-col'>
          <DialogHeader>
            <DialogTitle>删除重复节点</DialogTitle>
            <DialogDescription>
              发现 {duplicateGroups.length} 组重复节点，共 {duplicateGroups.reduce((sum, g) => sum + g.nodes.length - 1, 0)} 个重复节点将被删除（每组保留最早创建的节点）
            </DialogDescription>
          </DialogHeader>
          <div className='flex-1 overflow-y-auto space-y-4 py-4'>
            {duplicateGroups.map((group, groupIndex) => (
              <div key={groupIndex} className='border rounded-lg p-3 space-y-2'>
                <div className='flex items-center justify-between'>
                  <span className='text-sm font-medium'>
                    重复组 {groupIndex + 1}（{group.nodes.length} 个节点）
                  </span>
                  <Badge variant='secondary'>
                    将删除 {group.nodes.length - 1} 个
                  </Badge>
                </div>
                <div className='space-y-1'>
                  {[...group.nodes]
                    .sort((a, b) => new Date(a.created_at).getTime() - new Date(b.created_at).getTime())
                    .map((node, nodeIndex) => (
                      <div
                        key={node.id}
                        className={`flex items-center justify-between text-sm p-2 rounded ${
                          nodeIndex === 0
                            ? 'bg-green-500/10 border border-green-500/20'
                            : 'bg-red-500/10 border border-red-500/20'
                        }`}
                      >
                        <div className='flex items-center gap-2 flex-1 min-w-0'>
                          <Badge variant='outline' className='shrink-0'>
                            {node.protocol.toUpperCase()}
                          </Badge>
                          <span className='truncate'>{node.node_name}</span>
                          {node.tag && (
                            <Badge variant='secondary' className='shrink-0'>
                              {node.tag}
                            </Badge>
                          )}
                        </div>
                        <span className={`text-xs shrink-0 ml-2 ${nodeIndex === 0 ? 'text-green-600' : 'text-red-600'}`}>
                          {nodeIndex === 0 ? '保留' : '删除'}
                        </span>
                      </div>
                    ))}
                </div>
              </div>
            ))}
          </div>
          <div className='flex justify-end gap-2 pt-4 border-t'>
            <Button
              variant='outline'
              onClick={() => {
                setDuplicateDialogOpen(false)
                setDuplicateGroups([])
              }}
              disabled={deletingDuplicates}
            >
              取消
            </Button>
            <Button
              variant='destructive'
              onClick={handleDeleteDuplicates}
              disabled={deletingDuplicates}
            >
              {deletingDuplicates ? '删除中...' : `确认删除 ${duplicateGroups.reduce((sum, g) => sum + g.nodes.length - 1, 0)} 个重复节点`}
            </Button>
          </div>
        </DialogContent>
      </Dialog>

      {/* 临时订阅对话框 */}
      <Dialog
        open={tempSubDialogOpen}
        onOpenChange={(open) => {
          setTempSubDialogOpen(open)
          if (!open) {
            setTempSubUrl('')
            setTempSubSingleNodeId(null)
          }
        }}
      >
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>生成临时订阅</DialogTitle>
            <DialogDescription>
              {tempSubSingleNodeId !== null
                ? `为节点 "${savedNodes.find(n => n.id === tempSubSingleNodeId)?.node_name || '未知'}" 生成临时订阅链接`
                : `为选中的 ${selectedNodeIds.size} 个节点生成临时订阅链接`
              }
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='grid grid-cols-2 gap-4'>
              <div className='space-y-2'>
                <Label htmlFor='temp-sub-max-access' className='text-sm font-medium'>
                  访问次数
                </Label>
                <Input
                  id='temp-sub-max-access'
                  type='number'
                  min={1}
                  max={100}
                  value={tempSubMaxAccess}
                  onChange={(e) => setTempSubMaxAccess(parseInt(e.target.value) || 1)}
                  className='text-sm'
                />
              </div>
              <div className='space-y-2'>
                <Label htmlFor='temp-sub-expire' className='text-sm font-medium'>
                  过期时间（秒）
                </Label>
                <Input
                  id='temp-sub-expire'
                  type='number'
                  min={10}
                  max={3600}
                  value={tempSubExpireSeconds}
                  onChange={(e) => setTempSubExpireSeconds(parseInt(e.target.value) || 60)}
                  className='text-sm'
                />
              </div>
            </div>
            <div className='space-y-2'>
              <Label className='text-sm font-medium'>临时订阅链接</Label>
              <div className='flex gap-2'>
                <Input
                  value={tempSubGenerating ? '生成中...' : tempSubUrl}
                  readOnly
                  placeholder='自动生成中...'
                  className='text-sm font-mono'
                />
                {tempSubUrl && !tempSubGenerating && (
                  <Button
                    variant='outline'
                    size='icon'
                    onClick={async () => {
                      try {
                        await navigator.clipboard.writeText(tempSubUrl)
                        toast.success('链接已复制')
                        setTempSubDialogOpen(false)
                        setTempSubUrl('')
                        setTempSubSingleNodeId(null)
                      } catch {
                        toast.error('复制失败，请手动复制')
                      }
                    }}
                  >
                    <Copy className='h-4 w-4' />
                  </Button>
                )}
              </div>
              {tempSubUrl && !tempSubGenerating && (
                <p className='text-xs text-muted-foreground'>
                  链接将在 {tempSubExpireSeconds} 秒后或访问 {tempSubMaxAccess} 次后失效
                </p>
              )}
            </div>
            <div className='flex justify-end pt-2'>
              <Button
                variant='outline'
                onClick={() => {
                  setTempSubDialogOpen(false)
                  setTempSubUrl('')
                  setTempSubSingleNodeId(null)
                }}
              >
                关闭
              </Button>
            </div>
          </div>
        </DialogContent>
      </Dialog>
    </div>
  )
}
