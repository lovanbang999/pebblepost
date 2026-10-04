import { useState } from 'react'
import {
  Bookmark,
  Trash2,
  Copy,
  Check,
  Clock,
  Database,
  Loader2,
  Calendar,
} from 'lucide-react'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import { formatBytes, formatDuration } from '../../lib/utils'
import type { ExampleResponse } from '../../types'

interface RequestExamplesTabProps {
  filePath: string
  examples?: ExampleResponse[]
  onExamplesChange: (examples: ExampleResponse[]) => void
}

export function RequestExamplesTab({
  filePath,
  examples = [],
  onExamplesChange,
}: RequestExamplesTabProps) {
  const [selectedId, setSelectedId] = useState<string>(examples[0]?.id || '')
  const [copiedId, setCopiedId] = useState<string | null>(null)
  const [deletingId, setDeletingId] = useState<string | null>(null)
  const [activeTab, setActiveTab] = useState<'body' | 'headers'>('body')

  const selectedExample =
    examples.find((e) => e.id === selectedId) || examples[0] || null

  const handleCopyBody = (body?: string, id?: string) => {
    if (!body) return
    navigator.clipboard.writeText(body)
    if (id) {
      setCopiedId(id)
      setTimeout(() => setCopiedId(null), 2000)
    }
  }

  const handleDelete = async (exampleId: string) => {
    if (!confirm('Are you sure you want to delete this saved example?')) return

    setDeletingId(exampleId)
    try {
      const res = await fetch('/api/request/example/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ filePath, exampleId }),
      })

      if (!res.ok) {
        throw new Error('Failed to delete example')
      }

      const updated = examples.filter((e) => e.id !== exampleId)
      onExamplesChange(updated)
      if (selectedId === exampleId) {
        setSelectedId(updated[0]?.id || '')
      }
    } catch (err: unknown) {
      alert(err instanceof Error ? err.message : 'Error deleting example')
    } finally {
      setDeletingId(null)
    }
  }

  if (examples.length === 0) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center p-8 text-center bg-white dark:bg-zinc-950">
        <div className="w-12 h-12 rounded-2xl bg-blue-500/10 dark:bg-blue-500/20 text-blue-600 dark:text-blue-400 flex items-center justify-center border border-blue-500/20 mb-3">
          <Bookmark className="w-6 h-6 stroke-[1.5]" />
        </div>
        <h4 className="font-semibold text-sm text-zinc-900 dark:text-zinc-100">
          No Saved Examples Yet
        </h4>
        <p className="text-xs text-zinc-500 max-w-sm mt-1.5 leading-relaxed">
          Execute this request using <strong>Send</strong>, then click <strong>Save Example</strong> in the response header to snapshot response data.
        </p>
        <p className="text-[11px] text-zinc-400 dark:text-zinc-600 max-w-sm mt-2">
          Saved examples are stored directly in the request file with active secrets masked, and are rendered in generated documentation and OpenAPI specifications.
        </p>
      </div>
    )
  }

  return (
    <div className="flex-1 flex h-full overflow-hidden bg-white dark:bg-zinc-950">
      {/* Left List of Examples */}
      <div className="w-72 border-r border-zinc-200 dark:border-zinc-800 flex flex-col shrink-0 bg-zinc-50/50 dark:bg-zinc-900/30">
        <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between">
          <div className="flex items-center gap-2">
            <Bookmark className="w-3.5 h-3.5 text-blue-500" />
            <span className="font-semibold text-xs text-zinc-800 dark:text-zinc-200">
              Saved Examples
            </span>
          </div>
          <Badge variant="secondary" className="text-[10px] font-mono px-1.5">
            {examples.length} / 10
          </Badge>
        </div>

        <div className="flex-1 overflow-y-auto divide-y divide-zinc-200/60 dark:divide-zinc-800/60">
          {examples.map((ex) => {
            const isSelected = selectedExample?.id === ex.id
            const isSuccess = ex.statusCode >= 200 && ex.statusCode < 300
            return (
              <div
                key={ex.id}
                onClick={() => setSelectedId(ex.id)}
                className={`p-3 cursor-pointer transition-colors group flex items-start justify-between gap-2 ${
                  isSelected
                    ? 'bg-blue-50/80 dark:bg-blue-950/30 border-l-2 border-blue-600'
                    : 'hover:bg-zinc-100/70 dark:hover:bg-zinc-800/40'
                }`}
              >
                <div className="min-w-0 flex-1">
                  <div className="flex items-center gap-1.5 mb-1">
                    <Badge
                      variant={isSuccess ? 'success' : 'destructive'}
                      className="text-[10px] py-0 px-1 font-mono font-bold shrink-0"
                    >
                      {ex.statusCode}
                    </Badge>
                    <span className="font-medium text-xs text-zinc-900 dark:text-zinc-100 truncate">
                      {ex.name}
                    </span>
                  </div>
                  <div className="flex items-center gap-2 text-[10px] text-zinc-400 font-mono">
                    {ex.durationMs !== undefined && (
                      <span className="flex items-center gap-0.5">
                        <Clock className="w-2.5 h-2.5" />
                        {formatDuration(ex.durationMs)}
                      </span>
                    )}
                    {ex.size !== undefined && (
                      <span className="flex items-center gap-0.5">
                        <Database className="w-2.5 h-2.5" />
                        {formatBytes(ex.size)}
                      </span>
                    )}
                  </div>
                </div>

                <button
                  type="button"
                  title="Delete Example"
                  disabled={deletingId === ex.id}
                  onClick={(e) => {
                    e.stopPropagation()
                    handleDelete(ex.id)
                  }}
                  className="opacity-0 group-hover:opacity-100 text-zinc-400 hover:text-red-500 p-1 rounded transition-opacity cursor-pointer"
                >
                  {deletingId === ex.id ? (
                    <Loader2 className="w-3.5 h-3.5 animate-spin" />
                  ) : (
                    <Trash2 className="w-3.5 h-3.5" />
                  )}
                </button>
              </div>
            )
          })}
        </div>
      </div>

      {/* Right Example Inspector */}
      {selectedExample ? (
        <div className="flex-1 flex flex-col h-full overflow-hidden">
          {/* Top Inspector Bar */}
          <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-zinc-50/50 dark:bg-zinc-900/30 flex-wrap gap-2">
            <div className="flex items-center gap-2 flex-wrap">
              <Badge
                variant={
                  selectedExample.statusCode >= 200 && selectedExample.statusCode < 300
                    ? 'success'
                    : 'destructive'
                }
                className="text-xs py-0.5 px-2 font-mono font-bold"
              >
                {selectedExample.statusCode} {selectedExample.statusText}
              </Badge>
              <span className="font-semibold text-xs text-zinc-900 dark:text-zinc-100">
                {selectedExample.name}
              </span>
              {selectedExample.contentType && (
                <Badge variant="outline" className="text-[10px] font-mono text-zinc-500">
                  {selectedExample.contentType}
                </Badge>
              )}
            </div>

            <div className="flex items-center gap-2">
              <Button
                size="sm"
                variant="outline"
                onClick={() => handleCopyBody(selectedExample.body, selectedExample.id)}
                className="h-7 px-2.5 text-xs gap-1 cursor-pointer"
              >
                {copiedId === selectedExample.id ? (
                  <>
                    <Check className="w-3.5 h-3.5 text-emerald-500" />
                    <span>Copied</span>
                  </>
                ) : (
                  <>
                    <Copy className="w-3.5 h-3.5" />
                    <span>Copy Body</span>
                  </>
                )}
              </Button>
              <Button
                size="sm"
                variant="outline"
                disabled={deletingId === selectedExample.id}
                onClick={() => handleDelete(selectedExample.id)}
                className="h-7 px-2 text-xs text-red-600 hover:text-red-700 hover:bg-red-50 dark:hover:bg-red-950/20 cursor-pointer"
              >
                <Trash2 className="w-3.5 h-3.5" />
              </Button>
            </div>
          </div>

          {/* Sub-tabs: Body / Headers */}
          <div className="px-3 pt-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between bg-white dark:bg-zinc-950">
            <div className="flex items-center gap-1">
              <button
                type="button"
                onClick={() => setActiveTab('body')}
                className={`px-3 py-1 text-xs font-medium border-b-2 cursor-pointer transition-colors ${
                  activeTab === 'body'
                    ? 'border-blue-600 text-blue-600 dark:text-blue-400 font-semibold'
                    : 'border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'
                }`}
              >
                Body
              </button>
              <button
                type="button"
                onClick={() => setActiveTab('headers')}
                className={`px-3 py-1 text-xs font-medium border-b-2 cursor-pointer transition-colors ${
                  activeTab === 'headers'
                    ? 'border-blue-600 text-blue-600 dark:text-blue-400 font-semibold'
                    : 'border-transparent text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'
                }`}
              >
                Headers ({selectedExample.headers?.length || 0})
              </button>
            </div>

            {selectedExample.savedAt && (
              <div className="flex items-center gap-1 text-[10px] text-zinc-400 font-mono pb-1">
                <Calendar className="w-3 h-3" />
                <span>{new Date(selectedExample.savedAt).toLocaleString()}</span>
              </div>
            )}
          </div>

          {/* Tab content */}
          <div className="flex-1 overflow-y-auto p-4 bg-zinc-50/30 dark:bg-zinc-900/10">
            {activeTab === 'body' && (
              <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 overflow-hidden shadow-xs">
                <pre className="p-4 font-mono text-[11px] leading-relaxed text-zinc-800 dark:text-zinc-200 whitespace-pre-wrap break-all overflow-x-auto">
                  {selectedExample.body || '(empty body)'}
                </pre>
              </div>
            )}

            {activeTab === 'headers' && (
              <div className="rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 overflow-hidden shadow-xs">
                {selectedExample.headers && selectedExample.headers.length > 0 ? (
                  <table className="w-full text-left text-xs">
                    <thead className="bg-zinc-50 dark:bg-zinc-900 text-zinc-500 font-mono text-[11px] border-b border-zinc-200 dark:border-zinc-800">
                      <tr>
                        <th className="px-3 py-2">Header Name</th>
                        <th className="px-3 py-2">Header Value</th>
                      </tr>
                    </thead>
                    <tbody className="divide-y divide-zinc-200 dark:divide-zinc-800 font-mono text-[11px]">
                      {selectedExample.headers.map((h, i) => (
                        <tr key={i} className="hover:bg-zinc-50/50 dark:hover:bg-zinc-900/50">
                          <td className="px-3 py-2 font-medium text-zinc-700 dark:text-zinc-300">
                            {h.key}
                          </td>
                          <td className="px-3 py-2 text-zinc-600 dark:text-zinc-400 break-all">
                            {h.value}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                ) : (
                  <div className="p-6 text-center text-xs text-zinc-400">
                    No headers recorded for this example.
                  </div>
                )}
              </div>
            )}
          </div>
        </div>
      ) : null}
    </div>
  )
}
