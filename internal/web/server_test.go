package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func TestHealthAliases(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	r1 := httptest.NewRequest("GET", "http://example.com/health", nil)
	w1 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w1, r1)
	if w1.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /health, got %d", w1.Code)
	}
	var healthBody map[string]any
	if err := json.Unmarshal(w1.Body.Bytes(), &healthBody); err != nil {
		t.Fatalf("expected /health json body, got error: %v", err)
	}
	if status, _ := healthBody["status"].(string); status != "ok" {
		t.Fatalf("expected /health status ok, got %#v", healthBody["status"])
	}

	r2 := httptest.NewRequest("GET", "http://example.com/health/db", nil)
	w2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(w2, r2)
	if w2.Code != http.StatusOK {
		t.Fatalf("expected status 200 for /health/db, got %d", w2.Code)
	}
	var readyBody map[string]any
	if err := json.Unmarshal(w2.Body.Bytes(), &readyBody); err != nil {
		t.Fatalf("expected /health/db json body, got error: %v", err)
	}
	if status, _ := readyBody["status"].(string); status != "ok" {
		t.Fatalf("expected /health/db status ok, got %#v", readyBody["status"])
	}
}

func TestRootEndpoint(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	srv := NewServer(cfg, store.NewNoopStoreFactory())

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

func TestCompatibilityPageRoute(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	srv := NewServer(cfg, store.NewNoopStoreFactory())

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
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	for _, path := range []string{"/query/help", "/metrics/help/anomaly", "/settings/help/notifications", "/table-explorer/help", "/summary/help"} {
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
	srv := NewServer(cfg, store.NewNoopStoreFactory())

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
