import '@testing-library/jest-dom'

// Provide minimal localStorage stub in jsdom
const mockStorage: Record<string, string> = {}
Object.defineProperty(global, 'localStorage', {
  value: {
    getItem: (key: string) => mockStorage[key] ?? null,
    setItem: (key: string, val: string) => { mockStorage[key] = val },
    removeItem: (key: string) => { delete mockStorage[key] },
    clear: () => { for (const k of Object.keys(mockStorage)) delete mockStorage[k] },
    key: (i: number) => Object.keys(mockStorage)[i] ?? null,
    get length() { return Object.keys(mockStorage).length },
  },
  writable: false,
})
