package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func seedDataListTables(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	_, err = store.Exec(t.Context(), "CREATE TABLE IF NOT EXISTS otel_logs (Timestamp String, SeverityText String, ServiceName String, Body String, TraceId String, SpanId String) ENGINE = MergeTree ORDER BY Timestamp")
	if err != nil {
		t.Fatalf("create otel_logs: %v", err)
	}
	_, err = store.Exec(t.Context(), "CREATE TABLE IF NOT EXISTS otel_traces (Timestamp String, TraceId String, SpanId String, ParentSpanId String, SpanName String, ServiceName String, Duration Int64, StatusCode Int32) ENGINE = MergeTree ORDER BY Timestamp")
	if err != nil {
		t.Fatalf("create otel_traces: %v", err)
	}

	_, err = store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId) VALUES (?, ?, ?, ?, ?, ?)", "2026-04-19T10:00:00Z", "INFO", "api", "hello", "trace-a", "span-a")
	if err != nil {
		t.Fatalf("insert info log: %v", err)
	}
	_, err = store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId) VALUES (?, ?, ?, ?, ?, ?)", "2026-04-19T10:01:00Z", "ERROR", "api", "boom", "trace-b", "span-b")
	if err != nil {
		t.Fatalf("insert error log: %v", err)
	}
	_, err = store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "2026-04-19T10:01:00Z", "trace-b", "span-b", "", "GET /health", "api", int64(50000000), int32(0))
	if err != nil {
		t.Fatalf("insert trace: %v", err)
	}
}

func TestDataListAPIs_EndToEnd(t *testing.T) {
	srv := newTestServer()
	seedDataListTables(t, srv)

	logsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/logs/list?service=api&limit=5", nil)
	logsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(logsRec, logsReq)
	if logsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for logs list, got %d", logsRec.Code)
	}
	if !strings.Contains(logsRec.Body.String(), `"logs"`) || !strings.Contains(logsRec.Body.String(), `"boom"`) {
		t.Fatalf("expected logs payload with seeded row, got %s", logsRec.Body.String())
	}

	errorsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/errors/list?service=api&limit=5", nil)
	errorsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(errorsRec, errorsReq)
	if errorsRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for errors list, got %d", errorsRec.Code)
	}
	if !strings.Contains(errorsRec.Body.String(), `"errors"`) || !strings.Contains(errorsRec.Body.String(), `"ERROR"`) {
		t.Fatalf("expected errors payload with seeded error row, got %s", errorsRec.Body.String())
	}

	tracesReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/traces/list?service=api&limit=5", nil)
	tracesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tracesRec, tracesReq)
	if tracesRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for traces list, got %d", tracesRec.Code)
	}
	if !strings.Contains(tracesRec.Body.String(), `"traces"`) || !strings.Contains(tracesRec.Body.String(), `"GET /health"`) {
		t.Fatalf("expected traces payload with seeded span row, got %s", tracesRec.Body.String())
	}
}
