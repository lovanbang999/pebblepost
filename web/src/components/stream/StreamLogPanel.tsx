import { useState, useMemo, useRef, useEffect } from 'react'
import {
  ArrowUpRight,
  ArrowDownLeft,
  Activity,
  AlertTriangle,
  Info,
  Search,
  Trash2,
  Download,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
  Radio,
  Clock,
} from 'lucide-react'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Tabs, TabsList, TabsTrigger } from '../ui/tabs'
import { Tooltip } from '../ui/tooltip'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { formatBytes } from '../../lib/utils'
import type { StreamLogEntry, StreamSessionStatus } from '../../types'

interface StreamLogPanelProps {
  logs: StreamLogEntry[]
  status?: StreamSessionStatus | null
  isLive?: boolean
  closeCode?: number
  closeReason?: string
  evictedCount?: number
  onClearLogs?: () => void
}

const RFC_CLOSE_CODES: Record<number, string> = {
  1000: 'Normal Closure',
  1001: 'Going Away',
  1002: 'Protocol Error',
  1003: 'Unsupported Data',
  1005: 'No Status Received',
  1006: 'Abnormal Closure',
  1007: 'Invalid Frame Payload',
  1008: 'Policy Violation',
  1009: 'Message Too Big',
  1010: 'Mandatory Extension',
  1011: 'Internal Server Error',
  1015: 'TLS Handshake Failed',
}

export function StreamLogPanel({
  logs = [],
  status,
  isLive = false,
  closeCode,
  closeReason,
  evictedCount = 0,
  onClearLogs,
}: StreamLogPanelProps) {
  const { theme } = useWorkspaceStore()
  const [filterDirection, setFilterDirection] = useState<'all' | 'send' | 'receive' | 'system' | 'pingpong'>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const [expandedIds, setExpandedIds] = useState<Record<string, boolean>>({})
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [autoScroll, setAutoScroll] = useState(true)
  const listEndRef = useRef<HTMLDivElement>(null)

  // Auto-scroll to bottom when new logs arrive
  useEffect(() => {
    if (autoScroll && listEndRef.current) {
      listEndRef.current.scrollIntoView({ behavior: 'smooth' })
    }
  }, [logs.length, autoScroll])

  // Counts
  const counts = useMemo(() => {
    let send = 0
    let receive = 0
    let system = 0
    let pingpong = 0
    for (const l of logs) {
      if (l.direction === 'send') send++
      else if (l.direction === 'receive') receive++
      else if (l.direction === 'system') system++
      if (l.type === 'ping' || l.type === 'pong') pingpong++
    }
    return { all: logs.length, send, receive, system, pingpong }
  }, [logs])

  // Filtered logs
  const filteredLogs = useMemo(() => {
    return logs.filter((log) => {
      // Direction filter
      if (filterDirection === 'send' && log.direction !== 'send') return false
      if (filterDirection === 'receive' && log.direction !== 'receive') return false
      if (filterDirection === 'system' && log.direction !== 'system') return false
      if (filterDirection === 'pingpong' && log.type !== 'ping' && log.type !== 'pong') return false

      // Search query
      if (searchQuery.trim()) {
        const query = searchQuery.toLowerCase()
        const payloadMatch = log.payload?.toLowerCase().includes(query)
        const typeMatch = log.type?.toLowerCase().includes(query)
        const reasonMatch = log.closeReason?.toLowerCase().includes(query)
        if (!payloadMatch && !typeMatch && !reasonMatch) return false
      }

      return true
    })
  }, [logs, filterDirection, searchQuery])

  const toggleExpand = (id: string) => {
    setExpandedIds((prev) => ({ ...prev, [id]: !prev[id] }))
  }

  const handleCopyPayload = (id: string, payload: string) => {
    navigator.clipboard.writeText(payload)
    setCopiedId(id)
    setTimeout(() => setCopiedId(null), 1500)
  }

  const handleExportJSON = () => {
    const dataStr = 'data:text/json;charset=utf-8,' + encodeURIComponent(JSON.stringify(logs, null, 2))
    const downloadAnchor = document.createElement('a')
    downloadAnchor.setAttribute('href', dataStr)
    downloadAnchor.setAttribute('download', `stream-logs-${Date.now()}.json`)
    document.body.appendChild(downloadAnchor)
    downloadAnchor.click()
    downloadAnchor.remove()
  }

  const effectiveCloseCode = closeCode ?? status?.closeCode
  const effectiveCloseReason = closeReason || status?.closeReason
  const effectiveEvicted = (evictedCount || 0) + (status?.evictedCount || 0)

  return (
    <div className="flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden font-sans">
      {/* ── Top Control Bar ── */}
      <div className="p-2.5 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between gap-2 flex-wrap bg-zinc-50/50 dark:bg-zinc-900/40">
        <div className="flex items-center gap-2 flex-wrap">
          {/* Live Indicator */}
          {isLive ? (
            <div className="flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-semibold bg-emerald-100 dark:bg-emerald-950/60 text-emerald-700 dark:text-emerald-300 border border-emerald-300 dark:border-emerald-800/80">
              <span className="flex h-2 w-2 relative">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              <span>LIVE</span>
            </div>
          ) : (
            <div className="flex items-center gap-1.5 px-2 py-0.5 rounded-full text-xs font-semibold bg-zinc-100 dark:bg-zinc-800 text-zinc-600 dark:text-zinc-400">
              <span className="h-1.5 w-1.5 rounded-full bg-zinc-400"></span>
              <span>OFFLINE</span>
            </div>
          )}

          {/* Close Code Badge */}
          {effectiveCloseCode !== undefined && effectiveCloseCode > 0 && (
            <Tooltip content={effectiveCloseReason ? `Reason: ${effectiveCloseReason}` : 'Stream connection closed'}>
              <Badge
                variant={effectiveCloseCode === 1000 ? 'outline' : 'destructive'}
                className="text-xs font-mono py-0.5 px-2 gap-1.5 cursor-help"
              >
                <span>Close {effectiveCloseCode}</span>
                <span className="opacity-80">
                  ({RFC_CLOSE_CODES[effectiveCloseCode] || 'Custom Code'})
                </span>
              </Badge>
            </Tooltip>
          )}

          {/* Eviction Warning Badge */}
          {effectiveEvicted > 0 && (
            <Tooltip content="Oldest messages evicted by memory limit (ring buffer)">
              <div className="flex items-center gap-1 text-[11px] font-mono text-amber-700 dark:text-amber-400 bg-amber-50 dark:bg-amber-950/50 border border-amber-200 dark:border-amber-800 px-2 py-0.5 rounded">
                <AlertTriangle className="w-3 h-3 text-amber-500" />
                <span>{effectiveEvicted} evicted</span>
              </div>
            </Tooltip>
          )}
        </div>

        {/* Action Controls */}
        <div className="flex items-center gap-1.5">
          <Tooltip content={autoScroll ? 'Pause auto-scroll' : 'Resume auto-scroll'}>
            <Button
              variant={autoScroll ? 'default' : 'outline'}
              size="sm"
              onClick={() => setAutoScroll((p) => !p)}
              className="h-7 text-xs px-2"
            >
              <Clock className="w-3 h-3 mr-1" />
              <span>{autoScroll ? 'Autoscroll On' : 'Paused'}</span>
            </Button>
          </Tooltip>

          {onClearLogs && (
            <Tooltip content="Clear log entries">
              <Button
                variant="outline"
                size="sm"
                onClick={onClearLogs}
                className="h-7 text-xs px-2 text-zinc-600 hover:text-rose-600"
              >
                <Trash2 className="w-3 h-3 mr-1" />
                <span>Clear</span>
              </Button>
            </Tooltip>
          )}

          <Tooltip content="Export logs as JSON">
            <Button
              variant="outline"
              size="sm"
              onClick={handleExportJSON}
              disabled={logs.length === 0}
              className="h-7 text-xs px-2"
            >
              <Download className="w-3 h-3 mr-1" />
              <span>Export</span>
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* ── Filter Bar ── */}
      <div className="p-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between gap-2 flex-wrap">
        <Tabs
          value={filterDirection}
          onValueChange={(v) =>
            setFilterDirection(v as 'all' | 'send' | 'receive' | 'system' | 'pingpong')
          }
          className="h-7"
        >
          <TabsList className="h-7 p-0.5">
            <TabsTrigger value="all" className="text-xs h-6 px-2.5">
              All ({counts.all})
            </TabsTrigger>
            <TabsTrigger value="receive" className="text-xs h-6 px-2.5 text-emerald-600 dark:text-emerald-400">
              Received ({counts.receive})
            </TabsTrigger>
            <TabsTrigger value="send" className="text-xs h-6 px-2.5 text-cyan-600 dark:text-cyan-400">
              Sent ({counts.send})
            </TabsTrigger>
            <TabsTrigger value="system" className="text-xs h-6 px-2.5 text-purple-600 dark:text-purple-400">
              System ({counts.system})
            </TabsTrigger>
            {counts.pingpong > 0 && (
              <TabsTrigger value="pingpong" className="text-xs h-6 px-2.5 text-amber-600 dark:text-amber-400">
                Ping/Pong ({counts.pingpong})
              </TabsTrigger>
            )}
          </TabsList>
        </Tabs>

        {/* Search */}
        <div className="relative w-48">
          <Search className="w-3 h-3 absolute left-2 top-2.5 text-zinc-400" />
          <Input
            placeholder="Search payload..."
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            className="h-7 text-xs pl-7 pr-2 font-mono"
          />
        </div>
      </div>

      {/* ── Log Messages List ── */}
      <div className="flex-1 overflow-y-auto p-2 space-y-1.5 font-mono text-xs">
        {filteredLogs.length === 0 ? (
          <div className="flex flex-col items-center justify-center h-48 text-zinc-400 text-xs">
            <Radio className="w-8 h-8 text-zinc-300 dark:text-zinc-700 stroke-[1.5] mb-2" />
            <p className="font-semibold text-zinc-600 dark:text-zinc-400">No stream messages to display</p>
            <p className="text-[11px] text-zinc-400 mt-0.5">
              {logs.length > 0 ? 'No messages match current filter/search query.' : 'Connect to a stream to receive real-time events.'}
            </p>
          </div>
        ) : (
          filteredLogs.map((log) => {
            const isSend = log.direction === 'send'
            const isRecv = log.direction === 'receive'
            const isSys = log.direction === 'system'
            const isPingPong = log.type === 'ping' || log.type === 'pong'
            const isExpanded = !!expandedIds[log.id]
            const isError = log.isError || log.type === 'error'

            let prettyPayload = log.payload
            let isJSON = false
            try {
              if (log.payload && (log.payload.startsWith('{') || log.payload.startsWith('['))) {
                const parsed = JSON.parse(log.payload)
                prettyPayload = JSON.stringify(parsed, null, 2)
                isJSON = true
              }
            } catch {
              // not json
            }

            return (
              <div
                key={log.id}
                className={`rounded border transition-colors ${
                  isError
                    ? 'border-rose-300 dark:border-rose-900 bg-rose-50/50 dark:bg-rose-950/20'
                    : isSend
                    ? 'border-cyan-200 dark:border-cyan-950 bg-cyan-50/30 dark:bg-cyan-950/10'
                    : isRecv
                    ? 'border-emerald-200 dark:border-emerald-950 bg-emerald-50/30 dark:bg-emerald-950/10'
                    : isPingPong
                    ? 'border-amber-200 dark:border-amber-950 bg-amber-50/30 dark:bg-amber-950/10'
                    : 'border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/30'
                }`}
              >
                {/* Header row */}
                <div
                  className="px-2.5 py-1.5 flex items-center justify-between gap-2 cursor-pointer select-none hover:bg-zinc-100/50 dark:hover:bg-zinc-800/40"
                  onClick={() => toggleExpand(log.id)}
                >
                  <div className="flex items-center gap-2 min-w-0">
                    {/* Expand icon */}
                    {isExpanded ? (
                      <ChevronDown className="w-3.5 h-3.5 text-zinc-400 shrink-0" />
                    ) : (
                      <ChevronRight className="w-3.5 h-3.5 text-zinc-400 shrink-0" />
                    )}

                    {/* Direction badge */}
                    {isSend && (
                      <span className="flex items-center gap-1 font-bold text-[10px] uppercase text-cyan-600 dark:text-cyan-400 bg-cyan-100 dark:bg-cyan-950/80 px-1.5 py-0.5 rounded">
                        <ArrowUpRight className="w-3 h-3" />
                        <span>SENT</span>
                      </span>
                    )}
                    {isRecv && (
                      <span className="flex items-center gap-1 font-bold text-[10px] uppercase text-emerald-600 dark:text-emerald-400 bg-emerald-100 dark:bg-emerald-950/80 px-1.5 py-0.5 rounded">
                        <ArrowDownLeft className="w-3 h-3" />
                        <span>RECV</span>
                      </span>
                    )}
                    {isSys && (
                      <span className="flex items-center gap-1 font-bold text-[10px] uppercase text-purple-600 dark:text-purple-400 bg-purple-100 dark:bg-purple-950/80 px-1.5 py-0.5 rounded">
                        <Info className="w-3 h-3" />
                        <span>SYS</span>
                      </span>
                    )}
                    {isPingPong && (
                      <span className="flex items-center gap-1 font-bold text-[10px] uppercase text-amber-600 dark:text-amber-400 bg-amber-100 dark:bg-amber-950/80 px-1.5 py-0.5 rounded">
                        <Activity className="w-3 h-3" />
                        <span>{log.type}</span>
                      </span>
                    )}

                    {/* Timestamp */}
                    <span className="text-[11px] text-zinc-400 shrink-0">
                      {new Date(log.timestamp).toLocaleTimeString([], {
                        hour12: false,
                        hour: '2-digit',
                        minute: '2-digit',
                        second: '2-digit',
                      })}
                      .{String(new Date(log.timestamp).getMilliseconds()).padStart(3, '0')}
                    </span>

                    {/* Snippet */}
                    <span className="text-zinc-700 dark:text-zinc-300 truncate">
                      {log.payload.slice(0, 100)}
                      {log.payload.length > 100 ? '...' : ''}
                    </span>
                  </div>

                  {/* Size and copy */}
                  <div className="flex items-center gap-2 shrink-0">
                    {log.size > 0 && (
                      <span className="text-[10px] text-zinc-400">{formatBytes(log.size)}</span>
                    )}

                    <Tooltip content="Copy payload">
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={(e) => {
                          e.stopPropagation()
                          handleCopyPayload(log.id, log.payload)
                        }}
                        className="h-6 w-6 p-0 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
                      >
                        {copiedId === log.id ? (
                          <Check className="w-3 h-3 text-emerald-500" />
                        ) : (
                          <Copy className="w-3 h-3" />
                        )}
                      </Button>
                    </Tooltip>
                  </div>
                </div>

                {/* Expanded Payload */}
                {isExpanded && (
                  <div className="border-t border-zinc-200 dark:border-zinc-800 p-2 bg-white dark:bg-zinc-950">
                    {isJSON ? (
                      <CodeMirror
                        value={prettyPayload}
                        extensions={[json()]}
                        theme={theme === 'dark' ? 'dark' : 'light'}
                        readOnly
                        className="text-xs font-mono rounded overflow-hidden"
                      />
                    ) : (
                      <pre className="text-xs font-mono text-zinc-800 dark:text-zinc-200 whitespace-pre-wrap break-all p-2 bg-zinc-50 dark:bg-zinc-900 rounded border border-zinc-200 dark:border-zinc-800">
                        {log.payload}
                      </pre>
                    )}
                  </div>
                )}
              </div>
            )
          })
        )}
        <div ref={listEndRef} />
      </div>
    </div>
  )
}
