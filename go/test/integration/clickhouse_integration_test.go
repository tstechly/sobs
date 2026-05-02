package integration_test

import (
	"testing"

	"github.com/chdb-io/chdb-go/chdb"
	"github.com/stretchr/testify/require"
)

func TestClickHouseIntegration(t *testing.T) {

	s, err := chdb.NewSession("")
	require.NoError(t, err)
	defer s.Close()

	_, err = s.Query(`
CREATE TABLE IF NOT EXISTS otel_logs (
    Timestamp DateTime64(9),
    TimestampTime DateTime DEFAULT toDateTime(Timestamp),
    TraceId String,
    SpanId String,
    TraceFlags UInt8,
    SeverityText LowCardinality(String),
    SeverityNumber UInt8,
    ServiceName LowCardinality(String),
    Body String,
    ResourceSchemaUrl LowCardinality(String),
    ResourceAttributes Map(LowCardinality(String), String),
    ScopeSchemaUrl LowCardinality(String),
    ScopeName String,
    ScopeVersion LowCardinality(String),
    ScopeAttributes Map(LowCardinality(String), String),
    LogAttributes Map(LowCardinality(String), String),
    EventName String
) ENGINE = MergeTree()
PARTITION BY toDate(TimestampTime)
ORDER BY (ServiceName, TimestampTime, Timestamp)
`)
	require.NoError(t, err)

	_, err = s.Query(`INSERT INTO otel_logs (Timestamp, TimestampTime, TraceId, SpanId, TraceFlags, SeverityText, SeverityNumber, ServiceName, Body, ResourceSchemaUrl, ResourceAttributes, ScopeSchemaUrl, ScopeName, ScopeVersion, ScopeAttributes, LogAttributes, EventName) VALUES (
	now64(9),
	now(),
	'', '', 0, 'INFO', 9, 'integration-test', 'embedded chDB smoke test',
	'', CAST(map(), 'Map(LowCardinality(String), String)'), '', '', '', CAST(map(), 'Map(LowCardinality(String), String)'), CAST(map(), 'Map(LowCardinality(String), String)'), ''
)`)
	require.NoError(t, err)

	res, err := s.Query("SELECT count() FROM otel_logs WHERE ServiceName = 'integration-test'")
	require.NoError(t, err)
	t.Logf("query result: %s", res)
	_, err = s.Query("SELECT version()")
	require.NoError(t, err)
}
