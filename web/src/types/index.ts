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
  operationName?: string
  schemaCache?: unknown
  schemaUrl?: string
  lastIntrospectedAt?: string
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

export interface GrpcDefinition {
  address: string
  protoSource: 'reflection' | 'file'
  protoFiles?: string[]
  importPaths?: string[]
  service: string
  method: string
  metadata?: KeyValue[]
  message?: string
  messages?: string[]
  useTls: boolean
  insecureSkipVerify?: boolean
  rootCaPath?: string
}

export interface GrpcStreamMessage {
  index: number
  direction: 'send' | 'receive'
  timestamp: string
  payload: string
  isError?: boolean
}

export interface GrpcMethodInfo {
  name: string
  fullMethod: string
  clientStreaming: boolean
  serverStreaming: boolean
  inputType: string
  outputType: string
}

export interface GrpcServiceInfo {
  name: string
  methods: GrpcMethodInfo[]
}

export interface WebSocketMessage {
  id: string
  name?: string
  payload: string
  type?: 'text' | 'binary' | 'ping' | 'pong'
}

export interface StreamDefinition {
  subprotocols?: string[]
  autoReconnect?: boolean
  maxReconnectAttempts?: number
  reconnectIntervalMs?: number
  pingIntervalMs?: number
  maxLogEntries?: number
  maxLogBytes?: number
  outgoingMessages?: WebSocketMessage[]
  timeoutMs?: number
  maxWaitMessages?: number
}

export interface StreamLogEntry {
  id: string
  index: number
  direction: 'send' | 'receive' | 'system'
  type: 'text' | 'binary' | 'ping' | 'pong' | 'open' | 'close' | 'error'
  timestamp: string
  payload: string
  size: number
  closeCode?: number
  closeReason?: string
  isError?: boolean
}

export interface StreamSessionStatus {
  streamId: string
  protocol: 'websocket' | 'sse'
  state: 'connecting' | 'connected' | 'disconnected' | 'reconnecting'
  url: string
  subprotocol?: string
  reconnectCount: number
  totalSent: number
  totalReceived: number
  evictedCount: number
  closeCode?: number
  closeReason?: string
}

export interface ExampleResponse {
  id: string
  name: string
  statusCode: number
  statusText?: string
  headers?: KeyValue[]
  body?: string
  contentType?: string
  durationMs?: number
  size?: number
  savedAt?: string
}

export interface RequestDefinition {
  $schema?: string
  schemaVersion?: number
  version?: string
  id?: string
  name: string
  description?: string
  order?: number
  protocol?: 'http' | 'grpc' | 'websocket' | 'sse'
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE' | 'HEAD' | 'OPTIONS' | 'GRPC' | 'WS' | 'SSE' | string
  url: string
  headers?: KeyValue[]
  params?: KeyValue[]
  auth: AuthDefinition
  body: BodyDefinition
  grpc?: GrpcDefinition
  stream?: StreamDefinition
  scripts: ScriptDefinition
  settings: SettingDefinition
  examples?: ExampleResponse[]
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

export interface RedirectHop {
  statusCode: number
  method: string
  url: string
  headers?: Record<string, string>
}

export interface SentRequestSummary {
  method: string
  url: string
  headers: Record<string, string[]>
  body?: string
}

export interface ExecutionResult {
  statusCode: number
  statusText: string
  headers: Record<string, string[]>
  body: string
  bodyTruncated?: boolean
  bodySizeBytes?: number
  tempBodyFile?: string
  size: number
  timing: TimingMetrics
  redirectChain?: RedirectHop[]
  sentRequest?: SentRequestSummary
  grpcStatus?: number
  grpcStatusText?: string
  grpcMetadata?: Record<string, string[]>
  grpcTrailers?: Record<string, string[]>
  grpcMessages?: GrpcStreamMessage[]
  streamLogs?: StreamLogEntry[]
  streamCloseCode?: number
  streamCloseReason?: string
  streamEvicted?: number
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
  gitStatus?: string
  children?: TreeNode[]
}

// ── Git Types ────────────────────────────────────────────────────────────────

export interface GitFileStatus {
  path: string
  code: string
  staged: boolean
  old?: string
}

export interface GitStatus {
  isRepo: boolean
  branch: string
  ahead: number
  behind: number
  dirty: boolean
  files: GitFileStatus[]
}

export interface GitBranch {
  name: string
  current: boolean
  remote?: string
}

export interface GitCommit {
  hash: string
  short: string
  author: string
  date: string
  subject: string
}

export interface GitDiffEntry {
  field: string
  before: string
  after: string
  type: 'text' | 'json' | 'kv' | 'js'
}

export interface GitPushPullResponse {
  success: boolean
  output: string
  error?: string
}

export interface GitCheckoutResponse {
  success: boolean
  message: string
  stashed: boolean
}

export interface HistoryEntry {
  id: number
  workspacePath: string
  requestName: string
  method: string
  url: string
  statusCode: number
  durationMs: number
  sizeBytes: number
  responseBody?: string       // only in detail view
  responseHeaders?: Record<string, string[]>
  resolvedRequest?: Record<string, unknown>
  executedAt: string          // ISO-8601
}

export interface HistoryListResponse {
  entries: HistoryEntry[]
  total: number
  page: number
  limit: number
}

// ── Runner Types ─────────────────────────────────────────────────────────────

export interface RequestRunResult {
  iteration?: number
  filePath: string
  relPath: string
  request?: RequestDefinition
  result?: ExecutionResult
  passed: boolean
  error?: string
  duration: number
  retryCount?: number
  skipped?: boolean
}

export interface IterationSummary {
  iteration: number
  dataRow?: Record<string, unknown>
  results: RequestRunResult[]
  totalTests: number
  passedTests: number
  failedTests: number
  duration: number
  passed: boolean
  error?: string
}

export interface RunSummary {
  target: string
  environment?: string
  totalIterations: number
  passedIterations: number
  failedIterations: number
  totalRequests: number
  passedRequests: number
  failedRequests: number
  skippedRequests?: number
  totalTests: number
  passedTests: number
  failedTests: number
  totalDurationMs: number
  avgDurationMs: number
  p95DurationMs: number
  passRate: number
  bailed: boolean
  dryRun?: boolean
  success: boolean
  results: RequestRunResult[]
  iterations?: IterationSummary[]
}

export interface RunnerTabConfig {
  folderPath?: string
  folderName?: string
}

export interface DocsTabConfig {
  folderPath?: string
  folderName?: string
}

export interface UpdateInfo {
  currentVersion: string
  latestVersion: string
  hasUpdate: boolean
  releaseNotes?: string
  releaseUrl?: string
  publishedAt?: string
  assetUrl?: string
  signatureUrl?: string
}

export interface UpdateSettings {
  checkOnStartup: boolean
  channel: string
}

