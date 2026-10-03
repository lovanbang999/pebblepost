package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"pebblepost/internal/history"
	"pebblepost/internal/scripting"
	"pebblepost/internal/security"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

// Handler exposes HTTP API endpoints for executing HTTP requests, cookie management, and OAuth2.
type Handler struct {
	client              Client
	workspaceSvc        *workspace.WorkspaceService
	environmentSvc      *workspace.EnvironmentService
	interpolator        *workspace.Interpolator
	scriptEngine        *scripting.Engine
	inheritanceResolver *workspace.InheritanceResolver
	historySvc          *history.Store // may be nil (e.g. in tests)
	streamsMu           sync.Mutex
	activeStreams       map[string]context.CancelFunc
}

// NewHandler creates a new HTTP Client API Handler.
func NewHandler(
	client Client,
	wsSvc *workspace.WorkspaceService,
	envSvc *workspace.EnvironmentService,
	in *workspace.Interpolator,
	scriptEngine *scripting.Engine,
	historySvc *history.Store,
) *Handler {
	return &Handler{
		client:              client,
		workspaceSvc:        wsSvc,
		environmentSvc:      envSvc,
		interpolator:        in,
		scriptEngine:        scriptEngine,
		inheritanceResolver: workspace.NewInheritanceResolver(wsSvc),
		historySvc:          historySvc,
		activeStreams:       make(map[string]context.CancelFunc),
	}
}

// RegisterRoutes registers execution and auth/cookie endpoints on the provided HTTP ServeMux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/request/execute", h.handleExecute)
	mux.HandleFunc("/api/cookies", h.handleCookies)
	mux.HandleFunc("/api/oauth2/authorize", h.handleOAuth2Authorize)
	mux.HandleFunc("/api/oauth2/token", h.handleOAuth2Token)
	mux.HandleFunc("/api/grpc/reflect/services", h.handleGrpcReflectServices)
	mux.HandleFunc("/api/grpc/proto/services", h.handleGrpcProtoServices)
	mux.HandleFunc("/api/grpc/sample-message", h.handleGrpcSampleMessage)
	mux.HandleFunc("/api/grpc/stream", h.handleGrpcStream)
	mux.HandleFunc("/api/grpc/stream/cancel", h.handleGrpcStreamCancel)
}

// ExecutePayload defines the JSON body for the /api/request/execute endpoint.
type ExecutePayload struct {
	WorkspacePath   string                   `json:"workspacePath,omitempty"`
	EnvironmentName string                   `json:"environmentName,omitempty"`
	Path            string                   `json:"path,omitempty"`
	Request         *types.RequestDefinition `json:"request"`
	Overrides       map[string]string        `json:"overrides,omitempty"`
	Trusted         *bool                    `json:"trusted,omitempty"` // nil or true = allow scripts, false = block scripts
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

	if payload.WorkspacePath != "" {
		h.client.SetWorkspace(payload.WorkspacePath)
	}

	var folderChain []workspace.FolderChainItem
	if payload.Path != "" && h.inheritanceResolver != nil {
		folderChain, _ = h.inheritanceResolver.DiscoverFolderChain(payload.WorkspacePath, payload.Path)
	}

	reqToExecute := payload.Request
	varMap := make(map[string]string)
	var env *types.EnvironmentDefinition

	// 1. Build variable map and run initial interpolation
	if h.interpolator != nil {
		if h.environmentSvc != nil && payload.WorkspacePath != "" && payload.EnvironmentName != "" {
			env, _ = h.environmentSvc.GetEnvironment(payload.WorkspacePath, payload.EnvironmentName)
		}

		baseVarMap := h.interpolator.BuildVariableMap(env, nil)
		if h.inheritanceResolver != nil && len(folderChain) > 0 {
			varMap, _ = h.inheritanceResolver.MergeVariables(baseVarMap, folderChain, payload.Overrides)
		} else {
			varMap = h.interpolator.BuildVariableMap(env, payload.Overrides)
		}
	}

	// Merge headers and resolve auth if folderChain is present
	if h.inheritanceResolver != nil && len(folderChain) > 0 {
		mergedHeaders, _ := h.inheritanceResolver.MergeHeaders(folderChain, reqToExecute.Headers)
		reqToExecute.Headers = mergedHeaders
		resolvedAuth, _ := h.inheritanceResolver.ResolveAuth(folderChain, reqToExecute.Auth)
		reqToExecute.Auth = resolvedAuth
	}

	// Interpolate request fields (URL, headers, params, body, auth, settings)
	if h.interpolator != nil {
		interpolated, err := h.interpolator.InterpolateRequestWithError(reqToExecute, varMap)
		if err != nil {
			h.jsonResponse(w, types.ExecutionResult{
				StatusCode: 0,
				StatusText: "Interpolation Error",
				Error:      err.Error(),
				Logs:       []string{"Error during variable interpolation: " + err.Error()},
				Tests: []types.TestAssertionResult{
					{Name: "Variable Interpolation", Passed: false, Message: err.Error()},
				},
				ExecutedAt: time.Now(),
			})
			return
		}
		reqToExecute = interpolated
	}

	// Determine if scripts are allowed (untrusted workspace blocks script execution)
	scriptsAllowed := true
	if payload.Trusted != nil && !*payload.Trusted {
		scriptsAllowed = false
	}

	var preLogs []string

	// Build script chains
	var preScripts []workspace.ScriptChainItem
	var postScripts []workspace.ScriptChainItem
	if h.inheritanceResolver != nil && len(folderChain) > 0 {
		preScripts, postScripts = h.inheritanceResolver.ResolveScripts(folderChain, reqToExecute.Scripts)
	} else {
		if reqToExecute.Scripts.PreRequest != "" {
			preScripts = append(preScripts, workspace.ScriptChainItem{Source: "request", Script: reqToExecute.Scripts.PreRequest})
		}
		if reqToExecute.Scripts.PostResponse != "" {
			postScripts = append(postScripts, workspace.ScriptChainItem{Source: "request", Script: reqToExecute.Scripts.PostResponse})
		}
	}

	var preConsoleLogs []types.ConsoleLogEntry

	// 2. Pre-request Script Sandbox Execution (Root-to-Leaf)
	for _, s := range preScripts {
		if !scriptsAllowed {
			warnMsg := fmt.Sprintf("[WARN] Pre-request script (%s) blocked: workspace is not trusted", s.Source)
			preLogs = append(preLogs, warnMsg)
			preConsoleLogs = append(preConsoleLogs, types.ConsoleLogEntry{
				Timestamp: time.Now(),
				Level:     "warn",
				Source:    s.Source,
				Message:   "Pre-request script blocked: workspace is not trusted",
			})
			continue
		}
		if h.scriptEngine != nil {
			scriptTimeout := scriptTimeoutFor(reqToExecute.Settings.ScriptTimeoutMs)
			preResult, preErr := h.scriptEngine.ExecutePreRequestNamed(s.Source, s.Script, reqToExecute, varMap, scriptTimeout)
			if preResult != nil {
				preLogs = append(preLogs, preResult.Logs...)
				preConsoleLogs = append(preConsoleLogs, preResult.ConsoleLogs...)
				for k, v := range preResult.ExtractedEnvVars {
					varMap[k] = v
				}
				reqToExecute = preResult.Request
			}
			if preErr != nil {
				// Abort immediately with pre-request error
				h.jsonResponse(w, &types.ExecutionResult{
					Error:       fmt.Sprintf("[%s] Pre-request script error: %v", s.Source, preErr),
					Logs:        preLogs,
					ConsoleLogs: preConsoleLogs,
					Tests:       []types.TestAssertionResult{},
				})
				return
			}
		}
	}

	// 3. Execute HTTP request
	result, err := h.client.Execute(r.Context(), reqToExecute)
	if err != nil && result == nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result.Logs == nil {
		result.Logs = make([]string, 0)
	}
	if len(preLogs) > 0 {
		result.Logs = append(preLogs, result.Logs...)
	}
	if result.ConsoleLogs == nil {
		result.ConsoleLogs = make([]types.ConsoleLogEntry, 0)
	}
	if len(preConsoleLogs) > 0 {
		result.ConsoleLogs = append(preConsoleLogs, result.ConsoleLogs...)
	}

	// 4. Post-response / Test Script Sandbox Execution (Leaf-to-Root)
	for _, s := range postScripts {
		if !scriptsAllowed {
			warnMsg := fmt.Sprintf("[WARN] Post-response script (%s) blocked: workspace is not trusted", s.Source)
			result.Logs = append(result.Logs, warnMsg)
			result.ConsoleLogs = append(result.ConsoleLogs, types.ConsoleLogEntry{
				Timestamp: time.Now(),
				Level:     "warn",
				Source:    s.Source,
				Message:   "Post-response script blocked: workspace is not trusted",
			})
			continue
		}
		if h.scriptEngine != nil {
			scriptTimeout := scriptTimeoutFor(reqToExecute.Settings.ScriptTimeoutMs)
			postResult, _ := h.scriptEngine.ExecutePostResponseNamed(s.Source, s.Script, reqToExecute, result, varMap, scriptTimeout)
			if postResult != nil {
				result.Tests = append(result.Tests, postResult.Tests...)
				result.Logs = append(result.Logs, postResult.Logs...)
				result.ConsoleLogs = append(result.ConsoleLogs, postResult.ConsoleLogs...)
				if result.ExtractedEnvVars == nil {
					result.ExtractedEnvVars = make(map[string]string)
				}
				for k, v := range postResult.ExtractedEnvVars {
					varMap[k] = v
					result.ExtractedEnvVars[k] = v
				}
			}
		}
	}

	// 5. Mask secrets in logs and error before returning
	var secretValues []string
	if env != nil {
		for _, v := range env.Variables {
			if v.Secret && v.Value != "" {
				secretValues = append(secretValues, v.Value)
			}
		}
	}
	// Never print auth secrets in logs
	auth := reqToExecute.Auth
	for _, sec := range []string{auth.Token, auth.Password, auth.SecretKey, auth.ClientSecret, auth.RefreshToken} {
		if strings.TrimSpace(sec) != "" {
			secretValues = append(secretValues, sec)
		}
	}
	if strings.EqualFold(auth.Type, "apiKey") && strings.TrimSpace(auth.Value) != "" {
		secretValues = append(secretValues, auth.Value)
	}

	if len(secretValues) > 0 {
		for i, log := range result.Logs {
			result.Logs[i] = security.MaskSecrets(log, secretValues)
		}
		for i, cl := range result.ConsoleLogs {
			result.ConsoleLogs[i].Message = security.MaskSecrets(cl.Message, secretValues)
		}
		if result.Error != "" {
			result.Error = security.MaskSecrets(result.Error, secretValues)
		}
	}

	h.jsonResponse(w, result)

	// Record to history asynchronously — never block the HTTP response.
	if h.historySvc != nil && result != nil {
		recordInput := h.buildHistoryInput(payload, reqToExecute, secretValues, result)
		go func() { _ = h.historySvc.Record(recordInput) }()
	}
}

// buildHistoryInput assembles a RecordInput from the execution context.
func (h *Handler) buildHistoryInput(
	payload ExecutePayload,
	req *types.RequestDefinition,
	secretValues []string,
	result *types.ExecutionResult,
) history.RecordInput {
	name := ""
	if req != nil {
		name = req.Name
	}
	if name == "" && payload.Request != nil {
		name = payload.Request.Name
	}
	url := ""
	if req != nil {
		url = req.URL
	}
	return history.RecordInput{
		WorkspacePath: payload.WorkspacePath,
		RequestName:   name,
		Method:        req.Method,
		URL:           url,
		Result:        result,
		SecretsToMask: secretValues,
	}
}

// handleCookies handles GET, POST, DELETE for workspace cookies.
func (h *Handler) handleCookies(w http.ResponseWriter, r *http.Request) {
	wsPath := r.URL.Query().Get("workspacePath")
	jar := h.client.GetCookieJar()
	if jar == nil {
		h.jsonError(w, "Cookie jar not available", http.StatusInternalServerError)
		return
	}
	if wsPath != "" {
		jar.SetWorkspace(wsPath)
	}

	switch r.Method {
	case http.MethodGet:
		grouped := jar.GetCookiesByDomain()
		h.jsonResponse(w, grouped)

	case http.MethodPost:
		var payload struct {
			WorkspacePath string           `json:"workspacePath"`
			Cookie        types.CookieItem `json:"cookie"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			h.jsonError(w, "Invalid cookie payload: "+err.Error(), http.StatusBadRequest)
			return
		}
		if payload.WorkspacePath != "" {
			jar.SetWorkspace(payload.WorkspacePath)
		}
		if err := jar.SetCookie(payload.Cookie); err != nil {
			h.jsonError(w, err.Error(), http.StatusBadRequest)
			return
		}
		h.jsonResponse(w, map[string]any{"success": true})

	case http.MethodDelete:
		domain := r.URL.Query().Get("domain")
		path := r.URL.Query().Get("path")
		name := r.URL.Query().Get("name")

		if domain == "" {
			// Clear all cookies
			_ = jar.ClearAll()
			h.jsonResponse(w, map[string]any{"success": true, "cleared": "all"})
			return
		}

		if name != "" {
			_ = jar.DeleteCookie(domain, path, name)
			h.jsonResponse(w, map[string]any{"success": true, "deleted": name})
			return
		}

		// Clear domain
		_ = jar.ClearDomain(domain)
		h.jsonResponse(w, map[string]any{"success": true, "clearedDomain": domain})

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleOAuth2Authorize initiates Authorization Code + PKCE flow with loopback listener and browser.
func (h *Handler) handleOAuth2Authorize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Auth types.AuthDefinition `json:"auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	token, err := GetOAuth2Manager().AuthorizeCodePKCE(r.Context(), &payload.Auth)
	if err != nil {
		h.jsonError(w, "Authorization failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, token)
}

// handleOAuth2Token retrieves or refreshes an OAuth2 token without opening a browser.
func (h *Handler) handleOAuth2Token(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Auth types.AuthDefinition `json:"auth"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	token, err := GetOAuth2Manager().GetToken(r.Context(), &payload.Auth)
	if err != nil {
		h.jsonError(w, "Failed to get token: "+err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, token)
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

// scriptTimeoutFor converts the per-request ScriptTimeoutMs setting to a
// Duration. Zero or negative values fall back to the engine default (5 s).
func scriptTimeoutFor(ms int) time.Duration {
	if ms <= 0 {
		return scripting.DefaultScriptTimeout
	}
	return time.Duration(ms) * time.Millisecond
}

// ─── gRPC API Handlers ────────────────────────────────────────────────────────

func (h *Handler) handleGrpcReflectServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		Address            string `json:"address"`
		UseTLS             bool   `json:"useTls"`
		InsecureSkipVerify bool   `json:"insecureSkipVerify"`
		RootCAPath         string `json:"rootCaPath"`
		WorkspacePath      string `json:"workspacePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	if payload.Address == "" {
		h.jsonError(w, "address is required", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		h.client.SetWorkspace(payload.WorkspacePath)
	}

	grpcClient := h.client.GetGrpcClient()
	if grpcClient == nil {
		h.jsonError(w, "gRPC client unavailable", http.StatusInternalServerError)
		return
	}

	services, err := grpcClient.ReflectServices(r.Context(), payload.Address, payload.UseTLS, payload.InsecureSkipVerify, payload.RootCAPath)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusInternalServerError)
		return
	}

	h.jsonResponse(w, services)
}

func (h *Handler) handleGrpcProtoServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		ProtoFiles    []string `json:"protoFiles"`
		ImportPaths   []string `json:"importPaths"`
		WorkspacePath string   `json:"workspacePath"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(payload.ProtoFiles) == 0 {
		h.jsonError(w, "protoFiles are required", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		h.client.SetWorkspace(payload.WorkspacePath)
	}

	grpcClient := h.client.GetGrpcClient()
	if grpcClient == nil {
		h.jsonError(w, "gRPC client unavailable", http.StatusInternalServerError)
		return
	}

	services, err := grpcClient.LoadProtoServices(payload.ProtoFiles, payload.ImportPaths)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, services)
}

func (h *Handler) handleGrpcSampleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		WorkspacePath string                `json:"workspacePath"`
		Grpc          *types.GrpcDefinition `json:"grpc"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	if payload.Grpc == nil {
		h.jsonError(w, "grpc definition is required", http.StatusBadRequest)
		return
	}

	if payload.WorkspacePath != "" {
		h.client.SetWorkspace(payload.WorkspacePath)
	}

	grpcClient := h.client.GetGrpcClient()
	if grpcClient == nil {
		h.jsonError(w, "gRPC client unavailable", http.StatusInternalServerError)
		return
	}

	sample, err := grpcClient.GenerateSampleMessage(r.Context(), payload.Grpc)
	if err != nil {
		h.jsonError(w, err.Error(), http.StatusBadRequest)
		return
	}

	h.jsonResponse(w, map[string]string{"sample": sample})
}

func (h *Handler) handleGrpcStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		h.jsonError(w, "Streaming unsupported by response writer", http.StatusInternalServerError)
		return
	}

	var payload struct {
		StreamID        string                   `json:"streamId"`
		WorkspacePath   string                   `json:"workspacePath,omitempty"`
		EnvironmentName string                   `json:"environmentName,omitempty"`
		Path            string                   `json:"path,omitempty"`
		Request         *types.RequestDefinition `json:"request"`
		Overrides       map[string]string        `json:"overrides,omitempty"`
		Trusted         *bool                    `json:"trusted,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}
	if payload.Request == nil || payload.Request.Grpc == nil {
		h.jsonError(w, "Request definition with grpc is required", http.StatusBadRequest)
		return
	}

	streamID := payload.StreamID
	if streamID == "" {
		streamID = fmt.Sprintf("stream_%d", time.Now().UnixNano())
	}

	ctx, cancel := context.WithCancel(r.Context())
	h.streamsMu.Lock()
	h.activeStreams[streamID] = cancel
	h.streamsMu.Unlock()
	defer func() {
		h.streamsMu.Lock()
		delete(h.activeStreams, streamID)
		h.streamsMu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if payload.WorkspacePath != "" {
		h.client.SetWorkspace(payload.WorkspacePath)
	}

	reqToExecute := payload.Request
	varMap := make(map[string]string)
	if h.interpolator != nil {
		var env *types.EnvironmentDefinition
		if h.environmentSvc != nil && payload.WorkspacePath != "" && payload.EnvironmentName != "" {
			env, _ = h.environmentSvc.GetEnvironment(payload.WorkspacePath, payload.EnvironmentName)
		}
		varMap = h.interpolator.BuildVariableMap(env, payload.Overrides)
		interpolated, err := h.interpolator.InterpolateRequestWithError(reqToExecute, varMap)
		if err == nil {
			reqToExecute = interpolated
		}
	}

	onMessage := func(msg types.GrpcStreamMessage) {
		b, err := json.Marshal(msg)
		if err == nil {
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", b)
			flusher.Flush()
		}
	}

	grpcClient := h.client.GetGrpcClient()
	result, execErr := grpcClient.ExecuteStream(ctx, reqToExecute, onMessage)
	if execErr != nil && result == nil {
		result = &types.ExecutionResult{
			StatusCode: 500,
			StatusText: "Execution Error",
			Error:      execErr.Error(),
			ExecutedAt: time.Now(),
		}
	}

	// Post-response scripts & assertions
	if h.scriptEngine != nil && reqToExecute.Scripts.PostResponse != "" {
		scriptTimeout := scriptTimeoutFor(reqToExecute.Settings.ScriptTimeoutMs)
		postResult, _ := h.scriptEngine.ExecutePostResponseNamed("request", reqToExecute.Scripts.PostResponse, reqToExecute, result, varMap, scriptTimeout)
		if postResult != nil {
			result.Tests = append(result.Tests, postResult.Tests...)
			result.Logs = append(result.Logs, postResult.Logs...)
			result.ConsoleLogs = append(result.ConsoleLogs, postResult.ConsoleLogs...)
		}
	}

	// Save history if available
	if h.historySvc != nil && payload.WorkspacePath != "" && result != nil {
		historyPayload := ExecutePayload{
			WorkspacePath:   payload.WorkspacePath,
			EnvironmentName: payload.EnvironmentName,
			Path:            payload.Path,
			Request:         reqToExecute,
		}
		recordInput := h.buildHistoryInput(historyPayload, reqToExecute, nil, result)
		go func() { _ = h.historySvc.Record(recordInput) }()
	}

	doneBytes, _ := json.Marshal(result)
	fmt.Fprintf(w, "event: done\ndata: %s\n\n", doneBytes)
	flusher.Flush()
}

func (h *Handler) handleGrpcStreamCancel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var payload struct {
		StreamID string `json:"streamId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		h.jsonError(w, "Invalid payload: "+err.Error(), http.StatusBadRequest)
		return
	}

	h.streamsMu.Lock()
	cancel, ok := h.activeStreams[payload.StreamID]
	if ok {
		cancel()
		delete(h.activeStreams, payload.StreamID)
	}
	h.streamsMu.Unlock()

	h.jsonResponse(w, map[string]any{"cancelled": ok})
}
