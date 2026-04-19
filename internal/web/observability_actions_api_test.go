package web

import (
	"net/http"
	"net/http/httptest"
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
}
