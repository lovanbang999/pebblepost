import { useState, useMemo } from 'react'
import {
  CheckCircle2,
  XCircle,
  Clock,
  Database,
  Copy,
  Check,
  Terminal,
  AlertCircle,
  Search,
  FileCode,
} from 'lucide-react'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
import { javascript } from '@codemirror/lang-javascript'
import { xml } from '@codemirror/lang-xml'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { useTabStore } from '../../store/tabStore'
import { formatBytes, formatDuration } from '../../lib/utils'
import type { ConsoleLogEntry } from '../../types'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '../ui/table'
import { Tooltip } from '../ui/tooltip'

// Detect the best CodeMirror language extension from Content-Type header
function detectLanguageExtension(headers: Record<string, string[]>) {
  const ct = Object.entries(headers).find(([k]) => k.toLowerCase() === 'content-type')?.[1]?.[0] || ''
  if (ct.includes('json')) return [json()]
  if (ct.includes('xml') || ct.includes('html')) return [xml()]
  if (ct.includes('javascript')) return [javascript()]
  return [json()] // fallback
}

export function ResponsePanel() {
  const { theme, lastResult: wsLastResult, isExecuting } = useWorkspaceStore()
  const { tabs, activeTabId } = useTabStore()
  const currentTab = tabs.find((t) => t.id === activeTabId)
  const lastResult = currentTab ? currentTab.lastResult : wsLastResult

  const [activeSubTab, setActiveSubTab] = useState<'body' | 'headers' | 'tests' | 'timing' | 'console'>('body')
  const [copied, setCopied] = useState(false)
  const [consoleFilterLevel, setConsoleFilterLevel] = useState<'all' | 'log' | 'info' | 'warn' | 'error'>('all')
  const [consoleSearch, setConsoleSearch] = useState('')

  const langExtensions = useMemo(
    () => lastResult ? detectLanguageExtension(lastResult.headers || {}) : [json()],
    [lastResult]
  )

  const consoleEntries = useMemo(() => {
    if (!lastResult) return []
    if (lastResult.consoleLogs && lastResult.consoleLogs.length > 0) {
      return lastResult.consoleLogs
    }
    // Fallback: parse string logs if structured consoleLogs not provided
    return (lastResult.logs || []).map((l): ConsoleLogEntry => {
      let level: 'log' | 'info' | 'warn' | 'error' = 'log'
      let source = 'Script'
      let msg = l
      if (l.includes('[WARN]')) level = 'warn'
      else if (l.includes('[INFO]')) level = 'info'
      else if (l.includes('[ERROR]') || l.includes('Error]')) level = 'error'

      const match = l.match(/^\[(.*?)\]\s*\[(.*?)\]\s*\[(.*?)\]\s*(.*)$/)
      if (match) {
        return {
          timestamp: match[1],
          source: match[2],
          level: match[3].toLowerCase() as any,
          message: match[4],
        }
      }
      return {
        timestamp: '',
        level,
        source,
        message: msg,
      }
    })
  }, [lastResult])

  const filteredConsoleEntries = useMemo(() => {
    return consoleEntries.filter((entry) => {
      if (consoleFilterLevel !== 'all' && entry.level !== consoleFilterLevel) {
        return false
      }
      if (consoleSearch.trim()) {
        const q = consoleSearch.toLowerCase()
        return entry.message.toLowerCase().includes(q) || entry.source.toLowerCase().includes(q)
      }
      return true
    })
  }, [consoleEntries, consoleFilterLevel, consoleSearch])

  const consoleCounts = useMemo(() => {
    return {
      all: consoleEntries.length,
      log: consoleEntries.filter((e) => e.level === 'log').length,
      info: consoleEntries.filter((e) => e.level === 'info').length,
      warn: consoleEntries.filter((e) => e.level === 'warn').length,
      error: consoleEntries.filter((e) => e.level === 'error').length,
    }
  }, [consoleEntries])

  if (isExecuting) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-white dark:bg-zinc-950 text-zinc-500 dark:text-zinc-400 gap-3 border-l border-zinc-200 dark:border-zinc-800">
        <div className="w-6 h-6 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        <span className="text-xs font-mono">Executing request & network timing trace...</span>
      </div>
    )
  }

  if (!lastResult) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-white dark:bg-zinc-950 text-zinc-500 text-xs gap-1 border-l border-zinc-200 dark:border-zinc-800 p-6 text-center">
        <Clock className="w-8 h-8 text-zinc-300 dark:text-zinc-700 stroke-[1.5] mb-2" />
        <span className="font-semibold text-zinc-900 dark:text-zinc-300 text-sm">No Response Yet</span>
        <span className="max-w-xs text-zinc-500 mt-1">
          Click "Send" or press Ctrl+Enter to execute the request and view response payload, headers, and timing metrics.
        </span>
      </div>
    )
  }

  const isSuccess = lastResult.statusCode >= 200 && lastResult.statusCode < 300
  const isError = lastResult.statusCode >= 400 || lastResult.statusCode === 0

  const handleCopyBody = () => {
    navigator.clipboard.writeText(lastResult.body)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  return (
    <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 border-l border-zinc-200 dark:border-zinc-800 overflow-hidden transition-colors duration-150">
      {/* Response Status Bar */}
      <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-white dark:bg-zinc-950">
        <div className="flex items-center gap-2.5">
          {/* Status Badge */}
          <Badge
            variant={isSuccess ? 'success' : isError ? 'destructive' : 'warning'}
            className="text-xs py-1 px-2.5 gap-1.5 font-bold"
          >
            {isSuccess ? (
              <CheckCircle2 className="w-3.5 h-3.5" />
            ) : (
              <XCircle className="w-3.5 h-3.5" />
            )}
            <span>{lastResult.statusCode}</span>
            <span className="font-normal font-sans opacity-90">{lastResult.statusText}</span>
          </Badge>

          {/* Timing Badge */}
          <Tooltip content="Total Roundtrip Duration">
            <div className="flex items-center gap-1.5 text-xs text-zinc-700 dark:text-zinc-300 font-mono bg-zinc-100 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 px-2 py-1 rounded-md">
              <Clock className="w-3 h-3 text-zinc-400" />
              <span>{formatDuration(lastResult.timing.totalDurationMs)}</span>
            </div>
          </Tooltip>

          {/* Size Badge */}
          <Tooltip content="Response Payload Size">
            <div className="flex items-center gap-1.5 text-xs text-zinc-700 dark:text-zinc-300 font-mono bg-zinc-100 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 px-2 py-1 rounded-md">
              <Database className="w-3 h-3 text-zinc-400" />
              <span>{formatBytes(lastResult.size)}</span>
            </div>
          </Tooltip>
        </div>

        <Tooltip content="Copy Response Body to Clipboard">
          <Button
            variant="outline"
            size="sm"
            onClick={handleCopyBody}
            className="h-7 gap-1 px-2.5 text-xs text-zinc-700 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-white"
          >
            {copied ? (
              <Check className="w-3.5 h-3.5 text-emerald-500 dark:text-emerald-400" />
            ) : (
              <Copy className="w-3.5 h-3.5 text-zinc-400" />
            )}
            <span>{copied ? 'Copied' : 'Copy'}</span>
          </Button>
        </Tooltip>
      </div>

      {/* Sub Tabs */}
      <Tabs
        value={activeSubTab}
        onValueChange={(val) => setActiveSubTab(val as any)}
        className="flex-1 overflow-hidden"
      >
        <TabsList>
          {(['body', 'headers', 'tests', 'timing', 'console'] as const).map((tab) => (
            <TabsTrigger key={tab} value={tab} className="capitalize">
              {tab === 'console' ? <><Terminal className="w-3 h-3 mr-1" />Console</> : tab}
              {tab === 'tests' && lastResult.tests && lastResult.tests.length > 0 && (
                <Badge
                  variant={lastResult.tests.every((t) => t.passed) ? 'success' : 'destructive'}
                  className="ml-1.5 px-1 py-0 text-[9px]"
                >
                  {lastResult.tests.filter((t) => t.passed).length}/{lastResult.tests.length}
                </Badge>
              )}
              {tab === 'console' && lastResult.logs && lastResult.logs.length > 0 && (
                <Badge variant="secondary" className="ml-1.5 px-1 py-0 text-[9px]">
                  {lastResult.logs.length}
                </Badge>
              )}
            </TabsTrigger>
          ))}
        </TabsList>

        {/* Tab Content Panels */}
        <div className="flex-1 overflow-y-auto p-3">
          <TabsContent value="body">
            <div className="h-full rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden bg-white dark:bg-zinc-950">
              <CodeMirror
                value={lastResult.body}
                height="100%"
                extensions={langExtensions}
                theme={theme === 'dark' ? 'dark' : 'light'}
                readOnly
                className="text-xs font-mono"
              />
            </div>
          </TabsContent>

          <TabsContent value="headers">
            <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-1/3">Header</TableHead>
                    <TableHead>Value</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {Object.entries(lastResult.headers || {}).map(([key, vals]) => (
                    <TableRow key={key}>
                      <TableCell className="text-zinc-800 dark:text-zinc-300 font-semibold">{key}</TableCell>
                      <TableCell className="text-zinc-600 dark:text-zinc-400">{vals.join(', ')}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </TabsContent>

          <TabsContent value="tests">
            <div className="space-y-2 text-xs">
              {lastResult.tests && lastResult.tests.length > 0 ? (
                lastResult.tests.map((test, idx) => (
                  <div
                    key={idx}
                    className={`p-2.5 rounded-md border flex items-start gap-2 ${
                      test.passed
                        ? 'bg-emerald-50 dark:bg-emerald-950/20 border-emerald-200 dark:border-emerald-900/50 text-emerald-800 dark:text-emerald-300'
                        : 'bg-rose-50 dark:bg-rose-950/20 border-rose-200 dark:border-rose-900/50 text-rose-800 dark:text-rose-300'
                    }`}
                  >
                    {test.passed ? (
                      <CheckCircle2 className="w-4 h-4 text-emerald-500 dark:text-emerald-400 shrink-0 mt-0.5" />
                    ) : (
                      <XCircle className="w-4 h-4 text-rose-500 dark:text-rose-400 shrink-0 mt-0.5" />
                    )}
                    <div>
                      <div className="font-semibold">{test.name}</div>
                      {test.message && (
                        <div className="text-[11px] font-mono opacity-80 mt-0.5">{test.message}</div>
                      )}
                    </div>
                  </div>
                ))
              ) : (
                <div className="text-zinc-500 text-center py-6">No test assertions configured.</div>
              )}
            </div>
          </TabsContent>

          <TabsContent value="timing">
            <div className="space-y-4 text-xs">
              <span className="font-semibold text-zinc-900 dark:text-zinc-300">Network Timing Breakdown</span>

              {/* Visual Waterfall Bar */}
              {(() => {
                const t = lastResult.timing
                const total = t.totalDurationMs || 1
                const phases = [
                  { label: 'DNS', ms: t.dnsLookupMs, color: 'bg-violet-500' },
                  { label: 'TCP', ms: t.tcpConnectMs, color: 'bg-blue-500' },
                  { label: 'TLS', ms: t.tlsHandshakeMs, color: 'bg-cyan-500' },
                  { label: 'TTFB', ms: t.ttfbMs, color: 'bg-amber-500' },
                  { label: 'Download', ms: t.downloadMs, color: 'bg-emerald-500' },
                ]
                return (
                  <div className="space-y-2.5">
                    {/* Stacked bar */}
                    <div className="h-3 w-full flex rounded-full overflow-hidden border border-zinc-200 dark:border-zinc-800">
                      {phases.map(p => (
                        <div
                          key={p.label}
                          className={`${p.color} h-full transition-all`}
                          style={{ width: `${Math.max((p.ms / total) * 100, 0.5)}%` }}
                          title={`${p.label}: ${p.ms.toFixed(2)}ms`}
                        />
                      ))}
                    </div>

                    {/* Phase rows */}
                    {phases.map(p => (
                      <div key={p.label} className="flex items-center gap-2">
                        <div className={`w-2.5 h-2.5 rounded-sm shrink-0 ${p.color}`} />
                        <span className="text-zinc-600 dark:text-zinc-400 w-24 shrink-0">{p.label}</span>
                        <div className="flex-1 h-1.5 bg-zinc-200 dark:bg-zinc-900 rounded-full overflow-hidden">
                          <div
                            className={`${p.color} h-full rounded-full transition-all`}
                            style={{ width: `${Math.max((p.ms / total) * 100, 0.5)}%` }}
                          />
                        </div>
                        <span className="font-mono text-zinc-800 dark:text-zinc-300 w-20 text-right shrink-0">{p.ms.toFixed(2)} ms</span>
                      </div>
                    ))}

                    <div className="border-t border-zinc-200 dark:border-zinc-800 pt-2.5 flex justify-between font-bold text-zinc-900 dark:text-zinc-200 font-mono">
                      <span>Total Duration:</span>
                      <span className="text-blue-600 dark:text-blue-400">{total.toFixed(2)} ms</span>
                    </div>
                  </div>
                )
              })()}
            </div>
          </TabsContent>

          <TabsContent value="console">
            <div className="flex flex-col h-full space-y-3">
              {/* Console Toolbar */}
              <div className="flex flex-wrap items-center justify-between gap-2 pb-2 border-b border-zinc-200 dark:border-zinc-800">
                {/* Level Filter Chips */}
                <div className="flex items-center gap-1">
                  {(['all', 'log', 'info', 'warn', 'error'] as const).map((lvl) => {
                    const count = consoleCounts[lvl]
                    const isActive = consoleFilterLevel === lvl
                    return (
                      <button
                        key={lvl}
                        onClick={() => setConsoleFilterLevel(lvl)}
                        className={`text-[11px] px-2 py-0.5 rounded-full font-medium transition-colors flex items-center gap-1 cursor-pointer ${
                          isActive
                            ? lvl === 'error'
                              ? 'bg-rose-500 text-white dark:bg-rose-600'
                              : lvl === 'warn'
                              ? 'bg-amber-500 text-white dark:bg-amber-600'
                              : lvl === 'info'
                              ? 'bg-blue-500 text-white dark:bg-blue-600'
                              : 'bg-zinc-800 text-white dark:bg-zinc-200 dark:text-zinc-900'
                            : 'bg-zinc-100 dark:bg-zinc-900 text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800'
                        }`}
                      >
                        <span className="capitalize">{lvl}</span>
                        <span className="text-[10px] opacity-80">({count})</span>
                      </button>
                    )
                  })}
                </div>

                {/* Search Bar */}
                <div className="flex items-center gap-2">
                  <div className="relative">
                    <Search className="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-zinc-400" />
                    <input
                      type="text"
                      placeholder="Filter console..."
                      value={consoleSearch}
                      onChange={(e) => setConsoleSearch(e.target.value)}
                      className="h-6.5 text-[11px] pl-7 pr-2 rounded bg-zinc-50 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 text-zinc-800 dark:text-zinc-200 focus:outline-hidden focus:border-zinc-400 dark:focus:border-zinc-600 w-36"
                    />
                  </div>
                  {consoleEntries.length > 0 && (
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        const text = consoleEntries
                          .map((e) => `[${e.timestamp || 'N/A'}] [${e.source}] [${e.level.toUpperCase()}] ${e.message}`)
                          .join('\n')
                        navigator.clipboard.writeText(text)
                      }}
                      className="h-6.5 px-2 text-[10px] gap-1 text-zinc-600 dark:text-zinc-400 cursor-pointer"
                      title="Copy all console output"
                    >
                      <Copy className="w-3 h-3" />
                      Copy
                    </Button>
                  )}
                </div>
              </div>

              {/* Dedicated Error Alert if script failed */}
              {lastResult.error && (
                <div className="p-3 rounded-md bg-rose-50 dark:bg-rose-950/20 border border-rose-200 dark:border-rose-900/50 text-rose-800 dark:text-rose-300 text-xs">
                  <div className="flex items-start gap-2">
                    <AlertCircle className="w-4 h-4 text-rose-500 shrink-0 mt-0.5" />
                    <div className="flex-1 font-mono">
                      <div className="font-semibold text-rose-900 dark:text-rose-200 mb-1 flex items-center gap-2">
                        Script Execution Error
                      </div>
                      <pre className="text-[11px] whitespace-pre-wrap break-all opacity-90">{lastResult.error}</pre>
                    </div>
                  </div>
                </div>
              )}

              {/* Console Entries List */}
              <div className="space-y-1.5 overflow-y-auto">
                {filteredConsoleEntries.length > 0 ? (
                  filteredConsoleEntries.map((entry, idx) => {
                    const isError = entry.level === 'error'
                    const isWarn = entry.level === 'warn'
                    const isInfo = entry.level === 'info'

                    return (
                      <div
                        key={idx}
                        className={`flex items-start gap-2 font-mono text-[11px] p-2 rounded border transition-colors ${
                          isError
                            ? 'bg-rose-50/60 dark:bg-rose-950/20 border-rose-200/70 dark:border-rose-900/40 text-rose-900 dark:text-rose-300'
                            : isWarn
                            ? 'bg-amber-50/60 dark:bg-amber-950/20 border-amber-200/70 dark:border-amber-900/40 text-amber-900 dark:text-amber-300'
                            : isInfo
                            ? 'bg-blue-50/60 dark:bg-blue-950/20 border-blue-200/70 dark:border-blue-900/40 text-blue-900 dark:text-blue-300'
                            : 'bg-zinc-50 dark:bg-zinc-900/50 border-zinc-200 dark:border-zinc-800/60 text-zinc-800 dark:text-zinc-200'
                        }`}
                      >
                        {/* Timestamp */}
                        {entry.timestamp && (
                          <span className="text-zinc-400 dark:text-zinc-500 text-[10px] shrink-0 select-none">
                            {entry.timestamp}
                          </span>
                        )}

                        {/* Level Badge */}
                        <span
                          className={`text-[9px] uppercase px-1.5 py-0.2 rounded font-bold shrink-0 ${
                            isError
                              ? 'bg-rose-500 text-white dark:bg-rose-600'
                              : isWarn
                              ? 'bg-amber-500 text-white dark:bg-amber-600'
                              : isInfo
                              ? 'bg-blue-500 text-white dark:bg-blue-600'
                              : 'bg-zinc-200 dark:bg-zinc-800 text-zinc-700 dark:text-zinc-300'
                          }`}
                        >
                          {entry.level}
                        </span>

                        {/* Script Source Badge */}
                        <span className="text-[10px] text-zinc-500 dark:text-zinc-400 font-sans px-1 py-0.2 bg-zinc-200/60 dark:bg-zinc-800/60 rounded shrink-0 flex items-center gap-1">
                          <FileCode className="w-2.5 h-2.5" />
                          {entry.source}
                          {entry.line ? `:${entry.line}` : ''}
                        </span>

                        {/* Message content */}
                        <div className="flex-1 min-w-0">
                          <pre className="whitespace-pre-wrap break-all text-[11px] font-mono leading-relaxed">
                            {entry.message}
                          </pre>
                        </div>
                      </div>
                    )
                  })
                ) : (
                  <div className="flex flex-col items-center justify-center py-10 text-zinc-500 text-xs">
                    <Terminal className="w-7 h-7 mb-2 opacity-30" />
                    <span>
                      {consoleEntries.length > 0
                        ? 'No console entries match the active filter.'
                        : 'No console output from scripts.'}
                    </span>
                    <span className="text-zinc-600 dark:text-zinc-500 mt-1">
                      Use <code className="font-mono">console.log(...)</code>,{' '}
                      <code className="font-mono">console.info(...)</code>,{' '}
                      <code className="font-mono">console.warn(...)</code>, or{' '}
                      <code className="font-mono">console.error(...)</code> in scripts.
                    </span>
                  </div>
                )}
              </div>
            </div>
          </TabsContent>
        </div>
      </Tabs>
    </div>
  )
}
