package app

import (
	"encoding/json"
	"net/http"

	"pebblepost/internal/httpclient"
	"pebblepost/internal/scripting"
	"pebblepost/internal/workspace"
)

// App manages global application services and routes.
type App struct {
	Mux            *http.ServeMux
	DataDir        string
	WorkspaceSvc   *workspace.WorkspaceService
	EnvironmentSvc *workspace.EnvironmentService
	Interpolator   *workspace.Interpolator
	HttpClient     httpclient.Client
	ScriptEngine   *scripting.Engine
}

// Bootstrap initializes the application services.
func Bootstrap(dataDir string) (*App, error) {
	mux := http.NewServeMux()
	wsSvc := workspace.NewWorkspaceService()
	envSvc := workspace.NewEnvironmentService()
	interpolator := workspace.NewInterpolator()
	client := httpclient.NewClient()
	scriptEngine := scripting.NewEngine()

	a := &App{
		Mux:            mux,
		DataDir:        dataDir,
		WorkspaceSvc:   wsSvc,
		EnvironmentSvc: envSvc,
		Interpolator:   interpolator,
		HttpClient:     client,
		ScriptEngine:   scriptEngine,
	}

	a.registerRoutes()
	return a, nil
}

// Close gracefully releases application resources.
func (a *App) Close() error {
	return nil
}

func (a *App) registerRoutes() {
	// Health check
	a.Mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": "0.1.0",
			"name":    "PebblePost",
		})
	})

	// Workspace & Collection endpoints
	wsHandler := workspace.NewHandler(a.WorkspaceSvc, a.EnvironmentSvc, a.Interpolator)
	wsHandler.RegisterRoutes(a.Mux)

	// HTTP Execution endpoints with Scripting sandbox
	httpHandler := httpclient.NewHandler(a.HttpClient, a.WorkspaceSvc, a.EnvironmentSvc, a.Interpolator, a.ScriptEngine)
	httpHandler.RegisterRoutes(a.Mux)
}


