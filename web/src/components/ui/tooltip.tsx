import * as React from 'react'
import { cn } from '../../lib/utils'

interface TooltipProps {
  content: React.ReactNode
  children: React.ReactNode
  className?: string
  side?: 'top' | 'bottom' | 'left' | 'right'
}

function Tooltip({
  content,
  children,
  className,
  side = 'bottom',
}: TooltipProps) {
  const [isOpen, setIsOpen] = React.useState(false)

  const sideClasses = {
    top: 'bottom-full left-1/2 -translate-x-1/2 mb-1.5',
    bottom: 'top-full left-1/2 -translate-x-1/2 mt-1.5',
    left: 'right-full top-1/2 -translate-y-1/2 mr-1.5',
    right: 'left-full top-1/2 -translate-y-1/2 ml-1.5',
  }

  return (
    <div
      className="relative inline-flex"
      onMouseEnter={() => setIsOpen(true)}
      onMouseLeave={() => setIsOpen(false)}
      onFocus={() => setIsOpen(true)}
      onBlur={() => setIsOpen(false)}
    >
      {children}
      {isOpen && (
        <div
          className={cn(
            'absolute z-50 overflow-hidden rounded bg-zinc-900 border border-zinc-700 px-2 py-1 text-[11px] text-zinc-100 shadow-md animate-in fade-in-0 zoom-in-95 pointer-events-none whitespace-nowrap',
            sideClasses[side],
            className
          )}
        >
          {content}
        </div>
      )}
    </div>
  )
}

export { Tooltip }
