package autoupdate

import (
	"encoding/json"
	"net/http"
	"strings"
)

// Handler handles HTTP requests for auto-update and settings.
type Handler struct {
	service *Service
}

// NewHandler creates a new update handler.
func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// RegisterRoutes registers update routes on the mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/update/check", h.handleCheck)
	mux.HandleFunc("/api/update/settings", h.handleSettings)
	mux.HandleFunc("/api/update/verify", h.handleVerify)
}

func (h *Handler) handleCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	force := r.URL.Query().Get("force") == "true"
	info, err := h.service.CheckForUpdate(r.Context(), force)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":   err.Error(),
			"success": false,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

func (h *Handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		settings := h.service.GetSettings()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(settings)

	case http.MethodPost, http.MethodPut:
		var incoming UpdateSettings
		if err := json.NewDecoder(r.Body).Decode(&incoming); err != nil {
			http.Error(w, "invalid json payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		if incoming.Channel == "" {
			incoming.Channel = "stable"
		}
		if err := h.service.UpdateSettings(incoming); err != nil {
			http.Error(w, "failed to save settings: "+err.Error(), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success":  true,
			"settings": incoming,
		})

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

type verifyRequest struct {
	Message   string `json:"message"`
	Signature string `json:"signature"`
	PublicKey string `json:"publicKey,omitempty"`
}

func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req verifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json payload", http.StatusBadRequest)
		return
	}

	if strings.TrimSpace(req.Message) == "" || strings.TrimSpace(req.Signature) == "" {
		http.Error(w, "missing required fields: message and signature", http.StatusBadRequest)
		return
	}

	pubKey := h.service.publicKey
	if req.PublicKey != "" {
		parsed, err := ParsePublicKey([]byte(req.PublicKey))
		if err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"valid": false,
				"error": err.Error(),
			})
			return
		}
		pubKey = parsed
	}

	if len(pubKey) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"valid": false,
			"error": "no public key provided or configured",
		})
		return
	}

	valid := VerifySignature([]byte(req.Message), []byte(req.Signature), pubKey)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"valid": valid,
	})
}
