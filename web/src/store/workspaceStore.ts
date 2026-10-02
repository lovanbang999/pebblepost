import { create } from 'zustand'
import type { RequestDefinition, ExecutionResult, TreeNode, EnvironmentDefinition } from '../types'

export interface DiskConflict {
  filePath: string
  diskRequest: RequestDefinition
  localRequest: RequestDefinition
}

interface WorkspaceState {
  workspacePath: string | null
  activeEnv: string
  environments: EnvironmentDefinition[]
  tree: TreeNode[]
  activeRequest: RequestDefinition | null
  activeFilePath: string | null
  savedRequestSnapshot: string | null
  diskConflict: DiskConflict | null
  isExecuting: boolean
  isLoadingWorkspace: boolean
  lastResult: ExecutionResult | null
  activeTab: 'params' | 'headers' | 'auth' | 'body' | 'scripts' | 'settings'

  setWorkspacePath: (path: string | null) => void
  setActiveEnv: (env: string) => void
  setEnvironments: (envs: EnvironmentDefinition[]) => void
  setTree: (tree: TreeNode[]) => void
  setActiveRequest: (req: RequestDefinition | null, filePath?: string | null) => void
  setIsExecuting: (isExecuting: boolean) => void
  setLastResult: (result: ExecutionResult | null) => void
  setActiveTab: (tab: 'params' | 'headers' | 'auth' | 'body' | 'scripts' | 'settings') => void
  updateActiveRequest: (updater: (prev: RequestDefinition) => RequestDefinition) => void
  isDirty: () => boolean
  resolveConflict: (choice: 'mine' | 'disk') => void
  dismissConflict: () => void

  theme: 'light' | 'dark'
  toggleTheme: () => void
  setTheme: (theme: 'light' | 'dark') => void

  gitignoreStatus: 'ok' | 'missing' | 'no_rule' | null
  ensureGitignore: () => Promise<boolean>
  isWorkspaceTrusted: (path?: string) => boolean
  setWorkspaceTrusted: (trusted: boolean, path?: string) => void

  // Backend integration actions
  loadWorkspace: (path?: string) => Promise<void>
  loadRequest: (filePath: string) => Promise<void>
  saveCurrentRequest: () => Promise<boolean>
  createNewRequest: (
    folderPath?: string,
    name?: string,
    method?: RequestDefinition['method']
  ) => Promise<string | null>
  initWatcher: () => void
  cleanupWatcher: () => void
}

const defaultRequest: RequestDefinition = {
  name: 'Untitled Request',
  method: 'GET',
  url: 'http://localhost:8080/api/health',
  headers: [
    { key: 'Accept', value: 'application/json', enabled: true },
  ],
  params: [],
  auth: { type: 'none' },
  body: { type: 'none' },
  scripts: {
    preRequest: '',
    postResponse: 'pb.test("Status code is 200", () => {\n  pb.expect(pb.response.status).to.eql(200);\n});',
  },
  settings: {
    followRedirects: true,
    verifySSL: true,
    timeoutMs: 30000,
  },
}

function getInitialTheme(): 'light' | 'dark' {
  if (typeof window !== 'undefined') {
    const saved = localStorage.getItem('pebblepost-theme')
    if (saved === 'light' || saved === 'dark') {
      document.documentElement.classList.toggle('dark', saved === 'dark')
      return saved
    }
  }
  if (typeof document !== 'undefined') {
    document.documentElement.classList.add('dark')
  }
  return 'dark'
}

let activeEventSource: EventSource | null = null

function setupWatcher(
  workspacePath: string,
  getState: () => WorkspaceState,
  setState: (partial: Partial<WorkspaceState> | ((state: WorkspaceState) => Partial<WorkspaceState>)) => void
) {
  if (typeof window === 'undefined') return
  if (activeEventSource) {
    activeEventSource.close()
    activeEventSource = null
  }

  const ws = workspacePath || '.'
  const url = `/api/workspace/events?workspacePath=${encodeURIComponent(ws)}`
  const es = new EventSource(url)
  activeEventSource = es

  es.onmessage = async (event) => {
    try {
      const data = JSON.parse(event.data)
      if (data.type === 'connected') return

      const state = getState()
      const { activeFilePath, activeRequest, savedRequestSnapshot } = state

      // If active file was modified on disk
      if (activeFilePath && data.path) {
        const targetClean = activeFilePath.replace(/^\.\//, '')
        const eventClean = (data.path || '').replace(/^\.\//, '')
        const eventRelClean = (data.relPath || '').replace(/^\.\//, '')

        const isCurrentFile =
          targetClean === eventClean ||
          targetClean === eventRelClean ||
          eventClean.endsWith(targetClean) ||
          targetClean.endsWith(eventRelClean)

        if (isCurrentFile) {
          try {
            const res = await fetch(
              `/api/request?path=${encodeURIComponent(activeFilePath)}&workspacePath=${encodeURIComponent(state.workspacePath || '.')}`
            )
            if (res.ok) {
              const diskReq: RequestDefinition = await res.json()
              const isDirty = Boolean(
                activeRequest &&
                savedRequestSnapshot &&
                JSON.stringify(activeRequest) !== savedRequestSnapshot
              )

              if (isDirty) {
                setState({
                  diskConflict: {
                    filePath: activeFilePath,
                    diskRequest: diskReq,
                    localRequest: activeRequest!,
                  },
                })
              } else {
                setState({
                  activeRequest: diskReq,
                  savedRequestSnapshot: JSON.stringify(diskReq),
                })
              }
            }
          } catch (err) {
            console.error('Failed to reload changed request file:', err)
          }
        }
      }

      // Always reload workspace tree to reflect file renames/additions/deletions/ordering
      state.loadWorkspace(state.workspacePath || '.')
    } catch (err) {
      console.error('Error handling workspace event:', err)
    }
  }

  es.onerror = () => {
    // EventSource automatically retries connection
  }
}

export const useWorkspaceStore = create<WorkspaceState>((set, get) => ({
  theme: getInitialTheme(),
  workspacePath:
    (typeof window !== 'undefined' && localStorage.getItem('pebblepost-workspace-path')) ||
    '.',
  activeEnv: 'dev',
  environments: [
    {
      name: 'dev',
      variables: [
        { key: 'BASE_URL', value: 'http://localhost:8080', enabled: true },
      ],
    },
  ],
  tree: [],
  activeRequest: defaultRequest,
  activeFilePath: null,
  savedRequestSnapshot: JSON.stringify(defaultRequest),
  diskConflict: null,
  isExecuting: false,
  isLoadingWorkspace: false,
  lastResult: null,
  activeTab: 'params',

  setWorkspacePath: (path) => {
    if (typeof window !== 'undefined') {
      if (path) {
        localStorage.setItem('pebblepost-workspace-path', path)
      } else {
        localStorage.removeItem('pebblepost-workspace-path')
      }
    }
    set({ workspacePath: path })
    get().loadWorkspace(path || '.')
    get().initWatcher()
  },
  setActiveEnv: (env) => set({ activeEnv: env }),
  setEnvironments: (environments) => set({ environments }),
  setTree: (tree) => set({ tree }),
  setActiveRequest: (activeRequest, activeFilePath = null) =>
    set({
      activeRequest,
      activeFilePath,
      savedRequestSnapshot: activeRequest ? JSON.stringify(activeRequest) : null,
      diskConflict: null,
      lastResult: null,
    }),
  setIsExecuting: (isExecuting) => set({ isExecuting }),
  setLastResult: (lastResult) => set({ lastResult }),
  setActiveTab: (activeTab) => set({ activeTab }),
  updateActiveRequest: (updater) =>
    set((state) => ({
      activeRequest: state.activeRequest ? updater(state.activeRequest) : null,
    })),

  isDirty: () => {
    const { activeRequest, savedRequestSnapshot } = get()
    return Boolean(
      activeRequest &&
      savedRequestSnapshot &&
      JSON.stringify(activeRequest) !== savedRequestSnapshot
    )
  },

  resolveConflict: (choice: 'mine' | 'disk') => {
    const { diskConflict } = get()
    if (!diskConflict) return
    if (choice === 'disk') {
      set({
        activeRequest: diskConflict.diskRequest,
        savedRequestSnapshot: JSON.stringify(diskConflict.diskRequest),
        diskConflict: null,
      })
    } else {
      set({ diskConflict: null })
    }
  },

  dismissConflict: () => set({ diskConflict: null }),

  initWatcher: () => {
    setupWatcher(get().workspacePath || '.', get, set)
  },

  cleanupWatcher: () => {
    if (activeEventSource) {
      activeEventSource.close()
      activeEventSource = null
    }
  },

  setTheme: (theme: 'light' | 'dark') => {
    if (typeof window !== 'undefined') {
      localStorage.setItem('pebblepost-theme', theme)
      document.documentElement.classList.toggle('dark', theme === 'dark')
    }
    set({ theme })
  },

  toggleTheme: () => {
    const nextTheme = get().theme === 'dark' ? 'light' : 'dark'
    get().setTheme(nextTheme)
  },

  gitignoreStatus: null,

  ensureGitignore: async () => {
    const ws = get().workspacePath || '.'
    try {
      const res = await fetch('/api/workspace/gitignore/ensure', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspacePath: ws }),
      })
      if (res.ok) {
        set({ gitignoreStatus: 'ok' })
        return true
      }
    } catch (err) {
      console.error('Failed to update gitignore:', err)
    }
    return false
  },

  isWorkspaceTrusted: (path?: string) => {
    const ws = path || get().workspacePath || '.'
    if (typeof window === 'undefined') return true
    try {
      const trusted = JSON.parse(localStorage.getItem('pebblepost-trusted-workspaces') || '{}')
      return Boolean(trusted[ws])
    } catch {
      return false
    }
  },

  setWorkspaceTrusted: (trusted: boolean, path?: string) => {
    const ws = path || get().workspacePath || '.'
    if (typeof window !== 'undefined') {
      try {
        const stored = JSON.parse(localStorage.getItem('pebblepost-trusted-workspaces') || '{}')
        stored[ws] = trusted
        localStorage.setItem('pebblepost-trusted-workspaces', JSON.stringify(stored))
      } catch (err) {
        console.error('Failed to store trusted workspace state:', err)
      }
    }
  },

  loadWorkspace: async (path?: string) => {
    const targetPath = path || get().workspacePath || '.'
    set({ isLoadingWorkspace: true })
    try {
      const res = await fetch(`/api/workspace/scan?path=${encodeURIComponent(targetPath)}`)
      if (res.ok) {
        const data = await res.json()
        set({
          tree: data.tree || [],
          environments: data.environments?.length ? data.environments : get().environments,
          gitignoreStatus: data.gitignoreStatus || null,
        })
        const currentActive = get().activeFilePath
        if (currentActive && !get().isDirty()) {
          get().loadRequest(currentActive)
        }
      }
    } catch (err) {
      console.warn('Could not scan workspace via API (running offline or standalone mode):', err)
    } finally {
      set({ isLoadingWorkspace: false })
    }
  },

  loadRequest: async (filePath: string) => {
    try {
      const ws = get().workspacePath || '.'
      const res = await fetch(`/api/request?path=${encodeURIComponent(filePath)}&workspacePath=${encodeURIComponent(ws)}`)
      if (res.ok) {
        const reqData: RequestDefinition = await res.json()
        set({
          activeRequest: reqData,
          activeFilePath: filePath,
          savedRequestSnapshot: JSON.stringify(reqData),
          diskConflict: null,
          lastResult: null,
        })
      }
    } catch (err) {
      console.error('Failed to load request:', err)
    }
  },

  saveCurrentRequest: async () => {
    const { activeFilePath, activeRequest, workspacePath } = get()
    if (!activeFilePath || !activeRequest) return false

    const reqToSave: RequestDefinition = {
      ...activeRequest,
      schemaVersion: 1,
    }

    try {
      const res = await fetch('/api/request/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          workspacePath: workspacePath || '.',
          path: activeFilePath,
          request: reqToSave,
        }),
      })
      if (res.ok) {
        set({
          activeRequest: reqToSave,
          savedRequestSnapshot: JSON.stringify(reqToSave),
          diskConflict: null,
        })
        if (workspacePath) {
          get().loadWorkspace(workspacePath)
        }
        return true
      }
      return false
    } catch (err) {
      console.error('Failed to save request:', err)
      return false
    }
  },

  createNewRequest: async (
    folderPath?: string,
    name = 'new-request',
    method: RequestDefinition['method'] = 'GET',
  ) => {
    const currentWs = get().workspacePath || '.'
    const targetDir =
      folderPath ||
      (currentWs && currentWs !== '.' ? `${currentWs}/collections` : 'collections')
    const fileName = name.endsWith('.pebble.json') ? name : `${name}.pebble.json`
    const fullPath = `${targetDir}/${fileName}`

    const newReq: RequestDefinition = {
      $schema: 'https://pebblepost.dev/schemas/v1/request.json',
      schemaVersion: 1,
      version: '1.0',
      name: name.replace('.pebble.json', ''),
      method: method,
      url: '{{BASE_URL}}/api/v1/endpoint',
      headers: [{ key: 'Accept', value: 'application/json', enabled: true }],
      params: [],
      auth: { type: 'none' },
      body: { type: 'none' },
      scripts: { preRequest: '', postResponse: '' },
      settings: { followRedirects: true, verifySSL: true, timeoutMs: 30000 },
    }

    try {
      const res = await fetch('/api/request/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ workspacePath: currentWs, path: fullPath, request: newReq }),
      })
      if (res.ok) {
        await get().loadWorkspace(currentWs)
        set({
          activeRequest: newReq,
          activeFilePath: fullPath,
          savedRequestSnapshot: JSON.stringify(newReq),
          diskConflict: null,
        })
        return fullPath
      }
    } catch (err) {
      console.error('Failed to create new request file:', err)
    }
    return null
  },
}))
