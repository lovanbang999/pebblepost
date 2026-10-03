package workspace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"pebblepost/internal/types"
)

// FolderChainItem holds folder metadata discovered in the directory hierarchy.
type FolderChainItem struct {
	DirName   string
	Path      string // Absolute path to directory
	RelPath   string // Path relative to workspace root
	FolderDef *types.FolderDefinition
}

// ScriptChainItem represents a single script in the execution chain.
type ScriptChainItem struct {
	Source string // Folder display name, folder relPath, or "request"
	Path   string // Directory path or file path
	Script string // JavaScript code
}

// InheritanceResolver resolves configuration inheritance (headers, auth, variables, scripts)
// along the directory tree from workspace/collection root to the request.
type InheritanceResolver struct {
	workspaceSvc *WorkspaceService
}

// NewInheritanceResolver creates a new InheritanceResolver.
func NewInheritanceResolver(wsSvc *WorkspaceService) *InheritanceResolver {
	if wsSvc == nil {
		wsSvc = NewWorkspaceService()
	}
	return &InheritanceResolver{workspaceSvc: wsSvc}
}

// DiscoverFolderChain locates all ancestor directories between collection/workspace root
// and the folder containing requestFilePath, reading any _folder.pebble.json files in root-to-leaf order.
func (r *InheritanceResolver) DiscoverFolderChain(workspaceRoot, requestFilePath string) ([]FolderChainItem, error) {
	if workspaceRoot == "" {
		workspaceRoot = "."
	}
	absWorkspace, err := filepath.Abs(workspaceRoot)
	if err != nil {
		absWorkspace = workspaceRoot
	}

	absReqPath := requestFilePath
	if !filepath.IsAbs(absReqPath) {
		absReqPath = filepath.Join(absWorkspace, requestFilePath)
	}

	// Request's immediate parent directory
	reqDir := filepath.Dir(absReqPath)

	// Determine collection root (prefer collections/ if present)
	collRoot := filepath.Join(absWorkspace, CollectionsDir)
	if fi, err := os.Stat(collRoot); err != nil || !fi.IsDir() {
		collRoot = absWorkspace
	}

	// Compute path relative to collection root
	relToColl, err := filepath.Rel(collRoot, reqDir)
	var dirSegments []string
	if err == nil && !strings.HasPrefix(relToColl, "..") && relToColl != "." && relToColl != "" {
		dirSegments = strings.Split(filepath.ToSlash(relToColl), "/")
	}

	var chain []FolderChainItem

	// 1. Check collection root itself
	r.appendFolderIfPresent(absWorkspace, collRoot, &chain)

	// 2. Traverse child directories from top to bottom
	currPath := collRoot
	for _, segment := range dirSegments {
		if segment == "" || segment == "." {
			continue
		}
		currPath = filepath.Join(currPath, segment)
		r.appendFolderIfPresent(absWorkspace, currPath, &chain)
	}

	return chain, nil
}

func (r *InheritanceResolver) appendFolderIfPresent(absWorkspace, dirPath string, chain *[]FolderChainItem) {
	folderMetaPath := filepath.Join(dirPath, FolderFile)
	data, err := os.ReadFile(folderMetaPath)
	if err != nil {
		return
	}

	var folderDef types.FolderDefinition
	if err := json.Unmarshal(data, &folderDef); err != nil {
		return
	}

	migrated, _ := MigrateFolder(&folderDef)
	dirName := filepath.Base(dirPath)
	_, _, displayName := ParseOrderPrefix(dirName)
	if migrated.Name == "" {
		migrated.Name = displayName
	}

	relPath, err := filepath.Rel(absWorkspace, dirPath)
	if err != nil {
		relPath = dirPath
	}

	*chain = append(*chain, FolderChainItem{
		DirName:   migrated.Name,
		Path:      dirPath,
		RelPath:   relPath,
		FolderDef: migrated,
	})
}

// MergeHeaders merges headers along the chain (Root → Parent → Child → Request).
// Matching is case-insensitive. Request headers take final precedence.
// Provenance records which folder provided the inherited header.
func (r *InheritanceResolver) MergeHeaders(chain []FolderChainItem, reqHeaders []types.KeyValue) ([]types.KeyValue, map[string]types.InheritedItemInfo) {
	type entry struct {
		kv         types.KeyValue
		provenance *types.InheritedItemInfo
	}

	order := make([]string, 0)
	lookup := make(map[string]entry)

	// Merge folders root-to-leaf
	for _, item := range chain {
		if item.FolderDef == nil {
			continue
		}
		for _, h := range item.FolderDef.Headers {
			if strings.TrimSpace(h.Key) == "" {
				continue
			}
			lowerKey := strings.ToLower(strings.TrimSpace(h.Key))
			if _, exists := lookup[lowerKey]; !exists {
				order = append(order, lowerKey)
			}
			lookup[lowerKey] = entry{
				kv: h,
				provenance: &types.InheritedItemInfo{
					SourceFolder: item.DirName,
					SourcePath:   item.RelPath,
				},
			}
		}
	}

	// Merge request headers (overriding folder headers)
	for _, h := range reqHeaders {
		if strings.TrimSpace(h.Key) == "" {
			continue
		}
		lowerKey := strings.ToLower(strings.TrimSpace(h.Key))
		if _, exists := lookup[lowerKey]; !exists {
			order = append(order, lowerKey)
		}
		// Request header overrides provenance (it is not inherited)
		lookup[lowerKey] = entry{
			kv:         h,
			provenance: nil,
		}
	}

	merged := make([]types.KeyValue, 0, len(order))
	provenanceMap := make(map[string]types.InheritedItemInfo)

	for _, lowerKey := range order {
		e := lookup[lowerKey]
		merged = append(merged, e.kv)
		if e.provenance != nil {
			provenanceMap[lowerKey] = *e.provenance
		}
	}

	return merged, provenanceMap
}

// ResolveAuth resolves authentication. If reqAuth.Type is "inherit" or empty,
// it walks backwards from the leaf folder to the root folder until a non-inherit
// auth definition is found. If none found, defaults to "none".
func (r *InheritanceResolver) ResolveAuth(chain []FolderChainItem, reqAuth types.AuthDefinition) (types.AuthDefinition, *types.InheritedItemInfo) {
	// If request explicitly configures auth other than "inherit", it takes precedence
	if reqAuth.Type != "" && reqAuth.Type != "inherit" {
		return reqAuth, nil
	}

	// Walk from closest parent folder (leaf) up to root
	for i := len(chain) - 1; i >= 0; i-- {
		item := chain[i]
		if item.FolderDef == nil {
			continue
		}
		auth := item.FolderDef.Auth
		if auth.Type != "" && auth.Type != "inherit" {
			return auth, &types.InheritedItemInfo{
				SourceFolder: item.DirName,
				SourcePath:   item.RelPath,
			}
		}
	}

	// Default fallback when nothing in chain specifies auth
	return types.AuthDefinition{Type: "none"}, nil
}

// MergeVariables merges local folder variables into environment variables.
// Precedence: baseEnvVars → Root Folder → Parent Folder → Child Folder → overrides.
func (r *InheritanceResolver) MergeVariables(baseEnvVars map[string]string, chain []FolderChainItem, overrides map[string]string) (map[string]string, map[string]types.InheritedItemInfo) {
	merged := make(map[string]string)
	provenanceMap := make(map[string]types.InheritedItemInfo)

	for k, v := range baseEnvVars {
		merged[k] = v
	}

	// Merge folders root-to-leaf
	for _, item := range chain {
		if item.FolderDef == nil {
			continue
		}
		for _, v := range item.FolderDef.Variables {
			if v.Enabled && v.Key != "" {
				merged[v.Key] = v.Value
				provenanceMap[v.Key] = types.InheritedItemInfo{
					SourceFolder: item.DirName,
					SourcePath:   item.RelPath,
				}
			}
		}
	}

	// Apply request/execution overrides
	for k, v := range overrides {
		merged[k] = v
		delete(provenanceMap, k)
	}

	return merged, provenanceMap
}

// ResolveScripts builds the pre-request and post-response script chains.
// Pre-request scripts run Root-to-Leaf (Root → Parent → Child → Request).
// Post-response / Test scripts run Leaf-to-Root (Request → Child → Parent → Root).
func (r *InheritanceResolver) ResolveScripts(chain []FolderChainItem, reqScripts types.ScriptDefinition) ([]ScriptChainItem, []ScriptChainItem) {
	var preScripts []ScriptChainItem
	var postScripts []ScriptChainItem

	// 1. Pre-request: Root down to leaf
	for _, item := range chain {
		if item.FolderDef == nil {
			continue
		}
		if script := strings.TrimSpace(item.FolderDef.Scripts.PreRequest); script != "" {
			preScripts = append(preScripts, ScriptChainItem{
				Source: item.DirName,
				Path:   item.RelPath,
				Script: item.FolderDef.Scripts.PreRequest,
			})
		}
	}
	if script := strings.TrimSpace(reqScripts.PreRequest); script != "" {
		preScripts = append(preScripts, ScriptChainItem{
			Source: "request",
			Path:   "request",
			Script: reqScripts.PreRequest,
		})
	}

	// 2. Post-response / Test scripts: Request first, then leaf up to root
	if script := strings.TrimSpace(reqScripts.PostResponse); script != "" {
		postScripts = append(postScripts, ScriptChainItem{
			Source: "request",
			Path:   "request",
			Script: reqScripts.PostResponse,
		})
	}
	for i := len(chain) - 1; i >= 0; i-- {
		item := chain[i]
		if item.FolderDef == nil {
			continue
		}
		if script := strings.TrimSpace(item.FolderDef.Scripts.PostResponse); script != "" {
			postScripts = append(postScripts, ScriptChainItem{
				Source: item.DirName,
				Path:   item.RelPath,
				Script: item.FolderDef.Scripts.PostResponse,
			})
		}
	}

	return preScripts, postScripts
}

// ResolveRequest computes the fully merged effective RequestDefinition and provenance metadata.
func (r *InheritanceResolver) ResolveRequest(
	workspaceRoot, requestFilePath string,
	req *types.RequestDefinition,
	baseEnvVars map[string]string,
	overrides map[string]string,
) (*types.ResolvedRequestResult, []ScriptChainItem, []ScriptChainItem, map[string]string, error) {
	chain, err := r.DiscoverFolderChain(workspaceRoot, requestFilePath)
	if err != nil {
		return nil, nil, nil, nil, err
	}

	// Make a shallow copy of request to modify
	reqCopy := *req

	// Track folder headers to detect which request headers override them
	folderHeaders := make(map[string]types.InheritedItemInfo)
	for _, item := range chain {
		if item.FolderDef == nil {
			continue
		}
		for _, h := range item.FolderDef.Headers {
			if strings.TrimSpace(h.Key) == "" {
				continue
			}
			folderHeaders[strings.ToLower(strings.TrimSpace(h.Key))] = types.InheritedItemInfo{
				SourceFolder: item.DirName,
				SourcePath:   item.RelPath,
			}
		}
	}

	overriddenHeaders := make(map[string]types.InheritedItemInfo)
	for _, h := range req.Headers {
		if strings.TrimSpace(h.Key) == "" {
			continue
		}
		lowerKey := strings.ToLower(strings.TrimSpace(h.Key))
		if info, exists := folderHeaders[lowerKey]; exists {
			overriddenHeaders[lowerKey] = info
		}
	}

	// Merge headers
	mergedHeaders, headerProvenance := r.MergeHeaders(chain, req.Headers)
	reqCopy.Headers = mergedHeaders

	// Find parent auth in chain
	var parentAuth *types.AuthDefinition
	var parentAuthSource *types.InheritedItemInfo
	for i := len(chain) - 1; i >= 0; i-- {
		item := chain[i]
		if item.FolderDef != nil && item.FolderDef.Auth.Type != "" && item.FolderDef.Auth.Type != "inherit" {
			authCopy := item.FolderDef.Auth
			parentAuth = &authCopy
			parentAuthSource = &types.InheritedItemInfo{
				SourceFolder: item.DirName,
				SourcePath:   item.RelPath,
			}
			break
		}
	}

	// Resolve auth
	resolvedAuth, authProvenance := r.ResolveAuth(chain, req.Auth)
	reqCopy.Auth = resolvedAuth

	// Merge variables
	mergedVars, varProvenance := r.MergeVariables(baseEnvVars, chain, overrides)

	// Resolve scripts
	preScripts, postScripts := r.ResolveScripts(chain, req.Scripts)

	folderPreScripts := make([]string, 0)
	for _, s := range preScripts {
		if s.Source != "Request" {
			folderPreScripts = append(folderPreScripts, s.Source)
		}
	}

	folderPostScripts := make([]string, 0)
	for _, s := range postScripts {
		if s.Source != "Request" {
			folderPostScripts = append(folderPostScripts, s.Source)
		}
	}

	result := &types.ResolvedRequestResult{
		Request:           reqCopy,
		InheritedHeaders:  headerProvenance,
		OverriddenHeaders: overriddenHeaders,
		InheritedAuth:     authProvenance,
		ParentAuth:        parentAuth,
		ParentAuthSource:  parentAuthSource,
		InheritedVars:     varProvenance,
		FolderPreScripts:  folderPreScripts,
		FolderPostScripts: folderPostScripts,
	}

	return result, preScripts, postScripts, mergedVars, nil
}
