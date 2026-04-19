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
	host, scheme := hostAndSchemeFromRequest(r, trustedProxyMode)
	expectedHost := strings.TrimSpace(host)
	expectedScheme := strings.TrimSpace(scheme)
	if expectedHost == "" || expectedScheme == "" {
		return false
	}
	expectedOrigin := strings.ToLower(expectedScheme + "://" + expectedHost)

	actualOrigin := requestOriginFromHeaders(r.Header.Get("Origin"), r.Header.Get("Referer"))
	if actualOrigin != "" && strings.EqualFold(actualOrigin, expectedOrigin) {
		return true
	}

	return false
}

func normalizeOrigin(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

func requestOriginFromHeaders(originHeader string, refererHeader string) string {
	origin := normalizeOrigin(originHeader)
	if origin != "" {
		return origin
	}
	return normalizeOrigin(refererHeader)
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
