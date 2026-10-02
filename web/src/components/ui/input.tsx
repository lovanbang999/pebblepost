import * as React from 'react'
import { cn } from '../../lib/utils'

export interface InputProps
  extends React.InputHTMLAttributes<HTMLInputElement> {}

const Input = React.forwardRef<HTMLInputElement, InputProps>(
  ({ className, type, ...props }, ref) => {
    return (
      <input
        type={type}
        className={cn(
          'flex h-8 w-full rounded-md border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 px-3 py-1 text-xs text-zinc-900 dark:text-zinc-100 shadow-xs transition-colors file:border-0 file:bg-transparent file:text-xs file:font-medium placeholder:text-zinc-400 dark:placeholder:text-zinc-500 focus-visible:outline-none focus-visible:border-blue-500 focus-visible:ring-1 focus-visible:ring-blue-500 disabled:cursor-not-allowed disabled:opacity-50 font-mono',
          className
        )}
        ref={ref}
        {...props}
      />
    )
  }
)
Input.displayName = 'Input'

export { Input }
