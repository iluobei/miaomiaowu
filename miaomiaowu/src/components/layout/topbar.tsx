import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Activity, Link as LinkIcon, Radar, Users, Files, Zap, Network, Menu, FileCode, Settings, Send, FileStack } from 'lucide-react'
import { ThemeSwitch } from '@/components/theme-switch'
import { UserMenu } from './user-menu'
import { useAuthStore } from '@/stores/auth-store'
import { profileQueryFn } from '@/lib/profile'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { useState, useRef, useEffect, useCallback } from 'react'

const baseNavLinks = [
  {
    title: '流量信息',
    to: '/',
    icon: Activity,
  },
  {
    title: '订阅链接',
    to: '/subscription',
    icon: LinkIcon,
  },
]

const adminNavLinks = [
  {
    title: '生成订阅',
    to: '/generator',
    icon: Zap,
  },
  {
    title: '节点管理',
    to: '/nodes',
    icon: Network,
  },
  {
    title: '订阅管理',
    to: '/subscribe-files',
    icon: Files,
  },
  {
    title: '模板管理',
    to: '/templates-v3',
    icon: FileStack,
  },
  {
    title: '规则管理',
    to: '/custom-rules',
    icon: FileCode,
  },
  {
    title: '探针管理',
    to: '/probe',
    icon: Radar,
  },
  {
    title: '用户管理',
    to: '/users',
    icon: Users,
  },
  {
    title: '系统设置',
    to: '/system-settings',
    icon: Settings,
  },
]

export function Topbar() {
  const { auth } = useAuthStore()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const navRef = useRef<HTMLElement>(null)
  const [iconOnlyCount, setIconOnlyCount] = useState(0)
  const [hideLogoText, setHideLogoText] = useState(false)

  const { data: profile } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  const isAdmin = Boolean(profile?.is_admin)

  // 计算所有导航链接
  const allNavLinks = isAdmin ? [...baseNavLinks, ...adminNavLinks] : baseNavLinks
  const totalLinks = allNavLinks.length

  // 计算需要隐藏文字的按钮数量（从后往前）
  const calculateIconOnlyCount = useCallback(() => {
    if (!navRef.current) return

    // 直接获取窗口宽度
    const windowWidth = window.innerWidth
    // 预留空间：logo图片约60px，右侧按钮区约200px，左右padding约48px，间距约24px
    const logoTextWidth = 90 // "妙妙屋" 文字宽度
    const baseReservedSpace = 340 // 不含logo文字的预留空间

    // 每个带文字按钮约115px（4字+图标+padding），纯图标按钮约44px，gap约12px
    const fullButtonWidth = 115
    const iconButtonWidth = 44
    const gap = 12

    // 计算全部显示文字需要的宽度
    const fullWidth = totalLinks * (fullButtonWidth + gap) - gap
    const availableWithLogoText = windowWidth - baseReservedSpace - logoTextWidth

    if (fullWidth <= availableWithLogoText) {
      // 空间够，全部显示
      setIconOnlyCount(0)
      setHideLogoText(false)
      return
    }

    // 空间不够，先隐藏"妙妙屋"文字
    setHideLogoText(true)
    const availableWithoutLogoText = windowWidth - baseReservedSpace

    if (fullWidth <= availableWithoutLogoText) {
      // 隐藏logo文字后空间够了
      setIconOnlyCount(0)
      return
    }

    // 还不够，需要隐藏部分按钮文字
    const savedPerButton = fullButtonWidth - iconButtonWidth
    const overflowWidth = fullWidth - availableWithoutLogoText
    const needed = Math.ceil(overflowWidth / savedPerButton)
    setIconOnlyCount(Math.min(needed, totalLinks))
  }, [totalLinks])

  useEffect(() => {
    calculateIconOnlyCount()

    const resizeObserver = new ResizeObserver(() => {
      calculateIconOnlyCount()
    })

    if (navRef.current?.parentElement?.parentElement) {
      resizeObserver.observe(navRef.current.parentElement.parentElement)
    }

    window.addEventListener('resize', calculateIconOnlyCount)

    return () => {
      resizeObserver.disconnect()
      window.removeEventListener('resize', calculateIconOnlyCount)
    }
  }, [calculateIconOnlyCount])

  return (
    <header className='fixed top-0 left-0 right-0 z-50 border-b border-[color:rgba(241,140,110,0.22)] bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60'>
      <div className='flex h-16 items-center justify-between px-4 sm:px-6 overflow-hidden'>
        <div className='flex items-center gap-4 sm:gap-6 min-w-0'>
          <Link
            to='/'
            className='flex items-center gap-3 font-semibold text-lg tracking-tight transition hover:text-primary outline-none focus:outline-none shrink-0'
          >
            <img
              src='/images/logo.webp'
              alt='妙妙屋 Logo'
              className='h-10 w-10 border-2 border-[color:rgba(241,140,110,0.4)] shadow-[4px_4px_0_rgba(0,0,0,0.2)] shrink-0'
            />
            {!hideLogoText && <span className='hidden md:inline pixel-text text-primary text-base whitespace-nowrap'>妙妙屋</span>}
          </Link>

          {/* Desktop Navigation - Base links + Admin links */}
          <nav ref={navRef} className='hidden md:flex items-center gap-2 md:gap-3'>
            {allNavLinks.map(({ title, to, icon: Icon }, index) => {
              // 从后往前计算，index >= totalLinks - iconOnlyCount 的按钮只显示图标
              const showIconOnly = index >= totalLinks - iconOnlyCount

              return (
                <Link
                  key={to}
                  to={to}
                  aria-label={title}
                  title={title}
                  className={`pixel-button inline-flex items-center gap-2 py-2 h-9 text-sm font-semibold uppercase tracking-widest bg-background/75 text-foreground border-[color:rgba(137,110,96,0.45)] hover:bg-accent/35 hover:text-accent-foreground dark:bg-input/30 dark:border-[color:rgba(255,255,255,0.18)] dark:hover:bg-accent/45 dark:hover:text-accent-foreground transition-all whitespace-nowrap ${
                    showIconOnly ? 'justify-center px-2 w-9' : 'justify-start px-3'
                  }`}
                  activeProps={{
                    className: 'bg-primary/20 text-primary border-[color:rgba(217,119,87,0.55)] dark:bg-primary/20 dark:border-[color:rgba(217,119,87,0.55)]'
                  }}
                >
                  <Icon className='size-[18px] shrink-0' />
                  {!showIconOnly && <span>{title}</span>}
                </Link>
              )
            })}
          </nav>

          {/* Mobile Base Navigation - Only show on mobile */}
          <nav className='md:hidden flex items-center gap-2'>
            {baseNavLinks.map(({ title, to, icon: Icon }) => (
              <Link
                key={to}
                to={to}
                aria-label={title}
                className='pixel-button inline-flex items-center justify-center gap-2 px-2 py-2 h-9 text-sm font-semibold uppercase tracking-widest bg-background/75 text-foreground border-[color:rgba(137,110,96,0.45)] hover:bg-accent/35 hover:text-accent-foreground dark:bg-input/30 dark:border-[color:rgba(255,255,255,0.18)] dark:hover:bg-accent/45 dark:hover:text-accent-foreground transition-all'
                activeProps={{
                  className: 'bg-primary/20 text-primary border-[color:rgba(217,119,87,0.55)] dark:bg-primary/20 dark:border-[color:rgba(217,119,87,0.55)]'
                }}
              >
                <Icon className='size-[18px] shrink-0' />
              </Link>
            ))}
          </nav>

          {/* Mobile Navigation Dropdown - Only show on mobile for admin */}
          {isAdmin && (
            <DropdownMenu open={mobileMenuOpen} onOpenChange={setMobileMenuOpen}>
              <DropdownMenuTrigger asChild>
                <Button
                  variant='outline'
                  size='icon'
                  className='md:hidden pixel-button h-9 w-9 bg-background/75 border-[color:rgba(137,110,96,0.45)] hover:bg-accent/35 dark:bg-input/30 dark:border-[color:rgba(255,255,255,0.18)] dark:hover:bg-accent/45'
                >
                  <Menu className='h-5 w-5' />
                  <span className='sr-only'>打开菜单</span>
                </Button>
              </DropdownMenuTrigger>
              <DropdownMenuContent align='start' className='w-48 pixel-border'>
                {adminNavLinks.map(({ title, to, icon: Icon }) => (
                  <DropdownMenuItem key={to} asChild>
                    <Link
                      to={to}
                      className='flex items-center gap-3 px-3 py-2 cursor-pointer hover:bg-accent/35 focus:bg-accent/35'
                      onClick={() => setMobileMenuOpen(false)}
                    >
                      <Icon className='size-[18px] shrink-0' />
                      <span>{title}</span>
                    </Link>
                  </DropdownMenuItem>
                ))}
              </DropdownMenuContent>
            </DropdownMenu>
          )}
        </div>

        <div className='flex items-center gap-2 sm:gap-3 pl-2 sm:pl-0'>
          <a
            href='https://t.me/miaomiaowux'
            target='_blank'
            rel='noopener noreferrer'
            aria-label='Telegram 交流群组'
            title='Telegram 交流群组'
            className='pixel-button inline-flex items-center justify-center h-9 w-9 px-2 py-2 text-sm font-semibold bg-background/75 text-foreground border-[color:rgba(137,110,96,0.45)] hover:bg-accent/35 hover:text-accent-foreground dark:bg-input/30 dark:border-[color:rgba(255,255,255,0.18)] dark:hover:bg-accent/45 dark:hover:text-accent-foreground transition-all relative animate-pulse'
          >
            <Send className='size-[18px] animate-bounce' />
            <span className='absolute -top-1 -right-1 flex h-3 w-3'>
              <span className='animate-ping absolute inline-flex h-full w-full rounded-full bg-primary opacity-75'></span>
              <span className='relative inline-flex rounded-full h-3 w-3 bg-primary'></span>
            </span>
          </a>
          <ThemeSwitch />
          <UserMenu />
        </div>
      </div>
    </header>
  )
}
