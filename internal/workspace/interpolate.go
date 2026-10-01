package workspace

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"time"

	"pebblepost/internal/types"
)

var (
	// Regex matching {{VARIABLE}} or {{ VARIABLE }}
	varRegex = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_$.-]+)\s*\}\}`)
	maxRecursionDepth = 10
)

// Interpolator replaces dynamic variables ({{VAR}}) within requests and strings.
type Interpolator struct{}

// NewInterpolator creates a new Interpolator instance.
func NewInterpolator() *Interpolator {
	return &Interpolator{}
}

// BuildVariableMap constructs a flat key-value lookup map from an environment definition
// and optional runtime overrides. Only enabled variables are included.
func (in *Interpolator) BuildVariableMap(env *types.EnvironmentDefinition, overrides map[string]string) map[string]string {
	vars := make(map[string]string)

	if env != nil {
		for _, kv := range env.Variables {
			if kv.Enabled && kv.Key != "" {
				vars[kv.Key] = kv.Value
			}
		}
	}

	for k, v := range overrides {
		if k != "" {
			vars[k] = v
		}
	}

	return vars
}

// InterpolateString recursively resolves {{VARIABLE}} placeholders within input string.
func (in *Interpolator) InterpolateString(input string, vars map[string]string) string {
	if input == "" || !strings.Contains(input, "{{") {
		return input
	}

	current := input
	for depth := 0; depth < maxRecursionDepth; depth++ {
		replaced := varRegex.ReplaceAllStringFunc(current, func(match string) string {
			submatch := varRegex.FindStringSubmatch(match)
			if len(submatch) < 2 {
				return match
			}
			key := strings.TrimSpace(submatch[1])

			// 1. Check user-defined environment/runtime variables
			if val, exists := vars[key]; exists {
				return val
			}

			// 2. Check built-in dynamic variables
			if dynamicVal, isDynamic := in.resolveDynamicVariable(key); isDynamic {
				return dynamicVal
			}

			// Unresolved variable remains untouched
			return match
		})

		// If no more replacements happened, stop recursion
		if replaced == current {
			break
		}
		current = replaced
	}

	return current
}

// resolveDynamicVariable resolves built-in variables like {{$uuid}}, {{$timestamp}}, etc.
func (in *Interpolator) resolveDynamicVariable(key string) (string, bool) {
	switch strings.ToLower(key) {
	case "$guid", "$uuid":
		return generateUUID(), true
	case "$timestamp":
		return fmt.Sprintf("%d", time.Now().Unix()), true
	case "$timestampms":
		return fmt.Sprintf("%d", time.Now().UnixMilli()), true
	case "$isotimestamp":
		return time.Now().UTC().Format(time.RFC3339), true
	case "$randomint":
		n, _ := rand.Int(rand.Reader, big.NewInt(1000))
		return fmt.Sprintf("%d", n.Int64()), true
	case "$randomemail":
		n, _ := rand.Int(rand.Reader, big.NewInt(10000))
		return fmt.Sprintf("user_%d@example.com", n.Int64()), true
	default:
		return "", false
	}
}

// InterpolateKeyValues interpolates a slice of KeyValues.
func (in *Interpolator) InterpolateKeyValues(kvs []types.KeyValue, vars map[string]string) []types.KeyValue {
	if len(kvs) == 0 {
		return nil
	}

	result := make([]types.KeyValue, len(kvs))
	for i, kv := range kvs {
		result[i] = types.KeyValue{
			Key:     in.InterpolateString(kv.Key, vars),
			Value:   in.InterpolateString(kv.Value, vars),
			Enabled: kv.Enabled,
			Type:    kv.Type,
		}
	}
	return result
}

// InterpolateAuth interpolates authentication tokens and credentials.
func (in *Interpolator) InterpolateAuth(auth types.AuthDefinition, vars map[string]string) types.AuthDefinition {
	return types.AuthDefinition{
		Type:     auth.Type,
		Token:    in.InterpolateString(auth.Token, vars),
		Username: in.InterpolateString(auth.Username, vars),
		Password: in.InterpolateString(auth.Password, vars),
		Key:      in.InterpolateString(auth.Key, vars),
		Value:    in.InterpolateString(auth.Value, vars),
		AddTo:    auth.AddTo,
	}
}

// InterpolateBody interpolates request body definitions including raw text, formData, urlEncoded, and GraphQL.
func (in *Interpolator) InterpolateBody(body types.BodyDefinition, vars map[string]string) types.BodyDefinition {
	result := types.BodyDefinition{
		Type:       body.Type,
		Raw:        in.InterpolateString(body.Raw, vars),
		FormData:   in.InterpolateKeyValues(body.FormData, vars),
		UrlEncoded: in.InterpolateKeyValues(body.UrlEncoded, vars),
	}

	if body.GraphQL != nil {
		result.GraphQL = &types.GraphQL{
			Query:     in.InterpolateString(body.GraphQL.Query, vars),
			Variables: in.InterpolateString(body.GraphQL.Variables, vars),
		}
	}

	return result
}

// InterpolateRequest produces a deep copy of RequestDefinition with all variables resolved.
func (in *Interpolator) InterpolateRequest(req *types.RequestDefinition, vars map[string]string) *types.RequestDefinition {
	if req == nil {
		return nil
	}

	return &types.RequestDefinition{
		Schema:      req.Schema,
		Version:     req.Version,
		ID:          req.ID,
		Name:        in.InterpolateString(req.Name, vars),
		Description: in.InterpolateString(req.Description, vars),
		Method:      req.Method,
		URL:         in.InterpolateString(req.URL, vars),
		Headers:     in.InterpolateKeyValues(req.Headers, vars),
		Params:      in.InterpolateKeyValues(req.Params, vars),
		Auth:        in.InterpolateAuth(req.Auth, vars),
		Body:        in.InterpolateBody(req.Body, vars),
		Scripts:     req.Scripts, // Scripts are executed at runtime, not interpolated
		Settings:    req.Settings,
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
