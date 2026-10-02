import { test, describe, beforeEach } from 'node:test'
import assert from 'node:assert/strict'
import { useTabStore } from './tabStore.ts'
import type { RequestDefinition } from '../types.ts'

// Provide minimal browser polyfills for headless node:test environment
const mockStorage: Record<string, string> = {}
globalThis.localStorage = {
  getItem: (key: string) => mockStorage[key] ?? null,
  setItem: (key: string, val: string) => {
    mockStorage[key] = val
  },
  removeItem: (key: string) => {
    delete mockStorage[key]
  },
  clear: () => {
    for (const k of Object.keys(mockStorage)) delete mockStorage[k]
  },
  key: (i: number) => Object.keys(mockStorage)[i] ?? null,
  length: Object.keys(mockStorage).length,
}
;(globalThis as any).window = globalThis

const sampleReq: RequestDefinition = {
  schemaVersion: 1,
  id: 'req-1',
  name: 'Get User',
  method: 'GET',
  url: 'https://api.example.com/user',
  headers: [],
  params: [],
  auth: { type: 'none' },
}

const sampleReq2: RequestDefinition = {
  schemaVersion: 1,
  id: 'req-2',
  name: 'Create User',
  method: 'POST',
  url: 'https://api.example.com/user',
  headers: [],
  params: [],
  auth: { type: 'none' },
}

describe('useTabStore', () => {
  beforeEach(() => {
    useTabStore.setState({
      tabs: [],
      activeTabId: null,
      closedTabsHistory: [],
      pendingCloseTab: null,
    })
    globalThis.localStorage.clear()
  })

  test('opens a preview tab on single-click', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, true)

    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 1)
    assert.equal(state.tabs[0].isPreview, true)
    assert.equal(state.tabs[0].isDirty, false)
    assert.equal(state.tabs[0].title, 'Get User')
    assert.equal(state.activeTabId, 'collections/user/get-user.pebble.json')
  })

  test('single-click on another request replaces the existing unpinned preview tab', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, true)
    store.openTab('collections/user/create-user.pebble.json', sampleReq2, true)

    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 1)
    assert.equal(state.tabs[0].id, 'collections/user/create-user.pebble.json')
    assert.equal(state.tabs[0].title, 'Create User')
    assert.equal(state.tabs[0].isPreview, true)
  })

  test('double-click pins the tab and opens additional tabs without replacing', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, false)
    store.openTab('collections/user/create-user.pebble.json', sampleReq2, false)

    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 2)
    assert.equal(state.tabs[0].isPreview, false)
    assert.equal(state.tabs[1].isPreview, false)
  })

  test('pinTab explicitly converts a preview tab to pinned', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, true)
    store.pinTab('collections/user/get-user.pebble.json')

    const state = useTabStore.getState()
    assert.equal(state.tabs[0].isPreview, false)
  })

  test('editing a request marks tab as dirty and automatically promotes from preview to pinned', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, true)

    store.updateActiveRequest((prev) => ({
      ...prev,
      url: 'https://api.example.com/user?modified=1',
    }))

    const state = useTabStore.getState()
    assert.equal(state.tabs[0].isDirty, true)
    assert.equal(state.tabs[0].isPreview, false)
  })

  test('closing an unsaved dirty tab triggers confirmation requirement', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, false)
    store.updateActiveRequest((prev) => ({
      ...prev,
      name: 'Renamed Query',
    }))

    const closed = store.closeTab('collections/user/get-user.pebble.json')
    assert.equal(closed, false)

    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 1)
    assert.notEqual(state.pendingCloseTab, null)
    assert.equal(state.pendingCloseTab?.title, 'Renamed Query')
  })

  test('confirmCloseTab with cancel restores normal state without closing', async () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, false)
    store.updateActiveRequest((prev) => ({ ...prev, name: 'Renamed' }))
    store.closeTab('collections/user/get-user.pebble.json')

    await store.confirmCloseTab('cancel')

    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 1)
    assert.equal(state.pendingCloseTab, null)
  })

  test('confirmCloseTab with discard closes the tab immediately', async () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, false)
    store.updateActiveRequest((prev) => ({ ...prev, name: 'Renamed' }))
    store.closeTab('collections/user/get-user.pebble.json')

    await store.confirmCloseTab('discard')

    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 0)
    assert.equal(state.pendingCloseTab, null)
    assert.equal(state.closedTabsHistory.length, 1)
  })

  test('reopenLastClosedTab brings back the most recently closed tab', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, false)
    store.closeTab('collections/user/get-user.pebble.json')

    assert.equal(useTabStore.getState().tabs.length, 0)

    useTabStore.getState().reopenLastClosedTab()
    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 1)
    assert.equal(state.tabs[0].title, 'Get User')
  })

  test('cycleTabs navigates forward and backward circularly', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, false)
    store.openTab('collections/user/create-user.pebble.json', sampleReq2, false)

    assert.equal(useTabStore.getState().activeTabId, 'collections/user/create-user.pebble.json')

    useTabStore.getState().cycleTabs('next')
    assert.equal(useTabStore.getState().activeTabId, 'collections/user/get-user.pebble.json')

    useTabStore.getState().cycleTabs('prev')
    assert.equal(useTabStore.getState().activeTabId, 'collections/user/create-user.pebble.json')
  })

  test('onFileRenamed updates open tab path and title', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/old-name.pebble.json', sampleReq, false)

    store.onFileRenamed(
      'collections/user/old-name.pebble.json',
      'collections/user/new-name.pebble.json',
      'new-name'
    )

    const state = useTabStore.getState()
    assert.equal(state.tabs[0].id, 'collections/user/new-name.pebble.json')
    assert.equal(state.tabs[0].filePath, 'collections/user/new-name.pebble.json')
    assert.equal(state.tabs[0].title, 'new-name')
    assert.equal(state.activeTabId, 'collections/user/new-name.pebble.json')
  })

  test('onFileDeleted removes tab if deleted', () => {
    const store = useTabStore.getState()
    store.openTab('collections/user/get-user.pebble.json', sampleReq, false)
    store.openTab('collections/user/create-user.pebble.json', sampleReq2, false)

    store.onFileDeleted('collections/user/get-user.pebble.json')

    const state = useTabStore.getState()
    assert.equal(state.tabs.length, 1)
    assert.equal(state.tabs[0].id, 'collections/user/create-user.pebble.json')
  })
})
