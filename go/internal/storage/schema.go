package storage

// Schema contains the DDL for P0 tables required by the Go API.
// Copied from the Python app.py schema definitions.
const Schema = `
CREATE TABLE IF NOT EXISTS otel_logs (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimestampTime DateTime DEFAULT toDateTime(Timestamp) CODEC(Delta(4), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    TraceFlags UInt8 CODEC(T64, ZSTD(1)),
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    SeverityNumber UInt8 CODEC(T64, ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Body String CODEC(ZSTD(1)),
    ResourceSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion LowCardinality(String) CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    LogAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    EventName String CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(TimestampTime)
ORDER BY (ServiceName, TimestampTime, Timestamp)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS otel_traces (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    ParentSpanId String CODEC(ZSTD(1)),
    TraceState String CODEC(ZSTD(1)),
    SpanName LowCardinality(String) CODEC(ZSTD(1)),
    SpanKind LowCardinality(String) CODEC(ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion String CODEC(ZSTD(1)),
    SpanAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Duration UInt64 CODEC(T64, ZSTD(1)),
    StatusCode LowCardinality(String) CODEC(ZSTD(1)),
    StatusMessage String CODEC(ZSTD(1)),
    Events Nested (
        Timestamp DateTime64(9),
        Name LowCardinality(String),
        Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1)),
    Links Nested (
        TraceId String,
        SpanId String,
        TraceState String,
        Attributes Map(LowCardinality(String), String)
    ) CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(Timestamp)
ORDER BY (ServiceName, SpanName, toDateTime(Timestamp))
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS hyperdx_sessions (
    Timestamp DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimestampTime DateTime DEFAULT toDateTime(Timestamp) CODEC(Delta(4), ZSTD(1)),
    TraceId String CODEC(ZSTD(1)),
    SpanId String CODEC(ZSTD(1)),
    TraceFlags UInt8 CODEC(T64, ZSTD(1)),
    SeverityText LowCardinality(String) CODEC(ZSTD(1)),
    SeverityNumber UInt8 CODEC(T64, ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    Body String CODEC(ZSTD(1)),
    ResourceSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ResourceAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    ScopeSchemaUrl LowCardinality(String) CODEC(ZSTD(1)),
    ScopeName String CODEC(ZSTD(1)),
    ScopeVersion LowCardinality(String) CODEC(ZSTD(1)),
    ScopeAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    LogAttributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    EventName String CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(TimestampTime)
ORDER BY (ServiceName, TimestampTime, Timestamp)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS otel_metrics_gauge (
    TimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimeUnixMs DateTime DEFAULT toDateTime(TimeUnix) CODEC(Delta(4), ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    MetricDescription String CODEC(ZSTD(1)),
    MetricUnit LowCardinality(String) CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Value Float64 CODEC(ZSTD(1)),
    Flags UInt32 DEFAULT 0 CODEC(T64, ZSTD(1)),
    AttrFingerprint String CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(TimeUnixMs)
ORDER BY (ServiceName, MetricName, AttrFingerprint, TimeUnixMs, TimeUnix)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS otel_metrics_sum (
    TimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimeUnixMs DateTime DEFAULT toDateTime(TimeUnix) CODEC(Delta(4), ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    MetricDescription String CODEC(ZSTD(1)),
    MetricUnit LowCardinality(String) CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Value Float64 CODEC(ZSTD(1)),
    Flags UInt32 DEFAULT 0 CODEC(T64, ZSTD(1)),
    IsMonotonic UInt8 DEFAULT 0 CODEC(T64, ZSTD(1)),
    AggregationTemporality Int32 DEFAULT 0 CODEC(T64, ZSTD(1)),
    AttrFingerprint String CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(TimeUnixMs)
ORDER BY (ServiceName, MetricName, AttrFingerprint, TimeUnixMs, TimeUnix)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;

CREATE TABLE IF NOT EXISTS otel_metrics_histogram (
    TimeUnix DateTime64(9) CODEC(Delta(8), ZSTD(1)),
    TimeUnixMs DateTime DEFAULT toDateTime(TimeUnix) CODEC(Delta(4), ZSTD(1)),
    ServiceName LowCardinality(String) CODEC(ZSTD(1)),
    MetricName LowCardinality(String) CODEC(ZSTD(1)),
    MetricDescription String CODEC(ZSTD(1)),
    MetricUnit LowCardinality(String) CODEC(ZSTD(1)),
    Attributes Map(LowCardinality(String), String) CODEC(ZSTD(1)),
    Count UInt64 DEFAULT 0 CODEC(T64, ZSTD(1)),
    Sum Float64 CODEC(ZSTD(1)),
    BucketCounts Array(UInt64) CODEC(ZSTD(1)),
    ExplicitBounds Array(Float64) CODEC(ZSTD(1)),
    Flags UInt32 DEFAULT 0 CODEC(T64, ZSTD(1)),
    AggregationTemporality Int32 DEFAULT 0 CODEC(T64, ZSTD(1)),
    AttrFingerprint String CODEC(ZSTD(1))
) ENGINE = MergeTree()
PARTITION BY toDate(TimeUnixMs)
ORDER BY (ServiceName, MetricName, AttrFingerprint, TimeUnixMs, TimeUnix)
SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1;
`
