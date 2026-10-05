import { Layers, Clock, GitBranch } from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { useGitStore } from '../../store/gitStore'
import { Tooltip } from '../ui/tooltip'
import { cn } from '../../lib/utils'

export function ActivityBar() {
  const { sidebarView, setSidebarView } = useWorkspaceStore()
  const { isPanelOpen, openPanel, closePanel, status } = useGitStore()

  const handleToggleGit = () => {
    if (isPanelOpen) {
      closePanel()
    } else {
      openPanel('files')
    }
  }

  return (
    <nav
      aria-label="Activity Bar"
      className="w-11 bg-zinc-100 dark:bg-zinc-900/90 border-r border-zinc-200 dark:border-zinc-800 flex flex-col items-center py-2 gap-1.5 shrink-0 select-none z-10 transition-colors duration-150"
    >
      {/* Collections Button */}
      <Tooltip content="Collections" side="right">
        <button
          type="button"
          aria-label="Collections"
          onClick={() => setSidebarView('collections')}
          className={cn(
            'w-8 h-8 rounded-lg flex items-center justify-center transition-all relative',
            sidebarView === 'collections'
              ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 shadow-2xs font-semibold'
              : 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 hover:bg-zinc-200/60 dark:hover:bg-zinc-800/50'
          )}
        >
          <Layers className="w-4 h-4" />
          {sidebarView === 'collections' && (
            <span className="absolute left-0 top-1.5 bottom-1.5 w-0.5 bg-blue-600 dark:bg-blue-500 rounded-r" />
          )}
        </button>
      </Tooltip>

      {/* Git Source Control Button */}
      <Tooltip content="Source Control (Git)" side="right">
        <button
          type="button"
          aria-label="Source Control"
          onClick={handleToggleGit}
          className={cn(
            'w-8 h-8 rounded-lg flex items-center justify-center transition-all relative',
            isPanelOpen
              ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 shadow-2xs font-semibold'
              : 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 hover:bg-zinc-200/60 dark:hover:bg-zinc-800/50'
          )}
        >
          <GitBranch className="w-4 h-4" />
          {status?.dirty && (
            <span
              className="absolute top-1 right-1 w-2 h-2 rounded-full bg-amber-500"
              title="Uncommitted changes"
            />
          )}
          {isPanelOpen && (
            <span className="absolute left-0 top-1.5 bottom-1.5 w-0.5 bg-blue-600 dark:bg-blue-500 rounded-r" />
          )}
        </button>
      </Tooltip>

      {/* History Button */}
      <Tooltip content="Request History" side="right">
        <button
          type="button"
          aria-label="Request History"
          onClick={() => setSidebarView('history')}
          className={cn(
            'w-8 h-8 rounded-lg flex items-center justify-center transition-all relative',
            sidebarView === 'history'
              ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 shadow-2xs font-semibold'
              : 'text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 hover:bg-zinc-200/60 dark:hover:bg-zinc-800/50'
          )}
        >
          <Clock className="w-4 h-4" />
          {sidebarView === 'history' && (
            <span className="absolute left-0 top-1.5 bottom-1.5 w-0.5 bg-blue-600 dark:bg-blue-500 rounded-r" />
          )}
        </button>
      </Tooltip>
    </nav>
  )
}
