import { createFileRoute, redirect } from '@tanstack/react-router'
import { useAuthStore } from '@/stores/auth-store'
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from '@/components/ui/card'

export const Route = createFileRoute('/subscribe-files/custom')({
  beforeLoad: () => {
    const token = useAuthStore.getState().auth.accessToken
    if (!token) {
      throw redirect({ to: '/' })
    }
  },
  component: CustomProxyGroupPage,
})

function CustomProxyGroupPage() {
  return (
    <main className='mx-auto w-full max-w-7xl px-4 py-8 sm:px-6 pt-24'>
      <section className='space-y-4'>
        <div>
          <h1 className='text-3xl font-semibold tracking-tight'>自定义代理组</h1>
          <p className='text-muted-foreground mt-2'>
            创建和管理自定义代理组配置
          </p>
        </div>

        <Card>
          <CardHeader>
            <CardTitle>功能开发中</CardTitle>
            <CardDescription>
              自定义代理组功能正在规划中，敬请期待
            </CardDescription>
          </CardHeader>
          <CardContent>
            <p className='text-sm text-muted-foreground'>
              此功能将允许您创建自定义的代理组配置，包括策略组、规则设置等。
            </p>
          </CardContent>
        </Card>
      </section>
    </main>
  )
}
