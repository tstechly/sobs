package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/auth"
	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func TestSessionEndpointUsesConfiguredCookieName(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.SessionCookieName = "custom_session"
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("GET", "http://example.com/auth/session", nil)
	r.Header.Set("Authorization", "Bearer test")
	r.Header.Set("Origin", "http://example.com")
	r.AddCookie(&http.Cookie{Name: "custom_session", Value: "tok123"})
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestSessionEndpointIgnoresForwardedHeadersWhenNotTrusted(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TrustedProxyMode = false
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("POST", "http://real.example.com/auth/session", nil)
	r.Host = "real.example.com"
	r.Header.Set("Authorization", "Bearer test")
	r.Header.Set("Origin", "http://spoofed.example.com")
	r.Header.Set("X-Forwarded-Host", "spoofed.example.com")
	r.Header.Set("X-Forwarded-Proto", "http")
	r.AddCookie(&http.Cookie{Name: "session", Value: "tok123"})
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected status 403, got %d", w.Code)
	}
}

func TestSessionEndpointUsesForwardedHeadersWhenTrusted(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TrustedProxyMode = true
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("POST", "http://internal.service/auth/session", nil)
	r.Host = "internal.service"
	r.Header.Set("Authorization", "Bearer test")
	r.Header.Set("Origin", "https://public.example.com")
	r.Header.Set("X-Forwarded-Host", "public.example.com")
	r.Header.Set("X-Forwarded-Proto", "https")
	r.AddCookie(&http.Cookie{Name: "session", Value: "tok123"})
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestReadyzEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("GET", "http://example.com/readyz", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
}

func TestHealthAliases(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r1 := httptest.NewRequest("GET", "http://example.com/health", nil)
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /health, got %d", w1.Code)
	}

	r2 := httptest.NewRequest("GET", "http://example.com/health/db", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /health/db, got %d", w2.Code)
	}
}

func TestRootEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("GET", "http://example.com/", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SOBS") {
		t.Fatalf("expected rendered body to contain title, got %q", w.Body.String())
	}
}

func TestGoSmokeEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("GET", "http://example.com/go/smoke", nil)
	w := httptest.NewRecorder()

	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "SOBS Go Migration") {
		t.Fatalf("expected rendered body to contain title, got %q", w.Body.String())
	}
}

func TestCompatibilityPageRoute(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("GET", "http://example.com/logs", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Fatalf("expected html content type, got %q", ct)
	}
}

func TestCompatibilityHelpRoutes(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	for _, path := range []string{"/query/help", "/metrics/help/anomaly", "/settings/help/notifications"} {
		r := httptest.NewRequest("GET", "http://example.com"+path, nil)
		w := httptest.NewRecorder()
		srv.Handler().ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("expected status 200 for %s, got %d", path, w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("expected html content type for %s, got %q", path, ct)
		}
	}
}

func TestNotificationsCheckEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	r := httptest.NewRequest("POST", "http://example.com/api/notifications/check", nil)
	w := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected application/json content type, got %q", ct)
	}
}
