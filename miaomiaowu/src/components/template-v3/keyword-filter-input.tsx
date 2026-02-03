import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Badge } from '@/components/ui/badge'
import { keywordsToRegex } from '@/lib/template-v3-utils'

interface KeywordFilterInputProps {
  value: string
  onChange: (value: string) => void
  label: string
  placeholder?: string
  description?: string
}

export function KeywordFilterInput({
  value,
  onChange,
  label,
  placeholder = '输入关键词，用逗号分隔',
  description,
}: KeywordFilterInputProps) {
  const regex = keywordsToRegex(value)

  return (
    <div className="space-y-2">
      <Label>{label}</Label>
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={placeholder}
      />
      {description && (
        <p className="text-xs text-muted-foreground">{description}</p>
      )}
      {regex && (
        <div className="flex items-center gap-2">
          <span className="text-xs text-muted-foreground">正则:</span>
          <Badge variant="secondary" className="font-mono text-xs">
            {regex}
          </Badge>
        </div>
      )}
    </div>
  )
}
