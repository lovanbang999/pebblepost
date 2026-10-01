import React, { useState } from 'react'
import {
  Folder,
  FolderOpen,
  FileCode,
  ChevronRight,
  ChevronDown,
  Plus,
  FolderPlus,
  RefreshCw,
  Loader2,
} from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { TreeNode } from '../../types'
import { getMethodColor } from '../../lib/utils'
import { Button } from '../ui/button'
import { Tooltip } from '../ui/tooltip'

export const CollectionTree: React.FC = () => {
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
    'collections/example': true,
  })

  const toggleFolder = (path: string) => {
    setExpandedFolders((prev) => ({ ...prev, [path]: !prev[path] }))
  }

  const handleSelectRequest = (node: TreeNode) => {
    loadRequest(node.path)
  }

  const handleNewRequest = async () => {
    const name = prompt('Enter request name (e.g. get-user-profile):', 'new-request')
    if (name) {
      await createNewRequest(undefined, name)
    }
  }

  const handleNewFolder = async () => {
    const folderName = prompt('Enter folder name (e.g. users):', 'new-folder')
    if (folderName && workspacePath) {
      const folderPath = `${workspacePath}/collections/${folderName}`
      try {
        const res = await fetch('/api/workspace/folder', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ path: folderPath }),
        })
        if (res.ok) {
          loadWorkspace(workspacePath)
        }
      } catch (err) {
        console.error('Failed to create folder:', err)
      }
    }
  }

  const handleRefresh = () => {
    if (workspacePath) {
      loadWorkspace(workspacePath)
    }
  }

  // Fallback default sample nodes when workspace is not yet loaded from backend
  const displayTree: TreeNode[] = tree.length > 0 ? tree : [
    {
      id: '1',
      name: 'collections',
      path: 'collections',
      relPath: 'collections',
      isDir: true,
      children: [
        {
          id: '1-1',
          name: 'auth',
          path: 'collections/auth',
          relPath: 'collections/auth',
          isDir: true,
          children: [
            {
              id: '1-1-1',
              name: 'login.pebble.json',
              path: 'collections/auth/login.pebble.json',
              relPath: 'auth/login.pebble.json',
              isDir: false,
              method: 'POST',
            },
            {
              id: '1-1-2',
              name: 'refresh-token.pebble.json',
              path: 'collections/auth/refresh-token.pebble.json',
              relPath: 'auth/refresh-token.pebble.json',
              isDir: false,
              method: 'POST',
            },
          ],
        },
        {
          id: '1-2',
          name: 'health-check.pebble.json',
          path: 'collections/health-check.pebble.json',
          relPath: 'health-check.pebble.json',
          isDir: false,
          method: 'GET',
        },
      ],
    },
  ]

  const renderNode = (node: TreeNode, depth = 0) => {
    const isExpanded = expandedFolders[node.path] ?? true
    const isSelected = activeFilePath === node.path

    if (node.isDir) {
      return (
        <div key={node.path} className="select-none">
          <button
            onClick={() => toggleFolder(node.path)}
            style={{ paddingLeft: `${depth * 12 + 8}px` }}
            className="w-full flex items-center gap-1.5 py-1 text-xs text-zinc-300 hover:text-white hover:bg-zinc-900/60 rounded group transition-colors cursor-pointer"
          >
            {isExpanded ? (
              <ChevronDown className="w-3.5 h-3.5 text-zinc-500" />
            ) : (
              <ChevronRight className="w-3.5 h-3.5 text-zinc-500" />
            )}
            {isExpanded ? (
              <FolderOpen className="w-3.5 h-3.5 text-amber-400 shrink-0" />
            ) : (
              <Folder className="w-3.5 h-3.5 text-amber-400/80 shrink-0" />
            )}
            <span className="truncate font-medium text-zinc-300">{node.name}</span>
          </button>

          {isExpanded && node.children && (
            <div className="flex flex-col">
              {node.children.map((child) => renderNode(child, depth + 1))}
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
            ? 'bg-blue-600/15 text-blue-200 border-l-2 border-blue-500 font-medium'
            : 'text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900/50'
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
    <aside className="w-64 bg-zinc-950 border-r border-zinc-800 flex flex-col shrink-0 select-none">
      {/* Sidebar Header */}
      <div className="h-10 px-3 border-b border-zinc-800 flex items-center justify-between text-xs text-zinc-400">
        <span className="font-semibold text-zinc-300 flex items-center gap-1.5 uppercase text-[11px] tracking-wider">
          <FileCode className="w-3.5 h-3.5 text-blue-400" />
          Collections
        </span>
        <div className="flex items-center gap-0.5">
          <Tooltip content="New Request (*.pebble.json)">
            <Button
              variant="ghost"
              size="icon"
              onClick={handleNewRequest}
              className="h-6 w-6 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900"
            >
              <Plus className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="New Collection Folder">
            <Button
              variant="ghost"
              size="icon"
              onClick={handleNewFolder}
              className="h-6 w-6 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900"
            >
              <FolderPlus className="w-3.5 h-3.5" />
            </Button>
          </Tooltip>
          <Tooltip content="Refresh Files from Disk">
            <Button
              variant="ghost"
              size="icon"
              onClick={handleRefresh}
              className="h-6 w-6 text-zinc-400 hover:text-zinc-200 hover:bg-zinc-900"
            >
              {isLoadingWorkspace ? (
                <Loader2 className="w-3.5 h-3.5 animate-spin text-blue-400" />
              ) : (
                <RefreshCw className="w-3.5 h-3.5" />
              )}
            </Button>
          </Tooltip>
        </div>
      </div>

      {/* Tree Content */}
      <div className="flex-1 overflow-y-auto p-1.5 space-y-0.5">
        {displayTree.map((node) => renderNode(node))}
      </div>

      {/* Git-friendly info footer */}
      <div className="p-2 border-t border-zinc-900 text-[10px] text-zinc-500 flex items-center justify-between bg-zinc-950/50">
        <span>Format: *.pebble.json</span>
        <span className="text-emerald-500 font-mono">Git-synced</span>
      </div>
    </aside>
  )
}
