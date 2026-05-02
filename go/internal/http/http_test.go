package http

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSecurityMiddleware_SetsCommonHeaders(t *testing.T) {
	mw := SecurityMiddleware(false, []string{"http://localhost:*"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	require.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
	require.Equal(t, "strict-origin-when-cross-origin", rr.Header().Get("Referrer-Policy"))
	require.Equal(t, "camera=(), microphone=(), geolocation=()", rr.Header().Get("Permissions-Policy"))
	require.Equal(t, "frame-ancestors 'self'; object-src 'none'; base-uri 'self'", rr.Header().Get("Content-Security-Policy"))
	require.Empty(t, rr.Header().Get("Strict-Transport-Security"))
}

func TestSecurityMiddleware_SetsHSTSForSecureContext(t *testing.T) {
	tests := []struct {
		name   string
		behind bool
		req    func() *http.Request
	}{
		{
			name:   "behind tls flag",
			behind: true,
			req: func() *http.Request {
				return httptest.NewRequest(http.MethodGet, "/health", nil)
			},
		},
		{
			name:   "forwarded proto https",
			behind: false,
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/health", nil)
				r.Header.Set("X-Forwarded-Proto", "https, http")
				return r
			},
		},
		{
			name:   "tls connection",
			behind: false,
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "/health", nil)
				r.TLS = &tls.ConnectionState{}
				return r
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mw := SecurityMiddleware(tc.behind, []string{"http://localhost:*"})
			handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))

			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, tc.req())

			require.Equal(t, "max-age=31536000; includeSubDomains", rr.Header().Get("Strict-Transport-Security"))
		})
	}
}

func TestSecurityMiddleware_SetsOTLPCORSHeaders(t *testing.T) {
	mw := SecurityMiddleware(false, []string{"https://localhost:*", "http://127.0.0.1:*"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodPost, "/v1/logs", nil)
	req.Header.Set("Origin", "https://localhost:3000")
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	require.Equal(t, http.StatusNoContent, rr.Code)
	require.Equal(t, "https://localhost:3000", rr.Header().Get("Access-Control-Allow-Origin"))
	require.Equal(t, "Origin", rr.Header().Get("Vary"))
	require.Equal(t, "true", rr.Header().Get("Access-Control-Allow-Credentials"))
	require.Equal(t, "POST, OPTIONS", rr.Header().Get("Access-Control-Allow-Methods"))
	require.Equal(t, "Content-Type, Authorization, X-API-Key, X-SOBS-RUM-Client, X-SOBS-RUM-Signature, X-SOBS-RUM-Timestamp, X-SOBS-Asset-Timestamp, X-SOBS-Asset-Signature", rr.Header().Get("Access-Control-Allow-Headers"))
	require.Equal(t, "600", rr.Header().Get("Access-Control-Max-Age"))
}

func TestSecurityMiddleware_UsesAssetMethodsForSubpaths(t *testing.T) {
	mw := SecurityMiddleware(false, []string{"https://localhost:*"})
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "exact assets path", path: "/v1/rum/assets", want: "POST, OPTIONS"},
		{name: "asset subpath", path: "/v1/rum/assets/bundle.js", want: "GET, HEAD, OPTIONS"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tc.path, nil)
			req.Header.Set("Origin", "https://localhost:3000")
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			require.Equal(t, tc.want, rr.Header().Get("Access-Control-Allow-Methods"))
		})
	}
}

func TestOriginAllowedForOTLP(t *testing.T) {
	tests := []struct {
		name    string
		origin  string
		allowed []string
		want    bool
	}{
		{name: "default localhost port wildcard", origin: "https://localhost:3000", allowed: []string{"https://localhost:*"}, want: true},
		{name: "default port stripped", origin: "https://localhost:443", allowed: []string{"https://localhost"}, want: true},
		{name: "non default port must match", origin: "https://example.com:8443", allowed: []string{"https://example.com"}, want: false},
		{name: "invalid scheme", origin: "ftp://localhost", allowed: []string{"https://localhost:*"}, want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, originAllowedForOTLP(tc.origin, tc.allowed))
		})
	}
}
