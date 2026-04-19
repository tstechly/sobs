package otlpreceiver

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 used for non-cryptographic fingerprinting only
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracev1 "go.opentelemetry.io/proto/otlp/trace/v1"
)

// fingerprintSkipPrefixes matches Python's _FINGERPRINT_SKIP_PREFIXES.
var fingerprintSkipPrefixes = []string{"telemetry.", "process.", "os.", "runtime."}

// severityNumbers matches Python's _severity_number mapping.
var severityNumbers = map[string]int{
	"TRACE": 1, "DEBUG": 5, "INFO": 9, "WARN": 13, "WARNING": 13,
	"ERROR": 17, "CRITICAL": 21, "FATAL": 21, "METRIC": 9,
}

type StorePipeline struct {
	factory    extensionpoints.StoreFactory
	schemaOnce sync.Once
	schemaErr  error
}

func NewStorePipeline(factory extensionpoints.StoreFactory) Pipeline {
	return &StorePipeline{factory: factory}
}

// ensureSchema creates all otel_* tables matching the Python schema exactly.
func (p *StorePipeline) ensureSchema(ctx context.Context) error {
	p.schemaOnce.Do(func() {
		store, err := persist.Open(ctx, p.factory)
		if err != nil {
			p.schemaErr = err
			return
		}
		defer func() { _ = store.Close() }()

		ddls := []string{
			`CREATE TABLE IF NOT EXISTS otel_logs (
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
			SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1`,

			`CREATE TABLE IF NOT EXISTS otel_traces (
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
			SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1`,

			`CREATE TABLE IF NOT EXISTS otel_metrics_gauge (
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
			SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1`,

			`CREATE TABLE IF NOT EXISTS otel_metrics_sum (
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
			SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1`,

			`CREATE TABLE IF NOT EXISTS otel_metrics_histogram (
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
			SETTINGS index_granularity = 8192, ttl_only_drop_parts = 1`,

			// Staging table for opaque JSON payloads (/v1/rum, /v1/ai, /v1/errors).
			`CREATE TABLE IF NOT EXISTS sobs_ingest_opaque (
				Id String,
				Path String,
				PayloadJson String,
				IsDeleted UInt8 DEFAULT 0,
				Version UInt64 DEFAULT 0,
				UpdatedAt DateTime64(3) DEFAULT now64(3)
			) ENGINE = ReplacingMergeTree(Version) ORDER BY (Path, UpdatedAt, Id)`,
		}

		for _, ddl := range ddls {
			if _, err = store.Exec(ctx, ddl); err != nil {
				p.schemaErr = err
				return
			}
		}
	})
	return p.schemaErr
}

// insertJSONEachRow serialises rows as JSONEachRow and executes a bulk INSERT.
func (p *StorePipeline) insertJSONEachRow(ctx context.Context, table string, rows []map[string]any) error {
	if len(rows) == 0 {
		return nil
	}
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		b, err := json.Marshal(row)
		if err != nil {
			return fmt.Errorf("marshal row: %w", err)
		}
		lines = append(lines, string(b))
	}
	_, err = store.Exec(ctx, "INSERT INTO "+table+" FORMAT JSONEachRow\n"+strings.Join(lines, "\n"))
	return err
}

// ── helpers matching Python's helper functions exactly ────────────────────────

// nsToISO converts a nanosecond Unix timestamp to a ClickHouse-compatible
// DateTime64 string matching Python's _normalize_ch_timestamp output.
func nsToISO(nanos uint64) string {
	t := time.Unix(0, int64(nanos)).UTC()
	return t.Format("2006-01-02 15:04:05.000000")
}

// nowISO returns the current UTC time in ClickHouse DateTime64 format.
func nowISO() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05.000000")
}

// anyValueToString converts an OTel AnyValue to its string representation.
// Complex types (array, kvlist) are JSON-encoded, matching _stringify_attrs.
func anyValueToString(val *commonv1.AnyValue) string {
	if val == nil {
		return ""
	}
	switch v := val.GetValue().(type) {
	case *commonv1.AnyValue_StringValue:
		return v.StringValue
	case *commonv1.AnyValue_IntValue:
		return fmt.Sprintf("%d", v.IntValue)
	case *commonv1.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", v.DoubleValue)
	case *commonv1.AnyValue_BoolValue:
		if v.BoolValue {
			return "true"
		}
		return "false"
	case *commonv1.AnyValue_BytesValue:
		return base64.StdEncoding.EncodeToString(v.BytesValue)
	case *commonv1.AnyValue_ArrayValue:
		parts := make([]any, 0)
		if v.ArrayValue != nil {
			for _, item := range v.ArrayValue.Values {
				parts = append(parts, anyValueToString(item))
			}
		}
		b, _ := json.Marshal(parts)
		return string(b)
	case *commonv1.AnyValue_KvlistValue:
		m := kvListToStringMap(v.KvlistValue.GetValues())
		b, _ := json.Marshal(m)
		return string(b)
	}
	return ""
}

// kvListToStringMap converts a proto KeyValue list to map[string]string,
// matching Python's _proto_kvlist_to_dict + _stringify_attrs.
func kvListToStringMap(attrs []*commonv1.KeyValue) map[string]string {
	out := make(map[string]string, len(attrs))
	for _, kv := range attrs {
		out[kv.GetKey()] = anyValueToString(kv.GetValue())
	}
	return out
}

// severityNumber maps a severity text to its OTel severity number,
// matching Python's _severity_number.
func severityNumber(level string) int {
	if n, ok := severityNumbers[strings.ToUpper(level)]; ok {
		return n
	}
	return 9 // default INFO
}

// traceStatusCode converts a proto StatusCode to the string representation
// stored in otel_traces.StatusCode, matching Python's _trace_status_code.
func traceStatusCode(code tracev1.Status_StatusCode) string {
	switch code {
	case tracev1.Status_STATUS_CODE_OK:
		return "STATUS_CODE_OK"
	case tracev1.Status_STATUS_CODE_ERROR:
		return "STATUS_CODE_ERROR"
	default:
		return "STATUS_CODE_UNSET"
	}
}

// attrFingerprint produces a stable 16-char hex fingerprint of data-point
// attributes, matching Python's _attr_fingerprint exactly.
func attrFingerprint(attrs map[string]string) string {
	pairs := make([]string, 0, len(attrs))
	for k, v := range attrs {
		skip := false
		for _, prefix := range fingerprintSkipPrefixes {
			if strings.HasPrefix(k, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			pairs = append(pairs, k+"="+v)
		}
	}
	sort.Strings(pairs)
	if len(pairs) > 8 {
		pairs = pairs[:8]
	}
	sum := md5.Sum([]byte(strings.Join(pairs, "|"))) //nolint:gosec
	return hex.EncodeToString(sum[:])[:16]
}

// ── Pipeline implementation ───────────────────────────────────────────────────

func (p *StorePipeline) ConsumeTraces(ctx context.Context, req *coltracepb.ExportTraceServiceRequest) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	rows := make([]map[string]any, 0)
	for _, rs := range req.GetResourceSpans() {
		resourceAttrs := kvListToStringMap(rs.GetResource().GetAttributes())
		service := resourceAttrs["service.name"]
		for _, ss := range rs.GetScopeSpans() {
			scope := ss.GetScope()
			for _, span := range ss.GetSpans() {
				startNs := span.GetStartTimeUnixNano()
				endNs := span.GetEndTimeUnixNano()
				var durationNs uint64
				if endNs > startNs {
					durationNs = endNs - startNs
				}
				spanAttrs := kvListToStringMap(span.GetAttributes())
				spanKind := "INTERNAL"
				if k := span.GetKind(); k != tracev1.Span_SPAN_KIND_UNSPECIFIED {
					spanKind = strings.TrimPrefix(k.String(), "SPAN_KIND_")
				}
				statusCode := traceStatusCode(span.GetStatus().GetCode())
				rows = append(rows, map[string]any{
					"Timestamp":          nsToISO(startNs),
					"TraceId":            hex.EncodeToString(span.GetTraceId()),
					"SpanId":             hex.EncodeToString(span.GetSpanId()),
					"ParentSpanId":       hex.EncodeToString(span.GetParentSpanId()),
					"TraceState":         span.GetTraceState(),
					"SpanName":           span.GetName(),
					"SpanKind":           spanKind,
					"ServiceName":        service,
					"ResourceAttributes": resourceAttrs,
					"ScopeName":          scope.GetName(),
					"ScopeVersion":       scope.GetVersion(),
					"SpanAttributes":     spanAttrs,
					"Duration":           durationNs,
					"StatusCode":         statusCode,
					"StatusMessage":      span.GetStatus().GetMessage(),
					"Events": map[string]any{
						"Timestamp":  []string{},
						"Name":       []string{},
						"Attributes": []map[string]string{},
					},
					"Links": map[string]any{
						"TraceId":    []string{},
						"SpanId":     []string{},
						"TraceState": []string{},
						"Attributes": []map[string]string{},
					},
				})
			}
		}
	}
	return p.insertJSONEachRow(ctx, "otel_traces", rows)
}

func (p *StorePipeline) ConsumeLogs(ctx context.Context, req *collogspb.ExportLogsServiceRequest) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	rows := make([]map[string]any, 0)
	for _, rl := range req.GetResourceLogs() {
		resourceAttrs := kvListToStringMap(rl.GetResource().GetAttributes())
		service := resourceAttrs["service.name"]
		for _, sl := range rl.GetScopeLogs() {
			scope := sl.GetScope()
			scopeAttrs := kvListToStringMap(scope.GetAttributes())
			for _, record := range sl.GetLogRecords() {
				recordAttrs := kvListToStringMap(record.GetAttributes())
				// merged attrs = resource + scope + record (record wins)
				merged := make(map[string]string, len(resourceAttrs)+len(scopeAttrs)+len(recordAttrs))
				for k, v := range resourceAttrs {
					merged[k] = v
				}
				for k, v := range scopeAttrs {
					merged[k] = v
				}
				for k, v := range recordAttrs {
					merged[k] = v
				}
				body := anyValueToString(record.GetBody())
				level := record.GetSeverityText()
				if level == "" {
					level = "INFO"
				}
				rows = append(rows, map[string]any{
					"Timestamp":          nsToISO(record.GetTimeUnixNano()),
					"TraceId":            hex.EncodeToString(record.GetTraceId()),
					"SpanId":             hex.EncodeToString(record.GetSpanId()),
					"TraceFlags":         0,
					"SeverityText":       strings.ToUpper(level),
					"SeverityNumber":     severityNumber(level),
					"ServiceName":        service,
					"Body":               body,
					"ResourceSchemaUrl":  "",
					"ResourceAttributes": resourceAttrs,
					"ScopeSchemaUrl":     "",
					"ScopeName":          scope.GetName(),
					"ScopeVersion":       scope.GetVersion(),
					"ScopeAttributes":    scopeAttrs,
					"LogAttributes":      merged,
					"EventName":          merged["event.name"],
				})
			}
		}
	}
	return p.insertJSONEachRow(ctx, "otel_logs", rows)
}

func (p *StorePipeline) ConsumeMetrics(ctx context.Context, req *colmetricpb.ExportMetricsServiceRequest) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	var gaugeRows, sumRows, histogramRows []map[string]any

	for _, rm := range req.GetResourceMetrics() {
		resourceAttrs := kvListToStringMap(rm.GetResource().GetAttributes())
		service := resourceAttrs["service.name"]
		if service == "" {
			service = "metrics"
		}
		for _, sm := range rm.GetScopeMetrics() {
			for _, metric := range sm.GetMetrics() {
				name := metric.GetName()
				desc := metric.GetDescription()
				unit := metric.GetUnit()

				switch d := metric.GetData().(type) {
				case *metricsv1.Metric_Gauge:
					for _, dp := range d.Gauge.GetDataPoints() {
						attrs := kvListToStringMap(dp.GetAttributes())
						value := dpValue(dp)
						gaugeRows = append(gaugeRows, map[string]any{
							"TimeUnix":          nsToISO(dp.GetTimeUnixNano()),
							"ServiceName":       service,
							"MetricName":        name,
							"MetricDescription": desc,
							"MetricUnit":        unit,
							"Attributes":        attrs,
							"Value":             value,
							"Flags":             dp.GetFlags(),
							"AttrFingerprint":   attrFingerprint(attrs),
						})
					}
				case *metricsv1.Metric_Sum:
					isMonotonic := uint8(0)
					if d.Sum.GetIsMonotonic() {
						isMonotonic = 1
					}
					aggTemp := int32(d.Sum.GetAggregationTemporality())
					for _, dp := range d.Sum.GetDataPoints() {
						attrs := kvListToStringMap(dp.GetAttributes())
						value := dpValue(dp)
						sumRows = append(sumRows, map[string]any{
							"TimeUnix":               nsToISO(dp.GetTimeUnixNano()),
							"ServiceName":            service,
							"MetricName":             name,
							"MetricDescription":      desc,
							"MetricUnit":             unit,
							"Attributes":             attrs,
							"Value":                  value,
							"Flags":                  dp.GetFlags(),
							"IsMonotonic":            isMonotonic,
							"AggregationTemporality": aggTemp,
							"AttrFingerprint":        attrFingerprint(attrs),
						})
					}
				case *metricsv1.Metric_Histogram:
					aggTemp := int32(d.Histogram.GetAggregationTemporality())
					for _, dp := range d.Histogram.GetDataPoints() {
						attrs := kvListToStringMap(dp.GetAttributes())
						count := dp.GetCount()
						histSum := dp.GetSum()
						histogramRows = append(histogramRows, map[string]any{
							"TimeUnix":               nsToISO(dp.GetTimeUnixNano()),
							"ServiceName":            service,
							"MetricName":             name,
							"MetricDescription":      desc,
							"MetricUnit":             unit,
							"Attributes":             attrs,
							"Count":                  count,
							"Sum":                    histSum,
							"BucketCounts":           dp.GetBucketCounts(),
							"ExplicitBounds":         dp.GetExplicitBounds(),
							"Flags":                  dp.GetFlags(),
							"AggregationTemporality": aggTemp,
							"AttrFingerprint":        attrFingerprint(attrs),
						})
					}
				default:
					// Unsupported metric type (exponential histogram, summary):
					// fall back to a minimal gauge-like entry, matching Python behaviour.
					if service != "" && name != "" {
						gaugeRows = append(gaugeRows, map[string]any{
							"TimeUnix":          nowISO(),
							"ServiceName":       service,
							"MetricName":        name,
							"MetricDescription": desc,
							"MetricUnit":        unit,
							"Attributes":        map[string]string{},
							"Value":             0.0,
							"Flags":             uint32(0),
							"AttrFingerprint":   attrFingerprint(map[string]string{}),
						})
					}
				}
			}
		}
	}

	if err := p.insertJSONEachRow(ctx, "otel_metrics_gauge", gaugeRows); err != nil {
		return err
	}
	if err := p.insertJSONEachRow(ctx, "otel_metrics_sum", sumRows); err != nil {
		return err
	}
	return p.insertJSONEachRow(ctx, "otel_metrics_histogram", histogramRows)
}

// ConsumeOpaqueJSON stages raw JSON payloads from /v1/rum, /v1/ai, /v1/errors.
// These require domain-specific processing beyond OTLP normalisation and are
// held in sobs_ingest_opaque for downstream enrichment.
func (p *StorePipeline) ConsumeOpaqueJSON(ctx context.Context, path string, payload any) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	raw := persist.JSONString(payload)
	_, err = store.Exec(ctx, "INSERT INTO sobs_ingest_opaque (Id, Path, PayloadJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)",
		persist.NewID(), path, raw, 0, persist.Version())
	return err
}

// dpValue extracts the float64 value from a NumberDataPoint.
func dpValue(dp *metricsv1.NumberDataPoint) float64 {
	switch v := dp.GetValue().(type) {
	case *metricsv1.NumberDataPoint_AsDouble:
		return v.AsDouble
	case *metricsv1.NumberDataPoint_AsInt:
		return float64(v.AsInt)
	}
	return 0
}
