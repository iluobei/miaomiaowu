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
import { RULE_CATEGORIES, PREDEFINED_RULE_SETS } from '@/lib/sublink/predefined-rules'
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

  // Update selected categories when ruleset changes
  useEffect(() => {
    if (ruleSet !== 'custom') {
      const presetCategories = PREDEFINED_RULE_SETS[ruleSet] || []
      onCategoriesChange(presetCategories)
    }
  }, [ruleSet, onCategoriesChange])

  const handleCategoryToggle = (categoryName: string) => {
    if (selectedCategories.includes(categoryName)) {
      onCategoriesChange(selectedCategories.filter((c) => c !== categoryName))
    } else {
      onCategoriesChange([...selectedCategories, categoryName])
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
            <div className='grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3'>
              {RULE_CATEGORIES.map((category) => (
                <div key={category.name} className='flex items-center space-x-2'>
                  <Checkbox
                    id={`category-${category.name}`}
                    checked={selectedCategories.includes(category.name)}
                    onCheckedChange={() => handleCategoryToggle(category.name)}
                  />
                  <label
                    htmlFor={`category-${category.name}`}
                    className='flex cursor-pointer items-center gap-1.5 text-sm leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70'
                  >
                    <span>{category.icon}</span>
                    <span>{category.label}</span>
                  </label>
                </div>
              ))}
            </div>
          </CollapsibleContent>
        </div>
      </Collapsible>
    </div>
  )
}
