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
		if !isWriteMethod(r.Method) || !requiresWriteAuth(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if !sameOriginRequest(r, s.cfg.TrustedProxyMode) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		id, err := s.authProvider.Authenticate(r.Context(), r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if err := s.authProvider.Authorize(r.Context(), id, "session:write"); err != nil {
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

func requiresWriteAuth(path string) bool {
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
