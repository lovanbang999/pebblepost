package httpclient

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"pebblepost/internal/types"
)

// ─── Large body / streaming ───────────────────────────────────────────────────

func TestClient_LargeBodyTruncation(t *testing.T) {
	largeBody := strings.Repeat("A", bodyDisplayThreshold+100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = io.WriteString(w, largeBody)
	}))
	defer srv.Close()

	result, err := NewClient().Execute(context.Background(), &types.RequestDefinition{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.BodyTruncated {
		t.Error("expected BodyTruncated=true for large body")
	}
	if int64(len(result.Body)) != bodyDisplayThreshold {
		t.Errorf("expected inline body=%d bytes, got %d", bodyDisplayThreshold, len(result.Body))
	}
	if result.TempBodyFile == "" {
		t.Error("expected TempBodyFile to be set")
	}
	if result.BodySizeBytes != int64(len(largeBody)) {
		t.Errorf("expected BodySizeBytes=%d, got %d", len(largeBody), result.BodySizeBytes)
	}
	raw, rerr := os.ReadFile(result.TempBodyFile)
	if rerr != nil {
		t.Fatalf("cannot read temp file: %v", rerr)
	}
	if string(raw) != largeBody {
		t.Errorf("temp file length mismatch: got %d, want %d", len(raw), len(largeBody))
	}
	_ = os.Remove(result.TempBodyFile)
}

func TestClient_SmallBodyNotTruncated(t *testing.T) {
	const body = `{"ok":true}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()

	result, err := NewClient().Execute(context.Background(), &types.RequestDefinition{Method: "GET", URL: srv.URL})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.BodyTruncated {
		t.Error("expected BodyTruncated=false for small body")
	}
	if result.Body != body {
		t.Errorf("body mismatch: got %q, want %q", result.Body, body)
	}
}

// ─── Redirect chain ───────────────────────────────────────────────────────────

func TestClient_RedirectChainCaptured(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/hop1":
			http.Redirect(w, r, srv.URL+"/hop2", http.StatusFound)
		case "/hop2":
			http.Redirect(w, r, srv.URL+"/final", http.StatusMovedPermanently)
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	result, err := NewClient().Execute(context.Background(), &types.RequestDefinition{
		Method: "GET", URL: srv.URL + "/hop1",
		Settings: types.SettingDefinition{FollowRedirects: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("expected final 200, got %d", result.StatusCode)
	}
	if len(result.RedirectChain) != 2 {
		t.Errorf("expected 2 hops, got %d: %+v", len(result.RedirectChain), result.RedirectChain)
	}
}

func TestClient_NoRedirectsWhenDisabled(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/redir" {
			http.Redirect(w, r, srv.URL+"/final", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result, err := NewClient().Execute(context.Background(), &types.RequestDefinition{
		Method: "GET", URL: srv.URL + "/redir",
		Settings: types.SettingDefinition{FollowRedirects: false},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != http.StatusFound {
		t.Errorf("expected 302, got %d", result.StatusCode)
	}
	if len(result.RedirectChain) != 0 {
		t.Errorf("expected 0 hops, got %d", len(result.RedirectChain))
	}
}

// ─── Sent-request summary ─────────────────────────────────────────────────────

func TestClient_SentRequestSummaryPopulated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	result, err := NewClient().Execute(context.Background(), &types.RequestDefinition{
		Method: "POST", URL: srv.URL + "/test",
		Headers: []types.KeyValue{{Key: "X-Custom", Value: "yes", Enabled: true}},
		Body:    types.BodyDefinition{Type: "raw", Raw: "hello"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.SentRequest == nil {
		t.Fatal("expected SentRequest to be set")
	}
	if result.SentRequest.Method != "POST" {
		t.Errorf("expected POST, got %s", result.SentRequest.Method)
	}
	if !strings.HasSuffix(result.SentRequest.URL, "/test") {
		t.Errorf("expected URL ending /test, got %s", result.SentRequest.URL)
	}
	if result.SentRequest.Body != "hello" {
		t.Errorf("expected body 'hello', got %q", result.SentRequest.Body)
	}
}

// ─── Content-type pass-through ────────────────────────────────────────────────

func TestClient_ContentTypePassedThrough(t *testing.T) {
	cases := []struct{ ct, body string }{
		{"application/json", `{"a":1}`},
		{"application/xml", `<a/>`},
		{"text/html", `<html/>`},
	}
	for _, tc := range cases {
		t.Run(tc.ct, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", tc.ct)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer srv.Close()
			result, err := NewClient().Execute(context.Background(), &types.RequestDefinition{Method: "GET", URL: srv.URL})
			if err != nil {
				t.Fatalf("error: %v", err)
			}
			got := ""
			for k, vs := range result.Headers {
				if strings.EqualFold(k, "content-type") {
					got = vs[0]
				}
			}
			if !strings.HasPrefix(got, tc.ct) {
				t.Errorf("expected %q, got %q", tc.ct, got)
			}
		})
	}
}
