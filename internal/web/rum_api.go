package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

type createRUMAssetRequest struct {
	Content string `json:"content"`
}

func (s *Server) v1RUMAssets(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req createRUMAssetRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	a, err := s.rumService.CreateAsset(strings.TrimSpace(req.Content))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

func (s *Server) v1RUMAssetByID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/v1/rum/assets/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	a, ok := s.rumService.GetAsset(id)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, a)
}

// v1RUMClientToken mirrors Python's issue_rum_client_token endpoint.
// When SOBS_RUM_CLIENT_AUTH_MODE is "none"/"off"/"disabled"/unset it returns
// {"enabled":false,"token":"","error":"RUM client auth is disabled"}.
// When configured it returns a signed token with origin/app/exp claims.
func (s *Server) v1RUMClientToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	mode := s.rumService.AuthMode()
	if mode == "" || mode == "none" || mode == "off" || mode == "disabled" {
		writeJSON(w, http.StatusOK, map[string]any{"enabled": false, "token": "", "error": "RUM client auth is disabled"})
		return
	}

	if mode != "origin" && mode != "origin-session" {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "Invalid SOBS_RUM_CLIENT_AUTH_MODE"})
		return
	}

	signingKey := s.rumService.SigningKey()
	if signingKey == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "RUM client signing key is not configured"})
		return
	}

	var payload map[string]any
	_ = json.NewDecoder(r.Body).Decode(&payload)
	if payload == nil {
		payload = map[string]any{}
	}

	appName := strings.TrimSpace(stringVal(payload, "appName", stringVal(payload, "app", "")))

	origin := strings.TrimSpace(stringVal(payload, "origin", ""))
	if origin == "" {
		origin = requestOrigin(r)
	}
	if origin == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "origin is required"})
		return
	}

	ttlSec := s.rumService.TokenTTL()
	if v, ok := payload["ttlSec"]; ok {
		switch n := v.(type) {
		case float64:
			ttlSec = int(n)
		}
	}
	if ttlSec < 30 {
		ttlSec = 30
	}
	if ttlSec > 86400 {
		ttlSec = 86400
	}

	token, exp := s.rumService.NewClientToken(signingKey, origin, appName, ttlSec)
	writeJSON(w, http.StatusOK, map[string]any{
		"enabled":   true,
		"token":     token,
		"expiresAt": exp,
		"origin":    origin,
		"app":       appName,
	})
}

// requestOrigin derives the request origin from Origin or Referer headers,
// matching Python's _request_origin().
func requestOrigin(r *http.Request) string {
	if origin := r.Header.Get("Origin"); origin != "" {
		return normalizeOrigin(origin)
	}
	if referer := r.Header.Get("Referer"); referer != "" {
		// Extract scheme://host from referer.
		if idx := strings.Index(referer, "://"); idx != -1 {
			rest := referer[idx+3:]
			if end := strings.IndexAny(rest, "/?#"); end != -1 {
				return strings.ToLower(referer[:idx+3+end])
			}
			return strings.ToLower(referer)
		}
	}
	return ""
}

func normalizeOrigin(origin string) string {
	origin = strings.TrimSpace(origin)
	if origin == "" {
		return ""
	}
	return strings.ToLower(origin)
}

func stringVal(m map[string]any, key, def string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}
