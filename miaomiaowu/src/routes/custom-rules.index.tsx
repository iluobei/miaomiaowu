import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Plus, Pencil, Trash2 } from 'lucide-react'

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
import { Textarea } from '@/components/ui/textarea'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import {
	Select,
	SelectContent,
	SelectItem,
	SelectTrigger,
	SelectValue,
} from '@/components/ui/select'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import { RULE_TEMPLATES, RULE_PROVIDER_RULES } from '../config/custom-rules-templates'

export const Route = createFileRoute('/custom-rules/')({
	component: CustomRulesPage,
})

interface CustomRule {
	id: number
	name: string
	type: 'dns' | 'rules' | 'rule-providers'
	mode: 'replace' | 'prepend' | 'append'
	content: string
	enabled: boolean
	created_at: string
	updated_at: string
	added_proxy_groups?: string[]
}

type RuleFormData = Omit<CustomRule, 'id' | 'created_at' | 'updated_at'>

function CustomRulesPage() {
	const queryClient = useQueryClient()
	const [filterType, setFilterType] = useState<string>('')
	const [isDialogOpen, setIsDialogOpen] = useState(false)
	const [isDeleteDialogOpen, setIsDeleteDialogOpen] = useState(false)
	const [editingRule, setEditingRule] = useState<CustomRule | null>(null)
	const [deletingRuleId, setDeletingRuleId] = useState<number | null>(null)
	const [formData, setFormData] = useState<RuleFormData>({
		name: '',
		type: 'dns',
		mode: 'replace',
		content: '',
		enabled: true,
	})
	const [selectedTemplate, setSelectedTemplate] = useState<string | null>(null)
	const [isRuleProviderConfirmOpen, setIsRuleProviderConfirmOpen] = useState(false)
	const [pendingRuleProviderData, setPendingRuleProviderData] = useState<RuleFormData | null>(null)

	// Fetch rules
	const { data: rules = [], isLoading } = useQuery<CustomRule[]>({
		queryKey: ['custom-rules', filterType],
		queryFn: async () => {
			const params = filterType ? { type: filterType } : {}
			const response = await api.get('/api/admin/custom-rules', { params })
			return response.data
		},
	})

	// Create rule mutation
	const createMutation = useMutation({
		mutationFn: async (rule: RuleFormData) => {
			// 如果是启用状态且模式为替换，需要先禁用同类型的其他替换模式规则
			if (rule.enabled && rule.mode === 'replace') {
				const conflictingRules = rules.filter(
					r => r.type === rule.type &&
					r.mode === 'replace' &&
					r.enabled
				)

				for (const conflictRule of conflictingRules) {
					await api.put(`/api/admin/custom-rules/${conflictRule.id}`, {
						name: conflictRule.name,
						type: conflictRule.type,
						mode: conflictRule.mode,
						content: conflictRule.content,
						enabled: false,
					})
				}
			}

			const response = await api.post('/api/admin/custom-rules', rule)
			return response.data
		},
		onSuccess: (data: CustomRule) => {
			queryClient.invalidateQueries({ queryKey: ['custom-rules'] })
			setIsDialogOpen(false)
			resetForm()

			// 如果有新增的代理组，显示通知
			if (data.added_proxy_groups && data.added_proxy_groups.length > 0) {
				toast.success(
					`自定义规则已创建。已新增 ${data.added_proxy_groups.join('、')} 代理组，默认节点：🚀 节点选择、DIRECT，如需修改请编辑订阅`,
					{ duration: 8000 }
				)
			} else {
				toast.success('自定义规则已创建')
			}
		},
		onError: (error: any) => {
			toast.error(error.response?.data?.error || '创建规则时出错')
		},
	})

	// Update rule mutation
	const updateMutation = useMutation({
		mutationFn: async ({
			id,
			...rule
		}: RuleFormData & { id: number }) => {
			// 如果是启用状态且模式为替换，需要先禁用同类型的其他替换模式规则
			if (rule.enabled && rule.mode === 'replace') {
				const conflictingRules = rules.filter(
					r => r.id !== id &&
					r.type === rule.type &&
					r.mode === 'replace' &&
					r.enabled
				)

				for (const conflictRule of conflictingRules) {
					await api.put(`/api/admin/custom-rules/${conflictRule.id}`, {
						name: conflictRule.name,
						type: conflictRule.type,
						mode: conflictRule.mode,
						content: conflictRule.content,
						enabled: false,
					})
				}
			}

			const response = await api.put(`/api/admin/custom-rules/${id}`, rule)
			return response.data
		},
		onSuccess: (data: CustomRule) => {
			queryClient.invalidateQueries({ queryKey: ['custom-rules'] })
			setIsDialogOpen(false)
			resetForm()

			// 如果有新增的代理组，显示通知
			if (data.added_proxy_groups && data.added_proxy_groups.length > 0) {
				toast.success(
					`自定义规则已更新。已新增 ${data.added_proxy_groups.join('、')} 代理组，默认节点：🚀 节点选择、DIRECT，如需修改请编辑订阅`,
					{ duration: 8000 }
				)
			} else {
				toast.success('自定义规则已更新')
			}
		},
		onError: (error: any) => {
			toast.error(error.response?.data?.error || '更新规则时出错')
		},
	})

	// Delete rule mutation
	const deleteMutation = useMutation({
		mutationFn: async (id: number) => {
			await api.delete(`/api/admin/custom-rules/${id}`)
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ['custom-rules'] })
			setIsDeleteDialogOpen(false)
			setDeletingRuleId(null)
			toast.success('自定义规则已删除')
		},
		onError: (error: any) => {
			toast.error(error.response?.data?.error || '删除规则时出错')
		},
	})

	// Toggle enabled state mutation
	const toggleEnabledMutation = useMutation({
		mutationFn: async ({ id, enabled }: { id: number; enabled: boolean }) => {
			const rule = rules.find(r => r.id === id)
			if (!rule) throw new Error('规则不存在')

			// 如果是启用操作且模式为替换，需要检查同类型的其他替换模式规则
			if (enabled && rule.mode === 'replace') {
				// 找出同类型且为替换模式的其他已启用规则
				const conflictingRules = rules.filter(
					r => r.id !== id &&
					r.type === rule.type &&
					r.mode === 'replace' &&
					r.enabled
				)

				// 如果有冲突的规则，先禁用它们
				for (const conflictRule of conflictingRules) {
					await api.put(`/api/admin/custom-rules/${conflictRule.id}`, {
						name: conflictRule.name,
						type: conflictRule.type,
						mode: conflictRule.mode,
						content: conflictRule.content,
						enabled: false,
					})
				}
			}

			// 更新当前规则
			const response = await api.put(`/api/admin/custom-rules/${id}`, {
				name: rule.name,
				type: rule.type,
				mode: rule.mode,
				content: rule.content,
				enabled: enabled,
			})
			return response.data
		},
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ['custom-rules'] })
			toast.success('状态已更新')
		},
		onError: (error: any) => {
			toast.error(error.response?.data?.error || '更新状态时出错')
		},
	})

	const resetForm = () => {
		setFormData({
			name: '',
			type: 'dns',
			mode: 'replace',
			content: '',
			enabled: true,
		})
		setEditingRule(null)
		setSelectedTemplate(null)
	}

	const handleCreate = () => {
		resetForm()
		setIsDialogOpen(true)
	}

	const handleEdit = (rule: CustomRule) => {
		setEditingRule(rule)
		setFormData({
			name: rule.name,
			type: rule.type,
			mode: rule.mode,
			content: rule.content,
			enabled: rule.enabled,
		})
		setIsDialogOpen(true)
	}

	const handleDelete = (id: number) => {
		setDeletingRuleId(id)
		setIsDeleteDialogOpen(true)
	}

	const handleSubmit = () => {
		if (!formData.name.trim()) {
			toast.error('请输入规则名称')
			return
		}

		if (!formData.content.trim()) {
			toast.error('请输入规则内容')
			return
		}

		// 如果是新建规则集且使用了模板，询问是否创建对应的规则配置
		if (!editingRule && formData.type === 'rule-providers' && selectedTemplate && selectedTemplate in RULE_PROVIDER_RULES) {
			setPendingRuleProviderData(formData)
			setIsRuleProviderConfirmOpen(true)
			return
		}

		if (editingRule) {
			updateMutation.mutate({ id: editingRule.id, ...formData })
		} else {
			createMutation.mutate(formData)
		}
	}


	// 处理规则集确认创建
	const handleRuleProviderConfirm = async (createRuleConfig: boolean) => {
		if (!pendingRuleProviderData) return

		try {
			// 先创建规则集
			await createMutation.mutateAsync(pendingRuleProviderData)

			// 如果用户选择创建规则配置
			if (createRuleConfig && selectedTemplate && selectedTemplate in RULE_PROVIDER_RULES) {
				const ruleContent = RULE_PROVIDER_RULES[selectedTemplate as keyof typeof RULE_PROVIDER_RULES]

				// 重新获取最新的规则列表
				await queryClient.invalidateQueries({ queryKey: ['custom-rules'] })
				const latestRules = await api.get('/api/admin/custom-rules').then(res => res.data)

				// 查找现有的 rules 类型规则
				const existingRulesRules = latestRules?.filter((r: CustomRule) => r.type === 'rules')

				// 处理规则内容：移除与现有规则重复的部分（忽略大小写）
				let finalRuleContent = ruleContent

				if (existingRulesRules && existingRulesRules.length > 0) {
					// 收集所有现有 rules 的内容
					const allExistingLines: string[] = []
					existingRulesRules.forEach((rule: CustomRule) => {
						const lines = rule.content.split('\n').map((line: string) => line.trim()).filter((line: string) => line)
						// 从第二行开始收集（跳过第一行，通常是注释或标题）
						allExistingLines.push(...lines.slice(1))
					})

					const newLines = ruleContent.split('\n').map((line: string) => line.trim()).filter((line: string) => line)
					const existingLinesLower = allExistingLines.map((line: string) => line.toLowerCase())

					// 过滤掉与现有规则重复的内容
					const filteredNewLines = newLines.filter((line: string) =>
						!existingLinesLower.includes(line.toLowerCase())
					)

					// 如果过滤后还有内容，则使用过滤后的内容
					if (filteredNewLines.length > 0) {
						finalRuleContent = filteredNewLines.join('\n')
					}
				}

				// 创建新的 rules 规则
				const rulesData: RuleFormData = {
					name: `路由规则 - ${pendingRuleProviderData.name}`,
					type: 'rules',
					mode: 'append',
					content: finalRuleContent,
					enabled: true
				}
				await createMutation.mutateAsync(rulesData)
				toast.success('规则集和规则配置已创建')
			} else {
				toast.success('规则集已创建')
			}
		} catch (error) {
			console.error('创建规则配置失败:', error)
			toast.error('创建规则配置时出错，请检查控制台')
		} finally {
			setIsRuleProviderConfirmOpen(false)
			setPendingRuleProviderData(null)
			setIsDialogOpen(false)
			resetForm()
		}
	}
	const getTypeLabel = (type: string) => {
		switch (type) {
			case 'dns':
				return 'DNS'
			case 'rules':
				return '规则'
			case 'rule-providers':
				return '规则集'
			default:
				return type
		}
	}

	const getTypeBadgeClass = (type: string) => {
		switch (type) {
			case 'dns':
				return 'bg-blue-500/10 text-blue-700 border-blue-500/20 dark:bg-blue-500/20 dark:text-blue-400 dark:border-blue-500/30'
			case 'rules':
				return 'bg-green-500/10 text-green-700 border-green-500/20 dark:bg-green-500/20 dark:text-green-400 dark:border-green-500/30'
			case 'rule-providers':
				return 'bg-purple-500/10 text-purple-700 border-purple-500/20 dark:bg-purple-500/20 dark:text-purple-400 dark:border-purple-500/30'
			default:
				return ''
		}
	}

	const getModeLabel = (mode: string) => {
		switch (mode) {
			case 'replace':
				return '替换'
			case 'prepend':
				return '添加至头部'
			case 'append':
				return '添加至尾部'
			default:
				return mode
		}
	}

	return (
		<main className='mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24'>
			<div className='space-y-6'>
				<div className='flex items-center justify-between'>
					<div>
						<h1 className='text-3xl font-bold'>自定义规则</h1>
						<p className='text-muted-foreground mt-2'>
							管理 DNS、规则和规则集的自定义配置
						</p>
					</div>
					<Button onClick={handleCreate}>
						<Plus className='mr-2 h-4 w-4' />
						新建规则
					</Button>
				</div>

				<Card>
					<CardHeader>
						<div className='flex items-center justify-between'>
							<div>
								<CardTitle>规则列表</CardTitle>
								<CardDescription>
									{rules.length} 条规则
								</CardDescription>
							</div>
							<Tabs value={filterType} onValueChange={setFilterType}>
								<TabsList>
									<TabsTrigger value=''>全部</TabsTrigger>
									<TabsTrigger value='dns'>DNS</TabsTrigger>
									<TabsTrigger value='rules'>规则</TabsTrigger>
									<TabsTrigger value='rule-providers'>规则集</TabsTrigger>
								</TabsList>
							</Tabs>
						</div>
					</CardHeader>
					<CardContent>
						{isLoading ? (
							<div className='text-center py-8 text-muted-foreground'>
								加载中...
							</div>
						) : (
							<DataTable
								data={rules}
								getRowKey={(rule) => rule.id}
								emptyText='暂无规则'

								columns={[
									{
										header: '名称',
										cell: (rule) => rule.name,
										cellClassName: 'font-medium'
									},
									{
										header: '类型',
										cell: (rule) => (
											<Badge variant='outline' className={getTypeBadgeClass(rule.type)}>
												{getTypeLabel(rule.type)}
											</Badge>
										)
									},
									{
										header: '模式',
										cell: (rule) => getModeLabel(rule.mode)
									},
									{
										header: '状态',
										cell: (rule) => (
											<div className='flex items-center gap-2'>
												<Switch
													checked={rule.enabled}
													onCheckedChange={(checked) => {
														toggleEnabledMutation.mutate({
															id: rule.id,
															enabled: checked,
														})
													}}
													disabled={toggleEnabledMutation.isPending}
												/>
												<span className='text-sm text-muted-foreground'>
													{rule.enabled ? '启用' : '禁用'}
												</span>
											</div>
										)
									},
									{
										header: '创建时间',
										cell: (rule) => (
											<span className='text-sm text-muted-foreground'>
												{new Date(rule.created_at).toLocaleString('zh-CN')}
											</span>
										)
									},
									{
										header: '操作',
										cell: (rule) => (
											<div className='flex justify-end gap-2'>
												<Button
													variant='ghost'
													size='icon'
													onClick={() => handleEdit(rule)}
												>
													<Pencil className='h-4 w-4' />
												</Button>
												<Button
													variant='ghost'
													size='icon'
													onClick={() => handleDelete(rule.id)}
												>
													<Trash2 className='h-4 w-4' />
												</Button>
											</div>
										),
										headerClassName: 'text-right',
										cellClassName: 'text-right'
									}
								] as DataTableColumn<CustomRule>[]}

								mobileCard={{
									header: (rule) => (
										<div className='space-y-1'>
											{/* 第一行：标签 + 名称 + 删除按钮 */}
											<div className='flex items-start justify-between gap-2'>
												<div className='flex items-center gap-2 flex-1 min-w-0'>
													<Badge variant='outline' className={`${getTypeBadgeClass(rule.type)} shrink-0`}>
														{getTypeLabel(rule.type)}
													</Badge>
													<div className='font-medium text-sm truncate flex-1 min-w-0'>{rule.name}</div>
												</div>
												<Button
													variant='outline'
													size='icon'
													className='size-8 shrink-0 text-destructive hover:text-destructive hover:bg-destructive/10'
													onClick={(e) => {
														e.stopPropagation()
														handleDelete(rule.id)
													}}
												>
													<Trash2 className='size-4' />
												</Button>
											</div>

											{/* 第二行：模式 + 状态 */}
											<div className='flex items-center justify-between gap-4 text-xs'>
												<div className='flex items-center gap-2 min-w-0'>
													<span className='text-muted-foreground shrink-0'>模式:</span>
													<span className='truncate'>{getModeLabel(rule.mode)}</span>
												</div>
												<div className='flex items-center gap-2 shrink-0'>
													<span className='text-muted-foreground shrink-0'>状态:</span>
													<div className='flex items-center gap-2'>
														<Switch
															checked={rule.enabled}
															onCheckedChange={(checked) => {
																toggleEnabledMutation.mutate({
																	id: rule.id,
																	enabled: checked,
																})
															}}
															disabled={toggleEnabledMutation.isPending}
														/>
														<span>{rule.enabled ? '启用' : '禁用'}</span>
													</div>
												</div>
											</div>

											{/* 第三行：创建时间 */}
											<div className='flex items-center gap-2 text-xs'>
												<span className='text-muted-foreground shrink-0'>创建时间:</span>
												<span>{new Date(rule.created_at).toLocaleString('zh-CN')}</span>
											</div>
										</div>
									),
									fields: [],
									actions: (rule) => (
										<Button
											variant='outline'
											size='sm'
											className='w-full'
											onClick={() => handleEdit(rule)}
										>
											<Pencil className='mr-1 h-4 w-4' />
											编辑
										</Button>
									)
								}}
							/>
						)}
					</CardContent>
				</Card>
			</div>

			{/* Create/Edit Dialog */}
			<Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
				<DialogContent className='max-w-3xl max-h-[90vh] overflow-y-auto'>
					<DialogHeader>
						<DialogTitle>
							{editingRule ? '编辑规则' : '新建规则'}
						</DialogTitle>
						<DialogDescription>
							{editingRule
								? '修改自定义规则配置'
								: '创建新的自定义规则'}
						</DialogDescription>
					</DialogHeader>

				{/* 顶部操作区 */}
				<div className='flex items-center justify-between border-b pb-4'>
					<div className='flex items-center space-x-2'>
						<Switch
							id='enabled'
							checked={formData.enabled}
							onCheckedChange={(checked) =>
								setFormData({ ...formData, enabled: checked })
							}
						/>
						<Label htmlFor='enabled'>启用此规则</Label>
					</div>
					<div className='flex items-center space-x-2'>
						<Button
							variant='outline'
							onClick={() => {
								setIsDialogOpen(false)
								resetForm()
							}}
						>
							取消
						</Button>
						<Button
							onClick={handleSubmit}
							disabled={
								createMutation.isPending || updateMutation.isPending
							}
						>
							{createMutation.isPending || updateMutation.isPending
								? '保存中...'
								: '保存'}
						</Button>
					</div>
				</div>

					<div className='space-y-4 py-4'>
						<div className='space-y-2'>
							<Label htmlFor='name'>名称</Label>
							<Input
								id='name'
								value={formData.name}
								onChange={(e) =>
									setFormData({ ...formData, name: e.target.value })
								}
								placeholder='规则名称'
							/>
						</div>

						<div className={`grid gap-4 ${!editingRule ? 'grid-cols-4' : 'grid-cols-2'}`}>
							<div className='space-y-2'>
								<Label htmlFor='type'>类型</Label>
								<Select
									value={formData.type}
									onValueChange={(value: any) => {
										const newFormData = {
											...formData,
											type: value,
										}
										// DNS type always uses replace mode
										if (value === 'dns') {
											newFormData.mode = 'replace'
										}
										setFormData(newFormData)
										// Reset selected template when changing type
										setSelectedTemplate(null)
									}}
								>
									<SelectTrigger id='type'>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value='dns'>DNS</SelectItem>
										<SelectItem value='rules'>规则</SelectItem>
										<SelectItem value='rule-providers'>规则集</SelectItem>
									</SelectContent>
								</Select>
							</div>

							<div className='space-y-2'>
								<Label htmlFor='mode'>模式</Label>
								<Select
									value={formData.mode}
									onValueChange={(value: any) =>
										setFormData({ ...formData, mode: value })
									}
									disabled={formData.type === 'dns'}
								>
									<SelectTrigger id='mode'>
										<SelectValue />
									</SelectTrigger>
									<SelectContent>
										<SelectItem value='replace'>替换</SelectItem>
										<SelectItem value='prepend'>添加至头部</SelectItem>
										<SelectItem value='append'>添加至尾部</SelectItem>
									</SelectContent>
								</Select>
							</div>
				{/* 模板选择 - 仅在新建时显示 */}
				{!editingRule && (
					<div className='space-y-2 col-span-2'>
						<Label htmlFor='template'>模板（可选）</Label>
						<Select
							value={selectedTemplate || 'none'}
							onValueChange={(value: string) => {
								if (value === 'none') {
									setSelectedTemplate(null)
									return
								}

								const templates = RULE_TEMPLATES[formData.type as keyof typeof RULE_TEMPLATES]
								const template = templates[value as keyof typeof templates] as { name: string; content: string } | undefined

if (template) {
									setSelectedTemplate(value)

									// 检查当前名称是否为空或是某个模板的名称
									const allTemplates = RULE_TEMPLATES[formData.type as keyof typeof RULE_TEMPLATES]
									const isTemplateName = Object.values(allTemplates).some(
										(t: any) => t.name === formData.name
									)

									setFormData({
										...formData,
										// 只在名称为空或当前名称是模板名称时才更新名称
										name: (formData.name === '' || isTemplateName) ? template.name : formData.name,
										content: template.content
									})
								}
							}}
						>
							<SelectTrigger id='template'>
								<SelectValue placeholder='选择模板或手动输入' />
							</SelectTrigger>
							<SelectContent>
								<SelectItem value='none'>不使用模板</SelectItem>
								{Object.entries(RULE_TEMPLATES[formData.type as keyof typeof RULE_TEMPLATES]).map(([key, template]) => (
									<SelectItem key={key} value={key}>
										{template.name}
									</SelectItem>
								))}
							</SelectContent>
						</Select>
						</div>
					)}
					</div>
						<div className='space-y-2'>
							<Label htmlFor='content'>规则内容（YAML 格式）</Label>
							<Textarea
								id='content'
								value={formData.content}
								onChange={(e) =>
									setFormData({ ...formData, content: e.target.value })
								}
								placeholder='输入 YAML 格式的规则内容...'
								className='font-mono text-sm min-h-[300px] whitespace-pre-wrap break-all [field-sizing:fixed]'
							/>
							<p className='text-xs text-muted-foreground'>
								请确保内容符合 YAML 格式规范
							</p>
						</div>

					</div>
				</DialogContent>
			</Dialog>

			{/* Delete Confirmation Dialog */}
			<AlertDialog
				open={isDeleteDialogOpen}
				onOpenChange={setIsDeleteDialogOpen}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>确认删除</AlertDialogTitle>
						<AlertDialogDescription>
							此操作无法撤销。确定要删除这条规则吗？
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel>取消</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => {
								if (deletingRuleId) {
									deleteMutation.mutate(deletingRuleId)
								}
							}}
							disabled={deleteMutation.isPending}
						>
							{deleteMutation.isPending ? '删除中...' : '删除'}
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>

			{/* Rule Provider Confirmation Dialog */}
			<AlertDialog
				open={isRuleProviderConfirmOpen}
				onOpenChange={setIsRuleProviderConfirmOpen}
			>
				<AlertDialogContent>
					<AlertDialogHeader>
						<AlertDialogTitle>创建规则配置</AlertDialogTitle>
						<AlertDialogDescription>
							检测到您使用了规则集模板，是否同时创建对应的规则配置？
							<br /><br />
							规则配置将会追加到现有规则的末尾，系统会自动去除重复的规则（忽略大小写）。
						</AlertDialogDescription>
					</AlertDialogHeader>
					<AlertDialogFooter>
						<AlertDialogCancel onClick={() => handleRuleProviderConfirm(false)}>
							仅创建规则集
						</AlertDialogCancel>
						<AlertDialogAction
							onClick={() => handleRuleProviderConfirm(true)}
						>
							创建规则集和规则配置
						</AlertDialogAction>
					</AlertDialogFooter>
				</AlertDialogContent>
			</AlertDialog>
		</main>
	)
}
