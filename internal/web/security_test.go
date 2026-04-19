package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionTokenFromRequestUsesConfiguredCookieName(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/auth/session", nil)
	r.AddCookie(&http.Cookie{Name: "custom_session", Value: "abc123"})

	got := sessionTokenFromRequest(r, "custom_session")
	if got != "abc123" {
		t.Fatalf("expected token from configured cookie name, got %q", got)
	}
}

func TestSessionTokenFromRequestDoesNotFallBackToHardcodedSessionWhenConfiguredNameDiffers(t *testing.T) {
	r := httptest.NewRequest("GET", "http://example.com/auth/session", nil)
	r.AddCookie(&http.Cookie{Name: "session", Value: "legacy"})

	got := sessionTokenFromRequest(r, "custom_session")
	if got != "" {
		t.Fatalf("expected empty token when configured cookie is missing, got %q", got)
	}
}

func TestSameOriginRequestIgnoresForwardedHeadersWhenNotTrustedProxy(t *testing.T) {
	r := httptest.NewRequest("POST", "http://real.example.com/auth/session", nil)
	r.Host = "real.example.com"
	r.Header.Set("Origin", "http://spoofed.example.com")
	r.Header.Set("X-Forwarded-Host", "spoofed.example.com")
	r.Header.Set("X-Forwarded-Proto", "http")

	if sameOriginRequest(r, false) {
		t.Fatal("expected sameOriginRequest to reject spoofed forwarded headers in non-trusted mode")
	}
}

func TestSameOriginRequestUsesForwardedHeadersWhenTrustedProxy(t *testing.T) {
	r := httptest.NewRequest("POST", "http://internal.service/auth/session", nil)
	r.Host = "internal.service"
	r.Header.Set("Origin", "https://public.example.com")
	r.Header.Set("X-Forwarded-Host", "public.example.com")
	r.Header.Set("X-Forwarded-Proto", "https")

	if !sameOriginRequest(r, true) {
		t.Fatal("expected sameOriginRequest to accept trusted forwarded host/proto")
	}
}
