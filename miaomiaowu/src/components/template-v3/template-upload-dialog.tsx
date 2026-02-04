import { useState, useEffect } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Select, SelectContent, SelectGroup, SelectItem, SelectLabel, SelectTrigger, SelectValue } from '@/components/ui/select'
import { Upload, FileText, Plus, RefreshCw } from 'lucide-react'
import { toast } from 'sonner'
import { createBlankTemplate } from '@/lib/template-v3-utils'
import { api } from '@/lib/api'
import { RULE_TEMPLATES } from '@/config/custom-rules-templates'
import { ALL_TEMPLATE_PRESETS } from '@/lib/template-presets'

interface UserTemplate {
  id: number
  name: string
  rule_source: string
}

interface TemplateUploadDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  onUpload: (file: File) => void
  onCreate: (name: string, content: string) => void
  isLoading?: boolean
}

export function TemplateUploadDialog({
  open,
  onOpenChange,
  onUpload,
  onCreate,
  isLoading = false,
}: TemplateUploadDialogProps) {
  const [tab, setTab] = useState<'upload' | 'paste' | 'blank' | 'v2import'>('upload')
  const [pasteContent, setPasteContent] = useState('')
  const [newTemplateName, setNewTemplateName] = useState('')
  const [selectedFile, setSelectedFile] = useState<File | null>(null)
  const [isConverting, setIsConverting] = useState(false)
  const [selectedDnsPreset, setSelectedDnsPreset] = useState<string>('fake_ip_no_dnsleak')

  // V2 import states
  const [userTemplates, setUserTemplates] = useState<UserTemplate[]>([])
  const [selectedV2Template, setSelectedV2Template] = useState<string>('')
  const [isFetchingTemplates, setIsFetchingTemplates] = useState(false)

  // Fetch user templates when dialog opens and v2import tab is selected
  useEffect(() => {
    if (open && tab === 'v2import' && userTemplates.length === 0) {
      fetchUserTemplates()
    }
  }, [open, tab])

  const fetchUserTemplates = async () => {
    setIsFetchingTemplates(true)
    try {
      const response = await api.get('/api/admin/templates')
      setUserTemplates(response.data.templates || [])
    } catch (error) {
      toast.error('获取模板列表失败')
    } finally {
      setIsFetchingTemplates(false)
    }
  }

  const resetForm = () => {
    setPasteContent('')
    setNewTemplateName('')
    setSelectedFile(null)
    setSelectedDnsPreset('fake_ip_no_dnsleak')
    setSelectedV2Template('')
    setTab('upload')
  }

  const handleClose = () => {
    resetForm()
    onOpenChange(false)
  }

  const handleFileChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0]
    if (file) {
      if (!file.name.endsWith('.yaml') && !file.name.endsWith('.yml')) {
        toast.error('请选择 YAML 文件')
        return
      }
      setSelectedFile(file)
    }
  }

  const handleSubmit = () => {
    if (tab === 'upload') {
      if (!selectedFile) {
        toast.error('请选择文件')
        return
      }
      onUpload(selectedFile)
    } else if (tab === 'paste') {
      if (!pasteContent.trim()) {
        toast.error('请输入模板内容')
        return
      }
      if (!newTemplateName.trim()) {
        toast.error('请输入模板名称')
        return
      }
      let name = newTemplateName.trim()
      if (!name.endsWith('.yaml') && !name.endsWith('.yml')) {
        name += '.yaml'
      }
      onCreate(name, pasteContent)
    } else if (tab === 'blank') {
      if (!newTemplateName.trim()) {
        toast.error('请输入模板名称')
        return
      }
      let name = newTemplateName.trim()
      if (!name.endsWith('.yaml') && !name.endsWith('.yml')) {
        name += '.yaml'
      }
      onCreate(name, createBlankTemplate())
    } else if (tab === 'v2import') {
      handleV2Import()
      return // Don't reset form yet, wait for conversion
    }
    resetForm()
  }

  const handleV2Import = async () => {
    if (!selectedV2Template) {
      toast.error('请选择 V2 模板')
      return
    }
    if (!newTemplateName.trim()) {
      toast.error('请输入模板名称')
      return
    }

    setIsConverting(true)
    try {
      // Determine if it's a user template or preset template based on prefix
      let ruleSourceUrl: string
      if (selectedV2Template.startsWith('user:')) {
        const templateId = selectedV2Template.replace('user:', '')
        const template = userTemplates.find(t => t.id.toString() === templateId)
        if (!template) {
          toast.error('未找到选中的模板')
          return
        }
        ruleSourceUrl = template.rule_source
      } else if (selectedV2Template.startsWith('preset:')) {
        const presetName = selectedV2Template.replace('preset:', '')
        const preset = ALL_TEMPLATE_PRESETS.find(p => p.name === presetName)
        if (!preset) {
          toast.error('未找到选中的预设模板')
          return
        }
        ruleSourceUrl = preset.url
      } else {
        toast.error('无效的模板选择')
        return
      }

      // Fetch the template content from URL
      const fetchResponse = await api.post('/api/admin/templates/fetch-source', {
        url: ruleSourceUrl,
        use_proxy: false,
      })
      const v2Content = fetchResponse.data.content

      // Convert to v3
      const response = await api.post('/api/admin/template-v3/convert-v2', {
        content: v2Content,
      })

      const { proxy_groups, rules, rule_providers } = response.data

      // Generate v3 template YAML
      const v3Content = generateV3TemplateFromConversion(proxy_groups, rules, rule_providers)

      let name = newTemplateName.trim()
      if (!name.endsWith('.yaml') && !name.endsWith('.yml')) {
        name += '__v3.yaml'
      } else if (!name.includes('v3')) {
        name = name.replace(/\.(yaml|yml)$/, '__v3.$1')
      }

      onCreate(name, v3Content)
      resetForm()
    } catch (error: any) {
      toast.error(error.response?.data?.error || '转换失败')
    } finally {
      setIsConverting(false)
    }
  }

  // Generate v3 template YAML from converted data
  const generateV3TemplateFromConversion = (
    proxyGroups: any[],
    rules: string[],
    ruleProviders: Record<string, any>
  ): string => {
    const lines: string[] = []

    // Basic config
    lines.push('mode: rule')

    // DNS config from preset
    const dnsPreset = RULE_TEMPLATES.dns[selectedDnsPreset as keyof typeof RULE_TEMPLATES.dns]
    if (dnsPreset) {
      lines.push('dns:')
      // Indent the DNS content
      const dnsLines = dnsPreset.content.split('\n')
      for (const line of dnsLines) {
        lines.push('  ' + line)
      }
    } else {
      // Fallback to basic DNS config
      lines.push('dns:')
      lines.push('  enable: true')
      lines.push('  enhanced-mode: fake-ip')
      lines.push('  nameserver:')
      lines.push('    - https://1.12.12.12/dns-query')
      lines.push('  ipv6: false')
    }

    lines.push('proxies: null')

    // Proxy groups
    lines.push('proxy-groups:')
    for (const group of proxyGroups) {
      lines.push(`  - name: ${group.name}`)
      lines.push(`    type: ${group.type}`)
      if (group['include-all']) {
        lines.push('    include-all: true')
      }
      if (group['include-all-proxies']) {
        lines.push('    include-all-proxies: true')
      }
      if (group.filter) {
        lines.push(`    filter: ${group.filter}`)
      }
      if (group['exclude-filter']) {
        lines.push(`    exclude-filter: ${group['exclude-filter']}`)
      }
      if (group.proxies && group.proxies.length > 0) {
        lines.push('    proxies:')
        for (const proxy of group.proxies) {
          lines.push(`      - ${proxy}`)
        }
      }
      if (group.url) {
        lines.push(`    url: ${group.url}`)
      }
      if (group.interval) {
        lines.push(`    interval: ${group.interval}`)
      }
      if (group.tolerance) {
        lines.push(`    tolerance: ${group.tolerance}`)
      }
    }

    // Rules
    if (rules.length > 0) {
      lines.push('rules:')
      for (const rule of rules) {
        lines.push(`  - ${rule}`)
      }
    }

    // Rule providers
    if (Object.keys(ruleProviders).length > 0) {
      lines.push('rule-providers:')
      for (const [name, provider] of Object.entries(ruleProviders)) {
        lines.push(`  ${name}:`)
        lines.push(`    type: ${(provider as any).type}`)
        lines.push(`    behavior: ${(provider as any).behavior}`)
        lines.push(`    url: ${(provider as any).url}`)
        lines.push(`    path: ${(provider as any).path}`)
        lines.push(`    interval: ${(provider as any).interval}`)
      }
    }

    return lines.join('\n')
  }

  // Auto-fill template name when selecting a template (append __v3)
  const handleV2TemplateSelect = (value: string) => {
    setSelectedV2Template(value)
    let baseName = ''
    if (value.startsWith('user:')) {
      const templateId = value.replace('user:', '')
      const template = userTemplates.find(t => t.id.toString() === templateId)
      if (template) {
        baseName = template.name
      }
    } else if (value.startsWith('preset:')) {
      const presetName = value.replace('preset:', '')
      const preset = ALL_TEMPLATE_PRESETS.find(p => p.name === presetName)
      if (preset) {
        baseName = preset.label
      }
    }
    if (baseName) {
      // Append __v3 to the base name
      setNewTemplateName(baseName + '__v3')
    }
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[600px]">
        <DialogHeader>
          <DialogTitle>创建模板</DialogTitle>
          <DialogDescription>
            上传 YAML 文件、粘贴内容、创建空白模板或从 V2 模板导入
          </DialogDescription>
        </DialogHeader>

        <Tabs value={tab} onValueChange={(v) => setTab(v as typeof tab)}>
          <TabsList className="w-full grid grid-cols-4">
            <TabsTrigger value="upload">
              <Upload className="h-4 w-4 mr-1" />
              上传
            </TabsTrigger>
            <TabsTrigger value="paste">
              <FileText className="h-4 w-4 mr-1" />
              粘贴
            </TabsTrigger>
            <TabsTrigger value="blank">
              <Plus className="h-4 w-4 mr-1" />
              空白
            </TabsTrigger>
            <TabsTrigger value="v2import">
              <RefreshCw className="h-4 w-4 mr-1" />
              V2导入
            </TabsTrigger>
          </TabsList>

          <TabsContent value="upload" className="space-y-4 mt-4">
            <div className="space-y-2">
              <Label>选择 YAML 文件</Label>
              <Input
                type="file"
                accept=".yaml,.yml"
                onChange={handleFileChange}
              />
              {selectedFile && (
                <p className="text-sm text-muted-foreground">
                  已选择: {selectedFile.name}
                </p>
              )}
            </div>
          </TabsContent>

          <TabsContent value="paste" className="space-y-4 mt-4">
            <div className="space-y-2">
              <Label>模板名称</Label>
              <Input
                value={newTemplateName}
                onChange={(e) => setNewTemplateName(e.target.value)}
                placeholder="my_template__v3.yaml"
              />
            </div>
            <div className="space-y-2">
              <Label>YAML 内容</Label>
              <Textarea
                value={pasteContent}
                onChange={(e) => setPasteContent(e.target.value)}
                placeholder="粘贴 YAML 内容..."
                className="min-h-[200px] font-mono text-sm"
              />
            </div>
          </TabsContent>

          <TabsContent value="blank" className="space-y-4 mt-4">
            <div className="space-y-2">
              <Label>模板名称</Label>
              <Input
                value={newTemplateName}
                onChange={(e) => setNewTemplateName(e.target.value)}
                placeholder="my_template__v3.yaml"
              />
            </div>
            <p className="text-sm text-muted-foreground">
              将创建包含基础结构的空白 v3 模板，包含节点选择、自动选择和全球直连三个代理组。
            </p>
          </TabsContent>

          <TabsContent value="v2import" className="space-y-4 mt-4">
            <div className="space-y-2">
              <Label>选择 V2 模板</Label>
              <Select
                value={selectedV2Template}
                onValueChange={handleV2TemplateSelect}
                disabled={isFetchingTemplates}
              >
                <SelectTrigger>
                  <SelectValue placeholder={isFetchingTemplates ? '加载中...' : '选择模板'} />
                </SelectTrigger>
                <SelectContent>
                  {userTemplates.length > 0 && (
                    <SelectGroup>
                      <SelectLabel>我的模板</SelectLabel>
                      {userTemplates.map((template) => (
                        <SelectItem key={`user-${template.id}`} value={`user:${template.id}`}>
                          {template.name}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  )}
                  <SelectGroup>
                    <SelectLabel>预设模板</SelectLabel>
                    {ALL_TEMPLATE_PRESETS.map((preset) => (
                      <SelectItem key={`preset-${preset.name}`} value={`preset:${preset.name}`}>
                        {preset.label}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </div>

            <div className="space-y-2">
              <Label>新模板名称</Label>
              <Input
                value={newTemplateName}
                onChange={(e) => setNewTemplateName(e.target.value)}
                placeholder="my_template__v3.yaml"
              />
            </div>

            <div className="space-y-2">
              <Label>DNS 配置</Label>
              <Select value={selectedDnsPreset} onValueChange={setSelectedDnsPreset}>
                <SelectTrigger>
                  <SelectValue placeholder="选择 DNS 配置" />
                </SelectTrigger>
                <SelectContent>
                  {Object.entries(RULE_TEMPLATES.dns).map(([key, preset]) => (
                    <SelectItem key={key} value={key}>
                      {preset.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>

            <p className="text-sm text-muted-foreground">
              将自动转换 custom_proxy_group 和 ruleset 配置为 v3 格式。
              <br />
              • <code>.*</code> 会转换为 <code>include-all: true</code>
              <br />
              • 正则表达式会转换为 <code>filter</code> 字段
            </p>
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={isLoading || isConverting}>
            {isLoading || isConverting ? '处理中...' : tab === 'v2import' ? '转换并创建' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
