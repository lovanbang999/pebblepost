package workspace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"pebblepost/internal/impexp"
	"pebblepost/internal/security"
	"pebblepost/internal/types"
)

// Handler exposes HTTP API endpoints for workspace and collection management.
type Handler struct {
	workspaceSvc        *WorkspaceService
	environmentSvc      *EnvironmentService
	interpolator        *Interpolator
	inheritanceResolver *InheritanceResolver
	importSvc           *ImportService
	impexpSvc           *impexp.Service
	watchersMu          sync.Mutex
	watchers            map[string]*Watcher
}

// NewHandler creates a new Handler instance.
func NewHandler(ws *WorkspaceService, env *EnvironmentService, in *Interpolator) *Handler {
	return &Handler{
		workspaceSvc:        ws,
		environmentSvc:      env,
		interpolator:        in,
		inheritanceResolver: NewInheritanceResolver(ws),
		importSvc:           NewImportService(),
		impexpSvc:           impexp.NewService(),
		watchers:            make(map[string]*Watcher),
	}
}

// RegisterRoutes registers all workspace API routes on the provided HTTP ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/workspace/scan", h.handleScan)
	mux.HandleFunc("/api/workspace/init", h.handleInit)
	mux.HandleFunc("/api/workspace/info", h.handleInfo)
	mux.HandleFunc("/api/workspace/folder", h.handleCreateFolder)
	mux.HandleFunc("/api/workspace/rename", h.handleRename)
	mux.HandleFunc("/api/workspace/move", h.handleMove)
	mux.HandleFunc("/api/workspace/duplicate", h.handleDuplicate)
	mux.HandleFunc("/api/workspace/gitignore/ensure", h.handleEnsureGitignore)
	mux.HandleFunc("/api/workspace/events", h.handleEvents)
	mux.HandleFunc("/api/folder", h.handleFolder)
	mux.HandleFunc("/api/folder/save", h.handleSaveFolder)
	mux.HandleFunc("/api/request", h.handleGetRequest)
	mux.HandleFunc("/api/request/resolved", h.handleResolvedRequest)
	mux.HandleFunc("/api/request/save", h.handleSaveRequest)
	mux.HandleFunc("/api/request/delete", h.handleDeleteRequest)
	mux.HandleFunc("/api/request/example/save", h.handleSaveExample)
	mux.HandleFunc("/api/request/example/delete", h.handleDeleteExample)
	mux.HandleFunc("/api/request/interpolate", h.handleInterpolate)
	mux.HandleFunc("/api/environments", h.handleEnvironments)
	mux.HandleFunc("/api/environments/save", h.handleSaveEnvironment)
	mux.HandleFunc("/api/import", h.handleImport)
	mux.HandleFunc("/api/impexp/export", h.handleExport)
	mux.HandleFunc("/api/response/body", h.handleResponseBody)
}

func (h *Handler) handleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rootPath := r.URL.Query().Get("path")
	if rootPath == "" {
		http.Error(w, "Query parameter 'path' is required", http.StatusBadRequest)
		return
	}

	// Reject traversal payloads in the workspace root itself.
	if err := security.ValidateWorkspaceRoot(rootPath); err != nil {
		h.jsonError(w, "invalid workspace path: "+err.Error(), http.StatusBadRequest)
		return
	}

	tree, err := h.workspaceSvc.ScanTree(rootPath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	wsInfo, _ := h.workspaceSvc.GetWorkspaceInfo(rootPath)
	envs, _ := h.environmentSvc.ListEnvironments(rootPath)

	gitStatus, _ := security.CheckGitignore(rootPath)
	var gitStatusStr string
	switch gitStatus {
	case security.GitignoreOK:
		gitStatusStr = "ok"
	case security.GitignoreMissing:
		gitStatusStr = "missing"
	case security.GitignoreNoRule:
		gitStatusStr = "no_rule"
	}

	h.jsonResponse(w, map[string]any{
		"workspace":       wsInfo,
		"tree":            tree,
		"environments":    envs,
		"gitignoreStatus": gitStatusStr,
	})
}

func (h *Handler) handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Path string `json:"path"`
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	wsDef, err := h.workspaceSvc.Init(payload.Path, payload.Name)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success":   true,
		"workspace": wsDef,
	})
}

func (h *Handler) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	rootPath := r.URL.Query().Get("path")
	if rootPath == "" {
		http.Error(w, "Query parameter 'path' is required", http.StatusBadRequest)
		return
	}

	wsInfo, err := h.workspaceSvc.GetWorkspaceInfo(rootPath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, wsInfo)
}

func (h *Handler) handleGetRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Query parameter 'path' is required", http.StatusBadRequest)
		return
	}

	// Validate path stays inside the declared workspace root when provided.
	if workspacePath := r.URL.Query().Get("workspacePath"); workspacePath != "" {
		if _, err := security.SafeAbsolute(workspacePath, filePath); err != nil {
			h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(filePath); err != nil {
		h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
		return
	}

	reqDef, err := h.workspaceSvc.ReadRequest(filePath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	h.jsonResponse(w, reqDef)
}

func (h *Handler) handleFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderPath := r.URL.Query().Get("path")
	if folderPath == "" {
		http.Error(w, "Query parameter 'path' is required", http.StatusBadRequest)
		return
	}

	if workspacePath := r.URL.Query().Get("workspacePath"); workspacePath != "" {
		if _, err := security.SafeAbsolute(workspacePath, folderPath); err != nil {
			h.jsonError(w, "invalid folder path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(folderPath); err != nil {
		h.jsonError(w, "invalid folder path: "+err.Error(), http.StatusBadRequest)
		return
	}

	folderDef, err := h.workspaceSvc.ReadFolder(folderPath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, folderDef)
}

func (h *Handler) handleSaveFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string                  `json:"workspacePath"`
		Path          string                  `json:"path"`
		Folder        *types.FolderDefinition `json:"folder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.Folder == nil {
		h.jsonError(w, "Folder definition cannot be empty", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.Path); err != nil {
			h.jsonError(w, "invalid folder path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(payload.Path); err != nil {
		h.jsonError(w, "invalid folder path: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.workspaceSvc.SaveFolder(payload.Path, payload.Folder); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
		"path":    payload.Path,
	})
}

func (h *Handler) handleResolvedRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.URL.Query().Get("path")
	if filePath == "" {
		http.Error(w, "Query parameter 'path' is required", http.StatusBadRequest)
		return
	}

	workspacePath := r.URL.Query().Get("workspacePath")
	if workspacePath != "" {
		if _, err := security.SafeAbsolute(workspacePath, filePath); err != nil {
			h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(filePath); err != nil {
		h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
		return
	}

	reqDef, err := h.workspaceSvc.ReadRequest(filePath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	var baseEnvVars map[string]string
	if workspacePath != "" {
		envName := r.URL.Query().Get("environmentName")
		if envName != "" {
			if env, _ := h.environmentSvc.GetEnvironment(workspacePath, envName); env != nil {
				baseEnvVars = h.interpolator.BuildVariableMap(env, nil)
			}
		}
	}

	resolved, _, _, _, err := h.inheritanceResolver.ResolveRequest(workspacePath, filePath, reqDef, baseEnvVars, nil)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, resolved)
}

func (h *Handler) handleSaveRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string                   `json:"workspacePath"`
		Path          string                   `json:"path"`
		Request       *types.RequestDefinition `json:"request"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.Request == nil {
		h.jsonError(w, "Request definition cannot be empty", http.StatusBadRequest)
		return
	}

	// Validate path against workspace root when available.
	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.Path); err != nil {
			h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(payload.Path); err != nil {
		h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.workspaceSvc.SaveRequest(payload.Path, payload.Request); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
		"path":    payload.Path,
	})
}

func (h *Handler) handleDeleteRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string `json:"workspacePath"`
		Path          string `json:"path"`
		Permanent     bool   `json:"permanent"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.Path == "" {
		h.jsonError(w, "path cannot be empty", http.StatusBadRequest)
		return
	}

	// Validate path against workspace root when available.
	if payload.WorkspacePath != "" {
		if filepath.Clean(payload.Path) == filepath.Clean(payload.WorkspacePath) {
			h.jsonError(w, "cannot delete workspace root", http.StatusBadRequest)
			return
		}
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.Path); err != nil {
			h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(payload.Path); err != nil {
		h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
		return
	}

	var err error
	if !payload.Permanent && payload.WorkspacePath != "" {
		err = h.workspaceSvc.TrashPath(payload.WorkspacePath, payload.Path)
	} else {
		err = h.workspaceSvc.DeletePath(payload.Path)
	}

	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
	})
}

func (h *Handler) handleSaveExample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string                `json:"workspacePath"`
		Path          string                `json:"path"`
		Example       types.ExampleResponse `json:"example"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.Path == "" {
		h.jsonError(w, "request path cannot be empty", http.StatusBadRequest)
		return
	}

	// Validate path against workspace root when available
	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.Path); err != nil {
			h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(payload.Path); err != nil {
		h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
		return
	}

	req, err := h.workspaceSvc.ReadRequest(payload.Path)
	if err != nil {
		h.jsonError(w, "failed to load request: "+err.Error(), http.StatusNotFound)
		return
	}

	// 1. Gather secret values from workspace/active environment to mask in example body
	var secretValues []string
	if payload.WorkspacePath != "" {
		wsInfo, _ := h.workspaceSvc.GetWorkspaceInfo(payload.WorkspacePath)
		activeEnv := ""
		if wsInfo != nil {
			activeEnv = wsInfo.ActiveEnvironment
		}
		envs, _ := h.environmentSvc.ListEnvironments(payload.WorkspacePath)
		for _, env := range envs {
			if env.Name == activeEnv || activeEnv == "" {
				for _, v := range env.Variables {
					if v.Secret && v.Value != "" {
						secretValues = append(secretValues, v.Value)
					}
				}
			}
		}
	}

	// 2. Sanitize sensitive headers
	sensitiveHeaderKeys := map[string]bool{
		"authorization":       true,
		"proxy-authorization": true,
		"cookie":              true,
		"set-cookie":          true,
		"x-api-key":           true,
		"x-auth-token":        true,
	}
	sanitizedHeaders := make([]types.KeyValue, len(payload.Example.Headers))
	for i, hdr := range payload.Example.Headers {
		sanitizedHeaders[i] = hdr
		if sensitiveHeaderKeys[strings.ToLower(hdr.Key)] {
			sanitizedHeaders[i].Value = "[REDACTED]"
		} else if len(secretValues) > 0 {
			sanitizedHeaders[i].Value = security.MaskSecrets(hdr.Value, secretValues)
		}
	}
	payload.Example.Headers = sanitizedHeaders

	// 3. Enforce 512 KB size limit on body
	const maxBodyBytes = 512 * 1024
	bodyStr := payload.Example.Body
	if len(bodyStr) > maxBodyBytes {
		bodyStr = bodyStr[:maxBodyBytes] + "\n\n[Truncated: response body exceeds 512KB limit]"
	}
	if len(secretValues) > 0 {
		bodyStr = security.MaskSecrets(bodyStr, secretValues)
	}
	payload.Example.Body = bodyStr
	payload.Example.Size = int64(len(bodyStr))

	if payload.Example.ID == "" {
		payload.Example.ID = fmt.Sprintf("ex_%d", time.Now().UnixNano())
	}
	if payload.Example.SavedAt.IsZero() {
		payload.Example.SavedAt = time.Now()
	}
	if payload.Example.Name == "" {
		payload.Example.Name = fmt.Sprintf("%d %s Example", payload.Example.StatusCode, payload.Example.StatusText)
	}

	// 4. Update existing example or append
	updated := false
	for i, ex := range req.Examples {
		if ex.ID == payload.Example.ID {
			req.Examples[i] = payload.Example
			updated = true
			break
		}
	}
	if !updated {
		req.Examples = append(req.Examples, payload.Example)
	}

	// 5. Cap at 10 examples maximum (FIFO eviction)
	const maxSavedExamples = 10
	if len(req.Examples) > maxSavedExamples {
		req.Examples = req.Examples[len(req.Examples)-maxSavedExamples:]
	}

	// 6. Save request back to disk
	if err := h.workspaceSvc.SaveRequest(payload.Path, req); err != nil {
		h.jsonError(w, "failed to save example: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success":  true,
		"example":  payload.Example,
		"examples": req.Examples,
	})
}

func (h *Handler) handleDeleteExample(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string `json:"workspacePath"`
		Path          string `json:"path"`
		ExampleID     string `json:"exampleId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.Path == "" || payload.ExampleID == "" {
		h.jsonError(w, "path and exampleId are required", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.Path); err != nil {
			h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(payload.Path); err != nil {
		h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
		return
	}

	req, err := h.workspaceSvc.ReadRequest(payload.Path)
	if err != nil {
		h.jsonError(w, "failed to load request: "+err.Error(), http.StatusNotFound)
		return
	}

	filtered := make([]types.ExampleResponse, 0, len(req.Examples))
	for _, ex := range req.Examples {
		if ex.ID != payload.ExampleID {
			filtered = append(filtered, ex)
		}
	}
	req.Examples = filtered

	if err := h.workspaceSvc.SaveRequest(payload.Path, req); err != nil {
		h.jsonError(w, "failed to save request: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success":  true,
		"examples": req.Examples,
	})
}

func (h *Handler) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string `json:"workspacePath"`
		Path          string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.Path); err != nil {
			h.jsonError(w, "invalid folder path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(payload.Path); err != nil {
		h.jsonError(w, "invalid folder path: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.workspaceSvc.CreateFolder(payload.Path); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
	})
}

func (h *Handler) handleRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string `json:"workspacePath"`
		OldPath       string `json:"oldPath"`
		NewPath       string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.OldPath); err != nil {
			h.jsonError(w, "invalid old path: "+err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.NewPath); err != nil {
			h.jsonError(w, "invalid new path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := security.RejectTraversal(payload.OldPath); err != nil {
			h.jsonError(w, "invalid old path: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := security.RejectTraversal(payload.NewPath); err != nil {
			h.jsonError(w, "invalid new path: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	if err := h.workspaceSvc.Rename(payload.OldPath, payload.NewPath); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
	})
}

func (h *Handler) handleDuplicate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string `json:"workspacePath"`
		Path          string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.Path == "" {
		h.jsonError(w, "path cannot be empty", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.Path); err != nil {
			h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(payload.Path); err != nil {
		h.jsonError(w, "invalid file path: "+err.Error(), http.StatusBadRequest)
		return
	}

	newPath, err := h.workspaceSvc.DuplicateRequest(payload.Path)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
		"newPath": newPath,
	})
}

func (h *Handler) handleMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string `json:"workspacePath"`
		SourcePath    string `json:"sourcePath"`
		TargetFolder  string `json:"targetFolder"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.SourcePath == "" || payload.TargetFolder == "" {
		h.jsonError(w, "sourcePath and targetFolder cannot be empty", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.SourcePath); err != nil {
			h.jsonError(w, "invalid source path: "+err.Error(), http.StatusBadRequest)
			return
		}
		if _, err := security.SafeAbsolute(payload.WorkspacePath, payload.TargetFolder); err != nil {
			h.jsonError(w, "invalid target folder: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		if err := security.RejectTraversal(payload.SourcePath); err != nil {
			h.jsonError(w, "invalid source path: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := security.RejectTraversal(payload.TargetFolder); err != nil {
			h.jsonError(w, "invalid target folder: "+err.Error(), http.StatusBadRequest)
			return
		}
	}

	newPath, err := h.workspaceSvc.MovePath(payload.SourcePath, payload.TargetFolder)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
		"newPath": newPath,
	})
}

func (h *Handler) handleEnsureGitignore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string `json:"workspacePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath == "" {
		h.jsonError(w, "workspacePath is required", http.StatusBadRequest)
		return
	}

	if err := security.ValidateWorkspaceRoot(payload.WorkspacePath); err != nil {
		h.jsonError(w, "invalid workspace path: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := security.EnsureGitignoreEntry(payload.WorkspacePath); err != nil {
		h.jsonError(w, "failed to update .gitignore: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
	})
}

func (h *Handler) handleEnvironments(w http.ResponseWriter, r *http.Request) {
	rootPath := r.URL.Query().Get("workspacePath")
	if rootPath == "" {
		http.Error(w, "Query parameter 'workspacePath' is required", http.StatusBadRequest)
		return
	}

	if err := security.ValidateWorkspaceRoot(rootPath); err != nil {
		h.jsonError(w, "invalid workspace path: "+err.Error(), http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		envs, err := h.environmentSvc.ListEnvironments(rootPath)
		if err != nil {
			h.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.jsonResponse(w, envs)

	case http.MethodDelete:
		envName := r.URL.Query().Get("name")
		if envName == "" {
			http.Error(w, "Query parameter 'name' is required", http.StatusBadRequest)
			return
		}
		if err := security.ValidateSafeIdentifier(envName); err != nil {
			h.jsonError(w, "invalid environment name: "+err.Error(), http.StatusBadRequest)
			return
		}
		if err := h.environmentSvc.DeleteEnvironment(rootPath, envName); err != nil {
			h.jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		h.jsonResponse(w, map[string]any{"success": true})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) handleSaveEnvironment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string                      `json:"workspacePath"`
		Environment   types.EnvironmentDefinition `json:"environment"`
		IsSecret      bool                        `json:"isSecret"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath == "" {
		h.jsonError(w, "workspacePath is required", http.StatusBadRequest)
		return
	}

	if err := security.ValidateWorkspaceRoot(payload.WorkspacePath); err != nil {
		h.jsonError(w, "invalid workspace path: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := security.ValidateSafeIdentifier(payload.Environment.Name); err != nil {
		h.jsonError(w, "invalid environment name: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.environmentSvc.SaveEnvironment(payload.WorkspacePath, payload.Environment, payload.IsSecret); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
	})
}

func (h *Handler) handleInterpolate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath   string                   `json:"workspacePath"`
		EnvironmentName string                   `json:"environmentName"`
		Request         *types.RequestDefinition `json:"request"`
		Overrides       map[string]string        `json:"overrides"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if payload.Request == nil {
		h.jsonError(w, "Request definition is required", http.StatusBadRequest)
		return
	}

	var env *types.EnvironmentDefinition
	if payload.WorkspacePath != "" && payload.EnvironmentName != "" {
		env, _ = h.environmentSvc.GetEnvironment(payload.WorkspacePath, payload.EnvironmentName)
	}

	varMap := h.interpolator.BuildVariableMap(env, payload.Overrides)
	interpolatedReq := h.interpolator.InterpolateRequest(payload.Request, varMap)

	h.jsonResponse(w, map[string]any{
		"interpolatedRequest": interpolatedReq,
		"variables":           varMap,
	})
}

func (h *Handler) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Source    string `json:"source"`    // "curl", "postman", "openapi", "bruno", "insomnia", "har" (or empty for auto)
		Content   string `json:"content"`   // raw text or JSON string
		Save      bool   `json:"save"`      // if true, write directly to workspace filesystem
		OutDir    string `json:"outDir"`    // target folder in workspace (e.g. collections/my-api)
		Workspace string `json:"workspace"` // workspace root path
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if len(payload.Content) > impexp.MaxImportFileSize {
		h.jsonError(w, "File size exceeds maximum allowed size of 50 MB", http.StatusBadRequest)
		return
	}

	result, err := h.impexpSvc.Parse([]byte(payload.Content), payload.Source, impexp.Format(payload.Source))
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	var reqs []*types.RequestDefinition
	for _, it := range result.Requests {
		if it.Request != nil {
			reqs = append(reqs, it.Request)
		}
	}

	report := result.ToReport()

	if payload.Save {
		wsRoot := payload.Workspace
		if wsRoot == "" {
			wsRoot = "."
		}
		savedReport, saveErr := h.impexpSvc.SaveToWorkspace(wsRoot, payload.OutDir, result)
		if saveErr != nil {
			h.jsonError(w, fmt.Sprintf("Failed to save imported collection: %v", saveErr), http.StatusInternalServerError)
			return
		}
		report = *savedReport
	}

	h.jsonResponse(w, map[string]any{
		"requests": reqs,
		"count":    len(reqs),
		"report":   report,
	})
}

func (h *Handler) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Format         string `json:"format"` // "postman" or "openapi"
		CollectionName string `json:"collectionName"`
		FolderPath     string `json:"folderPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	folderPath := payload.FolderPath
	if folderPath == "" {
		folderPath = CollectionsDir
	}

	var items []impexp.ImportedItem
	var folders []impexp.ImportedFolder
	var variables []types.KeyValue

	_ = filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		if strings.HasSuffix(info.Name(), PebbleExt) && info.Name() != FolderFile {
			req, err := h.workspaceSvc.ReadRequest(path)
			if err == nil && req != nil {
				rel, _ := filepath.Rel(folderPath, path)
				items = append(items, impexp.ImportedItem{
					Name:    req.Name,
					RelPath: rel,
					Request: req,
				})
			}
		} else if info.Name() == FolderFile {
			fDef, err := h.workspaceSvc.ReadFolder(filepath.Dir(path))
			if err == nil && fDef != nil {
				relDir, _ := filepath.Rel(folderPath, filepath.Dir(path))
				folders = append(folders, impexp.ImportedFolder{
					Name:       fDef.Name,
					RelPath:    relDir,
					Definition: *fDef,
				})
				if relDir == "." || relDir == "" {
					variables = append(variables, fDef.Variables...)
				}
			}
		}
		return nil
	})

	colName := payload.CollectionName
	if colName == "" {
		colName = filepath.Base(folderPath)
		if colName == "." || colName == "" || colName == CollectionsDir {
			colName = "PebblePost Export"
		}
	}

	exportBytes, err := h.impexpSvc.Export(colName, impexp.Format(payload.Format), items, folders, variables)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	filename := impexp.SanitizeFilename(colName) + "_" + payload.Format + ".json"

	h.jsonResponse(w, map[string]any{
		"format":   payload.Format,
		"filename": filename,
		"content":  string(exportBytes),
		"count":    len(items),
	})
}

func (h *Handler) jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) jsonError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": message,
	})
}

func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	workspacePath := r.URL.Query().Get("workspacePath")
	if workspacePath == "" {
		http.Error(w, "Query parameter 'workspacePath' is required", http.StatusBadRequest)
		return
	}

	if err := security.ValidateWorkspaceRoot(workspacePath); err != nil {
		h.jsonError(w, "invalid workspace path: "+err.Error(), http.StatusBadRequest)
		return
	}

	watcher, err := h.getOrCreateWatcher(workspacePath)
	if err != nil {
		h.jsonError(w, "failed to start workspace watcher: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	// Send initial connection confirmation event
	_, _ = fmt.Fprintf(w, "data: {\"type\":\"connected\",\"workspacePath\":%q}\n\n", workspacePath)
	flusher.Flush()

	events, unsubscribe := watcher.Subscribe()
	defer unsubscribe()

	for {
		select {
		case <-r.Context().Done():
			return
		case evt, ok := <-events:
			if !ok {
				return
			}
			data, err := json.Marshal(evt)
			if err == nil {
				_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
				flusher.Flush()
			}
		}
	}
}

func (h *Handler) getOrCreateWatcher(rootPath string) (*Watcher, error) {
	h.watchersMu.Lock()
	defer h.watchersMu.Unlock()

	clean := filepath.Clean(rootPath)
	if w, exists := h.watchers[clean]; exists {
		return w, nil
	}

	w, err := NewWatcher(clean)
	if err != nil {
		return nil, err
	}

	h.watchers[clean] = w
	h.workspaceSvc.SetWatcher(w)
	h.environmentSvc.SetWatcher(w)

	return w, nil
}

// Close gracefully closes all active workspace file watchers.
func (h *Handler) Close() {
	h.watchersMu.Lock()
	defer h.watchersMu.Unlock()

	for _, w := range h.watchers {
		_ = w.Close()
	}
	h.watchers = make(map[string]*Watcher)
}

// handleResponseBody serves a byte-range chunk from a previously spooled temp body file.
// Query params: file (required), offset (default 0), limit (default 5MB).
func (h *Handler) handleResponseBody(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	filePath := r.URL.Query().Get("file")
	if filePath == "" {
		h.jsonError(w, "query parameter 'file' is required", http.StatusBadRequest)
		return
	}

	// Safety: only allow OS temp files created by pebblepost.
	if !strings.HasPrefix(filepath.Base(filePath), "pebble-body-") {
		h.jsonError(w, "invalid file reference", http.StatusBadRequest)
		return
	}

	offsetStr := r.URL.Query().Get("offset")
	limitStr := r.URL.Query().Get("limit")

	offset, _ := strconv.ParseInt(offsetStr, 10, 64)
	limit, err := strconv.ParseInt(limitStr, 10, 64)
	if err != nil || limit <= 0 || limit > 5*1024*1024 {
		limit = 5 * 1024 * 1024 // 5 MB default chunk
	}

	f, err := os.Open(filePath)
	if err != nil {
		h.jsonError(w, "temp file not found or expired", http.StatusNotFound)
		return
	}
	defer f.Close()

	stat, _ := f.Stat()
	totalSize := stat.Size()

	if offset > 0 {
		if _, err := f.Seek(offset, io.SeekStart); err != nil {
			h.jsonError(w, "seek error: "+err.Error(), http.StatusInternalServerError)
			return
		}
	}

	chunk, err := io.ReadAll(io.LimitReader(f, limit))
	if err != nil {
		h.jsonError(w, "read error: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"chunk":     string(chunk),
		"offset":    offset,
		"chunkSize": int64(len(chunk)),
		"totalSize": totalSize,
		"hasMore":   offset+int64(len(chunk)) < totalSize,
	})
}
