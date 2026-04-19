package web

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"
)

var externalAuthClient = &http.Client{Timeout: 5 * time.Second}

func (s *Server) wrapSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !allowV1APIKey(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !requiresUIAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		mode, invalid := uiAuthMode()
		if invalid {
			http.Error(w, "server auth misconfiguration", http.StatusInternalServerError)
			return
		}

		if mode != "none" && csrfOriginCheckEnabled(s.cfg.TrustedProxyMode) && isWriteMethod(r.Method) && !sameOriginRequest(r, s.cfg.TrustedProxyMode) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		if mode == "none" {
			next.ServeHTTP(w, r)
			return
		}

		authz := strings.TrimSpace(r.Header.Get("Authorization"))
		switch mode {
		case "basic":
			if allowBasicAuth(authz) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `Basic realm="SOBS"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		case "external":
			if !strings.HasPrefix(authz, "Bearer ") {
				token := strings.TrimSpace(sessionTokenFromRequest(r, s.cfg.SessionCookieName))
				if token != "" && !strings.ContainsAny(token, "\r\n") {
					authz = "Bearer " + token
				}
			}
			if strings.HasPrefix(authz, "Bearer ") && allowExternalBearer(authz) {
				next.ServeHTTP(w, r)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="SOBS"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		default:
			http.Error(w, "server auth misconfiguration", http.StatusInternalServerError)
			return
		}
	})
}

func allowV1APIKey(r *http.Request) bool {
	if !requiresV1APIKey(r.URL.Path, r.Method) {
		return true
	}
	expected := strings.TrimSpace(os.Getenv("SOBS_API_KEY"))
	if expected == "" {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
	if provided == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
}

func requiresV1APIKey(path string, method string) bool {
	if path == "/v1/logs" && method == http.MethodPost {
		return true
	}
	if path == "/v1/traces" && method == http.MethodPost {
		return true
	}
	if path == "/v1/metrics" && method == http.MethodPost {
		return true
	}
	if path == "/v1/errors" && method == http.MethodPost {
		return true
	}
	if path == "/v1/rum" && method == http.MethodPost {
		return true
	}
	if path == "/v1/ai" && method == http.MethodPost {
		return true
	}
	if path == "/v1/rum/assets" && method == http.MethodPost {
		return true
	}
	if path == "/v1/rum/client-token" && method == http.MethodPost {
		return true
	}
	if strings.HasPrefix(path, "/v1/apps") || strings.HasPrefix(path, "/v1/releases/") {
		return true
	}
	return false
}

func requiresUIAuth(path string) bool {
	if strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/readyz") {
		return false
	}
	if strings.HasPrefix(path, "/static/") || path == "/service-worker.js" {
		return false
	}
	if strings.HasPrefix(path, "/mcp") || path == "/auth/session" {
		return false
	}
	if strings.HasPrefix(path, "/v1/rum/assets/") {
		return true
	}
	if strings.HasPrefix(path, "/v1/") {
		return false
	}
	return true
}

func uiAuthMode() (mode string, invalid bool) {
	hasUser := strings.TrimSpace(os.Getenv("SOBS_BASIC_AUTH_USERNAME")) != ""
	hasPass := strings.TrimSpace(os.Getenv("SOBS_BASIC_AUTH_PASSWORD")) != ""
	hasExternal := strings.TrimSpace(os.Getenv("SOBS_EXTERNAL_AUTH_URL")) != ""

	if hasExternal && (hasUser || hasPass) {
		return "", true
	}
	if hasUser != hasPass {
		return "", true
	}
	if hasExternal {
		return "external", false
	}
	if hasUser && hasPass {
		return "basic", false
	}
	return "none", false
}

func allowBasicAuth(authz string) bool {
	if !strings.HasPrefix(authz, "Basic ") {
		return false
	}
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(strings.TrimPrefix(authz, "Basic ")))
	if err != nil {
		return false
	}
	decoded := string(raw)
	username, password, ok := strings.Cut(decoded, ":")
	if !ok {
		return false
	}
	expectedUser := strings.TrimSpace(os.Getenv("SOBS_BASIC_AUTH_USERNAME"))
	expectedPass := strings.TrimSpace(os.Getenv("SOBS_BASIC_AUTH_PASSWORD"))
	userOK := subtle.ConstantTimeCompare([]byte(username), []byte(expectedUser)) == 1
	passOK := subtle.ConstantTimeCompare([]byte(password), []byte(expectedPass)) == 1
	return userOK && passOK
}

func allowExternalBearer(authz string) bool {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("SOBS_EXTERNAL_AUTH_URL")), "/")
	if base == "" {
		return false
	}
	req, err := http.NewRequest(http.MethodPost, base+"/internal/auth/validate", nil)
	if err != nil {
		return false
	}
	req.Header.Set("Authorization", authz)
	resp, err := externalAuthClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func csrfOriginCheckEnabled(defaultValue bool) bool {
	if raw := strings.TrimSpace(os.Getenv("SOBS_CSRF_ORIGIN_CHECK")); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return defaultValue
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
