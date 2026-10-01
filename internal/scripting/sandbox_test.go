package scripting

import (
	"strings"
	"testing"

	"pebblepost/internal/types"
)

func TestEngine_PreRequestScript(t *testing.T) {
	engine := NewEngine()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    "https://api.example.com/v1/users",
		Headers: []types.KeyValue{
			{Key: "Existing-Header", Value: "123", Enabled: true},
		},
		Body: types.BodyDefinition{
			Type: "raw",
			Raw:  "initial body",
		},
	}

	vars := map[string]string{
		"SECRET_KEY": "my-secret-key",
	}

	script := `
		console.log("Running pre-request script for:", pb.request.url);
		const secret = pb.environment.get("SECRET_KEY");
		const hash = pb.crypto.sha256(secret + ":salt");
		
		pb.request.headers.set("X-Signature", hash);
		pb.request.headers.add("X-Request-ID", pb.crypto.uuid());
		pb.environment.set("LAST_SIGNATURE", hash);
		pb.request.body.setRaw("updated body payload");
	`

	res, err := engine.ExecutePreRequest(script, req, vars)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1. Check Header updates
	var sigHeader, reqIdHeader string
	for _, h := range res.Request.Headers {
		if h.Key == "X-Signature" {
			sigHeader = h.Value
		}
		if h.Key == "X-Request-ID" {
			reqIdHeader = h.Value
		}
	}

	if sigHeader == "" {
		t.Errorf("expected X-Signature header to be set")
	}
	if reqIdHeader == "" || len(reqIdHeader) < 30 {
		t.Errorf("expected X-Request-ID header to contain UUID")
	}

	// 2. Check Body update
	if res.Request.Body.Raw != "updated body payload" {
		t.Errorf("expected body to be 'updated body payload', got '%s'", res.Request.Body.Raw)
	}

	// 3. Check extracted env var
	if res.ExtractedEnvVars["LAST_SIGNATURE"] != sigHeader {
		t.Errorf("expected extracted var LAST_SIGNATURE to match %s", sigHeader)
	}

	// 4. Check console logs
	if len(res.Logs) == 0 || !strings.Contains(res.Logs[0], "Running pre-request script") {
		t.Errorf("expected console log output, got %v", res.Logs)
	}
}

func TestEngine_PostResponseScriptAndAssertions(t *testing.T) {
	engine := NewEngine()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    "https://api.example.com/v1/auth",
	}

	resp := &types.ExecutionResult{
		StatusCode: 200,
		StatusText: "200 OK",
		Headers: map[string][]string{
			"Content-Type": {"application/json; charset=utf-8"},
		},
		Body: `{"status":"success","user":{"id":100,"name":"Alice","roles":["admin","editor"]},"token":"jwt-token-xyz"}`,
		Timing: types.TimingMetrics{
			TotalDurationMs: 85.5,
		},
	}

	script := `
		console.log("Analyzing response status:", pb.response.status);

		pb.test("Status code is 200", function() {
			pb.expect(pb.response.status).to.equal(200);
		});

		pb.test("Response time is below 500ms", function() {
			pb.expect(pb.response.responseTime).to.be.below(500);
		});

		pb.test("Response body has valid user object", function() {
			const json = pb.response.json();
			pb.expect(json).to.have.property("user");
			pb.expect(json.user.id).to.equal(100);
			pb.expect(json.user.name).to.equal("Alice");
			pb.expect(json.user.roles).to.be.an("array");
			pb.expect(json.user.roles).to.include("admin");
			
			// Save token to environment
			pb.environment.set("AUTH_TOKEN", json.token);
		});

		pb.test("Should intentionally fail test", function() {
			pb.expect(pb.response.status).to.equal(404);
		});
	`

	res, err := engine.ExecutePostResponse(script, req, resp, map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.Tests) != 4 {
		t.Fatalf("expected 4 tests, got %d", len(res.Tests))
	}

	// Test 1: Pass
	if !res.Tests[0].Passed || res.Tests[0].Name != "Status code is 200" {
		t.Errorf("Test 1 failed: %v", res.Tests[0])
	}

	// Test 2: Pass
	if !res.Tests[1].Passed {
		t.Errorf("Test 2 failed: %v", res.Tests[1])
	}

	// Test 3: Pass
	if !res.Tests[2].Passed {
		t.Errorf("Test 3 failed: %v", res.Tests[2])
	}

	// Test 4: Fail
	if res.Tests[3].Passed {
		t.Errorf("Test 4 should have failed")
	}
	if !strings.Contains(res.Tests[3].Message, "expected 200 to equal 404") {
		t.Errorf("unexpected failure message: %s", res.Tests[3].Message)
	}

	// Check saved environment variable
	if res.ExtractedEnvVars["AUTH_TOKEN"] != "jwt-token-xyz" {
		t.Errorf("expected AUTH_TOKEN to be 'jwt-token-xyz', got '%s'", res.ExtractedEnvVars["AUTH_TOKEN"])
	}
}

func TestEngine_CryptoModule(t *testing.T) {
	crypto := NewCryptoModule()

	// 1. MD5
	md5Res := crypto.MD5("hello world")
	if md5Res != "5eb63bbbe01eeed093cb22bb8f5acdc3" {
		t.Errorf("MD5 mismatch: got %s", md5Res)
	}

	// 2. SHA256
	sha256Res := crypto.SHA256("hello world")
	if sha256Res != "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9" {
		t.Errorf("SHA256 mismatch: got %s", sha256Res)
	}

	// 3. HMAC-SHA256
	hmacRes, err := crypto.HMAC("sha256", "secret-key", "message to sign")
	if err != nil || hmacRes == "" {
		t.Fatalf("HMAC failed: %v", err)
	}

	// 4. UUID
	uuidStr := crypto.UUID()
	if len(uuidStr) != 36 || strings.Count(uuidStr, "-") != 4 {
		t.Errorf("invalid UUID generated: %s", uuidStr)
	}

	// 5. Base64
	b64Encoded := crypto.Base64Encode("hello base64")
	b64Decoded, err := crypto.Base64Decode(b64Encoded)
	if err != nil || b64Decoded != "hello base64" {
		t.Errorf("base64 roundtrip failed: %v, got '%s'", err, b64Decoded)
	}
}
