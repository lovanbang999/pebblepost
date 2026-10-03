package types

import "time"

// KeyValue represents a generic key-value pair with enabled state.
type KeyValue struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	Type    string `json:"type,omitempty"`   // "text" or "file"
	Secret  bool   `json:"secret,omitempty"` // value must be masked in logs/output
}

// AuthDefinition represents authentication configuration for a request or folder.
type AuthDefinition struct {
	Type     string `json:"type"` // "inherit", "none", "bearer", "basic", "apiKey", "digest", "oauth2", "awsSigV4"
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Key      string `json:"key,omitempty"`
	Value    string `json:"value,omitempty"`
	AddTo    string `json:"addTo,omitempty"` // "header" or "query"

	// Digest Auth fields
	Realm string `json:"realm,omitempty"`

	// OAuth2 fields
	GrantType      string `json:"grantType,omitempty"` // "authorization_code" or "client_credentials"
	AuthURL        string `json:"authUrl,omitempty"`
	TokenURL       string `json:"tokenUrl,omitempty"`
	ClientID       string `json:"clientId,omitempty"`
	ClientSecret   string `json:"clientSecret,omitempty"`
	Scope          string `json:"scope,omitempty"`
	RedirectURL    string `json:"redirectUrl,omitempty"`
	CodeVerifier   string `json:"codeVerifier,omitempty"`
	RefreshToken   string `json:"refreshToken,omitempty"`
	TokenExpiresAt int64  `json:"tokenExpiresAt,omitempty"`

	// AWS Signature v4 fields
	AccessKey    string `json:"accessKey,omitempty"`
	SecretKey    string `json:"secretKey,omitempty"`
	Region       string `json:"region,omitempty"`
	Service      string `json:"service,omitempty"`
	SessionToken string `json:"sessionToken,omitempty"`
}

// BodyDefinition represents the request body payload.
type BodyDefinition struct {
	Type       string     `json:"type"` // "none", "json", "raw", "formData", "urlEncoded", "graphql", "file"
	Raw        string     `json:"raw,omitempty"`
	FilePath   string     `json:"filePath,omitempty"` // Relative path reference to large payload file
	FormData   []KeyValue `json:"formData,omitempty"`
	UrlEncoded []KeyValue `json:"urlEncoded,omitempty"`
	GraphQL    *GraphQL   `json:"graphql,omitempty"`
}

// GraphQL represents a GraphQL query and variables.
type GraphQL struct {
	Query     string `json:"query"`
	Variables string `json:"variables,omitempty"`
}

// ScriptDefinition represents pre-request and post-response JavaScript scripts.
type ScriptDefinition struct {
	PreRequest   string `json:"preRequest,omitempty"`
	PostResponse string `json:"postResponse,omitempty"`
}

// SettingDefinition holds per-request execution settings.
type SettingDefinition struct {
	FollowRedirects  bool   `json:"followRedirects"`
	VerifySSL        bool   `json:"verifySSL"`
	TimeoutMs        int    `json:"timeoutMs"`
	ScriptTimeoutMs  int    `json:"scriptTimeoutMs,omitempty"`  // 0 = engine default (5 s)
	ConnectTimeoutMs int    `json:"connectTimeoutMs,omitempty"` // 0 = default (10 s)
	MaxRedirects     int    `json:"maxRedirects,omitempty"`     // 0 = default 10
	EnableCookies    *bool  `json:"enableCookies,omitempty"`    // nil or true = enabled, false = disabled
	UserAgent        string `json:"userAgent,omitempty"`        // custom User-Agent, default "PebblePost/1.0"
	ProxyURL         string `json:"proxyUrl,omitempty"`         // HTTP/HTTPS/SOCKS5 proxy URL
	ClientCertPath   string `json:"clientCertPath,omitempty"`   // mTLS client certificate path
	ClientKeyPath    string `json:"clientKeyPath,omitempty"`    // mTLS client key path
}

// CookieItem represents an HTTP cookie managed in the workspace cookie jar.
type CookieItem struct {
	Name     string    `json:"name"`
	Value    string    `json:"value"`
	Domain   string    `json:"domain"`
	Path     string    `json:"path"`
	Expires  time.Time `json:"expires,omitempty"`
	MaxAge   int       `json:"maxAge,omitempty"`
	Secure   bool      `json:"secure"`
	HTTPOnly bool      `json:"httpOnly"`
	SameSite string    `json:"sameSite,omitempty"`
}

// RequestDefinition is the schema for a *.pebble.json file.
type RequestDefinition struct {
	Schema        string            `json:"$schema,omitempty"`
	SchemaVersion int               `json:"schemaVersion"`
	Version       string            `json:"version,omitempty"`
	ID            string            `json:"id,omitempty"`
	Name          string            `json:"name"`
	Description   string            `json:"description,omitempty"`
	Order         int               `json:"order,omitempty"`
	Tags          []string          `json:"tags,omitempty"` // used for --tag filtering in CLI
	Method        string            `json:"method"`
	URL           string            `json:"url"`
	Headers       []KeyValue        `json:"headers,omitempty"`
	Params        []KeyValue        `json:"params,omitempty"`
	Auth          AuthDefinition    `json:"auth"`
	Body          BodyDefinition    `json:"body"`
	Scripts       ScriptDefinition  `json:"scripts"`
	Settings      SettingDefinition `json:"settings"`
}

// EnvironmentDefinition represents an environment file (*.env.json or *.secret.env.json).
type EnvironmentDefinition struct {
	SchemaVersion int        `json:"schemaVersion"`
	Name          string     `json:"name"`
	Variables     []KeyValue `json:"variables"`
}

// WorkspaceDefinition represents workspace-level metadata (.pebble/workspace.json).
type WorkspaceDefinition struct {
	SchemaVersion     int    `json:"schemaVersion"`
	Version           string `json:"version,omitempty"`
	Name              string `json:"name"`
	ActiveEnvironment string `json:"activeEnvironment,omitempty"`
	Trusted           bool   `json:"trusted,omitempty"`
}

// FolderDefinition represents folder-level metadata, configuration, and ordering (_folder.pebble.json).
type FolderDefinition struct {
	SchemaVersion int              `json:"schemaVersion"`
	Name          string           `json:"name,omitempty"`
	Description   string           `json:"description,omitempty"`
	Order         int              `json:"order,omitempty"`
	ItemOrder     []string         `json:"itemOrder,omitempty"` // Explicit sequence of child filenames or subfolder names
	Headers       []KeyValue       `json:"headers,omitempty"`
	Auth          AuthDefinition   `json:"auth,omitempty"`
	Variables     []KeyValue       `json:"variables,omitempty"`
	Scripts       ScriptDefinition `json:"scripts,omitempty"`
}

// InheritedItemInfo describes the origin folder of an inherited configuration item.
type InheritedItemInfo struct {
	SourceFolder string `json:"sourceFolder"` // e.g. "01-auth" or folder name
	SourcePath   string `json:"sourcePath"`   // directory path relative to workspace or absolute
}

// ResolvedRequestResult returns the fully merged effective request and provenance metadata for UI/execution.
type ResolvedRequestResult struct {
	Request           RequestDefinition            `json:"request"`
	InheritedHeaders  map[string]InheritedItemInfo `json:"inheritedHeaders"`            // lowercase header key -> provenance
	OverriddenHeaders map[string]InheritedItemInfo `json:"overriddenHeaders,omitempty"` // lowercase header key -> original folder provenance
	InheritedAuth     *InheritedItemInfo           `json:"inheritedAuth,omitempty"`
	ParentAuth        *AuthDefinition              `json:"parentAuth,omitempty"`
	ParentAuthSource  *InheritedItemInfo           `json:"parentAuthSource,omitempty"`
	InheritedVars     map[string]InheritedItemInfo `json:"inheritedVars"`               // var key -> provenance
	FolderPreScripts  []string                     `json:"folderPreScripts,omitempty"`  // folder names with pre-request scripts
	FolderPostScripts []string                     `json:"folderPostScripts,omitempty"` // folder names with post-response scripts
}

// RedirectHop captures a single step in an HTTP redirect chain.
type RedirectHop struct {
	StatusCode int               `json:"statusCode"`
	Method     string            `json:"method"`
	URL        string            `json:"url"`
	Headers    map[string]string `json:"headers,omitempty"` // relevant response headers (Location, etc.)
}

// SentRequestSummary captures the final, fully-resolved HTTP request that was sent over the wire.
type SentRequestSummary struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body,omitempty"`
}

// TimingMetrics records detailed network roundtrip breakdown in milliseconds.
type TimingMetrics struct {
	DNSLookupMs     float64 `json:"dnsLookupMs"`
	TCPConnectMs    float64 `json:"tcpConnectMs"`
	TLSHandshakeMs  float64 `json:"tlsHandshakeMs"`
	TTFBMs          float64 `json:"ttfbMs"` // Time to First Byte
	DownloadMs      float64 `json:"downloadMs"`
	TotalDurationMs float64 `json:"totalDurationMs"`
}

// TestAssertionResult holds the pass/fail output of a single test assertion.
type TestAssertionResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// ConsoleLogEntry represents a structured log line generated by console.log/info/warn/error during script execution.
type ConsoleLogEntry struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`  // "log", "info", "warn", "error"
	Source    string    `json:"source"` // e.g. "Pre-request", "Post-response", folder name
	Message   string    `json:"message"`
	Line      int       `json:"line,omitempty"`
	Column    int       `json:"column,omitempty"`
}

// ExecutionResult is the response returned after executing an API request.
type ExecutionResult struct {
	StatusCode       int                   `json:"statusCode"`
	StatusText       string                `json:"statusText"`
	Headers          map[string][]string   `json:"headers"`
	Body             string                `json:"body"`
	BodyTruncated    bool                  `json:"bodyTruncated,omitempty"` // true when body exceeded the display threshold
	BodySizeBytes    int64                 `json:"bodySizeBytes,omitempty"` // actual byte length of the full body
	TempBodyFile     string                `json:"tempBodyFile,omitempty"`  // OS temp file path when body is large
	Size             int64                 `json:"size"`
	Timing           TimingMetrics         `json:"timing"`
	RedirectChain    []RedirectHop         `json:"redirectChain,omitempty"` // hops before the final response
	SentRequest      *SentRequestSummary   `json:"sentRequest,omitempty"`   // the actual request sent over the wire
	Tests            []TestAssertionResult `json:"tests"`
	Logs             []string              `json:"logs"`
	ConsoleLogs      []ConsoleLogEntry     `json:"consoleLogs,omitempty"`
	ExtractedEnvVars map[string]string     `json:"extractedEnvVars,omitempty"`
	ExecutedAt       time.Time             `json:"executedAt"`
	Error            string                `json:"error,omitempty"`
}

// TreeNode represents a file or folder in the collection directory explorer.
type TreeNode struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	DisplayName string      `json:"displayName,omitempty"`
	Path        string      `json:"path"`
	RelPath     string      `json:"relPath"`
	IsDir       bool        `json:"isDir"`
	Order       int         `json:"order,omitempty"`
	Method      string      `json:"method,omitempty"`
	Children    []*TreeNode `json:"children,omitempty"`
}
