package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	sobsstore "github.com/abartrim/sobs/internal/store"
)

func newRenderedWebTrafficTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestWebTrafficHelpPageParity(t *testing.T) {
	srv := newRenderedWebTrafficTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/web-traffic/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Web Traffic &amp; CVE Enrichment Help") {
		t.Fatalf("expected web traffic help content")
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/web-traffic/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestWebTrafficPageAggregatesAndTimeWindowParity(t *testing.T) {
	srv := newRenderedWebTrafficTestServer()
	seedWebTrafficRows(t, srv)

	allReq := httptest.NewRequest(http.MethodGet, "http://example.com/web-traffic", nil)
	allRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(allRec, allReq)
	if allRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", allRec.Code, allRec.Body.String())
	}
	allBody := allRec.Body.String()
	if !containsAll(allBody, "Web Traffic", "Top URLs") {
		t.Fatalf("expected web traffic page to render populated dashboard sections")
	}
	if strings.Contains(allBody, "No RUM events yet") {
		t.Fatalf("expected populated web traffic page to avoid empty-state content")
	}

	windowReq := httptest.NewRequest(http.MethodGet, "http://example.com/web-traffic?from_ts=2026-04-20%2010:59:00&to_ts=2026-04-20%2011:30:00", nil)
	windowRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(windowRec, windowReq)
	if windowRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", windowRec.Code, windowRec.Body.String())
	}
	windowBody := windowRec.Body.String()
	if !containsAll(windowBody, "1 RUM events") {
		t.Fatalf("expected web traffic page to honor from/to timestamp filtering")
	}
}

func seedWebTrafficRows(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS hyperdx_sessions",
		"CREATE TABLE IF NOT EXISTS hyperdx_sessions (Timestamp DateTime64(3), ServiceName String, TraceId String, SpanId String, Body String, LogAttributes Map(String, String), EventName String, SeverityNumber UInt8, SeverityText String) ENGINE = MergeTree ORDER BY Timestamp",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	rows := []struct {
		ts        string
		traceID   string
		spanID    string
		eventName string
		url       string
	}{
		{"2026-04-20 10:00:00.000", "trace-wt-1", "span-wt-1", "pageview", "/checkout"},
		{"2026-04-20 10:01:00.000", "trace-wt-2", "span-wt-2", "error", "/checkout"},
		{"2026-04-20 11:05:00.000", "trace-wt-3", "span-wt-3", "click", "/pricing"},
	}
	for _, row := range rows {
		if _, err := store.Exec(t.Context(), "INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('url', ?), ?, ?, ?)", row.ts, "svc-web", row.traceID, row.spanID, "{}", row.url, row.eventName, uint8(9), "INFO"); err != nil {
			t.Fatalf("insert web traffic row: %v", err)
		}
	}
}
