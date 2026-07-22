import { memo, useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { useVirtualizer } from '@tanstack/react-virtual'
import { Check, Copy, Download, Eye, Pencil } from 'lucide-react'
import { toast } from 'sonner'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'

type ClashConfigViewerProps = {
  value: string
  onChange: (value: string) => void
  className?: string
  height?: number
}

const LINE_HEIGHT = 18
/** 超过该行数默认只读虚拟列表，避免巨型 textarea 拖垮页面 */
const AUTO_VIEW_LINES = 200
/** 超过该字符数也强制默认预览模式 */
const AUTO_VIEW_CHARS = 40_000

function formatBytes(n: number): string {
  if (n < 1024) return `${n} B`
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`
  return `${(n / (1024 * 1024)).toFixed(2)} MB`
}

export const ClashConfigViewer = memo(function ClashConfigViewer({
  value,
  onChange,
  className,
  height = 400,
}: ClashConfigViewerProps) {
  const lines = useMemo(() => (value.length ? value.split('\n') : ['']), [value])
  const lineCount = lines.length
  const isLarge = lineCount >= AUTO_VIEW_LINES || value.length >= AUTO_VIEW_CHARS

  const [mode, setMode] = useState<'view' | 'edit'>('view')
  const [draft, setDraft] = useState(value)
  const [copied, setCopied] = useState(false)
  const scrollRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  // 避免外部 value 与本地编辑互相抢写
  const editingRef = useRef(false)

  useEffect(() => {
    if (editingRef.current) return
    setDraft(value)
  }, [value])

  const virtualizer = useVirtualizer({
    count: lineCount,
    getScrollElement: () => scrollRef.current,
    estimateSize: () => LINE_HEIGHT,
    overscan: 24,
  })

  const enterEdit = useCallback(() => {
    setDraft(value)
    editingRef.current = true
    setMode('edit')
    requestAnimationFrame(() => {
      textareaRef.current?.focus()
    })
  }, [value])

  const applyEdit = useCallback(() => {
    editingRef.current = false
    onChange(draft)
    setMode('view')
  }, [draft, onChange])

  const cancelEdit = useCallback(() => {
    editingRef.current = false
    setDraft(value)
    setMode('view')
  }, [value])

  const handleCopy = useCallback(async () => {
    const text = mode === 'edit' ? draft : value
    try {
      await navigator.clipboard.writeText(text)
      setCopied(true)
      toast.success('配置已复制到剪贴板')
      window.setTimeout(() => setCopied(false), 1500)
    } catch {
      toast.error('复制失败，请手动选择文本')
    }
  }, [draft, mode, value])

  const handleDownload = useCallback(() => {
    const text = mode === 'edit' ? draft : value
    const blob = new Blob([text], { type: 'text/yaml;charset=utf-8' })
    const url = URL.createObjectURL(blob)
    const a = document.createElement('a')
    a.href = url
    a.download = 'clash-config.yaml'
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    toast.success('已开始下载配置文件')
  }, [draft, mode, value])

  return (
    <div className={cn('space-y-2', className)}>
      <div className='flex flex-wrap items-center justify-between gap-2 text-xs text-muted-foreground'>
        <span>
          {lineCount.toLocaleString()} 行 · {formatBytes(value.length)}
          {isLarge && mode === 'view' ? ' · 预览模式（虚拟滚动）' : ''}
          {mode === 'edit' ? ' · 编辑中（本地缓冲，完成后再写回）' : ''}
        </span>
        <div className='flex flex-wrap items-center gap-1'>
          <Button type='button' variant='outline' size='sm' className='h-8' onClick={handleCopy}>
            {copied ? <Check className='h-3.5 w-3.5' /> : <Copy className='h-3.5 w-3.5' />}
            <span className='ml-1'>{copied ? '已复制' : '复制'}</span>
          </Button>
          <Button type='button' variant='outline' size='sm' className='h-8' onClick={handleDownload}>
            <Download className='h-3.5 w-3.5' />
            <span className='ml-1'>下载</span>
          </Button>
          {mode === 'view' ? (
            <Button type='button' variant='outline' size='sm' className='h-8' onClick={enterEdit}>
              <Pencil className='h-3.5 w-3.5' />
              <span className='ml-1'>编辑</span>
            </Button>
          ) : (
            <>
              <Button type='button' variant='outline' size='sm' className='h-8' onClick={cancelEdit}>
                <Eye className='h-3.5 w-3.5' />
                <span className='ml-1'>取消</span>
              </Button>
              <Button type='button' size='sm' className='h-8' onClick={applyEdit}>
                完成编辑
              </Button>
            </>
          )}
        </div>
      </div>

      {mode === 'view' ? (
        <div
          ref={scrollRef}
          className='rounded-lg border bg-muted/30 overflow-auto font-mono text-xs overscroll-contain'
          style={{ height, maxHeight: height }}
        >
          <div
            className='relative w-full'
            style={{ height: virtualizer.getTotalSize() }}
          >
            {virtualizer.getVirtualItems().map((item) => (
              <div
                key={item.key}
                className='absolute left-0 top-0 flex w-max min-w-full whitespace-pre px-3 text-[12px] leading-[18px] text-foreground/90'
                style={{
                  height: item.size,
                  transform: `translateY(${item.start}px)`,
                }}
              >
                <span className='mr-3 inline-block w-10 select-none text-right text-muted-foreground/70'>
                  {item.index + 1}
                </span>
                <span>{lines[item.index] ?? ''}</span>
              </div>
            ))}
          </div>
        </div>
      ) : (
        <div className='rounded-lg border bg-muted/30'>
          {isLarge && (
            <div className='border-b px-3 py-2 text-xs text-amber-600 dark:text-amber-400'>
              配置较大，编辑时请尽量少改；编辑完成前不会触发整页重渲染。
            </div>
          )}
          <textarea
            ref={textareaRef}
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            spellCheck={false}
            autoComplete='off'
            autoCorrect='off'
            autoCapitalize='off'
            className='w-full resize-none overflow-auto border-0 bg-transparent px-3 py-2 font-mono text-xs leading-[18px] outline-none focus:ring-0 [field-sizing:fixed]'
            style={{ height, maxHeight: height }}
            placeholder='生成配置后显示在这里...'
          />
        </div>
      )}
    </div>
  )
})
