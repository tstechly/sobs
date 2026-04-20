package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFieldHintEndpoints(t *testing.T) {
	srv := newTestServer()
	for _, p := range []string{"/api/ai/field-hints"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}

func TestLogsFieldHintsParity(t *testing.T) {
	srv := newTestServer()
	seedLogsValidationTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/logs/field-hints", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal field hints: %v", err)
	}
	for _, key := range []string{"fields", "tag_keys", "operators", "keywords", "functions", "snippets", "attr_keys", "tag_values"} {
		if _, ok := payload[key]; !ok {
			t.Fatalf("expected %s in field hints payload: %v", key, payload)
		}
	}
	fields, _ := payload["fields"].([]any)
	fieldNames := make([]string, 0, len(fields))
	for _, raw := range fields {
		field, _ := raw.(map[string]any)
		fieldNames = append(fieldNames, anyToString(field["name"]))
	}
	if !sliceContains(fieldNames, "level") || !sliceContains(fieldNames, "service") || !sliceContains(fieldNames, "body") {
		t.Fatalf("expected logs field names, got %v", fieldNames)
	}
	if !sliceContains(anySliceToStrings(payload["tag_keys"]), "priority") {
		t.Fatalf("expected priority tag key in payload: %v", payload)
	}
	if !sliceContains(anySliceToStrings(payload["attr_keys"]), "http.route") {
		t.Fatalf("expected http.route attr key in payload: %v", payload)
	}
	functions, _ := payload["functions"].([]any)
	foundHasTag := false
	for _, raw := range functions {
		fn, _ := raw.(map[string]any)
		if anyToString(fn["name"]) == "has_tag" {
			foundHasTag = true
			break
		}
	}
	if !foundHasTag {
		t.Fatalf("expected has_tag function in payload: %v", payload)
	}
}

func TestFilterValidationEndpoints(t *testing.T) {
	srv := newTestServer()
	body := []byte(`{"sql":"service = 'api'"}`)
	for _, p := range []string{"/api/logs/validate-filter", "/api/ai/validate-filter"} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+p, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatalf("unmarshal %s: %v", p, err)
		}
		if payload["ok"] != true {
			t.Fatalf("expected ok=true for %s, got %v", p, payload["ok"])
		}
		normalized, _ := payload["normalized"].(string)
		if normalized == "" || !bytes.Contains([]byte(normalized), []byte("ServiceName")) {
			t.Fatalf("expected normalized sql alias expansion for %s, got %q", p, normalized)
		}
	}
}

func TestLogsValidateFilterParity(t *testing.T) {
	srv := newTestServer()
	seedLogsValidationTables(t, srv)

	okReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-filter", bytes.NewReader([]byte(`{"sql":"level='INFO' AND service='svc-a'"}`)))
	okRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", okRec.Code, okRec.Body.String())
	}
	var okPayload map[string]any
	if err := json.Unmarshal(okRec.Body.Bytes(), &okPayload); err != nil {
		t.Fatalf("unmarshal ok response: %v", err)
	}
	if okPayload["ok"] != true {
		t.Fatalf("expected ok=true, got %v", okPayload)
	}
	normalized := anyToString(okPayload["normalized"])
	if !strings.Contains(normalized, "SeverityText='INFO'") || !strings.Contains(normalized, "ServiceName='svc-a'") {
		t.Fatalf("expected normalized aliases, got %q", normalized)
	}

	hasTagReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-filter", bytes.NewReader([]byte(`{"sql":"has_tag('priority','high')"}`)))
	hasTagRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(hasTagRec, hasTagReq)
	var hasTagPayload map[string]any
	if err := json.Unmarshal(hasTagRec.Body.Bytes(), &hasTagPayload); err != nil {
		t.Fatalf("unmarshal has_tag response: %v", err)
	}
	if hasTagPayload["ok"] != true || !strings.Contains(anyToString(hasTagPayload["normalized"]), "RecordId FROM sobs_record_tags") {
		t.Fatalf("expected has_tag normalization, got %v", hasTagPayload)
	}

	badReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-filter", bytes.NewReader([]byte(`{"sql":"level='INFO"}`)))
	badRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(badRec, badReq)
	var badPayload map[string]any
	if err := json.Unmarshal(badRec.Body.Bytes(), &badPayload); err != nil {
		t.Fatalf("unmarshal bad response: %v", err)
	}
	if badPayload["ok"] != false {
		t.Fatalf("expected ok=false, got %v", badPayload)
	}
	issues, _ := badPayload["issues"].([]any)
	if len(issues) == 0 {
		t.Fatalf("expected validation issues, got %v", badPayload)
	}
}

func TestFilterValidationRejectsUnsafeKeywords(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-filter", bytes.NewReader([]byte(`{"sql":"service='api'; DROP TABLE otel_logs"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if payload["ok"] != false {
		t.Fatalf("expected ok=false, got %v", payload["ok"])
	}
}

func TestRegexValidationEndpoints(t *testing.T) {
	srv := newTestServer()
	body := []byte(`{"pattern":"^foo.*$"}`)
	for _, p := range []string{
		"/api/errors/validate-regex",
		"/api/traces/validate-regex",
		"/api/metrics/validate-regex",
		"/api/rum/validate-regex",
	} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+p, bytes.NewReader(body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}

func TestLogsValidateRegexParity(t *testing.T) {
	srv := newTestServer()
	seedLogsValidationTables(t, srv)

	okReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-regex", bytes.NewReader([]byte(`{"pattern":"\\d+"}`)))
	okRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", okRec.Code, okRec.Body.String())
	}
	var okPayload map[string]any
	if err := json.Unmarshal(okRec.Body.Bytes(), &okPayload); err != nil {
		t.Fatalf("unmarshal ok regex response: %v", err)
	}
	if okPayload["ok"] != true || anyToString(okPayload["error"]) != "" {
		t.Fatalf("expected ok regex response, got %v", okPayload)
	}

	badReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-regex", bytes.NewReader([]byte(`{"pattern":"[unclosed"}`)))
	badRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(badRec, badReq)
	var badPayload map[string]any
	if err := json.Unmarshal(badRec.Body.Bytes(), &badPayload); err != nil {
		t.Fatalf("unmarshal bad regex response: %v", err)
	}
	if badPayload["ok"] != false || anyToString(badPayload["error"]) == "" {
		t.Fatalf("expected invalid regex response, got %v", badPayload)
	}

	emptyReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-regex", bytes.NewReader([]byte(`{"pattern":""}`)))
	emptyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(emptyRec, emptyReq)
	var emptyPayload map[string]any
	if err := json.Unmarshal(emptyRec.Body.Bytes(), &emptyPayload); err != nil {
		t.Fatalf("unmarshal empty regex response: %v", err)
	}
	if emptyPayload["ok"] != true || emptyPayload["sample"] != nil {
		t.Fatalf("expected empty regex response with nil sample, got %v", emptyPayload)
	}

	literalReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-regex", bytes.NewReader([]byte(`{"pattern":"literal \\&& marker","scope":{"service":"regex-validate-logs"}}`)))
	literalRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(literalRec, literalReq)
	var literalPayload map[string]any
	if err := json.Unmarshal(literalRec.Body.Bytes(), &literalPayload); err != nil {
		t.Fatalf("unmarshal literal regex response: %v", err)
	}
	if literalPayload["ok"] != true || anyToString(literalPayload["sample"]) != "literal && marker" {
		t.Fatalf("expected literal sample match, got %v", literalPayload)
	}

	negativeReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/logs/validate-regex", bytes.NewReader([]byte(`{"pattern":"!known-noise-marker","scope":{"service":"regex-validate-logs"}}`)))
	negativeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(negativeRec, negativeReq)
	var negativePayload map[string]any
	if err := json.Unmarshal(negativeRec.Body.Bytes(), &negativePayload); err != nil {
		t.Fatalf("unmarshal negative regex response: %v", err)
	}
	if negativePayload["ok"] != true {
		t.Fatalf("expected ok negative regex response, got %v", negativePayload)
	}
	sample := anyToString(negativePayload["sample"])
	if sample == "" || strings.Contains(sample, "known-noise-marker") {
		t.Fatalf("expected exclusion-respecting sample, got %v", negativePayload)
	}
}

func seedLogsValidationTables(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS otel_logs",
		"DROP TABLE IF EXISTS sobs_record_tags",
		"DROP TABLE IF EXISTS sobs_log_attr_keys",
		"CREATE TABLE IF NOT EXISTS otel_logs (Timestamp DateTime64(6), SeverityText String, ServiceName String, Body String, TraceId String, SpanId String, EventName String, ScopeName String, LogAttributes Map(String, String)) ENGINE = MergeTree ORDER BY Timestamp",
		"CREATE TABLE IF NOT EXISTS sobs_record_tags (RecordType String, RecordId String, TagKey String, TagValue String, IsAuto UInt8, IsDeleted UInt8, Version UInt64) ENGINE = ReplacingMergeTree(Version) ORDER BY (RecordType, RecordId, TagKey)",
		"CREATE TABLE IF NOT EXISTS sobs_log_attr_keys (RecordType String, AttrKey String, IsDeleted UInt8, Version UInt64) ENGINE = ReplacingMergeTree(Version) ORDER BY (RecordType, AttrKey)",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	now := time.Now().UTC()
	rows := []struct {
		ts      string
		level   string
		service string
		body    string
		traceID string
		spanID  string
		event   string
		scope   string
		route   string
	}{
		{ts: now.Format("2006-01-02 15:04:05.000000"), level: "INFO", service: "svc-a", body: "body 123 marker", traceID: "trace-a", spanID: "span-a", event: "log", scope: "scope-a", route: "/svc-a"},
		{ts: now.Add(time.Microsecond).Format("2006-01-02 15:04:05.000000"), level: "INFO", service: "regex-validate-logs", body: "literal && marker", traceID: "trace-b", spanID: "span-b", event: "log", scope: "scope-b", route: "/regex"},
		{ts: now.Add(2 * time.Microsecond).Format("2006-01-02 15:04:05.000000"), level: "INFO", service: "regex-validate-logs", body: "known-noise-marker", traceID: "trace-c", spanID: "span-c", event: "log", scope: "scope-b", route: "/regex"},
	}
	for _, row := range rows {
		if _, err := store.Exec(t.Context(), "INSERT INTO otel_logs (Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId, EventName, ScopeName, LogAttributes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, map('http.route', ?))", row.ts, row.level, row.service, row.body, row.traceID, row.spanID, row.event, row.scope, row.route); err != nil {
			t.Fatalf("insert log row: %v", err)
		}
	}
	recordID := webRecordIDForLog(rows[0].ts, rows[0].service, rows[0].traceID, rows[0].spanID)
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_record_tags (RecordType, RecordId, TagKey, TagValue, IsAuto, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", "log", recordID, "priority", "high", uint8(0), uint8(0), uint64(1)); err != nil {
		t.Fatalf("insert tag row: %v", err)
	}
	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_log_attr_keys (RecordType, AttrKey, IsDeleted, Version) VALUES (?, ?, ?, ?)", "log", "http.route", uint8(0), uint64(1)); err != nil {
		t.Fatalf("insert attr key row: %v", err)
	}
}

func anySliceToStrings(raw any) []string {
	items, _ := raw.([]any)
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, anyToString(item))
	}
	return out
}

func sliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
