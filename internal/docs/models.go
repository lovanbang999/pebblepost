package docs

import (
	"pebblepost/internal/types"
)

// CodeSnippets contains code samples across multiple programming languages.
type CodeSnippets struct {
	CURL   string `json:"curl"`
	Go     string `json:"go"`
	Node   string `json:"node"`
	Python string `json:"python"`
	CSharp string `json:"csharp"`
}

// DocRequest represents a documented HTTP/gRPC request endpoint.
type DocRequest struct {
	ID          string                   `json:"id"`
	Name        string                   `json:"name"`
	Description string                   `json:"description,omitempty"`
	Method      string                   `json:"method"`
	URL         string                   `json:"url"`
	Protocol    string                   `json:"protocol,omitempty"`
	RelPath     string                   `json:"relPath"`
	Headers     []types.KeyValue         `json:"headers,omitempty"`
	Params      []types.KeyValue         `json:"params,omitempty"`
	Auth        types.AuthDefinition     `json:"auth"`
	Body        types.BodyDefinition     `json:"body"`
	Examples    []types.ExampleResponse  `json:"examples,omitempty"`
	Snippets    CodeSnippets             `json:"snippets"`
	RawRequest  *types.RequestDefinition `json:"-"`
}

// DocFolder represents a folder group containing requests and subfolders.
type DocFolder struct {
	Name        string     `json:"name"`
	Description string     `json:"description,omitempty"`
	RelPath     string     `json:"relPath"`
	Order       int        `json:"order"`
	Items       []*DocItem `json:"items"`
}

// DocItem wraps either a Request or a Folder in the documentation hierarchy.
type DocItem struct {
	IsFolder bool        `json:"isFolder"`
	Folder   *DocFolder  `json:"folder,omitempty"`
	Request  *DocRequest `json:"request,omitempty"`
}

// DocCollection represents the complete collection documentation.
type DocCollection struct {
	Title       string     `json:"title"`
	Description string     `json:"description,omitempty"`
	Version     string     `json:"version,omitempty"`
	Items       []*DocItem `json:"items"`
}
