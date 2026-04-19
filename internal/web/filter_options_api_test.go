package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFilterOptionsAPIs(t *testing.T) {
	srv := newTestServer()

	for _, path := range []string{
		"/api/logs/options",
		"/api/errors/options",
		"/api/traces/options",
		"/api/metrics/options",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, rec.Code)
		}
		if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("expected application/json for %s, got %q", path, ct)
		}
	}

	logsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/logs/options", nil)
	logsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(logsRec, logsReq)
	if !strings.Contains(logsRec.Body.String(), "services") || !strings.Contains(logsRec.Body.String(), "levels") {
		t.Fatalf("expected logs options payload, got %s", logsRec.Body.String())
	}

	metricsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/metrics/options", nil)
	metricsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metricsRec, metricsReq)
	if !strings.Contains(metricsRec.Body.String(), "signals") || !strings.Contains(metricsRec.Body.String(), "sources") {
		t.Fatalf("expected metrics options payload, got %s", metricsRec.Body.String())
	}
}
