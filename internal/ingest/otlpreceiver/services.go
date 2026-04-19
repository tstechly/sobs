package otlpreceiver

import (
	"context"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type TraceService struct {
	coltracepb.UnimplementedTraceServiceServer
	receiver *Receiver
}

type MetricsService struct {
	colmetricpb.UnimplementedMetricsServiceServer
	receiver *Receiver
}

type LogsService struct {
	collogspb.UnimplementedLogsServiceServer
	receiver *Receiver
}

func NewTraceService(receiver *Receiver) *TraceService {
	return &TraceService{receiver: receiver}
}

func NewMetricsService(receiver *Receiver) *MetricsService {
	return &MetricsService{receiver: receiver}
}

func NewLogsService(receiver *Receiver) *LogsService {
	return &LogsService{receiver: receiver}
}

func (s *TraceService) Export(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	if err := s.receiver.pipeline.ConsumeTraces(ctx, req); err != nil {
		return nil, err
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}

func (s *MetricsService) Export(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) (*colmetricpb.ExportMetricsServiceResponse, error) {
	if err := s.receiver.pipeline.ConsumeMetrics(ctx, req); err != nil {
		return nil, err
	}
	return &colmetricpb.ExportMetricsServiceResponse{}, nil
}

func (s *LogsService) Export(ctx context.Context, req *collogspb.ExportLogsServiceRequest) (*collogspb.ExportLogsServiceResponse, error) {
	if err := s.receiver.pipeline.ConsumeLogs(ctx, req); err != nil {
		return nil, err
	}
	return &collogspb.ExportLogsServiceResponse{}, nil
}
