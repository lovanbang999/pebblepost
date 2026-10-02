import { useState, useEffect } from 'react'
import { TitleBar } from './components/layout/TitleBar'
import { CollectionTree } from './components/sidebar/CollectionTree'
import { RequestPanel } from './components/request/RequestPanel'
import { ResponsePanel } from './components/response/ResponsePanel'
import { QuickSearch } from './components/common/QuickSearch'
import { TooltipProvider } from './components/ui/tooltip'
import { useWorkspaceStore } from './store/workspaceStore'

export default function App() {
  const [quickSearchOpen, setQuickSearchOpen] = useState(false)
  const { loadWorkspace, workspacePath } = useWorkspaceStore()

  useEffect(() => {
    loadWorkspace(workspacePath || '.')
  }, [])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'k') {
        e.preventDefault()
        setQuickSearchOpen(prev => !prev)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [])

  return (
    <TooltipProvider delay={150}>
      <div className="flex flex-col h-screen w-screen overflow-hidden bg-zinc-50 dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100 select-none transition-colors duration-150">
        {/* Frameless / Web Titlebar */}
        <TitleBar onOpenQuickSearch={() => setQuickSearchOpen(true)} />

      {/* Main Workspace Layout */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Sidebar */}
        <CollectionTree />

        {/* Center Request Editor */}
        <div className="flex-1 flex overflow-hidden">
          <RequestPanel />

          {/* Right Response Viewer */}
          <ResponsePanel />
        </div>
      </div>

      {/* Quick Search Modal (Ctrl+K) */}
      <QuickSearch
        open={quickSearchOpen}
        onClose={() => setQuickSearchOpen(false)}
      />
    </div>
    </TooltipProvider>
  )
}

