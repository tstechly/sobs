package otlpreceiver

import (
	"context"
	"errors"
	"testing"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type stubPipeline struct {
	tracesErr  error
	metricsErr error
	logsErr    error
}

func (p *stubPipeline) ConsumeTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	_ = ctx
	_ = req
	return p.tracesErr
}

func (p *stubPipeline) ConsumeMetrics(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) error {
	_ = ctx
	_ = req
	return p.metricsErr
}

func (p *stubPipeline) ConsumeLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error {
	_ = ctx
	_ = req
	return p.logsErr
}

func TestTraceExport(t *testing.T) {
	receiver := NewReceiver(&stubPipeline{})
	svc := NewTraceService(receiver)
	resp, err := svc.Export(context.Background(), &coltracepb.ExportTraceServiceRequest{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestTraceExportError(t *testing.T) {
	receiver := NewReceiver(&stubPipeline{tracesErr: errors.New("boom")})
	svc := NewTraceService(receiver)
	resp, err := svc.Export(context.Background(), &coltracepb.ExportTraceServiceRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if resp != nil {
		t.Fatal("expected nil response on error")
	}
}

func TestMetricsExport(t *testing.T) {
	receiver := NewReceiver(&stubPipeline{})
	svc := NewMetricsService(receiver)
	resp, err := svc.Export(context.Background(), &colmetricpb.ExportMetricsServiceRequest{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestLogsExport(t *testing.T) {
	receiver := NewReceiver(&stubPipeline{})
	svc := NewLogsService(receiver)
	resp, err := svc.Export(context.Background(), &collogspb.ExportLogsServiceRequest{})
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
}

func TestAsyncLogPipelineSurfacesWorkerErrorsWhenRequested(t *testing.T) {
	t.Setenv("SOBS_INGEST_WAIT_FOR_RESULT", "1")
	pipeline := NewAsyncLogPipeline(&stubPipeline{tracesErr: errors.New("boom")})
	err := pipeline.ConsumeTraces(context.Background(), &coltracepb.ExportTraceServiceRequest{})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected boom error, got %v", err)
	}
}
