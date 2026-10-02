import { useState, useRef, useEffect } from 'react'
import {
  Folder,
  FolderOpen,
  FileCode,
  ChevronRight,
  ChevronDown,
  FilePlus2,
  FolderPlus,
  RefreshCw,
  ChevronsDownUp,
  Loader2,
} from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { TreeNode, RequestDefinition } from '../../types'
import { getMethodColor, cn } from '../../lib/utils'
import { Button } from '../ui/button'
import { Tooltip } from '../ui/tooltip'

const VALID_METHODS: RequestDefinition['method'][] = [
  'GET',
  'POST',
  'PUT',
  'PATCH',
  'DELETE',
  'HEAD',
  'OPTIONS',
]

function parseRequestInput(input: string): {
  method: RequestDefinition['method']
  name: string
} {
  const trimmed = input.trim()
  const parts = trimmed.split(/\s+/)
  if (parts.length > 1) {
    const firstUpper = parts[0].toUpperCase() as RequestDefinition['method']
    if (VALID_METHODS.includes(firstUpper)) {
      const restName = parts.slice(1).join('-')
      return {
        method: firstUpper,
        name: restName.replace(/\.pebble\.json$/, '').replace(/\.json$/, ''),
      }
    }
  }
  return {
    method: 'GET',
    name: trimmed.replace(/\.pebble\.json$/, '').replace(/\.json$/, ''),
  }
}

interface InlineCreationState {
  type: 'file' | 'folder'
  parentPath: string
}

export function CollectionTree() {
  const {
    tree,
    workspacePath,
    activeFilePath,
    isLoadingWorkspace,
    loadRequest,
    loadWorkspace,
    createNewRequest,
  } = useWorkspaceStore()

  const [expandedFolders, setExpandedFolders] = useState<Record<string, boolean>>({
    collections: true,
    'collections/auth': true,
  })

  // Selected item path in the tree (folder or file)
  const [selectedPath, setSelectedPath] = useState<string | null>(null)

  // VS Code-style inline item creation state
  const [creatingNode, setCreatingNode] = useState<InlineCreationState | null>(null)
  const [inlineName, setInlineName] = useState('')
  const inlineInputRef = useRef<HTMLInputElement>(null)
  const isCommittingRef = useRef(false)

  // Auto-expand loaded folders on mount
  useEffect(() => {
    if (tree.length > 0) {
      setExpandedFolders((prev) => {
        const next = { ...prev }
        tree.forEach((node) => {
          if (node.isDir && next[node.path] === undefined) {
            next[node.path] = true
          }
        })
        return next
      })
    }
  }, [tree])

  // Focus and select input on mount
  useEffect(() => {
    if (creatingNode) {
      const timer = setTimeout(() => {
        inlineInputRef.current?.focus()
        inlineInputRef.current?.select()
      }, 20)
      return () => clearTimeout(timer)
    }
  }, [creatingNode])

  const toggleFolder = (path: string) => {
    setExpandedFolders((prev) => ({ ...prev, [path]: !prev[path] }))
  }

  const handleSelectRequest = (node: TreeNode) => {
    setSelectedPath(node.path)
    loadRequest(node.path)
  }

  const handleRefresh = () => {
    loadWorkspace(workspacePath || '.')
  }

  const handleCollapseAll = () => {
    setExpandedFolders({})
  }

  // Check if a path is a directory inside the tree
  const isPathDir = (nodes: TreeNode[], targetPath: string): boolean => {
    for (const node of nodes) {
      if (node.path === targetPath) return node.isDir
      if (node.children && isPathDir(node.children, targetPath)) return true
    }
    return false
  }

  // Determine the target directory for file/folder creation like VS Code
  const getTargetDirectory = (explicitParent?: string): string => {
    if (explicitParent) return explicitParent

    const current = selectedPath || activeFilePath
    if (current) {
      if (isPathDir(tree, current)) {
        return current
      }
      const lastSlash = current.lastIndexOf('/')
      if (lastSlash > 0) {
        return current.substring(0, lastSlash)
      }
    }

    if (tree.length > 0 && tree[0].isDir) {
      return tree[0].path
    }
    const currentWs = workspacePath || '.'
    return currentWs !== '.' ? `${currentWs}/collections` : 'collections'
  }

  // Trigger inline creation like VS Code
  const startInlineCreation = (type: 'file' | 'folder', explicitParent?: string) => {
    const targetDir = getTargetDirectory(explicitParent)
    // Expand the target folder so the input is immediately visible
    setExpandedFolders((prev) => ({ ...prev, [targetDir]: true }))
    setCreatingNode({ type, parentPath: targetDir })
    setInlineName('')
  }

  const cancelInlineCreation = () => {
    setCreatingNode(null)
    setInlineName('')
    isCommittingRef.current = false
  }

  const commitInlineCreation = async () => {
    if (!creatingNode || isCommittingRef.current) return
    const raw = inlineName.trim()
    if (!raw) {
      cancelInlineCreation()
      return
    }

    isCommittingRef.current = true
    const { type, parentPath } = creatingNode
    setCreatingNode(null)
    setInlineName('')

    try {
      if (type === 'file') {
        const { method, name } = parseRequestInput(raw)
        const createdPath = await createNewRequest(parentPath, name, method)
        if (createdPath) {
          await loadRequest(createdPath)
          setSelectedPath(createdPath)
        }
      } else {
        const cleanFolder = raw.replace(/^\/+|\/+$/g, '')
        const fullFolderPath = `${parentPath}/${cleanFolder}`

        const res = await fetch('/api/workspace/folder', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: fullFolderPath }),
        })
        if (res.ok) {
          await loadWorkspace(workspacePath || '.')
          setExpandedFolders((prev) => ({ ...prev, [fullFolderPath]: true }))
          setSelectedPath(fullFolderPath)
        }
      }
    } catch (err) {
      console.error('Failed to create item:', err)
    } finally {
      isCommittingRef.current = false
    }
  }

  const handleInlineKeyDown = (e: React.KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter') {
      e.preventDefault()
      commitInlineCreation()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      cancelInlineCreation()
    }
  }

  const handleInlineBlur = () => {
    if (inlineName.trim()) {
      commitInlineCreation()
    } else {
      cancelInlineCreation()
    }
  }

  // Render the VS Code-style inline input row
  const renderInlineInput = (depth: number) => {
    if (!creatingNode) return null

    const isFolder = creatingNode.type === 'folder'
    const parsed = !isFolder ? parseRequestInput(inlineName) : null

    return (
      <div
        key="inline-creating-input"
        style={{ paddingLeft: `${depth * 12 + 8}px` }}
        className="w-full flex items-center gap-1.5 py-0.5 pr-2 my-0.5 animate-in fade-in duration-100"
      >
        {isFolder ? (
          <Folder className="w-3.5 h-3.5 text-amber-500 shrink-0" />
        ) : (
          <span
            className={cn(
              'text-[9px] font-mono font-bold px-1 py-0.2 rounded border uppercase shrink-0 transition-colors',
              getMethodColor(parsed?.method || 'GET')
            )}
          >
            {parsed?.method || 'GET'}
          </span>
        )}
        <input
          ref={inlineInputRef}
          type="text"
          value={inlineName}
          onChange={(e) => setInlineName(e.target.value)}
          onKeyDown={handleInlineKeyDown}
          onBlur={handleInlineBlur}
          placeholder={isFolder ? 'folder-name' : 'request-name (or: post login)'}
          className="flex-1 h-6 px-1.5 py-0 text-xs font-mono bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 border border-blue-500 rounded focus:outline-none ring-1 ring-blue-500 shadow-xs"
        />
      </div>
    )
  }

  function renderNode(node: TreeNode, depth = 0) {
    const isExpanded = expandedFolders[node.path] ?? true
    const isSelected = (selectedPath || activeFilePath) === node.path
    const isCreatingHere = creatingNode?.parentPath === node.path

    if (node.isDir) {
      return (
        <div key={node.path} className="select-none">
          <div
            onClick={() => setSelectedPath(node.path)}
            className={cn(
              'w-full flex items-center justify-between group py-0.5 pr-1 rounded hover:bg-zinc-200/60 dark:hover:bg-zinc-900/60 transition-colors cursor-pointer',
              selectedPath === node.path && 'bg-zinc-150 dark:bg-zinc-900/80'
            )}
            style={{ paddingLeft: `${depth * 12 + 8}px` }}
          >
            <button
              onClick={() => toggleFolder(node.path)}
              className="flex-1 flex items-center gap-1.5 py-0.5 text-xs text-zinc-700 dark:text-zinc-300 hover:text-zinc-900 dark:hover:text-white rounded truncate cursor-pointer text-left"
            >
              {isExpanded ? (
                <ChevronDown className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-500 shrink-0" />
              ) : (
                <ChevronRight className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-500 shrink-0" />
              )}
              {isExpanded ? (
                <FolderOpen className="w-3.5 h-3.5 text-amber-500 dark:text-amber-400 shrink-0" />
              ) : (
                <Folder className="w-3.5 h-3.5 text-amber-500/80 dark:text-amber-400/80 shrink-0" />
              )}
              <span className="truncate font-medium text-zinc-700 dark:text-zinc-300">
                {node.name}
              </span>
            </button>

            {/* Quick folder action buttons on hover (VS Code style) */}
            <div className="opacity-0 group-hover:opacity-100 flex items-center gap-0.5 transition-opacity">
              <Tooltip content={`New Request in ${node.name}`}>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    startInlineCreation('file', node.path)
                  }}
                  className="h-5 w-5 flex items-center justify-center rounded text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-300/50 dark:hover:bg-zinc-800 transition-colors cursor-pointer"
                >
                  <FilePlus2 className="w-3.5 h-3.5" />
                </button>
              </Tooltip>
              <Tooltip content={`New Folder in ${node.name}`}>
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    startInlineCreation('folder', node.path)
                  }}
                  className="h-5 w-5 flex items-center justify-center rounded text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-300/50 dark:hover:bg-zinc-800 transition-colors cursor-pointer"
                >
                  <FolderPlus className="w-3.5 h-3.5" />
                </button>
              </Tooltip>
            </div>
          </div>

          {isExpanded && (
            <div className="flex flex-col">
              {/* Render inline creation at top of directory */}
              {isCreatingHere && renderInlineInput(depth + 1)}

              {/* Existing child nodes */}
              {node.children && node.children.map((child) => renderNode(child, depth + 1))}
            </div>
          )}
        </div>
      )
    }

    return (
      <button
        key={node.path}
        onClick={() => handleSelectRequest(node)}
        style={{ paddingLeft: `${depth * 12 + 12}px` }}
        className={`w-full flex items-center gap-2 py-1 text-xs rounded transition-colors group text-left cursor-pointer ${
          isSelected
            ? 'bg-blue-50 dark:bg-blue-600/15 text-blue-700 dark:text-blue-200 border-l-2 border-blue-600 dark:border-blue-500 font-medium'
            : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 hover:bg-zinc-200/50 dark:hover:bg-zinc-900/50'
        }`}
      >
        <span
          className={`text-[9px] font-mono font-bold px-1 py-0.5 rounded border uppercase shrink-0 ${getMethodColor(
            node.method || 'GET'
          )}`}
        >
          {node.method || 'GET'}
        </span>
        <span className="truncate flex-1">
          {node.name.replace('.pebble.json', '')}
        </span>
      </button>
    )
  }

  return (
    <aside className="w-64 bg-white dark:bg-zinc-950 border-r border-zinc-200 dark:border-zinc-800 flex flex-col shrink-0 select-none transition-colors duration-150">
      {/* Sidebar Header (VS Code Explorer Actions) */}
      <div className="h-10 px-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400">
        <span className="font-semibold text-zinc-700 dark:text-zinc-300 flex items-center gap-1.5 uppercase text-[11px] tracking-wider">
          <FileCode className="w-3.5 h-3.5 text-blue-500 dark:text-blue-400" />
          Collections
        </span>
        <div className="flex items-center gap-0.5">
          <Tooltip content="New Request (Enter to create, Esc to cancel)">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => startInlineCreation('file')}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/70 dark:hover:bg-zinc-900"
            >
              <FilePlus2 className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="New Folder (Enter to create, Esc to cancel)">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => startInlineCreation('folder')}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/70 dark:hover:bg-zinc-900"
            >
              <FolderPlus className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="Refresh Files from Disk">
            <Button
              variant="ghost"
              size="icon"
              onClick={handleRefresh}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/70 dark:hover:bg-zinc-900"
            >
              {isLoadingWorkspace ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin text-blue-500 dark:text-blue-400" />
              ) : (
                <RefreshCw className="w-3.5 h-3.5" />
              )}
            </Button>
          </Tooltip>
          <Tooltip content="Collapse All Folders">
            <Button
              variant="ghost"
              size="icon"
              onClick={handleCollapseAll}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-200/70 dark:hover:bg-zinc-900"
            >
              <ChevronsDownUp className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* Tree Content */}
      <div className="flex-1 overflow-y-auto p-1.5 space-y-0.5">
        {/* Render at root if target is not inside any rendered directory */}
        {creatingNode &&
          !tree.some((n) => isPathDir([n], creatingNode.parentPath)) &&
          renderInlineInput(0)}

        {tree.length === 0 && !isLoadingWorkspace && !creatingNode ? (
          <div className="p-4 text-center text-xs text-zinc-400 dark:text-zinc-500">
            No requests in workspace.
            <br />
            Click <strong className="text-zinc-700 dark:text-zinc-300">+</strong> above to create one.
          </div>
        ) : (
          tree.map((node) => renderNode(node))
        )}
      </div>

      {/* Git-friendly info footer */}
      <div className="p-2 border-t border-zinc-200 dark:border-zinc-900 text-[10px] text-zinc-500 flex items-center justify-between bg-zinc-50 dark:bg-zinc-950/50">
        <span>Format: *.pebble.json</span>
        <span className="text-emerald-600 dark:text-emerald-500 font-mono font-medium">Git-synced</span>
      </div>
    </aside>
  )
}
