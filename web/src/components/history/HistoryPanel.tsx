import { useEffect, useState, useMemo } from 'react'
import {
  Search,
  RotateCcw,
  Trash2,
  ArrowLeftRight,
  ExternalLink,
  Clock,
  AlertTriangle,
} from 'lucide-react'
import { useHistoryStore, type TimeRangeFilter } from '../../store/historyStore'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { useTabStore } from '../../store/tabStore'
import type { HistoryEntry } from '../../types'
import { getMethodTextColor, cn } from '../../lib/utils'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Tooltip } from '../ui/tooltip'
import {
  Select,
  SelectTrigger,
  SelectValue,
  SelectContent,
  SelectItem,
} from '../ui/select'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../ui/dialog'
import { CompareHistoryDialog } from './CompareHistoryDialog'

function getStatusBadgeClass(code: number): string {
  if (code >= 200 && code < 300) {
    return 'bg-emerald-50 dark:bg-emerald-950/40 text-emerald-600 dark:text-emerald-400 border-emerald-300 dark:border-emerald-800'
  }
  if (code >= 300 && code < 400) {
    return 'bg-sky-50 dark:bg-sky-950/40 text-sky-600 dark:text-sky-400 border-sky-300 dark:border-sky-800'
  }
  if (code >= 400 && code < 500) {
    return 'bg-amber-50 dark:bg-amber-950/40 text-amber-600 dark:text-amber-400 border-amber-300 dark:border-amber-800'
  }
  return 'bg-rose-50 dark:bg-rose-950/40 text-rose-600 dark:text-rose-400 border-rose-300 dark:border-rose-800'
}

function formatBytes(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  return `${(bytes / 1024).toFixed(1)} KB`
}

function formatTime(iso: string): string {
  try {
    const d = new Date(iso)
    return d.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return ''
  }
}

function getDateGroupLabel(iso: string): string {
  try {
    const d = new Date(iso)
    const today = new Date()
    const yesterday = new Date(today)
    yesterday.setDate(yesterday.getDate() - 1)

    if (d.toDateString() === today.toDateString()) return 'Today'
    if (d.toDateString() === yesterday.toDateString()) return 'Yesterday'
    return d.toLocaleDateString([], { month: 'short', day: 'numeric', year: 'numeric' })
  } catch {
    return 'Earlier'
  }
}

export function HistoryPanel() {
  const { workspacePath } = useWorkspaceStore()
  const {
    entries,
    total,
    isLoading,
    filter,
    setFilter,
    loadHistory,
    deleteEntry,
    clearHistory,
    compareIds,
    selectForCompare,
    clearCompare,
    loadCompare,
  } = useHistoryStore()

  const { openHistoryTab, restoreRequestFromHistory } = useTabStore()

  const [searchInput, setSearchInput] = useState(filter.search)
  const [clearDialogOpen, setClearDialogOpen] = useState(false)
  const [compareModalOpen, setCompareModalOpen] = useState(false)

  // Reload history when workspace or filter changes
  useEffect(() => {
    if (workspacePath) {
      loadHistory(workspacePath)
    }
  }, [workspacePath, filter, loadHistory])

  // Group entries by date
  const groupedEntries = useMemo(() => {
    const map = new Map<string, HistoryEntry[]>()
    for (const e of entries) {
      const group = getDateGroupLabel(e.executedAt)
      if (!map.has(group)) map.set(group, [])
      map.get(group)!.push(e)
    }
    return Array.from(map.entries())
  }, [entries])

  const handleSearchSubmit = (e: React.FormEvent) => {
    e.preventDefault()
    setFilter({ search: searchInput })
  }

  const handleClearHistory = async () => {
    if (workspacePath) {
      await clearHistory(workspacePath)
      setClearDialogOpen(false)
    }
  }

  const handleStartCompare = async () => {
    if (compareIds && compareIds[0] !== compareIds[1]) {
      await loadCompare()
      setCompareModalOpen(true)
    }
  }

  return (
    <aside className="w-72 bg-white dark:bg-zinc-950 border-r border-zinc-200 dark:border-zinc-800 flex flex-col shrink-0 select-none transition-colors duration-150 h-full">
      {/* Search and Filters Header */}
      <div className="p-2.5 border-b border-zinc-200 dark:border-zinc-800 space-y-2">
        <form onSubmit={handleSearchSubmit} className="relative">
          <Search className="w-3.5 h-3.5 absolute left-2.5 top-2.5 text-zinc-400 pointer-events-none" />
          <Input
            value={searchInput}
            onChange={(e) => setSearchInput(e.target.value)}
            onBlur={() => setFilter({ search: searchInput })}
            placeholder="Search history (URL, name)..."
            className="h-8 pl-8 pr-2 text-xs bg-zinc-50 dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800"
          />
        </form>

        {/* Filter Selectors Bar */}
        <div className="grid grid-cols-3 gap-1 text-[11px]">
          {/* Method Filter */}
          <Select
            value={filter.method || 'ALL'}
            onValueChange={(val) => {
              if (typeof val === 'string') {
                setFilter({ method: val === 'ALL' ? '' : val })
              }
            }}
          >
            <SelectTrigger className="h-7 px-2 text-[11px] bg-zinc-100 dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800 text-zinc-700 dark:text-zinc-300">
              <SelectValue placeholder="Method" />
            </SelectTrigger>
            <SelectContent align="start">
              <SelectItem value="ALL">All Methods</SelectItem>
              <SelectItem value="GET">GET</SelectItem>
              <SelectItem value="POST">POST</SelectItem>
              <SelectItem value="PUT">PUT</SelectItem>
              <SelectItem value="DELETE">DELETE</SelectItem>
              <SelectItem value="PATCH">PATCH</SelectItem>
            </SelectContent>
          </Select>

          {/* Status Filter */}
          <Select
            value={String(filter.statusCode)}
            onValueChange={(val) => {
              if (typeof val === 'string') {
                setFilter({ statusCode: Number(val) })
              }
            }}
          >
            <SelectTrigger className="h-7 px-2 text-[11px] bg-zinc-100 dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800 text-zinc-700 dark:text-zinc-300">
              <SelectValue placeholder="Status" />
            </SelectTrigger>
            <SelectContent align="center">
              <SelectItem value="0">All Status</SelectItem>
              <SelectItem value="200">200 OK</SelectItem>
              <SelectItem value="201">201 Created</SelectItem>
              <SelectItem value="400">400 Bad Request</SelectItem>
              <SelectItem value="401">401 Unauthorized</SelectItem>
              <SelectItem value="404">404 Not Found</SelectItem>
              <SelectItem value="500">500 Server Error</SelectItem>
            </SelectContent>
          </Select>

          {/* Time Range Filter */}
          <Select
            value={filter.timeRange}
            onValueChange={(val) => {
              if (typeof val === 'string') {
                setFilter({ timeRange: val as TimeRangeFilter })
              }
            }}
          >
            <SelectTrigger className="h-7 px-2 text-[11px] bg-zinc-100 dark:bg-zinc-900 border-zinc-200 dark:border-zinc-800 text-zinc-700 dark:text-zinc-300">
              <SelectValue placeholder="Time" />
            </SelectTrigger>
            <SelectContent align="end">
              <SelectItem value="all">All Time</SelectItem>
              <SelectItem value="today">Today</SelectItem>
              <SelectItem value="week">Past 7d</SelectItem>
              <SelectItem value="month">Past 30d</SelectItem>
            </SelectContent>
          </Select>
        </div>

        {/* Actions row: total count & clear history */}
        <div className="flex items-center justify-between pt-1 text-[11px] text-zinc-500">
          <span>{total} recorded runs</span>
          <div className="flex items-center gap-1">
            <Tooltip content="Refresh History">
              <Button
                variant="ghost"
                size="icon"
                onClick={() => workspacePath && loadHistory(workspacePath)}
                className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-200"
              >
                <RotateCcw className="w-3.5 h-3.5" />
              </Button>
            </Tooltip>
            {entries.length > 0 && (
              <Tooltip content="Clear History (Latest 500 auto-kept)">
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => setClearDialogOpen(true)}
                  className="h-6 w-6 text-rose-500 hover:text-rose-600 hover:bg-rose-50 dark:hover:bg-rose-950/40"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </Button>
              </Tooltip>
            )}
          </div>
        </div>
      </div>

      {/* Compare Mode Banner */}
      {compareIds && (
        <div className="px-3 py-2 bg-indigo-50 dark:bg-indigo-950/50 border-b border-indigo-200 dark:border-indigo-800/80 text-xs flex items-center justify-between animate-in fade-in duration-150">
          <div className="flex items-center gap-1.5 text-indigo-700 dark:text-indigo-300 font-medium">
            <ArrowLeftRight className="w-3.5 h-3.5" />
            <span>
              {compareIds[0] === compareIds[1]
                ? 'Select 2nd entry to compare'
                : '2 entries selected'}
            </span>
          </div>
          <div className="flex items-center gap-1">
            {compareIds[0] !== compareIds[1] && (
              <Button
                size="sm"
                onClick={handleStartCompare}
                className="h-6 px-2 text-[10px] bg-indigo-600 hover:bg-indigo-500 text-white font-semibold"
              >
                Compare
              </Button>
            )}
            <Button
              variant="ghost"
              size="sm"
              onClick={clearCompare}
              className="h-6 px-1.5 text-[10px] text-zinc-500 hover:text-zinc-900"
            >
              Cancel
            </Button>
          </div>
        </div>
      )}

      {/* Entry List */}
      <div className="flex-1 overflow-y-auto divide-y divide-zinc-100 dark:divide-zinc-900">
        {isLoading && entries.length === 0 ? (
          <div className="p-8 text-center text-xs text-zinc-400">Loading history...</div>
        ) : entries.length === 0 ? (
          <div className="p-8 text-center text-xs text-zinc-400 flex flex-col items-center gap-2">
            <Clock className="w-8 h-8 text-zinc-300 dark:text-zinc-700" />
            <p className="font-semibold text-zinc-600 dark:text-zinc-400">No Request History</p>
            <p className="text-[11px] text-zinc-400">
              Every request execution is automatically recorded with secrets masked.
            </p>
          </div>
        ) : (
          groupedEntries.map(([dateGroup, items]) => (
            <div key={dateGroup} className="py-1">
              <div className="px-3 py-1 text-[10px] font-semibold text-zinc-400 uppercase tracking-wider bg-zinc-50/50 dark:bg-zinc-900/30">
                {dateGroup}
              </div>
              {items.map((entry) => {
                const isSelectedForCompare =
                  compareIds && (compareIds[0] === entry.id || compareIds[1] === entry.id)

                return (
                  <div
                    key={entry.id}
                    className={cn(
                      'group relative px-3 py-2 hover:bg-zinc-100/70 dark:hover:bg-zinc-900/60 transition-colors cursor-pointer text-xs',
                      isSelectedForCompare && 'bg-indigo-50/80 dark:bg-indigo-950/40 border-l-2 border-indigo-500'
                    )}
                    onClick={() => openHistoryTab(entry)}
                  >
                    {/* Top row: Method, Path/Name, Status */}
                    <div className="flex items-center justify-between gap-1.5 mb-1">
                      <div className="flex items-center gap-1.5 min-w-0 flex-1">
                        <span
                          className={cn(
                            'font-mono font-bold text-[10px] uppercase shrink-0',
                            getMethodTextColor(entry.method)
                          )}
                        >
                          {entry.method}
                        </span>
                        <span
                          className="font-medium truncate text-zinc-800 dark:text-zinc-200"
                          title={entry.url}
                        >
                          {entry.requestName || entry.url}
                        </span>
                      </div>
                      <Badge
                        variant="outline"
                        className={cn(
                          'text-[9px] px-1 py-0 h-4 shrink-0 font-mono font-semibold',
                          getStatusBadgeClass(entry.statusCode)
                        )}
                      >
                        {entry.statusCode}
                      </Badge>
                    </div>

                    {/* Bottom row: Timing, Size, Timestamp */}
                    <div className="flex items-center justify-between text-[10px] text-zinc-400">
                      <div className="flex items-center gap-1.5">
                        <span>{entry.durationMs.toFixed(0)}ms</span>
                        <span>•</span>
                        <span>{formatBytes(entry.sizeBytes)}</span>
                      </div>
                      <span>{formatTime(entry.executedAt)}</span>
                    </div>

                    {/* Action buttons overlay on hover */}
                    <div className="absolute right-2 top-2 hidden group-hover:flex items-center gap-1 bg-white dark:bg-zinc-900 p-0.5 rounded shadow-sm border border-zinc-200 dark:border-zinc-800">
                      <Tooltip content="Reopen in read-only tab">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            openHistoryTab(entry)
                          }}
                          className="p-1 hover:bg-zinc-100 dark:hover:bg-zinc-800 rounded text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-200"
                        >
                          <ExternalLink className="w-3 h-3" />
                        </button>
                      </Tooltip>

                      <Tooltip content="Restore this request">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            restoreRequestFromHistory(entry)
                          }}
                          className="p-1 hover:bg-zinc-100 dark:hover:bg-zinc-800 rounded text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-200"
                        >
                          <RotateCcw className="w-3 h-3" />
                        </button>
                      </Tooltip>

                      <Tooltip content="Select for compare">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            selectForCompare(entry.id)
                          }}
                          className={cn(
                            'p-1 hover:bg-zinc-100 dark:hover:bg-zinc-800 rounded',
                            isSelectedForCompare
                              ? 'text-indigo-600 dark:text-indigo-400'
                              : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-200'
                          )}
                        >
                          <ArrowLeftRight className="w-3 h-3" />
                        </button>
                      </Tooltip>

                      <Tooltip content="Delete entry">
                        <button
                          type="button"
                          onClick={(e) => {
                            e.stopPropagation()
                            if (workspacePath) deleteEntry(workspacePath, entry.id)
                          }}
                          className="p-1 hover:bg-rose-50 dark:hover:bg-rose-950/50 rounded text-zinc-400 hover:text-rose-500"
                        >
                          <Trash2 className="w-3 h-3" />
                        </button>
                      </Tooltip>
                    </div>
                  </div>
                )
              })}
            </div>
          ))
        )}
      </div>

      {/* Clear History Confirmation Dialog */}
      {clearDialogOpen && (
        <Dialog open={clearDialogOpen} onOpenChange={setClearDialogOpen}>
          <DialogContent className="max-w-md p-6">
            <DialogHeader>
              <div className="flex items-center gap-2 text-rose-600 mb-1">
                <AlertTriangle className="w-5 h-5 shrink-0" />
                <DialogTitle className="text-base font-semibold">Clear Request History</DialogTitle>
              </div>
              <DialogDescription className="text-xs text-zinc-600 dark:text-zinc-400 leading-relaxed">
                Are you sure you want to clear all historical execution records for this workspace?
                This action cannot be undone.
              </DialogDescription>
            </DialogHeader>

            <DialogFooter className="flex items-center justify-end gap-2 pt-3">
              <Button
                variant="ghost"
                size="sm"
                onClick={() => setClearDialogOpen(false)}
                className="text-xs"
              >
                Cancel
              </Button>
              <Button
                variant="default"
                size="sm"
                onClick={handleClearHistory}
                className="text-xs font-semibold bg-rose-600 hover:bg-rose-500 text-white"
              >
                Clear All History
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}

      {/* Compare Modal */}
      <CompareHistoryDialog
        open={compareModalOpen}
        onClose={() => setCompareModalOpen(false)}
      />
    </aside>
  )
}
