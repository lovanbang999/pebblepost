import React, { useState } from 'react'
import { CheckCircle2, XCircle, Clock, Database, Copy, Check } from 'lucide-react'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
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

export const ResponsePanel: React.FC = () => {
  const { lastResult, isExecuting } = useWorkspaceStore()
  const [activeSubTab, setActiveSubTab] = useState<'body' | 'headers' | 'tests' | 'timing'>('body')
  const [copied, setCopied] = useState(false)

  if (isExecuting) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-zinc-950 text-zinc-400 gap-3 border-l border-zinc-800">
        <div className="w-6 h-6 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        <span className="text-xs font-mono">Executing request & network timing trace...</span>
      </div>
    )
  }

  if (!lastResult) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-zinc-950 text-zinc-500 text-xs gap-1 border-l border-zinc-800 p-6 text-center">
        <Clock className="w-8 h-8 text-zinc-700 stroke-[1.5] mb-2" />
        <span className="font-semibold text-zinc-300 text-sm">No Response Yet</span>
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
    <div className="flex-1 flex flex-col h-full bg-zinc-950 border-l border-zinc-800 overflow-hidden">
      {/* Response Status Bar */}
      <div className="p-3 border-b border-zinc-800 flex items-center justify-between">
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
            <div className="flex items-center gap-1.5 text-xs text-zinc-300 font-mono bg-zinc-900 border border-zinc-800 px-2 py-1 rounded-md">
              <Clock className="w-3 h-3 text-zinc-400" />
              <span>{formatDuration(lastResult.timing.totalDurationMs)}</span>
            </div>
          </Tooltip>

          {/* Size Badge */}
          <Tooltip content="Response Payload Size">
            <div className="flex items-center gap-1.5 text-xs text-zinc-300 font-mono bg-zinc-900 border border-zinc-800 px-2 py-1 rounded-md">
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
            className="h-7 gap-1 px-2.5 text-xs text-zinc-300 hover:text-white"
          >
            {copied ? (
              <Check className="w-3.5 h-3.5 text-emerald-400" />
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
          {(['body', 'headers', 'tests', 'timing'] as const).map((tab) => (
            <TabsTrigger key={tab} value={tab} className="capitalize">
              {tab}
              {tab === 'tests' && lastResult.tests && lastResult.tests.length > 0 && (
                <Badge
                  variant={lastResult.tests.every((t) => t.passed) ? 'success' : 'destructive'}
                  className="ml-1.5 px-1 py-0 text-[9px]"
                >
                  {lastResult.tests.filter((t) => t.passed).length}/{lastResult.tests.length}
                </Badge>
              )}
            </TabsTrigger>
          ))}
        </TabsList>

        {/* Tab Content Panels */}
        <div className="flex-1 overflow-y-auto p-3">
          <TabsContent value="body">
            <div className="h-full rounded-md border border-zinc-800 overflow-hidden bg-zinc-950">
              <CodeMirror
                value={lastResult.body}
                height="100%"
                extensions={[json()]}
                theme="dark"
                readOnly
                className="text-xs font-mono"
              />
            </div>
          </TabsContent>

          <TabsContent value="headers">
            <div className="border border-zinc-800 rounded-md overflow-hidden bg-zinc-950">
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
                      <TableCell className="text-zinc-300 font-semibold">{key}</TableCell>
                      <TableCell className="text-zinc-400">{vals.join(', ')}</TableCell>
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
                        ? 'bg-emerald-950/20 border-emerald-900/50 text-emerald-300'
                        : 'bg-rose-950/20 border-rose-900/50 text-rose-300'
                    }`}
                  >
                    {test.passed ? (
                      <CheckCircle2 className="w-4 h-4 text-emerald-400 shrink-0 mt-0.5" />
                    ) : (
                      <XCircle className="w-4 h-4 text-rose-400 shrink-0 mt-0.5" />
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
            <div className="space-y-3 text-xs">
              <span className="font-semibold text-zinc-300">Network Timing Breakdown</span>
              <div className="border border-zinc-800 rounded-md p-3.5 space-y-2.5 bg-zinc-900/40">
                <div className="flex justify-between text-zinc-400 font-mono">
                  <span>DNS Lookup:</span>
                  <span>{lastResult.timing.dnsLookupMs.toFixed(2)} ms</span>
                </div>
                <div className="flex justify-between text-zinc-400 font-mono">
                  <span>TCP Connection:</span>
                  <span>{lastResult.timing.tcpConnectMs.toFixed(2)} ms</span>
                </div>
                <div className="flex justify-between text-zinc-400 font-mono">
                  <span>TLS Handshake:</span>
                  <span>{lastResult.timing.tlsHandshakeMs.toFixed(2)} ms</span>
                </div>
                <div className="flex justify-between text-zinc-400 font-mono">
                  <span>Time To First Byte (TTFB):</span>
                  <span>{lastResult.timing.ttfbMs.toFixed(2)} ms</span>
                </div>
                <div className="flex justify-between text-zinc-400 font-mono">
                  <span>Content Download:</span>
                  <span>{lastResult.timing.downloadMs.toFixed(2)} ms</span>
                </div>
                <div className="border-t border-zinc-800 pt-2.5 flex justify-between font-bold text-zinc-200 font-mono">
                  <span>Total Duration:</span>
                  <span className="text-blue-400">{lastResult.timing.totalDurationMs.toFixed(2)} ms</span>
                </div>
              </div>
            </div>
          </TabsContent>
        </div>
      </Tabs>
    </div>
  )
}
