package httpclient

import (
	"encoding/json"
	"net/http"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// Handler exposes HTTP API endpoints for executing HTTP requests.
type Handler struct {
	client         Client
	workspaceSvc   *workspace.WorkspaceService
	environmentSvc *workspace.EnvironmentService
	interpolator   *workspace.Interpolator
}

// NewHandler creates a new HTTP Client API Handler.
func NewHandler(
	client Client,
	wsSvc *workspace.WorkspaceService,
	envSvc *workspace.EnvironmentService,
	in *workspace.Interpolator,
) *Handler {
	return &Handler{
		client:         client,
		workspaceSvc:   wsSvc,
		environmentSvc: envSvc,
		interpolator:   in,
	}
}

// RegisterRoutes registers execution endpoints on the provided HTTP ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/request/execute", h.handleExecute)
}

// ExecutePayload defines the JSON body for the /api/request/execute endpoint.
type ExecutePayload struct {
	WorkspacePath   string                   `json:"workspacePath,omitempty"`
	EnvironmentName string                   `json:"environmentName,omitempty"`
	Request         *types.RequestDefinition `json:"request"`
	Overrides       map[string]string        `json:"overrides,omitempty"`
}

func (h *Handler) handleExecute(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload ExecutePayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if payload.Request == nil {
		h.jsonError(w, "Request definition is required", http.StatusBadRequest)
		return
	}

	reqToExecute := payload.Request

	// If environment or overrides are supplied, run interpolation
	if h.interpolator != nil {
		var env *types.EnvironmentDefinition
		if h.environmentSvc != nil && payload.WorkspacePath != "" && payload.EnvironmentName != "" {
			env, _ = h.environmentSvc.GetEnvironment(payload.WorkspacePath, payload.EnvironmentName)
		}

		varMap := h.interpolator.BuildVariableMap(env, payload.Overrides)
		reqToExecute = h.interpolator.InterpolateRequest(payload.Request, varMap)
	}

	// Execute HTTP request
	result, err := h.client.Execute(r.Context(), reqToExecute)
	if err != nil && result == nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, result)
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
