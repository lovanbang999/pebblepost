import * as React from 'react'
import { cn } from '../../lib/utils'

interface TabsContextValue {
  value: string
  onValueChange: (value: string) => void
}

const TabsContext = React.createContext<TabsContextValue | undefined>(undefined)

interface TabsProps extends React.HTMLAttributes<HTMLDivElement> {
  value: string
  onValueChange: (value: string) => void
}

function Tabs({ value, onValueChange, className, children, ...props }: TabsProps) {
  return (
    <TabsContext.Provider value={{ value, onValueChange }}>
      <div className={cn('flex flex-col', className)} {...props}>
        {children}
      </div>
    </TabsContext.Provider>
  )
}

function TabsList({
  className,
  children,
  ...props
}: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div
      className={cn(
        'inline-flex h-9 items-center justify-start border-b border-zinc-200 dark:border-zinc-800 bg-transparent px-2 text-zinc-500 dark:text-zinc-400 gap-1',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

interface TabsTriggerProps extends React.ButtonHTMLAttributes<HTMLButtonElement> {
  value: string
}

function TabsTrigger({
  value,
  className,
  children,
  ...props
}: TabsTriggerProps) {
  const context = React.useContext(TabsContext)
  if (!context) throw new Error('TabsTrigger must be used within Tabs')

  const isActive = context.value === value

  return (
    <button
      type="button"
      onClick={() => context.onValueChange(value)}
      className={cn(
        'inline-flex items-center justify-center whitespace-nowrap px-3 py-1.5 text-xs font-medium transition-all focus-visible:outline-none disabled:pointer-events-none disabled:opacity-50 border-b-2 cursor-pointer',
        isActive
          ? 'border-blue-600 dark:border-blue-500 text-blue-600 dark:text-blue-400 font-semibold'
          : 'border-transparent text-zinc-500 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 hover:border-zinc-300 dark:hover:border-zinc-700',
        className
      )}
      {...props}
    >
      {children}
    </button>
  )
}

interface TabsContentProps extends React.HTMLAttributes<HTMLDivElement> {
  value: string
}

function TabsContent({
  value,
  className,
  children,
  ...props
}: TabsContentProps) {
  const context = React.useContext(TabsContext)
  if (!context) throw new Error('TabsContent must be used within Tabs')

  if (context.value !== value) return null

  return (
    <div
      className={cn(
        'flex-1 focus-visible:outline-none animate-in fade-in-50 duration-150',
        className
      )}
      {...props}
    >
      {children}
    </div>
  )
}

export { Tabs, TabsList, TabsTrigger, TabsContent }
