import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '../../lib/utils'

const badgeVariants = cva(
  'inline-flex items-center rounded px-1.5 py-0.5 text-[10px] font-semibold transition-colors focus:outline-none focus:ring-1 focus:ring-ring font-mono uppercase tracking-wider',
  {
    variants: {
      variant: {
        default:
          'border border-transparent bg-blue-600/20 text-blue-400 border-blue-500/30',
        secondary:
          'border border-transparent bg-zinc-100 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300',
        destructive:
          'border border-red-500/30 bg-red-100 dark:bg-red-950/40 text-red-600 dark:text-red-400',
        outline: 'border border-zinc-200 dark:border-zinc-800 text-zinc-600 dark:text-zinc-400',
        success:
          'border border-emerald-500/30 bg-emerald-100 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400',
        warning:
          'border border-amber-500/30 bg-amber-100 dark:bg-amber-950/40 text-amber-600 dark:text-amber-400',
        info:
          'border border-sky-500/30 bg-sky-100 dark:bg-sky-950/40 text-sky-600 dark:text-sky-400',
      },
    },
    defaultVariants: {
      variant: 'default',
    },
  }
)

export interface BadgeProps
  extends React.HTMLAttributes<HTMLDivElement>,
    VariantProps<typeof badgeVariants> {}

function Badge({ className, variant, ...props }: BadgeProps) {
  return (
    <div className={cn(badgeVariants({ variant }), className)} {...props} />
  )
}

export { Badge, badgeVariants }
