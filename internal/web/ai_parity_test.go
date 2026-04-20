package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	sobsstore "github.com/abartrim/sobs/internal/store"
)

func newRenderedAITestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestAIHelpPageParity(t *testing.T) {
	srv := newRenderedAITestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/ai/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "AI Transparency Help") {
		t.Fatalf("expected ai help content")
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/ai/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestAIPageParity(t *testing.T) {
	srv := newRenderedAITestServer()
	seedAITracesTable(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/ai", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "AI Transparency") {
		t.Fatalf("expected ai page to render transparency header")
	}
	if !strings.Contains(rec.Body.String(), "svc-ai") {
		t.Fatalf("expected ai page filters to include observed service")
	}
	if !strings.Contains(rec.Body.String(), "gpt-4o-mini") {
		t.Fatalf("expected ai page filters/pricing to include observed model")
	}
	if strings.Contains(rec.Body.String(), `var AI_PRICING = "{}"`) {
		t.Fatalf("expected ai pricing context object, got placeholder string")
	}
}

func TestAPIAISpanAttributesAndConversationGetParity(t *testing.T) {
	srv := newRenderedAITestServer()
	seedAITracesTable(t, srv)

	missingReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/span-attributes", nil)
	missingRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(missingRec, missingReq)
	if missingRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing params, got %d body=%s", missingRec.Code, missingRec.Body.String())
	}

	attrsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/span-attributes?ts=2026-04-20%2010:00:00.000&service=svc-ai&trace_id=trace-ai-1&span_name=ai.chat", nil)
	attrsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(attrsRec, attrsReq)
	if attrsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", attrsRec.Code, attrsRec.Body.String())
	}
	var attrsPayload map[string]any
	if err := json.Unmarshal(attrsRec.Body.Bytes(), &attrsPayload); err != nil {
		t.Fatalf("unmarshal span attrs payload: %v", err)
	}
	if okVal, ok := attrsPayload["ok"].(bool); !ok || !okVal {
		t.Fatalf("expected ok=true from span attrs payload, got %#v", attrsPayload["ok"])
	}
	rawAttrs := anyToString(attrsPayload["raw_attrs"])
	if !strings.Contains(rawAttrs, "gen_ai.request.model") {
		t.Fatalf("expected raw_attrs to include gen_ai.request.model, got %s", rawAttrs)
	}

	convMissingReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/conversation", nil)
	convMissingRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(convMissingRec, convMissingReq)
	if convMissingRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing conversation params, got %d body=%s", convMissingRec.Code, convMissingRec.Body.String())
	}

	convReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/conversation?ts=2026-04-20%2010:00:00.000&service=svc-ai&trace_id=trace-ai-1&span_name=ai.chat", nil)
	convRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(convRec, convReq)
	if convRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", convRec.Code, convRec.Body.String())
	}
	if !containsAll(convRec.Body.String(), "Response", "How many errors", "There were 5 errors") {
		t.Fatalf("expected conversation html to include conversation content, got %s", convRec.Body.String())
	}
}

func TestAPIAIExportParity(t *testing.T) {
	srv := newRenderedAITestServer()
	seedAITracesTable(t, srv)

	jsonReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/export?format=json&service=svc-ai", nil)
	jsonRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(jsonRec, jsonReq)
	if jsonRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", jsonRec.Code, jsonRec.Body.String())
	}
	var jsonPayload map[string]any
	if err := json.Unmarshal(jsonRec.Body.Bytes(), &jsonPayload); err != nil {
		t.Fatalf("unmarshal ai export json payload: %v", err)
	}
	if okVal, ok := jsonPayload["ok"].(bool); !ok || !okVal {
		t.Fatalf("expected ok=true in ai export payload, got %#v", jsonPayload["ok"])
	}
	if _, exists := jsonPayload["records"]; !exists {
		t.Fatalf("expected records key in ai export json payload")
	}

	jsonlReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/export?service=svc-ai", nil)
	jsonlRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(jsonlRec, jsonlReq)
	if jsonlRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", jsonlRec.Code, jsonlRec.Body.String())
	}
	if !strings.Contains(jsonlRec.Body.String(), "\"service\":\"svc-ai\"") {
		t.Fatalf("expected jsonl export to include seeded service row, got %s", jsonlRec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/ai/export", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func seedAITracesTable(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS otel_traces",
		"CREATE TABLE IF NOT EXISTS otel_traces (Timestamp DateTime64(3), TraceId String, SpanId String, ParentSpanId String, SpanName String, ServiceName String, Duration Int64, StatusCode Int32, SpanAttributes Map(String, String)) ENGINE = MergeTree ORDER BY Timestamp",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	attrs := "map('gen_ai.request.model','gpt-4o-mini','gen_ai.provider.name','openai','gen_ai.operation.name','chat','gen_ai.input.messages','[{\"role\":\"user\",\"content\":\"How many errors?\"}]','gen_ai.output.messages','[{\"role\":\"assistant\",\"content\":\"There were 5 errors.\"}]','gen_ai.usage.input_tokens','12','gen_ai.usage.output_tokens','8')"
	insertSQL := "INSERT INTO otel_traces (Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes) VALUES (?, ?, ?, ?, ?, ?, ?, ?, " + attrs + ")"
	if _, err := store.Exec(t.Context(), insertSQL, "2026-04-20 10:00:00.000", "trace-ai-1", "span-ai-1", "", "ai.chat", "svc-ai", int64(125000000), int32(0)); err != nil {
		t.Fatalf("insert ai trace row: %v", err)
	}
}
