package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	keyring "github.com/zalando/go-keyring"
)

// keychainIndex is a small JSON file that tracks which keys have been stored
// under a given service in the keychain. The Secret Service API and macOS
// Keychain do not expose a native "list all keys" operation, so we maintain
// this index ourselves.
// Location: ~/.config/pebblepost/keychain-index.json
type keychainIndex struct {
	// Services maps service name → set of keys stored in the keychain.
	Services map[string][]string `json:"services"`
}

// KeychainStore implements SecretStore using the OS native keychain via
// github.com/zalando/go-keyring.
//
// Platform support:
//   - Linux:   D-Bus Secret Service (requires a running daemon, e.g. GNOME Keyring or KWallet).
//   - macOS:   Keychain via /usr/bin/security (no CGO required).
//   - Windows: Windows Credential Manager.
type KeychainStore struct {
	// servicePrefix is prepended to the environment name to form the keyring
	// "service" string, e.g. "pebblepost/my-workspace".
	servicePrefix string
	indexPath     string
	mu            sync.Mutex
}

// NewKeychainStore returns a KeychainStore whose service names are prefixed with
// "pebblepost/<workspaceName>".
func NewKeychainStore(workspaceName string) *KeychainStore {
	configDir, _ := os.UserConfigDir()
	indexPath := filepath.Join(configDir, "pebblepost", "keychain-index.json")
	return &KeychainStore{
		servicePrefix: "pebblepost/" + workspaceName,
		indexPath:     indexPath,
	}
}

func (s *KeychainStore) Backend() SecretBackendType { return BackendKeychain }

func (s *KeychainStore) Get(service, key string) (string, error) {
	val, err := keyring.Get(s.svc(service), key)
	if err != nil {
		if err == keyring.ErrNotFound {
			return "", fmt.Errorf("%w: %s/%s", ErrNotFound, service, key)
		}
		return "", fmt.Errorf("keychain get %s/%s: %w", service, key, err)
	}
	return val, nil
}

func (s *KeychainStore) Set(service, key, value string) error {
	if err := keyring.Set(s.svc(service), key, value); err != nil {
		return fmt.Errorf("keychain set %s/%s: %w", service, key, err)
	}
	return s.indexAdd(service, key)
}

func (s *KeychainStore) Delete(service, key string) error {
	err := keyring.Delete(s.svc(service), key)
	if err != nil {
		if err == keyring.ErrNotFound {
			return fmt.Errorf("%w: %s/%s", ErrNotFound, service, key)
		}
		return fmt.Errorf("keychain delete %s/%s: %w", service, key, err)
	}
	return s.indexRemove(service, key)
}

func (s *KeychainStore) List(service string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, _ := s.loadIndex()
	if idx.Services == nil {
		return nil, nil
	}
	return idx.Services[service], nil
}

// svc constructs the full keyring service name.
func (s *KeychainStore) svc(envName string) string {
	return s.servicePrefix + "/" + envName
}

func (s *KeychainStore) loadIndex() (keychainIndex, error) {
	var idx keychainIndex
	data, err := os.ReadFile(s.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			return keychainIndex{Services: make(map[string][]string)}, nil
		}
		return idx, err
	}
	err = json.Unmarshal(data, &idx)
	if idx.Services == nil {
		idx.Services = make(map[string][]string)
	}
	return idx, err
}

func (s *KeychainStore) saveIndex(idx keychainIndex) error {
	if err := os.MkdirAll(filepath.Dir(s.indexPath), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.indexPath, data, 0600)
}

func (s *KeychainStore) indexAdd(service, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, _ := s.loadIndex()
	keys := idx.Services[service]
	for _, k := range keys {
		if k == key {
			return nil // already present
		}
	}
	idx.Services[service] = append(keys, key)
	return s.saveIndex(idx)
}

func (s *KeychainStore) indexRemove(service, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx, _ := s.loadIndex()
	keys := idx.Services[service]
	filtered := keys[:0]
	for _, k := range keys {
		if k != key {
			filtered = append(filtered, k)
		}
	}
	idx.Services[service] = filtered
	return s.saveIndex(idx)
}
