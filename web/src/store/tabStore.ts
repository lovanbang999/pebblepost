import { create } from 'zustand'
import type { RequestDefinition, ExecutionResult, FolderDefinition, HistoryEntry, RunnerTabConfig, DocsTabConfig } from '../types'

export type TabType = 'request' | 'folder' | 'history' | 'runner' | 'docs'

export interface RequestTab {
  id: string // Unique identifier, equal to filePath (for requests) or "folder:" + folderPath (for folders) or "history:" + id or "runner:" + path or "docs:" + path
  filePath: string
  title: string
  type?: TabType
  method?: RequestDefinition['method']
  isPreview: boolean // true when opened via single click; replaced by next preview tab
  isDirty: boolean
  isReadOnly?: boolean
  historyEntry?: HistoryEntry
  runnerConfig?: RunnerTabConfig
  docsConfig?: DocsTabConfig
  request?: RequestDefinition
  folder?: FolderDefinition
  savedSnapshot: string // JSON representation when loaded/saved
  lastResult: ExecutionResult | null
  activeSubTab: 'params' | 'headers' | 'auth' | 'body' | 'scripts' | 'settings' | 'vars' | 'grpc' | 'stream' | 'docs' | 'examples' | 'extract'
  scrollPosition?: number
}

interface TabState {
  tabs: RequestTab[]
  activeTabId: string | null
  closedTabsHistory: RequestTab[]
  pendingCloseTab: RequestTab | null

  // Tab management actions
  openTab: (filePath: string, request: RequestDefinition, isPreview?: boolean) => void
  openFolderTab: (folderPath: string, folder?: FolderDefinition, isPreview?: boolean) => Promise<void>
  openHistoryTab: (entry: HistoryEntry) => void
  openRunnerTab: (folderPath?: string, folderName?: string) => void
  openDocsTab: (folderPath?: string, folderName?: string) => void
  restoreRequestFromHistory: (entry: HistoryEntry) => void
  pinTab: (tabId: string) => void
  closeTab: (tabId: string, force?: boolean) => boolean
  confirmCloseTab: (choice: 'save' | 'discard' | 'cancel') => Promise<void>
  reopenLastClosedTab: () => void
  setActiveTabId: (tabId: string) => void
  cycleTabs: (direction?: 'next' | 'prev') => void

  // Active tab state actions
  updateActiveRequest: (updater: (prev: RequestDefinition) => RequestDefinition) => void
  updateActiveFolder: (updater: (prev: FolderDefinition) => FolderDefinition) => void
  setActiveSubTab: (subTab: RequestTab['activeSubTab']) => void
  setLastResult: (
    result:
      | ExecutionResult
      | null
      | ((prev: ExecutionResult | null) => ExecutionResult | null),
  ) => void
  setScrollPosition: (scroll: number) => void
  saveCurrentTab: () => Promise<boolean>

  // Synchronization with filesystem events
  onFileRenamed: (oldPath: string, newPath: string, newName?: string) => void
  onFileDeleted: (path: string) => void

  // Workspace persistence
  restoreTabs: (workspacePath: string) => Promise<void>
  persistTabs: (workspacePath: string) => void
}

function makeHistoryRequestDefinition(entry: HistoryEntry): RequestDefinition {
  const base: RequestDefinition = {
    name: entry.requestName || `${entry.method} ${entry.url}`,
    method: (entry.method as RequestDefinition['method']) || 'GET',
    url: entry.url,
    headers: [],
    params: [],
    auth: { type: 'none' },
    body: { type: 'none' },
    scripts: {},
    settings: { followRedirects: true, verifySSL: true, timeoutMs: 30000 },
  }

  if (entry.resolvedRequest && typeof entry.resolvedRequest === 'object') {
    const raw = entry.resolvedRequest as Partial<RequestDefinition>
    return {
      ...base,
      ...raw,
      name: raw.name || base.name,
      method: raw.method || base.method,
      url: raw.url || base.url,
      auth: raw.auth || base.auth,
      body: raw.body || base.body,
      scripts: raw.scripts || base.scripts,
      settings: raw.settings || base.settings,
    }
  }

  return base
}

export const useTabStore = create<TabState>((set, get) => ({
  tabs: [],
  activeTabId: null,
  closedTabsHistory: [],
  pendingCloseTab: null,

  openTab: (filePath: string, request: RequestDefinition, isPreview = false) => {
    const { tabs } = get()
    const cleanPath = filePath.replace(/^\.\//, '')
    const existingIndex = tabs.findIndex(
      (t) => t.id === cleanPath || t.filePath.replace(/^\.\//, '') === cleanPath
    )

    if (existingIndex !== -1) {
      const existing = tabs[existingIndex]
      // If user opened with double-click (pinned intent) on an existing preview tab, pin it
      if (!isPreview && existing.isPreview) {
        set({
          tabs: tabs.map((t, idx) => (idx === existingIndex ? { ...t, isPreview: false } : t)),
          activeTabId: existing.id,
        })
      } else {
        set({ activeTabId: existing.id })
      }
      get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
      return
    }

    const cleanTitle =
      request.name ||
      cleanPath.split('/').pop()?.replace('.pebble.json', '') ||
      'Untitled Request'

    const newTab: RequestTab = {
      id: cleanPath,
      filePath: cleanPath,
      title: cleanTitle,
      type: 'request',
      method: request.method || 'GET',
      isPreview: isPreview,
      isDirty: false,
      request: request,
      savedSnapshot: JSON.stringify(request),
      lastResult: null,
      activeSubTab:
        request.method === 'GRPC' || request.protocol === 'grpc'
          ? 'grpc'
          : request.method === 'WS' || request.method === 'SSE' || request.protocol === 'websocket' || request.protocol === 'sse'
            ? 'stream'
            : 'params',
      scrollPosition: 0,
    }

    // If opening as a preview tab, replace any existing unpinned clean preview tab
    if (isPreview) {
      const previewIndex = tabs.findIndex((t) => t.isPreview && !t.isDirty)
      if (previewIndex !== -1) {
        const nextTabs = [...tabs]
        nextTabs[previewIndex] = newTab
        set({ tabs: nextTabs, activeTabId: newTab.id })
        get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
        return
      }
    }

    set((state) => ({
      tabs: [...state.tabs, newTab],
      activeTabId: newTab.id,
    }))
    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
  },

  openFolderTab: async (folderPath: string, folderDef?: FolderDefinition, isPreview = false) => {
    const { tabs } = get()
    const cleanPath = folderPath.replace(/^\.\//, '')
    const folderTabId = `folder:${cleanPath}`

    const existingIndex = tabs.findIndex(
      (t) => t.id === folderTabId || (t.type === 'folder' && t.filePath.replace(/^\.\//, '') === cleanPath)
    )

    if (existingIndex !== -1) {
      const existing = tabs[existingIndex]
      if (!isPreview && existing.isPreview) {
        set({
          tabs: tabs.map((t, idx) => (idx === existingIndex ? { ...t, isPreview: false } : t)),
          activeTabId: existing.id,
        })
      } else {
        set({ activeTabId: existing.id })
      }
      get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
      return
    }

    let loadedFolder = folderDef
    if (!loadedFolder && typeof window !== 'undefined') {
      try {
        const ws = localStorage.getItem('pebblepost-workspace-path') || '.'
        const res = await fetch(
          `/api/folder?workspacePath=${encodeURIComponent(ws)}&path=${encodeURIComponent(cleanPath)}`
        )
        if (res.ok) {
          loadedFolder = await res.json()
        }
      } catch (err) {
        console.error('Failed to load folder settings:', err)
      }
    }

    const dirName = cleanPath.split('/').pop() || 'Folder'
    const finalFolder: FolderDefinition = loadedFolder || {
      schemaVersion: 1,
      name: dirName,
      auth: { type: 'inherit' },
      headers: [],
      variables: [],
    }

    const cleanTitle = finalFolder.name || dirName

    const newTab: RequestTab = {
      id: folderTabId,
      filePath: cleanPath,
      title: cleanTitle,
      type: 'folder',
      isPreview: isPreview,
      isDirty: false,
      folder: finalFolder,
      savedSnapshot: JSON.stringify(finalFolder),
      lastResult: null,
      activeSubTab: 'headers',
      scrollPosition: 0,
    }

    if (isPreview) {
      const previewIndex = tabs.findIndex((t) => t.isPreview && !t.isDirty)
      if (previewIndex !== -1) {
        const nextTabs = [...tabs]
        nextTabs[previewIndex] = newTab
        set({ tabs: nextTabs, activeTabId: newTab.id })
        get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
        return
      }
    }

    set((state) => ({
      tabs: [...state.tabs, newTab],
      activeTabId: newTab.id,
    }))
    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
  },

  openHistoryTab: (entry: HistoryEntry) => {
    const tabId = `history:${entry.id}`
    const { tabs } = get()
    const existing = tabs.find((t) => t.id === tabId)
    if (existing) {
      set({ activeTabId: tabId })
      return
    }

    const reqDef = makeHistoryRequestDefinition(entry)

    const histResult: ExecutionResult = {
      statusCode: entry.statusCode,
      statusText: entry.statusCode >= 200 && entry.statusCode < 300 ? 'OK' : `${entry.statusCode}`,
      headers: entry.responseHeaders || {},
      body: entry.responseBody || '',
      size: entry.sizeBytes,
      timing: {
        dnsLookupMs: 0,
        tcpConnectMs: 0,
        tlsHandshakeMs: 0,
        ttfbMs: 0,
        downloadMs: 0,
        totalDurationMs: entry.durationMs,
      },
      tests: [],
      logs: [],
      executedAt: entry.executedAt,
    }

    const newTab: RequestTab = {
      id: tabId,
      filePath: '',
      title: `[History] ${entry.requestName || entry.method + ' ' + entry.url}`,
      type: 'history',
      method: (reqDef.method || entry.method || 'GET') as RequestDefinition['method'],
      isPreview: false,
      isDirty: false,
      isReadOnly: true,
      historyEntry: entry,
      request: reqDef,
      savedSnapshot: JSON.stringify(reqDef),
      lastResult: histResult,
      activeSubTab: 'params',
      scrollPosition: 0,
    }

    set((state) => ({
      tabs: [...state.tabs, newTab],
      activeTabId: newTab.id,
    }))
  },

  openRunnerTab: (folderPath?: string, folderName?: string) => {
    const tabId = folderPath ? `runner:${folderPath}` : 'runner:workspace'
    const title = folderName ? `Runner: ${folderName}` : 'Collection Runner'
    const { tabs } = get()
    const existing = tabs.find((t) => t.id === tabId)
    if (existing) {
      set({ activeTabId: tabId })
      return
    }

    const newTab: RequestTab = {
      id: tabId,
      filePath: folderPath || '',
      title,
      type: 'runner',
      isPreview: false,
      isDirty: false,
      isReadOnly: true,
      runnerConfig: {
        folderPath,
        folderName,
      },
      savedSnapshot: '',
      lastResult: null,
      activeSubTab: 'params',
      scrollPosition: 0,
    }

    set((state) => ({
      tabs: [...state.tabs, newTab],
      activeTabId: newTab.id,
    }))
  },

  openDocsTab: (folderPath?: string, folderName?: string) => {
    const tabId = folderPath ? `docs:${folderPath}` : 'docs:workspace'
    const { tabs } = get()
    const existing = tabs.find((t) => t.id === tabId)
    if (existing) {
      set({ activeTabId: tabId })
      return
    }

    const title = folderName
      ? `Docs: ${folderName}`
      : folderPath
      ? `Docs: ${folderPath.split('/').pop()}`
      : 'API Documentation'

    const newTab: RequestTab = {
      id: tabId,
      filePath: folderPath || '',
      title,
      type: 'docs',
      isPreview: false,
      isDirty: false,
      isReadOnly: true,
      docsConfig: {
        folderPath,
        folderName,
      },
      savedSnapshot: '',
      lastResult: null,
      activeSubTab: 'docs',
      scrollPosition: 0,
    }

    set((state) => ({
      tabs: [...state.tabs, newTab],
      activeTabId: newTab.id,
    }))
  },

  restoreRequestFromHistory: (entry: HistoryEntry) => {
    const { tabs, activeTabId, updateActiveRequest } = get()
    const active = tabs.find((t) => t.id === activeTabId)
    const reqDef = makeHistoryRequestDefinition(entry)

    if (active && active.type !== 'history' && active.type !== 'folder') {
      updateActiveRequest((prev) => ({
        ...prev,
        method: reqDef.method || prev.method,
        url: reqDef.url || prev.url,
        params: reqDef.params ? [...reqDef.params] : prev.params,
        headers: reqDef.headers ? [...reqDef.headers] : prev.headers,
        auth: reqDef.auth ? { ...reqDef.auth } : prev.auth,
        body: reqDef.body ? { ...reqDef.body } : prev.body,
      }))
    } else {
      const tabId = `draft:restore-${Date.now()}`
      const newTab: RequestTab = {
        id: tabId,
        filePath: '',
        title: reqDef.name || 'Restored Request',
        type: 'request',
        method: (reqDef.method || 'GET') as RequestDefinition['method'],
        isPreview: false,
        isDirty: true,
        request: reqDef,
        savedSnapshot: '',
        lastResult: null,
        activeSubTab: 'params',
        scrollPosition: 0,
      }
      set((state) => ({
        tabs: [...state.tabs, newTab],
        activeTabId: newTab.id,
      }))
    }
  },

  pinTab: (tabId: string) => {
    set((state) => ({
      tabs: state.tabs.map((t) => (t.id === tabId ? { ...t, isPreview: false } : t)),
    }))
    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
  },

  closeTab: (tabId: string, force = false): boolean => {
    const { tabs, activeTabId, closedTabsHistory } = get()
    const targetTab = tabs.find((t) => t.id === tabId)
    if (!targetTab) return true

    // If tab has unsaved modifications and close is not forced, request confirmation
    if (!force && targetTab.isDirty) {
      set({ pendingCloseTab: targetTab })
      return false
    }

    const tabIndex = tabs.findIndex((t) => t.id === tabId)
    const nextTabs = tabs.filter((t) => t.id !== tabId)
    let nextActiveId = activeTabId

    if (activeTabId === tabId) {
      if (nextTabs.length > 0) {
        const newActiveIndex = Math.min(tabIndex, nextTabs.length - 1)
        nextActiveId = nextTabs[newActiveIndex].id
      } else {
        nextActiveId = null
      }
    }

    set({
      tabs: nextTabs,
      activeTabId: nextActiveId,
      pendingCloseTab: null,
      closedTabsHistory: [targetTab, ...closedTabsHistory.slice(0, 19)],
    })

    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
    return true
  },

  confirmCloseTab: async (choice: 'save' | 'discard' | 'cancel') => {
    const { pendingCloseTab } = get()
    if (!pendingCloseTab) return

    if (choice === 'cancel') {
      set({ pendingCloseTab: null })
      return
    }

    if (choice === 'save') {
      const ws = localStorage.getItem('pebblepost-workspace-path') || '.'
      const reqToSave = { ...pendingCloseTab.request, schemaVersion: 1 }
      try {
        await fetch('/api/request/save', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            workspacePath: ws,
            path: pendingCloseTab.filePath,
            request: reqToSave,
          }),
        })
      } catch (err) {
        console.error('Failed to save request before closing tab:', err)
      }
    }

    // Force close after save or discard
    get().closeTab(pendingCloseTab.id, true)
  },

  reopenLastClosedTab: () => {
    const { closedTabsHistory } = get()
    if (closedTabsHistory.length === 0) return
    const [lastClosed, ...rest] = closedTabsHistory
    set({ closedTabsHistory: rest })
    if (lastClosed.type === 'folder' && lastClosed.folder) {
      get().openFolderTab(lastClosed.filePath, lastClosed.folder, false)
    } else if (lastClosed.request) {
      get().openTab(lastClosed.filePath, lastClosed.request, false)
    }
  },

  setActiveTabId: (tabId: string) => {
    set({ activeTabId: tabId })
    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
  },

  cycleTabs: (direction = 'next') => {
    const { tabs, activeTabId } = get()
    if (tabs.length <= 1) return
    const currentIndex = tabs.findIndex((t) => t.id === activeTabId)
    const nextIndex =
      direction === 'next'
        ? (currentIndex + 1) % tabs.length
        : (currentIndex - 1 + tabs.length) % tabs.length
    set({ activeTabId: tabs[nextIndex].id })
    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
  },

  updateActiveRequest: (updater: (prev: RequestDefinition) => RequestDefinition) => {
    const { tabs, activeTabId } = get()
    if (!activeTabId) return

    set({
      tabs: tabs.map((tab) => {
        if (tab.id !== activeTabId || !tab.request) return tab
        const updatedReq = updater(tab.request)
        const isDirty = JSON.stringify(updatedReq) !== tab.savedSnapshot
        return {
          ...tab,
          request: updatedReq,
          title: updatedReq.name || tab.title,
          method: updatedReq.method || tab.method,
          isDirty,
          isPreview: false, // Any edit automatically promotes to a pinned tab
        }
      }),
    })
  },

  updateActiveFolder: (updater: (prev: FolderDefinition) => FolderDefinition) => {
    const { tabs, activeTabId } = get()
    if (!activeTabId) return

    set({
      tabs: tabs.map((tab) => {
        if (tab.id !== activeTabId || tab.type !== 'folder' || !tab.folder) return tab
        const updatedFolder = updater(tab.folder)
        const isDirty = JSON.stringify(updatedFolder) !== tab.savedSnapshot
        return {
          ...tab,
          folder: updatedFolder,
          title: updatedFolder.name || tab.title,
          isDirty,
          isPreview: false,
        }
      }),
    })
  },

  setActiveSubTab: (subTab: RequestTab['activeSubTab']) => {
    const { tabs, activeTabId } = get()
    if (!activeTabId) return
    set({
      tabs: tabs.map((t) => (t.id === activeTabId ? { ...t, activeSubTab: subTab } : t)),
    })
  },

  setLastResult: (
    result:
      | ExecutionResult
      | null
      | ((prev: ExecutionResult | null) => ExecutionResult | null),
  ) => {
    const { tabs, activeTabId } = get()
    if (!activeTabId) return
    set({
      tabs: tabs.map((t) => {
        if (t.id !== activeTabId) return t
        const nextResult =
          typeof result === 'function' ? result(t.lastResult || null) : result
        return { ...t, lastResult: nextResult }
      }),
    })
  },

  setScrollPosition: (scroll: number) => {
    const { tabs, activeTabId } = get()
    if (!activeTabId) return
    set({
      tabs: tabs.map((t) => (t.id === activeTabId ? { ...t, scrollPosition: scroll } : t)),
    })
  },

  saveCurrentTab: async () => {
    const { tabs, activeTabId } = get()
    const activeTab = tabs.find((t) => t.id === activeTabId)
    if (!activeTab) return false
    const ws = (typeof window !== 'undefined' ? localStorage.getItem('pebblepost-workspace-path') : null) || '.'

    if (activeTab.type === 'folder' && activeTab.folder) {
      const folderToSave: FolderDefinition = {
        ...activeTab.folder,
        schemaVersion: 1,
      }
      try {
        const res = await fetch('/api/folder/save', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            workspacePath: ws,
            path: activeTab.filePath,
            folder: folderToSave,
          }),
        })
        if (res.ok) {
          set({
            tabs: tabs.map((t) =>
              t.id === activeTab.id
                ? {
                    ...t,
                    folder: folderToSave,
                    savedSnapshot: JSON.stringify(folderToSave),
                    isDirty: false,
                    title: folderToSave.name || t.title,
                  }
                : t
            ),
          })
          get().persistTabs(ws)
          return true
        }
        return false
      } catch (err) {
        console.error('Failed to save folder settings:', err)
        return false
      }
    }

    if (!activeTab.request) return false

    const reqToSave: RequestDefinition = {
      ...activeTab.request,
      schemaVersion: 1,
    }

    try {
      const res = await fetch('/api/request/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath: ws,
          path: activeTab.filePath,
          request: reqToSave,
        }),
      })

      if (res.ok) {
        set({
          tabs: tabs.map((t) =>
            t.id === activeTab.id
              ? {
                  ...t,
                  request: reqToSave,
                  savedSnapshot: JSON.stringify(reqToSave),
                  isDirty: false,
                  title: reqToSave.name || t.title,
                  method: reqToSave.method || t.method,
                }
              : t
          ),
        })
        get().persistTabs(ws)
        return true
      }
      return false
    } catch (err) {
      console.error('Failed to save tab request:', err)
      return false
    }
  },

  onFileRenamed: (oldPath: string, newPath: string, newName?: string) => {
    const cleanOld = oldPath.replace(/^\.\//, '')
    const cleanNew = newPath.replace(/^\.\//, '')

    set((state) => {
      let activeId = state.activeTabId
      const updatedTabs = state.tabs.map((tab) => {
        const tabClean = tab.filePath.replace(/^\.\//, '')
        // Direct file match
        if (tabClean === cleanOld) {
          const title =
            newName ||
            cleanNew.split('/').pop()?.replace('.pebble.json', '') ||
            tab.title
          if (activeId === tab.id) {
            activeId = cleanNew
          }
          return {
            ...tab,
            id: cleanNew,
            filePath: cleanNew,
            title,
          }
        }
        // Directory match
        if (tabClean.startsWith(cleanOld + '/')) {
          const subPath = tabClean.substring(cleanOld.length)
          const updatedPath = cleanNew + subPath
          if (activeId === tab.id) {
            activeId = updatedPath
          }
          return {
            ...tab,
            id: updatedPath,
            filePath: updatedPath,
          }
        }
        return tab
      })

      return {
        tabs: updatedTabs,
        activeTabId: activeId,
      }
    })
    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
  },

  onFileDeleted: (path: string) => {
    const cleanDel = path.replace(/^\.\//, '')
    const tabsToKeep = get().tabs.filter((tab) => {
      const tabClean = tab.filePath.replace(/^\.\//, '')
      return tabClean !== cleanDel && !tabClean.startsWith(cleanDel + '/')
    })

    let nextActiveId = get().activeTabId
    if (nextActiveId && !tabsToKeep.some((t) => t.id === nextActiveId)) {
      nextActiveId = tabsToKeep.length > 0 ? tabsToKeep[0].id : null
    }

    set({
      tabs: tabsToKeep,
      activeTabId: nextActiveId,
    })
    get().persistTabs(localStorage.getItem('pebblepost-workspace-path') || '.')
  },

  persistTabs: (workspacePath: string) => {
    if (typeof window === 'undefined') return
    try {
      const ws = workspacePath || '.'
      const data = {
        activeTabId: get().activeTabId,
        tabSummaries: get().tabs.map((t) => ({
          filePath: t.filePath,
          isPreview: t.isPreview,
          type: t.type || 'request',
        })),
      }
      localStorage.setItem(`pebblepost-tabs-${ws}`, JSON.stringify(data))
    } catch (err) {
      console.error('Failed to persist tabs:', err)
    }
  },

  restoreTabs: async (workspacePath: string) => {
    if (typeof window === 'undefined') return
    try {
      const ws = workspacePath || '.'
      const raw = localStorage.getItem(`pebblepost-tabs-${ws}`)
      if (!raw) return
      const parsed = JSON.parse(raw)
      if (!Array.isArray(parsed.tabSummaries) || parsed.tabSummaries.length === 0) return

      for (const item of parsed.tabSummaries) {
        try {
          if (item.type === 'folder') {
            await get().openFolderTab(item.filePath, undefined, item.isPreview)
          } else {
            const res = await fetch(
              `/api/request?path=${encodeURIComponent(item.filePath)}&workspacePath=${encodeURIComponent(ws)}`
            )
            if (res.ok) {
              const req: RequestDefinition = await res.json()
              get().openTab(item.filePath, req, item.isPreview)
            }
          }
        } catch (err) {
          console.warn(`Could not restore tab ${item.filePath}:`, err)
        }
      }

      if (parsed.activeTabId && get().tabs.some((t) => t.id === parsed.activeTabId)) {
        set({ activeTabId: parsed.activeTabId })
      }
    } catch (err) {
      console.error('Failed to restore tabs:', err)
    }
  },
}))
