package docs

import (
	"encoding/json"
	"net/http"

	"pebblepost/internal/security"
)

// Handler handles HTTP requests for documentation viewing and generation.
type Handler struct {
	generator *Generator
}

// NewHandler creates a new docs Handler.
func NewHandler(gen *Generator) *Handler {
	if gen == nil {
		gen = NewGenerator()
	}
	return &Handler{generator: gen}
}

// RegisterRoutes registers the documentation endpoints on the router.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/docs", h.handleGetDocs)
}

func (h *Handler) handleGetDocs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	targetPath := r.URL.Query().Get("path")
	workspacePath := r.URL.Query().Get("workspacePath")
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "data"
	}

	if targetPath == "" {
		targetPath = workspacePath
	}
	if targetPath == "" {
		targetPath = "."
	}

	if workspacePath != "" {
		if _, err := security.SafeAbsolute(workspacePath, targetPath); err != nil {
			h.jsonError(w, "invalid path: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else if err := security.RejectTraversal(targetPath); err != nil {
		h.jsonError(w, "invalid path: "+err.Error(), http.StatusBadRequest)
		return
	}

	if format == "data" {
		col, err := h.generator.BuildDocCollection(workspacePath, targetPath)
		if err != nil {
			h.jsonError(w, "failed to build documentation: "+err.Error(), http.StatusInternalServerError)
			return
		}
		h.jsonResponse(w, col)
		return
	}

	data, filename, err := h.generator.Generate(workspacePath, targetPath, format)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, map[string]any{
		"content":  string(data),
		"filename": filename,
		"format":   format,
	})
}

func (h *Handler) jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error": msg,
	})
}
