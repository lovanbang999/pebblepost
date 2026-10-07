import { useState, useMemo, useCallback } from 'react'
import CodeMirror from '@uiw/react-codemirror'
import type { Extension } from '@codemirror/state'
import { json } from '@codemirror/lang-json'
import { graphql } from 'cm6-graphql'
import {
  RefreshCw,
  BookOpen,
  CheckCircle2,
  AlertCircle,
  Columns,
  Square,
} from 'lucide-react'
import { Button } from '../ui/button'
import { Badge } from '../ui/badge'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '../ui/select'
import { Tabs, TabsList, TabsTrigger, TabsContent } from '../ui/tabs'
import { GraphQLDocsExplorer } from './GraphQLDocsExplorer'
import {
  parseOperations,
  validateVariablesJson,
  buildClientSchemaSafe,
} from '../../lib/graphql-utils'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { RequestDefinition, GraphQLDefinition } from '../../types'

interface GraphQLEditorProps {
  request: RequestDefinition
  onChange: (updater: (prev: RequestDefinition) => RequestDefinition) => void
  theme: 'light' | 'dark'
  variableExtensions?: Extension[]
}

export function GraphQLEditor({
  request,
  onChange,
  theme,
  variableExtensions = [],
}: GraphQLEditorProps) {
  const { workspacePath, activeEnv } = useWorkspaceStore()
  const [activeTab, setActiveTab] = useState<'query' | 'variables'>('query')
  const [isSplitView, setIsSplitView] = useState(false)
  const [showDocs, setShowDocs] = useState(false)
  const [isLoadingSchema, setIsLoadingSchema] = useState(false)
  const [schemaError, setSchemaError] = useState<string | null>(null)

  const gql: GraphQLDefinition = request.body?.graphql || {
    query: '',
    variables: '',
    operationName: '',
  }

  const updateGraphQL = useCallback(
    (updater: (prev: GraphQLDefinition) => GraphQLDefinition) => {
      onChange((prevReq) => {
        const currentGql = prevReq.body?.graphql || {
          query: '',
          variables: '',
          operationName: '',
        }
        const updated = updater(currentGql)
        return {
          ...prevReq,
          body: {
            ...prevReq.body,
            type: 'graphql',
            graphql: updated,
          },
        }
      })
    },
    [onChange]
  )

  // Extract operations for multi-operation operationName selector
  const operations = useMemo(() => {
    return parseOperations(gql.query || '')
  }, [gql.query])

  // Validate variables JSON
  const variablesValidation = useMemo(() => {
    return validateVariablesJson(gql.variables || '')
  }, [gql.variables])

  // Build client schema for CodeMirror autocompletion and diagnostics
  const clientSchema = useMemo(() => {
    return buildClientSchemaSafe(gql.schemaCache)
  }, [gql.schemaCache])

  // CodeMirror extensions for GraphQL
  const graphqlExtensions = useMemo(() => {
    return [graphql(clientSchema || undefined), ...variableExtensions]
  }, [clientSchema, variableExtensions])

  // CodeMirror extensions for JSON Variables
  const jsonExtensions = useMemo(() => {
    return [json(), ...variableExtensions]
  }, [variableExtensions])

  // Fetch or refresh schema introspection
  const handleRefreshSchema = async () => {
    setIsLoadingSchema(true)
    setSchemaError(null)
    try {
      const res = await fetch('/api/graphql/introspect', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath,
          environmentName: activeEnv,
          path: request.id,
          request,
        }),
      })

      if (!res.ok) {
        const errData = await res.json().catch(() => ({}))
        throw new Error(errData.error || `HTTP ${res.status}: ${res.statusText}`)
      }

      const schemaJson = await res.json()
      updateGraphQL((prev) => ({
        ...prev,
        schemaCache: schemaJson,
        lastIntrospectedAt: new Date().toISOString(),
      }))
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'Failed to introspect schema'
      setSchemaError(message)
    } finally {
      setIsLoadingSchema(false)
    }
  }

  const hasSchema = Boolean(gql.schemaCache)
  const formattedIntrospectTime = useMemo(() => {
    if (!gql.lastIntrospectedAt) return null
    try {
      return new Date(gql.lastIntrospectedAt).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
      })
    } catch {
      return null
    }
  }, [gql.lastIntrospectedAt])

  return (
    <div className="flex flex-col flex-1 h-full min-h-0 relative">
      {/* ── TOOLBAR ── */}
      <div className="flex items-center justify-between gap-2 px-3 py-1.5 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/40 text-xs">
        <div className="flex items-center gap-2 flex-wrap">
          {/* Operation Name Selector (if multiple or named operations exist) */}
          {operations.length > 0 && (
            <div className="flex items-center gap-1.5">
              <span className="text-[11px] font-medium text-zinc-500">Operation:</span>
              <Select
                value={gql.operationName || '__default__'}
                onValueChange={(val) =>
                  updateGraphQL((prev) => ({
                    ...prev,
                    operationName: !val || val === '__default__' ? '' : val,
                  }))
                }
              >
                <SelectTrigger className="h-6 text-[11px] font-mono min-w-30 max-w-45 bg-white dark:bg-zinc-950">
                  <SelectValue placeholder="Auto (First)" />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="__default__" className="text-xs">
                    <span className="text-zinc-500 italic">Auto (First operation)</span>
                  </SelectItem>
                  {operations.map((op) => (
                    <SelectItem key={op.name} value={op.name} className="text-xs font-mono">
                      <span className="text-blue-500 uppercase font-semibold mr-1.5 text-[10px]">
                        {op.type}
                      </span>
                      {op.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </div>
          )}

          {/* Schema Introspection Status Badge */}
          {hasSchema ? (
            <Badge
              variant="outline"
              className="gap-1 text-[10px] text-emerald-600 dark:text-emerald-400 bg-emerald-500/10 border-emerald-500/20"
            >
              <CheckCircle2 className="w-2.5 h-2.5" />
              <span>Schema Cached{formattedIntrospectTime ? ` (${formattedIntrospectTime})` : ''}</span>
            </Badge>
          ) : (
            <Badge variant="outline" className="text-[10px] text-zinc-500 border-zinc-300 dark:border-zinc-700">
              No Schema Cached
            </Badge>
          )}

          {/* Variables Validation Status */}
          {gql.variables && gql.variables.trim() && (
            variablesValidation.isValid ? (
              <Badge variant="outline" className="text-[10px] text-zinc-500 border-zinc-200 dark:border-zinc-800">
                JSON Valid
              </Badge>
            ) : (
              <Badge variant="destructive" className="gap-1 text-[10px]">
                <AlertCircle className="w-2.5 h-2.5" />
                <span>Invalid JSON</span>
              </Badge>
            )
          )}
        </div>

        {/* Toolbar Actions */}
        <div className="flex items-center gap-1.5">
          {/* Split / Tab View Toggle */}
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setIsSplitView((prev) => !prev)}
            title={isSplitView ? 'Tabbed View' : 'Split View'}
            className="h-6 px-1.5 text-zinc-500 hover:text-zinc-900 dark:hover:text-zinc-100"
          >
            {isSplitView ? <Square className="w-3.5 h-3.5" /> : <Columns className="w-3.5 h-3.5" />}
          </Button>

          {/* Docs Explorer Button */}
          <Button
            variant={showDocs ? 'secondary' : 'ghost'}
            size="sm"
            onClick={() => setShowDocs((prev) => !prev)}
            disabled={!hasSchema}
            title={hasSchema ? 'Explore Schema Documentation' : 'Fetch schema to explore docs'}
            className="h-6 px-2 gap-1 text-[11px]"
          >
            <BookOpen className="w-3 h-3 text-blue-500" />
            <span>Docs</span>
          </Button>

          {/* Refresh Schema Button */}
          <Button
            variant="ghost"
            size="sm"
            onClick={handleRefreshSchema}
            disabled={isLoadingSchema}
            className="h-6 px-2 gap-1 text-[11px] text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100"
          >
            <RefreshCw className={`w-3 h-3 ${isLoadingSchema ? 'animate-spin text-blue-500' : ''}`} />
            <span>{hasSchema ? 'Refresh' : 'Fetch Schema'}</span>
          </Button>
        </div>
      </div>

      {/* Error alert if schema fetch failed */}
      {schemaError && (
        <div className="flex items-center justify-between px-3 py-1.5 bg-red-500/10 border-b border-red-500/20 text-red-600 dark:text-red-400 text-xs">
          <div className="flex items-center gap-1.5">
            <AlertCircle className="w-3.5 h-3.5 shrink-0" />
            <span>Introspection failed: {schemaError}</span>
          </div>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSchemaError(null)}
            className="h-5 px-1 text-xs text-red-500 hover:bg-red-500/10"
          >
            Dismiss
          </Button>
        </div>
      )}

      {/* ── MAIN WORKSPACE / EDITORS ── */}
      <div className="flex-1 min-h-0 flex overflow-hidden">
        {/* Editor Area */}
        <div className="flex-1 flex flex-col min-h-0 overflow-hidden">
          {isSplitView ? (
            /* SPLIT VIEW (Query left, Variables right) */
            <div className="flex-1 grid grid-cols-2 min-h-0 divide-x divide-zinc-200 dark:divide-zinc-800">
              {/* Left: Query */}
              <div className="flex flex-col min-h-0">
                <div className="px-3 py-1 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/30 dark:bg-zinc-900/20 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">
                  Query
                </div>
                <div className="flex-1 min-h-0 overflow-hidden">
                  <CodeMirror
                    value={gql.query || ''}
                    height="100%"
                    extensions={graphqlExtensions}
                    theme={theme === 'dark' ? 'dark' : 'light'}
                    onChange={(val) =>
                      updateGraphQL((prev) => ({ ...prev, query: val }))
                    }
                    className="text-xs font-mono h-full"
                  />
                </div>
              </div>

              {/* Right: Variables */}
              <div className="flex flex-col min-h-0">
                <div className="flex items-center justify-between px-3 py-1 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/30 dark:bg-zinc-900/20 text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">
                  <span>Variables (JSON)</span>
                  {!variablesValidation.isValid && (
                    <span className="text-[10px] text-red-500 normal-case font-normal truncate max-w-45">
                      {variablesValidation.error}
                    </span>
                  )}
                </div>
                <div className="flex-1 min-h-0 overflow-hidden">
                  <CodeMirror
                    value={gql.variables || ''}
                    height="100%"
                    extensions={jsonExtensions}
                    theme={theme === 'dark' ? 'dark' : 'light'}
                    onChange={(val) =>
                      updateGraphQL((prev) => ({ ...prev, variables: val }))
                    }
                    placeholder="e.g. {&#10;  &quot;id&quot;: &quot;123&quot;&#10;}"
                    className="text-xs font-mono h-full"
                  />
                </div>
              </div>
            </div>
          ) : (
            /* TABBED VIEW (Query vs Variables tabs) */
            <Tabs
              value={activeTab}
              onValueChange={(val) => setActiveTab(val as 'query' | 'variables')}
              className="flex-1 flex flex-col min-h-0 overflow-hidden"
            >
              <div className="flex items-center justify-between px-3 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/30 dark:bg-zinc-900/20">
                <TabsList className="bg-transparent h-7 p-0 gap-3">
                  <TabsTrigger
                    value="query"
                    className="text-xs px-2 py-1 data-[state=active]:font-semibold data-[state=active]:text-zinc-900 dark:data-[state=active]:text-zinc-100"
                  >
                    Query
                  </TabsTrigger>
                  <TabsTrigger
                    value="variables"
                    className="text-xs px-2 py-1 gap-1.5 data-[state=active]:font-semibold data-[state=active]:text-zinc-900 dark:data-[state=active]:text-zinc-100"
                  >
                    <span>Variables</span>
                    {gql.variables && gql.variables.trim() && (
                      <span
                        className={`w-1.5 h-1.5 rounded-full ${
                          variablesValidation.isValid ? 'bg-emerald-500' : 'bg-red-500'
                        }`}
                      />
                    )}
                  </TabsTrigger>
                </TabsList>

                {activeTab === 'variables' && !variablesValidation.isValid && (
                  <span className="text-[11px] text-red-500 font-medium truncate max-w-75">
                    {variablesValidation.error}
                  </span>
                )}
              </div>

              <TabsContent value="query" className="flex-1 min-h-0 m-0 overflow-hidden">
                <CodeMirror
                  value={gql.query || ''}
                  height="100%"
                  extensions={graphqlExtensions}
                  theme={theme === 'dark' ? 'dark' : 'light'}
                  onChange={(val) =>
                    updateGraphQL((prev) => ({ ...prev, query: val }))
                  }
                  className="text-xs font-mono h-full"
                />
              </TabsContent>

              <TabsContent value="variables" className="flex-1 min-h-0 m-0 overflow-hidden">
                <CodeMirror
                  value={gql.variables || ''}
                  height="100%"
                  extensions={jsonExtensions}
                  theme={theme === 'dark' ? 'dark' : 'light'}
                  onChange={(val) =>
                    updateGraphQL((prev) => ({ ...prev, variables: val }))
                  }
                  placeholder="e.g. {&#10;  &quot;id&quot;: &quot;123&quot;&#10;}"
                  className="text-xs font-mono h-full"
                />
              </TabsContent>
            </Tabs>
          )}
        </div>

        {/* Right Drawer: Schema Documentation Explorer */}
        {showDocs && hasSchema && (
          <div className="w-80 min-w-70 max-w-100 h-full shadow-lg z-10 animate-in slide-in-from-right duration-150">
            <GraphQLDocsExplorer
              schemaData={gql.schemaCache}
              onClose={() => setShowDocs(false)}
            />
          </div>
        )}
      </div>
    </div>
  )
}
