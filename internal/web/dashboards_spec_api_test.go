package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardsSpecEndpoints(t *testing.T) {
	srv := newTestServer()

	for _, p := range []string{"/api/dashboards/spec/templates", "/api/dashboards/spec/options"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}

	body := []byte(`{"prompt":"latency chart","spec":{"type":"line"}}`)
	for _, p := range []string{
		"/api/dashboards/spec/compile",
		"/api/dashboards/spec/dry-run",
		"/api/dashboards/spec/validate",
		"/api/dashboards/spec/render",
		"/api/dashboards/render",
		"/api/dashboards/spec/ai-build",
	} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+p, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}
