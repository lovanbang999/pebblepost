import { create } from 'zustand'
import type { HistoryEntry, HistoryListResponse } from '../types'

export type TimeRangeFilter = 'all' | 'today' | 'week' | 'month'

export interface HistoryFilter {
  search: string
  statusCode: number   // 0 = any
  method: string       // '' = any
  timeRange: TimeRangeFilter
  page: number
  limit: number
}

interface HistoryState {
  entries: HistoryEntry[]
  total: number
  isLoading: boolean
  error: string | null
  filter: HistoryFilter

  // Selection for compare
  compareIds: [number, number] | null
  compareEntries: [HistoryEntry, HistoryEntry] | null
  isComparing: boolean

  // Actions
  setFilter: (patch: Partial<HistoryFilter>) => void
  loadHistory: (workspacePath: string) => Promise<void>
  loadEntry: (id: number) => Promise<HistoryEntry | null>
  deleteEntry: (workspacePath: string, id: number) => Promise<void>
  clearHistory: (workspacePath: string) => Promise<void>
  selectForCompare: (id: number) => void
  clearCompare: () => void
  loadCompare: () => Promise<void>
}

const API_BASE = '/api'

function buildUrl(workspacePath: string, filter: HistoryFilter): string {
  const params = new URLSearchParams({ workspace: workspacePath })
  if (filter.search) params.set('search', filter.search)
  if (filter.statusCode > 0) params.set('status', String(filter.statusCode))
  if (filter.method) params.set('method', filter.method)
  if (filter.timeRange !== 'all') {
    const now = new Date()
    let since: Date | null = null
    if (filter.timeRange === 'today') {
      since = new Date(now.getFullYear(), now.getMonth(), now.getDate())
    } else if (filter.timeRange === 'week') {
      since = new Date(now.getTime() - 7 * 24 * 60 * 60 * 1000)
    } else if (filter.timeRange === 'month') {
      since = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000)
    }
    if (since) {
      params.set('since', since.toISOString())
    }
  }
  params.set('page', String(filter.page))
  params.set('limit', String(filter.limit))
  return `${API_BASE}/history?${params.toString()}`
}

export const useHistoryStore = create<HistoryState>((set, get) => ({
  entries: [],
  total: 0,
  isLoading: false,
  error: null,
  filter: { search: '', statusCode: 0, method: '', timeRange: 'all', page: 1, limit: 50 },
  compareIds: null,
  compareEntries: null,
  isComparing: false,

  setFilter: (patch) => {
    set((s) => ({ filter: { ...s.filter, ...patch, page: 1 } }))
  },

  loadHistory: async (workspacePath) => {
    set({ isLoading: true, error: null })
    try {
      const url = buildUrl(workspacePath, get().filter)
      const res = await fetch(url)
      if (!res.ok) throw new Error(`HTTP ${res.status}`)
      const data: HistoryListResponse = await res.json()
      set({ entries: data.entries ?? [], total: data.total, isLoading: false })
    } catch (e) {
      set({ error: String(e), isLoading: false })
    }
  },

  loadEntry: async (id) => {
    try {
      const res = await fetch(`${API_BASE}/history/${id}`)
      if (!res.ok) return null
      return (await res.json()) as HistoryEntry
    } catch {
      return null
    }
  },

  deleteEntry: async (workspacePath, id) => {
    await fetch(`${API_BASE}/history/${id}`, { method: 'DELETE' })
    await get().loadHistory(workspacePath)
  },

  clearHistory: async (workspacePath) => {
    const params = new URLSearchParams({ workspace: workspacePath })
    await fetch(`${API_BASE}/history?${params}`, { method: 'DELETE' })
    set({ entries: [], total: 0 })
  },

  selectForCompare: (id) => {
    const prev = get().compareIds
    if (!prev) {
      set({ compareIds: [id, id], isComparing: false })
      return
    }
    if (prev[0] === id) {
      set({ compareIds: null, compareEntries: null, isComparing: false })
      return
    }
    set({ compareIds: [prev[0], id], isComparing: false })
  },

  clearCompare: () => set({ compareIds: null, compareEntries: null, isComparing: false }),

  loadCompare: async () => {
    const ids = get().compareIds
    if (!ids || ids[0] === ids[1]) return
    set({ isComparing: true })
    const [a, b] = await Promise.all([get().loadEntry(ids[0]), get().loadEntry(ids[1])])
    if (a && b) {
      set({ compareEntries: [a, b], isComparing: false })
    } else {
      set({ isComparing: false })
    }
  },
}))
