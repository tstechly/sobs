package otlpreceiver

import (
	"context"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/extensionpoints"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
)

type captureResult struct{}

func (captureResult) RowsAffected() (int64, error) { return 1, nil }

type captureRows struct{}

func (captureRows) Next() bool                 { return false }
func (captureRows) Scan(_ ...any) error        { return nil }
func (captureRows) Err() error                 { return nil }
func (captureRows) Close() error               { return nil }

type captureStore struct {
	execs []string
}

func (s *captureStore) Ping(_ context.Context) error { return nil }
func (s *captureStore) Query(_ context.Context, _ string, _ ...any) (extensionpoints.RowIterator, error) {
	return captureRows{}, nil
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
	if err := pipeline.ConsumeOpaqueJSON(context.Background(), "/v1/errors", map[string]any{"ok": true}); err != nil {
		t.Fatalf("consume opaque: %v", err)
	}

	joined := strings.Join(store.execs, "\n")
	for _, expected := range []string{
		"CREATE TABLE IF NOT EXISTS sobs_ingest_traces",
		"CREATE TABLE IF NOT EXISTS sobs_ingest_metrics",
		"CREATE TABLE IF NOT EXISTS sobs_ingest_logs",
		"CREATE TABLE IF NOT EXISTS sobs_ingest_opaque",
		"INSERT INTO sobs_ingest_traces",
		"INSERT INTO sobs_ingest_metrics",
		"INSERT INTO sobs_ingest_logs",
		"INSERT INTO sobs_ingest_opaque",
		"INSERT INTO sobs_ingest_events",
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("expected query containing %q, got:\n%s", expected, joined)
		}
	}
}
