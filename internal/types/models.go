package types

import "time"

// KeyValue represents a generic key-value pair with enabled state.
type KeyValue struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	Type    string `json:"type,omitempty"` // "text" or "file"
}

// AuthDefinition represents authentication configuration for a request.
type AuthDefinition struct {
	Type     string `json:"type"` // "none", "bearer", "basic", "apiKey", "oauth2"
	Token    string `json:"token,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Key      string `json:"key,omitempty"`
	Value    string `json:"value,omitempty"`
	AddTo    string `json:"addTo,omitempty"` // "header" or "query"
}

// BodyDefinition represents the request body payload.
type BodyDefinition struct {
	Type       string     `json:"type"` // "none", "json", "raw", "formData", "urlEncoded", "graphql"
	Raw        string     `json:"raw,omitempty"`
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
	FollowRedirects bool `json:"followRedirects"`
	VerifySSL       bool `json:"verifySSL"`
	TimeoutMs       int  `json:"timeoutMs"`
}

// RequestDefinition is the schema for a *.pebble.json file.
type RequestDefinition struct {
	Schema      string            `json:"$schema,omitempty"`
	Version     string            `json:"version,omitempty"`
	ID          string            `json:"id,omitempty"`
	Name        string            `json:"name"`
	Description string            `json:"description,omitempty"`
	Method      string            `json:"method"`
	URL         string            `json:"url"`
	Headers     []KeyValue        `json:"headers,omitempty"`
	Params      []KeyValue        `json:"params,omitempty"`
	Auth        AuthDefinition    `json:"auth"`
	Body        BodyDefinition    `json:"body"`
	Scripts     ScriptDefinition  `json:"scripts"`
	Settings    SettingDefinition `json:"settings"`
}

// EnvironmentDefinition represents an environment file (*.env.json or *.secret.env.json).
type EnvironmentDefinition struct {
	Name      string     `json:"name"`
	Variables []KeyValue `json:"variables"`
}

// WorkspaceDefinition represents workspace-level metadata (.pebble/workspace.json).
type WorkspaceDefinition struct {
	Version           string `json:"version"`
	Name              string `json:"name"`
	ActiveEnvironment string `json:"activeEnvironment,omitempty"`
}

// TimingMetrics records detailed network roundtrip breakdown in milliseconds.
type TimingMetrics struct {
	DNSLookupMs    float64 `json:"dnsLookupMs"`
	TCPConnectMs   float64 `json:"tcpConnectMs"`
	TLSHandshakeMs float64 `json:"tlsHandshakeMs"`
	TTFBMs         float64 `json:"ttfbMs"` // Time to First Byte
	DownloadMs     float64 `json:"downloadMs"`
	TotalDurationMs float64 `json:"totalDurationMs"`
}

// TestAssertionResult holds the pass/fail output of a single test assertion.
type TestAssertionResult struct {
	Name    string `json:"name"`
	Passed  bool   `json:"passed"`
	Message string `json:"message,omitempty"`
}

// ExecutionResult is the response returned after executing an API request.
type ExecutionResult struct {
	StatusCode       int                   `json:"statusCode"`
	StatusText       string                `json:"statusText"`
	Headers          map[string][]string   `json:"headers"`
	Body             string                `json:"body"`
	Size             int64                 `json:"size"`
	Timing           TimingMetrics         `json:"timing"`
	Tests            []TestAssertionResult `json:"tests"`
	Logs             []string              `json:"logs"`
	ExtractedEnvVars map[string]string     `json:"extractedEnvVars,omitempty"`
	ExecutedAt       time.Time             `json:"executedAt"`
	Error            string                `json:"error,omitempty"`
}

// TreeNode represents a file or folder in the collection directory explorer.
type TreeNode struct {
	ID       string      `json:"id"`
	Name     string      `json:"name"`
	Path     string      `json:"path"`
	RelPath  string      `json:"relPath"`
	IsDir    bool        `json:"isDir"`
	Method   string      `json:"method,omitempty"`
	Children []*TreeNode `json:"children,omitempty"`
}
