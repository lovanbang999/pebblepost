package secrets

import (
	"fmt"
	"path/filepath"
)

// OpenStore constructs a SecretStore for the given environment name based on
// the workspace configuration. The returned store uses the effective backend
// type for envName (global default, or per-env override).
//
// Parameters:
//   - cfg:           workspace-level secret backend configuration.
//   - envName:       the environment whose backend should be returned.
//   - envDir:        absolute path to <workspace>/.pebble/environments (for file backend).
//   - workspaceName: short workspace identifier used as the keychain service prefix.
func OpenStore(cfg SecretBackendConfig, envName, envDir, workspaceName string) (SecretStore, error) {
	backend := cfg.BackendFor(envName)
	switch backend {
	case BackendFile, "":
		return NewFileStore(envDir), nil
	case BackendKeychain:
		return NewKeychainStore(workspaceName), nil
	case BackendEnv:
		return NewEnvStore(), nil
	default:
		return nil, fmt.Errorf("unknown secret backend %q", backend)
	}
}

// OpenStoreForAll constructs a map of environment→SecretStore for all envNames,
// reusing store instances where the effective backend is the same.
func OpenStoreForAll(cfg SecretBackendConfig, envNames []string, envDir, workspaceName string) (map[string]SecretStore, error) {
	stores := make(map[string]SecretStore, len(envNames))

	// Cache per backend type to avoid multiple keychain handles.
	cache := make(map[SecretBackendType]SecretStore)

	for _, name := range envNames {
		bt := cfg.BackendFor(name)
		if s, ok := cache[bt]; ok {
			stores[name] = s
			continue
		}
		s, err := OpenStore(cfg, name, envDir, workspaceName)
		if err != nil {
			return nil, fmt.Errorf("open store for env %q: %w", name, err)
		}
		cache[bt] = s
		stores[name] = s
	}
	return stores, nil
}

// EnvDirFromRoot derives the environments directory path from a workspace root.
func EnvDirFromRoot(rootPath string) string {
	return filepath.Join(rootPath, ".pebble", "environments")
}
