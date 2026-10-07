import { describe, it, expect } from 'vitest'
import {
  parseOperations,
  validateVariablesJson,
  formatTypeRef,
  buildClientSchemaSafe,
} from './graphql-utils'

describe('graphql-utils', () => {
  describe('parseOperations', () => {
    it('returns empty array for empty query', () => {
      expect(parseOperations('')).toEqual([])
      expect(parseOperations('   ')).toEqual([])
    })

    it('extracts named queries, mutations, and subscriptions', () => {
      const doc = `
        query GetUserProfile($id: ID!) {
          user(id: $id) { id name }
        }

        mutation UpdateUser($name: String!) {
          updateUser(name: $name) { id }
        }

        subscription OnUserOnline {
          userOnline { id }
        }
      `
      const ops = parseOperations(doc)
      expect(ops).toHaveLength(3)
      expect(ops[0]).toEqual({ type: 'query', name: 'GetUserProfile' })
      expect(ops[1]).toEqual({ type: 'mutation', name: 'UpdateUser' })
      expect(ops[2]).toEqual({ type: 'subscription', name: 'OnUserOnline' })
    })

    it('ignores anonymous queries', () => {
      const doc = `{ user { id } }`
      expect(parseOperations(doc)).toEqual([])
    })
  })

  describe('validateVariablesJson', () => {
    it('considers empty string valid', () => {
      expect(validateVariablesJson('')).toEqual({ isValid: true })
      expect(validateVariablesJson('   ')).toEqual({ isValid: true })
    })

    it('considers valid JSON object valid', () => {
      expect(validateVariablesJson('{"id": "123", "limit": 10}')).toEqual({ isValid: true })
    })

    it('rejects invalid JSON syntax', () => {
      const result = validateVariablesJson('{id: 123}')
      expect(result.isValid).toBe(false)
      expect(result.error).toBeDefined()
    })

    it('rejects non-object JSON values like numbers or arrays', () => {
      expect(validateVariablesJson('123').isValid).toBe(false)
      expect(validateVariablesJson('"hello"').isValid).toBe(false)
      expect(validateVariablesJson('[1, 2, 3]').isValid).toBe(false)
    })
  })

  describe('formatTypeRef', () => {
    it('formats scalar type', () => {
      expect(formatTypeRef({ kind: 'SCALAR', name: 'String' })).toBe('String')
    })

    it('formats non-null scalar', () => {
      expect(
        formatTypeRef({
          kind: 'NON_NULL',
          ofType: { kind: 'SCALAR', name: 'ID' },
        })
      ).toBe('ID!')
    })

    it('formats non-null list of non-null objects', () => {
      expect(
        formatTypeRef({
          kind: 'NON_NULL',
          ofType: {
            kind: 'LIST',
            ofType: {
              kind: 'NON_NULL',
              ofType: { kind: 'OBJECT', name: 'User' },
            },
          },
        })
      ).toBe('[User!]!')
    })
  })

  describe('buildClientSchemaSafe', () => {
    it('returns null for empty data', () => {
      expect(buildClientSchemaSafe(null)).toBeNull()
      expect(buildClientSchemaSafe({})).toBeNull()
    })

    it('creates schema from introspection', () => {
      const introspection = {
        data: {
          __schema: {
            queryType: { name: 'Query' },
            mutationType: null,
            subscriptionType: null,
            types: [
              {
                kind: 'OBJECT',
                name: 'Query',
                fields: [
                  {
                    name: 'ping',
                    args: [],
                    type: { kind: 'SCALAR', name: 'String', ofType: null },
                    isDeprecated: false,
                  },
                ],
                interfaces: [],
              },
              {
                kind: 'SCALAR',
                name: 'String',
              },
            ],
            directives: [],
          },
        },
      }
      const schema = buildClientSchemaSafe(introspection)
      expect(schema).not.toBeNull()
      expect(schema?.getQueryType()?.name).toBe('Query')
    })
  })
})
