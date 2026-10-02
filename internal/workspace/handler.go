package workspace

import (
	"encoding/json"
	"net/http"

	"pebblepost/internal/types"
)

// Handler exposes HTTP API endpoints for workspace and collection management.
type Handler struct {
	workspaceSvc   *WorkspaceService
	environmentSvc *EnvironmentService
	interpolator   *Interpolator
	importSvc      *ImportService
}

// NewHandler creates a new Handler instance.
func NewHandler(ws *WorkspaceService, env *EnvironmentService, in *Interpolator) *Handler {
	return &Handler{
		workspaceSvc:   ws,
		environmentSvc: env,
		interpolator:   in,
		importSvc:      NewImportService(),
	}
}

// RegisterRoutes registers all workspace API routes on the provided HTTP ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/workspace/scan", h.handleScan)
	mux.HandleFunc("/api/workspace/init", h.handleInit)
	mux.HandleFunc("/api/workspace/info", h.handleInfo)
	mux.HandleFunc("/api/workspace/folder", h.handleCreateFolder)
	mux.HandleFunc("/api/workspace/rename", h.handleRename)
	mux.HandleFunc("/api/request", h.handleGetRequest)
	mux.HandleFunc("/api/request/save", h.handleSaveRequest)
	mux.HandleFunc("/api/request/delete", h.handleDeleteRequest)
	mux.HandleFunc("/api/request/interpolate", h.handleInterpolate)
	mux.HandleFunc("/api/environments", h.handleEnvironments)
	mux.HandleFunc("/api/environments/save", h.handleSaveEnvironment)
	mux.HandleFunc("/api/import", h.handleImport)
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

	tree, err := h.workspaceSvc.ScanTree(rootPath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	wsInfo, _ := h.workspaceSvc.GetWorkspaceInfo(rootPath)
	envs, _ := h.environmentSvc.ListEnvironments(rootPath)

	h.jsonResponse(w, map[string]any{
		"workspace":    wsInfo,
		"tree":         tree,
		"environments": envs,
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

	reqDef, err := h.workspaceSvc.ReadRequest(filePath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusNotFound)
		return
	}

	h.jsonResponse(w, reqDef)
}

func (h *Handler) handleSaveRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Path    string                   `json:"path"`
		Request *types.RequestDefinition `json:"request"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	if payload.Request == nil {
		h.jsonError(w, "Request definition cannot be empty", http.StatusBadRequest)
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
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if err := h.workspaceSvc.DeletePath(payload.Path); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{
		"success": true,
	})
}

func (h *Handler) handleCreateFolder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Path string `json:"path"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
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
		OldPath string `json:"oldPath"`
		NewPath string `json:"newPath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	if err := h.workspaceSvc.Rename(payload.OldPath, payload.NewPath); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
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
		Source string `json:"source"` // "curl", "postman", "openapi"
		Content string `json:"content"` // raw text or JSON string
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid request payload", http.StatusBadRequest)
		return
	}

	var reqs []*types.RequestDefinition
	var err error

	switch payload.Source {
	case "curl":
		var req *types.RequestDefinition
		req, err = h.importSvc.ParseCURL(payload.Content)
		if err == nil {
			reqs = []*types.RequestDefinition{req}
		}
	case "postman":
		reqs, err = h.importSvc.ParsePostman([]byte(payload.Content))
	case "openapi":
		reqs, err = h.importSvc.ParseOpenAPI([]byte(payload.Content))
	default:
		h.jsonError(w, "source must be one of: curl, postman, openapi", http.StatusBadRequest)
		return
	}

	if err != nil {
		h.jsonError(w, err.Error(), http.StatusUnprocessableEntity)
		return
	}

	h.jsonResponse(w, map[string]any{
		"requests": reqs,
		"count":    len(reqs),
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
