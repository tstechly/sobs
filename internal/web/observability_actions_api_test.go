package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMetricsPageAndObservabilityActions(t *testing.T) {
	srv := newTestServer()

	metricsReq := httptest.NewRequest(http.MethodGet, "http://example.com/metrics", nil)
	metricsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", metricsRec.Code)
	}

	resolveReq := httptest.NewRequest(http.MethodPost, "http://example.com/errors/err-1/resolve", nil)
	resolveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resolveRec, resolveReq)
	if resolveRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resolveRec.Code)
	}

	spanReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/traces/span/span-1", nil)
	spanRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(spanRec, spanReq)
	if spanRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", spanRec.Code)
	}

	summaryReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/metrics/summary", nil)
	summaryRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(summaryRec, summaryReq)
	if summaryRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for metrics summary, got %d", summaryRec.Code)
	}
	if !strings.Contains(summaryRec.Body.String(), "total_series") {
		t.Fatalf("expected metrics summary payload, got %s", summaryRec.Body.String())
	}

	tsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/metrics/timeseries?limit=20", nil)
	tsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tsRec, tsReq)
	if tsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for metrics timeseries, got %d", tsRec.Code)
	}
	if !strings.Contains(tsRec.Body.String(), "rows") {
		t.Fatalf("expected metrics timeseries payload, got %s", tsRec.Body.String())
	}
}
