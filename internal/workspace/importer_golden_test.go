package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadTestdata(t *testing.T, filename string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", filename))
	if err != nil {
		t.Fatalf("failed to load testdata/%s: %v", filename, err)
	}
	return data
}

// ---------------------------------------------------------------------------
// Postman v2.1 golden tests
// ---------------------------------------------------------------------------

func TestParsePostman_Golden_RequestCount(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "postman_sample.json")

	reqs, err := svc.ParsePostman(data)
	if err != nil {
		t.Fatalf("ParsePostman returned error: %v", err)
	}

	// fixture has 5 requests (Login, Get Profile, List Users, Create User, Upload Avatar)
	if len(reqs) != 5 {
		t.Errorf("expected 5 requests, got %d", len(reqs))
		for i, r := range reqs {
			t.Logf("  [%d] %s %s", i, r.Method, r.Name)
		}
	}
}

func TestParsePostman_Golden_Login(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "postman_sample.json")
	reqs, _ := svc.ParsePostman(data)

	var login *struct {
		method, name, url, bodyType, body string
		headerCount                       int
	}
	for _, r := range reqs {
		if r.Name == "Login" {
			login = &struct {
				method, name, url, bodyType, body string
				headerCount                       int
			}{r.Method, r.Name, r.URL, r.Body.Type, r.Body.Raw, len(r.Headers)}
		}
	}
	if login == nil {
		t.Fatal("Login request not found in parsed output")
	}
	if login.method != "POST" {
		t.Errorf("Login method: want POST, got %s", login.method)
	}
	if login.url != "https://api.example.com/auth/login" {
		t.Errorf("Login URL: got %s", login.url)
	}
	if login.bodyType != "json" {
		t.Errorf("Login body type: want json, got %s", login.bodyType)
	}
	if !strings.Contains(login.body, "admin") {
		t.Errorf("Login body does not contain expected content: %s", login.body)
	}
	if login.headerCount != 1 {
		t.Errorf("Login headers: want 1 (Content-Type), got %d", login.headerCount)
	}
}

func TestParsePostman_Golden_BearerAuth(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "postman_sample.json")
	reqs, _ := svc.ParsePostman(data)

	for _, r := range reqs {
		if r.Name == "Get Profile" {
			if r.Auth.Type != "bearer" {
				t.Errorf("Get Profile auth type: want bearer, got %s", r.Auth.Type)
			}
			if r.Auth.Token != "{{TOKEN}}" {
				t.Errorf("Get Profile bearer token: want {{TOKEN}}, got %s", r.Auth.Token)
			}
			if len(r.Params) != 1 || r.Params[0].Key != "include" {
				t.Errorf("Get Profile query params: want [include], got %v", r.Params)
			}
			return
		}
	}
	t.Fatal("Get Profile request not found")
}

func TestParsePostman_Golden_ApiKeyAuth(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "postman_sample.json")
	reqs, _ := svc.ParsePostman(data)

	for _, r := range reqs {
		if r.Name == "List Users" {
			if r.Auth.Type != "apiKey" {
				t.Errorf("List Users auth type: want apiKey, got %s", r.Auth.Type)
			}
			if len(r.Params) != 2 {
				t.Errorf("List Users params: want 2 (page, limit), got %d", len(r.Params))
			}
			return
		}
	}
	t.Fatal("List Users request not found")
}

func TestParsePostman_Golden_BasicAuth(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "postman_sample.json")
	reqs, _ := svc.ParsePostman(data)

	for _, r := range reqs {
		if r.Name == "Upload Avatar" {
			if r.Auth.Type != "basic" {
				t.Errorf("Upload Avatar auth type: want basic, got %s", r.Auth.Type)
			}
			if r.Auth.Username != "admin" {
				t.Errorf("Upload Avatar username: want admin, got %s", r.Auth.Username)
			}
			if r.Body.Type != "formData" {
				t.Errorf("Upload Avatar body type: want formData, got %s", r.Body.Type)
			}
			return
		}
	}
	t.Fatal("Upload Avatar request not found")
}

func TestParsePostman_Golden_UrlEncodedBody(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "postman_sample.json")
	reqs, _ := svc.ParsePostman(data)

	for _, r := range reqs {
		if r.Name == "Create User" {
			if r.Body.Type != "urlEncoded" {
				t.Errorf("Create User body type: want urlEncoded, got %s", r.Body.Type)
			}
			if len(r.Body.UrlEncoded) != 2 {
				t.Errorf("Create User urlEncoded fields: want 2, got %d", len(r.Body.UrlEncoded))
			}
			// Disabled header should be disabled
			for _, h := range r.Headers {
				if h.Key == "X-Request-ID" && h.Enabled {
					t.Error("disabled header X-Request-ID should have Enabled=false")
				}
			}
			return
		}
	}
	t.Fatal("Create User request not found")
}

func TestParsePostman_InvalidJSON(t *testing.T) {
	svc := NewImportService()
	_, err := svc.ParsePostman([]byte(`not json`))
	if err == nil {
		t.Error("expected error on invalid JSON, got nil")
	}
}

func TestParsePostman_EmptyCollection(t *testing.T) {
	svc := NewImportService()
	_, err := svc.ParsePostman([]byte(`{"info":{"name":"Empty"},"item":[]}`))
	if err == nil {
		t.Error("expected error for collection with no requests")
	}
}

// ---------------------------------------------------------------------------
// OpenAPI 3.0 golden tests
// ---------------------------------------------------------------------------

func TestParseOpenAPI_Golden_RequestCount(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "openapi_sample.json")

	reqs, err := svc.ParseOpenAPI(data)
	if err != nil {
		t.Fatalf("ParseOpenAPI returned error: %v", err)
	}

	// fixture: /users (GET + POST) + /users/{id} (GET + PUT + DELETE) + /health (GET) = 6
	if len(reqs) != 6 {
		t.Errorf("expected 6 operations, got %d", len(reqs))
		for i, r := range reqs {
			t.Logf("  [%d] %s %s (name: %s)", i, r.Method, r.URL, r.Name)
		}
	}
}

func TestParseOpenAPI_Golden_ServerBaseURL(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "openapi_sample.json")
	reqs, _ := svc.ParseOpenAPI(data)

	for _, r := range reqs {
		if !strings.HasPrefix(r.URL, "https://api.example.com/v1") {
			t.Errorf("request %s URL does not start with base URL: %s", r.Name, r.URL)
		}
	}
}

func TestParseOpenAPI_Golden_ListUsers(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "openapi_sample.json")
	reqs, _ := svc.ParseOpenAPI(data)

	for _, r := range reqs {
		if r.Name == "List all users" || r.Name == "listUsers" {
			if r.Method != "GET" {
				t.Errorf("listUsers method: want GET, got %s", r.Method)
			}
			// Should have query param "page" and header "X-Tenant-ID"
			hasPageParam := false
			hasTenantHeader := false
			for _, p := range r.Params {
				if p.Key == "page" {
					hasPageParam = true
				}
			}
			for _, h := range r.Headers {
				if h.Key == "X-Tenant-ID" {
					hasTenantHeader = true
				}
			}
			if !hasPageParam {
				t.Error("listUsers: missing query param 'page'")
			}
			if !hasTenantHeader {
				t.Error("listUsers: missing header 'X-Tenant-ID'")
			}
			return
		}
	}
	t.Fatal("listUsers operation not found in parsed OpenAPI output")
}

func TestParseOpenAPI_Golden_CreateUser_JSONBody(t *testing.T) {
	svc := NewImportService()
	data := loadTestdata(t, "openapi_sample.json")
	reqs, _ := svc.ParseOpenAPI(data)

	for _, r := range reqs {
		if r.Name == "Create a user" || r.Name == "createUser" {
			if r.Method != "POST" {
				t.Errorf("createUser method: want POST, got %s", r.Method)
			}
			if r.Body.Type != "json" {
				t.Errorf("createUser body type: want json, got %s", r.Body.Type)
			}
			return
		}
	}
	t.Fatal("createUser operation not found in parsed OpenAPI output")
}

func TestParseOpenAPI_InvalidJSON(t *testing.T) {
	svc := NewImportService()
	_, err := svc.ParseOpenAPI([]byte(`not json`))
	if err == nil {
		t.Error("expected error on invalid JSON, got nil")
	}
}

func TestParseOpenAPI_UnsupportedVersion(t *testing.T) {
	svc := NewImportService()
	_, err := svc.ParseOpenAPI([]byte(`{"openapi":"2.0","paths":{}}`))
	if err == nil {
		t.Error("expected error for OpenAPI v2, got nil")
	}
}

func TestParseOpenAPI_EmptyPaths(t *testing.T) {
	svc := NewImportService()
	_, err := svc.ParseOpenAPI([]byte(`{"openapi":"3.0.0","info":{"title":"Test","version":"1"},"paths":{}}`))
	if err == nil {
		t.Error("expected error for spec with no operations, got nil")
	}
}

func TestParsePostman_URLReconstruction_HostAndPath(t *testing.T) {
	rawJSON := `{
		"info": { "name": "Test Col" },
		"item": [
			{
				"name": "List Users",
				"request": {
					"method": "GET",
					"url": {
						"raw": "{{BASE_URL}}",
						"host": ["{{BASE_URL}}"],
						"path": ["api", "v1", "users"],
						"query": [
							{ "key": "limit", "value": "20" }
						]
					}
				}
			}
		]
	}`

	svc := NewImportService()
	reqs, err := svc.ParsePostman([]byte(rawJSON))
	if err != nil {
		t.Fatalf("ParsePostman error: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
	req := reqs[0]
	if req.URL != "{{BASE_URL}}/api/v1/users" {
		t.Errorf("want URL {{BASE_URL}}/api/v1/users, got %s", req.URL)
	}
	if len(req.Params) != 1 || req.Params[0].Key != "limit" {
		t.Errorf("expected 1 query param, got %+v", req.Params)
	}
}
