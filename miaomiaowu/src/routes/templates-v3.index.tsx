// @ts-nocheck
import { useState, useEffect, useCallback, useRef } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, Eye, Upload, Save, X, Star, Globe2 } from 'lucide-react'

import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { useMediaQuery } from '@/hooks/use-media-query'
import { cn } from '@/lib/utils'

const TEMPLATE_DRAFT_KEY_PREFIX = 'mmw_template_v3_draft_'

import { DataTable } from '@/components/data-table'
import type { DataTableColumn } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle, DialogFooter } from '@/components/ui/dialog'
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from '@/components/ui/alert-dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { ScrollArea } from '@/components/ui/scroll-area'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { Badge } from '@/components/ui/badge'

import { ProxyGroupEditor } from '@/components/template-v3/proxy-group-editor'
import { TemplatePreview } from '@/components/template-v3/template-preview'
import { TemplateUploadDialog } from '@/components/template-v3/template-upload-dialog'
import { Switch } from '@/components/ui/switch'
import { Label } from '@/components/ui/label'
import {
  extractProxyGroups,
  extractTemplateVariables,
  updateProxyGroups,
  createDefaultFormState,
  parseTemplate,
  generateProxyGroupsPreview,
  generateRegionProxyGroups,
  getRegionProxyGroupNames,
  PROXY_NODES_MARKER,
  PROXY_PROVIDERS_MARKER,
  REGION_PROXY_GROUPS_MARKER,
  PROXY_NODES_DISPLAY,
  PROXY_PROVIDERS_DISPLAY,
  REGION_PROXY_GROUPS_DISPLAY,
  type ProxyGroupFormState,
} from '@/lib/template-v3-utils'

export const Route = createFileRoute('/templates-v3/')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: TemplatesV3Page,
})

function TemplatesV3Page() {
  const queryClient = useQueryClient()
  const isMobile = useMediaQuery('(max-width: 767px)')
  const isTablet = useMediaQuery('(min-width: 768px) and (max-width: 1024px)')
  const isDesktop = useMediaQuery('(min-width: 1025px)')

  // Dialog states
  const [isEditorOpen, setIsEditorOpen] = useState(false)
  const [isUploadDialogOpen, setIsUploadDialogOpen] = useState(false)
  const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
  const [isRenameDialogOpen, setIsRenameDialogOpen] = useState(false)
  const [isCloseConfirmOpen, setIsCloseConfirmOpen] = useState(false)
  const [isDraftRecoveryOpen, setIsDraftRecoveryOpen] = useState(false)

  // Editing state
  const [editingTemplateName, setEditingTemplateName] = useState<string | null>(null)
	const isSurgeTemplate = editingTemplateName?.toLowerCase().endsWith('.conf') ?? false
	const isLoonTemplate = editingTemplateName?.toLowerCase().endsWith('.lcf') ?? false
	// Surge / Loon 模板都是纯文本段落式配置,只走文本编辑,不进可视化解析。
	const isTextTemplate = isSurgeTemplate || isLoonTemplate
  const [templateContent, setTemplateContent] = useState('')
  const [proxyGroups, setProxyGroups] = useState<ProxyGroupFormState[]>([])
  const [editorTab, setEditorTab] = useState<'visual' | 'yaml'>('visual')
  const [isDirty, setIsDirty] = useState(false)
  const isInitLoadRef = useRef(false)
  const pendingDraftRef = useRef<any>(null)
  const [enableRegionProxyGroups, setEnableRegionProxyGroups] = useState(false)
  const [templateVariables, setTemplateVariables] = useState<Record<string, string>>({})

  // Delete/Rename state
  const [deletingTemplateName, setDeletingTemplateName] = useState<string | null>(null)
  const [renamingTemplate, setRenamingTemplate] = useState<string | null>(null)
  const [newTemplateName, setNewTemplateName] = useState('')

  // Preview state
  const [previewContent, setPreviewContent] = useState('')
  const [isPreviewLoading, setIsPreviewLoading] = useState(false)
  const [isPreviewOpen, setIsPreviewOpen] = useState(false)

  // List preview state (for eye button in table)
  const [listPreviewOpen, setListPreviewOpen] = useState(false)
  const [listPreviewContent, setListPreviewContent] = useState('')
  const [listPreviewLoading, setListPreviewLoading] = useState(false)
  const [listPreviewTemplateName, setListPreviewTemplateName] = useState<string | null>(null)
  const [listPreviewTemplateContent, setListPreviewTemplateContent] = useState('')

  // Fetch templates list
  const { data: templateListData, isLoading } = useQuery<{ templates: string[]; visibility: Record<string, boolean> }>({
    queryKey: ['rule-templates'],
    queryFn: async () => {
      const response = await api.get('/api/admin/rule-templates')
	  return response.data
	},
  })
	const templates = templateListData?.templates ?? []
	const visibility = templateListData?.visibility ?? {}
	const { data: defaultTemplates } = useQuery({ queryKey: ['user-default-template'], queryFn: async () => (await api.get('/api/user/default-template')).data })
	const { data: profile } = useQuery({ queryKey: ['user-profile'], queryFn: async () => (await api.get('/api/user/profile')).data })
	const defaultMutation = useMutation({
	  mutationFn: async (name: string) => {
		const lower = name.toLowerCase()
		const isConf = lower.endsWith('.conf')
		const isLcf = lower.endsWith('.lcf')
		return api.put('/api/user/default-template', {
		  default_template_filename: isConf || isLcf ? (defaultTemplates?.default_template_filename ?? '') : name,
		  default_surge_template_filename: isConf ? name : (defaultTemplates?.default_surge_template_filename ?? ''),
		  default_loon_template_filename: isLcf ? name : (defaultTemplates?.default_loon_template_filename ?? ''),
		})
	  },
	  onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['user-default-template'] }); toast.success('默认模板已更新') },
	  onError: (error: any) => toast.error(error.response?.data?.error || '设置默认模板失败'),
	})
	const visibilityMutation = useMutation({
	  mutationFn: async (name: string) => api.put('/api/admin/rule-templates/visibility', { filename: name, public: !visibility[name] }),
	  onSuccess: () => { queryClient.invalidateQueries({ queryKey: ['rule-templates'] }); toast.success('模板可见性已更新') },
	  onError: (error: any) => toast.error(error.response?.data || '更新可见性失败'),
	})

  // Fetch template content when editing
  const { data: templateData } = useQuery({
    queryKey: ['rule-template', editingTemplateName],
    queryFn: async () => {
      const response = await api.get(`/api/admin/rule-templates/${encodeURIComponent(editingTemplateName!)}`)
      return response.data.content as string
    },
    enabled: !!editingTemplateName && isEditorOpen,
  })

  // Fetch nodes for preview
  const { data: nodesData } = useQuery({
    queryKey: ['nodes-for-preview'],
    queryFn: async () => {
      const response = await api.get('/api/admin/nodes')
      const nodes = response.data.nodes || []
      // Convert nodes to Clash format by parsing clash_config
      return nodes.map((node: any) => {
        if (node.clash_config) {
          try {
            return JSON.parse(node.clash_config)
          } catch {
            return { name: node.node_name, type: node.protocol }
          }
        }
        return { name: node.node_name, type: node.protocol }
      }).filter((n: any) => n.name && n.type)
    },
    enabled: isEditorOpen,
  })

  // Update template mutation
  const updateMutation = useMutation({
    mutationFn: async ({ name, content }: { name: string; content: string }) => {
      await api.put(`/api/admin/rule-templates/${encodeURIComponent(name)}`, { content })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      queryClient.invalidateQueries({ queryKey: ['rule-template', editingTemplateName] })
      if (editingTemplateName) {
        localStorage.removeItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName)
      }
      toast.success('模板保存成功')
      setIsDirty(false)
      // Close editor after successful save
      setIsEditorOpen(false)
      setEditingTemplateName(null)
      setTemplateContent('')
      setProxyGroups([])
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '保存失败')
    },
  })

  // Delete template mutation
  const deleteMutation = useMutation({
    mutationFn: async (name: string) => {
      await api.delete(`/api/admin/rule-templates/${encodeURIComponent(name)}`)
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板已删除')
      setIsDeleteDialogOpen(false)
      setDeletingTemplateName(null)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '删除失败')
    },
  })

  // Upload template mutation
  const uploadMutation = useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData()
      formData.append('template', file)
      await api.post('/api/admin/rule-templates/upload', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板上传成功')
      setIsUploadDialogOpen(false)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '上传失败')
    },
  })

  // Create template mutation (for paste/blank)
  const createMutation = useMutation({
    mutationFn: async ({ name, content }: { name: string; content: string }) => {
      const formData = new FormData()
      const blob = new Blob([content], { type: 'text/yaml' })
      formData.append('template', blob, name)
      await api.post('/api/admin/rule-templates/upload', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板创建成功')
      setIsUploadDialogOpen(false)
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '创建失败')
    },
  })

  // Rename template mutation
  const renameMutation = useMutation({
    mutationFn: async ({ oldName, newName }: { oldName: string; newName: string }) => {
      await api.post('/api/admin/rule-templates/rename', { old_name: oldName, new_name: newName })
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ['rule-templates'] })
      toast.success('模板重命名成功')
      setIsRenameDialogOpen(false)
      setRenamingTemplate(null)
      setNewTemplateName('')
    },
    onError: (error: any) => {
      toast.error(error.response?.data?.error || '重命名失败')
    },
  })

  // Load template content when data is fetched
  useEffect(() => {
    if (templateData && isEditorOpen) {
      isInitLoadRef.current = true
      setTemplateContent(templateData)
	  if (isTextTemplate) {
		setEditorTab('yaml')
		setTemplateVariables({})
		setProxyGroups([])
		setEnableRegionProxyGroups(false)
		setIsDirty(false)
		isInitLoadRef.current = false
		return
	  }
      const vars = extractTemplateVariables(templateData)
      setTemplateVariables(vars)
      const groups = extractProxyGroups(templateData, vars)
      setProxyGroups(groups)
      // Auto-enable region proxy groups toggle if any group has includeRegionProxyGroups
      const hasRegionProxyGroups = groups.some(g => g.includeRegionProxyGroups)
      setEnableRegionProxyGroups(hasRegionProxyGroups)
      setIsDirty(false)
      // Allow ProxyGroupSelect's ensureMarkers setTimeout to finish before enabling dirty tracking
      setTimeout(() => {
        isInitLoadRef.current = false
        // Check for local draft
        if (editingTemplateName) {
          const draftJson = localStorage.getItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName)
          if (draftJson) {
            try {
              const draft = JSON.parse(draftJson)
              // Normalize templateData the same way draft was saved
              const vars = extractTemplateVariables(templateData)
              const groups = extractProxyGroups(templateData, vars)
              const normalizedData = groups.length > 0 ? updateProxyGroups(templateData, groups) : templateData
              if (draft.templateContent !== normalizedData) {
                pendingDraftRef.current = draft
                setIsDraftRecoveryOpen(true)
              } else {
                localStorage.removeItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName)
              }
            } catch {
              localStorage.removeItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName)
            }
          }
        }
      }, 50)
    }
  }, [templateData, isEditorOpen, isTextTemplate])

  // Auto-refresh proxy-groups preview when proxyGroups changes
  useEffect(() => {
    if (!isEditorOpen) return

    // Generate proxy-groups YAML preview locally (no API call needed)
    if (proxyGroups.length > 0) {
      const preview = generateProxyGroupsPreview(proxyGroups)
      setPreviewContent(preview)
    } else {
      setPreviewContent('')
    }
  }, [proxyGroups, isEditorOpen])

  // Write draft to localStorage when dirty
  useEffect(() => {
    if (!isDirty || !editingTemplateName || isInitLoadRef.current) return
    let content = templateContent
    if (editorTab === 'visual' && proxyGroups.length > 0) {
      content = updateProxyGroups(templateContent, proxyGroups)
    }
    const draft = {
      templateContent: content,
      proxyGroups,
      enableRegionProxyGroups,
      templateVariables,
      editorTab,
      savedAt: Date.now(),
    }
    localStorage.setItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName, JSON.stringify(draft))
  }, [isDirty, templateContent, proxyGroups, enableRegionProxyGroups, templateVariables, editorTab, editingTemplateName])

  // Sync proxy groups to YAML when switching tabs
  const syncProxyGroupsToYaml = useCallback(() => {
    if (proxyGroups.length > 0) {
      const newContent = updateProxyGroups(templateContent, proxyGroups)
      setTemplateContent(newContent)
    }
  }, [proxyGroups, templateContent])

  // Handle tab change
  const handleTabChange = (tab: string) => {
    if (editorTab === 'visual' && tab === 'yaml') {
      syncProxyGroupsToYaml()
    } else if (editorTab === 'yaml' && tab === 'visual') {
      const vars = extractTemplateVariables(templateContent)
      setTemplateVariables(vars)
      setProxyGroups(extractProxyGroups(templateContent, vars))
    }
    setEditorTab(tab as 'visual' | 'yaml')
  }

  // Handle edit
  const handleEdit = (name: string) => {
    setEditingTemplateName(name)
    setIsEditorOpen(true)
    setEditorTab('visual')
    setPreviewContent('')
  }

  // Handle delete
  const handleDelete = (name: string) => {
    setDeletingTemplateName(name)
    setIsDeleteDialogOpen(true)
  }

  // Handle rename
  const handleRename = (name: string) => {
    setRenamingTemplate(name)
    setNewTemplateName(name)
    setIsRenameDialogOpen(true)
  }

  // Handle list preview (eye button in table)
  const handleListPreview = async (name: string) => {
    setListPreviewTemplateName(name)
    setListPreviewOpen(true)
    setListPreviewLoading(true)
    setListPreviewContent('')
    setListPreviewTemplateContent('')

    try {
      // Fetch template content
      const templateResponse = await api.get(`/api/admin/rule-templates/${encodeURIComponent(name)}`)
      const content = templateResponse.data.content
      setListPreviewTemplateContent(content)

      // Fetch nodes for preview
      const nodesResponse = await api.get('/api/admin/nodes')
      const nodes = (nodesResponse.data.nodes || []).map((node: any) => {
        if (node.clash_config) {
          try {
            return JSON.parse(node.clash_config)
          } catch {
            return { name: node.node_name, type: node.protocol }
          }
        }
        return { name: node.node_name, type: node.protocol }
      }).filter((n: any) => n.name && n.type)

      // Generate preview
      const previewResponse = await api.post('/api/admin/template-v3/preview', {
        template_content: content,
        proxies: nodes,
      })
      setListPreviewContent(previewResponse.data.content)
    } catch (error: any) {
      toast.error(error.response?.data?.error || '预览生成失败')
      setListPreviewOpen(false)
    } finally {
      setListPreviewLoading(false)
    }
  }

  // Handle save
  const handleSave = () => {
    if (!editingTemplateName) return
    let content = templateContent
    if (editorTab === 'visual') {
      content = updateProxyGroups(templateContent, proxyGroups)
    }
    updateMutation.mutate({ name: editingTemplateName, content })
  }

  // Handle close editor
  const handleCloseEditor = () => {
    if (isDirty) {
      setIsCloseConfirmOpen(true)
      return
    }
    doCloseEditor()
  }

  const doCloseEditor = () => {
    setIsEditorOpen(false)
    setEditingTemplateName(null)
    setTemplateContent('')
    setProxyGroups([])
    setPreviewContent('')
    setIsDirty(false)
    setIsCloseConfirmOpen(false)
    setEnableRegionProxyGroups(false)
  }

  const handleRecoverDraft = () => {
    const draft = pendingDraftRef.current
    if (!draft) return
    isInitLoadRef.current = true
    setTemplateContent(draft.templateContent)
    setProxyGroups(draft.proxyGroups)
    setEnableRegionProxyGroups(draft.enableRegionProxyGroups)
    setTemplateVariables(draft.templateVariables)
    setEditorTab(draft.editorTab)
    setIsDirty(true)
    setTimeout(() => { isInitLoadRef.current = false }, 50)
    setIsDraftRecoveryOpen(false)
    pendingDraftRef.current = null
  }

  const handleDiscardDraft = () => {
    if (editingTemplateName) {
      localStorage.removeItem(TEMPLATE_DRAFT_KEY_PREFIX + editingTemplateName)
    }
    setIsDraftRecoveryOpen(false)
    pendingDraftRef.current = null
  }

  // Region proxy group names for checking
  const regionGroupNames = getRegionProxyGroupNames()

  // Handle region proxy groups toggle
  const handleRegionProxyGroupsToggle = (enabled: boolean) => {
    setEnableRegionProxyGroups(enabled)
    setIsDirty(true)

    if (enabled) {
      // Add region proxy groups at the end
      const regionGroups = generateRegionProxyGroups('url-test')
      // Filter out any existing region groups to avoid duplicates
      const nonRegionGroups = proxyGroups.filter(g => !regionGroupNames.includes(g.name))
      setProxyGroups([...nonRegionGroups, ...regionGroups])
    } else {
      // Remove region proxy groups and clear includeRegionProxyGroups from all groups
      const updatedGroups = proxyGroups
        .filter(g => !regionGroupNames.includes(g.name))
        .map(g => ({
          ...g,
          includeRegionProxyGroups: false,
          // Remove REGION_PROXY_GROUPS_MARKER from proxyOrder
          proxyOrder: g.proxyOrder.filter(item => item !== REGION_PROXY_GROUPS_MARKER),
        }))
      setProxyGroups(updatedGroups)
    }
  }

  // Handle proxy group change
  const handleProxyGroupChange = (index: number, group: ProxyGroupFormState) => {
    const newGroups = [...proxyGroups]
    newGroups[index] = group
    setProxyGroups(newGroups)
    if (!isInitLoadRef.current) {
      setIsDirty(true)
    }
  }

  // Handle proxy group delete
  const handleProxyGroupDelete = (index: number) => {
    setProxyGroups(proxyGroups.filter((_, i) => i !== index))
    setIsDirty(true)
  }

  // Handle proxy group move
  const handleProxyGroupMoveUp = (index: number) => {
    if (index === 0) return
    const newGroups = [...proxyGroups]
    ;[newGroups[index - 1], newGroups[index]] = [newGroups[index], newGroups[index - 1]]
    setProxyGroups(newGroups)
    setIsDirty(true)
  }

  const handleProxyGroupMoveDown = (index: number) => {
    if (index === proxyGroups.length - 1) return
    const newGroups = [...proxyGroups]
    ;[newGroups[index], newGroups[index + 1]] = [newGroups[index + 1], newGroups[index]]
    setProxyGroups(newGroups)
    setIsDirty(true)
  }

  // Handle add proxy group
  const handleAddProxyGroup = () => {
    setProxyGroups([...proxyGroups, createDefaultFormState(`新代理组 ${proxyGroups.length + 1}`)])
    setIsDirty(true)
  }

  // Handle preview
  const handlePreview = async () => {
    setIsPreviewLoading(true)
    try {
      let content = templateContent
      if (editorTab === 'visual') {
        content = updateProxyGroups(templateContent, proxyGroups)
      }
      const response = await api.post('/api/admin/template-v3/preview', {
        template_content: content,
        proxies: nodesData || [],
      })
      setPreviewContent(response.data.content)
    } catch (error: any) {
      toast.error(error.response?.data?.error || '预览生成失败')
    } finally {
      setIsPreviewLoading(false)
    }
  }

  // Handle YAML content change
  const handleYamlChange = (value: string) => {
    setTemplateContent(value)
    setIsDirty(true)
  }

  // Replace markers with Chinese display names for preview
  const formatTemplateForDisplay = (content: string) => {
    return content
      .replace(new RegExp(PROXY_NODES_MARKER, 'g'), PROXY_NODES_DISPLAY)
      .replace(new RegExp(PROXY_PROVIDERS_MARKER, 'g'), PROXY_PROVIDERS_DISPLAY)
      .replace(new RegExp(REGION_PROXY_GROUPS_MARKER, 'g'), REGION_PROXY_GROUPS_DISPLAY)
  }

  // Table columns
  const columns: DataTableColumn<string>[] = [
    {
      header: '模板名称',
		cell: (name) => <span className="flex items-center gap-2 font-medium">{name}{(name === defaultTemplates?.default_template_filename || name === defaultTemplates?.default_surge_template_filename || name === defaultTemplates?.default_loon_template_filename) && <Badge>默认</Badge>}</span>,
    },
    {
      header: '操作',
      cell: (name) => (
        <div className="flex items-center gap-1">
          <Button variant="ghost" size="icon" onClick={() => handleEdit(name)} title="编辑">
            <Pencil className="h-4 w-4" />
          </Button>
		  <Button variant="ghost" size="icon" onClick={() => handleListPreview(name)} title="预览">
            <Eye className="h-4 w-4" />
		  </Button>
		  <Button variant="ghost" size="icon" onClick={() => defaultMutation.mutate(name)} title="设为个人默认"><Star className="h-4 w-4" /></Button>
		  {profile?.role === 'admin' && <Button variant="ghost" size="icon" onClick={() => visibilityMutation.mutate(name)} title={visibility[name] ? '设为私有' : '公开给普通用户'}><Globe2 className={cn('h-4 w-4', visibility[name] && 'text-primary')} /></Button>}
          <Button variant="ghost" size="icon" onClick={() => handleDelete(name)} title="删除">
            <Trash2 className="h-4 w-4 text-destructive" />
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div className="min-h-svh bg-background">
      <main className="mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24">
      <Card>
        <CardHeader className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <CardTitle>模板管理</CardTitle>
            <CardDescription>
              管理 Clash/Mihomo、Surge 与 Loon 模板，并设置个人默认模板
            </CardDescription>
          </div>
          <Button onClick={() => setIsUploadDialogOpen(true)} className="w-full sm:w-auto">
            <Plus className="h-4 w-4 mr-2" />
            新建模板
          </Button>
        </CardHeader>
        <CardContent>
          <div className="mb-4 grid gap-2 rounded-lg border bg-muted/30 p-3 text-sm sm:grid-cols-2">
            <div><span className="text-muted-foreground">个人默认 Clash：</span>{defaultTemplates?.default_template_filename || '未设置'}</div>
            <div><span className="text-muted-foreground">个人默认 Surge：</span>{defaultTemplates?.default_surge_template_filename || '未设置'}</div>
            <div><span className="text-muted-foreground">个人默认 Loon：</span>{defaultTemplates?.default_loon_template_filename || '未设置'}</div>
          </div>
          <DataTable
            columns={columns}
            data={templates}
            getRowKey={(name) => name}
            emptyText="暂无模板，点击上方按钮创建"
            mobileCard={{
              header: (name) => <span className="font-medium text-base">{name}</span>,
              actions: (name) => (
                <div className="flex flex-wrap items-center gap-2 w-full justify-between px-2">
                  <Button variant="ghost" size="sm" onClick={() => handleEdit(name)} className="flex-1">
                    <Pencil className="h-4 w-4 mr-1.5" /> 编辑
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => handleListPreview(name)} className="flex-1">
                    <Eye className="h-4 w-4 mr-1.5" /> 预览
                  </Button>
                  <Button variant="ghost" size="sm" onClick={() => defaultMutation.mutate(name)} className="flex-1">
                    <Star className="h-4 w-4 mr-1.5" /> 默认
                  </Button>
                  {profile?.role === 'admin' && <Button variant="ghost" size="sm" onClick={() => visibilityMutation.mutate(name)} className="flex-1">
                    <Globe2 className={cn('h-4 w-4 mr-1.5', visibility[name] && 'text-primary')} /> {visibility[name] ? '公开' : '私有'}
                  </Button>}
                  <Button variant="ghost" size="sm" onClick={() => handleDelete(name)} className="flex-1 text-destructive hover:text-destructive hover:bg-destructive/10">
                    <Trash2 className="h-4 w-4 mr-1.5" /> 删除
                  </Button>
                </div>
              )
            }}
          />
        </CardContent>
      </Card>

      {/* Editor Dialog */}
      <Dialog open={isEditorOpen} onOpenChange={(open) => !open && handleCloseEditor()}>
        <DialogContent className={cn(
          "h-[90vh] flex flex-col",
          isMobile ? "!w-[95vw] !max-w-[95vw] p-4" : "!w-[85vw] !max-w-[85vw]"
        )} showCloseButton={false}>
          <DialogHeader className="flex-shrink-0">
            <div className={cn(
              "flex justify-between gap-4",
              isMobile ? "flex-col items-start" : "items-center"
            )}>
              <div>
                <DialogTitle className="break-all">{editingTemplateName}</DialogTitle>
                <DialogDescription>编辑模板配置</DialogDescription>
              </div>
              <div className={cn(
                "flex items-center gap-2",
                isMobile ? "w-full justify-between" : ""
              )}>
                {isDirty && <Badge variant="secondary">未保存</Badge>}
                <div className="flex gap-2">
                  <Button onClick={handleSave} disabled={updateMutation.isPending} size={isMobile ? "sm" : "default"}>
                    <Save className="h-4 w-4 mr-1 sm:mr-2" />
                    保存
                  </Button>
                  <Button variant="outline" onClick={handleCloseEditor} size={isMobile ? "sm" : "default"}>
                    关闭
                  </Button>
                </div>
              </div>
            </div>
          </DialogHeader>

          {/* Mobile: Preview below save button */}
          {isMobile && (
            <div className="flex-shrink-0 border-b pb-4 mt-2">
              <Collapsible open={isPreviewOpen} onOpenChange={setIsPreviewOpen}>
                <CollapsibleTrigger asChild>
                  <Button variant="outline" className="w-full h-8 text-sm">
                    {isPreviewOpen ? '收起配置预览' : '展开配置预览'}
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="mt-4 h-[250px]">
                  <TemplatePreview
                    content={previewContent}
                    isLoading={isPreviewLoading}
                    onRefresh={handlePreview}
                    title="代理组配置"
                    className="h-full"
                  />
                </CollapsibleContent>
              </Collapsible>
            </div>
          )}

          <div className={cn(
            "flex-1 flex gap-4 overflow-hidden mt-4",
            isMobile ? "flex-col" : "flex-row"
          )}>
            {/* Editor Panel - Left column on tablet/desktop */}
            <div className={cn(
              "flex flex-col overflow-hidden",
              isMobile ? "w-full flex-1" : isTablet ? "w-[55%]" : "w-[40%]"
            )}>
              <Tabs value={editorTab} onValueChange={handleTabChange} className="flex flex-col h-full overflow-hidden">
				<TabsList className={cn("flex-shrink-0 w-full grid", isTextTemplate ? "grid-cols-1" : "grid-cols-2")}>
				  {!isTextTemplate && <TabsTrigger value="visual">可视化编辑</TabsTrigger>}
				  <TabsTrigger value="yaml">{isSurgeTemplate ? 'Surge 配置' : isLoonTemplate ? 'Loon 配置' : 'YAML 代码'}</TabsTrigger>
                </TabsList>

				{!isTextTemplate && <TabsContent value="visual" className="flex-1 min-h-0 overflow-hidden mt-4 flex flex-col data-[state=inactive]:hidden">
                  <ScrollArea className="flex-1 h-full">
                    <div className="space-y-3 pb-4 pr-3">
                      {/* Region Proxy Groups Toggle */}
                      <div className="flex items-center justify-between p-3 border rounded-lg bg-muted/30">
                        <div className="flex flex-col sm:flex-row sm:items-center gap-1 sm:gap-2">
                          <Label htmlFor="region-toggle" className="font-medium">开启区域代理组</Label>
                          <span className="text-xs text-muted-foreground">自动添加按地区分类的代理组</span>
                        </div>
                        <Switch
                          id="region-toggle"
                          checked={enableRegionProxyGroups}
                          onCheckedChange={handleRegionProxyGroupsToggle}
                        />
                      </div>

                      {proxyGroups.map((group, index) => (
                        <ProxyGroupEditor
                          key={index}
                          group={group}
                          index={index}
                          allGroupNames={proxyGroups.map(g => g.name)}
                          onChange={handleProxyGroupChange}
                          onDelete={handleProxyGroupDelete}
                          onMoveUp={handleProxyGroupMoveUp}
                          onMoveDown={handleProxyGroupMoveDown}
                          isFirst={index === 0}
                          isLast={index === proxyGroups.length - 1}
                          showRegionToggle={enableRegionProxyGroups}
                          isRegionGroup={regionGroupNames.includes(group.name)}
                          variables={templateVariables}
                        />
                      ))}
                      <Button variant="outline" className="w-full mt-2" onClick={handleAddProxyGroup}>
                        <Plus className="h-4 w-4 mr-2" />
                        添加代理组
                      </Button>
                    </div>
                  </ScrollArea>
				</TabsContent>}

                <TabsContent value="yaml" className="flex-1 min-h-0 overflow-hidden mt-4 flex flex-col data-[state=inactive]:hidden">
                  <Textarea
                    value={templateContent}
                    onChange={(e) => handleYamlChange(e.target.value)}
                    className="flex-1 font-mono text-xs sm:text-sm resize-none p-4"
					placeholder={isSurgeTemplate ? 'Surge .conf 配置内容...' : isLoonTemplate ? 'Loon .lcf 配置内容...' : 'YAML 内容...'}
                  />
                </TabsContent>
              </Tabs>
            </div>

            {/* Preview Panel - Right column(s) on tablet/desktop */}
            {!isMobile && (
              <div className={cn(
                "border-l pl-4 flex overflow-hidden",
                isTablet ? "w-[45%]" : "w-[60%]"
              )}>
                <TemplatePreview
                  content={previewContent}
                  isLoading={isPreviewLoading}
                  onRefresh={handlePreview}
                  className="flex-1 h-full"
                  title="代理组配置"
                />
              </div>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* Upload Dialog */}
      <TemplateUploadDialog
        open={isUploadDialogOpen}
        onOpenChange={setIsUploadDialogOpen}
        onUpload={(file) => uploadMutation.mutate(file)}
        onCreate={(name, content) => createMutation.mutate({ name, content })}
        isLoading={uploadMutation.isPending || createMutation.isPending}
      />

      {/* List Preview Dialog */}
      <Dialog open={listPreviewOpen} onOpenChange={setListPreviewOpen}>
        <DialogContent className={cn(
          "h-[85vh] flex flex-col",
          isMobile ? "!w-[95vw] !max-w-[95vw] p-4" : "!w-[90vw] !max-w-[90vw]"
        )} showCloseButton={false}>
          <DialogHeader className="flex-shrink-0">
            <div className="flex items-center justify-between">
              <div>
                <DialogTitle className="break-all truncate w-[200px] sm:w-auto">预览: {listPreviewTemplateName}</DialogTitle>
                <DialogDescription className="hidden sm:block">左侧为模板配置，右侧为最终订阅配置</DialogDescription>
              </div>
              <Button variant="outline" onClick={() => setListPreviewOpen(false)} size={isMobile ? "sm" : "default"}>
                关闭
              </Button>
            </div>
          </DialogHeader>
          <div className={cn("flex-1 overflow-hidden flex gap-4", isMobile ? "flex-col" : "flex-row")}>
            {listPreviewLoading ? (
              <div className="flex items-center justify-center w-full h-full">
                <span className="text-muted-foreground">正在生成预览...</span>
              </div>
            ) : (
              <>
                {/* Left: Template Config */}
                <div className={cn("flex flex-col overflow-hidden", isMobile ? "h-1/2 w-full" : "w-1/2")}>
                  <div className="text-sm font-medium mb-2 text-muted-foreground">模板配置</div>
                  <Card className="flex-1 overflow-hidden">
                    <ScrollArea className="h-full">
                      <pre className="text-xs p-2 sm:p-4 font-mono whitespace-pre-wrap break-all">
                        {formatTemplateForDisplay(listPreviewTemplateContent)}
                      </pre>
                    </ScrollArea>
                  </Card>
                </div>
                {/* Right: Final Subscription Config */}
                <div className={cn("flex flex-col overflow-hidden", isMobile ? "h-1/2 w-full" : "w-1/2")}>
                  <div className="text-sm font-medium mb-2 text-muted-foreground">最终订阅配置</div>
                  <Card className="flex-1 overflow-hidden">
                    <ScrollArea className="h-full">
                      <pre className="text-xs p-2 sm:p-4 font-mono whitespace-pre-wrap break-all">
                        {listPreviewContent}
                      </pre>
                    </ScrollArea>
                  </Card>
                </div>
              </>
            )}
          </div>
        </DialogContent>
      </Dialog>

      {/* Delete Confirmation Dialog */}
      <AlertDialog open={isDeleteDialogOpen} onOpenChange={setIsDeleteDialogOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认删除</AlertDialogTitle>
            <AlertDialogDescription>
              确定要删除模板 "{deletingTemplateName}" 吗？此操作无法撤销。
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => deletingTemplateName && deleteMutation.mutate(deletingTemplateName)}
              className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
            >
              删除
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Rename Dialog */}
      <Dialog open={isRenameDialogOpen} onOpenChange={setIsRenameDialogOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>重命名模板</DialogTitle>
            <DialogDescription>输入新的模板名称</DialogDescription>
          </DialogHeader>
          <div className="py-4">
            <Input
              value={newTemplateName}
              onChange={(e) => setNewTemplateName(e.target.value)}
              placeholder="新模板名称"
            />
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setIsRenameDialogOpen(false)}>
              取消
            </Button>
            <Button
              onClick={() => renamingTemplate && renameMutation.mutate({ oldName: renamingTemplate, newName: newTemplateName })}
              disabled={renameMutation.isPending || !newTemplateName.trim()}
            >
              确认
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      {/* Close Confirmation Dialog */}
      <AlertDialog open={isCloseConfirmOpen} onOpenChange={setIsCloseConfirmOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>确认关闭</AlertDialogTitle>
            <AlertDialogDescription>
              有未保存的更改，确定要关闭吗？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>取消</AlertDialogCancel>
            <AlertDialogAction onClick={doCloseEditor}>
              确定关闭
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>

      {/* Draft Recovery Dialog */}
      <AlertDialog open={isDraftRecoveryOpen} onOpenChange={setIsDraftRecoveryOpen}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>恢复本地缓存</AlertDialogTitle>
            <AlertDialogDescription>
              检测到未保存的本地缓存，是否恢复？
            </AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel onClick={handleDiscardDraft}>放弃</AlertDialogCancel>
            <AlertDialogAction onClick={handleRecoverDraft}>恢复</AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
      </main>
    </div>
  )
}
