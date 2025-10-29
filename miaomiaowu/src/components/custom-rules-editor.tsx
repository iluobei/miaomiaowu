import { useState } from 'react'
import { Plus, Trash2, ChevronDown, ChevronUp } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import type { CustomRule } from '@/lib/sublink/types'

interface CustomRulesEditorProps {
  rules: CustomRule[]
  onChange: (rules: CustomRule[]) => void
}

export function CustomRulesEditor({ rules, onChange }: CustomRulesEditorProps) {
  const [isOpen, setIsOpen] = useState(false)

  const handleAddRule = () => {
    onChange([
      ...rules,
      {
        name: '',
        site: '',
        ip: '',
        domain_suffix: '',
        domain_keyword: '',
        ip_cidr: '',
        protocol: '',
      },
    ])
    setIsOpen(true)
  }

  const handleRemoveRule = (index: number) => {
    onChange(rules.filter((_, i) => i !== index))
  }

  const handleUpdateRule = (index: number, field: keyof CustomRule, value: string) => {
    const newRules = [...rules]
    newRules[index] = { ...newRules[index], [field]: value }
    onChange(newRules)
  }

  return (
    <Collapsible open={isOpen} onOpenChange={setIsOpen}>
      <Card>
        <CardHeader>
          <div className='flex items-center justify-between'>
            <div>
              <CardTitle>自定义规则</CardTitle>
              <CardDescription>
                添加自定义分流规则，支持域名、IP、协议等多种匹配方式
              </CardDescription>
            </div>
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
        </CardHeader>

        <CollapsibleContent>
          <CardContent className='space-y-4'>
            {rules.length === 0 ? (
              <div className='rounded-lg border border-dashed p-8 text-center'>
                <p className='mb-4 text-sm text-muted-foreground'>
                  还没有自定义规则，点击下方按钮添加
                </p>
                <Button onClick={handleAddRule} variant='outline'>
                  <Plus className='mr-2 h-4 w-4' />
                  添加规则
                </Button>
              </div>
            ) : (
              <>
                <div className='space-y-4'>
                  {rules.map((rule, index) => (
                    <Card key={index} className='border-muted'>
                      <CardHeader className='pb-3'>
                        <div className='flex items-center justify-between'>
                          <CardTitle className='text-base'>
                            规则 #{index + 1}
                            {rule.name && ` - ${rule.name}`}
                          </CardTitle>
                          <Button
                            variant='ghost'
                            size='sm'
                            onClick={() => handleRemoveRule(index)}
                          >
                            <Trash2 className='h-4 w-4 text-destructive' />
                          </Button>
                        </div>
                      </CardHeader>
                      <CardContent className='space-y-3'>
                        <div className='grid gap-3 sm:grid-cols-2'>
                          <div className='space-y-1.5'>
                            <Label htmlFor={`rule-${index}-name`}>
                              出站名称 <span className='text-destructive'>*</span>
                            </Label>
                            <Input
                              id={`rule-${index}-name`}
                              placeholder='例如: Netflix, OpenAI'
                              value={rule.name}
                              onChange={(e) =>
                                handleUpdateRule(index, 'name', e.target.value)
                              }
                            />
                            <p className='text-xs text-muted-foreground'>
                              将创建对应的策略组
                            </p>
                          </div>

                          <div className='space-y-1.5'>
                            <Label htmlFor={`rule-${index}-site`}>GeoSite</Label>
                            <Input
                              id={`rule-${index}-site`}
                              placeholder='例如: netflix, openai'
                              value={rule.site || ''}
                              onChange={(e) =>
                                handleUpdateRule(index, 'site', e.target.value)
                              }
                            />
                            <p className='text-xs text-muted-foreground'>
                              多个用逗号分隔
                            </p>
                          </div>
                        </div>

                        <div className='grid gap-3 sm:grid-cols-2'>
                          <div className='space-y-1.5'>
                            <Label htmlFor={`rule-${index}-domain-suffix`}>
                              域名后缀
                            </Label>
                            <Input
                              id={`rule-${index}-domain-suffix`}
                              placeholder='例如: netflix.com, openai.com'
                              value={rule.domain_suffix || ''}
                              onChange={(e) =>
                                handleUpdateRule(index, 'domain_suffix', e.target.value)
                              }
                            />
                          </div>

                          <div className='space-y-1.5'>
                            <Label htmlFor={`rule-${index}-domain-keyword`}>
                              域名关键词
                            </Label>
                            <Input
                              id={`rule-${index}-domain-keyword`}
                              placeholder='例如: google, youtube'
                              value={rule.domain_keyword || ''}
                              onChange={(e) =>
                                handleUpdateRule(index, 'domain_keyword', e.target.value)
                              }
                            />
                          </div>
                        </div>

                        <div className='grid gap-3 sm:grid-cols-2'>
                          <div className='space-y-1.5'>
                            <Label htmlFor={`rule-${index}-ip`}>GeoIP</Label>
                            <Input
                              id={`rule-${index}-ip`}
                              placeholder='例如: us, jp, hk'
                              value={rule.ip || ''}
                              onChange={(e) =>
                                handleUpdateRule(index, 'ip', e.target.value)
                              }
                            />
                          </div>

                          <div className='space-y-1.5'>
                            <Label htmlFor={`rule-${index}-ip-cidr`}>IP-CIDR</Label>
                            <Input
                              id={`rule-${index}-ip-cidr`}
                              placeholder='例如: 1.1.1.1/24'
                              value={rule.ip_cidr || ''}
                              onChange={(e) =>
                                handleUpdateRule(index, 'ip_cidr', e.target.value)
                              }
                            />
                          </div>
                        </div>

                        <div className='space-y-1.5'>
                          <Label htmlFor={`rule-${index}-protocol`}>协议</Label>
                          <Input
                            id={`rule-${index}-protocol`}
                            placeholder='例如: http, https, quic'
                            value={rule.protocol || ''}
                            onChange={(e) =>
                              handleUpdateRule(index, 'protocol', e.target.value)
                            }
                          />
                          <p className='text-xs text-muted-foreground'>
                            多个用逗号分隔
                          </p>
                        </div>
                      </CardContent>
                    </Card>
                  ))}
                </div>

                <Button onClick={handleAddRule} variant='outline' className='w-full'>
                  <Plus className='mr-2 h-4 w-4' />
                  添加规则
                </Button>
              </>
            )}

            <div className='rounded-lg border bg-muted/50 p-4'>
              <h4 className='mb-2 text-sm font-semibold'>规则说明</h4>
              <ul className='space-y-1 text-xs text-muted-foreground'>
                <li>• <strong>出站名称</strong>：必填，将创建对应的策略组</li>
                <li>• <strong>GeoSite</strong>：使用 GeoSite 数据库匹配域名集合</li>
                <li>• <strong>域名后缀</strong>：匹配完整域名后缀，如 google.com</li>
                <li>• <strong>域名关键词</strong>：匹配域名中包含的关键词</li>
                <li>• <strong>GeoIP</strong>：使用 GeoIP 数据库匹配 IP 地址归属国家/地区</li>
                <li>• <strong>IP-CIDR</strong>：匹配 IP 地址段</li>
                <li>• <strong>协议</strong>：匹配网络协议类型</li>
              </ul>
            </div>
          </CardContent>
        </CollapsibleContent>
      </Card>
    </Collapsible>
  )
}
