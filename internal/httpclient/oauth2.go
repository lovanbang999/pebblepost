package httpclient

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/pkg/browser"
	"pebblepost/internal/types"
)

// OAuth2Token represents a token response from an OAuth2 provider.
type OAuth2Token struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Scope        string `json:"scope"`
	ExpiresAt    int64  `json:"expires_at"` // Unix timestamp when access token expires
}

// OAuth2Manager manages token caching, refresh, and PKCE loopback authorization.
type OAuth2Manager struct {
	mu     sync.RWMutex
	tokens map[string]*OAuth2Token // cache key -> OAuth2Token
	client *http.Client
}

// GlobalOAuth2Manager is the singleton manager for OAuth2 tokens.
var (
	globalOAuth2Manager *OAuth2Manager
	oauth2Once          sync.Once
)

// GetOAuth2Manager returns the shared OAuth2Manager instance.
func GetOAuth2Manager() *OAuth2Manager {
	oauth2Once.Do(func() {
		globalOAuth2Manager = &OAuth2Manager{
			tokens: make(map[string]*OAuth2Token),
			client: &http.Client{Timeout: 30 * time.Second},
		}
	})
	return globalOAuth2Manager
}

// CacheKey generates a deterministic cache lookup key for the given auth definition.
func (m *OAuth2Manager) CacheKey(auth types.AuthDefinition) string {
	grant := strings.ToLower(strings.TrimSpace(auth.GrantType))
	if grant == "" {
		grant = "client_credentials"
	}
	return fmt.Sprintf("%s|%s|%s|%s", auth.TokenURL, auth.ClientID, grant, auth.Scope)
}

// GetToken returns a valid access token for the given auth definition, refreshing or fetching if necessary.
func (m *OAuth2Manager) GetToken(ctx context.Context, auth *types.AuthDefinition) (*OAuth2Token, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth definition cannot be nil")
	}

	key := m.CacheKey(*auth)

	m.mu.RLock()
	cached, ok := m.tokens[key]
	m.mu.RUnlock()

	now := time.Now().Unix()

	// Check if cached token is still valid (give 30 seconds buffer)
	if ok && cached != nil && cached.AccessToken != "" && (cached.ExpiresAt == 0 || cached.ExpiresAt > now+30) {
		return cached, nil
	}

	// If cached token has a refresh token and expired, try refreshing
	if ok && cached != nil && cached.RefreshToken != "" {
		refreshed, err := m.RefreshToken(ctx, auth.TokenURL, auth.ClientID, auth.ClientSecret, cached.RefreshToken)
		if err == nil && refreshed != nil {
			m.mu.Lock()
			m.tokens[key] = refreshed
			m.mu.Unlock()
			return refreshed, nil
		}
	}

	// If request already has a valid token and refresh token configured in auth, check it
	if auth.Token != "" && (auth.TokenExpiresAt == 0 || auth.TokenExpiresAt > now+30) {
		tok := &OAuth2Token{
			AccessToken:  auth.Token,
			RefreshToken: auth.RefreshToken,
			ExpiresAt:    auth.TokenExpiresAt,
		}
		m.mu.Lock()
		m.tokens[key] = tok
		m.mu.Unlock()
		return tok, nil
	}

	// Try refresh with auth.RefreshToken if provided
	if auth.RefreshToken != "" {
		refreshed, err := m.RefreshToken(ctx, auth.TokenURL, auth.ClientID, auth.ClientSecret, auth.RefreshToken)
		if err == nil && refreshed != nil {
			m.mu.Lock()
			m.tokens[key] = refreshed
			m.mu.Unlock()
			auth.Token = refreshed.AccessToken
			auth.RefreshToken = refreshed.RefreshToken
			auth.TokenExpiresAt = refreshed.ExpiresAt
			return refreshed, nil
		}
	}

	grant := strings.ToLower(strings.TrimSpace(auth.GrantType))
	if grant == "client_credentials" || grant == "" {
		token, err := m.FetchClientCredentials(ctx, auth.TokenURL, auth.ClientID, auth.ClientSecret, auth.Scope)
		if err != nil {
			return nil, err
		}
		m.mu.Lock()
		m.tokens[key] = token
		m.mu.Unlock()
		auth.Token = token.AccessToken
		auth.TokenExpiresAt = token.ExpiresAt
		return token, nil
	}

	return nil, fmt.Errorf("no valid OAuth2 access token available; authorization code flow requires interactive login")
}

// FetchClientCredentials executes the client_credentials grant flow.
func (m *OAuth2Manager) FetchClientCredentials(ctx context.Context, tokenURL, clientID, clientSecret, scope string) (*OAuth2Token, error) {
	if strings.TrimSpace(tokenURL) == "" {
		return nil, fmt.Errorf("OAuth2 token URL is required")
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	if clientID != "" {
		data.Set("client_id", clientID)
	}
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}
	if scope != "" {
		data.Set("scope", scope)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token request returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var token OAuth2Token
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return nil, fmt.Errorf("failed to parse token response: %w", err)
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Unix() + token.ExpiresIn
	}

	return &token, nil
}

// RefreshToken executes the refresh_token grant flow.
func (m *OAuth2Manager) RefreshToken(ctx context.Context, tokenURL, clientID, clientSecret, refreshToken string) (*OAuth2Token, error) {
	if strings.TrimSpace(tokenURL) == "" {
		return nil, fmt.Errorf("token URL is required for refresh")
	}
	if strings.TrimSpace(refreshToken) == "" {
		return nil, fmt.Errorf("refresh token is empty")
	}

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)
	if clientID != "" {
		data.Set("client_id", clientID)
	}
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh token request failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("refresh token request returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var token OAuth2Token
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return nil, fmt.Errorf("failed to parse token refresh response: %w", err)
	}

	// Preserve refresh token if not returned in response
	if token.RefreshToken == "" {
		token.RefreshToken = refreshToken
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Unix() + token.ExpiresIn
	}

	return &token, nil
}

// AuthorizeCodePKCE starts a local loopback server, opens the browser for authorization, and exchanges the code.
func (m *OAuth2Manager) AuthorizeCodePKCE(ctx context.Context, auth *types.AuthDefinition) (*OAuth2Token, error) {
	if auth == nil {
		return nil, fmt.Errorf("auth definition cannot be nil")
	}
	if auth.AuthURL == "" || auth.TokenURL == "" {
		return nil, fmt.Errorf("both Auth URL and Token URL are required for Authorization Code flow")
	}

	// 1. Generate PKCE Verifier and Challenge
	codeVerifier, err := generateRandomString(64)
	if err != nil {
		return nil, fmt.Errorf("failed to generate code verifier: %w", err)
	}
	h := sha256.Sum256([]byte(codeVerifier))
	codeChallenge := base64.RawURLEncoding.EncodeToString(h[:])

	state, err := generateRandomString(16)
	if err != nil {
		return nil, fmt.Errorf("failed to generate state: %w", err)
	}

	// 2. Start loopback listener on dynamic port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to start loopback listener: %w", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	redirectURI := fmt.Sprintf("http://127.0.0.1:%d/callback", port)

	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// 3. Setup temporary HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("state") != state {
			http.Error(w, "State mismatch", http.StatusBadRequest)
			errChan <- fmt.Errorf("state mismatch in OAuth2 callback")
			return
		}
		if errMsg := q.Get("error"); errMsg != "" {
			desc := q.Get("error_description")
			http.Error(w, fmt.Sprintf("OAuth error: %s (%s)", errMsg, desc), http.StatusBadRequest)
			errChan <- fmt.Errorf("oauth error: %s - %s", errMsg, desc)
			return
		}
		code := q.Get("code")
		if code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			errChan <- fmt.Errorf("missing code in callback")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!DOCTYPE html>
<html>
<head><title>PebblePost - Authorization Successful</title>
<style>
body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #09090b; color: #f4f4f5; display: flex; align-items: center; justify-content: center; height: 100vh; margin: 0; }
.card { background: #18181b; padding: 2rem; border-radius: 0.75rem; border: 1px solid #27272a; text-align: center; max-width: 400px; }
h2 { color: #22c55e; margin-top: 0; }
p { color: #a1a1aa; font-size: 0.9rem; }
</style>
</head>
<body>
<div class="card">
  <h2>Authorization Successful</h2>
  <p>PebblePost has received your authorization code. You can close this window and return to the application.</p>
</div>
</body>
</html>`))

		codeChan <- code
	})

	server := &http.Server{Handler: mux}
	go func() {
		if serveErr := server.Serve(listener); serveErr != nil && serveErr != http.ErrServerClosed {
			errChan <- serveErr
		}
	}()
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	// 4. Construct Auth URL and open system browser
	authURLParsed, err := url.Parse(auth.AuthURL)
	if err != nil {
		return nil, fmt.Errorf("invalid Auth URL: %w", err)
	}

	q := authURLParsed.Query()
	q.Set("response_type", "code")
	q.Set("client_id", auth.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("code_challenge", codeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", state)
	if auth.Scope != "" {
		q.Set("scope", auth.Scope)
	}
	authURLParsed.RawQuery = q.Encode()

	// Open browser
	targetURL := authURLParsed.String()
	_ = browser.OpenURL(targetURL)

	// 5. Wait for callback or timeout (2 minutes)
	select {
	case code := <-codeChan:
		// Exchange code for token
		token, err := m.ExchangeCodePKCE(ctx, auth.TokenURL, auth.ClientID, auth.ClientSecret, code, redirectURI, codeVerifier)
		if err != nil {
			return nil, err
		}

		key := m.CacheKey(*auth)
		m.mu.Lock()
		m.tokens[key] = token
		m.mu.Unlock()

		auth.Token = token.AccessToken
		auth.RefreshToken = token.RefreshToken
		auth.TokenExpiresAt = token.ExpiresAt
		auth.CodeVerifier = codeVerifier

		return token, nil

	case err := <-errChan:
		return nil, err

	case <-time.After(2 * time.Minute):
		return nil, fmt.Errorf("OAuth2 authorization timed out waiting for browser callback")

	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// ExchangeCodePKCE exchanges the received authorization code and PKCE verifier for tokens.
func (m *OAuth2Manager) ExchangeCodePKCE(
	ctx context.Context,
	tokenURL, clientID, clientSecret, code, redirectURI, codeVerifier string,
) (*OAuth2Token, error) {
	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", redirectURI)
	data.Set("code_verifier", codeVerifier)
	if clientID != "" {
		data.Set("client_id", clientID)
	}
	if clientSecret != "" {
		data.Set("client_secret", clientSecret)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("token exchange returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	var token OAuth2Token
	if err := json.Unmarshal(bodyBytes, &token); err != nil {
		return nil, fmt.Errorf("failed to parse token exchange response: %w", err)
	}

	if token.ExpiresIn > 0 {
		token.ExpiresAt = time.Now().Unix() + token.ExpiresIn
	}

	return &token, nil
}

func generateRandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:n], nil
}
