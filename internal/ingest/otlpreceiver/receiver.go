package otlpreceiver

import (
	"context"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type Pipeline interface {
	ConsumeTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error
	ConsumeMetrics(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) error
	ConsumeLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error
}

type Receiver struct {
	pipeline Pipeline
}

func NewReceiver(pipeline Pipeline) *Receiver {
	return &Receiver{pipeline: pipeline}
}
