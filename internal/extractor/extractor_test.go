package extractor

import (
	"testing"

	"pebblepost/internal/types"
)

func TestExtract_Primitives(t *testing.T) {
	jsonBody := `{
		"token": "secret_jwt_xyz",
		"userId": 1042,
		"active": true,
		"score": 98.6,
		"nullable": null
	}`

	cases := []struct {
		name     string
		path     string
		expected string
	}{
		{"root token", "$.token", "secret_jwt_xyz"},
		{"unprefixed token", "token", "secret_jwt_xyz"},
		{"integer userId", "$.userId", "1042"},
		{"boolean active", "$.active", "true"},
		{"float score", "$.score", "98.6"},
		{"null value", "$.nullable", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ext := types.ExtractorDefinition{
				ID:      "ext-1",
				Name:    "VAR",
				Path:    tc.path,
				Scope:   "runtime",
				Enabled: true,
			}
			res := Extract(ext, jsonBody)
			if !res.Success {
				t.Fatalf("expected success, got warning: %s", res.Warning)
			}
			if res.Value != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, res.Value)
			}
		})
	}
}

func TestExtract_NestedAndBrackets(t *testing.T) {
	jsonBody := `{
		"data": {
			"user-profile": {
				"email": "alice@example.com"
			},
			"items": [
				{"id": "item_1", "price": 10},
				{"id": "item_2", "price": 25}
			]
		}
	}`

	cases := []struct {
		name     string
		path     string
		expected string
	}{
		{"nested bracket key", "$.data['user-profile'].email", "alice@example.com"},
		{"array first item id", "$.data.items[0].id", "item_1"},
		{"array last item id", "$.data.items[-1].id", "item_2"},
		{"nested object serialized to json", "$.data.items[0]", `{"id":"item_1","price":10}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ext := types.ExtractorDefinition{
				ID:      "ext-nested",
				Name:    "VAR",
				Path:    tc.path,
				Scope:   "environment",
				Enabled: true,
			}
			res := Extract(ext, jsonBody)
			if !res.Success {
				t.Fatalf("expected success, got warning: %s", res.Warning)
			}
			if res.Value != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, res.Value)
			}
		})
	}
}

func TestExtract_MissingPathProducesWarningNotCrash(t *testing.T) {
	jsonBody := `{"status": "ok", "items": []}`

	ext := types.ExtractorDefinition{
		ID:      "ext-missing",
		Name:    "auth_token",
		Path:    "$.auth.token",
		Scope:   "runtime",
		Enabled: true,
	}

	res := Extract(ext, jsonBody)
	if res.Success {
		t.Fatalf("expected failure for missing path, got value: %q", res.Value)
	}
	if res.Warning == "" {
		t.Errorf("expected non-empty warning message")
	}
}

func TestExtract_InvalidJSONProducesWarning(t *testing.T) {
	invalidBody := `<html>502 Bad Gateway</html>`

	ext := types.ExtractorDefinition{
		ID:      "ext-err",
		Name:    "my_var",
		Path:    "$.id",
		Scope:   "runtime",
		Enabled: true,
	}

	res := Extract(ext, invalidBody)
	if res.Success {
		t.Fatalf("expected failure for invalid JSON")
	}
	if res.Warning == "" {
		t.Errorf("expected warning for invalid JSON")
	}
}

func TestExtractAll(t *testing.T) {
	jsonBody := `{
		"auth": {"token": "jwt_123"},
		"user": {"id": 42}
	}`

	extractors := []types.ExtractorDefinition{
		{
			ID:      "1",
			Name:    "TOKEN",
			Path:    "$.auth.token",
			Scope:   "environment",
			Enabled: true,
		},
		{
			ID:      "2",
			Name:    "USER_ID",
			Path:    "$.user.id",
			Scope:   "runtime",
			Enabled: true,
		},
		{
			ID:      "3",
			Name:    "DISABLED",
			Path:    "$.auth.token",
			Scope:   "runtime",
			Enabled: false,
		},
		{
			ID:      "4",
			Name:    "NON_EXISTENT",
			Path:    "$.missing.field",
			Scope:   "runtime",
			Enabled: true,
		},
	}

	results, vars, warnings := ExtractAll(extractors, jsonBody)
	if len(results) != 3 { // 3 enabled
		t.Errorf("expected 3 results for enabled extractors, got %d", len(results))
	}
	if vars["TOKEN"] != "jwt_123" {
		t.Errorf("expected vars[TOKEN] = jwt_123, got %q", vars["TOKEN"])
	}
	if vars["USER_ID"] != "42" {
		t.Errorf("expected vars[USER_ID] = 42, got %q", vars["USER_ID"])
	}
	if _, exists := vars["DISABLED"]; exists {
		t.Errorf("expected disabled extractor not to be extracted")
	}
	if len(warnings) != 1 {
		t.Errorf("expected 1 warning for NON_EXISTENT, got %d", len(warnings))
	}
}
