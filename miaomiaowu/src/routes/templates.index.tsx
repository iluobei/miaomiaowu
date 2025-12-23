// @ts-nocheck
import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Plus, Pencil, Trash2, Eye, Copy } from 'lucide-react'

import { DataTable } from '@/components/data-table'
import type { DataTableColumn } from '@/components/data-table'
import { Button } from '@/components/ui/button'
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from '@/components/ui/card'
import {
	Dialog,
	DialogContent,
	DialogDescription,
	DialogHeader,
	DialogTitle,
} from '@/components/ui/dialog'
import {
	AlertDialog,
	AlertDialogAction,
	AlertDialogCancel,
	AlertDialogContent,
	AlertDialogDescription,
	AlertDialogFooter,
	AlertDialogHeader,
	AlertDialogTitle,
} from '@/components/ui/alert-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from '@/components/ui/select'
import { toast } from 'sonner'
import { api } from '@/lib/api'

export const Route = createFileRoute('/templates/')({
	component: TemplatesPage,
})

interface Template {
	id: number
	name: string
	category: 'clash' | 'surge'
	template_url: string
	rule_source: string
	use_proxy: boolean
	enable_include_all: boolean
	created_at: string
	updated_at: string
}

interface ACL4SSRPreset {
	name: string
	url: string
	label: string
}

const Aethersailor_PRESETS: ACL4SSRPreset[] = [
	{ name: 'Custom_Clash', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash.ini', label: 'Aethersailor - 标准 (推荐使用)' },
	{ name: 'Custom_Clash_Full', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_Full.ini', label: 'Aethersailor - 全分组	(节点较多)' },
	{ name: 'Custom_Clash_GFW', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_GFW.ini', label: 'Aethersailor - 极简 (GFW)' },
	{ name: 'Custom_Clash_Lite', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_Lite.ini', label: 'Aethersailor - 轻量 (国内直连，国外代理)' },
	// { name: 'Custom_Clash_Mainland', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_Mainland.ini', label: 'Aethersailor - 标准 (推荐使用)' },
	// { name: 'Custom_Clash_SSRDOG', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_SSRDOG.ini', label: 'Aethersailor - 标准 (推荐使用)' },
	// { name: 'Custom_Clash_Test', url: 'https://raw.githubusercontent.com//Aethersailor/Custom_OpenClash_Rules/main/cfg/Custom_Clash_Test.ini', label: 'Aethersailor - 标准 (推荐使用)' },
]

// 内置 ACL4SSR 预设列表
const ACL4SSR_PRESETS: ACL4SSRPreset[] = [
	// 作者自用
	{ name: 'sublinkPro作者自用', url: 'https://raw.githubusercontent.com/ZeroDeng01/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_NoCountry.ini', label: 'sublinkPro作者自用 - 不区分国家' },
	// 标准版
	{ name: 'ACL4SSR', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR.ini', label: '标准版 - 典型分组' },
	{ name: 'ACL4SSR_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_AdblockPlus.ini', label: '标准版 - 典型分组+去广告' },
	// 回国版
	{ name: 'ACL4SSR_BackCN', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_BackCN.ini', label: '回国版 - 回国专用' },
	// 精简版
	{ name: 'ACL4SSR_Mini', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini.ini', label: '精简版 - 少量分组' },
	{ name: 'ACL4SSR_Mini_Fallback', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini_Fallback.ini', label: '精简版 - 故障转移' },
	{ name: 'ACL4SSR_Mini_MultiMode', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini_MultiMode.ini', label: '精简版 - 多模式 (自动/手动)' },
	{ name: 'ACL4SSR_Mini_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Mini_NoAuto.ini', label: '精简版 - 无自动测速' },
	// 无苹果/微软分流版
	{ name: 'ACL4SSR_NoApple', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoApple.ini', label: '无苹果 - 无苹果分流' },
	{ name: 'ACL4SSR_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoAuto.ini', label: '无测速 - 无自动测速' },
	{ name: 'ACL4SSR_NoAuto_NoApple', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoAuto_NoApple.ini', label: '无测速&苹果 - 无测速&无苹果分流' },
	{ name: 'ACL4SSR_NoAuto_NoApple_NoMicrosoft', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoAuto_NoApple_NoMicrosoft.ini', label: '无测速&苹果&微软 - 无测速&无苹果&无微软分流' },
	{ name: 'ACL4SSR_NoMicrosoft', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_NoMicrosoft.ini', label: '无微软 - 无微软分流' },
	// 在线版
	{ name: 'ACL4SSR_Online', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online.ini', label: '在线版 - 典型分组' },
	{ name: 'ACL4SSR_Online_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_AdblockPlus.ini', label: '在线版 - 典型分组+去广告' },
	// 在线全分组版
	{ name: 'ACL4SSR_Online_Full', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full.ini', label: '在线全分组 - 比较全' },
	{ name: 'ACL4SSR_Online_Full_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_AdblockPlus.ini', label: '在线全分组 - 带广告拦截' },
	{ name: 'ACL4SSR_Online_Full_Google', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_Google.ini', label: '在线全分组 - 谷歌分流' },
	{ name: 'ACL4SSR_Online_Full_MultiMode', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_MultiMode.ini', label: '在线全分组 - 多模式' },
	{ name: 'ACL4SSR_Online_Full_Netflix', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_Netflix.ini', label: '在线全分组 - 奈飞分流' },
	{ name: 'ACL4SSR_Online_Full_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Full_NoAuto.ini', label: '在线全分组 - 无自动测速' },
	// 在线精简版
	{ name: 'ACL4SSR_Online_Mini', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini.ini', label: '在线精简版 - 少量分组' },
	{ name: 'ACL4SSR_Online_Mini_AdblockPlus', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_AdblockPlus.ini', label: '在线精简版 - 带广告拦截' },
	{ name: 'ACL4SSR_Online_Mini_Ai', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_Ai.ini', label: '在线精简版 - AI' },
	{ name: 'ACL4SSR_Online_Mini_Fallback', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_Fallback.ini', label: '在线精简版 - 故障转移' },
	{ name: 'ACL4SSR_Online_Mini_MultiCountry', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_MultiCountry.ini', label: '在线精简版 - 多国家' },
	{ name: 'ACL4SSR_Online_Mini_MultiMode', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_MultiMode.ini', label: '在线精简版 - 多模式' },
	{ name: 'ACL4SSR_Online_Mini_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_Mini_NoAuto.ini', label: '在线精简版 - 无自动测速' },
	{ name: 'ACL4SSR_Online_MultiCountry', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_MultiCountry.ini', label: '在线版 - 多国家' },
	{ name: 'ACL4SSR_Online_NoAuto', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_NoAuto.ini', label: '在线版 - 无自动测速' },
	{ name: 'ACL4SSR_Online_NoReject', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_Online_NoReject.ini', label: '在线版 - 无拒绝规则' },
	// 特殊版
	{ name: 'ACL4SSR_WithChinaIp', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_WithChinaIp.ini', label: '特殊版 - 包含回国IP' },
	{ name: 'ACL4SSR_WithChinaIp_WithGFW', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_WithChinaIp_WithGFW.ini', label: '特殊版 - 包含回国IP&GFW列表' },
	{ name: 'ACL4SSR_WithGFW', url: 'https://raw.githubusercontent.com/ACL4SSR/ACL4SSR/master/Clash/config/ACL4SSR_WithGFW.ini', label: '特殊版 - 包含GFW列表' },
]

type TemplateFormData = Omit<Template, 'id' | 'created_at' | 'updated_at'>

function TemplatesPage() {
	const queryClient = useQueryClient()
	const [isDialogOpen, setIsDialogOpen] = useState(false)
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
	const [isPreviewDialogOpen, setIsPreviewDialogOpen] = useState(false)
	const [editingTemplate, setEditingTemplate] = useState<Template | null>(null)
	const [deletingTemplateId, setDeletingTemplateId] = useState<number | null>(null)
	const [previewContent, setPreviewContent] = useState('')
	const [isPreviewLoading, setIsPreviewLoading] = useState(false)
	const [formData, setFormData] = useState<TemplateFormData>({
		name: '',
		category: 'clash',
		template_url: '',
		rule_source: '',
		use_proxy: false,
		enable_include_all: true,
	})

	// Fetch templates
	const { data: templates = [], isLoading } = useQuery<Template[]>({
		queryKey: ['templates'],
		queryFn: async () => {
			const response = await api.get('/api/admin/templates')
			return response.data.templates || []
		},
	})

	// Create template mutation
	const createMutation = useMutation({
		mutationFn: async (template: TemplateFormData) => {
			const response = await api.post('/api/admin/templates', template)
			return response.data
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ['templates'] })
			setIsDialogOpen(false)
			resetForm()
			toast.success('模板已创建')
		},
		onError: (error: any) => {
			toast.error(error.response?.data?.error || '创建模板时出错')
		},
	})

	// Update template mutation
	const updateMutation = useMutation({
		mutationFn: async ({
			id,
			...template
		}: TemplateFormData & { id: number }) => {
			const response = await api.put(`/api/admin/templates/${id}`, template)
			return response.data
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ['templates'] })
			setIsDialogOpen(false)
			resetForm()
			toast.success('模板已更新')
		},
		onError: (error: any) => {
			toast.error(error.response?.data?.error || '更新模板时出错')
		},
	})

	// Delete template mutation
	const deleteMutation = useMutation({
		mutationFn: async (id: number) => {
			await api.delete(`/api/admin/templates/${id}`)
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ['templates'] })
			setIsDeleteDialogOpen(false)
			setDeletingTemplateId(null)
			toast.success('模板已删除')
		},
		onError: (error: any) => {
			toast.error(error.response?.data?.error || '删除模板时出错')
		},
	})

	const resetForm = () => {
		setFormData({
			name: '',
			category: 'clash',
			template_url: '',
			rule_source: '',
			use_proxy: false,
			enable_include_all: true,
		})
		setEditingTemplate(null)
	}

	const handleCreate = () => {
		resetForm()
		setIsDialogOpen(true)
	}

	const handleEdit = (template: Template) => {
		setEditingTemplate(template)
		setFormData({
			name: template.name,
			category: template.category,
			template_url: template.template_url,
			rule_source: template.rule_source,
			use_proxy: template.use_proxy,
			enable_include_all: template.enable_include_all,
		})
		setIsDialogOpen(true)
	}

	const handleDelete = (id: number) => {
		setDeletingTemplateId(id)
		setIsDeleteDialogOpen(true)
	}

	const handlePreview = async (template: Template) => {
		if (!template.rule_source) {
			toast.error('请先配置规则源')
			return
		}

		setIsPreviewLoading(true)
		setIsPreviewDialogOpen(true)

		try {
			const response = await api.post('/api/admin/templates/convert', {
				template_url: template.template_url,
				rule_source: template.rule_source,
				category: template.category,
				use_proxy: template.use_proxy,
				enable_include_all: template.enable_include_all,
			})
			setPreviewContent(response.data.content)
		} catch (error: any) {
			toast.error(error.response?.data?.error || '生成预览时出错')
			setIsPreviewDialogOpen(false)
		} finally {
			setIsPreviewLoading(false)
		}
	}

	const handleSubmit = () => {
		if (!formData.name.trim()) {
			toast.error('请输入模板名称')
			return
		}

		if (editingTemplate) {
			updateMutation.mutate({ id: editingTemplate.id, ...formData })
		} else {
			createMutation.mutate(formData)
		}
	}

	const handlePresetSelect = (url: string) => {
		setFormData({ ...formData, rule_source: url })
	}

	const columns: DataTableColumn<Template>[] = [
		{
			header: '名称',
			cell: (template) => (
				<span className="font-medium">{template.name}</span>
			),
		},
		{
			header: '类型',
			cell: (template) => (
				<Badge variant={template.category === 'clash' ? 'default' : 'secondary'}>
					{template.category === 'clash' ? 'Clash' : 'Surge'}
				</Badge>
			),
		},
		{
			header: '规则源',
			cell: (template) => (
				<span className="text-sm text-muted-foreground truncate max-w-[200px] block">
					{template.rule_source ? (
						<span title={template.rule_source}>
							{template.rule_source.split('/').pop()}
						</span>
					) : (
						<span className="text-muted-foreground/50">未配置</span>
					)}
				</span>
			),
		},
		{
			header: 'Include-All',
			cell: (template) => (
				<Badge variant={template.enable_include_all ? 'default' : 'outline'}>
					{template.enable_include_all ? '启用' : '禁用'}
				</Badge>
			),
		},
		{
			header: '更新时间',
			cell: (template) => (
				<span className="text-sm text-muted-foreground">
					{template.updated_at}
				</span>
			),
		},
		{
			header: '操作',
			cell: (template) => (
				<div className="flex items-center gap-1">
					<Button
						variant="ghost"
						size="icon"
						onClick={() => handlePreview(template)}
						title="预览"
					>
						<Eye className="h-4 w-4" />
					</Button>
					<Button
						variant="ghost"
						size="icon"
						onClick={() => handleEdit(template)}
						title="编辑"
					>
						<Pencil className="h-4 w-4" />
					</Button>
					<Button
						variant="ghost"
						size="icon"
						onClick={() => handleDelete(template.id)}
						title="删除"
					>
						<Trash2 className="h-4 w-4 text-destructive" />
					</Button>
				</div>
			),
		},
	]

	return (
		<div className="container mx-auto py-6 space-y-6">
			<Card>
				<CardHeader className="flex flex-row items-center justify-between">
					<div>
						<CardTitle>模板管理</CardTitle>
						<CardDescription>
							管理 ACL4SSR 规则模板，从远程配置自动生成代理组和规则
						</CardDescription>
					</div>
					<Button onClick={handleCreate}>
						<Plus className="h-4 w-4 mr-2" />
						新建模板
					</Button>
				</CardHeader>
				<CardContent>
					<DataTable
						columns={columns}
						data={templates}
						getRowKey={(template) => template.id}
						emptyText="暂无模板，点击上方按钮创建"
					/>
				</CardContent>
			</Card>

			{/* Create/Edit Dialog */}
			<Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
				<DialogContent className="max-w-2xl">
					<DialogHeader>
						<DialogTitle>
							{editingTemplate ? '编辑模板' : '新建模板'}
						</DialogTitle>
						<DialogDescription>
							配置模板信息和规则源
						</DialogDescription>
					</DialogHeader>

					<div className="space-y-4 py-4">
						<div className="space-y-2">
							<Label htmlFor="name">模板名称</Label>
							<Input
								id="name"
								value={formData.name}
								onChange={(e) =>
									setFormData({ ...formData, name: e.target.value })
								}
								placeholder="输入模板名称"
							/>
						</div>

						<div className="space-y-2">
							<Label htmlFor="category">输出格式</Label>
							<Select
								value={formData.category}
								onValueChange={(value: 'clash' | 'surge') =>
									setFormData({ ...formData, category: value })
								}
							>
								<SelectTrigger>
									<SelectValue />
								</SelectTrigger>
								<SelectContent>
									<SelectItem value="clash">Clash</SelectItem>
									<SelectItem value="surge">Surge</SelectItem>
								</SelectContent>
							</Select>
						</div>

						<div className="space-y-2">
							<Label htmlFor="template_url">模板 URL (可选)</Label>
							<Input
								id="template_url"
								value={formData.template_url}
								onChange={(e) =>
									setFormData({ ...formData, template_url: e.target.value })
								}
								placeholder="GitHub 原始文件 URL，留空使用默认模板"
							/>
							<p className="text-xs text-muted-foreground">
								配置文件的基础模板，包含 DNS、General 等设置
							</p>
						</div>

						<div className="space-y-2">
							<Label htmlFor="rule_source">规则源</Label>
							<div className="flex gap-2">
								<Input
									id="rule_source"
									value={formData.rule_source}
									onChange={(e) =>
										setFormData({ ...formData, rule_source: e.target.value })
									}
									placeholder="ACL4SSR 配置 URL"
									className="flex-1"
								/>
								<Select onValueChange={handlePresetSelect}>
									<SelectTrigger className="w-[180px]">
										<SelectValue placeholder="选择预设" />
									</SelectTrigger>
									<SelectContent>
										{ACL4SSR_PRESETS.map((preset) => (
											<SelectItem key={preset.name} value={preset.url}>
												{preset.label}
											</SelectItem>
										))}
									</SelectContent>
								</Select>
							</div>
							<p className="text-xs text-muted-foreground">
								ACL4SSR 格式的规则配置 URL
							</p>
						</div>

						<div className="flex items-center justify-between">
							<div className="space-y-0.5">
								<Label>启用 Include-All</Label>
								<p className="text-xs text-muted-foreground">
									代理组自动包含所有节点
								</p>
							</div>
							<Switch
								checked={formData.enable_include_all}
								onCheckedChange={(checked) =>
									setFormData({ ...formData, enable_include_all: checked })
								}
							/>
						</div>

						<div className="flex items-center justify-between">
							<div className="space-y-0.5">
								<Label>使用代理下载</Label>
								<p className="text-xs text-muted-foreground">
									通过代理下载远程配置
								</p>
							</div>
							<Switch
								checked={formData.use_proxy}
								onCheckedChange={(checked) =>
									setFormData({ ...formData, use_proxy: checked })
								}
							/>
						</div>
					</div>

					<div className="flex justify-end gap-2">
						<Button variant="outline" onClick={() => setIsDialogOpen(false)}>
							取消
						</Button>
						<Button
							onClick={handleSubmit}
							disabled={createMutation.isPending || updateMutation.isPending}
						>
							{editingTemplate ? '保存' : '创建'}
						</Button>
					</div>
				</DialogContent>
			</Dialog>

			{/* Delete Confirmation Dialog */}
			<AlertDialog open={isDeleteDialogOpen} onOpenChange={setIsDeleteDialogOpen}>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>确认删除</AlertDialogTitle>
						<AlertDialogDescription>
							确定要删除这个模板吗？此操作无法撤销。
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>取消</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => deletingTemplateId && deleteMutation.mutate(deletingTemplateId)}
							className="bg-destructive text-destructive-foreground hover:bg-destructive/90"
						>
							删除
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			{/* Preview Dialog */}
			<Dialog open={isPreviewDialogOpen} onOpenChange={setIsPreviewDialogOpen}>
				<DialogContent className="max-w-4xl max-h-[80vh]">
					<DialogHeader>
						<DialogTitle className="flex items-center justify-between">
							<span>配置预览</span>
							<Button
								variant="outline"
								size="sm"
								onClick={() => {
									navigator.clipboard.writeText(previewContent)
									toast.success('已复制到剪贴板')
								}}
							>
								<Copy className="h-4 w-4 mr-2" />
								复制
							</Button>
						</DialogTitle>
						<DialogDescription>
							生成的配置文件预览
						</DialogDescription>
					</DialogHeader>

					<div className="overflow-auto max-h-[60vh]">
						{isPreviewLoading ? (
							<div className="flex items-center justify-center py-8">
								<span className="text-muted-foreground">正在生成预览...</span>
							</div>
						) : (
							<pre className="text-xs bg-muted p-4 rounded-md whitespace-pre-wrap font-mono">
								{previewContent}
							</pre>
						)}
					</div>
				</DialogContent>
			</Dialog>
		</div>
	)
}
