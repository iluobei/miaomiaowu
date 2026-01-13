import { useState, useEffect } from 'react'
import { ChevronDown, ChevronUp, HelpCircle } from 'lucide-react'
import { Label } from '@/components/ui/label'
import { Checkbox } from '@/components/ui/checkbox'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Button } from '@/components/ui/button'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { useProxyGroupCategories } from '@/hooks/use-proxy-groups'
import type { PredefinedRuleSetType } from '@/lib/sublink/types'

interface RuleSelectorProps {
  ruleSet: PredefinedRuleSetType
  onRuleSetChange: (value: PredefinedRuleSetType) => void
  selectedCategories: string[]
  onCategoriesChange: (categories: string[]) => void
}

export function RuleSelector({
  ruleSet,
  onRuleSetChange,
  selectedCategories,
  onCategoriesChange,
}: RuleSelectorProps) {
  const [isOpen, setIsOpen] = useState(true)
  const { data: categories = [], isLoading, isError } = useProxyGroupCategories()

  // Track the previous ruleset to detect changes
  const [prevRuleSet, setPrevRuleSet] = useState<PredefinedRuleSetType>(ruleSet)
  // Track whether we've initialized
  const [initialized, setInitialized] = useState(false)

  // Initialize selected categories on first load when categories are available
  useEffect(() => {
    if (!initialized && categories.length > 0 && ruleSet !== 'custom') {
      // Calculate preset categories for initial ruleset
      let presetCategories: string[] = []
      if (ruleSet === 'minimal') {
        presetCategories = categories.filter((c) => c.presets.includes('minimal')).map((c) => c.name)
      } else if (ruleSet === 'balanced') {
        presetCategories = categories.filter((c) => c.presets.includes('balanced')).map((c) => c.name)
      } else if (ruleSet === 'comprehensive') {
        presetCategories = categories.map((c) => c.name)
      }

      if (presetCategories.length > 0) {
        onCategoriesChange(presetCategories)
        setInitialized(true)
      }
    }
  }, [categories, ruleSet, initialized, onCategoriesChange])

  // Update selected categories when ruleset changes (not on initial load)
  useEffect(() => {
    // Only run when ruleSet actually changes after initialization
    if (!initialized) {
      return
    }

    if (ruleSet === prevRuleSet) {
      return
    }

    setPrevRuleSet(ruleSet)

    if (ruleSet !== 'custom') {
      // Calculate preset categories directly to avoid dependency on predefinedRuleSets
      let presetCategories: string[] = []
      if (ruleSet === 'minimal') {
        presetCategories = categories.filter((c) => c.presets.includes('minimal')).map((c) => c.name)
      } else if (ruleSet === 'balanced') {
        presetCategories = categories.filter((c) => c.presets.includes('balanced')).map((c) => c.name)
      } else if (ruleSet === 'comprehensive') {
        presetCategories = categories.map((c) => c.name)
      }
      onCategoriesChange(presetCategories)
    }
  }, [ruleSet, categories, prevRuleSet, initialized, onCategoriesChange])

  const handleCategoryToggle = (categoryName: string) => {
    if (selectedCategories.includes(categoryName)) {
      onCategoriesChange(selectedCategories.filter((c) => c !== categoryName))
    } else {
      // 添加新类别后，按 categories 中的顺序排序
      const newCategories = [...selectedCategories, categoryName]
      const orderedCategories = categories
        .map((c) => c.name)
        .filter((name) => newCategories.includes(name))
      onCategoriesChange(orderedCategories)
    }
  }

  const handleRuleSetChange = (value: string) => {
    const newRuleSet = value as PredefinedRuleSetType
    onRuleSetChange(newRuleSet)

    // Always show categories, expanded by default
    setIsOpen(true)
  }

  return (
    <div className='space-y-2'>
      <div className='flex items-center gap-2'>
        <Label htmlFor='ruleset'>规则选择</Label>
        <TooltipProvider>
          <Tooltip>
            <TooltipTrigger asChild>
              <HelpCircle className='h-4 w-4 text-muted-foreground' />
            </TooltipTrigger>
            <TooltipContent className='max-w-xs'>
              <p>这个功能是从https://github.com/7Sageer/sublink-worker复制粘贴过来的</p>
            </TooltipContent>
          </Tooltip>
        </TooltipProvider>
      </div>

      <Select value={ruleSet} onValueChange={handleRuleSetChange}>
        <SelectTrigger id='ruleset'>
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value='custom'>自定义</SelectItem>
          <SelectItem value='minimal'>极简规则</SelectItem>
          <SelectItem value='balanced'>均衡规则（推荐）</SelectItem>
          <SelectItem value='comprehensive'>完整规则</SelectItem>
        </SelectContent>
      </Select>

      <p className='text-sm text-muted-foreground'>
        {ruleSet === 'custom' && '自定义选择需要的规则类别'}
        {ruleSet === 'minimal' && '已自动选择基础规则，可以手动调整'}
        {ruleSet === 'balanced' && '已自动选择常用规则，可以手动调整'}
        {ruleSet === 'comprehensive' && '已自动选择所有规则，可以手动调整'}
      </p>

      <Collapsible open={isOpen} onOpenChange={setIsOpen}>
        <div className='rounded-lg border p-4'>
          <div className='mb-3 flex items-center justify-between'>
            <p className='text-sm font-medium'>
              已选择 {selectedCategories.length} 个类别
            </p>
            <CollapsibleTrigger asChild>
              <Button variant='ghost' size='sm'>
                {isOpen ? (
                  <ChevronUp className='h-4 w-4' />
                ) : (
                  <ChevronDown className='h-4 w-4' />
                )}
              </Button>
            </CollapsibleTrigger>
          </div>

          <CollapsibleContent>
            {isLoading && (
              <p className='text-sm text-muted-foreground'>正在加载规则分类...</p>
            )}
            {isError && (
              <p className='text-sm text-destructive'>
                无法加载规则分类，请稍后重试或联系管理员。
              </p>
            )}
            {!isLoading && !isError && categories.length === 0 && (
              <p className='text-sm text-muted-foreground'>暂无可用的规则分类</p>
            )}
            {!isLoading && !isError && categories.length > 0 && (
              <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3'>
                {categories.map((category) => (
                  <div
                    key={category.name}
                    className='flex cursor-pointer items-center space-x-2'
                    onClick={() => handleCategoryToggle(category.name)}
                  >
                    <Checkbox
                      id={`category-${category.name}`}
                      checked={selectedCategories.includes(category.name)}
                      onCheckedChange={() => {}}
                    />
                    <div className='flex items-center gap-1.5 text-sm leading-none'>
                      <span>{category.icon}</span>
                      <span>{category.label}</span>
                    </div>
                  </div>
                ))}
              </div>
            )}
          </CollapsibleContent>
        </div>
      </Collapsible>
    </div>
  )
}
