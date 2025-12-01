import { type QueryClient } from '@tanstack/react-query'
import { createRootRouteWithContext, Outlet } from '@tanstack/react-router'
import { ReactQueryDevtools } from '@tanstack/react-query-devtools'
import { TanStackRouterDevtools } from '@tanstack/react-router-devtools'
import { Toaster } from '@/components/ui/sonner'
import { NavigationProgress } from '@/components/navigation-progress'

export const Route = createRootRouteWithContext<{
  queryClient: QueryClient
}>()({
  component: () => {
    return (
      <>
        <NavigationProgress />
        <Outlet />
        <Toaster duration={5000} visibleToasts={5} />
        {import.meta.env.MODE === 'development' && (
          <>
            <ReactQueryDevtools buttonPosition='bottom-left' />
            <TanStackRouterDevtools position='bottom-right' />
          </>
        )}
      </>
    )
  },
  notFoundComponent: () => (
    <div className='flex min-h-svh flex-col items-center justify-center gap-4 px-4 text-center'>
      <h1 className='text-3xl font-semibold tracking-tight'>页面不存在</h1>
      <p className='text-muted-foreground'>请检查链接或返回首页。</p>
    </div>
  ),
  errorComponent: ({ error }) => (
    <div className='flex min-h-svh flex-col items-center justify-center gap-4 px-4 text-center'>
      <h1 className='text-3xl font-semibold tracking-tight'>发生错误</h1>
      <p className='text-muted-foreground'>{error?.message ?? '请稍后重试。'}</p>
    </div>
  ),
})
