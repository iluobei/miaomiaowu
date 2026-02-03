import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Switch } from '@/components/ui/switch'
import { Badge } from '@/components/ui/badge'
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from '@/components/ui/collapsible'
import { ChevronDown, ChevronUp, Trash2, GripVertical } from 'lucide-react'
import { useState } from 'react'
import { KeywordFilterInput } from './keyword-filter-input'
import { ProxyTypeSelect } from './proxy-type-select'
import { PROXY_GROUP_TYPES, type ProxyGroupFormState, type ProxyGroupType } from '@/lib/template-v3-utils'

interface ProxyGroupEditorProps {
  group: ProxyGroupFormState
  index: number
  onChange: (index: number, group: ProxyGroupFormState) => void
  onDelete: (index: number) => void
  onMoveUp?: (index: number) => void
  onMoveDown?: (index: number) => void
  isFirst?: boolean
  isLast?: boolean
}

const GROUP_TYPE_LABELS: Record<ProxyGroupType, string> = {
  'select': '手动选择',
  'url-test': '自动测速',
  'fallback': '故障转移',
  'load-balance': '负载均衡',
  'relay': '链式代理',
}

export function ProxyGroupEditor({
  group,
  index,
  onChange,
  onDelete,
  onMoveUp,
  onMoveDown,
  isFirst = false,
  isLast = false,
}: ProxyGroupEditorProps) {
  const [isOpen, setIsOpen] = useState(false)

  const updateField = <K extends keyof ProxyGroupFormState>(
    field: K,
    value: ProxyGroupFormState[K]
  ) => {
    onChange(index, { ...group, [field]: value })
  }

  const needsUrlTestOptions = ['url-test', 'fallback', 'load-balance'].includes(group.type)

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <div className="border rounded-lg">
        <CollapsibleTrigger asChild>
          <div className="flex items-center justify-between p-3 cursor-pointer hover:bg-accent/50">
            <div className="flex items-center gap-3">
              <GripVertical className="h-4 w-4 text-muted-foreground" />
              <span className="font-medium">{group.name}</span>
              <Badge variant="outline" className="text-xs">
                {GROUP_TYPE_LABELS[group.type]}
              </Badge>
              {group.includeAllProxies && (
                <Badge variant="secondary" className="text-xs">全部节点</Badge>
              )}
              {group.filterKeywords && (
                <Badge variant="secondary" className="text-xs">有过滤</Badge>
              )}
            </div>
            <div className="flex items-center gap-1">
              {onMoveUp && !isFirst && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  onClick={(e) => { e.stopPropagation(); onMoveUp(index) }}
                >
                  <ChevronUp className="h-4 w-4" />
                </Button>
              )}
              {onMoveDown && !isLast && (
                <Button
                  variant="ghost"
                  size="icon"
                  className="h-8 w-8"
                  onClick={(e) => { e.stopPropagation(); onMoveDown(index) }}
                >
                  <ChevronDown className="h-4 w-4" />
                </Button>
              )}
              <Button
                variant="ghost"
                size="icon"
                className="h-8 w-8 text-destructive"
                onClick={(e) => { e.stopPropagation(); onDelete(index) }}
              >
                <Trash2 className="h-4 w-4" />
              </Button>
              <ChevronDown className={`h-4 w-4 transition-transform ${isOpen ? 'rotate-180' : ''}`} />
            </div>
          </div>
        </CollapsibleTrigger>

        <CollapsibleContent>
          <div className="p-4 pt-0 space-y-4 border-t">
            {/* Row 1: Name and Type */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div className="space-y-2">
                <Label>组名称</Label>
                <Input
                  value={group.name}
                  onChange={(e) => updateField('name', e.target.value)}
                  placeholder="代理组名称"
                />
              </div>
              <div className="space-y-2">
                <Label>组类型</Label>
                <Select
                  value={group.type}
                  onValueChange={(v) => updateField('type', v as ProxyGroupType)}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {PROXY_GROUP_TYPES.map((type) => (
                      <SelectItem key={type} value={type}>
                        {GROUP_TYPE_LABELS[type]}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
            </div>

            {/* Row 2: Include Options */}
            <div className="space-y-2">
              <Label>节点来源</Label>
              <div className="flex flex-wrap gap-4">
                <div className="flex items-center gap-2">
                  <Switch
                    checked={group.includeAll}
                    onCheckedChange={(v) => updateField('includeAll', v)}
                  />
                  <span className="text-sm">include-all</span>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    checked={group.includeAllProxies}
                    onCheckedChange={(v) => updateField('includeAllProxies', v)}
                  />
                  <span className="text-sm">include-all-proxies</span>
                </div>
                <div className="flex items-center gap-2">
                  <Switch
                    checked={group.includeAllProviders}
                    onCheckedChange={(v) => updateField('includeAllProviders', v)}
                  />
                  <span className="text-sm">include-all-providers</span>
                </div>
              </div>
            </div>

            {/* Row 3-4: Filter Keywords */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <KeywordFilterInput
                label="筛选关键词 (filter)"
                value={group.filterKeywords}
                onChange={(v) => updateField('filterKeywords', v)}
                placeholder="香港, HK, 港"
                description="匹配节点名称，用逗号分隔"
              />
              <KeywordFilterInput
                label="排除关键词 (exclude-filter)"
                value={group.excludeFilterKeywords}
                onChange={(v) => updateField('excludeFilterKeywords', v)}
                placeholder="游戏, IPLC"
                description="排除匹配的节点"
              />
            </div>

            {/* Row 5: Type Filters */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <ProxyTypeSelect
                label="包含类型 (include-type)"
                value={group.includeTypes}
                onChange={(v) => updateField('includeTypes', v)}
                placeholder="选择要包含的代理类型"
              />
              <ProxyTypeSelect
                label="排除类型 (exclude-type)"
                value={group.excludeTypes}
                onChange={(v) => updateField('excludeTypes', v)}
                placeholder="选择要排除的代理类型"
              />
            </div>

            {/* Row 6: URL Test Options */}
            {needsUrlTestOptions && (
              <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
                <div className="space-y-2">
                  <Label>测试 URL</Label>
                  <Input
                    value={group.url}
                    onChange={(e) => updateField('url', e.target.value)}
                    placeholder="https://www.gstatic.com/generate_204"
                  />
                </div>
                <div className="space-y-2">
                  <Label>测试间隔 (秒)</Label>
                  <Input
                    type="number"
                    value={group.interval}
                    onChange={(e) => updateField('interval', parseInt(e.target.value) || 300)}
                  />
                </div>
                {group.type !== 'load-balance' && (
                  <div className="space-y-2">
                    <Label>容差 (ms)</Label>
                    <Input
                      type="number"
                      value={group.tolerance}
                      onChange={(e) => updateField('tolerance', parseInt(e.target.value) || 50)}
                    />
                  </div>
                )}
              </div>
            )}
          </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}
