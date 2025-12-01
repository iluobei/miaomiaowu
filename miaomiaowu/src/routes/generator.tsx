import { useState, useRef, useMemo, useEffect } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { Copy, Download, Loader2, Save, Layers, Activity, Upload } from 'lucide-react'
import { type DragEndEvent, type DragStartEvent } from '@dnd-kit/core'
import { arrayMove } from '@dnd-kit/sortable'
import { Topbar } from '@/components/layout/topbar'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { EditNodesDialog } from '@/components/edit-nodes-dialog'
import { useNodeDragDrop } from '@/hooks/use-node-drag-drop'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from 'sonner'
import { ClashConfigBuilder } from '@/lib/sublink/clash-builder'
import { CustomRulesEditor } from '@/components/custom-rules-editor'
import { RuleSelector } from '@/components/rule-selector'
import type { PredefinedRuleSetType, CustomRule } from '@/lib/sublink/types'
import type { ProxyConfig } from '@/lib/sublink/types'
import yaml from 'js-yaml'

// 确保 short-id 字段始终作为字符串处理
function ensureShortIdAsString(obj: any): any {
  if (typeof obj !== 'object' || obj === null) {
    return obj
  }

  if (Array.isArray(obj)) {
    return obj.map(ensureShortIdAsString)
  }

  const result: any = {}
  for (const [key, value] of Object.entries(obj)) {
    if (key === 'short-id') {
      // 强制转换为字符串
      if (value === null || value === undefined) {
        result[key] = ''
      } else if (typeof value === 'string') {
        result[key] = value
      } else {
        // 数字等其他类型转为字符串
        result[key] = String(value)
      }
    } else if (typeof value === 'object' && value !== null) {
      result[key] = ensureShortIdAsString(value)
    } else {
      result[key] = value
    }
  }
  return result
}

// 修复 YAML 中的 short-id 空值显示
function fixShortIdInYaml(yamlStr: string): string {
  let result = yamlStr
  // 1. 将 short-id: '' (单引号空字符串) 替换为 short-id: ""
  result = result.replace(/^([ \t]*)short-id:[ \t]*''[ \t]*$/gm, '$1short-id: ""')
  // 2. 将 short-id: 后面没有值的行替换为 short-id: ""
  result = result.replace(/^([ \t]*)short-id:[ \t]*$/gm, '$1short-id: ""')
  // 3. 将 short-id: 'value' (单引号非空值) 替换为 short-id: "value"
  result = result.replace(/^([ \t]*)short-id:[ \t]*'([^']*)'[ \t]*$/gm, '$1short-id: "$2"')
  return result
}

// 重新排序代理节点字段，将 name, type, server, port 放在最前面
function reorderProxyFields(proxy: ProxyConfig): ProxyConfig {
  const ordered: any = {}
  const priorityKeys = ['name', 'type', 'server', 'port']

  // 先添加优先字段
  for (const key of priorityKeys) {
    if (key in proxy) {
      ordered[key] = (proxy as any)[key]
    }
  }

  // 再添加其他字段
  for (const [key, value] of Object.entries(proxy)) {
    if (!priorityKeys.includes(key)) {
      ordered[key] = value
    }
  }

  return ordered as ProxyConfig
}

type SavedNode = {
  id: number
  raw_url: string
  node_name: string
  protocol: string
  parsed_config: string
  clash_config: string
  enabled: boolean
  tag: string
  probe_server: string
  created_at: string
  updated_at: string
}

export const Route = createFileRoute('/generator')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/login' })
    }
  },
  component: SubscriptionGeneratorPage,
})

function SubscriptionGeneratorPage() {
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()
  const [ruleSet, setRuleSet] = useState<PredefinedRuleSetType>('balanced')
  const [selectedCategories, setSelectedCategories] = useState<string[]>([])
  const [customRules, setCustomRules] = useState<CustomRule[]>([])
  const [loading, setLoading] = useState(false)
  const [clashConfig, setClashConfig] = useState('')
  const [selectedNodeIds, setSelectedNodeIds] = useState<Set<number>>(new Set())
  const [protocolFilter, setProtocolFilter] = useState<string>('all')
  const [tagFilter, setTagFilter] = useState<string>('all')

  // 规则模式状态
  const [ruleMode, setRuleMode] = useState<'custom' | 'template'>('custom')
  const [selectedTemplate, setSelectedTemplate] = useState<string>('')
  const [hasManuallyGrouped, setHasManuallyGrouped] = useState(false)

  // 上传模板状态
  const [uploadDialogOpen, setUploadDialogOpen] = useState(false)
  const fileInputRef = useRef<HTMLInputElement>(null)

  // 保存订阅对话框状态
  const [saveDialogOpen, setSaveDialogOpen] = useState(false)
  const [subscribeName, setSubscribeName] = useState('')
  const [subscribeFilename, setSubscribeFilename] = useState('')
  const [subscribeDescription, setSubscribeDescription] = useState('')

  // 手动分组对话框状态
  const [groupDialogOpen, setGroupDialogOpen] = useState(false)
  const [proxyGroups, setProxyGroups] = useState<ProxyGroup[]>([])
  const [allProxies, setAllProxies] = useState<string[]>([])
  const [activeCard, setActiveCard] = useState<{ name: string; type: string; proxies: string[] } | null>(null)
  const [showAllNodes, setShowAllNodes] = useState(true)
  const dragTimeoutRef = useRef<NodeJS.Timeout | null>(null)

  // 使用拖拽 hook (generator 需要过滤特殊节点)
  const {
    draggedNode: draggedItem,
    activeGroupTitle,
    setActiveGroupTitle,
    handleDragStart: handleDragStartBase,
    handleDragEnd: handleDragEndBase,
    handleDragEnterGroup: handleDragEnterGroupBase,
    handleDragLeaveGroup: handleDragLeaveGroupBase,
    handleDrop: handleDropBase,
    handleDropToAvailable: handleDropToAvailableBase
  } = useNodeDragDrop({
    proxyGroups,
    onProxyGroupsChange: setProxyGroups,
    specialNodesToFilter: ['♻️ 自动选择', '🚀 节点选择', 'DIRECT', 'REJECT']
  })

  // 自定义 dragOverGroup 状态（用于防抖）
  const [dragOverGroup, setDragOverGroup] = useState<string | null>(null)

  // 适配器函数：将 generator 的参数名适配到 hook
  const handleDragStart = (proxy: string, sourceGroup: string | null, sourceIndex: number, filteredNodes?: string[]) => {
    handleDragStartBase(proxy, sourceGroup, sourceIndex, filteredNodes)
  }

  const handleDrop = (targetGroupName: string, targetIndex?: number) => {
    handleDropBase(targetGroupName, targetIndex)
  }

  const handleDropToAvailable = () => {
    handleDropToAvailableBase()
  }

  const handleDragEnd = () => {
    handleDragEndBase()
    setDragOverGroup(null)
  }

  // 带防抖的拖拽进入/离开处理
  const handleDragEnterGroup = (groupName: string) => {
    // 清除之前的定时器
    if (dragTimeoutRef.current) {
      clearTimeout(dragTimeoutRef.current)
    }
    // 立即设置高亮状态
    setDragOverGroup(groupName)
    handleDragEnterGroupBase(groupName)
  }

  const handleDragLeaveGroup = () => {
    // 使用防抖延迟清除高亮，避免在节点交界处抖动
    if (dragTimeoutRef.current) {
      clearTimeout(dragTimeoutRef.current)
    }
    dragTimeoutRef.current = setTimeout(() => {
      setDragOverGroup(null)
    }, 50)
    handleDragLeaveGroupBase()
  }

  // 缺失节点替换对话框状态
  const [missingNodesDialogOpen, setMissingNodesDialogOpen] = useState(false)
  const [missingNodes, setMissingNodes] = useState<string[]>([])
  const [replacementChoice, setReplacementChoice] = useState<string>('DIRECT')
  const [pendingConfigAfterGrouping, setPendingConfigAfterGrouping] = useState<string>('')

  // 获取已保存的节点
  const { data: nodesData } = useQuery({
    queryKey: ['nodes'],
    queryFn: async () => {
      const response = await api.get('/api/admin/nodes')
      return response.data as { nodes: SavedNode[] }
    },
    enabled: Boolean(auth.accessToken),
  })

  // 获取规则模板列表
  const { data: templatesData } = useQuery({
    queryKey: ['rule-templates'],
    queryFn: async () => {
      const response = await api.get('/api/admin/rule-templates')
      return response.data as { templates: string[] }
    },
    enabled: Boolean(auth.accessToken),
  })

  const savedNodes = nodesData?.nodes ?? []
  const enabledNodes = savedNodes.filter(n => n.enabled)
  const templates = templatesData?.templates ?? []

  // 默认选择第一个模板
  useEffect(() => {
    if (ruleMode === 'template' && templates.length > 0 && !selectedTemplate) {
      setSelectedTemplate(templates[0])
    }
  }, [ruleMode, templates, selectedTemplate])

  // 上传模板 mutation
  const uploadTemplateMutation = useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData()
      formData.append('template', file)
      const response = await api.post('/api/admin/rule-templates/upload', formData, {
        headers: { 'Content-Type': 'multipart/form-data' }
      })
      return response.data as { filename: string }
    },
    onSuccess: (data) => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      setSelectedTemplate(data.filename)
      setUploadDialogOpen(false)
      toast.success('模板上传成功')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '上传模板失败')
    }
  })

  const handleUploadTemplate = () => {
    const file = fileInputRef.current?.files?.[0]
    if (!file) {
      toast.error('请选择文件')
      return
    }

    // 检查文件扩展名
    if (!file.name.endsWith('.yaml') && !file.name.endsWith('.yml')) {
      toast.error('只支持 .yaml 或 .yml 文件')
      return
    }

    uploadTemplateMutation.mutate(file)
  }

  // 获取所有协议类型
  const protocols = Array.from(new Set(enabledNodes.map(n => n.protocol.toLowerCase()))).sort()

  // 获取所有标签类型
  const tags = Array.from(new Set(enabledNodes.map(n => n.tag))).sort()

  // 根据协议和标签筛选节点
  const filteredNodes = enabledNodes.filter(n => {
    const protocolMatch = protocolFilter === 'all' || n.protocol.toLowerCase() === protocolFilter
    const tagMatch = tagFilter === 'all' || n.tag === tagFilter
    return protocolMatch && tagMatch
  })

  const handleToggleNode = (nodeId: number) => {
    const newSet = new Set(selectedNodeIds)
    if (newSet.has(nodeId)) {
      newSet.delete(nodeId)
    } else {
      newSet.add(nodeId)
    }
    setSelectedNodeIds(newSet)
  }

  const handleToggleAll = () => {
    if (selectedNodeIds.size === filteredNodes.length) {
      setSelectedNodeIds(new Set())
    } else {
      setSelectedNodeIds(new Set(filteredNodes.map(n => n.id)))
    }
  }

  type ProxyGroup = {
    name: string
    type: string
    proxies: string[]
    url?: string
    interval?: number
    lazy?: boolean
  }

  // 计算可用节点（根据 showAllNodes 状态过滤）
  const availableProxies = useMemo(() => {
    if (showAllNodes) {
      return allProxies
    }

    // 收集所有已使用的节点
    const usedNodes = new Set<string>()
    proxyGroups.forEach(group => {
      group.proxies.forEach(proxy => {
        usedNodes.add(proxy)
      })
    })

    // 只返回未使用的节点
    return allProxies.filter(name => !usedNodes.has(name))
  }, [allProxies, proxyGroups, showAllNodes])

  // 加载模板并插入节点
  const handleLoadTemplate = async () => {
    if (selectedNodeIds.size === 0) {
      toast.error('请选择至少一个节点')
      return
    }

    if (!selectedTemplate) {
      toast.error('请选择一个模板')
      return
    }

    setLoading(true)
    try {
      // 获取选中的节点并转换为ProxyConfig
      const selectedNodes = savedNodes.filter(n => selectedNodeIds.has(n.id))
      const proxies: ProxyConfig[] = selectedNodes.map(node => {
        try {
          return JSON.parse(node.clash_config) as ProxyConfig
        } catch (e) {
          console.error('Failed to parse clash config for node:', node.node_name, e)
          return null
        }
      }).filter((p): p is ProxyConfig => p !== null)

      if (proxies.length === 0) {
        toast.error('未能解析到任何有效节点')
        return
      }

      // 读取模板文件
      const response = await api.get(`/api/admin/rule-templates/${selectedTemplate}`)
      const templateContent = response.data.content as string

      // 解析模板
      const templateConfig = yaml.load(templateContent) as any

      // 插入代理节点，并重新排序字段
      templateConfig.proxies = proxies.map(proxy => reorderProxyFields(proxy))

      // 确保 short-id 字段始终作为字符串
      const processedConfig = ensureShortIdAsString(templateConfig)

      // 转换回 YAML
      let finalConfig = yaml.dump(processedConfig, {
        lineWidth: -1,
        noRefs: true,
      })

      // 修复 short-id 空值显示
      finalConfig = fixShortIdInYaml(finalConfig)

      // 应用自定义规则
      try {
        const applyRulesResponse = await api.post('/api/admin/apply-custom-rules', {
          yaml_content: finalConfig
        })
        finalConfig = applyRulesResponse.data.yaml_content
      } catch (error) {
        console.error('Apply custom rules error:', error)
        // 应用规则失败不影响主流程，继续使用原配置
      }

      setClashConfig(finalConfig)
      setHasManuallyGrouped(false) // 加载模板后重置手动分组状态
      toast.success(`成功加载模板并插入 ${proxies.length} 个节点`)
    } catch (error) {
      console.error('Load template error:', error)
      toast.error('加载模板失败')
    } finally {
      setLoading(false)
    }
  }

  const handleGenerate = async () => {
    if (selectedNodeIds.size === 0) {
      toast.error('请选择至少一个节点')
      return
    }

    setLoading(true)
    try {
      // 获取选中的节点并转换为ProxyConfig
      const selectedNodes = savedNodes.filter(n => selectedNodeIds.has(n.id))
      const proxies: ProxyConfig[] = selectedNodes.map(node => {
        try {
          return JSON.parse(node.clash_config) as ProxyConfig
        } catch (e) {
          console.error('Failed to parse clash config for node:', node.node_name, e)
          return null
        }
      }).filter((p): p is ProxyConfig => p !== null)

      if (proxies.length === 0) {
        toast.error('未能解析到任何有效节点')
        return
      }

      toast.success(`成功加载 ${proxies.length} 个节点`)

      // Validate custom rules
      const validCustomRules = customRules.filter((rule) => rule.name.trim() !== '')
      if (validCustomRules.length > 0) {
        toast.info(`应用 ${validCustomRules.length} 条自定义规则`)
      }

      // All rule sets now use selected categories
      if (selectedCategories.length > 0) {
        toast.info(`应用 ${selectedCategories.length} 个规则类别`)
      }

      // Build Clash config using new builder
      const clashBuilder = new ClashConfigBuilder(
        proxies,
        selectedCategories,
        validCustomRules
      )
      let generatedConfig = clashBuilder.build()

      // 应用自定义规则
      let addedProxyGroups: string[] = []
      try {
        const applyRulesResponse = await api.post('/api/admin/apply-custom-rules', {
          yaml_content: generatedConfig
        })
        generatedConfig = applyRulesResponse.data.yaml_content
        addedProxyGroups = applyRulesResponse.data.added_proxy_groups || []
      } catch (error) {
        console.error('Apply custom rules error:', error)
        // 应用规则失败不影响主流程，继续使用原配置
      }

      setClashConfig(generatedConfig)
      setHasManuallyGrouped(true) // 自定义规则模式生成后自动标记为已分组

      // 显示生成成功通知，如果有新增代理组则包含提示
      if (addedProxyGroups.length > 0) {
        toast.success(
          `Clash 配置生成成功！已应用自定义规则，新增了以下代理组：${addedProxyGroups.join('、')}，默认节点：🚀 节点选择、DIRECT`,
          { duration: 8000 }
        )
      } else {
        toast.success('Clash 配置生成成功！')
      }
    } catch (error) {
      console.error('Generation error:', error)
      toast.error('生成订阅链接失败')
    } finally {
      setLoading(false)
    }
  }

  const copyToClipboard = () => {
    navigator.clipboard.writeText(clashConfig)
    toast.success('Clash 配置已复制到剪贴板')
  }

  const downloadClashConfig = () => {
    const blob = new Blob([clashConfig], { type: 'text/yaml;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'clash-config.yaml'
    document.body.appendChild(a)
    a.click()
    document.body.removeChild(a)
    URL.revokeObjectURL(url)
    toast.success('clash-config.yaml 下载成功')
  }

  const handleClear = () => {
    setSelectedNodeIds(new Set())
    setSelectedCategories([])
    setCustomRules([])
    setClashConfig('')
    toast.info('已清空所有内容')
  }

  // 保存订阅 mutation
  const saveSubscribeMutation = useMutation({
    mutationFn: async (data: { name: string; filename: string; description: string; content: string }) => {
      const response = await api.post('/api/admin/subscribe-files/create-from-config', data)
      return response.data
    },
    onSuccess: () => {
      toast.success('订阅保存成功！')
      toast.info('请前往"订阅文件"页面查看')
      setSaveDialogOpen(false)
      setSubscribeName('')
      setSubscribeFilename('')
      setSubscribeDescription('')
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      queryClient.invalidateQueries({ queryKey: ['user-subscriptions'] })
    },
    onError: (error: any) => {
      const message = error.response?.data?.error || '保存订阅失败'
      toast.error(message)
    },
  })

  const handleOpenSaveDialog = () => {
    if (!clashConfig) {
      toast.error('请先生成配置')
      return
    }
    // 使用模板模式时，必须先手动分组
    if (ruleMode === 'template' && !hasManuallyGrouped) {
      toast.error('请先手动分组节点')
      return
    }
    setSaveDialogOpen(true)
  }

  const handleSaveSubscribe = () => {
    if (!subscribeName.trim()) {
      toast.error('请输入订阅名称')
      return
    }

    saveSubscribeMutation.mutate({
      name: subscribeName.trim(),
      filename: subscribeFilename.trim(),
      description: subscribeDescription.trim(),
      content: clashConfig,
    })
  }

  // 手动分组功能
  const handleOpenGroupDialog = () => {
    if (!clashConfig) {
      toast.error('请先生成配置')
      return
    }

    try {
      // 解析当前的 Clash 配置
      const parsedConfig = yaml.load(clashConfig) as any

      if (!parsedConfig['proxy-groups']) {
        toast.error('配置中没有找到代理组')
        return
      }

      // 获取所有代理组，确保每个组都有 proxies 数组
      const groups = (parsedConfig['proxy-groups'] as any[]).map(group => ({
        ...group,
        proxies: group.proxies || []
      })) as ProxyGroup[]

      // 获取用户选中的节点，添加默认的特殊节点
      const selectedNodes = savedNodes.filter(n => selectedNodeIds.has(n.id))
      const nodeNames = selectedNodes.map(n => n.node_name)
      const specialNodes = ['♻️ 自动选择', '🚀 节点选择', 'DIRECT', 'REJECT']
      const availableNodes = [...specialNodes, ...nodeNames]

      setProxyGroups(groups)
      setAllProxies(availableNodes)
      setGroupDialogOpen(true)
    } catch (error) {
      console.error('解析配置失败:', error)
      toast.error('解析配置失败，请检查配置格式')
    }
  }

  const handleApplyGrouping = () => {
    try {
      // 解析当前配置
      const parsedConfig = yaml.load(clashConfig) as any

      // 更新代理组，过滤掉 undefined 值
      parsedConfig['proxy-groups'] = proxyGroups.map(group => ({
        ...group,
        proxies: group.proxies.filter((p): p is string => p !== undefined)
      }))

      // 处理链式代理：给落地节点组中的节点添加 dialer-proxy 参数
      const landingGroup = proxyGroups.find(g => g.name === '🌄 落地节点')
      const hasRelayGroup = proxyGroups.some(g => g.name === '🌠 中转节点')

      if (landingGroup && hasRelayGroup && parsedConfig.proxies && Array.isArray(parsedConfig.proxies)) {
        // 获取落地节点组中的所有节点名称
        const landingNodeNames = new Set(landingGroup.proxies.filter((p): p is string => p !== undefined))

        // 创建节点名称到协议的映射
        const nodeProtocolMap = new Map<string, string>()
        savedNodes.forEach(node => {
          nodeProtocolMap.set(node.node_name, node.protocol)
        })

        // 给这些节点添加 dialer-proxy 参数（跳过已经是链式代理的节点）
        parsedConfig.proxies = parsedConfig.proxies.map((proxy: any) => {
          if (landingNodeNames.has(proxy.name)) {
            // 通过协议判断是否为链式代理节点（协议包含 ⇋）
            const protocol = nodeProtocolMap.get(proxy.name)
            if (protocol && protocol.includes('⇋')) {
              return proxy
            }
            return {
              ...proxy,
              'dialer-proxy': '🌠 中转节点'
            }
          }
          return proxy
        })
      }

      // 重新排序 proxies 字段
      if (parsedConfig.proxies && Array.isArray(parsedConfig.proxies)) {
        parsedConfig.proxies = parsedConfig.proxies.map((proxy: any) => reorderProxyFields(proxy))
      }

      // 确保 short-id 字段始终作为字符串
      const processedConfig = ensureShortIdAsString(parsedConfig)

      // 转换回 YAML
      let newConfig = yaml.dump(processedConfig, {
        lineWidth: -1,
        noRefs: true,
      })

      // 修复 short-id 空值显示
      newConfig = fixShortIdInYaml(newConfig)

      // 验证 rules 中引用的节点是否都存在
      const validationResult = validateRulesNodes(parsedConfig)

      if (validationResult.missingNodes.length > 0) {
        // 有缺失的节点，显示替换对话框
        setMissingNodes(validationResult.missingNodes)
        setPendingConfigAfterGrouping(newConfig)
        setMissingNodesDialogOpen(true)
      } else {
        // 没有缺失节点，直接应用
        setClashConfig(newConfig)
        setGroupDialogOpen(false)
        setHasManuallyGrouped(true)
        toast.success('分组已应用到配置')
      }
    } catch (error) {
      console.error('应用分组失败:', error)
      toast.error('应用分组失败，请检查配置')
    }
  }

  // 验证 rules 中的节点是否存在于 proxy-groups 中
  const validateRulesNodes = (parsedConfig: any) => {
    const rules = parsedConfig.rules || []
    const proxyGroupNames = new Set(parsedConfig['proxy-groups']?.map((g: any) => g.name) || [])

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
        toast(`[validateRulesNodes] 规则 ${index} 不是字符串或对象格式:`, rule)
        return
      }

      // 如果节点名称不在 proxy-groups 中，添加到缺失列表
      if (nodeName && !proxyGroupNames.has(nodeName)) {
        toast(`[validateRulesNodes] 发现缺失节点: "${nodeName}"`)
        // 此处改为rule, 更直观一点
        missingNodes.add(rule)
      }
    })

    return {
      missingNodes: Array.from(missingNodes)
    }
  }

  // 应用缺失节点替换
  const handleApplyReplacement = () => {
    try {
      const parsedConfig = yaml.load(pendingConfigAfterGrouping) as any
      const rules = parsedConfig.rules || []
      const proxyGroupNames = new Set(parsedConfig['proxy-groups']?.map((g: any) => g.name) || [])

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
          // 如果节点缺失，替换为用户选择的值
          if (nodeName && !proxyGroupNames.has(nodeName)) {
            parts[parts.length - 1] = replacementChoice
            return parts.join(',')
          }
        } else if (typeof rule === 'object' && rule !== null) {
          // 对象格式的规则，检查并替换可能的节点字段
          const nodeName = rule.target || rule.group || rule.proxy || rule.ruleset
          if (nodeName && !proxyGroupNames.has(nodeName)) {
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

      // 重新排序 proxies 字段
      if (parsedConfig.proxies && Array.isArray(parsedConfig.proxies)) {
        parsedConfig.proxies = parsedConfig.proxies.map((proxy: any) => reorderProxyFields(proxy))
      }

      // 确保 short-id 字段始终作为字符串
      const processedConfigFinal = ensureShortIdAsString(parsedConfig)

      // 转换回 YAML
      let finalConfig = yaml.dump(processedConfigFinal, {
        lineWidth: -1,
        noRefs: true,
      })

      // 修复 short-id 空值显示
      finalConfig = fixShortIdInYaml(finalConfig)

      setClashConfig(finalConfig)
      setGroupDialogOpen(false)
      setMissingNodesDialogOpen(false)
      setHasManuallyGrouped(true)
      setPendingConfigAfterGrouping('')
      setMissingNodes([])
      toast.success(`已将缺失节点替换为 ${replacementChoice}`)
    } catch (error) {
      console.error('应用替换失败:', error)
      toast.error('应用替换失败，请检查配置')
    }
  }

  // 配置链式代理
  const handleConfigureChainProxy = () => {
    // 检查是否已存在这两个代理组
    const hasLandingNode = proxyGroups.some(g => g.name === '🌄 落地节点')
    const hasRelayNode = proxyGroups.some(g => g.name === '🌠 中转节点')

    // 从链式代理节点中提取落地节点和中转节点
    const chainProxyNodes = enabledNodes.filter(node => node.node_name.includes('⇋'))

    const landingNodeNames = new Set<string>()
    const relayNodeNames = new Set<string>()

    chainProxyNodes.forEach(node => {
      const parts = node.node_name.split('⇋')
      if (parts.length === 2) {
        landingNodeNames.add(parts[0].trim())
        relayNodeNames.add(parts[1].trim())
      }
    })

    const newGroups: ProxyGroup[] = []

    if (!hasLandingNode) {
      newGroups.push({
        name: '🌄 落地节点',
        type: 'select',
        proxies: Array.from(landingNodeNames)
      })
    }

    if (!hasRelayNode) {
      newGroups.push({
        name: '🌠 中转节点',
        type: 'select',
        proxies: Array.from(relayNodeNames)
      })
    }

    if (newGroups.length > 0) {
      setProxyGroups(groups => {
        const updatedGroups = [...newGroups, ...groups]

        // 如果添加了落地节点，将其添加到"🚀 节点选择"组的第一位
        if (newGroups.some(g => g.name === '🌄 落地节点')) {
          return updatedGroups.map(group => {
            if (group.name === '🚀 节点选择') {
              // 过滤掉已存在的"🌄 落地节点"（如果有的话）
              const filteredProxies = (group.proxies || []).filter(p => p !== '🌄 落地节点')
              // 将"🌄 落地节点"添加到第一位
              return {
                ...group,
                proxies: ['🌄 落地节点', ...filteredProxies]
              }
            }
            return group
          })
        }

        return updatedGroups
      })
      toast.success(`已添加 ${newGroups.map(g => g.name).join('、')}`)
    } else {
      toast.info('链式代理节点已存在')
    }
  }

  // DND Kit 卡片排序处理函数
  // DND Kit 辅助函数 - 解析放置目标
  const resolveTargetGroup = (overItem: any) => {
    if (!overItem) {
      return null
    }
    const overId = String(overItem.id)
    const ensureValidGroup = (groupName: string | null) =>
      groupName && proxyGroups.some(group => group.name === groupName) ? groupName : null
    if (overId.startsWith('drop-')) {
      return ensureValidGroup(overId.replace('drop-', ''))
    }
    const overData = overItem.data?.current as { groupName?: string } | undefined
    if (overData?.groupName) {
      return ensureValidGroup(overData.groupName)
    }
    return ensureValidGroup(overId || null)
  }

  const handleCardDragStart = (event: DragStartEvent) => {
    const activeId = String(event.active.id)

    if (activeId.startsWith('group-title-')) {
      const groupName = activeId.replace('group-title-', '')
      handleDragStart(groupName, null, -1)
      setActiveGroupTitle(groupName)
    } else {
      // 拖动整个卡片
      const group = proxyGroups.find(g => g.name === activeId)
      if (group) {
        setActiveCard(group)
      }
    }
  }

  const handleCardDragEnd = (event: DragEndEvent) => {
    const { active, over } = event

    // 清除拖动状态
    setActiveCard(null)
    setActiveGroupTitle(null)

    if (!over) {
      if (String(active.id).startsWith('group-title-')) {
        handleDragEnd()
      }
      setDragOverGroup(null)
      return
    }

    const activeId = String(active.id)

    // 处理卡片排序（拖动卡片顶部按钮）
    if (!activeId.startsWith('group-title-') && !activeId.startsWith('drop-')) {
      if (active.id === over.id) {
        return
      }
      setProxyGroups((groups) => {
        const oldIndex = groups.findIndex((g) => g.name === active.id)
        const newIndex = groups.findIndex((g) => g.name === over.id)
        return arrayMove(groups, oldIndex, newIndex)
      })
      return
    }

    // 处理拖动代理组标题作为节点
    if (activeId.startsWith('group-title-')) {
      const groupName = activeId.replace('group-title-', '')
      const targetGroupName = resolveTargetGroup(over)

      if (targetGroupName && targetGroupName !== groupName) {
        setProxyGroups((groups) => {
          return groups.map((group) => {
            if (group.name === targetGroupName) {
              if (!group.proxies.includes(groupName)) {
                return {
                  ...group,
                  proxies: [...group.proxies, groupName],
                }
              }
            }
            return group
          })
        })
      }

      handleDragEnd()
    }

    setDragOverGroup(null)
  }

  // DND Kit 节点排序处理函数（在同一个组内）
  const handleNodeDragEnd = (groupName: string) => (event: DragEndEvent) => {
    const { active, over } = event

    if (!over || active.id === over.id) {
      return
    }

    setProxyGroups((groups) => {
      return groups.map((group) => {
        if (group.name !== groupName) {
          return group
        }

        const proxies = group.proxies || []
        const oldIndex = proxies.findIndex((p) => `${groupName}-${p}` === active.id)
        const newIndex = proxies.findIndex((p) => `${groupName}-${p}` === over.id)

        return {
          ...group,
          proxies: arrayMove(proxies, oldIndex, newIndex),
        }
      })
    })
  }

  // 删除节点
  const handleRemoveProxy = (groupName: string, proxyIndex: number) => {
    setProxyGroups(groups =>
      groups.map(group => {
        if (group.name === groupName) {
          return {
            ...group,
            proxies: group.proxies.filter((_, idx) => idx !== proxyIndex)
          }
        }
        return group
      })
    )
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

    // 同时更新待处理的配置（如果存在）
    if (pendingConfigAfterGrouping) {
      try {
        const parsedConfig = yaml.load(pendingConfigAfterGrouping) as any
        if (parsedConfig && parsedConfig['proxy-groups']) {
          // 更新 proxy-groups 中的组名
          parsedConfig['proxy-groups'] = parsedConfig['proxy-groups'].map((group: any) => ({
            ...group,
            name: group.name === oldName ? newName : group.name,
            proxies: group.proxies.map((proxy: string) => proxy === oldName ? newName : proxy)
          }))
        }

        // 更新 rules 中的代理组引用
        if (parsedConfig && parsedConfig['rules'] && Array.isArray(parsedConfig['rules'])) {
          const updatedRules = parsedConfig['rules'].map((rule: any) => {
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
          parsedConfig['rules'] = updatedRules
        }

        // 确保 short-id 字段始终作为字符串
        const processedParsedConfig = ensureShortIdAsString(parsedConfig)

        // 转换回YAML并更新待处理配置
        let newConfig = yaml.dump(processedParsedConfig, { lineWidth: -1, noRefs: true })

        // 修复 short-id 空值显示
        newConfig = fixShortIdInYaml(newConfig)

        setPendingConfigAfterGrouping(newConfig)
      } catch (error) {
        console.error('更新待处理配置中的代理组引用失败:', error)
      }
    }

    // 更新当前显示的配置（如果存在）
    if (clashConfig) {
      try {
        const parsedConfig = yaml.load(clashConfig) as any
        if (parsedConfig && parsedConfig['proxy-groups']) {
          // 更新 proxy-groups 中的组名
          parsedConfig['proxy-groups'] = parsedConfig['proxy-groups'].map((group: any) => ({
            ...group,
            name: group.name === oldName ? newName : group.name,
            proxies: group.proxies.map((proxy: string) => proxy === oldName ? newName : proxy)
          }))
        }

        // 更新 rules 中的代理组引用
        if (parsedConfig && parsedConfig['rules'] && Array.isArray(parsedConfig['rules'])) {
          const updatedRules = parsedConfig['rules'].map((rule: any) => {
            if (typeof rule === 'string') {
              const parts = rule.split(',')
              if (parts.length >= 3 && parts[2] === oldName) {
                parts[2] = newName
                return parts.join(',')
              }
            } else if (typeof rule === 'object' && rule.target) {
              if (rule.target === oldName) {
                return { ...rule, target: newName }
              }
            }
            return rule
          })
          parsedConfig['rules'] = updatedRules
        }

        // 确保 short-id 字段始终作为字符串
        const processedCurrentConfig = ensureShortIdAsString(parsedConfig)

        // 转换回YAML并更新当前配置
        let newConfig = yaml.dump(processedCurrentConfig, { lineWidth: -1, noRefs: true })

        // 修复 short-id 空值显示
        newConfig = fixShortIdInYaml(newConfig)

        setClashConfig(newConfig)
      } catch (error) {
        console.error('更新当前配置中的代理组引用失败:', error)
      }
    }
  }

  // 处理手动分组对话框关闭
  const handleGroupDialogOpenChange = (open: boolean) => {
    if (!open) {
      // 先关闭对话框
      setGroupDialogOpen(false)

      // 延迟重置数据，避免用户看到复位动画
      setTimeout(() => {
        setProxyGroups([])
        setAllProxies([])
        setActiveGroupTitle(null)
      }, 200)
    } else {
      setGroupDialogOpen(open)
    }
  }

  return (
    <div className='flex min-h-screen flex-col bg-background'>
      <Topbar />

      <main className='mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24'>
        <div className='mx-auto space-y-6'>
          <div className='space-y-2'>
            <h1 className='text-3xl font-bold tracking-tight'>订阅链接生成器</h1>
            <p className='text-muted-foreground'>
              从节点管理中选择节点，快速生成 Clash 订阅配置
            </p>
          </div>

          <Card>
            <CardHeader>
              <CardTitle>选择节点</CardTitle>
              <CardDescription>
                从已保存的节点中选择需要添加到订阅的节点（已选择 {selectedNodeIds.size} 个）
              </CardDescription>
            </CardHeader>
            <CardContent className='space-y-4'>
              {enabledNodes.length === 0 ? (
                <div className='text-center py-8 text-muted-foreground'>
                  暂无可用节点，请先在节点管理中添加节点
                </div>
              ) : (
                <>
                  {/* 协议筛选按钮 */}
                  <div className='flex flex-wrap gap-2 mb-4'>
                    <Button
                      variant={protocolFilter === 'all' ? 'default' : 'outline'}
                      size='sm'
                      onClick={() => {
                        setProtocolFilter('all')
                        // 全选所有符合当前标签筛选的节点
                        const nodesToSelect = enabledNodes.filter(n => {
                          const tagMatch = tagFilter === 'all' || n.tag === tagFilter
                          return tagMatch
                        })
                        setSelectedNodeIds(new Set(nodesToSelect.map(n => n.id)))
                      }}
                    >
                      全部 ({enabledNodes.length})
                    </Button>
                    {protocols.map((protocol) => {
                      const count = enabledNodes.filter(n => n.protocol.toLowerCase() === protocol).length
                      return (
                        <Button
                          key={protocol}
                          variant={protocolFilter === protocol ? 'default' : 'outline'}
                          size='sm'
                          onClick={() => {
                            setProtocolFilter(protocol)
                            // 全选符合该协议和当前标签筛选的节点
                            const nodesToSelect = enabledNodes.filter(n => {
                              const protocolMatch = n.protocol.toLowerCase() === protocol
                              const tagMatch = tagFilter === 'all' || n.tag === tagFilter
                              return protocolMatch && tagMatch
                            })
                            setSelectedNodeIds(new Set(nodesToSelect.map(n => n.id)))
                          }}
                        >
                          {protocol.toUpperCase()} ({count})
                        </Button>
                      )
                    })}
                  </div>

                  {/* 标签筛选按钮 */}
                  {tags.length > 0 && (
                    <div className='flex flex-wrap gap-2 mb-4'>
                      <Button
                        variant={tagFilter === 'all' ? 'default' : 'outline'}
                        size='sm'
                        onClick={() => {
                          setTagFilter('all')
                          // 计算应该选中的节点
                          const nodesToSelect = enabledNodes.filter(n => {
                            const protocolMatch = protocolFilter === 'all' || n.protocol.toLowerCase() === protocolFilter
                            return protocolMatch
                          })
                          const nodeIdsToSelect = new Set(nodesToSelect.map(n => n.id))

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
                        全部标签 ({enabledNodes.length})
                      </Button>
                      {tags.map((tag) => {
                        const count = enabledNodes.filter(n => n.tag === tag).length
                        return (
                          <Button
                            key={tag}
                            variant={tagFilter === tag ? 'default' : 'outline'}
                            size='sm'
                            onClick={() => {
                              setTagFilter(tag)
                              // 计算应该选中的节点
                              const nodesToSelect = enabledNodes.filter(n => {
                                const protocolMatch = protocolFilter === 'all' || n.protocol.toLowerCase() === protocolFilter
                                const tagMatch = n.tag === tag
                                return protocolMatch && tagMatch
                              })
                              const nodeIdsToSelect = new Set(nodesToSelect.map(n => n.id))

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
                            {tag} ({count})
                          </Button>
                        )
                      })}
                    </div>
                  )}

                  <div className='rounded-md border max-h-[440px] overflow-y-auto'>
                  <Table>
                    <TableHeader className='sticky top-0 bg-background z-10'>
                      <TableRow>
                        <TableHead className='w-[50px]'>
                          <Checkbox
                            checked={filteredNodes.length > 0 && filteredNodes.every(n => selectedNodeIds.has(n.id))}
                            onCheckedChange={handleToggleAll}
                          />
                        </TableHead>
                        <TableHead>节点名称</TableHead>
                        <TableHead className='w-[100px]'>协议</TableHead>
                        <TableHead className='min-w-[150px]'>服务器地址</TableHead>
                        <TableHead className='w-[100px]'>标签</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {filteredNodes.map((node) => {
                        // 从 clash_config 中提取服务器地址
                        let serverAddress = '-'
                        try {
                          if (node.clash_config) {
                            const clashConfig = JSON.parse(node.clash_config)
                            if (clashConfig.server) {
                              const port = clashConfig.port ? `:${clashConfig.port}` : ''
                              serverAddress = `${clashConfig.server}${port}`
                            }
                          }
                        } catch (e) {
                          // 解析失败，使用默认值
                        }

                        return (
                        <TableRow key={node.id}>
                          <TableCell>
                            <Checkbox
                              checked={selectedNodeIds.has(node.id)}
                              onCheckedChange={() => handleToggleNode(node.id)}
                            />
                          </TableCell>
                          <TableCell className='font-medium'>{node.node_name}</TableCell>
                          <TableCell>
                            <Badge variant='outline'>{node.protocol.toUpperCase()}</Badge>
                          </TableCell>
                          <TableCell className='font-mono text-sm'>{serverAddress}</TableCell>
                          <TableCell>
                            <div className='flex flex-wrap gap-1'>
                              {node.tag && (
                                <Badge variant='secondary' className='text-xs'>
                                  {node.tag}
                                </Badge>
                              )}
                              {node.probe_server && (
                                <Badge variant='secondary' className='text-xs flex items-center gap-1'>
                                  <Activity className='size-3' />
                                  {node.probe_server}
                                </Badge>
                              )}
                            </div>
                          </TableCell>
                        </TableRow>
                        )
                      })}
                    </TableBody>
                  </Table>
                  </div>
                </>
              )}

              {/* 规则模式选择 */}
              <div className='space-y-4'>
                <Label>规则模式</Label>
                <div className='flex gap-2'>
                  <Button
                    variant={ruleMode === 'custom' ? 'default' : 'outline'}
                    onClick={() => setRuleMode('custom')}
                    className='flex-1'
                  >
                    自定义规则
                  </Button>
                  <Button
                    variant={ruleMode === 'template' ? 'default' : 'outline'}
                    onClick={() => setRuleMode('template')}
                    className='flex-1'
                  >
                    使用模板
                  </Button>
                </div>
              </div>

              {/* 自定义规则模式 */}
              {ruleMode === 'custom' && (
                <RuleSelector
                  ruleSet={ruleSet}
                  onRuleSetChange={setRuleSet}
                  selectedCategories={selectedCategories}
                  onCategoriesChange={setSelectedCategories}
                />
              )}

              {/* 模板模式 */}
              {ruleMode === 'template' && (
                <div className='space-y-4'>
                  <div className='space-y-2'>
                    <Label htmlFor='template-select'>选择模板</Label>
                    <p className='text-sm text-muted-foreground'>
                      模板为静态文件模板(源代码rule_templates目录中)，不会提交节点到转换后端，放心使用。
                    </p>
                  </div>
                  <div className='flex gap-2'>
                    <div className='flex-1'>
                      <Select value={selectedTemplate} onValueChange={setSelectedTemplate}>
                        <SelectTrigger id='template-select'>
                          <SelectValue placeholder='请选择模板' />
                        </SelectTrigger>
                        <SelectContent>
                          {templates.map((template) => (
                            <SelectItem key={template} value={template}>
                              {template}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </div>
                    <Button
                      variant='outline'
                      onClick={() => setUploadDialogOpen(true)}
                    >
                      <Upload className='mr-2 h-4 w-4' />
                      上传
                    </Button>
                    <div className='flex items-end'>
                      <div
                        onClick={() => {
                          if (selectedNodeIds.size === 0) {
                            toast.error('请先选择节点')
                          } else if (!selectedTemplate) {
                            toast.error('请先选择模板')
                          }
                        }}
                      >
                        <Button
                          onClick={handleLoadTemplate}
                          disabled={loading || selectedNodeIds.size === 0 || !selectedTemplate}
                        >
                          {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
                          加载
                        </Button>
                      </div>
                    </div>
                  </div>
                </div>
              )}

              {ruleMode === 'custom' && (
                <div className='flex gap-2'>
                  <div
                    className='flex-1'
                    onClick={() => {
                      if (selectedNodeIds.size === 0) {
                        toast.error('请先选择节点')
                      }
                    }}
                  >
                    <Button onClick={handleGenerate} disabled={loading || selectedNodeIds.size === 0} className='w-full'>
                      {loading && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
                      {loading ? '生成中...' : '生成订阅文件'}
                    </Button>
                  </div>
                  <Button variant='outline' onClick={handleClear}>
                    清空
                  </Button>
                </div>
              )}
            </CardContent>
          </Card>

          <CustomRulesEditor rules={customRules} onChange={setCustomRules} />

          {clashConfig && (
            <Card>
              <CardHeader>
                <div className='flex flex-col gap-4 md:flex-row md:items-center md:justify-between'>
                  <div>
                    <CardTitle>生成的 Clash 配置</CardTitle>
                    <CardDescription>
                      预览生成的 YAML 配置文件，可复制或下载
                    </CardDescription>
                  </div>
                  <div className='flex flex-wrap gap-2'>
                    <Button variant='outline' size='sm' onClick={copyToClipboard}>
                      <Copy className='mr-2 h-4 w-4' />
                      复制
                    </Button>
                    <Button variant='outline' size='sm' onClick={downloadClashConfig}>
                      <Download className='mr-2 h-4 w-4' />
                      下载
                    </Button>
                    <Button variant='outline' size='sm' onClick={handleOpenGroupDialog}>
                      <Layers className='mr-2 h-4 w-4' />
                      手动分组
                    </Button>
                    <Button size='sm' onClick={handleOpenSaveDialog}>
                      <Save className='mr-2 h-4 w-4' />
                      保存为订阅
                    </Button>
                  </div>
                </div>
              </CardHeader>
              <CardContent>
                <div className='rounded-lg border bg-muted/30'>
                  <Textarea
                    value={clashConfig}
                    readOnly
                    className='min-h-[400px] resize-none border-0 bg-transparent font-mono text-xs'
                  />
                </div>
                <div className='mt-4 flex justify-end gap-2'>
                  <Button variant='outline' onClick={handleOpenGroupDialog}>
                    <Layers className='mr-2 h-4 w-4' />
                    手动分组
                  </Button>
                  <Button onClick={handleOpenSaveDialog}>
                    <Save className='mr-2 h-4 w-4' />
                    保存为订阅
                  </Button>
                </div>
                <div className='mt-4 rounded-lg border bg-muted/50 p-4'>
                  <h3 className='mb-2 font-semibold'>使用说明</h3>
                  <ul className='space-y-1 text-sm text-muted-foreground'>
                    <li>• 点击"复制"按钮将配置复制到剪贴板</li>
                    <li>• 点击"下载"按钮下载为 clash-config.yaml 文件</li>
                    <li>• 将配置文件导入 Clash 客户端即可使用</li>
                    <li>• 支持 Clash、Clash Meta、Mihomo 等客户端</li>
                  </ul>
                </div>
              </CardContent>
            </Card>
          )}
        </div>
      </main>

      {/* 保存订阅对话框 */}
      <Dialog open={saveDialogOpen} onOpenChange={setSaveDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>保存为订阅</DialogTitle>
            <DialogDescription>
              将生成的配置保存为订阅文件，保存后可以在订阅管理中查看和使用
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label htmlFor='subscribe-name'>
                订阅名称 <span className='text-destructive'>*</span>
              </Label>
              <Input
                id='subscribe-name'
                placeholder='例如：我的订阅'
                value={subscribeName}
                onChange={(e) => setSubscribeName(e.target.value)}
              />
            </div>
            <div className='space-y-2'>
              <Label htmlFor='subscribe-filename'>文件名（可选）</Label>
              <Input
                id='subscribe-filename'
                placeholder='默认使用订阅名称'
                value={subscribeFilename}
                onChange={(e) => setSubscribeFilename(e.target.value)}
              />
              <p className='text-xs text-muted-foreground'>
                文件将保存到 subscribes 目录，自动添加 .yaml 扩展名
              </p>
            </div>
            <div className='space-y-2'>
              <Label htmlFor='subscribe-description'>说明（可选）</Label>
              <Textarea
                id='subscribe-description'
                placeholder='订阅说明...'
                value={subscribeDescription}
                onChange={(e) => setSubscribeDescription(e.target.value)}
                rows={3}
              />
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setSaveDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={handleSaveSubscribe} disabled={saveSubscribeMutation.isPending}>
              {saveSubscribeMutation.isPending && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
              保存
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 手动分组对话框 */}
      <EditNodesDialog
        open={groupDialogOpen}
        onOpenChange={handleGroupDialogOpenChange}
        title="手动分组节点"
        proxyGroups={proxyGroups}
        availableNodes={availableProxies}
        allNodes={savedNodes.filter(n => selectedNodeIds.has(n.id))}
        onProxyGroupsChange={setProxyGroups}
        onSave={handleApplyGrouping}
        onConfigureChainProxy={handleConfigureChainProxy}
        showAllNodes={showAllNodes}
        onShowAllNodesChange={setShowAllNodes}
        draggedNode={draggedItem}
        onDragStart={handleDragStart}
        onDragEnd={handleDragEnd}
        dragOverGroup={dragOverGroup}
        onDragEnterGroup={handleDragEnterGroup}
        onDragLeaveGroup={handleDragLeaveGroup}
        onDrop={handleDrop}
        onDropToAvailable={handleDropToAvailable}
        onRemoveNodeFromGroup={handleRemoveProxy}
        onRemoveGroup={handleRemoveGroup}
        onRenameGroup={handleRenameGroup}
        handleCardDragStart={handleCardDragStart}
        handleCardDragEnd={handleCardDragEnd}
        handleNodeDragEnd={handleNodeDragEnd}
        activeGroupTitle={activeGroupTitle}
        activeCard={activeCard}
        saveButtonText="确定"
      />

      {/* 缺失节点替换对话框 */}
      <Dialog open={missingNodesDialogOpen} onOpenChange={setMissingNodesDialogOpen}>
        <DialogContent className='max-w-md'>
          <DialogHeader>
            <DialogTitle>发现缺失节点</DialogTitle>
            <DialogDescription>
              以下节点在 rules 中被引用，但不存在于 proxy-groups 中
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
                    const parsedConfig = yaml.load(pendingConfigAfterGrouping) as any
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
            <Button variant='outline' onClick={() => setMissingNodesDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={handleApplyReplacement}>
              确认替换
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* 上传模板对话框 */}
      <Dialog open={uploadDialogOpen} onOpenChange={setUploadDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>上传模板</DialogTitle>
            <DialogDescription>
              选择一个 YAML 格式的模板文件上传到 rule_templates 文件夹
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
            <div className='space-y-2'>
              <Label htmlFor='template-file'>模板文件</Label>
              <Input
                id='template-file'
                type='file'
                accept='.yaml,.yml'
                ref={fileInputRef}
              />
              <p className='text-xs text-muted-foreground'>
                支持 .yaml 或 .yml 格式
              </p>
            </div>
          </div>
          <DialogFooter>
            <Button variant='outline' onClick={() => setUploadDialogOpen(false)}>
              取消
            </Button>
            <Button onClick={handleUploadTemplate} disabled={uploadTemplateMutation.isPending}>
              {uploadTemplateMutation.isPending && <Loader2 className='mr-2 h-4 w-4 animate-spin' />}
              上传
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
