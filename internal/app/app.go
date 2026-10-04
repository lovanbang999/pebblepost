package app

import (
	"encoding/json"
	"net/http"
	"path/filepath"

	"pebblepost/internal/autoupdate"
	"pebblepost/internal/docs"
	"pebblepost/internal/history"
	"pebblepost/internal/httpclient"
	"pebblepost/internal/runner"
	"pebblepost/internal/scripting"
	"pebblepost/internal/security"
	"pebblepost/internal/workspace"
)

const maxBodyBytes = 10 * 1024 * 1024 // 10 MB

// App manages global application services and routes.
type App struct {
	Mux            *http.ServeMux
	handler        http.Handler // Mux wrapped with security middleware
	DataDir        string
	Token          string // empty = no auth enforcement (desktop mode)
	WorkspaceSvc   *workspace.WorkspaceService
	EnvironmentSvc *workspace.EnvironmentService
	Interpolator   *workspace.Interpolator
	HttpClient     httpclient.Client
	ScriptEngine   *scripting.Engine
	HistorySvc     *history.Store
	UpdateSvc      *autoupdate.Service
}

// ServeHTTP implements http.Handler, running requests through the full
// middleware stack (security headers, CORS, body limit, token auth).
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.handler.ServeHTTP(w, r)
}

// Bootstrap initializes the application services.
func Bootstrap(dataDir string) (*App, error) {
	return BootstrapWithToken(dataDir, "")
}

// BootstrapWithToken initializes services and configures token auth.
// When token is non-empty, all API endpoints except /api/health require
// a valid Bearer token or ?token= query parameter.
func BootstrapWithToken(dataDir, token string) (*App, error) {
	mux := http.NewServeMux()
	wsSvc := workspace.NewWorkspaceService()
	envSvc := workspace.NewEnvironmentService()
	interpolator := workspace.NewInterpolator()
	client := httpclient.NewClient()
	scriptEngine := scripting.NewEngine()

	// Open history SQLite store under <dataDir>/history/
	historySvc, err := history.Open(filepath.Join(dataDir, "history"))
	if err != nil {
		return nil, err
	}

	updateSvc := autoupdate.NewService("0.2.0", dataDir)

	a := &App{
		Mux:            mux,
		DataDir:        dataDir,
		Token:          token,
		WorkspaceSvc:   wsSvc,
		EnvironmentSvc: envSvc,
		Interpolator:   interpolator,
		HttpClient:     client,
		ScriptEngine:   scriptEngine,
		HistorySvc:     historySvc,
		UpdateSvc:      updateSvc,
	}

	a.registerRoutes()

	// Wrap the raw mux with the security middleware stack.
	// Order: security headers → CORS → body limit → token auth → mux.
	a.handler = security.Chain(
		security.SecurityHeadersMiddleware,
		security.CORSMiddleware,
		security.BodyLimitMiddleware(maxBodyBytes),
		security.TokenMiddleware(token),
	)(mux)

	return a, nil
}

// Close gracefully releases application resources.
func (a *App) Close() error {
	if a.HistorySvc != nil {
		return a.HistorySvc.Close()
	}
	return nil
}

func (a *App) registerRoutes() {
	// Health check (no auth required – checked in TokenMiddleware)
	healthHandler := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": "0.2.0",
			"name":    "PebblePost",
		})
	}
	a.Mux.HandleFunc("/api/health", healthHandler)
	a.Mux.HandleFunc("/health", healthHandler)

	// Workspace & Collection endpoints
	wsHandler := workspace.NewHandler(a.WorkspaceSvc, a.EnvironmentSvc, a.Interpolator)
	wsHandler.RegisterRoutes(a.Mux)

	// HTTP Execution endpoints with Scripting sandbox
	httpHandler := httpclient.NewHandler(a.HttpClient, a.WorkspaceSvc, a.EnvironmentSvc, a.Interpolator, a.ScriptEngine, a.HistorySvc)
	httpHandler.RegisterRoutes(a.Mux)

	// History endpoints
	historyHandler := history.NewHandler(a.HistorySvc)
	historyHandler.RegisterRoutes(a.Mux)

	// Collection Runner endpoints
	runnerHandler := runner.NewHandler(runner.NewCustomRunner(a.WorkspaceSvc, a.EnvironmentSvc, a.Interpolator, a.HttpClient, a.ScriptEngine))
	runnerHandler.RegisterRoutes(a.Mux)

	// Documentation endpoints
	docsHandler := docs.NewHandler(docs.NewGenerator())
	docsHandler.RegisterRoutes(a.Mux)

	// Auto-update endpoints
	if a.UpdateSvc != nil {
		updateHandler := autoupdate.NewHandler(a.UpdateSvc)
		updateHandler.RegisterRoutes(a.Mux)
	}
}
