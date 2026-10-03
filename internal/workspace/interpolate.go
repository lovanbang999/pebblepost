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
	varRegex          = regexp.MustCompile(`\{\{\s*([a-zA-Z0-9_$.-]+)\s*\}\}`)
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

const (
	escapeSentinel = "\x00_PEBBLE_ESCAPED_BRACE_\x00"
)

// InterpolateString recursively resolves {{VARIABLE}} placeholders within input string.
// If a circular reference or error occurs, it returns the input with placeholders intact.
func (in *Interpolator) InterpolateString(input string, vars map[string]string) string {
	res, err := in.InterpolateStringWithError(input, vars)
	if err != nil {
		return input
	}
	return res
}

// InterpolateStringWithError resolves placeholders and returns an error if a circular reference
// or excessive recursion depth is detected.
func (in *Interpolator) InterpolateStringWithError(input string, vars map[string]string) (string, error) {
	if input == "" {
		return "", nil
	}

	// 1. Handle literal escape syntax: \{{ -> sentinel placeholder
	escaped := strings.ReplaceAll(input, `\{{`, escapeSentinel)
	if !strings.Contains(escaped, "{{") {
		return strings.ReplaceAll(escaped, escapeSentinel, "{{"), nil
	}

	res, err := in.resolveWithChain(escaped, vars, nil)
	if err != nil {
		return input, err
	}

	// Restore escaped literal braces
	return strings.ReplaceAll(res, escapeSentinel, "{{"), nil
}

func (in *Interpolator) resolveWithChain(text string, vars map[string]string, chain []string) (string, error) {
	if text == "" || !strings.Contains(text, "{{") {
		return text, nil
	}

	if len(chain) >= maxRecursionDepth {
		return "", fmt.Errorf("variable recursion depth exceeded maximum limit (%d): %s", maxRecursionDepth, strings.Join(chain, " -> "))
	}

	current := text
	for depth := 0; depth < maxRecursionDepth; depth++ {
		var firstErr error
		replaced := varRegex.ReplaceAllStringFunc(current, func(match string) string {
			if firstErr != nil {
				return match
			}

			submatch := varRegex.FindStringSubmatch(match)
			if len(submatch) < 2 {
				return match
			}
			key := strings.TrimSpace(submatch[1])

			// 1. Check user-defined environment/runtime variables
			if val, exists := vars[key]; exists {
				// Check for circular reference in resolution chain
				for _, prev := range chain {
					if prev == key {
						cycle := append(chain, key)
						firstErr = fmt.Errorf("circular variable reference detected: %s", strings.Join(cycle, " -> "))
						return match
					}
				}

				// Resolve the value recursively with updated chain
				resolvedVal, err := in.resolveWithChain(val, vars, append(chain, key))
				if err != nil {
					firstErr = err
					return match
				}
				return resolvedVal
			}

			// 2. Check built-in dynamic variables
			if dynamicVal, isDynamic := in.resolveDynamicVariable(key); isDynamic {
				return dynamicVal
			}

			// Unresolved variable remains untouched
			return match
		})

		if firstErr != nil {
			return "", firstErr
		}

		if replaced == current {
			break
		}
		current = replaced
	}

	return current, nil
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
		Type:           auth.Type,
		Token:          in.InterpolateString(auth.Token, vars),
		Username:       in.InterpolateString(auth.Username, vars),
		Password:       in.InterpolateString(auth.Password, vars),
		Key:            in.InterpolateString(auth.Key, vars),
		Value:          in.InterpolateString(auth.Value, vars),
		AddTo:          auth.AddTo,
		Realm:          in.InterpolateString(auth.Realm, vars),
		GrantType:      auth.GrantType,
		AuthURL:        in.InterpolateString(auth.AuthURL, vars),
		TokenURL:       in.InterpolateString(auth.TokenURL, vars),
		ClientID:       in.InterpolateString(auth.ClientID, vars),
		ClientSecret:   in.InterpolateString(auth.ClientSecret, vars),
		Scope:          in.InterpolateString(auth.Scope, vars),
		RedirectURL:    in.InterpolateString(auth.RedirectURL, vars),
		CodeVerifier:   in.InterpolateString(auth.CodeVerifier, vars),
		RefreshToken:   in.InterpolateString(auth.RefreshToken, vars),
		TokenExpiresAt: auth.TokenExpiresAt,
		AccessKey:      in.InterpolateString(auth.AccessKey, vars),
		SecretKey:      in.InterpolateString(auth.SecretKey, vars),
		Region:         in.InterpolateString(auth.Region, vars),
		Service:        in.InterpolateString(auth.Service, vars),
		SessionToken:   in.InterpolateString(auth.SessionToken, vars),
	}
}

// InterpolateSettings interpolates variable placeholders inside string settings.
func (in *Interpolator) InterpolateSettings(settings types.SettingDefinition, vars map[string]string) types.SettingDefinition {
	return types.SettingDefinition{
		FollowRedirects:  settings.FollowRedirects,
		VerifySSL:        settings.VerifySSL,
		TimeoutMs:        settings.TimeoutMs,
		ScriptTimeoutMs:  settings.ScriptTimeoutMs,
		ConnectTimeoutMs: settings.ConnectTimeoutMs,
		MaxRedirects:     settings.MaxRedirects,
		EnableCookies:    settings.EnableCookies,
		UserAgent:        in.InterpolateString(settings.UserAgent, vars),
		ProxyURL:         in.InterpolateString(settings.ProxyURL, vars),
		ClientCertPath:   in.InterpolateString(settings.ClientCertPath, vars),
		ClientKeyPath:    in.InterpolateString(settings.ClientKeyPath, vars),
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

// InterpolateRequestWithError produces a deep copy of RequestDefinition with all variables resolved,
// returning an error if a circular variable reference or recursion depth limit is encountered.
func (in *Interpolator) InterpolateRequestWithError(req *types.RequestDefinition, vars map[string]string) (*types.RequestDefinition, error) {
	if req == nil {
		return nil, nil
	}

	interp := func(s string) (string, error) {
		return in.InterpolateStringWithError(s, vars)
	}

	name, err := interp(req.Name)
	if err != nil {
		return nil, err
	}
	desc, err := interp(req.Description)
	if err != nil {
		return nil, err
	}
	url, err := interp(req.URL)
	if err != nil {
		return nil, err
	}

	// Headers
	var headers []types.KeyValue
	if len(req.Headers) > 0 {
		headers = make([]types.KeyValue, len(req.Headers))
		for i, h := range req.Headers {
			k, err := interp(h.Key)
			if err != nil {
				return nil, err
			}
			v, err := interp(h.Value)
			if err != nil {
				return nil, err
			}
			headers[i] = types.KeyValue{Key: k, Value: v, Enabled: h.Enabled, Type: h.Type, Secret: h.Secret}
		}
	}

	// Params
	var params []types.KeyValue
	if len(req.Params) > 0 {
		params = make([]types.KeyValue, len(req.Params))
		for i, p := range req.Params {
			k, err := interp(p.Key)
			if err != nil {
				return nil, err
			}
			v, err := interp(p.Value)
			if err != nil {
				return nil, err
			}
			params[i] = types.KeyValue{Key: k, Value: v, Enabled: p.Enabled, Type: p.Type, Secret: p.Secret}
		}
	}

	// Auth
	auth := req.Auth
	if auth.Token != "" {
		if auth.Token, err = interp(auth.Token); err != nil {
			return nil, err
		}
	}
	if auth.Username != "" {
		if auth.Username, err = interp(auth.Username); err != nil {
			return nil, err
		}
	}
	if auth.Password != "" {
		if auth.Password, err = interp(auth.Password); err != nil {
			return nil, err
		}
	}
	if auth.Key != "" {
		if auth.Key, err = interp(auth.Key); err != nil {
			return nil, err
		}
	}
	if auth.Value != "" {
		if auth.Value, err = interp(auth.Value); err != nil {
			return nil, err
		}
	}

	// Body
	body := in.InterpolateBody(req.Body, vars)
	if body.Raw != "" {
		if body.Raw, err = interp(body.Raw); err != nil {
			return nil, err
		}
	}

	// gRPC
	var grpcDef *types.GrpcDefinition
	if req.Grpc != nil {
		g := req.Grpc
		addr, err := interp(g.Address)
		if err != nil {
			return nil, err
		}
		svc, err := interp(g.Service)
		if err != nil {
			return nil, err
		}
		method, err := interp(g.Method)
		if err != nil {
			return nil, err
		}
		msg, err := interp(g.Message)
		if err != nil {
			return nil, err
		}
		rootCA, err := interp(g.RootCAPath)
		if err != nil {
			return nil, err
		}
		var meta []types.KeyValue
		if len(g.Metadata) > 0 {
			meta = make([]types.KeyValue, len(g.Metadata))
			for i, m := range g.Metadata {
				k, err := interp(m.Key)
				if err != nil {
					return nil, err
				}
				v, err := interp(m.Value)
				if err != nil {
					return nil, err
				}
				meta[i] = types.KeyValue{Key: k, Value: v, Enabled: m.Enabled, Type: m.Type, Secret: m.Secret}
			}
		}
		var messages []string
		if len(g.Messages) > 0 {
			messages = make([]string, len(g.Messages))
			for i, m := range g.Messages {
				im, err := interp(m)
				if err != nil {
					return nil, err
				}
				messages[i] = im
			}
		}
		grpcDef = &types.GrpcDefinition{
			Address:            addr,
			ProtoSource:        g.ProtoSource,
			ProtoFiles:         g.ProtoFiles,
			ImportPaths:        g.ImportPaths,
			Service:            svc,
			Method:             method,
			Metadata:           meta,
			Message:            msg,
			Messages:           messages,
			UseTLS:             g.UseTLS,
			InsecureSkipVerify: g.InsecureSkipVerify,
			RootCAPath:         rootCA,
		}
	}

	return &types.RequestDefinition{
		Schema:        req.Schema,
		SchemaVersion: req.SchemaVersion,
		Version:       req.Version,
		ID:            req.ID,
		Name:          name,
		Description:   desc,
		Order:         req.Order,
		Tags:          req.Tags,
		Protocol:      req.Protocol,
		Method:        req.Method,
		URL:           url,
		Headers:       headers,
		Params:        params,
		Auth:          auth,
		Body:          body,
		Grpc:          grpcDef,
		Scripts:       req.Scripts,
		Settings:      in.InterpolateSettings(req.Settings, vars),
	}, nil
}

// InterpolateGrpc interpolates gRPC configuration fields.
func (in *Interpolator) InterpolateGrpc(g *types.GrpcDefinition, vars map[string]string) *types.GrpcDefinition {
	if g == nil {
		return nil
	}
	var messages []string
	if len(g.Messages) > 0 {
		messages = make([]string, len(g.Messages))
		for i, m := range g.Messages {
			messages[i] = in.InterpolateString(m, vars)
		}
	}
	return &types.GrpcDefinition{
		Address:            in.InterpolateString(g.Address, vars),
		ProtoSource:        g.ProtoSource,
		ProtoFiles:         g.ProtoFiles,
		ImportPaths:        g.ImportPaths,
		Service:            in.InterpolateString(g.Service, vars),
		Method:             in.InterpolateString(g.Method, vars),
		Metadata:           in.InterpolateKeyValues(g.Metadata, vars),
		Message:            in.InterpolateString(g.Message, vars),
		Messages:           messages,
		UseTLS:             g.UseTLS,
		InsecureSkipVerify: g.InsecureSkipVerify,
		RootCAPath:         in.InterpolateString(g.RootCAPath, vars),
	}
}

// InterpolateRequest produces a deep copy of RequestDefinition with all variables resolved.
func (in *Interpolator) InterpolateRequest(req *types.RequestDefinition, vars map[string]string) *types.RequestDefinition {
	if req == nil {
		return nil
	}

	return &types.RequestDefinition{
		Schema:        req.Schema,
		SchemaVersion: req.SchemaVersion,
		Version:       req.Version,
		ID:            req.ID,
		Name:          in.InterpolateString(req.Name, vars),
		Description:   in.InterpolateString(req.Description, vars),
		Order:         req.Order,
		Tags:          req.Tags,
		Protocol:      req.Protocol,
		Method:        req.Method,
		URL:           in.InterpolateString(req.URL, vars),
		Headers:       in.InterpolateKeyValues(req.Headers, vars),
		Params:        in.InterpolateKeyValues(req.Params, vars),
		Auth:          in.InterpolateAuth(req.Auth, vars),
		Body:          in.InterpolateBody(req.Body, vars),
		Grpc:          in.InterpolateGrpc(req.Grpc, vars),
		Scripts:       req.Scripts, // Scripts are executed at runtime, not interpolated
		Settings:      in.InterpolateSettings(req.Settings, vars),
	}
}

func generateUUID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40 // Version 4
	b[8] = (b[8] & 0x3f) | 0x80 // Variant RFC4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
