package httpclient

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"pebblepost/internal/types"
)

func TestOAuth2_ClientCredentials(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("failed to parse form: %v", err)
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("expected grant_type=client_credentials, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "test-client" || r.Form.Get("client_secret") != "test-secret" {
			t.Errorf("credentials mismatch: %s / %s", r.Form.Get("client_id"), r.Form.Get("client_secret"))
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OAuth2Token{
			AccessToken: "access-token-123",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer ts.Close()

	mgr := &OAuth2Manager{
		tokens: make(map[string]*OAuth2Token),
		client: ts.Client(),
	}

	auth := &types.AuthDefinition{
		Type:         "oauth2",
		GrantType:    "client_credentials",
		TokenURL:     ts.URL,
		ClientID:     "test-client",
		ClientSecret: "test-secret",
	}

	tok, err := mgr.GetToken(context.Background(), auth)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tok.AccessToken != "access-token-123" {
		t.Fatalf("expected token access-token-123, got %s", tok.AccessToken)
	}

	// Verify caching
	cached, err := mgr.GetToken(context.Background(), auth)
	if err != nil {
		t.Fatalf("unexpected error fetching cached: %v", err)
	}
	if cached.AccessToken != "access-token-123" {
		t.Fatalf("expected cached token access-token-123, got %s", cached.AccessToken)
	}
}

func TestOAuth2_AuthorizeCodePKCE_SecurityAndLoopback(t *testing.T) {
	// Mock Token server to verify code exchange
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("grant_type") != "authorization_code" {
			t.Errorf("expected authorization_code, got %s", r.Form.Get("grant_type"))
		}
		if r.Form.Get("code") != "valid-auth-code" {
			t.Errorf("expected valid-auth-code, got %s", r.Form.Get("code"))
		}
		if r.Form.Get("code_verifier") == "" {
			t.Errorf("missing code_verifier")
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(OAuth2Token{
			AccessToken:  "pkce-access-token",
			RefreshToken: "pkce-refresh-token",
			ExpiresIn:    3600,
		})
	}))
	defer tokenServer.Close()

	var capturedAuthURL string
	capturedChan := make(chan string, 1)

	oldOpenURL := openURL
	defer func() { openURL = oldOpenURL }()
	openURL = func(target string) error {
		capturedAuthURL = target
		capturedChan <- target
		return nil
	}

	mgr := &OAuth2Manager{
		tokens: make(map[string]*OAuth2Token),
		client: tokenServer.Client(),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	auth := &types.AuthDefinition{
		Type:      "oauth2",
		GrantType: "authorization_code",
		AuthURL:   "https://auth.example.com/oauth/authorize",
		TokenURL:  tokenServer.URL,
		ClientID:  "pkce-client-id",
	}

	resultChan := make(chan *OAuth2Token, 1)
	errChan := make(chan error, 1)

	go func() {
		tok, err := mgr.AuthorizeCodePKCE(ctx, auth)
		if err != nil {
			errChan <- err
			return
		}
		resultChan <- tok
	}()

	select {
	case <-capturedChan:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for openURL to be called")
	}

	parsed, err := url.Parse(capturedAuthURL)
	if err != nil {
		t.Fatalf("failed to parse captured auth URL: %v", err)
	}

	redirectURI := parsed.Query().Get("redirect_uri")
	state := parsed.Query().Get("state")
	if redirectURI == "" || state == "" {
		t.Fatalf("missing redirect_uri or state: uri=%s, state=%s", redirectURI, state)
	}

	client := &http.Client{Timeout: 2 * time.Second}

	// 1. Test Host Header DNS Rebinding rejection
	{
		req, err := http.NewRequest(http.MethodGet, redirectURI+"?code=xyz&state="+state, nil)
		if err != nil {
			t.Fatalf("failed to create req: %v", err)
		}
		req.Host = "attacker-domain.evil.com"
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("rebinding probe request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for hostile Host header, got %d", resp.StatusCode)
		}
	}

	// 2. Test Mismatched State probe (should be rejected with 400 and NOT kill the server)
	{
		req, err := http.NewRequest(http.MethodGet, redirectURI+"?code=xyz&state=wrong-state-1234", nil)
		if err != nil {
			t.Fatalf("failed to create req: %v", err)
		}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("mismatched state probe request failed: %v", err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected 400 for mismatched state, got %d", resp.StatusCode)
		}
	}

	// 3. Test Valid Callback
	{
		validURL := fmt.Sprintf("%s?code=valid-auth-code&state=%s", redirectURI, state)
		resp, err := client.Get(validURL)
		if err != nil {
			t.Fatalf("valid callback request failed: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200 OK for valid callback, got %d: %s", resp.StatusCode, string(body))
		}
	}

	// 4. Verify AuthorizeCodePKCE finished successfully
	select {
	case tok := <-resultChan:
		if tok.AccessToken != "pkce-access-token" {
			t.Fatalf("expected pkce-access-token, got %s", tok.AccessToken)
		}
	case err := <-errChan:
		t.Fatalf("AuthorizeCodePKCE returned error: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatalf("AuthorizeCodePKCE timed out after valid callback")
	}
}
