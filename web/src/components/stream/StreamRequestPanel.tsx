import { useState } from 'react'
import {
  Send,
  Plus,
  Trash2,
  Sliders,
  MessageSquare,
  Clock,
  HardDrive,
  Copy,
  Check,
} from 'lucide-react'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'
import { Tooltip } from '../ui/tooltip'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { RequestDefinition, StreamDefinition, WebSocketMessage, StreamSessionStatus } from '../../types'

interface StreamRequestPanelProps {
  request: RequestDefinition
  onChange: (updater: (prev: RequestDefinition) => RequestDefinition) => void
  isStreaming: boolean
  streamStatus?: StreamSessionStatus | null
  onSend?: (payload: string, type?: 'text' | 'binary' | 'ping' | 'pong') => Promise<void>
}

export function StreamRequestPanel({
  request,
  onChange,
  isStreaming,
  streamStatus,
  onSend,
}: StreamRequestPanelProps) {
  const { theme } = useWorkspaceStore()
  const isWs = request.protocol === 'websocket' || request.method === 'WS'

  const stream: StreamDefinition = request.stream || {
    autoReconnect: true,
    maxReconnectAttempts: 5,
    reconnectIntervalMs: 1000,
    pingIntervalMs: isWs ? 30000 : 0,
    maxLogEntries: 1000,
    maxLogBytes: 5 * 1024 * 1024,
    outgoingMessages: [],
    subprotocols: [],
    timeoutMs: 10000,
    maxWaitMessages: 5,
  }

  const [activeSubTab, setActiveSubTab] = useState<'messages' | 'config'>(isWs ? 'messages' : 'config')
  const [composerPayload, setComposerPayload] = useState('{\n  "type": "hello"\n}')
  const [composerType, setComposerType] = useState<'text' | 'binary' | 'ping' | 'pong'>('text')
  const [composerName, setComposerName] = useState('')
  const [isSending, setIsSending] = useState(false)
  const [savedCopiedId, setSavedCopiedId] = useState<string | null>(null)

  const updateStream = (updater: (prev: StreamDefinition) => StreamDefinition) => {
    onChange((prev) => {
      const current = prev.stream || {
        autoReconnect: true,
        maxReconnectAttempts: 5,
        reconnectIntervalMs: 1000,
        pingIntervalMs: isWs ? 30000 : 0,
        maxLogEntries: 1000,
        maxLogBytes: 5 * 1024 * 1024,
        outgoingMessages: [],
        subprotocols: [],
        timeoutMs: 10000,
        maxWaitMessages: 5,
      }
      return {
        ...prev,
        stream: updater(current),
      }
    })
  }

  const handleSendComposer = async () => {
    if (!onSend || !composerPayload.trim()) return
    setIsSending(true)
    try {
      await onSend(composerPayload, composerType)
    } finally {
      setIsSending(false)
    }
  }

  const handleSaveToOutgoing = () => {
    if (!composerPayload.trim()) return
    const newMsg: WebSocketMessage = {
      id: 'msg_' + Date.now(),
      name: composerName.trim() || `Message ${(stream.outgoingMessages?.length || 0) + 1}`,
      payload: composerPayload,
      type: composerType,
    }
    updateStream((prev) => ({
      ...prev,
      outgoingMessages: [...(prev.outgoingMessages || []), newMsg],
    }))
    setComposerName('')
  }

  const handleDeleteSaved = (id: string) => {
    updateStream((prev) => ({
      ...prev,
      outgoingMessages: (prev.outgoingMessages || []).filter((m) => m.id !== id),
    }))
  }

  const handleSendSaved = async (msg: WebSocketMessage) => {
    if (!onSend) return
    setIsSending(true)
    try {
      await onSend(msg.payload, msg.type || 'text')
    } finally {
      setIsSending(false)
    }
  }

  const handleCopySaved = (msg: WebSocketMessage) => {
    navigator.clipboard.writeText(msg.payload)
    setSavedCopiedId(msg.id)
    setTimeout(() => setSavedCopiedId(null), 1500)
  }

  return (
    <div className="flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden">
      <Tabs
        value={activeSubTab}
        onValueChange={(v) => setActiveSubTab(v as 'messages' | 'config')}
        className="flex-1 flex flex-col overflow-hidden"
      >
        <div className="px-3 pt-2 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between">
          <TabsList className="h-8">
            {isWs && (
              <TabsTrigger value="messages" className="text-xs gap-1.5 px-3">
                <MessageSquare className="w-3.5 h-3.5" />
                <span>Messages & Composer</span>
                {(stream.outgoingMessages?.length || 0) > 0 && (
                  <Badge variant="secondary" className="ml-1 px-1 py-0 text-[10px]">
                    {stream.outgoingMessages?.length}
                  </Badge>
                )}
              </TabsTrigger>
            )}
            <TabsTrigger value="config" className="text-xs gap-1.5 px-3">
              <Sliders className="w-3.5 h-3.5" />
              <span>Connection Settings</span>
            </TabsTrigger>
          </TabsList>

          {isStreaming && (
            <div className="flex items-center gap-2 text-xs font-mono">
              <span className="flex h-2 w-2 relative">
                <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"></span>
                <span className="relative inline-flex rounded-full h-2 w-2 bg-emerald-500"></span>
              </span>
              <span className="text-emerald-600 dark:text-emerald-400 font-semibold">
                {streamStatus?.state === 'reconnecting' ? 'Reconnecting...' : 'Live Connected'}
              </span>
            </div>
          )}
        </div>

        {/* ── MESSAGES TAB ── */}
        {isWs && (
          <TabsContent value="messages" className="flex-1 overflow-y-auto p-4 m-0 space-y-5">
            {/* Live Message Composer */}
            <div className="space-y-2 border border-zinc-200 dark:border-zinc-800 rounded-lg p-3 bg-zinc-50/50 dark:bg-zinc-900/30">
              <div className="flex items-center justify-between gap-2">
                <div className="flex items-center gap-2">
                  <span className="text-xs font-bold text-zinc-800 dark:text-zinc-200">Message Composer</span>
                  <Select
                    value={composerType}
                    onValueChange={(v) => setComposerType(v as 'text' | 'binary' | 'ping' | 'pong')}
                  >
                    <SelectTrigger className="h-6 text-[11px] w-24">
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="text">Text (JSON)</SelectItem>
                      <SelectItem value="binary">Binary</SelectItem>
                      <SelectItem value="ping">Ping</SelectItem>
                      <SelectItem value="pong">Pong</SelectItem>
                    </SelectContent>
                  </Select>
                </div>

                <div className="flex items-center gap-2">
                  <Input
                    placeholder="Saved message label (optional)"
                    value={composerName}
                    onChange={(e) => setComposerName(e.target.value)}
                    className="h-7 text-xs w-48"
                  />
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleSaveToOutgoing}
                    className="h-7 text-xs gap-1"
                  >
                    <Plus className="w-3 h-3" />
                    <span>Save</span>
                  </Button>
                  <Button
                    variant="default"
                    size="sm"
                    disabled={!isStreaming || isSending}
                    onClick={handleSendComposer}
                    className="h-7 text-xs gap-1.5 px-3 bg-cyan-600 hover:bg-cyan-700 text-white font-semibold"
                  >
                    <Send className="w-3 h-3" />
                    <span>Send Message</span>
                  </Button>
                </div>
              </div>

              <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-950">
                <CodeMirror
                  value={composerPayload}
                  height="120px"
                  extensions={[json()]}
                  theme={theme === 'dark' ? 'dark' : 'light'}
                  onChange={(val) => setComposerPayload(val)}
                  className="text-xs font-mono"
                />
              </div>
              <div className="text-[11px] text-zinc-500 flex justify-between">
                <span>Supports JSON, strings, or raw binary payloads.</span>
                {!isStreaming && (
                  <span className="text-amber-600 dark:text-amber-400 font-medium">
                    Connect stream to send live messages.
                  </span>
                )}
              </div>
            </div>

            {/* Saved Outgoing Messages */}
            <div className="space-y-3">
              <div className="flex items-center justify-between">
                <div className="flex items-center gap-2">
                  <h4 className="text-xs font-bold text-zinc-800 dark:text-zinc-200">
                    Saved Outgoing Messages
                  </h4>
                  <span className="text-[11px] text-zinc-500">
                    (Saved in request definition file for reuse & tests)
                  </span>
                </div>
              </div>

              {(!stream.outgoingMessages || stream.outgoingMessages.length === 0) ? (
                <div className="border border-dashed border-zinc-200 dark:border-zinc-800 rounded-lg p-6 text-center text-zinc-500 text-xs">
                  <MessageSquare className="w-6 h-6 text-zinc-400 mx-auto mb-1.5 stroke-[1.5]" />
                  <p className="font-semibold text-zinc-700 dark:text-zinc-300">No saved outgoing messages</p>
                  <p className="text-[11px] mt-0.5 text-zinc-400">
                    Type a message in the composer above and click "Save" to keep templates ready to dispatch.
                  </p>
                </div>
              ) : (
                <div className="space-y-2">
                  {stream.outgoingMessages.map((msg) => (
                    <div
                      key={msg.id}
                      className="border border-zinc-200 dark:border-zinc-800 rounded-lg p-2.5 bg-white dark:bg-zinc-900/40 hover:border-zinc-300 dark:hover:border-zinc-700 transition-colors flex items-start justify-between gap-3"
                    >
                      <div className="flex-1 min-w-0">
                        <div className="flex items-center gap-2 mb-1">
                          <span className="text-xs font-bold text-zinc-900 dark:text-zinc-100 truncate">
                            {msg.name || 'Untitled Message'}
                          </span>
                          <Badge variant="outline" className="text-[10px] uppercase font-mono py-0 px-1.5">
                            {msg.type || 'text'}
                          </Badge>
                        </div>
                        <pre className="text-[11px] font-mono text-zinc-600 dark:text-zinc-400 bg-zinc-50 dark:bg-zinc-950 p-1.5 rounded border border-zinc-100 dark:border-zinc-800/80 max-h-20 overflow-y-auto whitespace-pre-wrap break-all">
                          {msg.payload}
                        </pre>
                      </div>

                      <div className="flex items-center gap-1 shrink-0 pt-1">
                        <Tooltip content="Copy payload">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleCopySaved(msg)}
                            className="h-7 w-7 p-0 text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-200"
                          >
                            {savedCopiedId === msg.id ? (
                              <Check className="w-3.5 h-3.5 text-emerald-500" />
                            ) : (
                              <Copy className="w-3.5 h-3.5" />
                            )}
                          </Button>
                        </Tooltip>
                        <Tooltip content="Load into composer">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => {
                              setComposerPayload(msg.payload)
                              setComposerType(msg.type || 'text')
                              setComposerName(msg.name || '')
                            }}
                            className="h-7 px-2 text-xs text-zinc-600 dark:text-zinc-400"
                          >
                            Load
                          </Button>
                        </Tooltip>
                        <Tooltip content="Send now">
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={!isStreaming || isSending}
                            onClick={() => handleSendSaved(msg)}
                            className="h-7 px-2.5 text-xs gap-1 border-cyan-500/40 text-cyan-600 dark:text-cyan-400 hover:bg-cyan-50 dark:hover:bg-cyan-950/40"
                          >
                            <Send className="w-3 h-3" />
                            <span>Send</span>
                          </Button>
                        </Tooltip>
                        <Tooltip content="Delete saved message">
                          <Button
                            variant="ghost"
                            size="sm"
                            onClick={() => handleDeleteSaved(msg.id)}
                            className="h-7 w-7 p-0 text-zinc-400 hover:text-rose-500"
                          >
                            <Trash2 className="w-3.5 h-3.5" />
                          </Button>
                        </Tooltip>
                      </div>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </TabsContent>
        )}

        {/* ── CONNECTION SETTINGS TAB ── */}
        <TabsContent value="config" className="flex-1 overflow-y-auto p-4 m-0 space-y-6">
          {/* Subprotocols (WebSocket only) */}
          {isWs && (
            <div className="space-y-2">
              <div className="flex items-center justify-between">
                <label className="text-xs font-bold text-zinc-800 dark:text-zinc-200">
                  WebSocket Subprotocols
                </label>
                <span className="text-[11px] text-zinc-500">Comma-separated protocols</span>
              </div>
              <Input
                placeholder="e.g. graphql-transport-ws, v1.stream"
                value={(stream.subprotocols || []).join(', ')}
                onChange={(e) => {
                  const parts = e.target.value
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean)
                  updateStream((prev) => ({ ...prev, subprotocols: parts }))
                }}
                className="h-8 text-xs font-mono"
              />
            </div>
          )}

          {/* Auto Reconnect Settings */}
          <div className="space-y-3 border-t border-zinc-200 dark:border-zinc-800 pt-4">
            <div className="flex items-center justify-between">
              <div>
                <label className="text-xs font-bold text-zinc-800 dark:text-zinc-200 block">
                  Automatic Reconnection
                </label>
                <span className="text-[11px] text-zinc-500">
                  Re-establish connection using exponential backoff when disconnected unexpectedly
                </span>
              </div>
              <input
                type="checkbox"
                checked={stream.autoReconnect ?? true}
                onChange={(e) => updateStream((prev) => ({ ...prev, autoReconnect: e.target.checked }))}
                className="h-4 w-4 rounded border-zinc-300 text-cyan-600 focus:ring-cyan-500 cursor-pointer"
              />
            </div>

            {stream.autoReconnect && (
              <div className="grid grid-cols-2 gap-4 pt-1">
                <div className="space-y-1.5">
                  <span className="text-[11px] font-medium text-zinc-600 dark:text-zinc-400">
                    Max Reconnect Attempts
                  </span>
                  <Input
                    type="number"
                    min={1}
                    max={50}
                    value={stream.maxReconnectAttempts ?? 5}
                    onChange={(e) =>
                      updateStream((prev) => ({
                        ...prev,
                        maxReconnectAttempts: parseInt(e.target.value) || 5,
                      }))
                    }
                    className="h-8 text-xs font-mono"
                  />
                </div>
                <div className="space-y-1.5">
                  <span className="text-[11px] font-medium text-zinc-600 dark:text-zinc-400">
                    Backoff Base Interval (ms)
                  </span>
                  <Input
                    type="number"
                    min={100}
                    step={500}
                    value={stream.reconnectIntervalMs ?? 1000}
                    onChange={(e) =>
                      updateStream((prev) => ({
                        ...prev,
                        reconnectIntervalMs: parseInt(e.target.value) || 1000,
                      }))
                    }
                    className="h-8 text-xs font-mono"
                  />
                </div>
              </div>
            )}
          </div>

          {/* Ping Heartbeat (WebSocket only) */}
          {isWs && (
            <div className="space-y-2 border-t border-zinc-200 dark:border-zinc-800 pt-4">
              <div className="flex items-center justify-between">
                <div>
                  <label className="text-xs font-bold text-zinc-800 dark:text-zinc-200 block">
                    Ping / Keep-Alive Interval
                  </label>
                  <span className="text-[11px] text-zinc-500">
                    Automatically send WebSocket ping frames (0 to disable)
                  </span>
                </div>
                <Badge variant="outline" className="text-[10px] font-mono">
                  {stream.pingIntervalMs && stream.pingIntervalMs > 0
                    ? `${stream.pingIntervalMs / 1000}s`
                    : 'Disabled'}
                </Badge>
              </div>
              <Input
                type="number"
                min={0}
                step={5000}
                placeholder="30000"
                value={stream.pingIntervalMs ?? 30000}
                onChange={(e) =>
                  updateStream((prev) => ({
                    ...prev,
                    pingIntervalMs: parseInt(e.target.value) || 0,
                  }))
                }
                className="h-8 text-xs font-mono w-48"
              />
            </div>
          )}

          {/* Memory Limits / Circular Ring Buffer */}
          <div className="space-y-3 border-t border-zinc-200 dark:border-zinc-800 pt-4">
            <div className="flex items-center gap-2">
              <HardDrive className="w-4 h-4 text-zinc-500" />
              <div>
                <label className="text-xs font-bold text-zinc-800 dark:text-zinc-200 block">
                  Ring Buffer & Memory Limits
                </label>
                <span className="text-[11px] text-zinc-500">
                  Caps circular message buffer to prevent runaway memory usage during long streaming sessions
                </span>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <span className="text-[11px] font-medium text-zinc-600 dark:text-zinc-400">
                  Max Message Entries
                </span>
                <Input
                  type="number"
                  min={100}
                  step={100}
                  value={stream.maxLogEntries ?? 1000}
                  onChange={(e) =>
                    updateStream((prev) => ({
                      ...prev,
                      maxLogEntries: parseInt(e.target.value) || 1000,
                    }))
                  }
                  className="h-8 text-xs font-mono"
                />
              </div>
              <div className="space-y-1.5">
                <span className="text-[11px] font-medium text-zinc-600 dark:text-zinc-400">
                  Max Buffer Size (Bytes)
                </span>
                <Input
                  type="number"
                  min={1024 * 1024}
                  step={1024 * 1024}
                  value={stream.maxLogBytes ?? 5 * 1024 * 1024}
                  onChange={(e) =>
                    updateStream((prev) => ({
                      ...prev,
                      maxLogBytes: parseInt(e.target.value) || 5 * 1024 * 1024,
                    }))
                  }
                  className="h-8 text-xs font-mono"
                />
              </div>
            </div>
            <p className="text-[11px] text-zinc-400">
              Default: 1,000 messages or 5 MB. Oldest messages will be discarded when limit is exceeded.
            </p>
          </div>

          {/* CLI & Runner Execution Options */}
          <div className="space-y-3 border-t border-zinc-200 dark:border-zinc-800 pt-4">
            <div className="flex items-center gap-2">
              <Clock className="w-4 h-4 text-zinc-500" />
              <div>
                <label className="text-xs font-bold text-zinc-800 dark:text-zinc-200 block">
                  CLI & Test Execution Timeout
                </label>
                <span className="text-[11px] text-zinc-500">
                  How long the CLI runner collects stream messages before executing tests
                </span>
              </div>
            </div>

            <div className="grid grid-cols-2 gap-4">
              <div className="space-y-1.5">
                <span className="text-[11px] font-medium text-zinc-600 dark:text-zinc-400">
                  Timeout (ms)
                </span>
                <Input
                  type="number"
                  min={500}
                  step={1000}
                  value={stream.timeoutMs ?? 10000}
                  onChange={(e) =>
                    updateStream((prev) => ({
                      ...prev,
                      timeoutMs: parseInt(e.target.value) || 10000,
                    }))
                  }
                  className="h-8 text-xs font-mono"
                />
              </div>
              <div className="space-y-1.5">
                <span className="text-[11px] font-medium text-zinc-600 dark:text-zinc-400">
                  Max Wait Messages
                </span>
                <Input
                  type="number"
                  min={1}
                  value={stream.maxWaitMessages ?? 5}
                  onChange={(e) =>
                    updateStream((prev) => ({
                      ...prev,
                      maxWaitMessages: parseInt(e.target.value) || 5,
                    }))
                  }
                  className="h-8 text-xs font-mono"
                />
              </div>
            </div>
          </div>
        </TabsContent>
      </Tabs>
    </div>
  )
}
