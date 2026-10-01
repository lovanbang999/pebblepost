package app

import (
	"encoding/json"
	"net/http"
)

// App manages global application services and routes.
type App struct {
	Mux     *http.ServeMux
	DataDir string
}

// Bootstrap initializes the application services.
func Bootstrap(dataDir string) (*App, error) {
	mux := http.NewServeMux()
	a := &App{
		Mux:     mux,
		DataDir: dataDir,
	}

	a.registerRoutes()
	return a, nil
}

// Close gracefully releases application resources.
func (a *App) Close() error {
	return nil
}

func (a *App) registerRoutes() {
	a.Mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":  "ok",
			"version": "0.1.0",
			"name":    "PebblePost",
		})
	})
}
