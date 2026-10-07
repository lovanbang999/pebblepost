import { useState, useEffect } from 'react'
import { Folder, Save, Plus, Trash2, Check, FileText, Eye, Edit3, Columns, FileCode2, Link2, RefreshCw, AlertCircle, CheckCircle2 } from 'lucide-react'
import { MarkdownView } from '../common/MarkdownView'
import CodeMirror from '@uiw/react-codemirror'
import { autocompletion } from '@codemirror/autocomplete'
import { javascript } from '@codemirror/lang-javascript'
import { pebbleScriptCompletions } from '../../lib/codemirror-completions'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { useTabStore, type RequestTab } from '../../store/tabStore'
import { cn } from '../../lib/utils'
import type { FolderDefinition, KeyValue, AuthDefinition } from '../../types'
import { Button } from '../ui/button'
import { Input } from '../ui/input'
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
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'
import { Checkbox } from '../ui/checkbox'
import { OpenAPISyncDialog } from './OpenAPISyncDialog'

interface FolderSettingsPanelProps {
  currentTab: RequestTab
}

export function FolderSettingsPanel({ currentTab }: FolderSettingsPanelProps) {
  const { theme } = useWorkspaceStore()
  const { updateActiveFolder, saveCurrentTab } = useTabStore()
  const [isSaved, setIsSaved] = useState(false)
  const [activeTab, setActiveTab] = useState<'headers' | 'auth' | 'vars' | 'scripts' | 'docs' | 'openapi'>('headers')
  const [folderDocsMode, setFolderDocsMode] = useState<'split' | 'edit' | 'preview'>('split')
  const [isSyncDialogOpen, setIsSyncDialogOpen] = useState(false)
  const [specInput, setSpecInput] = useState('')
  const [linking, setLinking] = useState(false)
  const [linkStatusMessage, setLinkStatusMessage] = useState<{ type: 'success' | 'error'; text: string } | null>(null)

  const folder: FolderDefinition = currentTab.folder || {
    schemaVersion: 2,
    name: currentTab.title,
    auth: { type: 'inherit' },
    headers: [],
    variables: [],
    scripts: {},
  }

  useEffect(() => {
    if (folder.openApiSync?.specLocation) {
      setSpecInput(folder.openApiSync.specLocation)
    }
  }, [folder.openApiSync?.specLocation])

  const handleSave = async () => {
    const success = await saveCurrentTab()
    if (success) {
      setIsSaved(true)
      setTimeout(() => setIsSaved(false), 2000)
    }
  }

  const updateHeaders = (newHeaders: KeyValue[]) => {
    updateActiveFolder((prev) => ({
      ...prev,
      headers: newHeaders,
    }))
  }

  const addHeader = () => {
    const current = folder.headers || []
    updateHeaders([...current, { key: '', value: '', enabled: true }])
  }

  const updateHeaderRow = (index: number, updates: Partial<KeyValue>) => {
    const current = [...(folder.headers || [])]
    current[index] = { ...current[index], ...updates }
    updateHeaders(current)
  }

  const removeHeader = (index: number) => {
    const current = [...(folder.headers || [])]
    current.splice(index, 1)
    updateHeaders(current)
  }

  const updateAuth = (updates: Partial<AuthDefinition>) => {
    updateActiveFolder((prev) => ({
      ...prev,
      auth: {
        ...(prev.auth || { type: 'inherit' }),
        ...updates,
      },
    }))
  }

  const updateVariables = (newVars: KeyValue[]) => {
    updateActiveFolder((prev) => ({
      ...prev,
      variables: newVars,
    }))
  }

  const addVariable = () => {
    const current = folder.variables || []
    updateVariables([...current, { key: '', value: '', enabled: true }])
  }

  const updateVariableRow = (index: number, updates: Partial<KeyValue>) => {
    const current = [...(folder.variables || [])]
    current[index] = { ...current[index], ...updates }
    updateVariables(current)
  }

  const removeVariable = (index: number) => {
    const current = [...(folder.variables || [])]
    current.splice(index, 1)
    updateVariables(current)
  }

  const updateScripts = (field: 'preRequest' | 'postResponse', val: string) => {
    updateActiveFolder((prev) => ({
      ...prev,
      scripts: {
        ...(prev.scripts || {}),
        [field]: val,
      },
    }))
  }

  const currentAuthType = folder.auth?.type || 'inherit'

  const handleLinkSpec = async () => {
    if (!specInput.trim()) return
    setLinking(true)
    setLinkStatusMessage(null)
    try {
      const res = await fetch('/api/sync/link', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          folderPath: currentTab.filePath,
          specLocation: specInput.trim(),
        }),
      })
      if (!res.ok) {
        const text = await res.text()
        throw new Error(text || 'Failed to link OpenAPI spec')
      }
      const data = await res.json()
      updateActiveFolder((prev) => ({
        ...prev,
        schemaVersion: 2,
        openApiSync: {
          specLocation: data.specLocation,
          specHash: data.specHash,
          lastSyncedAt: data.lastSyncedAt,
        },
      }))
      setLinkStatusMessage({
        type: 'success',
        text: 'Successfully linked specification to this collection folder.',
      })
      setIsSyncDialogOpen(true)
    } catch (err: unknown) {
      setLinkStatusMessage({
        type: 'error',
        text: err instanceof Error ? err.message : String(err),
      })
    } finally {
      setLinking(false)
    }
  }

  const handleUnlinkSpec = () => {
    updateActiveFolder((prev) => ({
      ...prev,
      openApiSync: undefined,
    }))
    setSpecInput('')
    setLinkStatusMessage(null)
  }

  return (
    <div className="flex-1 flex flex-col h-full bg-white dark:bg-zinc-950 overflow-hidden">
      {/* Top Banner / Folder Header */}
      <div className="p-4 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/30 flex items-center justify-between gap-4">
        <div className="flex items-center gap-3 min-w-0">
          <div className="w-9 h-9 rounded-lg bg-amber-500/10 dark:bg-amber-500/20 text-amber-600 dark:text-amber-400 flex items-center justify-center shrink-0 border border-amber-500/20">
            <Folder className="w-5 h-5" />
          </div>
          <div className="min-w-0">
            <div className="flex items-center gap-2">
              <input
                type="text"
                value={folder.name || ''}
                onChange={(e) => updateActiveFolder((prev) => ({ ...prev, name: e.target.value }))}
                placeholder="Folder Name"
                className="font-semibold text-sm bg-transparent hover:bg-zinc-100 dark:hover:bg-zinc-800 focus:bg-white dark:focus:bg-zinc-900 px-1.5 py-0.5 rounded border border-transparent focus:border-zinc-300 dark:focus:border-zinc-700 outline-none text-zinc-900 dark:text-zinc-100 truncate"
              />
              <Badge variant="outline" className="font-mono text-[10px] text-zinc-500 shrink-0">
                Folder Settings
              </Badge>
            </div>
            <p className="text-[11px] text-zinc-500 dark:text-zinc-400 px-1.5 truncate">
              {currentTab.filePath}
            </p>
          </div>
        </div>

        <div className="flex items-center gap-2 shrink-0">
          <Button
            size="sm"
            onClick={handleSave}
            className={cn(
              'h-8 px-3 text-xs gap-1.5 font-medium transition-all shadow-xs cursor-pointer',
              isSaved
                ? 'bg-emerald-600 hover:bg-emerald-600 text-white'
                : currentTab.isDirty
                ? 'bg-blue-600 hover:bg-blue-700 text-white'
                : 'bg-zinc-200 hover:bg-zinc-300 dark:bg-zinc-800 dark:hover:bg-zinc-700 text-zinc-800 dark:text-zinc-200'
            )}
          >
            {isSaved ? (
              <>
                <Check className="w-3.5 h-3.5" />
                Saved
              </>
            ) : (
              <>
                <Save className="w-3.5 h-3.5" />
                Save (Ctrl+S)
                {currentTab.isDirty && <span className="w-1.5 h-1.5 rounded-full bg-amber-400 ml-0.5" />}
              </>
            )}
          </Button>
        </div>
      </div>

      {/* Tabs navigation */}
      <Tabs
        value={activeTab}
        onValueChange={(val) => setActiveTab(val as typeof activeTab)}
        className="flex-1 flex flex-col overflow-hidden"
      >
        <TabsList>
          <TabsTrigger value="headers" className="capitalize">
            Headers
            {folder.headers && folder.headers.length > 0 && (
              <Badge
                variant="secondary"
                className="ml-1.5 px-1 py-0 text-[9px]"
              >
                {folder.headers.length}
              </Badge>
            )}
          </TabsTrigger>

          <TabsTrigger value="auth" className="capitalize">
            Auth
            {currentAuthType !== 'inherit' && currentAuthType !== 'none' && (
              <span className="w-1.5 h-1.5 rounded-full bg-emerald-500 ml-1.5" />
            )}
          </TabsTrigger>

          <TabsTrigger value="vars" className="capitalize">
            Variables
            {folder.variables && folder.variables.length > 0 && (
              <Badge
                variant="secondary"
                className="ml-1.5 px-1 py-0 text-[9px]"
              >
                {folder.variables.length}
              </Badge>
            )}
          </TabsTrigger>

          <TabsTrigger value="scripts" className="capitalize">
            Scripts
            {((folder.scripts?.preRequest && folder.scripts.preRequest.trim()) ||
              (folder.scripts?.postResponse && folder.scripts.postResponse.trim())) && (
              <span className="w-1.5 h-1.5 rounded-full bg-blue-500 ml-1.5" />
            )}
          </TabsTrigger>

          <TabsTrigger value="docs" className="capitalize">
            Docs
            {Boolean(folder.description?.trim()) && (
              <span className="w-1.5 h-1.5 rounded-full bg-blue-500 ml-1.5" />
            )}
          </TabsTrigger>

          <TabsTrigger value="openapi" className="capitalize">
            OpenAPI Sync
            {Boolean(folder.openApiSync?.specLocation) && (
              <span className="w-1.5 h-1.5 rounded-full bg-teal-500 ml-1.5" />
            )}
          </TabsTrigger>
        </TabsList>

        {/* Tab 1: Headers */}
        <TabsContent value="headers" className="flex-1 flex flex-col p-4 overflow-y-auto m-0">
          <div className="mb-3 p-3 rounded-md bg-blue-50/50 dark:bg-blue-950/20 border border-blue-200/60 dark:border-blue-900/40 text-xs text-blue-900 dark:text-blue-300">
            <strong>Folder-level Headers:</strong> Headers defined here are inherited by all requests in this folder and subdirectories (case-insensitive). Requests or child folders can override values or disable them.
          </div>

          <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-900 mb-3">
            <Table>
              <TableHeader>
                <TableRow className="bg-zinc-50/50 dark:bg-zinc-900/50">
                  <TableHead className="w-10 text-center"></TableHead>
                  <TableHead className="w-1/3">Key</TableHead>
                  <TableHead>Value</TableHead>
                  <TableHead className="w-12 text-center"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(!folder.headers || folder.headers.length === 0) ? (
                  <TableRow>
                    <TableCell colSpan={4} className="h-20 text-center text-xs text-zinc-400">
                      No folder-level headers configured. Click "Add Header" below to inherit common headers across requests.
                    </TableCell>
                  </TableRow>
                ) : (
                  folder.headers.map((h, idx) => (
                    <TableRow key={idx}>
                      <TableCell className="text-center p-2">
                        <Checkbox
                          checked={h.enabled}
                          onCheckedChange={(checked) =>
                            updateHeaderRow(idx, { enabled: Boolean(checked) })
                          }
                        />
                      </TableCell>
                      <TableCell className="p-2">
                        <Input
                          placeholder="Header Name (e.g. Accept)"
                          value={h.key}
                          onChange={(e) => updateHeaderRow(idx, { key: e.target.value })}
                          className="h-7 text-xs font-mono"
                        />
                      </TableCell>
                      <TableCell className="p-2">
                        <Input
                          placeholder="Header Value (e.g. application/json)"
                          value={h.value}
                          onChange={(e) => updateHeaderRow(idx, { value: e.target.value })}
                          className="h-7 text-xs font-mono"
                        />
                      </TableCell>
                      <TableCell className="text-center p-2">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => removeHeader(idx)}
                          className="h-7 w-7 text-zinc-400 hover:text-red-500"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          <Button
            variant="outline"
            size="sm"
            onClick={addHeader}
            className="w-fit text-xs gap-1.5 h-8 cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            Add Header
          </Button>
        </TabsContent>

        {/* Tab 2: Auth */}
        <TabsContent value="auth" className="flex-1 p-4 overflow-y-auto m-0 max-w-2xl space-y-4">
          <div className="p-3 rounded-md bg-zinc-50 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 text-xs text-zinc-600 dark:text-zinc-400">
            <strong>Folder-level Authentication:</strong> Requests inside this directory default to <em>Inherit from parent</em> and will automatically use this configuration.
          </div>

          <div className="space-y-1.5">
            <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
              Authentication Type
            </label>
            <Select
              value={currentAuthType}
              onValueChange={(val) => {
                if (val) updateAuth({ type: val as AuthDefinition['type'] })
              }}
            >
              <SelectTrigger className="h-8 text-xs font-medium">
                <SelectValue placeholder="Select Auth Type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="inherit">Inherit from parent folder</SelectItem>
                <SelectItem value="none">No Auth (none)</SelectItem>
                <SelectItem value="bearer">Bearer Token</SelectItem>
                <SelectItem value="basic">Basic Auth</SelectItem>
                <SelectItem value="apiKey">API Key</SelectItem>
                <SelectItem value="digest">Digest Auth</SelectItem>
                <SelectItem value="oauth2">OAuth 2.0</SelectItem>
                <SelectItem value="awsSigV4">AWS Signature v4</SelectItem>
              </SelectContent>
            </Select>
          </div>

          {currentAuthType === 'inherit' && (
            <div className="p-4 rounded-lg bg-zinc-50 dark:bg-zinc-900/50 border border-zinc-200 dark:border-zinc-800 text-xs text-zinc-500 space-y-1">
              <p className="font-medium text-zinc-700 dark:text-zinc-300">Inheriting from parent</p>
              <p>This folder delegates authentication to higher parent folders or collection root.</p>
            </div>
          )}

          {currentAuthType === 'bearer' && (
            <div className="space-y-3 pt-2">
              <div className="space-y-1">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                  Bearer Token
                </label>
                <Input
                  type="password"
                  placeholder="token or {{VARIABLE}}"
                  value={folder.auth?.token || ''}
                  onChange={(e) => updateAuth({ token: e.target.value })}
                  className="h-8 text-xs font-mono"
                />
              </div>
            </div>
          )}

          {currentAuthType === 'basic' && (
            <div className="space-y-3 pt-2">
              <div className="space-y-1">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                  Username
                </label>
                <Input
                  placeholder="username or {{VARIABLE}}"
                  value={folder.auth?.username || ''}
                  onChange={(e) => updateAuth({ username: e.target.value })}
                  className="h-8 text-xs"
                />
              </div>
              <div className="space-y-1">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                  Password
                </label>
                <Input
                  type="password"
                  placeholder="password or {{VARIABLE}}"
                  value={folder.auth?.password || ''}
                  onChange={(e) => updateAuth({ password: e.target.value })}
                  className="h-8 text-xs font-mono"
                />
              </div>
            </div>
          )}

          {currentAuthType === 'apiKey' && (
            <div className="space-y-3 pt-2">
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Key</label>
                  <Input
                    placeholder="X-API-Key"
                    value={folder.auth?.key || ''}
                    onChange={(e) => updateAuth({ key: e.target.value })}
                    className="h-8 text-xs font-mono"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Value</label>
                  <Input
                    placeholder="secret-key or {{VARIABLE}}"
                    value={folder.auth?.value || ''}
                    onChange={(e) => updateAuth({ value: e.target.value })}
                    className="h-8 text-xs font-mono"
                  />
                </div>
              </div>
              <div className="space-y-1">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Add to</label>
                <Select
                  value={folder.auth?.addTo || 'header'}
                  onValueChange={(val) => {
                    if (val) updateAuth({ addTo: val as 'header' | 'query' })
                  }}
                >
                  <SelectTrigger className="h-8 text-xs font-medium">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="header">Header</SelectItem>
                    <SelectItem value="query">Query Parameter</SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
          )}

          {currentAuthType === 'digest' && (
            <div className="space-y-3 pt-2">
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Username</label>
                  <Input
                    placeholder="username or {{VARIABLE}}"
                    value={folder.auth?.username || ''}
                    onChange={(e) => updateAuth({ username: e.target.value })}
                    className="h-8 text-xs"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Password</label>
                  <Input
                    type="password"
                    placeholder="password or {{VARIABLE}}"
                    value={folder.auth?.password || ''}
                    onChange={(e) => updateAuth({ password: e.target.value })}
                    className="h-8 text-xs font-mono"
                  />
                </div>
              </div>
              <div className="space-y-1">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Realm (optional)</label>
                <Input
                  placeholder="Realm (leave empty to auto-negotiate)"
                  value={folder.auth?.realm || ''}
                  onChange={(e) => updateAuth({ realm: e.target.value })}
                  className="h-8 text-xs"
                />
              </div>
            </div>
          )}

          {currentAuthType === 'oauth2' && (
            <div className="space-y-3 pt-2">
              <div className="space-y-1">
                <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">
                  Access Token
                </label>
                <Input
                  type="password"
                  placeholder="eyJhbGciOi... or {{ACCESS_TOKEN}}"
                  value={folder.auth?.token || ''}
                  onChange={(e) => updateAuth({ token: e.target.value })}
                  className="h-8 text-xs font-mono"
                />
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Token URL</label>
                  <Input
                    placeholder="https://.../oauth/token"
                    value={folder.auth?.tokenUrl || ''}
                    onChange={(e) => updateAuth({ tokenUrl: e.target.value })}
                    className="h-8 text-xs font-mono"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Client ID</label>
                  <Input
                    placeholder="client-id"
                    value={folder.auth?.clientId || ''}
                    onChange={(e) => updateAuth({ clientId: e.target.value })}
                    className="h-8 text-xs"
                  />
                </div>
              </div>
            </div>
          )}

          {currentAuthType === 'awsSigV4' && (
            <div className="space-y-3 pt-2">
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Access Key</label>
                  <Input
                    placeholder="AKIA..."
                    value={folder.auth?.accessKey || ''}
                    onChange={(e) => updateAuth({ accessKey: e.target.value })}
                    className="h-8 text-xs font-mono"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Secret Key</label>
                  <Input
                    type="password"
                    placeholder="secret-key"
                    value={folder.auth?.secretKey || ''}
                    onChange={(e) => updateAuth({ secretKey: e.target.value })}
                    className="h-8 text-xs font-mono"
                  />
                </div>
              </div>
              <div className="grid grid-cols-2 gap-3">
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Region</label>
                  <Input
                    placeholder="us-east-1"
                    value={folder.auth?.region || ''}
                    onChange={(e) => updateAuth({ region: e.target.value })}
                    className="h-8 text-xs"
                  />
                </div>
                <div className="space-y-1">
                  <label className="text-xs font-medium text-zinc-700 dark:text-zinc-300">Service</label>
                  <Input
                    placeholder="s3 or execute-api"
                    value={folder.auth?.service || ''}
                    onChange={(e) => updateAuth({ service: e.target.value })}
                    className="h-8 text-xs"
                  />
                </div>
              </div>
            </div>
          )}
        </TabsContent>

        {/* Tab 3: Variables */}
        <TabsContent value="vars" className="flex-1 flex flex-col p-4 overflow-y-auto m-0">
          <div className="mb-3 p-3 rounded-md bg-purple-50/50 dark:bg-purple-950/20 border border-purple-200/60 dark:border-purple-900/40 text-xs text-purple-900 dark:text-purple-300">
            <strong>Folder-scoped Variables:</strong> Variables defined here are local to this folder and subdirectories. They override workspace environment variables with the same key. Use <code>{"{{KEY}}"}</code> to reference them.
          </div>

          <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden bg-white dark:bg-zinc-900 mb-3">
            <Table>
              <TableHeader>
                <TableRow className="bg-zinc-50/50 dark:bg-zinc-900/50">
                  <TableHead className="w-10 text-center"></TableHead>
                  <TableHead className="w-1/3">Variable Name</TableHead>
                  <TableHead>Value</TableHead>
                  <TableHead className="w-12 text-center"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {(!folder.variables || folder.variables.length === 0) ? (
                  <TableRow>
                    <TableCell colSpan={4} className="h-20 text-center text-xs text-zinc-400">
                      No folder variables defined. Click "Add Variable" below.
                    </TableCell>
                  </TableRow>
                ) : (
                  folder.variables.map((v, idx) => (
                    <TableRow key={idx}>
                      <TableCell className="text-center p-2">
                        <Checkbox
                          checked={v.enabled}
                          onCheckedChange={(checked) =>
                            updateVariableRow(idx, { enabled: Boolean(checked) })
                          }
                        />
                      </TableCell>
                      <TableCell className="p-2">
                        <Input
                          placeholder="VARIABLE_NAME"
                          value={v.key}
                          onChange={(e) => updateVariableRow(idx, { key: e.target.value })}
                          className="h-7 text-xs font-mono uppercase"
                        />
                      </TableCell>
                      <TableCell className="p-2">
                        <Input
                          placeholder="Variable Value"
                          value={v.value}
                          onChange={(e) => updateVariableRow(idx, { value: e.target.value })}
                          className="h-7 text-xs font-mono"
                        />
                      </TableCell>
                      <TableCell className="text-center p-2">
                        <Button
                          variant="ghost"
                          size="icon"
                          onClick={() => removeVariable(idx)}
                          className="h-7 w-7 text-zinc-400 hover:text-red-500"
                        >
                          <Trash2 className="w-3.5 h-3.5" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          </div>

          <Button
            variant="outline"
            size="sm"
            onClick={addVariable}
            className="w-fit text-xs gap-1.5 h-8 cursor-pointer"
          >
            <Plus className="w-3.5 h-3.5" />
            Add Variable
          </Button>
        </TabsContent>

        {/* Tab 4: Scripts */}
        <TabsContent value="scripts" className="flex-1 flex flex-col p-4 overflow-y-auto m-0 space-y-4">
          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-xs font-semibold text-zinc-900 dark:text-zinc-100">
                  Folder Pre-request Script
                </h4>
                <p className="text-[11px] text-zinc-500">
                  Runs before request pre-request script (Root → Parent → Child → Request). If this script throws an error, execution stops immediately.
                </p>
              </div>
            </div>
            <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden">
              <CodeMirror
                value={folder.scripts?.preRequest || ''}
                height="150px"
                extensions={[javascript(), autocompletion({ override: [pebbleScriptCompletions] })]}
                theme={theme === 'dark' ? 'dark' : 'light'}
                onChange={(val) => updateScripts('preRequest', val)}
                className="text-xs font-mono"
              />
            </div>
          </div>

          <div className="space-y-1.5">
            <div className="flex items-center justify-between">
              <div>
                <h4 className="text-xs font-semibold text-zinc-900 dark:text-zinc-100">
                  Folder Shared Test Script
                </h4>
                <p className="text-[11px] text-zinc-500">
                  Shared assertions that run after the request's test script (Request → Child → Parent → Root).
                </p>
              </div>
            </div>
            <div className="border border-zinc-200 dark:border-zinc-800 rounded-md overflow-hidden">
              <CodeMirror
                value={folder.scripts?.postResponse || ''}
                height="150px"
                extensions={[javascript(), autocompletion({ override: [pebbleScriptCompletions] })]}
                theme={theme === 'dark' ? 'dark' : 'light'}
                onChange={(val) => updateScripts('postResponse', val)}
                className="text-xs font-mono"
              />
            </div>
          </div>
        </TabsContent>

        {/* Tab 5: Docs / Markdown Description */}
        <TabsContent value="docs" className="flex-1 flex flex-col p-4 overflow-hidden m-0">
          <div className="flex items-center justify-between mb-3 pb-2 border-b border-zinc-200 dark:border-zinc-800">
            <div>
              <h4 className="text-xs font-semibold text-zinc-900 dark:text-zinc-100 flex items-center gap-1.5">
                <FileText className="w-3.5 h-3.5 text-blue-500" />
                Folder Description & Documentation
              </h4>
              <p className="text-[11px] text-zinc-500 mt-0.5">
                Included as section overview in generated API docs and OpenAPI tag descriptions.
              </p>
            </div>

            <div className="flex items-center gap-1 bg-zinc-100 dark:bg-zinc-800/80 p-0.5 rounded-lg border border-zinc-200/80 dark:border-zinc-700/60">
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => setFolderDocsMode('edit')}
                className={`h-6 px-2 text-[11px] gap-1 cursor-pointer ${
                  folderDocsMode === 'edit'
                    ? 'bg-white dark:bg-zinc-900 text-blue-600 dark:text-blue-400 shadow-xs'
                    : 'text-zinc-600 dark:text-zinc-400'
                }`}
              >
                <Edit3 className="w-3 h-3" />
                Edit
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => setFolderDocsMode('split')}
                className={`h-6 px-2 text-[11px] gap-1 cursor-pointer ${
                  folderDocsMode === 'split'
                    ? 'bg-white dark:bg-zinc-900 text-blue-600 dark:text-blue-400 shadow-xs'
                    : 'text-zinc-600 dark:text-zinc-400'
                }`}
              >
                <Columns className="w-3 h-3" />
                Split
              </Button>
              <Button
                type="button"
                size="sm"
                variant="ghost"
                onClick={() => setFolderDocsMode('preview')}
                className={`h-6 px-2 text-[11px] gap-1 cursor-pointer ${
                  folderDocsMode === 'preview'
                    ? 'bg-white dark:bg-zinc-900 text-blue-600 dark:text-blue-400 shadow-xs'
                    : 'text-zinc-600 dark:text-zinc-400'
                }`}
              >
                <Eye className="w-3 h-3" />
                Preview
              </Button>
            </div>
          </div>

          <div className="flex-1 flex overflow-hidden rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950">
            {(folderDocsMode === 'edit' || folderDocsMode === 'split') && (
              <div
                className={`flex flex-col h-full overflow-hidden ${
                  folderDocsMode === 'split' ? 'w-1/2 border-r border-zinc-200 dark:border-zinc-800' : 'w-full'
                }`}
              >
                <textarea
                  value={folder.description || ''}
                  onChange={(e) =>
                    updateActiveFolder((prev) => ({
                      ...prev,
                      description: e.target.value,
                    }))
                  }
                  placeholder="Describe the endpoints, data models, or purpose of this folder...&#10;&#10;### Overview&#10;This group contains authentication and user management APIs."
                  className="flex-1 w-full p-4 bg-white dark:bg-zinc-950 text-xs font-mono text-zinc-900 dark:text-zinc-100 placeholder:text-zinc-400 dark:placeholder:text-zinc-600 resize-none outline-none focus:ring-0 leading-relaxed"
                />
              </div>
            )}

            {(folderDocsMode === 'preview' || folderDocsMode === 'split') && (
              <div
                className={`flex-1 h-full overflow-y-auto p-4 bg-zinc-50/50 dark:bg-zinc-900/20 ${
                  folderDocsMode === 'split' ? 'w-1/2' : 'w-full'
                }`}
              >
                <MarkdownView
                  content={folder.description}
                  emptyMessage="No description written for this folder yet. Type in the editor to see formatted Markdown."
                />
              </div>
            )}
          </div>
        </TabsContent>

        {/* Tab 6: OpenAPI Sync */}
        <TabsContent value="openapi" className="flex-1 flex flex-col p-4 overflow-y-auto m-0 space-y-4">
          <div className="p-3.5 rounded-lg bg-teal-50/50 dark:bg-teal-950/20 border border-teal-200/60 dark:border-teal-900/40 text-xs text-teal-900 dark:text-teal-300 flex items-start gap-3">
            <FileCode2 className="w-5 h-5 text-teal-600 dark:text-teal-400 shrink-0 mt-0.5" />
            <div>
              <p className="font-semibold text-teal-950 dark:text-teal-200">OpenAPI Specification Synchronization</p>
              <p className="mt-0.5 opacity-90 leading-relaxed">
                Link this folder to an OpenAPI 3.x specification (local file path or remote URL). PebblePost compares operations with your request files, detects drift (added, changed, removed endpoints), preserves your scripts and custom headers, and allows selective merging.
              </p>
            </div>
          </div>

          {linkStatusMessage && (
            <div
              className={cn(
                'p-3 rounded-lg border text-xs flex items-center gap-2',
                linkStatusMessage.type === 'success'
                  ? 'bg-emerald-500/10 border-emerald-500/20 text-emerald-600 dark:text-emerald-400'
                  : 'bg-red-500/10 border-red-500/20 text-red-600 dark:text-red-400'
              )}
            >
              {linkStatusMessage.type === 'success' ? (
                <CheckCircle2 className="w-4 h-4 shrink-0" />
              ) : (
                <AlertCircle className="w-4 h-4 shrink-0" />
              )}
              <span>{linkStatusMessage.text}</span>
            </div>
          )}

          {/* Configuration Card */}
          <div className="p-4 rounded-xl border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900 shadow-xs space-y-4">
            <div>
              <label className="text-xs font-semibold text-zinc-900 dark:text-zinc-100 flex items-center gap-1.5">
                <Link2 className="w-3.5 h-3.5 text-zinc-500" />
                Specification Location
              </label>
              <p className="text-[11px] text-zinc-500 dark:text-zinc-400 mt-0.5">
                Path relative to collection (e.g. <code className="font-mono text-[10px]">./specs/api.yaml</code>) or full HTTP/HTTPS URL.
              </p>
            </div>

            <div className="flex items-center gap-2">
              <Input
                type="text"
                value={specInput}
                onChange={(e) => setSpecInput(e.target.value)}
                placeholder="https://api.example.com/openapi.json or ./openapi.yaml"
                className="font-mono text-xs h-9 bg-zinc-50 dark:bg-zinc-950"
              />
              <Button
                size="sm"
                onClick={handleLinkSpec}
                disabled={linking || !specInput.trim()}
                className="h-9 px-4 text-xs bg-teal-600 hover:bg-teal-700 text-white font-medium shrink-0 cursor-pointer shadow-xs gap-1.5"
              >
                {linking ? <RefreshCw className="w-3.5 h-3.5 animate-spin" /> : <Link2 className="w-3.5 h-3.5" />}
                {folder.openApiSync?.specLocation ? 'Update Link' : 'Link Spec'}
              </Button>
            </div>

            {folder.openApiSync?.specLocation && (
              <div className="pt-3 border-t border-zinc-100 dark:border-zinc-800 flex flex-wrap items-center justify-between gap-3">
                <div className="flex flex-wrap items-center gap-2">
                  <Badge variant="outline" className="text-teal-600 dark:text-teal-400 bg-teal-500/10 border-teal-500/20 text-[11px]">
                    Linked
                  </Badge>
                  {folder.openApiSync.specHash && (
                    <Badge variant="outline" className="font-mono text-[10px] text-zinc-500 border-zinc-300 dark:border-zinc-700">
                      SHA: {folder.openApiSync.specHash.slice(0, 10)}
                    </Badge>
                  )}
                  {folder.openApiSync.lastSyncedAt && (
                    <span className="text-[11px] text-zinc-500 dark:text-zinc-400">
                      Last synced: {new Date(folder.openApiSync.lastSyncedAt).toLocaleString()}
                    </span>
                  )}
                </div>

                <div className="flex items-center gap-2">
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={handleUnlinkSpec}
                    className="h-8 text-xs text-rose-600 hover:text-rose-700 dark:text-rose-400 hover:bg-rose-500/10 cursor-pointer"
                  >
                    Unlink
                  </Button>
                  <Button
                    size="sm"
                    onClick={() => setIsSyncDialogOpen(true)}
                    className="h-8 text-xs bg-zinc-800 hover:bg-zinc-900 dark:bg-zinc-100 dark:hover:bg-white text-white dark:text-zinc-900 cursor-pointer gap-1.5"
                  >
                    <RefreshCw className="w-3.5 h-3.5" />
                    Check Drift & Sync
                  </Button>
                </div>
              </div>
            )}
          </div>
        </TabsContent>
      </Tabs>

      <OpenAPISyncDialog
        open={isSyncDialogOpen}
        onOpenChange={setIsSyncDialogOpen}
        folderPath={currentTab.filePath}
        specLocation={folder.openApiSync?.specLocation || specInput}
        onApplied={() => {
          saveCurrentTab()
        }}
      />
    </div>
  )
}
