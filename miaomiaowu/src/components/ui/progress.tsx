import * as React from 'react'
import { cn } from '@/lib/utils'

export interface ProgressProps extends React.ComponentProps<'div'> {
  value?: number
  max?: number
}

export const Progress = React.forwardRef<HTMLDivElement, ProgressProps>(
  ({ className, value = 0, max = 100, ...props }, ref) => {
    const percentage = Math.min(Math.max((value / max) * 100, 0), 100)

    return (
      <div
        ref={ref}
        data-slot='progress'
        role='progressbar'
        aria-valuemin={0}
        aria-valuemax={max}
        aria-valuenow={percentage}
        className={cn(
          'relative h-2 w-full overflow-hidden rounded-full bg-muted',
          className
        )}
        {...props}
      >
        <div
          className='h-full w-full flex-1 bg-primary transition-all'
          style={{ transform: `translateX(${percentage - 100}%)` }}
        />
      </div>
    )
  }
)

Progress.displayName = 'Progress'
