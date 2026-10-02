package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"

	"pebblepost"
	"pebblepost/internal/app"
)

func main() {
	host := flag.String("host", envOr("PEBBLE_HOST", "127.0.0.1"),
		"Interface to listen on. Use 0.0.0.0 to expose to the network (requires --token or PEBBLE_TOKEN).")
	port := flag.String("port", envOr("PORT", "8080"), "Port to listen on.")
	token := flag.String("token", os.Getenv("PEBBLE_TOKEN"),
		"Bearer token for API authentication. Generated automatically in loopback mode.")
	dataDir := flag.String("data-dir", envOr("DATA_DIR", "./data"), "Data directory.")
	flag.Parse()

	authToken := *token

	isLoopback := *host == "127.0.0.1" || *host == "::1" || *host == "localhost"

	if !isLoopback {
		// Binding to a network interface is dangerous without authentication.
		if authToken == "" {
			log.Fatal("ERROR: --token or PEBBLE_TOKEN is required when --host is not 127.0.0.1.\n" +
				"Generate one with: openssl rand -hex 16")
		}
	}

	// Always use a token, even in loopback mode (generated if not supplied).
	if authToken == "" {
		authToken = generateToken()
	}

	log.Printf("PebblePost API token: %s", authToken)

	inst, err := app.BootstrapWithToken(*dataDir, authToken)
	if err != nil {
		log.Fatalf("bootstrap error: %v", err)
	}
	defer inst.Close()

	// Embedded frontend SPA.
	frontendFS, err := pebblepost.FrontendFS()
	if err != nil {
		log.Printf("notice: embedded frontend not available: %v", err)
	} else {
		inst.Mux.Handle("/", http.FileServer(http.FS(frontendFS)))
	}

	addr := fmt.Sprintf("%s:%s", *host, *port)
	log.Printf("PebblePost server listening on %s (data: %s)", addr, *dataDir)

	if err := http.ListenAndServe(addr, inst); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func generateToken() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		log.Fatalf("failed to generate token: %v", err)
	}
	return hex.EncodeToString(b)
}

func envOr(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
