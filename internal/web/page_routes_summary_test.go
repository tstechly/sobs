package web

import (
	"crypto/md5"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSummaryPageUsesPythonDerivedDataFlow(t *testing.T) {
	srv := newTestServer()
	seedSummaryPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body,
		"boom unresolved",
		"svc-signal",
		"gpt-4o-mini",
		"1 critical",
		"1 high",
		"Last scan: 2026-04-19T12:00:00",
		"pageview",
	) {
		t.Fatalf("expected summary body to include seeded parity data, got %s", body)
	}
	if strings.Contains(body, `style="max-width:360px;" title="boom resolved" data-label="Message">boom resolved</td>`) {
		t.Fatalf("expected resolved error to be excluded from recent open errors, got %s", body)
	}
}

func seedSummaryPageTables(t *testing.T, srv *Server) {
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
		"DROP TABLE IF EXISTS sobs_error_resolutions",
		"DROP TABLE IF EXISTS v_derived_signals_anomaly",
		"DROP TABLE IF EXISTS sobs_anomaly_rules",
		"DROP TABLE IF EXISTS sobs_cve_findings",
		"CREATE TABLE IF NOT EXISTS otel_logs (Timestamp DateTime64(3), ServiceName String, TraceId String, SpanId String, Body String, LogAttributes Map(String, String), EventName String, SeverityNumber UInt8, SeverityText String) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS hyperdx_sessions (Timestamp DateTime64(3), ServiceName String, TraceId String, SpanId String, Body String, LogAttributes Map(String, String), EventName String, SeverityNumber UInt8, SeverityText String) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS otel_traces (Timestamp DateTime64(3), ServiceName String, SpanAttributes Map(String, String)) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS sobs_error_resolutions (ErrorId String, CreatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(CreatedAt) ORDER BY (ErrorId)",
		"CREATE TABLE IF NOT EXISTS v_derived_signals_anomaly (time DateTime64(3), ServiceName String, SignalSource String, SignalName String, AttrFingerprint String, value Float64, SampleCount UInt32) ENGINE = MergeTree ORDER BY time",
		"CREATE TABLE IF NOT EXISTS sobs_anomaly_rules (Id String, Name String, RuleType String DEFAULT 'threshold', SignalSource String, SignalName String, ServiceName String, AttrFingerprint String, Comparator String, WarningThreshold Float64, CriticalThreshold Float64, SecondarySignalSource String DEFAULT '', SecondarySignalName String DEFAULT '', SecondaryComparator String DEFAULT 'gt', SecondaryWarningThreshold Float64 DEFAULT 0, SecondaryCriticalThreshold Float64 DEFAULT 0, MinSampleCount UInt32 DEFAULT 1, SeasonalBucketsJson String DEFAULT '', IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (SignalSource, SignalName, ServiceName, AttrFingerprint, Id)",
		"CREATE TABLE IF NOT EXISTS sobs_cve_findings (Severity String) ENGINE = MergeTree ORDER BY Severity",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	resolvedTS := "2026-04-19 10:01:00"
	resolvedService := "svc-resolved"
	resolvedErrType := "ValueError"
	resolvedMessage := "boom resolved"
	resolvedTraceID := "trace-resolved"
	resolvedSpanID := "span-resolved"
	resolvedErrorID := summaryTestErrorID(resolvedTS, resolvedService, resolvedErrType, resolvedMessage, resolvedTraceID, resolvedSpanID)

	if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('exception.type', 'RuntimeError', 'exception.message', 'boom unresolved'), ?, ?, ?)",
		"2026-04-19 10:00:00",
		"svc-errors",
		"trace-unresolved",
		"span-unresolved",
		"raw unresolved body",
		"exception",
		uint8(17),
		"ERROR",
	); err != nil {
		t.Fatalf("insert unresolved error log: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('exception.type', ?, 'exception.message', ?), ?, ?, ?)",
		resolvedTS,
		resolvedService,
		resolvedTraceID,
		resolvedSpanID,
		resolvedMessage,
		resolvedErrType,
		resolvedMessage,
		"exception",
		uint8(17),
		"ERROR",
	); err != nil {
		t.Fatalf("insert resolved error log: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_error_resolutions (ErrorId) VALUES (?)", resolvedErrorID); err != nil {
		t.Fatalf("insert error resolution: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map(), ?, ?, ?)",
		"2026-04-19 10:02:00",
		"svc-logs",
		"trace-log",
		"span-log",
		"plain info log",
		"log",
		uint8(9),
		"INFO",
	); err != nil {
		t.Fatalf("insert info log: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map(), ?, ?, ?)",
		"2026-04-19 10:03:00",
		"svc-rum",
		"trace-rum",
		"span-rum",
		"{}",
		"pageview",
		uint8(9),
		"INFO",
	); err != nil {
		t.Fatalf("insert rum pageview: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO otel_traces (Timestamp, ServiceName, SpanAttributes) VALUES (?, ?, map('gen_ai.provider.name', 'openai', 'gen_ai.request.model', 'gpt-4o-mini', 'gen_ai.usage.input_tokens', '12', 'gen_ai.usage.output_tokens', '34'))",
		"2026-04-19 10:04:00",
		"svc-ai",
	); err != nil {
		t.Fatalf("insert ai trace: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO v_derived_signals_anomaly (time, ServiceName, SignalSource, SignalName, AttrFingerprint, value, SampleCount) VALUES (?, ?, ?, ?, ?, ?, ?)",
		time.Now().UTC().Format("2006-01-02 15:04:05.000"),
		"svc-signal",
		"rum",
		"page_load_ms",
		"fp-1",
		250.0,
		uint32(8),
	); err != nil {
		t.Fatalf("insert anomaly row: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_anomaly_rules (Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, MinSampleCount, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-1",
		"Slow page load",
		"threshold",
		"rum",
		"page_load_ms",
		"svc-signal",
		"fp-1",
		"gt",
		100.0,
		200.0,
		uint32(1),
		uint8(0),
		uint64(1),
	); err != nil {
		t.Fatalf("insert anomaly rule: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_cve_findings (Severity) VALUES (?), (?)", "CRITICAL", "HIGH"); err != nil {
		t.Fatalf("insert cve findings: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_app_settings (Key, Value) VALUES (?, ?), (?, ?)",
		"enrichment.cve_enabled", "true",
		"enrichment.cve_last_scan", "2026-04-19T12:00:00Z",
	); err != nil {
		t.Fatalf("insert app settings: %v", err)
	}
}

func summaryTestErrorID(ts string, service string, errType string, message string, traceID string, spanID string) string {
	sum := md5.Sum([]byte(ts + "|" + service + "|" + errType + "|" + message + "|" + traceID + "|" + spanID))
	return fmt.Sprintf("%x", sum)
}

func containsAll(body string, needles ...string) bool {
	for _, needle := range needles {
		if !strings.Contains(body, needle) {
			return false
		}
	}
	return true
}
