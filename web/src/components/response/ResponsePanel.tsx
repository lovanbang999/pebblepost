import { useState, useMemo, useEffect, useRef, useCallback } from 'react'
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
  Filter,
  ArrowRight,
  Download,
  ChevronDown,
  ChevronRight,
  Terminal as CurlIcon,
  Link,
  Radio,
} from 'lucide-react'
import { GrpcStreamTimeline } from '../grpc/GrpcStreamTimeline'
import { StreamLogPanel } from '../stream/StreamLogPanel'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
import { javascript } from '@codemirror/lang-javascript'
import { xml } from '@codemirror/lang-xml'
import { JSONPath } from 'jsonpath-plus'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { useTabStore } from '../../store/tabStore'
import { formatBytes, formatDuration } from '../../lib/utils'
import type { ConsoleLogEntry, RedirectHop, ExecutionResult } from '../../types'
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
import { HexViewer } from './HexViewer'

// ─── Content Category Detection ──────────────────────────────────────────────

type ContentCategory = 'json' | 'xml' | 'html' | 'image' | 'pdf' | 'binary' | 'text'

function detectContentCategory(headers: Record<string, string[]>, body: string): ContentCategory {
  const ct = Object.entries(headers).find(([k]) => k.toLowerCase() === 'content-type')?.[1]?.[0] || ''
  const ctLower = ct.toLowerCase()
  if (ctLower.includes('json')) return 'json'
  if (ctLower.includes('xml')) return 'xml'
  if (ctLower.includes('html')) return 'html'
  if (ctLower.startsWith('image/')) return 'image'
  if (ctLower.includes('pdf')) return 'pdf'
  if (ctLower.includes('javascript')) return 'text'
  if (ctLower.includes('text/')) return 'text'
  // Content-sniff: detect JSON/XML from body
  const trimmed = body.trimStart()
  if (trimmed.startsWith('{') || trimmed.startsWith('[')) return 'json'
  if (trimmed.startsWith('<')) return 'xml'
  return 'binary'
}

function detectLangExtension(category: ContentCategory) {
  if (category === 'json') return [json()]
  if (category === 'xml' || category === 'html') return [xml()]
  if (category === 'text') return [javascript()]
  return [json()]
}

// ─── Copy-as-cURL Builder ────────────────────────────────────────────────────

function buildCurlCommand(result: ExecutionResult | null | undefined): string {
  if (!result?.sentRequest) return ''
  const { method, url, headers, body } = result.sentRequest
  const parts = [`curl -X ${method} '${url}'`]
  for (const [key, vals] of Object.entries(headers || {})) {
    parts.push(`  -H '${key}: ${vals.join(', ')}'`)
  }
  if (body) {
    const escaped = body.replace(/'/g, "'\\''")
    parts.push(`  -d '${escaped}'`)
  }
  return parts.join(' \\\n')
}

// ─── Small reusable helpers ───────────────────────────────────────────────────

function MetaBadge({ icon: Icon, label, tooltip }: { icon: React.ElementType; label: string; tooltip: string }) {
  return (
    <Tooltip content={tooltip}>
      <div className="flex items-center gap-1.5 text-xs text-zinc-700 dark:text-zinc-300 font-mono bg-zinc-100 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 px-2 py-1 rounded-md">
        <Icon className="w-3 h-3 text-zinc-400" />
        <span>{label}</span>
      </div>
    </Tooltip>
  )
}

// ─── Body Search Bar ─────────────────────────────────────────────────────────

interface BodySearchBarProps {
  body: string
  onHighlight: (matches: number[], current: number) => void
  onClose: () => void
}

function BodySearchBar({ body, onHighlight, onClose }: BodySearchBarProps) {
  const [query, setQuery] = useState('')
  const [matchCount, setMatchCount] = useState(0)
  const [matchIndex, setMatchIndex] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => { inputRef.current?.focus() }, [])

  useEffect(() => {
    if (!query) { setMatchCount(0); onHighlight([], 0); return }
    const regex = new RegExp(query.replace(/[.*+?^${}()|[\]\\]/g, '\\$&'), 'gi')
    const positions: number[] = []
    let m: RegExpExecArray | null
    while ((m = regex.exec(body)) !== null) positions.push(m.index)
    setMatchCount(positions.length)
    setMatchIndex(0)
    onHighlight(positions, 0)
  }, [query, body, onHighlight])

  const nav = (dir: 1 | -1) => {
    if (matchCount === 0) return
    const next = (matchIndex + dir + matchCount) % matchCount
    setMatchIndex(next)
    onHighlight([], next)
  }

  return (
    <div className="flex items-center gap-2 px-3 py-1.5 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/60">
      <Search className="w-3.5 h-3.5 text-zinc-400 shrink-0" />
      <input
        ref={inputRef}
        value={query}
        onChange={(e) => setQuery(e.target.value)}
        onKeyDown={(e) => { if (e.key === 'Enter') nav(e.shiftKey ? -1 : 1) }}
        placeholder="Find in response…"
        className="flex-1 text-xs bg-transparent outline-none text-zinc-800 dark:text-zinc-200 placeholder:text-zinc-400"
      />
      {matchCount > 0 && (
        <span className="text-[11px] text-zinc-500 font-mono shrink-0">
          {matchIndex + 1}/{matchCount}
        </span>
      )}
      <button onClick={() => nav(1)} className="text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200" title="Next match (Enter)">
        <ChevronDown className="w-3.5 h-3.5" />
      </button>
      <button onClick={() => nav(-1)} className="text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200" title="Prev match (Shift+Enter)">
        <ChevronRight className="w-3.5 h-3.5 -rotate-90" />
      </button>
      <button onClick={onClose} className="text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 text-xs" title="Close (Esc)">✕</button>
    </div>
  )
}

// ─── JSONPath Filter ──────────────────────────────────────────────────────────

interface JsonPathFilterProps {
  body: string
  theme: string
}

function JsonPathFilter({ body, theme }: JsonPathFilterProps) {
  const [expr, setExpr] = useState('$')
  const [result, setResult] = useState<string>('')
  const [error, setError] = useState<string>('')

  const evaluate = useCallback(() => {
    try {
      const parsed = JSON.parse(body)
      const found = JSONPath({ path: expr, json: parsed })
      setResult(JSON.stringify(found, null, 2))
      setError('')
    } catch (e: unknown) {
      setError(e instanceof Error ? e.message : String(e))
      setResult('')
    }
  }, [body, expr])

  return (
    <div className="flex flex-col gap-2">
      <div className="flex items-center gap-2">
        <div className="relative flex-1">
          <Filter className="w-3.5 h-3.5 absolute left-2.5 top-1/2 -translate-y-1/2 text-zinc-400 pointer-events-none" />
          <input
            value={expr}
            onChange={(e) => setExpr(e.target.value)}
            onKeyDown={(e) => { if (e.key === 'Enter') evaluate() }}
            placeholder="JSONPath, e.g. $.data[*].id"
            className="w-full text-xs pl-8 pr-3 h-8 rounded-md border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900 text-zinc-800 dark:text-zinc-200 placeholder:text-zinc-400 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>
        <Button size="sm" variant="outline" onClick={evaluate} className="h-8 px-3 text-xs gap-1">
          <Filter className="w-3 h-3" />
          Run
        </Button>
      </div>
      {error && (
        <div className="text-xs text-rose-600 dark:text-rose-400 font-mono px-2 py-1 bg-rose-50 dark:bg-rose-950/20 rounded border border-rose-200 dark:border-rose-900/50">
          {error}
        </div>
      )}
      {result && (
        <div className="rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden">
          <CodeMirror
            value={result}
            height="200px"
            extensions={[json()]}
            theme={theme === 'dark' ? 'dark' : 'light'}
            readOnly
            className="text-xs font-mono"
          />
        </div>
      )}
    </div>
  )
}

// ─── Redirect Chain ───────────────────────────────────────────────────────────

function RedirectChainView({ hops }: { hops: RedirectHop[] }) {
  const [expanded, setExpanded] = useState(false)

  if (hops.length === 0) return null

  return (
    <div className="mt-2">
      <button
        onClick={() => setExpanded((p) => !p)}
        className="flex items-center gap-1.5 text-xs text-amber-600 dark:text-amber-400 hover:text-amber-800 dark:hover:text-amber-300 font-medium"
      >
        {expanded ? <ChevronDown className="w-3.5 h-3.5" /> : <ChevronRight className="w-3.5 h-3.5" />}
        <Link className="w-3 h-3" />
        {hops.length} redirect{hops.length !== 1 ? 's' : ''} followed
      </button>
      {expanded && (
        <div className="mt-1.5 space-y-1.5 pl-4 border-l-2 border-amber-200 dark:border-amber-900">
          {hops.map((hop, i) => (
            <div key={i} className="text-xs font-mono text-zinc-600 dark:text-zinc-400 flex items-center gap-2">
              <span className="text-amber-500 font-bold shrink-0">{hop.statusCode || '→'}</span>
              <span className="text-zinc-500 shrink-0">{hop.method}</span>
              <span className="truncate">{hop.url}</span>
              {hop.headers?.Location && (
                <>
                  <ArrowRight className="w-3 h-3 shrink-0 text-zinc-400" />
                  <span className="text-blue-500 dark:text-blue-400 truncate">{hop.headers.Location}</span>
                </>
              )}
            </div>
          ))}
        </div>
      )}
    </div>
  )
}

// ─── Main Component ───────────────────────────────────────────────────────────

type SubTab = 'body' | 'stream' | 'headers' | 'trailers' | 'tests' | 'timing' | 'console'

export function ResponsePanel() {
  const { theme, lastResult: wsLastResult, isExecuting } = useWorkspaceStore()
  const { tabs, activeTabId, setLastResult } = useTabStore()
  const currentTab = tabs.find((t) => t.id === activeTabId)
  const lastResult = currentTab ? currentTab.lastResult : wsLastResult

  const [activeSubTab, setActiveSubTab] = useState<SubTab>('body')
  const [copied, setCopied] = useState(false)
  const [curlCopied, setCurlCopied] = useState(false)
  const [consoleFilterLevel, setConsoleFilterLevel] = useState<'all' | 'log' | 'info' | 'warn' | 'error'>('all')
  const [consoleSearch, setConsoleSearch] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const [showJsonPath, setShowJsonPath] = useState(false)
  const [loadedBody, setLoadedBody] = useState<string | null>(null)
  const [isLoadingMore, setIsLoadingMore] = useState(false)
  const [loadOffset, setLoadOffset] = useState(0)

  // Reset per-result state when the result changes
  useEffect(() => {
    setLoadedBody(null)
    setLoadOffset(0)
    setSearchOpen(false)
    setShowJsonPath(false)
    if ((lastResult?.grpcMessages && lastResult.grpcMessages.length > 0) || (lastResult?.streamLogs && lastResult.streamLogs.length > 0)) {
      setActiveSubTab('stream')
    }
  }, [lastResult?.executedAt, lastResult?.grpcMessages, lastResult?.streamLogs])

  // Ctrl+F shortcut
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && e.key === 'f' && lastResult) {
        e.preventDefault()
        setActiveSubTab('body')
        setSearchOpen(true)
      }
      if (e.key === 'Escape') setSearchOpen(false)
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [lastResult])

  const displayBody = loadedBody ?? lastResult?.body ?? ''
  const category = useMemo(
    () => (lastResult ? detectContentCategory(lastResult.headers || {}, displayBody) : 'json'),
    [lastResult, displayBody],
  )
  const langExtensions = useMemo(() => detectLangExtension(category), [category])

  const consoleEntries = useMemo(() => {
    if (!lastResult) return []
    if (lastResult.consoleLogs && lastResult.consoleLogs.length > 0) return lastResult.consoleLogs
    return (lastResult.logs || []).map((l): ConsoleLogEntry => {
      let level: ConsoleLogEntry['level'] = 'log'
      if (l.includes('[WARN]')) level = 'warn'
      else if (l.includes('[INFO]')) level = 'info'
      else if (l.includes('[ERROR]') || l.includes('Error]')) level = 'error'
      const match = l.match(/^\[(.*?)\]\s*\[(.*?)\]\s*\[(.*?)\]\s*(.*)$/)
      if (match) {
        return { timestamp: match[1], source: match[2], level: match[3].toLowerCase() as ConsoleLogEntry['level'], message: match[4] }
      }
      return { timestamp: '', level, source: 'Script', message: l }
    })
  }, [lastResult])

  const filteredConsoleEntries = useMemo(() => {
    return consoleEntries.filter((entry) => {
      if (consoleFilterLevel !== 'all' && entry.level !== consoleFilterLevel) return false
      if (consoleSearch.trim()) {
        const q = consoleSearch.toLowerCase()
        return entry.message.toLowerCase().includes(q) || entry.source.toLowerCase().includes(q)
      }
      return true
    })
  }, [consoleEntries, consoleFilterLevel, consoleSearch])

  const consoleCounts = useMemo(() => ({
    all: consoleEntries.length,
    log: consoleEntries.filter((e) => e.level === 'log').length,
    info: consoleEntries.filter((e) => e.level === 'info').length,
    warn: consoleEntries.filter((e) => e.level === 'warn').length,
    error: consoleEntries.filter((e) => e.level === 'error').length,
  }), [consoleEntries])

  // ── Assertions grouping ────────────────────────────────────────────────────
  const passedTests = useMemo(() => (lastResult?.tests || []).filter((t) => t.passed), [lastResult])
  const failedTests = useMemo(() => (lastResult?.tests || []).filter((t) => !t.passed), [lastResult])

  const handleCopyBody = () => {
    navigator.clipboard.writeText(displayBody)
    setCopied(true)
    setTimeout(() => setCopied(false), 2000)
  }

  const handleCopyCurl = () => {
    if (!lastResult) return
    navigator.clipboard.writeText(buildCurlCommand(lastResult))
    setCurlCopied(true)
    setTimeout(() => setCurlCopied(false), 2000)
  }

  const handleLoadMore = async () => {
    if (!lastResult?.tempBodyFile) return
    setIsLoadingMore(true)
    const nextOffset = loadOffset + 5 * 1024 * 1024
    try {
      const res = await fetch(
        `/api/response/body?file=${encodeURIComponent(lastResult.tempBodyFile)}&offset=${nextOffset}`,
      )
      if (res.ok) {
        const data = await res.json()
        setLoadedBody((prev) => (prev ?? lastResult.body) + data.chunk)
        setLoadOffset(nextOffset)
      }
    } finally {
      setIsLoadingMore(false)
    }
  }

  const handleSaveToFile = () => {
    if (!lastResult) return
    const blob = new Blob([displayBody], { type: 'application/octet-stream' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = 'response-body.bin'
    a.click()
    URL.revokeObjectURL(a.href)
  }

  // ── Loading state ──────────────────────────────────────────────────────────
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
  const displaySize = lastResult.bodySizeBytes ?? lastResult.size

  // ── Render ─────────────────────────────────────────────────────────────────
  return (
    <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 border-l border-zinc-200 dark:border-zinc-800 overflow-hidden transition-colors duration-150">

      {/* ── Status Bar ── */}
      <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-white dark:bg-zinc-950 flex-wrap gap-2">
        <div className="flex items-center gap-2 flex-wrap">
          {/* Status Code Badge */}
          <Badge
            variant={isSuccess ? 'success' : isError ? 'destructive' : 'warning'}
            className="text-xs py-1 px-2.5 gap-1.5 font-bold"
          >
            {isSuccess ? <CheckCircle2 className="w-3.5 h-3.5" /> : <XCircle className="w-3.5 h-3.5" />}
            <span>{lastResult.statusCode}</span>
            <span className="font-normal font-sans opacity-90">{lastResult.statusText}</span>
          </Badge>

          {/* gRPC Status Badge */}
          {lastResult.grpcStatus !== undefined && (
            <Badge
              variant={lastResult.grpcStatus === 0 ? 'success' : 'destructive'}
              className="text-xs py-1 px-2.5 gap-1.5 font-bold font-mono"
            >
              {lastResult.grpcStatus === 0 ? <CheckCircle2 className="w-3.5 h-3.5" /> : <XCircle className="w-3.5 h-3.5" />}
              <span>gRPC: {lastResult.grpcStatus} {lastResult.grpcStatusText}</span>
            </Badge>
          )}

          {/* Timing */}
          <MetaBadge icon={Clock} label={formatDuration(lastResult.timing.totalDurationMs)} tooltip="Total Roundtrip Duration" />

          {/* Size */}
          <MetaBadge icon={Database} label={formatBytes(displaySize)} tooltip={`Body size: ${displaySize.toLocaleString()} bytes`} />
        </div>

        <div className="flex items-center gap-1.5">
          {/* Copy as cURL */}
          {lastResult.sentRequest && (
            <Tooltip content="Copy as cURL command">
              <Button
                variant="outline"
                size="sm"
                onClick={handleCopyCurl}
                className="h-7 gap-1 px-2.5 text-xs text-zinc-700 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-white"
              >
                {curlCopied ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <CurlIcon className="w-3.5 h-3.5 text-zinc-400" />}
                <span>{curlCopied ? 'Copied!' : 'cURL'}</span>
              </Button>
            </Tooltip>
          )}

          {/* Ctrl+F Search toggle */}
          <Tooltip content="Search in body (Ctrl+F)">
            <Button
              variant={searchOpen ? 'default' : 'outline'}
              size="sm"
              onClick={() => { setActiveSubTab('body'); setSearchOpen((p) => !p) }}
              className="h-7 gap-1 px-2.5 text-xs"
            >
              <Search className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>

          {/* Copy Body */}
          <Tooltip content="Copy Response Body">
            <Button
              variant="outline"
              size="sm"
              onClick={handleCopyBody}
              className="h-7 gap-1 px-2.5 text-xs text-zinc-700 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-white"
            >
              {copied ? <Check className="w-3.5 h-3.5 text-emerald-500" /> : <Copy className="w-3.5 h-3.5 text-zinc-400" />}
              <span>{copied ? 'Copied' : 'Copy'}</span>
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* ── Redirect Chain ── */}
      {(lastResult.redirectChain?.length ?? 0) > 0 && (
        <div className="px-3 py-1.5 border-b border-amber-200 dark:border-amber-900/40 bg-amber-50 dark:bg-amber-950/10">
          <RedirectChainView hops={lastResult.redirectChain!} />
        </div>
      )}

      {/* ── Sub Tabs ── */}
      <Tabs
        value={activeSubTab}
        onValueChange={(val) => setActiveSubTab(val as SubTab)}
        className="flex-1 overflow-hidden flex flex-col"
      >
        <TabsList>
          {/* Stream Tab if stream logs or gRPC stream messages present */}
          {((lastResult.streamLogs && lastResult.streamLogs.length > 0) || (lastResult.grpcMessages && lastResult.grpcMessages.length > 0)) && (
            <TabsTrigger value="stream" className="gap-1.5">
              <Radio className="w-3.5 h-3.5 text-cyan-500" />
              <span>Stream</span>
              <Badge variant="secondary" className="ml-1 px-1 py-0 text-[9px] bg-cyan-100 dark:bg-cyan-950 text-cyan-700 dark:text-cyan-300 font-bold">
                {lastResult.streamLogs ? lastResult.streamLogs.length : lastResult.grpcMessages?.length}
              </Badge>
            </TabsTrigger>
          )}

          {(['body', 'headers'] as const).map((tab) => (
            <TabsTrigger key={tab} value={tab} className="capitalize">
              {tab === 'headers' && (lastResult.grpcMetadata ? 'Metadata / Headers' : 'Headers')}
            </TabsTrigger>
          ))}

          {/* Trailers Tab if gRPC trailers present */}
          {lastResult.grpcTrailers && Object.keys(lastResult.grpcTrailers).length > 0 && (
            <TabsTrigger value="trailers" className="gap-1.5">
              <span>Trailers</span>
              <Badge variant="secondary" className="ml-1 px-1 py-0 text-[9px]">
                {Object.keys(lastResult.grpcTrailers).length}
              </Badge>
            </TabsTrigger>
          )}

          {(['tests', 'timing', 'console'] as const).map((tab) => (
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

        {/* ── Body Search Bar ── */}
        {searchOpen && activeSubTab === 'body' && (
          <BodySearchBar
            body={displayBody}
            onHighlight={() => { /* highlighting via future CodeMirror search extension */ }}
            onClose={() => setSearchOpen(false)}
          />
        )}

        <div className="flex-1 overflow-y-auto p-3">
          {/* ── BODY TAB ── */}
          <TabsContent value="body">
            <div className="space-y-2">
              {/* JSONPath Filter Toggle */}
              {category === 'json' && (
                <div className="flex items-center gap-2">
                  <button
                    onClick={() => setShowJsonPath((p) => !p)}
                    className="text-[11px] flex items-center gap-1 text-zinc-500 dark:text-zinc-400 hover:text-zinc-800 dark:hover:text-zinc-200 transition-colors"
                  >
                    <Filter className="w-3 h-3" />
                    JSONPath filter
                    {showJsonPath ? <ChevronDown className="w-3 h-3" /> : <ChevronRight className="w-3 h-3" />}
                  </button>
                </div>
              )}
              {showJsonPath && category === 'json' && (
                <JsonPathFilter body={displayBody} theme={theme} />
              )}

              {/* Large body banner */}
              {lastResult.bodyTruncated && (
                <div className="flex items-center justify-between gap-3 px-3 py-2 rounded-md bg-amber-50 dark:bg-amber-950/20 border border-amber-200 dark:border-amber-900/50 text-xs text-amber-800 dark:text-amber-300">
                  <span>
                    Showing first 5 MB of {formatBytes(lastResult.bodySizeBytes ?? lastResult.size)}.
                    Large responses are not fully rendered to preserve performance.
                  </span>
                  <div className="flex items-center gap-2 shrink-0">
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-6 px-2 text-[11px] gap-1 border-amber-300 dark:border-amber-700 text-amber-800 dark:text-amber-300"
                      onClick={handleLoadMore}
                      disabled={isLoadingMore}
                    >
                      {isLoadingMore ? (
                        <span className="w-3 h-3 border border-amber-400 border-t-transparent rounded-full animate-spin inline-block" />
                      ) : (
                        <ChevronDown className="w-3 h-3" />
                      )}
                      Load more
                    </Button>
                    <Button
                      size="sm"
                      variant="outline"
                      className="h-6 px-2 text-[11px] gap-1 border-amber-300 dark:border-amber-700 text-amber-800 dark:text-amber-300"
                      onClick={handleSaveToFile}
                    >
                      <Download className="w-3 h-3" />
                      Save file
                    </Button>
                  </div>
                </div>
              )}

              {/* Body renderer based on content category */}
              {category === 'image' ? (
                <div className="flex items-center justify-center p-6 rounded-md border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900">
                  <img
                    src={`data:${Object.entries(lastResult.headers).find(([k]) => k.toLowerCase() === 'content-type')?.[1]?.[0] ?? 'image/*'};base64,${btoa(displayBody)}`}
                    alt="Response image"
                    className="max-w-full max-h-96 rounded shadow-sm"
                    onError={(e) => { (e.currentTarget as HTMLImageElement).style.display = 'none' }}
                  />
                </div>
              ) : category === 'html' ? (
                <div className="rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden">
                  <div className="flex items-center gap-2 px-3 py-1.5 bg-zinc-100 dark:bg-zinc-900 border-b border-zinc-200 dark:border-zinc-800 text-[11px] text-zinc-500">
                    <span className="bg-orange-400 rounded-full w-2 h-2" />
                    Sandboxed HTML Preview
                  </div>
                  <iframe
                    srcDoc={displayBody}
                    sandbox="allow-scripts"
                    title="HTML Preview"
                    className="w-full h-64 bg-white"
                  />
                </div>
              ) : category === 'pdf' ? (
                <div className="rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden h-96">
                  <embed
                    src={`data:application/pdf;base64,${btoa(displayBody)}`}
                    type="application/pdf"
                    className="w-full h-full"
                  />
                </div>
              ) : category === 'binary' ? (
                <HexViewer data={displayBody} maxBytes={1024} />
              ) : (
                <div className="rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden bg-white dark:bg-zinc-950">
                  <CodeMirror
                    value={displayBody}
                    height="100%"
                    minHeight="200px"
                    extensions={langExtensions}
                    theme={theme === 'dark' ? 'dark' : 'light'}
                    readOnly
                    className="text-xs font-mono"
                  />
                </div>
              )}
            </div>
          </TabsContent>

          {/* ── STREAM TIMELINE / LOG TAB ── */}
          {lastResult.streamLogs && lastResult.streamLogs.length > 0 ? (
            <TabsContent value="stream" className="h-[calc(100vh-230px)] min-h-100 m-0 -m-3">
              <StreamLogPanel
                logs={lastResult.streamLogs}
                isLive={isExecuting}
                closeCode={lastResult.streamCloseCode}
                closeReason={lastResult.streamCloseReason}
                evictedCount={lastResult.streamEvicted}
                onClearLogs={() => {
                  setLastResult((prev) => (prev ? { ...prev, streamLogs: [] } : null))
                }}
              />
            </TabsContent>
          ) : lastResult.grpcMessages && lastResult.grpcMessages.length > 0 ? (
            <TabsContent value="stream" className="h-[calc(100vh-230px)] min-h-100 m-0 -m-3">
              <GrpcStreamTimeline messages={lastResult.grpcMessages} isLive={isExecuting} />
            </TabsContent>
          ) : null}

          {/* ── TRAILERS TAB ── */}
          {lastResult.grpcTrailers && Object.keys(lastResult.grpcTrailers).length > 0 && (
            <TabsContent value="trailers">
              <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
                <div className="px-3 py-2 border-b border-zinc-200 dark:border-zinc-800 text-[11px] font-semibold text-zinc-500 dark:text-zinc-400 uppercase tracking-wide">
                  gRPC Response Trailers
                </div>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead className="w-1/3">Trailer</TableHead>
                      <TableHead>Value</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {Object.entries(lastResult.grpcTrailers).map(([key, vals]) => (
                      <TableRow key={key}>
                        <TableCell className="text-zinc-800 dark:text-zinc-300 font-semibold font-mono">{key}</TableCell>
                        <TableCell className="text-zinc-600 dark:text-zinc-400 font-mono">{vals.join(', ')}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            </TabsContent>
          )}

          {/* ── HEADERS TAB ── */}
          <TabsContent value="headers">
            <div className="space-y-3">
              {/* Response headers */}
              <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
                <div className="px-3 py-2 border-b border-zinc-200 dark:border-zinc-800 text-[11px] font-semibold text-zinc-500 dark:text-zinc-400 uppercase tracking-wide">
                  Response Headers
                </div>
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

              {/* Sent request headers */}
              {lastResult.sentRequest && (
                <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
                  <div className="px-3 py-2 border-b border-zinc-200 dark:border-zinc-800 text-[11px] font-semibold text-zinc-500 dark:text-zinc-400 uppercase tracking-wide">
                    Request Headers Sent
                  </div>
                  <Table>
                    <TableHeader>
                      <TableRow>
                        <TableHead className="w-1/3">Header</TableHead>
                        <TableHead>Value</TableHead>
                      </TableRow>
                    </TableHeader>
                    <TableBody>
                      {Object.entries(lastResult.sentRequest.headers || {}).map(([key, vals]) => (
                        <TableRow key={key}>
                          <TableCell className="text-zinc-800 dark:text-zinc-300 font-semibold">{key}</TableCell>
                          <TableCell className="text-zinc-600 dark:text-zinc-400 font-mono text-[11px]">
                            {vals.join(', ')}
                          </TableCell>
                        </TableRow>
                      ))}
                    </TableBody>
                  </Table>
                </div>
              )}
            </div>
          </TabsContent>

          {/* ── TESTS TAB ── */}
          <TabsContent value="tests">
            <div className="space-y-3 text-xs">
              {lastResult.tests && lastResult.tests.length > 0 ? (
                <>
                  {/* Summary bar */}
                  <div className="flex items-center gap-3 px-3 py-2 rounded-md bg-zinc-50 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800">
                    {passedTests.length > 0 && (
                      <span className="flex items-center gap-1.5 text-emerald-700 dark:text-emerald-400 font-semibold">
                        <CheckCircle2 className="w-3.5 h-3.5" />
                        {passedTests.length} passed
                      </span>
                    )}
                    {failedTests.length > 0 && (
                      <span className="flex items-center gap-1.5 text-rose-600 dark:text-rose-400 font-semibold">
                        <XCircle className="w-3.5 h-3.5" />
                        {failedTests.length} failed
                      </span>
                    )}
                  </div>

                  {/* Failed assertions first */}
                  {failedTests.length > 0 && (
                    <div className="space-y-1.5">
                      <div className="text-[10px] uppercase tracking-wide font-bold text-rose-500 dark:text-rose-400 px-0.5">
                        Failures
                      </div>
                      {failedTests.map((test, idx) => (
                        <div
                          key={idx}
                          className="p-2.5 rounded-md border flex items-start gap-2 bg-rose-50 dark:bg-rose-950/20 border-rose-200 dark:border-rose-900/50 text-rose-800 dark:text-rose-300"
                        >
                          <XCircle className="w-4 h-4 text-rose-500 dark:text-rose-400 shrink-0 mt-0.5" />
                          <div className="min-w-0">
                            <div className="font-semibold">{test.name}</div>
                            {test.message && (
                              <div className="text-[11px] font-mono opacity-80 mt-0.5 whitespace-pre-wrap break-all">
                                {test.message}
                              </div>
                            )}
                          </div>
                        </div>
                      ))}
                    </div>
                  )}

                  {/* Passed assertions */}
                  {passedTests.length > 0 && (
                    <div className="space-y-1.5">
                      <div className="text-[10px] uppercase tracking-wide font-bold text-emerald-600 dark:text-emerald-400 px-0.5">
                        Passed
                      </div>
                      {passedTests.map((test, idx) => (
                        <div
                          key={idx}
                          className="p-2.5 rounded-md border flex items-start gap-2 bg-emerald-50 dark:bg-emerald-950/20 border-emerald-200 dark:border-emerald-900/50 text-emerald-800 dark:text-emerald-300"
                        >
                          <CheckCircle2 className="w-4 h-4 text-emerald-500 dark:text-emerald-400 shrink-0 mt-0.5" />
                          <div className="font-semibold">{test.name}</div>
                        </div>
                      ))}
                    </div>
                  )}
                </>
              ) : (
                <div className="text-zinc-500 text-center py-6">No test assertions configured.</div>
              )}
            </div>
          </TabsContent>

          {/* ── TIMING TAB ── */}
          <TabsContent value="timing">
            <div className="space-y-4 text-xs">
              <span className="font-semibold text-zinc-900 dark:text-zinc-300">Network Timing Breakdown</span>

              {(() => {
                const t = lastResult.timing
                const total = t.totalDurationMs || 1
                const phases = [
                  { label: 'DNS', ms: t.dnsLookupMs, color: 'bg-violet-500', key: 'dns' },
                  { label: 'TCP', ms: t.tcpConnectMs, color: 'bg-blue-500', key: 'tcp' },
                  { label: 'TLS', ms: t.tlsHandshakeMs, color: 'bg-cyan-500', key: 'tls' },
                  { label: 'TTFB', ms: t.ttfbMs, color: 'bg-amber-500', key: 'ttfb' },
                  { label: 'Download', ms: t.downloadMs, color: 'bg-emerald-500', key: 'dl' },
                ]
                const isConnectionReused = t.dnsLookupMs === 0 && t.tcpConnectMs === 0
                return (
                  <div className="space-y-2.5">
                    {isConnectionReused && (
                      <div className="px-3 py-2 rounded-md bg-blue-50 dark:bg-blue-950/20 border border-blue-200 dark:border-blue-900/50 text-blue-700 dark:text-blue-400 text-[11px]">
                        <span className="font-semibold">DNS/TCP/TLS = 0 ms</span> — connection was reused from a previous keep-alive pool.
                        This is expected and efficient behaviour; no new handshake was needed.
                      </div>
                    )}

                    {/* Stacked waterfall bar */}
                    <div className="h-3 w-full flex rounded-full overflow-hidden border border-zinc-200 dark:border-zinc-800">
                      {phases.map((p) => (
                        <div
                          key={p.key}
                          className={`${p.color} h-full transition-all`}
                          style={{ width: `${Math.max((p.ms / total) * 100, p.ms > 0 ? 0.5 : 0)}%` }}
                          title={`${p.label}: ${p.ms.toFixed(2)} ms`}
                        />
                      ))}
                    </div>

                    {/* Phase rows */}
                    {phases.map((p) => (
                      <div key={p.key} className="flex items-center gap-2">
                        <div className={`w-2.5 h-2.5 rounded-sm shrink-0 ${p.color}`} />
                        <Tooltip content={
                          (p.key === 'dns' || p.key === 'tcp' || p.key === 'tls') && p.ms === 0
                            ? 'Connection was reused — no new handshake required'
                            : `${p.label} latency`
                        }>
                          <span className="text-zinc-600 dark:text-zinc-400 w-24 shrink-0 cursor-default">{p.label}</span>
                        </Tooltip>
                        <div className="flex-1 h-1.5 bg-zinc-200 dark:bg-zinc-900 rounded-full overflow-hidden">
                          <div
                            className={`${p.color} h-full rounded-full transition-all`}
                            style={{ width: `${Math.max((p.ms / total) * 100, p.ms > 0 ? 0.5 : 0)}%` }}
                          />
                        </div>
                        <span className="font-mono text-zinc-800 dark:text-zinc-300 w-20 text-right shrink-0">
                          {p.ms === 0 ? <span className="text-zinc-400">—</span> : `${p.ms.toFixed(2)} ms`}
                        </span>
                      </div>
                    ))}

                    <div className="border-t border-zinc-200 dark:border-zinc-800 pt-2.5 flex justify-between font-bold text-zinc-900 dark:text-zinc-200 font-mono">
                      <span>Total Duration:</span>
                      <span className="text-blue-600 dark:text-blue-400">{total.toFixed(2)} ms</span>
                    </div>

                    {/* Header/Body size breakdown */}
                    <div className="flex gap-4 pt-1 text-[11px] text-zinc-500">
                      <span>Body: <span className="font-mono text-zinc-700 dark:text-zinc-300">{formatBytes(displaySize)}</span></span>
                    </div>
                  </div>
                )
              })()}
            </div>
          </TabsContent>

          {/* ── CONSOLE TAB ── */}
          <TabsContent value="console">
            <div className="flex flex-col h-full space-y-3">
              {/* Toolbar */}
              <div className="flex flex-wrap items-center justify-between gap-2 pb-2 border-b border-zinc-200 dark:border-zinc-800">
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

              {lastResult.error && (
                <div className="p-3 rounded-md bg-rose-50 dark:bg-rose-950/20 border border-rose-200 dark:border-rose-900/50 text-rose-800 dark:text-rose-300 text-xs">
                  <div className="flex items-start gap-2">
                    <AlertCircle className="w-4 h-4 text-rose-500 shrink-0 mt-0.5" />
                    <div className="flex-1 font-mono">
                      <div className="font-semibold text-rose-900 dark:text-rose-200 mb-1">Script Execution Error</div>
                      <pre className="text-[11px] whitespace-pre-wrap break-all opacity-90">{lastResult.error}</pre>
                    </div>
                  </div>
                </div>
              )}

              <div className="space-y-1.5 overflow-y-auto">
                {filteredConsoleEntries.length > 0 ? (
                  filteredConsoleEntries.map((entry, idx) => {
                    const isErr = entry.level === 'error'
                    const isWarn = entry.level === 'warn'
                    const isInfo = entry.level === 'info'
                    return (
                      <div
                        key={idx}
                        className={`flex items-start gap-2 font-mono text-[11px] p-2 rounded border transition-colors ${
                          isErr
                            ? 'bg-rose-50/60 dark:bg-rose-950/20 border-rose-200/70 dark:border-rose-900/40 text-rose-900 dark:text-rose-300'
                            : isWarn
                            ? 'bg-amber-50/60 dark:bg-amber-950/20 border-amber-200/70 dark:border-amber-900/40 text-amber-900 dark:text-amber-300'
                            : isInfo
                            ? 'bg-blue-50/60 dark:bg-blue-950/20 border-blue-200/70 dark:border-blue-900/40 text-blue-900 dark:text-blue-300'
                            : 'bg-zinc-50 dark:bg-zinc-900/50 border-zinc-200 dark:border-zinc-800/60 text-zinc-800 dark:text-zinc-200'
                        }`}
                      >
                        {entry.timestamp && (
                          <span className="text-zinc-400 dark:text-zinc-500 text-[10px] shrink-0 select-none">
                            {entry.timestamp}
                          </span>
                        )}
                        <span
                          className={`text-[9px] uppercase px-1.5 py-0.2 rounded font-bold shrink-0 ${
                            isErr
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
                        <span className="text-[10px] text-zinc-500 dark:text-zinc-400 font-sans px-1 py-0.2 bg-zinc-200/60 dark:bg-zinc-800/60 rounded shrink-0 flex items-center gap-1">
                          <FileCode className="w-2.5 h-2.5" />
                          {entry.source}
                          {entry.line ? `:${entry.line}` : ''}
                        </span>
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
