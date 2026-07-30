import type { LucideIcon } from 'lucide-react'
import { cn } from '@/lib/utils'

const ANIME_NAV_ICONS: Record<string, string> = {
  '/': '/images/anime-nav/home.png',
  '/subscription': '/images/anime-nav/subscription.png',
  '/generator': '/images/anime-nav/generator.png',
  '/nodes': '/images/anime-nav/nodes.png',
  '/subscribe-files': '/images/anime-nav/subscribe-files.png',
  '/templates-v3': '/images/anime-nav/templates.png',
  '/custom-rules': '/images/anime-nav/custom-rules.png',
  '/probe': '/images/anime-nav/servers.png',
  '/users': '/images/anime-nav/users.png',
  '/system-settings': '/images/anime-nav/settings.png',
}

interface NavIconProps {
  icon: LucideIcon
  to: string
  className?: string
}

export function NavIcon({ icon: Icon, to, className }: NavIconProps) {
  const anime = ANIME_NAV_ICONS[to]

  return (
    <>
      <Icon className={cn(className, anime && 'nav-icon-lucide')} />
      {anime && (
        <img
          src={anime}
          alt=''
          aria-hidden='true'
          className={cn(className, 'nav-icon-anime')}
        />
      )}
    </>
  )
}
