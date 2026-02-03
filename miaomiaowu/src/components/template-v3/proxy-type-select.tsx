import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { Popover, PopoverContent, PopoverTrigger } from '@/components/ui/popover'
import { ScrollArea } from '@/components/ui/scroll-area'
import { ChevronDown } from 'lucide-react'
import { PROXY_TYPES, type ProxyType } from '@/lib/template-v3-utils'

interface ProxyTypeSelectProps {
  value: ProxyType[]
  onChange: (types: ProxyType[]) => void
  label: string
  placeholder?: string
}

export function ProxyTypeSelect({
  value,
  onChange,
  label,
  placeholder = '选择代理类型',
}: ProxyTypeSelectProps) {
  const [open, setOpen] = useState(false)

  const handleToggle = (type: ProxyType) => {
    if (value.includes(type)) {
      onChange(value.filter((t) => t !== type))
    } else {
      onChange([...value, type])
    }
  }

  const handleSelectAll = () => {
    if (value.length === PROXY_TYPES.length) {
      onChange([])
    } else {
      onChange([...PROXY_TYPES])
    }
  }

  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Popover open={open} onOpenChange={setOpen}>
        <PopoverTrigger asChild>
          <Button
            variant="outline"
            className="w-full justify-between font-normal"
          >
            {value.length > 0 ? (
              <span className="flex items-center gap-2">
                <Badge variant="secondary">{value.length}</Badge>
                <span className="text-muted-foreground truncate">
                  {value.slice(0, 3).join(', ')}
                  {value.length > 3 && '...'}
                </span>
              </span>
            ) : (
              <span className="text-muted-foreground">{placeholder}</span>
            )}
            <ChevronDown className="h-4 w-4 opacity-50" />
          </Button>
        </PopoverTrigger>
        <PopoverContent className="w-[200px] p-0" align="start">
          <div className="p-2 border-b">
            <Button
              variant="ghost"
              size="sm"
              className="w-full justify-start"
              onClick={handleSelectAll}
            >
              {value.length === PROXY_TYPES.length ? '取消全选' : '全选'}
            </Button>
          </div>
          <ScrollArea className="h-[200px]">
            <div className="p-2 space-y-1">
              {PROXY_TYPES.map((type) => (
                <div
                  key={type}
                  className="flex items-center space-x-2 p-2 rounded hover:bg-accent cursor-pointer"
                  onClick={() => handleToggle(type)}
                >
                  <Checkbox
                    checked={value.includes(type)}
                    onCheckedChange={() => handleToggle(type)}
                  />
                  <span className="text-sm font-mono">{type}</span>
                </div>
              ))}
            </div>
          </ScrollArea>
        </PopoverContent>
      </Popover>
    </div>
  )
}
