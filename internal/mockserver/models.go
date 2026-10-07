package mockserver

import (
	"time"

	"pebblepost/internal/types"
)

// PathSegment represents a parsed segment of a route path.
type PathSegment struct {
	Raw     string
	IsParam bool
	IsWild  bool
	Param   string // name of the parameter, e.g. "id"
}

// RouteOverride defines runtime overrides for a mock route.
type RouteOverride struct {
	StatusCode int           `json:"statusCode,omitempty"` // 0 = use example statusCode
	Delay      time.Duration `json:"delay,omitempty"`      // 0 = no artificial delay
	DelayMs    int64         `json:"delayMs,omitempty"`    // millisecond helper for JSON
	ErrorRate  float64       `json:"errorRate,omitempty"`  // 0.0 to 1.0 (probability of 500 injection)
}

// MockRoute represents an active endpoint served by the mock server.
type MockRoute struct {
	ID          string                  `json:"id"`
	RequestName string                  `json:"requestName"`
	FilePath    string                  `json:"filePath"`
	Method      string                  `json:"method"`
	RawURL      string                  `json:"rawUrl"`
	PathPattern string                  `json:"pathPattern"` // normalized path, e.g. "/users/:id"
	Segments    []PathSegment           `json:"-"`
	Specificity int                     `json:"-"`
	Examples    []types.ExampleResponse `json:"examples"`
	Override    *RouteOverride          `json:"override,omitempty"`
}

// MockRouteSummary is a lightweight representation for API status.
type MockRouteSummary struct {
	ID           string         `json:"id"`
	RequestName  string         `json:"requestName"`
	FilePath     string         `json:"filePath"`
	Method       string         `json:"method"`
	PathPattern  string         `json:"pathPattern"`
	ExampleCount int            `json:"exampleCount"`
	ExampleNames []string       `json:"exampleNames"`
	Override     *RouteOverride `json:"override,omitempty"`
}

// RequestLog represents a logged mock server transaction.
type RequestLog struct {
	ID             string            `json:"id"`
	Timestamp      time.Time         `json:"timestamp"`
	Method         string            `json:"method"`
	Path           string            `json:"path"`
	Query          string            `json:"query,omitempty"`
	Headers        map[string]string `json:"headers,omitempty"`
	StatusCode     int               `json:"statusCode"`
	DurationMs     int64             `json:"durationMs"`
	Matched        bool              `json:"matched"`
	MatchedRoute   string            `json:"matchedRoute,omitempty"`
	MatchedExample string            `json:"matchedExample,omitempty"`
	ErrorMessage   string            `json:"errorMessage,omitempty"`
}

// ServerConfig holds the configuration to start a mock server.
type ServerConfig struct {
	Host             string                    `json:"host"`
	Port             int                       `json:"port"`
	WorkspacePath    string                    `json:"workspacePath"`
	TargetPath       string                    `json:"targetPath"`
	GlobalDelayMs    int64                     `json:"globalDelayMs,omitempty"`
	GlobalStatusCode int                       `json:"globalStatusCode,omitempty"`
	GlobalErrorRate  float64                   `json:"globalErrorRate,omitempty"`
	Overrides        map[string]*RouteOverride `json:"overrides,omitempty"` // key: route ID or method:path
}

// ServerStatus describes the runtime status of the mock server.
type ServerStatus struct {
	Running     bool                      `json:"running"`
	Host        string                    `json:"host"`
	Port        int                       `json:"port"`
	URL         string                    `json:"url"`
	Target      string                    `json:"target"`
	RoutesCount int                       `json:"routesCount"`
	Routes      []MockRouteSummary        `json:"routes"`
	Overrides   map[string]*RouteOverride `json:"overrides,omitempty"`
	NonLoopback bool                      `json:"nonLoopback"`
	Warning     string                    `json:"warning,omitempty"`
	Error       string                    `json:"error,omitempty"`
}

// UnmatchedResponse represents the 404 diagnostic JSON payload.
type UnmatchedResponse struct {
	Error           string                  `json:"error"`
	Method          string                  `json:"method"`
	Path            string                  `json:"path"`
	Message         string                  `json:"message"`
	AvailableRoutes []AvailableRouteSummary `json:"availableRoutes"`
}

// AvailableRouteSummary provides candidate route info in 404 responses.
type AvailableRouteSummary struct {
	Method   string   `json:"method"`
	Path     string   `json:"path"`
	Examples []string `json:"examples"`
}
