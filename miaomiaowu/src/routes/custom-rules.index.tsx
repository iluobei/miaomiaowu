import { createFileRoute } from '@tanstack/react-router'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { Plus, Pencil, Trash2 } from 'lucide-react'

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
	Table,
	TableBody,
	TableCell,
	TableHead,
	TableHeader,
	TableRow,
} from '@/components/ui/table'
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
import { RULE_TEMPLATES } from './custom-rules-templates'

export const Route = createFileRoute('/custom-rules/')({
	component: CustomRulesPage,
})

interface CustomRule {
	id: number
	name: string
	type: 'dns' | 'rules' | 'rule-providers'
	mode: 'replace' | 'prepend'
	content: string
	enabled: boolean
	created_at: string
	updated_at: string
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
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ['custom-rules'] })
			setIsDialogOpen(false)
			resetForm()
			toast.success('自定义规则已创建')
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
		onSuccess: () => {
			queryClient.invalidateQueries({ queryKey: ['custom-rules'] })
			setIsDialogOpen(false)
			resetForm()
			toast.success('自定义规则已更新')
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

		if (editingRule) {
			updateMutation.mutate({ id: editingRule.id, ...formData })
		} else {
			createMutation.mutate(formData)
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

	const getModeLabel = (mode: string) => {
		switch (mode) {
			case 'replace':
				return '替换'
			case 'prepend':
				return '添加至头部'
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
						) : rules.length === 0 ? (
							<div className='text-center py-8 text-muted-foreground'>
								暂无规则
							</div>
						) : (
							<Table>
								<TableHeader>
									<TableRow>
										<TableHead>名称</TableHead>
										<TableHead>类型</TableHead>
										<TableHead>模式</TableHead>
										<TableHead>状态</TableHead>
										<TableHead>创建时间</TableHead>
										<TableHead className='text-right'>操作</TableHead>
									</TableRow>
								</TableHeader>
								<TableBody>
									{rules.map((rule) => (
										<TableRow key={rule.id}>
											<TableCell className='font-medium'>
												{rule.name}
											</TableCell>
											<TableCell>
												<Badge variant='outline'>
													{getTypeLabel(rule.type)}
												</Badge>
											</TableCell>
											<TableCell>
												{getModeLabel(rule.mode)}
											</TableCell>
											<TableCell>
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
											</TableCell>
											<TableCell className='text-sm text-muted-foreground'>
												{new Date(rule.created_at).toLocaleString('zh-CN')}
											</TableCell>
											<TableCell className='text-right'>
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
											</TableCell>
										</TableRow>
									))}
								</TableBody>
							</Table>
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
		</main>
	)
}
