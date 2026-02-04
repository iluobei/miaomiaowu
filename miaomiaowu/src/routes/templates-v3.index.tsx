// @ts-nocheck
import { useState, useEffect, useCallback } from 'react'
import { createFileRoute, redirect } from '@tanstack/react-router'
import { useQuery, useMutation, useQueryClient } from '@tanstack/react-query'
import { toast } from 'sonner'
import { Plus, Pencil, Trash2, Eye, Upload, Save, X } from 'lucide-react'

import { Topbar } from '@/components/layout/topbar'
import { useAuthStore } from '@/stores/auth-store'
import { api } from '@/lib/api'
import { useMediaQuery } from '@/hooks/use-media-query'
import { cn } from '@/lib/utils'

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
import {
  extractProxyGroups,
  updateProxyGroups,
  createDefaultFormState,
  parseTemplate,
  generateProxyGroupsPreview,
  PROXY_NODES_MARKER,
  PROXY_PROVIDERS_MARKER,
  PROXY_NODES_DISPLAY,
  PROXY_PROVIDERS_DISPLAY,
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

  // Editing state
  const [editingTemplateName, setEditingTemplateName] = useState<string | null>(null)
  const [templateContent, setTemplateContent] = useState('')
  const [proxyGroups, setProxyGroups] = useState<ProxyGroupFormState[]>([])
  const [editorTab, setEditorTab] = useState<'visual' | 'yaml'>('visual')
  const [isDirty, setIsDirty] = useState(false)

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
  const { data: templates = [], isLoading } = useQuery<string[]>({
    queryKey: ['rule-templates'],
    queryFn: async () => {
      const response = await api.get('/api/admin/rule-templates')
      return response.data.templates || []
    },
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
      setTemplateContent(templateData)
      setProxyGroups(extractProxyGroups(templateData))
      setIsDirty(false)
    }
  }, [templateData, isEditorOpen])

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
      setProxyGroups(extractProxyGroups(templateContent))
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
  }

  // Handle proxy group change
  const handleProxyGroupChange = (index: number, group: ProxyGroupFormState) => {
    const newGroups = [...proxyGroups]
    newGroups[index] = group
    setProxyGroups(newGroups)
    setIsDirty(true)
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
  }

  // Table columns
  const columns: DataTableColumn<string>[] = [
    {
      header: '模板名称',
      cell: (name) => <span className="font-medium">{name}</span>,
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
          <Button variant="ghost" size="icon" onClick={() => handleDelete(name)} title="删除">
            <Trash2 className="h-4 w-4 text-destructive" />
          </Button>
        </div>
      ),
    },
  ]

  return (
    <div className="min-h-svh bg-background">
      <Topbar />
      <main className="mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24">
      <Card>
        <CardHeader className="flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
          <div>
            <CardTitle>V3 模板管理</CardTitle>
            <CardDescription>
              管理 mihomo 风格的规则模板，支持 include-all、filter 等高级特性
            </CardDescription>
          </div>
          <Button onClick={() => setIsUploadDialogOpen(true)} className="w-full sm:w-auto">
            <Plus className="h-4 w-4 mr-2" />
            新建模板
          </Button>
        </CardHeader>
        <CardContent>
          <DataTable
            columns={columns}
            data={templates}
            getRowKey={(name) => name}
            emptyText="暂无模板，点击上方按钮创建"
          />
        </CardContent>
      </Card>

      {/* Editor Dialog */}
      <Dialog open={isEditorOpen} onOpenChange={(open) => !open && handleCloseEditor()}>
        <DialogContent className="!w-[85vw] !max-w-[85vw] h-[90vh] flex flex-col" showCloseButton={false}>
          <DialogHeader className="flex-shrink-0">
            <div className="flex items-center justify-between">
              <div>
                <DialogTitle>{editingTemplateName}</DialogTitle>
                <DialogDescription>编辑模板配置</DialogDescription>
              </div>
              <div className="flex items-center gap-2">
                {isDirty && <Badge variant="secondary">未保存</Badge>}
                <Button onClick={handleSave} disabled={updateMutation.isPending}>
                  <Save className="h-4 w-4 mr-2" />
                  保存
                </Button>
                <Button variant="outline" onClick={handleCloseEditor}>
                  关闭
                </Button>
              </div>
            </div>
          </DialogHeader>

          {/* Mobile: Preview below save button */}
          {isMobile && (
            <div className="flex-shrink-0 border-b pb-4">
              <Collapsible open={isPreviewOpen} onOpenChange={setIsPreviewOpen}>
                <CollapsibleTrigger asChild>
                  <Button variant="outline" className="w-full">
                    {isPreviewOpen ? '收起配置预览' : '展开配置预览'}
                  </Button>
                </CollapsibleTrigger>
                <CollapsibleContent className="mt-4">
                  <TemplatePreview
                    content={previewContent}
                    isLoading={isPreviewLoading}
                    onRefresh={handlePreview}
                    title="代理组配置"
                  />
                </CollapsibleContent>
              </Collapsible>
            </div>
          )}

          <div className={cn(
            "flex-1 flex gap-4 overflow-hidden",
            isMobile ? "flex-col" : "flex-row"
          )}>
            {/* Editor Panel - Left column on tablet/desktop */}
            <div className={cn(
              "flex flex-col overflow-hidden",
              isMobile ? "flex-1" : isTablet ? "w-[55%]" : "w-[40%]"
            )}>
              <Tabs value={editorTab} onValueChange={handleTabChange} className="flex flex-col h-full">
                <TabsList className="flex-shrink-0">
                  <TabsTrigger value="visual">可视化编辑</TabsTrigger>
                  <TabsTrigger value="yaml">YAML 代码</TabsTrigger>
                </TabsList>

                <TabsContent value="visual" className="flex-1 overflow-hidden mt-4">
                  <ScrollArea className="h-full pr-4">
                    <div className="space-y-3">
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
                        />
                      ))}
                      <Button variant="outline" className="w-full" onClick={handleAddProxyGroup}>
                        <Plus className="h-4 w-4 mr-2" />
                        添加代理组
                      </Button>
                    </div>
                  </ScrollArea>
                </TabsContent>

                <TabsContent value="yaml" className="flex-1 overflow-hidden mt-4">
                  <Textarea
                    value={templateContent}
                    onChange={(e) => handleYamlChange(e.target.value)}
                    className="h-full font-mono text-sm resize-none"
                    placeholder="YAML 内容..."
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
        <DialogContent className="!w-[90vw] !max-w-[90vw] h-[85vh] flex flex-col" showCloseButton={false}>
          <DialogHeader className="flex-shrink-0">
            <div className="flex items-center justify-between">
              <div>
                <DialogTitle>预览: {listPreviewTemplateName}</DialogTitle>
                <DialogDescription>左侧为模板配置，右侧为最终订阅配置</DialogDescription>
              </div>
              <Button variant="outline" onClick={() => setListPreviewOpen(false)}>
                关闭
              </Button>
            </div>
          </DialogHeader>
          <div className="flex-1 overflow-hidden flex gap-4">
            {listPreviewLoading ? (
              <div className="flex items-center justify-center w-full h-full">
                <span className="text-muted-foreground">正在生成预览...</span>
              </div>
            ) : (
              <>
                {/* Left: Template Config */}
                <div className="w-1/2 flex flex-col overflow-hidden">
                  <div className="text-sm font-medium mb-2 text-muted-foreground">模板配置</div>
                  <Card className="flex-1 overflow-hidden">
                    <ScrollArea className="h-full">
                      <pre className="text-xs p-4 font-mono whitespace-pre-wrap break-all">
                        {formatTemplateForDisplay(listPreviewTemplateContent)}
                      </pre>
                    </ScrollArea>
                  </Card>
                </div>
                {/* Right: Final Subscription Config */}
                <div className="w-1/2 flex flex-col overflow-hidden">
                  <div className="text-sm font-medium mb-2 text-muted-foreground">最终订阅配置</div>
                  <Card className="flex-1 overflow-hidden">
                    <ScrollArea className="h-full">
                      <pre className="text-xs p-4 font-mono whitespace-pre-wrap break-all">
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
      </main>
    </div>
  )
}
