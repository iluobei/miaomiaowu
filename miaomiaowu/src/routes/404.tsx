import { createFileRoute } from '@tanstack/react-router'

export const Route = createFileRoute('/404')({
  component: NotFoundPage,
})

function NotFoundPage() {
  return (
    <div className='flex min-h-svh flex-col items-center justify-center gap-4 bg-background px-4 text-center'>
      <h1 className='text-3xl font-semibold tracking-tight'>404 Not Found</h1>
    </div>
  )
}
