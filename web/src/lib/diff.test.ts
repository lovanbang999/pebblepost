import { describe, it, expect } from 'vitest'
import { computeLineDiff, formatJsonForDiff } from './diff'

describe('computeLineDiff', () => {
  it('detects unchanged lines', () => {
    const text = 'line 1\nline 2'
    const diff = computeLineDiff(text, text)
    expect(diff.length).toBe(2)
    expect(diff[0].type).toBe('unchanged')
    expect(diff[1].type).toBe('unchanged')
  })

  it('detects additions and deletions', () => {
    const oldText = '{\n  "status": "pending"\n}'
    const newText = '{\n  "status": "active",\n  "count": 5\n}'
    const diff = computeLineDiff(oldText, newText)

    expect(diff.some((d) => d.type === 'removed' && d.text.includes('pending'))).toBe(true)
    expect(diff.some((d) => d.type === 'added' && d.text.includes('active'))).toBe(true)
    expect(diff.some((d) => d.type === 'added' && d.text.includes('count'))).toBe(true)
  })

  it('handles empty strings gracefully', () => {
    const diff = computeLineDiff('', '{\n  "a": 1\n}')
    expect(diff.every((d) => d.type === 'added')).toBe(true)
  })
})

describe('formatJsonForDiff', () => {
  it('pretty prints valid JSON', () => {
    const minified = '{"a":1,"b":true}'
    const formatted = formatJsonForDiff(minified)
    expect(formatted).toBe('{\n  "a": 1,\n  "b": true\n}')
  })

  it('returns raw string for non-json', () => {
    const raw = 'plain text'
    expect(formatJsonForDiff(raw)).toBe(raw)
  })
})
