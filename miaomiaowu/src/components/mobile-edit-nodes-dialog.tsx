import { useState, useMemo } from 'react'
import { ChevronDown, ChevronUp, Search, X, Edit2, Check, Plus } from 'lucide-react'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
  SheetFooter,
} from '@/components/ui/sheet'
import { Card, CardContent } from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Checkbox } from '@/components/ui/checkbox'
import { Badge } from '@/components/ui/badge'

interface ProxyGroup {
  name: string
  type: string
  proxies: string[]
}

interface Node {
  node_name: string
  tag?: string
  [key: string]: any
}

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
                    <div className="p-3 bg-muted/30">
                      <div className="flex items-center justify-between gap-2">
                        <div className="flex items-center gap-2 flex-1 min-w-0">
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-6 w-6 p-0"
                            onClick={() => toggleGroup(group.name)}
                          >
                            {expandedGroups.has(group.name) ? (
                              <ChevronUp className="h-4 w-4" />
                            ) : (
                              <ChevronDown className="h-4 w-4" />
                            )}
                          </Button>

                          {editingGroupName === group.name ? (
                            <div className="flex items-center gap-1 flex-1">
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
                                className="font-medium text-sm truncate flex-1 cursor-pointer"
                                onClick={() => toggleGroup(group.name)}
                              >
                                {group.name}
                              </span>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-6 w-6 p-0 shrink-0"
                                onClick={() => startEditGroupName(group.name)}
                              >
                                <Edit2 className="h-3 w-3" />
                              </Button>
                            </>
                          )}
                        </div>

                        <div className="flex items-center gap-2 shrink-0">
                          <Badge variant="secondary" className="text-xs">
                            {group.proxies.length} 个节点
                          </Badge>
                          <Button
                            variant="outline"
                            size="sm"
                            className="h-7 text-xs"
                            onClick={() => openEditSheet(group.name)}
                          >
                            编辑
                          </Button>
                          <Button
                            variant="ghost"
                            size="sm"
                            className="h-6 w-6 p-0 text-destructive"
                            onClick={() => onRemoveGroup(group.name)}
                          >
                            <X className="h-4 w-4" />
                          </Button>
                        </div>
                      </div>
                    </div>

                    {/* 分组内容（展开时显示） */}
                    {expandedGroups.has(group.name) && (
                      <div className="p-3 space-y-1.5">
                        {group.proxies.length === 0 ? (
                          <p className="text-sm text-muted-foreground text-center py-2">
                            暂无节点，点击"编辑"按钮添加
                          </p>
                        ) : (
                          group.proxies.map((proxy, idx) => (
                            <div
                              key={idx}
                              className="flex items-center justify-between gap-2 p-2 rounded bg-muted/50"
                            >
                              <span className="text-sm truncate flex-1">{proxy}</span>
                              <Button
                                variant="ghost"
                                size="sm"
                                className="h-6 w-6 p-0 shrink-0 text-destructive"
                                onClick={() => onRemoveNodeFromGroup(group.name, idx)}
                              >
                                <X className="h-3 w-3" />
                              </Button>
                            </div>
                          ))
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
                {filteredAvailableNodes.length === 0 ? (
                  <p className="text-sm text-muted-foreground text-center py-8">
                    {searchQuery || selectedTag !== 'all' ? '没有找到匹配的节点' : '暂无可用节点'}
                  </p>
                ) : (
                  filteredAvailableNodes.map((nodeName) => {
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
                          <p className="text-sm font-medium truncate">{nodeName}</p>
                          {node?.tag && (
                            <Badge variant="secondary" className="text-xs mt-1">
                              {node.tag}
                            </Badge>
                          )}
                        </div>
                      </div>
                    )
                  })
                )}
              </div>
            </div>
          </div>

          <SheetFooter className="shrink-0">
            <div className="flex items-center justify-between w-full">
              <span className="text-sm text-muted-foreground">
                已选择 {proxyGroups.find(g => g.name === currentEditingGroup)?.proxies.length || 0} 个节点
              </span>
              <Button onClick={closeEditSheet}>完成</Button>
            </div>
          </SheetFooter>
        </SheetContent>
      </Sheet>
    </>
  )
}
