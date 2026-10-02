import { create } from 'zustand'
import type { RequestDefinition, ExecutionResult, TreeNode, EnvironmentDefinition } from '../types'

interface WorkspaceState {
  workspacePath: string | null
  activeEnv: string
  environments: EnvironmentDefinition[]
  tree: TreeNode[]
  activeRequest: RequestDefinition | null
  activeFilePath: string | null
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

  theme: 'light' | 'dark'
  toggleTheme: () => void
  setTheme: (theme: 'light' | 'dark') => void

  // Backend integration actions
  loadWorkspace: (path?: string) => Promise<void>
  loadRequest: (filePath: string) => Promise<void>
  saveCurrentRequest: () => Promise<boolean>
  createNewRequest: (
    folderPath?: string,
    name?: string,
    method?: RequestDefinition['method']
  ) => Promise<string | null>
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
  },
  setActiveEnv: (env) => set({ activeEnv: env }),
  setEnvironments: (environments) => set({ environments }),
  setTree: (tree) => set({ tree }),
  setActiveRequest: (activeRequest, activeFilePath = null) =>
    set({ activeRequest, activeFilePath, lastResult: null }),
  setIsExecuting: (isExecuting) => set({ isExecuting }),
  setLastResult: (lastResult) => set({ lastResult }),
  setActiveTab: (activeTab) => set({ activeTab }),
  updateActiveRequest: (updater) =>
    set((state) => ({
      activeRequest: state.activeRequest ? updater(state.activeRequest) : null,
    })),

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
        })
      }
    } catch (err) {
      console.warn('Could not scan workspace via API (running offline or standalone mode):', err)
    } finally {
      set({ isLoadingWorkspace: false })
    }
  },

  loadRequest: async (filePath: string) => {
    try {
      const res = await fetch(`/api/request?path=${encodeURIComponent(filePath)}`)
      if (res.ok) {
        const reqData: RequestDefinition = await res.json()
        set({ activeRequest: reqData, activeFilePath: filePath, lastResult: null })
      }
    } catch (err) {
      console.error('Failed to load request:', err)
    }
  },

  saveCurrentRequest: async () => {
    const { activeFilePath, activeRequest } = get()
    if (!activeFilePath || !activeRequest) return false

    try {
      const res = await fetch('/api/request/save', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({
          path: activeFilePath,
          request: activeRequest,
        }),
      })
      if (res.ok) {
        // Refresh tree to update method and name badges
        const { workspacePath, loadWorkspace } = get()
        if (workspacePath) {
          loadWorkspace(workspacePath)
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
        body: JSON.stringify({ path: fullPath, request: newReq }),
      })
      if (res.ok) {
        await get().loadWorkspace(currentWs)
        set({ activeRequest: newReq, activeFilePath: fullPath })
        return fullPath
      }
    } catch (err) {
      console.error('Failed to create new request file:', err)
    }
    return null
  },
}))
