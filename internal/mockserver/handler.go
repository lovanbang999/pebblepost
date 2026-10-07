package mockserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"

	"pebblepost/internal/security"
	"pebblepost/internal/workspace"
)

// Handler exposes REST and SSE endpoints for the Mock Server in the PebblePost backend API.
type Handler struct {
	mu           sync.Mutex
	activeServer *Server
	wsService    *workspace.WorkspaceService
}

// NewHandler creates a new mock server API handler.
func NewHandler(wsSvc *workspace.WorkspaceService) *Handler {
	if wsSvc == nil {
		wsSvc = workspace.NewWorkspaceService()
	}
	return &Handler{
		wsService: wsSvc,
	}
}

// Close gracefully stops any running mock server.
func (h *Handler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.activeServer != nil && h.activeServer.IsRunning() {
		return h.activeServer.Stop()
	}
	return nil
}

// RegisterRoutes hooks the mock management endpoints onto the main application ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/mock/start", h.handleStart)
	mux.HandleFunc("/api/mock/stop", h.handleStop)
	mux.HandleFunc("/api/mock/status", h.handleStatus)
	mux.HandleFunc("/api/mock/logs", h.handleLogs)
	mux.HandleFunc("/api/mock/logs/stream", h.handleLogsStream)
	mux.HandleFunc("/api/mock/overrides", h.handleOverrides)
}

type startMockRequest struct {
	Path             string                    `json:"path"`
	WorkspacePath    string                    `json:"workspacePath"`
	Port             int                       `json:"port"`
	Host             string                    `json:"host"`
	GlobalDelayMs    int64                     `json:"globalDelayMs"`
	GlobalStatusCode int                       `json:"globalStatusCode"`
	GlobalErrorRate  float64                   `json:"globalErrorRate"`
	Overrides        map[string]*RouteOverride `json:"overrides"`
}

func (h *Handler) handleStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req startMockRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err.Error() != "EOF" {
		h.jsonError(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	targetPath := req.Path
	if targetPath == "" {
		targetPath = req.WorkspacePath
	}
	if targetPath == "" {
		targetPath = "."
	}

	if req.WorkspacePath != "" {
		if _, err := security.SafeAbsolute(req.WorkspacePath, targetPath); err != nil {
			h.jsonError(w, "Path traversal rejected: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(targetPath); err != nil {
		h.jsonError(w, "Path traversal rejected: "+err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	// Stop existing server if running
	if h.activeServer != nil && h.activeServer.IsRunning() {
		_ = h.activeServer.Stop()
	}

	cfg := ServerConfig{
		Host:             req.Host,
		Port:             req.Port,
		WorkspacePath:    req.WorkspacePath,
		TargetPath:       targetPath,
		GlobalDelayMs:    req.GlobalDelayMs,
		GlobalStatusCode: req.GlobalStatusCode,
		GlobalErrorRate:  req.GlobalErrorRate,
		Overrides:        req.Overrides,
	}

	srv := NewServer(cfg, h.wsService)
	if err := srv.LoadRoutes(targetPath); err != nil {
		h.jsonError(w, "Failed to load mock routes: "+err.Error(), http.StatusBadRequest)
		return
	}

	if err := srv.Start(); err != nil {
		h.jsonError(w, "Failed to start mock server: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.activeServer = srv

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(srv.Status())
}

func (h *Handler) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	if h.activeServer != nil {
		_ = h.activeServer.Stop()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(h.activeServer.Status())
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(ServerStatus{Running: false})
}

func (h *Handler) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if h.activeServer != nil {
		_ = json.NewEncoder(w).Encode(h.activeServer.Status())
		return
	}

	_ = json.NewEncoder(w).Encode(ServerStatus{Running: false})
}

func (h *Handler) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.mu.Lock()
	srv := h.activeServer
	h.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	if srv == nil {
		_ = json.NewEncoder(w).Encode([]RequestLog{})
		return
	}

	_ = json.NewEncoder(w).Encode(srv.Logs())
}

func (h *Handler) handleLogsStream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	h.mu.Lock()
	srv := h.activeServer
	h.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	if srv == nil {
		fmt.Fprintf(w, "data: {\"type\":\"status\",\"running\":false}\n\n")
		flusher.Flush()
		return
	}

	fmt.Fprintf(w, "data: {\"type\":\"status\",\"running\":%t,\"url\":\"%s\"}\n\n", srv.IsRunning(), srv.URL())
	flusher.Flush()

	logChan := srv.SubscribeLogs(r.Context())
	for {
		select {
		case <-r.Context().Done():
			return
		case entry, open := <-logChan:
			if !open {
				return
			}
			bytes, err := json.Marshal(entry)
			if err == nil {
				fmt.Fprintf(w, "data: %s\n\n", string(bytes))
				flusher.Flush()
			}
		}
	}
}

type updateOverrideRequest struct {
	RouteID  string         `json:"routeId"`
	Override *RouteOverride `json:"override"`
}

func (h *Handler) handleOverrides(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodPut {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req updateOverrideRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	h.mu.Lock()
	srv := h.activeServer
	h.mu.Unlock()

	if srv == nil || !srv.IsRunning() {
		h.jsonError(w, "Mock server is not running", http.StatusBadRequest)
		return
	}

	srv.SetRouteOverride(req.RouteID, req.Override)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success": true,
		"routeId": req.RouteID,
	})
}

func (h *Handler) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}
