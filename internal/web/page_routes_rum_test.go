package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	sobsstore "github.com/abartrim/sobs/internal/store"
)

func newRenderedRUMTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestRUMHelpPageParity(t *testing.T) {
	srv := newRenderedRUMTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/rum/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "RUM Help") {
		t.Fatalf("expected rum help content, got %s", getRec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/rum/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestRUMPageMethodAndEmptyStateParity(t *testing.T) {
	srv := newRenderedRUMTestServer()
	seedRUMTable(t, srv)

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/rum", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/rum?view=sessions", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	body := getRec.Body.String()
	if !strings.Contains(body, "No RUM sessions yet") {
		t.Fatalf("expected empty sessions message, got %s", body)
	}
}

func TestRUMPageSessionsAndEventsFiltersParity(t *testing.T) {
	srv := newRenderedRUMTestServer()
	seedRUMTable(t, srv)
	seedRUMEvents(t, srv)

	sessionsReq := httptest.NewRequest(http.MethodGet, "http://example.com/rum?view=sessions&error_source=frontend", nil)
	sessionsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(sessionsRec, sessionsReq)
	if sessionsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", sessionsRec.Code, sessionsRec.Body.String())
	}
	sessionsBody := sessionsRec.Body.String()
	if !containsAll(sessionsBody, "1 sessions", "Error session", "session-aaaa1111bbbb2222", "trace-rum-1", "frontend boom") {
		t.Fatalf("expected sessions view to include seeded frontend session evidence")
	}

	eventsReq := httptest.NewRequest(http.MethodGet, "http://example.com/rum?view=events&type=error&error_source=frontend&q=frontend%20boom", nil)
	eventsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(eventsRec, eventsReq)
	if eventsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", eventsRec.Code, eventsRec.Body.String())
	}
	eventsBody := eventsRec.Body.String()
	if !containsAll(eventsBody, "1 events", "frontend boom", "trace-rum-1") {
		t.Fatalf("expected events view to include filtered frontend error event")
	}
}

func TestBuildRUMEventItemDerivesFlagsAndSessionFields(t *testing.T) {
	item := buildRUMEventItem(
		"2026-04-19 10:11:12",
		"error",
		`{"message":"boom","artifact":{"id":"artifact-1"},"replay":{"url":"https://example.test/replay"}}`,
		`{"url":"/checkout","url.full":"https://example.test/checkout"}`,
		"trace-123",
		"span-456",
		"session-abcdef123456",
	)

	if got := anyToString(item["session_key"]); got != "session-abcdef123456" {
		t.Fatalf("expected session_key to be preserved, got %q", got)
	}
	if got := anyToString(item["session_id"]); got != "session-" {
		t.Fatalf("expected truncated session_id, got %q", got)
	}
	if got := anyToString(item["url"]); got != "/checkout" {
		t.Fatalf("expected url from attrs, got %q", got)
	}
	if got := anyToString(item["trace_id"]); got != "trace-123" {
		t.Fatalf("expected trace_id, got %q", got)
	}
	if got := anyToString(item["span_id"]); got != "span-456" {
		t.Fatalf("expected span_id, got %q", got)
	}
	if got, ok := item["has_artifact"].(bool); !ok || !got {
		t.Fatalf("expected has_artifact=true, got %#v", item["has_artifact"])
	}
	if got, ok := item["has_replay"].(bool); !ok || !got {
		t.Fatalf("expected has_replay=true, got %#v", item["has_replay"])
	}

	data, ok := item["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data map, got %#v", item["data"])
	}
	if got := anyToString(data["message"]); got != "boom" {
		t.Fatalf("expected body message, got %q", got)
	}
	if got := anyToString(data["traceId"]); got != "trace-123" {
		t.Fatalf("expected traceId backfilled into data, got %q", got)
	}
	if got := anyToString(data["spanId"]); got != "span-456" {
		t.Fatalf("expected spanId backfilled into data, got %q", got)
	}
}

func TestBuildRUMEventItemDerivesSessionKeyFromNativeMapAttrs(t *testing.T) {
	item := buildRUMEventItem(
		"2026-04-20 10:11:12",
		"error",
		map[string]any{"message": "boom", "traceId": "trace-native"},
		map[string]any{"session.id": "session-native-123456", "url": "/native"},
		"",
		"",
		"",
	)

	if got := anyToString(item["session_key"]); got != "session-native-123456" {
		t.Fatalf("expected session_key from native attrs, got %q", got)
	}
	if got := anyToString(item["session_id"]); got != "session-" {
		t.Fatalf("expected truncated session_id, got %q", got)
	}
	if got := anyToString(item["url"]); got != "/native" {
		t.Fatalf("expected url from native attrs, got %q", got)
	}
	if got := anyToString(item["trace_id"]); got != "trace-native" {
		t.Fatalf("expected trace_id from body when column is empty, got %q", got)
	}
}

func TestRUMEventCapabilityHelpers(t *testing.T) {
	events := []map[string]any{
		{"trace_id": "", "has_replay": false, "has_artifact": false},
		{"trace_id": "trace-1", "has_replay": true, "has_artifact": false},
		{"trace_id": "trace-2", "has_replay": false, "has_artifact": true},
	}

	if !rumEventsHaveCapability(events, "has_replay") {
		t.Fatalf("expected replay capability to be detected")
	}
	if !rumEventsHaveCapability(events, "has_artifact") {
		t.Fatalf("expected artifact capability to be detected")
	}
	if got := firstTraceID(events); got != "trace-1" {
		t.Fatalf("expected first trace id trace-1, got %q", got)
	}
}

func seedRUMTable(t *testing.T, srv *Server) {
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
}

func seedRUMEvents(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := store.Exec(t.Context(), "INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('session.id', ?, 'url', ?, 'errorSource', ?), ?, ?, ?)", "2026-04-20 10:05:00.000", "svc-rum", "trace-rum-1", "span-rum-1", `{"message":"frontend boom","errorSource":"frontend","traceId":"trace-rum-1","artifact":{"id":"artifact-1"}}`, "session-aaaa1111bbbb2222", "/checkout", "frontend", "error", uint8(17), "ERROR"); err != nil {
		t.Fatalf("insert frontend error row: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('session.id', ?, 'url', ?, 'errorSource', ?), ?, ?, ?)", "2026-04-20 10:04:00.000", "svc-rum", "trace-rum-2", "span-rum-2", `{"message":"backend crash","errorSource":"backend","traceId":"trace-rum-2"}`, "session-zzzz9999yyyy0000", "/admin", "backend", "error", uint8(17), "ERROR"); err != nil {
		t.Fatalf("insert backend error row: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('session.id', ?, 'url', ?), ?, ?, ?)", "2026-04-20 10:06:00.000", "svc-rum", "trace-rum-1", "span-rum-3", `{"title":"Checkout","traceId":"trace-rum-1"}`, "session-aaaa1111bbbb2222", "/checkout", "pageview", uint8(9), "INFO"); err != nil {
		t.Fatalf("insert pageview row: %v", err)
	}
}
