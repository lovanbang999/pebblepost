package history

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// Handler exposes the history REST API.
type Handler struct {
	store *Store
}

// NewHandler creates a new history HTTP handler.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers history endpoints on the provided mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/history", h.handleList)
	mux.HandleFunc("/api/history/", h.handleEntry)
}

// GET /api/history?workspace=...&page=...&limit=...&status=...&search=...
// DELETE /api/history?workspace=... (clear all for workspace)
func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	ws := r.URL.Query().Get("workspace")
	if ws == "" {
		jsonError(w, "workspace parameter required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		f := ListFilter{
			WorkspacePath: ws,
			Search:        r.URL.Query().Get("search"),
			Method:        r.URL.Query().Get("method"),
		}
		if s := r.URL.Query().Get("status"); s != "" {
			f.StatusCode, _ = strconv.Atoi(s)
		}
		if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
			if t, err := time.Parse(time.RFC3339, sinceStr); err == nil {
				f.Since = t
			}
		}
		if p := r.URL.Query().Get("page"); p != "" {
			f.Page, _ = strconv.Atoi(p)
		}
		if l := r.URL.Query().Get("limit"); l != "" {
			f.Limit, _ = strconv.Atoi(l)
		}

		entries, total, err := h.store.List(f)
		if err != nil {
			jsonError(w, "list failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		if entries == nil {
			entries = []HistoryEntry{}
		}
		jsonOK(w, map[string]any{
			"entries": entries,
			"total":   total,
			"page":    f.Page,
			"limit":   f.Limit,
		})

	case http.MethodDelete:
		if err := h.store.Clear(ws); err != nil {
			jsonError(w, "clear failed: "+err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]string{"status": "cleared"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// GET /api/history/{id}
// DELETE /api/history/{id}
func (h *Handler) handleEntry(w http.ResponseWriter, r *http.Request) {
	// Parse id from path  e.g. /api/history/42
	idStr := r.URL.Path[len("/api/history/"):]
	if idStr == "" {
		http.NotFound(w, r)
		return
	}
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		jsonError(w, "invalid id", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodGet:
		entry, err := h.store.Get(id)
		if err != nil {
			jsonError(w, err.Error(), http.StatusNotFound)
			return
		}
		jsonOK(w, entry)

	case http.MethodDelete:
		if err := h.store.Delete(id); err != nil {
			jsonError(w, err.Error(), http.StatusInternalServerError)
			return
		}
		jsonOK(w, map[string]string{"status": "deleted"})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func jsonOK(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}

func jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
