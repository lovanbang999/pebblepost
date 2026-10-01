package httpclient

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
