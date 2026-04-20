package web

import (
	"bytes"
	"encoding/json"
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

	assertTrafficShape := func(path string, key string) {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", path, rec.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if okVal, ok := payload["ok"].(bool); !ok || !okVal {
			t.Fatalf("expected ok=true for %s, got %#v", path, payload["ok"])
		}
		if _, exists := payload[key]; !exists {
			t.Fatalf("expected key %q in %s payload", key, path)
		}
	}

	assertTrafficShape("/api/web-traffic/geo", "country_counts")
	assertTrafficShape("/api/web-traffic/browsers", "browsers")
	assertTrafficShape("/api/web-traffic/os", "operating_systems")
	assertTrafficShape("/api/web-traffic/timezones", "timezones")
	assertTrafficShape("/api/web-traffic/languages", "languages")
	assertTrafficShape("/api/web-traffic/devices", "devices")

	// Setting a disposition for a non-existent finding returns 404 — correct behaviour
	// when no scan has been run and no findings exist.
	dispositionReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/enrichment/cve/findings/OSV-UNKNOWN/disposition", bytes.NewReader([]byte(`{"disposition":"accepted-risk"}`)))
	dispositionRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dispositionRec, dispositionReq)
	if dispositionRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown finding, got %d", dispositionRec.Code)
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
