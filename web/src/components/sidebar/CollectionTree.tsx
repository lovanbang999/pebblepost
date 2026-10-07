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
  Copy,
  Trash2,
  Edit3,
  Files,
  ExternalLink,
  AlertTriangle,
  Settings,
  Play,
  BookOpen,
  MoreHorizontal,
  Upload,
  Download,
  Server,
} from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import { useTabStore } from '../../store/tabStore'
import type { TreeNode, RequestDefinition } from '../../types'
import { getMethodTextColor, cn } from '../../lib/utils'
import { Button } from '../ui/button'
import { Tooltip } from '../ui/tooltip'
import { Skeleton } from '../ui/skeleton'
import { ImportDialog } from '../common/ImportDialog'
import { ExportDialog } from '../common/ExportDialog'
import { GitBadge } from '../git/GitBadge'
import { GitPanel } from '../git/GitPanel'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '../ui/dialog'

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

function findNodeByPath(nodes: TreeNode[], targetPath: string): TreeNode | null {
  for (const node of nodes) {
    if (node.path === targetPath) return node
    if (node.children) {
      const found = findNodeByPath(node.children, targetPath)
      if (found) return found
    }
  }
  return null
}

function CollectionTreeSkeleton() {
  return (
    <div className="p-1 space-y-0.5 animate-in fade-in-50 duration-200">
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
    </div>
  )
}

export function CollectionTree() {
  const {
    tree,
    workspacePath,
    activeFilePath,
    isLoadingWorkspace,
    loadWorkspace,
    createNewRequest,
  } = useWorkspaceStore()

  const { openTab, openFolderTab, openRunnerTab, openDocsTab, openMockTab, onFileRenamed, onFileDeleted } = useTabStore()

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

  // F2 inline renaming state
  const [renamingPath, setRenamingPath] = useState<string | null>(null)
  const [renameValue, setRenameValue] = useState('')
  const renameInputRef = useRef<HTMLInputElement>(null)

  // Context menu state
  const [contextMenu, setContextMenu] = useState<{
    x: number
    y: number
    node: TreeNode
  } | null>(null)
  const menuRef = useRef<HTMLDivElement>(null)

  // Delete confirmation modal state
  const [deleteConfirmNode, setDeleteConfirmNode] = useState<TreeNode | null>(null)

  // Secondary actions overflow menu state
  const [isMoreMenuOpen, setIsMoreMenuOpen] = useState(false)
  const moreMenuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      if (moreMenuRef.current && !moreMenuRef.current.contains(e.target as Node)) {
        setIsMoreMenuOpen(false)
      }
    }
    if (isMoreMenuOpen) {
      document.addEventListener('mousedown', handleOutsideClick)
      return () => document.removeEventListener('mousedown', handleOutsideClick)
    }
  }, [isMoreMenuOpen])

  // Drag & drop state
  const [dragOverFolderPath, setDragOverFolderPath] = useState<string | null>(null)

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

  // Focus and select input on creation
  useEffect(() => {
    if (creatingNode) {
      const timer = setTimeout(() => {
        inlineInputRef.current?.focus()
        inlineInputRef.current?.select()
      }, 20)
      return () => clearTimeout(timer)
    }
  }, [creatingNode])

  // Close context menu on outside click or scroll
  useEffect(() => {
    const handleOutsideClick = (e: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
        setContextMenu(null)
      }
    }
    const handleScroll = () => setContextMenu(null)

    if (contextMenu) {
      window.addEventListener('mousedown', handleOutsideClick)
      window.addEventListener('scroll', handleScroll, true)
    }
    return () => {
      window.removeEventListener('mousedown', handleOutsideClick)
      window.removeEventListener('scroll', handleScroll, true)
    }
  }, [contextMenu])

  // F2 global keydown listener
  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'F2' && selectedPath && !renamingPath && !creatingNode) {
        e.preventDefault()
        const node = findNodeByPath(tree, selectedPath)
        if (node) {
          startInlineRename(node)
        }
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [selectedPath, renamingPath, creatingNode, tree])

  const allFolderPaths = useMemo(() => getAllFolderPaths(tree), [tree])

  const isAnyFolderExpanded =
    allFolderPaths.length > 0 &&
    allFolderPaths.some((path) => (expandedFolders[path] ?? true) === true)

  const toggleFolder = (path: string) => {
    setExpandedFolders((prev) => ({
      ...prev,
      [path]: !(prev[path] ?? true),
    }))
  }

  // Handle single click (preview tab) and double click (pinned tab)
  const handleSelectRequest = async (node: TreeNode, isPreview = true) => {
    setSelectedPath(node.path)
    try {
      const ws = workspacePath || '.'
      const res = await fetch(
        `/api/request?path=${encodeURIComponent(node.path)}&workspacePath=${encodeURIComponent(ws)}`
      )
      if (res.ok) {
        const req: RequestDefinition = await res.json()
        openTab(node.path, req, isPreview)
      }
    } catch (err) {
      console.error('Failed to open request tab:', err)
    }
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

  const isPathDir = (nodes: TreeNode[], targetPath: string): boolean => {
    for (const node of nodes) {
      if (node.path === targetPath) return node.isDir
      if (node.children && isPathDir(node.children, targetPath)) return true
    }
    return false
  }

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

  const startInlineCreation = (type: 'file' | 'folder', explicitParent?: string) => {
    const targetDir = getTargetDirectory(explicitParent)
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
          const ws = workspacePath || '.'
          const res = await fetch(
            `/api/request?path=${encodeURIComponent(createdPath)}&workspacePath=${encodeURIComponent(ws)}`
          )
          if (res.ok) {
            const req = await res.json()
            openTab(createdPath, req, false)
          }
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

  // Renaming handlers
  const startInlineRename = (node: TreeNode) => {
    const clean = node.isDir
      ? node.name
      : node.name.replace(/\.pebble\.json$/, '').replace(/\.json$/, '')
    setRenamingPath(node.path)
    setRenameValue(clean)
    setContextMenu(null)
    setTimeout(() => {
      renameInputRef.current?.focus()
      renameInputRef.current?.select()
    }, 30)
  }

  const cancelInlineRename = () => {
    setRenamingPath(null)
    setRenameValue('')
  }

  const commitInlineRename = async (node: TreeNode) => {
    const trimmed = renameValue.trim()
    if (!trimmed) {
      cancelInlineRename()
      return
    }

    const lastSlash = node.path.lastIndexOf('/')
    const parentDir = lastSlash > 0 ? node.path.substring(0, lastSlash) : ''
    const newFileName = node.isDir ? trimmed : `${trimmed}.pebble.json`
    const newFullPath = parentDir ? `${parentDir}/${newFileName}` : newFileName

    if (newFullPath === node.path) {
      cancelInlineRename()
      return
    }

    setRenamingPath(null)

    try {
      const res = await fetch('/api/workspace/rename', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath: workspacePath || '.',
          oldPath: node.path,
          newPath: newFullPath,
        }),
      })

      if (res.ok) {
        onFileRenamed(node.path, newFullPath, trimmed)
        await loadWorkspace(workspacePath || '.')
        setSelectedPath(newFullPath)
      }
    } catch (err) {
      console.error('Failed to rename item:', err)
    }
  }

  // Duplicate handler
  const handleDuplicate = async (node: TreeNode) => {
    setContextMenu(null)
    try {
      const res = await fetch('/api/workspace/duplicate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath: workspacePath || '.',
          path: node.path,
        }),
      })
      if (res.ok) {
        const data = await res.json()
        await loadWorkspace(workspacePath || '.')
        if (data.newPath) {
          const reqRes = await fetch(
            `/api/request?path=${encodeURIComponent(data.newPath)}&workspacePath=${encodeURIComponent(workspacePath || '.')}`
          )
          if (reqRes.ok) {
            const req = await reqRes.json()
            openTab(data.newPath, req, false)
            setSelectedPath(data.newPath)
          }
        }
      }
    } catch (err) {
      console.error('Failed to duplicate request:', err)
    }
  }

  // Delete handler
  const handleDeleteConfirm = async () => {
    if (!deleteConfirmNode) return
    const node = deleteConfirmNode
    setDeleteConfirmNode(null)
    setContextMenu(null)

    try {
      const res = await fetch('/api/request/delete', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath: workspacePath || '.',
          path: node.path,
          permanent: false,
        }),
      })

      if (res.ok) {
        onFileDeleted(node.path)
        await loadWorkspace(workspacePath || '.')
        if (selectedPath === node.path) {
          setSelectedPath(null)
        }
      }
    } catch (err) {
      console.error('Failed to delete item:', err)
    }
  }

  // Drag & drop handlers
  const handleDragStart = (e: React.DragEvent, node: TreeNode) => {
    e.dataTransfer.setData('text/plain', node.path)
    e.dataTransfer.effectAllowed = 'move'
  }

  const handleDragOver = (e: React.DragEvent, folderNode: TreeNode) => {
    if (!folderNode.isDir) return
    e.preventDefault()
    e.dataTransfer.dropEffect = 'move'
    setDragOverFolderPath(folderNode.path)
  }

  const handleDragLeave = (e: React.DragEvent) => {
    e.preventDefault()
    setDragOverFolderPath(null)
  }

  const handleDrop = async (e: React.DragEvent, targetFolderNode: TreeNode) => {
    e.preventDefault()
    setDragOverFolderPath(null)
    const sourcePath = e.dataTransfer.getData('text/plain')
    if (!sourcePath || sourcePath === targetFolderNode.path) return

    // Prevent dropping into subfolder of itself
    if (targetFolderNode.path.startsWith(sourcePath + '/')) return

    try {
      const res = await fetch('/api/workspace/move', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath: workspacePath || '.',
          sourcePath,
          targetFolder: targetFolderNode.path,
        }),
      })

      if (res.ok) {
        const data = await res.json()
        if (data.newPath) {
          onFileRenamed(sourcePath, data.newPath)
        }
        await loadWorkspace(workspacePath || '.')
        setExpandedFolders((prev) => ({ ...prev, [targetFolderNode.path]: true }))
      }
    } catch (err) {
      console.error('Failed to move item:', err)
    }
  }

  const handleContextMenu = (e: React.MouseEvent, node: TreeNode) => {
    e.preventDefault()
    e.stopPropagation()
    setSelectedPath(node.path)
    setContextMenu({
      x: e.clientX,
      y: e.clientY,
      node,
    })
  }

  const copyPathToClipboard = (path: string) => {
    navigator.clipboard.writeText(path)
    setContextMenu(null)
  }

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
          onKeyDown={(e) => {
            if (e.key === 'Enter') {
              e.preventDefault()
              commitInlineCreation()
            } else if (e.key === 'Escape') {
              e.preventDefault()
              cancelInlineCreation()
            }
          }}
          onBlur={() => {
            if (inlineName.trim()) {
              commitInlineCreation()
            } else {
              cancelInlineCreation()
            }
          }}
          placeholder={isFolder ? 'folder-name' : 'request-name (or: post login)'}
          className="flex-1 h-6 px-1.5 py-0 text-xs font-mono bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 border border-zinc-300 dark:border-zinc-700 rounded focus:outline-none focus:border-zinc-500 dark:focus:border-zinc-400 ring-1 ring-zinc-400/20 shadow-2xs"
        />
      </div>
    )
  }

  function renderNode(node: TreeNode, depth = 0) {
    const isExpanded = expandedFolders[node.path] ?? true
    const isSelected = selectedPath === node.path
    const isCreatingHere = creatingNode?.parentPath === node.path
    const isRenaming = renamingPath === node.path
    const isDragOver = dragOverFolderPath === node.path

    if (node.isDir) {
      return (
        <div key={node.path} className="select-none">
          <div
            onClick={() => setSelectedPath(node.path)}
            onContextMenu={(e) => handleContextMenu(e, node)}
            draggable={!isRenaming}
            onDragStart={(e) => handleDragStart(e, node)}
            onDragOver={(e) => handleDragOver(e, node)}
            onDragLeave={handleDragLeave}
            onDrop={(e) => handleDrop(e, node)}
            className={cn(
              'w-full flex items-center justify-between group py-1 px-1.5 rounded-md transition-colors cursor-pointer',
              isDragOver
                ? 'bg-blue-100/70 dark:bg-blue-950/60 ring-2 ring-blue-500'
                : isSelected
                ? 'bg-zinc-200/60 dark:bg-zinc-800/70 text-zinc-900 dark:text-zinc-100'
                : 'hover:bg-zinc-100 dark:hover:bg-zinc-900/60 text-zinc-700 dark:text-zinc-300'
            )}
            style={{ paddingLeft: `${depth * 14 + 8}px` }}
          >
            {isRenaming ? (
              <div className="flex-1 flex items-center gap-1.5 py-0.5">
                <Folder className="w-3.5 h-3.5 text-zinc-400 shrink-0" />
                <input
                  ref={renameInputRef}
                  type="text"
                  value={renameValue}
                  onChange={(e) => setRenameValue(e.target.value)}
                  onKeyDown={(e) => {
                    if (e.key === 'Enter') {
                      e.preventDefault()
                      commitInlineRename(node)
                    } else if (e.key === 'Escape') {
                      e.preventDefault()
                      cancelInlineRename()
                    }
                  }}
                  onBlur={() => commitInlineRename(node)}
                  className="flex-1 h-6 px-1.5 py-0 text-xs font-mono bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 border border-zinc-400 dark:border-zinc-600 rounded focus:outline-none"
                />
              </div>
            ) : (
              <button
                onClick={() => {
                  toggleFolder(node.path)
                  openFolderTab(node.path, undefined, true)
                }}
                onDoubleClick={(e) => {
                  e.stopPropagation()
                  openFolderTab(node.path, undefined, false)
                }}
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
                  {node.displayName || node.name}
                </span>
                {node.gitStatus && (
                  <span
                    className={cn(
                      'text-[9px] font-mono font-bold px-1 py-0.2 rounded shrink-0 ml-1',
                      node.gitStatus === 'M' && 'text-amber-600 dark:text-amber-400 bg-amber-500/10',
                      node.gitStatus === 'A' && 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10',
                      node.gitStatus === 'D' && 'text-rose-600 dark:text-rose-400 bg-rose-500/10',
                      node.gitStatus === '?' && 'text-zinc-500 bg-zinc-500/10'
                    )}
                    title={`Git status: ${node.gitStatus}`}
                  >
                    {node.gitStatus}
                  </span>
                )}
              </button>
            )}

            {/* Quick folder action buttons on hover */}
            <div className="opacity-0 group-hover:opacity-100 flex items-center gap-0.5 transition-opacity">
              <Tooltip content="Folder Settings">
                <button
                  type="button"
                  onClick={(e) => {
                    e.stopPropagation()
                    openFolderTab(node.path, undefined, false)
                  }}
                  className="h-5 w-5 flex items-center justify-center rounded text-zinc-400 hover:text-zinc-900 dark:hover:text-zinc-100 hover:bg-zinc-200/80 dark:hover:bg-zinc-800 transition-colors cursor-pointer"
                >
                  <Settings className="w-3.5 h-3.5" />
                </button>
              </Tooltip>
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
              {isCreatingHere && renderInlineInput(depth + 1)}
              {node.children && node.children.map((child) => renderNode(child, depth + 1))}
            </div>
          )}
        </div>
      )
    }

    return (
      <div
        key={node.path}
        style={{ paddingLeft: `${depth * 14 + 10}px` }}
        className="w-full select-none"
      >
        {isRenaming ? (
          <div className="flex items-center gap-1.5 py-0.5 pr-2">
            <span
              className={cn(
                'w-10 text-[10px] font-mono font-bold tracking-tight uppercase shrink-0 text-left',
                getMethodTextColor(node.method || 'GET')
              )}
            >
              {node.method || 'GET'}
            </span>
            <input
              ref={renameInputRef}
              type="text"
              value={renameValue}
              onChange={(e) => setRenameValue(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') {
                  e.preventDefault()
                  commitInlineRename(node)
                } else if (e.key === 'Escape') {
                  e.preventDefault()
                  cancelInlineRename()
                }
              }}
              onBlur={() => commitInlineRename(node)}
              className="flex-1 h-6 px-1.5 py-0 text-xs font-mono bg-white dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100 border border-zinc-400 dark:border-zinc-600 rounded focus:outline-none"
            />
          </div>
        ) : (
          <button
            onClick={() => handleSelectRequest(node, true)}
            onDoubleClick={() => handleSelectRequest(node, false)}
            onContextMenu={(e) => handleContextMenu(e, node)}
            draggable={!isRenaming}
            onDragStart={(e) => handleDragStart(e, node)}
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
              {(node.displayName || node.name).replace('.pebble.json', '')}
            </span>
            {node.gitStatus && (
              <span
                className={cn(
                  'text-[9px] font-mono font-bold px-1 py-0.2 rounded shrink-0 ml-1',
                  node.gitStatus === 'M' && 'text-amber-600 dark:text-amber-400 bg-amber-500/10',
                  node.gitStatus === 'A' && 'text-emerald-600 dark:text-emerald-400 bg-emerald-500/10',
                  node.gitStatus === 'D' && 'text-rose-600 dark:text-rose-400 bg-rose-500/10',
                  node.gitStatus === '?' && 'text-zinc-500 bg-zinc-500/10'
                )}
                title={`Git status: ${node.gitStatus}`}
              >
                {node.gitStatus}
              </span>
            )}
          </button>
        )}
      </div>
    )
  }

  return (
    <aside className="w-64 bg-white dark:bg-zinc-950 border-r border-zinc-200 dark:border-zinc-800 flex flex-col shrink-0 select-none transition-colors duration-150">
      {/* Sidebar Header */}
      <div className="h-9 px-3 border-b border-zinc-200 dark:border-zinc-800 flex items-center justify-between text-xs text-zinc-500 dark:text-zinc-400">
        <span className="font-semibold text-zinc-500 dark:text-zinc-400 tracking-wider uppercase text-[10.5px]">
          Collections
        </span>
        <div className="flex items-center gap-0.5">
          <Tooltip content="New Request (Enter to create, Esc to cancel)">
            <Button
              aria-label="New Request"
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
              aria-label="New Folder"
              variant="ghost"
              size="icon"
              onClick={() => startInlineCreation('folder')}
              className="h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900"
            >
              <FolderPlus className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="Collection Runner">
            <Button
              aria-label="Collection Runner"
              variant="ghost"
              size="icon"
              onClick={() => openRunnerTab()}
              className="h-6 w-6 text-emerald-600 hover:text-emerald-700 dark:text-emerald-400 dark:hover:text-emerald-300 hover:bg-emerald-50 dark:hover:bg-emerald-950/30"
            >
              <Play className="w-3.5 h-3.5 fill-current" />
            </Button>
          </Tooltip>

          {/* Secondary Actions Overflow Menu */}
          <div className="relative" ref={moreMenuRef}>
            <Tooltip content="More Actions">
              <Button
                aria-label="More Actions"
                variant="ghost"
                size="icon"
                onClick={() => setIsMoreMenuOpen((v) => !v)}
                className={cn(
                  'h-6 w-6 text-zinc-500 hover:text-zinc-900 dark:text-zinc-400 dark:hover:text-zinc-200 hover:bg-zinc-100 dark:hover:bg-zinc-900',
                  isMoreMenuOpen && 'bg-zinc-100 dark:bg-zinc-900 text-zinc-900 dark:text-zinc-100'
                )}
              >
                <MoreHorizontal className="w-3.5 h-3.5" />
              </Button>
            </Tooltip>

            {isMoreMenuOpen && (
              <div className="absolute right-0 top-7 z-50 w-52 py-1 bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-md shadow-lg text-xs animate-in fade-in zoom-in-95 duration-100">
                <button
                  type="button"
                  onClick={() => {
                    handleRefresh()
                    setIsMoreMenuOpen(false)
                  }}
                  disabled={isRefreshing || isLoadingWorkspace}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors text-left disabled:opacity-50 cursor-pointer"
                >
                  <RefreshCw
                    className={cn(
                      'w-3.5 h-3.5',
                      (isRefreshing || isLoadingWorkspace) && 'animate-spin text-zinc-700 dark:text-zinc-300'
                    )}
                  />
                  <span>Refresh Files from Disk</span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    handleToggleExpandCollapseAll()
                    setIsMoreMenuOpen(false)
                  }}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors text-left cursor-pointer"
                >
                  {isAnyFolderExpanded ? (
                    <ChevronsDownUp className="w-3.5 h-3.5" />
                  ) : (
                    <ChevronsUpDown className="w-3.5 h-3.5" />
                  )}
                  <span>{isAnyFolderExpanded ? 'Collapse All Folders' : 'Expand All Folders'}</span>
                </button>

                <div className="my-1 border-t border-zinc-200 dark:border-zinc-800" />

                <ImportDialog
                  trigger={
                    <button
                      type="button"
                      onClick={() => setIsMoreMenuOpen(false)}
                      className="w-full flex items-center gap-2 px-2.5 py-1.5 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors text-left cursor-pointer"
                    >
                      <Upload className="w-3.5 h-3.5" />
                      <span>Import Request / Collection...</span>
                    </button>
                  }
                />
                <ExportDialog
                  trigger={
                    <button
                      type="button"
                      onClick={() => setIsMoreMenuOpen(false)}
                      className="w-full flex items-center gap-2 px-2.5 py-1.5 text-zinc-700 dark:text-zinc-300 hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors text-left cursor-pointer"
                    >
                      <Download className="w-3.5 h-3.5" />
                      <span>Export Collection...</span>
                    </button>
                  }
                />

                <div className="my-1 border-t border-zinc-200 dark:border-zinc-800" />

                <button
                  type="button"
                  onClick={() => {
                    openDocsTab()
                    setIsMoreMenuOpen(false)
                  }}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 text-blue-600 dark:text-blue-400 hover:bg-blue-50 dark:hover:bg-blue-950/30 transition-colors text-left cursor-pointer"
                >
                  <BookOpen className="w-3.5 h-3.5" />
                  <span>API Documentation</span>
                </button>
                <button
                  type="button"
                  onClick={() => {
                    openMockTab()
                    setIsMoreMenuOpen(false)
                  }}
                  className="w-full flex items-center gap-2 px-2.5 py-1.5 text-teal-600 dark:text-teal-400 hover:bg-teal-50 dark:hover:bg-teal-950/30 transition-colors text-left cursor-pointer"
                >
                  <Server className="w-3.5 h-3.5" />
                  <span>Mock Server...</span>
                </button>
              </div>
            )}
          </div>
        </div>
      </div>

      {/* Tree Content */}
      <div className="flex-1 overflow-y-auto p-1.5 space-y-0.5">
        {isRefreshing || isLoadingWorkspace ? (
          <CollectionTreeSkeleton />
        ) : (
          <>
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

      {/* Footer / Status Bar */}
      <div className="h-9 px-3 border-t border-zinc-200 dark:border-zinc-800 text-xs flex items-center justify-between bg-zinc-50/80 dark:bg-zinc-900/50 select-none shrink-0">
        <GitBadge workspacePath={workspacePath} />
      </div>

      {/* Context Menu Popup */}
      {contextMenu && (
        <div
          ref={menuRef}
          style={{ top: `${contextMenu.y}px`, left: `${contextMenu.x}px` }}
          className="fixed z-50 min-w-44 py-1 bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-800 rounded-lg shadow-xl text-xs text-zinc-700 dark:text-zinc-200 animate-in fade-in zoom-in-95 duration-100"
        >
          {contextMenu.node.isDir ? (
            <>
              <button
                type="button"
                onClick={() => {
                  openFolderTab(contextMenu.node.path, undefined, false)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer font-medium"
              >
                <Folder className="w-3.5 h-3.5 text-amber-500" />
                <span>Folder Settings</span>
              </button>
              <div className="h-px bg-zinc-200 dark:bg-zinc-800 my-1" />
              <button
                type="button"
                onClick={() => {
                  startInlineCreation('file', contextMenu.node.path)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <FilePlus2 className="w-3.5 h-3.5 text-zinc-400" />
                <span>New Request</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  startInlineCreation('folder', contextMenu.node.path)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <FolderPlus className="w-3.5 h-3.5 text-zinc-400" />
                <span>New Folder</span>
              </button>
              <div className="h-px bg-zinc-200 dark:bg-zinc-800 my-1" />
              <button
                type="button"
                onClick={() => {
                  openDocsTab(contextMenu.node.path, contextMenu.node.displayName || contextMenu.node.name)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-blue-50 dark:hover:bg-blue-950/30 text-blue-600 dark:text-blue-400 text-left cursor-pointer font-medium"
              >
                <BookOpen className="w-3.5 h-3.5" />
                <span>View Documentation</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  openRunnerTab(contextMenu.node.path, contextMenu.node.displayName || contextMenu.node.name)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-emerald-50 dark:hover:bg-emerald-950/30 text-emerald-600 dark:text-emerald-400 text-left cursor-pointer font-medium"
              >
                <Play className="w-3.5 h-3.5 fill-current" />
                <span>Run Folder</span>
              </button>
              <button
                type="button"
                onClick={() => {
                  openMockTab(contextMenu.node.path, contextMenu.node.displayName || contextMenu.node.name)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-teal-50 dark:hover:bg-teal-950/30 text-teal-600 dark:text-teal-400 text-left cursor-pointer font-medium"
              >
                <Server className="w-3.5 h-3.5" />
                <span>Start Mock Server</span>
              </button>
              <div className="h-px bg-zinc-200 dark:bg-zinc-800 my-1" />
              <button
                type="button"
                onClick={() => startInlineRename(contextMenu.node)}
                className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <span className="flex items-center gap-2">
                  <Edit3 className="w-3.5 h-3.5 text-zinc-400" /> Rename
                </span>
                <span className="text-[10px] text-zinc-400">F2</span>
              </button>
              <button
                type="button"
                onClick={() => copyPathToClipboard(contextMenu.node.path)}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <Copy className="w-3.5 h-3.5 text-zinc-400" /> Copy Path
              </button>
              <div className="h-px bg-zinc-200 dark:bg-zinc-800 my-1" />
              <button
                type="button"
                onClick={() => {
                  setDeleteConfirmNode(contextMenu.node)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-rose-50 dark:hover:bg-rose-950/40 text-rose-600 dark:text-rose-400 text-left cursor-pointer"
              >
                <Trash2 className="w-3.5 h-3.5" /> Delete Folder
              </button>
            </>
          ) : (
            <>
              <button
                type="button"
                onClick={() => {
                  handleSelectRequest(contextMenu.node, false)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <ExternalLink className="w-3.5 h-3.5 text-zinc-400" /> Open in New Tab
              </button>
              <button
                type="button"
                onClick={() => startInlineRename(contextMenu.node)}
                className="w-full flex items-center justify-between px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <span className="flex items-center gap-2">
                  <Edit3 className="w-3.5 h-3.5 text-zinc-400" /> Rename
                </span>
                <span className="text-[10px] text-zinc-400">F2</span>
              </button>
              <button
                type="button"
                onClick={() => handleDuplicate(contextMenu.node)}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <Files className="w-3.5 h-3.5 text-zinc-400" /> Duplicate
              </button>
              <button
                type="button"
                onClick={() => copyPathToClipboard(contextMenu.node.path)}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-zinc-100 dark:hover:bg-zinc-800 text-left cursor-pointer"
              >
                <Copy className="w-3.5 h-3.5 text-zinc-400" /> Copy Path
              </button>
              <div className="h-px bg-zinc-200 dark:bg-zinc-800 my-1" />
              <button
                type="button"
                onClick={() => {
                  setDeleteConfirmNode(contextMenu.node)
                  setContextMenu(null)
                }}
                className="w-full flex items-center gap-2 px-3 py-1.5 hover:bg-rose-50 dark:hover:bg-rose-950/40 text-rose-600 dark:text-rose-400 text-left cursor-pointer"
              >
                <Trash2 className="w-3.5 h-3.5" /> Delete Request
              </button>
            </>
          )}
        </div>
      )}

      {/* Delete Confirmation Modal */}
      {deleteConfirmNode && (
        <Dialog open={true} onOpenChange={(open) => !open && setDeleteConfirmNode(null)}>
          <DialogContent className="max-w-md p-6">
            <DialogHeader>
              <div className="flex items-center gap-2 text-rose-600 mb-1">
                <AlertTriangle className="w-5 h-5 shrink-0" />
                <DialogTitle className="text-base font-semibold text-zinc-900 dark:text-zinc-100">
                  {deleteConfirmNode.isDir ? 'Delete Folder' : 'Delete Request'}
                </DialogTitle>
              </div>
              <DialogDescription className="text-xs text-zinc-600 dark:text-zinc-400 leading-relaxed">
                Are you sure you want to delete{' '}
                <strong className="text-zinc-900 dark:text-zinc-100">
                  "{deleteConfirmNode.displayName || deleteConfirmNode.name}"
                </strong>
                ?
              </DialogDescription>
            </DialogHeader>

            <p className="text-[11px] text-zinc-500 dark:text-zinc-400">
              The item will be moved to <code className="font-mono text-zinc-700 dark:text-zinc-300">.pebble/trash/</code> and any associated open tabs will be closed.
            </p>

            <DialogFooter className="flex items-center justify-end gap-2 pt-3">
              <Button
                type="button"
                variant="ghost"
                size="sm"
                onClick={() => setDeleteConfirmNode(null)}
                className="text-xs"
              >
                Cancel
              </Button>
              <Button
                type="button"
                variant="default"
                size="sm"
                onClick={handleDeleteConfirm}
                className="text-xs font-semibold bg-rose-600 hover:bg-rose-500 text-white"
              >
                Delete
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      )}

      {/* Slide-in Git Panel */}
      <GitPanel workspacePath={workspacePath} />
    </aside>
  )
}
