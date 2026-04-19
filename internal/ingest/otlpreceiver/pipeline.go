package otlpreceiver

import (
	"context"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type NoopPipeline struct{}

func NewNoopPipeline() Pipeline {
	return &NoopPipeline{}
}

func (p *NoopPipeline) ConsumeTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	_ = ctx
	_ = req
	return nil
}

func (p *NoopPipeline) ConsumeMetrics(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) error {
	_ = ctx
	_ = req
	return nil
}

func (p *NoopPipeline) ConsumeLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error {
	_ = ctx
	_ = req
	return nil
}
