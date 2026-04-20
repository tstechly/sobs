package web

import (
	"context"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/features/repositories"
)

var externalAuthClient = &http.Client{Timeout: 5 * time.Second}

func (s *Server) wrapSecurity(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.allowV1APIKey(r) {
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

		if mode != "none" && csrfOriginCheckEnabled() && isWriteMethod(r.Method) && !sameOriginRequest(r, s.cfg.TrustedProxyMode) {
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
			if strings.HasPrefix(authz, "Bearer ") && allowExternalBearer(r.Context(), authz) {
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

func (s *Server) allowV1APIKey(r *http.Request) bool {
	if !s.cfg.EnforceAPIAuth {
		return true
	}
	if !requiresV1APIKey(r.URL.Path, r.Method) {
		return true
	}
	provided := strings.TrimSpace(r.Header.Get("X-API-Key"))
	expected := strings.TrimSpace(os.Getenv("SOBS_API_KEY"))
	staticOK := expected != "" && subtle.ConstantTimeCompare([]byte(provided), []byte(expected)) == 1
	managedConfigured, managedOK := s.managedV1KeyStatus(r.URL.Path, provided)

	if expected != "" {
		return staticOK || managedOK
	}
	if managedConfigured {
		return managedOK
	}
	return true
}

func (s *Server) managedV1KeyStatus(path string, provided string) (configured bool, ok bool) {
	appID := ""
	if strings.HasPrefix(path, "/v1/apps/") {
		rest := strings.TrimPrefix(path, "/v1/apps/")
		parts := strings.Split(rest, "/")
		if len(parts) > 0 {
			appID = strings.TrimSpace(parts[0])
		}
	}
	if appID == "" && strings.HasPrefix(path, "/v1/releases/") {
		rest := strings.TrimPrefix(path, "/v1/releases/")
		parts := strings.Split(rest, "/")
		if len(parts) > 0 {
			releaseID := strings.TrimSpace(parts[0])
			if releaseID != "" {
				if rel, found := s.appService.GetRelease(releaseID); found {
					appID = strings.TrimSpace(rel.AppID)
				}
			}
		}
	}
	if appID == "" {
		return false, false
	}
	for _, repo := range s.repositoryService.List() {
		if strings.TrimSpace(repo.ID) != appID {
			continue
		}
		configured := strings.HasPrefix(strings.TrimSpace(repo.CIIngestKeyHash), "scrypt:v1:") || strings.TrimSpace(repo.CIIngestKey) != ""
		if !configured {
			return false, false
		}
		return true, repositories.VerifyCIIngestKey(provided, repo.CIIngestKeyHash, repo.CIIngestKey)
	}
	return false, false
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
	if strings.HasPrefix(path, "/health") {
		return false
	}
	if strings.HasPrefix(path, "/static/") || path == "/service-worker.js" {
		return false
	}
	if strings.HasPrefix(path, "/mcp") {
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

func allowExternalBearer(ctx context.Context, authz string) bool {
	base := strings.TrimRight(strings.TrimSpace(os.Getenv("SOBS_EXTERNAL_AUTH_URL")), "/")
	if base == "" {
		return false
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/internal/auth/validate", nil)
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

func csrfOriginCheckEnabled() bool {
	if raw := strings.TrimSpace(os.Getenv("SOBS_CSRF_ORIGIN_CHECK")); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	if raw := strings.TrimSpace(os.Getenv("SOBS_BEHIND_TLS")); raw != "" {
		switch strings.ToLower(raw) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return false
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}
