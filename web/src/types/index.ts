export interface KeyValue {
  key: string
  value: string
  enabled: boolean
  type?: 'text' | 'file'
  secret?: boolean
}

export interface AuthDefinition {
  type: 'inherit' | 'none' | 'bearer' | 'basic' | 'apiKey' | 'digest' | 'oauth2' | 'awsSigV4'
  token?: string
  username?: string
  password?: string
  key?: string
  value?: string
  addTo?: 'header' | 'query'

  // Digest
  realm?: string

  // OAuth2
  grantType?: 'authorization_code' | 'client_credentials'
  authUrl?: string
  tokenUrl?: string
  clientId?: string
  clientSecret?: string
  scope?: string
  redirectUrl?: string
  codeVerifier?: string
  refreshToken?: string
  tokenExpiresAt?: number

  // AWS Signature v4
  accessKey?: string
  secretKey?: string
  region?: string
  service?: string
  sessionToken?: string
}

export interface GraphQLDefinition {
  query: string
  variables?: string
}

export interface BodyDefinition {
  type: 'none' | 'json' | 'raw' | 'formData' | 'urlEncoded' | 'graphql' | 'file'
  raw?: string
  filePath?: string
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
  connectTimeoutMs?: number
  maxRedirects?: number
  enableCookies?: boolean
  userAgent?: string
  proxyUrl?: string
  clientCertPath?: string
  clientKeyPath?: string
}

export interface CookieItem {
  name: string
  value: string
  domain: string
  path: string
  expires?: string
  maxAge?: number
  secure: boolean
  httpOnly: boolean
  sameSite?: string
}

export interface RequestDefinition {
  $schema?: string
  schemaVersion?: number
  version?: string
  id?: string
  name: string
  description?: string
  order?: number
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
  secret?: boolean
  isSecret?: boolean
}

export interface EnvironmentDefinition {
  schemaVersion?: number
  name: string
  variables: EnvironmentVariable[]
}

export interface FolderDefinition {
  schemaVersion?: number
  name?: string
  description?: string
  order?: number
  itemOrder?: string[]
  headers?: KeyValue[]
  auth?: AuthDefinition
  variables?: KeyValue[]
  scripts?: ScriptDefinition
}

export interface InheritedItemInfo {
  sourceFolder: string
  sourcePath: string
}

export interface ResolvedRequestResult {
  request: RequestDefinition
  inheritedHeaders?: Record<string, InheritedItemInfo>
  overriddenHeaders?: Record<string, InheritedItemInfo>
  inheritedAuth?: InheritedItemInfo
  parentAuth?: AuthDefinition
  parentAuthSource?: InheritedItemInfo
  inheritedVars?: Record<string, InheritedItemInfo>
  folderPreScripts?: string[]
  folderPostScripts?: string[]
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

export interface ConsoleLogEntry {
  timestamp: string
  level: 'log' | 'info' | 'warn' | 'error'
  source: string
  message: string
  line?: number
  column?: number
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
  consoleLogs?: ConsoleLogEntry[]
  extractedEnvVars?: Record<string, string>
  executedAt: string
  error?: string
}

export interface TreeNode {
  id: string
  name: string
  displayName?: string
  order?: number
  path: string
  relPath: string
  isDir: boolean
  method?: string
  children?: TreeNode[]
}

