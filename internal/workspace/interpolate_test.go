package workspace

import (
	"strings"
	"testing"

	"pebblepost/internal/types"
)

func TestInterpolator_DirectAndRecursive(t *testing.T) {
	interpolator := NewInterpolator()

	vars := map[string]string{
		"PORT":     "8080",
		"HOST":     "localhost:{{PORT}}",
		"BASE_URL": "http://{{HOST}}/api/v1",
		"USER_ID":  "usr_12345",
	}

	// 1. Recursive interpolation
	urlTemplate := "{{BASE_URL}}/users/{{USER_ID}}/profile"
	expectedURL := "http://localhost:8080/api/v1/users/usr_12345/profile"

	resolved := interpolator.InterpolateString(urlTemplate, vars)
	if resolved != expectedURL {
		t.Errorf("expected '%s', got '%s'", expectedURL, resolved)
	}

	// 2. Spaces within handlebars: {{  USER_ID  }}
	spacedTemplate := "ID: {{  USER_ID  }}"
	expectedSpaced := "ID: usr_12345"
	if res := interpolator.InterpolateString(spacedTemplate, vars); res != expectedSpaced {
		t.Errorf("expected '%s', got '%s'", expectedSpaced, res)
	}

	// 3. Unresolved variable should remain untouched
	unresolvedTemplate := "{{UNKNOWN_KEY}}/data"
	if res := interpolator.InterpolateString(unresolvedTemplate, vars); res != "{{UNKNOWN_KEY}}/data" {
		t.Errorf("expected unresolved to stay '{{UNKNOWN_KEY}}/data', got '%s'", res)
	}
}

func TestInterpolator_CircularReference(t *testing.T) {
	interpolator := NewInterpolator()

	// Circular reference A -> B -> A
	vars := map[string]string{
		"A": "{{B}}",
		"B": "{{A}}",
	}

	// Should not hang or loop infinitely
	res := interpolator.InterpolateString("Val: {{A}}", vars)
	if !strings.Contains(res, "{{") {
		t.Errorf("expected unresolved circular placeholder, got '%s'", res)
	}
}

func TestInterpolator_DynamicVariables(t *testing.T) {
	interpolator := NewInterpolator()

	// {{$uuid}}
	uuidStr := interpolator.InterpolateString("id_{{$uuid}}", nil)
	if !strings.HasPrefix(uuidStr, "id_") || len(uuidStr) < 30 {
		t.Errorf("unexpected uuid output: %s", uuidStr)
	}

	// {{$timestamp}}
	tsStr := interpolator.InterpolateString("ts_{{$timestamp}}", nil)
	if !strings.HasPrefix(tsStr, "ts_") || len(tsStr) < 12 {
		t.Errorf("unexpected timestamp output: %s", tsStr)
	}

	// {{$randomEmail}}
	emailStr := interpolator.InterpolateString("{{$randomEmail}}", nil)
	if !strings.Contains(emailStr, "@example.com") {
		t.Errorf("unexpected random email output: %s", emailStr)
	}
}

func TestInterpolator_InterpolateRequest(t *testing.T) {
	interpolator := NewInterpolator()

	env := &types.EnvironmentDefinition{
		Name: "dev",
		Variables: []types.KeyValue{
			{Key: "HOST", Value: "api.service.io", Enabled: true},
			{Key: "API_KEY", Value: "secret-token-xyz", Enabled: true},
			{Key: "DISABLED_VAR", Value: "ignore-me", Enabled: false},
		},
	}

	varMap := interpolator.BuildVariableMap(env, map[string]string{
		"RUNTIME_PARAM": "limit=50",
	})

	// Check disabled var not loaded
	if _, exists := varMap["DISABLED_VAR"]; exists {
		t.Errorf("disabled variable should not be present in variable map")
	}

	req := &types.RequestDefinition{
		Name:   "Fetch Data for {{HOST}}",
		Method: "POST",
		URL:    "https://{{HOST}}/v1/search?{{RUNTIME_PARAM}}",
		Headers: []types.KeyValue{
			{Key: "X-Api-Key", Value: "{{API_KEY}}", Enabled: true},
		},
		Auth: types.AuthDefinition{
			Type:  "bearer",
			Token: "Bearer {{API_KEY}}",
		},
		Body: types.BodyDefinition{
			Type: "json",
			Raw:  `{"host": "{{HOST}}", "token": "{{API_KEY}}"}`,
		},
	}

	interpolated := interpolator.InterpolateRequest(req, varMap)

	if interpolated.Name != "Fetch Data for api.service.io" {
		t.Errorf("interpolated name mismatch: %s", interpolated.Name)
	}
	if interpolated.URL != "https://api.service.io/v1/search?limit=50" {
		t.Errorf("interpolated URL mismatch: %s", interpolated.URL)
	}
	if interpolated.Headers[0].Value != "secret-token-xyz" {
		t.Errorf("interpolated Header mismatch: %s", interpolated.Headers[0].Value)
	}
	if interpolated.Auth.Token != "Bearer secret-token-xyz" {
		t.Errorf("interpolated Auth mismatch: %s", interpolated.Auth.Token)
	}
	if !strings.Contains(interpolated.Body.Raw, `"host": "api.service.io"`) {
		t.Errorf("interpolated Body mismatch: %s", interpolated.Body.Raw)
	}
}
