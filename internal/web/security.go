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

	origin := strings.TrimSpace(r.Header.Get("Origin"))
	if origin != "" {
		u, err := url.Parse(origin)
		if err == nil && u.Host != "" && u.Scheme != "" {
			if strings.EqualFold(strings.ToLower(u.Scheme+"://"+u.Host), expectedOrigin) {
				return true
			}
		}
	}

	referer := strings.TrimSpace(r.Header.Get("Referer"))
	if referer != "" {
		u, err := url.Parse(referer)
		if err == nil && u.Host != "" && u.Scheme != "" {
			if strings.EqualFold(strings.ToLower(u.Scheme+"://"+u.Host), expectedOrigin) {
				return true
			}
		}
	}

	return false
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
