export interface KeyValue {
  key: string
  value: string
  enabled: boolean
  type?: 'text' | 'file'
}

export interface AuthDefinition {
  type: 'none' | 'bearer' | 'basic' | 'apiKey' | 'oauth2'
  token?: string
  username?: string
  password?: string
  key?: string
  value?: string
  addTo?: 'header' | 'query'
}

export interface GraphQLDefinition {
  query: string
  variables?: string
}

export interface BodyDefinition {
  type: 'none' | 'json' | 'raw' | 'formData' | 'urlEncoded' | 'graphql'
  raw?: string
  formData?: KeyValue[]
  urlEncoded?: KeyValue[]
  graphql?: GraphQLDefinition
}

export interface ScriptDefinition {
  preRequest?: string
  postResponse?: string
}

export interface SettingDefinition {
  followRedirects: boolean
  verifySSL: boolean
  timeoutMs: number
}

export interface RequestDefinition {
  $schema?: string
  version?: string
  id?: string
  name: string
  description?: string
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS'
  url: string
  headers?: KeyValue[]
  params?: KeyValue[]
  auth: AuthDefinition
  body: BodyDefinition
  scripts: ScriptDefinition
  settings: SettingDefinition
}

export interface EnvironmentVariable {
  key: string
  value: string
  enabled: boolean
  isSecret?: boolean
}

export interface EnvironmentDefinition {
  name: string
  variables: EnvironmentVariable[]
}

export interface TimingMetrics {
  dnsLookupMs: number
  tcpConnectMs: number
  tlsHandshakeMs: number
  ttfbMs: number
  downloadMs: number
  totalDurationMs: number
}

export interface TestAssertionResult {
  name: string
  passed: boolean
  message?: string
}

export interface ExecutionResult {
  statusCode: number
  statusText: string
  headers: Record<string, string[]>
  body: string
  size: number
  timing: TimingMetrics
  tests: TestAssertionResult[]
  logs: string[]
  extractedEnvVars?: Record<string, string>
  executedAt: string
  error?: string
}

export interface TreeNode {
  id: string
  name: string
  path: string
  relPath: string
  isDir: boolean
  method?: string
  children?: TreeNode[]
}
