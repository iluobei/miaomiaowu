// 导航菜单的**唯一**定义。
//
// 原先清单写在 topbar 里;做「排序 / 显示隐藏」时配置卡片也要拿同一份清单,
// 于是挪到这里,顶栏与设置卡片都从这里取。
import {
  Activity,
  FileCode,
  Files,
  FileStack,
  Link as LinkIcon,
  ListChecks,
  Network,
  Radar,
  ScrollText,
  Settings,
  Users,
  Zap,
} from 'lucide-react'
import { api } from '@/lib/api'

export const baseNavLinks = [
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
  {
    title: '模板管理',
    to: '/templates-v3',
    icon: FileStack,
  },
]

export const adminNavLinks = [
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
    title: '覆写管理',
    to: '/custom-rules',
    icon: FileCode,
  },
  {
    title: '规则集',
    to: '/rule-providers',
    icon: ListChecks,
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
  {
    title: '日志管理',
    to: '/logs',
    icon: ScrollText,
  },
]

export type NavLink = (typeof baseNavLinks)[number]

/** 管理员配的菜单顺序与隐藏项。 */
export interface NavMenuConfig {
  order: string[]
  hidden: string[]
}

/**
 * 把管理员配置套到算好的菜单上。
 *
 * 两条刻意的规矩:
 *  - 配置里没提到的菜单项**排在最后、保持原有相对顺序**,而不是消失。新版本加了
 *    菜单、而管理员的旧配置里没有它时,不该悄悄看不见。
 *  - 隐藏只影响导航,不影响路由 —— 知道地址照样进得去。它是整理界面用的,不是权限。
 */
export function applyNavMenuConfig<T extends { to: string }>(
  links: T[],
  config?: NavMenuConfig | null
): T[] {
  if (!config) return links
  const hidden = new Set(config.hidden ?? [])
  const visible = links.filter((l) => !hidden.has(l.to))
  const order = config.order ?? []
  if (order.length === 0) return visible
  const rank = new Map(order.map((to, i) => [to, i]))
  return visible
    .map((link, i) => ({ link, i }))
    .sort((a, b) => {
      const ra = rank.get(a.link.to)
      const rb = rank.get(b.link.to)
      if (ra === undefined && rb === undefined) return a.i - b.i
      if (ra === undefined) return 1
      if (rb === undefined) return -1
      return ra - rb
    })
    .map((x) => x.link)
}

/** 读管理员配的菜单顺序/隐藏。所有登录用户都要拿(导航是每个人都渲染的)。 */
export const navMenuQueryFn = async (): Promise<NavMenuConfig> => {
  const res = await api.get('/api/user/nav-menu')
  return (res.data?.config ?? { order: [], hidden: [] }) as NavMenuConfig
}

/**
 * 配置界面要列出的全部菜单项。
 *
 * 与运行时不同:这里不按角色过滤 —— 管理员得能给**所有**可能出现的菜单项排序。
 */
export function allNavLinksForConfig(): NavLink[] {
  return [...baseNavLinks, ...adminNavLinks]
}
