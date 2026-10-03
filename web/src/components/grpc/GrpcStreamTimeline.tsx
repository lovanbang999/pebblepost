import { useState, useMemo } from 'react'
import {
  ArrowUpRight,
  ArrowDownLeft,
  Copy,
  Check,
  AlertCircle,
  Search,
} from 'lucide-react'
import type { GrpcStreamMessage } from '../../types'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'

interface GrpcStreamTimelineProps {
  messages: GrpcStreamMessage[]
  isLive?: boolean
}

export function GrpcStreamTimeline({ messages, isLive }: GrpcStreamTimelineProps) {
  const [filterDirection, setFilterDirection] = useState<'all' | 'send' | 'receive'>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const [copiedIndex, setCopiedIndex] = useState<number | null>(null)

  const filtered = useMemo(() => {
    return (messages || []).filter((msg) => {
      if (filterDirection !== 'all' && msg.direction !== filterDirection) {
        return false
      }
      if (searchQuery.trim()) {
        const q = searchQuery.toLowerCase()
        if (!msg.payload.toLowerCase().includes(q)) {
          return false
        }
      }
      return true
    })
  }, [messages, filterDirection, searchQuery])

  const handleCopy = (payload: string, index: number) => {
    navigator.clipboard.writeText(payload)
    setCopiedIndex(index)
    setTimeout(() => setCopiedIndex(null), 2000)
  }

  const sendCount = useMemo(() => messages.filter((m) => m.direction === 'send').length, [messages])
  const recvCount = useMemo(() => messages.filter((m) => m.direction === 'receive').length, [messages])

  return (
    <div className="flex flex-col h-full overflow-hidden bg-white dark:bg-zinc-950">
      {/* Filter bar */}
      <div className="p-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/40 flex items-center justify-between gap-2 flex-wrap text-xs">
        <div className="flex items-center gap-1.5">
          <button
            onClick={() => setFilterDirection('all')}
            className={`px-2 py-0.5 rounded font-semibold transition-colors ${
              filterDirection === 'all'
                ? 'bg-zinc-200 dark:bg-zinc-700 text-zinc-900 dark:text-zinc-100'
                : 'text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300'
            }`}
          >
            All ({messages.length})
          </button>
          <button
            onClick={() => setFilterDirection('send')}
            className={`px-2 py-0.5 rounded font-semibold transition-colors flex items-center gap-1 ${
              filterDirection === 'send'
                ? 'bg-blue-100 dark:bg-blue-950/80 text-blue-700 dark:text-blue-300'
                : 'text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300'
            }`}
          >
            <ArrowUpRight className="w-3 h-3 text-blue-500" />
            Sent ({sendCount})
          </button>
          <button
            onClick={() => setFilterDirection('receive')}
            className={`px-2 py-0.5 rounded font-semibold transition-colors flex items-center gap-1 ${
              filterDirection === 'receive'
                ? 'bg-purple-100 dark:bg-purple-950/80 text-purple-700 dark:text-purple-300'
                : 'text-zinc-500 hover:text-zinc-700 dark:hover:text-zinc-300'
            }`}
          >
            <ArrowDownLeft className="w-3 h-3 text-purple-500" />
            Received ({recvCount})
          </button>
        </div>

        <div className="flex items-center gap-2">
          {isLive && (
            <Badge variant="outline" className="h-5 px-1.5 text-[10px] border-emerald-500 text-emerald-600 bg-emerald-50 dark:bg-emerald-950/40 animate-pulse">
              ● Live Stream
            </Badge>
          )}
          <div className="relative w-40">
            <Search className="w-3.5 h-3.5 absolute left-2 top-2 text-zinc-400" />
            <Input
              value={searchQuery}
              onChange={(e) => setSearchQuery(e.target.value)}
              placeholder="Search stream..."
              className="h-7 text-xs pl-7"
            />
          </div>
        </div>
      </div>

      {/* Message list */}
      <div className="flex-1 overflow-y-auto p-3 space-y-2.5">
        {filtered.length === 0 && (
          <div className="text-center py-12 text-zinc-400 text-xs">
            {messages.length === 0 ? 'No stream messages exchanged yet.' : 'No messages matching the filter.'}
          </div>
        )}

        {filtered.map((msg, i) => {
          const isSend = msg.direction === 'send'
          return (
            <div
              key={i}
              className={`border rounded-md overflow-hidden transition-shadow ${
                isSend
                  ? 'border-blue-200 dark:border-blue-900/60 bg-blue-50/20 dark:bg-blue-950/10'
                  : 'border-purple-200 dark:border-purple-900/60 bg-purple-50/20 dark:bg-purple-950/10'
              }`}
            >
              <div className="px-3 py-1.5 flex items-center justify-between border-b border-zinc-200/80 dark:border-zinc-800/80 text-xs bg-zinc-50/40 dark:bg-zinc-900/30">
                <div className="flex items-center gap-2">
                  <span
                    className={`inline-flex items-center gap-1 font-bold text-[10px] px-1.5 py-0.5 rounded uppercase tracking-wider ${
                      isSend
                        ? 'bg-blue-100 dark:bg-blue-900/50 text-blue-700 dark:text-blue-300'
                        : 'bg-purple-100 dark:bg-purple-900/50 text-purple-700 dark:text-purple-300'
                    }`}
                  >
                    {isSend ? (
                      <>
                        <ArrowUpRight className="w-3 h-3" /> Sent
                      </>
                    ) : (
                      <>
                        <ArrowDownLeft className="w-3 h-3" /> Received
                      </>
                    )}
                  </span>
                  <span className="text-[11px] font-mono text-zinc-400">#{msg.index + 1}</span>
                  {msg.timestamp && (
                    <span className="text-[10px] text-zinc-400 font-mono">
                      {new Date(msg.timestamp).toLocaleTimeString([], {
                        hour12: false,
                        hour: '2-digit',
                        minute: '2-digit',
                        second: '2-digit',
                        fractionalSecondDigits: 3,
                      } as Intl.DateTimeFormatOptions)}
                    </span>
                  )}
                </div>

                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => handleCopy(msg.payload, i)}
                  className="h-6 px-1.5 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 text-xs"
                >
                  {copiedIndex === i ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
                </Button>
              </div>

              <div className="p-2.5 overflow-x-auto text-xs font-mono bg-white dark:bg-zinc-950">
                {msg.isError ? (
                  <div className="text-rose-600 dark:text-rose-400 flex items-center gap-1.5">
                    <AlertCircle className="w-3.5 h-3.5 shrink-0" />
                    <span>{msg.payload}</span>
                  </div>
                ) : (
                  <pre className="whitespace-pre-wrap break-all text-zinc-800 dark:text-zinc-200">
                    {msg.payload}
                  </pre>
                )}
              </div>
            </div>
          )
        })}
      </div>
    </div>
  )
}
