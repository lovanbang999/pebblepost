package impexp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"pebblepost/internal/types"
)

func TestDetectFormat(t *testing.T) {
	tests := []struct {
		name     string
		filename string
		content  string
		expected Format
	}{
		{"Bruno extension", "req.bru", "get {\n url: https://example.com\n}", FormatBruno},
		{"Bruno meta prefix", "custom.txt", "meta {\n  name: Test\n}", FormatBruno},
		{"Insomnia v4", "export.json", `{"_type":"export","__export_format":4}`, FormatInsomnia},
		{"HAR 1.2", "traffic.har", `{"log":{"version":"1.2","entries":[]}}`, FormatHAR},
		{"Postman collection", "collection.json", `{"info":{"name":"Col","schema":"https://schema.getpostman.com/json/collection/v2.1.0/collection.json"}}`, FormatPostman},
		{"OpenAPI JSON", "spec.json", `{"openapi":"3.0.1","info":{"title":"API"}}`, FormatOpenAPI},
		{"cURL command", "cmd.sh", `curl -X POST https://api.com/users`, FormatCURL},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DetectFormat(tt.filename, []byte(tt.content))
			if got != tt.expected {
				t.Errorf("DetectFormat(%q) = %v, expected %v", tt.filename, got, tt.expected)
			}
		})
	}
}

func TestScriptConverter(t *testing.T) {
	input := `
pm.environment.set("token", "secret");
var id = pm.variables.get("userId");
var data = pm.response.json();
pm.test("Status code is 200", function() {
    pm.expect(pm.response.code).to.equal(200);
});
pm.expect(data.success).to.be.true;
pm.sendRequest("https://unsupported.com", function() {});
`
	converted, warnings := ConvertPostmanScript(input)

	if !strings.Contains(converted, `pb.environment.set("token", "secret")`) {
		t.Errorf("expected pb.environment.set, got:\n%s", converted)
	}
	if !strings.Contains(converted, `pb.variables.get("userId")`) {
		t.Errorf("expected pb.variables.get, got:\n%s", converted)
	}
	if !strings.Contains(converted, `pb.response.json()`) {
		t.Errorf("expected pb.response.json, got:\n%s", converted)
	}
	if !strings.Contains(converted, `pb.expect(pb.response.status).toBe(200)`) {
		t.Errorf("expected pb.expect(...).toBe(200), got:\n%s", converted)
	}
	if !strings.Contains(converted, `Could not auto-convert: pm.sendRequest`) {
		t.Errorf("expected comment for unconvertible pm.sendRequest, got:\n%s", converted)
	}
	if len(warnings) == 0 {
		t.Errorf("expected warnings for pm.sendRequest, got 0")
	}
}

func TestBrunoFileImport(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.bru")
	if err != nil {
		t.Fatalf("failed to read testdata/sample.bru: %v", err)
	}

	req, err := ParseBrunoFile(string(data), "sample")
	if err != nil {
		t.Fatalf("ParseBrunoFile failed: %v", err)
	}

	if req.Name != "Get User Profile" {
		t.Errorf("expected Name 'Get User Profile', got %q", req.Name)
	}
	if req.Method != "GET" {
		t.Errorf("expected Method 'GET', got %q", req.Method)
	}
	if req.URL != "https://api.example.com/v1/users/42" {
		t.Errorf("unexpected URL %q", req.URL)
	}
	if req.Auth.Type != "bearer" || req.Auth.Token != "{{apiKey}}" {
		t.Errorf("unexpected Auth: %+v", req.Auth)
	}
	if len(req.Headers) != 3 || req.Headers[0].Key != "Accept" || req.Headers[2].Enabled != false {
		t.Errorf("unexpected Headers: %+v", req.Headers)
	}
	if len(req.Params) != 2 || req.Params[0].Key != "include_orders" || req.Params[1].Enabled != false {
		t.Errorf("unexpected Params: %+v", req.Params)
	}
	if !strings.Contains(req.Scripts.PreRequest, "pb.environment.set") {
		t.Errorf("expected PreRequest script, got %q", req.Scripts.PreRequest)
	}
	if !strings.Contains(req.Scripts.PostResponse, "pb.test") {
		t.Errorf("expected PostResponse script, got %q", req.Scripts.PostResponse)
	}
}

func TestBrunoCollectionImport(t *testing.T) {
	svc := NewService()
	res, err := svc.ParseDir("testdata/bruno_collection")
	if err != nil {
		t.Fatalf("ParseDir failed: %v", err)
	}

	if res.CollectionName != "E-Commerce API" {
		t.Errorf("expected CollectionName 'E-Commerce API', got %q", res.CollectionName)
	}
	if len(res.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(res.Requests))
	}

	reqMap := make(map[string]ImportedItem)
	for _, it := range res.Requests {
		reqMap[it.Name] = it
	}

	login, ok := reqMap["Login"]
	if !ok {
		t.Fatalf("missing Login request")
	}
	if login.Request.Method != "POST" || login.Request.Body.Type != "json" {
		t.Errorf("unexpected Login request: %+v", login.Request)
	}

	users, ok := reqMap["List Users"]
	if !ok {
		t.Fatalf("missing List Users request")
	}
	if users.Request.Method != "GET" || users.Request.Auth.Type != "bearer" {
		t.Errorf("unexpected List Users request: %+v", users.Request)
	}
}

func TestInsomniaV4Import(t *testing.T) {
	data, err := os.ReadFile("testdata/insomnia_v4.json")
	if err != nil {
		t.Fatalf("failed to read testdata/insomnia_v4.json: %v", err)
	}

	res, err := ParseInsomnia(data)
	if err != nil {
		t.Fatalf("ParseInsomnia failed: %v", err)
	}

	if res.CollectionName != "Payments API" {
		t.Errorf("expected CollectionName 'Payments API', got %q", res.CollectionName)
	}
	if len(res.Variables) != 2 {
		t.Errorf("expected 2 collection variables, got %d", len(res.Variables))
	}
	if len(res.Folders) != 1 || res.Folders[0].Name != "transactions" {
		t.Errorf("expected 1 folder 'transactions', got %+v", res.Folders)
	}
	if len(res.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(res.Requests))
	}

	chargeReq := res.Requests[0].Request
	if chargeReq.Name != "Charge Credit Card" || chargeReq.Method != "POST" {
		t.Errorf("unexpected Charge request: %+v", chargeReq)
	}
	if chargeReq.Auth.Type != "bearer" || chargeReq.Auth.Token != "sk_test_SECRET_TOKEN" {
		t.Errorf("unexpected Auth: %+v", chargeReq.Auth)
	}
	if chargeReq.Body.Type != "json" || !strings.Contains(chargeReq.Body.Raw, "5000") {
		t.Errorf("unexpected Body: %+v", chargeReq.Body)
	}

	refundReq := res.Requests[1].Request
	if refundReq.Name != "Refund Charge" || refundReq.Auth.Type != "basic" {
		t.Errorf("unexpected Refund request: %+v", refundReq)
	}
	if refundReq.Body.Type != "urlEncoded" || len(refundReq.Body.UrlEncoded) != 2 {
		t.Errorf("unexpected UrlEncoded Body: %+v", refundReq.Body)
	}
}

func TestHARImport(t *testing.T) {
	data, err := os.ReadFile("testdata/sample.har")
	if err != nil {
		t.Fatalf("failed to read testdata/sample.har: %v", err)
	}

	res, err := ParseHAR(data)
	if err != nil {
		t.Fatalf("ParseHAR failed: %v", err)
	}

	if len(res.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(res.Requests))
	}

	r1 := res.Requests[0].Request
	if r1.Method != "GET" {
		t.Errorf("expected GET, got %s", r1.Method)
	}
	if r1.Auth.Type != "bearer" || r1.Auth.Token != "ghp_sampletoken12345" {
		t.Errorf("expected bearer token from Authorization header, got %+v", r1.Auth)
	}
	if len(r1.Params) != 2 {
		t.Errorf("expected 2 query params, got %d", len(r1.Params))
	}

	r2 := res.Requests[1].Request
	if r2.Method != "POST" || r2.Body.Type != "json" {
		t.Errorf("expected POST with JSON body, got %+v", r2)
	}
}

func TestPostmanV21EnhancedImport(t *testing.T) {
	data, err := os.ReadFile("testdata/postman_v21.json")
	if err != nil {
		t.Fatalf("failed to read testdata/postman_v21.json: %v", err)
	}

	res, err := ParsePostman(data)
	if err != nil {
		t.Fatalf("ParsePostman failed: %v", err)
	}

	if res.CollectionName != "Acme SaaS Platform" {
		t.Errorf("unexpected CollectionName %q", res.CollectionName)
	}
	if len(res.Variables) != 2 {
		t.Errorf("expected 2 collection variables, got %d", len(res.Variables))
	}
	if len(res.Folders) < 1 {
		t.Fatalf("expected at least 1 folder, got %d", len(res.Folders))
	}

	// Verify folder auth and folder scripts exist
	var authFolder *ImportedFolder
	for i := range res.Folders {
		if res.Folders[i].Name == "Auth" {
			authFolder = &res.Folders[i]
			break
		}
	}
	if authFolder == nil {
		t.Fatalf("folder 'Auth' not found")
	}
	if authFolder.Definition.Auth.Type != "bearer" || authFolder.Definition.Auth.Token != "acme_folder_bearer_token" {
		t.Errorf("folder Auth not preserved: %+v", authFolder.Definition.Auth)
	}
	if !strings.Contains(authFolder.Definition.Scripts.PreRequest, "pb.environment.set") {
		t.Errorf("folder script not converted to pb: %q", authFolder.Definition.Scripts.PreRequest)
	}

	if len(res.Requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(res.Requests))
	}

	// Check "Get Profile" request
	var getProfile *types.RequestDefinition
	for _, it := range res.Requests {
		if it.Name == "Get Profile" {
			getProfile = it.Request
			break
		}
	}
	if getProfile == nil {
		t.Fatalf("request 'Get Profile' not found")
	}

	// Check inherited auth: request had no auth, so it inherited folder's bearer token!
	if getProfile.Auth.Type != "bearer" || getProfile.Auth.Token != "acme_folder_bearer_token" {
		t.Errorf("expected inherited bearer auth, got %+v", getProfile.Auth)
	}

	// Check script conversion
	if !strings.Contains(getProfile.Scripts.PostResponse, "pb.test") {
		t.Errorf("expected PostResponse with pb.test, got %q", getProfile.Scripts.PostResponse)
	}
	if !strings.Contains(getProfile.Scripts.PostResponse, "Could not auto-convert: pm.sendRequest") {
		t.Errorf("expected comment for pm.sendRequest, got %q", getProfile.Scripts.PostResponse)
	}

	// Verify warnings were populated
	if len(res.Warnings) == 0 {
		t.Errorf("expected warnings for pm.sendRequest")
	}
}

func TestOpenAPIImport(t *testing.T) {
	data, err := os.ReadFile("testdata/openapi_v3.json")
	if err != nil {
		t.Fatalf("failed to read testdata/openapi_v3.json: %v", err)
	}

	res, err := ParseOpenAPI(data)
	if err != nil {
		t.Fatalf("ParseOpenAPI failed: %v", err)
	}

	if res.CollectionName != "Petstore Service API" {
		t.Errorf("expected 'Petstore Service API', got %q", res.CollectionName)
	}
	if len(res.Requests) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(res.Requests))
	}
}

func TestCURLImport(t *testing.T) {
	cmd := `curl -X POST https://api.example.com/items -H "Authorization: Bearer token123" -H "Content-Type: application/json" -d '{"name":"widget","price":9.99}'`
	res, err := ParseCURL(cmd)
	if err != nil {
		t.Fatalf("ParseCURL failed: %v", err)
	}

	if len(res.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(res.Requests))
	}
	req := res.Requests[0].Request
	if req.Method != "POST" || req.Auth.Type != "bearer" || req.Auth.Token != "token123" {
		t.Errorf("unexpected request: %+v", req)
	}
	if req.Body.Type != "json" || !strings.Contains(req.Body.Raw, "widget") {
		t.Errorf("unexpected body: %+v", req.Body)
	}
}

func TestExportPostman(t *testing.T) {
	items := []ImportedItem{
		{
			Name:    "Get Items",
			RelPath: "catalog/get-items.pebble.json",
			Request: &types.RequestDefinition{
				Name:   "Get Items",
				Method: "GET",
				URL:    "https://api.example.com/items",
				Auth:   types.AuthDefinition{Type: "bearer", Token: "abc"},
				Scripts: types.ScriptDefinition{
					PostResponse: `pb.test("Status 200", () => { pb.expect(pb.response.status).toBe(200); });`,
				},
			},
		},
	}
	folders := []ImportedFolder{
		{
			Name:    "catalog",
			RelPath: "catalog",
			Definition: types.FolderDefinition{
				Name: "catalog",
			},
		},
	}
	vars := []types.KeyValue{
		{Key: "baseUrl", Value: "https://api.example.com", Enabled: true},
	}

	data, err := ExportPostman("Test Collection", items, folders, vars)
	if err != nil {
		t.Fatalf("ExportPostman failed: %v", err)
	}

	str := string(data)
	if !strings.Contains(str, `"name": "Test Collection"`) {
		t.Errorf("missing collection name in export")
	}
	if !strings.Contains(str, `pm.expect(pm.response.code).to.equal(200)`) {
		t.Errorf("expected script converted back to pm.*, got:\n%s", str)
	}
	if !strings.Contains(str, `"key": "baseUrl"`) {
		t.Errorf("missing variable in export")
	}
}

func TestExportOpenAPI(t *testing.T) {
	items := []ImportedItem{
		{
			Name:    "Create User",
			RelPath: "users/create-user.pebble.json",
			Request: &types.RequestDefinition{
				Name:   "Create User",
				Method: "POST",
				URL:    "https://api.example.com/v1/users/:id",
				Params: []types.KeyValue{
					{Key: "notify", Value: "true", Enabled: true},
				},
				Body: types.BodyDefinition{
					Type: "json",
					Raw:  `{"name":"Bob","age":25,"active":true}`,
				},
			},
		},
	}

	data, err := ExportOpenAPI("Test API", "2.0.0", items)
	if err != nil {
		t.Fatalf("ExportOpenAPI failed: %v", err)
	}

	str := string(data)
	if !strings.Contains(str, `"openapi": "3.0.3"`) {
		t.Errorf("missing openapi version")
	}
	if !strings.Contains(str, `"/v1/users/{id}"`) {
		t.Errorf("expected path parameter converted to {id}, got:\n%s", str)
	}
	if !strings.Contains(str, `"name": "notify"`) {
		t.Errorf("missing query parameter notify")
	}
	if !strings.Contains(str, `"type": "integer"`) || !strings.Contains(str, `"type": "boolean"`) {
		t.Errorf("missing inferred JSON schema properties")
	}
}

func TestPanicSafetyAndMalformedInput(t *testing.T) {
	svc := NewService()

	malformedInputs := []struct {
		name    string
		content []byte
		format  Format
	}{
		{"corrupt JSON", []byte(`{ invalid json : 123 ]`), FormatPostman},
		{"empty byte slice", []byte(``), FormatInsomnia},
		{"null bytes", []byte("\x00\x00\x00\x00"), FormatHAR},
		{"random binary", []byte("\xff\xfe\x01\x02\x03"), FormatOpenAPI},
		{"truncated Bruno", []byte("meta {\n  name: Test\n"), FormatBruno},
		{"malformed cURL", []byte("curl --missing-arg"), FormatCURL},
		{"empty json object", []byte(`{}`), FormatPostman},
		{"empty insomnia resources", []byte(`{"resources":[]}`), FormatInsomnia},
		{"empty har entries", []byte(`{"log":{"entries":[]}}`), FormatHAR},
	}

	for _, tt := range malformedInputs {
		t.Run(tt.name, func(t *testing.T) {
			// This MUST NOT panic
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("Parse panicked on %s: %v", tt.name, r)
				}
			}()

			_, err := svc.Parse(tt.content, tt.name, tt.format)
			// Returning an error is expected and acceptable, but never a panic
			_ = err
		})
	}
}

func TestFileLimitEnforcement(t *testing.T) {
	svc := NewService()

	// Create dummy data exceeding MaxImportFileSize
	hugeData := make([]byte, MaxImportFileSize+10)

	_, err := svc.Parse(hugeData, "huge.json", FormatPostman)
	if err == nil {
		t.Fatalf("expected error for file exceeding MaxImportFileSize, got nil")
	}
	if !strings.Contains(err.Error(), "exceeds maximum allowed size") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestSaveToWorkspace(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "pebblepost_impexp_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	svc := NewService()
	res := &ImportResult{
		Format:         FormatPostman,
		CollectionName: "Integration Test Suite",
		Variables: []types.KeyValue{
			{Key: "env", Value: "prod", Enabled: true},
		},
		Folders: []ImportedFolder{
			{
				Name:    "auth",
				RelPath: "auth",
				Definition: types.FolderDefinition{
					Name: "auth",
					Auth: types.AuthDefinition{Type: "bearer", Token: "abc123token"},
				},
			},
		},
		Requests: []ImportedItem{
			{
				Name:    "Login",
				RelPath: "auth/login.pebble.json",
				Request: &types.RequestDefinition{
					Name:   "Login",
					Method: "POST",
					URL:    "https://api.example.com/login",
				},
			},
		},
	}

	report, err := svc.SaveToWorkspace(tmpDir, "collections/integration-test", res)
	if err != nil {
		t.Fatalf("SaveToWorkspace failed: %v", err)
	}

	if report.TotalRequests != 1 {
		t.Errorf("expected 1 total request in report, got %d", report.TotalRequests)
	}

	// Verify request file on disk
	reqFile := filepath.Join(tmpDir, "collections/integration-test/auth/login.pebble.json")
	if _, err := os.Stat(reqFile); os.IsNotExist(err) {
		t.Fatalf("request file was not created at %s", reqFile)
	}

	// Verify folder file on disk
	folderFile := filepath.Join(tmpDir, "collections/integration-test/auth/_folder.pebble.json")
	if _, err := os.Stat(folderFile); os.IsNotExist(err) {
		t.Fatalf("folder definition was not created at %s", folderFile)
	}
}

func TestPostman_URLReconstruction_HostAndPath(t *testing.T) {
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

	res, err := ParsePostman([]byte(rawJSON))
	if err != nil {
		t.Fatalf("ParsePostman error: %v", err)
	}
	if len(res.Requests) != 1 {
		t.Fatalf("expected 1 request, got %d", len(res.Requests))
	}
	req := res.Requests[0]
	if req.Request.URL != "{{BASE_URL}}/api/v1/users" {
		t.Errorf("want URL {{BASE_URL}}/api/v1/users, got %s", req.Request.URL)
	}
	if len(req.Request.Params) != 1 || req.Request.Params[0].Key != "limit" {
		t.Errorf("expected 1 query param, got %+v", req.Request.Params)
	}
}
