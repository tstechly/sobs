package otlpreceiver

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	rumfeature "github.com/abartrim/sobs/internal/features/rum"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
)

type capturePipeline struct {
	traces   int
	metrics  int
	logs     int
	rums     int
	ais      int
	errorsV1 int
	traceErr error
	metricErr error
	logErr   error
	rumErr   error
	aiErr    error
	errorErr error
	lastRUM  *RUMIngestRequest
	lastAI   *AIIngestRequest
	lastError *ErrorIngestRequest
}

func (p *capturePipeline) ConsumeTraces(_ context.Context, _ *coltracepb.ExportTraceServiceRequest) error {
	p.traces++
	return p.traceErr
}

func (p *capturePipeline) ConsumeMetrics(_ context.Context, _ *colmetricpb.ExportMetricsServiceRequest) error {
	p.metrics++
	return p.metricErr
}

func (p *capturePipeline) ConsumeLogs(_ context.Context, _ *collogspb.ExportLogsServiceRequest) error {
	p.logs++
	return p.logErr
}

func (p *capturePipeline) ConsumeRUM(_ context.Context, req *RUMIngestRequest) error {
	p.rums++
	p.lastRUM = req
	return p.rumErr
}

func (p *capturePipeline) ConsumeAI(_ context.Context, req *AIIngestRequest) error {
	p.ais++
	p.lastAI = req
	return p.aiErr
}

func (p *capturePipeline) ConsumeErrorsV1(_ context.Context, req *ErrorIngestRequest) error {
	p.errorsV1++
	p.lastError = req
	return p.errorErr
}

func TestHTTPAcceptEndpoints(t *testing.T) {
	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	tests := []struct {
		path string
		body []byte
	}{
		{path: "/v1/traces", body: mustMarshalProto(t, &coltracepb.ExportTraceServiceRequest{})},
		{path: "/v1/metrics", body: mustMarshalProto(t, &colmetricpb.ExportMetricsServiceRequest{})},
		{path: "/v1/logs", body: mustMarshalProto(t, &collogspb.ExportLogsServiceRequest{})},
	}
	for _, test := range tests {
		req := httptest.NewRequest("POST", "http://example.com"+test.path, bytes.NewReader(test.body))
		req.Header.Set("Content-Type", "application/x-protobuf")
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", test.path, w.Code)
		}
	}
	if pipeline.traces != 1 || pipeline.metrics != 1 || pipeline.logs != 1 {
		t.Fatalf("expected all OTLP pipelines to be invoked once, got traces=%d metrics=%d logs=%d", pipeline.traces, pipeline.metrics, pipeline.logs)
	}
}

func TestHTTPAcceptTracesReturnsAcceptedCount(t *testing.T) {
	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	body := []byte(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"AAAAAAAAAAA=","name":"root span"},{"traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"AQAAAAAAAAA=","name":"child span"}]}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/traces", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := int(payload["accepted"].(float64)); got != 2 {
		t.Fatalf("expected accepted=2, got %d payload=%v", got, payload)
	}
	if pipeline.traces != 1 {
		t.Fatalf("expected traces pipeline to be invoked once, got %d", pipeline.traces)
	}
}

func TestHTTPAcceptTracesReturns503WhenWriteQueueIsFull(t *testing.T) {
	pipeline := &capturePipeline{traceErr: WriteQueueFullError{}}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/traces", bytes.NewReader([]byte(`{"resourceSpans":[{"scopeSpans":[{"spans":[{"traceId":"AAAAAAAAAAAAAAAAAAAAAA==","spanId":"AAAAAAAAAAA=","name":"root span"}]}]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "write queue is full" {
		t.Fatalf("expected queue full error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptTracesPublishesTraceAndAIEvents(t *testing.T) {
	pipeline := &capturePipeline{}
	tail := NewTailBroker()
	subscriber, unsubscribe := tail.Subscribe(4)
	defer unsubscribe()
	srv := NewHTTPServerWithPipelineAndTailBroker(pipeline, tail)
	mux := http.NewServeMux()
	srv.Register(mux)

	reqBody := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "svc-trace"}},
			}}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Spans: []*tracev1.Span{{
					TraceId:           []byte{0x01, 0x02, 0x03},
					SpanId:            []byte{0x0a, 0x0b},
					Name:              "gen-ai span",
					StartTimeUnixNano: 1713520800000000000,
					EndTimeUnixNano:   1713520801500000000,
					Status:            &tracev1.Status{Code: tracev1.Status_STATUS_CODE_OK},
					Attributes: []*commonv1.KeyValue{
						{Key: "gen_ai.provider.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "openai"}}},
						{Key: "gen_ai.request.model", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "gpt-5"}}},
						{Key: "gen_ai.operation.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "chat.completions"}}},
					},
				}},
			}},
		}},
	}
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/traces", bytes.NewReader(mustMarshalProto(t, reqBody)))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	traceEvent := readTailEvent(t, subscriber)
	aiEvent := readTailEvent(t, subscriber)
	if traceEvent.Source != "traces" {
		t.Fatalf("expected trace event, got %#v", traceEvent)
	}
	if traceEvent.Service != "svc-trace" || traceEvent.Name != "gen-ai span" || traceEvent.Status != "OK" {
		t.Fatalf("unexpected trace event payload: %#v", traceEvent)
	}
	if aiEvent.Source != "ai" {
		t.Fatalf("expected ai event, got %#v", aiEvent)
	}
	if aiEvent.Provider != "openai" || aiEvent.Model != "gpt-5" || aiEvent.Operation != "chat.completions" {
		t.Fatalf("unexpected ai event payload: %#v", aiEvent)
	}
}

func TestHTTPAcceptMetricsReturnsAcceptedCount(t *testing.T) {
	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	body := []byte(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"requests","sum":{"dataPoints":[{"asInt":"1"},{"asInt":"2"}]}}]}]}]}`)
	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/metrics", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected json content type, got %q", ct)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := int(payload["accepted"].(float64)); got != 2 {
		t.Fatalf("expected accepted=2, got %d payload=%v", got, payload)
	}
	if pipeline.metrics != 1 {
		t.Fatalf("expected metrics pipeline to be invoked once, got %d", pipeline.metrics)
	}
}

func TestHTTPAcceptMetricsReturns503WhenWriteQueueIsFull(t *testing.T) {
	pipeline := &capturePipeline{metricErr: WriteQueueFullError{}}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/metrics", bytes.NewReader([]byte(`{"resourceMetrics":[{"scopeMetrics":[{"metrics":[{"name":"requests","sum":{"dataPoints":[{"asInt":"1"}]}}]}]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "write queue is full" {
		t.Fatalf("expected queue full error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptRUMReturnsAcceptedCount(t *testing.T) {
	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum", bytes.NewReader([]byte(`{"events":[{"type":"pageview","sessionId":"sess-1"},{"type":"error","sessionId":"sess-2"}]}`)))
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := int(payload["accepted"].(float64)); got != 2 {
		t.Fatalf("expected accepted=2, got %d payload=%v", got, payload)
	}
	if pipeline.rums != 1 || pipeline.lastRUM == nil {
		t.Fatalf("expected rum pipeline to be invoked once, got %d", pipeline.rums)
	}
	if pipeline.lastRUM.ClientIP != "203.0.113.10" {
		t.Fatalf("expected forwarded client ip, got %q", pipeline.lastRUM.ClientIP)
	}
}

func TestHTTPAcceptRUMPersistsRowsThroughRealPipelinePath(t *testing.T) {
	t.Setenv("SOBS_INGEST_WAIT_FOR_RESULT", "1")
	resetRUMBrowserContextCache()
	store := &captureStore{}
	factory := &captureStoreFactory{store: store}
	pipeline := NewAsyncLogPipeline(NewStorePipeline(factory))
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum", bytes.NewReader([]byte(`{"events":[{"type":"pageview","timestamp":"2024-01-01T00:00:00Z","sessionId":"sess-http-1","url":"https://example.com/"}]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS hyperdx_sessions",
		"INSERT INTO hyperdx_sessions",
		`"sessionId":"sess-http-1"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
}

func TestHTTPAcceptRUMReturns503WhenWriteQueueIsFull(t *testing.T) {
	pipeline := &capturePipeline{rumErr: WriteQueueFullError{}}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum", bytes.NewReader([]byte(`[{"type":"pageview","sessionId":"sess-1"}]`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "write queue is full" {
		t.Fatalf("expected queue full error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptRUMReturns500WhenAsyncWorkerFailsAndWaitIsEnabled(t *testing.T) {
	t.Setenv("SOBS_INGEST_WAIT_FOR_RESULT", "1")
	pipeline := NewAsyncLogPipeline(&capturePipeline{rumErr: errors.New("boom")})
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum", bytes.NewReader([]byte(`[ {"type":"pageview","sessionId":"sess-1"} ]`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "rum ingest write failed" {
		t.Fatalf("expected worker failure error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptRUMOriginBoundClientTokenAuth(t *testing.T) {
	t.Setenv("SOBS_RUM_CLIENT_AUTH_MODE", "origin")
	t.Setenv("SOBS_RUM_CLIENT_SIGNING_KEY", "rum-client-secret")
	var svc rumfeature.Service
	token, _ := svc.NewClientToken("rum-client-secret", "https://example.com", "my-app", 900)

	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	okReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum", bytes.NewReader([]byte(`[{"type":"pageview","sessionId":"sess-auth-1","appName":"my-app","clientAuthToken":"`+token+`"}]`)))
	okReq.Header.Set("Origin", "https://example.com")
	okRec := httptest.NewRecorder()
	mux.ServeHTTP(okRec, okReq)
	if okRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", okRec.Code, okRec.Body.String())
	}

	badReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum", bytes.NewReader([]byte(`[{"type":"pageview","sessionId":"sess-auth-2","appName":"my-app","clientAuthToken":"`+token+`"}]`)))
	badReq.Header.Set("Origin", "https://evil.example")
	badRec := httptest.NewRecorder()
	mux.ServeHTTP(badRec, badReq)
	if badRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", badRec.Code, badRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(badRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "RUM client token origin mismatch" {
		t.Fatalf("expected origin mismatch error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptAIReturnsOKAndPublishesTail(t *testing.T) {
	pipeline := &capturePipeline{}
	tail := NewTailBroker()
	subscriber, unsubscribe := tail.Subscribe(2)
	defer unsubscribe()
	srv := NewHTTPServerWithPipelineAndTailBroker(pipeline, tail)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/ai", bytes.NewReader([]byte(`{"timestamp":"2024-01-01T00:00:00Z","model":"gpt-5","operation":" CHAT ","duration_ms":12.34,"provider":"openai","service":"svc-ai","tokens_in":12,"tokens_out":34}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["ok"]; got != true {
		t.Fatalf("expected ok=true, got %v payload=%v", got, payload)
	}
	if pipeline.ais != 1 || pipeline.lastAI == nil {
		t.Fatalf("expected ai pipeline to be invoked once, got %d", pipeline.ais)
	}
	if pipeline.lastAI.Operation != "chat" || pipeline.lastAI.SpanName != "chat gpt-5" {
		t.Fatalf("unexpected normalized ai request: %#v", pipeline.lastAI)
	}
	aiEvent := readTailEvent(t, subscriber)
	if aiEvent.Source != "ai" || aiEvent.Provider != "openai" || aiEvent.Model != "gpt-5" {
		t.Fatalf("unexpected ai tail event: %#v", aiEvent)
	}
	if aiEvent.TokensIn != 12 || aiEvent.TokensOut != 34 {
		t.Fatalf("unexpected token counts in ai event: %#v", aiEvent)
	}
}

func TestHTTPAcceptAIReturns503WhenWriteQueueIsFull(t *testing.T) {
	pipeline := &capturePipeline{aiErr: WriteQueueFullError{}}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/ai", bytes.NewReader([]byte(`{"model":"gpt-5"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "write queue is full" {
		t.Fatalf("expected queue full error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptErrorsReturnsOK(t *testing.T) {
	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/errors", bytes.NewReader([]byte(`{"timestamp":"2024-01-01T00:00:00Z","type":"ReferenceError","message":"boom","stack":"at fn (bundle.js:1:1)","service":"svc-errors","trace_id":"trace-1","span_id":"span-1","trace_flags":1,"attributes":{"release":"2024.01.01","handled":false}}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["ok"]; got != true {
		t.Fatalf("expected ok=true, got %v payload=%v", got, payload)
	}
	if pipeline.errorsV1 != 1 || pipeline.lastError == nil {
		t.Fatalf("expected errors pipeline to be invoked once, got %d", pipeline.errorsV1)
	}
	if pipeline.lastError.ExceptionType != "ReferenceError" || pipeline.lastError.Message != "boom" {
		t.Fatalf("unexpected normalized error request: %#v", pipeline.lastError)
	}
	if pipeline.lastError.TraceFlags != 0 {
		t.Fatalf("expected direct error trace flags to normalize to 0, got %#v", pipeline.lastError)
	}
	if pipeline.lastError.Attributes["release"] != "2024.01.01" || pipeline.lastError.Attributes["handled"] != "false" {
		t.Fatalf("unexpected normalized error attributes: %#v", pipeline.lastError.Attributes)
	}
	if pipeline.lastError.Attributes["exception.type"] != "ReferenceError" || pipeline.lastError.Attributes["exception.message"] != "boom" {
		t.Fatalf("expected exception attributes to be populated, got %#v", pipeline.lastError.Attributes)
	}
}

func TestHTTPAcceptErrorsReturns503WhenWriteQueueIsFull(t *testing.T) {
	pipeline := &capturePipeline{errorErr: WriteQueueFullError{}}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/errors", bytes.NewReader([]byte(`{"message":"boom"}`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "write queue is full" {
		t.Fatalf("expected queue full error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptErrorsRejectsInvalidJSON(t *testing.T) {
	srv := NewHTTPServer()
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/errors", bytes.NewReader([]byte(`not-json`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "invalid json" {
		t.Fatalf("expected invalid json error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptLogsReturnsAcceptedCount(t *testing.T) {
	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	body := []byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"a"}},{"body":{"stringValue":"b"}}]}]}]}`)
	req := httptest.NewRequest("POST", "http://example.com/v1/logs", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("expected json content type, got %q", ct)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := int(payload["accepted"].(float64)); got != 2 {
		t.Fatalf("expected accepted=2, got %d payload=%v", got, payload)
	}
	if pipeline.logs != 1 {
		t.Fatalf("expected logs pipeline to be invoked once, got %d", pipeline.logs)
	}
}

func TestHTTPAcceptLogsSupportsGzipJSON(t *testing.T) {
	pipeline := &capturePipeline{}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	var compressed bytes.Buffer
	gzipWriter := gzip.NewWriter(&compressed)
	if _, err := gzipWriter.Write([]byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"gzip"}}]}]}]}`)); err != nil {
		t.Fatalf("write gzip body: %v", err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatalf("close gzip writer: %v", err)
	}

	req := httptest.NewRequest("POST", "http://example.com/v1/logs", bytes.NewReader(compressed.Bytes()))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := int(payload["accepted"].(float64)); got != 1 {
		t.Fatalf("expected accepted=1, got %d payload=%v", got, payload)
	}
}

func TestHTTPAcceptLogsRejectsJSONArrayPayload(t *testing.T) {
	srv := NewHTTPServer()
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest("POST", "http://example.com/v1/logs", bytes.NewReader([]byte(`[]`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "failed to parse json body" {
		t.Fatalf("expected parse json error, got %v payload=%v", got, payload)
	}
}

func TestHTTPAcceptLogsReturns503WhenWriteQueueIsFull(t *testing.T) {
	pipeline := &capturePipeline{logErr: WriteQueueFullError{}}
	srv := NewHTTPServerWithPipeline(pipeline)
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest("POST", "http://example.com/v1/logs", bytes.NewReader([]byte(`{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"body":{"stringValue":"a"}}]}]}]}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d body=%s", w.Code, w.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := payload["error"]; got != "write queue is full" {
		t.Fatalf("expected queue full error, got %v payload=%v", got, payload)
	}
	if !errors.As(pipeline.logErr, new(WriteQueueFullError)) {
		t.Fatal("expected queue full error type")
	}
}

func readTailEvent(t *testing.T, subscriber <-chan TailEvent) TailEvent {
	t.Helper()
	select {
	case event := <-subscriber:
		return event
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for tail event")
	}
	return TailEvent{}
}

func TestAdditionalIngestEndpoints(t *testing.T) {
	srv := NewHTTPServer()
	mux := http.NewServeMux()
	srv.Register(mux)

	paths := []string{"/v1/errors"}
	for _, p := range paths {
		req := httptest.NewRequest("POST", "http://example.com"+p, bytes.NewReader([]byte(`{"ok":true}`)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, w.Code)
		}
	}
}

func TestAdditionalIngestEndpointsAcceptJSONArray(t *testing.T) {
	srv := NewHTTPServer()
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest("POST", "http://example.com/v1/rum", bytes.NewReader([]byte(`[{"type":"pageview","appName":"demo"}]`)))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /v1/rum array payload, got %d", w.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if got := int(payload["accepted"].(float64)); got != 1 {
		t.Fatalf("expected accepted=1, got %d payload=%v", got, payload)
	}
}

func TestRejectInvalidIngestPayload(t *testing.T) {
	srv := NewHTTPServer()
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest("POST", "http://example.com/v1/traces", bytes.NewReader([]byte("not-protobuf")))
	req.Header.Set("Content-Type", "application/x-protobuf")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestRejectNonPostIngestMethod(t *testing.T) {
	srv := NewHTTPServer()
	mux := http.NewServeMux()
	srv.Register(mux)

	req := httptest.NewRequest("GET", "http://example.com/v1/traces", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func mustMarshalProto(t *testing.T, msg proto.Message) []byte {
	t.Helper()
	body, err := proto.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal proto: %v", err)
	}
	return body
}
