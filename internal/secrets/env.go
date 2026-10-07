package secrets

import (
	"fmt"
	"os"
	"strings"
)

// OSEnvPrefix is the interpolation prefix for OS environment variable references.
// Usage in variable values: ${OS_ENV:DATABASE_URL}
const OSEnvPrefix = "OS_ENV:"

// EnvStore implements SecretStore by reading values from the process environment.
// It is read-only: Set and Delete return ErrReadOnly.
//
// This backend is useful in CI where secrets are injected as environment
// variables rather than stored in files or the OS keychain.
//
// Variables are referenced in pebblepost values with the syntax:
//
//	${OS_ENV:MY_VAR_NAME}
//
// The prefix is handled by the interpolator (see workspace.BuildVariableMap),
// not by SecretStore.Get directly. The EnvStore.Get method accepts the bare
// OS environment variable name (without the "OS_ENV:" prefix).
type EnvStore struct{}

// NewEnvStore returns a new read-only EnvStore.
func NewEnvStore() *EnvStore { return &EnvStore{} }

func (s *EnvStore) Backend() SecretBackendType { return BackendEnv }

// Get retrieves the value of the OS environment variable named `key`.
// The `service` argument is ignored; all lookups are global.
func (s *EnvStore) Get(_, key string) (string, error) {
	val, ok := os.LookupEnv(key)
	if !ok {
		return "", fmt.Errorf("%w: OS env %q is not set", ErrNotFound, key)
	}
	return val, nil
}

func (s *EnvStore) Set(_, _, _ string) error        { return ErrReadOnly }
func (s *EnvStore) Delete(_, _ string) error        { return ErrReadOnly }
func (s *EnvStore) List(_ string) ([]string, error) { return nil, ErrReadOnly }

// ResolveOSEnvRefs scans a variable value string for ${OS_ENV:NAME} references
// and returns the resolved value plus a list of resolved secret values (for
// the masker). Unresolved references are left intact and a warning is emitted
// to stderr.
func ResolveOSEnvRefs(value string, warn func(string)) (string, []string) {
	if !strings.Contains(value, "${OS_ENV:") {
		return value, nil
	}

	var secrets []string
	result := value

	// Iteratively replace all ${OS_ENV:NAME} occurrences.
	for {
		start := strings.Index(result, "${OS_ENV:")
		if start == -1 {
			break
		}
		end := strings.Index(result[start:], "}")
		if end == -1 {
			break
		}
		end += start

		placeholder := result[start : end+1]            // ${OS_ENV:NAME}
		varName := result[start+len("${OS_ENV:") : end] // NAME

		varName = strings.TrimSpace(varName)
		envVal, ok := os.LookupEnv(varName)
		if !ok {
			if warn != nil {
				warn(fmt.Sprintf("${OS_ENV:%s} is not set in the environment; placeholder left unresolved", varName))
			}
			// advance past this placeholder to avoid infinite loop
			result = result[:end+1] + result[end+1:]
			break
		}

		secrets = append(secrets, envVal)
		result = strings.Replace(result, placeholder, envVal, 1)
	}

	return result, secrets
}
