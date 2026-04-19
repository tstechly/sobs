package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebTrafficAndEnrichmentEndpoints(t *testing.T) {
	srv := newTestServer()

	for _, p := range []string{
		"/api/web-traffic/geo",
		"/api/web-traffic/browsers",
		"/api/web-traffic/os",
		"/api/web-traffic/timezones",
		"/api/web-traffic/languages",
		"/api/web-traffic/devices",
		"/api/enrichment/libraries",
		"/api/enrichment/github/repo-health",
		"/api/enrichment/cve/findings",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}

	dispositionReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/enrichment/cve/findings/OSV-2026-0001/disposition", bytes.NewReader([]byte(`{"disposition":"accepted-risk"}`)))
	dispositionRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dispositionRec, dispositionReq)
	if dispositionRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", dispositionRec.Code)
	}

	scanReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/enrichment/cve/scan", nil)
	scanRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(scanRec, scanReq)
	if scanRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", scanRec.Code)
	}

	pageReq := httptest.NewRequest(http.MethodGet, "http://example.com/enrichment/cve", nil)
	pageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", pageRec.Code)
	}
}
