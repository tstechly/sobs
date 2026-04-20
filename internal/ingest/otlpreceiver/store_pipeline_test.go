package otlpreceiver

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/extensionpoints"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	logsv1 "go.opentelemetry.io/proto/otlp/logs/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

type captureResult struct{}

func (captureResult) RowsAffected() (int64, error) { return 1, nil }

type captureRows struct {
	rows  [][]any
	index int
}

func (r *captureRows) Next() bool { return r.index < len(r.rows) }
func (r *captureRows) Scan(dest ...any) error {
	row := r.rows[r.index]
	r.index++
	for index, target := range dest {
		if index >= len(row) {
			continue
		}
		value := row[index]
		switch typed := target.(type) {
		case *string:
			*typed = fmt.Sprint(value)
		case *uint64:
			switch cast := value.(type) {
			case uint64:
				*typed = cast
			case int:
				*typed = uint64(cast)
			default:
				return fmt.Errorf("unsupported uint64 value %T", value)
			}
		case *any:
			*typed = value
		default:
			return fmt.Errorf("unsupported scan target %T", target)
		}
	}
	return nil
}
func (r *captureRows) Err() error   { return nil }
func (r *captureRows) Close() error { return nil }

type captureStore struct {
	execs     []string
	queryFunc func(query string, args ...any) (extensionpoints.RowIterator, error)
}

func (s *captureStore) Ping(_ context.Context) error { return nil }
func (s *captureStore) Query(_ context.Context, query string, args ...any) (extensionpoints.RowIterator, error) {
	if s.queryFunc != nil {
		return s.queryFunc(query, args...)
	}
	return &captureRows{}, nil
}
func (s *captureStore) Exec(_ context.Context, query string, _ ...any) (extensionpoints.Result, error) {
	s.execs = append(s.execs, query)
	return captureResult{}, nil
}
func (s *captureStore) Close() error { return nil }

type captureStoreFactory struct {
	store *captureStore
}

func (f *captureStoreFactory) Open(_ context.Context) (extensionpoints.ClickHouseStore, error) {
	return f.store, nil
}

func TestStorePipelinePersistsPerSignalTables(t *testing.T) {
	store := &captureStore{}
	factory := &captureStoreFactory{store: store}
	pipeline := NewStorePipeline(factory).(*StorePipeline)

	if err := pipeline.ConsumeTraces(context.Background(), &coltracepb.ExportTraceServiceRequest{}); err != nil {
		t.Fatalf("consume traces: %v", err)
	}
	if err := pipeline.ConsumeMetrics(context.Background(), &colmetricpb.ExportMetricsServiceRequest{}); err != nil {
		t.Fatalf("consume metrics: %v", err)
	}
	if err := pipeline.ConsumeLogs(context.Background(), &collogspb.ExportLogsServiceRequest{}); err != nil {
		t.Fatalf("consume logs: %v", err)
	}
	if err := pipeline.ConsumeOpaqueJSON(context.Background(), "/v1/opaque-sample", map[string]any{"ok": true}); err != nil {
		t.Fatalf("consume opaque: %v", err)
	}

	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS otel_logs",
		"CREATE TABLE IF NOT EXISTS otel_traces",
		"CREATE TABLE IF NOT EXISTS otel_metrics_gauge",
		"CREATE TABLE IF NOT EXISTS otel_metrics_sum",
		"CREATE TABLE IF NOT EXISTS otel_metrics_histogram",
		"CREATE TABLE IF NOT EXISTS sobs_ingest_opaque",
		"CREATE TABLE IF NOT EXISTS sobs_tag_rules",
		"CREATE TABLE IF NOT EXISTS sobs_record_tags",
		"INSERT INTO sobs_ingest_opaque",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
	// Empty requests should produce no data INSERTs (nothing to insert).
	for _, unexpected := range []string{
		"INSERT INTO otel_logs",
		"INSERT INTO otel_traces",
		"INSERT INTO otel_metrics_gauge",
		"INSERT INTO otel_metrics_sum",
		"INSERT INTO otel_metrics_histogram",
	} {
		if strings.Contains(joined, unexpected) {
			t.Fatalf("unexpected INSERT for empty request: %q", unexpected)
		}
	}
}

func TestStorePipelineLogIngestAutoAppliesTagRules(t *testing.T) {
	store := &captureStore{}
	store.queryFunc = func(query string, _ ...any) (extensionpoints.RowIterator, error) {
		switch {
		case strings.Contains(query, "FROM sobs_tag_rules"):
			return &captureRows{rows: [][]any{{
				"rule-1",
				"Error priority",
				"log",
				"severity",
				"eq",
				"ERROR",
				"",
				"priority",
				"high",
				"[]",
			}}}, nil
		default:
			return &captureRows{}, nil
		}
	}
	factory := &captureStoreFactory{store: store}
	pipeline := NewStorePipeline(factory).(*StorePipeline)

	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "svc-logs"}},
			}}},
			ScopeLogs: []*logsv1.ScopeLogs{{
				LogRecords: []*logsv1.LogRecord{{
					TimeUnixNano: 1713520800000000000,
					SeverityText: "ERROR",
					Body:         &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "boom"}},
				}},
			}},
		}},
	}

	if err := pipeline.ConsumeLogs(context.Background(), req); err != nil {
		t.Fatalf("consume logs: %v", err)
	}

	expectedRecordID := recordIDForLog("2024-04-19 10:00:00.000000", "svc-logs", "", "")
	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"INSERT INTO sobs_record_tags",
		`"RecordType":"log"`,
		`"TagKey":"priority"`,
		`"TagValue":"high"`,
		`"IsAuto":1`,
		`"RecordId":"` + expectedRecordID + `"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
}

func TestStorePipelineRUMIngestPersistsSessionsErrorsAndTags(t *testing.T) {
	resetRUMBrowserContextCache()
	store := &captureStore{}
	store.queryFunc = func(query string, _ ...any) (extensionpoints.RowIterator, error) {
		switch {
		case strings.Contains(query, "FROM sobs_tag_rules"):
			return &captureRows{rows: [][]any{
				{"rum-rule", "RUM type", "rum", "event_type", "eq", "pageview", "", "channel", "web", "[]"},
				{"error-rule", "Error type", "error", "event_type", "eq", "exception", "", "priority", "high", "[]"},
			}}, nil
		default:
			return &captureRows{}, nil
		}
	}
	factory := &captureStoreFactory{store: store}
	pipeline := NewStorePipeline(factory).(*StorePipeline)
	req := &RUMIngestRequest{
		ClientIP: "203.0.113.5",
		Events: []map[string]any{
			{
				"type":       "pageview",
				"timestamp":  "2024-01-01T00:00:00Z",
				"sessionId":  "sess-rum-1",
				"service":    "browser-frontend",
				"url":        "https://example.com/",
				"traceparent": "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
				"contextHash": "hash-1",
				"browserContext": map[string]any{
					"timezone":    "America/Los_Angeles",
					"browserName": "firefox",
				},
			},
			{
				"type":             "error",
				"timestamp":        "2024-01-01T00:00:01Z",
				"sessionId":        "sess-rum-1",
				"url":              "https://example.com/app",
				"message":          "Cannot read properties of null",
				"errorType":        "TypeError",
				"errorSource":      "window.onerror",
				"stack":            "TypeError: Cannot read...",
				"contextHash":      "hash-1",
				"contextUnchanged": true,
				"page": map[string]any{
					"title":    "Orders",
					"viewport": "1440x900",
				},
				"artifact": map[string]any{
					"type": "screenshot",
					"id":   "shot-001",
					"url":  "https://example.com/artifacts/shot-001.png",
				},
				"replay": map[string]any{
					"id":  "replay-001",
					"url": "https://example.com/replays/replay-001",
				},
				"clientAuthToken": "secret-token",
			},
		},
	}

	if err := pipeline.ConsumeRUM(context.Background(), req); err != nil {
		t.Fatalf("consume rum: %v", err)
	}

	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS hyperdx_sessions",
		"INSERT INTO hyperdx_sessions",
		`"TraceId":"4bf92f3577b34da6a3ce929d0e0e4736"`,
		`"SpanId":"00f067aa0ba902b7"`,
		`"TraceFlags":1`,
		`"browser.context.timezone":"America/Los_Angeles"`,
		`"browser.context.browserName":"firefox"`,
		`"client.ip":"203.0.113.5"`,
		`"RecordType":"rum"`,
		`"TagKey":"channel"`,
		`"TagValue":"web"`,
		"INSERT INTO otel_logs",
		`"EventName":"exception"`,
		`"error.source":"window.onerror"`,
		`"browser.page.title":"Orders"`,
		`"artifact.type":"screenshot"`,
		`"replay.id":"replay-001"`,
		`"RecordType":"error"`,
		`"TagKey":"priority"`,
		`"TagValue":"high"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
	if strings.Contains(joined, "clientAuthToken") {
		t.Fatalf("expected client auth token to be stripped from stored payloads, got:\n%s", joined)
	}
}

func TestStorePipelineAIIngestPersistsTraceAndTags(t *testing.T) {
	store := &captureStore{}
	store.queryFunc = func(query string, _ ...any) (extensionpoints.RowIterator, error) {
		switch {
		case strings.Contains(query, "FROM sobs_tag_rules"):
			return &captureRows{rows: [][]any{{
				"ai-rule", "AI model", "ai", "attribute", "eq", "gpt-5", "gen_ai.request.model", "provider", "openai", "[]",
			}}}, nil
		default:
			return &captureRows{}, nil
		}
	}
	factory := &captureStoreFactory{store: store}
	pipeline := NewStorePipeline(factory).(*StorePipeline)
	req := &AIIngestRequest{
		Timestamp:  "2024-01-01T00:00:00Z",
		Model:      "gpt-5",
		Operation:  "chat",
		DurationMS: 12.34,
		Provider:   "openai",
		Service:    "svc-ai",
		TraceID:    "trace-ai-1",
		SpanID:     "span-ai-1",
		SpanName:   "chat gpt-5",
		TokensIn:   12,
		TokensOut:  34,
		SpanAttributes: map[string]string{
			"gen_ai.operation.name":      "chat",
			"gen_ai.provider.name":       "openai",
			"gen_ai.request.model":       "gpt-5",
			"gen_ai.usage.input_tokens":  "12",
			"gen_ai.usage.output_tokens": "34",
			"sobs.gen_ai.prompt":         "hello",
		},
	}

	if err := pipeline.ConsumeAI(context.Background(), req); err != nil {
		t.Fatalf("consume ai: %v", err)
	}

	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"INSERT INTO otel_traces",
		`"SpanName":"chat gpt-5"`,
		`"SpanKind":"CLIENT"`,
		`"ScopeName":"sobs-ai"`,
		`"gen_ai.provider.name":"openai"`,
		`"gen_ai.request.model":"gpt-5"`,
		`"gen_ai.usage.input_tokens":"12"`,
		`"Duration":12340000`,
		`"RecordType":"ai"`,
		`"TagKey":"provider"`,
		`"TagValue":"openai"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
}

func TestStorePipelineErrorsV1PersistsLogsAndTags(t *testing.T) {
	store := &captureStore{}
	store.queryFunc = func(query string, _ ...any) (extensionpoints.RowIterator, error) {
		switch {
		case strings.Contains(query, "FROM sobs_tag_rules"):
			return &captureRows{rows: [][]any{{
				"error-rule", "Error release", "error", "attribute", "eq", "2024.01.01", "release", "priority", "high", "[]",
			}}}, nil
		default:
			return &captureRows{}, nil
		}
	}
	factory := &captureStoreFactory{store: store}
	pipeline := NewStorePipeline(factory).(*StorePipeline)
	req := &ErrorIngestRequest{
		Timestamp:      "2024-01-01T00:00:00Z",
		TraceID:        "trace-err-1",
		SpanID:         "span-err-1",
		TraceFlags:     1,
		Service:        "svc-errors",
		Message:        "boom",
		ExceptionType:  "ReferenceError",
		ExceptionStack: "at fn (bundle.js:1:1)",
		Attributes: map[string]string{
			"release":              "2024.01.01",
			"handled":              "false",
			"exception.type":       "ReferenceError",
			"exception.message":    "boom",
			"exception.stacktrace": "at fn (bundle.js:1:1)",
		},
	}

	if err := pipeline.ConsumeErrorsV1(context.Background(), req); err != nil {
		t.Fatalf("consume errors v1: %v", err)
	}

	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"INSERT INTO otel_logs",
		`"SeverityText":"ERROR"`,
		`"ServiceName":"svc-errors"`,
		`"Body":"boom"`,
		`"EventName":"exception"`,
		`"exception.type":"ReferenceError"`,
		`"exception.message":"boom"`,
		`"exception.stacktrace":"at fn (bundle.js:1:1)"`,
		`"RecordType":"log"`,
		`"AttrKey":"release"`,
		`"RecordType":"error"`,
		`"TagKey":"priority"`,
		`"TagValue":"high"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
	if strings.Contains(joined, "INSERT INTO sobs_ingest_opaque") {
		t.Fatalf("expected /v1/errors to bypass opaque staging, got:\n%s", joined)
	}
}

func TestStorePipelineTraceIngestPersistsDerivedErrorsAndAutoTags(t *testing.T) {
	store := &captureStore{}
	store.queryFunc = func(query string, _ ...any) (extensionpoints.RowIterator, error) {
		switch {
		case strings.Contains(query, "FROM sobs_tag_rules"):
			return &captureRows{rows: [][]any{
				{"trace-rule", "Trace flow", "trace", "span_name", "eq", "root span", "", "flow", "ingress", "[]"},
				{"error-rule", "Error priority", "error", "event_type", "eq", "exception", "", "priority", "high", "[]"},
			}}, nil
		default:
			return &captureRows{}, nil
		}
	}
	factory := &captureStoreFactory{store: store}
	pipeline := NewStorePipeline(factory).(*StorePipeline)

	req := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracev1.ResourceSpans{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{{
				Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "svc-trace"}},
			}}},
			ScopeSpans: []*tracev1.ScopeSpans{{
				Scope: &commonv1.InstrumentationScope{Attributes: []*commonv1.KeyValue{{
					Key: "scope.attr", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "scope-value"}},
				}}},
				Spans: []*tracev1.Span{{
					TraceId:           []byte{0x01, 0x02, 0x03},
					SpanId:            []byte{0x0a, 0x0b},
					Name:              "root span",
					StartTimeUnixNano: 1713520800000000000,
					EndTimeUnixNano:   1713520801500000000,
					Status:            &tracev1.Status{Code: tracev1.Status_STATUS_CODE_ERROR},
					Attributes: []*commonv1.KeyValue{
						{Key: "exception.type", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "ValueError"}}},
						{Key: "exception.message", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "boom"}}},
						{Key: "span.kind", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "SERVER"}}},
					},
				}},
			}},
		}},
	}

	if err := pipeline.ConsumeTraces(context.Background(), req); err != nil {
		t.Fatalf("consume traces: %v", err)
	}

	joined := strings.Join(store.execs, "\n")
	traceID := "010203"
	spanID := "0a0b"
	for _, expected := range []string{
		"INSERT INTO otel_traces",
		`"SpanAttributes":{"exception.message":"boom","exception.type":"ValueError","scope.attr":"scope-value","service.name":"svc-trace","span.kind":"SERVER"}`,
		`"SpanKind":"SERVER"`,
		`"StatusCode":"STATUS_CODE_ERROR"`,
		"INSERT INTO otel_logs",
		`"EventName":"exception"`,
		`"SeverityText":"ERROR"`,
		`"Body":"boom"`,
		`"RecordType":"trace"`,
		`"TagKey":"flow"`,
		`"TagValue":"ingress"`,
		`"RecordId":"` + recordIDForSpan(traceID, spanID) + `"`,
		`"RecordType":"error"`,
		`"TagKey":"priority"`,
		`"TagValue":"high"`,
		`"RecordId":"` + recordIDForLog("2024-04-19 10:00:00.000000", "svc-trace", traceID, spanID) + `"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
}

func TestStorePipelineLogIngestPersistsDiscoveredAttrKeys(t *testing.T) {
	store := &captureStore{}
	factory := &captureStoreFactory{store: store}
	pipeline := NewStorePipeline(factory).(*StorePipeline)

	req := &collogspb.ExportLogsServiceRequest{
		ResourceLogs: []*logsv1.ResourceLogs{{
			Resource: &resourcev1.Resource{Attributes: []*commonv1.KeyValue{
				{Key: "service.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "svc-logs"}}},
				{Key: "deployment.environment", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "prod"}}},
			}},
			ScopeLogs: []*logsv1.ScopeLogs{{
				Scope: &commonv1.InstrumentationScope{Attributes: []*commonv1.KeyValue{
					{Key: "scope.attr", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "scope-value"}}},
				}},
				LogRecords: []*logsv1.LogRecord{{
					TimeUnixNano: 1713520800000000000,
					SeverityText: "ERROR",
					Body:         &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "boom"}},
					Attributes: []*commonv1.KeyValue{
						{Key: "event.name", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "exception"}}},
						{Key: "http.method", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: "GET"}}},
					},
				}},
			}},
		}},
	}

	if err := pipeline.ConsumeLogs(context.Background(), req); err != nil {
		t.Fatalf("consume logs: %v", err)
	}

	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS sobs_log_attr_keys",
		"INSERT INTO otel_logs",
		"INSERT INTO sobs_log_attr_keys",
		`"AttrKey":"event.name"`,
		`"AttrKey":"http.method"`,
		`"AttrKey":"deployment.environment"`,
		`"AttrKey":"service.name"`,
		`"AttrKey":"scope.attr"`,
		`"RecordType":"log"`,
		`"RecordType":"resource"`,
		`"RecordType":"scope"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
}
