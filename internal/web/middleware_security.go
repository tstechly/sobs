package web

import (
	"net/http"
	"strings"
)

func (s *Server) wrapSecurity(next http.Handler) http.Handler {
	if !s.cfg.EnforceAPIAuth {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !requiresAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		permission := requiredPermission(r.URL.Path, r.Method)
		if isWriteMethod(r.Method) {
			if !sameOriginRequest(r, s.cfg.TrustedProxyMode) {
				http.Error(w, "forbidden", http.StatusForbidden)
				return
			}
		}
		id, err := s.authProvider.Authenticate(r.Context(), r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := s.authProvider.Authorize(r.Context(), id, permission); err != nil {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isWriteMethod(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	default:
		return false
	}
}

func requiresAuth(path string) bool {
	if strings.HasPrefix(path, "/health") || strings.HasPrefix(path, "/readyz") {
		return false
	}
	if path == "/" || strings.HasPrefix(path, "/static/") || strings.HasPrefix(path, "/service-worker.js") {
		return false
	}
	if strings.HasPrefix(path, "/auth/session") || strings.HasPrefix(path, "/mcp") {
		return false
	}
	if strings.HasPrefix(path, "/api/") || strings.HasPrefix(path, "/settings/") {
		return true
	}
	if strings.HasPrefix(path, "/dashboards") || strings.HasPrefix(path, "/reports/") {
		return true
	}
	if strings.HasPrefix(path, "/v1/apps") || strings.HasPrefix(path, "/v1/releases/") {
		return true
	}
	return false
}

func requiredPermission(path string, method string) string {
	action := "read"
	if isWriteMethod(method) {
		action = "write"
	}
	domain := "session"
	switch {
	case strings.HasPrefix(path, "/api/reports") || strings.HasPrefix(path, "/reports/"):
		domain = "reports"
	case strings.HasPrefix(path, "/api/dashboards") || strings.HasPrefix(path, "/dashboards"):
		domain = "dashboards"
	case strings.HasPrefix(path, "/api/agent") || strings.HasPrefix(path, "/settings/agents"):
		domain = "agents"
	case strings.HasPrefix(path, "/api/notifications") || strings.HasPrefix(path, "/settings/notifications"):
		domain = "notifications"
	case strings.HasPrefix(path, "/api/query") || strings.HasPrefix(path, "/api/table-explorer") || strings.HasPrefix(path, "/table-explorer"):
		domain = "query"
	case strings.HasPrefix(path, "/v1/apps") || strings.HasPrefix(path, "/v1/releases/"):
		domain = "apps"
	case strings.HasPrefix(path, "/api/settings/") || strings.HasPrefix(path, "/settings/"):
		domain = "settings"
	}
	return domain + ":" + action
}
