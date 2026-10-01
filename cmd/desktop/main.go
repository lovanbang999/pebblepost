package main

import (
	"context"
	"log"
	"time"

	"pebblepost"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

func main() {
	app := NewApp()

	frontendFS, err := pebblepost.FrontendFS()
	if err != nil {
		log.Printf("warning: frontend assets not loaded: %v", err)
	}

	err = wails.Run(&options.App{
		Title:            "PebblePost Studio",
		Width:            1366,
		Height:           850,
		MinWidth:         1024,
		MinHeight:        640,
		WindowStartState: options.Maximised,
		Frameless:        true,
		StartHidden:      true,
		OnDomReady: func(ctx context.Context) {
			// Ensure compositor has flushed the dark theme frame
			time.Sleep(120 * time.Millisecond)
			runtime.WindowShow(ctx)
		},
		AssetServer: &assetserver.Options{
			Assets:  frontendFS,
			Handler: app.Mux(),
		},
		BackgroundColour: &options.RGBA{R: 9, G: 9, B: 11, A: 255}, // Zinc-950 dark theme
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []interface{}{
			app,
		},
		Linux: &linux.Options{
			Icon:                nil,
			WindowIsTranslucent: false,
			WebviewGpuPolicy:    linux.WebviewGpuPolicyOnDemand,
			ProgramName:         "pebblepost",
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
			DisableWindowIcon:    false,
		},
	})

	if err != nil {
		log.Fatalf("Error running desktop app: %v", err)
	}
}
