import { useEffect, useState } from 'react'
import { type QueryClient } from '@tanstack/react-query'
import {
  createRootRouteWithContext,
  Outlet,
  useRouterState,
} from '@tanstack/react-router'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { useAuthStore } from '@/stores/auth-store'
import { Toaster } from '@/components/ui/sonner'
import { AnimeStarfield } from '@/components/anime-starfield'
import { DebugFloatingViewer } from '@/components/debug-floating-viewer'
import { NavigationProgress } from '@/components/navigation-progress'
import { Topbar } from '@/components/layout/topbar'

function RootComponent() {
  const [isMobile, setIsMobile] = useState(false)
  const { auth } = useAuthStore()
  const pathname = useRouterState({ select: (s) => s.location.pathname })

  // Topbar 挂在根组件里、只渲染一次:页面间导航时它不再卸载重建 —— 既省掉每次
  // 重新测量溢出/重新拉 profile,也让液态玻璃的「流动滑块」能真正滑动而不是跳变
  // (滑块靠对同一个持久 DOM 容器做布局动画,topbar 每页重建就没有「上一个位置」可动)。
  const showTopbar =
    Boolean(auth.accessToken) && pathname !== '/login' && pathname !== '/404'

  useEffect(() => {
    const checkMobile = () => {
      setIsMobile(window.innerWidth < 768)
    }

    checkMobile()
    window.addEventListener('resize', checkMobile)

    return () => window.removeEventListener('resize', checkMobile)
  }, [])

  return (
    <>
      <NavigationProgress />
      {auth.accessToken && <AnimeStarfield />}
      {showTopbar && <Topbar />}
      <Outlet />
      <DebugFloatingViewer />
      <Toaster
        duration={5000}
        visibleToasts={5}
        position={isMobile ? 'top-center' : 'bottom-right'}
      />
      {import.meta.env.MODE === 'development' && (
        <>
          <ReactQueryDevtools buttonPosition='bottom-left' />
          <TanStackRouterDevtools position='bottom-right' />
        </>
      )}
    </>
  )
}

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient
}>()({
  component: RootComponent,
  notFoundComponent: () => (
    <div className='flex min-h-svh flex-col items-center justify-center gap-4 px-4 text-center'>
      <h1 className='text-3xl font-semibold tracking-tight'>页面不存在</h1>
      <p className='text-muted-foreground'>请检查链接或返回首页。</p>
    </div>
  ),
  errorComponent: ({ error }) => (
    <div className='flex min-h-svh flex-col items-center justify-center gap-4 px-4 text-center'>
      <h1 className='text-3xl font-semibold tracking-tight'>发生错误</h1>
      <p className='text-muted-foreground'>
        {error?.message ?? '请稍后重试。'}
      </p>
    </div>
  ),
})
