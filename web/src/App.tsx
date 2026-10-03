import { useState, useEffect } from 'react'
import { TitleBar } from './components/layout/TitleBar'
import { CollectionTree } from './components/sidebar/CollectionTree'
import { RequestPanel } from './components/request/RequestPanel'
import { ResponsePanel } from './components/response/ResponsePanel'
import { QuickSearch } from './components/common/QuickSearch'
import { ConflictDialog } from './components/common/ConflictDialog'
import { CloseTabConfirmDialog } from './components/common/CloseTabConfirmDialog'
import { TooltipProvider } from './components/ui/tooltip'
import { useWorkspaceStore } from './store/workspaceStore'
import { useTabStore } from './store/tabStore'

export default function App() {
  const [quickSearchOpen, setQuickSearchOpen] = useState(false)
  const { loadWorkspace, workspacePath, initWatcher, cleanupWatcher } = useWorkspaceStore()
  const { restoreTabs, saveCurrentTab } = useTabStore()

  useEffect(() => {
    const ws = workspacePath || '.'
    loadWorkspace(ws)
    initWatcher()
    restoreTabs(ws)
    return () => {
      cleanupWatcher()
    }
  }, [])

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      const isMod = e.ctrlKey || e.metaKey

      // Quick Search (Ctrl+K / Cmd+K)
      if (isMod && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setQuickSearchOpen((prev) => !prev)
        return
      }

      // Save Request (Ctrl+S / Cmd+S)
      if (isMod && !e.shiftKey && e.key.toLowerCase() === 's') {
        e.preventDefault()
        saveCurrentTab()
        return
      }

      // Close Active Tab (Ctrl+W / Cmd+W)
      if (isMod && !e.shiftKey && e.key.toLowerCase() === 'w') {
        e.preventDefault()
        const currentActive = useTabStore.getState().activeTabId
        if (currentActive) {
          useTabStore.getState().closeTab(currentActive)
        }
        return
      }

      // Reopen Closed Tab (Ctrl+Shift+T / Cmd+Shift+T)
      if (isMod && e.shiftKey && e.key.toLowerCase() === 't') {
        e.preventDefault()
        useTabStore.getState().reopenLastClosedTab()
        return
      }

      // Cycle Tabs (Ctrl+Tab or Ctrl+Shift+Tab)
      if (isMod && e.key === 'Tab') {
        e.preventDefault()
        useTabStore.getState().cycleTabs(e.shiftKey ? 'prev' : 'next')
        return
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

        {/* External File Change Conflict Dialog */}
        <ConflictDialog />

        {/* Unsaved Tab Close Confirmation Dialog */}
        <CloseTabConfirmDialog />
      </div>
    </TooltipProvider>
  )
}

