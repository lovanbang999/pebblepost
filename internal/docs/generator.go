package docs

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// Generator builds documentation collections and produces formatted outputs.
type Generator struct {
	wsService *workspace.WorkspaceService
}

// NewGenerator creates a new documentation Generator.
func NewGenerator() *Generator {
	return &Generator{
		wsService: workspace.NewWorkspaceService(),
	}
}

// BuildDocCollection inspects targetPath and constructs a hierarchical DocCollection.
func (g *Generator) BuildDocCollection(workspacePath, targetPath string) (*DocCollection, error) {
	if targetPath == "" {
		targetPath = workspacePath
	}
	if targetPath == "" {
		targetPath = "."
	}

	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		return nil, fmt.Errorf("invalid target path: %w", err)
	}

	fi, err := os.Stat(absTarget)
	if err != nil {
		return nil, fmt.Errorf("cannot access path %q: %w", targetPath, err)
	}

	col := &DocCollection{
		Title:   filepath.Base(absTarget),
		Version: "1.0.0",
	}

	// If workspace root, try to get workspace title and info
	if workspacePath != "" {
		if wsInfo, err := g.wsService.GetWorkspaceInfo(workspacePath); err == nil && wsInfo != nil {
			if wsInfo.Name != "" {
				col.Title = wsInfo.Name
			}
			if wsInfo.Version != "" {
				col.Version = wsInfo.Version
			}
		}
	}

	if !fi.IsDir() {
		// Single request file
		if strings.HasSuffix(absTarget, workspace.PebbleExt) {
			req, err := g.wsService.ReadRequest(absTarget)
			if err != nil {
				return nil, fmt.Errorf("failed to read request file: %w", err)
			}
			col.Title = req.Name
			col.Description = req.Description
			docReq := g.makeDocRequest(filepath.Base(absTarget), req)
			col.Items = append(col.Items, &DocItem{
				IsFolder: false,
				Request:  docReq,
			})
			return col, nil
		}
		return nil, fmt.Errorf("path is not a directory or a .pebble.json file")
	}

	// Target is a directory. Check if target itself has _folder.pebble.json
	folderFile := filepath.Join(absTarget, workspace.FolderFile)
	if fDef, err := g.readFolderDef(folderFile); err == nil && fDef != nil {
		if fDef.Name != "" {
			col.Title = fDef.Name
		}
		col.Description = fDef.Description
	}

	// Build items recursively
	items, err := g.scanDocItems(absTarget, absTarget)
	if err != nil {
		return nil, err
	}
	col.Items = items

	return col, nil
}

func (g *Generator) scanDocItems(dirPath, rootPath string) ([]*DocItem, error) {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, err
	}

	// Read _folder.pebble.json if present
	var folderDef *types.FolderDefinition
	folderFilePath := filepath.Join(dirPath, workspace.FolderFile)
	if fDef, err := g.readFolderDef(folderFilePath); err == nil {
		folderDef = fDef
	}

	var folderMap = make(map[string]*DocItem)
	var requestMap = make(map[string]*DocItem)

	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || name == workspace.PebbleDir {
			continue
		}
		fullPath := filepath.Join(dirPath, name)
		relPath, _ := filepath.Rel(rootPath, fullPath)

		if entry.IsDir() {
			subItems, err := g.scanDocItems(fullPath, rootPath)
			if err != nil {
				continue
			}
			folderName := name
			folderDesc := ""
			folderOrder := 0

			// Read subfolder's _folder.pebble.json
			subFolderDefPath := filepath.Join(fullPath, workspace.FolderFile)
			if subFDef, err := g.readFolderDef(subFolderDefPath); err == nil && subFDef != nil {
				if subFDef.Name != "" {
					folderName = subFDef.Name
				}
				folderDesc = subFDef.Description
				folderOrder = subFDef.Order
			} else {
				order, _, cleanName := workspace.ParseOrderPrefix(name)
				folderName = cleanName
				folderOrder = order
			}

			folderMap[name] = &DocItem{
				IsFolder: true,
				Folder: &DocFolder{
					Name:        folderName,
					Description: folderDesc,
					RelPath:     relPath,
					Order:       folderOrder,
					Items:       subItems,
				},
			}
		} else if strings.HasSuffix(name, workspace.PebbleExt) && name != workspace.FolderFile {
			req, err := g.wsService.ReadRequest(fullPath)
			if err != nil {
				continue
			}
			docReq := g.makeDocRequest(relPath, req)
			requestMap[name] = &DocItem{
				IsFolder: false,
				Request:  docReq,
			}
		}
	}

	// Order items according to folderDef.ItemOrder or natural sorting
	var result []*DocItem
	used := make(map[string]bool)

	if folderDef != nil && len(folderDef.ItemOrder) > 0 {
		for _, itemName := range folderDef.ItemOrder {
			if it, ok := requestMap[itemName]; ok {
				result = append(result, it)
				used[itemName] = true
			} else if it, ok := folderMap[itemName]; ok {
				result = append(result, it)
				used[itemName] = true
			}
		}
	}

	// Append remaining folders
	var remainingFolders []string
	for k := range folderMap {
		if !used[k] {
			remainingFolders = append(remainingFolders, k)
		}
	}
	sort.Slice(remainingFolders, func(i, j int) bool {
		f1 := folderMap[remainingFolders[i]].Folder
		f2 := folderMap[remainingFolders[j]].Folder
		if f1.Order != f2.Order {
			return f1.Order < f2.Order
		}
		return f1.Name < f2.Name
	})
	for _, k := range remainingFolders {
		result = append(result, folderMap[k])
	}

	// Append remaining requests
	var remainingReqs []string
	for k := range requestMap {
		if !used[k] {
			remainingReqs = append(remainingReqs, k)
		}
	}
	sort.Slice(remainingReqs, func(i, j int) bool {
		r1 := requestMap[remainingReqs[i]].Request
		r2 := requestMap[remainingReqs[j]].Request
		if r1.RawRequest != nil && r2.RawRequest != nil && r1.RawRequest.Order != r2.RawRequest.Order {
			return r1.RawRequest.Order < r2.RawRequest.Order
		}
		return r1.Name < r2.Name
	})
	for _, k := range remainingReqs {
		result = append(result, requestMap[k])
	}

	return result, nil
}

func (g *Generator) readFolderDef(path string) (*types.FolderDefinition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var f types.FolderDefinition
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

func (g *Generator) makeDocRequest(relPath string, req *types.RequestDefinition) *DocRequest {
	return &DocRequest{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
		Method:      req.Method,
		URL:         req.URL,
		Protocol:    req.Protocol,
		RelPath:     relPath,
		Headers:     req.Headers,
		Params:      req.Params,
		Auth:        req.Auth,
		Body:        req.Body,
		Examples:    req.Examples,
		Snippets:    GenerateAllSnippets(req),
		RawRequest:  req,
	}
}

// Generate renders the target documentation into the requested format (md, html, openapi, json).
func (g *Generator) Generate(workspacePath, targetPath, format string) ([]byte, string, error) {
	col, err := g.BuildDocCollection(workspacePath, targetPath)
	if err != nil {
		return nil, "", err
	}

	fmtLower := strings.ToLower(strings.TrimSpace(format))
	switch fmtLower {
	case "md", "markdown", "":
		content := GenerateMarkdown(col)
		return []byte(content), "index.md", nil
	case "html", "htm":
		content := GenerateHTML(col)
		return []byte(content), "index.html", nil
	case "openapi", "yaml", "yml":
		bytes, err := GenerateOpenAPI(col, "yaml")
		if err != nil {
			return nil, "", err
		}
		return bytes, "openapi.yaml", nil
	case "json":
		bytes, err := GenerateOpenAPI(col, "json")
		if err != nil {
			return nil, "", err
		}
		return bytes, "openapi.json", nil
	default:
		return nil, "", fmt.Errorf("unsupported format %q (supported: md, html, openapi, json)", format)
	}
}

// WriteDocs renders and writes documentation to the target output directory or file.
func (g *Generator) WriteDocs(workspacePath, targetPath, format, outPath string) (string, error) {
	data, defaultFilename, err := g.Generate(workspacePath, targetPath, format)
	if err != nil {
		return "", err
	}

	if outPath == "" {
		outPath = filepath.Join("docs", defaultFilename)
	}

	// Check if outPath is an existing directory or ends with a slash
	fi, err := os.Stat(outPath)
	if err == nil && fi.IsDir() {
		outPath = filepath.Join(outPath, defaultFilename)
	} else if strings.HasSuffix(outPath, "/") || strings.HasSuffix(outPath, "\\") {
		outPath = filepath.Join(outPath, defaultFilename)
	}

	// Ensure parent directories exist
	parentDir := filepath.Dir(outPath)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return "", fmt.Errorf("failed to create output directory %q: %w", parentDir, err)
	}

	if err := os.WriteFile(outPath, data, 0644); err != nil {
		return "", fmt.Errorf("failed to write docs file %q: %w", outPath, err)
	}

	return outPath, nil
}
