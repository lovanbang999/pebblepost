package secrets

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// fileRecord is the on-disk shape of a *.secret.env.json file.
type fileRecord struct {
	SchemaVersion int            `json:"schemaVersion"`
	Name          string         `json:"name"`
	Variables     []fileKeyValue `json:"variables"`
}

type fileKeyValue struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Enabled bool   `json:"enabled"`
	Secret  bool   `json:"secret,omitempty"`
}

// FileStore implements SecretStore using *.secret.env.json files in the
// workspace's .pebble/environments directory. This is the default backend and
// preserves the existing on-disk format unchanged.
type FileStore struct {
	// envDir is the absolute path to .pebble/environments/.
	envDir string
}

const secretFileSuffix = ".secret.env.json"

// NewFileStore creates a FileStore rooted at envDir.
// envDir should point to <workspace>/.pebble/environments.
func NewFileStore(envDir string) *FileStore {
	return &FileStore{envDir: envDir}
}

func (s *FileStore) Backend() SecretBackendType { return BackendFile }

func (s *FileStore) Get(service, key string) (string, error) {
	kvs, err := s.readAll(service)
	if err != nil {
		return "", err
	}
	for _, kv := range kvs {
		if kv.Key == key {
			return kv.Value, nil
		}
	}
	return "", fmt.Errorf("%w: %s/%s", ErrNotFound, service, key)
}

func (s *FileStore) Set(service, key, value string) error {
	kvs, _ := s.readAll(service) // ignore missing file

	found := false
	for i, kv := range kvs {
		if kv.Key == key {
			kvs[i].Value = value
			found = true
			break
		}
	}
	if !found {
		kvs = append(kvs, fileKeyValue{Key: key, Value: value, Enabled: true, Secret: true})
	}
	return s.writeAll(service, kvs)
}

func (s *FileStore) Delete(service, key string) error {
	kvs, err := s.readAll(service)
	if err != nil {
		return err
	}
	filtered := kvs[:0]
	deleted := false
	for _, kv := range kvs {
		if kv.Key == key {
			deleted = true
			continue
		}
		filtered = append(filtered, kv)
	}
	if !deleted {
		return fmt.Errorf("%w: %s/%s", ErrNotFound, service, key)
	}
	if len(filtered) == 0 {
		_ = os.Remove(s.path(service))
		return nil
	}
	return s.writeAll(service, filtered)
}

func (s *FileStore) List(service string) ([]string, error) {
	kvs, err := s.readAll(service)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	keys := make([]string, 0, len(kvs))
	for _, kv := range kvs {
		keys = append(keys, kv.Key)
	}
	return keys, nil
}

// Path returns the absolute path to the secret file for the given environment name.
func (s *FileStore) Path(service string) string {
	return filepath.Join(s.envDir, service+secretFileSuffix)
}

// unexported alias used internally.
func (s *FileStore) path(service string) string { return s.Path(service) }

func (s *FileStore) readAll(service string) ([]fileKeyValue, error) {
	data, err := os.ReadFile(s.path(service))
	if err != nil {
		return nil, err
	}
	var rec fileRecord
	if err := json.Unmarshal(data, &rec); err != nil {
		return nil, fmt.Errorf("parse %s: %w", s.path(service), err)
	}
	return rec.Variables, nil
}

func (s *FileStore) writeAll(service string, kvs []fileKeyValue) error {
	if err := os.MkdirAll(s.envDir, 0755); err != nil {
		return err
	}
	rec := fileRecord{
		SchemaVersion: 1,
		Name:          service,
		Variables:     kvs,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	// Write atomically via temp file.
	tmp := s.path(service) + fmt.Sprintf(".%d.tmp", time.Now().UnixNano())
	if err := os.WriteFile(tmp, data, 0600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(service))
}

// ReadAllKV is a convenience used by EnvironmentService to bulk-read all
// secret variables from this backend for a given environment.
func (s *FileStore) ReadAllKV(service string) (map[string]string, error) {
	kvs, err := s.readAll(service)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	out := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		if strings.TrimSpace(kv.Key) != "" {
			out[kv.Key] = kv.Value
		}
	}
	return out, nil
}
