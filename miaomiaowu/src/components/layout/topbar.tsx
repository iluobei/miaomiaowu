import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { Activity, Link as LinkIcon, Radar, Users, Database, Zap, Network, Menu, FileCode } from 'lucide-react'
import { ThemeSwitch } from '@/components/theme-switch'
import { UserMenu } from './user-menu'
import { useAuthStore } from '@/stores/auth-store'
import { profileQueryFn } from '@/lib/profile'
import { cn } from '@/lib/utils'
import { api } from '@/lib/api'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { Button } from '@/components/ui/button'
import { useState } from 'react'

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
    title: '探针管理',
    to: '/probe',
    icon: Radar,
  },
  {
    title: '订阅管理',
    to: '/subscribe-files',
    icon: Database,
  },
  // {
  //   title: '规则配置',
  //   to: '/rules',
  //   icon: Settings2,
  // },
  {
    title: '用户管理',
    to: '/users',
    icon: Users,
  },
]

export function Topbar() {
  const { auth } = useAuthStore()
  const [mobileMenuOpen, setMobileMenuOpen] = useState(false)
  const { data: profile } = useQuery({
    queryKey: ['profile'],
    queryFn: profileQueryFn,
    enabled: Boolean(auth.accessToken),
    staleTime: 5 * 60 * 1000,
  })

  // Fetch user config to check if custom rules are enabled
  const { data: userConfig } = useQuery({
    queryKey: ['user-config'],
    queryFn: async () => {
      const response = await api.get('/api/user/config')
      return response.data as {
        custom_rules_enabled: boolean
      }
    },
    enabled: Boolean(auth.accessToken) && Boolean(profile?.is_admin),
    staleTime: 5 * 60 * 1000,
  })

  const isAdmin = Boolean(profile?.is_admin)
  const customRulesEnabled = Boolean(userConfig?.custom_rules_enabled)

  // Add custom rules link conditionally
  const adminNavLinksWithCustomRules = customRulesEnabled
    ? [
        ...adminNavLinks,
        {
          title: '自定义规则',
          to: '/custom-rules',
          icon: FileCode,
        },
      ]
    : adminNavLinks

  const navLinks = isAdmin ? [...baseNavLinks, ...adminNavLinksWithCustomRules] : baseNavLinks

  return (
    <header className='fixed top-0 left-0 right-0 z-50 border-b border-[color:rgba(241,140,110,0.22)] bg-background/80 backdrop-blur supports-[backdrop-filter]:bg-background/60'>
      <div className='flex h-16 items-center justify-between px-4 sm:px-6'>
        <div className='flex items-center gap-4 sm:gap-6'>
          <Link
            to='/'
            className='flex items-center gap-3 font-semibold text-lg tracking-tight transition hover:text-primary outline-none focus:outline-none'
          >
            <img
              src='/images/logo.webp'
              alt='妙妙屋 Logo'
              className='h-10 w-10 border-2 border-[color:rgba(241,140,110,0.4)] shadow-[4px_4px_0_rgba(0,0,0,0.2)]'
            />
            <span className='hidden sm:inline pixel-text text-primary text-base'>妙妙屋</span>
          </Link>

          {/* Desktop Navigation */}
          <nav className='hidden md:flex items-center gap-2 sm:gap-3'>
            {navLinks.map(({ title, to, icon: Icon }) => (
              <Link
                key={to}
                to={to}
                aria-label={title}
                className={cn(
                  'pixel-button items-center justify-center gap-2 px-2 py-2 h-9 text-sm font-semibold uppercase tracking-widest lg:justify-start lg:gap-3 lg:min-w-[90px] lg:px-3 bg-background/75 text-foreground border-[color:rgba(137,110,96,0.45)] hover:bg-accent/35 hover:text-accent-foreground dark:bg-input/30 dark:border-[color:rgba(255,255,255,0.18)] dark:hover:bg-accent/45 dark:hover:text-accent-foreground transition-all',
                  isAdmin && to === '/' ? 'hidden lg:inline-flex' : 'inline-flex'
                )}
                activeProps={{
                  className: 'bg-primary/20 text-primary border-[color:rgba(217,119,87,0.55)] dark:bg-primary/20 dark:border-[color:rgba(217,119,87,0.55)]'
                }}
              >
                <Icon className='size-[18px] shrink-0' />
                <span className='hidden lg:inline'>{title}</span>
              </Link>
            ))}
          </nav>

          {/* Mobile Base Navigation - Always show these */}
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

          {/* Mobile Navigation Dropdown - Only show for admin */}
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
                {adminNavLinksWithCustomRules.map(({ title, to, icon: Icon }) => (
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
          <ThemeSwitch />
          <UserMenu />
        </div>
      </div>
    </header>
  )
}
