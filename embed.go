// Package pebblepost embeds the compiled frontend assets into the Go binary.
package pebblepost

import (
	"embed"
	"fmt"
	"io/fs"
)

//go:embed web/dist/*
var distFS embed.FS

// FrontendFS returns an fs.FS rooted at the compiled web assets (stripping "web/dist").
func FrontendFS() (fs.FS, error) {
	sub, err := fs.Sub(distFS, "web/dist")
	if err != nil {
		return nil, fmt.Errorf("embed: sub fs: %w", err)
	}
	return sub, nil
}
