import React, { useState, useMemo, useCallback } from 'react'
import {
  ChevronRight,
  ChevronDown,
  Variable,
  Copy,
  Check,
  Maximize2,
  Minimize2,
  Search,
} from 'lucide-react'
import { buildJsonPathFromKeyPath, suggestVariableName } from '../../lib/extractor-utils'

interface ResponseJsonTreeProps {
  body: string
  onSelectNode: (jsonPath: string, formattedValue: string, suggestedName: string) => void
}

interface TreeNodeProps {
  keyName?: string
  value: unknown
  path: (string | number)[]
  depth: number
  searchQuery: string
  expandedKeys: Set<string>
  toggleExpand: (key: string) => void
  onSelectNode: (jsonPath: string, formattedValue: string, suggestedName: string) => void
}

function JsonTreeNode({
  keyName,
  value,
  path,
  depth,
  searchQuery,
  expandedKeys,
  toggleExpand,
  onSelectNode,
}: TreeNodeProps) {
  const [copied, setCopied] = useState(false)
  const pathKey = path.join('.')
  const isExpanded = expandedKeys.has(pathKey)

  const isObject = value !== null && typeof value === 'object'
  const isArray = Array.isArray(value)

  const jsonPath = useMemo(() => buildJsonPathFromKeyPath(path), [path])

  const handleCopyPath = (e: React.MouseEvent) => {
    e.stopPropagation()
    navigator.clipboard.writeText(jsonPath)
    setCopied(true)
    setTimeout(() => setCopied(false), 1500)
  }

  const handleSetVariable = (e: React.MouseEvent) => {
    e.stopPropagation()
    const strVal =
      typeof value === 'string'
        ? value
        : typeof value === 'number' || typeof value === 'boolean'
        ? String(value)
        : JSON.stringify(value)
    const suggested = suggestVariableName(path)
    onSelectNode(jsonPath, strVal, suggested)
  }

  // Filter check
  const matchesSearch = useMemo(() => {
    if (!searchQuery) return true
    const q = searchQuery.toLowerCase()
    if (keyName && keyName.toLowerCase().includes(q)) return true
    if (!isObject && String(value).toLowerCase().includes(q)) return true
    return false
  }, [searchQuery, keyName, value, isObject])

  if (!matchesSearch && !isObject) {
    return null
  }

  return (
    <div className="font-mono text-xs select-text">
      <div
        className="group flex items-center gap-1.5 py-0.5 px-1.5 rounded-sm hover:bg-zinc-100 dark:hover:bg-zinc-800/60 transition-colors"
        style={{ paddingLeft: `${depth * 16 + 6}px` }}
      >
        {isObject ? (
          <button
            type="button"
            onClick={() => toggleExpand(pathKey)}
            className="p-0.5 -ml-1 text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200"
          >
            {isExpanded ? (
              <ChevronDown className="w-3.5 h-3.5" />
            ) : (
              <ChevronRight className="w-3.5 h-3.5" />
            )}
          </button>
        ) : (
          <span className="w-3.5 h-3.5 inline-block shrink-0" />
        )}

        {/* Key name */}
        {keyName !== undefined && (
          <span className="text-zinc-700 dark:text-zinc-300 font-medium shrink-0">
            {keyName}
            <span className="text-zinc-400 dark:text-zinc-600 mr-1">:</span>
          </span>
        )}

        {/* Value rendering */}
        {isObject ? (
          <span
            onClick={() => toggleExpand(pathKey)}
            className="text-zinc-500 dark:text-zinc-400 cursor-pointer text-[11px]"
          >
            {isArray ? `Array [${(value as unknown[]).length}]` : `Object {…}`}
          </span>
        ) : typeof value === 'string' ? (
          <span className="text-emerald-600 dark:text-emerald-400 truncate max-w-md">
            &quot;{value}&quot;
          </span>
        ) : typeof value === 'number' ? (
          <span className="text-blue-600 dark:text-blue-400">{value}</span>
        ) : typeof value === 'boolean' ? (
          <span className="text-purple-600 dark:text-purple-400 font-semibold">{String(value)}</span>
        ) : value === null ? (
          <span className="text-zinc-400 dark:text-zinc-500 italic">null</span>
        ) : (
          <span className="text-zinc-600 dark:text-zinc-300">{String(value)}</span>
        )}

        {/* Hover Action Buttons */}
        <div className="opacity-0 group-hover:opacity-100 ml-auto flex items-center gap-1 shrink-0 transition-opacity">
          <button
            type="button"
            onClick={handleSetVariable}
            title={`Set ${jsonPath} as variable`}
            className="flex items-center gap-1 px-1.5 py-0.5 rounded text-[10px] bg-blue-50 hover:bg-blue-100 dark:bg-blue-950/60 dark:hover:bg-blue-900/80 text-blue-600 dark:text-blue-300 border border-blue-200 dark:border-blue-800/60 shadow-2xs font-sans"
          >
            <Variable className="w-3 h-3" />
            <span>Set as variable</span>
          </button>

          <button
            type="button"
            onClick={handleCopyPath}
            title={`Copy ${jsonPath}`}
            className="p-1 rounded text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200 hover:bg-zinc-200 dark:hover:bg-zinc-700/60"
          >
            {copied ? <Check className="w-3 h-3 text-emerald-500" /> : <Copy className="w-3 h-3" />}
          </button>
        </div>
      </div>

      {/* Render children if expanded */}
      {isObject && isExpanded && (
        <div>
          {isArray
            ? (value as unknown[]).map((item, idx) => (
                <JsonTreeNode
                  key={idx}
                  keyName={String(idx)}
                  value={item}
                  path={[...path, idx]}
                  depth={depth + 1}
                  searchQuery={searchQuery}
                  expandedKeys={expandedKeys}
                  toggleExpand={toggleExpand}
                  onSelectNode={onSelectNode}
                />
              ))
            : Object.entries(value as Record<string, unknown>).map(([k, val]) => (
                <JsonTreeNode
                  key={k}
                  keyName={k}
                  value={val}
                  path={[...path, k]}
                  depth={depth + 1}
                  searchQuery={searchQuery}
                  expandedKeys={expandedKeys}
                  toggleExpand={toggleExpand}
                  onSelectNode={onSelectNode}
                />
              ))}
        </div>
      )}
    </div>
  )
}

export function ResponseJsonTree({ body, onSelectNode }: ResponseJsonTreeProps) {
  const [searchQuery, setSearchQuery] = useState('')
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(() => new Set(['']))

  const parsedData = useMemo(() => {
    try {
      const trimmed = body.trim()
      if (!trimmed) return null
      return JSON.parse(trimmed)
    } catch {
      return null
    }
  }, [body])

  const toggleExpand = useCallback((key: string) => {
    setExpandedKeys((prev) => {
      const next = new Set(prev)
      if (next.has(key)) {
        next.delete(key)
      } else {
        next.add(key)
      }
      return next
    })
  }, [])

  const handleExpandAll = useCallback(() => {
    if (!parsedData) return
    const keys = new Set<string>()
    const walk = (val: unknown, path: (string | number)[]) => {
      if (val !== null && typeof val === 'object') {
        keys.add(path.join('.'))
        if (Array.isArray(val)) {
          val.forEach((item, idx) => walk(item, [...path, idx]))
        } else {
          Object.entries(val as Record<string, unknown>).forEach(([k, v]) =>
            walk(v, [...path, k])
          )
        }
      }
    }
    walk(parsedData, [])
    setExpandedKeys(keys)
  }, [parsedData])

  const handleCollapseAll = useCallback(() => {
    setExpandedKeys(new Set(['']))
  }, [])

  if (parsedData === null) {
    return (
      <div className="p-4 text-xs text-zinc-500 font-mono text-center">
        Unable to parse response body as JSON.
      </div>
    )
  }

  return (
    <div className="flex flex-col h-full bg-white dark:bg-zinc-950 rounded-md border border-zinc-200 dark:border-zinc-800 overflow-hidden">
      {/* Toolbar */}
      <div className="flex items-center justify-between gap-2 px-3 py-1.5 border-b border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/60 text-xs">
        <div className="relative flex-1 max-w-xs">
          <Search className="w-3.5 h-3.5 absolute left-2 top-1/2 -translate-y-1/2 text-zinc-400 pointer-events-none" />
          <input
            type="text"
            value={searchQuery}
            onChange={(e) => setSearchQuery(e.target.value)}
            placeholder="Search JSON keys or values..."
            className="w-full pl-7 pr-2 py-1 text-xs rounded border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 focus:outline-none focus:ring-1 focus:ring-blue-500"
          />
        </div>

        <div className="flex items-center gap-1.5 shrink-0">
          <button
            type="button"
            onClick={handleExpandAll}
            className="flex items-center gap-1 px-2 py-1 text-[11px] rounded text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800 transition-colors"
          >
            <Maximize2 className="w-3 h-3" /> Expand All
          </button>
          <button
            type="button"
            onClick={handleCollapseAll}
            className="flex items-center gap-1 px-2 py-1 text-[11px] rounded text-zinc-600 dark:text-zinc-400 hover:bg-zinc-200 dark:hover:bg-zinc-800 transition-colors"
          >
            <Minimize2 className="w-3 h-3" /> Collapse All
          </button>
        </div>
      </div>

      {/* Interactive Tree View */}
      <div className="flex-1 overflow-auto p-3 space-y-0.5">
        {Array.isArray(parsedData) ? (
          parsedData.map((item, idx) => (
            <JsonTreeNode
              key={idx}
              keyName={String(idx)}
              value={item}
              path={[idx]}
              depth={0}
              searchQuery={searchQuery}
              expandedKeys={expandedKeys}
              toggleExpand={toggleExpand}
              onSelectNode={onSelectNode}
            />
          ))
        ) : typeof parsedData === 'object' ? (
          Object.entries(parsedData as Record<string, unknown>).map(([k, val]) => (
            <JsonTreeNode
              key={k}
              keyName={k}
              value={val}
              path={[k]}
              depth={0}
              searchQuery={searchQuery}
              expandedKeys={expandedKeys}
              toggleExpand={toggleExpand}
              onSelectNode={onSelectNode}
            />
          ))
        ) : (
          <JsonTreeNode
            value={parsedData}
            path={[]}
            depth={0}
            searchQuery={searchQuery}
            expandedKeys={expandedKeys}
            toggleExpand={toggleExpand}
            onSelectNode={onSelectNode}
          />
        )}
      </div>
    </div>
  )
}
