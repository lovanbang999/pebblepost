package workspace

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"pebblepost/internal/security"
	"pebblepost/internal/types"
)

const (
	EnvPublicSuffix = ".env.json"
	EnvSecretSuffix = ".secret.env.json"
)

// EnvironmentService manages public and secret environment variables.
type EnvironmentService struct {
	watcher *Watcher
}

// NewEnvironmentService creates a new EnvironmentService instance.
func NewEnvironmentService() *EnvironmentService {
	return &EnvironmentService{}
}

// SetWatcher connects an active file watcher to the service for self-write suppression.
func (s *EnvironmentService) SetWatcher(w *Watcher) {
	s.watcher = w
}

// ListEnvironments scans .pebble/environments and returns all merged environments.
func (s *EnvironmentService) ListEnvironments(rootPath string) ([]types.EnvironmentDefinition, error) {
	envDir := filepath.Join(rootPath, PebbleDir, EnvironmentsDir)
	entries, err := os.ReadDir(envDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []types.EnvironmentDefinition{}, nil
		}
		return nil, fmt.Errorf("failed to read environments directory: %w", err)
	}

	envMap := make(map[string]*types.EnvironmentDefinition)
	secretMap := make(map[string][]types.KeyValue)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		fullPath := filepath.Join(envDir, name)

		if strings.HasSuffix(name, EnvSecretSuffix) {
			envName := strings.TrimSuffix(name, EnvSecretSuffix)
			vars, err := s.readVariables(fullPath)
			if err == nil {
				secretMap[envName] = vars
			}
		} else if strings.HasSuffix(name, EnvPublicSuffix) {
			envName := strings.TrimSuffix(name, EnvPublicSuffix)
			vars, err := s.readVariables(fullPath)
			if err == nil {
				envMap[envName] = &types.EnvironmentDefinition{
					SchemaVersion: CurrentSchemaVersion,
					Name:          envName,
					Variables:     vars,
				}
			}
		}
	}

	// Also handle any secret environments that don't have a public counterpart
	for secretEnvName := range secretMap {
		if _, exists := envMap[secretEnvName]; !exists {
			envMap[secretEnvName] = &types.EnvironmentDefinition{
				SchemaVersion: CurrentSchemaVersion,
				Name:          secretEnvName,
				Variables:     []types.KeyValue{},
			}
		}
	}

	// Merge secrets into environments (secret variables override public with same key)
	var result []types.EnvironmentDefinition
	for envName, envDef := range envMap {
		mergedVars := s.mergeVariables(envDef.Variables, secretMap[envName])
		result = append(result, types.EnvironmentDefinition{
			SchemaVersion: CurrentSchemaVersion,
			Name:          envName,
			Variables:     mergedVars,
		})
	}

	// Sort alphabetically by environment name
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})

	return result, nil
}

// GetEnvironment retrieves a merged environment by name.
func (s *EnvironmentService) GetEnvironment(rootPath string, envName string) (*types.EnvironmentDefinition, error) {
	envs, err := s.ListEnvironments(rootPath)
	if err != nil {
		return nil, err
	}

	for _, env := range envs {
		if strings.EqualFold(env.Name, envName) {
			return &env, nil
		}
	}

	return nil, fmt.Errorf("environment '%s' not found", envName)
}

// SaveEnvironment writes an environment definition to public or secret file.
func (s *EnvironmentService) SaveEnvironment(rootPath string, env types.EnvironmentDefinition, isSecret bool) error {
	if err := security.ValidateSafeIdentifier(env.Name); err != nil {
		return fmt.Errorf("invalid environment name: %w", err)
	}

	envDir := filepath.Join(rootPath, PebbleDir, EnvironmentsDir)
	if err := os.MkdirAll(envDir, 0755); err != nil {
		return fmt.Errorf("failed to create environments directory: %w", err)
	}

	var fileName string
	if isSecret {
		fileName = env.Name + EnvSecretSuffix
	} else {
		fileName = env.Name + EnvPublicSuffix
	}

	filePath := filepath.Join(envDir, fileName)

	// Suppress watcher for internal write
	if s.watcher != nil {
		s.watcher.Suppress(filePath, 1000*time.Millisecond)
	}

	env.SchemaVersion = CurrentSchemaVersion
	return WriteFileStable(filePath, env)
}

// DeleteEnvironment removes both public and secret files for an environment.
func (s *EnvironmentService) DeleteEnvironment(rootPath string, envName string) error {
	if err := security.ValidateSafeIdentifier(envName); err != nil {
		return fmt.Errorf("invalid environment name: %w", err)
	}

	envDir := filepath.Join(rootPath, PebbleDir, EnvironmentsDir)
	publicFile := filepath.Join(envDir, envName+EnvPublicSuffix)
	secretFile := filepath.Join(envDir, envName+EnvSecretSuffix)

	if s.watcher != nil {
		s.watcher.Suppress(publicFile, 1000*time.Millisecond)
		s.watcher.Suppress(secretFile, 1000*time.Millisecond)
	}

	_ = os.Remove(publicFile)
	_ = os.Remove(secretFile)

	return nil
}

func (s *EnvironmentService) readVariables(filePath string) ([]types.KeyValue, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}

	var env types.EnvironmentDefinition
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, err
	}

	migrated, _ := MigrateEnvironment(&env)
	return migrated.Variables, nil
}

func (s *EnvironmentService) mergeVariables(publicVars []types.KeyValue, secretVars []types.KeyValue) []types.KeyValue {
	mergedMap := make(map[string]types.KeyValue)
	var orderedKeys []string

	for _, v := range publicVars {
		if _, exists := mergedMap[v.Key]; !exists {
			orderedKeys = append(orderedKeys, v.Key)
		}
		mergedMap[v.Key] = v
	}

	for _, v := range secretVars {
		if _, exists := mergedMap[v.Key]; !exists {
			orderedKeys = append(orderedKeys, v.Key)
		}
		// Secret overrides public if key exists
		mergedMap[v.Key] = v
	}

	var result []types.KeyValue
	for _, key := range orderedKeys {
		result = append(result, mergedMap[key])
	}

	return result
}
