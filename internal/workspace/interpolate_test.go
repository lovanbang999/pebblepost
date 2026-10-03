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

func TestInterpolator_Escaping(t *testing.T) {
	interpolator := NewInterpolator()
	vars := map[string]string{
		"NAME": "Alice",
		"ROLE": "Admin",
	}

	// 1. Pure escaped literal
	res1 := interpolator.InterpolateString(`\{{LITERAL_VAR}}`, vars)
	if res1 != "{{LITERAL_VAR}}" {
		t.Errorf("expected '{{LITERAL_VAR}}', got '%s'", res1)
	}

	// 2. Escaped literal mixed with interpolated variable
	res2 := interpolator.InterpolateString(`Hello {{NAME}}, your template is \{{USER_ROLE}} and role is {{ROLE}}`, vars)
	expected2 := "Hello Alice, your template is {{USER_ROLE}} and role is Admin"
	if res2 != expected2 {
		t.Errorf("expected '%s', got '%s'", expected2, res2)
	}

	// 3. Double backslash before braces: \\{{FOO}}
	res3 := interpolator.InterpolateString(`Prefix \{{STAY}} Suffix`, nil)
	if res3 != "Prefix {{STAY}} Suffix" {
		t.Errorf("expected 'Prefix {{STAY}} Suffix', got '%s'", res3)
	}
}

func TestInterpolator_CircularReferenceChain(t *testing.T) {
	interpolator := NewInterpolator()

	// 1. Two-node cycle: A -> B -> A
	vars2 := map[string]string{
		"A": "{{B}}",
		"B": "{{A}}",
	}
	_, err2 := interpolator.InterpolateStringWithError("{{A}}", vars2)
	if err2 == nil {
		t.Fatalf("expected circular reference error for A -> B -> A, got nil")
	}
	if !strings.Contains(err2.Error(), "circular variable reference detected") || !strings.Contains(err2.Error(), "A -> B -> A") {
		t.Errorf("unexpected error message: %v", err2)
	}

	// 2. Three-node cycle: X -> Y -> Z -> X
	vars3 := map[string]string{
		"X": "{{Y}}",
		"Y": "{{Z}}",
		"Z": "{{X}}",
	}
	_, err3 := interpolator.InterpolateStringWithError("Val: {{X}}", vars3)
	if err3 == nil {
		t.Fatalf("expected circular reference error for X -> Y -> Z -> X, got nil")
	}
	if !strings.Contains(err3.Error(), "X -> Y -> Z -> X") {
		t.Errorf("expected 'X -> Y -> Z -> X' in error, got: %v", err3)
	}

	// 3. Self-referential: LOOP -> LOOP
	varsSelf := map[string]string{
		"LOOP": "{{LOOP}}",
	}
	_, errSelf := interpolator.InterpolateStringWithError("{{LOOP}}", varsSelf)
	if errSelf == nil {
		t.Fatalf("expected circular reference error for self-reference, got nil")
	}
	if !strings.Contains(errSelf.Error(), "LOOP -> LOOP") {
		t.Errorf("expected 'LOOP -> LOOP' in error, got: %v", errSelf)
	}
}

func TestInterpolator_DynamicVariables_All(t *testing.T) {
	interpolator := NewInterpolator()

	// $uuid
	resUUID := interpolator.InterpolateString("{{$uuid}}", nil)
	if len(resUUID) != 36 {
		t.Errorf("expected 36-char UUID, got: %s", resUUID)
	}

	// $timestamp (seconds)
	resTS := interpolator.InterpolateString("{{$timestamp}}", nil)
	if len(resTS) < 10 {
		t.Errorf("expected unix timestamp, got: %s", resTS)
	}

	// $isoTimestamp (RFC3339)
	resISO := interpolator.InterpolateString("{{$isoTimestamp}}", nil)
	if !strings.Contains(resISO, "T") || !strings.HasSuffix(resISO, "Z") {
		t.Errorf("expected ISO timestamp RFC3339, got: %s", resISO)
	}

	// $randomInt
	resInt := interpolator.InterpolateString("{{$randomInt}}", nil)
	if resInt == "" {
		t.Errorf("expected non-empty randomInt, got empty")
	}

	// $randomEmail
	resEmail := interpolator.InterpolateString("{{$randomEmail}}", nil)
	if !strings.Contains(resEmail, "@example.com") {
		t.Errorf("expected random email ending with @example.com, got: %s", resEmail)
	}
}

func TestInterpolator_InterpolateRequestWithError(t *testing.T) {
	interpolator := NewInterpolator()

	// Circular reference in request URL
	req := &types.RequestDefinition{
		Name:   "Cycle Test",
		Method: "GET",
		URL:    "https://api.example.com/{{A}}",
	}
	vars := map[string]string{
		"A": "{{B}}",
		"B": "{{A}}",
	}

	_, err := interpolator.InterpolateRequestWithError(req, vars)
	if err == nil {
		t.Fatalf("expected error from InterpolateRequestWithError on circular reference, got nil")
	}
	if !strings.Contains(err.Error(), "circular variable reference detected") {
		t.Errorf("unexpected error message: %v", err)
	}
}
