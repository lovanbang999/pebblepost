import { create } from 'zustand'
import type {
  GitStatus,
  GitBranch,
  GitCommit,
  GitDiffEntry,
  GitPushPullResponse,
  GitCheckoutResponse,
} from '../types'

interface GitStoreState {
  status: GitStatus | null
  isLoading: boolean
  isPanelOpen: boolean
  activeTab: 'files' | 'branches' | 'log'
  selectedFile: string | null
  diffEntries: GitDiffEntry[]
  isDiffLoading: boolean
  branches: GitBranch[]
  log: GitCommit[]
  pushPullOutput: GitPushPullResponse | null
  isPushPullRunning: boolean
  lastCheckoutResult: GitCheckoutResponse | null
  error: string | null

  // Actions
  openPanel: (tab?: 'files' | 'branches' | 'log') => void
  closePanel: () => void
  setActiveTab: (tab: 'files' | 'branches' | 'log') => void
  clearError: () => void
  clearPushPullOutput: () => void
  clearCheckoutResult: () => void

  refresh: (workspacePath: string) => Promise<void>
  stage: (paths: string[], workspacePath: string) => Promise<void>
  unstage: (paths: string[], workspacePath: string) => Promise<void>
  commit: (message: string, workspacePath: string) => Promise<void>
  fetchBranches: (workspacePath: string) => Promise<void>
  checkout: (branch: string, workspacePath: string) => Promise<GitCheckoutResponse>
  createBranch: (name: string, workspacePath: string) => Promise<void>
  fetchLog: (workspacePath: string, limit?: number) => Promise<void>
  fetchDiff: (relPath: string, workspacePath: string) => Promise<void>
  closeDiff: () => void
  pull: (workspacePath: string) => Promise<GitPushPullResponse>
  push: (workspacePath: string) => Promise<GitPushPullResponse>
  initRepo: (workspacePath: string) => Promise<void>
}

export const useGitStore = create<GitStoreState>((set, get) => ({
  status: null,
  isLoading: false,
  isPanelOpen: false,
  activeTab: 'files',
  selectedFile: null,
  diffEntries: [],
  isDiffLoading: false,
  branches: [],
  log: [],
  pushPullOutput: null,
  isPushPullRunning: false,
  lastCheckoutResult: null,
  error: null,

  openPanel: (tab) => set({ isPanelOpen: true, ...(tab ? { activeTab: tab } : {}) }),
  closePanel: () => set({ isPanelOpen: false, selectedFile: null, diffEntries: [] }),
  setActiveTab: (activeTab) => set({ activeTab }),
  clearError: () => set({ error: null }),
  clearPushPullOutput: () => set({ pushPullOutput: null }),
  clearCheckoutResult: () => set({ lastCheckoutResult: null }),
  closeDiff: () => set({ selectedFile: null, diffEntries: [] }),

  refresh: async (workspacePath: string) => {
    if (!workspacePath) return
    try {
      const res = await fetch(`/api/git/status?workspacePath=${encodeURIComponent(workspacePath)}`)
      if (!res.ok) {
        throw new Error(`Failed to fetch git status: ${res.statusText}`)
      }
      const data: GitStatus = await res.json()
      if (data && !Array.isArray(data.files)) {
        data.files = []
      }
      set({ status: data, error: null })

      const currentTab = get().activeTab
      if (get().isPanelOpen) {
        if (currentTab === 'branches') {
          void get().fetchBranches(workspacePath)
        } else if (currentTab === 'log') {
          void get().fetchLog(workspacePath)
        }
      }
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      console.warn('Git status check error:', msg)
    }
  },

  stage: async (paths: string[], workspacePath: string) => {
    if (!workspacePath || paths.length === 0) return
    set({ isLoading: true, error: null })
    try {
      const res = await fetch(`/api/git/stage?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error || `Failed to stage: ${res.statusText}`)
      }
      await get().refresh(workspacePath)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg })
      throw err
    } finally {
      set({ isLoading: false })
    }
  },

  unstage: async (paths: string[], workspacePath: string) => {
    if (!workspacePath || paths.length === 0) return
    set({ isLoading: true, error: null })
    try {
      const res = await fetch(`/api/git/unstage?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ paths }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error || `Failed to unstage: ${res.statusText}`)
      }
      await get().refresh(workspacePath)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg })
      throw err
    } finally {
      set({ isLoading: false })
    }
  },

  commit: async (message: string, workspacePath: string) => {
    if (!workspacePath || !message.trim()) return
    set({ isLoading: true, error: null })
    try {
      const res = await fetch(`/api/git/commit?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ message }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error || `Failed to commit: ${res.statusText}`)
      }
      await get().refresh(workspacePath)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg })
      throw err
    } finally {
      set({ isLoading: false })
    }
  },

  fetchBranches: async (workspacePath: string) => {
    if (!workspacePath) return
    try {
      const res = await fetch(`/api/git/branches?workspacePath=${encodeURIComponent(workspacePath)}`)
      if (!res.ok) return
      const data: GitBranch[] = await res.json()
      set({ branches: Array.isArray(data) ? data : [] })
    } catch (err) {
      console.warn('Failed to fetch branches:', err)
    }
  },

  checkout: async (branch: string, workspacePath: string) => {
    if (!workspacePath || !branch) throw new Error('Missing branch or workspace')
    set({ isLoading: true, error: null })
    try {
      const res = await fetch(`/api/git/checkout?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ branch }),
      })
      const data: GitCheckoutResponse = await res.json()
      if (!res.ok) {
        throw new Error((data as unknown as { error?: string }).error || `Failed to checkout: ${res.statusText}`)
      }
      set({ lastCheckoutResult: data })
      await get().refresh(workspacePath)
      await get().fetchBranches(workspacePath)
      return data
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg })
      throw err
    } finally {
      set({ isLoading: false })
    }
  },

  createBranch: async (name: string, workspacePath: string) => {
    if (!workspacePath || !name.trim()) return
    set({ isLoading: true, error: null })
    try {
      const res = await fetch(`/api/git/branch/create?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ name: name.trim() }),
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error || `Failed to create branch: ${res.statusText}`)
      }
      await get().refresh(workspacePath)
      await get().fetchBranches(workspacePath)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg })
      throw err
    } finally {
      set({ isLoading: false })
    }
  },

  fetchLog: async (workspacePath: string, limit = 20) => {
    if (!workspacePath) return
    try {
      const res = await fetch(`/api/git/log?workspacePath=${encodeURIComponent(workspacePath)}&limit=${limit}`)
      if (!res.ok) return
      const data: GitCommit[] = await res.json()
      set({ log: Array.isArray(data) ? data : [] })
    } catch (err) {
      console.warn('Failed to fetch git log:', err)
    }
  },

  fetchDiff: async (relPath: string, workspacePath: string) => {
    if (!workspacePath || !relPath) return
    set({ isDiffLoading: true, selectedFile: relPath, error: null })
    try {
      const res = await fetch(`/api/git/diff?workspacePath=${encodeURIComponent(workspacePath)}&path=${encodeURIComponent(relPath)}`)
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error || `Failed to fetch diff: ${res.statusText}`)
      }
      const data: GitDiffEntry[] = await res.json()
      set({ diffEntries: Array.isArray(data) ? data : [] })
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg })
    } finally {
      set({ isDiffLoading: false })
    }
  },

  pull: async (workspacePath: string) => {
    if (!workspacePath) throw new Error('No workspace path')
    set({ isPushPullRunning: true, error: null })
    try {
      const res = await fetch(`/api/git/pull?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
      })
      const data: GitPushPullResponse = await res.json()
      set({ pushPullOutput: data })
      await get().refresh(workspacePath)
      return data
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      const fallbackResp: GitPushPullResponse = { success: false, output: '', error: msg }
      set({ pushPullOutput: fallbackResp })
      return fallbackResp
    } finally {
      set({ isPushPullRunning: false })
    }
  },

  push: async (workspacePath: string) => {
    if (!workspacePath) throw new Error('No workspace path')
    set({ isPushPullRunning: true, error: null })
    try {
      const res = await fetch(`/api/git/push?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
      })
      const data: GitPushPullResponse = await res.json()
      set({ pushPullOutput: data })
      await get().refresh(workspacePath)
      return data
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      const fallbackResp: GitPushPullResponse = { success: false, output: '', error: msg }
      set({ pushPullOutput: fallbackResp })
      return fallbackResp
    } finally {
      set({ isPushPullRunning: false })
    }
  },

  initRepo: async (workspacePath: string) => {
    if (!workspacePath) return
    set({ isLoading: true, error: null })
    try {
      const res = await fetch(`/api/git/init?workspacePath=${encodeURIComponent(workspacePath)}`, {
        method: 'POST',
      })
      if (!res.ok) {
        const body = await res.json().catch(() => ({}))
        throw new Error(body.error || `Failed to init git: ${res.statusText}`)
      }
      await get().refresh(workspacePath)
    } catch (err: unknown) {
      const msg = err instanceof Error ? err.message : String(err)
      set({ error: msg })
      throw err
    } finally {
      set({ isLoading: false })
    }
  },
}))
