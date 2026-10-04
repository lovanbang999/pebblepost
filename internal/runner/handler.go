package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

// Handler exposes HTTP API endpoints for collection runner execution, stopping, and data parsing.
type Handler struct {
	runner       *Runner
	activeRunsMu sync.Mutex
	activeRuns   map[string]context.CancelFunc
}

// NewHandler creates a new collection runner HTTP Handler.
func NewHandler(r *Runner) *Handler {
	if r == nil {
		r = NewRunner()
	}
	return &Handler{
		runner:     r,
		activeRuns: make(map[string]context.CancelFunc),
	}
}

// RegisterRoutes registers runner endpoints on the provided HTTP ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/runner/run", h.handleRun)
	mux.HandleFunc("/api/runner/stop", h.handleStop)
	mux.HandleFunc("/api/runner/parse-data", h.handleParseData)
}

// ParseDataPayload represents data parsing input.
type ParseDataPayload struct {
	Filename string `json:"filename"`
	Content  string `json:"content"`
}

func (h *Handler) handleParseData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	contentType := r.Header.Get("Content-Type")
	var filename string
	var dataBytes []byte

	if strings.HasPrefix(contentType, "multipart/form-data") {
		err := r.ParseMultipartForm(10 << 20) // 10MB limit
		if err != nil {
			h.jsonError(w, "Failed to parse multipart form: "+err.Error(), http.StatusBadRequest)
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			h.jsonError(w, "File upload missing: "+err.Error(), http.StatusBadRequest)
			return
		}
		defer func() { _ = file.Close() }()
		filename = header.Filename
		dataBytes, err = io.ReadAll(file)
		if err != nil {
			h.jsonError(w, "Failed to read uploaded file: "+err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		var payload ParseDataPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			h.jsonError(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		filename = payload.Filename
		dataBytes = []byte(payload.Content)
	}

	if len(dataBytes) == 0 {
		h.jsonError(w, "Data content cannot be empty", http.StatusBadRequest)
		return
	}

	rows, err := ParseDataBytes(dataBytes, filename)
	if err != nil {
		h.jsonError(w, "Failed to parse data: "+err.Error(), http.StatusBadRequest)
		return
	}

	var columns []string
	if len(rows) > 0 {
		seen := make(map[string]bool)
		for k := range rows[0] {
			if !seen[k] {
				seen[k] = true
				columns = append(columns, k)
			}
		}
		sort.Strings(columns)
	}

	h.jsonResponse(w, map[string]any{
		"rows":      rows,
		"totalRows": len(rows),
		"columns":   columns,
	})
}

// RunPayload defines the options for a collection run through the API.
type RunPayload struct {
	RunID           string            `json:"runId,omitempty"`
	WorkspacePath   string            `json:"workspacePath,omitempty"`
	TargetPath      string            `json:"targetPath,omitempty"`
	RequestPaths    []string          `json:"requestPaths,omitempty"`
	EnvironmentName string            `json:"environmentName,omitempty"`
	Iterations      int               `json:"iterations,omitempty"`
	DelayMs         int               `json:"delayMs,omitempty"`
	Bail            bool              `json:"bail,omitempty"`
	DryRun          bool              `json:"dryRun,omitempty"`
	DataFile        string            `json:"dataFile,omitempty"`
	DataRows        []map[string]any  `json:"dataRows,omitempty"`
	ExtraVars       map[string]string `json:"extraVars,omitempty"`
	Stream          bool              `json:"stream,omitempty"`
}

func (h *Handler) handleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload RunPayload
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid JSON payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	runID := payload.RunID
	if runID == "" {
		runID = fmt.Sprintf("run_%d", time.Now().UnixNano())
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	h.activeRunsMu.Lock()
	h.activeRuns[runID] = cancel
	h.activeRunsMu.Unlock()

	defer func() {
		h.activeRunsMu.Lock()
		delete(h.activeRuns, runID)
		h.activeRunsMu.Unlock()
	}()

	isSSE := payload.Stream || strings.Contains(r.Header.Get("Accept"), "text/event-stream")

	var sseRep *sseReporter
	var customReporters []ReporterConfig
	if isSSE {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			h.jsonError(w, "Streaming unsupported", http.StatusInternalServerError)
			return
		}
		flusher.Flush()

		sseRep = &sseReporter{w: w, f: flusher}
	}

	opts := RunOptions{
		TargetPath:      payload.TargetPath,
		RequestPaths:    payload.RequestPaths,
		EnvironmentName: payload.EnvironmentName,
		Iterations:      payload.Iterations,
		DelayMs:         payload.DelayMs,
		Bail:            payload.Bail,
		DryRun:          payload.DryRun,
		DataFile:        payload.DataFile,
		DataRows:        payload.DataRows,
		ExtraVars:       payload.ExtraVars,
		Reporters:       customReporters,
	}

	if payload.WorkspacePath != "" && h.runner.client != nil {
		h.runner.client.SetWorkspace(payload.WorkspacePath)
	}

	if sseRep != nil {
		opts.Reporters = nil
		opts.Writer = io.Discard
		opts.CustomReporter = sseRep

		summary, err := h.runner.Run(ctx, opts)
		if err != nil {
			sseRep.emit("error", map[string]any{"error": err.Error()})
		}
		sseRep.emit("complete", summary)
		return
	}

	// Synchronous execution returning JSON summary
	summary, err := h.runner.Run(ctx, opts)
	if err != nil {
		h.jsonError(w, "Execution failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, summary)
}

func (h *Handler) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		RunID string `json:"runId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	if payload.RunID == "" {
		h.jsonError(w, "runId is required", http.StatusBadRequest)
		return
	}

	h.activeRunsMu.Lock()
	cancel, ok := h.activeRuns[payload.RunID]
	if ok {
		cancel()
		delete(h.activeRuns, payload.RunID)
	}
	h.activeRunsMu.Unlock()

	h.jsonResponse(w, map[string]any{"stopped": ok})
}

func (h *Handler) jsonResponse(w http.ResponseWriter, data any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(data)
}

func (h *Handler) jsonError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]any{"error": msg})
}

// sseReporter sends real-time run progress as Server-Sent Events.
type sseReporter struct {
	w  io.Writer
	f  http.Flusher
	mu sync.Mutex
}

func (s *sseReporter) emit(eventType string, data any) {
	s.mu.Lock()
	defer s.mu.Unlock()

	payload, err := json.Marshal(data)
	if err != nil {
		return
	}
	_, _ = fmt.Fprintf(s.w, "event: %s\ndata: %s\n\n", eventType, string(payload))
	if s.f != nil {
		s.f.Flush()
	}
}

func (s *sseReporter) PrintHeader(target, env string, count int, bail, dryRun bool) {
	s.emit("start", map[string]any{
		"target":       target,
		"environment":  env,
		"requestCount": count,
		"bail":         bail,
		"dryRun":       dryRun,
	})
}

func (s *sseReporter) PrintRequestResult(res RequestRunResult) {
	s.emit("request_result", res)
}

func (s *sseReporter) PrintSummary(summary *RunSummary) {
	s.emit("summary", summary)
}

func (s *sseReporter) PrintWarning(msg string) {
	s.emit("warning", map[string]any{"message": msg})
}
