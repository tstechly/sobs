package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFieldHintEndpoints(t *testing.T) {
	srv := newTestServer()
	for _, p := range []string{"/api/logs/field-hints", "/api/ai/field-hints"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}

func TestFilterValidationEndpoints(t *testing.T) {
	srv := newTestServer()
	body := []byte(`{"filter":"service.name = 'api'"}`)
	for _, p := range []string{"/api/logs/validate-filter", "/api/ai/validate-filter"} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+p, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}

func TestRegexValidationEndpoints(t *testing.T) {
	srv := newTestServer()
	body := []byte(`{"pattern":"^foo.*$"}`)
	for _, p := range []string{
		"/api/logs/validate-regex",
		"/api/errors/validate-regex",
		"/api/traces/validate-regex",
		"/api/metrics/validate-regex",
		"/api/rum/validate-regex",
	} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+p, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}
