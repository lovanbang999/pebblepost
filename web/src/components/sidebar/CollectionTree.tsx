import { useState, useRef, useEffect, useMemo } from 'react'
import {
  Folder,
  FolderOpen,
  ChevronRight,
  ChevronDown,
  FilePlus2,
  FolderPlus,
  RefreshCw,
  ChevronsDownUp,
  ChevronsUpDown,
} from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { TreeNode, RequestDefinition } from '../../types'
import { getMethodTextColor, cn } from '../../lib/utils'
import { Button } from '../ui/button'
import { Tooltip } from '../ui/tooltip'
import { Skeleton } from '../ui/skeleton'

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

function getAllFolderPaths(nodes: TreeNode[]): string[] {
  const paths: string[] = []
  const traverse = (items: TreeNode[]) => {
    for (const item of items) {
      if (item.isDir) {
        paths.push(item.path)
        if (item.children && item.children.length > 0) {
          traverse(item.children)
        }
      }
    }
  }
  traverse(nodes)
  return paths
}

function CollectionTreeSkeleton() {
  return (
    <div className="p-1 space-y-0.5 animate-in fade-in-50 duration-200">
      {/* Folder 1: 01-auth */}
      <div className="h-7 px-1.5 flex items-center gap-2">
        <Skeleton className="w-3 h-3 rounded-xs shrink-0" />
        <Skeleton className="w-3.5 h-3.5 rounded-xs shrink-0" />
        <Skeleton className="h-3.5 w-20 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-20 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-32 rounded-xs" />
      </div>

      {/* Folder 2: 02-users */}
      <div className="h-7 px-1.5 flex items-center gap-2 pt-1">
        <Skeleton className="w-3 h-3 rounded-xs shrink-0" />
        <Skeleton className="w-3.5 h-3.5 rounded-xs shrink-0" />
        <Skeleton className="h-3.5 w-22 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-24 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-20 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-28 rounded-xs" />
      </div>

      {/* Folder 3: 03-products */}
      <div className="h-7 px-1.5 flex items-center gap-2 pt-1">
        <Skeleton className="w-3 h-3 rounded-xs shrink-0" />
        <Skeleton className="w-3.5 h-3.5 rounded-xs shrink-0" />
        <Skeleton className="h-3.5 w-24 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-36 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-24 rounded-xs" />
      </div>

      {/* Folder 4: 04-httpbin-advanced */}
      <div className="h-7 px-1.5 flex items-center gap-2 pt-1">
        <Skeleton className="w-3 h-3 rounded-xs shrink-0" />
        <Skeleton className="w-3.5 h-3.5 rounded-xs shrink-0" />
        <Skeleton className="h-3.5 w-32 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-32 rounded-xs" />
      </div>
      <div className="h-7 pl-6 pr-1.5 flex items-center gap-2">
        <Skeleton className="h-3 w-10 rounded-xs shrink-0" />
        <Skeleton className="h-3 w-24 rounded-xs" />
      </div>
    </div>
  )
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

  // Auto-expand loaded folders on mount & updates
  useEffect(() => {
    if (tree.length > 0) {
      setExpandedFolders((prev) => {
        const next = { ...prev }
        const expandRecursive = (nodes: TreeNode[]) => {
          nodes.forEach((node) => {
            if (node.isDir) {
              if (next[node.path] === undefined) {
                next[node.path] = true
              }
              if (node.children && node.children.length > 0) {
                expandRecursive(node.children)
              }
            }
          })
        }
        expandRecursive(tree)
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

  const allFolderPaths = useMemo(() => getAllFolderPaths(tree), [tree])

  // Check if at least one folder is currently expanded
  const isAnyFolderExpanded =
    allFolderPaths.length > 0 &&
    allFolderPaths.some((path) => (expandedFolders[path] ?? true) === true)

  const toggleFolder = (path: string) => {
    setExpandedFolders((prev) => ({
      ...prev,
      [path]: !(prev[path] ?? true),
    }))
  }

  const handleSelectRequest = (node: TreeNode) => {
    setSelectedPath(node.path)
    loadRequest(node.path)
  }

  const [isRefreshing, setIsRefreshing] = useState(false)

  const handleRefresh = async () => {
    setIsRefreshing(true)
    try {
      await loadWorkspace(workspacePath || '.')
    } finally {
      setTimeout(() => {
        setIsRefreshing(false)
      }, 500)
    }
  }

  const handleToggleExpandCollapseAll = () => {
    const next: Record<string, boolean> = {}
    const shouldExpand = !isAnyFolderExpanded
    allFolderPaths.forEach((path) => {
      next[path] = shouldExpand
    })
    setExpandedFolders(next)
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
          body: JSON.stringify({ workspacePath: workspacePath || '.', path: fullFolderPath }),
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
        style={{ paddingLeft: `${depth * 14 + 10}px` }}
        className="w-full flex items-center gap-2 py-0.5 pr-2 my-0.5 animate-in fade-in duration-100"
      >
        {isFolder ? (
          <Folder className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-500 shrink-0" />
        ) : (
          <span
            className={cn(
              'w-10 text-[10px] font-mono font-bold tracking-tight uppercase shrink-0 text-left transition-colors',
              getMethodTextColor(parsed?.method || 'GET')
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
          className="flex-1 h-6 px-1.5 py-0 text-xs font-mono bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 border border-zinc-300 dark:border-zinc-700 rounded focus:outline-none focus:border-zinc-500 dark:focus:border-zinc-400 ring-1 ring-zinc-400/20 shadow-2xs"
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
              'w-full flex items-center justify-between group py-1 px-1.5 rounded-md transition-colors cursor-pointer',
              selectedPath === node.path
                ? 'bg-zinc-200/60 dark:bg-zinc-800/70 text-zinc-900 dark:text-zinc-100'
                : 'hover:bg-zinc-100 dark:hover:bg-zinc-900/60 text-zinc-700 dark:text-zinc-300'
            )}
            style={{ paddingLeft: `${depth * 14 + 8}px` }}
          >
            <button
              onClick={() => toggleFolder(node.path)}
              className="flex-1 flex items-center gap-1.5 py-0.5 text-xs rounded truncate cursor-pointer text-left"
            >
              {isExpanded ? (
                <ChevronDown className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-500 shrink-0" />
              ) : (
                <ChevronRight className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-500 shrink-0" />
              )}
              {isExpanded ? (
                <FolderOpen className="w-3.5 h-3.5 text-zinc-400 dark:text-zinc-500 group-hover:text-zinc-600 dark:group-hover:text-zinc-300 transition-colors shrink-0" />
              ) : (
                <Folder className="w-3.5 h-3.5 text-zinc-400/90 dark:text-zinc-500/90 group-hover:text-zinc-600 dark:group-hover:text-zinc-300 transition-colors shrink-0" />
              )}
              <span className="truncate font-medium text-xs text-zinc-700 dark:text-zinc-300 group-hover:text-zinc-900 dark:group-hover:text-zinc-100 transition-colors">
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
                  className="h-5 w-5 flex items-center justify-center rounded text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-200/80 dark:hover:bg-zinc-800 transition-colors cursor-pointer"
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
                  className="h-5 w-5 flex items-center justify-center rounded text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-200/80 dark:hover:bg-zinc-800 transition-colors cursor-pointer"
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
        style={{ paddingLeft: `${depth * 14 + 10}px` }}
        className={cn(
          'w-full flex items-center gap-2 py-1 px-1.5 text-xs rounded-md transition-all group text-left cursor-pointer select-none',
          isSelected
            ? 'bg-zinc-200/70 dark:bg-zinc-800/80 text-zinc-900 dark:text-zinc-100 font-medium shadow-2xs'
            : 'text-zinc-600 dark:text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900/60'
        )}
      >
        <span
          className={cn(
            'w-10 text-[10px] font-mono font-bold tracking-tight uppercase shrink-0 text-left transition-colors',
            getMethodTextColor(node.method || 'GET')
          )}
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
      {/* Sidebar Header (Developer Tool Actions) */}
      <div className="h-9 px-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400">
        <span className="font-semibold text-zinc-500 dark:text-zinc-400 tracking-wider uppercase text-[10.5px]">
          Collections
        </span>
        <div className="flex items-center gap-0.5">
          <Tooltip content="New Request (Enter to create, Esc to cancel)">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => startInlineCreation('file')}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              <FilePlus2 className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="New Folder (Enter to create, Esc to cancel)">
            <Button
              variant="ghost"
              size="icon"
              onClick={() => startInlineCreation('folder')}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              <FolderPlus className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="Refresh Files from Disk">
            <Button
              variant="ghost"
              size="icon"
              onClick={handleRefresh}
              disabled={isRefreshing || isLoadingWorkspace}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900 disabled:opacity-70"
            >
              <RefreshCw
                className={cn(
                  "w-3.5 h-3.5 transition-all duration-300",
                  (isRefreshing || isLoadingWorkspace) && "animate-spin text-zinc-700 dark:text-zinc-300"
                )}
              />
            </Button>
          </Tooltip>
          <Tooltip content={isAnyFolderExpanded ? 'Collapse All Folders' : 'Expand All Folders'}>
            <Button
              variant="ghost"
              size="icon"
              onClick={handleToggleExpandCollapseAll}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              {isAnyFolderExpanded ? (
                <ChevronsDownUp className="w-3.5 h-3.5" />
              ) : (
                <ChevronsUpDown className="w-3.5 h-3.5" />
              )}
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* Tree Content */}
      <div className="flex-1 overflow-y-auto p-1.5 space-y-0.5">
        {isRefreshing || isLoadingWorkspace ? (
          <CollectionTreeSkeleton />
        ) : (
          <>
            {/* Render at root if target is not inside any rendered directory */}
            {creatingNode &&
              !tree.some((n) => isPathDir([n], creatingNode.parentPath)) &&
              renderInlineInput(0)}

            {tree.length === 0 && !creatingNode ? (
              <div className="p-4 text-center text-xs text-zinc-400 dark:text-zinc-500">
                No requests in workspace.
                <br />
                Click <strong className="text-zinc-700 dark:text-zinc-300">+</strong> above to create one.
              </div>
            ) : (
              tree.map((node) => renderNode(node))
            )}
          </>
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
