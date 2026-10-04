import { useState, useEffect, useMemo, useCallback } from 'react'
import {
  BookOpen,
  Search,
  Download,
  Copy,
  Check,
  ChevronDown,
  ChevronRight,
  RefreshCw,
} from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { type RequestTab } from '../../store/tabStore'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { cn, getMethodTextColor } from '../../lib/utils'
import type { KeyValue, ExampleResponse } from '../../types'

interface DocsPanelProps {
  currentTab: RequestTab
}

interface DocRequestItem {
  id: string
  name: string
  description?: string
  method: string
  url: string
  protocol?: string
  relPath: string
  headers?: KeyValue[]
  params?: KeyValue[]
  body?: {
    type: string
    raw?: string
    urlEncoded?: KeyValue[]
    formData?: KeyValue[]
  }
  examples?: ExampleResponse[]
  snippets: {
    curl: string
    go: string
    node: string
    python: string
    csharp: string
  }
}

interface DocFolderItem {
  name: string
  description?: string
  relPath: string
  order: number
  items: DocHierarchyItem[]
}

interface DocHierarchyItem {
  isFolder: boolean
  folder?: DocFolderItem
  request?: DocRequestItem
}

interface DocCollectionData {
  title: string
  description?: string
  version?: string
  items: DocHierarchyItem[]
}

type ViewMode = 'preview' | 'md' | 'html' | 'openapi'

export function DocsPanel({ currentTab }: DocsPanelProps) {
  const { workspacePath } = useWorkspaceStore()
  const targetFolder = currentTab.docsConfig?.folderPath || ''
  const folderTitle =
    currentTab.docsConfig?.folderName ||
    (targetFolder ? targetFolder.split('/').pop() : 'All Collections')

  const [loading, setLoading] = useState(true)
  const [error, setError] = useState<string | null>(null)
  const [collectionData, setCollectionData] = useState<DocCollectionData | null>(null)
  const [searchFilter, setSearchFilter] = useState('')
  const [viewMode, setViewMode] = useState<ViewMode>('preview')
  const [rawContent, setRawContent] = useState<string>('')
  const [loadingRaw, setLoadingRaw] = useState(false)
  const [selectedLanguage, setSelectedLanguage] = useState<'curl' | 'go' | 'node' | 'python' | 'csharp'>('curl')
  const [copiedKey, setCopiedKey] = useState<string | null>(null)
  const [expandedExamples, setExpandedExamples] = useState<Record<string, boolean>>({})

  // Fetch structured doc data
  const fetchDocData = useCallback(async () => {
    setLoading(true)
    setError(null)
    try {
      const q = new URLSearchParams({
        workspacePath: workspacePath || '.',
        path: targetFolder || '.',
        format: 'data',
      })
      const res = await fetch(`/api/docs?${q.toString()}`)
      if (!res.ok) {
        const err = await res.json()
        throw new Error(err.error || 'Failed to load documentation data')
      }
      const data = await res.json()
      setCollectionData(data)
    } catch (err: unknown) {
      setError(err instanceof Error ? err.message : String(err))
    } finally {
      setLoading(false)
    }
  }, [workspacePath, targetFolder])

  useEffect(() => {
    fetchDocData()
  }, [fetchDocData])

  // Fetch raw format content when viewMode changes
  useEffect(() => {
    if (viewMode === 'preview') return

    const fetchRaw = async () => {
      setLoadingRaw(true)
      try {
        const q = new URLSearchParams({
          workspacePath: workspacePath || '.',
          path: targetFolder || '.',
          format: viewMode,
        })
        const res = await fetch(`/api/docs?${q.toString()}`)
        if (!res.ok) {
          const err = await res.json()
          throw new Error(err.error || 'Failed to generate raw documentation')
        }
        const data = await res.json()
        setRawContent(data.content || '')
      } catch (err: unknown) {
        setRawContent(`Error: ${err instanceof Error ? err.message : String(err)}`)
      } finally {
        setLoadingRaw(false)
      }
    }
    fetchRaw()
  }, [viewMode, workspacePath, targetFolder])

  // Copy helper
  const handleCopy = (text: string, key: string) => {
    navigator.clipboard.writeText(text)
    setCopiedKey(key)
    setTimeout(() => setCopiedKey(null), 1500)
  }

  // Export helper
  const handleExport = async (format: 'md' | 'html' | 'openapi') => {
    try {
      const q = new URLSearchParams({
        workspacePath: workspacePath || '.',
        path: targetFolder || '.',
        format,
      })
      const res = await fetch(`/api/docs?${q.toString()}`)
      if (!res.ok) {
        const err = await res.json()
        alert(`Export failed: ${err.error || 'Unknown error'}`)
        return
      }
      const data = await res.json()
      const mime =
        format === 'html'
          ? 'text/html'
          : format === 'md'
          ? 'text/markdown'
          : 'application/x-yaml'
      const blob = new Blob([data.content], { type: mime })
      const url = URL.createObjectURL(blob)
      const a = document.createElement('a')
      a.href = url
      a.download = data.filename || `documentation.${format}`
      a.click()
      URL.revokeObjectURL(url)
    } catch (err: unknown) {
      alert(`Export error: ${err instanceof Error ? err.message : String(err)}`)
    }
  }

  // Flattened list of requests for navigation and search
  const flatRequests = useMemo(() => {
    const list: DocRequestItem[] = []
    const traverse = (items: DocHierarchyItem[] | undefined) => {
      if (!items) return
      for (const item of items) {
        if (item.isFolder && item.folder) {
          traverse(item.folder.items)
        } else if (!item.isFolder && item.request) {
          list.push(item.request)
        }
      }
    }
    if (collectionData) {
      traverse(collectionData.items)
    }
    return list
  }, [collectionData])

  const filteredRequests = useMemo(() => {
    if (!searchFilter.trim()) return flatRequests
    const q = searchFilter.toLowerCase()
    return flatRequests.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        r.url.toLowerCase().includes(q) ||
        r.method.toLowerCase().includes(q) ||
        (r.description && r.description.toLowerCase().includes(q))
    )
  }, [flatRequests, searchFilter])

  // Simple Markdown renderer
  const renderSimpleMarkdown = (text: string | undefined) => {
    if (!text) return null
    const lines = text.split('\n')
    return (
      <div className="space-y-1.5 text-xs text-zinc-600 dark:text-zinc-300 leading-relaxed">
        {lines.map((line, idx) => {
          if (line.startsWith('### ')) {
            return (
              <h4 key={idx} className="font-bold text-sm text-zinc-900 dark:text-zinc-100 mt-2">
                {line.slice(4)}
              </h4>
            )
          }
          if (line.startsWith('## ')) {
            return (
              <h3 key={idx} className="font-bold text-base text-zinc-900 dark:text-zinc-100 mt-3">
                {line.slice(3)}
              </h3>
            )
          }
          if (line.startsWith('# ')) {
            return (
              <h2 key={idx} className="font-bold text-lg text-zinc-900 dark:text-zinc-100 mt-3">
                {line.slice(2)}
              </h2>
            )
          }
          if (line.startsWith('- ') || line.startsWith('* ')) {
            return (
              <li key={idx} className="ml-4 list-disc">
                {line.slice(2)}
              </li>
            )
          }
          if (!line.trim()) {
            return <div key={idx} className="h-1" />
          }
          return <p key={idx}>{line}</p>
        })}
      </div>
    )
  }

  return (
    <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 text-zinc-900 dark:text-zinc-100 overflow-hidden">
      {/* Header Toolbar */}
      <div className="h-12 border-b border-zinc-200 dark:border-zinc-800 px-4 flex items-center justify-between gap-3 bg-zinc-50/50 dark:bg-zinc-900/40 shrink-0">
        <div className="flex items-center gap-2.5 min-w-0">
          <BookOpen className="w-4 h-4 text-blue-500 shrink-0" />
          <div className="flex items-center gap-2 min-w-0">
            <h1 className="text-sm font-semibold text-zinc-900 dark:text-zinc-100 truncate">
              {collectionData?.title || folderTitle}
            </h1>
            <Badge variant="outline" className="text-[10px] font-mono border-zinc-200 dark:border-zinc-800">
              Docs
            </Badge>
            {collectionData?.version && (
              <Badge variant="secondary" className="text-[10px] font-mono">
                v{collectionData.version}
              </Badge>
            )}
          </div>
        </div>

        {/* View mode switcher & Export actions */}
        <div className="flex items-center gap-2">
          <div className="flex items-center bg-zinc-200/60 dark:bg-zinc-800/60 p-0.5 rounded-lg text-xs font-medium">
            <button
              onClick={() => setViewMode('preview')}
              className={cn(
                'px-2.5 py-1 rounded-md transition-colors cursor-pointer',
                viewMode === 'preview'
                  ? 'bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-xs'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100'
              )}
            >
              Interactive Preview
            </button>
            <button
              onClick={() => setViewMode('md')}
              className={cn(
                'px-2.5 py-1 rounded-md transition-colors cursor-pointer',
                viewMode === 'md'
                  ? 'bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-xs'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100'
              )}
            >
              Markdown
            </button>
            <button
              onClick={() => setViewMode('html')}
              className={cn(
                'px-2.5 py-1 rounded-md transition-colors cursor-pointer',
                viewMode === 'html'
                  ? 'bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-xs'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100'
              )}
            >
              HTML
            </button>
            <button
              onClick={() => setViewMode('openapi')}
              className={cn(
                'px-2.5 py-1 rounded-md transition-colors cursor-pointer',
                viewMode === 'openapi'
                  ? 'bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 shadow-xs'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100'
              )}
            >
              OpenAPI
            </button>
          </div>

          <Button
            variant="outline"
            size="sm"
            onClick={fetchDocData}
            title="Refresh Documentation"
            className="h-7 w-7 p-0"
          >
            <RefreshCw className={cn('w-3.5 h-3.5', loading && 'animate-spin')} />
          </Button>

          <Button
            variant="outline"
            size="sm"
            onClick={() => handleExport(viewMode === 'preview' ? 'html' : viewMode)}
            className="h-7 text-xs gap-1.5 bg-white dark:bg-zinc-900"
          >
            <Download className="w-3.5 h-3.5" />
            <span>Export {viewMode.toUpperCase()}</span>
          </Button>
        </div>
      </div>

      {/* Main Dual-Column Content */}
      <div className="flex-1 flex overflow-hidden">
        {/* Left Column: TOC / Navigation */}
        <div className="w-72 border-r border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/30 flex flex-col shrink-0">
          <div className="p-2.5 border-b border-zinc-200 dark:border-zinc-800">
            <div className="relative">
              <Search className="w-3.5 h-3.5 absolute left-2.5 top-2.5 text-zinc-400 pointer-events-none" />
              <Input
                placeholder="Filter endpoints..."
                value={searchFilter}
                onChange={(e) => setSearchFilter(e.target.value)}
                className="pl-8 h-7 text-xs bg-white dark:bg-zinc-950"
              />
            </div>
          </div>

          <div className="flex-1 overflow-y-auto p-2 space-y-0.5">
            {filteredRequests.length === 0 ? (
              <div className="text-center py-6 text-xs text-zinc-400">
                {loading ? 'Loading endpoints...' : 'No endpoints found.'}
              </div>
            ) : (
              filteredRequests.map((req) => (
                <a
                  key={req.id || req.relPath}
                  href={`#endpoint-${req.id || req.relPath}`}
                  className="flex items-center gap-2 px-2 py-1.5 rounded-md hover:bg-zinc-200/50 dark:hover:bg-zinc-800/50 text-xs transition-colors group"
                >
                  <span
                    className={cn(
                      'text-[9px] font-mono font-bold tracking-tight uppercase w-10 shrink-0',
                      getMethodTextColor(req.method)
                    )}
                  >
                    {req.method}
                  </span>
                  <span className="truncate text-zinc-700 dark:text-zinc-300 group-hover:text-zinc-900 dark:group-hover:text-zinc-100">
                    {req.name}
                  </span>
                </a>
              ))
            )}
          </div>
        </div>

        {/* Right Column: Content Viewer */}
        <div className="flex-1 overflow-y-auto">
          {viewMode !== 'preview' ? (
            /* Raw Code / Spec View */
            <div className="p-6 relative max-w-4xl mx-auto">
              <div className="flex justify-between items-center mb-3">
                <span className="text-xs font-semibold text-zinc-500 uppercase tracking-wider font-mono">
                  Generated {viewMode.toUpperCase()} Output
                </span>
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => handleCopy(rawContent, 'raw-output')}
                  className="h-7 text-xs gap-1.5"
                >
                  {copiedKey === 'raw-output' ? (
                    <>
                      <Check className="w-3.5 h-3.5 text-emerald-500" />
                      <span>Copied!</span>
                    </>
                  ) : (
                    <>
                      <Copy className="w-3.5 h-3.5" />
                      <span>Copy Output</span>
                    </>
                  )}
                </Button>
              </div>

              {loadingRaw ? (
                <div className="p-8 text-center text-xs text-zinc-400">Generating output...</div>
              ) : (
                <pre className="p-4 rounded-lg bg-zinc-950 text-zinc-100 font-mono text-xs overflow-x-auto leading-relaxed border border-zinc-800">
                  <code>{rawContent}</code>
                </pre>
              )}
            </div>
          ) : (
            /* Interactive Rich Preview */
            <div className="p-8 max-w-4xl mx-auto space-y-10">
              {/* Collection Header Intro */}
              <div className="border-b border-zinc-200 dark:border-zinc-800 pb-6">
                <h1 className="text-2xl font-bold text-zinc-900 dark:text-zinc-100 tracking-tight">
                  {collectionData?.title || 'API Reference'}
                </h1>
                {collectionData?.description && (
                  <div className="mt-2">{renderSimpleMarkdown(collectionData.description)}</div>
                )}
              </div>

              {error && (
                <div className="p-4 rounded-lg bg-rose-500/10 border border-rose-500/20 text-rose-600 dark:text-rose-400 text-xs">
                  {error}
                </div>
              )}

              {/* Endpoints List */}
              {flatRequests.map((req) => (
                <div
                  key={req.id || req.relPath}
                  id={`endpoint-${req.id || req.relPath}`}
                  className="space-y-4 pt-4 border-b border-zinc-100 dark:border-zinc-900 pb-10"
                >
                  {/* Endpoint Header */}
                  <div className="space-y-1.5">
                    <div className="flex items-center gap-2.5">
                      <Badge
                        variant="outline"
                        className={cn(
                          'text-xs font-mono font-bold uppercase px-2 py-0.5',
                          getMethodTextColor(req.method)
                        )}
                      >
                        {req.method}
                      </Badge>
                      <h2 className="text-lg font-bold text-zinc-900 dark:text-zinc-100">
                        {req.name}
                      </h2>
                    </div>

                    {req.description && (
                      <div className="pt-1">{renderSimpleMarkdown(req.description)}</div>
                    )}
                  </div>

                  {/* URL badge */}
                  <div className="flex items-center justify-between p-2.5 rounded-lg bg-zinc-100 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 font-mono text-xs text-zinc-800 dark:text-zinc-200">
                    <span className="truncate">{req.url}</span>
                    <button
                      onClick={() => handleCopy(req.url, `url-${req.id}`)}
                      className="text-zinc-400 hover:text-zinc-600 dark:hover:text-zinc-200 ml-2 cursor-pointer"
                      title="Copy URL"
                    >
                      {copiedKey === `url-${req.id}` ? (
                        <Check className="w-3.5 h-3.5 text-emerald-500" />
                      ) : (
                        <Copy className="w-3.5 h-3.5" />
                      )}
                    </button>
                  </div>

                  {/* Query Parameters Table */}
                  {req.params && req.params.filter((p) => p.enabled && p.key).length > 0 && (
                    <div className="space-y-1.5">
                      <h4 className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                        Query Parameters
                      </h4>
                      <table className="w-full border-collapse text-xs border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden">
                        <thead>
                          <tr className="bg-zinc-50 dark:bg-zinc-900/60 border-b border-zinc-200 dark:border-zinc-800 text-left text-zinc-500">
                            <th className="p-2 font-mono">Parameter</th>
                            <th className="p-2">Example Value</th>
                          </tr>
                        </thead>
                        <tbody>
                          {req.params
                            .filter((p) => p.enabled && p.key)
                            .map((p) => (
                              <tr
                                key={p.key}
                                className="border-b border-zinc-100 dark:border-zinc-900"
                              >
                                <td className="p-2 font-mono font-medium text-zinc-900 dark:text-zinc-100">
                                  {p.key}
                                </td>
                                <td className="p-2 font-mono text-zinc-500">{p.value}</td>
                              </tr>
                            ))}
                        </tbody>
                      </table>
                    </div>
                  )}

                  {/* Headers Table */}
                  {req.headers && req.headers.filter((h) => h.enabled && h.key).length > 0 && (
                    <div className="space-y-1.5">
                      <h4 className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                        Headers
                      </h4>
                      <table className="w-full border-collapse text-xs border border-zinc-200 dark:border-zinc-800 rounded-lg overflow-hidden">
                        <thead>
                          <tr className="bg-zinc-50 dark:bg-zinc-900/60 border-b border-zinc-200 dark:border-zinc-800 text-left text-zinc-500">
                            <th className="p-2 font-mono">Header</th>
                            <th className="p-2">Value</th>
                          </tr>
                        </thead>
                        <tbody>
                          {req.headers
                            .filter((h) => h.enabled && h.key)
                            .map((h) => (
                              <tr
                                key={h.key}
                                className="border-b border-zinc-100 dark:border-zinc-900"
                              >
                                <td className="p-2 font-mono font-medium text-zinc-900 dark:text-zinc-100">
                                  {h.key}
                                </td>
                                <td className="p-2 font-mono text-zinc-500">{h.value}</td>
                              </tr>
                            ))}
                        </tbody>
                      </table>
                    </div>
                  )}

                  {/* Request Body */}
                  {req.body && (req.body.type === 'json' || req.body.type === 'raw') && req.body.raw && (
                    <div className="space-y-1.5">
                      <h4 className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                        Sample Request Body
                      </h4>
                      <div className="rounded-lg bg-zinc-950 p-3 font-mono text-xs text-zinc-200 border border-zinc-800 overflow-x-auto">
                        <pre>{req.body.raw}</pre>
                      </div>
                    </div>
                  )}

                  {/* Code Samples Tabs */}
                  <div className="space-y-2 pt-2">
                    <div className="flex items-center justify-between">
                      <h4 className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                        Client Code Samples
                      </h4>
                      <div className="flex items-center gap-1 bg-zinc-100 dark:bg-zinc-900 p-0.5 rounded-md text-[11px]">
                        {(['curl', 'go', 'node', 'python', 'csharp'] as const).map((lang) => (
                          <button
                            key={lang}
                            onClick={() => setSelectedLanguage(lang)}
                            className={cn(
                              'px-2 py-0.5 rounded font-medium transition-colors cursor-pointer',
                              selectedLanguage === lang
                                ? 'bg-white dark:bg-zinc-800 text-zinc-900 dark:text-zinc-100 shadow-xs'
                                : 'text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100'
                            )}
                          >
                            {lang === 'curl'
                              ? 'cURL'
                              : lang === 'node'
                              ? 'Node.js'
                              : lang === 'csharp'
                              ? 'C#'
                              : lang.toUpperCase()}
                          </button>
                        ))}
                      </div>
                    </div>

                    <div className="relative rounded-lg bg-zinc-950 border border-zinc-800 p-3 group">
                      <button
                        onClick={() =>
                          handleCopy(
                            req.snippets[selectedLanguage],
                            `snippet-${req.id}-${selectedLanguage}`
                          )
                        }
                        className="absolute right-2.5 top-2.5 p-1 rounded bg-zinc-800/80 hover:bg-zinc-700 text-zinc-300 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer text-xs flex items-center gap-1"
                        title="Copy code"
                      >
                        {copiedKey === `snippet-${req.id}-${selectedLanguage}` ? (
                          <>
                            <Check className="w-3 h-3 text-emerald-400" />
                            <span className="text-[10px]">Copied</span>
                          </>
                        ) : (
                          <>
                            <Copy className="w-3 h-3" />
                            <span className="text-[10px]">Copy</span>
                          </>
                        )}
                      </button>
                      <pre className="text-xs font-mono text-zinc-200 overflow-x-auto leading-relaxed">
                        <code>{req.snippets[selectedLanguage]}</code>
                      </pre>
                    </div>
                  </div>

                  {/* Saved Response Examples */}
                  {req.examples && req.examples.length > 0 && (
                    <div className="space-y-3 pt-3">
                      <h4 className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                        Saved Response Examples ({req.examples.length})
                      </h4>
                      <div className="space-y-2">
                        {req.examples.map((ex, exIdx) => {
                          const isExpanded =
                            expandedExamples[`${req.id || req.relPath}_${ex.id || exIdx}`] ?? true
                          return (
                            <div
                              key={ex.id || exIdx}
                              className="rounded-lg border border-zinc-200 dark:border-zinc-800 overflow-hidden text-xs"
                            >
                              <div
                                onClick={() =>
                                  setExpandedExamples((prev) => ({
                                    ...prev,
                                    [`${req.id || req.relPath}_${ex.id || exIdx}`]: !isExpanded,
                                  }))
                                }
                                className="flex items-center justify-between p-2.5 bg-zinc-50/80 dark:bg-zinc-900/60 cursor-pointer select-none hover:bg-zinc-100 dark:hover:bg-zinc-900"
                              >
                                <div className="flex items-center gap-2">
                                  {isExpanded ? (
                                    <ChevronDown className="w-3.5 h-3.5 text-zinc-400" />
                                  ) : (
                                    <ChevronRight className="w-3.5 h-3.5 text-zinc-400" />
                                  )}
                                  <Badge
                                    variant="outline"
                                    className={cn(
                                      'text-[10px] font-mono font-semibold',
                                      ex.statusCode >= 200 && ex.statusCode < 300
                                        ? 'text-emerald-600 border-emerald-500/30'
                                        : 'text-rose-600 border-rose-500/30'
                                    )}
                                  >
                                    {ex.statusCode} {ex.statusText}
                                  </Badge>
                                  <span className="font-semibold text-zinc-800 dark:text-zinc-200">
                                    {ex.name}
                                  </span>
                                </div>

                                <span className="text-[10px] text-zinc-400 font-mono">
                                  {ex.durationMs ? `${ex.durationMs}ms` : ''}
                                </span>
                              </div>

                              {isExpanded && (
                                <div className="p-3 bg-zinc-950 border-t border-zinc-800 text-zinc-200 font-mono overflow-x-auto relative group">
                                  <button
                                    onClick={() => handleCopy(ex.body || '', `example-${ex.id}`)}
                                    className="absolute right-2.5 top-2.5 p-1 rounded bg-zinc-800/80 hover:bg-zinc-700 text-zinc-300 opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer text-xs flex items-center gap-1"
                                  >
                                    {copiedKey === `example-${ex.id}` ? (
                                      <>
                                        <Check className="w-3 h-3 text-emerald-400" />
                                        <span className="text-[10px]">Copied</span>
                                      </>
                                    ) : (
                                      <>
                                        <Copy className="w-3 h-3" />
                                        <span className="text-[10px]">Copy</span>
                                      </>
                                    )}
                                  </button>
                                  <pre className="text-xs">
                                    <code>{ex.body || '// Empty Body'}</code>
                                  </pre>
                                </div>
                              )}
                            </div>
                          )
                        })}
                      </div>
                    </div>
                  )}
                </div>
              ))}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
