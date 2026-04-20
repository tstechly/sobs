package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func newRenderedLogsTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, store.NewNoopStoreFactory())
}

func TestLogsPageUsesPythonDerivedDataFlow(t *testing.T) {
	srv := newRenderedLogsTestServer()
	seedLogsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/logs?event_name=exception&trace_id=trace-a&trace_ids=trace-b&stats=1&analyze=1&q=boom", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body,
		"boom runtime failure",
		"priority=high",
		"Filtering by 2 trace IDs",
		"Top Message Patterns",
		"By Level",
		"exception",
	) {
		t.Fatalf("expected logs page to include seeded parity data, got %s", body)
	}
	if !strings.Contains(body, `aria-expanded="true"`) || !strings.Contains(body, `id="statsPanel" class="accordion-collapse collapse show"`) {
		t.Fatalf("expected stats panel to stay open when stats=1, got %s", body)
	}
	if strings.Contains(body, "plain info log") {
		t.Fatalf("expected event_name filter to exclude non-exception logs, got %s", body)
	}
	if strings.Contains(body, `No logs found`) {
		t.Fatalf("expected non-empty logs result, got %s", body)
	}
}

func TestLogsPageSupportsHasTagSQLFilter(t *testing.T) {
	srv := newRenderedLogsTestServer()
	seedLogsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/logs?sql=has_tag('priority','high')", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "boom runtime failure") {
		t.Fatalf("expected tagged log to be present, got %s", body)
	}
	if strings.Contains(body, "boom timeout failure") {
		t.Fatalf("expected untagged log to be excluded by has_tag SQL filter, got %s", body)
	}
}

func TestLogsPageRequiresGET(t *testing.T) {
	srv := newRenderedLogsTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/logs", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestLogsHelpPageParity(t *testing.T) {
	srv := newRenderedLogsTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/logs/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Logs Help") {
		t.Fatalf("expected logs help content, got %s", getRec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/logs/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func seedLogsPageTables(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS otel_logs",
		"DROP TABLE IF EXISTS sobs_record_tags",
		"CREATE TABLE IF NOT EXISTS otel_logs (Timestamp DateTime64(6), SeverityText String, ServiceName String, Body String, TraceId String, SpanId String, EventName String, LogAttributes Map(String, String)) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS sobs_record_tags (RecordType String, RecordId String, TagKey String, TagValue String, IsAuto UInt8, IsDeleted UInt8, Version UInt64) ENGINE = ReplacingMergeTree(Version) ORDER BY (RecordType, RecordId, TagKey)",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	rows := []struct {
		ts      string
		level   string
		service string
		body    string
		traceID string
		spanID  string
		event   string
		excType string
	}{
		{ts: "2026-04-20 10:00:00.000000", level: "ERROR", service: "svc-logs", body: "boom runtime failure", traceID: "trace-a", spanID: "span-a", event: "exception", excType: "RuntimeError"},
		{ts: "2026-04-20 10:01:00.000000", level: "ERROR", service: "svc-logs", body: "boom timeout failure", traceID: "trace-b", spanID: "span-b", event: "exception", excType: "TimeoutError"},
		{ts: "2026-04-20 10:02:00.000000", level: "INFO", service: "svc-logs", body: "plain info log", traceID: "trace-c", spanID: "span-c", event: "log", excType: ""},
	}
	for _, row := range rows {
		if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId, EventName, LogAttributes) VALUES (?, ?, ?, ?, ?, ?, ?, map('exception.type', ?))", row.ts, row.level, row.service, row.body, row.traceID, row.spanID, row.event, row.excType); err != nil {
			t.Fatalf("insert log row: %v", err)
		}
	}

	recordID := webRecordIDForLog("2026-04-20 10:00:00.000000", "svc-logs", "trace-a", "span-a")
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_record_tags (RecordType, RecordId, TagKey, TagValue, IsAuto, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", "log", recordID, "priority", "high", uint8(0), uint8(0), uint64(1)); err != nil {
		t.Fatalf("insert tag row: %v", err)
	}
}
