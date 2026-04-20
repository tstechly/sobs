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

func TestTracesHelpAndMethodParity(t *testing.T) {
	srv := newTestServer()

	helpReq := httptest.NewRequest(http.MethodGet, "http://example.com/traces/help", nil)
	helpRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(helpRec, helpReq)
	if helpRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", helpRec.Code, helpRec.Body.String())
	}

	postHelpReq := httptest.NewRequest(http.MethodPost, "http://example.com/traces/help", nil)
	postHelpRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postHelpRec, postHelpReq)
	if postHelpRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postHelpRec.Code, postHelpRec.Body.String())
	}

	postTracesReq := httptest.NewRequest(http.MethodPost, "http://example.com/traces", nil)
	postTracesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postTracesRec, postTracesReq)
	if postTracesRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postTracesRec.Code, postTracesRec.Body.String())
	}
}

func TestAPITraceSpanPythonParityShapeAndTraceFilter(t *testing.T) {
	srv := newTestServer()
	seedTraceSpanFilterTable(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/traces/span/shared-span?trace_id=trace-b", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	span, ok := payload["span"].(map[string]any)
	if !ok {
		t.Fatalf("expected span object, got %#v", payload["span"])
	}
	if span["trace_id"] != "trace-b" {
		t.Fatalf("expected trace-qualified row, got trace_id=%v", span["trace_id"])
	}
	if span["trace_state"] != "" {
		t.Fatalf("expected fallback trace_state field, got %v", span["trace_state"])
	}
	if span["kind"] != "" {
		t.Fatalf("expected fallback kind field, got %v", span["kind"])
	}
	if span["scope_name"] != "" {
		t.Fatalf("expected fallback scope_name field, got %v", span["scope_name"])
	}
	if span["scope_version"] != "" {
		t.Fatalf("expected fallback scope_version field, got %v", span["scope_version"])
	}
	if span["status_code"] != "2" {
		t.Fatalf("expected raw status_code string parity, got %v", span["status_code"])
	}
	if span["status_message"] != "" {
		t.Fatalf("expected fallback status_message field, got %v", span["status_message"])
	}
	if _, ok := span["resource_attributes"].(map[string]any); !ok {
		t.Fatalf("expected resource_attributes map, got %#v", span["resource_attributes"])
	}
	if _, ok := payload["raw"].(string); !ok {
		t.Fatalf("expected raw JSON string field, got %#v", payload["raw"])
	}
	if _, ok := payload["truncated"].(bool); !ok {
		t.Fatalf("expected truncated bool field, got %#v", payload["truncated"])
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

func seedTraceSpanFilterTable(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer store.Close()

	stmts := []string{
		"DROP TABLE IF EXISTS otel_traces",
		"CREATE TABLE IF NOT EXISTS otel_traces (Timestamp String, TraceId String, SpanId String, ParentSpanId String, SpanName String, ServiceName String, Duration Int64, StatusCode Int32) ENGINE = MergeTree ORDER BY Timestamp",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "2026-04-20T11:00:00Z", "trace-a", "shared-span", "", "dup span", "svc-a", int64(1000000), int32(1)); err != nil {
		t.Fatalf("insert trace-a span: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "2026-04-20T11:01:00Z", "trace-b", "shared-span", "", "dup span", "svc-b", int64(2300000), int32(2)); err != nil {
		t.Fatalf("insert trace-b span: %v", err)
	}
}
