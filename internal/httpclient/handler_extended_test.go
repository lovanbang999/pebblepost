package httpclient

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"pebblepost/internal/history"
	"pebblepost/internal/scripting"
	"pebblepost/internal/types"
	"pebblepost/internal/workspace"
)

func TestHandler_CookiesAndHistory(t *testing.T) {
	tempDir := t.TempDir()
	wsSvc := workspace.NewWorkspaceService()
	envSvc := workspace.NewEnvironmentService()
	interpolator := workspace.NewInterpolator()
	scriptEngine := scripting.NewEngine()

	histStore, err := history.Open(tempDir)
	if err != nil {
		t.Fatalf("history.Open: %v", err)
	}
	defer histStore.Close()

	client := NewClient()
	client.SetWorkspace(tempDir)

	handler := NewHandler(client, wsSvc, envSvc, interpolator, scriptEngine, histStore)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Target server for request execution
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"hello":"world"}`))
	}))
	defer ts.Close()

	// 1. handleExecute with historyStore active (covers buildHistoryInput)
	execPayload := ExecutePayload{
		WorkspacePath: tempDir,
		Request: &types.RequestDefinition{
			Name:   "HistoryReq",
			Method: "GET",
			URL:    ts.URL,
		},
	}
	execBody, _ := json.Marshal(execPayload)
	req := httptest.NewRequest(http.MethodPost, "/api/request/execute", bytes.NewReader(execBody))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleExecute failed: %d", w.Code)
	}

	// Verify history was recorded (recorded asynchronously in background goroutine)
	var entries []history.HistoryEntry
	var total int
	for i := 0; i < 25; i++ {
		entries, total, err = histStore.List(history.ListFilter{WorkspacePath: tempDir})
		if err == nil && total > 0 && len(entries) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil || total < 1 || len(entries) < 1 {
		t.Fatalf("expected history entry recorded, got total=%d, err=%v", total, err)
	}

	// 2. handleCookies - POST (add cookie)
	cookieItem := types.CookieItem{
		Domain: "example.com",
		Path:   "/",
		Name:   "session_id",
		Value:  "xyz123",
	}
	cookieBody, _ := json.Marshal(map[string]any{
		"workspacePath": tempDir,
		"cookie":        cookieItem,
	})
	req = httptest.NewRequest(http.MethodPost, "/api/cookies", bytes.NewReader(cookieBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleCookies POST failed: %d", w.Code)
	}

	// GET cookies
	req = httptest.NewRequest(http.MethodGet, "/api/cookies?workspacePath="+tempDir, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleCookies GET failed: %d", w.Code)
	}
	var groupedCookies map[string][]types.CookieItem
	if err := json.NewDecoder(w.Body).Decode(&groupedCookies); err != nil {
		t.Fatalf("decode cookies failed: %v", err)
	}
	if len(groupedCookies["example.com"]) != 1 {
		t.Fatalf("expected 1 cookie for example.com, got %d", len(groupedCookies["example.com"]))
	}

	// DELETE cookie by name
	req = httptest.NewRequest(http.MethodDelete, "/api/cookies?workspacePath="+tempDir+"&domain=example.com&path=/&name=session_id", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleCookies DELETE single cookie failed: %d", w.Code)
	}

	// Add 2 cookies for domain clear test
	jar := client.GetCookieJar()
	_ = jar.SetCookie(types.CookieItem{Domain: "sub.example.com", Path: "/", Name: "c1", Value: "v1"})
	_ = jar.SetCookie(types.CookieItem{Domain: "sub.example.com", Path: "/", Name: "c2", Value: "v2"})

	// DELETE by domain (ClearDomain)
	req = httptest.NewRequest(http.MethodDelete, "/api/cookies?workspacePath="+tempDir+"&domain=sub.example.com", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleCookies DELETE domain failed: %d", w.Code)
	}

	// Add cookie and test ClearAll & Load
	_ = jar.SetCookie(types.CookieItem{Domain: "other.com", Path: "/", Name: "c3", Value: "v3"})
	if err := jar.Load(); err != nil {
		t.Fatalf("jar.Load failed: %v", err)
	}

	// DELETE all cookies
	req = httptest.NewRequest(http.MethodDelete, "/api/cookies?workspacePath="+tempDir, nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleCookies DELETE all failed: %d", w.Code)
	}
}

func TestHandler_OAuth2Token(t *testing.T) {
	// Mock OAuth2 token endpoint
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		grantType := r.FormValue("grant_type")
		if grantType == "client_credentials" || grantType == "authorization_code" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "access-tok-abc",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "refresh-tok-xyz",
			})
			return
		}
		if grantType == "refresh_token" {
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token":  "refreshed-tok-def",
				"token_type":    "Bearer",
				"expires_in":    3600,
				"refresh_token": "refresh-tok-xyz-2",
			})
			return
		}
		http.Error(w, "unsupported grant", http.StatusBadRequest)
	}))
	defer tokenServer.Close()

	client := NewClient()
	handler := NewHandler(client, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test handleOAuth2Token via HTTP
	payload := map[string]any{
		"auth": types.AuthDefinition{
			Type:         "oauth2",
			GrantType:    "client_credentials",
			TokenURL:     tokenServer.URL,
			ClientID:     "my-client",
			ClientSecret: "my-secret",
		},
	}
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/api/oauth2/token", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleOAuth2Token failed: %d: %s", w.Code, w.Body.String())
	}

	var tokenResp OAuth2Token
	if err := json.NewDecoder(w.Body).Decode(&tokenResp); err != nil {
		t.Fatalf("decode token failed: %v", err)
	}
	if tokenResp.AccessToken != "access-tok-abc" {
		t.Fatalf("expected access-tok-abc, got %s", tokenResp.AccessToken)
	}

	// Test RefreshToken directly on OAuth2Manager
	mgr := GetOAuth2Manager()
	refreshed, err := mgr.RefreshToken(context.Background(), tokenServer.URL, "my-client", "my-secret", "refresh-tok-xyz")
	if err != nil {
		t.Fatalf("RefreshToken failed: %v", err)
	}
	if refreshed.AccessToken != "refreshed-tok-def" {
		t.Fatalf("expected refreshed-tok-def, got %s", refreshed.AccessToken)
	}

	// Test ExchangeCodePKCE directly on OAuth2Manager
	exchanged, err := mgr.ExchangeCodePKCE(context.Background(), tokenServer.URL, "my-client", "my-secret", "code-123", "http://127.0.0.1/cb", "verifier-abc")
	if err != nil {
		t.Fatalf("ExchangeCodePKCE failed: %v", err)
	}
	if exchanged.AccessToken != "access-tok-abc" {
		t.Fatalf("expected access-tok-abc, got %s", exchanged.AccessToken)
	}
}

func TestHandler_StreamAndGrpcEndpoints(t *testing.T) {
	client := NewClient()
	handler := NewHandler(client, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. handleStreamStatus on non-existent stream
	req := httptest.NewRequest(http.MethodGet, "/api/stream/status?streamId=missing-123", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleStreamStatus failed: %d", w.Code)
	}
	var status types.StreamSessionStatus
	_ = json.NewDecoder(w.Body).Decode(&status)
	if status.State != "disconnected" {
		t.Fatalf("expected disconnected state, got %s", status.State)
	}

	// 1. handleStreamStatus: missing streamId -> 400
	req = httptest.NewRequest(http.MethodGet, "/api/stream/status", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing streamId on status, got %d", w.Code)
	}

	// 2. handleStreamSend: GET -> 405
	req = httptest.NewRequest(http.MethodGet, "/api/stream/send", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET on /api/stream/send, got %d", w.Code)
	}

	// handleStreamSend: missing streamId -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/stream/send", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty streamId on send, got %d", w.Code)
	}

	// handleStreamSend on missing session -> 400
	sendBody, _ := json.Marshal(map[string]string{
		"streamId": "missing-123",
		"payload":  "hello",
		"type":     "text",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/stream/send", bytes.NewReader(sendBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for send on missing stream, got %d", w.Code)
	}

	// 3. handleStreamDisconnect: GET -> 405
	req = httptest.NewRequest(http.MethodGet, "/api/stream/disconnect", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET on /api/stream/disconnect, got %d", w.Code)
	}

	// handleStreamDisconnect: missing streamId -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/stream/disconnect", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty streamId on disconnect, got %d", w.Code)
	}

	// handleStreamDisconnect on missing stream -> success
	discBody, _ := json.Marshal(map[string]string{
		"streamId": "missing-123",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/stream/disconnect", bytes.NewReader(discBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleStreamDisconnect failed: %d", w.Code)
	}

	// Helper function scriptTimeoutFor test
	if scriptTimeoutFor(0) <= 0 || scriptTimeoutFor(100) != 100*time.Millisecond {
		t.Fatal("unexpected scriptTimeoutFor duration")
	}

	// 4. handleGrpcStreamCancel on missing stream
	cancelBody, _ := json.Marshal(map[string]string{
		"streamId": "missing-grpc-123",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/stream/cancel", bytes.NewReader(cancelBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("handleGrpcStreamCancel failed: %d", w.Code)
	}

	// 5. handleGrpcSampleMessage validation
	sampleBody, _ := json.Marshal(map[string]string{
		"endpoint": "localhost:50051",
	})
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/sample-message", bytes.NewReader(sampleBody))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	// Missing service/method returns 400
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for incomplete sample message request, got %d", w.Code)
	}
}

func TestClient_ExecuteStream(t *testing.T) {
	sseServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			return
		}
		_, _ = w.Write([]byte("data: test-stream-msg\n\n"))
		flusher.Flush()
	}))
	defer sseServer.Close()

	client := NewClient()
	req := &types.RequestDefinition{
		Protocol: "sse",
		URL:      sseServer.URL,
		Stream: &types.StreamDefinition{
			TimeoutMs:       500,
			MaxWaitMessages: 1,
		},
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	result, err := client.Execute(ctx, req)
	if err != nil {
		t.Fatalf("Execute stream failed: %v", err)
	}
	if result.StatusCode != 200 {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
}

func TestHandler_StreamConnect(t *testing.T) {
	client := NewClient()
	handler := NewHandler(client, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Stream connect with invalid request body -> 400
	req := httptest.NewRequest(http.MethodPost, "/api/stream/connect", bytes.NewReader([]byte(`{}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty request definition, got %d", w.Code)
	}
}

func TestClient_BuildBodyBytes(t *testing.T) {
	client := NewClient()

	// 1. External file reference
	tmpFile, err := os.CreateTemp("", "body-ext-*.txt")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	_, _ = tmpFile.WriteString("external file content")
	_ = tmpFile.Close()

	data, ctype, err := client.buildBodyBytes(types.BodyDefinition{
		Type:     "raw",
		FilePath: tmpFile.Name(),
	})
	if err != nil || string(data) != "external file content" || ctype != "text/plain" {
		t.Fatalf("external file test failed: data=%s, ctype=%s, err=%v", string(data), ctype, err)
	}

	// 2. Type "file"
	data, ctype, err = client.buildBodyBytes(types.BodyDefinition{
		Type: "file",
		Raw:  tmpFile.Name(),
	})
	if err != nil || string(data) != "external file content" || ctype != "application/octet-stream" {
		t.Fatalf("type file test failed: data=%s, ctype=%s, err=%v", string(data), ctype, err)
	}

	// 3. Type "urlencoded"
	data, ctype, err = client.buildBodyBytes(types.BodyDefinition{
		Type: "urlencoded",
		UrlEncoded: []types.KeyValue{
			{Key: "foo", Value: "bar baz", Enabled: true},
			{Key: "disabled", Value: "ignore", Enabled: false},
		},
	})
	if err != nil || string(data) != "foo=bar+baz" || ctype != "application/x-www-form-urlencoded" {
		t.Fatalf("urlencoded test failed: data=%s, ctype=%s", string(data), ctype)
	}

	// 4. Type "formdata" with text and file
	data, ctype, err = client.buildBodyBytes(types.BodyDefinition{
		Type: "formdata",
		FormData: []types.KeyValue{
			{Key: "field1", Value: "val1", Enabled: true},
			{Key: "upload", Value: tmpFile.Name(), Type: "file", Enabled: true},
		},
	})
	if err != nil || len(data) == 0 || !strings.Contains(ctype, "multipart/form-data") {
		t.Fatalf("formdata test failed: len=%d, ctype=%s", len(data), ctype)
	}
}

func TestAWSSigV4_WithQueryParams(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "https://s3.amazonaws.com/mybucket?list-type=2&prefix=photos%2F", nil)
	auth := types.AuthDefinition{
		Type:         "awsSigV4",
		AccessKey:    "AKIAIOSFODNN7EXAMPLE",
		SecretKey:    "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:       "us-east-1",
		Service:      "s3",
		SessionToken: "session-token-test",
	}

	err := SignAWSSigV4(req, auth, []byte(""), time.Now())
	if err != nil {
		t.Fatalf("SignAWSSigV4 failed: %v", err)
	}

	authHeader := req.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "AWS4-HMAC-SHA256") {
		t.Fatalf("expected AWS4-HMAC-SHA256 header, got %s", authHeader)
	}
	if req.Header.Get("X-Amz-Security-Token") != "session-token-test" {
		t.Fatalf("expected session token header")
	}
}

func TestDigest_SHA256(t *testing.T) {
	challengeStr := `Digest realm="testrealm", nonce="dcd98b7102dd2f0e8b11d0f600bfb0c093", qop="auth", algorithm="SHA-256"`
	challenge, err := ParseDigestChallenge(challengeStr)
	if err != nil {
		t.Fatalf("ParseDigestChallenge failed: %v", err)
	}
	authHeader, err := BuildDigestAuthorization(http.MethodGet, "/dir/index.html", "Mufasa", "Circle Of Life", challenge, []byte(""))
	if err != nil {
		t.Fatalf("BuildDigestAuthorization with SHA-256 failed: %v", err)
	}
	if !strings.Contains(authHeader, `username="Mufasa"`) || !strings.Contains(authHeader, `response=`) {
		t.Fatalf("unexpected digest auth header: %s", authHeader)
	}
}

func TestCookieJar_Matches(t *testing.T) {
	if !domainMatches("sub.example.com", "example.com") {
		t.Error("expected sub.example.com to match example.com")
	}
	if domainMatches("notexample.com", "example.com") {
		t.Error("expected notexample.com not to match example.com")
	}
	if !pathMatches("/api/v1/users", "/api") {
		t.Error("expected /api/v1/users to match /api")
	}
	if pathMatches("/apiv1", "/api") {
		t.Error("expected /apiv1 not to match /api")
	}
}

func TestHandler_GrpcAndOAuth2Handlers(t *testing.T) {
	client := NewClient()
	if client.GetGrpcClient() == nil {
		t.Fatal("expected non-nil GrpcClient")
	}

	handler := NewHandler(client, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// 1. handleOAuth2Authorize: Method not allowed on GET
	req := httptest.NewRequest(http.MethodGet, "/api/oauth2/authorize", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 for GET on /api/oauth2/authorize, got %d", w.Code)
	}

	// 2. handleOAuth2Authorize: Invalid payload
	req = httptest.NewRequest(http.MethodPost, "/api/oauth2/authorize", bytes.NewReader([]byte(`{invalid`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid payload, got %d", w.Code)
	}

	// 3. handleGrpcReflectServices: Missing address
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/reflect/services", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing address, got %d", w.Code)
	}

	// 4. handleGrpcReflectServices: Unreachable address -> 500
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/reflect/services", bytes.NewReader([]byte(`{"address":"127.0.0.1:1"}`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for reflection failure, got %d", w.Code)
	}

	// 5. handleGrpcProtoServices: Missing protoFiles -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/proto/services", bytes.NewReader([]byte(`{}`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing protoFiles, got %d", w.Code)
	}

	// 6. handleGrpcProtoServices: Nonexistent protoFiles -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/proto/services", bytes.NewReader([]byte(`{"protoFiles":["/tmp/nonexistent.proto"]}`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid proto file, got %d", w.Code)
	}
}

func TestClient_BuildTLSConfig(t *testing.T) {
	client := NewClient()

	cfg, err := client.buildTLSConfig(types.SettingDefinition{VerifySSL: false})
	if err != nil || !cfg.InsecureSkipVerify {
		t.Fatalf("expected InsecureSkipVerify true, got %v, err=%v", cfg.InsecureSkipVerify, err)
	}

	_, err = client.buildTLSConfig(types.SettingDefinition{
		ClientCertPath: "/tmp/nonexistent.crt",
		ClientKeyPath:  "/tmp/nonexistent.key",
	})
	if err == nil {
		t.Fatal("expected error for nonexistent certs")
	}
}

func TestClient_ExecuteDigestAuthRetry(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth == "" {
			w.Header().Set("WWW-Authenticate", `Digest realm="pebble-test", nonce="test-nonce-12345", qop="auth"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if strings.HasPrefix(auth, "Digest ") && strings.Contains(auth, `username="mufasa"`) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"authenticated":true}`))
			return
		}
		w.WriteHeader(http.StatusForbidden)
	}))
	defer ts.Close()

	client := NewClient()
	req := &types.RequestDefinition{
		Method: "GET",
		URL:    ts.URL + "/secret",
		Auth: types.AuthDefinition{
			Type:     "digest",
			Username: "mufasa",
			Password: "password123",
		},
	}

	result, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute with digest failed: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 after digest retry, got %d", result.StatusCode)
	}
}

func TestClient_ExecuteApiKeyQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("apiKey") == "my-secret-key" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer ts.Close()

	client := NewClient()
	req := &types.RequestDefinition{
		Method: "GET",
		URL:    ts.URL + "/data",
		Auth: types.AuthDefinition{
			Type:  "apiKey",
			Key:   "apiKey",
			Value: "my-secret-key",
			AddTo: "query",
		},
	}

	result, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute with apiKey in query failed: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
}

func TestClient_ExecuteAWSSigV4(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "AWS4-HMAC-SHA256") {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer ts.Close()

	client := NewClient()
	req := &types.RequestDefinition{
		Method: "POST",
		URL:    ts.URL + "/api",
		Auth: types.AuthDefinition{
			Type:      "awsSigV4",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
			SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			Region:    "us-east-1",
			Service:   "execute-api",
		},
	}

	result, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("Execute with AWS SigV4 failed: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", result.StatusCode)
	}
}

func TestOAuth2_GenerateRandomString(t *testing.T) {
	s, err := generateRandomString(32)
	if err != nil || len(s) != 32 {
		t.Fatalf("generateRandomString failed: s=%s, len=%d, err=%v", s, len(s), err)
	}

	mgr := GetOAuth2Manager()
	if _, err := mgr.AuthorizeCodePKCE(context.Background(), nil); err == nil {
		t.Fatal("expected error on nil auth")
	}
	if _, err := mgr.AuthorizeCodePKCE(context.Background(), &types.AuthDefinition{}); err == nil {
		t.Fatal("expected error on empty auth URLs")
	}
}

func TestHandler_GrpcStreamValidation(t *testing.T) {
	client := NewClient()
	handler := NewHandler(client, nil, nil, nil, nil, nil)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// GET -> 405
	req := httptest.NewRequest(http.MethodGet, "/api/grpc/stream", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}

	// POST with invalid body -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/stream", bytes.NewReader([]byte(`{invalid`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	// POST without grpc request -> 400
	req = httptest.NewRequest(http.MethodPost, "/api/grpc/stream", bytes.NewReader([]byte(`{"request":{}}`)))
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}
