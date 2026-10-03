package runner

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"pebblepost/internal/workspace"
)

// BuildVarMap constructs the merged variable map applying precedence rules:
//
//	--var flags > PEBBLE_VAR_* env vars > --env-file > selected environment (pre-loaded via envDef)
//
// envVars is the already-loaded selected environment variable map (lowest precedence).
// secretValues is populated with raw values of secret variables (for masking).
func BuildVarMap(
	envVars map[string]string,
	extraVars map[string]string,
	envFilePath string,
	secretValues *[]string,
	interpolator *workspace.Interpolator,
) map[string]string {
	merged := make(map[string]string)

	// 1. Lowest: selected environment (already interpolated by caller)
	for k, v := range envVars {
		merged[k] = v
	}

	// 2. --env-file (overrides selected env)
	if envFilePath != "" {
		fileVars, secrets, err := loadEnvFile(envFilePath, interpolator)
		if err == nil {
			for k, v := range fileVars {
				merged[k] = v
			}
			if secretValues != nil {
				*secretValues = append(*secretValues, secrets...)
			}
		}
	}

	// 3. PEBBLE_VAR_* environment variables (override env-file)
	for _, e := range os.Environ() {
		const prefix = "PEBBLE_VAR_"
		if !strings.HasPrefix(e, prefix) {
			continue
		}
		kv := strings.SplitN(e, "=", 2)
		if len(kv) != 2 {
			continue
		}
		// PEBBLE_VAR_MY_KEY → MY_KEY
		key := strings.TrimPrefix(kv[0], prefix)
		merged[key] = kv[1]
	}

	// 4. Highest: --var KEY=VALUE flags
	for k, v := range extraVars {
		merged[k] = v
	}

	return merged
}

// ParseVarFlags converts a slice of "KEY=VALUE" strings into a map.
func ParseVarFlags(vars []string) (map[string]string, error) {
	out := make(map[string]string, len(vars))
	for _, v := range vars {
		parts := strings.SplitN(v, "=", 2)
		if len(parts) != 2 || parts[0] == "" {
			return nil, fmt.Errorf("invalid --var %q: must be KEY=VALUE", v)
		}
		out[parts[0]] = parts[1]
	}
	return out, nil
}

// loadEnvFile reads a simple KEY=VALUE env file (one per line, # comments ignored).
// It returns the parsed variables and secret values (lines starting with SECRET_).
func loadEnvFile(path string, _ *workspace.Interpolator) (map[string]string, []string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, nil, err
	}
	defer f.Close()

	vars := make(map[string]string)
	var secrets []string

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		vars[key] = val

		// Treat values whose keys contain "SECRET" or "PASSWORD" as secret
		upper := strings.ToUpper(key)
		if strings.Contains(upper, "SECRET") || strings.Contains(upper, "PASSWORD") || strings.Contains(upper, "TOKEN") {
			secrets = append(secrets, val)
		}
	}
	return vars, secrets, scanner.Err()
}
