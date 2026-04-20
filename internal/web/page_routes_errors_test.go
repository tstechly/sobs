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

func newRenderedErrorsTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestErrorsHelpPageParity(t *testing.T) {
	srv := newRenderedErrorsTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/errors/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Errors Help") {
		t.Fatalf("expected errors help content, got %s", getRec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/errors/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestErrorsPageResolvedFilterAndResolveURLs(t *testing.T) {
	srv := newRenderedErrorsTestServer()
	openID, resolvedID := seedErrorsPageTables(t, srv)

	openReq := httptest.NewRequest(http.MethodGet, "http://example.com/errors?resolved=0", nil)
	openRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(openRec, openReq)
	if openRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", openRec.Code, openRec.Body.String())
	}
	openBody := openRec.Body.String()
	if !containsAll(openBody, "boom open", "/errors/"+openID+"/resolve") {
		t.Fatalf("expected open errors page to include unresolved record and resolve URL, got %s", openBody)
	}
	if strings.Contains(openBody, "cache resolved") || strings.Contains(openBody, resolvedID) {
		t.Fatalf("expected resolved record to be excluded from open filter, got %s", openBody)
	}

	resolvedReq := httptest.NewRequest(http.MethodGet, "http://example.com/errors?resolved=1", nil)
	resolvedRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(resolvedRec, resolvedReq)
	if resolvedRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", resolvedRec.Code, resolvedRec.Body.String())
	}
	resolvedBody := resolvedRec.Body.String()
	if !strings.Contains(resolvedBody, "cache resolved") {
		t.Fatalf("expected resolved errors page to include resolved record, got %s", resolvedBody)
	}
	if strings.Contains(resolvedBody, "boom open") {
		t.Fatalf("expected unresolved record to be excluded from resolved filter, got %s", resolvedBody)
	}
}

func TestErrorsPageGroupedModeShowsCounts(t *testing.T) {
	srv := newRenderedErrorsTestServer()
	seedErrorsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/errors?grouped=1&resolved=0", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body, "Grouped mode is best effort", "×2", "boom open") {
		t.Fatalf("expected grouped errors page to show grouped count badge, got %s", body)
	}
	if strings.Contains(body, "cache resolved") {
		t.Fatalf("expected grouped open filter to exclude resolved record, got %s", body)
	}
}

func seedErrorsPageTables(t *testing.T, srv *Server) (string, string) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS otel_logs",
		"DROP TABLE IF EXISTS hyperdx_sessions",
		"DROP TABLE IF EXISTS sobs_error_resolutions",
		"CREATE TABLE IF NOT EXISTS otel_logs (Timestamp DateTime64(3), ServiceName String, TraceId String, SpanId String, Body String, LogAttributes Map(String, String), EventName String, SeverityNumber UInt8, SeverityText String) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS hyperdx_sessions (Timestamp DateTime64(3), ServiceName String, TraceId String, SpanId String, Body String, LogAttributes Map(String, String), EventName String, SeverityNumber UInt8, SeverityText String) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS sobs_error_resolutions (ErrorId String, CreatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(CreatedAt) ORDER BY (ErrorId)",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	openRows := []struct {
		ts      string
		traceID string
		spanID  string
	}{
		{"2026-04-20 10:00:00.000", "trace-open-1", "span-open-1"},
		{"2026-04-20 10:05:00.000", "trace-open-2", "span-open-2"},
	}
	for _, row := range openRows {
		if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('exception.type', ?, 'exception.message', ?), ?, ?, ?)", row.ts, "svc-errors", row.traceID, row.spanID, "boom open", "TimeoutError", "boom open", "exception", uint8(17), "ERROR"); err != nil {
			t.Fatalf("insert open error row: %v", err)
		}
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('exception.type', ?, 'exception.message', ?), ?, ?, ?)", "2026-04-20 11:00:00.000", "svc-errors", "trace-resolved", "span-resolved", "cache resolved", "CacheError", "cache resolved", "error", uint8(17), "ERROR"); err != nil {
		t.Fatalf("insert resolved error row: %v", err)
	}

	openID := lookupSeededErrorID(t, store, "trace-open-2")
	resolvedID := lookupSeededErrorID(t, store, "trace-resolved")
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_error_resolutions (ErrorId) VALUES (?)", resolvedID); err != nil {
		t.Fatalf("insert resolved error id: %v", err)
	}

	return openID, resolvedID
}

func lookupSeededErrorID(t *testing.T, store extensionpoints.ClickHouseStore, traceID string) string {
	t.Helper()
	rows, err := store.Query(t.Context(), "SELECT "+summaryErrorIDSQLExpr()+" AS ErrorId FROM ("+summaryErrorSourcesSQL()+") WHERE TraceId = ? ORDER BY Timestamp DESC LIMIT 1", traceID)
	if err != nil {
		t.Fatalf("query seeded error id: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		t.Fatalf("expected seeded error id for trace %s", traceID)
	}
	var errorID any
	if err := rows.Scan(&errorID); err != nil {
		t.Fatalf("scan seeded error id: %v", err)
	}
	return anyToString(errorID)
}
