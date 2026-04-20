package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	sobsstore "github.com/abartrim/sobs/internal/store"
)

func newRenderedEnrichmentTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestCVEHelpPageParity(t *testing.T) {
	srv := newRenderedEnrichmentTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/cve/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "CVE Findings Help") {
		t.Fatalf("expected cve help content")
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/cve/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestEnrichmentCVEPageMethodParity(t *testing.T) {
	srv := newRenderedEnrichmentTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/enrichment/cve", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/enrichment/cve", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestEnrichmentAPIShapeAndMethodParity(t *testing.T) {
	srv := newTestServer()

	getCases := []struct {
		path       string
		requiredKey string
	}{
		{"/api/enrichment/libraries", "libraries"},
		{"/api/enrichment/github/repo-health", "repos"},
		{"/api/enrichment/cve/findings", "findings"},
	}

	for _, tc := range getCases {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+tc.path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d body=%s", tc.path, rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal %s: %v", tc.path, err)
		}
		if okVal, ok := payload["ok"].(bool); !ok || !okVal {
			t.Fatalf("expected ok=true for %s, got %#v", tc.path, payload["ok"])
		}
		if _, exists := payload[tc.requiredKey]; !exists {
			t.Fatalf("expected key %q in payload for %s", tc.requiredKey, tc.path)
		}
	}

	for _, path := range []string{"/api/enrichment/libraries", "/api/enrichment/github/repo-health", "/api/enrichment/cve/findings"} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for %s, got %d", path, rec.Code)
		}
	}

	dispositionReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/enrichment/cve/findings/OSV-UNKNOWN/disposition", bytes.NewReader([]byte(`{"disposition":"accepted-risk"}`)))
	dispositionRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dispositionRec, dispositionReq)
	if dispositionRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown finding disposition, got %d body=%s", dispositionRec.Code, dispositionRec.Body.String())
	}

	scanReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/enrichment/cve/scan", nil)
	scanRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(scanRec, scanReq)
	if scanRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for cve scan, got %d body=%s", scanRec.Code, scanRec.Body.String())
	}
}
