import { useState } from 'react'
import { Folder, Save, Plus, Trash2, Check, ShieldCheck, Key, Terminal, Variable } from 'lucide-react'
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

interface FolderSettingsPanelProps {
  currentTab: RequestTab
}

export function FolderSettingsPanel({ currentTab }: FolderSettingsPanelProps) {
  const { theme } = useWorkspaceStore()
  const { updateActiveFolder, saveCurrentTab } = useTabStore()
  const [isSaved, setIsSaved] = useState(false)
  const [activeTab, setActiveTab] = useState<'headers' | 'auth' | 'vars' | 'scripts'>('headers')

  const folder: FolderDefinition = currentTab.folder || {
    schemaVersion: 1,
    name: currentTab.title,
    auth: { type: 'inherit' },
    headers: [],
    variables: [],
    scripts: {},
  }

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
        onValueChange={(val) => setActiveTab(val as any)}
        className="flex-1 flex flex-col overflow-hidden"
      >
        <div className="px-4 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/20 dark:bg-zinc-900/10">
          <TabsList className="h-9 p-0 bg-transparent gap-2">
            <TabsTrigger
              value="headers"
              className="text-xs h-8 px-3 gap-1.5 data-[state=active]:bg-white dark:data-[state=active]:bg-zinc-900 data-[state=active]:shadow-xs rounded-md"
            >
              <Key className="w-3.5 h-3.5 text-zinc-500" />
              Headers
              {folder.headers && folder.headers.length > 0 && (
                <span className="ml-1 text-[10px] px-1.5 py-0.2 rounded-full bg-zinc-200 dark:bg-zinc-800 font-mono">
                  {folder.headers.length}
                </span>
              )}
            </TabsTrigger>
            <TabsTrigger
              value="auth"
              className="text-xs h-8 px-3 gap-1.5 data-[state=active]:bg-white dark:data-[state=active]:bg-zinc-900 data-[state=active]:shadow-xs rounded-md"
            >
              <ShieldCheck className="w-3.5 h-3.5 text-zinc-500" />
              Auth
              {currentAuthType !== 'inherit' && currentAuthType !== 'none' && (
                <span className="w-2 h-2 rounded-full bg-emerald-500 ml-1" />
              )}
            </TabsTrigger>
            <TabsTrigger
              value="vars"
              className="text-xs h-8 px-3 gap-1.5 data-[state=active]:bg-white dark:data-[state=active]:bg-zinc-900 data-[state=active]:shadow-xs rounded-md"
            >
              <Variable className="w-3.5 h-3.5 text-zinc-500" />
              Variables
              {folder.variables && folder.variables.length > 0 && (
                <span className="ml-1 text-[10px] px-1.5 py-0.2 rounded-full bg-zinc-200 dark:bg-zinc-800 font-mono">
                  {folder.variables.length}
                </span>
              )}
            </TabsTrigger>
            <TabsTrigger
              value="scripts"
              className="text-xs h-8 px-3 gap-1.5 data-[state=active]:bg-white dark:data-[state=active]:bg-zinc-900 data-[state=active]:shadow-xs rounded-md"
            >
              <Terminal className="w-3.5 h-3.5 text-zinc-500" />
              Scripts
              {((folder.scripts?.preRequest && folder.scripts.preRequest.trim()) ||
                (folder.scripts?.postResponse && folder.scripts.postResponse.trim())) && (
                <span className="w-2 h-2 rounded-full bg-blue-500 ml-1" />
              )}
            </TabsTrigger>
          </TabsList>
        </div>

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
              onValueChange={(val: any) => updateAuth({ type: val })}
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
                  onValueChange={(val: any) => updateAuth({ addTo: val })}
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
      </Tabs>
    </div>
  )
}
