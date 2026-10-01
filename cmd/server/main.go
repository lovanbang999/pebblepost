package main

import (
	"log"
	"net/http"
	"os"

	"pebblepost"
	"pebblepost/internal/app"
)

func main() {
	port := envOr("PORT", "8080")
	dataDir := envOr("DATA_DIR", "./data")

	inst, err := app.Bootstrap(dataDir)
	if err != nil {
		log.Fatalf("bootstrap error: %v", err)
	}
	defer inst.Close()

	// Embedded frontend SPA routes
	frontendFS, err := pebblepost.FrontendFS()
	if err != nil {
		log.Printf("notice: embedded frontend not available: %v", err)
	} else {
		inst.Mux.Handle("/", http.FileServer(http.FS(frontendFS)))
	}

	addr := ":" + port
	log.Printf("PebblePost server listening on %s (data: %s)", addr, dataDir)
	if err := http.ListenAndServe(addr, inst.Mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
