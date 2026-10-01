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
} from 'lucide-react'
import { useWorkspaceStore } from '../../store/workspaceStore'
import type { TreeNode, RequestDefinition } from '../../types'
import { getMethodColor } from '../../lib/utils'

export const CollectionTree: React.FC = () => {
  const { tree, activeFilePath, setActiveRequest } = useWorkspaceStore()
  const [expandedFolders, setExpandedFolders] = useState<Record<string, boolean>>({
    'collections': true,
    'collections/auth': true,
  })

  const toggleFolder = (path: string) => {
    setExpandedFolders((prev) => ({ ...prev, [path]: !prev[path] }))
  }

  const handleSelectRequest = (node: TreeNode) => {
    // If request node is selected, create or load sample representation
    const sampleReq: RequestDefinition = {
      name: node.name.replace('.pebble.json', ''),
      method: (node.method as any) || 'GET',
      url: '{{BASE_URL}}/api/v1/' + (node.name.includes('login') ? 'auth/login' : 'health'),
      headers: [
        { key: 'Accept', value: 'application/json', enabled: true },
        { key: 'Content-Type', value: 'application/json', enabled: true },
      ],
      params: [],
      auth: { type: 'none' },
      body: node.method === 'POST' ? {
        type: 'json',
        raw: JSON.stringify({ email: 'developer@pebble.io', password: '{{ADMIN_PASSWORD}}' }, null, 2),
      } : { type: 'none' },
      scripts: {
        preRequest: '// Pre-request scripts\npb.request.headers.set("X-Timestamp", Date.now().toString());',
        postResponse: 'pb.test("Response status is 200", () => {\n  pb.expect(pb.response.status).to.eql(200);\n});',
      },
      settings: {
        followRedirects: true,
        verifySSL: true,
        timeoutMs: 30000,
      },
    }
    setActiveRequest(sampleReq, node.path)
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
    const isExpanded = expandedFolders[node.path] ?? false
    const isSelected = activeFilePath === node.path

    if (node.isDir) {
      return (
        <div key={node.path} className="select-none">
          <button
            onClick={() => toggleFolder(node.path)}
            style={{ paddingLeft: `${depth * 12 + 8}px` }}
            className="w-full flex items-center gap-1.5 py-1 text-xs text-zinc-300 hover:text-white hover:bg-zinc-900/60 rounded group transition-colors"
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
        className={`w-full flex items-center gap-2 py-1 text-xs rounded transition-colors group text-left ${
          isSelected
            ? 'bg-blue-600/15 text-blue-200 border-l-2 border-blue-500'
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
        <div className="flex items-center gap-1">
          <button
            className="p-1 hover:text-zinc-200 hover:bg-zinc-900 rounded transition-colors"
            title="New Request"
          >
            <Plus className="w-3.5 h-3.5" />
          </button>
          <button
            className="p-1 hover:text-zinc-200 hover:bg-zinc-900 rounded transition-colors"
            title="New Folder"
          >
            <FolderPlus className="w-3.5 h-3.5" />
          </button>
          <button
            className="p-1 hover:text-zinc-200 hover:bg-zinc-900 rounded transition-colors"
            title="Refresh Files"
          >
            <RefreshCw className="w-3.5 h-3.5" />
          </button>
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
