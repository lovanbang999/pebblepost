package scripting

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

	res, err := engine.ExecutePreRequest(script, req, vars, 0)
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

	res, err := engine.ExecutePostResponse(script, req, resp, map[string]string{}, 0)
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

// TestEngine_InfiniteLoopIsInterrupted verifies that a script containing an
// infinite loop is interrupted by the configurable timeout.
func TestEngine_InfiniteLoopIsInterrupted(t *testing.T) {
	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}

	script := `while(true) {}`

	start := time.Now()
	_, err := engine.ExecutePreRequest(script, req, nil, 500*time.Millisecond)
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected timeout error for infinite-loop script, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected 'timed out' in error, got: %v", err)
	}
	// Should finish well within 2× the configured timeout.
	if elapsed > 2*time.Second {
		t.Errorf("script took too long to interrupt: %s", elapsed)
	}
}

// TestEngine_ConsoleCap verifies that console output is capped at maxConsoleLogs
// entries and that individual long entries are truncated.
func TestEngine_ConsoleCap(t *testing.T) {
	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}

	// Log maxConsoleLogs+50 messages; only maxConsoleLogs should survive.
	script := `
		for (var i = 0; i < 560; i++) {
			console.log("line " + i);
		}
	`
	res, err := engine.ExecutePreRequest(script, req, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Logs) > maxConsoleLogs {
		t.Errorf("expected ≤%d log entries, got %d", maxConsoleLogs, len(res.Logs))
	}
}

// TestEngine_ConsoleLongEntryTruncated verifies very long log lines are capped.
func TestEngine_ConsoleLongEntryTruncated(t *testing.T) {
	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}

	script := `console.log("x".repeat(10000));`
	res, err := engine.ExecutePreRequest(script, req, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Logs) == 0 {
		t.Fatal("expected at least one log entry")
	}
	if len(res.Logs[0]) > maxLogEntryLen+20 { // +20 for "…[truncated]"
		t.Errorf("log entry not truncated: length=%d", len(res.Logs[0]))
	}
}

// TestEngine_SandboxNoRequire verifies that scripts cannot call require() or
// access Node.js-style globals (fs, process, etc.).
func TestEngine_SandboxNoRequire(t *testing.T) {
	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}

	tests := []struct {
		name   string
		script string
	}{
		{"require not defined", `require('fs')`},
		{"process not defined", `process.exit(0)`},
		{"global fetch not defined", `fetch('http://example.com')`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := engine.ExecutePreRequest(tt.script, req, nil, 0)
			if err == nil {
				t.Errorf("%s: expected an error (sandbox should block access), got nil", tt.name)
			}
		})
	}
}

// TestEngine_ConsoleLevels verifies console.log/info/warn/error generate structured ConsoleLogEntry records.
func TestEngine_ConsoleLevels(t *testing.T) {
	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}

	script := `
		console.log("This is a log message", 123);
		console.info("This is an info message");
		console.warn("This is a warning");
		console.error("This is an error message");
	`

	res, err := engine.ExecutePreRequestNamed("CustomScript", script, req, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res.ConsoleLogs) != 4 {
		t.Fatalf("expected 4 ConsoleLog entries, got %d", len(res.ConsoleLogs))
	}

	levels := []string{"log", "info", "warn", "error"}
	for i, entry := range res.ConsoleLogs {
		if entry.Level != levels[i] {
			t.Errorf("entry %d: expected level %s, got %s", i, levels[i], entry.Level)
		}
		if entry.Source != "CustomScript" {
			t.Errorf("entry %d: expected source 'CustomScript', got %s", i, entry.Source)
		}
		if entry.Timestamp.IsZero() {
			t.Errorf("entry %d: expected non-zero timestamp", i)
		}
	}
}

// TestEngine_ExtendedPbAPIs verifies pb.uuid, pb.base64, pb.hash, pb.hmac, pb.jwt, pb.date, pb.random, pb.variables.
func TestEngine_ExtendedPbAPIs(t *testing.T) {
	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}

	script := `
		// 1. UUID
		const id = pb.uuid();
		console.log("UUID:", id);
		if (!id || id.length < 32) throw new Error("invalid UUID");

		// 2. Base64
		const b64 = pb.base64.encode("hello world");
		const raw = pb.base64.decode(b64);
		if (raw !== "hello world") throw new Error("base64 mismatch: " + raw);

		// 3. Hash
		const sha = pb.hash.sha256("test-data");
		const md5 = pb.hash.md5("test-data");
		if (sha.length !== 64 || md5.length !== 32) throw new Error("hash length mismatch");

		// 4. HMAC
		const hmac = pb.hmac.sha256("secret", "message");
		if (hmac.length !== 64) throw new Error("hmac length mismatch");

		// 5. JWT
		// Example JWT header={"alg":"HS256","typ":"JWT"} payload={"sub":"1234567890","name":"John Doe","admin":true}
		const fakeJwt = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIiwibmFtZSI6IkpvaG4gRG9lIiwiYWRtaW4iOnRydWV9.SflKxwRJSMeKKF2QT4fwpMeJf36POk6yJV_adQssw5c";
		const jwt = pb.jwt.decode(fakeJwt);
		if (!jwt.header || jwt.header.alg !== "HS256") throw new Error("jwt header mismatch");
		if (!jwt.payload || jwt.payload.name !== "John Doe" || !jwt.payload.admin) throw new Error("jwt payload mismatch");

		// 6. Date
		const nowMs = pb.date.now();
		if (typeof nowMs !== "number" || nowMs <= 0) throw new Error("invalid date now");
		const formatted = pb.date.format(1700000000000, "YYYY-MM-DD");
		if (formatted !== "2023-11-14") throw new Error("date format mismatch: " + formatted);
		const added = pb.date.add("2023-11-14T00:00:00Z", 2, "days");
		if (!added.startsWith("2023-11-16")) throw new Error("date add mismatch: " + added);

		// 7. Random
		const rInt = pb.random.int(10, 20);
		if (rInt < 10 || rInt > 20) throw new Error("random int out of range: " + rInt);
		const rStr = pb.random.string(8, "ABCDEF");
		if (rStr.length !== 8) throw new Error("random string length mismatch: " + rStr);

		// 8. Variables (run-local)
		pb.variables.set("runVar", "tempValue");
		if (pb.variables.get("runVar") !== "tempValue") throw new Error("variables get mismatch");
		if (!pb.variables.has("runVar")) throw new Error("variables has mismatch");
		pb.variables.unset("runVar");
		if (pb.variables.has("runVar")) throw new Error("variables unset failed");
	`

	res, err := engine.ExecutePreRequest(script, req, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v, logs: %v", err, res.Logs)
	}
}

// TestEngine_SendRequest verifies pb.sendRequest auxiliary HTTP execution.
func TestEngine_SendRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Auth") != "secret-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"received":true,"method":"` + r.Method + `"}`))
	}))
	defer ts.Close()

	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}

	script := fmt.Sprintf(`
		const res = pb.sendRequest({
			url: "%s",
			method: "POST",
			headers: { "X-Custom-Auth": "secret-token" },
			body: JSON.stringify({ ping: "pong" }),
			timeoutMs: 3000
		});

		console.log("Auxiliary response status:", res.status);
		if (res.status !== 200) throw new Error("expected status 200, got " + res.status);
		const data = res.json();
		if (!data.received || data.method !== "POST") throw new Error("json parse failed: " + JSON.stringify(data));
		if (!res.text().includes("received")) throw new Error("text() failed: " + res.text());
	`, ts.URL)

	res, err := engine.ExecutePreRequest(script, req, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v, logs: %v", err, res.Logs)
	}
}

func TestEngine_SendRequest_Security(t *testing.T) {
	engine := NewEngine()

	// 1. Call limit: maximum 5 calls per script execution
	t.Run("Call limit enforcement", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		defer ts.Close()

		script := fmt.Sprintf(`
			for (let i = 0; i < 6; i++) {
				pb.sendRequest({ url: "%s" });
			}
		`, ts.URL)
		req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}
		_, err := engine.ExecutePreRequest(script, req, nil, 0)
		if err == nil {
			t.Fatalf("expected error exceeding 5 calls, got nil")
		}
		if !strings.Contains(err.Error(), "limit exceeded") {
			t.Errorf("expected limit exceeded error, got: %v", err)
		}
	})

	// 2. SSRF block to cloud instance metadata
	t.Run("Cloud metadata SSRF blocked", func(t *testing.T) {
		script := `pb.sendRequest({ url: "http://169.254.169.254/latest/meta-data/" });`
		req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}
		_, err := engine.ExecutePreRequest(script, req, nil, 0)
		if err == nil {
			t.Fatalf("expected error targeting cloud metadata, got nil")
		}
		if !strings.Contains(err.Error(), "blocked") {
			t.Errorf("expected blocked error, got: %v", err)
		}
	})

	// 3. Secret masking in error messages
	t.Run("Secret masking in errors", func(t *testing.T) {
		secretToken := "SUPER_SECRET_API_TOKEN_999"
		// Port that won't connect
		script := fmt.Sprintf(`pb.sendRequest({ url: "http://127.0.0.1:59999/api?token=%s", timeoutMs: 500 });`, secretToken)
		req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}
		vars := map[string]string{"SECRET_KEY": secretToken}
		_, err := engine.ExecutePreRequest(script, req, vars, 0)
		if err == nil {
			t.Fatalf("expected connection error, got nil")
		}
		if strings.Contains(err.Error(), secretToken) {
			t.Errorf("error contains raw secret token: %v", err)
		}
		if !strings.Contains(err.Error(), "***") {
			t.Errorf("expected masked placeholder *** in error: %v", err)
		}
	})

	// 4. Redirect limit: maximum 3 redirects
	t.Run("Redirect limit capped at 3", func(t *testing.T) {
		var loopServer *httptest.Server
		loopServer = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, loopServer.URL+"/loop", http.StatusFound)
		}))
		defer loopServer.Close()

		script := fmt.Sprintf(`pb.sendRequest({ url: "%s" });`, loopServer.URL)
		req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}
		_, err := engine.ExecutePreRequest(script, req, nil, 0)
		if err == nil {
			t.Fatalf("expected error from redirect loop, got nil")
		}
		if !strings.Contains(err.Error(), "stopped after 3 redirects") {
			t.Errorf("expected stopped after 3 redirects, got: %v", err)
		}
	})

	// 5. TLS policy: insecureSkipVerify support
	t.Run("TLS insecureSkipVerify support", func(t *testing.T) {
		tlsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"tls":true}`))
		}))
		defer tlsServer.Close()

		script := fmt.Sprintf(`
			const res = pb.sendRequest({
				url: "%s",
				insecureSkipVerify: true
			});
			if (res.status !== 200) throw new Error("status: " + res.status);
		`, tlsServer.URL)
		req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}
		_, err := engine.ExecutePreRequest(script, req, nil, 0)
		if err != nil {
			t.Fatalf("expected insecureSkipVerify to succeed on self-signed cert: %v", err)
		}
	})
}

// TestEngine_NewMatchers verifies status, have.length, deep.equal, exist, oneOf, jsonSchema.
func TestEngine_NewMatchers(t *testing.T) {
	engine := NewEngine()
	req := &types.RequestDefinition{Method: "GET", URL: "http://example.com"}
	resp := &types.ExecutionResult{
		StatusCode: 200,
		StatusText: "OK",
		Headers:    map[string][]string{"content-type": {"application/json"}},
		Body:       `{"id": 42, "user": {"name": "Alice", "tags": ["admin", "staff"]}}`,
		Timing:     types.TimingMetrics{TotalDurationMs: 45.0},
	}

	script := `
		// Status matcher
		pb.test("Status is 200", function() {
			pb.expect(pb.response).to.have.status(200);
			pb.expect(pb.response.status).to.have.status(200);
			pb.expect(pb.response).to.not.have.status(404);
		});

		// Property matcher
		pb.test("Property checks", function() {
			const data = pb.response.json();
			pb.expect(data).to.have.property("id", 42);
			pb.expect(data.user).to.have.property("name", "Alice");
			pb.expect(data).to.not.have.property("nonexistent");
		});

		// Length matcher
		pb.test("Length checks", function() {
			const data = pb.response.json();
			pb.expect(data.user.tags).to.have.length(2);
			pb.expect("hello").to.have.length(5);
			pb.expect(data.user.tags).to.not.have.length(10);
		});

		// Deep equal matcher
		pb.test("Deep equal checks", function() {
			pb.expect({ a: 1, b: [2, 3] }).to.deep.equal({ a: 1, b: [2, 3] });
			pb.expect({ a: 1 }).to.not.deep.equal({ a: 2 });
		});

		// Exist matcher
		pb.test("Exist checks", function() {
			pb.expect("something").to.exist;
			pb.expect(0).to.exist;
			pb.expect(false).to.exist;
			pb.expect(null).to.not.exist;
			pb.expect(undefined).to.not.exist;
		});

		// OneOf matcher
		pb.test("OneOf checks", function() {
			pb.expect(200).to.be.oneOf([200, 201, 204]);
			pb.expect(404).to.not.be.oneOf([200, 201, 204]);
		});

		// JSON Schema matcher
		pb.test("JSON schema checks", function() {
			const data = pb.response.json();
			const schema = {
				type: "object",
				required: ["id", "user"],
				properties: {
					id: { type: "integer", minimum: 1 },
					user: {
						type: "object",
						required: ["name", "tags"],
						properties: {
							name: { type: "string", minLength: 2 },
							tags: { type: "array", minItems: 1, items: { type: "string" } }
						}
					}
				}
			};
			pb.expect(data).to.have.jsonSchema(schema);
		});
	`

	res, err := engine.ExecutePostResponse(script, req, resp, nil, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, test := range res.Tests {
		if !test.Passed {
			t.Errorf("test '%s' failed: %s", test.Name, test.Message)
		}
	}
}

// TestEngine_StaticSyntaxValidation verifies static detection of unsupported Goja syntax.
func TestEngine_StaticSyntaxValidation(t *testing.T) {
	tests := []struct {
		name           string
		script         string
		expectedErrSub string
	}{
		{"async function", "async function getData() { return 1; }", "'async' functions are not supported"},
		{"await expression", "const data = await pb.sendRequest('http://example.com');", "'await' expressions are not supported"},
		{"require call", "const fs = require('fs');", "'require()' is not available"},
		{"import statement", "import axios from 'axios';", "ES module 'import' statements are not supported"},
		{"export statement", "export const foo = 123;", "ES module 'export' statements are not supported"},
		{"generator function", "function* myGen() { yield 1; }", "Generator functions"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateScriptSyntax("testScript", tt.script)
			if err == nil {
				t.Fatalf("expected error containing '%s', got nil", tt.expectedErrSub)
			}
			if !strings.Contains(err.Error(), tt.expectedErrSub) {
				t.Errorf("expected error to contain '%s', got: %v", tt.expectedErrSub, err)
			}
		})
	}
}
