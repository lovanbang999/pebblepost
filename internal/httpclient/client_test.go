package httpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"pebblepost/internal/types"
)

func TestClient_AllHTTPMethods(t *testing.T) {
	client := NewClient()

	methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

	for _, method := range methods {
		t.Run("Method_"+method, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != method {
					t.Errorf("expected method %s, got %s", method, r.Method)
				}
				w.WriteHeader(http.StatusOK)
				if method != "HEAD" {
					_, _ = w.Write([]byte("method=" + method))
				}
			}))
			defer server.Close()

			req := &types.RequestDefinition{
				Method: method,
				URL:    server.URL,
			}

			result, err := client.Execute(context.Background(), req)
			if err != nil {
				t.Fatalf("unexpected error for method %s: %v", method, err)
			}

			if result.StatusCode != http.StatusOK {
				t.Errorf("expected status 200, got %d", result.StatusCode)
			}

			if method != "HEAD" && result.Body != "method="+method {
				t.Errorf("expected body 'method=%s', got '%s'", method, result.Body)
			}
		})
	}
}

func TestClient_QueryParams(t *testing.T) {
	client := NewClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("page") != "1" || q.Get("limit") != "20" || q.Get("filter") != "active" {
			t.Errorf("unexpected query params: %s", r.URL.RawQuery)
		}
		if q.Get("disabled") != "" {
			t.Errorf("disabled param should not be sent")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer server.Close()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL + "/search?page=1",
		Params: []types.KeyValue{
			{Key: "limit", Value: "20", Enabled: true},
			{Key: "filter", Value: "active", Enabled: true},
			{Key: "disabled", Value: "ignore", Enabled: false},
		},
	}

	res, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if res.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}
}

func TestClient_Headers(t *testing.T) {
	client := NewClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Custom-Header") != "CustomValue" {
			t.Errorf("missing or wrong X-Custom-Header: %s", r.Header.Get("X-Custom-Header"))
		}
		if r.Header.Get("Disabled-Header") != "" {
			t.Errorf("disabled header should not be present")
		}
		if r.Header.Get("User-Agent") != "PebblePost/1.0" {
			t.Errorf("expected default User-Agent PebblePost/1.0, got %s", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL,
		Headers: []types.KeyValue{
			{Key: "X-Custom-Header", Value: "CustomValue", Enabled: true},
			{Key: "Disabled-Header", Value: "No", Enabled: false},
		},
	}

	res, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if res.StatusCode != 200 {
		t.Errorf("expected status 200, got %d", res.StatusCode)
	}
}

func TestClient_Auth(t *testing.T) {
	client := NewClient()

	// 1. Bearer Token
	t.Run("BearerAuth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer secret-token-123" {
				t.Errorf("expected 'Bearer secret-token-123', got '%s'", r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    server.URL,
			Auth: types.AuthDefinition{
				Type:  "bearer",
				Token: "secret-token-123",
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("Bearer auth failed: %v, status: %d", err, res.StatusCode)
		}
	})

	// 2. Basic Auth
	t.Run("BasicAuth", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			expectedAuth := "Basic " + base64.StdEncoding.EncodeToString([]byte("admin:secretPass"))
			if r.Header.Get("Authorization") != expectedAuth {
				t.Errorf("expected '%s', got '%s'", expectedAuth, r.Header.Get("Authorization"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    server.URL,
			Auth: types.AuthDefinition{
				Type:     "basic",
				Username: "admin",
				Password: "secretPass",
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("Basic auth failed: %v, status: %d", err, res.StatusCode)
		}
	})

	// 3. API Key Header & Query
	t.Run("ApiKeyHeader", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("X-API-KEY") != "my-api-key" {
				t.Errorf("expected X-API-KEY header, got '%s'", r.Header.Get("X-API-KEY"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    server.URL,
			Auth: types.AuthDefinition{
				Type:  "apiKey",
				Key:   "X-API-KEY",
				Value: "my-api-key",
				AddTo: "header",
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("ApiKey header failed: %v", err)
		}
	})

	t.Run("ApiKeyQuery", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Query().Get("api_token") != "query-token-99" {
				t.Errorf("expected api_token query param, got '%s'", r.URL.Query().Get("api_token"))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    server.URL,
			Auth: types.AuthDefinition{
				Type:  "apiKey",
				Key:   "api_token",
				Value: "query-token-99",
				AddTo: "query",
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("ApiKey query failed: %v", err)
		}
	})
}

func TestClient_BodyTypes(t *testing.T) {
	client := NewClient()

	// 1. JSON Body
	t.Run("JSONBody", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
				t.Errorf("expected application/json, got %s", r.Header.Get("Content-Type"))
			}
			body, _ := io.ReadAll(r.Body)
			var data map[string]any
			_ = json.Unmarshal(body, &data)
			if data["name"] != "PebblePost" {
				t.Errorf("unexpected body payload: %s", string(body))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "json",
				Raw:  `{"name":"PebblePost"}`,
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("JSON body test failed: %v", err)
		}
	})

	// 2. URL Encoded
	t.Run("UrlEncodedBody", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			if r.FormValue("grant_type") != "password" || r.FormValue("username") != "testuser" {
				t.Errorf("unexpected urlencoded form: %v", r.Form)
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "urlEncoded",
				UrlEncoded: []types.KeyValue{
					{Key: "grant_type", Value: "password", Enabled: true},
					{Key: "username", Value: "testuser", Enabled: true},
					{Key: "ignored", Value: "disabled", Enabled: false},
				},
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("UrlEncoded body test failed: %v", err)
		}
	})

	// 3. Form Data (multipart)
	t.Run("FormData", func(t *testing.T) {
		tempDir := t.TempDir()
		testFile := filepath.Join(tempDir, "sample.txt")
		_ = os.WriteFile(testFile, []byte("hello upload world"), 0644)

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			err := r.ParseMultipartForm(10 << 20)
			if err != nil {
				t.Fatalf("failed to parse multipart form: %v", err)
			}
			if r.FormValue("title") != "Document A" {
				t.Errorf("form field title mismatch: %s", r.FormValue("title"))
			}

			file, header, err := r.FormFile("attachment")
			if err != nil {
				t.Fatalf("failed to read form file: %v", err)
			}
			defer file.Close()

			if header.Filename != "sample.txt" {
				t.Errorf("expected filename sample.txt, got %s", header.Filename)
			}

			content, _ := io.ReadAll(file)
			if string(content) != "hello upload world" {
				t.Errorf("unexpected file content: %s", string(content))
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "formData",
				FormData: []types.KeyValue{
					{Key: "title", Value: "Document A", Enabled: true, Type: "text"},
					{Key: "attachment", Value: testFile, Enabled: true, Type: "file"},
				},
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("FormData test failed: %v", err)
		}
	})

	// 4. GraphQL
	t.Run("GraphQL", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			var gqlPayload map[string]any
			_ = json.Unmarshal(body, &gqlPayload)

			if gqlPayload["query"] != "query GetUser { user { id name } }" {
				t.Errorf("unexpected GraphQL query: %v", gqlPayload["query"])
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":{"user":{"id":"1","name":"Alice"}}}`))
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "graphql",
				GraphQL: &types.GraphQL{
					Query:     "query GetUser { user { id name } }",
					Variables: `{"id": "1"}`,
				},
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("GraphQL test failed: %v", err)
		}
		if !strings.Contains(res.Body, "Alice") {
			t.Errorf("expected response to contain Alice, got %s", res.Body)
		}
	})
}

func TestClient_SettingsAndRedirects(t *testing.T) {
	client := NewClient()

	// 1. FollowRedirects = false
	t.Run("NoFollowRedirects", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/redirect" {
				http.Redirect(w, r, "/target", http.StatusFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("landed"))
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    server.URL + "/redirect",
			Settings: types.SettingDefinition{
				FollowRedirects: false,
			},
		}

		res, err := client.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("execute error: %v", err)
		}
		if res.StatusCode != http.StatusFound {
			t.Errorf("expected 302 Found without redirect, got %d", res.StatusCode)
		}
	})

	// 2. FollowRedirects = true
	t.Run("FollowRedirects", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/redirect" {
				http.Redirect(w, r, "/target", http.StatusFound)
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("landed"))
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    server.URL + "/redirect",
			Settings: types.SettingDefinition{
				FollowRedirects: true,
			},
		}

		res, err := client.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("execute error: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK after following redirect, got %d", res.StatusCode)
		}
		if res.Body != "landed" {
			t.Errorf("expected body 'landed', got '%s'", res.Body)
		}
	})

	// 2b. Cross-Origin Redirect strips sensitive headers
	t.Run("CrossOriginRedirect_StripsSensitiveHeaders", func(t *testing.T) {
		var receivedAuthHeader string
		var receivedCookieHeader string

		// Target server (different port/origin)
		targetServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAuthHeader = r.Header.Get("Authorization")
			receivedCookieHeader = r.Header.Get("Cookie")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("target reached"))
		}))
		defer targetServer.Close()

		// Origin server that redirects to targetServer
		originServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, targetServer.URL+"/redirected", http.StatusFound)
		}))
		defer originServer.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    originServer.URL + "/start",
			Headers: []types.KeyValue{
				{Key: "Authorization", Value: "Bearer secret-token", Enabled: true},
				{Key: "Cookie", Value: "session=secret-session-id", Enabled: true},
			},
			Settings: types.SettingDefinition{
				FollowRedirects: true,
			},
		}

		res, err := client.Execute(context.Background(), req)
		if err != nil {
			t.Fatalf("execute error: %v", err)
		}
		if res.StatusCode != http.StatusOK {
			t.Errorf("expected 200 OK after cross-origin redirect, got %d", res.StatusCode)
		}
		if receivedAuthHeader != "" {
			t.Errorf("expected Authorization header to be stripped across origins, but received: %q", receivedAuthHeader)
		}
		if receivedCookieHeader != "" {
			t.Errorf("expected Cookie header to be stripped across origins, but received: %q", receivedCookieHeader)
		}
	})

	// 3. TimeoutMs
	t.Run("TimeoutExceeded", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		}))
		defer server.Close()

		req := &types.RequestDefinition{
			Method: "GET",
			URL:    server.URL,
			Settings: types.SettingDefinition{
				TimeoutMs: 20, // 20ms < 100ms server sleep
			},
		}

		res, err := client.Execute(context.Background(), req)
		if err == nil {
			t.Errorf("expected timeout error, got nil")
		}
		if res == nil || res.Error == "" {
			t.Errorf("expected result with error message")
		}
	})
}

func TestClient_TimingMetrics(t *testing.T) {
	client := NewClient()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("response payload with timing"))
	}))
	defer server.Close()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL,
	}

	res, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}

	if res.Timing.TotalDurationMs <= 0 {
		t.Errorf("expected positive TotalDurationMs, got %f", res.Timing.TotalDurationMs)
	}
	if res.Size != int64(len("response payload with timing")) {
		t.Errorf("expected size %d, got %d", len("response payload with timing"), res.Size)
	}
}

func TestClient_DigestAuth(t *testing.T) {
	client := NewClient()

	const (
		expectedUser = "admin"
		expectedPass = "secret123"
		realm        = "pebble-secure"
		nonce        = "dcd98b7102dd2f0e8b11d0f600bfb0c093"
		opaque       = "5ccc069c403ebaf9f0171e9517f40e41"
	)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" || !strings.HasPrefix(authHeader, "Digest ") {
			w.Header().Set("WWW-Authenticate", fmt.Sprintf(`Digest realm="%s", nonce="%s", qop="auth", opaque="%s", algorithm="MD5"`, realm, nonce, opaque))
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte("Unauthorized"))
			return
		}

		// Verify digest response
		if !strings.Contains(authHeader, `username="admin"`) || !strings.Contains(authHeader, `realm="pebble-secure"`) {
			t.Errorf("digest header missing username or realm: %s", authHeader)
			w.WriteHeader(http.StatusForbidden)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Digest Auth Success"))
	}))
	defer server.Close()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL + "/digest-endpoint",
		Auth: types.AuthDefinition{
			Type:     "digest",
			Username: expectedUser,
			Password: expectedPass,
		},
	}

	result, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("digest auth request failed: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 after digest retry, got %d (body: %s)", result.StatusCode, result.Body)
	}
	if result.Body != "Digest Auth Success" {
		t.Errorf("expected body 'Digest Auth Success', got '%s'", result.Body)
	}
}

func TestClient_OAuth2ClientCredentials(t *testing.T) {
	client := NewClient()

	// Fake OAuth2 Token & Resource Server
	var issuedToken = "test-token-xyz-123"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/oauth/token" {
			_ = r.ParseForm()
			if r.FormValue("grant_type") != "client_credentials" {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = w.Write([]byte(`{"error":"unsupported_grant_type"}`))
				return
			}
			if r.FormValue("client_id") != "test-client" || r.FormValue("client_secret") != "test-secret" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"error":"invalid_client"}`))
				return
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"access_token": issuedToken,
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
			return
		}

		if r.URL.Path == "/api/resource" {
			authHeader := r.Header.Get("Authorization")
			if authHeader != "Bearer "+issuedToken {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte("Unauthorized resource"))
				return
			}
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"data":"protected-resource"}`))
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL + "/api/resource",
		Auth: types.AuthDefinition{
			Type:         "oauth2",
			GrantType:    "client_credentials",
			TokenURL:     server.URL + "/oauth/token",
			ClientID:     "test-client",
			ClientSecret: "test-secret",
			Scope:        "read",
		},
	}

	result, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("oauth2 request failed: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d (body: %s)", result.StatusCode, result.Body)
	}
	if !strings.Contains(result.Body, "protected-resource") {
		t.Errorf("expected resource data, got: %s", result.Body)
	}
}

func TestClient_OAuth2ExchangeCodePKCE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.FormValue("grant_type") != "authorization_code" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("code") != "valid-auth-code" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.FormValue("code_verifier") == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "pkce-access-token-999",
			"refresh_token": "pkce-refresh-token-888",
			"expires_in":    3600,
		})
	}))
	defer server.Close()

	mgr := GetOAuth2Manager()
	token, err := mgr.ExchangeCodePKCE(
		context.Background(),
		server.URL,
		"client-123",
		"secret-123",
		"valid-auth-code",
		"http://127.0.0.1:8080/callback",
		"test-verifier-abcdef",
	)
	if err != nil {
		t.Fatalf("exchange code failed: %v", err)
	}
	if token.AccessToken != "pkce-access-token-999" {
		t.Errorf("expected access token pkce-access-token-999, got %s", token.AccessToken)
	}
	if token.RefreshToken != "pkce-refresh-token-888" {
		t.Errorf("expected refresh token pkce-refresh-token-888, got %s", token.RefreshToken)
	}
}

func TestClient_AWSSigV4(t *testing.T) {
	client := NewClient()

	var receivedAuthHeader string
	var receivedDateHeader string
	var receivedContentSha string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuthHeader = r.Header.Get("Authorization")
		receivedDateHeader = r.Header.Get("X-Amz-Date")
		receivedContentSha = r.Header.Get("X-Amz-Content-Sha256")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("AWS OK"))
	}))
	defer server.Close()

	req := &types.RequestDefinition{
		Method: "POST",
		URL:    server.URL + "/items",
		Auth: types.AuthDefinition{
			Type:      "awsSigV4",
			AccessKey: "AKIAIOSFODNN7EXAMPLE",
			SecretKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
			Region:    "us-west-2",
			Service:   "s3",
		},
		Body: types.BodyDefinition{
			Type: "json",
			Raw:  `{"name":"pebble"}`,
		},
	}

	result, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("aws sigv4 request failed: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected 200 OK, got %d", result.StatusCode)
	}
	if !strings.HasPrefix(receivedAuthHeader, "AWS4-HMAC-SHA256 Credential=AKIAIOSFODNN7EXAMPLE/") {
		t.Errorf("invalid AWS Authorization header: %s", receivedAuthHeader)
	}
	if !strings.Contains(receivedAuthHeader, "us-west-2/s3/aws4_request") {
		t.Errorf("AWS Authorization header missing region/service scope: %s", receivedAuthHeader)
	}
	if receivedDateHeader == "" {
		t.Errorf("missing X-Amz-Date header")
	}
	if receivedContentSha == "" {
		t.Errorf("missing X-Amz-Content-Sha256 header")
	}
}

func TestClient_CookieJar(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "pebble-cookies-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	client := NewClient()
	client.SetWorkspace(tempDir)

	var requestCount int
	var receivedCookie string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		receivedCookie = r.Header.Get("Cookie")
		if requestCount == 1 {
			http.SetCookie(w, &http.Cookie{
				Name:  "session_token",
				Value: "token_abc123",
				Path:  "/",
			})
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("cookie ok"))
	}))
	defer server.Close()

	// 1. First request receives Set-Cookie
	req1 := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL + "/login",
	}
	_, err = client.Execute(context.Background(), req1)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}

	// 2. Second request should automatically send stored Cookie
	req2 := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL + "/profile",
	}
	_, err = client.Execute(context.Background(), req2)
	if err != nil {
		t.Fatalf("second request failed: %v", err)
	}
	if !strings.Contains(receivedCookie, "session_token=token_abc123") {
		t.Errorf("expected session_token cookie sent on second request, got: %q", receivedCookie)
	}

	// 3. Verify cookie persistence in .pebble/cookies.json
	cookieFile := filepath.Join(tempDir, ".pebble", "cookies.json")
	if _, err := os.Stat(cookieFile); os.IsNotExist(err) {
		t.Errorf("expected .pebble/cookies.json to exist on disk")
	}

	// 4. Per-request cookie toggle: when disabled, cookie must NOT be sent
	falseVal := false
	req3 := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL + "/no-cookies",
		Settings: types.SettingDefinition{
			EnableCookies: &falseVal,
		},
	}
	receivedCookie = ""
	_, err = client.Execute(context.Background(), req3)
	if err != nil {
		t.Fatalf("third request failed: %v", err)
	}
	if receivedCookie != "" {
		t.Errorf("expected no cookies when EnableCookies is false, got: %q", receivedCookie)
	}
}

func TestClient_CustomUserAgentAndSettings(t *testing.T) {
	client := NewClient()

	var receivedUA string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedUA = r.Header.Get("User-Agent")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	req := &types.RequestDefinition{
		Method: "GET",
		URL:    server.URL,
		Settings: types.SettingDefinition{
			UserAgent: "CustomAgent/2.5",
		},
	}

	_, err := client.Execute(context.Background(), req)
	if err != nil {
		t.Fatalf("execute failed: %v", err)
	}
	if receivedUA != "CustomAgent/2.5" {
		t.Errorf("expected custom user agent 'CustomAgent/2.5', got %q", receivedUA)
	}
}

func TestClient_FileBodyPathGuardAndAtFileSyntax(t *testing.T) {
	wsDir := t.TempDir()
	secretDir := t.TempDir()

	secretFile := filepath.Join(secretDir, "outside_secret.txt")
	_ = os.WriteFile(secretFile, []byte("super-secret-outside"), 0644)

	sampleFile := filepath.Join(wsDir, "sample.json")
	_ = os.WriteFile(sampleFile, []byte(`{"message":"safe workspace body"}`), 0644)

	client := NewClient()
	client.SetWorkspace(wsDir)

	var lastReceivedBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		bodyBytes, _ := io.ReadAll(r.Body)
		lastReceivedBody = string(bodyBytes)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	// 1. @file: syntax within workspace succeeds
	t.Run("Valid @file syntax inside workspace", func(t *testing.T) {
		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "json",
				Raw:  "@file:sample.json",
			},
		}
		res, err := client.Execute(context.Background(), req)
		if err != nil || res.StatusCode != 200 {
			t.Fatalf("execute failed: %v", err)
		}
		if lastReceivedBody != `{"message":"safe workspace body"}` {
			t.Errorf("expected body from sample.json, got: %s", lastReceivedBody)
		}
	})

	// 2. Traversal attempt with @file: escaping workspace fails
	t.Run("Traversal with @file escapes workspace rejected", func(t *testing.T) {
		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "json",
				Raw:  "@file:../../outside_secret.txt",
			},
		}
		_, err := client.Execute(context.Background(), req)
		if err == nil {
			t.Fatalf("expected path traversal error, got nil")
		}
		if !strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "traversal") {
			t.Errorf("expected security traversal error, got: %v", err)
		}
	})

	// 3. Absolute path pointing outside workspace fails
	t.Run("Absolute file path outside workspace rejected", func(t *testing.T) {
		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type:     "json",
				FilePath: secretFile,
			},
		}
		_, err := client.Execute(context.Background(), req)
		if err == nil {
			t.Fatalf("expected error for file outside workspace, got nil")
		}
		if !strings.Contains(err.Error(), "forbidden") && !strings.Contains(err.Error(), "outside workspace") {
			t.Errorf("expected security forbidden error, got: %v", err)
		}
	})

	// 4. Multipart form-data file outside workspace fails
	t.Run("Multipart file outside workspace rejected", func(t *testing.T) {
		req := &types.RequestDefinition{
			Method: "POST",
			URL:    server.URL,
			Body: types.BodyDefinition{
				Type: "formdata",
				FormData: []types.KeyValue{
					{Key: "file", Value: secretFile, Type: "file", Enabled: true},
				},
			},
		}
		_, err := client.Execute(context.Background(), req)
		if err == nil {
			t.Fatalf("expected error for multipart file outside workspace, got nil")
		}
	})
}

func TestPersistentJar_FilePermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("skipping POSIX file mode assertions on Windows")
	}

	wsDir := t.TempDir()
	jar := NewPersistentJar(wsDir)

	err := jar.SetCookie(types.CookieItem{
		Name:   "sid",
		Value:  "secret-session-token",
		Domain: "example.com",
		Path:   "/",
	})
	if err != nil {
		t.Fatalf("SetCookie failed: %v", err)
	}

	// Verify .pebble directory permissions 0700
	pebbleDir := filepath.Join(wsDir, ".pebble")
	dirFi, err := os.Stat(pebbleDir)
	if err != nil {
		t.Fatalf("stat .pebble dir failed: %v", err)
	}
	if perm := dirFi.Mode().Perm(); perm != 0700 {
		t.Errorf(".pebble dir mode = %o; want 0700", perm)
	}

	// Verify cookies.json file permissions 0600
	cookieFile := filepath.Join(pebbleDir, "cookies.json")
	fileFi, err := os.Stat(cookieFile)
	if err != nil {
		t.Fatalf("stat cookies.json failed: %v", err)
	}
	if perm := fileFi.Mode().Perm(); perm != 0600 {
		t.Errorf("cookies.json mode = %o; want 0600", perm)
	}
}
