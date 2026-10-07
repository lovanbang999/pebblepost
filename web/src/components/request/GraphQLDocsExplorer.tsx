import { useState, useMemo } from 'react'
import {
  Search,
  BookOpen,
  ArrowLeft,
  Layers,
  Tag,
  AlertTriangle,
  Code,
  X,
} from 'lucide-react'
import { Input } from '../ui/input'
import { Badge } from '../ui/badge'
import { Button } from '../ui/button'
import { formatTypeRef, type GraphQLTypeRef } from '../../lib/graphql-utils'

interface SchemaFieldArg {
  name: string
  description?: string
  type: GraphQLTypeRef
  defaultValue?: string
}

interface SchemaField {
  name: string
  description?: string
  args?: SchemaFieldArg[]
  type: GraphQLTypeRef
  isDeprecated?: boolean
  deprecationReason?: string
}

interface SchemaInputValue {
  name: string
  description?: string
  type: GraphQLTypeRef
  defaultValue?: string
}

interface SchemaEnumValue {
  name: string
  description?: string
  isDeprecated?: boolean
  deprecationReason?: string
}

interface SchemaType {
  kind: string
  name: string
  description?: string
  fields?: SchemaField[]
  inputFields?: SchemaInputValue[]
  enumValues?: SchemaEnumValue[]
}

interface GraphQLSchemaRoot {
  queryType?: { name: string }
  mutationType?: { name: string }
  subscriptionType?: { name: string }
  types?: SchemaType[]
}

interface GraphQLDocsExplorerProps {
  schemaData: unknown
  onClose: () => void
}

export function GraphQLDocsExplorer({ schemaData, onClose }: GraphQLDocsExplorerProps) {
  const [search, setSearch] = useState('')
  const [selectedTypeName, setSelectedTypeName] = useState<string | null>(null)

  const container = schemaData as { data?: { __schema?: GraphQLSchemaRoot }; __schema?: GraphQLSchemaRoot } | undefined
  const schema = container?.data?.__schema || container?.__schema

  const typesList: SchemaType[] = useMemo(() => {
    if (!schema?.types) return []
    // Filter out standard built-in introspection types (__Type, etc.) unless searched
    return schema.types.filter((t: SchemaType) => {
      if (!search && t.name.startsWith('__')) return false
      return true
    })
  }, [schema, search])

  const filteredTypes = useMemo(() => {
    if (!search.trim()) return typesList
    const q = search.toLowerCase()
    return typesList.filter(
      (t: SchemaType) =>
        t.name?.toLowerCase().includes(q) ||
        t.description?.toLowerCase().includes(q) ||
        t.fields?.some((f: SchemaField) => f.name?.toLowerCase().includes(q))
    )
  }, [typesList, search])

  const rootQueryType = schema?.queryType?.name
  const rootMutationType = schema?.mutationType?.name
  const rootSubscriptionType = schema?.subscriptionType?.name

  const selectedType: SchemaType | null = useMemo(() => {
    if (!selectedTypeName) return null
    return typesList.find((t: SchemaType) => t.name === selectedTypeName) || null
  }, [typesList, selectedTypeName])

  return (
    <div className="flex flex-col h-full bg-white dark:bg-zinc-950 border-l border-zinc-200 dark:border-zinc-800 text-xs">
      {/* Top Header */}
      <div className="flex items-center justify-between px-3 py-2 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/50">
        <div className="flex items-center gap-1.5 font-semibold text-zinc-900 dark:text-zinc-100">
          <BookOpen className="w-3.5 h-3.5 text-blue-500" />
          <span>GraphQL Schema Docs</span>
        </div>
        <Button
          variant="ghost"
          size="sm"
          onClick={onClose}
          className="h-6 w-6 p-0 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
        >
          <X className="w-3.5 h-3.5" />
        </Button>
      </div>

      {/* Breadcrumbs / Back Bar */}
      {selectedType && (
        <div className="flex items-center gap-2 px-3 py-1.5 bg-zinc-100/70 dark:bg-zinc-900/70 border-b border-zinc-200 dark:border-zinc-800">
          <Button
            variant="ghost"
            size="sm"
            onClick={() => setSelectedTypeName(null)}
            className="h-5 px-1.5 gap-1 text-[11px] text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100"
          >
            <ArrowLeft className="w-3 h-3" />
            <span>All Types</span>
          </Button>
          <span className="text-zinc-400">/</span>
          <span className="font-mono font-medium text-zinc-800 dark:text-zinc-200 truncate">
            {selectedType.name}
          </span>
        </div>
      )}

      {/* Search Input */}
      {!selectedType && (
        <div className="p-2 border-b border-zinc-200 dark:border-zinc-800">
          <div className="relative">
            <Search className="w-3 h-3 absolute left-2.5 top-2 text-zinc-400" />
            <Input
              type="text"
              placeholder="Search types, fields..."
              value={search}
              onChange={(e) => setSearch(e.target.value)}
              className="h-7 pl-7 text-xs bg-white dark:bg-zinc-900"
            />
          </div>
        </div>
      )}

      {/* Content Area */}
      <div className="flex-1 overflow-y-auto p-3 space-y-4">
        {selectedType ? (
          /* TYPE DETAIL VIEW */
          <div className="space-y-3">
            <div>
              <div className="flex items-center gap-2">
                <span className="font-mono font-bold text-sm text-zinc-900 dark:text-zinc-100">
                  {selectedType.name}
                </span>
                <Badge variant="outline" className="text-[10px] uppercase font-mono">
                  {selectedType.kind}
                </Badge>
                {selectedType.name === rootQueryType && (
                  <Badge variant="secondary" className="text-[10px] bg-blue-500/10 text-blue-600 border-blue-500/20">
                    Query Root
                  </Badge>
                )}
                {selectedType.name === rootMutationType && (
                  <Badge variant="secondary" className="text-[10px] bg-amber-500/10 text-amber-600 border-amber-500/20">
                    Mutation Root
                  </Badge>
                )}
                {selectedType.name === rootSubscriptionType && (
                  <Badge variant="secondary" className="text-[10px] bg-purple-500/10 text-purple-600 border-purple-500/20">
                    Subscription Root
                  </Badge>
                )}
              </div>
              {selectedType.description && (
                <p className="text-zinc-600 dark:text-zinc-400 mt-1.5 leading-relaxed">
                  {selectedType.description}
                </p>
              )}
            </div>

            {/* Fields List */}
            {selectedType.fields && selectedType.fields.length > 0 && (
              <div className="space-y-2 pt-2 border-t border-zinc-200 dark:border-zinc-800">
                <span className="font-semibold text-zinc-700 dark:text-zinc-300 text-[11px] uppercase tracking-wider">
                  Fields ({selectedType.fields.length})
                </span>
                <div className="divide-y divide-zinc-200 dark:divide-zinc-800/60 rounded border border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/30">
                  {selectedType.fields.map((f: SchemaField) => {
                    const isDep = f.isDeprecated
                    return (
                      <div key={f.name} className="p-2 space-y-1">
                        <div className="flex items-center justify-between flex-wrap gap-1">
                          <div className="font-mono">
                            <span className="font-semibold text-zinc-900 dark:text-zinc-100">
                              {f.name}
                            </span>
                            {f.args && f.args.length > 0 && (
                              <span className="text-zinc-500">
                                (
                                {f.args.map((a: SchemaFieldArg, idx: number) => (
                                  <span key={a.name}>
                                    <span className="text-zinc-600 dark:text-zinc-300">{a.name}</span>: <span className="text-blue-600 dark:text-blue-400">{formatTypeRef(a.type)}</span>
                                    {idx < (f.args?.length ?? 0) - 1 ? ', ' : ''}
                                  </span>
                                ))}
                                )
                              </span>
                            )}
                            : <span className="text-blue-600 dark:text-blue-400 font-semibold">{formatTypeRef(f.type)}</span>
                          </div>
                          {isDep && (
                            <Badge variant="destructive" className="text-[9px] h-4 px-1 gap-1">
                              <AlertTriangle className="w-2.5 h-2.5" />
                              <span>Deprecated</span>
                            </Badge>
                          )}
                        </div>

                        {f.description && (
                          <p className="text-[11px] text-zinc-600 dark:text-zinc-400">
                            {f.description}
                          </p>
                        )}

                        {isDep && f.deprecationReason && (
                          <div className="text-[10px] text-amber-600 dark:text-amber-400 bg-amber-500/10 px-1.5 py-0.5 rounded inline-block">
                            Reason: {f.deprecationReason}
                          </div>
                        )}
                      </div>
                    )
                  })}
                </div>
              </div>
            )}

            {/* Input Fields (for INPUT_OBJECT) */}
            {selectedType.inputFields && selectedType.inputFields.length > 0 && (
              <div className="space-y-2 pt-2 border-t border-zinc-200 dark:border-zinc-800">
                <span className="font-semibold text-zinc-700 dark:text-zinc-300 text-[11px] uppercase tracking-wider">
                  Input Fields ({selectedType.inputFields.length})
                </span>
                <div className="divide-y divide-zinc-200 dark:divide-zinc-800/60 rounded border border-zinc-200 dark:border-zinc-800 bg-zinc-50/50 dark:bg-zinc-900/30">
                  {selectedType.inputFields.map((inf: SchemaInputValue) => (
                    <div key={inf.name} className="p-2 space-y-1">
                      <div className="font-mono">
                        <span className="font-semibold text-zinc-900 dark:text-zinc-100">{inf.name}</span>: <span className="text-blue-600 dark:text-blue-400">{formatTypeRef(inf.type)}</span>
                      </div>
                      {inf.description && (
                        <p className="text-[11px] text-zinc-600 dark:text-zinc-400">{inf.description}</p>
                      )}
                    </div>
                  ))}
                </div>
              </div>
            )}

            {/* Enum Values */}
            {selectedType.enumValues && selectedType.enumValues.length > 0 && (
              <div className="space-y-2 pt-2 border-t border-zinc-200 dark:border-zinc-800">
                <span className="font-semibold text-zinc-700 dark:text-zinc-300 text-[11px] uppercase tracking-wider">
                  Enum Values
                </span>
                <div className="flex flex-wrap gap-1.5">
                  {selectedType.enumValues.map((ev: SchemaEnumValue) => (
                    <Badge key={ev.name} variant="outline" className="font-mono text-[11px]">
                      {ev.name}
                    </Badge>
                  ))}
                </div>
              </div>
            )}
          </div>
        ) : (
          /* ALL TYPES OVERVIEW LIST */
          <div className="space-y-4">
            {/* Quick Root Links */}
            <div className="space-y-1.5">
              <span className="text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">
                Root Operation Types
              </span>
              <div className="grid grid-cols-1 gap-1.5">
                {rootQueryType && (
                  <button
                    onClick={() => setSelectedTypeName(rootQueryType)}
                    className="flex items-center justify-between p-2 rounded-md border border-blue-500/20 bg-blue-500/5 hover:bg-blue-500/10 text-left transition-colors"
                  >
                    <div className="flex items-center gap-2">
                      <Tag className="w-3.5 h-3.5 text-blue-500" />
                      <span className="font-mono font-semibold text-zinc-900 dark:text-zinc-100">
                        {rootQueryType}
                      </span>
                    </div>
                    <Badge variant="outline" className="text-[10px] text-blue-600">Query</Badge>
                  </button>
                )}
                {rootMutationType && (
                  <button
                    onClick={() => setSelectedTypeName(rootMutationType)}
                    className="flex items-center justify-between p-2 rounded-md border border-amber-500/20 bg-amber-500/5 hover:bg-amber-500/10 text-left transition-colors"
                  >
                    <div className="flex items-center gap-2">
                      <Code className="w-3.5 h-3.5 text-amber-500" />
                      <span className="font-mono font-semibold text-zinc-900 dark:text-zinc-100">
                        {rootMutationType}
                      </span>
                    </div>
                    <Badge variant="outline" className="text-[10px] text-amber-600">Mutation</Badge>
                  </button>
                )}
                {rootSubscriptionType && (
                  <button
                    onClick={() => setSelectedTypeName(rootSubscriptionType)}
                    className="flex items-center justify-between p-2 rounded-md border border-purple-500/20 bg-purple-500/5 hover:bg-purple-500/10 text-left transition-colors"
                  >
                    <div className="flex items-center gap-2">
                      <Layers className="w-3.5 h-3.5 text-purple-500" />
                      <span className="font-mono font-semibold text-zinc-900 dark:text-zinc-100">
                        {rootSubscriptionType}
                      </span>
                    </div>
                    <Badge variant="outline" className="text-[10px] text-purple-600">Subscription</Badge>
                  </button>
                )}
              </div>
            </div>

            {/* Types Directory */}
            <div className="space-y-1.5">
              <span className="text-[11px] font-semibold text-zinc-500 uppercase tracking-wider">
                All Types ({filteredTypes.length})
              </span>
              <div className="divide-y divide-zinc-200 dark:divide-zinc-800 rounded border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-900/50">
                {filteredTypes.map((t: SchemaType) => (
                  <button
                    key={t.name}
                    onClick={() => setSelectedTypeName(t.name)}
                    className="w-full flex items-center justify-between p-2 text-left hover:bg-zinc-50 dark:hover:bg-zinc-800/50 transition-colors"
                  >
                    <div className="min-w-0 pr-2">
                      <div className="font-mono font-medium text-zinc-900 dark:text-zinc-100 truncate">
                        {t.name}
                      </div>
                      {t.description && (
                        <div className="text-[10px] text-zinc-500 truncate">
                          {t.description}
                        </div>
                      )}
                    </div>
                    <Badge variant="outline" className="text-[9px] font-mono shrink-0">
                      {t.kind}
                    </Badge>
                  </button>
                ))}
              </div>
            </div>
          </div>
        )}
      </div>
    </div>
  )
}
