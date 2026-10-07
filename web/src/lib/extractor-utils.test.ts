import { describe, it, expect } from 'vitest'
import {
  buildJsonPathFromKeyPath,
  suggestVariableName,
  evaluateJsonPath,
} from './extractor-utils'

describe('extractor-utils', () => {
  describe('buildJsonPathFromKeyPath', () => {
    it('returns $ for empty segments', () => {
      expect(buildJsonPathFromKeyPath([])).toBe('$')
    })

    it('builds standard dot notation for valid identifiers', () => {
      expect(buildJsonPathFromKeyPath(['data', 'user', 'id'])).toBe('$.data.user.id')
    })

    it('builds bracket index notation for arrays', () => {
      expect(buildJsonPathFromKeyPath(['items', 0, 'name'])).toBe('$.items[0].name')
    })

    it('escapes special characters using bracket string notation', () => {
      expect(buildJsonPathFromKeyPath(['data', 'user-name', 'special.key'])).toBe(
        '$.data["user-name"]["special.key"]'
      )
    })
  })

  describe('suggestVariableName', () => {
    it('uses last segment string as variable name', () => {
      expect(suggestVariableName(['data', 'token'])).toBe('token')
      expect(suggestVariableName(['user-id'])).toBe('user_id')
    })

    it('handles array index by using singular form of parent segment', () => {
      expect(suggestVariableName(['items', 0])).toBe('item_item')
    })

    it('falls back to default for empty segments', () => {
      expect(suggestVariableName([])).toBe('extracted_val')
    })
  })

  describe('evaluateJsonPath', () => {
    const jsonStr = JSON.stringify({
      token: 'jwt_abc_123',
      user: { id: 42, active: true },
      items: [{ name: 'keyboard', price: 99 }],
    })

    it('extracts top-level string primitive', () => {
      const res = evaluateJsonPath(jsonStr, '$.token')
      expect(res.success).toBe(true)
      expect(res.value).toBe('jwt_abc_123')
    })

    it('extracts nested number and boolean', () => {
      expect(evaluateJsonPath(jsonStr, '$.user.id').value).toBe('42')
      expect(evaluateJsonPath(jsonStr, '$.user.active').value).toBe('true')
    })

    it('extracts array element and serializes object to compact JSON', () => {
      const res = evaluateJsonPath(jsonStr, '$.items[0]')
      expect(res.success).toBe(true)
      expect(res.value).toBe('{"name":"keyboard","price":99}')
    })

    it('returns warning when path is missing', () => {
      const res = evaluateJsonPath(jsonStr, '$.nonExistent')
      expect(res.success).toBe(false)
      expect(res.warning).toContain('not found')
    })

    it('returns warning for invalid JSON body', () => {
      const res = evaluateJsonPath('invalid json string', '$.token')
      expect(res.success).toBe(false)
      expect(res.warning).toContain('not valid JSON')
    })
  })
})
