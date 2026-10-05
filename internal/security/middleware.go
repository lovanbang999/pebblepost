package security

import (
	"net"
	"net/http"
	"strings"
)

// HostHeaderMiddleware validates incoming request Host headers against DNS rebinding attacks.
// By default, it allows localhost, 127.0.0.1, and [::1] (with any port).
func HostHeaderMiddleware(allowedHosts ...string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			host := r.Host
			if sh, _, err := net.SplitHostPort(host); err == nil {
				host = sh
			}
			host = strings.ToLower(strings.TrimSpace(host))

			if r.URL.Path == "/api/health" {
				next.ServeHTTP(w, r)
				return
			}

			allowed := false
			if len(allowedHosts) == 0 {
				if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "127.0.0.1" || host == "::1" || host == "[::1]" {
					allowed = true
				}
			} else {
				for _, h := range allowedHosts {
					if strings.EqualFold(host, h) {
						allowed = true
						break
					}
				}
			}

			if !allowed {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(`{"error":"forbidden: invalid host header"}`))
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// TokenMiddleware returns middleware that enforces Bearer token authentication.
// All routes except /api/health require a valid token. When token is empty the
// middleware is a no-op (desktop / loopback-only mode).
func TokenMiddleware(token string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip auth for health check, when running in desktop mode (token == ""),
			// or for non-API routes (static frontend SPA assets).
			if token == "" || r.URL.Path == "/api/health" || !strings.HasPrefix(r.URL.Path, "/api/") {
				next.ServeHTTP(w, r)
				return
			}

			// Accept token in Authorization: Bearer <token> header.
			if auth := r.Header.Get("Authorization"); strings.HasPrefix(auth, "Bearer ") {
				if strings.TrimPrefix(auth, "Bearer ") == token {
					next.ServeHTTP(w, r)
					return
				}
			}

			// Accept token in ?token= query parameter.
			if r.URL.Query().Get("token") == token {
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
		})
	}
}

// CORSMiddleware sets CORS headers that only allow localhost / 127.0.0.1
// origins. Requests from other origins are served without CORS headers (the
// browser will block them).
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if isAllowedOrigin(origin) {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}

		// Handle preflight.
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// SecurityHeadersMiddleware adds standard defensive response headers.
func SecurityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		// Permissive CSP for the embedded SPA; tightened in a future prompt.
		w.Header().Set("Content-Security-Policy", "default-src 'self' 'unsafe-inline' 'unsafe-eval' data: blob:")
		next.ServeHTTP(w, r)
	})
}

// BodyLimitMiddleware caps the request body size for POST/PUT requests.
func BodyLimitMiddleware(maxBytes int64) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodPut {
				r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// Chain composes a stack of middleware, executing left-to-right.
// Chain(A, B, C)(handler) = A(B(C(handler))).
func Chain(mws ...func(http.Handler) http.Handler) func(http.Handler) http.Handler {
	return func(final http.Handler) http.Handler {
		for i := len(mws) - 1; i >= 0; i-- {
			final = mws[i](final)
		}
		return final
	}
}

// isAllowedOrigin returns true when origin is localhost or 127.0.0.1 (any
// scheme, any port).
func isAllowedOrigin(origin string) bool {
	if origin == "" {
		return false
	}
	lower := strings.ToLower(origin)
	return strings.HasPrefix(lower, "http://localhost") ||
		strings.HasPrefix(lower, "https://localhost") ||
		strings.HasPrefix(lower, "http://127.0.0.1") ||
		strings.HasPrefix(lower, "https://127.0.0.1")
}
