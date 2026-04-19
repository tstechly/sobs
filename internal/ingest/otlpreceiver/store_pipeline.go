package otlpreceiver

import (
	"context"
	"sync"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

type StorePipeline struct {
	factory    extensionpoints.StoreFactory
	schemaOnce sync.Once
	schemaErr  error
}

func NewStorePipeline(factory extensionpoints.StoreFactory) Pipeline {
	return &StorePipeline{factory: factory}
}

func (p *StorePipeline) ensureSchema(ctx context.Context) error {
	p.schemaOnce.Do(func() {
		store, err := persist.Open(ctx, p.factory)
		if err != nil {
			p.schemaErr = err
			return
		}
		defer func() { _ = store.Close() }()
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_ingest_events (Id String, Kind String, PayloadJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (Kind, UpdatedAt, Id)")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_ingest_traces (Id String, PayloadJson String, ResourceSpanCount UInt32 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (UpdatedAt, Id)")
		}
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_ingest_metrics (Id String, PayloadJson String, ResourceMetricCount UInt32 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (UpdatedAt, Id)")
		}
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_ingest_logs (Id String, PayloadJson String, ResourceLogCount UInt32 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (UpdatedAt, Id)")
		}
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_ingest_opaque (Id String, Path String, PayloadJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (Path, UpdatedAt, Id)")
		}
		p.schemaErr = err
	})
	return p.schemaErr
}

func (p *StorePipeline) record(ctx context.Context, kind string, payload string) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_ingest_events (Id, Kind, PayloadJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", persist.NewID(), kind, payload, 0, persist.Version())
	return err
}

func (p *StorePipeline) recordTrace(ctx context.Context, payload string, resourceSpans int) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_ingest_traces (Id, PayloadJson, ResourceSpanCount, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", persist.NewID(), payload, resourceSpans, 0, persist.Version())
	return err
}

func (p *StorePipeline) recordMetric(ctx context.Context, payload string, resourceMetrics int) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_ingest_metrics (Id, PayloadJson, ResourceMetricCount, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", persist.NewID(), payload, resourceMetrics, 0, persist.Version())
	return err
}

func (p *StorePipeline) recordLog(ctx context.Context, payload string, resourceLogs int) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_ingest_logs (Id, PayloadJson, ResourceLogCount, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", persist.NewID(), payload, resourceLogs, 0, persist.Version())
	return err
}

func (p *StorePipeline) recordOpaque(ctx context.Context, path string, payload string) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_ingest_opaque (Id, Path, PayloadJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", persist.NewID(), path, payload, 0, persist.Version())
	return err
}

func (p *StorePipeline) ConsumeTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	payload, err := protojson.Marshal(req)
	if err != nil {
		return err
	}
	if err := p.recordTrace(ctx, string(payload), len(req.ResourceSpans)); err != nil {
		return err
	}
	return p.record(ctx, "traces", string(payload))
}

func (p *StorePipeline) ConsumeMetrics(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) error {
	payload, err := protojson.Marshal(req)
	if err != nil {
		return err
	}
	if err := p.recordMetric(ctx, string(payload), len(req.ResourceMetrics)); err != nil {
		return err
	}
	return p.record(ctx, "metrics", string(payload))
}

func (p *StorePipeline) ConsumeLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error {
	payload, err := protojson.Marshal(req)
	if err != nil {
		return err
	}
	if err := p.recordLog(ctx, string(payload), len(req.ResourceLogs)); err != nil {
		return err
	}
	return p.record(ctx, "logs", string(payload))
}

func (p *StorePipeline) ConsumeOpaqueJSON(ctx context.Context, path string, payload map[string]any) error {
	raw := persist.JSONString(payload)
	if err := p.recordOpaque(ctx, path, raw); err != nil {
		return err
	}
	return p.record(ctx, path, raw)
}
