package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/extensionpoints"
	sobsstore "github.com/abartrim/sobs/internal/store"
)

func newRenderedIncidentTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestIncidentHelpPageParity(t *testing.T) {
	srv := newRenderedIncidentTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/incident/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestIncidentPageNoReferenceAndWindowClampParity(t *testing.T) {
	srv := newRenderedIncidentTestServer()

	noRefReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident", nil)
	noRefRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(noRefRec, noRefReq)
	if noRefRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", noRefRec.Code, noRefRec.Body.String())
	}
	if !strings.Contains(noRefRec.Body.String(), "No incident reference provided. Specify trace_id, error_id, or rum_session.") {
		t.Fatalf("expected explicit missing-reference error, got %s", noRefRec.Body.String())
	}

	fiftyReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident?trace_id=trace-1&window_minutes=50", nil)
	fiftyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(fiftyRec, fiftyReq)
	if fiftyRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", fiftyRec.Code, fiftyRec.Body.String())
	}
	if !strings.Contains(fiftyRec.Body.String(), "50 min total") {
		t.Fatalf("expected clamped window to preserve 50 minutes, got %s", fiftyRec.Body.String())
	}

	upperReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident?trace_id=trace-1&window_minutes=200", nil)
	upperRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(upperRec, upperReq)
	if upperRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", upperRec.Code, upperRec.Body.String())
	}
	if !strings.Contains(upperRec.Body.String(), "180 min total") {
		t.Fatalf("expected upper-clamped window to 180 minutes, got %s", upperRec.Body.String())
	}

	lowerReq := httptest.NewRequest(http.MethodGet, "http://example.com/incident?trace_id=trace-1&window_minutes=-5", nil)
	lowerRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(lowerRec, lowerReq)
	if lowerRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", lowerRec.Code, lowerRec.Body.String())
	}
	if !strings.Contains(lowerRec.Body.String(), "1 min total") {
		t.Fatalf("expected lower-clamped window to 1 minute, got %s", lowerRec.Body.String())
	}
}

func TestIncidentPageRendersFullEvidenceContext(t *testing.T) {
	srv := newRenderedIncidentTestServer()
	primaryErrorID := seedIncidentPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/incident?error_id="+primaryErrorID, nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body,
		"Primary Event",
		"Metrics &amp; Preserved Raw Windows",
		"warning",
		"No preserved windows overlap this incident window yet.",
	) {
		t.Fatalf("expected incident page to render full evidence context, got %s", body)
	}
}

func seedIncidentPageTables(t *testing.T, srv *Server) string {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS otel_logs",
		"DROP TABLE IF EXISTS hyperdx_sessions",
		"DROP TABLE IF EXISTS otel_traces",
		"DROP TABLE IF EXISTS v_derived_signals_anomaly",
		"DROP TABLE IF EXISTS sobs_raw_windows",
		"DROP TABLE IF EXISTS sobs_raw_window_copy_state",
		"DROP TABLE IF EXISTS sobs_github_work_items",
		"DROP TABLE IF EXISTS sobs_error_resolutions",
		"CREATE TABLE IF NOT EXISTS otel_logs (Timestamp DateTime64(3), ServiceName String, TraceId String, SpanId String, Body String, LogAttributes Map(String, String), EventName String, SeverityNumber UInt8, SeverityText String) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS hyperdx_sessions (Timestamp DateTime64(3), ServiceName String, TraceId String, SpanId String, Body String, LogAttributes Map(String, String), EventName String, SeverityNumber UInt8, SeverityText String) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS otel_traces (Timestamp DateTime64(3), TraceId String, SpanId String, ParentSpanId String, SpanName String, ServiceName String, Duration Int64, StatusCode Int32, SpanAttributes Map(String, String)) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS v_derived_signals_anomaly (time DateTime64(3), ServiceName String, SignalSource String, anomaly_state String) ENGINE = MergeTree ORDER BY time",
		"CREATE TABLE IF NOT EXISTS sobs_raw_windows (Id String, SignalType String, SignalRef String, ServiceName String, Namespace String, NodeName String, WindowStart DateTime64(3), WindowEnd DateTime64(3)) ENGINE = MergeTree ORDER BY WindowStart",
		"CREATE TABLE IF NOT EXISTS sobs_raw_window_copy_state (WindowId String, SourceTable String) ENGINE = MergeTree ORDER BY WindowId",
		"CREATE TABLE IF NOT EXISTS sobs_github_work_items (AnomalyRuleId String, IssueUrl String, CanonicalIssueUrl String, IssueNumber UInt32, IssueState String, IsDeleted UInt8, CreatedAt DateTime64(3)) ENGINE = MergeTree ORDER BY CreatedAt",
		"CREATE TABLE IF NOT EXISTS sobs_error_resolutions (ErrorId String, CreatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(CreatedAt) ORDER BY (ErrorId)",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('exception.type', ?, 'exception.message', ?, 'exception.stacktrace', ?), ?, ?, ?)", "2026-04-20 10:00:00.000", "svc-inc", "trace-1", "span-1", "primary boom", "TimeoutError", "primary boom", "stack-line-1", "exception", uint8(17), "ERROR"); err != nil {
		t.Fatalf("insert primary error: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('exception.type', ?, 'exception.message', ?), ?, ?, ?)", "2026-04-20 10:05:00.000", "svc-inc", "trace-2", "span-2", "secondary boom", "TypeError", "secondary boom", "exception", uint8(17), "ERROR"); err != nil {
		t.Fatalf("insert related error: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, map())", "2026-04-20 09:59:59.000", "trace-1", "span-1", "", "root", "svc-inc", int64(60000000), int32(2)); err != nil {
		t.Fatalf("insert trace root: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, map())", "2026-04-20 10:00:01.000", "trace-1", "span-1-1", "span-1", "child", "svc-inc", int64(15000000), int32(2)); err != nil {
		t.Fatalf("insert trace child: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('service.name', ?, 'session.id', ?, 'url', ?), ?, ?, ?)", "2026-04-20 10:00:30.000", "svc-inc", "trace-1", "span-1", "{\"message\":\"rum boom\"}", "svc-inc", "sess-1", "https://example/inc", "error", uint8(17), "ERROR"); err != nil {
		t.Fatalf("insert rum event: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO v_derived_signals_anomaly (time, ServiceName, SignalSource, anomaly_state) VALUES (?, ?, ?, ?)", "2026-04-20 10:06:00.000", "svc-inc", "traces", "warning"); err != nil {
		t.Fatalf("insert anomaly state: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_raw_windows (Id, SignalType, SignalRef, ServiceName, Namespace, NodeName, WindowStart, WindowEnd) VALUES (?, ?, ?, ?, ?, ?, ?, ?)", "win-1", "trace", "trace-1", "svc-inc", "ns-a", "node-a", "2026-04-20 09:50:00.000", "2026-04-20 10:20:00.000"); err != nil {
		t.Fatalf("insert raw window: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_raw_window_copy_state (WindowId, SourceTable) VALUES (?, ?), (?, ?)", "win-1", "otel_metrics_gauge", "win-1", "otel_metrics_sum"); err != nil {
		t.Fatalf("insert raw copy state: %v", err)
	}

	primaryErrorID := lookupSeededIncidentErrorID(t, store, "trace-1")
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_github_work_items (AnomalyRuleId, IssueUrl, CanonicalIssueUrl, IssueNumber, IssueState, IsDeleted, CreatedAt) VALUES (?, ?, ?, ?, ?, ?, ?)", primaryErrorID, "https://github.com/abartrim/sobs/issues/999", "", uint32(999), "open", uint8(0), "2026-04-20 12:00:00.000"); err != nil {
		t.Fatalf("insert work item: %v", err)
	}

	return primaryErrorID
}

func lookupSeededIncidentErrorID(t *testing.T, store extensionpoints.ClickHouseStore, traceID string) string {
	t.Helper()
	rows, err := store.Query(t.Context(), "SELECT "+summaryErrorIDSQLExpr()+" AS ErrorId FROM ("+summaryErrorSourcesSQL()+") WHERE TraceId = ? ORDER BY Timestamp DESC LIMIT 1", traceID)
	if err != nil {
		t.Fatalf("query incident error id: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("expected incident error id for trace %s", traceID)
	}
	var errorID any
	if err := rows.Scan(&errorID); err != nil {
		t.Fatalf("scan incident error id: %v", err)
	}
	return anyToString(errorID)
}
