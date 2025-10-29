import * as React from 'react'
import { Slot } from '@radix-ui/react-slot'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '@/lib/utils'

const buttonVariants = cva(
  "pixel-button inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-none text-sm font-medium disabled:pointer-events-none disabled:opacity-50 [&_svg]:pointer-events-none [&_svg:not([class*='size-'])]:size-4 shrink-0 [&_svg]:shrink-0 outline-none focus-visible:ring-ring/60 focus-visible:ring-4 focus-visible:ring-offset-2 focus-visible:ring-offset-[color:var(--background)] transition-all",
  {
    variants: {
      variant: {
        default:
          "bg-primary text-primary-foreground border-[color:rgba(217,119,87,0.5)] hover:bg-primary/85 hover:border-[color:rgba(217,119,87,0.7)]",
        destructive:
          "bg-destructive text-white border-[color:rgba(239,68,68,0.65)] hover:bg-destructive/85 hover:border-[color:rgba(239,68,68,0.85)] focus-visible:ring-destructive/30 dark:bg-destructive/70",
        outline:
          "bg-background/75 text-foreground border-[color:rgba(137,110,96,0.45)] hover:bg-accent/35 hover:text-accent-foreground dark:bg-input/30 dark:border-[color:rgba(255,255,255,0.18)] dark:hover:bg-accent/45 dark:hover:text-accent-foreground",
        secondary:
          "bg-secondary text-secondary-foreground border-[color:rgba(241,140,110,0.38)] hover:bg-secondary/80",
        ghost:
          "border-transparent bg-transparent hover:bg-accent/40 hover:text-accent-foreground dark:hover:bg-accent/30",
        link:
          'border-transparent bg-transparent text-primary underline-offset-4 hover:underline',
      },
      size: {
        default: 'h-10 px-5 has-[>svg]:px-4',
        sm: 'h-9 gap-1.5 px-4 has-[>svg]:px-3',
        lg: 'h-11 px-7 has-[>svg]:px-5',
        icon: 'size-10',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  }
)

function Button({
  className,
  variant,
  size,
  asChild = false,
  ...props
}: React.ComponentProps<'button'> &
  VariantProps<typeof buttonVariants> & {
    asChild?: boolean
  }) {
  const Comp = asChild ? Slot : 'button'

  return (
    <Comp
      data-slot='button'
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
