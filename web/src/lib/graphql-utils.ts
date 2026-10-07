import { buildClientSchema, type GraphQLSchema, type IntrospectionQuery } from 'graphql'

export interface ParsedOperation {
  type: 'query' | 'mutation' | 'subscription'
  name: string
}

/**
 * Extracts named operations from a GraphQL query document.
 */
export function parseOperations(query: string): ParsedOperation[] {
  if (!query || !query.trim()) return []

  const operations: ParsedOperation[] = []
  // Matches query|mutation|subscription followed by an optional name and variable definitions
  const opRegex = /(?:^|\s)(query|mutation|subscription)\s+([_A-Za-z][_0-9A-Za-z]*)/g

  let match: RegExpExecArray | null
  while ((match = opRegex.exec(query)) !== null) {
    operations.push({
      type: match[1] as 'query' | 'mutation' | 'subscription',
      name: match[2],
    })
  }

  return operations
}

/**
 * Validates a JSON string (used for GraphQL variables editor).
 */
export function validateVariablesJson(jsonStr: string): { isValid: boolean; error?: string } {
  const trimmed = jsonStr.trim()
  if (!trimmed) {
    return { isValid: true }
  }

  try {
    const parsed = JSON.parse(trimmed)
    if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
      return { isValid: false, error: 'Variables must be a JSON object (e.g. {"id": 1})' }
    }
    return { isValid: true }
  } catch (err: unknown) {
    const message = err instanceof Error ? err.message : 'Invalid JSON format'
    return { isValid: false, error: message }
  }
}

/**
 * Converts an introspection query result JSON into a GraphQLSchema instance safely.
 */
export function buildClientSchemaSafe(data: unknown): GraphQLSchema | null {
  if (!data || typeof data !== 'object') return null
  try {
    const obj = data as Record<string, unknown>
    const introspection = (obj.data ? obj.data : obj) as IntrospectionQuery
    if (introspection && introspection.__schema) {
      return buildClientSchema(introspection)
    }
    return null
  } catch (err) {
    console.warn('Failed to build GraphQL client schema from introspection:', err)
    return null
  }
}

export interface GraphQLTypeRef {
  kind?: string
  name?: string | null
  ofType?: GraphQLTypeRef | null
}

/**
 * Formats a GraphQL introspection type reference into a human-readable signature (e.g. [User!]!).
 */
export function formatTypeRef(typeRef?: GraphQLTypeRef | null): string {
  if (!typeRef) return 'Unknown'

  if (typeRef.kind === 'NON_NULL') {
    return `${formatTypeRef(typeRef.ofType)}!`
  }
  if (typeRef.kind === 'LIST') {
    return `[${formatTypeRef(typeRef.ofType)}]`
  }
  return typeRef.name || 'Unknown'
}
