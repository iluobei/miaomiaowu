import { useState, useMemo } from 'react'
import { ChevronDown, ChevronUp, Search, X, Edit2, Check, Plus, Settings2 } from 'lucide-react'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from '@/components/ui/sheet'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'
import { Twemoji } from '@/components/twemoji'

interface ProxyGroup {
  name: string
  type: string
  proxies: string[]
  url?: string
  interval?: number
  strategy?: 'round-robin' | 'consistent-hashing' | 'sticky-sessions'
  use?: string[]
}

interface Node {
  node_name: string
  tag?: string
  [key: string]: any
}

// 特殊节点列表
const SPECIAL_NODES = ['♻️ 自动选择', '🚀 节点选择', 'DIRECT', 'REJECT']

interface MobileEditNodesDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  proxyGroups: ProxyGroup[]
  availableNodes: string[]
  allNodes: Node[]
  onProxyGroupsChange: (groups: ProxyGroup[]) => void
  onSave: () => void
  onRemoveNodeFromGroup: (groupName: string, nodeIndex: number) => void
  onRemoveGroup: (groupName: string) => void
  onRenameGroup: (oldName: string, newName: string) => void
  showSpecialNodesAtBottom?: boolean  // 是否在底部显示特殊节点
  proxyProviderConfigs?: Array<{ id: number; name: string }>  // 节点集合配置列表
}

export function MobileEditNodesDialog({
  open,
  onOpenChange,
  proxyGroups,
  availableNodes,
  allNodes,
  onProxyGroupsChange,
  onSave,
  onRemoveNodeFromGroup,
  onRemoveGroup,
  onRenameGroup,
  showSpecialNodesAtBottom = false,
  proxyProviderConfigs = [],
}: MobileEditNodesDialogProps) {
  const [expandedGroups, setExpandedGroups] = useState<Set<string>>(new Set())
  const [editingGroupName, setEditingGroupName] = useState<string | null>(null)
  const [editingGroupNewName, setEditingGroupNewName] = useState('')
  const [editSheetOpen, setEditSheetOpen] = useState(false)
  const [currentEditingGroup, setCurrentEditingGroup] = useState<string | null>(null)
  const [searchQuery, setSearchQuery] = useState('')
  const [selectedTag, setSelectedTag] = useState<string>('all')

  // 获取所有标签
  const allTags = useMemo(() => {
    const tags = new Set<string>()
    allNodes.forEach(node => {
      if (node.tag) {
        tags.add(node.tag)
      }
    })
    return Array.from(tags).sort()
  }, [allNodes])

  // 过滤可用节点
  const filteredAvailableNodes = useMemo(() => {
    return availableNodes.filter(nodeName => {
      const node = allNodes.find(n => n.node_name === nodeName)
      if (!node) return false

      // 搜索过滤
      const matchesSearch = nodeName.toLowerCase().includes(searchQuery.toLowerCase())
      if (!matchesSearch) return false

      // 标签过滤
      if (selectedTag === 'all') return true
      return node.tag === selectedTag
    })
  }, [availableNodes, allNodes, searchQuery, selectedTag])

  // 切换分组展开/折叠
  const toggleGroup = (groupName: string) => {
    const newExpanded = new Set(expandedGroups)
    if (newExpanded.has(groupName)) {
      newExpanded.delete(groupName)
    } else {
      newExpanded.add(groupName)
    }
    setExpandedGroups(newExpanded)
  }

  // 开始编辑分组名称
  const startEditGroupName = (groupName: string) => {
    setEditingGroupName(groupName)
    setEditingGroupNewName(groupName)
  }

  // 确认重命名
  const confirmRename = () => {
    if (editingGroupName && editingGroupNewName.trim() && editingGroupNewName !== editingGroupName) {
      onRenameGroup(editingGroupName, editingGroupNewName.trim())
    }
    setEditingGroupName(null)
    setEditingGroupNewName('')
  }

  // 取消重命名
  const cancelRename = () => {
    setEditingGroupName(null)
    setEditingGroupNewName('')
  }

  // 打开编辑抽屉
  const openEditSheet = (groupName: string) => {
    setCurrentEditingGroup(groupName)
    setEditSheetOpen(true)
    setSearchQuery('')
    setSelectedTag('all')
  }

  // 关闭编辑抽屉
  const closeEditSheet = () => {
    setEditSheetOpen(false)
    setCurrentEditingGroup(null)
    setSearchQuery('')
    setSelectedTag('all')
  }

  // 检查节点是否在当前编辑的组中
  const isNodeInCurrentGroup = (nodeName: string) => {
    if (!currentEditingGroup) return false
    const group = proxyGroups.find(g => g.name === currentEditingGroup)
    return group?.proxies.includes(nodeName) || false
  }

  // 检查节点集合是否在当前编辑的组中
  const isProviderInCurrentGroup = (providerName: string) => {
    if (!currentEditingGroup) return false
    const group = proxyGroups.find(g => g.name === currentEditingGroup)
    return group?.use?.includes(providerName) || false
  }

  // 切换节点选中状态
  const toggleNodeInGroup = (nodeName: string) => {
    if (!currentEditingGroup) return

    const groupIndex = proxyGroups.findIndex(g => g.name === currentEditingGroup)
    if (groupIndex === -1) return

    const newGroups = [...proxyGroups]
    const group = newGroups[groupIndex]
    const nodeIndex = group.proxies.indexOf(nodeName)

    if (nodeIndex > -1) {
      // 移除节点
      group.proxies = group.proxies.filter((_, idx) => idx !== nodeIndex)
    } else {
      // 添加节点
      group.proxies = [...group.proxies, nodeName]
    }

    onProxyGroupsChange(newGroups)
  }

  // 切换节点集合选中状态
  const toggleProviderInGroup = (providerName: string) => {
    if (!currentEditingGroup) return

    const groupIndex = proxyGroups.findIndex(g => g.name === currentEditingGroup)
    if (groupIndex === -1) return

    const newGroups = [...proxyGroups]
    const group = newGroups[groupIndex]
    const useArray = group.use || []
    const providerIndex = useArray.indexOf(providerName)

    if (providerIndex > -1) {
      // 移除节点集合
      group.use = useArray.filter((_, idx) => idx !== providerIndex)
      if (group.use.length === 0) {
        delete group.use
      }
    } else {
      // 添加节点集合
      group.use = [...useArray, providerName]
    }

    onProxyGroupsChange(newGroups)
  }

  // 添加新代理组
  const addNewGroup = () => {
    const newGroupName = `新分组 ${proxyGroups.length + 1}`
    const newGroup: ProxyGroup = {
      name: newGroupName,
      type: 'select',
      proxies: []
    }
    onProxyGroupsChange([...proxyGroups, newGroup])
    setExpandedGroups(new Set([...expandedGroups, newGroupName]))
  }

  // 代理组类型配置
  const proxyTypes = [
    { value: 'select', label: '手动选择', hasUrl: false, hasStrategy: false },
    { value: 'url-test', label: '自动选择', hasUrl: true, hasStrategy: false },
    { value: 'fallback', label: '自动回退', hasUrl: true, hasStrategy: false },
    { value: 'load-balance', label: '负载均衡', hasUrl: true, hasStrategy: true },
  ]

  // 处理代理组类型变更
  const handleGroupTypeChange = (groupName: string, newType: string) => {
    const typeConfig = proxyTypes.find(t => t.value === newType)
    const newGroups = proxyGroups.map(g => {
      if (g.name !== groupName) return g

      const updatedGroup: ProxyGroup = {
        ...g,
        type: newType,
      }

      if (typeConfig?.hasUrl) {
        updatedGroup.url = g.url || 'https://www.gstatic.com/generate_204'
        updatedGroup.interval = g.interval || 300
      } else {
        delete updatedGroup.url
        delete updatedGroup.interval
      }

      if (typeConfig?.hasStrategy) {
        updatedGroup.strategy = g.strategy || 'round-robin'
      } else {
        delete updatedGroup.strategy
      }

      return updatedGroup
    })
    onProxyGroupsChange(newGroups)
  }

  // 处理负载均衡策略变更
  const handleStrategyChange = (groupName: string, strategy: ProxyGroup['strategy']) => {
    const newGroups = proxyGroups.map(g => {
      if (g.name !== groupName) return g
      return { ...g, strategy }
    })
    onProxyGroupsChange(newGroups)
  }

  // 获取类型显示名称
  const getTypeLabel = (type: string) => {
    return proxyTypes.find(t => t.value === type)?.label || type
  }

  return (
    <>
      <Sheet open={open} onOpenChange={onOpenChange}>
        <SheetContent side="bottom" className="h-[90vh] flex flex-col p-4">
          <SheetHeader className="shrink-0">
            <SheetTitle>手动分组节点</SheetTitle>
            <SheetDescription>
              点击分组展开查看节点，点击编辑按钮添加或移除节点
            </SheetDescription>
          </SheetHeader>

          <div className="flex-1 overflow-y-auto -mx-2 px-2 pt-4">
            <div className="space-y-3">
              {proxyGroups.map((group) => (
                <Card key={group.name} className="overflow-hidden">
                  <CardContent className="p-0">
                    {/* 分组头部 */}
                    <div className="p-3 bg-muted/30 space-y-2">
                      {/* 第一行：代理组名称、类型、节点数量、删除按钮 */}
                      <div className="flex items-center gap-2">
                        {editingGroupName === group.name ? (
                          <div className="flex items-center gap-1 flex-1 min-w-0">
                            <Input
                              value={editingGroupNewName}
                              onChange={(e) => setEditingGroupNewName(e.target.value)}
                              className="h-7 text-sm"
                              autoFocus
                            />
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-6 w-6 p-0"
                              onClick={confirmRename}
                            >
                              <Check className="h-4 w-4 text-green-600" />
                            </Button>
                            <Button
                              variant="ghost"
                              size="sm"
                              className="h-6 w-6 p-0"
                              onClick={cancelRename}
                            >
                              <X className="h-4 w-4 text-red-600" />
                            </Button>
                          </div>
                        ) : (
                          <>
                            <span
                              className="font-medium text-sm flex-1 min-w-0 cursor-pointer"
                              onClick={() => toggleGroup(group.name)}
                            >
                              <Twemoji>{group.name}</Twemoji>
                            </span>
                            <Badge variant="secondary" className="text-xs shrink-0">
                              {getTypeLabel(group.type)}
                            </Badge>
                            <Badge variant="outline" className="text-xs shrink-0">
                              {group.proxies.length}{(group.use?.length || 0) > 0 && `+${group.use?.length}`}
                            </Badge>
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-7 w-7 p-0 shrink-0 text-destructive hover:text-destructive"
                              onClick={() => onRemoveGroup(group.name)}
                            >
                              <X className="h-4 w-4" />
                            </Button>
                          </>
                        )}
                      </div>

                      {/* 第二行：操作按钮 */}
                      {editingGroupName !== group.name && (
                        <div className="flex items-center justify-between gap-1.5">
                          <div className="flex items-center gap-1.5">
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-7 text-xs px-2"
                              onClick={() => startEditGroupName(group.name)}
                            >
                              <Edit2 className="h-3 w-3 mr-1" />
                              重命名
                            </Button>
                            <Popover>
                              <PopoverTrigger asChild>
                                <Button
                                  variant="outline"
                                  size="sm"
                                  className="h-7 text-xs px-2"
                                  title="切换代理组类型"
                                >
                                  <Settings2 className="h-3 w-3 mr-1" />
                                  类型
                                </Button>
                              </PopoverTrigger>
                              <PopoverContent className="w-48 p-2" align="start">
                                <div className="space-y-1">
                                  {proxyTypes.map(({ value, label }) => (
                                    <Button
                                      key={value}
                                      variant={group.type === value ? 'default' : 'ghost'}
                                      size="sm"
                                      className="w-full justify-start"
                                      onClick={() => handleGroupTypeChange(group.name, value)}
                                    >
                                      {label}
                                    </Button>
                                  ))}

                                  {group.type === 'load-balance' && (
                                    <div className="pt-2 border-t">
                                      <p className="text-xs text-muted-foreground mb-1">策略</p>
                                      <Select
                                        value={group.strategy || 'round-robin'}
                                        onValueChange={(value) => handleStrategyChange(group.name, value as ProxyGroup['strategy'])}
                                      >
                                        <SelectTrigger className="h-8 text-xs">
                                          <SelectValue />
                                        </SelectTrigger>
                                        <SelectContent>
                                          <SelectItem value="round-robin">轮询</SelectItem>
                                          <SelectItem value="consistent-hashing">一致性哈希</SelectItem>
                                          <SelectItem value="sticky-sessions">粘性会话</SelectItem>
                                        </SelectContent>
                                      </Select>
                                    </div>
                                  )}
                                </div>
                              </PopoverContent>
                            </Popover>
                            <Button
                              variant="outline"
                              size="sm"
                              className="h-7 text-xs"
                              onClick={() => openEditSheet(group.name)}
                            >
                              添加节点
                            </Button>
                          </div>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 items-center text-xs px-2"
                            onClick={() => toggleGroup(group.name)}
                          >
                            {expandedGroups.has(group.name) ? (
                              <>
                                <ChevronUp className="h-3 w-3" />
                              </>
                            ) : (
                              <>
                                <ChevronDown className="h-3 w-3" />
                              </>
                            )}
                          </Button>
                        </div>
                      )}
                    </div>

                    {/* 分组内容（展开时显示） */}
                    {expandedGroups.has(group.name) && (
                      <div className="p-3 space-y-1.5">
                        {group.proxies.length === 0 && (group.use?.length || 0) === 0 ? (
                          <p className="text-sm text-muted-foreground text-center py-2">
                            暂无节点，点击"添加节点"按钮添加
                          </p>
                        ) : (
                          <>
                            {group.proxies.map((proxy, idx) => (
                              <div
                                key={idx}
                                className="flex items-center justify-between gap-2 p-2 rounded border hover:bg-accent"
                              >
                                <span className="text-sm truncate flex-1"><Twemoji>{proxy}</Twemoji></span>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="h-6 w-6 p-0 shrink-0"
                                  onClick={() => onRemoveNodeFromGroup(group.name, idx)}
                                >
                                  <X className="h-4 w-4 text-muted-foreground hover:text-destructive" />
                                </Button>
                              </div>
                            ))}
                            {/* 节点集合（use）显示 */}
                            {(group.use || []).map((providerName, idx) => (
                              <div
                                key={`use-${idx}`}
                                className="flex items-center justify-between gap-2 p-2 rounded border border-purple-300 dark:border-purple-700 bg-purple-50 dark:bg-purple-950/20"
                              >
                                <span className="text-sm truncate flex-1 text-purple-700 dark:text-purple-300">📦 {providerName}</span>
                                <Button
                                  variant="ghost"
                                  size="sm"
                                  className="h-6 w-6 p-0 shrink-0"
                                  onClick={() => {
                                    const newGroups = proxyGroups.map(g => {
                                      if (g.name === group.name) {
                                        const newUse = (g.use || []).filter((_, i) => i !== idx)
                                        return { ...g, use: newUse.length > 0 ? newUse : undefined }
                                      }
                                      return g
                                    })
                                    onProxyGroupsChange(newGroups)
                                  }}
                                >
                                  <X className="h-4 w-4 text-purple-400 hover:text-destructive" />
                                </Button>
                              </div>
                            ))}
                          </>
                        )}
                      </div>
                    )}
                  </CardContent>
                </Card>
              ))}

              {/* 添加代理组按钮 */}
              <Button
                variant="outline"
                className="w-full"
                onClick={addNewGroup}
              >
                <Plus className="mr-2 h-4 w-4" />
                添加代理组
              </Button>
            </div>
          </div>

          <SheetFooter className="shrink-0 pt-4">
            <Button variant="outline" onClick={() => onOpenChange(false)}>
              取消
            </Button>
            <Button onClick={() => { onSave(); onOpenChange(false); }}>
              确定
            </Button>
          </SheetFooter>
        </SheetContent>
      </Sheet>

      {/* 编辑节点的底部抽屉 */}
      <Sheet open={editSheetOpen} onOpenChange={setEditSheetOpen}>
        <SheetContent side="bottom" className="h-[80vh] flex flex-col p-4">
          <SheetHeader className="shrink-0">
            <SheetTitle>编辑分组: {currentEditingGroup}</SheetTitle>
            <SheetDescription>
              选择要添加到此分组的节点
            </SheetDescription>
          </SheetHeader>

          <div className="flex-1 flex flex-col space-y-3 overflow-hidden">
            {/* 搜索框 */}
            <div className="relative shrink-0">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="搜索节点..."
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                className="pl-9"
              />
            </div>

            {/* 标签过滤 */}
            {allTags.length > 0 && (
              <div className="flex flex-wrap gap-2 shrink-0">
                <Badge
                  variant={selectedTag === 'all' ? 'default' : 'outline'}
                  className="cursor-pointer"
                  onClick={() => setSelectedTag('all')}
                >
                  全部
                </Badge>
                {allTags.map((tag) => (
                  <Badge
                    key={tag}
                    variant={selectedTag === tag ? 'default' : 'outline'}
                    className="cursor-pointer"
                    onClick={() => setSelectedTag(tag)}
                  >
                    {tag}
                  </Badge>
                ))}
              </div>
            )}

            {/* 节点列表 */}
            <div className="flex-1 overflow-y-auto -mx-2 px-2 pt-2">
              <div className="space-y-2">
                {filteredAvailableNodes.length === 0 && proxyProviderConfigs.length === 0 && !showSpecialNodesAtBottom ? (
                  <p className="text-sm text-muted-foreground text-center py-8">
                    {searchQuery || selectedTag !== 'all' ? '没有找到匹配的节点' : '暂无可用节点'}
                  </p>
                ) : (
                  <>
                    {filteredAvailableNodes.map((nodeName) => {
                      const node = allNodes.find(n => n.node_name === nodeName)
                      const isSelected = isNodeInCurrentGroup(nodeName)

                      return (
                        <div
                          key={nodeName}
                          className={`flex items-center gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                            isSelected ? 'bg-accent border-primary' : 'hover:bg-accent/50'
                          }`}
                          onClick={() => toggleNodeInGroup(nodeName)}
                        >
                          <Checkbox
                            checked={isSelected}
                            onCheckedChange={() => toggleNodeInGroup(nodeName)}
                            onClick={(e) => e.stopPropagation()}
                          />
                          <div className="flex-1 min-w-0">
                            <p className="text-sm font-medium truncate"><Twemoji>{nodeName}</Twemoji></p>
                            {node?.tag && (
                              <Badge variant="secondary" className="text-xs mt-1">
                                {node.tag}
                              </Badge>
                            )}
                          </div>
                        </div>
                      )
                    })}

                    {/* 节点集合区块 */}
                    {proxyProviderConfigs.length > 0 && (
                      <>
                        <div className='pt-3 pb-1 border-t mt-3'>
                          <span className='text-xs text-purple-600 dark:text-purple-400 font-medium'>📦 节点集合</span>
                        </div>
                        {proxyProviderConfigs.map((config) => {
                          const isSelected = isProviderInCurrentGroup(config.name)
                          return (
                            <div
                              key={`provider-${config.id}`}
                              className={`flex items-center gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                                isSelected
                                  ? 'bg-purple-100 dark:bg-purple-950/40 border-purple-500'
                                  : 'border-purple-300 dark:border-purple-700 bg-purple-50 dark:bg-purple-950/20 hover:bg-purple-100 dark:hover:bg-purple-900/30'
                              }`}
                              onClick={() => toggleProviderInGroup(config.name)}
                            >
                              <Checkbox
                                checked={isSelected}
                                onCheckedChange={() => toggleProviderInGroup(config.name)}
                                onClick={(e) => e.stopPropagation()}
                                className="border-purple-400 data-[state=checked]:bg-purple-600 data-[state=checked]:border-purple-600"
                              />
                              <div className="flex-1 min-w-0">
                                <p className="text-sm font-medium truncate text-purple-700 dark:text-purple-300">
                                  📦 {config.name}
                                </p>
                              </div>
                            </div>
                          )
                        })}
                      </>
                    )}

                    {/* 特殊节点区块 */}
                    {showSpecialNodesAtBottom && (
                      <>
                        <div className='pt-3 pb-1 border-t mt-3'>
                          <span className='text-xs text-muted-foreground font-medium'>特殊节点</span>
                        </div>
                        {SPECIAL_NODES.map((nodeName) => {
                          const isSelected = isNodeInCurrentGroup(nodeName)
                          return (
                            <div
                              key={`special-${nodeName}`}
                              className={`flex items-center gap-3 p-3 rounded-lg border cursor-pointer transition-colors ${
                                isSelected ? 'bg-accent border-primary' : 'hover:bg-accent/50'
                              }`}
                              onClick={() => toggleNodeInGroup(nodeName)}
                            >
                              <Checkbox
                                checked={isSelected}
                                onCheckedChange={() => toggleNodeInGroup(nodeName)}
                                onClick={(e) => e.stopPropagation()}
                              />
                              <div className="flex-1 min-w-0">
                                <p className="text-sm font-medium truncate"><Twemoji>{nodeName}</Twemoji></p>
                              </div>
                            </div>
                          )
                        })}
                      </>
                    )}
                  </>
                )}
              </div>
            </div>
          </div>

          <SheetFooter className="shrink-0">
            <div className="flex items-center justify-between w-full">
              <span className="text-sm text-muted-foreground">
                已选择 {proxyGroups.find(g => g.name === currentEditingGroup)?.proxies.length || 0} 个节点
                {(proxyGroups.find(g => g.name === currentEditingGroup)?.use?.length || 0) > 0 && (
                  <span className="text-purple-600 dark:text-purple-400">
                    {' '}+ {proxyGroups.find(g => g.name === currentEditingGroup)?.use?.length} 个集合
                  </span>
                )}
              </span>
              <Button onClick={closeEditSheet}>完成</Button>
            </div>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </>
  )
}
