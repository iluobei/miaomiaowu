import { useState } from 'react'
import { useMutation } from '@tanstack/react-query'
import { Download, Upload, HardDrive, AlertTriangle } from 'lucide-react'
import { toast } from 'sonner'
import { api } from '@/lib/api'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'

interface BackupDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
}

export function BackupDialog({ open, onOpenChange }: BackupDialogProps) {
  const [backupFile, setBackupFile] = useState<File | null>(null)
  const [isDownloading, setIsDownloading] = useState(false)

  // Download backup
  const handleDownload = async () => {
    setIsDownloading(true)
    try {
      const response = await api.get('/api/admin/backup/download', {
        responseType: 'blob',
      })
      const url = window.URL.createObjectURL(new Blob([response.data]))
      const link = document.createElement('a')
      link.href = url
      const timestamp = new Date().toISOString().replace(/[:.]/g, '-').slice(0, 19)
      link.setAttribute('download', `miaomiaowu-backup-${timestamp}.zip`)
      document.body.appendChild(link)
      link.click()
      link.remove()
      window.URL.revokeObjectURL(url)
      toast.success('备份下载成功')
    } catch {
      toast.error('备份下载失败')
    } finally {
      setIsDownloading(false)
    }
  }

  // Restore backup
  const restoreMutation = useMutation({
    mutationFn: async (file: File) => {
      const formData = new FormData()
      formData.append('backup', file)
      return api.post('/api/admin/backup/restore', formData, {
        headers: { 'Content-Type': 'multipart/form-data' },
      })
    },
    onSuccess: () => {
      toast.success('备份恢复成功，请刷新页面')
      setBackupFile(null)
      onOpenChange(false)
      // Reload page after a short delay
      setTimeout(() => {
        window.location.reload()
      }, 1500)
    },
    onError: () => {
      toast.error('备份恢复失败')
    },
  })

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='sm:max-w-md'>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2'>
            <HardDrive className='size-5' /> 数据备份
          </DialogTitle>
          <DialogDescription>
            备份包含数据库和订阅文件，恢复后页面会自动刷新
          </DialogDescription>
        </DialogHeader>

        <div className='space-y-6'>
          {/* Download backup */}
          <div className='space-y-2'>
            <Label>下载备份</Label>
            <Button
              onClick={handleDownload}
              disabled={isDownloading}
              className='w-full'
            >
              <Download className='size-4 mr-2' />
              {isDownloading ? '正在生成备份...' : '下载当前数据备份'}
            </Button>
          </div>

          {/* Restore backup */}
          <div className='space-y-3'>
            <Label>恢复备份</Label>
            <Input
              type='file'
              accept='.zip'
              onChange={(e) => setBackupFile(e.target.files?.[0] || null)}
              className='cursor-pointer'
            />
            <Button
              onClick={() => backupFile && restoreMutation.mutate(backupFile)}
              disabled={!backupFile || restoreMutation.isPending}
              variant='destructive'
              className='w-full'
            >
              <Upload className='size-4 mr-2' />
              {restoreMutation.isPending ? '恢复中...' : '恢复备份'}
            </Button>
            <div className='flex items-start gap-2 text-xs text-muted-foreground'>
              <AlertTriangle className='size-4 shrink-0 text-destructive' />
              <span>恢复备份将覆盖当前所有数据，请谨慎操作</span>
            </div>
          </div>
        </div>
      </DialogContent>
    </Dialog>
  )
}
