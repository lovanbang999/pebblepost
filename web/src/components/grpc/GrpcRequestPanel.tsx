import { useState } from 'react'
import {
  RefreshCw,
  Sparkles,
  Plus,
  Trash2,
  Lock,
  Unlock,
  AlertCircle,
} from 'lucide-react'
import CodeMirror from '@uiw/react-codemirror'
import { json } from '@codemirror/lang-json'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'
import {
  Table,
  TableHeader,
  TableBody,
  TableRow,
  TableHead,
  TableCell,
} from '../ui/table'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type {
  RequestDefinition,
  GrpcDefinition,
  GrpcServiceInfo,
  KeyValue,
} from '../../types'

interface GrpcRequestPanelProps {
  request: RequestDefinition
  onChange: (updater: (prev: RequestDefinition) => RequestDefinition) => void
  isExecuting?: boolean
  onCancel?: () => void
}

export function GrpcRequestPanel({
  request,
  onChange,
  isExecuting,
  onCancel,
}: GrpcRequestPanelProps) {
  const { workspacePath, theme } = useWorkspaceStore()
  const grpc: GrpcDefinition = request.grpc || {
    address: request.url || 'localhost:50051',
    protoSource: 'reflection',
    service: '',
    method: '',
    useTls: false,
    message: '{}',
    messages: ['{}'],
    metadata: [],
  }

  const [services, setServices] = useState<GrpcServiceInfo[]>([])
  const [isLoadingServices, setIsLoadingServices] = useState(false)
  const [serviceError, setServiceError] = useState<string | null>(null)
  const [isGeneratingSample, setIsGeneratingSample] = useState(false)
  const [activeSubTab, setActiveSubTab] = useState<'message' | 'metadata' | 'tls' | 'definitions'>('message')

  const updateGrpc = (updater: (prev: GrpcDefinition) => GrpcDefinition) => {
    onChange((prev) => {
      const currentGrpc = prev.grpc || {
        address: prev.url || 'localhost:50051',
        protoSource: 'reflection',
        service: '',
        method: '',
        useTls: false,
        message: '{}',
        messages: ['{}'],
        metadata: [],
      }
      const updated = updater(currentGrpc)
      return {
        ...prev,
        protocol: 'grpc',
        method: 'GRPC',
        url: updated.address,
        grpc: updated,
      }
    })
  }

  // Find currently selected method info
  const currentServiceInfo = services.find((s) => s.name === grpc.service)
  const currentMethodInfo = currentServiceInfo?.methods.find(
    (m) => m.name === grpc.method || m.fullMethod === grpc.method
  )

  // Fetch services via reflection or proto files
  const handleLoadServices = async () => {
    setIsLoadingServices(true)
    setServiceError(null)
    try {
      if (grpc.protoSource === 'file') {
        const protoFiles = grpc.protoFiles && grpc.protoFiles.length > 0 ? grpc.protoFiles : ['']
        const res = await fetch('/api/grpc/proto/services', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            workspacePath,
            protoFiles,
            importPaths: grpc.importPaths || [],
          }),
        })
        if (!res.ok) {
          const errData = await res.json()
          throw new Error(errData.error || res.statusText)
        }
        const data: GrpcServiceInfo[] = await res.json()
        setServices(data || [])
        if (data && data.length > 0 && !grpc.service) {
          updateGrpc((prev) => ({
            ...prev,
            service: data[0].name,
            method: data[0].methods[0]?.name || '',
          }))
        }
      } else {
        // Reflection
        const res = await fetch('/api/grpc/reflect/services', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            address: grpc.address,
            useTls: grpc.useTls,
            insecureSkipVerify: grpc.insecureSkipVerify,
            rootCaPath: grpc.rootCaPath,
            workspacePath,
          }),
        })
        if (!res.ok) {
          const errData = await res.json()
          throw new Error(errData.error || res.statusText)
        }
        const data: GrpcServiceInfo[] = await res.json()
        setServices(data || [])
        if (data && data.length > 0 && !grpc.service) {
          updateGrpc((prev) => ({
            ...prev,
            service: data[0].name,
            method: data[0].methods[0]?.name || '',
          }))
        }
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      setServiceError(msg)
    } finally {
      setIsLoadingServices(false)
    }
  }

  // Generate sample message
  const handleGenerateSample = async () => {
    if (!grpc.service || !grpc.method) return
    setIsGeneratingSample(true)
    try {
      const res = await fetch('/api/grpc/sample-message', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath,
          grpc,
        }),
      })
      if (res.ok) {
        const data = await res.json()
        if (data.sample) {
          updateGrpc((prev) => ({
            ...prev,
            message: data.sample,
            messages: [data.sample],
          }))
        }
      }
    } catch {
      // ignore
    } finally {
      setIsGeneratingSample(false)
    }
  }

  // Metadata handlers
  const handleAddMetadata = () => {
    updateGrpc((prev) => ({
      ...prev,
      metadata: [...(prev.metadata || []), { key: '', value: '', enabled: true }],
    }))
  }

  const handleUpdateMetadata = <K extends keyof KeyValue>(
    index: number,
    field: K,
    val: KeyValue[K],
  ) => {
    updateGrpc((prev) => {
      const nextMeta = [...(prev.metadata || [])]
      nextMeta[index] = { ...nextMeta[index], [field]: val }
      return { ...prev, metadata: nextMeta }
    })
  }

  const handleRemoveMetadata = (index: number) => {
    updateGrpc((prev) => ({
      ...prev,
      metadata: (prev.metadata || []).filter((_, i) => i !== index),
    }))
  }

  // Client-streaming message list handlers
  const handleAddStreamMessage = () => {
    updateGrpc((prev) => ({
      ...prev,
      messages: [...(prev.messages || []), prev.message || '{}'],
    }))
  }

  const handleUpdateStreamMessage = (index: number, val: string) => {
    updateGrpc((prev) => {
      const nextMsgs = [...(prev.messages || [])]
      nextMsgs[index] = val
      return { ...prev, messages: nextMsgs }
    })
  }

  const handleRemoveStreamMessage = (index: number) => {
    updateGrpc((prev) => ({
      ...prev,
      messages: (prev.messages || []).filter((_, i) => i !== index),
    }))
  }

  return (
    <div className="flex flex-col h-full overflow-hidden bg-white dark:bg-zinc-950">
      {/* Top Config: Service & Method Picker */}
      <div className="p-3 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/40 space-y-2.5">
        <div className="flex items-center gap-2 flex-wrap">
          {/* Definition Source Badge / Button */}
          <div className="flex items-center rounded-md border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 p-0.5 text-xs">
            <button
              onClick={() => updateGrpc((p) => ({ ...p, protoSource: 'reflection' }))}
              className={`px-2.5 py-1 rounded font-medium transition-colors ${
                grpc.protoSource === 'reflection'
                  ? 'bg-indigo-600 text-white shadow-xs'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200'
              }`}
            >
              Reflection
            </button>
            <button
              onClick={() => updateGrpc((p) => ({ ...p, protoSource: 'file' }))}
              className={`px-2.5 py-1 rounded font-medium transition-colors ${
                grpc.protoSource === 'file'
                  ? 'bg-indigo-600 text-white shadow-xs'
                  : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200'
              }`}
            >
              .proto File
            </button>
          </div>

          <Button
            size="sm"
            variant="outline"
            onClick={handleLoadServices}
            disabled={isLoadingServices}
            className="h-8 gap-1.5 text-xs font-semibold"
          >
            <RefreshCw className={`w-3.5 h-3.5 ${isLoadingServices ? 'animate-spin' : ''}`} />
            {isLoadingServices ? 'Loading...' : 'Load Services'}
          </Button>

          {/* Service Selector */}
          <div className="flex-1 min-w-50">
            <Select
              value={grpc.service}
              onValueChange={(val) => {
                const svcName = val || ''
                const svc = services.find((s) => s.name === svcName)
                updateGrpc((prev) => ({
                  ...prev,
                  service: svcName,
                  method: svc?.methods[0]?.name || '',
                }))
              }}
            >
              <SelectTrigger className="h-8 text-xs font-mono">
                <SelectValue placeholder="Select Service..." />
              </SelectTrigger>
              <SelectContent>
                {services.map((s) => (
                  <SelectItem key={s.name} value={s.name} className="text-xs font-mono">
                    {s.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Method Selector */}
          <div className="flex-1 min-w-50">
            <Select
              value={grpc.method}
              onValueChange={(val) => updateGrpc((prev) => ({ ...prev, method: val || '' }))}
              disabled={!grpc.service}
            >
              <SelectTrigger className="h-8 text-xs font-mono">
                <SelectValue placeholder="Select Method..." />
              </SelectTrigger>
              <SelectContent>
                {currentServiceInfo?.methods.map((m) => (
                  <SelectItem key={m.name} value={m.name} className="text-xs font-mono">
                    <div className="flex items-center justify-between gap-3 w-full">
                      <span>{m.name}</span>
                      <span className="text-[10px] px-1.5 py-0.5 rounded font-bold uppercase tracking-wider">
                        {m.clientStreaming && m.serverStreaming
                          ? 'Bidi Stream'
                          : m.serverStreaming
                          ? 'Server Stream'
                          : m.clientStreaming
                          ? 'Client Stream'
                          : 'Unary'}
                      </span>
                    </div>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* RPC Type Badge */}
          {currentMethodInfo && (
            <Badge
              variant="outline"
              className={`h-7 px-2 text-[10px] font-bold uppercase tracking-wider shrink-0 ${
                currentMethodInfo.clientStreaming && currentMethodInfo.serverStreaming
                  ? 'border-emerald-500 text-emerald-600 dark:text-emerald-400 bg-emerald-50/50 dark:bg-emerald-950/30'
                  : currentMethodInfo.serverStreaming
                  ? 'border-purple-500 text-purple-600 dark:text-purple-400 bg-purple-50/50 dark:bg-purple-950/30'
                  : currentMethodInfo.clientStreaming
                  ? 'border-amber-500 text-amber-600 dark:text-amber-400 bg-amber-50/50 dark:bg-amber-950/30'
                  : 'border-blue-500 text-blue-600 dark:text-blue-400 bg-blue-50/50 dark:bg-blue-950/30'
              }`}
            >
              {currentMethodInfo.clientStreaming && currentMethodInfo.serverStreaming
                ? 'Bidirectional'
                : currentMethodInfo.serverStreaming
                ? 'Server Streaming'
                : currentMethodInfo.clientStreaming
                ? 'Client Streaming'
                : 'Unary Call'}
            </Badge>
          )}

          {/* Sample Button */}
          <Button
            size="sm"
            variant="ghost"
            onClick={handleGenerateSample}
            disabled={!grpc.service || !grpc.method || isGeneratingSample}
            className="h-8 gap-1.5 text-xs text-indigo-600 dark:text-indigo-400 hover:text-indigo-700"
            title="Generate sample message from protobuf definition"
          >
            <Sparkles className={`w-3.5 h-3.5 ${isGeneratingSample ? 'animate-spin' : ''}`} />
            Sample Message
          </Button>

          {/* Cancel button if running */}
          {isExecuting && onCancel && (
            <Button
              size="sm"
              variant="destructive"
              onClick={onCancel}
              className="h-8 px-3 text-xs font-bold animate-pulse"
            >
              Cancel Call
            </Button>
          )}
        </div>

        {serviceError && (
          <div className="text-xs text-rose-600 dark:text-rose-400 bg-rose-50 dark:bg-rose-950/50 p-2 rounded flex items-center gap-1.5 border border-rose-200 dark:border-rose-900">
            <AlertCircle className="w-3.5 h-3.5 shrink-0" />
            <span className="truncate">{serviceError}</span>
          </div>
        )}
      </div>

      {/* Main Tabs */}
      <Tabs
        value={activeSubTab}
        onValueChange={(val) =>
          setActiveSubTab(val as 'message' | 'metadata' | 'tls' | 'definitions')
        }
        className="flex-1 flex flex-col overflow-hidden"
      >
        <div className="border-b border-zinc-200 dark:border-zinc-800 px-3 bg-zinc-50/30 dark:bg-zinc-900/20">
          <TabsList className="bg-transparent h-9 p-0 gap-4">
            <TabsTrigger
              value="message"
              className="h-9 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-600 data-[state=active]:text-indigo-600 dark:data-[state=active]:text-indigo-400 bg-transparent px-1 text-xs font-semibold"
            >
              {currentMethodInfo?.clientStreaming ? 'Messages (Client Stream)' : 'JSON Message'}
            </TabsTrigger>
            <TabsTrigger
              value="metadata"
              className="h-9 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-600 data-[state=active]:text-indigo-600 dark:data-[state=active]:text-indigo-400 bg-transparent px-1 text-xs font-semibold"
            >
              Metadata {grpc.metadata && grpc.metadata.length > 0 && `(${grpc.metadata.length})`}
            </TabsTrigger>
            <TabsTrigger
              value="tls"
              className="h-9 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-600 data-[state=active]:text-indigo-600 dark:data-[state=active]:text-indigo-400 bg-transparent px-1 text-xs font-semibold"
            >
              TLS & Security {grpc.useTls && '🔒'}
            </TabsTrigger>
            {grpc.protoSource === 'file' && (
              <TabsTrigger
                value="definitions"
                className="h-9 rounded-none border-b-2 border-transparent data-[state=active]:border-indigo-600 data-[state=active]:text-indigo-600 dark:data-[state=active]:text-indigo-400 bg-transparent px-1 text-xs font-semibold"
              >
                .proto Settings
              </TabsTrigger>
            )}
          </TabsList>
        </div>

        {/* Tab 1: Message Editor */}
        <TabsContent value="message" className="flex-1 flex flex-col overflow-hidden m-0 p-3">
          {currentMethodInfo?.clientStreaming ? (
            <div className="flex-1 flex flex-col overflow-hidden space-y-3">
              <div className="flex items-center justify-between text-xs text-zinc-500">
                <span>Client streaming sends a sequence of messages before completing.</span>
                <Button size="sm" variant="outline" onClick={handleAddStreamMessage} className="h-7 text-xs gap-1">
                  <Plus className="w-3 h-3" /> Add Message
                </Button>
              </div>
              <div className="flex-1 overflow-y-auto space-y-3">
                {(grpc.messages && grpc.messages.length > 0 ? grpc.messages : ['{}']).map((msg, i) => (
                  <div key={i} className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden">
                    <div className="bg-zinc-100 dark:bg-zinc-900 px-3 py-1.5 flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 text-xs">
                      <span className="font-semibold text-zinc-700 dark:text-zinc-300">Message #{i + 1}</span>
                      <button
                        onClick={() => handleRemoveStreamMessage(i)}
                        className="text-zinc-400 hover:text-rose-500"
                        title="Remove message"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </div>
                    <CodeMirror
                      value={msg}
                      height="120px"
                      theme={theme === 'dark' ? 'dark' : 'light'}
                      extensions={[json()]}
                      onChange={(val) => handleUpdateStreamMessage(i, val)}
                      className="text-xs font-mono"
                    />
                  </div>
                ))}
              </div>
            </div>
          ) : (
            <div className="flex-1 flex flex-col overflow-hidden border border-zinc-200 dark:border-zinc-800 rounded-md">
              <CodeMirror
                value={grpc.message || '{}'}
                height="100%"
                theme={theme === 'dark' ? 'dark' : 'light'}
                extensions={[json()]}
                onChange={(val) => updateGrpc((prev) => ({ ...prev, message: val }))}
                className="h-full text-xs font-mono"
              />
            </div>
          )}
        </TabsContent>

        {/* Tab 2: Metadata */}
        <TabsContent value="metadata" className="flex-1 overflow-y-auto m-0 p-3 space-y-3">
          <div className="flex items-center justify-between text-xs text-zinc-500">
            <span>gRPC Metadata headers sent with the call.</span>
            <Button size="sm" variant="outline" onClick={handleAddMetadata} className="h-7 text-xs gap-1">
              <Plus className="w-3 h-3" /> Add Metadata
            </Button>
          </div>

          <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden">
            <Table>
              <TableHeader>
                <TableRow className="h-8 text-xs bg-zinc-50/50 dark:bg-zinc-900/50">
                  <TableHead className="w-12 text-center">Use</TableHead>
                  <TableHead className="w-1/3">Key (Header Name)</TableHead>
                  <TableHead>Value</TableHead>
                  <TableHead className="w-10"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(!grpc.metadata || grpc.metadata.length === 0) && (
                  <TableRow>
                    <TableCell colSpan={4} className="text-center text-xs text-zinc-400 py-6">
                      No metadata headers configured. Click "Add Metadata" above.
                    </TableCell>
                  </TableRow>
                )}
                {(grpc.metadata || []).map((meta, i) => (
                  <TableRow key={i} className="h-9">
                    <TableCell className="text-center">
                      <input
                        type="checkbox"
                        checked={meta.enabled}
                        onChange={(e) => handleUpdateMetadata(i, 'enabled', e.target.checked)}
                        className="rounded border-zinc-300 dark:border-zinc-700"
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={meta.key}
                        onChange={(e) => handleUpdateMetadata(i, 'key', e.target.value)}
                        placeholder="e.g. authorization"
                        className="h-7 text-xs font-mono"
                      />
                    </TableCell>
                    <TableCell>
                      <Input
                        value={meta.value}
                        onChange={(e) => handleUpdateMetadata(i, 'value', e.target.value)}
                        placeholder="e.g. Bearer {{token}}"
                        className="h-7 text-xs font-mono"
                      />
                    </TableCell>
                    <TableCell>
                      <button
                        onClick={() => handleRemoveMetadata(i)}
                        className="text-zinc-400 hover:text-rose-500 p-1"
                      >
                        <Trash2 className="w-3.5 h-3.5" />
                      </button>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </div>
        </TabsContent>

        {/* Tab 3: TLS Settings */}
        <TabsContent value="tls" className="flex-1 overflow-y-auto m-0 p-4 space-y-4 max-w-xl">
          <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 pb-3">
            <div>
              <div className="text-sm font-semibold flex items-center gap-1.5">
                {grpc.useTls ? <Lock className="w-4 h-4 text-emerald-500" /> : <Unlock className="w-4 h-4 text-amber-500" />}
                Use Transport Layer Security (TLS)
              </div>
              <div className="text-xs text-zinc-500">
                Enable for secure endpoints (grpcs:// or port 443). Plaintext will be used if disabled.
              </div>
            </div>
            <input
              type="checkbox"
              checked={grpc.useTls}
              onChange={(e) => updateGrpc((p) => ({ ...p, useTls: e.target.checked }))}
              className="h-4 w-4 rounded border-zinc-300 text-indigo-600 focus:ring-indigo-500"
            />
          </div>

          {grpc.useTls && (
            <>
              <div className="flex items-center justify-between border-b border-zinc-200 dark:border-zinc-800 pb-3">
                <div>
                  <div className="text-xs font-semibold">Skip Server TLS Verification</div>
                  <div className="text-xs text-zinc-500">Accept self-signed certificates without validation</div>
                </div>
                <input
                  type="checkbox"
                  checked={grpc.insecureSkipVerify || false}
                  onChange={(e) => updateGrpc((p) => ({ ...p, insecureSkipVerify: e.target.checked }))}
                  className="h-4 w-4 rounded border-zinc-300 text-indigo-600 focus:ring-indigo-500"
                />
              </div>

              <div className="space-y-1.5">
                <label className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                  Custom Root CA Certificate Path
                </label>
                <Input
                  value={grpc.rootCaPath || ''}
                  onChange={(e) => updateGrpc((p) => ({ ...p, rootCaPath: e.target.value }))}
                  placeholder="e.g. certs/ca.pem or {{ca_path}}"
                  className="h-8 text-xs font-mono"
                />
              </div>
            </>
          )}
        </TabsContent>

        {/* Tab 4: .proto Settings */}
        {grpc.protoSource === 'file' && (
          <TabsContent value="definitions" className="flex-1 overflow-y-auto m-0 p-4 space-y-4 max-w-xl">
            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                Proto Files in Workspace (comma-separated or relative paths)
              </label>
              <Input
                value={(grpc.protoFiles || []).join(', ')}
                onChange={(e) =>
                  updateGrpc((p) => ({
                    ...p,
                    protoFiles: e.target.value
                      .split(',')
                      .map((s) => s.trim())
                      .filter(Boolean),
                  }))
                }
                placeholder="e.g. protos/service.proto, protos/models.proto"
                className="h-8 text-xs font-mono"
              />
            </div>

            <div className="space-y-1.5">
              <label className="text-xs font-semibold text-zinc-700 dark:text-zinc-300">
                Import Paths (directories searched for imported .proto files)
              </label>
              <Input
                value={(grpc.importPaths || []).join(', ')}
                onChange={(e) =>
                  updateGrpc((p) => ({
                    ...p,
                    importPaths: e.target.value
                      .split(',')
                      .map((s) => s.trim())
                      .filter(Boolean),
                  }))
                }
                placeholder="e.g. protos, third_party/protos"
                className="h-8 text-xs font-mono"
              />
            </div>
          </TabsContent>
        )}
      </Tabs>
    </div>
  )
}
