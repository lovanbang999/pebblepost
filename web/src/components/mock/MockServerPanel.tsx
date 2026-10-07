import { useState, useEffect, useMemo, useRef, useCallback } from 'react'
import {
  Server,
  Play,
  Square,
  Copy,
  Check,
  Search,
  Trash2,
  AlertTriangle,
  RotateCcw,
  Radio,
  SlidersHorizontal,
  FileText,
} from 'lucide-react'
import { type RequestTab } from '../../store/tabStore'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Tooltip } from '../ui/tooltip'
import { cn, getMethodTextColor } from '../../lib/utils'
import type {
  MockServerStatus,
  MockRequestLog,
  MockRouteOverride,
} from '../../types'

interface MockServerPanelProps {
  currentTab: RequestTab
}

export function MockServerPanel({ currentTab }: MockServerPanelProps) {
  const { workspacePath } = useWorkspaceStore()
  const targetPath = currentTab.mockConfig?.folderPath || workspacePath || '.'

  // Server state
  const [status, setStatus] = useState<MockServerStatus | null>(null)
  const [isLoading, setIsLoading] = useState(false)
  const [errorMsg, setErrorMsg] = useState('')

  // Config inputs
  const [port, setPort] = useState<number>(8080)
  const [host, setHost] = useState<string>('127.0.0.1')
  const [globalDelayMs, setGlobalDelayMs] = useState<number>(0)
  const [globalStatus, setGlobalStatus] = useState<number>(0)
  const [globalErrorRate, setGlobalErrorRate] = useState<number>(0)

  // Local overrides state (per-route: routeId -> MockRouteOverride)
  const [overrides, setOverrides] = useState<Record<string, MockRouteOverride>>({})

  // Logs state
  const [logs, setLogs] = useState<MockRequestLog[]>([])
  const [activeSection, setActiveSection] = useState<'routes' | 'logs'>('routes')
  const [searchQuery, setSearchQuery] = useState('')
  const [logFilter, setLogFilter] = useState<'all' | 'matched' | 'unmatched'>('all')
  const [copiedUrl, setCopiedUrl] = useState(false)
  const eventSourceRef = useRef<EventSource | null>(null)

  // Fetch current server status
  const fetchStatus = useCallback(async () => {
    try {
      const res = await fetch('/api/mock/status')
      if (res.ok) {
        const data: MockServerStatus = await res.json()
        setStatus(data)
        if (data.port && data.running) {
          setPort(data.port)
          setHost(data.host || '127.0.0.1')
        }
        if (data.overrides) {
          setOverrides(data.overrides)
        }
      }
    } catch {
      // offline / not reachable
    }
  }, [])

  // Poll status on mount
  useEffect(() => {
    fetchStatus()
  }, [fetchStatus])

  // Setup SSE stream for live logs
  useEffect(() => {
    if (!status?.running) {
      if (eventSourceRef.current) {
        eventSourceRef.current.close()
        eventSourceRef.current = null
      }
      return
    }

    const sse = new EventSource('/api/mock/logs/stream')
    eventSourceRef.current = sse

    sse.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data)
        if (data && data.id && data.method) {
          setLogs((prev) => [data, ...prev.slice(0, 499)])
        }
      } catch {
        // non-log SSE event
      }
    }

    sse.onerror = () => {
      // Stream reconnection handled automatically by browser EventSource
    }

    return () => {
      sse.close()
      eventSourceRef.current = null
    }
  }, [status?.running])

  // Fetch initial logs when server starts or logs section opened
  const fetchLogs = useCallback(async () => {
    try {
      const res = await fetch('/api/mock/logs')
      if (res.ok) {
        const data: MockRequestLog[] = await res.json()
        setLogs(data.reverse())
      }
    } catch {
      // ignore
    }
  }, [])

  useEffect(() => {
    if (status?.running) {
      fetchLogs()
    }
  }, [status?.running, fetchLogs])

  // Start mock server
  const handleStart = async () => {
    setIsLoading(true)
    setErrorMsg('')
    try {
      const res = await fetch('/api/mock/start', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          path: targetPath,
          workspacePath,
          port: Number(port) || 8080,
          host: host.trim() || '127.0.0.1',
          globalDelayMs: Number(globalDelayMs) || 0,
          globalStatusCode: Number(globalStatus) || 0,
          globalErrorRate: Number(globalErrorRate) || 0,
          overrides,
        }),
      })

      const data = await res.json()
      if (!res.ok) {
        throw new Error(data.error || 'Failed to start mock server')
      }
      setStatus(data)
      setLogs([])
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err))
    } finally {
      setIsLoading(false)
    }
  }

  // Stop mock server
  const handleStop = async () => {
    setIsLoading(true)
    setErrorMsg('')
    try {
      const res = await fetch('/api/mock/stop', { method: 'POST' })
      const data = await res.json()
      setStatus(data)
    } catch (err) {
      setErrorMsg(err instanceof Error ? err.message : String(err))
    } finally {
      setIsLoading(false)
    }
  }

  // Update a single route override
  const handleUpdateOverride = async (routeId: string, updated: MockRouteOverride | undefined) => {
    const nextOverrides = { ...overrides }
    if (!updated || (!updated.statusCode && !updated.delayMs && !updated.errorRate)) {
      delete nextOverrides[routeId]
    } else {
      nextOverrides[routeId] = updated
    }
    setOverrides(nextOverrides)

    if (status?.running) {
      try {
        await fetch('/api/mock/overrides', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            routeId,
            override: updated || null,
          }),
        })
      } catch {
        // ignore
      }
    }
  }

  // Filter routes
  const filteredRoutes = useMemo(() => {
    if (!status?.routes) return []
    if (!searchQuery.trim()) return status.routes

    const q = searchQuery.toLowerCase()
    return status.routes.filter(
      (r) =>
        r.pathPattern.toLowerCase().includes(q) ||
        r.method.toLowerCase().includes(q) ||
        r.requestName.toLowerCase().includes(q) ||
        r.exampleNames.some((ex) => ex.toLowerCase().includes(q))
    )
  }, [status?.routes, searchQuery])

  // Filter logs
  const filteredLogs = useMemo(() => {
    return logs.filter((log) => {
      if (logFilter === 'matched' && !log.matched) return false
      if (logFilter === 'unmatched' && log.matched) return false
      if (!searchQuery.trim()) return true
      const q = searchQuery.toLowerCase()
      return (
        log.path.toLowerCase().includes(q) ||
        log.method.toLowerCase().includes(q) ||
        (log.matchedExample && log.matchedExample.toLowerCase().includes(q))
      )
    })
  }, [logs, logFilter, searchQuery])

  const copyUrlToClipboard = () => {
    if (!status?.url) return
    navigator.clipboard.writeText(status.url)
    setCopiedUrl(true)
    setTimeout(() => setCopiedUrl(false), 2000)
  }

  const isNonLoopback = host !== '127.0.0.1' && host !== 'localhost' && host !== '::1' && host !== ''

  return (
    <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden text-zinc-900 dark:text-zinc-100">
      {/* Top Banner & Control Bar */}
      <div className="p-4 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/70 dark:bg-zinc-900/40 shrink-0 space-y-3">
        <div className="flex items-center justify-between">
          <div className="flex items-center gap-2.5">
            <div className="w-8 h-8 rounded-lg bg-teal-500/10 text-teal-600 dark:text-teal-400 flex items-center justify-center border border-teal-500/20">
              <Server className="w-4 h-4" />
            </div>
            <div>
              <div className="flex items-center gap-2">
                <h2 className="text-sm font-semibold">Mock Server</h2>
                {status?.running ? (
                  <Badge variant="outline" className="border-emerald-500/30 text-emerald-600 dark:text-emerald-400 gap-1.5 py-0 px-2 text-[11px] font-medium bg-emerald-50/50 dark:bg-emerald-950/20">
                    <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 animate-pulse" />
                    Running
                  </Badge>
                ) : (
                  <Badge variant="outline" className="border-zinc-300 dark:border-zinc-700 text-zinc-500 gap-1.5 py-0 px-2 text-[11px] font-medium">
                    <span className="w-1.5 h-1.5 rounded-full bg-zinc-400" />
                    Stopped
                  </Badge>
                )}
              </div>
              <p className="text-xs text-zinc-500 truncate max-w-xl">
                Serving saved Examples from <span className="font-mono text-zinc-700 dark:text-zinc-300">{targetPath}</span>
              </p>
            </div>
          </div>

          {/* Action buttons */}
          <div className="flex items-center gap-2">
            {status?.running ? (
              <>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={copyUrlToClipboard}
                  className="h-8 gap-1.5 font-mono text-xs border-emerald-500/30 text-emerald-700 dark:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-950/30"
                >
                  {copiedUrl ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5" />}
                  <span>{status.url}</span>
                </Button>
                <Button
                  variant="destructive"
                  size="sm"
                  disabled={isLoading}
                  onClick={handleStop}
                  className="h-8 gap-1.5 shadow-2xs font-medium"
                >
                  <Square className="w-3.5 h-3.5 fill-current" />
                  <span>Stop Server</span>
                </Button>
              </>
            ) : (
              <Button
                variant="default"
                size="sm"
                disabled={isLoading}
                onClick={handleStart}
                className="h-8 gap-1.5 bg-emerald-600 hover:bg-emerald-700 text-white font-medium shadow-2xs"
              >
                <Play className="w-3.5 h-3.5 fill-current" />
                <span>Start Mock Server</span>
              </Button>
            )}
          </div>
        </div>

        {/* Server Config Row */}
        <div className="flex flex-wrap items-center gap-3 pt-1 text-xs">
          <div className="flex items-center gap-1.5">
            <label htmlFor="mock-port-input" className="text-zinc-500 text-[11px] font-medium">Port:</label>
            <Input
              id="mock-port-input"
              type="number"
              disabled={status?.running}
              value={port}
              onChange={(e) => setPort(Number(e.target.value))}
              className="h-7 w-20 text-xs font-mono"
            />
          </div>

          <div className="flex items-center gap-1.5">
            <label htmlFor="mock-host-input" className="text-zinc-500 text-[11px] font-medium">Host:</label>
            <Input
              id="mock-host-input"
              disabled={status?.running}
              value={host}
              onChange={(e) => setHost(e.target.value)}
              className="h-7 w-28 text-xs font-mono"
            />
          </div>

          <div className="flex items-center gap-1.5">
            <label htmlFor="mock-delay-input" className="text-zinc-500 text-[11px] font-medium">Global Delay:</label>
            <Input
              id="mock-delay-input"
              type="number"
              disabled={status?.running}
              placeholder="0ms"
              value={globalDelayMs || ''}
              onChange={(e) => setGlobalDelayMs(Number(e.target.value))}
              className="h-7 w-20 text-xs font-mono"
            />
          </div>

          <div className="flex items-center gap-1.5">
            <label htmlFor="mock-status-override-input" className="text-zinc-500 text-[11px] font-medium">Global Status:</label>
            <Input
              id="mock-status-override-input"
              type="number"
              disabled={status?.running}
              placeholder="Default"
              value={globalStatus || ''}
              onChange={(e) => setGlobalStatus(Number(e.target.value))}
              className="h-7 w-20 text-xs font-mono"
            />
          </div>

          <div className="flex items-center gap-1.5">
            <label htmlFor="mock-error-rate-input" className="text-zinc-500 text-[11px] font-medium">Error Rate:</label>
            <Input
              id="mock-error-rate-input"
              type="number"
              step="0.05"
              min="0"
              max="1"
              disabled={status?.running}
              placeholder="0.0"
              value={globalErrorRate || ''}
              onChange={(e) => setGlobalErrorRate(Number(e.target.value))}
              className="h-7 w-20 text-xs font-mono"
            />
          </div>

          {isNonLoopback && (
            <div className="flex items-center gap-1 text-amber-600 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/40 px-2 py-0.5 rounded border border-amber-500/20 text-[11px]">
              <AlertTriangle className="w-3 h-3 shrink-0" />
              <span>External access enabled</span>
            </div>
          )}
        </div>

        {errorMsg && (
          <div className="flex items-center gap-2 p-2 bg-rose-50 dark:bg-rose-950/30 border border-rose-200 dark:border-rose-900 rounded text-rose-600 dark:text-rose-400 text-xs">
            <AlertTriangle className="w-4 h-4 shrink-0" />
            <span>{errorMsg}</span>
          </div>
        )}
      </div>

      {/* Navigation Subtabs & Search */}
      <div className="px-4 py-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-white dark:bg-zinc-950 shrink-0">
        <div className="flex items-center gap-1 bg-zinc-100 dark:bg-zinc-900 p-0.5 rounded-lg border border-zinc-200 dark:border-zinc-800 text-xs">
          <button
            type="button"
            onClick={() => setActiveSection('routes')}
            className={cn(
              'px-3 py-1 rounded-md font-medium transition-colors flex items-center gap-1.5',
              activeSection === 'routes'
                ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 shadow-2xs'
                : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-200'
            )}
          >
            <SlidersHorizontal className="w-3.5 h-3.5" />
            <span>Routes & Overrides</span>
            {status?.routes && (
              <Badge variant="secondary" className="px-1 py-0 text-[10px] ml-1">
                {status.routes.length}
              </Badge>
            )}
          </button>
          <button
            type="button"
            onClick={() => setActiveSection('logs')}
            className={cn(
              'px-3 py-1 rounded-md font-medium transition-colors flex items-center gap-1.5',
              activeSection === 'logs'
                ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 shadow-2xs'
                : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-200'
            )}
          >
            <Radio className="w-3.5 h-3.5 text-emerald-500" />
            <span>Live Request Log</span>
            {logs.length > 0 && (
              <Badge variant="secondary" className="px-1 py-0 text-[10px] ml-1">
                {logs.length}
              </Badge>
            )}
          </button>
        </div>

        {/* Search input & Section controls */}
        <div className="flex items-center gap-2">
          <div className="relative">
            <Search className="w-3.5 h-3.5 absolute left-2.5 top-2 text-zinc-400" />
            <Input
              type="text"
              placeholder={activeSection === 'routes' ? 'Filter routes...' : 'Filter request logs...'}
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              className="h-7 w-48 pl-8 text-xs"
            />
          </div>

          {activeSection === 'logs' && (
            <>
              <div className="flex items-center border border-zinc-200 dark:border-zinc-800 rounded text-xs overflow-hidden">
                <button
                  type="button"
                  onClick={() => setLogFilter('all')}
                  className={cn('px-2 py-1', logFilter === 'all' && 'bg-zinc-200 dark:bg-zinc-800 font-semibold')}
                >
                  All
                </button>
                <button
                  type="button"
                  onClick={() => setLogFilter('matched')}
                  className={cn('px-2 py-1', logFilter === 'matched' && 'bg-zinc-200 dark:bg-zinc-800 font-semibold')}
                >
                  Matched
                </button>
                <button
                  type="button"
                  onClick={() => setLogFilter('unmatched')}
                  className={cn('px-2 py-1', logFilter === 'unmatched' && 'bg-zinc-200 dark:bg-zinc-800 font-semibold')}
                >
                  404s
                </button>
              </div>
              <Tooltip content="Clear logs">
                <Button
                  variant="ghost"
                  size="icon"
                  onClick={() => setLogs([])}
                  className="h-7 w-7 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
                >
                  <Trash2 className="w-3.5 h-3.5" />
                </Button>
              </Tooltip>
            </>
          )}
        </div>
      </div>

      {/* Main Content Area */}
      <div className="flex-1 overflow-auto p-4">
        {activeSection === 'routes' ? (
          <div>
            {!status?.routes || status.routes.length === 0 ? (
              <div className="text-center py-16 space-y-3">
                <div className="w-12 h-12 rounded-full bg-zinc-100 dark:bg-zinc-900 flex items-center justify-center mx-auto text-zinc-400">
                  <FileText className="w-6 h-6" />
                </div>
                <h3 className="text-sm font-semibold">No Mock Routes Available</h3>
                <p className="text-xs text-zinc-500 max-w-sm mx-auto">
                  No requests with saved Examples were found in this collection or folder. Open an API request, run it, and click &quot;Save as Example&quot; to enable mock responses.
                </p>
                {!status?.running && (
                  <Button variant="outline" size="sm" onClick={handleStart} className="gap-1.5 text-xs">
                    <Play className="w-3 h-3" />
                    <span>Scan and Start Server</span>
                  </Button>
                )}
              </div>
            ) : (
              <div className="border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden shadow-2xs">
                <table className="w-full text-left text-xs border-collapse">
                  <thead>
                    <tr className="border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/60 text-zinc-500 text-[11px]">
                      <th className="py-2.5 px-3 font-semibold w-24">Method</th>
                      <th className="py-2.5 px-3 font-semibold">Path Pattern</th>
                      <th className="py-2.5 px-3 font-semibold">Request</th>
                      <th className="py-2.5 px-3 font-semibold w-24">Examples</th>
                      <th className="py-2.5 px-3 font-semibold w-28">Delay (ms)</th>
                      <th className="py-2.5 px-3 font-semibold w-28">Status</th>
                      <th className="py-2.5 px-3 font-semibold w-32">Error Rate</th>
                      <th className="py-2.5 px-2 font-semibold w-12 text-center">Reset</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800">
                    {filteredRoutes.map((route) => {
                      const ov = overrides[route.id] || route.override || {}
                      return (
                        <tr
                          key={route.id}
                          className="hover:bg-zinc-50/70 dark:hover:bg-zinc-900/40 transition-colors"
                        >
                          <td className="py-2.5 px-3">
                            <span
                              className={cn(
                                'text-[10px] font-mono font-bold tracking-tight uppercase px-1.5 py-0.5 rounded border border-zinc-200 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800',
                                getMethodTextColor(route.method)
                              )}
                            >
                              {route.method}
                            </span>
                          </td>
                          <td className="py-2.5 px-3 font-mono font-medium text-zinc-900 dark:text-zinc-100">
                            {route.pathPattern}
                          </td>
                          <td className="py-2.5 px-3 text-zinc-600 dark:text-zinc-400 truncate max-w-xs">
                            {route.requestName}
                          </td>
                          <td className="py-2.5 px-3">
                            <Tooltip content={`Examples: ${route.exampleNames.join(', ')}`}>
                              <Badge variant="secondary" className="cursor-help text-[10px]">
                                {route.exampleCount} saved
                              </Badge>
                            </Tooltip>
                          </td>
                          <td className="py-2.5 px-3">
                            <Input
                              type="number"
                              min="0"
                              step="50"
                              placeholder="0"
                              value={ov.delayMs || ''}
                              onChange={(e) =>
                                handleUpdateOverride(route.id, {
                                  ...ov,
                                  delayMs: Number(e.target.value) || 0,
                                })
                              }
                              className="h-6 w-20 text-xs font-mono"
                            />
                          </td>
                          <td className="py-2.5 px-3">
                            <Input
                              type="number"
                              min="100"
                              max="599"
                              placeholder="Def"
                              value={ov.statusCode || ''}
                              onChange={(e) =>
                                handleUpdateOverride(route.id, {
                                  ...ov,
                                  statusCode: Number(e.target.value) || 0,
                                })
                              }
                              className="h-6 w-20 text-xs font-mono"
                            />
                          </td>
                          <td className="py-2.5 px-3">
                            <div className="flex items-center gap-1.5">
                              <input
                                type="range"
                                min="0"
                                max="1"
                                step="0.05"
                                value={ov.errorRate || 0}
                                onChange={(e) =>
                                  handleUpdateOverride(route.id, {
                                    ...ov,
                                    errorRate: parseFloat(e.target.value),
                                  })
                                }
                                className="w-16 h-1 accent-emerald-500 cursor-pointer"
                              />
                              <span className="text-[10px] font-mono text-zinc-500 w-8">
                                {Math.round((ov.errorRate || 0) * 100)}%
                              </span>
                            </div>
                          </td>
                          <td className="py-2.5 px-2 text-center">
                            {(ov.statusCode || ov.delayMs || ov.errorRate) && (
                              <Tooltip content="Reset overrides for this route">
                                <Button
                                  variant="ghost"
                                  size="icon"
                                  onClick={() => handleUpdateOverride(route.id, undefined)}
                                  className="h-6 w-6 text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200"
                                >
                                  <RotateCcw className="w-3 h-3" />
                                </Button>
                              </Tooltip>
                            )}
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        ) : (
          <div>
            {filteredLogs.length === 0 ? (
              <div className="text-center py-16 space-y-2">
                <Radio className="w-8 h-8 text-zinc-400 animate-pulse mx-auto" />
                <h3 className="text-sm font-semibold">Waiting for requests...</h3>
                <p className="text-xs text-zinc-500">
                  Send HTTP requests to <code className="font-mono text-teal-600 dark:text-teal-400">{status?.url || `http://${host}:${port}`}</code> to inspect incoming traffic in real-time.
                </p>
              </div>
            ) : (
              <div className="border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden shadow-2xs">
                <table className="w-full text-left text-xs border-collapse">
                  <thead>
                    <tr className="border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/60 text-zinc-500 text-[11px]">
                      <th className="py-2.5 px-3 font-semibold w-20">Time</th>
                      <th className="py-2.5 px-3 font-semibold w-20">Method</th>
                      <th className="py-2.5 px-3 font-semibold">Path & Query</th>
                      <th className="py-2.5 px-3 font-semibold w-24">Status</th>
                      <th className="py-2.5 px-3 font-semibold">Matched Example</th>
                      <th className="py-2.5 px-3 font-semibold w-20 text-right">Duration</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800 font-mono">
                    {filteredLogs.map((log) => {
                      const timeStr = new Date(log.timestamp).toLocaleTimeString()
                      return (
                        <tr
                          key={log.id}
                          className="hover:bg-zinc-50/70 dark:hover:bg-zinc-900/40 transition-colors"
                        >
                          <td className="py-2 px-3 text-zinc-400 text-[11px]">{timeStr}</td>
                          <td className="py-2 px-3">
                            <span
                              className={cn(
                                'text-[10px] font-bold tracking-tight uppercase px-1.5 py-0.5 rounded border border-zinc-200 dark:border-zinc-700 bg-zinc-100 dark:bg-zinc-800',
                                getMethodTextColor(log.method)
                              )}
                            >
                              {log.method}
                            </span>
                          </td>
                          <td className="py-2 px-3 text-zinc-800 dark:text-zinc-200 truncate max-w-md">
                            <span>{log.path}</span>
                            {log.query && <span className="text-zinc-400">?{log.query}</span>}
                          </td>
                          <td className="py-2 px-3">
                            <Badge
                              variant="outline"
                              className={cn(
                                'px-1.5 py-0 text-[10px] font-bold',
                                log.statusCode >= 200 && log.statusCode < 300
                                  ? 'border-emerald-500/30 text-emerald-600 dark:text-emerald-400 bg-emerald-50/30 dark:bg-emerald-950/20'
                                  : log.statusCode === 404
                                  ? 'border-amber-500/30 text-amber-600 dark:text-amber-400 bg-amber-50/30 dark:bg-amber-950/20'
                                  : 'border-rose-500/30 text-rose-600 dark:text-rose-400 bg-rose-50/30 dark:bg-rose-950/20'
                              )}
                            >
                              {log.statusCode}
                            </Badge>
                          </td>
                          <td className="py-2 px-3 text-xs font-sans">
                            {log.matched ? (
                              <span className="text-zinc-700 dark:text-zinc-300 font-medium">
                                {log.matchedExample}
                              </span>
                            ) : (
                              <span className="text-amber-600 dark:text-amber-400 flex items-center gap-1 text-[11px]">
                                <AlertTriangle className="w-3 h-3 shrink-0" />
                                Unmatched (404)
                              </span>
                            )}
                          </td>
                          <td className="py-2 px-3 text-right text-zinc-500 text-[11px]">
                            {log.durationMs}ms
                          </td>
                        </tr>
                      )
                    })}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}
