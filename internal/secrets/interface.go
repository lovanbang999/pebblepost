// Package secrets provides a pluggable storage abstraction for sensitive
// environment variables. Callers interact exclusively with the SecretStore
// interface; the concrete backend is selected at startup from the workspace
// configuration.
package secrets

import "errors"

// ErrNotFound is returned when a requested secret does not exist in the store.
var ErrNotFound = errors.New("secret not found")

// ErrReadOnly is returned when a write is attempted on a read-only backend (e.g. env).
var ErrReadOnly = errors.New("secret store is read-only")

// ErrNotSupported is returned when the backend is unavailable on the current platform
// (e.g. keychain on a headless environment without an active Secret Service daemon).
var ErrNotSupported = errors.New("secret backend not supported on this platform")

// SecretBackendType identifies a concrete backend implementation.
type SecretBackendType string

const (
	// BackendFile stores secrets in *.secret.env.json files (default, current behaviour).
	BackendFile SecretBackendType = "file"
	// BackendKeychain stores secrets in the OS keychain (Linux Secret Service,
	// macOS Keychain, Windows Credential Manager).
	BackendKeychain SecretBackendType = "keychain"
	// BackendEnv resolves ${OS_ENV:VAR} references from the process environment.
	// This backend is read-only; Set/Delete return ErrReadOnly.
	BackendEnv SecretBackendType = "env"
)

// SecretStore is the primary abstraction over pluggable secret storage.
// All operations are scoped to a (service, key) pair where service is the
// environment name and key is the variable key.
type SecretStore interface {
	// Get retrieves the plaintext value of a secret.
	// Returns ErrNotFound if the key does not exist.
	Get(service, key string) (string, error)

	// Set stores or replaces a secret value.
	// Returns ErrReadOnly on read-only backends.
	Set(service, key, value string) error

	// Delete removes a secret.
	// Returns ErrReadOnly on read-only backends.
	// Returns ErrNotFound if the key does not exist.
	Delete(service, key string) error

	// List returns all known keys stored under the given service name.
	List(service string) ([]string, error)

	// Backend reports the type of the underlying implementation.
	Backend() SecretBackendType
}

// SecretBackendConfig describes the backend selection stored in .pebble/config.json.
// Default applies to all environments unless overridden by PerEnv.
type SecretBackendConfig struct {
	// Default backend used for all environments (default: "file").
	Default SecretBackendType `json:"default,omitempty"`
	// PerEnv overrides the default for specific environment names.
	PerEnv map[string]SecretBackendType `json:"perEnv,omitempty"`
}

// BackendFor returns the effective backend type for the given environment name.
func (c SecretBackendConfig) BackendFor(envName string) SecretBackendType {
	if c.PerEnv != nil {
		if t, ok := c.PerEnv[envName]; ok && t != "" {
			return t
		}
	}
	if c.Default == "" {
		return BackendFile
	}
	return c.Default
}

// FromTypesConfig converts a types.SecretBackendConfig (string-based, JSON DTO) into
// a strongly-typed secrets.SecretBackendConfig.
// Accepts any string that maps to a known SecretBackendType; unknown values are treated
// as BackendFile (safe default).
func FromTypesConfig(def string, perEnv map[string]string) SecretBackendConfig {
	cfg := SecretBackendConfig{
		Default: SecretBackendType(def),
		PerEnv:  make(map[string]SecretBackendType, len(perEnv)),
	}
	for k, v := range perEnv {
		cfg.PerEnv[k] = SecretBackendType(v)
	}
	return cfg
}
