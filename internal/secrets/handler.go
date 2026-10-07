package secrets

import (
	"encoding/json"
	"net/http"
	"strings"

	"pebblepost/internal/types"
)

// Handler exposes HTTP endpoints for managing the workspace secret backend
// configuration.
//
// Routes:
//
//	GET  /api/workspace/secret-backend          – returns current SecretBackendConfig
//	PUT  /api/workspace/secret-backend          – updates the config
//	POST /api/workspace/secret-backend/migrate  – migrate an env to another backend
type Handler struct {
	getWorkspaceConfig  func() (*types.WorkspaceDefinition, error)
	saveWorkspaceConfig func(*types.WorkspaceDefinition) error
	envDir              string
	workspaceName       string
}

// NewHandler creates a new secrets Handler.
func NewHandler(
	getWorkspaceConfig func() (*types.WorkspaceDefinition, error),
	saveWorkspaceConfig func(*types.WorkspaceDefinition) error,
	envDir string,
	workspaceName string,
) *Handler {
	return &Handler{
		getWorkspaceConfig:  getWorkspaceConfig,
		saveWorkspaceConfig: saveWorkspaceConfig,
		envDir:              envDir,
		workspaceName:       workspaceName,
	}
}

// RegisterRoutes registers the handler's routes on mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/workspace/secret-backend", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			h.getBackend(w, r)
		case http.MethodPut:
			h.setBackend(w, r)
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/workspace/secret-backend/migrate", h.migrate)
}

func (h *Handler) getBackend(w http.ResponseWriter, _ *http.Request) {
	def, err := h.getWorkspaceConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(def.SecretBackend)
}

func (h *Handler) setBackend(w http.ResponseWriter, r *http.Request) {
	var cfg types.SecretBackendConfig
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		http.Error(w, "invalid request body: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Validate backend values.
	valid := map[string]bool{"file": true, "keychain": true, "env": true, "": true}
	if !valid[cfg.Default] {
		http.Error(w, "invalid backend: "+cfg.Default, http.StatusBadRequest)
		return
	}
	for env, bt := range cfg.PerEnv {
		if !valid[bt] {
			http.Error(w, "invalid backend for env "+env+": "+bt, http.StatusBadRequest)
			return
		}
	}

	def, err := h.getWorkspaceConfig()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	def.SecretBackend = cfg
	if err := h.saveWorkspaceConfig(def); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(cfg)
}

type migrateRequest struct {
	EnvName string `json:"envName"`
	To      string `json:"to"` // "keychain" or "file"
}

type migrateResponse struct {
	BackupPath string `json:"backupPath,omitempty"`
	Message    string `json:"message"`
}

func (h *Handler) migrate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req migrateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request: "+err.Error(), http.StatusBadRequest)
		return
	}
	req.EnvName = strings.TrimSpace(req.EnvName)
	if req.EnvName == "" {
		http.Error(w, "envName is required", http.StatusBadRequest)
		return
	}

	src := NewFileStore(h.envDir)
	var dst SecretStore
	switch SecretBackendType(req.To) {
	case BackendKeychain:
		dst = NewKeychainStore(h.workspaceName)
	case BackendFile:
		dst = NewFileStore(h.envDir)
	default:
		http.Error(w, "unsupported target backend: "+req.To, http.StatusBadRequest)
		return
	}

	opts := MigrateOptions{
		Confirm: func(_ MigrationPlan) bool { return true }, // UI confirmed before calling
	}
	backupPath, err := Migrate(req.EnvName, src, dst, opts)
	if err != nil {
		http.Error(w, "migration failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(migrateResponse{
		BackupPath: backupPath,
		Message:    "migration complete",
	})
}
