import * as React from 'react'
import { cva, type VariantProps } from 'class-variance-authority'
import { cn } from '../../lib/utils'

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 whitespace-nowrap rounded-md text-xs font-medium transition-colors focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-blue-500 disabled:pointer-events-none disabled:opacity-50 select-none cursor-pointer',
  {
    variants: {
      variant: {
        default:
          'bg-blue-600 text-white shadow hover:bg-blue-500 active:bg-blue-700',
        destructive:
          'bg-red-600 text-white shadow-sm hover:bg-red-500 active:bg-red-700',
        outline:
          'border border-zinc-800 bg-transparent shadow-sm hover:bg-zinc-800/60 hover:text-zinc-100 text-zinc-300',
        secondary:
          'bg-zinc-800 text-zinc-100 shadow-sm hover:bg-zinc-700/80',
        ghost:
          'hover:bg-zinc-800/60 hover:text-zinc-100 text-zinc-400',
        link: 'text-blue-400 underline-offset-4 hover:underline',
        success:
          'bg-emerald-600/15 border border-emerald-500/40 text-emerald-400 hover:bg-emerald-600/25',
      },
      size: {
        default: 'h-8 px-3 py-1.5',
        sm: 'h-7 rounded-md px-2.5 text-xs',
        lg: 'h-9 rounded-md px-5 text-sm',
        icon: 'h-7 w-7',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'default',
    },
  }
)

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {}

const Button = React.forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, ...props }, ref) => {
    return (
      <button
        className={cn(buttonVariants({ variant, size, className }))}
        ref={ref}
        {...props}
      />
    )
  }
)
Button.displayName = 'Button'

export { Button, buttonVariants }
