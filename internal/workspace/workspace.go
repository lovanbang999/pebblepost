package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pebblepost/internal/types"
)

const (
	PebbleDir       = ".pebble"
	WorkspaceFile   = "workspace.json"
	EnvironmentsDir = "environments"
	CollectionsDir  = "collections"
	PebbleExt       = ".pebble.json"
)

// WorkspaceService handles reading, writing, and scanning API collections on the local filesystem.
type WorkspaceService struct{}

// NewWorkspaceService creates a new WorkspaceService instance.
func NewWorkspaceService() *WorkspaceService {
	return &WorkspaceService{}
}

// Init initializes a new PebblePost workspace directory structure.
func (s *WorkspaceService) Init(rootPath string, name string) (*types.WorkspaceDefinition, error) {
	if rootPath == "" {
		return nil, fmt.Errorf("workspace root path cannot be empty")
	}

	if name == "" {
		name = filepath.Base(rootPath)
		if name == "." || name == "/" || name == "" {
			name = "My Workspace"
		}
	}

	pebblePath := filepath.Join(rootPath, PebbleDir)
	envPath := filepath.Join(pebblePath, EnvironmentsDir)
	collectionsPath := filepath.Join(rootPath, CollectionsDir)

	// 1. Create directories
	if err := os.MkdirAll(envPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create environments directory: %w", err)
	}
	if err := os.MkdirAll(collectionsPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create collections directory: %w", err)
	}

	// 2. Create .pebble/.gitignore to protect secret environment files
	gitignorePath := filepath.Join(pebblePath, ".gitignore")
	gitignoreContent := "# Ignore secret environment files containing credentials/tokens\n*.secret.env.json\n"
	if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
		_ = os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644)
	}

	// 3. Create .pebble/workspace.json
	wsDef := &types.WorkspaceDefinition{
		Version:           "1.0",
		Name:              name,
		ActiveEnvironment: "dev",
	}
	wsJSONPath := filepath.Join(pebblePath, WorkspaceFile)
	if _, err := os.Stat(wsJSONPath); os.IsNotExist(err) {
		data, err := json.MarshalIndent(wsDef, "", "  ")
		if err == nil {
			_ = os.WriteFile(wsJSONPath, data, 0644)
		}
	}

	// 4. Create default dev.env.json if not exists
	devEnvPath := filepath.Join(envPath, "dev.env.json")
	if _, err := os.Stat(devEnvPath); os.IsNotExist(err) {
		devEnv := types.EnvironmentDefinition{
			Name: "dev",
			Variables: []types.KeyValue{
				{Key: "BASE_URL", Value: "https://httpbin.org", Enabled: true},
			},
		}
		data, err := json.MarshalIndent(devEnv, "", "  ")
		if err == nil {
			_ = os.WriteFile(devEnvPath, data, 0644)
		}
	}

	// 5. Create default example request in collections/example/get-started.pebble.json if empty
	exampleDir := filepath.Join(collectionsPath, "example")
	_ = os.MkdirAll(exampleDir, 0755)
	exampleReqPath := filepath.Join(exampleDir, "get-started.pebble.json")
	if _, err := os.Stat(exampleReqPath); os.IsNotExist(err) {
		exampleReq := types.RequestDefinition{
			Schema:      "https://pebblepost.dev/schemas/v1/request.json",
			Version:     "1.0",
			Name:        "Get Started (HTTPBin)",
			Description: "Sample request demonstrating PebblePost offline collection capabilities",
			Method:      "GET",
			URL:         "{{BASE_URL}}/get",
			Headers: []types.KeyValue{
				{Key: "Accept", Value: "application/json", Enabled: true},
				{Key: "User-Agent", Value: "PebblePost/0.1.0", Enabled: true},
			},
			Params: []types.KeyValue{
				{Key: "source", Value: "pebblepost", Enabled: true},
			},
			Auth: types.AuthDefinition{
				Type: "none",
			},
			Body: types.BodyDefinition{
				Type: "none",
			},
			Scripts: types.ScriptDefinition{
				PreRequest:   "// Pre-request script example\npb.request.headers.set('X-Timestamp', Date.now().toString());",
				PostResponse: "// Post-response test assertions\npb.test('Status code is 200', () => {\n  pb.expect(pb.response.status).to.eql(200);\n});",
			},
			Settings: types.SettingDefinition{
				FollowRedirects: true,
				VerifySSL:       true,
				TimeoutMs:       30000,
			},
		}
		data, err := json.MarshalIndent(exampleReq, "", "  ")
		if err == nil {
			_ = os.WriteFile(exampleReqPath, data, 0644)
		}
	}

	return wsDef, nil
}

// GetWorkspaceInfo reads workspace configuration from .pebble/workspace.json.
func (s *WorkspaceService) GetWorkspaceInfo(rootPath string) (*types.WorkspaceDefinition, error) {
	wsJSONPath := filepath.Join(rootPath, PebbleDir, WorkspaceFile)
	data, err := os.ReadFile(wsJSONPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Return default fallback
			return &types.WorkspaceDefinition{
				Version:           "1.0",
				Name:              filepath.Base(rootPath),
				ActiveEnvironment: "dev",
			}, nil
		}
		return nil, fmt.Errorf("failed to read workspace.json: %w", err)
	}

	var def types.WorkspaceDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse workspace.json: %w", err)
	}

	return &def, nil
}

// ScanTree recursively scans the collections directory (or root if collections doesn't exist)
// and builds a hierarchical tree of folders and *.pebble.json requests.
func (s *WorkspaceService) ScanTree(rootPath string) ([]*types.TreeNode, error) {
	if rootPath == "" {
		return []*types.TreeNode{}, nil
	}

	// Check if a dedicated 'collections' directory exists; if so, scan from it
	scanRoot := rootPath
	collDir := filepath.Join(rootPath, CollectionsDir)
	if fi, err := os.Stat(collDir); err == nil && fi.IsDir() {
		scanRoot = collDir
	}

	return s.scanDir(scanRoot, rootPath)
}

func (s *WorkspaceService) scanDir(dirPath string, workspaceRoot string) ([]*types.TreeNode, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*types.TreeNode{}, nil
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	var nodes []*types.TreeNode

	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files & directories (.git, .pebble, node_modules, etc.)
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" {
			continue
		}

		fullPath := filepath.Join(dirPath, name)
		relPath, _ := filepath.Rel(workspaceRoot, fullPath)

		if entry.IsDir() {
			children, err := s.scanDir(fullPath, workspaceRoot)
			if err != nil {
				return nil, err
			}

			nodes = append(nodes, &types.TreeNode{
				ID:       relPath,
				Name:     name,
				Path:     fullPath,
				RelPath:  relPath,
				IsDir:    true,
				Children: children,
			})
		} else if strings.HasSuffix(name, PebbleExt) {
			// Parse quick method from the *.pebble.json file
			method := "GET"
			if req, err := s.ReadRequest(fullPath); err == nil && req.Method != "" {
				method = req.Method
			}

			nodes = append(nodes, &types.TreeNode{
				ID:      relPath,
				Name:    name,
				Path:    fullPath,
				RelPath: relPath,
				IsDir:   false,
				Method:  method,
			})
		}
	}

	// Sort nodes: directories first, then files alphabetically
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})

	return nodes, nil
}

// ReadRequest reads and parses a *.pebble.json file.
func (s *WorkspaceService) ReadRequest(filePath string) (*types.RequestDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read request file %s: %w", filePath, err)
	}

	var req types.RequestDefinition
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", filePath, err)
	}

	return &req, nil
}

// SaveRequest writes a RequestDefinition to a *.pebble.json file with 2-space indentation.
func (s *WorkspaceService) SaveRequest(filePath string, req *types.RequestDefinition) error {
	if filePath == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// Ensure filename ends with .pebble.json
	if !strings.HasSuffix(filePath, PebbleExt) {
		filePath += PebbleExt
	}

	// Ensure parent directory exists
	parentDir := filepath.Dir(filePath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directories for %s: %w", filePath, err)
	}

	// Default schema and version if missing
	if req.Schema == "" {
		req.Schema = "https://pebblepost.dev/schemas/v1/request.json"
	}
	if req.Version == "" {
		req.Version = "1.0"
	}

	data, err := json.MarshalIndent(req, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to encode request to JSON: %w", err)
	}

	// Atomic write via temp file
	tmpPath := filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temporary file: %w", err)
	}

	if err := os.Rename(tmpPath, filePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("failed to save request file: %w", err)
	}

	return nil
}

// DeletePath removes a file or directory at targetPath.
func (s *WorkspaceService) DeletePath(targetPath string) error {
	if targetPath == "" {
		return fmt.Errorf("target path cannot be empty")
	}
	return os.RemoveAll(targetPath)
}

// CreateFolder creates a directory path recursively.
func (s *WorkspaceService) CreateFolder(folderPath string) error {
	if folderPath == "" {
		return fmt.Errorf("folder path cannot be empty")
	}
	return os.MkdirAll(folderPath, 0755)
}

// Rename renames or moves a file/folder.
func (s *WorkspaceService) Rename(oldPath, newPath string) error {
	if oldPath == "" || newPath == "" {
		return fmt.Errorf("paths cannot be empty")
	}
	return os.Rename(oldPath, newPath)
}
