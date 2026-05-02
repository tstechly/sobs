package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"
)

// JSONError writes a JSON error envelope: {"error": msg}.
func JSONError(w http.ResponseWriter, msg string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}

// JSON writes a JSON response with the given status code.
func JSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("json encode failed", "error", err)
	}
}

// CORSPreflight returns a handler for OPTIONS requests.
func CORSPreflight() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}
}

// LoggingMiddleware logs each request.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration_ms", time.Since(start).Milliseconds(),
		)
	})
}

// CORSMiddleware adds CORS headers to non-OTLP responses.
func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !pathNeedsOTLPCORS(r.URL.Path) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-API-Key, Authorization, Content-Encoding")
		}
		next.ServeHTTP(w, r)
	})
}

const (
	securityHeaderNoSniff       = "nosniff"
	securityHeaderSameOrigin    = "SAMEORIGIN"
	securityHeaderReferrer      = "strict-origin-when-cross-origin"
	securityHeaderPermissions   = "camera=(), microphone=(), geolocation=()"
	securityHeaderCSP           = "frame-ancestors 'self'; object-src 'none'; base-uri 'self'"
	securityHeaderHSTS          = "max-age=31536000; includeSubDomains"
	securityHeaderAllowCreds    = "true"
	securityHeaderMaxAge        = "600"
	otlpAllowHeaders            = "Content-Type, Authorization, X-API-Key, X-SOBS-RUM-Client, X-SOBS-RUM-Signature, X-SOBS-RUM-Timestamp, X-SOBS-Asset-Timestamp, X-SOBS-Asset-Signature"
	otlpAllowedMethodsStandard  = "POST, OPTIONS"
	otlpAllowedMethodsAssetPath = "GET, HEAD, OPTIONS"
)

var otlpCORSPaths = map[string]struct{}{
	"/v1/logs":             {},
	"/v1/traces":           {},
	"/v1/metrics":          {},
	"/v1/rum":              {},
	"/v1/rum/assets":       {},
	"/v1/rum/client-token": {},
	"/v1/errors":           {},
	"/v1/ai":               {},
}

// SecurityMiddleware adds security headers and OTLP/RUM CORS headers.
func SecurityMiddleware(behindTLS bool, otlpAllowedOrigins []string) func(http.Handler) http.Handler {
	allowed := make([]string, 0, len(otlpAllowedOrigins))
	for _, origin := range otlpAllowedOrigins {
		if trimmed := strings.TrimSpace(origin); trimmed != "" {
			allowed = append(allowed, strings.ToLower(trimmed))
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			mw := &securityResponseWriter{
				ResponseWriter: w,
				req:            r,
				behindTLS:      behindTLS,
				allowedOrigins: allowed,
			}
			next.ServeHTTP(mw, r)
			mw.apply()
		})
	}
}

type securityResponseWriter struct {
	http.ResponseWriter
	req            *http.Request
	behindTLS      bool
	allowedOrigins []string
	once           sync.Once
}

func (w *securityResponseWriter) WriteHeader(code int) {
	w.apply()
	w.ResponseWriter.WriteHeader(code)
}

func (w *securityResponseWriter) Write(p []byte) (int, error) {
	w.apply()
	return w.ResponseWriter.Write(p)
}

func (w *securityResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *securityResponseWriter) apply() {
	w.once.Do(func() {
		h := w.Header()
		setDefaultHeader(h, "X-Content-Type-Options", securityHeaderNoSniff)
		setDefaultHeader(h, "X-Frame-Options", securityHeaderSameOrigin)
		setDefaultHeader(h, "Referrer-Policy", securityHeaderReferrer)
		setDefaultHeader(h, "Permissions-Policy", securityHeaderPermissions)
		setDefaultHeader(h, "Content-Security-Policy", securityHeaderCSP)
		if w.requestIsSecureContext() {
			setDefaultHeader(h, "Strict-Transport-Security", securityHeaderHSTS)
		}

		if pathNeedsOTLPCORS(w.req.URL.Path) {
			origin := strings.TrimSpace(w.req.Header.Get("Origin"))
			if origin != "" && originAllowedForOTLP(origin, w.allowedOrigins) {
				h.Set("Access-Control-Allow-Origin", origin)
				appendVaryHeader(h, "Origin")
				setDefaultHeader(h, "Access-Control-Allow-Credentials", securityHeaderAllowCreds)
				setDefaultHeader(h, "Access-Control-Allow-Methods", otlpAllowedMethods(w.req.URL.Path))
				setDefaultHeader(h, "Access-Control-Allow-Headers", otlpAllowHeaders)
				setDefaultHeader(h, "Access-Control-Max-Age", securityHeaderMaxAge)
			}
		}
	})
}

func (w *securityResponseWriter) requestIsSecureContext() bool {
	if w.behindTLS {
		return true
	}
	forwardedProto := strings.ToLower(strings.TrimSpace(firstHeaderToken(w.req.Header.Get("X-Forwarded-Proto"))))
	if forwardedProto == "https" {
		return true
	}
	return w.req.TLS != nil
}

func firstHeaderToken(v string) string {
	if v == "" {
		return ""
	}
	parts := strings.SplitN(v, ",", 2)
	return strings.TrimSpace(parts[0])
}

func setDefaultHeader(h http.Header, key, value string) {
	if h.Get(key) == "" {
		h.Set(key, value)
	}
}

func appendVaryHeader(h http.Header, value string) {
	existing := strings.TrimSpace(h.Get("Vary"))
	if existing == "" {
		h.Set("Vary", value)
		return
	}
	parts := strings.Split(existing, ",")
	seen := make(map[string]struct{}, len(parts))
	out := make([]string, 0, len(parts)+1)
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed == "" {
			continue
		}
		lower := strings.ToLower(trimmed)
		if _, ok := seen[lower]; ok {
			continue
		}
		seen[lower] = struct{}{}
		out = append(out, trimmed)
	}
	if _, ok := seen[strings.ToLower(value)]; !ok {
		out = append(out, value)
	}
	h.Set("Vary", strings.Join(out, ", "))
}

func pathNeedsOTLPCORS(p string) bool {
	if _, ok := otlpCORSPaths[p]; ok {
		return true
	}
	return strings.HasPrefix(p, "/v1/rum/assets/")
}

func otlpAllowedMethods(p string) string {
	if strings.HasPrefix(p, "/v1/rum/assets/") {
		return otlpAllowedMethodsAssetPath
	}
	return otlpAllowedMethodsStandard
}

func originAllowedForOTLP(origin string, allowedOrigins []string) bool {
	parsed, err := url.Parse(origin)
	if err != nil {
		return false
	}
	scheme := strings.ToLower(strings.TrimSpace(parsed.Scheme))
	if scheme != "http" && scheme != "https" {
		return false
	}
	if parsed.Host == "" {
		return false
	}

	host := strings.ToLower(parsed.Hostname())
	hostWithPort := strings.ToLower(parsed.Host)
	candidates := []string{scheme + "://" + hostWithPort}

	port := parsed.Port()
	if port == "" || port == defaultPortForScheme(scheme) {
		if host != "" {
			candidates = append(candidates, scheme+"://"+host)
		}
	}

	for _, pattern := range allowedOrigins {
		pattern = strings.ToLower(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		for _, candidate := range candidates {
			matched, err := path.Match(pattern, candidate)
			if err == nil && matched {
				return true
			}
		}
	}
	return false
}

func defaultPortForScheme(scheme string) string {
	switch scheme {
	case "http":
		return "80"
	case "https":
		return "443"
	default:
		return ""
	}
}
