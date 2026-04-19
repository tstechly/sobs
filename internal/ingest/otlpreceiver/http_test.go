package otlpreceiver

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/proto"
)

type capturePipeline struct {
	traces  int
	metrics int
	logs    int
}

func (p *capturePipeline) ConsumeTraces(_ context.Context, _ *coltracepb.ExportTraceServiceRequest) error {
	p.traces++
	return nil
}

func (p *capturePipeline) ConsumeMetrics(_ context.Context, _ *colmetricpb.ExportMetricsServiceRequest) error {
	p.metrics++
	return nil
}

func (p *capturePipeline) ConsumeLogs(_ context.Context, _ *collogspb.ExportLogsServiceRequest) error {
	p.logs++
	return nil
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

func TestAdditionalIngestEndpoints(t *testing.T) {
	srv := NewHTTPServer()
	mux := http.NewServeMux()
	srv.Register(mux)

	paths := []string{"/v1/errors", "/v1/rum", "/v1/ai"}
	for _, p := range paths {
		req := httptest.NewRequest("POST", "http://example.com"+p, bytes.NewReader([]byte(`{"ok":true}`)))
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, w.Code)
		}
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
