package web

import (
	"net/http"
	"net/url"
	"strings"
)

func hostAndSchemeFromRequest(r *http.Request, trustedProxyMode bool) (string, string) {
	host := r.Host
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}

	if trustedProxyMode {
		if xfHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); xfHost != "" {
			host = xfHost
		}
		if xfProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); xfProto != "" {
			scheme = strings.ToLower(xfProto)
		}
	}

	return host, scheme
}

func sameOriginRequest(r *http.Request, trustedProxyMode bool) bool {
	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil || u.Host == "" || u.Scheme == "" {
		return false
	}
	host, scheme := hostAndSchemeFromRequest(r, trustedProxyMode)
	return strings.EqualFold(u.Host, host) && strings.EqualFold(u.Scheme, scheme)
}

func sessionTokenFromRequest(r *http.Request, sessionCookieName string) string {
	name := strings.TrimSpace(sessionCookieName)
	if name == "" {
		name = "session"
	}
	cookie, err := r.Cookie(name)
	if err != nil {
		return ""
	}
	return cookie.Value
}
