package gitclient

import (
	"encoding/json"
	"net/http"
	"strconv"

	"pebblepost/internal/security"
)

// Handler exposes HTTP API endpoints for git operations.
type Handler struct {
	svc *Service
}

// NewHandler creates a new git handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all git API routes on the provided ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/git/status", h.handleStatus)
	mux.HandleFunc("/api/git/branches", h.handleBranches)
	mux.HandleFunc("/api/git/diff", h.handleDiff)
	mux.HandleFunc("/api/git/log", h.handleLog)
	mux.HandleFunc("/api/git/stage", h.handleStage)
	mux.HandleFunc("/api/git/unstage", h.handleUnstage)
	mux.HandleFunc("/api/git/commit", h.handleCommit)
	mux.HandleFunc("/api/git/checkout", h.handleCheckout)
	mux.HandleFunc("/api/git/branch/create", h.handleCreateBranch)
	mux.HandleFunc("/api/git/pull", h.handlePull)
	mux.HandleFunc("/api/git/push", h.handlePush)
	mux.HandleFunc("/api/git/init", h.handleInit)
}

func (h *Handler) getWorkspacePath(r *http.Request) (string, error) {
	ws := r.URL.Query().Get("workspacePath")
	if ws == "" {
		ws = r.URL.Query().Get("path")
	}
	if ws == "" {
		return "", http.ErrMissingBoundary // use general check
	}
	if err := security.ValidateWorkspaceRoot(ws); err != nil {
		return "", err
	}
	return ws, nil
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	status, err := h.svc.Status(r.Context(), ws)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, status)
}

func (h *Handler) handleBranches(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	branches, err := h.svc.Branches(r.Context(), ws)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, branches)
}

func (h *Handler) handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	relPath := r.URL.Query().Get("path")
	if relPath == "" {
		relPath = r.URL.Query().Get("file")
	}
	if relPath == "" {
		h.jsonError(w, "query parameter 'path' or 'file' is required", http.StatusBadRequest)
		return
	}

	diffs, err := h.svc.DiffRequest(r.Context(), ws, relPath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, diffs)
}

func (h *Handler) handleLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if val, err := strconv.Atoi(l); err == nil && val > 0 {
			limit = val
		}
	}

	logs, err := h.svc.Log(r.Context(), ws, limit)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, logs)
}

func (h *Handler) handleStage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req StageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.Stage(r.Context(), ws, req.Paths); err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, map[string]any{"success": true})
}

func (h *Handler) handleUnstage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req StageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.Unstage(r.Context(), ws, req.Paths); err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, map[string]any{"success": true})
}

func (h *Handler) handleCommit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req CommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.Commit(r.Context(), ws, req.Message); err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, map[string]any{"success": true})
}

func (h *Handler) handleCheckout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req CheckoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.svc.Checkout(r.Context(), ws, req.Branch)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, resp)
}

func (h *Handler) handleCreateBranch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	var req CreateBranchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "invalid request payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.CreateBranch(r.Context(), ws, req.Name); err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, map[string]any{"success": true, "name": req.Name})
}

func (h *Handler) handlePull(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.svc.Pull(r.Context(), ws)
	if err != nil && resp != nil {
		// Return 400 with readable error in response body
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, resp)
}

func (h *Handler) handlePush(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	resp, err := h.svc.Push(r.Context(), ws)
	if err != nil && resp != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(resp)
		return
	}
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, resp)
}

func (h *Handler) handleInit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ws, err := h.getWorkspacePath(r)
	if err != nil {
		h.jsonError(w, "invalid or missing workspacePath: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.svc.Init(r.Context(), ws); err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, map[string]any{"success": true})
}

func (h *Handler) jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
