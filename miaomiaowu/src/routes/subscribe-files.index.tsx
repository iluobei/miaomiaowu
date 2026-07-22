// @ts-nocheck
import { useState, useEffect, useMemo, useRef } from 'react'
import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { load as parseYAML, dump as dumpYAML } from 'js-yaml'
import { toast } from 'sonner'
import { format, addDays, isPast, differenceInCalendarDays, isToday } from 'date-fns'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { handleServerError } from '@/lib/handle-server-error'
import { useMediaQuery } from '@/hooks/use-media-query'
import { DataTable } from '@/components/data-table'
import type { DataTableColumn } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Badge } from '@/components/ui/badge'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogTrigger, DialogFooter } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from '@/components/ui/alert-dialog'
import { Tooltip, TooltipContent, TooltipTrigger } from '@/components/ui/tooltip'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Progress } from '@/components/ui/progress'
import { Checkbox } from '@/components/ui/checkbox'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Calendar } from '@/components/ui/calendar'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { cn } from '@/lib/utils'
import { Copy } from 'lucide-react'
import { Upload, Download, Edit, Settings, FileText, Save, Trash2, RefreshCw, ChevronDown, ChevronUp, ExternalLink, Eye, Calendar as CalendarIcon, Plus, Check, Ban, Info, Clock } from 'lucide-react'
import { EditNodesDialog } from '@/components/edit-nodes-dialog'
import { MobileEditNodesDialog } from '@/components/mobile-edit-nodes-dialog'
import { Twemoji } from '@/components/twemoji'
import { useProxyGroupCategories } from '@/hooks/use-proxy-groups'
import { translateOutbound } from '@/lib/sublink/translations'
import { validateClashConfig, formatValidationIssues } from '@/lib/clash-validator'

export const Route = createFileRoute('/subscribe-files/')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: SubscribeFilesPage,
})

type SubscribeFile = {
  id: number
  name: string
  description: string
  type: 'create' | 'import' | 'upload'
  filename: string
  auto_sync_custom_rules: boolean
  selected_custom_rule_ids: number[]
  selected_override_script_ids: number[]
  template_filename: string
  selected_tags: string[]
  selected_node_ids?: number[]
  custom_short_code?: string
  raw_output: boolean
  traffic_limit?: number | null
  stats_server_ids: string
  expire_at?: string | null
  created_at: string
  updated_at: string
  latest_version?: number
}

const TYPE_COLORS = {
  create: 'bg-blue-500/10 text-blue-700 dark:text-blue-400',
  import: 'bg-green-500/10 text-green-700 dark:text-green-400',
  upload: 'bg-purple-500/10 text-purple-700 dark:text-purple-400',
}

const TYPE_LABELS = {
  create: '创建',
  import: '导入',
  upload: '上传',
}

type ExternalSubscription = {
  id: number
  name: string
  url: string
  user_agent: string
  node_count: number
  last_sync_at: string | null
  upload: number
  download: number
  total: number
  expire: string | null
  traffic_mode: 'download' | 'upload' | 'both' | 'none'
  auto_update: boolean
  update_interval_minutes: number
  created_at: string
  updated_at: string
}

type ProxyProviderConfig = {
  id: number
  external_subscription_id: number
  name: string
  type: string
  interval: number
  proxy: string
  size_limit: number
  header: string
  health_check_enabled: boolean
  health_check_url: string
  health_check_interval: number
  health_check_timeout: number
  health_check_lazy: boolean
  health_check_expected_status: number
  filter: string
  exclude_filter: string
  exclude_type: string
  override: string
  process_mode: 'client' | 'mmw'
  created_at: string
  updated_at: string
}

// 代理协议类型列表
const PROXY_TYPES = [
  'vmess', 'vless', 'trojan', 'ss', 'ssr', 'socks5', 'http',
  'hysteria', 'hysteria2', 'tuic', 'wireguard', 'anytls'
]

// 地域分裂配置（用于 Pro 批量创建）
// countryCode 用于 GeoIP 匹配（仅 MMW 模式生效）
const REGION_CONFIGS = [
  { name: '香港节点', emoji: '🇭🇰', filter: '🇭🇰|港|HK|hk|Hong Kong|HongKong|hongkong', countryCode: 'HK' },
  { name: '美国节点', emoji: '🇺🇸', filter: '🇺🇸|美|波特兰|达拉斯|俄勒冈|凤凰城|费利蒙|硅谷|拉斯维加斯|洛杉矶|圣何塞|圣克拉拉|西雅图|芝加哥|US|United States|UnitedStates', countryCode: 'US' },
  { name: '日本节点', emoji: '🇯🇵', filter: '🇯🇵|日本|川日|东京|大阪|泉日|埼玉|沪日|深日|JP|Japan', countryCode: 'JP' },
  { name: '新加坡节点', emoji: '🇸🇬', filter: '🇸🇬|新加坡|坡|狮城|SG|Singapore', countryCode: 'SG' },
  { name: '台湾节点', emoji: '🇹🇼', filter: '🇹🇼|台|新北|彰化|TW|Taiwan', countryCode: 'TW' },
  { name: '韩国节点', emoji: '🇰🇷', filter: '🇰🇷|韩|KR|Korea|KOR|首尔', countryCode: 'KR' },
  { name: '加拿大节点', emoji: '🇨🇦', filter: '🇨🇦|加拿大|CA|Canada', countryCode: 'CA' },
  { name: '英国节点', emoji: '🇬🇧', filter: '🇬🇧|英|UK|伦敦|英格兰|GB|United Kingdom', countryCode: 'GB' },
  { name: '法国节点', emoji: '🇫🇷', filter: '🇫🇷|法|FR|France|巴黎', countryCode: 'FR' },
  { name: '德国节点', emoji: '🇩🇪', filter: '🇩🇪|德|DE|Germany|法兰克福', countryCode: 'DE' },
  { name: '荷兰节点', emoji: '🇳🇱', filter: '🇳🇱|荷|NL|Netherlands|阿姆斯特丹', countryCode: 'NL' },
  { name: '土耳其节点', emoji: '🇹🇷', filter: '🇹🇷|土耳其|TR|Turkey|伊斯坦布尔', countryCode: 'TR' },
  { name: '其他地区', emoji: '🌍', filter: '', excludeFilter: '🇭🇰|🇺🇸|🇯🇵|🇸🇬|🇹🇼|🇰🇷|🇨🇦|🇬🇧|🇫🇷|🇩🇪|🇳🇱|🇹🇷|港|HK|hk|Hong Kong|HongKong|hongkong|美|波特兰|达拉斯|俄勒冈|凤凰城|费利蒙|硅谷|拉斯维加斯|洛杉矶|圣何塞|圣克拉拉|西雅图|芝加哥|US|United States|UnitedStates|日本|川日|东京|大阪|泉日|埼玉|沪日|深日|JP|Japan|新加坡|坡|狮城|SG|Singapore|台|新北|彰化|TW|Taiwan|韩|KR|Korea|KOR|首尔|加拿大|CA|Canada|英|UK|伦敦|英格兰|GB|United Kingdom|法|FR|France|巴黎|德|DE|Germany|法兰克福|荷|NL|Netherlands|阿姆斯特丹|土耳其|TR|Turkey|伊斯坦布尔', countryCode: '' },
]

// 协议分裂配置（用于 Pro 批量创建）
const PROTOCOL_CONFIGS = [
  { name: 'anytls', excludeType: 'wireguard|vmess|vless|trojan|ss|socks5|http|ssr|hysteria|tuic|hysteria2' },
  { name: 'wireguard', excludeType: 'anytls|vmess|vless|trojan|ss|socks5|http|ssr|hysteria|tuic|hysteria2' },
  { name: 'vmess', excludeType: 'anytls|wireguard|vless|trojan|ss|socks5|http|ssr|hysteria|tuic|hysteria2' },
  { name: 'vless', excludeType: 'anytls|wireguard|vmess|trojan|ss|socks5|http|ssr|hysteria|tuic|hysteria2' },
  { name: 'trojan', excludeType: 'anytls|wireguard|vmess|vless|ss|socks5|http|ssr|hysteria|tuic|hysteria2' },
  { name: 'ss', excludeType: 'anytls|wireguard|vmess|vless|trojan|socks5|http|ssr|hysteria|tuic|hysteria2' },
  { name: 'socks5', excludeType: 'anytls|wireguard|vmess|vless|trojan|ss|http|ssr|hysteria|tuic|hysteria2' },
  { name: 'http', excludeType: 'anytls|wireguard|vmess|vless|trojan|ss|socks5|ssr|hysteria|tuic|hysteria2' },
  { name: 'ssr', excludeType: 'anytls|wireguard|vmess|vless|trojan|ss|socks5|http|hysteria|tuic|hysteria2' },
  { name: 'hysteria', excludeType: 'anytls|wireguard|vmess|vless|trojan|ss|socks5|http|ssr|tuic|hysteria2' },
  { name: 'tuic', excludeType: 'anytls|wireguard|vmess|vless|trojan|ss|socks5|http|ssr|hysteria|hysteria2' },
  { name: 'hysteria2', excludeType: 'anytls|wireguard|vmess|vless|trojan|ss|socks5|http|ssr|hysteria|tuic' },
]

// IP 版本选项
const IP_VERSION_OPTIONS = [
  { value: '', label: '默认' },
  { value: 'dual', label: 'dual (双栈)' },
  { value: 'ipv4', label: 'ipv4' },
  { value: 'ipv6', label: 'ipv6' },
  { value: 'ipv4-prefer', label: 'ipv4-prefer' },
  { value: 'ipv6-prefer', label: 'ipv6-prefer' },
]

// Override 表单类型
type OverrideForm = {
  tfo: boolean
  mptcp: boolean
  udp: boolean
  udp_over_tcp: boolean
  skip_cert_verify: boolean
  dialer_proxy: string
  interface_name: string
  routing_mark: string
  ip_version: '' | 'dual' | 'ipv4' | 'ipv6' | 'ipv4-prefer' | 'ipv6-prefer'
  additional_prefix: string
  additional_suffix: string
}

// 默认 Override 表单值
const defaultOverrideForm: OverrideForm = {
  tfo: false,
  mptcp: false,
  udp: true,
  udp_over_tcp: false,
  skip_cert_verify: false,
  dialer_proxy: '',
  interface_name: '',
  routing_mark: '',
  ip_version: '',
  additional_prefix: '',
  additional_suffix: '',
}

// Override 表单转 JSON (保存时)
function overrideFormToJSON(form: OverrideForm): string {
  const obj: Record<string, any> = {}

  // 只添加非默认值的字段
  if (form.tfo) obj['tfo'] = true
  if (form.mptcp) obj['mptcp'] = true
  if (!form.udp) obj['udp'] = false  // 默认 true，只有 false 时添加
  if (form.udp_over_tcp) obj['udp-over-tcp'] = true
  if (form.skip_cert_verify) obj['skip-cert-verify'] = true
  if (form.dialer_proxy) obj['dialer-proxy'] = form.dialer_proxy
  if (form.interface_name) obj['interface-name'] = form.interface_name
  if (form.routing_mark) obj['routing-mark'] = parseInt(form.routing_mark)
  if (form.ip_version) obj['ip-version'] = form.ip_version
  if (form.additional_prefix) obj['additional-prefix'] = form.additional_prefix
  if (form.additional_suffix) obj['additional-suffix'] = form.additional_suffix

  return Object.keys(obj).length > 0 ? JSON.stringify(obj) : ''
}

// JSON 转 Override 表单 (编辑时)
function jsonToOverrideForm(json: string): OverrideForm {
  if (!json) return { ...defaultOverrideForm }

  try {
    const obj = JSON.parse(json)
    return {
      tfo: obj['tfo'] ?? false,
      mptcp: obj['mptcp'] ?? false,
      udp: obj['udp'] ?? true,
      udp_over_tcp: obj['udp-over-tcp'] ?? false,
      skip_cert_verify: obj['skip-cert-verify'] ?? false,
      dialer_proxy: obj['dialer-proxy'] ?? '',
      interface_name: obj['interface-name'] ?? '',
      routing_mark: obj['routing-mark']?.toString() ?? '',
      ip_version: obj['ip-version'] ?? '',
      additional_prefix: obj['additional-prefix'] ?? '',
      additional_suffix: obj['additional-suffix'] ?? '',
    }
  } catch {
    return { ...defaultOverrideForm }
  }
}

// 格式化流量
function formatTraffic(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(2)} KB`
  if (bytes < 1024 * 1024 * 1024) return `${(bytes / (1024 * 1024)).toFixed(2)} MB`
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

// 格式化流量为GB（用于外部订阅显示）
function formatTrafficGB(bytes: number): string {
  return `${(bytes / (1024 * 1024 * 1024)).toFixed(2)} GB`
}

function SubscribeFilesPage() {
  const EDIT_NODES_DRAFT_KEY_PREFIX = 'mmw_edit_nodes_draft_'
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const isMobile = useMediaQuery('(max-width: 640px)')

  // 获取代理组配置
  const { data: proxyGroupCategories = [] } = useProxyGroupCategories()

  // 获取用户配置，用于判断模板版本
  const { data: userConfig } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as { template_version: string }
    },
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })
  const templateVersion = userConfig?.template_version || 'v2'
  const isV3Mode = templateVersion === 'v3'

  // 日期格式化器
  const dateFormatter = useMemo(
    () =>
      new Intl.DateTimeFormat('zh-CN', {
        dateStyle: 'medium',
        timeStyle: 'short',
        hour12: false,
      }),
    []
  )

  // 对话框状态
  const [importDialogOpen, setImportDialogOpen] = useState(false)
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false)
  const [editDialogOpen, setEditDialogOpen] = useState(false)
  const [editingFile, setEditingFile] = useState<SubscribeFile | null>(null)
  const [editMetadataDialogOpen, setEditMetadataDialogOpen] = useState(false)
  const [editingMetadata, setEditingMetadata] = useState<SubscribeFile | null>(null)
  const [editConfigDialogOpen, setEditConfigDialogOpen] = useState(false)
  const [editingConfigFile, setEditingConfigFile] = useState<SubscribeFile | null>(null)

  // 过期时间Popover状态
  const [expirePopoverFileId, setExpirePopoverFileId] = useState<number | null>(null)
  const [customDateFileId, setCustomDateFileId] = useState<number | null>(null)
  const [mobileExpirePopoverFileId, setMobileExpirePopoverFileId] = useState<number | null>(null)
  const [mobileCustomDateFileId, setMobileCustomDateFileId] = useState<number | null>(null)

  // 自定义连接Popover状态
  const [customLinkFileId, setCustomLinkFileId] = useState<number | null>(null)
  const [customLinkInput, setCustomLinkInput] = useState('')
  const [userShortCodes, setUserShortCodes] = useState<Record<string, string>>({})

  // 编辑节点Dialog状态
  const [editNodesDialogOpen, setEditNodesDialogOpen] = useState(false)
  const [editingNodesFile, setEditingNodesFile] = useState<SubscribeFile | null>(null)
  const [proxyGroups, setProxyGroups] = useState<Array<{ name: string; type: string; proxies: string[]; use?: string[]; dialerProxyGroup?: string }>>([])
  const [showAllNodes, setShowAllNodes] = useState(true)
  const [isNodesDraftRecoveryOpen, setIsNodesDraftRecoveryOpen] = useState(false)
  const pendingNodesDraftRef = useRef<any>(null)
  const proxyGroupsOriginalRef = useRef<string>('')

  // 编辑器状态
  const [editorValue, setEditorValue] = useState('')
  const [isDirty, setIsDirty] = useState(false)
  const [validationError, setValidationError] = useState<string | null>(null)

  // 编辑配置状态
  const [configContent, setConfigContent] = useState('')

  // 缺失节点替换对话框状态
  const [missingNodesDialogOpen, setMissingNodesDialogOpen] = useState(false)
  const [missingNodes, setMissingNodes] = useState<string[]>([])
  const [replacementChoice, setReplacementChoice] = useState<string>('DIRECT')
  const [pendingConfigAfterSave, setPendingConfigAfterSave] = useState('')

  // 导入表单
  const [importForm, setImportForm] = useState({
    name: '',
    description: '',
    url: '',
    filename: '',
  })

  // 上传表单
  const [uploadForm, setUploadForm] = useState({
    name: '',
    description: '',
    filename: '',
    overwrite_id: 0,
    raw_output: false,
  })
  const [uploadFile, setUploadFile] = useState<File | null>(null)

  // 编辑元数据表单
  const [metadataForm, setMetadataForm] = useState({
    name: '',
    description: '',
    filename: '',
    template_filename: '',
    selected_tags: [] as string[],
    selected_node_ids: [] as number[],
    expire: undefined as Date | undefined,
    traffic_limit: '' as string,
    stats_server_ids: '' as string,
  })
  // 节点筛选模式:新订阅默认 node;编辑老订阅有 selected_tags 但无 selected_node_ids 时进 tag(向后兼容)
  const [pickerMode, setPickerMode] = useState<'tag' | 'node'>('node')

  // 外部订阅卡片折叠状态 - 默认折叠
  const [isExternalSubsExpanded, setIsExternalSubsExpanded] = useState(false)

  // 编辑外部订阅对话框状态
  const [editExternalSubDialogOpen, setEditExternalSubDialogOpen] = useState(false)
  const [editingExternalSub, setEditingExternalSub] = useState<ExternalSubscription | null>(null)
  const [editExternalSubForm, setEditExternalSubForm] = useState({
    name: '',
    url: '',
    user_agent: '',
    traffic_mode: 'both' as 'download' | 'upload' | 'both' | 'none',
    auto_update: false,
    update_interval_minutes: 60,
  })

  // 代理集合对话框状态
  const [proxyProviderDialogOpen, setProxyProviderDialogOpen] = useState(false)
  const [selectedExternalSub, setSelectedExternalSub] = useState<ExternalSubscription | null>(null)
  const [proxyProviderForm, setProxyProviderForm] = useState({
    name: '',
    type: 'http',
    interval: 3600,
    proxy: 'DIRECT',
    size_limit: 0,
    header_user_agent: 'Clash/v1.18.0',
    header_authorization: '',
    health_check_enabled: true,
    health_check_url: 'https://www.gstatic.com/generate_204',
    health_check_interval: 300,
    health_check_timeout: 5000,
    health_check_lazy: true,
    health_check_expected_status: 204,
    filter: '',
    exclude_filter: '',
    exclude_type: [] as string[],
    override: { ...defaultOverrideForm },
    process_mode: 'client' as 'client' | 'mmw',
  })
  const [editingProxyProvider, setEditingProxyProvider] = useState<ProxyProviderConfig | null>(null)
  const [isProxyProvidersExpanded, setIsProxyProvidersExpanded] = useState(false)

  // 代理集合Pro对话框状态
  const [proxyProviderProDialogOpen, setProxyProviderProDialogOpen] = useState(false)
  const [proSelectedExternalSub, setProSelectedExternalSub] = useState<ExternalSubscription | null>(null)
  const [proNamePrefix, setProNamePrefix] = useState('')
  const [proCreatingRegion, setProCreatingRegion] = useState(false)
  const [proCreatingProtocol, setProCreatingProtocol] = useState(false)
  const [proCreationResults, setProCreationResults] = useState<Array<{name: string, success: boolean, error?: string}>>([])
  const [enableGeoIPMatching, setEnableGeoIPMatching] = useState(true) // 根据IP位置分组开关

  // 代理集合批量操作状态
  const [selectedProxyProviderIds, setSelectedProxyProviderIds] = useState<Set<number>>(new Set())
  const [proxyProviderFilterSubId, setProxyProviderFilterSubId] = useState<number | 'all'>('all')
  const [batchDeleteDialogOpen, setBatchDeleteDialogOpen] = useState(false)

  // 代理集合预览状态（MMW 模式）
  const [previewDialogOpen, setPreviewDialogOpen] = useState(false)
  const [previewContent, setPreviewContent] = useState('')
  const [previewLoading, setPreviewLoading] = useState(false)
  const [previewConfigName, setPreviewConfigName] = useState('')

  // 获取订阅文件列表
  const { data: filesData, isLoading } = useQuery({
    queryKey: ['subscribe-files'],
    queryFn: async () => {
      const response = await api.get('/api/admin/subscribe-files')
      return response.data as { files: SubscribeFile[] }
    },
    enabled: Boolean(auth.accessToken),
  })

  const files = filesData?.files ?? []

  // 获取订阅流量数据（含探针总流量）
  const { data: subscribeTrafficData } = useQuery({
    queryKey: ['subscribe-traffic'],
    queryFn: async () => {
      const response = await api.get('/api/traffic/subscribe')
      return response.data as {
        items: Array<{ id: number; limit_gb: number; used_gb: number }>
        probe_total?: { limit_gb: number; used_gb: number }
      }
    },
    enabled: Boolean(auth.accessToken),
    refetchInterval: 5 * 60 * 1000,
  })
  const subscribeTrafficMap = useMemo(() => {
    const map = new Map<number, { limit_gb: number; used_gb: number }>()
    if (subscribeTrafficData?.items) {
      for (const item of subscribeTrafficData.items) {
        map.set(item.id, item)
      }
    }
    return map
  }, [subscribeTrafficData])
  const probeTotal = subscribeTrafficData?.probe_total

  // 获取 V3 模板列表
  const { data: templatesData } = useQuery({
    queryKey: ['template-v3-list'],
    queryFn: async () => {
      const response = await api.get('/api/admin/template-v3')
      return response.data as { templates: Array<{ name: string; filename: string }> }
    },
    enabled: Boolean(auth.accessToken),
  })

  const v3Templates = templatesData?.templates ?? []

  // 获取外部订阅列表
  const { data: externalSubsData, isLoading: isExternalSubsLoading } = useQuery({
    queryKey: ['external-subscriptions'],
    queryFn: async () => {
      const response = await api.get('/api/user/external-subscriptions')
      return response.data as ExternalSubscription[]
    },
    enabled: Boolean(auth.accessToken),
  })

  const externalSubs = externalSubsData ?? []

  // 获取用户订阅 token（用于代理集合 MMW 模式）
  const { data: userTokenData } = useQuery({
    queryKey: ['user-token'],
    queryFn: async () => {
      const response = await api.get('/api/user/token')
      return response.data as { token: string }
    },
    enabled: Boolean(auth.accessToken),
  })
  const userToken = userTokenData?.token ?? ''

  // 获取用户设置（用于判断是否显示代理集合）
  const { data: userConfigData } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as { enable_proxy_provider: boolean }
    },
    enabled: Boolean(auth.accessToken),
  })
  const enableProxyProvider = userConfigData?.enable_proxy_provider ?? false

  // 获取代理集合配置列表（仅在启用时查询）
  const { data: proxyProviderConfigsData, isLoading: isProxyProviderConfigsLoading } = useQuery({
    queryKey: ['proxy-provider-configs'],
    queryFn: async () => {
      const response = await api.get('/api/user/proxy-provider-configs')
      return response.data as ProxyProviderConfig[]
    },
    enabled: Boolean(auth.accessToken && enableProxyProvider),
  })
  const proxyProviderConfigs = proxyProviderConfigsData ?? []

  // 获取探针服务器列表（用于统计服务器选择）
  const { data: probeConfigData } = useQuery({
    queryKey: ['probe-config'],
    queryFn: async () => {
      const response = await api.get('/api/admin/probe-config')
      return response.data as {
        config: {
          servers: Array<{ id: number; name: string; server_id: string }>
        }
      }
    },
    enabled: Boolean(auth.accessToken),
  })
  const probeServers = probeConfigData?.config?.servers ?? []

  // 绑定v3模板
  const hasTemplateBindings = files.some(f => f.template_filename)

  // 获取所有节点（用于在外部订阅卡片中显示节点名称, v3模板订阅标签）
  const { data: allNodesData } = useQuery({
    queryKey: ['all-nodes-with-tags'],
    queryFn: async () => {
      const response = await api.get('/api/admin/nodes')
      return response.data as { nodes: Array<{ id: number; node_name: string; tag: string; tags: string[] }> }
    },
    enabled: Boolean(auth.accessToken && (isExternalSubsExpanded || hasTemplateBindings)),
  })

  const { data: customRulesData } = useQuery({
    queryKey: ['custom-rules'],
    queryFn: async () => {
      const response = await api.get('/api/admin/custom-rules')
      return response.data as { id: number; name: string; type: string; enabled: boolean }[]
    },
  })

  const { data: overrideScriptsData } = useQuery({
    queryKey: ['override-scripts'],
    queryFn: async () => {
      const response = await api.get('/api/admin/override-scripts')
      return response.data as { id: number; name: string; hook: string; enabled: boolean }[]
    },
  })

  const enabledCustomRules = useMemo(() =>
    (customRulesData ?? []).filter(r => r.enabled),
    [customRulesData]
  )

  const enabledOverrideScripts = useMemo(() =>
    (overrideScriptsData ?? []).filter(s => s.enabled),
    [overrideScriptsData]
  )

  // 按 tag 分组的节点名称
  const nodesByTag = useMemo(() => {
    const nodes = allNodesData?.nodes ?? []
    const grouped: Record<string, string[]> = {}
    for (const node of nodes) {
      const nodeTags = node.tags?.length ? node.tags : (node.tag ? [node.tag] : ['手动输入'])
      for (const t of nodeTags) {
        if (!grouped[t]) grouped[t] = []
        grouped[t].push(node.node_name)
      }
    }
    return grouped
  }, [allNodesData])

  // 获取所有唯一的节点标签
  const allNodeTags = useMemo(() => {
    const nodes = allNodesData?.nodes ?? []
    const tags = new Set<string>()
    for (const node of nodes) {
      const nodeTags = node.tags?.length ? node.tags : (node.tag ? [node.tag] : ['手动输入'])
      for (const t of nodeTags) tags.add(t)
    }
    return Array.from(tags).sort()
  }, [allNodesData])

  // 导入订阅
  const importMutation = useMutation({
    mutationFn: async (data: typeof importForm) => {
      const response = await api.post('/api/admin/subscribe-files/import', data)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
      toast.success('订阅导入成功')
      setImportDialogOpen(false)
      setImportForm({ name: '', description: '', url: '', filename: '' })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '导入失败')
    },
  })

  // 上传文件
  const uploadMutation = useMutation({
    mutationFn: async () => {
      if (!uploadFile) {
        throw new Error('请选择文件')
      }

      const formData = new FormData()
      formData.append('file', uploadFile)
      if (uploadForm.overwrite_id > 0) {
        formData.append('overwrite_id', String(uploadForm.overwrite_id))
      } else {
        formData.append('name', uploadForm.name || uploadFile.name)
        formData.append('description', uploadForm.description)
        formData.append('filename', uploadForm.filename || uploadFile.name)
      }
      if (uploadForm.raw_output) {
        formData.append('raw_output', 'true')
      }

      const response = await api.post('/api/admin/subscribe-files/upload', formData, {
        headers: {
          'Content-Type': 'multipart/form-data',
        },
      })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
      toast.success(uploadForm.overwrite_id > 0 ? '文件覆盖成功' : '文件上传成功')
      setUploadDialogOpen(false)
      setUploadForm({ name: '', description: '', filename: '', overwrite_id: 0, raw_output: false })
      setUploadFile(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '上传失败')
    },
  })

  // 排序订阅
  const reorderMutation = useMutation({
    mutationFn: async (ids: number[]) => {
      await api.put('/api/admin/subscribe-files/reorder', { ids })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '排序失败')
    },
  })

  const handleMoveUp = (index: number) => {
    if (index === 0) return
    const newFiles = [...files]
    ;[newFiles[index - 1], newFiles[index]] = [newFiles[index], newFiles[index - 1]]
    reorderMutation.mutate(newFiles.map(f => f.id))
  }

  const handleMoveDown = (index: number) => {
    if (index >= files.length - 1) return
    const newFiles = [...files]
    ;[newFiles[index], newFiles[index + 1]] = [newFiles[index + 1], newFiles[index]]
    reorderMutation.mutate(newFiles.map(f => f.id))
  }

  // 删除订阅
  const deleteMutation = useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/api/admin/subscribe-files/${id}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
      toast.success('订阅已删除')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '删除失败')
    },
  })

  // 更新订阅元数据
  const updateMetadataMutation = useMutation({
    mutationFn: async (payload: { id: number; data: typeof metadataForm }) => {
      const response = await api.put(`/api/admin/subscribe-files/${payload.id}`, payload.data)
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
      toast.success('订阅信息已更新')
      setEditMetadataDialogOpen(false)
      setEditingMetadata(null)
      setMetadataForm({ name: '', description: '', filename: '', template_filename: '', selected_tags: [], selected_node_ids: [], expire: undefined, traffic_limit: '', stats_server_ids: '' })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '更新失败')
    },
  })

  // 查询订阅关联的用户短码
  const subscriptionUsersQuery = useQuery({
    queryKey: ['subscription-users', customLinkFileId],
    queryFn: async () => {
      const response = await api.get(`/api/admin/subscribe-files/${customLinkFileId}/users`)
      return response.data.users as Array<{ username: string; user_short_code: string; custom_user_short_code: string }>
    },
    enabled: customLinkFileId !== null,
  })

  // 更新用户自定义短码
  const updateUserShortCodeMutation = useMutation({
    mutationFn: async (payload: { username: string; custom_short_code: string }) => {
      await api.post('/api/admin/users/custom-short-code', payload)
    },
    onSuccess: (_data, variables) => {
      toast.success(`${variables.username} 短链接已更新`)
      queryClient.invalidateQueries({ queryKey: ['subscription-users', customLinkFileId] })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '更新失败')
    },
  })

  // 删除外部订阅
  const deleteExternalSubMutation = useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/api/user/external-subscriptions?id=${id}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['external-subscriptions'] })
      queryClient.invalidateQueries({ queryKey: ['traffic-summary'] })
      toast.success('外部订阅已删除')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '删除失败')
    },
  })

  // 更新外部订阅
  const updateExternalSubMutation = useMutation({
    mutationFn: async (data: {
      id: number
      name: string
      url: string
      user_agent: string
      traffic_mode: string
      auto_update?: boolean
      update_interval_minutes?: number
    }) => {
      await api.put(`/api/user/external-subscriptions?id=${data.id}`, {
        name: data.name,
        url: data.url,
        user_agent: data.user_agent,
        traffic_mode: data.traffic_mode,
        auto_update: data.auto_update,
        update_interval_minutes: data.update_interval_minutes,
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['external-subscriptions'] })
      queryClient.invalidateQueries({ queryKey: ['traffic-summary'] })
      toast.success('外部订阅已更新')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '更新失败')
    },
  })

  // 同步外部订阅
  const syncExternalSubsMutation = useMutation({
    mutationFn: async () => {
      const response = await api.post('/api/admin/sync-external-subscriptions')
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['external-subscriptions'] })
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      queryClient.invalidateQueries({ queryKey: ['traffic-summary'] })
      toast.success('外部订阅同步成功')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '同步失败')
    },
  })

  // 同步单个外部订阅
  const [syncingSingleId, setSyncingSingleId] = useState<number | null>(null)
  const syncSingleExternalSubMutation = useMutation({
    mutationFn: async (id: number) => {
      setSyncingSingleId(id)
      const response = await api.post(`/api/admin/sync-external-subscription?id=${id}`)
      return response.data
    },
    onSuccess: (data: any) => {
      queryClient.invalidateQueries({ queryKey: ['external-subscriptions'] })
      queryClient.invalidateQueries({ queryKey: ['nodes'] })
      queryClient.invalidateQueries({ queryKey: ['all-nodes-with-tags'] })
      queryClient.invalidateQueries({ queryKey: ['traffic-summary'] })
      toast.success(data.message || '订阅同步成功')
      setSyncingSingleId(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '同步失败')
      setSyncingSingleId(null)
    },
  })

  // 创建代理集合配置
  const createProxyProviderMutation = useMutation({
    mutationFn: async (data: {
      external_subscription_id: number
      name: string
      type: string
      interval: number
      proxy: string
      size_limit: number
      header: string
      health_check_enabled: boolean
      health_check_url: string
      health_check_interval: number
      health_check_timeout: number
      health_check_lazy: boolean
      health_check_expected_status: number
      filter: string
      exclude_filter: string
      exclude_type: string
      override: string
      process_mode: string
    }) => {
      const response = await api.post('/api/user/proxy-provider-configs', data)
      // 如果是 MMW 模式，触发缓存刷新
      if (data.process_mode === 'mmw' && response.data?.id) {
        try {
          await api.post(`/api/user/proxy-provider-cache/refresh?id=${response.data.id}`)
        } catch (e) {
          console.warn('缓存刷新失败:', e)
        }
      }
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })
      toast.success('代理集合配置创建成功')
      setProxyProviderDialogOpen(false)
      // 重置表单
      setProxyProviderForm({
        name: '',
        type: 'http',
        interval: 3600,
        proxy: 'DIRECT',
        size_limit: 0,
        header_user_agent: 'Clash/v1.18.0',
        header_authorization: '',
        health_check_enabled: true,
        health_check_url: 'https://www.gstatic.com/generate_204',
        health_check_interval: 300,
        health_check_timeout: 5000,
        health_check_lazy: true,
        health_check_expected_status: 204,
        filter: '',
        exclude_filter: '',
        exclude_type: [],
        override: { ...defaultOverrideForm },
        process_mode: 'client',
      })
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '创建失败')
    },
  })

  // 更新代理集合配置
  const updateProxyProviderMutation = useMutation({
    mutationFn: async (data: {
      id: number
      name: string
      type: string
      interval: number
      proxy: string
      size_limit: number
      header: string
      health_check_enabled: boolean
      health_check_url: string
      health_check_interval: number
      health_check_timeout: number
      health_check_lazy: boolean
      health_check_expected_status: number
      filter: string
      exclude_filter: string
      exclude_type: string
      override: string
      process_mode: string
    }) => {
      const response = await api.put(`/api/user/proxy-provider-configs?id=${data.id}`, data)
      // 如果是 MMW 模式，触发缓存刷新
      if (data.process_mode === 'mmw') {
        try {
          await api.post(`/api/user/proxy-provider-cache/refresh?id=${data.id}`)
        } catch (e) {
          console.warn('缓存刷新失败:', e)
        }
      }
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })
      toast.success('代理集合配置更新成功')
      setProxyProviderDialogOpen(false)
      setEditingProxyProvider(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '更新失败')
    },
  })

  // 删除代理集合配置
  const deleteProxyProviderMutation = useMutation({
    mutationFn: async (id: number) => {
      await api.delete(`/api/user/proxy-provider-configs?id=${id}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })
      toast.success('代理集合配置已删除')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '删除失败')
    },
  })

  // 批量删除代理集合配置
  const batchDeleteProxyProviderMutation = useMutation({
    mutationFn: async (ids: number[]) => {
      // 并行删除所有选中的配置
      const results = await Promise.allSettled(
        ids.map(id => api.delete(`/api/user/proxy-provider-configs?id=${id}`))
      )
      const failed = results.filter(r => r.status === 'rejected').length
      if (failed > 0) {
        throw new Error(`${failed} 个配置删除失败`)
      }
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })
      setSelectedProxyProviderIds(new Set())
      setBatchDeleteDialogOpen(false)
      toast.success('批量删除成功')
    },
    onError: (error: any) => {
      queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })
      setSelectedProxyProviderIds(new Set())
      setBatchDeleteDialogOpen(false)
      toast.error(error.message || '批量删除失败')
    },
  })

  // 过滤后的代理集合配置列表
  const filteredProxyProviderConfigs = useMemo(() => {
    if (proxyProviderFilterSubId === 'all') {
      return proxyProviderConfigs
    }
    return proxyProviderConfigs.filter(c => c.external_subscription_id === proxyProviderFilterSubId)
  }, [proxyProviderConfigs, proxyProviderFilterSubId])

  // 处理全选/取消全选
  const handleSelectAllProxyProviders = (checked: boolean) => {
    if (checked) {
      setSelectedProxyProviderIds(new Set(filteredProxyProviderConfigs.map(c => c.id)))
    } else {
      setSelectedProxyProviderIds(new Set())
    }
  }

  // 处理单个选中/取消选中
  const handleSelectProxyProvider = (id: number, checked: boolean) => {
    setSelectedProxyProviderIds(prev => {
      const newSet = new Set(prev)
      if (checked) {
        newSet.add(id)
      } else {
        newSet.delete(id)
      }
      return newSet
    })
  }

  // 快速切换代理集合处理模式
  const toggleProcessModeMutation = useMutation({
    mutationFn: async (config: ProxyProviderConfig) => {
      const newMode = config.process_mode === 'mmw' ? 'client' : 'mmw'
      await api.put(`/api/user/proxy-provider-configs?id=${config.id}`, {
        ...config,
        process_mode: newMode,
      })
      return newMode
    },
    onSuccess: (newMode) => {
      queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })
      toast.success(`已切换为${newMode === 'mmw' ? '妙妙屋处理' : '客户端处理'}`)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '切换失败')
    },
  })

  // 批量创建代理集合 - 按地域
  // 使用 MMW 模式以支持 GeoIP 匹配
  const handleBatchCreateByRegion = async () => {
    if (!proSelectedExternalSub) {
      toast.error('请先选择外部订阅')
      return
    }
    if (!proNamePrefix.trim()) {
      toast.error('请输入名称前缀')
      return
    }

    setProCreatingRegion(true)
    setProCreationResults([])
    const results: Array<{name: string, success: boolean, error?: string, skipped?: boolean}> = []
    const prefix = proNamePrefix.trim()

    // 先获取外部订阅的节点名称列表（仅用于非 GeoIP 模式）
    let nodeNames: string[] = []
    if (!enableGeoIPMatching) {
      try {
        const response = await api.get(`/api/user/external-subscriptions/nodes?id=${proSelectedExternalSub.id}`)
        nodeNames = response.data.node_names || []
      } catch (error: any) {
        toast.error('获取节点列表失败: ' + (error.response?.data?.error || error.message))
        setProCreatingRegion(false)
        return
      }

      if (nodeNames.length === 0) {
        toast.error('订阅中没有节点')
        setProCreatingRegion(false)
        return
      }
    }

    // 检查每个地区是否有匹配的节点（仅用于非 GeoIP 模式的前端检查）
    const checkRegionHasNodesLocal = (filter: string, excludeFilter?: string): boolean => {
      if (!filter && !excludeFilter) return true // 无过滤条件，认为有节点

      let matchedNodes = nodeNames

      // 应用 filter（包含过滤）- 区分大小写
      if (filter) {
        const filterRegex = new RegExp(filter)
        matchedNodes = matchedNodes.filter(name => filterRegex.test(name))
      }

      // 应用 excludeFilter（排除过滤）- 区分大小写
      if (excludeFilter) {
        const excludeRegex = new RegExp(excludeFilter)
        matchedNodes = matchedNodes.filter(name => !excludeRegex.test(name))
      }

      return matchedNodes.length > 0
    }

    // 检查是否有匹配的节点（GeoIP 模式使用后端 API）
    const checkRegionHasNodes = async (filter: string, excludeFilter?: string, geoIPFilter?: string): Promise<boolean> => {
      // 如果开启 GeoIP 匹配，调用后端 API 检查（后端会同时检查正则和 IP 地域）
      if (enableGeoIPMatching) {
        try {
          const response = await api.post('/api/user/external-subscriptions/check-filter', {
            subscription_id: proSelectedExternalSub.id,
            filter: filter || '',
            exclude_filter: excludeFilter || '',
            geo_ip_filter: geoIPFilter || '',
          })
          return response.data.match_count > 0
        } catch (error) {
          // 如果 API 调用失败，默认不创建该地区（保守处理）
          console.error('检查过滤器失败:', error)
          return false
        }
      }

      // 非 GeoIP 模式，使用前端检查
      return checkRegionHasNodesLocal(filter, excludeFilter)
    }

    let skippedCount = 0
    for (const region of REGION_CONFIGS) {
      const providerName = `${prefix}-${region.emoji}${region.name}`

      // 检查该地区是否有匹配的节点
      const geoIPFilter = enableGeoIPMatching ? (region.countryCode || '') : ''
      const hasNodes = await checkRegionHasNodes(region.filter, region.excludeFilter, geoIPFilter)
      if (!hasNodes) {
        results.push({ name: providerName, success: false, skipped: true, error: '无匹配节点' })
        skippedCount++
        setProCreationResults([...results])
        continue
      }

      try {
        await api.post('/api/user/proxy-provider-configs', {
          external_subscription_id: proSelectedExternalSub.id,
          name: providerName,
          type: 'http',
          interval: 3600,
          proxy: 'DIRECT',
          size_limit: 0,
          header: JSON.stringify({ 'User-Agent': ['Clash/v1.18.0'] }),
          health_check_enabled: true,
          health_check_url: 'https://www.gstatic.com/generate_204',
          health_check_interval: 300,
          health_check_timeout: 5000,
          health_check_lazy: true,
          health_check_expected_status: 204,
          filter: region.filter || '',
          exclude_filter: region.excludeFilter || '',
          exclude_type: '',
          geo_ip_filter: enableGeoIPMatching ? (region.countryCode || '') : '', // GeoIP 过滤（仅开启时生效）
          override: '',
          process_mode: 'mmw', // 使用 MMW 模式以支持 GeoIP 匹配
        })
        results.push({ name: providerName, success: true })
      } catch (error: any) {
        results.push({ name: providerName, success: false, error: error.response?.data?.error || '创建失败' })
      }
      // 更新结果以显示进度
      setProCreationResults([...results])
    }

    setProCreatingRegion(false)
    queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })

    const successCount = results.filter(r => r.success).length
    const failedCount = results.filter(r => !r.success && !r.skipped).length
    if (skippedCount > 0) {
      toast.success(`创建完成: ${successCount} 个成功, ${skippedCount} 个跳过(无节点), ${failedCount} 个失败`)
    } else {
      toast.success(`创建完成: ${successCount}/${results.length} 个代理集合`)
    }
    // 清空名称前缀
    setProNamePrefix('')
  }

  // 批量创建代理集合 - 按协议
  // 使用 MMW 模式（妙妙屋处理）
  const handleBatchCreateByProtocol = async () => {
    if (!proSelectedExternalSub) {
      toast.error('请先选择外部订阅')
      return
    }
    if (!proNamePrefix.trim()) {
      toast.error('请输入名称前缀')
      return
    }

    setProCreatingProtocol(true)
    setProCreationResults([])
    const results: Array<{name: string, success: boolean, error?: string}> = []
    const prefix = proNamePrefix.trim()

    for (const protocol of PROTOCOL_CONFIGS) {
      const providerName = `${prefix}-${protocol.name}`
      try {
        await api.post('/api/user/proxy-provider-configs', {
          external_subscription_id: proSelectedExternalSub.id,
          name: providerName,
          type: 'http',
          interval: 3600,
          proxy: 'DIRECT',
          size_limit: 0,
          header: JSON.stringify({ 'User-Agent': ['Clash/v1.18.0'] }),
          health_check_enabled: true,
          health_check_url: 'https://www.gstatic.com/generate_204',
          health_check_interval: 300,
          health_check_timeout: 5000,
          health_check_lazy: true,
          health_check_expected_status: 204,
          filter: '',
          exclude_filter: '',
          exclude_type: protocol.excludeType,
          override: '',
          process_mode: 'mmw', // 使用 MMW 模式（妙妙屋处理）
        })
        results.push({ name: providerName, success: true })
      } catch (error: any) {
        results.push({ name: providerName, success: false, error: error.response?.data?.error || '创建失败' })
      }
      // 更新结果以显示进度
      setProCreationResults([...results])
    }

    setProCreatingProtocol(false)
    queryClient.invalidateQueries({ queryKey: ['proxy-provider-configs'] })

    const successCount = results.filter(r => r.success).length
    toast.success(`创建完成: ${successCount}/${results.length} 个代理集合`)
    // 清空名称前缀
    setProNamePrefix('')
  }

  // 预览妙妙屋处理后的配置
  const handlePreviewProxyProvider = async (config: ProxyProviderConfig) => {
    if (config.process_mode !== 'mmw') {
      toast.error('仅妙妙屋处理模式支持预览')
      return
    }

    setPreviewConfigName(config.name)
    setPreviewContent('')
    setPreviewLoading(true)
    setPreviewDialogOpen(true)

    try {
      const response = await api.get(`/api/proxy-provider/${config.id}?token=${userToken}`, {
        responseType: 'text',
      })
      setPreviewContent(response.data)
    } catch (error: any) {
      setPreviewContent(`# 预览失败\n# ${error.response?.data || error.message || '未知错误'}`)
      toast.error('预览失败')
    } finally {
      setPreviewLoading(false)
    }
  }

  // 生成代理集合YAML配置预览
  const generateProxyProviderYAML = () => {
    if (!selectedExternalSub) return ''

    const form = proxyProviderForm
    const isClientMode = form.process_mode === 'client'

    // 构建配置对象
    const config: Record<string, any> = {
      type: form.type,
      path: `./proxy_providers/${form.name}.yaml`,
      interval: form.interval,
    }

    // URL
    if (isClientMode) {
      config.url = selectedExternalSub.url
    } else {
      // 妙妙屋处理模式，URL 指向后端接口
      const baseUrl = typeof window !== 'undefined' ? window.location.origin : '{妙妙屋地址}'
      // 编辑模式使用实际 ID，新建模式使用占位符
      const configId = editingProxyProvider?.id || '{config_id}'
      config.url = `${baseUrl}/api/proxy-provider/${configId}?token=${userToken || '{user_token}'}`
    }

    // 下载代理
    if (form.proxy && form.proxy !== 'DIRECT') {
      config.proxy = form.proxy
    }

    // 文件大小限制
    if (form.size_limit > 0) {
      config['size-limit'] = form.size_limit
    }

    // 请求头
    if (form.header_user_agent || form.header_authorization) {
      config.header = {}
      if (form.header_user_agent) {
        config.header['User-Agent'] = form.header_user_agent.split(',').map((s: string) => s.trim())
      }
      if (form.header_authorization) {
        config.header['Authorization'] = [form.header_authorization]
      }
    }

    // 健康检查
    if (form.health_check_enabled) {
      config['health-check'] = {
        enable: true,
        url: form.health_check_url,
        interval: form.health_check_interval,
        timeout: form.health_check_timeout,
        lazy: form.health_check_lazy,
        'expected-status': form.health_check_expected_status,
      }
    }

    // 高级配置（仅客户端模式输出）
    if (isClientMode) {
      if (form.filter) {
        config.filter = form.filter
      }
      if (form.exclude_filter) {
        config['exclude-filter'] = form.exclude_filter
      }
      if (form.exclude_type.length > 0) {
        config['exclude-type'] = form.exclude_type.join('|')
      }
      // 将 override 表单转换为 JSON，然后解析为对象
      const overrideJSON = overrideFormToJSON(form.override)
      if (overrideJSON) {
        try {
          config.override = JSON.parse(overrideJSON)
        } catch {
          // 忽略无效JSON
        }
      }
    }

    // 生成YAML
    const yamlObj: Record<string, any> = {}
    yamlObj[form.name] = config

    return dumpYAML(yamlObj, { indent: 2, lineWidth: -1 })
  }

  // 获取文件内容
  const fileContentQuery = useQuery({
    queryKey: ['rule-file', editingFile?.filename],
    queryFn: async () => {
      if (!editingFile) return null
      const response = await api.get(`/api/admin/rules/${encodeURIComponent(editingFile.filename)}`)
      return response.data as {
        name: string
        content: string
        latest_version: number
      }
    },
    enabled: Boolean(editingFile && auth.accessToken),
    refetchOnWindowFocus: false,
  })

  // 查询配置文件内容（编辑配置用）
  const configFileContentQuery = useQuery({
    queryKey: ['subscribe-file-content', editingConfigFile?.filename],
    queryFn: async () => {
      if (!editingConfigFile) return null
      const response = await api.get(`/api/admin/subscribe-files/${encodeURIComponent(editingConfigFile.filename)}/content`)
      return response.data as { content: string }
    },
    enabled: Boolean(editingConfigFile && auth.accessToken),
    refetchOnWindowFocus: false,
  })

  // 查询节点列表（编辑节点用）
  const nodesQuery = useQuery({
    queryKey: ['nodes'],
    queryFn: async () => {
      const response = await api.get('/api/admin/nodes')
      return response.data as { nodes: Array<{ id: number; node_name: string; tag?: string }> }
    },
    enabled: Boolean(editNodesDialogOpen && auth.accessToken),
    refetchOnWindowFocus: false,
  })

  // 获取用户配置（包含节点排序）
  const userConfigQuery = useQuery({
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

  // 查询配置文件内容（编辑节点用）
  const nodesConfigQuery = useQuery({
    queryKey: ['nodes-config-content', editingNodesFile?.filename],
    queryFn: async () => {
      if (!editingNodesFile) return null
      const response = await api.get(`/api/admin/subscribe-files/${encodeURIComponent(editingNodesFile.filename)}/content`)
      return response.data as { content: string }
    },
    enabled: Boolean(editingNodesFile && auth.accessToken),
    refetchOnWindowFocus: false,
  })

  // 保存文件
  const saveMutation = useMutation({
    mutationFn: async (payload: { file: string; content: string }) => {
      const response = await api.put(`/api/admin/rules/${encodeURIComponent(payload.file)}`, {
        content: payload.content,
      })
      return response.data as { version: number }
    },
    onSuccess: () => {
      toast.success('规则已保存')
      setIsDirty(false)
      setValidationError(null)
      queryClient.invalidateQueries({ queryKey: ['rule-file', editingFile?.filename] })
      // 关闭编辑对话框
      setEditDialogOpen(false)
      setEditingFile(null)
      setEditorValue('')
    },
    onError: (error) => {
      handleServerError(error)
    },
  })

  // 保存配置文件内容
  const saveConfigMutation = useMutation({
    mutationFn: async (payload: { filename: string; content: string }) => {
      const response = await api.put(`/api/admin/subscribe-files/${encodeURIComponent(payload.filename)}/content`, {
        content: payload.content,
      })
      return response.data
    },
    onSuccess: () => {
      toast.success('配置已保存')
      queryClient.invalidateQueries({ queryKey: ['subscribe-file-content', editingConfigFile?.filename] })
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      setEditConfigDialogOpen(false)
      setEditingConfigFile(null)
      setConfigContent('')
    },
    onError: (error) => {
      handleServerError(error)
    },
  })



  // 当文件内容加载完成时，更新编辑器
  useEffect(() => {
    if (!fileContentQuery.data) return
    setEditorValue(fileContentQuery.data.content ?? '')
    setIsDirty(false)
    setValidationError(null)
  }, [fileContentQuery.data])

  // YAML 验证
  useEffect(() => {
    if (!editingFile || fileContentQuery.isLoading) return

    const timer = setTimeout(() => {
      const trimmed = editorValue.trim()
      if (!trimmed) {
        setValidationError('内容不能为空')
        return
      }

      try {
        parseYAML(editorValue)
        setValidationError(null)
      } catch (error) {
        const message = error instanceof Error ? error.message : 'YAML 解析失败'
        setValidationError(message)
      }
    }, 300)

    return () => clearTimeout(timer)
  }, [editorValue, editingFile, fileContentQuery.isLoading])

  // 加载配置文件内容
  useEffect(() => {
    if (!configFileContentQuery.data) return
    setConfigContent(configFileContentQuery.data.content ?? '')
  }, [configFileContentQuery.data])

  // 解析YAML配置并提取代理组（编辑节点用）
  useEffect(() => {
    if (!nodesConfigQuery.data?.content) return

    try {
      const parsed = parseYAML(nodesConfigQuery.data.content) as any
      if (parsed && parsed['proxy-groups']) {
        // 保留代理组的所有原始属性
        const groups = parsed['proxy-groups'].map((group: any) => ({
          ...group, // 保留所有原始属性
          name: group.name || '',
          type: group.type || '',
          proxies: Array.isArray(group.proxies) ? group.proxies : [],
          dialerProxyGroup: group['dialer-proxy-group'] || undefined,
        }))
        setProxyGroups(groups)
        proxyGroupsOriginalRef.current = JSON.stringify(groups)

        // Check for local draft — only when dialog is open
        if (editingNodesFile && editNodesDialogOpen) {
          const draftJson = localStorage.getItem(EDIT_NODES_DRAFT_KEY_PREFIX + editingNodesFile.id)
          if (draftJson) {
            try {
              const draft = JSON.parse(draftJson)
              // Compare only the meaningful fields to avoid false positives from property ordering
              const normalize = (gs: any[]) => gs.map(g => ({ name: g.name, type: g.type, proxies: g.proxies, use: g.use }))
              if (JSON.stringify(normalize(draft.proxyGroups)) !== JSON.stringify(normalize(groups))) {
                pendingNodesDraftRef.current = draft
                setIsNodesDraftRecoveryOpen(true)
              } else {
                localStorage.removeItem(EDIT_NODES_DRAFT_KEY_PREFIX + editingNodesFile.id)
              }
            } catch {
              localStorage.removeItem(EDIT_NODES_DRAFT_KEY_PREFIX + editingNodesFile.id)
            }
          }
        }
      }
    } catch (error) {
      console.error('解析YAML失败:', error)
      toast.error('解析配置文件失败')
    }
  }, [nodesConfigQuery.data, editNodesDialogOpen])

  // Write edit-nodes draft to localStorage when proxyGroups change
  useEffect(() => {
    if (!editNodesDialogOpen || !editingNodesFile) return
    const normalize = (gs: any[]) => gs.map(g => ({ name: g.name, type: g.type, proxies: g.proxies, use: g.use }))
    if (JSON.stringify(normalize(proxyGroups)) === JSON.stringify(normalize(JSON.parse(proxyGroupsOriginalRef.current || '[]')))) return
    localStorage.setItem(
      EDIT_NODES_DRAFT_KEY_PREFIX + editingNodesFile.id,
      JSON.stringify({ proxyGroups, savedAt: Date.now() })
    )
  }, [proxyGroups, editNodesDialogOpen, editingNodesFile])

  const handleRecoverNodesDraft = () => {
    const draft = pendingNodesDraftRef.current
    if (!draft) return
    setProxyGroups(draft.proxyGroups)
    setIsNodesDraftRecoveryOpen(false)
    pendingNodesDraftRef.current = null
  }

  const handleDiscardNodesDraft = () => {
    if (editingNodesFile) {
      localStorage.removeItem(EDIT_NODES_DRAFT_KEY_PREFIX + editingNodesFile.id)
    }
    setIsNodesDraftRecoveryOpen(false)
    pendingNodesDraftRef.current = null
  }

  const handleEdit = (file: SubscribeFile) => {
    setEditingFile(file)
    setEditDialogOpen(true)
    // 不要立即清空 editorValue，等待 useEffect 从 fileContentQuery 加载数据
    setIsDirty(false)
    setValidationError(null)
  }

  const handleSave = () => {
    if (!editingFile) return
    try {
      parseYAML(editorValue || '')
      setValidationError(null)
    } catch (error) {
      const message = error instanceof Error ? error.message : 'YAML 解析失败'
      setValidationError(message)
      toast.error('保存失败，YAML 格式错误')
      return
    }

    saveMutation.mutate({ file: editingFile.filename, content: editorValue })
  }

  const handleReset = () => {
    if (!fileContentQuery.data) return
    setEditorValue(fileContentQuery.data.content ?? '')
    setIsDirty(false)
    setValidationError(null)
  }

  const handleImport = () => {
    if (!importForm.name || !importForm.url) {
      toast.error('请填写订阅名称和链接')
      return
    }
    importMutation.mutate(importForm)
  }

  const handleUpload = () => {
    if (!uploadFile) {
      toast.error('请选择文件')
      return
    }
    if (uploadForm.overwrite_id === 0 && !uploadForm.name && !uploadFile.name) {
      toast.error('请填写订阅名称')
      return
    }
    uploadMutation.mutate()
  }

  const handleDelete = (id: number) => {
    deleteMutation.mutate(id)
  }

  const handleEditMetadata = (file: SubscribeFile) => {
    setEditingMetadata(file)
    const nodeIDs = file.selected_node_ids || []
    const tags = file.selected_tags || []
    setMetadataForm({
      name: file.name,
      description: file.description,
      filename: file.filename,
      template_filename: file.template_filename || '',
      selected_tags: tags,
      selected_node_ids: nodeIDs,
      expire: file.expire_at ? new Date(file.expire_at) : undefined,
      traffic_limit: file.traffic_limit != null ? String(file.traffic_limit) : '',
      stats_server_ids: file.stats_server_ids || '',
    })
    // 自动判 mode:有 node_ids → node 模式;只有 tags → tag 模式(老数据)
    setPickerMode(nodeIDs.length > 0 ? 'node' : (tags.length > 0 ? 'tag' : 'node'))
    setEditMetadataDialogOpen(true)
  }

  const handleUpdateMetadata = () => {
    if (!editingMetadata) return
    if (!metadataForm.name.trim()) {
      toast.error('请填写订阅名称')
      return
    }
    if (!metadataForm.filename.trim()) {
      toast.error('请填写文件名')
      return
    }
    updateMetadataMutation.mutate({
      id: editingMetadata.id,
      data: {
        name: metadataForm.name,
        description: metadataForm.description,
        filename: metadataForm.filename,
        template_filename: metadataForm.template_filename || null,
        // 节点 mode → 提交 node_ids 清空 tags;tag mode → 反之
        selected_tags: pickerMode === 'node' ? [] : metadataForm.selected_tags,
        selected_node_ids: pickerMode === 'node' ? metadataForm.selected_node_ids : [],
        expire_at: metadataForm.expire
          ? (() => {
              const endOfDay = new Date(metadataForm.expire)
              endOfDay.setHours(23, 59, 59, 999)
              return endOfDay.toISOString()
            })()
          : '',
        traffic_limit: metadataForm.traffic_limit ? parseFloat(metadataForm.traffic_limit) : null,
        stats_server_ids: metadataForm.stats_server_ids,
      },
    })
  }

  const handleEditConfig = (file: SubscribeFile) => {
    setEditingConfigFile(file)
    setEditConfigDialogOpen(true)
  }

  const handleSaveConfig = () => {
    if (!editingConfigFile) return

    let parsed: any
    try {
      parsed = parseYAML(configContent || '')
    } catch (error) {
      const message = error instanceof Error ? error.message : 'YAML 解析失败'
      toast.error('保存失败，YAML 格式错误：' + message)
      return
    }

    // 校验配置有效性
    const clashValidationResult = validateClashConfig(parsed)

    if (!clashValidationResult.valid) {
      // 有错误级别的问题，阻止保存
      const errorMessage = formatValidationIssues(clashValidationResult.issues)
      toast.error('配置校验失败', {
        description: errorMessage,
        duration: 10000
      })
      console.error('Clash配置校验失败:', clashValidationResult.issues)
      return
    }

    // 准备保存的内容
    let contentToSave = configContent

    // 如果有自动修复的内容，使用修复后的配置
    if (clashValidationResult.fixedConfig) {
      contentToSave = dumpYAML(clashValidationResult.fixedConfig, { lineWidth: -1, noRefs: true })

      // 显示修复提示
      const warningIssues = clashValidationResult.issues.filter(i => i.level === 'warning')
      if (warningIssues.length > 0) {
        toast.warning('配置已自动修复', {
          description: formatValidationIssues(warningIssues),
          duration: 8000
        })
      }
    }

    saveConfigMutation.mutate({ filename: editingConfigFile.filename, content: contentToSave })
  }

  const handleEditNodes = (file: SubscribeFile) => {
    setEditingNodesFile(file)
    setEditNodesDialogOpen(true)
    setShowAllNodes(false)
  }

  // 验证 rules 中的节点是否存在于 proxy-groups 或 proxies 中
  const validateRulesNodes = (parsedConfig: any) => {
    const rules = parsedConfig.rules || []
    const proxyGroupNames = new Set(parsedConfig['proxy-groups']?.map((g: any) => g.name) || [])
    const proxyNames = new Set(parsedConfig.proxies?.map((p: any) => p.name) || [])

    // 添加特殊节点
    proxyGroupNames.add('DIRECT')
    proxyGroupNames.add('REJECT')
    proxyGroupNames.add('PROXY')
    proxyGroupNames.add('no-resolve')

    const missingNodes = new Set<string>()

    // 检查每条规则
    rules.forEach((rule: any, index: number) => {
      let nodeName: string | null = null

      if (typeof rule === 'string') {
        // 字符串格式的规则: "DOMAIN-SUFFIX,google.com,PROXY_GROUP"
        const parts = rule.split(',')
        if (parts.length < 2) return
        nodeName = parts[parts.length - 1].trim()
      } else if (typeof rule === 'object' && rule !== null) {
        // 对象格式的规则，查找可能的节点字段
        nodeName = rule.target || rule.group || rule.proxy || rule.ruleset
      } else {
        return
      }

      // 如果节点名称不在 proxy-groups 和 proxies 中，添加到缺失列表
      if (nodeName && !proxyGroupNames.has(nodeName) && !proxyNames.has(nodeName)) {
        console.log(`[validateRulesNodes] 发现缺失节点: "${nodeName}"`)
        missingNodes.add(nodeName)
      }
    })

    return {
      missingNodes: Array.from(missingNodes)
    }
  }

  // 应用缺失节点替换
  const handleApplyReplacement = async () => {
    try {
      const parsedConfig = parseYAML(pendingConfigAfterSave) as any
      const rules = parsedConfig.rules || []
      const proxyGroupNames = new Set(parsedConfig['proxy-groups']?.map((g: any) => g.name) || [])
      const proxyNames = new Set(parsedConfig.proxies?.map((p: any) => p.name) || [])

      // 添加特殊节点
      proxyGroupNames.add('DIRECT')
      proxyGroupNames.add('REJECT')
      proxyGroupNames.add('PROXY')
      proxyGroupNames.add('no-resolve')

      // 替换 rules 中缺失的节点
      parsedConfig.rules = rules.map((rule: any) => {
        if (typeof rule === 'string') {
          const parts = rule.split(',')
          if (parts.length < 2) return rule
          const nodeName = parts[parts.length - 1].trim()
          // 如果节点缺失（不在代理组和节点中），替换为用户选择的值
          if (nodeName && !proxyGroupNames.has(nodeName) && !proxyNames.has(nodeName)) {
            parts[parts.length - 1] = replacementChoice
            return parts.join(',')
          }
        } else if (typeof rule === 'object' && rule !== null) {
          // 对象格式的规则，检查并替换可能的节点字段
          const nodeName = rule.target || rule.group || rule.proxy || rule.ruleset
          if (nodeName && !proxyGroupNames.has(nodeName) && !proxyNames.has(nodeName)) {
            const updatedRule = { ...rule }
            if (updatedRule.target) updatedRule.target = replacementChoice
            else if (updatedRule.group) updatedRule.group = replacementChoice
            else if (updatedRule.proxy) updatedRule.proxy = replacementChoice
            else if (updatedRule.ruleset) updatedRule.ruleset = replacementChoice
            return updatedRule
          }
        }

        return rule
      })

      // 转换回YAML
      const finalConfig = dumpYAML(parsedConfig, { lineWidth: -1, noRefs: true })

      // 保存到文件
      if (editingNodesFile) {
        await api.put(`/api/admin/subscribe-files/${encodeURIComponent(editingNodesFile.filename)}/content`, {
          content: finalConfig,
        })
      }

      setConfigContent(finalConfig)

      // 更新查询缓存
      queryClient.setQueryData(['nodes-config-content', editingNodesFile?.filename], {
        content: finalConfig
      })
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })

      // 关闭替换对话框和编辑节点对话框
      setMissingNodesDialogOpen(false)
      setEditNodesDialogOpen(false)
      if (editingNodesFile) {
        localStorage.removeItem(EDIT_NODES_DRAFT_KEY_PREFIX + editingNodesFile.id)
      }
      toast.success(`已保存节点配置（缺失节点已替换为 ${replacementChoice}）`)
    } catch (error) {
      const message = error instanceof Error ? error.message : '应用替换失败'
      toast.error(message)
      console.error('应用替换失败:', error)
    }
  }

  const handleSaveNodes = async () => {
    if (!editingNodesFile) return

    // 使用当前的 configContent（可能已经被 handleRenameGroup 修改过），如果没有则使用查询数据
    const currentContent = configContent || nodesConfigQuery.data?.content
    if (!currentContent) return

    // 辅助函数：重新排序节点属性，确保 name, type, server, port 在前4位
    const reorderProxyProperties = (proxy: any) => {
      const orderedProxy: any = {}
      // 前4个属性按顺序添加
      if ('name' in proxy) orderedProxy.name = proxy.name
      if ('type' in proxy) orderedProxy.type = proxy.type
      if ('server' in proxy) orderedProxy.server = proxy.server
      // 确保 port 是数字类型，而不是字符串
      if ('port' in proxy) {
        orderedProxy.port = typeof proxy.port === 'string' ? parseInt(proxy.port, 10) : proxy.port
      }
      // 添加其他所有属性
      Object.keys(proxy).forEach(key => {
        if (!['name', 'type', 'server', 'port'].includes(key)) {
          orderedProxy[key] = proxy[key]
        }
      })
      return orderedProxy
    }

    try {
      let parsed = parseYAML(currentContent) as any

      // 获取所有 MMW 模式代理集合的名称（用于后续检查）
      const allMmwProviderNames = proxyProviderConfigs
        .filter(c => c.process_mode === 'mmw')
        .map(c => c.name)

      // 先收集所有被使用的代理集合，提前获取它们的节点名称
      // 这样在过滤 proxies 时可以保留这些节点
      const usedProviderNames = new Set<string>()
      proxyGroups.forEach(group => {
        // 从 use 属性收集（客户端模式）
        if (group.use) {
          group.use.forEach(provider => usedProviderNames.add(provider))
        }
        // 从 proxies 属性收集 MMW 代理集合的引用（MMW 模式下代理集合名称作为代理组名称出现在 proxies 中）
        if (group.proxies) {
          group.proxies.forEach(proxy => {
            if (allMmwProviderNames.includes(proxy)) {
              usedProviderNames.add(proxy)
            }
          })
        }
      })

      // 筛选 MMW 模式的代理集合
      const mmwProviderConfigs = proxyProviderConfigs.filter(
        c => usedProviderNames.has(c.name) && c.process_mode === 'mmw'
      )

      // 获取 MMW 节点数据（提前获取，用于保留已有节点）
      const mmwNodesMap: Record<string, { nodes: any[], prefix: string }> = {}
      const mmwNodeNames = new Set<string>() // 所有 MMW 节点名称
      for (const config of mmwProviderConfigs) {
        try {
          const resp = await api.get(`/api/user/proxy-provider-nodes?id=${config.id}`)
          if (resp.data && resp.data.nodes) {
            mmwNodesMap[config.name] = resp.data
            // 收集所有 MMW 节点名称（带前缀）
            resp.data.nodes.forEach((node: any) => {
              mmwNodeNames.add(resp.data.prefix + node.name)
            })
          }
        } catch (err) {
          console.error(`获取代理集合 ${config.name} 节点失败:`, err)
        }
      }

      // 收集所有代理组中使用的节点名称
      const usedNodeNames = new Set<string>()
      proxyGroups.forEach(group => {
        group.proxies.forEach(proxy => {
          // 只添加实际节点（不是DIRECT、REJECT等特殊节点，也不是其他代理组）
          if (!['DIRECT', 'REJECT', 'PROXY', 'no-resolve'].includes(proxy) &&
              !proxyGroups.some(g => g.name === proxy)) {
            usedNodeNames.add(proxy)
          }
        })
      })

      // 如果有使用的节点，从nodesQuery获取它们的配置
      if (usedNodeNames.size > 0 && nodesQuery.data?.nodes) {
        // 获取使用的节点的Clash配置
        const nodeConfigs: any[] = []
        // 创建节点名称到节点ID的映射（用于后续排序）
        const nodeNameToIdMap = new Map<string, number>()

        nodesQuery.data.nodes.forEach((node: any) => {
          if (usedNodeNames.has(node.node_name) && node.clash_config) {
            try {
              const clashConfig = typeof node.clash_config === 'string'
                ? JSON.parse(node.clash_config)
                : node.clash_config
              // 重新排序属性，确保 name, type, server, port 在前4位
              const orderedConfig = reorderProxyProperties(clashConfig)
              nodeConfigs.push(orderedConfig)
              // 记录节点名称到ID的映射
              nodeNameToIdMap.set(node.node_name, node.id)
            } catch (e) {
              console.error(`解析节点 ${node.node_name} 的配置失败:`, e)
            }
          }
        })

        // 应用节点排序：根据用户配置的 node_order 对节点进行排序
        if (nodeConfigs.length > 0 && userConfigQuery.data?.node_order) {
          const nodeOrder = userConfigQuery.data.node_order
          // 创建节点ID到排序位置的映射
          const orderMap = new Map<number, number>()
          nodeOrder.forEach((id, index) => orderMap.set(id, index))

          // 按照 node_order 排序节点配置
          nodeConfigs.sort((a, b) => {
            const aId = nodeNameToIdMap.get(a.name)
            const bId = nodeNameToIdMap.get(b.name)

            const aOrder = aId !== undefined ? (orderMap.get(aId) ?? Infinity) : Infinity
            const bOrder = bId !== undefined ? (orderMap.get(bId) ?? Infinity) : Infinity

            return aOrder - bOrder
          })
        }

        // 更新proxies部分
        if (nodeConfigs.length > 0) {
          // 保留现有的proxies中不在usedNodeNames中的节点
          const existingProxies = parsed.proxies || []

          // 合并：使用新的节点配置，添加现有但未使用的节点
          const updatedProxies = [...nodeConfigs]

          // 只保留 MMW 代理集合的节点，移除其他未使用的节点
          existingProxies.forEach((proxy: any) => {
            if (!usedNodeNames.has(proxy.name) && !updatedProxies.some(p => p.name === proxy.name)) {
              // 只有 MMW 节点才保留（因为它们是通过代理集合同步的）
              if (mmwNodeNames.has(proxy.name)) {
                updatedProxies.push(reorderProxyProperties(proxy))
              }
              // 其他未使用的节点不再保留，会从 proxies 列表中移除
            }
          })

          parsed.proxies = updatedProxies
        }
      } else {
        // 如果没有使用的节点，保留原有的proxies或设置为空数组
        if (!parsed.proxies) {
          parsed.proxies = []
        }
      }

      // 处理链式代理：根据代理组的 dialerProxyGroup 配置添加 dialer-proxy
      if (parsed.proxies && Array.isArray(parsed.proxies)) {
        const nodeProtocolMap = new Map<string, string>()
        const nodeIDToName = new Map<number, string>()
        const nodeNameToChainID = new Map<string, number | null | undefined>()
        if (nodesQuery.data?.nodes) {
          nodesQuery.data.nodes.forEach((node: any) => {
            nodeProtocolMap.set(node.node_name, node.protocol)
            nodeIDToName.set(node.id, node.node_name)
            nodeNameToChainID.set(node.node_name, node.chain_proxy_node_id)
          })
        }

        // 为有 chain_proxy_node_id 的链式代理节点注入 dialer-proxy
        parsed.proxies.forEach((proxy: any) => {
          const chainID = nodeNameToChainID.get(proxy.name)
          if (chainID) {
            const targetName = nodeIDToName.get(chainID)
            if (targetName) {
              proxy['dialer-proxy'] = targetName
            }
          }
        })

        // 中转组：为有 relay_group_node_ids 的节点注入 dialer-proxy，按组名去重生成代理组
        if (nodesQuery.data?.nodes) {
          const relayGroupMap = new Map<string, { name: string; proxies: string[] }>()
          nodesQuery.data.nodes.forEach((node: any) => {
            if (node.relay_group_name && node.relay_group_node_ids?.length > 0) {
              const proxy = parsed.proxies.find((p: any) => p.name === node.node_name)
              if (proxy && !proxy['dialer-proxy']) {
                proxy['dialer-proxy'] = node.relay_group_name
              }
              if (!relayGroupMap.has(node.relay_group_name)) {
                const groupProxies = node.relay_group_node_ids
                  .map((id: number) => nodeIDToName.get(id))
                  .filter(Boolean) as string[]
                if (groupProxies.length > 0) {
                  relayGroupMap.set(node.relay_group_name, { name: node.relay_group_name, proxies: groupProxies })
                }
              }
            }
          })
          if (relayGroupMap.size > 0) {
            if (!parsed['proxy-groups']) parsed['proxy-groups'] = []
            relayGroupMap.forEach(rg => {
              parsed['proxy-groups'].push({
                name: rg.name,
                type: 'url-test',
                proxies: rg.proxies,
                url: 'http://www.gstatic.com/generate_204',
                interval: 300,
                tolerance: 50,
              })
            })
          }
        }

        for (const group of proxyGroups) {
          if (!group.dialerProxyGroup) continue
          if (!proxyGroups.some(g => g.name === group.dialerProxyGroup)) continue

          const nodeNames = new Set(group.proxies.filter((p): p is string => p !== undefined))
          parsed.proxies = parsed.proxies.map((proxy: any) => {
            if (nodeNames.has(proxy.name)) {
              const protocol = nodeProtocolMap.get(proxy.name)
              if (protocol && protocol.includes('⇋')) return proxy
              return { ...proxy, 'dialer-proxy': group.dialerProxyGroup }
            }
            return proxy
          })
        }
      }

      // 更新代理组，保留 use 字段
      if (parsed && parsed['proxy-groups']) {
        parsed['proxy-groups'] = proxyGroups.map(group => {
          const groupConfig: any = {
            ...group, // 保留所有原始属性（如 url, interval, strategy 等）
            proxies: group.proxies, // 更新 proxies
          }
          // 保留 use 字段（代理集合引用）
          if (group.use && group.use.length > 0) {
            groupConfig.use = group.use
          }
          // 保存中转代理组配置
          if (group.dialerProxyGroup) {
            groupConfig['dialer-proxy-group'] = group.dialerProxyGroup
          }
          delete groupConfig.dialerProxyGroup
          return groupConfig
        })
      }

      // 为预置代理组添加 rules 和 rule-providers
      if (proxyGroupCategories.length > 0 && proxyGroups.length > 0) {
        // 创建代理组名称到分类的映射
        const categoryMap = new Map(
          proxyGroupCategories.map(cat => [cat.group_label, cat])
        )

        // 收集需要添加的分类（只包含用户添加的预置代理组）
        const selectedCategories: string[] = []
        proxyGroups.forEach(group => {
          const category = categoryMap.get(group.name)
          if (category) {
            selectedCategories.push(category.name)
          }
        })

        if (selectedCategories.length > 0) {
          // 构建 rule-providers
          const ruleProviders: Record<string, any> = parsed['rule-providers'] || {}

          for (const categoryName of selectedCategories) {
            const category = proxyGroupCategories.find(c => c.name === categoryName)
            if (!category) continue

            // 添加 site rule providers
            for (const provider of category.site_rules) {
              if (!ruleProviders[provider.key]) {
                ruleProviders[provider.key] = {
                  type: provider.type,
                  format: provider.format,
                  behavior: provider.behavior,
                  url: provider.url,
                  path: provider.path,
                  interval: provider.interval,
                }
              }
            }

            // 添加 IP rule providers
            for (const provider of category.ip_rules) {
              if (!ruleProviders[provider.key]) {
                ruleProviders[provider.key] = {
                  type: provider.type,
                  format: provider.format,
                  behavior: provider.behavior,
                  url: provider.url,
                  path: provider.path,
                  interval: provider.interval,
                }
              }
            }
          }

          parsed['rule-providers'] = ruleProviders

          // 构建 rules（domain-based 规则在前，IP-based 规则在后）
          const existingRules: string[] = parsed.rules || []
          const newRules: string[] = []

          // 先添加 site rules（domain-based）
          for (const categoryName of selectedCategories) {
            const category = proxyGroupCategories.find(c => c.name === categoryName)
            if (!category || !category.rule_name) continue

            const outbound = category.group_label || translateOutbound(category.rule_name)

            // Site rules
            for (const provider of category.site_rules) {
              const ruleStr = `RULE-SET,${provider.key},${outbound}`
              if (!existingRules.includes(ruleStr) && !newRules.includes(ruleStr)) {
                newRules.push(ruleStr)
              }
            }
          }

          // 再添加 IP rules
          for (const categoryName of selectedCategories) {
            const category = proxyGroupCategories.find(c => c.name === categoryName)
            if (!category || !category.rule_name) continue

            const outbound = category.group_label || translateOutbound(category.rule_name)

            // IP rules
            for (const provider of category.ip_rules) {
              const ruleStr = `RULE-SET,${provider.key},${outbound},no-resolve`
              if (!existingRules.includes(ruleStr) && !newRules.includes(ruleStr)) {
                newRules.push(ruleStr)
              }
            }
          }

          // 合并新规则到现有规则中（插入到 MATCH 规则之前）
          const matchRuleIndex = existingRules.findIndex(r => r.startsWith('MATCH,'))
          if (matchRuleIndex >= 0) {
            // 在 MATCH 规则之前插入新规则
            parsed.rules = [
              ...existingRules.slice(0, matchRuleIndex),
              ...newRules,
              ...existingRules.slice(matchRuleIndex)
            ]
          } else {
            // 如果没有 MATCH 规则，追加到末尾
            parsed.rules = [...existingRules, ...newRules]
          }
        }
      }

      // 筛选非 MMW 模式的代理集合（MMW 相关数据已在函数开头获取）
      const nonMmwProviders = proxyProviderConfigs.filter(
        c => usedProviderNames.has(c.name) && c.process_mode !== 'mmw'
      )

      // 找出不再被使用的 MMW 代理集合（需要清理其自动创建的代理组和节点）
      // allMmwProviderNames 已在函数开头定义
      const unusedMmwProviders = allMmwProviderNames.filter(name => !usedProviderNames.has(name))

      // 清理不再使用的 MMW 代理集合的自动创建代理组和节点
      if (unusedMmwProviders.length > 0 && parsed['proxy-groups']) {
        // 删除自动创建的代理组（名称与代理集合相同的代理组）
        parsed['proxy-groups'] = parsed['proxy-groups'].filter((group: any) => {
          if (unusedMmwProviders.includes(group.name)) {
            console.log(`[MMW清理] 删除不再使用的代理组: ${group.name}`)
            return false
          }
          return true
        })

        // 删除这些代理集合的节点（根据前缀匹配）
        if (parsed.proxies && Array.isArray(parsed.proxies)) {
          // 构建需要清理的节点前缀列表
          const prefixesToRemove: string[] = []
          for (const providerName of unusedMmwProviders) {
            // 根据代理集合名称计算前缀
            let namePrefix = providerName
            if (providerName.includes('-')) {
              namePrefix = providerName.substring(0, providerName.indexOf('-'))
            }
            const prefix = `〖${namePrefix}〗`
            prefixesToRemove.push(prefix)
          }

          // 过滤掉匹配这些前缀的节点
          const beforeCount = parsed.proxies.length
          parsed.proxies = parsed.proxies.filter((proxy: any) => {
            const proxyName = proxy.name || ''
            for (const prefix of prefixesToRemove) {
              if (proxyName.startsWith(prefix)) {
                console.log(`[MMW清理] 删除节点: ${proxyName}`)
                return false
              }
            }
            return true
          })
          const removedCount = beforeCount - parsed.proxies.length
          if (removedCount > 0) {
            console.log(`[MMW清理] 共删除 ${removedCount} 个节点`)
          }
        }
      }

      // 处理 MMW 模式的代理集合（与获取订阅逻辑一致）
      if (Object.keys(mmwNodesMap).length > 0) {
        // 1. 更新使用 MMW 代理集合的代理组
        parsed['proxy-groups'] = parsed['proxy-groups'].map((group: any) => {
          const groupConfig: any = { ...group }

          if (group.use && group.use.length > 0) {
            const newUse: string[] = []
            const mmwGroupNames: string[] = []

            group.use.forEach((providerName: string) => {
              if (mmwNodesMap[providerName]) {
                // MMW 模式：添加代理组名称（而非节点名称）
                mmwGroupNames.push(providerName)
              } else {
                // 非 MMW 模式：保留 use 引用
                newUse.push(providerName)
              }
            })

            // 添加 MMW 代理组名称到 proxies
            if (mmwGroupNames.length > 0) {
              groupConfig.proxies = [...(groupConfig.proxies || []), ...mmwGroupNames]
            }

            // 只保留非 MMW 的 use 引用
            if (newUse.length > 0) {
              groupConfig.use = newUse
            } else {
              delete groupConfig.use
            }
          }

          return groupConfig
        })

        // 2. 为每个 MMW 代理集合创建或更新对应的代理组
        const mmwGroupsToAdd: any[] = []
        for (const [providerName, data] of Object.entries(mmwNodesMap)) {
          const nodeNames = data.nodes.map((node: any) => data.prefix + node.name)

          // 检查是否已存在同名代理组
          const existingGroupIndex = parsed['proxy-groups']?.findIndex(
            (g: any) => g.name === providerName
          )

          if (existingGroupIndex >= 0) {
            // 更新已存在的代理组的 proxies
            parsed['proxy-groups'][existingGroupIndex].proxies = nodeNames
          } else {
            // 创建新代理组（类型为 url-test）
            mmwGroupsToAdd.push({
              name: providerName,
              type: 'url-test',
              url: 'http://www.gstatic.com/generate_204',
              interval: 300,
              tolerance: 50,
              proxies: nodeNames
            })
          }
        }

        // 3. 将新创建的 MMW 代理组追加到 proxy-groups 末尾
        if (mmwGroupsToAdd.length > 0) {
          parsed['proxy-groups'] = [
            ...parsed['proxy-groups'],
            ...mmwGroupsToAdd
          ]
        }

        // 4. 添加 MMW 节点到 proxies
        for (const [, data] of Object.entries(mmwNodesMap)) {
          data.nodes.forEach((node: any) => {
            const prefixedNode = reorderProxyProperties({ ...node, name: data.prefix + node.name })
            // 检查是否已存在同名节点
            const existingIndex = parsed.proxies?.findIndex((p: any) => p.name === prefixedNode.name)
            if (existingIndex >= 0) {
              parsed.proxies[existingIndex] = prefixedNode
            } else {
              parsed.proxies.push(prefixedNode)
            }
          })
        }
      }

      // 只为非 MMW 代理集合生成 proxy-providers 配置
      if (nonMmwProviders.length > 0) {
        const providers: Record<string, any> = {}
        nonMmwProviders.forEach(config => {
          const baseUrl = window.location.origin
          const providerConfig: Record<string, any> = {
            type: config.type || 'http',
            path: `./proxy_providers/${config.name}.yaml`,
            url: `${baseUrl}/api/proxy-provider/${config.id}?token=${userToken}`,
            interval: config.interval || 3600,
          }
          if (config.health_check_enabled) {
            providerConfig['health-check'] = {
              enable: true,
              url: config.health_check_url || 'http://www.gstatic.com/generate_204',
              interval: config.health_check_interval || 300,
            }
          }
          providers[config.name] = providerConfig
        })
        if (Object.keys(providers).length > 0) {
          parsed['proxy-providers'] = providers
        }
      }

      // 校验配置有效性
      const clashValidationResult = validateClashConfig(parsed)

      if (!clashValidationResult.valid) {
        // 有错误级别的问题，阻止保存
        const errorMessage = formatValidationIssues(clashValidationResult.issues)
        toast.error('配置校验失败', {
          description: errorMessage,
          duration: 10000
        })
        console.error('Clash配置校验失败:', clashValidationResult.issues)
        return
      }

      // 如果有自动修复的内容，使用修复后的配置
      if (clashValidationResult.fixedConfig) {
        parsed = clashValidationResult.fixedConfig

        // 显示修复提示
        const warningIssues = clashValidationResult.issues.filter(i => i.level === 'warning')
        if (warningIssues.length > 0) {
          toast.warning('配置已自动修复', {
            description: formatValidationIssues(warningIssues),
            duration: 8000
          })
        }
      }

      // 转换回YAML
      const newContent = dumpYAML(parsed, { lineWidth: -1, noRefs: true })

      // 验证 rules 中引用的节点是否都存在
      const validationResult = validateRulesNodes(parsed)
      if (validationResult.missingNodes.length > 0) {
        // 有缺失的节点，显示替换对话框
        setMissingNodes(validationResult.missingNodes)
        setPendingConfigAfterSave(newContent)
        setMissingNodesDialogOpen(true)
      } else {
        // 没有缺失节点，直接保存到文件
        await api.put(`/api/admin/subscribe-files/${encodeURIComponent(editingNodesFile.filename)}/content`, {
          content: newContent,
        })
        setConfigContent(newContent)
        queryClient.setQueryData(['nodes-config-content', editingNodesFile?.filename], {
          content: newContent
        })
        queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
        setEditNodesDialogOpen(false)
        if (editingNodesFile) {
          localStorage.removeItem(EDIT_NODES_DRAFT_KEY_PREFIX + editingNodesFile.id)
        }
        toast.success('已保存节点配置')
      }
    } catch (error) {
      const message = error instanceof Error ? error.message : '应用配置失败'
      toast.error(message)
      console.error('应用节点配置失败:', error)
    }
  }

  const handleRemoveNodeFromGroup = (groupName: string, nodeIndex: number) => {
    const updatedGroups = proxyGroups.map(group => {
      if (group.name === groupName) {
        return {
          ...group,
          proxies: group.proxies.filter((_, idx) => idx !== nodeIndex)
        }
      }
      return group
    })
    setProxyGroups(updatedGroups)
  }

  // 删除整个代理组
  const handleRemoveGroup = (groupName: string) => {
    setProxyGroups(groups => {
      // 先过滤掉要删除的组
      const filteredGroups = groups.filter(group => group.name !== groupName)

      // 从所有剩余组的 proxies 列表中移除对被删除组的引用
      return filteredGroups.map(group => ({
        ...group,
        proxies: group.proxies.filter(proxy => proxy !== groupName)
      }))
    })
  }

  // 处理代理组改名
  const handleRenameGroup = (oldName: string, newName: string) => {
    setProxyGroups(groups => {
      // 更新被改名的组
      const updatedGroups = groups.map(group => {
        if (group.name === oldName) {
          return { ...group, name: newName }
        }
        // 更新其他组中对这个组的引用
        return {
          ...group,
          proxies: group.proxies.map(proxy => proxy === oldName ? newName : proxy)
        }
      })
      return updatedGroups
    })

    // 同时更新配置文件内容中的 rules 部分
    if (nodesConfigQuery.data?.content) {
      try {
        const parsed = parseYAML(nodesConfigQuery.data.content) as any
        if (parsed && parsed['rules'] && Array.isArray(parsed['rules'])) {
          // 更新 rules 中的代理组引用
          const updatedRules = parsed['rules'].map((rule: any) => {
            if (typeof rule === 'string') {
              // 规则格式: "DOMAIN-SUFFIX,google.com,PROXY_GROUP"
              const parts = rule.split(',')
              if (parts.length >= 3 && parts[2] === oldName) {
                parts[2] = newName
                return parts.join(',')
              }
            } else if (typeof rule === 'object' && rule.target) {
              // 对象格式的规则，更新 target 字段
              if (rule.target === oldName) {
                return { ...rule, target: newName }
              }
            }
            return rule
          })
          parsed['rules'] = updatedRules

          // 转换回YAML并更新配置内容
          const newContent = dumpYAML(parsed, { lineWidth: -1, noRefs: true })
          setConfigContent(newContent)

          // 更新 nodesConfigQuery 的缓存
          queryClient.setQueryData(['nodes-config-content', editingNodesFile?.filename], {
            content: newContent
          })
        }
      } catch (error) {
        console.error('更新配置文件中的代理组引用失败:', error)
      }
    }
  }

  // 计算可用节点（按节点管理的排序顺序）
  const availableNodes = useMemo(() => {
    if (!nodesQuery.data?.nodes) return []

    // 先按 node_order 排序节点
    const nodeOrder = userConfigQuery.data?.node_order || []
    const orderMap = new Map<number, number>()
    nodeOrder.forEach((id, index) => orderMap.set(id, index))

    const sortedNodes = [...nodesQuery.data.nodes].sort((a, b) => {
      const aOrder = orderMap.get(a.id) ?? Infinity
      const bOrder = orderMap.get(b.id) ?? Infinity
      return aOrder - bOrder
    })

    const allNodeNames = sortedNodes.map(n => n.node_name)

    if (showAllNodes) {
      return allNodeNames
    }

    // 获取所有代理组中已使用的节点
    const usedNodes = new Set<string>()
    proxyGroups.forEach(group => {
      group.proxies.forEach(proxy => usedNodes.add(proxy))
    })

    // 只返回未使用的节点
    return allNodeNames.filter(name => !usedNodes.has(name))
  }, [nodesQuery.data, proxyGroups, showAllNodes, userConfigQuery.data?.node_order])

  // 处理编辑节点对话框关闭
  const handleEditNodesDialogOpenChange = (open: boolean) => {
    if (!open) {
      // 先关闭对话框
      setEditNodesDialogOpen(false)

      // 延迟重置数据，避免用户看到复位动画
      setTimeout(() => {
        // 关闭时重新加载原始数据
        if (nodesConfigQuery.data?.content) {
          try {
            const parsed = parseYAML(nodesConfigQuery.data.content) as any
            if (parsed && parsed['proxy-groups']) {
              // 保留代理组的所有原始属性
              const groups = parsed['proxy-groups'].map((group: any) => ({
                ...group, // 保留所有原始属性
                name: group.name || '',
                type: group.type || '',
                proxies: Array.isArray(group.proxies) ? group.proxies : [],
              }))
              setProxyGroups(groups)
            }
          } catch (error) {
            console.error('重新加载配置失败:', error)
          }
        }
        setEditingNodesFile(null)
        setShowAllNodes(false)
      }, 200)
    } else {
      setEditNodesDialogOpen(open)
    }
  }

  return (
    <main className='mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24'>
      <section className='space-y-4'>
        <div className='flex flex-col gap-3 sm:gap-4'>
          <h1 className='text-3xl font-semibold tracking-tight'>订阅管理</h1>

          <div className='flex gap-2'>
            <p className='text-muted-foreground mt-2'>
              从Clash订阅链接导入或上传本地文件
            </p>
          </div>
        </div>

        <Card>
          <CardHeader>
            <div className='flex items-center justify-between'>
              <div>
                <CardTitle>订阅列表 ({files.length})</CardTitle>
                <CardDescription>已添加的订阅文件</CardDescription>
              </div>
              <div className='flex items-center gap-2'>
                {/* 上传文件 */}
                <Dialog open={uploadDialogOpen} onOpenChange={setUploadDialogOpen}>
                  <Tooltip>
                    <TooltipTrigger asChild>
                      <DialogTrigger asChild>
                        <Button variant='outline' size='sm'>
                          <Upload className='h-4 w-4' />
                        </Button>
                      </DialogTrigger>
                    </TooltipTrigger>
                    <TooltipContent>上传文件</TooltipContent>
                  </Tooltip>
                  <DialogContent>
                    <DialogHeader>
                      <DialogTitle>上传文件</DialogTitle>
                      <DialogDescription>
                        上传本地文件作为订阅配置，或覆盖已有订阅的文件内容
                      </DialogDescription>
                    </DialogHeader>
                    <div className='space-y-4 py-4'>
                      <div className='space-y-2'>
                        <Label>覆盖已有订阅</Label>
                        <Select
                          value={String(uploadForm.overwrite_id)}
                          onValueChange={(val) => setUploadForm({ ...uploadForm, overwrite_id: Number(val) })}
                        >
                          <SelectTrigger>
                            <SelectValue placeholder='新建订阅' />
                          </SelectTrigger>
                          <SelectContent>
                            <SelectItem value='0'>新建订阅</SelectItem>
                            {files.map((f) => (
                              <SelectItem key={f.id} value={String(f.id)}>
                                {f.name}
                              </SelectItem>
                            ))}
                          </SelectContent>
                        </Select>
                      </div>
                      <div className='flex items-center justify-between'>
                        <Label htmlFor='upload-raw-output'>这不是 Clash 配置</Label>
                        <Switch
                          id='upload-raw-output'
                          checked={uploadForm.raw_output}
                          onCheckedChange={(checked) => setUploadForm({ ...uploadForm, raw_output: checked })}
                        />
                      </div>
                      <div className='space-y-2'>
                        <Label htmlFor='upload-file'>选择文件 *</Label>
                        <Input
                          id='upload-file'
                          type='file'
                          accept={uploadForm.raw_output ? undefined : '.yaml,.yml'}
                          onChange={(e) => setUploadFile(e.target.files?.[0] || null)}
                        />
                      </div>
                      {uploadForm.overwrite_id === 0 && (
                        <>
                          <div className='space-y-2'>
                            <Label htmlFor='upload-name'>订阅名称（可选）</Label>
                            <Input
                              id='upload-name'
                              placeholder='留空则使用文件名'
                              value={uploadForm.name}
                              onChange={(e) => setUploadForm({ ...uploadForm, name: e.target.value })}
                            />
                          </div>
                          <div className='space-y-2'>
                            <Label htmlFor='upload-filename'>文件名（可选）</Label>
                            <Input
                              id='upload-filename'
                              placeholder='留空则使用原文件名'
                              value={uploadForm.filename}
                              onChange={(e) => setUploadForm({ ...uploadForm, filename: e.target.value })}
                            />
                          </div>
                          <div className='space-y-2'>
                            <Label htmlFor='upload-description'>说明（可选）</Label>
                            <Textarea
                              id='upload-description'
                              placeholder='订阅说明信息'
                              value={uploadForm.description}
                              onChange={(e) => setUploadForm({ ...uploadForm, description: e.target.value })}
                            />
                          </div>
                        </>
                      )}
                    </div>
                    <DialogFooter>
                      <Button variant='outline' onClick={() => setUploadDialogOpen(false)}>
                        取消
                      </Button>
                      <Button onClick={handleUpload} disabled={uploadMutation.isPending}>
                        {uploadMutation.isPending ? '上传中...' : (uploadForm.overwrite_id > 0 ? '覆盖上传' : '上传')}
                      </Button>
                    </DialogFooter>
                  </DialogContent>
                </Dialog>
                {/* 生成订阅 */}
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => navigate({ to: '/generator' })}
                    >
                      <Plus className='h-4 w-4' />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>生成订阅</TooltipContent>
                </Tooltip>
              </div>
            </div>
          </CardHeader>
          <CardContent>
            {isLoading ? (
              <div className='text-center py-8 text-muted-foreground'>加载中...</div>
            ) : files.length === 0 ? (
              <div className='text-center py-8 text-muted-foreground'>
                暂无订阅，点击上方按钮添加
              </div>
            ) : (
              <DataTable
                data={files}
                getRowKey={(file) => file.id}
                emptyText='暂无订阅，点击上方按钮添加'

                columns={[
                  {
                    header: '订阅名称',
                    cell: (file) => (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className='font-medium truncate block max-w-[16ch]'>{file.name}</span>
                        </TooltipTrigger>
                        <TooltipContent>{file.name}</TooltipContent>
                      </Tooltip>
                    ),
                  },
                  {
                    header: '',
                    cell: (file) => file.description ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Info className='h-4 w-4 text-muted-foreground cursor-help' />
                        </TooltipTrigger>
                        <TooltipContent className='max-w-xs'>
                          {file.description}
                        </TooltipContent>
                      </Tooltip>
                    ) : null,
                    width: '36px'
                  },
                  {
                    header: '过期时间',
                    cell: (file) => {
                      const handleQuickExpire = (days: number | 'expired' | Date) => {
                        let newExpireAt: string | null = null

                        if (days === 'expired') {
                          newExpireAt = new Date().toISOString()
                        } else if (days instanceof Date) {
                          newExpireAt = days.toISOString()
                        } else {
                          const baseDate = file.expire_at ? new Date(file.expire_at) : new Date()
                          newExpireAt = addDays(baseDate, days).toISOString()
                        }

                        updateMetadataMutation.mutate({
                          id: file.id,
                          data: {
                            name: file.name,
                            description: file.description,
                            auto_sync_custom_rules: file.auto_sync_custom_rules,
                            expire_at: newExpireAt,
                          }
                        }, {
                          onSuccess: () => {
                            setExpirePopoverFileId(null)
                            setCustomDateFileId(null)
                            toast.success('过期时间已更新')
                          }
                        })
                      }

                      const getExpirationStatus = () => {
                        if (!file.expire_at) return null

                        const expireDate = new Date(file.expire_at)

                        // 先检查是否是今天过期
                        if (isToday(expireDate)) {
                          return { status: 'expiring', label: '今天过期', className: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400' }
                        }

                        // 再检查是否已过期
                        if (isPast(expireDate)) {
                          return { status: 'expired', label: '已过期', className: 'bg-red-500/10 text-red-700 dark:text-red-400' }
                        }

                        // 计算剩余天数（使用日历天数）
                        const daysRemaining = differenceInCalendarDays(expireDate, new Date())

                        if (daysRemaining <= 7) {
                          return { status: 'expiring', label: `${daysRemaining}天后过期`, className: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400' }
                        }

                        return { status: 'valid', label: format(expireDate, 'yyyy-MM-dd HH:mm'), className: 'bg-green-500/10 text-green-700 dark:text-green-400' }
                      }

                      const expirationStatus = getExpirationStatus()

                      return (
                        <Popover
                          open={expirePopoverFileId === file.id}
                          onOpenChange={(open) => setExpirePopoverFileId(open ? file.id : null)}
                        >
                          <PopoverTrigger asChild>
                            <Button
                              variant='ghost'
                              size='sm'
                              className='h-8 px-2 whitespace-nowrap'
                            >
                              {expirationStatus ? (
                                <Badge variant='outline' className={expirationStatus.className}>
                                  {expirationStatus.label}
                                </Badge>
                              ) : (
                                <span className='text-sm text-muted-foreground'>未设置</span>
                              )}
                            </Button>
                          </PopoverTrigger>
                          <PopoverContent className='w-auto p-2' align='start'>
                            <div className='flex flex-col gap-2'>
                              <Button
                                variant='outline'
                                size='sm'
                                onClick={() => handleQuickExpire(30)}
                                disabled={updateMetadataMutation.isPending}
                              >
                                延长30天
                              </Button>
                              <Button
                                variant='outline'
                                size='sm'
                                onClick={() => handleQuickExpire('expired')}
                                disabled={updateMetadataMutation.isPending}
                              >
                                标记过期
                              </Button>
                              <Popover
                                open={customDateFileId === file.id}
                                onOpenChange={(open) => setCustomDateFileId(open ? file.id : null)}
                              >
                                <PopoverTrigger asChild>
                                  <Button
                                    variant='outline'
                                    size='sm'
                                  >
                                    <CalendarIcon className='mr-2 h-4 w-4' />
                                    选择时间
                                  </Button>
                                </PopoverTrigger>
                                <PopoverContent className='w-auto p-0' align='start'>
                                  <Calendar
                                    mode='single'
                                    selected={file.expire_at ? new Date(file.expire_at) : undefined}
                                    onSelect={(date) => {
                                      if (date) {
                                        handleQuickExpire(date)
                                      }
                                    }}
                                    disabled={(date) => date < new Date()}
                                    initialFocus
                                  />
                                </PopoverContent>
                              </Popover>
                              {file.expire_at && (
                                <Button
                                  variant='outline'
                                  size='sm'
                                  onClick={() => {
                                    updateMetadataMutation.mutate({
                                      id: file.id,
                                      data: {
                                        name: file.name,
                                        description: file.description,
                                        auto_sync_custom_rules: file.auto_sync_custom_rules,
                                        expire_at: null,
                                      }
                                    }, {
                                      onSuccess: () => {
                                        setExpirePopoverFileId(null)
                                        toast.success('已清除过期时间')
                                      }
                                    })
                                  }}
                                  disabled={updateMetadataMutation.isPending}
                                  className='text-destructive hover:text-destructive'
                                >
                                  清除过期时间
                                </Button>
                              )}
                            </div>
                          </PopoverContent>
                        </Popover>
                      )
                    },
                    width: '180px'
                  },
                  {
                    header: '覆写配置',
                    cell: (file) => {
                      const ruleIds = file.selected_custom_rule_ids || []
                      const scriptIds = file.selected_override_script_ids || []
                      const totalSelected = ruleIds.length + scriptIds.length
                      const totalAvailable = enabledCustomRules.length + enabledOverrideScripts.length
                      const isEnabled = file.auto_sync_custom_rules
                      return (
                        <Popover>
                          <PopoverTrigger asChild>
                            <Button
                              variant="outline"
                              size="sm"
                              className="w-[120px] h-8 text-xs justify-between"
                              disabled={updateMetadataMutation.isPending}
                            >
                              <span className="truncate">
                                {!isEnabled ? '未启用' : totalSelected === 0 ? `全部(${totalAvailable})` : `${totalSelected} 项`}
                              </span>
                              <ChevronDown className="h-3 w-3 shrink-0 opacity-50" />
                            </Button>
                          </PopoverTrigger>
                          <PopoverContent className="w-[240px] p-1" align="start">
                            <div className="flex flex-col max-h-[400px] overflow-y-auto">
                              <Button
                                variant="ghost"
                                size="sm"
                                className={cn("justify-start text-xs h-8", !isEnabled && "bg-accent")}
                                onClick={() => {
                                  updateMetadataMutation.mutate({
                                    id: file.id,
                                    data: {
                                      name: file.name,
                                      description: file.description,
                                      auto_sync_custom_rules: false,
                                      selected_custom_rule_ids: [],
                                      selected_override_script_ids: [],
                                    }
                                  }, { onSuccess: () => toast.success('已关闭覆写') })
                                }}
                              >
                                {!isEnabled && <Check className="h-3 w-3 mr-2" />}
                                <span className={!isEnabled ? '' : 'ml-5'}>不启用</span>
                              </Button>
                              <Button
                                variant="ghost"
                                size="sm"
                                className={cn("justify-start text-xs h-8", isEnabled && totalSelected === 0 && "bg-accent")}
                                onClick={() => {
                                  updateMetadataMutation.mutate({
                                    id: file.id,
                                    data: {
                                      name: file.name,
                                      description: file.description,
                                      auto_sync_custom_rules: true,
                                      selected_custom_rule_ids: [],
                                      selected_override_script_ids: [],
                                    }
                                  }, { onSuccess: () => toast.success('已启用全部覆写') })
                                }}
                              >
                                {isEnabled && totalSelected === 0 && <Check className="h-3 w-3 mr-2" />}
                                <span className={isEnabled && totalSelected === 0 ? '' : 'ml-5'}>全部启用</span>
                              </Button>
                              {enabledCustomRules.length > 0 && (
                                <>
                                  <div className="px-2 py-1.5 text-xs text-muted-foreground font-medium border-t mt-1 pt-1.5">自定义规则</div>
                                  {enabledCustomRules.map((rule) => {
                                    const isSelected = isEnabled && (totalSelected === 0 || ruleIds.includes(rule.id))
                                    return (
                                      <Button
                                        key={`rule-${rule.id}`}
                                        variant="ghost"
                                        size="sm"
                                        className={cn("justify-start text-xs h-8", isSelected && "bg-accent")}
                                        onClick={() => {
                                          let newRuleIds: number[]
                                          let newScriptIds: number[]
                                          if (!isEnabled) {
                                            newRuleIds = [rule.id]
                                            newScriptIds = []
                                          } else if (totalSelected === 0) {
                                            newRuleIds = enabledCustomRules.filter(r => r.id !== rule.id).map(r => r.id)
                                            newScriptIds = enabledOverrideScripts.map(s => s.id)
                                          } else {
                                            newRuleIds = ruleIds.includes(rule.id) ? ruleIds.filter(id => id !== rule.id) : [...ruleIds, rule.id]
                                            newScriptIds = scriptIds
                                          }
                                          if (newRuleIds.length === enabledCustomRules.length && newScriptIds.length === enabledOverrideScripts.length) {
                                            newRuleIds = []
                                            newScriptIds = []
                                          }
                                          const allEmpty = newRuleIds.length === 0 && newScriptIds.length === 0
                                          updateMetadataMutation.mutate({
                                            id: file.id,
                                            data: {
                                              name: file.name,
                                              description: file.description,
                                              auto_sync_custom_rules: !allEmpty,
                                              selected_custom_rule_ids: newRuleIds,
                                              selected_override_script_ids: newScriptIds,
                                            }
                                          })
                                        }}
                                      >
                                        {isSelected && <Check className="h-3 w-3 mr-2" />}
                                        <span className={isSelected ? '' : 'ml-5'}>{rule.name}</span>
                                        <Badge variant="outline" className="ml-auto text-[10px] px-1 py-0">{rule.type}</Badge>
                                      </Button>
                                    )
                                  })}
                                </>
                              )}
                              {enabledOverrideScripts.length > 0 && (
                                <>
                                  <div className="px-2 py-1.5 text-xs text-muted-foreground font-medium border-t mt-1 pt-1.5">覆写脚本</div>
                                  {enabledOverrideScripts.map((script) => {
                                    const isSelected = isEnabled && (totalSelected === 0 || scriptIds.includes(script.id))
                                    return (
                                      <Button
                                        key={`script-${script.id}`}
                                        variant="ghost"
                                        size="sm"
                                        className={cn("justify-start text-xs h-8", isSelected && "bg-accent")}
                                        onClick={() => {
                                          let newScriptIds: number[]
                                          let newRuleIds: number[]
                                          if (!isEnabled) {
                                            newScriptIds = [script.id]
                                            newRuleIds = []
                                          } else if (totalSelected === 0) {
                                            newScriptIds = enabledOverrideScripts.filter(s => s.id !== script.id).map(s => s.id)
                                            newRuleIds = enabledCustomRules.map(r => r.id)
                                          } else {
                                            newScriptIds = scriptIds.includes(script.id) ? scriptIds.filter(id => id !== script.id) : [...scriptIds, script.id]
                                            newRuleIds = ruleIds
                                          }
                                          if (newRuleIds.length === enabledCustomRules.length && newScriptIds.length === enabledOverrideScripts.length) {
                                            newRuleIds = []
                                            newScriptIds = []
                                          }
                                          const allEmpty = newRuleIds.length === 0 && newScriptIds.length === 0
                                          updateMetadataMutation.mutate({
                                            id: file.id,
                                            data: {
                                              name: file.name,
                                              description: file.description,
                                              auto_sync_custom_rules: !allEmpty,
                                              selected_custom_rule_ids: newRuleIds,
                                              selected_override_script_ids: newScriptIds,
                                            }
                                          })
                                        }}
                                      >
                                        {isSelected && <Check className="h-3 w-3 mr-2" />}
                                        <span className={isSelected ? '' : 'ml-5'}>{script.name}</span>
                                        <Badge variant="outline" className="ml-auto text-[10px] px-1 py-0">{script.hook === 'post_fetch' ? '获取后' : '保存前'}</Badge>
                                      </Button>
                                    )
                                  })}
                                </>
                              )}
                            </div>
                          </PopoverContent>
                        </Popover>
                      )
                    },
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '140px'
                  },
                  {
                    header: '自定义连接',
                    cell: (file) => {
                      const code = file.custom_short_code || ''
                      return (
                        <Popover
                          open={customLinkFileId === file.id}
                          onOpenChange={(open) => {
                            if (open) {
                              setCustomLinkFileId(file.id)
                              setCustomLinkInput(code)
                              setUserShortCodes({})
                            } else {
                              setCustomLinkFileId(null)
                            }
                          }}
                        >
                          <PopoverTrigger asChild>
                            <Button variant="ghost" size="sm" className="h-8 text-xs font-mono max-w-[80px] truncate px-2">
                              {code ? (
                                <Tooltip>
                                  <TooltipTrigger asChild>
                                    <span>{code.length > 6 ? code.slice(0, 6) + '…' : code}</span>
                                  </TooltipTrigger>
                                  {code.length > 6 && <TooltipContent>{code}</TooltipContent>}
                                </Tooltip>
                              ) : (
                                <span className="text-muted-foreground">-</span>
                              )}
                            </Button>
                          </PopoverTrigger>
                          <PopoverContent className="w-[280px] p-3" align="start">
                            <div className="space-y-2">
                              <p className="text-xs text-muted-foreground">订阅短链接（仅字母数字）</p>
                              <Input
                                value={customLinkInput}
                                onChange={(e) => setCustomLinkInput(e.target.value)}
                                placeholder="例: mylink"
                                className="h-8 text-xs font-mono"
                              />
                              <div className="flex gap-1">
                                <Button
                                  size="sm"
                                  className="h-7 text-xs flex-1"
                                  disabled={updateMetadataMutation.isPending}
                                  onClick={() => {
                                    updateMetadataMutation.mutate({
                                      id: file.id,
                                      data: {
                                        name: file.name,
                                        description: file.description,
                                        auto_sync_custom_rules: file.auto_sync_custom_rules,
                                        custom_short_code: customLinkInput.trim() || null,
                                      }
                                    }, {
                                      onSuccess: () => {
                                        setCustomLinkFileId(null)
                                        toast.success(customLinkInput.trim() ? '自定义连接已设置' : '自定义连接已清除')
                                      }
                                    })
                                  }}
                                >
                                  保存
                                </Button>
                                {code && (
                                  <Button
                                    variant="outline"
                                    size="sm"
                                    className="h-7 text-xs"
                                    disabled={updateMetadataMutation.isPending}
                                    onClick={() => {
                                      updateMetadataMutation.mutate({
                                        id: file.id,
                                        data: {
                                          name: file.name,
                                          description: file.description,
                                          auto_sync_custom_rules: file.auto_sync_custom_rules,
                                          custom_short_code: '',
                                        }
                                      }, {
                                        onSuccess: () => {
                                          setCustomLinkFileId(null)
                                          toast.success('自定义连接已清除')
                                        }
                                      })
                                    }}
                                  >
                                    清除
                                  </Button>
                                )}
                              </div>
                            </div>
                            {subscriptionUsersQuery.data && subscriptionUsersQuery.data.length > 0 && (
                              <div className="mt-3 pt-3 border-t space-y-2">
                                <p className="text-xs text-muted-foreground">用户短链接</p>
                                {subscriptionUsersQuery.data.map((user) => {
                                  const currentValue = userShortCodes[user.username] ?? (user.custom_user_short_code || user.user_short_code)
                                  return (
                                    <div key={user.username} className="space-y-1">
                                      <Label className="text-xs font-normal text-muted-foreground">{user.username}</Label>
                                      <div className="flex gap-1">
                                        <Input
                                          value={currentValue}
                                          onChange={(e) => setUserShortCodes(prev => ({ ...prev, [user.username]: e.target.value }))}
                                          placeholder="用户短码"
                                          className="h-7 text-xs font-mono flex-1"
                                        />
                                        <Button
                                          size="sm"
                                          className="h-7 text-xs px-2"
                                          disabled={updateUserShortCodeMutation.isPending}
                                          onClick={() => {
                                            const val = (userShortCodes[user.username] ?? currentValue).trim()
                                            updateUserShortCodeMutation.mutate({
                                              username: user.username,
                                              custom_short_code: val,
                                            })
                                          }}
                                        >
                                          保存
                                        </Button>
                                      </div>
                                    </div>
                                  )
                                })}
                              </div>
                            )}
                            {subscriptionUsersQuery.isLoading && (
                              <div className="mt-3 pt-3 border-t">
                                <p className="text-xs text-muted-foreground text-center">加载中...</p>
                              </div>
                            )}
                          </PopoverContent>
                        </Popover>
                      )
                    },
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '120px'
                  },
                  // V3 模板绑定列（仅 v3 模式显示）
                  ...(isV3Mode ? [{
                    header: 'V3 模板',
                    cell: (file: SubscribeFile) => {
                      const selectedTemplate = v3Templates.find(t => t.filename === file.template_filename)
                      return (
                        <Popover>
                          <PopoverTrigger asChild>
                            <Button
                              variant="outline"
                              size="sm"
                              className="w-[140px] h-8 text-xs justify-between"
                              disabled={updateMetadataMutation.isPending}
                            >
                              <span className="truncate">
                                {selectedTemplate ? selectedTemplate.name : '选择模板'}
                              </span>
                              <ChevronDown className="h-3 w-3 shrink-0 opacity-50" />
                            </Button>
                          </PopoverTrigger>
                          <PopoverContent className="w-[200px] p-1" align="start">
                            <div className="flex flex-col">
                              <Button
                                variant="ghost"
                                size="sm"
                                className={cn(
                                  "justify-start text-xs h-8",
                                  !file.template_filename && "bg-accent"
                                )}
                                onClick={() => {
                                  updateMetadataMutation.mutate({
                                    id: file.id,
                                    data: {
                                      name: file.name,
                                      description: file.description,
                                      auto_sync_custom_rules: file.auto_sync_custom_rules,
                                      template_filename: '',
                                    }
                                  }, {
                                    onSuccess: () => {
                                      toast.success('已解除模板绑定')
                                    }
                                  })
                                }}
                              >
                                {!file.template_filename && <Check className="h-3 w-3 mr-2" />}
                                <span className={!file.template_filename ? '' : 'ml-5'}>无</span>
                              </Button>
                              {v3Templates.map((template) => (
                                <Button
                                  key={template.filename}
                                  variant="ghost"
                                  size="sm"
                                  className={cn(
                                    "justify-start text-xs h-8",
                                    file.template_filename === template.filename && "bg-accent"
                                  )}
                                  onClick={() => {
                                    updateMetadataMutation.mutate({
                                      id: file.id,
                                      data: {
                                        name: file.name,
                                        description: file.description,
                                        auto_sync_custom_rules: file.auto_sync_custom_rules,
                                        template_filename: template.filename,
                                      }
                                    }, {
                                      onSuccess: () => {
                                        toast.success(`已绑定模板: ${template.name}`)
                                      }
                                    })
                                  }}
                                >
                                  {file.template_filename === template.filename && <Check className="h-3 w-3 mr-2" />}
                                  <span className={file.template_filename === template.filename ? '' : 'ml-5'}>{template.name}</span>
                                </Button>
                              ))}
                            </div>
                          </PopoverContent>
                        </Popover>
                      )
                    },
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '160px'
                  },
                  // 节点标签选择列（仅绑定模板时显示）
                  {
                    header: '节点标签',
                    cell: (file: SubscribeFile) => {
                      if (!file.template_filename) {
                        return <span className="text-muted-foreground text-xs">-</span>
                      }
                      const selectedTags = file.selected_tags || []
                      return (
                        <Popover>
                          <PopoverTrigger asChild>
                            <Button
                              variant="outline"
                              size="sm"
                              className="w-[120px] h-8 text-xs justify-between"
                              disabled={updateMetadataMutation.isPending}
                            >
                              <span className="truncate">
                                {selectedTags.length === 1 ? selectedTags[0] : selectedTags.length > 1 ? `${selectedTags.length} 个标签` : '全部节点'}
                              </span>
                              <ChevronDown className="h-3 w-3 shrink-0 opacity-50" />
                            </Button>
                          </PopoverTrigger>
                          <PopoverContent className="w-[200px] p-1" align="start">
                            <div className="flex flex-col max-h-[300px] overflow-y-auto">
                              <Button
                                variant="ghost"
                                size="sm"
                                className={cn(
                                  "justify-start text-xs h-8",
                                  selectedTags.length === 0 && "bg-accent"
                                )}
                                onClick={() => {
                                  updateMetadataMutation.mutate({
                                    id: file.id,
                                    data: {
                                      name: file.name,
                                      description: file.description,
                                      auto_sync_custom_rules: file.auto_sync_custom_rules,
                                      template_filename: file.template_filename,
                                      selected_tags: [],
                                    }
                                  }, {
                                    onSuccess: () => {
                                      toast.success('已设置为使用全部节点')
                                    }
                                  })
                                }}
                              >
                                {selectedTags.length === 0 && <Check className="h-3 w-3 mr-2" />}
                                <span className={selectedTags.length === 0 ? '' : 'ml-5'}>全部节点</span>
                              </Button>
                              {allNodeTags.map((tag) => {
                                const isSelected = selectedTags.includes(tag)
                                return (
                                  <Button
                                    key={tag}
                                    variant="ghost"
                                    size="sm"
                                    className={cn(
                                      "justify-start text-xs h-8",
                                      isSelected && "bg-accent"
                                    )}
                                    onClick={() => {
                                      const newTags = isSelected
                                        ? selectedTags.filter(t => t !== tag)
                                        : [...selectedTags, tag]
                                      updateMetadataMutation.mutate({
                                        id: file.id,
                                        data: {
                                          name: file.name,
                                          description: file.description,
                                          auto_sync_custom_rules: file.auto_sync_custom_rules,
                                          template_filename: file.template_filename,
                                          selected_tags: newTags,
                                        }
                                      }, {
                                        onSuccess: () => {
                                          toast.success(isSelected ? `已移除标签: ${tag}` : `已添加标签: ${tag}`)
                                        }
                                      })
                                    }}
                                  >
                                    {isSelected && <Check className="h-3 w-3 mr-2" />}
                                    <span className={isSelected ? '' : 'ml-5'}>{tag}</span>
                                  </Button>
                                )
                              })}
                            </div>
                          </PopoverContent>
                        </Popover>
                      )
                    },
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '140px'
                  }] as DataTableColumn<SubscribeFile>[] : []),
                  {
                    header: '流量',
                    cell: (file) => {
                      const traffic = subscribeTrafficMap.get(file.id)
                      const isCustom = file.traffic_limit != null || !!file.stats_server_ids
                      const displayTraffic = traffic || (probeTotal && probeTotal.limit_gb > 0 ? probeTotal : null)

                      const progressBar = (() => {
                        if (!displayTraffic || displayTraffic.limit_gb === 0) {
                          return <span className='text-muted-foreground text-xs'>未配置探针</span>
                        }
                        const percentage = Math.min((displayTraffic.used_gb / displayTraffic.limit_gb) * 100, 100)
                        const remainingGB = Math.max(displayTraffic.limit_gb - displayTraffic.used_gb, 0)
                        return (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <div className='w-20 space-y-1 cursor-pointer'>
                                <Progress value={percentage} className='h-2' />
                                <div className='text-xs text-center text-muted-foreground'>
                                  {percentage.toFixed(0)}%{!isCustom && ' (默认)'}
                                </div>
                              </div>
                            </TooltipTrigger>
                            <TooltipContent className='space-y-1'>
                              <div className='text-xs'>
                                <span className='font-medium'>已用: </span>
                                {displayTraffic.used_gb.toFixed(2)} GB
                              </div>
                              <div className='text-xs'>
                                <span className='font-medium'>总量: </span>
                                {displayTraffic.limit_gb.toFixed(2)} GB
                              </div>
                              <div className='text-xs'>
                                <span className='font-medium'>剩余: </span>
                                {remainingGB.toFixed(2)} GB
                              </div>
                              {!isCustom && <div className='text-xs text-muted-foreground'>点击可设置独立流量</div>}
                            </TooltipContent>
                          </Tooltip>
                        )
                      })()

                      let trafficInputRef: HTMLInputElement | null = null

                      return (
                        <Popover>
                          <PopoverTrigger asChild>
                            <div className='cursor-pointer'>{progressBar}</div>
                          </PopoverTrigger>
                          <PopoverContent className='w-72 space-y-3' align='center'>
                            <div className='space-y-1'>
                              <Label className='text-xs'>总流量上限（GB）</Label>
                              <Input
                                type='number'
                                min='0'
                                step='0.01'
                                placeholder='留空则使用探针总流量'
                                defaultValue={file.traffic_limit != null ? String(file.traffic_limit) : ''}
                                ref={(el) => { trafficInputRef = el }}
                                className='h-8 text-sm'
                              />
                            </div>
                            <div className='space-y-1'>
                              <Label className='text-xs'>统计服务器</Label>
                              {probeServers.length > 0 ? (
                                <div className='flex flex-wrap gap-1'>
                                  {probeServers.map((srv) => {
                                    const currentIds = file.stats_server_ids ? file.stats_server_ids.split(',').map(s => s.trim()).filter(Boolean) : []
                                    const isSelected = currentIds.includes(srv.server_id)
                                    return (
                                      <Button
                                        key={srv.server_id}
                                        variant={isSelected ? 'default' : 'outline'}
                                        size='sm'
                                        className='h-6 text-xs px-2'
                                        onClick={() => {
                                          const newIds = isSelected
                                            ? currentIds.filter(id => id !== srv.server_id)
                                            : [...currentIds, srv.server_id]
                                          updateMetadataMutation.mutate({
                                            id: file.id,
                                            data: {
                                              name: file.name,
                                              description: file.description,
                                              stats_server_ids: newIds.join(','),
                                              traffic_limit: file.traffic_limit ?? null,
                                            }
                                          }, {
                                            onSuccess: () => {
                                              queryClient.invalidateQueries({ queryKey: ['subscribe-traffic'] })
                                            }
                                          })
                                        }}
                                        disabled={updateMetadataMutation.isPending}
                                      >
                                        {srv.name}
                                      </Button>
                                    )
                                  })}
                                </div>
                              ) : (
                                <p className='text-xs text-muted-foreground'>未配置探针</p>
                              )}
                            </div>
                            <Button
                              size='sm'
                              className='w-full h-7 text-xs'
                              disabled={updateMetadataMutation.isPending}
                              onClick={() => {
                                const val = trafficInputRef?.value ?? ''
                                updateMetadataMutation.mutate({
                                  id: file.id,
                                  data: {
                                    name: file.name,
                                    description: file.description,
                                    traffic_limit: val ? parseFloat(val) : null,
                                    stats_server_ids: file.stats_server_ids,
                                  }
                                }, {
                                  onSuccess: () => {
                                    queryClient.invalidateQueries({ queryKey: ['subscribe-traffic'] })
                                  }
                                })
                              }}
                            >
                              {updateMetadataMutation.isPending ? '保存中...' : '保存流量设置'}
                            </Button>
                          </PopoverContent>
                        </Popover>
                      )
                    },
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '110px',
                  },
                  {
                    header: '',
                    cell: (file) => file.updated_at ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <Clock className='h-4 w-4 text-muted-foreground cursor-help' />
                        </TooltipTrigger>
                        <TooltipContent>
                          {dateFormatter.format(new Date(file.updated_at))}
                        </TooltipContent>
                      </Tooltip>
                    ) : null,
                    width: '36px'
                  },
                  {
                    header: '操作',
                    cell: (file) => {
                      const idx = files.findIndex(f => f.id === file.id)
                      return (
                      <div className='flex items-center gap-1'>
                        <Button
                          variant='ghost'
                          size='icon'
                          className='h-7 w-7'
                          onClick={() => handleMoveUp(idx)}
                          disabled={idx === 0 || reorderMutation.isPending}
                          title='上移'
                        >
                          <ChevronUp className='h-4 w-4' />
                        </Button>
                        <Button
                          variant='ghost'
                          size='icon'
                          className='h-7 w-7'
                          onClick={() => handleMoveDown(idx)}
                          disabled={idx >= files.length - 1 || reorderMutation.isPending}
                          title='下移'
                        >
                          <ChevronDown className='h-4 w-4' />
                        </Button>
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => handleEditMetadata(file)}
                          disabled={updateMetadataMutation.isPending}
                        >
                          <Settings className='h-4 w-4' />
                        </Button>
                        <Button
                          variant='ghost'
                          size='sm'
                          onClick={() => handleEditConfig(file)}
                        >
                          <Edit className='h-4 w-4' />
                        </Button>
                        <AlertDialog>
                          <AlertDialogTrigger asChild>
                            <Button
                              variant='ghost'
                              size='sm'
                              className='text-destructive hover:text-destructive'
                              disabled={deleteMutation.isPending}
                            >
                              <Trash2 className='h-4 w-4' />
                            </Button>
                          </AlertDialogTrigger>
                          <AlertDialogContent>
                            <AlertDialogHeader>
                              <AlertDialogTitle>确认删除</AlertDialogTitle>
                              <AlertDialogDescription>
                                确定要删除订阅 "{file.name}" 吗？此操作将同时删除对应的文件，不可撤销。
                              </AlertDialogDescription>
                            </AlertDialogHeader>
                            <AlertDialogFooter>
                              <AlertDialogCancel>取消</AlertDialogCancel>
                              <AlertDialogAction onClick={() => handleDelete(file.id)}>
                                删除
                              </AlertDialogAction>
                            </AlertDialogFooter>
                          </AlertDialogContent>
                        </AlertDialog>
                      </div>
                    )},
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '180px'
                  }
                ] as DataTableColumn<SubscribeFile>[]}

                mobileCard={{
                  header: (file) => (
                    <div className='flex items-center justify-between gap-2 mb-1'>
                      <div className='flex items-center gap-2 flex-1 min-w-0'>
                        <Badge variant='outline' className={TYPE_COLORS[file.type]}>
                          {TYPE_LABELS[file.type]}
                        </Badge>
                        {file.raw_output && (
                          <Badge variant='outline' className='bg-orange-500/10 text-orange-700 dark:text-orange-400'>
                            原始输出
                          </Badge>
                        )}
                        <div className='font-medium text-sm truncate'>{file.name}</div>
                      </div>
                      <AlertDialog>
                        <AlertDialogTrigger asChild>
                          <Button
                            variant='outline'
                            size='icon'
                            className='size-8 shrink-0 text-destructive hover:text-destructive hover:bg-destructive/10'
                            disabled={deleteMutation.isPending}
                            onClick={(e) => e.stopPropagation()}
                          >
                            <Trash2 className='size-4' />
                          </Button>
                        </AlertDialogTrigger>
                        <AlertDialogContent>
                          <AlertDialogHeader>
                            <AlertDialogTitle>确认删除</AlertDialogTitle>
                            <AlertDialogDescription>
                              确定要删除订阅 "{file.name}" 吗？此操作将同时删除对应的文件，不可撤销。
                            </AlertDialogDescription>
                          </AlertDialogHeader>
                          <AlertDialogFooter>
                            <AlertDialogCancel>取消</AlertDialogCancel>
                            <AlertDialogAction onClick={() => handleDelete(file.id)}>
                              删除
                            </AlertDialogAction>
                          </AlertDialogFooter>
                        </AlertDialogContent>
                      </AlertDialog>
                    </div>
                  ),
                  fields: [
                    {
                      label: '描述',
                      value: (file) => <span className='text-xs line-clamp-1'>{file.description}</span>,
                      hidden: (file) => !file.description
                    },
                    {
                      label: '文件',
                      value: (file) => <span className='font-mono break-all'>{file.filename}</span>
                    },
                    {
                      label: '更新时间',
                      value: (file) => (
                        <div className='flex items-center gap-2 flex-wrap'>
                          <span>{file.updated_at ? dateFormatter.format(new Date(file.updated_at)) : '-'}</span>
                          {file.latest_version && (
                            <>
                              <span className='text-muted-foreground'>·</span>
                              <Badge variant='secondary' className='text-xs'>v{file.latest_version}</Badge>
                            </>
                          )}
                        </div>
                      )
                    },
                    {
                      label: '覆写配置',
                      value: (file) => {
                        const ruleIds = file.selected_custom_rule_ids || []
                        const scriptIds = file.selected_override_script_ids || []
                        const totalSelected = ruleIds.length + scriptIds.length
                        const totalAvailable = enabledCustomRules.length + enabledOverrideScripts.length
                        const isEnabled = file.auto_sync_custom_rules
                        return (
                          <Popover>
                            <PopoverTrigger asChild>
                              <Button variant="outline" size="sm" className="h-8 text-xs justify-between w-[120px]">
                                <span className="truncate">
                                  {!isEnabled ? '未启用' : totalSelected === 0 ? `全部(${totalAvailable})` : `${totalSelected} 项`}
                                </span>
                                <ChevronDown className="h-3 w-3 shrink-0 opacity-50" />
                              </Button>
                            </PopoverTrigger>
                            <PopoverContent className="w-[240px] p-1" align="start">
                              <div className="flex flex-col max-h-[400px] overflow-y-auto">
                                <Button
                                  variant="ghost" size="sm"
                                  className={cn("justify-start text-xs h-8", !isEnabled && "bg-accent")}
                                  onClick={() => {
                                    updateMetadataMutation.mutate({
                                      id: file.id,
                                      data: { name: file.name, description: file.description, auto_sync_custom_rules: false, selected_custom_rule_ids: [], selected_override_script_ids: [] }
                                    }, { onSuccess: () => toast.success('已关闭覆写') })
                                  }}
                                >
                                  {!isEnabled && <Check className="h-3 w-3 mr-2" />}
                                  <span className={!isEnabled ? '' : 'ml-5'}>不启用</span>
                                </Button>
                                <Button
                                  variant="ghost" size="sm"
                                  className={cn("justify-start text-xs h-8", isEnabled && totalSelected === 0 && "bg-accent")}
                                  onClick={() => {
                                    updateMetadataMutation.mutate({
                                      id: file.id,
                                      data: { name: file.name, description: file.description, auto_sync_custom_rules: true, selected_custom_rule_ids: [], selected_override_script_ids: [] }
                                    }, { onSuccess: () => toast.success('已启用全部覆写') })
                                  }}
                                >
                                  {isEnabled && totalSelected === 0 && <Check className="h-3 w-3 mr-2" />}
                                  <span className={isEnabled && totalSelected === 0 ? '' : 'ml-5'}>全部启用</span>
                                </Button>
                                {enabledCustomRules.length > 0 && (
                                  <>
                                    <div className="px-2 py-1.5 text-xs text-muted-foreground font-medium border-t mt-1 pt-1.5">自定义规则</div>
                                    {enabledCustomRules.map((rule) => {
                                      const isSelected = isEnabled && (totalSelected === 0 || ruleIds.includes(rule.id))
                                      return (
                                        <Button key={`rule-${rule.id}`} variant="ghost" size="sm" className={cn("justify-start text-xs h-8", isSelected && "bg-accent")}
                                          onClick={() => {
                                            if (!isEnabled) return
                                            let newRuleIds: number[]
                                            let newScriptIds: number[]
                                            if (totalSelected === 0) {
                                              newRuleIds = enabledCustomRules.filter(r => r.id !== rule.id).map(r => r.id)
                                              newScriptIds = enabledOverrideScripts.map(s => s.id)
                                            } else {
                                              newRuleIds = ruleIds.includes(rule.id) ? ruleIds.filter(id => id !== rule.id) : [...ruleIds, rule.id]
                                              newScriptIds = scriptIds
                                            }
                                            if (newRuleIds.length === enabledCustomRules.length && newScriptIds.length === enabledOverrideScripts.length) { newRuleIds = []; newScriptIds = [] }
                                            const allEmpty = newRuleIds.length === 0 && newScriptIds.length === 0 && totalSelected > 0
                                            updateMetadataMutation.mutate({ id: file.id, data: { name: file.name, description: file.description, auto_sync_custom_rules: !allEmpty, selected_custom_rule_ids: newRuleIds, selected_override_script_ids: newScriptIds } })
                                          }}
                                        >
                                          {isSelected && <Check className="h-3 w-3 mr-2" />}
                                          <span className={isSelected ? '' : 'ml-5'}>{rule.name}</span>
                                          <Badge variant="outline" className="ml-auto text-[10px] px-1 py-0">{rule.type}</Badge>
                                        </Button>
                                      )
                                    })}
                                  </>
                                )}
                                {enabledOverrideScripts.length > 0 && (
                                  <>
                                    <div className="px-2 py-1.5 text-xs text-muted-foreground font-medium border-t mt-1 pt-1.5">覆写脚本</div>
                                    {enabledOverrideScripts.map((script) => {
                                      const isSelected = isEnabled && (totalSelected === 0 || scriptIds.includes(script.id))
                                      return (
                                        <Button key={`script-${script.id}`} variant="ghost" size="sm" className={cn("justify-start text-xs h-8", isSelected && "bg-accent")}
                                          onClick={() => {
                                            if (!isEnabled) return
                                            let newScriptIds: number[]
                                            let newRuleIds: number[]
                                            if (totalSelected === 0) {
                                              newScriptIds = enabledOverrideScripts.filter(s => s.id !== script.id).map(s => s.id)
                                              newRuleIds = enabledCustomRules.map(r => r.id)
                                            } else {
                                              newScriptIds = scriptIds.includes(script.id) ? scriptIds.filter(id => id !== script.id) : [...scriptIds, script.id]
                                              newRuleIds = ruleIds
                                            }
                                            if (newRuleIds.length === enabledCustomRules.length && newScriptIds.length === enabledOverrideScripts.length) { newRuleIds = []; newScriptIds = [] }
                                            const allEmpty = newRuleIds.length === 0 && newScriptIds.length === 0 && totalSelected > 0
                                            updateMetadataMutation.mutate({ id: file.id, data: { name: file.name, description: file.description, auto_sync_custom_rules: !allEmpty, selected_custom_rule_ids: newRuleIds, selected_override_script_ids: newScriptIds } })
                                          }}
                                        >
                                          {isSelected && <Check className="h-3 w-3 mr-2" />}
                                          <span className={isSelected ? '' : 'ml-5'}>{script.name}</span>
                                          <Badge variant="outline" className="ml-auto text-[10px] px-1 py-0">{script.hook === 'post_fetch' ? '获取后' : '保存前'}</Badge>
                                        </Button>
                                      )
                                    })}
                                  </>
                                )}
                              </div>
                            </PopoverContent>
                          </Popover>
                        )
                      }
                    },
                    {
                      label: '过期时间',
                      value: (file) => {
                        const handleQuickExpire = (days: number | 'expired' | Date) => {
                          let newExpireAt: string | null = null

                          if (days === 'expired') {
                            newExpireAt = new Date().toISOString()
                          } else if (days instanceof Date) {
                            newExpireAt = days.toISOString()
                          } else {
                            const baseDate = file.expire_at ? new Date(file.expire_at) : new Date()
                            newExpireAt = addDays(baseDate, days).toISOString()
                          }

                          updateMetadataMutation.mutate({
                            id: file.id,
                            data: {
                              name: file.name,
                              description: file.description,
                              auto_sync_custom_rules: file.auto_sync_custom_rules,
                              expire_at: newExpireAt,
                            }
                          }, {
                            onSuccess: () => {
                              setMobileExpirePopoverFileId(null)
                              setMobileCustomDateFileId(null)
                              toast.success('过期时间已更新')
                            }
                          })
                        }

                        const getExpirationStatus = () => {
                          if (!file.expire_at) return null
                          const expireDate = new Date(file.expire_at)
                          if (isToday(expireDate)) {
                            return { status: 'expiring', label: '今天过期', className: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400' }
                          }
                          if (isPast(expireDate)) {
                            return { status: 'expired', label: '已过期', className: 'bg-red-500/10 text-red-700 dark:text-red-400' }
                          }
                          const daysRemaining = differenceInCalendarDays(expireDate, new Date())
                          if (daysRemaining <= 7) {
                            return { status: 'expiring', label: `${daysRemaining}天后过期`, className: 'bg-yellow-500/10 text-yellow-700 dark:text-yellow-400' }
                          }
                          return { status: 'valid', label: format(expireDate, 'yyyy-MM-dd HH:mm'), className: 'bg-green-500/10 text-green-700 dark:text-green-400' }
                        }

                        const expirationStatus = getExpirationStatus()

                        return (
                          <Popover
                            open={mobileExpirePopoverFileId === file.id}
                            onOpenChange={(open) => setMobileExpirePopoverFileId(open ? file.id : null)}
                          >
                            <PopoverTrigger asChild>
                              <Button
                                variant='ghost'
                                size='sm'
                                className='h-auto px-0 py-0'
                              >
                                {expirationStatus ? (
                                  <Badge variant='outline' className={expirationStatus.className}>
                                    {expirationStatus.label}
                                  </Badge>
                                ) : (
                                  <span className='text-xs text-muted-foreground'>未设置</span>
                                )}
                              </Button>
                            </PopoverTrigger>
                            <PopoverContent className='w-auto p-2' align='start'>
                              <div className='flex flex-col gap-2'>
                                <Button
                                  variant='outline'
                                  size='sm'
                                  onClick={() => handleQuickExpire(30)}
                                  disabled={updateMetadataMutation.isPending}
                                >
                                  延长30天
                                </Button>
                                <Button
                                  variant='outline'
                                  size='sm'
                                  onClick={() => handleQuickExpire('expired')}
                                  disabled={updateMetadataMutation.isPending}
                                >
                                  标记过期
                                </Button>
                                <Popover
                                  open={mobileCustomDateFileId === file.id}
                                  onOpenChange={(open) => setMobileCustomDateFileId(open ? file.id : null)}
                                >
                                  <PopoverTrigger asChild>
                                    <Button
                                      variant='outline'
                                      size='sm'
                                    >
                                      <CalendarIcon className='mr-2 h-4 w-4' />
                                      选择时间
                                    </Button>
                                  </PopoverTrigger>
                                  <PopoverContent className='w-auto p-0' align='start'>
                                    <Calendar
                                      mode='single'
                                      selected={file.expire_at ? new Date(file.expire_at) : undefined}
                                      onSelect={(date) => {
                                        if (date) {
                                          handleQuickExpire(date)
                                        }
                                      }}
                                      disabled={(date) => date < new Date()}
                                      initialFocus
                                    />
                                  </PopoverContent>
                                </Popover>
                                {file.expire_at && (
                                  <Button
                                    variant='outline'
                                    size='sm'
                                    onClick={() => {
                                      updateMetadataMutation.mutate({
                                        id: file.id,
                                        data: {
                                          name: file.name,
                                          description: file.description,
                                          auto_sync_custom_rules: file.auto_sync_custom_rules,
                                          expire_at: null,
                                        }
                                      }, {
                                        onSuccess: () => {
                                          setMobileExpirePopoverFileId(null)
                                          toast.success('已清除过期时间')
                                        }
                                      })
                                    }}
                                    disabled={updateMetadataMutation.isPending}
                                    className='text-destructive hover:text-destructive'
                                  >
                                    清除过期时间
                                  </Button>
                                )}
                              </div>
                            </PopoverContent>
                          </Popover>
                        )
                      }
                    }
                  ],
                  actions: (file) => (
                    <>
                      <Button
                        variant='outline'
                        size='sm'
                        className='flex-1'
                        onClick={() => handleEditMetadata(file)}
                        disabled={updateMetadataMutation.isPending}
                      >
                        <Settings className='mr-1 h-4 w-4' />
                        编辑信息
                      </Button>
                      <Button
                        variant='outline'
                        size='sm'
                        className='flex-1'
                        onClick={() => handleEditConfig(file)}
                      >
                        <Edit className='mr-1 h-4 w-4' />
                        编辑配置
                      </Button>
                    </>
                  )
                }}
              />
            )}
          </CardContent>
        </Card>

        {/* 外部订阅卡片 - 默认折叠 */}
        <Collapsible open={isExternalSubsExpanded} onOpenChange={setIsExternalSubsExpanded}>
          <Card>
            <CollapsibleTrigger asChild>
              <CardHeader className='cursor-pointer'>
                <div className='flex items-center justify-between'>
                  <div>
                    <CardTitle className='flex items-center gap-2'>
                      <ExternalLink className='h-5 w-5' />
                      外部订阅 ({externalSubs.length})
                    </CardTitle>
                    <CardDescription>管理从节点管理导入的外部订阅源，用于从第三方订阅同步节点</CardDescription>
                  </div>
                  {isExternalSubsExpanded ? <ChevronUp className='h-5 w-5' /> : <ChevronDown className='h-5 w-5' />}
                </div>
              </CardHeader>
            </CollapsibleTrigger>
            <CollapsibleContent className='CollapsibleContent'>
              <CardContent>
              {/* 操作按钮 */}
              <div className='flex justify-end mb-4 gap-2'>
                <Tooltip>
                  <TooltipTrigger asChild>
                    <Button
                      variant='outline'
                      size='sm'
                      onClick={() => navigate({ to: '/nodes', search: { action: 'import-subscription' } })}
                    >
                      <Plus className='h-4 w-4' />
                    </Button>
                  </TooltipTrigger>
                  <TooltipContent>添加外部订阅</TooltipContent>
                </Tooltip>
                <Button
                  variant='outline'
                  size='sm'
                  onClick={() => syncExternalSubsMutation.mutate()}
                  disabled={syncExternalSubsMutation.isPending || externalSubs.length === 0}
                >
                  <RefreshCw className={`mr-2 h-4 w-4 ${syncExternalSubsMutation.isPending ? 'animate-spin' : ''}`} />
                  {syncExternalSubsMutation.isPending ? '同步中...' : '同步所有订阅'}
                </Button>
              </div>

              {isExternalSubsLoading ? (
                <div className='text-center py-8 text-muted-foreground'>加载中...</div>
              ) : externalSubs.length === 0 ? (
                <div className='text-center py-8 text-muted-foreground'>
                  暂无外部订阅，请在"生成订阅"页面添加
                </div>
              ) : (
                <DataTable
                  data={externalSubs}
                  getRowKey={(sub) => sub.id}
                  emptyText='暂无外部订阅'

                  columns={[
                    {
                      header: '名称',
                      cell: (sub) => (
                        <div className='flex flex-col gap-1'>
                          <span>{sub.name}</span>
                          {sub.auto_update && (
                            <Badge variant='secondary' className='w-fit text-[10px]'>
                              定时 {sub.update_interval_minutes >= 60
                                ? `${Math.round(sub.update_interval_minutes / 60)}h`
                                : `${sub.update_interval_minutes}m`}
                            </Badge>
                          )}
                        </div>
                      ),
                      cellClassName: 'font-medium'
                    },
                    {
                      header: '订阅链接',
                      cell: (sub) => (
                        <Tooltip>
                          <TooltipTrigger asChild>
                            <div className='max-w-[200px] truncate text-sm text-muted-foreground font-mono cursor-help'>
                              {sub.url}
                            </div>
                          </TooltipTrigger>
                          <TooltipContent className='max-w-md break-all font-mono text-xs'>
                            {sub.url}
                          </TooltipContent>
                        </Tooltip>
                      )
                    },
                    {
                      header: '节点数',
                      cell: (sub) => {
                        const nodes = nodesByTag[sub.name] ?? []
                        // 优先使用实际查询到的节点数量，如果还没加载则使用数据库存储的数量
                        const nodeCount = allNodesData ? nodes.length : sub.node_count
                        return (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Badge variant='secondary' className='cursor-help'>
                                {nodeCount}
                              </Badge>
                            </TooltipTrigger>
                            <TooltipContent className='max-w-64 max-h-60 overflow-y-auto p-2'>
                              <div className='text-xs font-medium mb-1'>{sub.name} 的节点</div>
                              {nodes.length > 0 ? (
                                <ul className='space-y-0.5'>
                                  {nodes.map((nodeName, idx) => (
                                    <li key={idx} className='text-xs truncate'>
                                      <Twemoji>{nodeName}</Twemoji>
                                    </li>
                                  ))}
                                </ul>
                              ) : (
                                <div className='text-xs'>暂无节点</div>
                              )}
                            </TooltipContent>
                          </Tooltip>
                        )
                      },
                      headerClassName: 'text-center',
                      cellClassName: 'text-center'
                    },
                    {
                      header: '流量使用',
                      cell: (sub) => {
                        if (sub.total <= 0) {
                          return <span className='text-sm text-muted-foreground'>-</span>
                        }
                        // 根据 traffic_mode 计算已用流量
                        const mode = sub.traffic_mode || 'both'
                        const modeLabel = mode === 'download' ? '仅下行' : mode === 'upload' ? '仅上行' : mode === 'none' ? '不统计' : '上下行'

                        // 不统计模式：显示提示而不是进度条
                        if (mode === 'none') {
                          return (
                            <div className='flex items-center gap-1'>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <div className='w-20 space-y-1 cursor-help'>
                                    <div className='text-xs text-center text-muted-foreground'>
                                      不统计
                                    </div>
                                  </div>
                                </TooltipTrigger>
                                <TooltipContent className='space-y-1'>
                                  <div className='text-xs text-muted-foreground'>
                                    统计方式: {modeLabel}
                                  </div>
                                  <div className='text-xs text-muted-foreground'>
                                    此订阅的流量不计入总流量
                                  </div>
                                </TooltipContent>
                              </Tooltip>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='h-6 w-6'
                                    onClick={() => {
                                      // 循环切换: both -> download -> upload -> none -> both
                                      const nextMode = mode === 'both' ? 'download' : mode === 'download' ? 'upload' : mode === 'upload' ? 'none' : 'both'
                                      updateExternalSubMutation.mutate({
                                        id: sub.id,
                                        name: sub.name,
                                        url: sub.url,
                                        user_agent: sub.user_agent,
                                        traffic_mode: nextMode
                                      })
                                    }}
                                    disabled={updateExternalSubMutation.isPending}
                                  >
                                    <Ban className='h-3 w-3' />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <span>切换统计方式: {modeLabel}</span>
                                </TooltipContent>
                              </Tooltip>
                            </div>
                          )
                        }

                        const used = mode === 'download' ? sub.download : mode === 'upload' ? sub.upload : sub.upload + sub.download
                        const percentage = Math.min((used / sub.total) * 100, 100)
                        const remaining = Math.max(sub.total - used, 0)
                        return (
                          <div className='flex items-center gap-1'>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <div className='w-20 space-y-1 cursor-help'>
                                  <Progress value={percentage} className='h-2' />
                                  <div className='text-xs text-center text-muted-foreground'>
                                    {percentage.toFixed(0)}%
                                  </div>
                                </div>
                              </TooltipTrigger>
                              <TooltipContent className='space-y-1'>
                                <div className='text-xs'>
                                  <span className='font-medium'>上传: </span>
                                  {formatTrafficGB(sub.upload)}
                                </div>
                                <div className='text-xs'>
                                  <span className='font-medium'>下载: </span>
                                  {formatTrafficGB(sub.download)}
                                </div>
                                <div className='text-xs'>
                                  <span className='font-medium'>总量: </span>
                                  {formatTrafficGB(sub.total)}
                                </div>
                                <div className='text-xs'>
                                  <span className='font-medium'>剩余: </span>
                                  {formatTrafficGB(remaining)}
                                </div>
                                <div className='text-xs text-muted-foreground'>
                                  统计方式: {modeLabel}
                                </div>
                              </TooltipContent>
                            </Tooltip>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  variant='ghost'
                                  size='icon'
                                  className='h-6 w-6'
                                  onClick={() => {
                                    // 循环切换: both -> download -> upload -> none -> both
                                    const nextMode = mode === 'both' ? 'download' : mode === 'download' ? 'upload' : mode === 'upload' ? 'none' : 'both'
                                    updateExternalSubMutation.mutate({
                                      id: sub.id,
                                      name: sub.name,
                                      url: sub.url,
                                      user_agent: sub.user_agent,
                                      traffic_mode: nextMode
                                    })
                                  }}
                                  disabled={updateExternalSubMutation.isPending}
                                >
                                  {mode === 'download' ? (
                                    <Download className='h-3 w-3' />
                                  ) : mode === 'upload' ? (
                                    <Upload className='h-3 w-3' />
                                  ) : (
                                    <svg className='h-3 w-3' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2' strokeLinecap='round' strokeLinejoin='round'>
                                      <path d='M12 5v14M5 12l7-7 7 7M5 12l7 7 7-7' />
                                    </svg>
                                  )}
                                </Button>
                              </TooltipTrigger>
                              <TooltipContent>
                                <span>切换统计方式: {modeLabel}</span>
                              </TooltipContent>
                            </Tooltip>
                          </div>
                        )
                      },
                      width: '140px'
                    },
                    {
                      header: '到期时间',
                      cell: (sub) => sub.expire ? (
                        <span className='text-sm'>
                          {dateFormatter.format(new Date(sub.expire))}
                        </span>
                      ) : (
                        <span className='text-sm text-muted-foreground'>-</span>
                      )
                    },
                    {
                      header: '最后同步',
                      cell: (sub) => (
                        <span className='text-sm text-muted-foreground'>
                          {sub.last_sync_at ? dateFormatter.format(new Date(sub.last_sync_at)) : '-'}
                        </span>
                      )
                    },
                    {
                      header: '操作',
                      cell: (sub) => (
                        <div className='flex items-center gap-1'>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() => {
                              setEditingExternalSub(sub)
                              setEditExternalSubForm({
                                name: sub.name,
                                url: sub.url,
                                user_agent: sub.user_agent,
                                traffic_mode: sub.traffic_mode || 'both',
                                auto_update: Boolean(sub.auto_update),
                                update_interval_minutes: sub.update_interval_minutes > 0 ? sub.update_interval_minutes : 60,
                              })
                              setEditExternalSubDialogOpen(true)
                            }}
                          >
                            <Edit className='h-4 w-4' />
                          </Button>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() => syncSingleExternalSubMutation.mutate(sub.id)}
                            disabled={syncingSingleId === sub.id || syncExternalSubsMutation.isPending}
                          >
                            <RefreshCw className={`h-4 w-4 ${syncingSingleId === sub.id ? 'animate-spin' : ''}`} />
                          </Button>
                          <AlertDialog>
                            <AlertDialogTrigger asChild>
                              <Button variant='ghost' size='sm' className='text-destructive hover:text-destructive' disabled={deleteExternalSubMutation.isPending}>
                                <Trash2 className='h-4 w-4' />
                              </Button>
                            </AlertDialogTrigger>
                            <AlertDialogContent>
                              <AlertDialogHeader>
                                <AlertDialogTitle>确认删除</AlertDialogTitle>
                                <AlertDialogDescription>
                                  确定要删除外部订阅 "{sub.name}" 吗？此操作不会删除已同步的节点，但会停止后续同步。
                                </AlertDialogDescription>
                              </AlertDialogHeader>
                              <AlertDialogFooter>
                                <AlertDialogCancel>取消</AlertDialogCancel>
                                <AlertDialogAction onClick={() => deleteExternalSubMutation.mutate(sub.id)}>
                                  删除
                                </AlertDialogAction>
                              </AlertDialogFooter>
                            </AlertDialogContent>
                          </AlertDialog>
                        </div>
                      ),
                      headerClassName: 'text-center',
                      cellClassName: 'text-center',
                      width: '130px'
                    }
                  ] as DataTableColumn<ExternalSubscription>[]}

                  mobileCard={{
                    header: (sub) => {
                      const nodes = nodesByTag[sub.name] ?? []
                      // 优先使用实际查询到的节点数量，如果还没加载则使用数据库存储的数量
                      const nodeCount = allNodesData ? nodes.length : sub.node_count
                      return (
                      <div className='flex items-center justify-between gap-2 mb-1'>
                        <div className='flex items-center gap-2 flex-1 min-w-0'>
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Badge variant='secondary' className='cursor-help'>
                                {nodeCount} 节点
                              </Badge>
                            </TooltipTrigger>
                            <TooltipContent className='max-w-64 max-h-60 overflow-y-auto p-2'>
                              <div className='text-xs font-medium mb-1'>{sub.name} 的节点</div>
                              {nodes.length > 0 ? (
                                <ul className='space-y-0.5'>
                                  {nodes.map((nodeName, idx) => (
                                    <li key={idx} className='text-xs truncate'>
                                      <Twemoji>{nodeName}</Twemoji>
                                    </li>
                                  ))}
                                </ul>
                              ) : (
                                <div className='text-xs'>暂无节点</div>
                              )}
                            </TooltipContent>
                          </Tooltip>
                          <div className='font-medium text-sm truncate'>{sub.name}</div>
                          {sub.auto_update && (
                            <div className='text-[10px] text-muted-foreground'>
                              定时更新 · {sub.update_interval_minutes >= 60
                                ? `${Math.round(sub.update_interval_minutes / 60)} 小时`
                                : `${sub.update_interval_minutes} 分钟`}
                            </div>
                          )}
                        </div>
                        <div className='flex items-center gap-1'>
                          <Button
                            variant='outline'
                            size='icon'
                            className='size-8 shrink-0'
                            onClick={(e) => {
                              e.stopPropagation()
                              setEditingExternalSub(sub)
                              setEditExternalSubForm({
                                name: sub.name,
                                url: sub.url,
                                user_agent: sub.user_agent,
                                traffic_mode: sub.traffic_mode || 'both',
                                auto_update: Boolean(sub.auto_update),
                                update_interval_minutes: sub.update_interval_minutes > 0 ? sub.update_interval_minutes : 60,
                              })
                              setEditExternalSubDialogOpen(true)
                            }}
                          >
                            <Edit className='size-4' />
                          </Button>
                          <Button
                            variant='outline'
                            size='icon'
                            className='size-8 shrink-0'
                            disabled={syncingSingleId === sub.id || syncExternalSubsMutation.isPending}
                            onClick={(e) => {
                              e.stopPropagation()
                              syncSingleExternalSubMutation.mutate(sub.id)
                            }}
                          >
                            <RefreshCw className={`size-4 ${syncingSingleId === sub.id ? 'animate-spin' : ''}`} />
                          </Button>
                          <AlertDialog>
                            <AlertDialogTrigger asChild>
                              <Button
                                variant='outline'
                                size='icon'
                                className='size-8 shrink-0 text-destructive hover:text-destructive hover:bg-destructive/10'
                                disabled={deleteExternalSubMutation.isPending}
                                onClick={(e) => e.stopPropagation()}
                              >
                                <Trash2 className='size-4' />
                              </Button>
                            </AlertDialogTrigger>
                            <AlertDialogContent>
                              <AlertDialogHeader>
                                <AlertDialogTitle>确认删除</AlertDialogTitle>
                                <AlertDialogDescription>
                                  确定要删除外部订阅 "{sub.name}" 吗？此操作不会删除已同步的节点，但会停止后续同步。
                                </AlertDialogDescription>
                              </AlertDialogHeader>
                              <AlertDialogFooter>
                                <AlertDialogCancel>取消</AlertDialogCancel>
                                <AlertDialogAction onClick={() => deleteExternalSubMutation.mutate(sub.id)}>
                                  删除
                                </AlertDialogAction>
                              </AlertDialogFooter>
                            </AlertDialogContent>
                          </AlertDialog>
                        </div>
                      </div>
                    )},
                    fields: [
                      {
                        label: '链接',
                        value: (sub) => <span className='font-mono text-xs break-all'>{sub.url}</span>
                      },
                      {
                        label: '流量',
                        value: (sub) => {
                          if (sub.total <= 0) {
                            return <span className='text-muted-foreground'>-</span>
                          }
                          // 根据 traffic_mode 计算已用流量
                          const mode = sub.traffic_mode || 'both'
                          const used = mode === 'download' ? sub.download : mode === 'upload' ? sub.upload : sub.upload + sub.download
                          const percentage = Math.min((used / sub.total) * 100, 100)
                          const remaining = Math.max(sub.total - used, 0)
                          const modeLabel = mode === 'download' ? '仅下行' : mode === 'upload' ? '仅上行' : mode === 'none' ? '不统计' : '上下行'
                          return (
                            <div className='flex items-center gap-2'>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <div className='flex items-center gap-2 cursor-help flex-1'>
                                    <Progress value={percentage} className='h-2 flex-1 max-w-20' />
                                    <span className='text-xs whitespace-nowrap'>
                                      {formatTrafficGB(used)} / {formatTrafficGB(sub.total)}
                                    </span>
                                  </div>
                                </TooltipTrigger>
                                <TooltipContent className='space-y-1'>
                                  <div className='text-xs'>
                                    <span className='font-medium'>上传: </span>
                                    {formatTrafficGB(sub.upload)}
                                  </div>
                                  <div className='text-xs'>
                                    <span className='font-medium'>下载: </span>
                                    {formatTrafficGB(sub.download)}
                                  </div>
                                  <div className='text-xs'>
                                    <span className='font-medium'>剩余: </span>
                                    {formatTrafficGB(remaining)}
                                  </div>
                                  <div className='text-xs text-muted-foreground'>
                                    统计方式: {modeLabel}
                                  </div>
                                </TooltipContent>
                              </Tooltip>
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant='ghost'
                                    size='icon'
                                    className='h-6 w-6 shrink-0'
                                    onClick={(e) => {
                                      e.stopPropagation()
                                      // 循环切换: both -> download -> upload -> none -> both
                                      const nextMode = mode === 'both' ? 'download' : mode === 'download' ? 'upload' : mode === 'upload' ? 'none' :  'both'
                                      updateExternalSubMutation.mutate({
                                        id: sub.id,
                                        name: sub.name,
                                        url: sub.url,
                                        user_agent: sub.user_agent,
                                        traffic_mode: nextMode
                                      })
                                    }}
                                    disabled={updateExternalSubMutation.isPending}
                                  >
                                    {mode === 'download' ? (
                                      <Download className='h-3 w-3' />
                                    ) : mode === 'upload' ? (
                                      <Upload className='h-3 w-3' />
                                    ) : (
                                      <svg className='h-3 w-3' viewBox='0 0 24 24' fill='none' stroke='currentColor' strokeWidth='2' strokeLinecap='round' strokeLinejoin='round'>
                                        <path d='M12 5v14M5 12l7-7 7 7M5 12l7 7 7-7' />
                                      </svg>
                                    )}
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>
                                  <span>切换统计方式: {modeLabel}</span>
                                </TooltipContent>
                              </Tooltip>
                            </div>
                          )
                        }
                      },
                      {
                        label: '到期',
                        value: (sub) => sub.expire ? dateFormatter.format(new Date(sub.expire)) : '-'
                      },
                      {
                        label: '最后同步',
                        value: (sub) => sub.last_sync_at ? dateFormatter.format(new Date(sub.last_sync_at)) : '-'
                      }
                    ]
                  }}
                />
              )}
              </CardContent>
            </CollapsibleContent>
          </Card>
        </Collapsible>

        {/* 代理集合配置 - 仅在启用时显示 */}
        {enableProxyProvider && (
        <Collapsible open={isProxyProvidersExpanded} onOpenChange={setIsProxyProvidersExpanded}>
          <Card>
            <CollapsibleTrigger asChild>
              <CardHeader className='cursor-pointer hover:bg-muted/50 transition-colors'>
                <div className='flex items-center justify-between'>
                  <div>
                    <CardTitle className='text-base'>代理集合配置</CardTitle>
                    <CardDescription>
                      管理 Clash Meta proxy-providers 配置，用于按需加载代理节点
                    </CardDescription>
                  </div>
                  <div className='flex items-center gap-2'>
                    <Badge variant='secondary'>{proxyProviderConfigs.length} 个配置</Badge>
                    {isProxyProvidersExpanded ? <ChevronUp className='h-4 w-4' /> : <ChevronDown className='h-4 w-4' />}
                  </div>
                </div>
              </CardHeader>
            </CollapsibleTrigger>
            <CollapsibleContent>
              <CardContent className='pt-0'>
                {/* 操作栏 */}
                <div className='flex flex-col sm:flex-row sm:items-center sm:justify-between gap-2 mb-4'>
                  {/* 左侧：选中状态 */}
                  <div className='flex items-center gap-2'>
                    {selectedProxyProviderIds.size > 0 && (
                      <>
                        <Badge variant='secondary'>{selectedProxyProviderIds.size} 项已选</Badge>
                        <Button
                          size='sm'
                          variant='destructive'
                          onClick={() => setBatchDeleteDialogOpen(true)}
                        >
                          <Trash2 className='h-4 w-4 mr-1' />
                          批量删除
                        </Button>
                      </>
                    )}
                  </div>
                  {/* 右侧：创建按钮 */}
                  <div className='flex flex-col sm:flex-row gap-2'>
                    <Button
                      size='sm'
                      variant='outline'
                      className='w-full sm:w-auto'
                      onClick={() => {
                        setProSelectedExternalSub(null)
                        setProCreationResults([])
                        setProxyProviderProDialogOpen(true)
                      }}
                    >
                      <Settings className='h-4 w-4 mr-2' />
                      创建代理集合(初级)
                    </Button>
                    <Button
                      size='sm'
                      className='w-full sm:w-auto'
                      onClick={() => {
                        setEditingProxyProvider(null)
                        setSelectedExternalSub(null)
                        setProxyProviderForm({
                          name: '',
                          type: 'http',
                          interval: 3600,
                          proxy: 'DIRECT',
                          size_limit: 0,
                          header_user_agent: 'Clash/v1.18.0',
                          header_authorization: '',
                          health_check_enabled: true,
                          health_check_url: 'https://www.gstatic.com/generate_204',
                          health_check_interval: 300,
                          health_check_timeout: 5000,
                          health_check_lazy: true,
                          health_check_expected_status: 204,
                          filter: '',
                          exclude_filter: '',
                          exclude_type: [],
                          override: { ...defaultOverrideForm },
                          process_mode: 'client',
                        })
                        setProxyProviderDialogOpen(true)
                      }}
                    >
                      <Settings className='h-4 w-4 mr-2' />
                      创建代理集合(高级)
                    </Button>
                  </div>
                </div>
                {/* 订阅筛选按钮 - 点击自动选中/反选该订阅下的所有代理集合 */}
                {externalSubs.length > 0 && (
                  <div className='flex flex-wrap gap-2 mb-4'>
                    <Button
                      size='sm'
                      variant={proxyProviderFilterSubId === 'all' ? 'default' : 'outline'}
                      onClick={() => {
                        setProxyProviderFilterSubId('all')
                        // 切换逻辑：如果已全选则取消，否则选中所有
                        const allIds = new Set(proxyProviderConfigs.map(c => c.id))
                        const isAllSelected = proxyProviderConfigs.length > 0 &&
                          proxyProviderConfigs.every(c => selectedProxyProviderIds.has(c.id))
                        if (isAllSelected) {
                          setSelectedProxyProviderIds(new Set())
                        } else {
                          setSelectedProxyProviderIds(allIds)
                        }
                      }}
                    >
                      全部 ({proxyProviderConfigs.length})
                    </Button>
                    {externalSubs.map(sub => {
                      const subConfigs = proxyProviderConfigs.filter(c => c.external_subscription_id === sub.id)
                      if (subConfigs.length === 0) return null
                      const subConfigIds = new Set(subConfigs.map(c => c.id))
                      // 检查是否已全选该订阅下的配置
                      const isAllSelected = subConfigs.length > 0 && subConfigs.every(c => selectedProxyProviderIds.has(c.id))
                      return (
                        <Button
                          key={sub.id}
                          size='sm'
                          variant={proxyProviderFilterSubId === sub.id ? 'default' : 'outline'}
                          onClick={() => {
                            setProxyProviderFilterSubId(sub.id)
                            if (isAllSelected) {
                              // 已全选，则取消选中
                              setSelectedProxyProviderIds(new Set())
                            } else {
                              // 未全选，则选中该订阅下的所有配置
                              setSelectedProxyProviderIds(subConfigIds)
                            }
                          }}
                        >
                          {sub.name} ({subConfigs.length})
                        </Button>
                      )
                    })}
                  </div>
                )}
                {isProxyProviderConfigsLoading ? (
                  <div className='text-center py-4 text-muted-foreground'>加载中...</div>
                ) : filteredProxyProviderConfigs.length === 0 ? (
                  <div className='text-center py-8 text-muted-foreground'>
                    <p>暂无代理集合配置</p>
                    <p className='text-sm mt-1'>点击上方按钮创建你的第一个代理集合</p>
                  </div>
                ) : (
                  <DataTable
                    data={filteredProxyProviderConfigs}
                    getRowKey={(config) => config.id}
                    columns={[
                      {
                        key: 'select',
                        header: (
                          <Checkbox
                            checked={filteredProxyProviderConfigs.length > 0 && filteredProxyProviderConfigs.every(c => selectedProxyProviderIds.has(c.id))}
                            onCheckedChange={handleSelectAllProxyProviders}
                            aria-label='全选'
                          />
                        ),
                        cell: (config) => (
                          <Checkbox
                            checked={selectedProxyProviderIds.has(config.id)}
                            onCheckedChange={(checked) => handleSelectProxyProvider(config.id, checked as boolean)}
                            aria-label={`选择 ${config.name}`}
                          />
                        ),
                        width: '40px',
                        cellClassName: 'text-center',
                        headerClassName: 'text-center'
                      },
                      {
                        key: 'name',
                        header: '名称',
                        cell: (config) => (
                          <div className='font-medium'>{config.name}</div>
                        )
                      },
                      {
                        key: 'external_subscription',
                        header: '关联订阅',
                        cell: (config) => {
                          const sub = externalSubs.find(s => s.id === config.external_subscription_id)
                          return sub ? (
                            <Badge variant='outline'>{sub.name}</Badge>
                          ) : (
                            <span className='text-muted-foreground'>未知</span>
                          )
                        }
                      },
                      {
                        key: 'process_mode',
                        header: '处理模式',
                        cell: (config) => (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <Button
                                variant='ghost'
                                size='sm'
                                className='h-auto p-0.5'
                                onClick={() => toggleProcessModeMutation.mutate(config)}
                                disabled={toggleProcessModeMutation.isPending}
                              >
                                <Badge
                                  variant={config.process_mode === 'mmw' ? 'default' : 'secondary'}
                                  className='cursor-pointer hover:opacity-80'
                                >
                                  {config.process_mode === 'mmw' ? '妙妙屋处理' : '客户端处理'}
                                </Badge>
                              </Button>
                            </TooltipTrigger>
                            <TooltipContent>
                              点击切换为{config.process_mode === 'mmw' ? '客户端处理' : '妙妙屋处理'}
                            </TooltipContent>
                          </Tooltip>
                        ),
                        headerClassName: 'text-center',
                        cellClassName: 'text-center'
                      },
                      {
                        key: 'filter',
                        header: '过滤规则',
                        cell: (config) => (
                          <div className='text-xs text-muted-foreground max-w-[150px] truncate'>
                            {config.filter || config.exclude_filter || config.exclude_type ? (
                              <span>
                                {config.filter && `保留: ${config.filter}`}
                                {config.exclude_filter && ` 排除: ${config.exclude_filter}`}
                                {config.exclude_type && ` 类型: ${config.exclude_type}`}
                              </span>
                            ) : '-'}
                          </div>
                        )
                      },
                      {
                        key: 'actions',
                        header: '操作',
                        cell: (config) => (
                          <div className='flex items-center gap-1'>
                            {config.process_mode === 'mmw' && (
                              <Tooltip>
                                <TooltipTrigger asChild>
                                  <Button
                                    variant='ghost'
                                    size='sm'
                                    onClick={() => handlePreviewProxyProvider(config)}
                                  >
                                    <Eye className='h-4 w-4' />
                                  </Button>
                                </TooltipTrigger>
                                <TooltipContent>预览处理结果</TooltipContent>
                              </Tooltip>
                            )}
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => {
                                    // 编辑配置
                                    setEditingProxyProvider(config)
                                    const sub = externalSubs.find(s => s.id === config.external_subscription_id)
                                    setSelectedExternalSub(sub || null)
                                    // 解析 header JSON
                                    let headerUserAgent = 'Clash/v1.18.0'
                                    let headerAuthorization = ''
                                    if (config.header) {
                                      try {
                                        const headerObj = JSON.parse(config.header)
                                        if (headerObj['User-Agent']) {
                                          headerUserAgent = Array.isArray(headerObj['User-Agent'])
                                            ? headerObj['User-Agent'].join(', ')
                                            : headerObj['User-Agent']
                                        }
                                        if (headerObj['Authorization']) {
                                          headerAuthorization = Array.isArray(headerObj['Authorization'])
                                            ? headerObj['Authorization'][0]
                                            : headerObj['Authorization']
                                        }
                                      } catch {}
                                    }
                                    setProxyProviderForm({
                                      name: config.name,
                                      type: config.type,
                                      interval: config.interval,
                                      proxy: config.proxy,
                                      size_limit: config.size_limit,
                                      header_user_agent: headerUserAgent,
                                      header_authorization: headerAuthorization,
                                      health_check_enabled: config.health_check_enabled,
                                      health_check_url: config.health_check_url,
                                      health_check_interval: config.health_check_interval,
                                      health_check_timeout: config.health_check_timeout,
                                      health_check_lazy: config.health_check_lazy,
                                      health_check_expected_status: config.health_check_expected_status,
                                      filter: config.filter,
                                      exclude_filter: config.exclude_filter,
                                      exclude_type: config.exclude_type ? config.exclude_type.split(',').map(s => s.trim()) : [],
                                      override: jsonToOverrideForm(config.override),
                                      process_mode: config.process_mode as 'client' | 'mmw',
                                    })
                                    setProxyProviderDialogOpen(true)
                                  }}
                                >
                                  <Edit className='h-4 w-4' />
                                </Button>
                              </TooltipTrigger>
                              <TooltipContent>编辑配置</TooltipContent>
                            </Tooltip>
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  onClick={() => {
                                    // 复制 YAML 配置
                                    const sub = externalSubs.find(s => s.id === config.external_subscription_id)
                                    if (!sub) return
                                    setSelectedExternalSub(sub)
                                    // 解析 header
                                    let headerUserAgent = ''
                                    let headerAuthorization = ''
                                    if (config.header) {
                                      try {
                                        const headerObj = JSON.parse(config.header)
                                        if (headerObj['User-Agent']) {
                                          headerUserAgent = Array.isArray(headerObj['User-Agent'])
                                            ? headerObj['User-Agent'].join(', ')
                                            : headerObj['User-Agent']
                                        }
                                        if (headerObj['Authorization']) {
                                          headerAuthorization = Array.isArray(headerObj['Authorization'])
                                            ? headerObj['Authorization'][0]
                                            : headerObj['Authorization']
                                        }
                                      } catch {}
                                    }
                                    // 生成 YAML
                                    const isClientMode = config.process_mode === 'client'
                                    const yamlConfig: Record<string, any> = {
                                      type: config.type,
                                      path: `./proxy_providers/${config.name}.yaml`,
                                      interval: config.interval,
                                    }
                                    if (isClientMode) {
                                      yamlConfig.url = sub.url
                                    } else {
                                      const baseUrl = typeof window !== 'undefined' ? window.location.origin : ''
                                      yamlConfig.url = `${baseUrl}/api/proxy-provider/${config.id}?token=${userToken}`
                                    }
                                    if (config.proxy && config.proxy !== 'DIRECT') {
                                      yamlConfig.proxy = config.proxy
                                    }
                                    if (config.size_limit > 0) {
                                      yamlConfig['size-limit'] = config.size_limit
                                    }
                                    if (headerUserAgent || headerAuthorization) {
                                      yamlConfig.header = {}
                                      if (headerUserAgent) {
                                        yamlConfig.header['User-Agent'] = headerUserAgent.split(',').map(s => s.trim())
                                      }
                                      if (headerAuthorization) {
                                        yamlConfig.header['Authorization'] = [headerAuthorization]
                                      }
                                    }
                                    if (config.health_check_enabled) {
                                      yamlConfig['health-check'] = {
                                        enable: true,
                                        url: config.health_check_url,
                                        interval: config.health_check_interval,
                                        timeout: config.health_check_timeout,
                                        lazy: config.health_check_lazy,
                                        'expected-status': config.health_check_expected_status,
                                      }
                                    }
                                    if (isClientMode) {
                                      if (config.filter) yamlConfig.filter = config.filter
                                      if (config.exclude_filter) yamlConfig['exclude-filter'] = config.exclude_filter
                                      if (config.exclude_type) yamlConfig['exclude-type'] = config.exclude_type
                                      if (config.override) {
                                        try {
                                          yamlConfig.override = JSON.parse(config.override)
                                        } catch {}
                                      }
                                    }
                                    const yamlObj: Record<string, any> = {}
                                    yamlObj[config.name] = yamlConfig
                                    const yamlStr = dumpYAML(yamlObj, { indent: 2, lineWidth: -1 })
                                    navigator.clipboard.writeText(yamlStr)
                                    toast.success('配置已复制到剪贴板')
                                  }}
                                >
                                  <Copy className='h-4 w-4' />
                                </Button>
                              </TooltipTrigger>
                              <TooltipContent>复制配置</TooltipContent>
                            </Tooltip>
                            <AlertDialog>
                              <AlertDialogTrigger asChild>
                                <Button variant='ghost' size='sm' className='text-destructive hover:text-destructive'>
                                  <Trash2 className='h-4 w-4' />
                                </Button>
                              </AlertDialogTrigger>
                              <AlertDialogContent>
                                <AlertDialogHeader>
                                  <AlertDialogTitle>确认删除</AlertDialogTitle>
                                  <AlertDialogDescription>
                                    确定要删除代理集合配置 "{config.name}" 吗？此操作无法撤销。
                                  </AlertDialogDescription>
                                </AlertDialogHeader>
                                <AlertDialogFooter>
                                  <AlertDialogCancel>取消</AlertDialogCancel>
                                  <AlertDialogAction onClick={() => deleteProxyProviderMutation.mutate(config.id)}>
                                    删除
                                  </AlertDialogAction>
                                </AlertDialogFooter>
                              </AlertDialogContent>
                            </AlertDialog>
                          </div>
                        ),
                        headerClassName: 'text-center',
                        cellClassName: 'text-center',
                        width: '120px'
                      }
                    ] as DataTableColumn<ProxyProviderConfig>[]}
                    mobileCard={{
                      header: (config) => (
                        <div className='flex items-center justify-between gap-2 mb-1'>
                          <div className='flex items-center gap-2 flex-1 min-w-0'>
                            <Checkbox
                              checked={selectedProxyProviderIds.has(config.id)}
                              onCheckedChange={(checked) => handleSelectProxyProvider(config.id, checked as boolean)}
                              onClick={(e) => e.stopPropagation()}
                              aria-label={`选择 ${config.name}`}
                              className='shrink-0'
                            />
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <Button
                                  variant='ghost'
                                  size='sm'
                                  className='h-auto p-0 shrink-0'
                                  onClick={(e) => {
                                    e.stopPropagation()
                                    toggleProcessModeMutation.mutate(config)
                                  }}
                                  disabled={toggleProcessModeMutation.isPending}
                                >
                                  <Badge
                                    variant={config.process_mode === 'mmw' ? 'default' : 'secondary'}
                                    className='cursor-pointer hover:opacity-80'
                                  >
                                    {config.process_mode === 'mmw' ? '妙妙屋' : '客户端'}
                                  </Badge>
                                </Button>
                              </TooltipTrigger>
                              <TooltipContent>
                                点击切换为{config.process_mode === 'mmw' ? '客户端处理' : '妙妙屋处理'}
                              </TooltipContent>
                            </Tooltip>
                            <div className='font-medium text-sm truncate'>{config.name}</div>
                          </div>
                          <div className='flex items-center gap-1'>
                            {config.process_mode === 'mmw' && (
                              <Button
                                variant='outline'
                                size='icon'
                                className='size-8 shrink-0'
                                onClick={(e) => {
                                  e.stopPropagation()
                                  handlePreviewProxyProvider(config)
                                }}
                              >
                                <Eye className='size-4' />
                              </Button>
                            )}
                            <Button
                              variant='outline'
                              size='icon'
                              className='size-8 shrink-0'
                              onClick={(e) => {
                                e.stopPropagation()
                                // 编辑
                                setEditingProxyProvider(config)
                                const sub = externalSubs.find(s => s.id === config.external_subscription_id)
                                setSelectedExternalSub(sub || null)
                                let headerUserAgent = 'Clash/v1.18.0'
                                let headerAuthorization = ''
                                if (config.header) {
                                  try {
                                    const headerObj = JSON.parse(config.header)
                                    if (headerObj['User-Agent']) {
                                      headerUserAgent = Array.isArray(headerObj['User-Agent'])
                                        ? headerObj['User-Agent'].join(', ')
                                        : headerObj['User-Agent']
                                    }
                                    if (headerObj['Authorization']) {
                                      headerAuthorization = Array.isArray(headerObj['Authorization'])
                                        ? headerObj['Authorization'][0]
                                        : headerObj['Authorization']
                                    }
                                  } catch {}
                                }
                                setProxyProviderForm({
                                  name: config.name,
                                  type: config.type,
                                  interval: config.interval,
                                  proxy: config.proxy,
                                  size_limit: config.size_limit,
                                  header_user_agent: headerUserAgent,
                                  header_authorization: headerAuthorization,
                                  health_check_enabled: config.health_check_enabled,
                                  health_check_url: config.health_check_url,
                                  health_check_interval: config.health_check_interval,
                                  health_check_timeout: config.health_check_timeout,
                                  health_check_lazy: config.health_check_lazy,
                                  health_check_expected_status: config.health_check_expected_status,
                                  filter: config.filter,
                                  exclude_filter: config.exclude_filter,
                                  exclude_type: config.exclude_type ? config.exclude_type.split(',').map(s => s.trim()) : [],
                                  override: jsonToOverrideForm(config.override),
                                  process_mode: config.process_mode as 'client' | 'mmw',
                                })
                                setProxyProviderDialogOpen(true)
                              }}
                            >
                              <Edit className='size-4' />
                            </Button>
                            <AlertDialog>
                              <AlertDialogTrigger asChild>
                                <Button
                                  variant='outline'
                                  size='icon'
                                  className='size-8 shrink-0 text-destructive'
                                  onClick={(e) => e.stopPropagation()}
                                >
                                  <Trash2 className='size-4' />
                                </Button>
                              </AlertDialogTrigger>
                              <AlertDialogContent>
                                <AlertDialogHeader>
                                  <AlertDialogTitle>确认删除</AlertDialogTitle>
                                  <AlertDialogDescription>
                                    确定要删除代理集合配置 "{config.name}" 吗？
                                  </AlertDialogDescription>
                                </AlertDialogHeader>
                                <AlertDialogFooter>
                                  <AlertDialogCancel>取消</AlertDialogCancel>
                                  <AlertDialogAction onClick={() => deleteProxyProviderMutation.mutate(config.id)}>
                                    删除
                                  </AlertDialogAction>
                                </AlertDialogFooter>
                              </AlertDialogContent>
                            </AlertDialog>
                          </div>
                        </div>
                      ),
                      fields: [
                        {
                          label: '关联订阅',
                          value: (config) => {
                            const sub = externalSubs.find(s => s.id === config.external_subscription_id)
                            return sub?.name || '未知'
                          }
                        },
                        {
                          label: '过滤规则',
                          value: (config) => config.filter || config.exclude_filter || config.exclude_type || '-'
                        }
                      ]
                    }}
                  />
                )}
              </CardContent>
            </CollapsibleContent>
          </Card>
        </Collapsible>
        )}
      </section>

      {/* 编辑文件 Dialog */}
      <Dialog open={editDialogOpen} onOpenChange={(open) => {
        setEditDialogOpen(open)
        if (!open) {
          // 关闭对话框时清理状态
          setEditingFile(null)
          setEditorValue('')
          setIsDirty(false)
          setValidationError(null)
        }
      }}>
        <DialogContent className='max-w-4xl h-[90vh] flex flex-col p-0'>
          <DialogHeader className='px-6 pt-6'>
            <DialogTitle>{editingFile?.name || '编辑文件'}</DialogTitle>
            <DialogDescription>
              编辑 {editingFile?.filename} 的内容，会自动验证 YAML 格式
            </DialogDescription>
          </DialogHeader>

          <div className='flex-1 flex flex-col overflow-hidden px-6'>
            <div className='flex items-center gap-3 py-4'>
              <Button
                size='sm'
                onClick={handleSave}
                disabled={!editingFile || !isDirty || saveMutation.isPending || fileContentQuery.isLoading}
              >
                {saveMutation.isPending ? '保存中...' : '保存修改'}
              </Button>
              <Button
                size='sm'
                variant='outline'
                disabled={!isDirty || fileContentQuery.isLoading || saveMutation.isPending}
                onClick={handleReset}
              >
                还原修改
              </Button>
              {fileContentQuery.data?.latest_version ? (
                <Badge variant='secondary'>版本 v{fileContentQuery.data.latest_version}</Badge>
              ) : null}
            </div>

            {validationError ? (
              <div className='rounded-md border border-destructive/60 bg-destructive/10 p-3 text-sm text-destructive mb-4'>
                {validationError}
              </div>
            ) : null}

            <div className='flex-1 rounded-lg border bg-muted/20 overflow-hidden mb-4'>
              {fileContentQuery.isLoading ? (
                <div className='p-4 text-center text-muted-foreground'>加载中...</div>
              ) : (
                <Textarea
                  value={editorValue}
                  onChange={(event) => {
                    const nextValue = event.target.value
                    setEditorValue(nextValue)
                    setIsDirty(nextValue !== (fileContentQuery.data?.content ?? ''))
                    if (validationError) {
                      setValidationError(null)
                    }
                  }}
                  className='w-full h-full font-mono text-sm resize-none border-0 bg-transparent focus-visible:ring-0 focus-visible:ring-offset-0'
                  disabled={!editingFile || saveMutation.isPending}
                  spellCheck={false}
                />
              )}
            </div>
          </div>

          <DialogFooter className='px-6 pb-6'>
            <Button variant='outline' onClick={() => setEditDialogOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑订阅信息 Dialog */}
      <Dialog open={editMetadataDialogOpen} onOpenChange={(open) => {
        setEditMetadataDialogOpen(open)
        if (!open) {
          setEditingMetadata(null)
          setMetadataForm({ name: '', description: '', filename: '', template_filename: '', selected_tags: [], expire: undefined, traffic_limit: '', stats_server_ids: '' })
        }
      }}>
        <DialogContent className='sm:max-w-lg max-h-[90vh] flex flex-col'>
          <DialogHeader>
            <DialogTitle>编辑订阅信息</DialogTitle>
            <DialogDescription>
              修改订阅名称、说明和文件名
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4 overflow-y-auto flex-1 min-h-0'>
            <div className='space-y-2'>
              <Label htmlFor='metadata-name'>订阅名称 *</Label>
              <Input
                id='metadata-name'
                value={metadataForm.name}
                onChange={(e) => setMetadataForm({ ...metadataForm, name: e.target.value })}
                placeholder='例如：机场A'
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='metadata-description'>说明（可选）</Label>
              <Textarea
                id='metadata-description'
                value={metadataForm.description}
                onChange={(e) => setMetadataForm({ ...metadataForm, description: e.target.value })}
                placeholder='订阅说明信息'
                rows={3}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='metadata-filename'>文件名 *</Label>
              <Input
                id='metadata-filename'
                value={metadataForm.filename}
                onChange={(e) => setMetadataForm({ ...metadataForm, filename: e.target.value })}
                placeholder='例如：subscription.yaml'
              />
              <p className='text-xs text-muted-foreground'>
                修改文件名后需确保该文件在 subscribes 目录中存在
              </p>
            </div>
            <div className='space-y-2'>
              <Label>过期时间（可选）</Label>
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    variant='outline'
                    className={cn(
                      'w-full justify-start text-left font-normal',
                      !metadataForm.expire && 'text-muted-foreground'
                    )}
                  >
                    <CalendarIcon className='mr-2 h-4 w-4' />
                    {metadataForm.expire ? format(metadataForm.expire, 'PPP') : <span>无过期时间</span>}
                  </Button>
                </PopoverTrigger>
                <PopoverContent className='w-auto p-0' align='start'>
                  <Calendar
                    mode='single'
                    selected={metadataForm.expire}
                    onSelect={(date) => setMetadataForm({ ...metadataForm, expire: date })}
                    initialFocus
                  />
                </PopoverContent>
              </Popover>
              <p className='text-xs text-muted-foreground'>
                设置订阅链接的过期时间，过期后链接将失效
              </p>
            </div>
            <div className='space-y-2'>
              <Label>绑定 V3 模板（可选）</Label>
              <Popover>
                <PopoverTrigger asChild>
                  <Button
                    variant="outline"
                    className="w-full justify-between"
                  >
                    <span className="truncate">
                      {metadataForm.template_filename
                        ? v3Templates.find(t => t.filename === metadataForm.template_filename)?.name || metadataForm.template_filename
                        : '不绑定模板'}
                    </span>
                    <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
                  </Button>
                </PopoverTrigger>
                <PopoverContent className="w-[--radix-popover-trigger-width] p-1" align="start">
                  <div className="flex flex-col max-h-[300px] overflow-y-auto">
                    <Button
                      variant="ghost"
                      size="sm"
                      className={cn(
                        "justify-start h-9",
                        !metadataForm.template_filename && "bg-accent"
                      )}
                      onClick={() => setMetadataForm({ ...metadataForm, template_filename: '' })}
                    >
                      {!metadataForm.template_filename && <Check className="h-4 w-4 mr-2" />}
                      <span className={!metadataForm.template_filename ? '' : 'ml-6'}>不绑定模板</span>
                    </Button>
                    {v3Templates.map((template) => (
                      <Button
                        key={template.filename}
                        variant="ghost"
                        size="sm"
                        className={cn(
                          "justify-start h-9",
                          metadataForm.template_filename === template.filename && "bg-accent"
                        )}
                        onClick={() => setMetadataForm({ ...metadataForm, template_filename: template.filename })}
                      >
                        {metadataForm.template_filename === template.filename && <Check className="h-4 w-4 mr-2" />}
                        <span className={metadataForm.template_filename === template.filename ? '' : 'ml-6'}>{template.name}</span>
                      </Button>
                    ))}
                  </div>
                </PopoverContent>
              </Popover>
              <p className='text-xs text-muted-foreground'>
                绑定模板后，获取订阅时将根据模板动态生成配置。绑定模板会自动禁用覆写开关。
              </p>
            </div>
            {/* 节点选择(绑定模板时显示):新模式 node mode(按 tag 分组列节点),老模式 tag mode(按标签筛) */}
            {metadataForm.template_filename && (
              <div className='space-y-2'>
                <div className='flex items-center justify-between gap-2'>
                  <Label>节点筛选</Label>
                  <div className='inline-flex rounded-md border text-xs'>
                    <button
                      type='button'
                      className={cn('px-2 py-0.5 rounded-l-md', pickerMode === 'node' ? 'bg-accent text-foreground' : 'text-muted-foreground')}
                      onClick={() => setPickerMode('node')}
                    >
                      节点选择
                    </button>
                    <button
                      type='button'
                      className={cn('px-2 py-0.5 rounded-r-md border-l', pickerMode === 'tag' ? 'bg-accent text-foreground' : 'text-muted-foreground')}
                      onClick={() => setPickerMode('tag')}
                    >
                      按标签筛选(兼容)
                    </button>
                  </div>
                </div>
                {pickerMode === 'node' ? (
                  <NodePickerByTag
                    allNodes={(allNodesData?.nodes ?? []) as Array<{ id: number; node_name: string; tag?: string; protocol?: string }>}
                    selectedIds={metadataForm.selected_node_ids}
                    onChange={(ids) => setMetadataForm({ ...metadataForm, selected_node_ids: ids })}
                  />
                ) : (
                <Popover>
                  <PopoverTrigger asChild>
                    <Button
                      variant="outline"
                      className="w-full justify-between"
                    >
                      {metadataForm.selected_tags.length > 0
                        ? `已选择 ${metadataForm.selected_tags.length} 个标签`
                        : '使用全部节点'}
                      <ChevronDown className="h-4 w-4 shrink-0 opacity-50" />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent className="w-[300px] p-1" align="start">
                    <div className="flex flex-col max-h-[300px] overflow-y-auto">
                      <Button
                        variant="ghost"
                        size="sm"
                        className={cn(
                          "justify-start h-9",
                          metadataForm.selected_tags.length === 0 && "bg-accent"
                        )}
                        onClick={() => setMetadataForm({ ...metadataForm, selected_tags: [] })}
                      >
                        {metadataForm.selected_tags.length === 0 && <Check className="h-4 w-4 mr-2" />}
                        <span className={metadataForm.selected_tags.length === 0 ? '' : 'ml-6'}>全部节点</span>
                      </Button>
                      {allNodeTags.map((tag) => {
                        const isSelected = metadataForm.selected_tags.includes(tag)
                        return (
                          <Button
                            key={tag}
                            variant="ghost"
                            size="sm"
                            className={cn(
                              "justify-start h-9",
                              isSelected && "bg-accent"
                            )}
                            onClick={() => {
                              const newTags = isSelected
                                ? metadataForm.selected_tags.filter(t => t !== tag)
                                : [...metadataForm.selected_tags, tag]
                              setMetadataForm({ ...metadataForm, selected_tags: newTags })
                            }}
                          >
                            {isSelected && <Check className="h-4 w-4 mr-2" />}
                            <span className={isSelected ? '' : 'ml-6'}>{tag}</span>
                          </Button>
                        )
                      })}
                    </div>
                  </PopoverContent>
                </Popover>
                )}
                <p className='text-xs text-muted-foreground'>
                  {pickerMode === 'node'
                    ? '选中具体节点;同时顶层 proxies 会自动裁掉未被代理组引用的孤儿。不选 = 使用全部节点。'
                    : '兼容模式:按标签筛选节点。不选则使用全部节点(老订阅自动进此模式)。'}
                </p>
              </div>
            )}
            <div className='space-y-2'>
              <Label htmlFor='traffic-limit'>总流量上限（GB，可选）</Label>
              <Input
                id='traffic-limit'
                type='number'
                min='0'
                step='0.01'
                value={metadataForm.traffic_limit}
                onChange={(e) => setMetadataForm({ ...metadataForm, traffic_limit: e.target.value })}
                placeholder='留空则使用探针服务器的总流量'
              />
              <p className='text-xs text-muted-foreground'>
                手动设置总流量上限，订阅信息中的总流量将使用此值。留空则跟随探针。
              </p>
            </div>
            <div className='space-y-2'>
              <Label>统计服务器（可选）</Label>
              {probeServers.length > 0 ? (
                <>
                  <div className='flex flex-wrap gap-2'>
                    {probeServers.map((srv) => {
                      const selectedIds = metadataForm.stats_server_ids ? metadataForm.stats_server_ids.split(',').map(s => s.trim()).filter(Boolean) : []
                      const isSelected = selectedIds.includes(srv.server_id)
                      return (
                        <Button
                          key={srv.server_id}
                          variant={isSelected ? 'default' : 'outline'}
                          size='sm'
                          onClick={() => {
                            const newIds = isSelected
                              ? selectedIds.filter(id => id !== srv.server_id)
                              : [...selectedIds, srv.server_id]
                            setMetadataForm({ ...metadataForm, stats_server_ids: newIds.join(',') })
                          }}
                        >
                          {srv.name}
                        </Button>
                      )
                    })}
                  </div>
                  <p className='text-xs text-muted-foreground'>
                    选择后，订阅信息中的已用流量将只统计选中服务器的流量。不选择则使用全部服务器。
                  </p>
                </>
              ) : (
                <p className='text-xs text-muted-foreground'>
                  未配置探针服务器，请先在节点管理中配置探针。
                </p>
              )}
            </div>
          </div>
          <DialogFooter className='shrink-0'>
            <Button
              variant='outline'
              onClick={() => setEditMetadataDialogOpen(false)}
              disabled={updateMetadataMutation.isPending}
            >
              取消
            </Button>
            <Button
              onClick={handleUpdateMetadata}
              disabled={updateMetadataMutation.isPending}
            >
              {updateMetadataMutation.isPending ? '保存中...' : '保存'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑配置对话框 */}
      <Dialog open={editConfigDialogOpen} onOpenChange={(open) => {
        setEditConfigDialogOpen(open)
        if (!open) {
          setEditingConfigFile(null)
          setConfigContent('')
        }
      }}>
        <DialogContent className='w-[95vw] sm:w-[80vw] sm:!max-w-[80vw] max-h-[90vh] flex flex-col'>
          <DialogHeader>
            <DialogTitle>编辑配置 - {editingConfigFile?.name}</DialogTitle>
            <DialogDescription>
              {editingConfigFile?.filename}
            </DialogDescription>
            <div className='flex gap-2 justify-center md:justify-end'>
              <Button
                variant='outline'
                size='sm'
                className='flex-1 md:flex-none'
                onClick={() => handleEditNodes(editingConfigFile!)}
              >
                <Edit className='mr-2 h-4 w-4' />
                编辑节点
              </Button>
              <Button
                size='sm'
                className='flex-1 md:flex-none'
                onClick={handleSaveConfig}
                disabled={saveConfigMutation.isPending}
              >
                <Save className='mr-2 h-4 w-4' />
                {saveConfigMutation.isPending ? '保存中...' : '保存'}
              </Button>
            </div>
          </DialogHeader>
          <div className='flex-1 overflow-y-auto space-y-4'>

            <div className='rounded-lg border bg-muted/30'>
              <Textarea
                value={configContent}
                onChange={(e) => setConfigContent(e.target.value)}
                className='min-h-[400px] resize-none border-0 bg-transparent font-mono text-xs'
                placeholder='加载配置中...'
              />
            </div>
            <div className='flex justify-end gap-2'>
              <Button onClick={handleSaveConfig} disabled={saveConfigMutation.isPending}>
                <Save className='mr-2 h-4 max-w-md' />
                {saveConfigMutation.isPending ? '保存中...' : '保存'}
              </Button>
            </div>
            <div className='rounded-lg border bg-muted/50 p-4'>
              <h3 className='mb-2 font-semibold'>使用说明</h3>
              <ul className='space-y-1 text-sm text-muted-foreground'>
                <li>• 点击"保存"按钮将修改保存到配置文件</li>
                <li>• 支持直接编辑 YAML 内容</li>
                <li>• 保存前会自动验证 YAML 格式</li>
                <li>• 支持 Clash、Clash Meta、Mihomo 等客户端</li>
              </ul>
            </div>
          </div>
        </DialogContent>
      </Dialog>

      {/* 编辑节点对话框 */}
      {!isMobile ? (
        <EditNodesDialog
          open={editNodesDialogOpen}
          onOpenChange={handleEditNodesDialogOpenChange}
          title={`编辑节点 - ${editingNodesFile?.name}`}
          proxyGroups={proxyGroups}
          availableNodes={availableNodes}
          allNodes={nodesQuery.data?.nodes || []}
          onProxyGroupsChange={setProxyGroups}
          onSave={handleSaveNodes}
          isSaving={saveConfigMutation.isPending}
          showAllNodes={showAllNodes}
          onShowAllNodesChange={setShowAllNodes}
          onRemoveNodeFromGroup={handleRemoveNodeFromGroup}
          onRemoveGroup={handleRemoveGroup}
          onRenameGroup={handleRenameGroup}
          saveButtonText='应用并保存'
          showSpecialNodesAtBottom={true}
          proxyProviderConfigs={enableProxyProvider ? proxyProviderConfigs : []}
        />
      ) : (
        <MobileEditNodesDialog
          open={editNodesDialogOpen}
          onOpenChange={handleEditNodesDialogOpenChange}
          proxyGroups={proxyGroups}
          availableNodes={availableNodes}
          allNodes={nodesQuery.data?.nodes || []}
          onProxyGroupsChange={setProxyGroups}
          onSave={handleSaveNodes}
          onRemoveNodeFromGroup={handleRemoveNodeFromGroup}
          onRemoveGroup={handleRemoveGroup}
          onRenameGroup={handleRenameGroup}
          showSpecialNodesAtBottom={true}
          proxyProviderConfigs={enableProxyProvider ? proxyProviderConfigs : []}
        />
      )}

      {/* 编辑节点草稿恢复对话框 */}
      <AlertDialog open={isNodesDraftRecoveryOpen} onOpenChange={setIsNodesDraftRecoveryOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>恢复本地缓存</AlertDialogTitle>
            <AlertDialogDescription>
              检测到未保存的节点编辑缓存，是否恢复？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={handleDiscardNodesDraft}>放弃</AlertDialogCancel>
            <AlertDialogAction onClick={handleRecoverNodesDraft}>恢复</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* 批量删除代理集合确认对话框 */}
      <AlertDialog open={batchDeleteDialogOpen} onOpenChange={setBatchDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认批量删除</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除选中的 {selectedProxyProviderIds.size} 个代理集合配置吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => batchDeleteProxyProviderMutation.mutate(Array.from(selectedProxyProviderIds))}
              disabled={batchDeleteProxyProviderMutation.isPending}
            >
              {batchDeleteProxyProviderMutation.isPending ? '删除中...' : '确认删除'}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* 代理集合配置对话框 */}
      <Dialog open={proxyProviderDialogOpen} onOpenChange={(open) => {
        setProxyProviderDialogOpen(open)
        if (!open) {
          setSelectedExternalSub(null)
          setEditingProxyProvider(null)
        }
      }}>
        <DialogContent className='w-[95vw] sm:w-auto sm:!max-w-fit max-h-[85vh] overflow-y-auto'>
          <DialogHeader>
            <DialogTitle>{editingProxyProvider ? '编辑代理集合配置' : '创建代理集合配置'}</DialogTitle>
            <DialogDescription>
              {editingProxyProvider
                ? `编辑代理集合 "${editingProxyProvider.name}" 的配置`
                : selectedExternalSub
                  ? `为外部订阅 "${selectedExternalSub.name}" 创建 proxy-provider 配置`
                  : '创建新的 proxy-provider 配置'
              }
            </DialogDescription>
          </DialogHeader>
          <div className='w-full sm:w-[600px] sm:max-w-[80vw]'>
            <div className='space-y-6'>
              {/* 基础配置 */}
              <div className='space-y-4'>
                <h4 className='font-medium text-sm'>基础配置</h4>
                <div className='grid grid-cols-1 sm:grid-cols-2 gap-4'>
                  {/* 外部订阅选择器 - 仅在创建模式下显示 */}
                  {!editingProxyProvider && (
                    <div className='space-y-2 sm:col-span-2'>
                      <Label htmlFor='pp-subscription'>外部订阅 *</Label>
                      <select
                        id='pp-subscription'
                        className='flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors'
                        value={selectedExternalSub?.id || ''}
                        onChange={(e) => {
                          const sub = externalSubs.find(s => s.id === Number(e.target.value))
                          setSelectedExternalSub(sub || null)
                        }}
                      >
                        <option value=''>请选择外部订阅</option>
                        {externalSubs.map(sub => (
                          <option key={sub.id} value={sub.id}>{sub.name}</option>
                        ))}
                      </select>
                    </div>
                  )}
                  <div className='space-y-2'>
                    <Label htmlFor='pp-name'>代理集合名称</Label>
                    <Input
                      id='pp-name'
                      value={proxyProviderForm.name}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, name: e.target.value }))}
                      placeholder='例如: 机场A'
                    />
                  </div>
                  {/* 妙妙屋处理模式显示 URL */}
                  {proxyProviderForm.process_mode === 'mmw' && (
                    <div className='space-y-2'>
                      <Label>订阅 URL</Label>
                      <div className='flex items-center gap-2'>
                        <Input
                          value={(() => {
                            const baseUrl = typeof window !== 'undefined' ? window.location.origin : ''
                            const configId = editingProxyProvider?.id || '{config_id}'
                            return `${baseUrl}/api/proxy-provider/${configId}?token=${userToken || '{user_token}'}`
                          })()}
                          readOnly
                          className='font-mono text-xs bg-muted'
                        />
                        <Button
                          type='button'
                          variant='outline'
                          size='sm'
                          onClick={() => {
                            const baseUrl = typeof window !== 'undefined' ? window.location.origin : ''
                            const configId = editingProxyProvider?.id || '{config_id}'
                            const url = `${baseUrl}/api/proxy-provider/${configId}?token=${userToken || '{user_token}'}`
                            navigator.clipboard.writeText(url)
                            toast.success('URL 已复制')
                          }}
                        >
                          <Copy className='h-4 w-4' />
                        </Button>
                      </div>
                      {!editingProxyProvider && (
                        <p className='text-xs text-muted-foreground'>保存后将生成实际的 config_id</p>
                      )}
                    </div>
                  )}
                  <div className='space-y-2'>
                    <Label htmlFor='pp-type'>类型</Label>
                    <select
                      id='pp-type'
                      className='flex h-9 w-full rounded-md border border-input bg-transparent px-3 py-1 text-sm shadow-sm transition-colors'
                      value={proxyProviderForm.type}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, type: e.target.value }))}
                    >
                      <option value='http'>http</option>
                      <option value='file'>file</option>
                    </select>
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='pp-interval'>更新间隔(秒)</Label>
                    <Input
                      id='pp-interval'
                      type='number'
                      value={proxyProviderForm.interval}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, interval: parseInt(e.target.value) || 3600 }))}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='pp-proxy'>下载代理</Label>
                    <Input
                      id='pp-proxy'
                      value={proxyProviderForm.proxy}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, proxy: e.target.value }))}
                      placeholder='DIRECT'
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='pp-size-limit'>文件大小限制</Label>
                    <Input
                      id='pp-size-limit'
                      type='number'
                      value={proxyProviderForm.size_limit}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, size_limit: parseInt(e.target.value) || 0 }))}
                    />
                  </div>
                </div>
              </div>

              {/* 请求头配置 */}
              <div className='space-y-4'>
                <h4 className='font-medium text-sm'>请求头</h4>
                <div className='grid grid-cols-1 sm:grid-cols-2 gap-4'>
                  <div className='space-y-2'>
                    <Label htmlFor='pp-user-agent'>User-Agent</Label>
                    <Input
                      id='pp-user-agent'
                      value={proxyProviderForm.header_user_agent}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, header_user_agent: e.target.value }))}
                      placeholder='Clash/v1.18.0'
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='pp-authorization'>Authorization</Label>
                    <Input
                      id='pp-authorization'
                      value={proxyProviderForm.header_authorization}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, header_authorization: e.target.value }))}
                      placeholder='鉴权token，如有则必填'
                    />
                  </div>
                </div>
              </div>

              {/* 健康检查配置 */}
              <div className='space-y-4'>
                <div className='flex items-center justify-between'>
                  <h4 className='font-medium text-sm'>健康检查</h4>
                  <Switch
                    checked={proxyProviderForm.health_check_enabled}
                    onCheckedChange={(checked) => setProxyProviderForm(prev => ({ ...prev, health_check_enabled: checked }))}
                  />
                </div>
                {proxyProviderForm.health_check_enabled && (
                  <div className='grid grid-cols-1 sm:grid-cols-2 gap-4'>
                    <div className='space-y-2 sm:col-span-2'>
                      <Label htmlFor='pp-hc-url'>检查URL</Label>
                      <Input
                        id='pp-hc-url'
                        value={proxyProviderForm.health_check_url}
                        onChange={(e) => setProxyProviderForm(prev => ({ ...prev, health_check_url: e.target.value }))}
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor='pp-hc-interval'>检查间隔(秒)</Label>
                      <Input
                        id='pp-hc-interval'
                        type='number'
                        value={proxyProviderForm.health_check_interval}
                        onChange={(e) => setProxyProviderForm(prev => ({ ...prev, health_check_interval: parseInt(e.target.value) || 300 }))}
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor='pp-hc-timeout'>超时(ms)</Label>
                      <Input
                        id='pp-hc-timeout'
                        type='number'
                        value={proxyProviderForm.health_check_timeout}
                        onChange={(e) => setProxyProviderForm(prev => ({ ...prev, health_check_timeout: parseInt(e.target.value) || 5000 }))}
                      />
                    </div>
                    <div className='space-y-2'>
                      <Label htmlFor='pp-hc-status'>期望状态码</Label>
                      <Input
                        id='pp-hc-status'
                        type='number'
                        value={proxyProviderForm.health_check_expected_status}
                        onChange={(e) => setProxyProviderForm(prev => ({ ...prev, health_check_expected_status: parseInt(e.target.value) || 204 }))}
                      />
                    </div>
                    <div className='flex items-center space-x-2'>
                      <Checkbox
                        id='pp-hc-lazy'
                        checked={proxyProviderForm.health_check_lazy}
                        onCheckedChange={(checked) => setProxyProviderForm(prev => ({ ...prev, health_check_lazy: !!checked }))}
                      />
                      <Label htmlFor='pp-hc-lazy' className='text-sm'>懒惰模式</Label>
                    </div>
                  </div>
                )}
              </div>

              {/* 高级配置处理方式 */}
              <div className='space-y-3'>
                <h4 className='font-medium text-sm'>高级配置处理方式</h4>
                <div className='grid grid-cols-1 sm:grid-cols-2 gap-2'>
                  <Button
                    type='button'
                    variant={proxyProviderForm.process_mode === 'client' ? 'default' : 'outline'}
                    className='h-auto py-3 px-4 flex flex-col items-start text-left'
                    onClick={() => setProxyProviderForm(prev => ({ ...prev, process_mode: 'client' }))}
                  >
                    <span className='font-medium'>由客户端处理</span>
                    <span className='text-xs opacity-70 font-normal'>高级配置输出到订阅配置中</span>
                  </Button>
                  <Button
                    type='button'
                    variant={proxyProviderForm.process_mode === 'mmw' ? 'default' : 'outline'}
                    className='h-auto py-3 px-4 flex flex-col items-start text-left'
                    onClick={() => setProxyProviderForm(prev => ({ ...prev, process_mode: 'mmw' }))}
                  >
                    <span className='font-medium'>由妙妙屋处理</span>
                    <span className='text-xs opacity-70 font-normal'>URL 指向妙妙屋接口</span>
                  </Button>
                </div>
              </div>

              {/* 高级配置 */}
              <div className='space-y-4'>
                <h4 className='font-medium text-sm'>高级配置 {proxyProviderForm.process_mode === 'client' ? '(输出到配置)' : '(由妙妙屋处理)'}</h4>
                <div className='grid grid-cols-1 sm:grid-cols-2 gap-4'>
                  <div className='space-y-2'>
                    <Label htmlFor='pp-filter'>节点过滤(正则)</Label>
                    <Input
                      id='pp-filter'
                      value={proxyProviderForm.filter}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, filter: e.target.value }))}
                      placeholder='例如: 香港|日本'
                    />
                    <p className='text-xs text-muted-foreground'>保留匹配的节点</p>
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='pp-exclude-filter'>节点排除(正则)</Label>
                    <Input
                      id='pp-exclude-filter'
                      value={proxyProviderForm.exclude_filter}
                      onChange={(e) => setProxyProviderForm(prev => ({ ...prev, exclude_filter: e.target.value }))}
                      placeholder='例如: 过期|剩余'
                    />
                    <p className='text-xs text-muted-foreground'>排除匹配的节点</p>
                  </div>
                </div>
                <div className='space-y-2'>
                  <Label>排除协议类型</Label>
                  <div className='flex flex-wrap gap-1.5'>
                    {PROXY_TYPES.map(type => {
                      const isSelected = proxyProviderForm.exclude_type.includes(type)
                      return (
                        <Button
                          key={type}
                          type='button'
                          variant={isSelected ? 'default' : 'outline'}
                          size='sm'
                          className='h-7 px-2.5 text-xs'
                          onClick={() => {
                            if (isSelected) {
                              setProxyProviderForm(prev => ({
                                ...prev,
                                exclude_type: prev.exclude_type.filter(t => t !== type)
                              }))
                            } else {
                              setProxyProviderForm(prev => ({
                                ...prev,
                                exclude_type: [...prev.exclude_type, type]
                              }))
                            }
                          }}
                        >
                          {type}
                        </Button>
                      )
                    })}
                  </div>
                </div>
                {/* 覆写配置 */}
                <div className='space-y-3'>
                  <h4 className='font-medium text-sm'>覆写配置</h4>

                  {/* 连接设置 */}
                  <div className='space-y-2'>
                    <Label className='text-xs text-muted-foreground'>连接设置</Label>
                    <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
                      <div className='flex items-center justify-between'>
                        <Label htmlFor='pp-override-tfo' className='text-xs'>TCP Fast Open</Label>
                        <Switch
                          id='pp-override-tfo'
                          checked={proxyProviderForm.override.tfo}
                          onCheckedChange={(checked) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, tfo: checked }
                          }))}
                        />
                      </div>
                      <div className='flex items-center justify-between'>
                        <Label htmlFor='pp-override-mptcp' className='text-xs'>Multipath TCP</Label>
                        <Switch
                          id='pp-override-mptcp'
                          checked={proxyProviderForm.override.mptcp}
                          onCheckedChange={(checked) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, mptcp: checked }
                          }))}
                        />
                      </div>
                      <div className='flex items-center justify-between'>
                        <Label htmlFor='pp-override-udp' className='text-xs'>启用 UDP</Label>
                        <Switch
                          id='pp-override-udp'
                          checked={proxyProviderForm.override.udp}
                          onCheckedChange={(checked) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, udp: checked }
                          }))}
                        />
                      </div>
                      <div className='flex items-center justify-between'>
                        <Label htmlFor='pp-override-uot' className='text-xs'>UDP over TCP</Label>
                        <Switch
                          id='pp-override-uot'
                          checked={proxyProviderForm.override.udp_over_tcp}
                          onCheckedChange={(checked) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, udp_over_tcp: checked }
                          }))}
                        />
                      </div>
                      <div className='flex items-center justify-between sm:col-span-2'>
                        <Label htmlFor='pp-override-skip-cert' className='text-xs'>跳过证书验证</Label>
                        <Switch
                          id='pp-override-skip-cert'
                          checked={proxyProviderForm.override.skip_cert_verify}
                          onCheckedChange={(checked) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, skip_cert_verify: checked }
                          }))}
                        />
                      </div>
                    </div>
                  </div>

                  {/* 代理设置 */}
                  <div className='space-y-2'>
                    <Label htmlFor='pp-override-dialer-proxy' className='text-xs text-muted-foreground'>链式代理 (dialer-proxy)</Label>
                    <Input
                      id='pp-override-dialer-proxy'
                      value={proxyProviderForm.override.dialer_proxy}
                      onChange={(e) => setProxyProviderForm(prev => ({
                        ...prev,
                        override: { ...prev.override, dialer_proxy: e.target.value }
                      }))}
                      placeholder='例如: 节点选择'
                      className='h-8 text-sm'
                    />
                  </div>

                  {/* 网络设置 */}
                  <div className='space-y-2'>
                    <Label className='text-xs text-muted-foreground'>网络设置</Label>
                    <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
                      <div className='space-y-1'>
                        <Label htmlFor='pp-override-interface' className='text-xs'>出站接口</Label>
                        <Input
                          id='pp-override-interface'
                          value={proxyProviderForm.override.interface_name}
                          onChange={(e) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, interface_name: e.target.value }
                          }))}
                          placeholder='例如: eth0'
                          className='h-8 text-sm'
                        />
                      </div>
                      <div className='space-y-1'>
                        <Label htmlFor='pp-override-routing-mark' className='text-xs'>路由标记</Label>
                        <Input
                          id='pp-override-routing-mark'
                          value={proxyProviderForm.override.routing_mark}
                          onChange={(e) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, routing_mark: e.target.value }
                          }))}
                          placeholder='例如: 255'
                          className='h-8 text-sm'
                        />
                      </div>
                    </div>
                    <div className='space-y-1'>
                      <Label htmlFor='pp-override-ip-version' className='text-xs'>IP 版本</Label>
                      <Select
                        value={proxyProviderForm.override.ip_version}
                        onValueChange={(value) => setProxyProviderForm(prev => ({
                          ...prev,
                          override: { ...prev.override, ip_version: value as OverrideForm['ip_version'] }
                        }))}
                      >
                        <SelectTrigger id='pp-override-ip-version' className='h-8 text-sm'>
                          <SelectValue placeholder='选择 IP 版本' />
                        </SelectTrigger>
                        <SelectContent>
                          {IP_VERSION_OPTIONS.map(opt => (
                            <SelectItem key={opt.value} value={opt.value || '_default'}>
                              {opt.label}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                  </div>

                  {/* 节点名称修改 */}
                  <div className='space-y-2'>
                    <Label className='text-xs text-muted-foreground'>节点名称修改</Label>
                    <div className='grid grid-cols-1 sm:grid-cols-2 gap-3'>
                      <div className='space-y-1'>
                        <Label htmlFor='pp-override-prefix' className='text-xs'>名称前缀</Label>
                        <Input
                          id='pp-override-prefix'
                          value={proxyProviderForm.override.additional_prefix}
                          onChange={(e) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, additional_prefix: e.target.value }
                          }))}
                          placeholder='例如: [机场A]'
                          className='h-8 text-sm'
                        />
                      </div>
                      <div className='space-y-1'>
                        <Label htmlFor='pp-override-suffix' className='text-xs'>名称后缀</Label>
                        <Input
                          id='pp-override-suffix'
                          value={proxyProviderForm.override.additional_suffix}
                          onChange={(e) => setProxyProviderForm(prev => ({
                            ...prev,
                            override: { ...prev.override, additional_suffix: e.target.value }
                          }))}
                          placeholder='例如: -Premium'
                          className='h-8 text-sm'
                        />
                      </div>
                    </div>
                  </div>
                </div>
              </div>

              {/* 生成的配置预览 */}
              <div className='space-y-2'>
                <div className='flex items-center justify-between'>
                  <h4 className='font-medium text-sm'>生成的配置预览</h4>
                  <Button
                    variant='outline'
                    size='sm'
                    onClick={() => {
                      const preview = generateProxyProviderYAML()
                      navigator.clipboard.writeText(preview)
                      toast.success('配置已复制到剪贴板')
                    }}
                  >
                    <Copy className='h-4 w-4 mr-1' />
                    复制
                  </Button>
                </div>
                <pre className='text-xs bg-muted p-3 rounded-md overflow-x-auto whitespace-pre-wrap'>
                  {generateProxyProviderYAML()}
                </pre>
              </div>
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setProxyProviderDialogOpen(false)}>
              取消
            </Button>
            <Button
              onClick={() => {
                // 构建 header JSON
                const headerObj: Record<string, string[]> = {}
                if (proxyProviderForm.header_user_agent) {
                  headerObj['User-Agent'] = proxyProviderForm.header_user_agent.split(',').map(s => s.trim())
                }
                if (proxyProviderForm.header_authorization) {
                  headerObj['Authorization'] = [proxyProviderForm.header_authorization]
                }

                const payload = {
                  name: proxyProviderForm.name,
                  type: proxyProviderForm.type,
                  interval: proxyProviderForm.interval,
                  proxy: proxyProviderForm.proxy,
                  size_limit: proxyProviderForm.size_limit,
                  header: Object.keys(headerObj).length > 0 ? JSON.stringify(headerObj) : '',
                  health_check_enabled: proxyProviderForm.health_check_enabled,
                  health_check_url: proxyProviderForm.health_check_url,
                  health_check_interval: proxyProviderForm.health_check_interval,
                  health_check_timeout: proxyProviderForm.health_check_timeout,
                  health_check_lazy: proxyProviderForm.health_check_lazy,
                  health_check_expected_status: proxyProviderForm.health_check_expected_status,
                  filter: proxyProviderForm.filter,
                  exclude_filter: proxyProviderForm.exclude_filter,
                  exclude_type: proxyProviderForm.exclude_type.join(','),
                  override: overrideFormToJSON(proxyProviderForm.override),
                  process_mode: proxyProviderForm.process_mode,
                }

                if (editingProxyProvider) {
                  // 编辑模式
                  updateProxyProviderMutation.mutate({
                    id: editingProxyProvider.id,
                    external_subscription_id: editingProxyProvider.external_subscription_id,
                    ...payload,
                  })
                } else {
                  // 创建模式
                  if (!selectedExternalSub) {
                    toast.error('请选择外部订阅')
                    return
                  }
                  createProxyProviderMutation.mutate({
                    external_subscription_id: selectedExternalSub.id,
                    ...payload,
                  })
                }
              }}
              disabled={
                !proxyProviderForm.name ||
                (!editingProxyProvider && !selectedExternalSub) ||
                createProxyProviderMutation.isPending ||
                updateProxyProviderMutation.isPending
              }
            >
              {editingProxyProvider ? '更新配置' : '保存配置'}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 缺失节点替换对话框 */}
      <Dialog open={missingNodesDialogOpen} onOpenChange={setMissingNodesDialogOpen}>
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>发现缺失节点</DialogTitle>
            <DialogDescription>
              以下节点在 rules 中被引用，但不存在于代理组与节点中
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            {/* 缺失节点列表 */}
            <div className='max-h-[200px] overflow-y-auto border rounded-md p-3 space-y-1'>
              {missingNodes.map((node, index) => (
                <div key={index} className='text-sm font-mono bg-muted px-2 py-1 rounded'>
                  {node}
                </div>
              ))}
            </div>
            {/* 替换选项 */}
            <div className='space-y-2'>
              <Label>选择替换为：</Label>
              <div className='grid grid-cols-3 gap-2'>
                <Button
                  variant={replacementChoice === 'DIRECT' ? 'default' : 'outline'}
                  onClick={() => setReplacementChoice('DIRECT')}
                  className='w-full'
                >
                  DIRECT
                </Button>
                <Button
                  variant={replacementChoice === 'REJECT' ? 'default' : 'outline'}
                  onClick={() => setReplacementChoice('REJECT')}
                  className='w-full'
                >
                  REJECT
                </Button>
                {(() => {
                  try {
                    const parsedConfig = parseYAML(pendingConfigAfterSave) as any
                    const proxyGroupNames = parsedConfig['proxy-groups']?.map((g: any) => g.name) || []
                    return proxyGroupNames.map((name: string) => (
                      <Button
                        key={name}
                        variant={replacementChoice === name ? 'default' : 'outline'}
                        onClick={() => setReplacementChoice(name)}
                        className='w-full'
                      >
                        {name}
                      </Button>
                    ))
                  } catch {
                    return null
                  }
                })()}
              </div>
              <p className='text-xs text-muted-foreground'>
                将把上述缺失的节点替换为 <span className='font-semibold'>{replacementChoice}</span>
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button
              variant='outline'
              onClick={() => setMissingNodesDialogOpen(false)}
            >
              取消
            </Button>
            <Button onClick={handleApplyReplacement}>
              应用替换
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 代理集合Pro对话框 */}
      <Dialog open={proxyProviderProDialogOpen} onOpenChange={setProxyProviderProDialogOpen}>
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>创建代理集合(初级)</DialogTitle>
            <DialogDescription>批量创建代理集合，支持按地域或协议分裂</DialogDescription>
          </DialogHeader>

          <div className='space-y-4'>
            {/* 选择外部订阅 */}
            <div className='space-y-2'>
              <Label>选择外部订阅</Label>
              <Select
                value={proSelectedExternalSub?.id?.toString() || ''}
                onValueChange={(v) => {
                  const sub = externalSubs.find(s => s.id === parseInt(v))
                  setProSelectedExternalSub(sub || null)
                  setProNamePrefix(sub?.name || '')
                  setProCreationResults([])
                }}
              >
                <SelectTrigger>
                  <SelectValue placeholder='请选择外部订阅' />
                </SelectTrigger>
                <SelectContent>
                  {externalSubs.map(sub => (
                    <SelectItem key={sub.id} value={sub.id.toString()}>
                      {sub.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            {/* 名称前缀输入框 */}
            <div className='space-y-2'>
              <Label>名称前缀</Label>
              <Input
                placeholder='输入名称前缀'
                value={proNamePrefix}
                onChange={(e) => setProNamePrefix(e.target.value)}
              />
              <p className='text-xs text-muted-foreground'>
                生成的代理集合名称格式: 前缀-地域/协议
              </p>
            </div>

            {/* 根据IP位置分组开关 */}
            <div className='flex items-center justify-between'>
              <div className='space-y-0.5'>
                <Label>根据IP位置分组</Label>
                <p className='text-xs text-muted-foreground'>
                  开启后，节点名称匹配不到时会根据服务器IP位置匹配
                </p>
              </div>
              <Switch
                checked={enableGeoIPMatching}
                onCheckedChange={setEnableGeoIPMatching}
              />
            </div>

            {/* 分裂按钮 */}
            <div className='flex gap-2'>
              <Button
                className='flex-1'
                disabled={!proSelectedExternalSub || !proNamePrefix.trim() || proCreatingRegion || proCreatingProtocol}
                onClick={handleBatchCreateByRegion}
              >
                {proCreatingRegion && <RefreshCw className='h-4 w-4 mr-2 animate-spin' />}
                按地域分裂
              </Button>
              <Button
                className='flex-1'
                variant='outline'
                disabled={!proSelectedExternalSub || !proNamePrefix.trim() || proCreatingRegion || proCreatingProtocol}
                onClick={handleBatchCreateByProtocol}
              >
                {proCreatingProtocol && <RefreshCw className='h-4 w-4 mr-2 animate-spin' />}
                按代理协议分裂
              </Button>
            </div>

            {/* 创建结果 */}
            {proCreationResults.length > 0 && (
              <div className='space-y-2'>
                <Label>创建结果 ({proCreationResults.filter(r => r.success).length}/{proCreationResults.length})</Label>
                <ScrollArea className='h-[200px] border rounded-md p-2'>
                  {proCreationResults.map((result, idx) => (
                    <div key={idx} className='flex items-center gap-2 text-sm py-1'>
                      {result.success ? (
                        <Badge variant='default' className='bg-green-500'>成功</Badge>
                      ) : (
                        <Badge variant='destructive'>失败</Badge>
                      )}
                      <span className='truncate flex-1'>{result.name}</span>
                      {result.error && <span className='text-destructive text-xs'>({result.error})</span>}
                    </div>
                  ))}
                </ScrollArea>
              </div>
            )}
          </div>

          <DialogFooter>
            <Button variant='outline' onClick={() => setProxyProviderProDialogOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 代理集合预览对话框（MMW 模式） */}
      <Dialog open={previewDialogOpen} onOpenChange={setPreviewDialogOpen}>
        <DialogContent className='max-w-3xl max-h-[80vh]'>
          <DialogHeader>
            <DialogTitle>预览处理结果 - {previewConfigName}</DialogTitle>
            <DialogDescription>妙妙屋处理后的代理节点配置</DialogDescription>
          </DialogHeader>

          <div className='relative'>
            {previewLoading ? (
              <div className='flex items-center justify-center py-8'>
                <RefreshCw className='h-6 w-6 animate-spin text-muted-foreground' />
                <span className='ml-2 text-muted-foreground'>加载中...</span>
              </div>
            ) : (
              <ScrollArea className='h-[50vh] border rounded-md'>
                <pre className='p-4 text-xs font-mono whitespace-pre-wrap break-all'>{previewContent}</pre>
              </ScrollArea>
            )}
          </div>

          <DialogFooter className='flex-row gap-2 sm:justify-between'>
            <Button
              variant='outline'
              size='sm'
              onClick={() => {
                navigator.clipboard.writeText(previewContent)
                toast.success('已复制到剪贴板')
              }}
              disabled={previewLoading || !previewContent}
            >
              <Copy className='h-4 w-4 mr-2' />
              复制
            </Button>
            <Button variant='outline' onClick={() => setPreviewDialogOpen(false)}>
              关闭
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 编辑外部订阅对话框 */}
      <Dialog open={editExternalSubDialogOpen} onOpenChange={(open) => {
        setEditExternalSubDialogOpen(open)
        if (!open) {
          setEditingExternalSub(null)
        }
      }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>编辑外部订阅</DialogTitle>
            <DialogDescription>
              修改外部订阅的地址和流量统计方式
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4'>
            <div className='space-y-2'>
              <Label>订阅地址</Label>
              <Input
                value={editExternalSubForm.url}
                onChange={(e) => setEditExternalSubForm(prev => ({ ...prev, url: e.target.value }))}
                placeholder='https://example.com/subscribe'
              />
            </div>
            <div className='space-y-2'>
              <Label>流量统计方式</Label>
              <Select
                value={editExternalSubForm.traffic_mode}
                onValueChange={(value: 'download' | 'upload' | 'both' | 'none') => setEditExternalSubForm(prev => ({ ...prev, traffic_mode: value }))}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='both'>上下行 (download + upload)</SelectItem>
                  <SelectItem value='download'>仅下行 (download)</SelectItem>
                  <SelectItem value='upload'>仅上行 (upload)</SelectItem>
                  <SelectItem value='none'>不统计 (none)</SelectItem>
                </SelectContent>
              </Select>
              <p className='text-xs text-muted-foreground'>
                选择如何计算已用流量：上下行为两者相加，仅下行或仅上行则只计算对应流量
              </p>
            </div>
            <div className='space-y-2 rounded-md border p-3'>
              <div className='flex items-center justify-between gap-3'>
                <div className='space-y-1'>
                  <Label htmlFor='edit-sub-auto-update' className='text-sm font-medium cursor-pointer'>
                    定时更新此订阅
                  </Label>
                  <p className='text-xs text-muted-foreground'>
                    开启后服务端按间隔自动拉取并同步节点
                  </p>
                </div>
                <Switch
                  id='edit-sub-auto-update'
                  checked={editExternalSubForm.auto_update}
                  onCheckedChange={(checked) => setEditExternalSubForm(prev => ({ ...prev, auto_update: checked }))}
                />
              </div>
              {editExternalSubForm.auto_update && (
                <div className='flex flex-wrap items-center gap-2 pt-1'>
                  <Label htmlFor='edit-sub-update-interval' className='text-sm whitespace-nowrap'>
                    更新间隔
                  </Label>
                  <Select
                    value={String(editExternalSubForm.update_interval_minutes)}
                    onValueChange={(v) => setEditExternalSubForm(prev => ({ ...prev, update_interval_minutes: Number(v) }))}
                  >
                    <SelectTrigger id='edit-sub-update-interval' className='w-[160px]'>
                      <SelectValue placeholder='选择间隔' />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='30'>30 分钟</SelectItem>
                      <SelectItem value='60'>1 小时</SelectItem>
                      <SelectItem value='180'>3 小时</SelectItem>
                      <SelectItem value='360'>6 小时</SelectItem>
                      <SelectItem value='720'>12 小时</SelectItem>
                      <SelectItem value='1440'>24 小时</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
              )}
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setEditExternalSubDialogOpen(false)}>
              取消
            </Button>
            <Button
              onClick={() => {
                if (editingExternalSub) {
                  updateExternalSubMutation.mutate({
                    id: editingExternalSub.id,
                    name: editingExternalSub.name,
                    url: editExternalSubForm.url,
                    user_agent: editingExternalSub.user_agent,
                    traffic_mode: editExternalSubForm.traffic_mode,
                    auto_update: editExternalSubForm.auto_update,
                    update_interval_minutes: editExternalSubForm.update_interval_minutes,
                  })
                  setEditExternalSubDialogOpen(false)
                  setEditingExternalSub(null)
                }
              }}
              disabled={updateExternalSubMutation.isPending || !editExternalSubForm.url}
            >
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </main>
  )
}

// NodePickerByTag:按节点 tag 分组列出全部节点;点 tag 标题 = 切换该 tag 下所有节点的勾选(用户原话"点击标签快速选中标签下所有节点")
// 存储模型:selected_node_ids(精确节点 ID);后端按 ID 过滤,不再依赖 tag。
function NodePickerByTag({
  allNodes,
  selectedIds,
  onChange,
}: {
  allNodes: Array<{ id: number; node_name: string; tag?: string; protocol?: string }>
  selectedIds: number[]
  onChange: (ids: number[]) => void
}) {
  const selected = new Set(selectedIds)
  // 按 tag 分组(无 tag 落到"未分类")
  const groups = new Map<string, Array<{ id: number; node_name: string; protocol?: string }>>()
  for (const n of allNodes) {
    const key = (n.tag || '').trim() || '未分类'
    const arr = groups.get(key) || []
    arr.push({ id: n.id, node_name: n.node_name, protocol: n.protocol })
    groups.set(key, arr)
  }
  const setIds = (next: number[]) => onChange(next)
  const toggleNode = (id: number) => {
    const s = new Set(selected)
    if (s.has(id)) s.delete(id)
    else s.add(id)
    setIds(Array.from(s))
  }
  const toggleGroupAll = (members: Array<{ id: number }>) => {
    const s = new Set(selected)
    const allIn = members.every((n) => s.has(n.id))
    if (allIn) members.forEach((n) => s.delete(n.id))
    else members.forEach((n) => s.add(n.id))
    setIds(Array.from(s))
  }

  if (allNodes.length === 0) {
    return <div className='text-xs text-muted-foreground border rounded-md p-3 text-center'>暂无节点</div>
  }

  return (
    <div className='space-y-1'>
      <div className='flex items-center justify-between text-xs text-muted-foreground'>
        <span>共 {allNodes.length} 个节点</span>
        <span className='tabular-nums'>已选 {selected.size}</span>
      </div>
      <div className='border rounded-md max-h-64 overflow-y-auto divide-y'>
        {Array.from(groups.entries()).map(([tag, members]) => {
          const allIn = members.every((n) => selected.has(n.id))
          return (
            <div key={tag} className='p-2 space-y-1'>
              <div className='flex items-center justify-between'>
                <button
                  type='button'
                  className='flex items-center gap-1.5 text-sm font-medium hover:text-primary'
                  onClick={() => toggleGroupAll(members)}
                  title='点击切换该标签下所有节点'
                >
                  <Checkbox checked={allIn} />
                  {tag}
                </button>
                <span className='text-[10px] text-muted-foreground tabular-nums'>
                  {members.filter((n) => selected.has(n.id)).length}/{members.length}
                </span>
              </div>
              <div className='pl-5 grid grid-cols-1 sm:grid-cols-2 gap-y-1'>
                {members.map((n) => (
                  <label key={n.id} className='flex items-center gap-1.5 text-xs cursor-pointer'>
                    <Checkbox checked={selected.has(n.id)} onCheckedChange={() => toggleNode(n.id)} />
                    <span className='truncate' title={n.node_name}>{n.node_name}</span>
                    {n.protocol && <span className='text-[9px] uppercase text-muted-foreground shrink-0'>{n.protocol}</span>}
                  </label>
                ))}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
