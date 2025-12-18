// @ts-nocheck
import { useState, useEffect, useMemo } from 'react'
import { createFileRoute, redirect, useNavigate } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { load as parseYAML, dump as dumpYAML } from 'js-yaml'
import { toast } from 'sonner'
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
import { Progress } from '@/components/ui/progress'
import { Upload, Download, Edit, Settings, FileText, Save, Trash2, RefreshCw, ChevronDown, ChevronUp, ExternalLink } from 'lucide-react'
import { EditNodesDialog } from '@/components/edit-nodes-dialog'
import { MobileEditNodesDialog } from '@/components/mobile-edit-nodes-dialog'
import { Twemoji } from '@/components/twemoji'

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
  created_at: string
  updated_at: string
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
  const { auth } = useAuthStore()
  const queryClient = useQueryClient()
  const navigate = useNavigate()
  const isMobile = useMediaQuery('(max-width: 640px)')

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

  // 编辑节点Dialog状态
  const [editNodesDialogOpen, setEditNodesDialogOpen] = useState(false)
  const [editingNodesFile, setEditingNodesFile] = useState<SubscribeFile | null>(null)
  const [proxyGroups, setProxyGroups] = useState<Array<{ name: string; type: string; proxies: string[] }>>([])
  const [showAllNodes, setShowAllNodes] = useState(true)

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
  })
  const [uploadFile, setUploadFile] = useState<File | null>(null)

  // 编辑元数据表单
  const [metadataForm, setMetadataForm] = useState({
    name: '',
    description: '',
    filename: '',
  })

  // 外部订阅卡片折叠状态 - 默认折叠
  const [isExternalSubsExpanded, setIsExternalSubsExpanded] = useState(false)

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

  // 获取所有节点（用于在外部订阅卡片中显示节点名称）
  const { data: allNodesData } = useQuery({
    queryKey: ['all-nodes-with-tags'],
    queryFn: async () => {
      const response = await api.get('/api/admin/nodes')
      return response.data as { nodes: Array<{ id: number; node_name: string; tag: string }> }
    },
    enabled: Boolean(auth.accessToken && isExternalSubsExpanded),
  })

  // 按 tag 分组的节点名称
  const nodesByTag = useMemo(() => {
    const nodes = allNodesData?.nodes ?? []
    const grouped: Record<string, string[]> = {}
    for (const node of nodes) {
      if (!grouped[node.tag]) {
        grouped[node.tag] = []
      }
      grouped[node.tag].push(node.node_name)
    }
    return grouped
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
      formData.append('name', uploadForm.name || uploadFile.name)
      formData.append('description', uploadForm.description)
      formData.append('filename', uploadForm.filename || uploadFile.name)

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
      toast.success('文件上传成功')
      setUploadDialogOpen(false)
      setUploadForm({ name: '', description: '', filename: '' })
      setUploadFile(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '上传失败')
    },
  })

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
      setMetadataForm({ name: '', description: '', filename: '' })
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
      return response.data as { nodes: Array<{ id: number; node_name: string }> }
    },
    enabled: Boolean(editNodesDialogOpen && auth.accessToken),
    refetchOnWindowFocus: false,
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

  const toggleAutoSyncMutation = useMutation({
    mutationFn: async (payload: { id: number; enabled: boolean }) => {
      const response = await api.patch(`/api/admin/subscribe-files/${payload.id}`, {
        auto_sync_custom_rules: payload.enabled,
      })
      return response.data
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['subscribe-files'] })
      toast.success('自动同步设置已更新')
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
        }))
        setProxyGroups(groups)
      }
    } catch (error) {
      console.error('解析YAML失败:', error)
      toast.error('解析配置文件失败')
    }
  }, [nodesConfigQuery.data])

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
    uploadMutation.mutate()
  }

  const handleDelete = (id: number) => {
    deleteMutation.mutate(id)
  }

  const handleEditMetadata = (file: SubscribeFile) => {
    setEditingMetadata(file)
    setMetadataForm({
      name: file.name,
      description: file.description,
      filename: file.filename,
    })
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
      data: metadataForm,
    })
  }

  const handleEditConfig = (file: SubscribeFile) => {
    setEditingConfigFile(file)
    setEditConfigDialogOpen(true)
  }

  const handleSaveConfig = () => {
    if (!editingConfigFile) return
    try {
      parseYAML(configContent || '')
    } catch (error) {
      const message = error instanceof Error ? error.message : 'YAML 解析失败'
      toast.error('保存失败，YAML 格式错误：' + message)
      return
    }
    saveConfigMutation.mutate({ filename: editingConfigFile.filename, content: configContent })
  }

  const handleToggleAutoSync = (id: number, enabled: boolean) => {
    toggleAutoSyncMutation.mutate({ id, enabled })
  }

  const handleEditNodes = (file: SubscribeFile) => {
    setEditingNodesFile(file)
    setEditNodesDialogOpen(true)
    setShowAllNodes(false)
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
        return
      }

      // 如果节点名称不在 proxy-groups 中，添加到缺失列表
      if (nodeName && !proxyGroupNames.has(nodeName)) {
        console.log(`[validateRulesNodes] 发现缺失节点: "${nodeName}"`)
        missingNodes.add(nodeName)
      }
    })

    return {
      missingNodes: Array.from(missingNodes)
    }
  }

  // 应用缺失节点替换
  const handleApplyReplacement = () => {
    try {
      const parsedConfig = parseYAML(pendingConfigAfterSave) as any
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

      // 转换回YAML
      const finalConfig = dumpYAML(parsedConfig, { lineWidth: -1, noRefs: true })
      setConfigContent(finalConfig)

      // 更新查询缓存
      queryClient.setQueryData(['nodes-config', editingNodesFile?.id], {
        content: finalConfig
      })

      // 只关闭替换对话框，不关闭编辑节点对话框
      setMissingNodesDialogOpen(false)
      toast.success(`已将缺失节点替换为 ${replacementChoice}`)
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
      const parsed = parseYAML(currentContent) as any

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
        nodesQuery.data.nodes.forEach((node: any) => {
          if (usedNodeNames.has(node.node_name) && node.clash_config) {
            try {
              const clashConfig = typeof node.clash_config === 'string'
                ? JSON.parse(node.clash_config)
                : node.clash_config
              // 重新排序属性，确保 name, type, server, port 在前4位
              const orderedConfig = reorderProxyProperties(clashConfig)
              nodeConfigs.push(orderedConfig)
            } catch (e) {
              console.error(`解析节点 ${node.node_name} 的配置失败:`, e)
            }
          }
        })

        // 更新proxies部分
        if (nodeConfigs.length > 0) {
          // 保留现有的proxies中不在usedNodeNames中的节点
          const existingProxies = parsed.proxies || []

          // 合并：使用新的节点配置，添加现有但未使用的节点
          const updatedProxies = [...nodeConfigs]

          // 添加现有但未使用的节点（也重新排序）
          existingProxies.forEach((proxy: any) => {
            if (!usedNodeNames.has(proxy.name) && !updatedProxies.some(p => p.name === proxy.name)) {
              updatedProxies.push(reorderProxyProperties(proxy))
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

      // 处理链式代理：给落地节点组中的节点添加 dialer-proxy 参数
      const landingGroup = proxyGroups.find(g => g.name === '🌄 落地节点')
      const hasRelayGroup = proxyGroups.some(g => g.name === '🌠 中转节点')

      if (landingGroup && hasRelayGroup && parsed.proxies && Array.isArray(parsed.proxies)) {
        // 获取落地节点组中的所有节点名称
        const landingNodeNames = new Set(landingGroup.proxies.filter((p): p is string => p !== undefined))

        // 创建节点名称到协议的映射（用于判断是否已是链式代理节点）
        const nodeProtocolMap = new Map<string, string>()
        if (nodesQuery.data?.nodes) {
          nodesQuery.data.nodes.forEach((node: any) => {
            nodeProtocolMap.set(node.node_name, node.protocol)
          })
        }

        // 给这些节点添加 dialer-proxy 参数（跳过已经是链式代理的节点）
        parsed.proxies = parsed.proxies.map((proxy: any) => {
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

      // 更新代理组
      if (parsed && parsed['proxy-groups']) {
        // 保留代理组的所有原始属性，只更新 proxies
        parsed['proxy-groups'] = proxyGroups.map(group => ({
          ...group, // 保留所有原始属性（如 url, interval, strategy 等）
          proxies: group.proxies, // 更新 proxies
        }))
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
        // 没有缺失节点，直接应用
        // 更新编辑配置对话框中的内容
        setConfigContent(newContent)
        // 只关闭编辑节点对话框，不保存到文件
        setEditNodesDialogOpen(false)
        toast.success('已应用节点配置')
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
          queryClient.setQueryData(['nodes-config', editingNodesFile?.id], {
            content: newContent
          })
        }
      } catch (error) {
        console.error('更新配置文件中的代理组引用失败:', error)
      }
    }
  }

  // 计算可用节点
  const availableNodes = useMemo(() => {
    if (!nodesQuery.data?.nodes) return []

    const allNodeNames = nodesQuery.data.nodes.map(n => n.node_name)

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
  }, [nodesQuery.data, proxyGroups, showAllNodes])

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

          <div className='flex gap-1 sm:gap-2 md:justify-start'>
            {/* 导入订阅 */}
            <Dialog open={importDialogOpen} onOpenChange={setImportDialogOpen}>
              <DialogTrigger asChild>
                <Button variant='outline' className='flex-1 md:flex-none text-xs sm:text-sm px-1.5 py-2 sm:px-4 sm:py-2'>
                  <Download className='mr-0.5 sm:mr-2 h-3.5 w-3.5 sm:h-4 sm:w-4 shrink-0' />
                  <span className='truncate'>导入订阅</span>
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>导入订阅</DialogTitle>
                  <DialogDescription>
                    从 Clash 订阅链接导入，系统会自动下载并保存文件
                  </DialogDescription>
                </DialogHeader>
                <div className='space-y-4 py-4'>
                  <div className='space-y-2'>
                    <Label htmlFor='import-name'>订阅名称 *</Label>
                    <Input
                      id='import-name'
                      placeholder='例如：机场A'
                      value={importForm.name}
                      onChange={(e) => setImportForm({ ...importForm, name: e.target.value })}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='import-url'>订阅链接 *</Label>
                    <Input
                      id='import-url'
                      placeholder='https://example.com/subscribe?token=xxx'
                      value={importForm.url}
                      onChange={(e) => setImportForm({ ...importForm, url: e.target.value })}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='import-filename'>文件名（可选）</Label>
                    <Input
                      id='import-filename'
                      placeholder='留空则自动获取'
                      value={importForm.filename}
                      onChange={(e) => setImportForm({ ...importForm, filename: e.target.value })}
                    />
                  </div>
                  <div className='space-y-2'>
                    <Label htmlFor='import-description'>说明（可选）</Label>
                    <Textarea
                      id='import-description'
                      placeholder='订阅说明信息'
                      value={importForm.description}
                      onChange={(e) => setImportForm({ ...importForm, description: e.target.value })}
                    />
                  </div>
                </div>
                <DialogFooter>
                  <Button variant='outline' onClick={() => setImportDialogOpen(false)}>
                    取消
                  </Button>
                  <Button onClick={handleImport} disabled={importMutation.isPending}>
                    {importMutation.isPending ? '导入中...' : '导入'}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            {/* 上传文件 */}
            <Dialog open={uploadDialogOpen} onOpenChange={setUploadDialogOpen}>
              <DialogTrigger asChild>
                <Button variant='outline' className='flex-1 md:flex-none text-xs sm:text-sm px-1.5 py-2 sm:px-4 sm:py-2'>
                  <Upload className='mr-0.5 sm:mr-2 h-3.5 w-3.5 sm:h-4 sm:w-4 shrink-0' />
                  <span className='truncate'>上传文件</span>
                </Button>
              </DialogTrigger>
              <DialogContent>
                <DialogHeader>
                  <DialogTitle>上传文件</DialogTitle>
                  <DialogDescription>
                    上传本地 YAML 格式的 Clash 订阅文件
                  </DialogDescription>
                </DialogHeader>
                <div className='space-y-4 py-4'>
                  <div className='space-y-2'>
                    <Label htmlFor='upload-file'>选择文件 *</Label>
                    <Input
                      id='upload-file'
                      type='file'
                      accept='.yaml,.yml'
                      onChange={(e) => setUploadFile(e.target.files?.[0] || null)}
                    />
                  </div>
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
                </div>
                <DialogFooter>
                  <Button variant='outline' onClick={() => setUploadDialogOpen(false)}>
                    取消
                  </Button>
                  <Button onClick={handleUpload} disabled={uploadMutation.isPending}>
                    {uploadMutation.isPending ? '上传中...' : '上传'}
                  </Button>
                </DialogFooter>
              </DialogContent>
            </Dialog>

            {/* 生成订阅 */}
            <Button variant='outline' className='flex-1 md:flex-none text-xs sm:text-sm px-1.5 py-2 sm:px-4 sm:py-2' onClick={() => navigate({ to: '/generator' })}>
              <FileText className='mr-0.5 sm:mr-2 h-3.5 w-3.5 sm:h-4 sm:w-4 shrink-0' />
              <span className='truncate'>生成订阅</span>
            </Button>

            {/* 自定义代理组 - 保留入口 */}
            {/* <Link to='/subscribe-files/custom'>
              <Button>
                <Plus className='mr-2 h-4 w-4' />
                自定义代理组
              </Button>
            </Link> */}
          </div>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>订阅列表 ({files.length})</CardTitle>
            <CardDescription>已添加的订阅文件</CardDescription>
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
                      <div className='flex items-center gap-2 flex-wrap'>
                        <Badge variant='outline' className={TYPE_COLORS[file.type]}>
                          {TYPE_LABELS[file.type]}
                        </Badge>
                        <span className='font-medium'>{file.name}</span>
                        {file.latest_version && (
                          <Badge variant='secondary'>v{file.latest_version}</Badge>
                        )}
                      </div>
                    ),
                  },
                  {
                    header: '说明',
                    cell: (file) => file.description ? (
                      <Tooltip>
                        <TooltipTrigger asChild>
                          <span className='text-sm text-muted-foreground truncate block cursor-help'>
                            {file.description}
                          </span>
                        </TooltipTrigger>
                        <TooltipContent className='max-w-xs'>
                          {file.description}
                        </TooltipContent>
                      </Tooltip>
                    ) : (
                      <span className='text-sm text-muted-foreground'>-</span>
                    ),
                    cellClassName: 'max-w-[200px]'
                  },
                  {
                    header: '最后更新',
                    cell: (file) => (
                      <span className='text-sm text-muted-foreground whitespace-nowrap'>
                        {file.updated_at ? dateFormatter.format(new Date(file.updated_at)) : '-'}
                      </span>
                    ),
                    width: '160px'
                  },
                  {
                    header: '自动同步',
                    cell: (file) => (
                      <Switch
                        checked={file.auto_sync_custom_rules || false}
                        onCheckedChange={(checked) => handleToggleAutoSync(file.id, checked)}
                      />
                    ),
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '90px'
                  },
                  {
                    header: '操作',
                    cell: (file) => (
                      <div className='flex items-center gap-1'>
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
                    ),
                    headerClassName: 'text-center',
                    cellClassName: 'text-center',
                    width: '120px'
                  }
                ] as DataTableColumn<SubscribeFile>[]}

                mobileCard={{
                  header: (file) => (
                    <div className='flex items-center justify-between gap-2 mb-1'>
                      <div className='flex items-center gap-2 flex-1 min-w-0'>
                        <Badge variant='outline' className={TYPE_COLORS[file.type]}>
                          {TYPE_LABELS[file.type]}
                        </Badge>
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
                      label: '自动同步',
                      value: (file) => (
                        <div className='flex items-center gap-2'>
                          <Switch
                            checked={file.auto_sync_custom_rules || false}
                            onCheckedChange={(checked) => handleToggleAutoSync(file.id, checked)}
                          />
                          <span className='text-xs'>{file.auto_sync_custom_rules ? '已启用' : '未启用'}</span>
                        </div>
                      )
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
                    <CardDescription>管理已添加的外部订阅源，用于从第三方订阅同步节点</CardDescription>
                  </div>
                  {isExternalSubsExpanded ? <ChevronUp className='h-5 w-5' /> : <ChevronDown className='h-5 w-5' />}
                </div>
              </CardHeader>
            </CollapsibleTrigger>
            <CollapsibleContent className='CollapsibleContent'>
              <CardContent>
              {/* 同步按钮 */}
              <div className='flex justify-end mb-4'>
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
                      cell: (sub) => sub.name,
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
                        const used = sub.upload + sub.download
                        const percentage = Math.min((used / sub.total) * 100, 100)
                        const remaining = Math.max(sub.total - used, 0)
                        return (
                          <Tooltip>
                            <TooltipTrigger asChild>
                              <div className='w-24 space-y-1 cursor-help'>
                                <Progress value={percentage} className='h-2' />
                                <div className='text-xs text-center text-muted-foreground'>
                                  {percentage.toFixed(0)}%
                                </div>
                              </div>
                            </TooltipTrigger>
                            <TooltipContent className='space-y-1'>
                              <div className='text-xs'>
                                <span className='font-medium'>已用: </span>
                                {formatTrafficGB(used)}
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
                                上传: {formatTrafficGB(sub.upload)} / 下载: {formatTrafficGB(sub.download)}
                              </div>
                            </TooltipContent>
                          </Tooltip>
                        )
                      },
                      width: '120px'
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
                      width: '100px'
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
                        </div>
                        <div className='flex items-center gap-1'>
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
                          const used = sub.upload + sub.download
                          const percentage = Math.min((used / sub.total) * 100, 100)
                          const remaining = Math.max(sub.total - used, 0)
                          return (
                            <Tooltip>
                              <TooltipTrigger asChild>
                                <div className='flex items-center gap-2 cursor-help'>
                                  <Progress value={percentage} className='h-2 flex-1 max-w-24' />
                                  <span className='text-xs whitespace-nowrap'>
                                    {formatTrafficGB(used)} / {formatTrafficGB(sub.total)}
                                  </span>
                                </div>
                              </TooltipTrigger>
                              <TooltipContent className='space-y-1'>
                                <div className='text-xs'>
                                  <span className='font-medium'>已用: </span>
                                  {formatTrafficGB(used)} ({percentage.toFixed(1)}%)
                                </div>
                                <div className='text-xs'>
                                  <span className='font-medium'>剩余: </span>
                                  {formatTrafficGB(remaining)}
                                </div>
                                <div className='text-xs text-muted-foreground'>
                                  上传: {formatTrafficGB(sub.upload)} / 下载: {formatTrafficGB(sub.download)}
                                </div>
                              </TooltipContent>
                            </Tooltip>
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
          setMetadataForm({ name: '', description: '', filename: '' })
        }
      }}>
        <DialogContent className='sm:max-w-lg'>
          <DialogHeader>
            <DialogTitle>编辑订阅信息</DialogTitle>
            <DialogDescription>
              修改订阅名称、说明和文件名
            </DialogDescription>
          </DialogHeader>
          <div className='space-y-4 py-4'>
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
          </div>
          <DialogFooter>
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
        <DialogContent className='!max-w-[80vw] w-[80vw] max-h-[90vh] flex flex-col'>
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
        />
      )}

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
    </main>
  )
}
