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
	seedTraceDetailTables(t, srv)

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

	spanReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/traces/span/span-1?trace_id=trace-1", nil)
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
	if _, ok := spanPayload["span"]; !ok {
		t.Fatalf("expected span object in span response")
	}
}

func TestTracesPageBuildsDetailView(t *testing.T) {
	srv := newTestServer()
	seedTraceDetailTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/traces?trace_id=trace-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Visible rows 1-") {
		t.Fatalf("expected trace detail pagination summary, got %s", body)
	}
	if !strings.Contains(body, "root span") {
		t.Fatalf("expected trace detail tree content, got %s", body)
	}
}

func seedTraceDetailTables(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	_, err = store.Exec(t.Context(), "DROP TABLE IF EXISTS otel_traces")
	if err != nil {
		t.Fatalf("drop otel_traces: %v", err)
	}
	_, err = store.Exec(t.Context(), "DROP TABLE IF EXISTS otel_logs")
	if err != nil {
		t.Fatalf("drop otel_logs: %v", err)
	}

	_, err = store.Exec(t.Context(), "CREATE TABLE IF NOT EXISTS otel_traces (Timestamp String, TraceId String, SpanId String, ParentSpanId String, SpanName String, ServiceName String, Duration Int64, StatusCode Int32) ENGINE = MergeTree ORDER BY Timestamp")
	if err != nil {
		t.Fatalf("create otel_traces: %v", err)
	}
	_, err = store.Exec(t.Context(), "CREATE TABLE IF NOT EXISTS otel_logs (Timestamp String, SeverityText String, ServiceName String, Body String, TraceId String, SpanId String) ENGINE = MergeTree ORDER BY Timestamp")
	if err != nil {
		t.Fatalf("create otel_logs: %v", err)
	}

	_, err = store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "2026-04-19T10:00:00Z", "trace-1", "span-1", "", "root span", "api", int64(120000000), int32(1))
	if err != nil {
		t.Fatalf("insert root span: %v", err)
	}
	_, err = store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "2026-04-19T10:00:01Z", "trace-1", "span-2", "span-1", "child span", "api", int64(60000000), int32(2))
	if err != nil {
		t.Fatalf("insert child span: %v", err)
	}
	_, err = store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId) VALUES (?, ?, ?, ?, ?, ?)", "2026-04-19T10:00:01Z", "ERROR", "api", "boom", "trace-1", "span-2")
	if err != nil {
		t.Fatalf("insert trace log: %v", err)
	}
}
