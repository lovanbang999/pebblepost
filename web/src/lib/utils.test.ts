import { describe, it, expect } from 'vitest'
import { formatBytes, formatDuration, getMethodColor, getMethodTextColor } from './utils'

describe('formatBytes', () => {
  it('returns "0 Bytes" for 0', () => {
    expect(formatBytes(0)).toBe('0 Bytes')
  })

  it('formats bytes correctly', () => {
    expect(formatBytes(1024)).toBe('1 KB')
    expect(formatBytes(1024 * 1024)).toBe('1 MB')
    expect(formatBytes(1536)).toBe('1.5 KB')
  })

  it('respects decimal places', () => {
    expect(formatBytes(1536, 0)).toBe('2 KB')
    expect(formatBytes(1500, 3)).toBe('1.465 KB')
  })
})

describe('formatDuration', () => {
  it('shows ms for sub-second values', () => {
    expect(formatDuration(0)).toBe('0 ms')
    expect(formatDuration(50)).toBe('50 ms')
    expect(formatDuration(999)).toBe('999 ms')
    expect(formatDuration(500)).toBe('500 ms')
  })

  it('shows seconds for >= 1000ms', () => {
    expect(formatDuration(1000)).toBe('1.00 s')
    expect(formatDuration(1500)).toBe('1.50 s')
    expect(formatDuration(30000)).toBe('30.00 s')
  })
})

describe('getMethodColor', () => {
  it('returns correct class for GET', () => {
    const cls = getMethodColor('GET')
    expect(cls).toContain('emerald')
  })

  it('returns correct class for POST', () => {
    const cls = getMethodColor('POST')
    expect(cls).toContain('blue')
  })

  it('returns correct class for PUT', () => {
    const cls = getMethodColor('PUT')
    expect(cls).toContain('amber')
  })

  it('returns correct class for PATCH', () => {
    const cls = getMethodColor('PATCH')
    expect(cls).toContain('violet')
  })

  it('returns correct class for DELETE', () => {
    const cls = getMethodColor('DELETE')
    expect(cls).toContain('rose')
  })

  it('is case-insensitive', () => {
    expect(getMethodColor('get')).toBe(getMethodColor('GET'))
    expect(getMethodColor('post')).toBe(getMethodColor('POST'))
  })

  it('returns default for unknown methods', () => {
    const cls = getMethodColor('CONNECT')
    expect(cls).toContain('zinc')
  })
})

describe('getMethodTextColor', () => {
  it('returns color for known methods', () => {
    expect(getMethodTextColor('GET')).toContain('emerald')
    expect(getMethodTextColor('DELETE')).toContain('rose')
    expect(getMethodTextColor('HEAD')).toContain('teal')
    expect(getMethodTextColor('OPTIONS')).toContain('teal')
  })

  it('returns default color for unknown methods', () => {
    expect(getMethodTextColor('TRACE')).toContain('zinc')
  })
})
