import { AxiosError } from 'axios'
import { toast } from 'sonner'

export function handleServerError(error: unknown) {
  // eslint-disable-next-line no-console
  console.log(error)

  let errMsg = 'Something went wrong!'

  if (
    error &&
    typeof error === 'object' &&
    'status' in error &&
    Number((error as { status?: unknown }).status) === 204
  ) {
    errMsg = 'Content not found.'
  }

  if (error instanceof AxiosError) {
    const data = error.response?.data as Record<string, unknown> | string | undefined
    if (typeof data === 'string') {
      errMsg = data
    } else if (data) {
      const record = data as Record<string, unknown>
      const messageFields = ['msg', 'message', 'error', 'title'] as const
      for (const field of messageFields) {
        const value = record[field]
        if (typeof value === 'string' && value.trim()) {
          errMsg = value
          break
        }
      }
    }
  }

  toast.error(errMsg)
}
