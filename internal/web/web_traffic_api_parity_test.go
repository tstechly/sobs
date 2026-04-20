package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWebTrafficAPIMethodParity(t *testing.T) {
	srv := newRenderedWebTrafficTestServer()

	paths := []string{
		"/api/web-traffic/geo",
		"/api/web-traffic/browsers",
		"/api/web-traffic/os",
		"/api/web-traffic/timezones",
		"/api/web-traffic/languages",
		"/api/web-traffic/devices",
	}
	for _, path := range paths {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405 for %s, got %d", path, rec.Code)
		}
	}
}

func TestWebTrafficAPIShapeAndFilteringParity(t *testing.T) {
	srv := newRenderedWebTrafficTestServer()
	seedWebTrafficAPIRows(t, srv)

	assertPayload := func(path string, key string) map[string]any {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d body=%s", path, rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal %s: %v", path, err)
		}
		if okVal, ok := payload["ok"].(bool); !ok || !okVal {
			t.Fatalf("expected ok=true for %s, got %#v", path, payload["ok"])
		}
		if _, exists := payload[key]; !exists {
			t.Fatalf("expected key %q in %s payload", key, path)
		}
		return payload
	}

	geoPayload := assertPayload("/api/web-traffic/geo", "country_counts")
	if ipDetails, ok := geoPayload["ip_details"].([]any); !ok || len(ipDetails) == 0 {
		t.Fatalf("expected non-empty ip_details in geo payload")
	}

	browsersPayload := assertPayload("/api/web-traffic/browsers", "browsers")
	if browsers, ok := browsersPayload["browsers"].([]any); !ok || len(browsers) == 0 {
		t.Fatalf("expected non-empty browsers payload")
	}

	osPayload := assertPayload("/api/web-traffic/os", "operating_systems")
	if systems, ok := osPayload["operating_systems"].([]any); !ok || len(systems) == 0 {
		t.Fatalf("expected non-empty operating_systems payload")
	}

	tzPayload := assertPayload("/api/web-traffic/timezones", "timezones")
	if timezones, ok := tzPayload["timezones"].([]any); !ok || len(timezones) == 0 {
		t.Fatalf("expected non-empty timezones payload")
	}

	langPayload := assertPayload("/api/web-traffic/languages", "languages")
	if languages, ok := langPayload["languages"].([]any); !ok || len(languages) == 0 {
		t.Fatalf("expected non-empty languages payload")
	}

	devicePayload := assertPayload("/api/web-traffic/devices", "devices")
	if devices, ok := devicePayload["devices"].([]any); !ok || len(devices) == 0 {
		t.Fatalf("expected non-empty devices payload")
	}

	filteredPayload := assertPayload("/api/web-traffic/languages?from_ts=2026-04-20%2011:00:00&to_ts=2026-04-20%2011:30:00", "languages")
	languages := filteredPayload["languages"].([]any)
	if len(languages) != 1 {
		t.Fatalf("expected filtered window to return one language bucket, got %d", len(languages))
	}
}

func seedWebTrafficAPIRows(t *testing.T, srv *Server) {
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
		ts         string
		traceID    string
		spanID     string
		eventName  string
		ip         string
		browser    string
		version    string
		osName     string
		osVersion  string
		timezone   string
		language   string
		deviceType string
	}{
		{"2026-04-20 10:00:00.000", "trace-api-1", "span-api-1", "pageview", "1.1.1.1", "Chrome", "123", "macOS", "14", "UTC", "en-US", "desktop"},
		{"2026-04-20 10:05:00.000", "trace-api-2", "span-api-2", "click", "2.2.2.2", "Safari", "17", "iOS", "18", "America/New_York", "en-US", "mobile"},
		{"2026-04-20 11:05:00.000", "trace-api-3", "span-api-3", "error", "3.3.3.3", "Firefox", "125", "Linux", "6", "Europe/Berlin", "de-DE", "desktop"},
	}
	for _, row := range rows {
		if _, err := store.Exec(
			t.Context(),
			"INSERT INTO hyperdx_sessions (Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, EventName, SeverityNumber, SeverityText) VALUES (?, ?, ?, ?, ?, map('client.ip', ?, 'browser.context.browserName', ?, 'browser.context.browserVersion', ?, 'browser.context.osName', ?, 'browser.context.osVersion', ?, 'browser.context.timezone', ?, 'browser.context.language', ?, 'browser.context.deviceClass', ?), ?, ?, ?)",
			row.ts,
			"svc-web-api",
			row.traceID,
			row.spanID,
			"{}",
			row.ip,
			row.browser,
			row.version,
			row.osName,
			row.osVersion,
			row.timezone,
			row.language,
			row.deviceType,
			row.eventName,
			uint8(9),
			"INFO",
		); err != nil {
			t.Fatalf("insert web traffic api row: %v", err)
		}
	}
}
