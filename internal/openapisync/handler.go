package openapisync

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// Handler provides HTTP endpoints for OpenAPI spec synchronization and drift management.
type Handler struct {
	wsService *workspace.WorkspaceService
}

// NewHandler initializes a new OpenAPI sync HTTP handler.
func NewHandler(wsSvc *workspace.WorkspaceService) *Handler {
	if wsSvc == nil {
		wsSvc = workspace.NewWorkspaceService()
	}
	return &Handler{
		wsService: wsSvc,
	}
}

// RegisterRoutes registers the OpenAPI sync endpoints onto the given ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/sync/diff", h.handleDiff)
	mux.HandleFunc("/api/sync/apply", h.handleApply)
	mux.HandleFunc("/api/sync/link", h.handleLink)
}

func (h *Handler) handleDiff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderPath := strings.TrimSpace(r.URL.Query().Get("folder"))
	if folderPath == "" {
		http.Error(w, "Query parameter 'folder' is required", http.StatusBadRequest)
		return
	}

	// Validate path traversal
	if strings.Contains(folderPath, "..") {
		http.Error(w, "Invalid folder path traversal detected", http.StatusBadRequest)
		return
	}

	specLocation := strings.TrimSpace(r.URL.Query().Get("spec"))
	if specLocation == "" {
		// Read from folder configuration
		folderDef, err := h.wsService.ReadFolder(folderPath)
		if err == nil && folderDef != nil && folderDef.OpenAPISync != nil {
			specLocation = folderDef.OpenAPISync.SpecLocation
		}
	}

	if specLocation == "" {
		http.Error(w, "No OpenAPI specification linked to this folder", http.StatusBadRequest)
		return
	}

	specData, specHash, err := LoadSpec(specLocation, folderPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load specification: %v", err), http.StatusBadRequest)
		return
	}

	spec, err := ParseSpec(specData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse specification: %v", err), http.StatusBadRequest)
		return
	}

	report, err := CompareSpecAndFolder(folderPath, spec, specHash, specLocation)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to calculate diff: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(report)
}

func (h *Handler) handleApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	folderPath := strings.TrimSpace(req.FolderPath)
	if folderPath == "" {
		http.Error(w, "Field 'folderPath' is required", http.StatusBadRequest)
		return
	}
	if strings.Contains(folderPath, "..") {
		http.Error(w, "Invalid folder path", http.StatusBadRequest)
		return
	}

	specLocation := strings.TrimSpace(req.SpecLocation)
	if specLocation == "" {
		folderDef, err := h.wsService.ReadFolder(folderPath)
		if err == nil && folderDef != nil && folderDef.OpenAPISync != nil {
			specLocation = folderDef.OpenAPISync.SpecLocation
		}
	}

	if specLocation == "" {
		http.Error(w, "No OpenAPI specification linked to this folder", http.StatusBadRequest)
		return
	}

	specData, specHash, err := LoadSpec(specLocation, folderPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load specification: %v", err), http.StatusBadRequest)
		return
	}

	spec, err := ParseSpec(specData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to parse specification: %v", err), http.StatusBadRequest)
		return
	}

	report, err := CompareSpecAndFolder(folderPath, spec, specHash, specLocation)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to calculate diff: %v", err), http.StatusInternalServerError)
		return
	}

	result, err := ApplyDiff(h.wsService, folderPath, report, req.AcceptedEndpoints, req.Force)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to apply diff: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}

func (h *Handler) handleLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req LinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON payload", http.StatusBadRequest)
		return
	}

	folderPath := strings.TrimSpace(req.FolderPath)
	if folderPath == "" {
		http.Error(w, "Field 'folderPath' is required", http.StatusBadRequest)
		return
	}
	specLocation := strings.TrimSpace(req.SpecLocation)
	if specLocation == "" {
		http.Error(w, "Field 'specLocation' is required", http.StatusBadRequest)
		return
	}

	specData, specHash, err := LoadSpec(specLocation, folderPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load specification: %v", err), http.StatusBadRequest)
		return
	}

	_, err = ParseSpec(specData)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid OpenAPI 3.x specification: %v", err), http.StatusBadRequest)
		return
	}

	folderDef, err := h.wsService.ReadFolder(folderPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read folder definition: %v", err), http.StatusInternalServerError)
		return
	}

	if folderDef.OpenAPISync == nil {
		folderDef.OpenAPISync = &types.OpenAPISyncConfig{}
	}
	folderDef.OpenAPISync.SpecLocation = specLocation
	folderDef.OpenAPISync.SpecHash = specHash
	folderDef.OpenAPISync.LastSyncedAt = time.Now().UTC().Format(time.RFC3339)

	if err := h.wsService.SaveFolder(folderPath, folderDef); err != nil {
		http.Error(w, fmt.Sprintf("Failed to save folder definition: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":      true,
		"specLocation": specLocation,
		"specHash":     specHash,
		"lastSyncedAt": folderDef.OpenAPISync.LastSyncedAt,
		"folder":       folderDef,
	})
}
