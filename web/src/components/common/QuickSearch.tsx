import { useState, useEffect, useRef, useMemo } from 'react'
import { Search, FileCode, Folder, ArrowRight, Command } from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { TreeNode } from '../../types'
import { getMethodColor } from '../../lib/utils'

interface QuickSearchProps {
  open: boolean
  onClose: () => void
}

// Flatten tree into a searchable list of leaf nodes and folders
function flattenTree(nodes: TreeNode[]): TreeNode[] {
  const results: TreeNode[] = []
  for (const node of nodes) {
    results.push(node)
    if (node.isDir && node.children) {
      results.push(...flattenTree(node.children))
    }
  }
  return results
}

function fuzzyMatch(query: string, text: string): boolean {
  if (!query) return true
  const q = query.toLowerCase()
  const t = text.toLowerCase()
  let qi = 0
  for (let i = 0; i < t.length && qi < q.length; i++) {
    if (t[i] === q[qi]) qi++
  }
  return qi === q.length
}

export function QuickSearch({ open, onClose }: QuickSearchProps) {
  const { tree, loadRequest } = useWorkspaceStore()
  const [query, setQuery] = useState('')
  const [selected, setSelected] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const listRef = useRef<HTMLDivElement>(null)

  const allNodes = useMemo(() => flattenTree(tree), [tree])

  const filtered = useMemo(() => {
    if (!query.trim()) return allNodes.filter(n => !n.isDir).slice(0, 15)
    return allNodes.filter(n => !n.isDir && fuzzyMatch(query, n.name))
  }, [query, allNodes])

  useEffect(() => {
    if (open) {
      setQuery('')
      setSelected(0)
      setTimeout(() => inputRef.current?.focus(), 50)
    }
  }, [open])

  useEffect(() => {
    setSelected(0)
  }, [query])

  const handleSelect = (node: TreeNode) => {
    loadRequest(node.path)
    onClose()
  }

  useEffect(() => {
    const handleKey = (e: KeyboardEvent) => {
      if (!open) return
      if (e.key === 'Escape') {
        onClose()
      } else if (e.key === 'ArrowDown') {
        e.preventDefault()
        setSelected(s => Math.min(s + 1, filtered.length - 1))
      } else if (e.key === 'ArrowUp') {
        e.preventDefault()
        setSelected(s => Math.max(s - 1, 0))
      } else if (e.key === 'Enter') {
        if (filtered[selected]) handleSelect(filtered[selected])
      }
    }
    window.addEventListener('keydown', handleKey)
    return () => window.removeEventListener('keydown', handleKey)
  }, [open, filtered, selected])

  // Scroll selected item into view
  useEffect(() => {
    const el = listRef.current?.querySelector(`[data-idx="${selected}"]`) as HTMLElement
    el?.scrollIntoView({ block: 'nearest' })
  }, [selected])

  if (!open) return null

  return (
    <div
      className="fixed inset-0 z-50 flex items-start justify-center pt-24 px-4"
      onClick={e => e.target === e.currentTarget && onClose()}
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/60 backdrop-blur-sm" onClick={onClose} />

      {/* Modal */}
      <div className="relative w-full max-w-xl bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-700 rounded-xl shadow-2xl overflow-hidden transition-colors">
        {/* Search Input */}
        <div className="flex items-center gap-3 px-4 py-3 border-b border-zinc-200 dark:border-zinc-800">
          <Search className="w-4 h-4 text-zinc-400 dark:text-zinc-500 shrink-0" />
          <input
            ref={inputRef}
            value={query}
            onChange={e => setQuery(e.target.value)}
            placeholder="Search requests by name..."
            className="flex-1 bg-transparent text-sm text-zinc-900 dark:text-zinc-100 placeholder-zinc-400 dark:placeholder-zinc-500 focus:outline-none"
          />
          <kbd className="px-1.5 py-0.5 text-[10px] font-mono text-zinc-500 dark:text-zinc-400 bg-zinc-100 dark:bg-zinc-950 border border-zinc-200 dark:border-zinc-700 rounded">
            ESC
          </kbd>
        </div>

        {/* Results List */}
        <div ref={listRef} className="max-h-80 overflow-y-auto py-1.5">
          {filtered.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-8 text-zinc-400 dark:text-zinc-500 text-sm">
              <FileCode className="w-8 h-8 mb-2 opacity-40" />
              <span>No requests found for "{query}"</span>
            </div>
          ) : (
            filtered.map((node, idx) => (
              <button
                key={node.path}
                data-idx={idx}
                onClick={() => handleSelect(node)}
                onMouseEnter={() => setSelected(idx)}
                className={`w-full flex items-center gap-3 px-4 py-2.5 text-left transition-colors ${
                  idx === selected
                    ? 'bg-blue-50 dark:bg-blue-600/20 text-blue-900 dark:text-zinc-100'
                    : 'text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800/60'
                }`}
              >
                <span
                  className={`text-[9px] font-mono font-bold px-1.5 py-0.5 rounded border uppercase shrink-0 ${getMethodColor(
                    node.method || 'GET'
                  )}`}
                >
                  {node.method || '?'}
                </span>
                <div className="flex-1 min-w-0">
                  <div className="text-sm font-medium truncate">
                    {node.name.replace('.pebble.json', '')}
                  </div>
                  <div className="text-[11px] text-zinc-500 dark:text-zinc-400 truncate flex items-center gap-1">
                    <Folder className="w-3 h-3" />
                    {node.relPath}
                  </div>
                </div>
                {idx === selected && (
                  <ArrowRight className="w-3.5 h-3.5 text-blue-600 dark:text-blue-400 shrink-0" />
                )}
              </button>
            ))
          )}
        </div>

        {/* Footer hints */}
        <div className="px-4 py-2 border-t border-zinc-200 dark:border-zinc-800 flex items-center gap-4 text-[10px] text-zinc-500 dark:text-zinc-400">
          <span className="flex items-center gap-1"><kbd className="bg-zinc-100 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded px-1 text-zinc-700 dark:text-zinc-300">↑↓</kbd> navigate</span>
          <span className="flex items-center gap-1"><kbd className="bg-zinc-100 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded px-1 text-zinc-700 dark:text-zinc-300">↵</kbd> open</span>
          <span className="flex items-center gap-1"><Command className="w-2.5 h-2.5" /> <kbd className="bg-zinc-100 dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded px-1 text-zinc-700 dark:text-zinc-300">K</kbd> to toggle</span>
          <span className="ml-auto">{filtered.length} result{filtered.length !== 1 ? 's' : ''}</span>
        </div>
      </div>
    </div>
  )
}
