package web

import (
	"context"
	"encoding/json"
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
	store, err := srv.storeFactory.Open(context.Background())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()
	rows, err := store.Query(context.Background(), "SELECT count() FROM sobs_error_resolutions WHERE ErrorId = ?", "err-1")
	if err != nil {
		t.Fatalf("query error resolutions: %v", err)
	}
	defer rows.Close()
	if !rows.Next() {
		t.Fatal("expected row count for resolved error")
	}
	var resolvedCount int
	if err := rows.Scan(&resolvedCount); err != nil {
		t.Fatalf("scan resolved count: %v", err)
	}
	if resolvedCount < 1 {
		t.Fatalf("expected resolved count >= 1, got %d", resolvedCount)
	}

	spanReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/traces/span/span-1", nil)
	spanRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(spanRec, spanReq)
	if spanRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", spanRec.Code)
	}
	var spanPayload map[string]any
	if err := json.Unmarshal(spanRec.Body.Bytes(), &spanPayload); err != nil {
		t.Fatalf("unmarshal span payload: %v", err)
	}
	if _, ok := spanPayload["raw"]; !ok {
		t.Fatalf("expected raw payload field in span response")
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
