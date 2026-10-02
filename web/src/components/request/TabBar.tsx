import { useState, useRef, useEffect } from 'react'
import { X, Plus, RotateCcw } from 'lucide-react'
import { useTabStore, type RequestTab } from '../../store/tabStore'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { getMethodTextColor, cn } from '../../lib/utils'
import { Tooltip } from '../ui/tooltip'

interface TabContextMenuState {
  x: number
  y: number
  tab: RequestTab
}

export function TabBar() {
  const {
    tabs,
    activeTabId,
    closedTabsHistory,
    setActiveTabId,
    closeTab,
    pinTab,
    reopenLastClosedTab,
  } = useTabStore()

  const { createNewRequest } = useWorkspaceStore()
  const [contextMenu, setContextMenu] = useState<TabContextMenuState | null>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  // Close context menu on outside click or scroll
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setContextMenu(null)
      }
    }
    const handleScroll = () => setContextMenu(null)

    if (contextMenu) {
      window.addEventListener('mousedown', handleOutsideClick)
      window.addEventListener('scroll', handleScroll, true)
    }
    return () => {
      window.removeEventListener('mousedown', handleOutsideClick)
      window.removeEventListener('scroll', handleScroll, true)
    }
  }, [contextMenu])

  const handleContextMenu = (e: React.MouseEvent, tab: RequestTab) => {
    e.preventDefault()
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      tab,
    })
  }

  const handleCloseOthers = (targetId: string) => {
    tabs.forEach((t) => {
      if (t.id !== targetId) {
        closeTab(t.id)
      }
    })
    setContextMenu(null)
  }

  const handleCloseToRight = (targetId: string) => {
    const targetIdx = tabs.findIndex((t) => t.id === targetId)
    if (targetIdx !== -1) {
      tabs.slice(targetIdx + 1).forEach((t) => closeTab(t.id))
    }
    setContextMenu(null)
  }

  const handleCloseSaved = () => {
    tabs.forEach((t) => {
      if (!t.isDirty) {
        closeTab(t.id)
      }
    })
    setContextMenu(null)
  }

  if (tabs.length === 0) {
    return null
  }

  return (
    <div className="h-9 bg-zinc-100/70 dark:bg-zinc-900/50 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between select-none overflow-hidden shrink-0">
      {/* Scrollable Tabs Container */}
      <div className="flex-1 flex items-center overflow-x-auto no-scrollbar h-full">
        {tabs.map((tab) => {
          const isActive = tab.id === activeTabId

          return (
            <div
              key={tab.id}
              onClick={() => setActiveTabId(tab.id)}
              onDoubleClick={() => pinTab(tab.id)}
              onAuxClick={(e) => {
                if (e.button === 1) {
                  e.preventDefault()
                  closeTab(tab.id)
                }
              }}
              onContextMenu={(e) => handleContextMenu(e, tab)}
              className={cn(
                'group relative flex items-center gap-2 h-full px-3 text-xs border-r border-zinc-200 dark:border-zinc-800 cursor-pointer transition-colors max-w-56 shrink-0',
                isActive
                  ? 'bg-white dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100 font-medium border-t-2 border-t-blue-500 shadow-2xs'
                  : 'text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200/50 dark:hover:bg-zinc-800/40 hover:text-zinc-900 dark:hover:text-zinc-200 border-t-2 border-t-transparent'
              )}
            >
              {/* Method badge */}
              <span
                className={cn(
                  'text-[9.5px] font-mono font-bold tracking-tight uppercase shrink-0',
                  getMethodTextColor(tab.method)
                )}
              >
                {tab.method}
              </span>

              {/* Tab Title (italic if preview) */}
              <span
                className={cn(
                  'truncate flex-1',
                  tab.isPreview && 'italic text-zinc-500 dark:text-zinc-400'
                )}
                title={tab.filePath}
              >
                {tab.title}
              </span>

              {/* Close Button or Unsaved Dot Indicator */}
              <div className="w-4 h-4 flex items-center justify-center shrink-0">
                {tab.isDirty ? (
                  <>
                    {/* Circle dot normally */}
                    <span className="w-2 h-2 rounded-full bg-blue-500 dark:bg-blue-400 group-hover:hidden" />
                    {/* Cross icon on hover */}
                    <button
                      type="button"
                      onClick={(e) => {
                        e.stopPropagation()
                        closeTab(tab.id)
                      }}
                      className="hidden group-hover:flex w-3.5 h-3.5 items-center justify-center rounded hover:bg-zinc-200 dark:hover:bg-zinc-800 text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 cursor-pointer"
                      title="Close (Ctrl+W)"
                    >
                      <X className="w-3 h-3" />
                    </button>
                  </>
                ) : (
                  <button
                    type="button"
                    onClick={(e) => {
                      e.stopPropagation()
                      closeTab(tab.id)
                    }}
                    className="opacity-0 group-hover:opacity-100 flex w-3.5 h-3.5 items-center justify-center rounded hover:bg-zinc-200 dark:hover:bg-zinc-800 text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 transition-opacity cursor-pointer"
                    title="Close (Ctrl+W / Middle Click)"
                  >
                    <X className="w-3 h-3" />
                  </button>
                )}
              </div>
            </div>
          )
        })}
      </div>

      {/* Right Action Icons (New Request, Reopen Closed Tab) */}
      <div className="flex items-center px-2 gap-1 shrink-0 text-zinc-400 dark:text-zinc-500">
        {closedTabsHistory.length > 0 && (
          <Tooltip content="Reopen Closed Tab (Ctrl+Shift+T)">
            <button
              type="button"
              onClick={reopenLastClosedTab}
              className="p-1 rounded hover:bg-zinc-200 dark:hover:bg-zinc-800 hover:text-zinc-800 dark:hover:text-zinc-200 transition-colors cursor-pointer"
            >
              <RotateCcw className="w-3.5 h-3.5" />
            </button>
          </Tooltip>
        )}
        <Tooltip content="New Request in collections">
          <button
            type="button"
            onClick={() => createNewRequest()}
            className="p-1 rounded hover:bg-zinc-200 dark:hover:bg-zinc-800 hover:text-zinc-800 dark:hover:text-zinc-200 transition-colors cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
        </Tooltip>
      </div>

      {/* Tab Context Menu */}
      {contextMenu && (
        <div
          ref={menuRef}
          style={{ top: `${contextMenu.y}px`, left: `${contextMenu.x}px` }}
          className="fixed z-50 min-w-44 py-1 bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg shadow-xl text-xs text-zinc-700 dark:text-zinc-200 animate-in fade-in zoom-in-95 duration-100"
        >
          <button
            type="button"
            onClick={() => {
              closeTab(contextMenu.tab.id)
              setContextMenu(null)
            }}
            className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
          >
            <span>Close</span>
            <span className="text-[10px] text-zinc-400">Ctrl+W</span>
          </button>
          <button
            type="button"
            onClick={() => handleCloseOthers(contextMenu.tab.id)}
            className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
          >
            <span>Close Others</span>
          </button>
          <button
            type="button"
            onClick={() => handleCloseToRight(contextMenu.tab.id)}
            className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
          >
            <span>Close to the Right</span>
          </button>
          <button
            type="button"
            onClick={handleCloseSaved}
            className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
          >
            <span>Close Saved</span>
          </button>
          <div className="h-px bg-zinc-200 dark:bg-zinc-800 my-1" />
          <button
            type="button"
            onClick={() => {
              pinTab(contextMenu.tab.id)
              setContextMenu(null)
            }}
            className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
          >
            <span>{contextMenu.tab.isPreview ? 'Pin Tab' : 'Unpin Tab'}</span>
            <span className="text-[10px] text-zinc-400">Double click</span>
          </button>
        </div>
      )}
    </div>
  )
}
