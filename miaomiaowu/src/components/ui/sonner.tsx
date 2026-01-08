import { Toaster as Sonner, ToasterProps } from 'sonner'
import { useTheme } from '@/context/theme-provider'

export function Toaster({ ...props }: ToasterProps) {
  const { theme = 'system' } = useTheme()

  return (
    <Sonner
      theme={theme as ToasterProps['theme']}
      className='toaster group [&_div[data-content]]:w-full'
      toastOptions={{
        classNames: {
          toast: 'border-2 border-border rounded-none shadow-[4px_4px_0px_0px_rgba(0,0,0,0.1)] dark:shadow-[4px_4px_0px_0px_rgba(255,255,255,0.1)]',
          title: 'font-medium',
          description: 'opacity-90',
          actionButton: 'bg-primary text-primary-foreground border-2 border-border rounded-none',
          cancelButton: 'bg-muted text-muted-foreground border-2 border-border rounded-none',
          error: 'border-destructive/50 bg-destructive/10 text-destructive',
          success: 'border-green-500/50 bg-green-500/10 text-green-700 dark:text-green-400',
          warning: 'border-yellow-500/50 bg-yellow-500/10 text-yellow-700 dark:text-yellow-400',
          info: 'border-blue-500/50 bg-blue-500/10 text-blue-700 dark:text-blue-400',
        },
      }}
      style={
        {
          '--normal-bg': 'var(--popover)',
          '--normal-text': 'var(--popover-foreground)',
          '--normal-border': 'var(--border)',
        } as React.CSSProperties
      }
      position='bottom-right'
      expand={true}
      richColors
      {...props}
    />
  )
}
