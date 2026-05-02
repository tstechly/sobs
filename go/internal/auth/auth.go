package auth

import (
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"strings"

	"github.com/sobs/sobs-api/internal/config"
	sobshttp "github.com/sobs/sobs-api/internal/http"
)

// RequireAPIKey returns middleware that validates the Authorization header.
func RequireAPIKey(cfg config.Cfg) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			key := strings.TrimSpace(r.Header.Get("Authorization"))
			if key == "" {
				sobshttp.JSONError(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			if cfg.APIKey != "" && subtle.ConstantTimeCompare([]byte(key), []byte(cfg.APIKey)) != 1 {
				sobshttp.JSONError(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// RequireBasicAuth returns middleware for basic/external auth.
func RequireBasicAuth(cfg config.Cfg) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if cfg.AuthMode == "none" {
				next.ServeHTTP(w, r)
				return
			}
			authHeader := r.Header.Get("Authorization")
			if cfg.AuthMode == "basic" && strings.HasPrefix(authHeader, "Basic ") {
				decoded, err := base64.StdEncoding.DecodeString(authHeader[6:])
				if err == nil {
					parts := strings.SplitN(string(decoded), ":", 2)
					if len(parts) == 2 {
						userOK := subtle.ConstantTimeCompare([]byte(parts[0]), []byte(cfg.BasicAuthUsername)) == 1
						passOK := subtle.ConstantTimeCompare([]byte(parts[1]), []byte(cfg.BasicAuthPassword)) == 1
						if userOK && passOK {
							next.ServeHTTP(w, r)
							return
						}
					}
				}
			}
			// External auth mode — placeholder for future implementation
			if cfg.AuthMode == "external" && strings.HasPrefix(authHeader, "Bearer ") {
				// TODO: validate token against cfg.ExternalAuthURL
				next.ServeHTTP(w, r)
				return
			}
			sobshttp.JSONError(w, "Unauthorized", http.StatusUnauthorized)
		})
	}
}
