package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pebblepost/internal/types"
)

const (
	PebbleDir       = ".pebble"
	WorkspaceFile   = "workspace.json"
	EnvironmentsDir = "environments"
	CollectionsDir  = "collections"
	PebbleExt       = ".pebble.json"
	FolderFile      = "_folder.pebble.json"
)

// WorkspaceService handles reading, writing, and scanning API collections on the local filesystem.
type WorkspaceService struct {
	watcher *Watcher
}

// NewWorkspaceService creates a new WorkspaceService instance.
func NewWorkspaceService() *WorkspaceService {
	return &WorkspaceService{}
}

// SetWatcher connects an active file watcher to the service for self-write suppression.
func (s *WorkspaceService) SetWatcher(w *Watcher) {
	s.watcher = w
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

	// 3. Create .pebble/workspace.json with SchemaVersion = 1
	wsDef := &types.WorkspaceDefinition{
		SchemaVersion:     CurrentSchemaVersion,
		Version:           "1.0",
		Name:              name,
		ActiveEnvironment: "dev",
		Trusted:           true,
	}
	wsJSONPath := filepath.Join(pebblePath, WorkspaceFile)
	if _, err := os.Stat(wsJSONPath); os.IsNotExist(err) {
		_ = WriteFileStable(wsJSONPath, wsDef)
	}

	// 4. Create default dev.env.json if not exists
	devEnvPath := filepath.Join(envPath, "dev.env.json")
	if _, err := os.Stat(devEnvPath); os.IsNotExist(err) {
		devEnv := types.EnvironmentDefinition{
			SchemaVersion: CurrentSchemaVersion,
			Name:          "dev",
			Variables: []types.KeyValue{
				{Key: "BASE_URL", Value: "https://httpbin.org", Enabled: true},
			},
		}
		_ = WriteFileStable(devEnvPath, devEnv)
	}

	// 5. Create default example request in collections/example/get-started.pebble.json if empty
	exampleDir := filepath.Join(collectionsPath, "example")
	_ = os.MkdirAll(exampleDir, 0755)
	exampleReqPath := filepath.Join(exampleDir, "get-started.pebble.json")
	if _, err := os.Stat(exampleReqPath); os.IsNotExist(err) {
		exampleReq := types.RequestDefinition{
			SchemaVersion: CurrentSchemaVersion,
			Schema:        DefaultRequestSchema,
			Version:       "1.0",
			Name:          "Get Started (HTTPBin)",
			Description:   "Sample request demonstrating PebblePost offline collection capabilities",
			Method:        "GET",
			URL:           "{{BASE_URL}}/get",
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
		_ = WriteFileStable(exampleReqPath, exampleReq)
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
				SchemaVersion:     CurrentSchemaVersion,
				Version:           "1.0",
				Name:              filepath.Base(rootPath),
				ActiveEnvironment: "dev",
				Trusted:           true,
			}, nil
		}
		return nil, fmt.Errorf("failed to read workspace.json: %w", err)
	}

	var def types.WorkspaceDefinition
	if err := json.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("failed to parse workspace.json: %w", err)
	}

	migrated, _ := MigrateWorkspace(&def)
	return migrated, nil
}

// ScanTree recursively scans the collections directory (or root if collections doesn't exist)
// and builds a hierarchical tree of folders and *.pebble.json requests with explicit ordering.
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

	// Check if _folder.pebble.json exists in this directory for explicit ordering
	var folderItemOrder []string
	folderFilePath := filepath.Join(dirPath, FolderFile)
	if data, err := os.ReadFile(folderFilePath); err == nil {
		var folderDef types.FolderDefinition
		if json.Unmarshal(data, &folderDef) == nil && len(folderDef.ItemOrder) > 0 {
			folderItemOrder = folderDef.ItemOrder
		}
	}

	var nodes []*types.TreeNode

	for _, entry := range entries {
		name := entry.Name()

		// Skip hidden files & directories (.git, .pebble, node_modules, etc.)
		// Also skip _folder.pebble.json (internal folder metadata)
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "dist" || name == FolderFile {
			continue
		}

		fullPath := filepath.Join(dirPath, name)
		relPath, _ := filepath.Rel(workspaceRoot, fullPath)

		order, _, displayName := ParseOrderPrefix(name)

		if entry.IsDir() {
			children, err := s.scanDir(fullPath, workspaceRoot)
			if err != nil {
				return nil, err
			}

			nodes = append(nodes, &types.TreeNode{
				ID:          relPath,
				Name:        name,
				DisplayName: displayName,
				Path:        fullPath,
				RelPath:     relPath,
				IsDir:       true,
				Order:       order,
				Children:    children,
			})
		} else if strings.HasSuffix(name, PebbleExt) {
			method := "GET"
			itemOrder := order

			if req, err := s.ReadRequest(fullPath); err == nil {
				if req.Method != "" {
					method = req.Method
				}
				if req.Order > 0 {
					itemOrder = req.Order
				}
			}

			nodes = append(nodes, &types.TreeNode{
				ID:          relPath,
				Name:        name,
				DisplayName: displayName,
				Path:        fullPath,
				RelPath:     relPath,
				IsDir:       false,
				Order:       itemOrder,
				Method:      method,
			})
		}
	}

	// Sort nodes using Option C: Directories first, then explicit itemOrder > Order > numeric prefix > alphabetical
	comparator := NewNodeComparator(folderItemOrder)
	sort.Slice(nodes, func(i, j int) bool {
		return comparator.Compare(nodes[i], nodes[j])
	})

	return nodes, nil
}

// ReadRequest reads and parses a *.pebble.json file, migrating legacy v0 files
// to schemaVersion 1 in memory. It NEVER modifies the file on disk.
func (s *WorkspaceService) ReadRequest(filePath string) (*types.RequestDefinition, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read request file %s: %w", filePath, err)
	}

	var req types.RequestDefinition
	if err := json.Unmarshal(data, &req); err != nil {
		return nil, fmt.Errorf("failed to parse JSON from %s: %w", filePath, err)
	}

	migratedReq, _ := MigrateRequest(&req)
	return migratedReq, nil
}

// SaveRequest writes a RequestDefinition to a *.pebble.json file with 2-space indentation
// and trailing newline, updating its schemaVersion to CurrentSchemaVersion (1).
func (s *WorkspaceService) SaveRequest(filePath string, req *types.RequestDefinition) error {
	if filePath == "" {
		return fmt.Errorf("file path cannot be empty")
	}

	// Ensure filename ends with .pebble.json
	if !strings.HasSuffix(filePath, PebbleExt) {
		filePath += PebbleExt
	}

	// Suppress watcher notifications for internal write
	if s.watcher != nil {
		s.watcher.Suppress(filePath, 1000*time.Millisecond)
	}

	// Ensure schema version is updated to v1
	req.SchemaVersion = CurrentSchemaVersion
	if req.Schema == "" {
		req.Schema = DefaultRequestSchema
	}
	if req.Version == "" {
		req.Version = "1.0"
	}

	return WriteFileStable(filePath, req)
}

// DeletePath removes a file or directory at targetPath.
func (s *WorkspaceService) DeletePath(targetPath string) error {
	if targetPath == "" {
		return fmt.Errorf("target path cannot be empty")
	}
	if s.watcher != nil {
		s.watcher.Suppress(targetPath, 1000*time.Millisecond)
	}
	return os.RemoveAll(targetPath)
}

// CreateFolder creates a directory path recursively.
func (s *WorkspaceService) CreateFolder(folderPath string) error {
	if folderPath == "" {
		return fmt.Errorf("folder path cannot be empty")
	}
	if s.watcher != nil {
		s.watcher.Suppress(folderPath, 1000*time.Millisecond)
	}
	return os.MkdirAll(folderPath, 0755)
}

// Rename renames or moves a file/folder.
func (s *WorkspaceService) Rename(oldPath, newPath string) error {
	if oldPath == "" || newPath == "" {
		return fmt.Errorf("paths cannot be empty")
	}
	if s.watcher != nil {
		s.watcher.Suppress(oldPath, 1000*time.Millisecond)
		s.watcher.Suppress(newPath, 1000*time.Millisecond)
	}
	return os.Rename(oldPath, newPath)
}

// MovePath moves a request file or folder into targetFolder.
func (s *WorkspaceService) MovePath(sourcePath, targetFolder string) (string, error) {
	if sourcePath == "" || targetFolder == "" {
		return "", fmt.Errorf("source path and target folder cannot be empty")
	}

	baseName := filepath.Base(sourcePath)
	newPath := filepath.Join(targetFolder, baseName)

	if filepath.Clean(sourcePath) == filepath.Clean(newPath) {
		return sourcePath, nil
	}

	if err := os.MkdirAll(targetFolder, 0755); err != nil {
		return "", fmt.Errorf("failed to create destination folder %s: %w", targetFolder, err)
	}

	if s.watcher != nil {
		s.watcher.Suppress(sourcePath, 1000*time.Millisecond)
		s.watcher.Suppress(newPath, 1000*time.Millisecond)
	}

	if err := os.Rename(sourcePath, newPath); err != nil {
		return "", fmt.Errorf("failed to move from %s to %s: %w", sourcePath, newPath, err)
	}

	return newPath, nil
}

// DuplicateRequest copies an existing request file to a new file in the same directory,
// generating a non-colliding name (e.g., "get-user (Copy).pebble.json")
// and writing it atomically using WriteFileStable.
func (s *WorkspaceService) DuplicateRequest(sourcePath string) (string, error) {
	if sourcePath == "" {
		return "", fmt.Errorf("source path cannot be empty")
	}

	req, err := s.ReadRequest(sourcePath)
	if err != nil {
		return "", fmt.Errorf("failed to read source request: %w", err)
	}

	dir := filepath.Dir(sourcePath)
	base := filepath.Base(sourcePath)

	cleanBase := base
	if strings.HasSuffix(cleanBase, PebbleExt) {
		cleanBase = strings.TrimSuffix(cleanBase, PebbleExt)
	} else if strings.HasSuffix(cleanBase, ".json") {
		cleanBase = strings.TrimSuffix(cleanBase, ".json")
	}

	targetName := cleanBase + " (Copy)"
	targetPath := filepath.Join(dir, targetName+PebbleExt)
	copyIndex := 2
	for {
		if _, err := os.Stat(targetPath); os.IsNotExist(err) {
			break
		}
		targetName = fmt.Sprintf("%s (Copy %d)", cleanBase, copyIndex)
		targetPath = filepath.Join(dir, targetName+PebbleExt)
		copyIndex++
	}

	if copyIndex > 2 {
		req.Name = fmt.Sprintf("%s (Copy %d)", req.Name, copyIndex-1)
	} else {
		req.Name = req.Name + " (Copy)"
	}
	req.ID = ""

	if err := s.SaveRequest(targetPath, req); err != nil {
		return "", fmt.Errorf("failed to write duplicate request: %w", err)
	}

	return targetPath, nil
}

// TrashPath moves a file or folder to .pebble/trash/ within the workspace if possible,
// or permanently deletes it as a fallback.
func (s *WorkspaceService) TrashPath(workspaceRoot, targetPath string) error {
	if targetPath == "" {
		return fmt.Errorf("target path cannot be empty")
	}

	if s.watcher != nil {
		s.watcher.Suppress(targetPath, 1000*time.Millisecond)
	}

	if workspaceRoot != "" {
		trashDir := filepath.Join(workspaceRoot, PebbleDir, "trash")
		if err := os.MkdirAll(trashDir, 0755); err == nil {
			timestamp := time.Now().Format("20060102-150405")
			baseName := filepath.Base(targetPath)
			dest := filepath.Join(trashDir, fmt.Sprintf("%s-%s", timestamp, baseName))
			if err := os.Rename(targetPath, dest); err == nil {
				return nil
			}
		}
	}

	return os.RemoveAll(targetPath)
}
