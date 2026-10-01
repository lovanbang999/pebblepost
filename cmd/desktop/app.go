package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"pebblepost/internal/app"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct holds desktop application state and lifecycle handlers.
type App struct {
	ctx  context.Context
	inst *app.App
}

// NewApp creates a new App struct and initializes the PebblePost core instance.
func NewApp() *App {
	a := &App{}

	dataDir := os.Getenv("DATA_DIR")
	if dataDir == "" {
		if configDir, err := os.UserConfigDir(); err == nil {
			dataDir = filepath.Join(configDir, "pebblepost")
		} else {
			dataDir = "./data"
		}
	}

	inst, err := app.Bootstrap(dataDir)
	if err != nil {
		log.Fatalf("error bootstrap pebblepost desktop core: %v", err)
	}
	a.inst = inst

	return a
}

// Mux returns the HTTP handler for Wails AssetServer.
func (a *App) Mux() http.Handler {
	if a.inst != nil {
		return a.inst.Mux
	}
	return nil
}

// startup is called when the app starts.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

// shutdown is called at termination.
func (a *App) shutdown(ctx context.Context) {
	if a.inst != nil {
		_ = a.inst.Close()
	}
}

// SelectDirectory opens a native system folder dialog and returns the selected path.
func (a *App) SelectDirectory() (string, error) {
	return runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Select API Collection Directory",
	})
}
