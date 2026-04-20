package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsRulesPageUsesPythonDerivedContext(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/metrics/rules?open_panel=auto-rules", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body,
		"Metrics Rules",
		"Slow LCP",
		"svc-metrics",
		"rum_vitals",
		"LCP",
		"Auto Make Metric Rules",
	) {
		t.Fatalf("expected metrics rules page to include seeded parity data, got %s", body)
	}
	if !strings.Contains(body, `value="svc-metrics"`) {
		t.Fatalf("expected service filter options to be populated from derived signal dimensions, got %s", body)
	}
}

func TestMetricsRulesHelpPageParity(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/metrics/help/rules", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Metric Rules Help") {
		t.Fatalf("expected metrics rules help content, got %s", rec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/metrics/help/rules", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestMetricsRulesAutoHelpPageParity(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/metrics/help/rules/auto", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Auto Make Metric Rules Help") {
		t.Fatalf("expected auto metrics rules help content, got %s", rec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/metrics/help/rules/auto", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}