package web

import (
	"bytes"
	"encoding/json"
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
	body := []byte(`{"sql":"service = 'api'"}`)
	for _, p := range []string{"/api/logs/validate-filter", "/api/ai/validate-filter"} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+p, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal %s: %v", p, err)
		}
		if payload["ok"] != true {
			t.Fatalf("expected ok=true for %s, got %v", p, payload["ok"])
		}
		normalized, _ := payload["normalized"].(string)
		if normalized == "" || !bytes.Contains([]byte(normalized), []byte("ServiceName")) {
			t.Fatalf("expected normalized sql alias expansion for %s, got %q", p, normalized)
		}
	}
}

func TestFilterValidationRejectsUnsafeKeywords(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-filter", bytes.NewReader([]byte(`{"sql":"service='api'; DROP TABLE otel_logs"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %v", payload["ok"])
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
