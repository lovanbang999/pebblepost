import { useState, useEffect, useMemo, useCallback } from 'react'
import {
  RefreshCw,
  CheckCircle2,
  AlertTriangle,
  FileCode2,
  ChevronDown,
  ChevronRight,
  Plus,
  Trash2,
  Edit3,
  ShieldAlert,
} from 'lucide-react'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../ui/dialog'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Checkbox } from '../ui/checkbox'
import { cn } from '../../lib/utils'
import type { SyncDiffReport, EndpointDiff } from '../../types'

interface OpenAPISyncDialogProps {
  open: boolean
  onOpenChange: (open: boolean) => void
  folderPath: string
  specLocation: string
  onApplied?: () => void
}

export function OpenAPISyncDialog({
  open,
  onOpenChange,
  folderPath,
  specLocation,
  onApplied,
}: OpenAPISyncDialogProps) {
  const [report, setReport] = useState<SyncDiffReport | null>(null)
  const [loading, setLoading] = useState(false)
  const [applying, setApplying] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const [force, setForce] = useState(false)
  const [activeFilter, setActiveFilter] = useState<'all' | 'added' | 'changed' | 'removed' | 'conflicts'>('all')
  const [expandedIds, setExpandedIds] = useState<Set<string>>(new Set())
  const [applySuccessMessage, setApplySuccessMessage] = useState<string | null>(null)

  const fetchDiff = useCallback(async () => {
    if (!folderPath) return
    setLoading(true)
    setError(null)
    setApplySuccessMessage(null)
    try {
      const params = new URLSearchParams({ folder: folderPath })
      if (specLocation) {
        params.set('spec', specLocation)
      }
      const res = await fetch(`/api/sync/diff?${params.toString()}`)
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `Failed to calculate sync diff (${res.status})`)
      }
      const data: SyncDiffReport = await res.json()
      setReport(data)

      // By default, select all non-conflicting additions and changes
      const defaultSelected = new Set<string>()
      data.endpoints.forEach((ep) => {
        if (ep.diffType === 'added' || ep.diffType === 'changed') {
          defaultSelected.add(ep.id)
        } else if (ep.diffType === 'removed' && !ep.conflict) {
          defaultSelected.add(ep.id)
        }
      })
      setSelectedIds(defaultSelected)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [folderPath, specLocation])

  useEffect(() => {
    if (open) {
      fetchDiff()
    } else {
      setReport(null)
      setError(null)
      setApplySuccessMessage(null)
    }
  }, [open, fetchDiff])

  const toggleSelect = (id: string) => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => {
      const next = new Set(prev)
      if (next.has(id)) {
        next.delete(id)
      } else {
        next.add(id)
      }
      return next
    })
  }

  const filteredEndpoints = useMemo(() => {
    if (!report) return []
    return report.endpoints.filter((ep) => {
      if (activeFilter === 'added') return ep.diffType === 'added'
      if (activeFilter === 'changed') return ep.diffType === 'changed'
      if (activeFilter === 'removed') return ep.diffType === 'removed'
      if (activeFilter === 'conflicts') return !!ep.conflict
      return ep.diffType !== 'unchanged'
    })
  }, [report, activeFilter])

  const selectAllFiltered = () => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      filteredEndpoints.forEach((ep) => next.add(ep.id))
      return next
    })
  }

  const deselectAllFiltered = () => {
    setSelectedIds((prev) => {
      const next = new Set(prev)
      filteredEndpoints.forEach((ep) => next.delete(ep.id))
      return next
    })
  }

  const handleApply = async () => {
    if (!report || selectedIds.size === 0) return
    setApplying(true)
    setError(null)
    try {
      const res = await fetch('/api/sync/apply', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          folderPath,
          specLocation: specLocation || report.specLocation,
          acceptedEndpoints: Array.from(selectedIds),
          force,
        }),
      })

      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || `Failed to apply changes (${res.status})`)
      }

      const result = await res.json()
      setApplySuccessMessage(
        `Applied ${result.appliedCount} change(s) successfully (${result.skippedCount} skipped).`
      )
      if (onApplied) {
        onApplied()
      }
      setTimeout(() => {
        fetchDiff()
      }, 1000)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setApplying(false)
    }
  }

  const getMethodBadgeClass = (method: string) => {
    switch (method.toUpperCase()) {
      case 'GET':
        return 'text-blue-600 dark:text-blue-400 bg-blue-500/10 border-blue-500/20'
      case 'POST':
        return 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border-emerald-500/20'
      case 'PUT':
        return 'text-amber-600 dark:text-amber-400 bg-amber-500/10 border-amber-500/20'
      case 'DELETE':
        return 'text-rose-600 dark:text-rose-400 bg-rose-500/10 border-rose-500/20'
      case 'PATCH':
        return 'text-teal-600 dark:text-teal-400 bg-teal-500/10 border-teal-500/20'
      default:
        return 'text-zinc-600 dark:text-zinc-400 bg-zinc-500/10 border-zinc-500/20'
    }
  }

  const getDiffBadge = (diff: EndpointDiff) => {
    if (diff.diffType === 'added') {
      return (
        <Badge variant="outline" className="text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border-emerald-500/20 text-[11px] gap-1">
          <Plus className="w-3 h-3" /> Added
        </Badge>
      )
    }
    if (diff.diffType === 'changed') {
      return (
        <Badge variant="outline" className="text-amber-600 dark:text-amber-400 bg-amber-500/10 border-amber-500/20 text-[11px] gap-1">
          <Edit3 className="w-3 h-3" /> Changed
        </Badge>
      )
    }
    if (diff.diffType === 'removed') {
      return (
        <Badge variant="outline" className="text-rose-600 dark:text-rose-400 bg-rose-500/10 border-rose-500/20 text-[11px] gap-1">
          <Trash2 className="w-3 h-3" /> Removed
        </Badge>
      )
    }
    return null
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-w-4xl max-h-[85vh] flex flex-col p-0 overflow-hidden bg-white dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-800">
        <DialogHeader className="p-4 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/40">
          <div className="flex items-center justify-between">
            <div className="flex items-center gap-2.5">
              <div className="w-8 h-8 rounded-lg bg-teal-500/10 text-teal-600 dark:text-teal-400 flex items-center justify-center border border-teal-500/20">
                <FileCode2 className="w-4 h-4" />
              </div>
              <div>
                <DialogTitle className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">
                  OpenAPI Spec Drift Review
                </DialogTitle>
                <DialogDescription className="text-xs text-zinc-500 dark:text-zinc-400">
                  {specLocation || 'Review and sync OpenAPI spec differences with local collection'}
                </DialogDescription>
              </div>
            </div>
            <Button
              variant="outline"
              size="sm"
              onClick={fetchDiff}
              disabled={loading}
              className="h-7 text-xs gap-1.5"
            >
              <RefreshCw className={cn('w-3.5 h-3.5', loading && 'animate-spin')} />
              Refresh
            </Button>
          </div>

          {/* Counters & Filter Tabs */}
          {report && (
            <div className="flex flex-wrap items-center justify-between gap-2 pt-3 mt-2 border-t border-zinc-200/60 dark:border-zinc-800/60">
              <div className="flex items-center gap-1.5">
                <button
                  type="button"
                  onClick={() => setActiveFilter('all')}
                  className={cn(
                    'px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer',
                    activeFilter === 'all'
                      ? 'bg-zinc-800 text-white dark:bg-zinc-200 dark:text-zinc-900'
                      : 'text-zinc-600 dark:text-zinc-400 hover:bg-zinc-100 dark:hover:bg-zinc-800'
                  )}
                >
                  All ({report.addedCount + report.changedCount + report.removedCount})
                </button>
                <button
                  type="button"
                  onClick={() => setActiveFilter('added')}
                  className={cn(
                    'px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer',
                    activeFilter === 'added'
                      ? 'bg-emerald-600 text-white'
                      : 'text-emerald-700 dark:text-emerald-400 hover:bg-emerald-500/10'
                  )}
                >
                  Added (+{report.addedCount})
                </button>
                <button
                  type="button"
                  onClick={() => setActiveFilter('changed')}
                  className={cn(
                    'px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer',
                    activeFilter === 'changed'
                      ? 'bg-amber-600 text-white'
                      : 'text-amber-700 dark:text-amber-400 hover:bg-amber-500/10'
                  )}
                >
                  Changed (~{report.changedCount})
                </button>
                <button
                  type="button"
                  onClick={() => setActiveFilter('removed')}
                  className={cn(
                    'px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer',
                    activeFilter === 'removed'
                      ? 'bg-rose-600 text-white'
                      : 'text-rose-700 dark:text-rose-400 hover:bg-rose-500/10'
                  )}
                >
                  Removed (-{report.removedCount})
                </button>
                {report.conflictCount > 0 && (
                  <button
                    type="button"
                    onClick={() => setActiveFilter('conflicts')}
                    className={cn(
                      'px-2.5 py-1 rounded text-xs font-medium transition-colors cursor-pointer flex items-center gap-1',
                      activeFilter === 'conflicts'
                        ? 'bg-red-600 text-white'
                        : 'text-red-600 dark:text-red-400 hover:bg-red-500/10'
                    )}
                  >
                    <AlertTriangle className="w-3 h-3" />
                    Conflicts ({report.conflictCount})
                  </button>
                )}
              </div>

              <div className="flex items-center gap-2">
                <Button variant="ghost" size="sm" onClick={selectAllFiltered} className="h-7 text-xs px-2">
                  Select All
                </Button>
                <Button variant="ghost" size="sm" onClick={deselectAllFiltered} className="h-7 text-xs px-2">
                  Deselect All
                </Button>
              </div>
            </div>
          )}
        </DialogHeader>

        {/* Content Body */}
        <div className="flex-1 overflow-y-auto p-4 space-y-3">
          {error && (
            <div className="p-3 rounded-lg border border-red-500/20 bg-red-500/10 text-red-600 dark:text-red-400 text-xs flex items-center gap-2">
              <AlertTriangle className="w-4 h-4 shrink-0" />
              <span>{error}</span>
            </div>
          )}

          {applySuccessMessage && (
            <div className="p-3 rounded-lg border border-emerald-500/20 bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 text-xs flex items-center gap-2">
              <CheckCircle2 className="w-4 h-4 shrink-0" />
              <span>{applySuccessMessage}</span>
            </div>
          )}

          {loading ? (
            <div className="py-12 flex flex-col items-center justify-center text-zinc-400 text-xs gap-2">
              <RefreshCw className="w-5 h-5 animate-spin text-teal-600" />
              <span>Analyzing OpenAPI spec and comparing folder requests...</span>
            </div>
          ) : report && !report.hasDrift ? (
            <div className="py-12 flex flex-col items-center justify-center text-center">
              <div className="w-10 h-10 rounded-full bg-emerald-500/10 text-emerald-600 dark:text-emerald-400 flex items-center justify-center mb-2">
                <CheckCircle2 className="w-5 h-5" />
              </div>
              <h4 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100">Collection is in sync</h4>
              <p className="text-xs text-zinc-500 dark:text-zinc-400 max-w-sm mt-1">
                All requests in this folder match the endpoints, parameters, and bodies declared in the OpenAPI specification.
              </p>
            </div>
          ) : filteredEndpoints.length === 0 ? (
            <div className="py-8 text-center text-xs text-zinc-400">
              No endpoints in this category.
            </div>
          ) : (
            filteredEndpoints.map((ep) => {
              const isSelected = selectedIds.has(ep.id)
              const isExpanded = expandedIds.has(ep.id)

              return (
                <div
                  key={ep.id}
                  className={cn(
                    'rounded-lg border transition-all text-xs',
                    isSelected
                      ? 'border-zinc-300 dark:border-zinc-700 bg-white dark:bg-zinc-900 shadow-xs'
                      : 'border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-950/50 opacity-80',
                    ep.conflict && 'border-red-300 dark:border-red-900/60'
                  )}
                >
                  <div className="p-3 flex items-center justify-between gap-3">
                    <div className="flex items-center gap-3 min-w-0">
                      <Checkbox
                        checked={isSelected}
                        onCheckedChange={() => toggleSelect(ep.id)}
                        className="cursor-pointer"
                      />
                      <Badge
                        variant="outline"
                        className={cn('font-mono font-bold text-[10px] px-1.5 py-0.5 border', getMethodBadgeClass(ep.method))}
                      >
                        {ep.method}
                      </Badge>
                      <div className="min-w-0 truncate">
                        <div className="flex items-center gap-2">
                          <span className="font-mono font-medium text-zinc-900 dark:text-zinc-100 truncate">
                            {ep.path}
                          </span>
                          {ep.summary && (
                            <span className="text-zinc-500 dark:text-zinc-400 text-[11px] truncate">
                              ({ep.summary})
                            </span>
                          )}
                        </div>
                      </div>
                    </div>

                    <div className="flex items-center gap-2 shrink-0">
                      {ep.conflict && (
                        <Badge variant="outline" className="text-red-600 dark:text-red-400 bg-red-500/10 border-red-500/20 text-[10px] gap-1">
                          <ShieldAlert className="w-3 h-3" /> User Assets Conflict
                        </Badge>
                      )}
                      {getDiffBadge(ep)}
                      <Button
                        variant="ghost"
                        size="icon"
                        onClick={() => toggleExpand(ep.id)}
                        className="h-6 w-6 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
                      >
                        {isExpanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
                      </Button>
                    </div>
                  </div>

                  {/* Expanded Diff Detail View */}
                  {isExpanded && (
                    <div className="px-3 pb-3 pt-1 border-t border-zinc-100 dark:border-zinc-800/80 space-y-2 text-[11px]">
                      {ep.conflict && (
                        <div className="p-2 rounded bg-red-500/10 border border-red-500/20 text-red-700 dark:text-red-400">
                          <p className="font-semibold flex items-center gap-1.5">
                            <AlertTriangle className="w-3.5 h-3.5" /> Preserved Customizations Warning
                          </p>
                          <p className="mt-0.5 text-[11px] opacity-90">{ep.conflict.details}</p>
                          <p className="mt-1 text-[10px] text-zinc-500 dark:text-zinc-400">
                            PebblePost will not silently overwrite local edits. If this endpoint was removed from the spec, it will only be deleted if &quot;Force apply conflicts&quot; is enabled.
                          </p>
                        </div>
                      )}

                      {ep.fieldDiffs && ep.fieldDiffs.length > 0 ? (
                        <div className="space-y-1.5">
                          <span className="font-semibold text-zinc-700 dark:text-zinc-300">Detailed Changes:</span>
                          <div className="rounded border border-zinc-200 dark:border-zinc-800 divide-y divide-zinc-100 dark:divide-zinc-800 bg-zinc-50 dark:bg-zinc-950">
                            {ep.fieldDiffs.map((fd, idx) => (
                              <div key={idx} className="p-2 flex items-start justify-between gap-4">
                                <span className="font-mono text-zinc-600 dark:text-zinc-400 shrink-0 font-medium">
                                  {fd.field}:
                                </span>
                                <span className="text-zinc-800 dark:text-zinc-200 text-right">
                                  {fd.description}
                                </span>
                              </div>
                            ))}
                          </div>
                        </div>
                      ) : ep.diffType === 'added' ? (
                        <p className="text-zinc-500 dark:text-zinc-400 italic">
                          New endpoint defined in spec. Accepting will create a new <code className="font-mono text-[10px]">.pebble.json</code> file with skeleton body and parameters.
                        </p>
                      ) : (
                        <p className="text-zinc-500 dark:text-zinc-400 italic">
                          No parameter or schema changes detected.
                        </p>
                      )}
                    </div>
                  )}
                </div>
              )
            })
          )}
        </div>

        {/* Footer Actions */}
        <DialogFooter className="p-4 border-t border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/40 flex items-center justify-between gap-4">
          <div className="flex items-center gap-2">
            {report && report.conflictCount > 0 && (
              <label className="flex items-center gap-2 text-xs text-red-600 dark:text-red-400 cursor-pointer">
                <Checkbox
                  checked={force}
                  onCheckedChange={(c) => setForce(!!c)}
                />
                <span>Force apply conflicting removals</span>
              </label>
            )}
          </div>

          <div className="flex items-center gap-2">
            <Button variant="outline" size="sm" onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              size="sm"
              disabled={applying || loading || !report || !report.hasDrift || selectedIds.size === 0}
              onClick={handleApply}
              className="bg-teal-600 hover:bg-teal-700 text-white gap-1.5 shadow-xs cursor-pointer"
            >
              {applying ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <CheckCircle2 className="w-3.5 h-3.5" />}
              Apply Selected ({selectedIds.size})
            </Button>
          </div>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
