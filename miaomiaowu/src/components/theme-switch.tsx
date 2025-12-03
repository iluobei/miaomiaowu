import { useEffect } from 'react'
import { Moon, Sun, SunMoon } from 'lucide-react'
import { useTheme } from '@/context/theme-provider'
import { Button } from '@/components/ui/button'

export function ThemeSwitch() {
  const { theme, setTheme } = useTheme()

  /* Update theme-color meta tag
   * when theme is updated */
  useEffect(() => {
    const themeColor = theme === 'dark' ? '#020817' : '#fff'
    const metaThemeColor = document.querySelector("meta[name='theme-color']")
    if (metaThemeColor) metaThemeColor.setAttribute('content', themeColor)
  }, [theme])

  // 循环切换: light -> dark -> system -> light
  const cycleTheme = () => {
    if (theme === 'light') {
      setTheme('dark')
    } else if (theme === 'dark') {
      setTheme('system')
    } else {
      setTheme('light')
    }
  }

  // 根据当前主题选择图标
  const Icon = theme === 'light' ? Sun : theme === 'dark' ? Moon : SunMoon
  const label = theme === 'light' ? '浅色模式' : theme === 'dark' ? '深色模式' : '跟随系统'

  return (
    <Button
      variant='outline'
      size='icon'
      aria-label={label}
      title={label}
      className='h-9 w-9'
      onClick={cycleTheme}
    >
      <Icon className='size-[18px]' />
      <span className='sr-only'>{label}</span>
    </Button>
  )
}
