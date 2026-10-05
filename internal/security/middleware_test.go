package security

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// ---------- TokenMiddleware ----------

func TestTokenMiddleware_ValidBearer(t *testing.T) {
	h := TokenMiddleware("secret123")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer secret123")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestTokenMiddleware_ValidQueryToken(t *testing.T) {
	h := TokenMiddleware("secret123")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/test?token=secret123", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("got %d, want 200", rr.Code)
	}
}

func TestTokenMiddleware_WrongToken(t *testing.T) {
	h := TokenMiddleware("secret123")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestTokenMiddleware_NoToken(t *testing.T) {
	h := TokenMiddleware("secret123")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("got %d, want 401", rr.Code)
	}
}

func TestTokenMiddleware_HealthSkipsAuth(t *testing.T) {
	h := TokenMiddleware("secret123")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("health endpoint should skip auth, got %d", rr.Code)
	}
}

func TestTokenMiddleware_StaticAssetsBypassAuth(t *testing.T) {
	h := TokenMiddleware("secret123")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/index.html", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("static asset /index.html should skip auth, got %d", rr.Code)
	}

	reqAsset := httptest.NewRequest(http.MethodGet, "/assets/index-123.js", nil)
	rrAsset := httptest.NewRecorder()
	h.ServeHTTP(rrAsset, reqAsset)
	if rrAsset.Code != http.StatusOK {
		t.Errorf("static asset JS should skip auth, got %d", rrAsset.Code)
	}
}

func TestTokenMiddleware_EmptyTokenAllowsAll(t *testing.T) {
	h := TokenMiddleware("")(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/any", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("empty token should skip auth (desktop mode), got %d", rr.Code)
	}
}

// ---------- CORSMiddleware ----------

func TestCORSMiddleware_AllowsLocalhost(t *testing.T) {
	origins := []string{
		"http://localhost:3000",
		"http://localhost",
		"http://127.0.0.1:8080",
		"https://localhost:443",
	}
	for _, origin := range origins {
		t.Run(origin, func(t *testing.T) {
			h := CORSMiddleware(okHandler())
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Header.Set("Origin", origin)
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != origin {
				t.Errorf("expected ACAO=%q, got %q", origin, got)
			}
		})
	}
}

func TestCORSMiddleware_BlocksExternal(t *testing.T) {
	origins := []string{"https://evil.com", "http://attacker.io:8080", ""}
	for _, origin := range origins {
		t.Run(origin, func(t *testing.T) {
			h := CORSMiddleware(okHandler())
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			if origin != "" {
				req.Header.Set("Origin", origin)
			}
			rr := httptest.NewRecorder()
			h.ServeHTTP(rr, req)
			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != "" {
				t.Errorf("external origin %q should not get CORS header, got %q", origin, got)
			}
		})
	}
}

func TestCORSMiddleware_Preflight(t *testing.T) {
	h := CORSMiddleware(okHandler())
	req := httptest.NewRequest(http.MethodOptions, "/api/test", nil)
	req.Header.Set("Origin", "http://localhost:5173")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Errorf("preflight: got %d, want 204", rr.Code)
	}
}

// ---------- SecurityHeadersMiddleware ----------

func TestSecurityHeadersMiddleware(t *testing.T) {
	h := SecurityHeadersMiddleware(okHandler())
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)

	checks := map[string]string{
		"X-Content-Type-Options": "nosniff",
		"X-Frame-Options":        "DENY",
		"Referrer-Policy":        "strict-origin-when-cross-origin",
	}
	for header, want := range checks {
		if got := rr.Header().Get(header); !strings.Contains(got, want) {
			t.Errorf("header %s: got %q, want to contain %q", header, got, want)
		}
	}
}

// ---------- helpers ----------

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// ---------- HostHeaderMiddleware ----------

func TestHostHeaderMiddleware(t *testing.T) {
	defaultMw := HostHeaderMiddleware()(okHandler())

	tests := []struct {
		name     string
		host     string
		mw       http.Handler
		wantCode int
	}{
		{"localhost allowed", "localhost", defaultMw, http.StatusOK},
		{"localhost with port allowed", "localhost:8080", defaultMw, http.StatusOK},
		{"127.0.0.1 allowed", "127.0.0.1", defaultMw, http.StatusOK},
		{"127.0.0.1 with port allowed", "127.0.0.1:3000", defaultMw, http.StatusOK},
		{"ipv6 loopback allowed", "[::1]:8080", defaultMw, http.StatusOK},
		{"attacker dns rebinding blocked", "attacker.evil.com", defaultMw, http.StatusForbidden},
		{"attacker with port blocked", "attacker.evil.com:8080", defaultMw, http.StatusForbidden},
		{"custom allowed host", "my-intranet.local", HostHeaderMiddleware("my-intranet.local")(okHandler()), http.StatusOK},
		{"custom blocked host", "other.local", HostHeaderMiddleware("my-intranet.local")(okHandler()), http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
			req.Host = tt.host
			rr := httptest.NewRecorder()
			tt.mw.ServeHTTP(rr, req)
			if rr.Code != tt.wantCode {
				t.Errorf("host %q got status %d, want %d", tt.host, rr.Code, tt.wantCode)
			}
		})
	}
}
