package main

import (
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"pebblepost"
	"pebblepost/internal/app"
)

var (
	version = "0.2.0"
	commit  = "none"
	date    = "unknown"
	builtBy = "source"
)

func main() {
	showVersion := flag.Bool("version", false, "Show server version")
	host := flag.String("host", envOr("PEBBLEPOST_HOST", envOr("PEBBLE_HOST", "127.0.0.1")),
		"Interface to listen on. Use 0.0.0.0 to expose to the network (requires --token or PEBBLEPOST_TOKEN).")
	port := flag.String("port", envOr("PORT", "8080"), "Port to listen on.")
	tokenEnv := envOr("PEBBLEPOST_TOKEN", os.Getenv("PEBBLE_TOKEN"))
	token := flag.String("token", tokenEnv,
		"Bearer token for API authentication. Generated automatically in loopback mode.")
	dataDir := flag.String("data-dir", envOr("PEBBLEPOST_DATA_DIR", envOr("DATA_DIR", "./data")), "Data directory.")
	flag.Parse()

	if *showVersion {
		fmt.Printf("PebblePost Server v%s (commit: %s, date: %s, built by: %s)\n", version, commit, date, builtBy)
		os.Exit(0)
	}

	authToken := *token

	isLoopback := *host == "127.0.0.1" || *host == "::1" || *host == "localhost"

	if !isLoopback {
		// Binding to a network interface is dangerous without authentication.
		if authToken == "" {
			log.Fatal("ERROR: --token or PEBBLEPOST_TOKEN (or PEBBLE_TOKEN) is required when --host is not 127.0.0.1.\n" +
				"Generate one with: openssl rand -hex 16")
		}
	}

	// Always use a token, even in loopback mode (generated if not supplied).
	if authToken == "" {
		authToken = generateToken()
	}

	tokenFilePath := filepath.Join(*dataDir, ".token")
	_ = os.MkdirAll(*dataDir, 0700)
	if err := os.WriteFile(tokenFilePath, []byte(authToken), 0600); err != nil {
		log.Printf("warning: failed to write token file: %v", err)
	}

	maskedToken := authToken
	if len(authToken) > 8 {
		maskedToken = authToken[:4] + "..." + authToken[len(authToken)-4:]
	}
	log.Printf("PebblePost API token: %s (saved to %s)", maskedToken, tokenFilePath)

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
