import React, { useState } from 'react'
import { CheckCircle2, XCircle, Clock, Database, Copy, Check } from 'lucide-react'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { formatBytes, formatDuration } from '../../lib/utils'

export const ResponsePanel: React.FC = () => {
  const { lastResult, isExecuting } = useWorkspaceStore()
  const [activeSubTab, setActiveSubTab] = useState<'body' | 'headers' | 'tests' | 'timing'>('body')
  const [copied, setCopied] = useState(false)

  if (isExecuting) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-zinc-950/60 text-zinc-400 gap-3">
        <div className="w-6 h-6 border-2 border-blue-500 border-t-transparent rounded-full animate-spin" />
        <span className="text-xs font-mono">Executing request & network timing trace...</span>
      </div>
    )
  }

  if (!lastResult) {
    return (
      <div className="flex-1 flex flex-col items-center justify-center bg-zinc-950/40 text-zinc-500 text-xs gap-1 border-l border-zinc-850">
        <Clock className="w-8 h-8 text-zinc-700 stroke-[1.5] mb-2" />
        <span className="font-medium text-zinc-400">No Response Yet</span>
        <span>Click "Send" to execute the request and view response payload & timing metrics.</span>
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
    <div className="flex-1 flex flex-col h-full bg-zinc-950 border-l border-zinc-850 overflow-hidden">
      {/* Response Status Bar */}
      <div className="p-3 border-b border-zinc-800 flex items-center justify-between">
        <div className="flex items-center gap-3">
          {/* Status Badge */}
          <div
            className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-mono font-bold border ${
              isSuccess
                ? 'bg-emerald-950/70 text-emerald-400 border-emerald-800/80'
                : isError
                ? 'bg-rose-950/70 text-rose-400 border-rose-800/80'
                : 'bg-amber-950/70 text-amber-400 border-amber-800/80'
            }`}
          >
            {isSuccess ? (
              <CheckCircle2 className="w-3.5 h-3.5" />
            ) : (
              <XCircle className="w-3.5 h-3.5" />
            )}
            <span>{lastResult.statusCode}</span>
            <span className="text-[10px] font-normal font-sans opacity-90">{lastResult.statusText}</span>
          </div>

          {/* Timing Badge */}
          <div className="flex items-center gap-1 text-xs text-zinc-400 font-mono bg-zinc-900 border border-zinc-800 px-2 py-0.5 rounded">
            <Clock className="w-3 h-3 text-zinc-500" />
            <span>{formatDuration(lastResult.timing.totalDurationMs)}</span>
          </div>

          {/* Size Badge */}
          <div className="flex items-center gap-1 text-xs text-zinc-400 font-mono bg-zinc-900 border border-zinc-800 px-2 py-0.5 rounded">
            <Database className="w-3 h-3 text-zinc-500" />
            <span>{formatBytes(lastResult.size)}</span>
          </div>
        </div>

        <button
          onClick={handleCopyBody}
          className="flex items-center gap-1 px-2 py-1 text-xs text-zinc-400 hover:text-zinc-200 bg-zinc-900 hover:bg-zinc-800 border border-zinc-800 rounded transition-colors"
          title="Copy Response Body"
        >
          {copied ? <Check className="w-3 h-3 text-emerald-400" /> : <Copy className="w-3 h-3" />}
          <span>{copied ? 'Copied' : 'Copy'}</span>
        </button>
      </div>

      {/* Sub Tabs */}
      <div className="flex items-center gap-1 px-3 border-b border-zinc-800 text-xs">
        {(['body', 'headers', 'tests', 'timing'] as const).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveSubTab(tab)}
            className={`px-3 py-2 border-b-2 font-medium capitalize transition-colors ${
              activeSubTab === tab
                ? 'border-blue-500 text-blue-400'
                : 'border-transparent text-zinc-400 hover:text-zinc-200'
            }`}
          >
            {tab}
            {tab === 'tests' && lastResult.tests && (
              <span
                className={`ml-1.5 px-1 py-0.2 text-[10px] rounded-full font-mono ${
                  lastResult.tests.every((t) => t.passed)
                    ? 'bg-emerald-950 text-emerald-400'
                    : 'bg-rose-950 text-rose-400'
                }`}
              >
                {lastResult.tests.filter((t) => t.passed).length}/{lastResult.tests.length}
              </span>
            )}
          </button>
        ))}
      </div>

      {/* Tab Content */}
      <div className="flex-1 overflow-y-auto p-3">
        {activeSubTab === 'body' && (
          <div className="h-full rounded-md border border-zinc-800 overflow-hidden">
            <CodeMirror
              value={lastResult.body}
              height="100%"
              extensions={[json()]}
              theme="dark"
              readOnly
              className="text-xs font-mono"
            />
          </div>
        )}

        {activeSubTab === 'headers' && (
          <div className="border border-zinc-800 rounded-md overflow-hidden font-mono text-xs">
            <table className="w-full text-left">
              <thead className="bg-zinc-900/80 text-zinc-400 border-b border-zinc-800 font-sans">
                <tr>
                  <th className="p-2 w-1/3">Header</th>
                  <th className="p-2">Value</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-zinc-800/60">
                {Object.entries(lastResult.headers || {}).map(([key, vals]) => (
                  <tr key={key} className="hover:bg-zinc-900/40">
                    <td className="p-2 text-zinc-300 font-semibold">{key}</td>
                    <td className="p-2 text-zinc-400">{vals.join(', ')}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}

        {activeSubTab === 'tests' && (
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
        )}

        {activeSubTab === 'timing' && (
          <div className="space-y-3 text-xs">
            <span className="font-semibold text-zinc-300">Network Timing Breakdown</span>
            <div className="border border-zinc-800 rounded-md p-3 space-y-2 bg-zinc-900/40">
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
              <div className="border-t border-zinc-800 pt-2 flex justify-between font-bold text-zinc-200 font-mono">
                <span>Total Duration:</span>
                <span className="text-blue-400">{lastResult.timing.totalDurationMs.toFixed(2)} ms</span>
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
