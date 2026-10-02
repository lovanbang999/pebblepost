import { useState, useMemo } from 'react'
import { CheckCircle2, XCircle, Clock, Database, Copy, Check, Terminal } from 'lucide-react'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
import { javascript } from '@codemirror/lang-javascript'
import { xml } from '@codemirror/lang-xml'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { formatBytes, formatDuration } from '../../lib/utils'
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
  const { theme, lastResult, isExecuting } = useWorkspaceStore()
  const [activeSubTab, setActiveSubTab] = useState<'body' | 'headers' | 'tests' | 'timing' | 'console'>('body')
  const [copied, setCopied] = useState(false)

  const langExtensions = useMemo(
    () => lastResult ? detectLanguageExtension(lastResult.headers || {}) : [json()],
    [lastResult]
  )

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
            <div className="space-y-1.5">
              {lastResult.logs && lastResult.logs.length > 0 ? (
                lastResult.logs.map((log, idx) => (
                  <div
                    key={idx}
                    className="flex items-start gap-2 font-mono text-[11px] bg-zinc-100 dark:bg-zinc-900/50 border border-zinc-200 dark:border-zinc-800/50 rounded px-2.5 py-1.5"
                  >
                    <span className="text-zinc-400 dark:text-zinc-600 shrink-0 select-none">{String(idx + 1).padStart(2, '0')}</span>
                    <span className="text-zinc-800 dark:text-zinc-300 break-all">{log}</span>
                  </div>
                ))
              ) : (
                <div className="flex flex-col items-center justify-center py-8 text-zinc-500 text-xs">
                  <Terminal className="w-7 h-7 mb-2 opacity-30" />
                  <span>No console output from scripts.</span>
                  <span className="text-zinc-600 dark:text-zinc-500 mt-1">Use <code className="font-mono">console.log()</code> in your scripts to see output here.</span>
                </div>
              )}
            </div>
          </TabsContent>
        </div>
      </Tabs>
    </div>
  )
}
