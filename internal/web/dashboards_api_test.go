package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardsList(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/dashboards/list", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestDashboardsQuery(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/dashboards/query", bytes.NewReader([]byte(`{"query":"select 1"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
