import { useState } from 'react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { Textarea } from '@/components/ui/textarea'
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from '@/components/ui/dialog'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { Upload, FileText, Plus } from 'lucide-react'
import { toast } from 'sonner'
import { createBlankTemplate } from '@/lib/template-v3-utils'

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
  const [tab, setTab] = useState<'upload' | 'paste' | 'blank'>('upload')
  const [pasteContent, setPasteContent] = useState('')
  const [newTemplateName, setNewTemplateName] = useState('')
  const [selectedFile, setSelectedFile] = useState<File | null>(null)

  const resetForm = () => {
    setPasteContent('')
    setNewTemplateName('')
    setSelectedFile(null)
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
    }
    resetForm()
  }

  return (
    <Dialog open={open} onOpenChange={handleClose}>
      <DialogContent className="sm:max-w-[500px]">
        <DialogHeader>
          <DialogTitle>创建模板</DialogTitle>
          <DialogDescription>
            上传 YAML 文件、粘贴内容或创建空白模板
          </DialogDescription>
        </DialogHeader>

        <Tabs value={tab} onValueChange={(v) => setTab(v as typeof tab)}>
          <TabsList className="w-full">
            <TabsTrigger value="upload" className="flex-1">
              <Upload className="h-4 w-4 mr-2" />
              上传文件
            </TabsTrigger>
            <TabsTrigger value="paste" className="flex-1">
              <FileText className="h-4 w-4 mr-2" />
              粘贴内容
            </TabsTrigger>
            <TabsTrigger value="blank" className="flex-1">
              <Plus className="h-4 w-4 mr-2" />
              空白模板
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
        </Tabs>

        <DialogFooter>
          <Button variant="outline" onClick={handleClose}>
            取消
          </Button>
          <Button onClick={handleSubmit} disabled={isLoading}>
            {isLoading ? '创建中...' : '创建'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
