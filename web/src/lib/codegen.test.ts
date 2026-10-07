import { describe, it, expect } from 'vitest'
import { generateCURL, generateNodeFetch, generateGo } from './codegen'
import type { RequestDefinition } from '../types'

function makeRequest(overrides: Partial<RequestDefinition> = {}): RequestDefinition {
  return {
    schemaVersion: 1,
    id: 'test-1',
    name: 'Test Request',
    method: 'GET',
    url: 'https://api.example.com/users',
    headers: [],
    params: [],
    body: { type: 'none' },
    auth: { type: 'none' },
    scripts: {},
    settings: { followRedirects: true, verifySSL: true, timeoutMs: 30000 },
    ...overrides,
  }
}

describe('generateCURL', () => {
  it('generates basic GET curl command', () => {
    const curl = generateCURL(makeRequest())
    expect(curl).toContain("curl -X GET 'https://api.example.com/users'")
  })

  it('includes POST method', () => {
    const curl = generateCURL(makeRequest({ method: 'POST' }))
    expect(curl).toContain('curl -X POST')
  })

  it('includes headers', () => {
    const curl = generateCURL(
      makeRequest({
        headers: [
          { key: 'Content-Type', value: 'application/json', enabled: true },
          { key: 'X-Disabled', value: 'ignored', enabled: false },
        ],
      }),
    )
    expect(curl).toContain("-H 'Content-Type: application/json'")
    expect(curl).not.toContain('X-Disabled')
  })

  it('appends query params', () => {
    const curl = generateCURL(
      makeRequest({
        params: [
          { key: 'page', value: '1', enabled: true },
          { key: 'limit', value: '20', enabled: true },
          { key: 'hidden', value: 'x', enabled: false },
        ],
      }),
    )
    expect(curl).toContain('page=1')
    expect(curl).toContain('limit=20')
    expect(curl).not.toContain('hidden')
  })

  it('includes Bearer auth header', () => {
    const curl = generateCURL(
      makeRequest({ auth: { type: 'bearer', token: 'my-jwt-token' } }),
    )
    expect(curl).toContain("-H 'Authorization: Bearer my-jwt-token'")
  })

  it('includes Basic auth header', () => {
    const curl = generateCURL(
      makeRequest({ auth: { type: 'basic', username: 'user', password: 'pass' } }),
    )
    expect(curl).toContain('Authorization: Basic')
  })

  it('includes JSON body', () => {
    const curl = generateCURL(
      makeRequest({
        method: 'POST',
        body: { type: 'json', raw: '{"name":"Alice"}' },
      }),
    )
    expect(curl).toContain('application/json')
    expect(curl).toContain('Alice')
  })

  it('escapes single quotes in values', () => {
    const curl = generateCURL(
      makeRequest({ url: "https://api.example.com/search?q=it's" }),
    )
    // Should not break shell quoting
    expect(curl).toContain("curl -X GET")
  })
})

describe('generateNodeFetch', () => {
  it('generates fetch with method', () => {
    const code = generateNodeFetch(makeRequest({ method: 'DELETE' }))
    expect(code).toContain('method: "DELETE"')
  })

  it('includes headers in fetch', () => {
    const code = generateNodeFetch(
      makeRequest({
        headers: [{ key: 'X-Custom', value: 'val', enabled: true }],
      }),
    )
    expect(code).toContain('"X-Custom": "val"')
  })

  it('includes JSON body in fetch', () => {
    const code = generateNodeFetch(
      makeRequest({
        method: 'POST',
        body: { type: 'json', raw: '{"ok":true}' },
      }),
    )
    expect(code).toContain('body:')
    expect(code).toContain('ok')
  })

  it('includes bearer auth header in fetch', () => {
    const code = generateNodeFetch(
      makeRequest({ auth: { type: 'bearer', token: 'tok123' } }),
    )
    expect(code).toContain('Authorization')
    expect(code).toContain('Bearer tok123')
  })
})

describe('generateGo', () => {
  it('generates axios.request call', () => {
    const code = generateGo(makeRequest())
    expect(code).toContain('http.NewRequest')
    expect(code).toContain('GET')
  })

  it('includes url', () => {
    const code = generateGo(makeRequest())
    expect(code).toContain('api.example.com')
  })

  it('generates GraphQL body with query and variables', () => {
    const gqlReq = makeRequest({
      method: 'POST',
      body: {
        type: 'graphql',
        graphql: {
          query: 'query GetUser($id: ID!) { user(id: $id) { id name } }',
          variables: '{"id": "42"}',
          operationName: 'GetUser',
        },
      },
    })

    const curl = generateCURL(gqlReq)
    expect(curl).toContain("-H 'Content-Type: application/json'")
    expect(curl).toContain('GetUser')
    expect(curl).toContain('"id":"42"')

    const fetchCode = generateNodeFetch(gqlReq)
    expect(fetchCode).toContain('application/json')
    expect(fetchCode).toContain('GetUser')

    const goCode = generateGo(gqlReq)
    expect(goCode).toContain('Content-Type')
    expect(goCode).toContain('GetUser')
  })
})

