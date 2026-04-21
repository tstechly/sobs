package otlpreceiver

import (
	"context"
	"crypto/md5" //nolint:gosec // MD5 used for non-cryptographic fingerprinting only
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
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

const logAttrKeysMax = 20000

type StorePipeline struct {
	factory    extensionpoints.StoreFactory
	schemaOnce sync.Once
	schemaErr  error
	attrKeysMu sync.Mutex
	attrKeys   map[string]map[string]struct{}
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

			`CREATE TABLE IF NOT EXISTS hyperdx_sessions (
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
			`CREATE TABLE IF NOT EXISTS sobs_tag_rules (
				Id String,
				Name String,
				RecordTypes String,
				MatchField String,
				MatchOperator String,
				MatchValue String,
				MatchAttrKey String,
				TagKey String,
				TagValue String,
				ConditionsJson String DEFAULT '',
				IsDeleted UInt8 DEFAULT 0,
				Version UInt64 DEFAULT 0
			) ENGINE = ReplacingMergeTree(Version) ORDER BY Id`,
			`CREATE TABLE IF NOT EXISTS sobs_record_tags (
				RecordType String,
				RecordId String,
				TagKey String,
				TagValue String,
				IsAuto UInt8 DEFAULT 0,
				IsDeleted UInt8 DEFAULT 0,
				Version UInt64 DEFAULT 0
			) ENGINE = ReplacingMergeTree(Version) ORDER BY (RecordType, RecordId, TagKey)`,
			`CREATE TABLE IF NOT EXISTS sobs_log_attr_keys (
				RecordType LowCardinality(String) CODEC(ZSTD(1)),
				AttrKey LowCardinality(String) CODEC(ZSTD(1)),
				IsDeleted UInt8 DEFAULT 0 CODEC(T64, ZSTD(1)),
				Version UInt64 DEFAULT 0 CODEC(T64, ZSTD(1))
			) ENGINE = ReplacingMergeTree(Version)
			ORDER BY (RecordType, AttrKey)
			SETTINGS index_granularity = 8192`,
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

func extractAttrMaps(rows []map[string]any, field string) []map[string]string {
	out := make([]map[string]string, 0, len(rows))
	for _, row := range rows {
		raw, ok := row[field]
		if !ok {
			continue
		}
		attrs, ok := raw.(map[string]string)
		if !ok || len(attrs) == 0 {
			continue
		}
		out = append(out, attrs)
	}
	return out
}

func (p *StorePipeline) rememberAttrKeys(ctx context.Context, attrMaps []map[string]string, recordType string) {
	if len(attrMaps) == 0 {
		return
	}
	p.primeAttrKeyCache(ctx)

	p.attrKeysMu.Lock()
	existing := p.ensureAttrKeySet(recordType)
	if len(existing) >= logAttrKeysMax {
		p.attrKeysMu.Unlock()
		return
	}
	candidates := make([]string, 0)
	seen := make(map[string]struct{})
	for _, attrs := range attrMaps {
		for rawKey := range attrs {
			key := strings.TrimSpace(rawKey)
			if key == "" {
				continue
			}
			if _, ok := existing[key]; ok {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			if len(existing)+len(candidates) >= logAttrKeysMax {
				break
			}
			seen[key] = struct{}{}
			candidates = append(candidates, key)
		}
	}
	if len(candidates) == 0 {
		p.attrKeysMu.Unlock()
		return
	}
	for _, key := range candidates {
		existing[key] = struct{}{}
	}
	p.attrKeysMu.Unlock()

	sort.Strings(candidates)
	version := persist.Version()
	rows := make([]map[string]any, 0, len(candidates))
	for index, key := range candidates {
		rows = append(rows, map[string]any{
			"RecordType": recordType,
			"AttrKey":    key,
			"IsDeleted":  0,
			"Version":    version + uint64(index),
		})
	}
	if err := p.insertJSONEachRow(ctx, "sobs_log_attr_keys", rows); err != nil {
		p.attrKeysMu.Lock()
		for _, key := range candidates {
			delete(existing, key)
		}
		p.attrKeysMu.Unlock()
	}
}

func (p *StorePipeline) primeAttrKeyCache(ctx context.Context) {
	p.attrKeysMu.Lock()
	if p.attrKeys != nil {
		p.attrKeysMu.Unlock()
		return
	}
	p.attrKeys = map[string]map[string]struct{}{
		"log":      {},
		"span":     {},
		"resource": {},
		"scope":    {},
	}
	p.attrKeysMu.Unlock()

	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return
	}
	defer func() { _ = store.Close() }()

	for _, recordType := range []string{"log", "span", "resource", "scope"} {
		rows, queryErr := store.Query(ctx, "SELECT DISTINCT AttrKey FROM sobs_log_attr_keys FINAL WHERE RecordType = ? AND IsDeleted = 0 ORDER BY AttrKey", recordType)
		if queryErr != nil {
			continue
		}
		keys := make([]string, 0)
		for rows.Next() {
			var value any
			if scanErr := rows.Scan(&value); scanErr == nil {
				key := strings.TrimSpace(fmt.Sprint(value))
				if key != "" {
					keys = append(keys, key)
				}
			}
		}
		_ = rows.Close()
		p.attrKeysMu.Lock()
		set := p.ensureAttrKeySet(recordType)
		for _, key := range keys {
			set[key] = struct{}{}
		}
		p.attrKeysMu.Unlock()
	}
}

func (p *StorePipeline) ensureAttrKeySet(recordType string) map[string]struct{} {
	if p.attrKeys == nil {
		p.attrKeys = map[string]map[string]struct{}{}
	}
	set, ok := p.attrKeys[recordType]
	if !ok {
		set = map[string]struct{}{}
		p.attrKeys[recordType] = set
	}
	return set
}

type tagRuleCondition struct {
	MatchField    string `json:"match_field"`
	MatchOperator string `json:"match_operator"`
	MatchValue    string `json:"match_value"`
	MatchAttrKey  string `json:"match_attr_key,omitempty"`
}

type tagRule struct {
	RecordTypes   []string
	MatchField    string
	MatchOperator string
	MatchValue    string
	MatchAttrKey  string
	TagKey        string
	TagValue      string
	Conditions    []tagRuleCondition
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

func normalizeIngestTimestamp(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nowISO()
	}
	for _, layout := range []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999",
		"2006-01-02 15:04:05",
		"2006-01-02 15:04:05.999999999 -0700 MST",
		"2006-01-02 15:04:05 -0700 MST",
	} {
		if parsed, err := time.Parse(layout, trimmed); err == nil {
			return parsed.UTC().Format("2006-01-02 15:04:05.000000")
		}
	}
	return trimmed
}

func recordIDForLog(ts string, service string, traceID string, spanID string) string {
	sum := md5.Sum([]byte(service + "|" + ts + "|" + traceID + "|" + spanID)) //nolint:gosec
	return hex.EncodeToString(sum[:])
}

func recordIDForSpan(traceID string, spanID string) string {
	sum := md5.Sum([]byte(traceID + "|" + spanID)) //nolint:gosec
	return hex.EncodeToString(sum[:])
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

func traceEventStatus(code tracev1.Status_StatusCode) string {
	switch code {
	case tracev1.Status_STATUS_CODE_OK:
		return "OK"
	case tracev1.Status_STATUS_CODE_ERROR:
		return "ERROR"
	default:
		return "UNSET"
	}
}

func traceStatusCodeForEventStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "OK":
		return "STATUS_CODE_OK"
	case "ERROR":
		return "STATUS_CODE_ERROR"
	default:
		return "STATUS_CODE_UNSET"
	}
}

func mergeStringMaps(attrMaps ...map[string]string) map[string]string {
	total := 0
	for _, attrMap := range attrMaps {
		total += len(attrMap)
	}
	merged := make(map[string]string, total)
	for _, attrMap := range attrMaps {
		for key, value := range attrMap {
			merged[key] = value
		}
	}
	return merged
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func eventStringValueWithDefault(event map[string]any, key string, defaultValue string) string {
	value, ok := event[key]
	if !ok {
		return defaultValue
	}
	return stringAny(value)
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
	errorRows := make([]map[string]any, 0)
	for _, rs := range req.GetResourceSpans() {
		resourceAttrs := kvListToStringMap(rs.GetResource().GetAttributes())
		service := resourceAttrs["service.name"]
		for _, ss := range rs.GetScopeSpans() {
			scopeAttrs := kvListToStringMap(ss.GetScope().GetAttributes())
			for _, span := range ss.GetSpans() {
				startNs := span.GetStartTimeUnixNano()
				endNs := span.GetEndTimeUnixNano()
				var durationNs uint64
				if endNs > startNs {
					durationNs = endNs - startNs
				}
				spanAttrs := kvListToStringMap(span.GetAttributes())
				mergedAttrs := mergeStringMaps(resourceAttrs, scopeAttrs, spanAttrs)
				status := traceEventStatus(span.GetStatus().GetCode())
				spanKind := firstNonEmptyString(mergedAttrs["span.kind"], "INTERNAL")
				rows = append(rows, map[string]any{
					"Timestamp":          nsToISO(startNs),
					"TraceId":            hex.EncodeToString(span.GetTraceId()),
					"SpanId":             hex.EncodeToString(span.GetSpanId()),
					"ParentSpanId":       hex.EncodeToString(span.GetParentSpanId()),
					"TraceState":         "",
					"SpanName":           span.GetName(),
					"SpanKind":           spanKind,
					"ServiceName":        service,
					"ResourceAttributes": resourceAttrs,
					"ScopeName":          "",
					"ScopeVersion":       "",
					"SpanAttributes":     mergedAttrs,
					"Duration":           durationNs,
					"StatusCode":         traceStatusCodeForEventStatus(status),
					"StatusMessage":      mergedAttrs["status.message"],
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
				if strings.Contains(status, "ERROR") {
					errorAttrs := mergeStringMaps(mergedAttrs)
					errType := firstNonEmptyString(spanAttrs["exception.type"], "SpanError")
					message := firstNonEmptyString(spanAttrs["exception.message"], spanAttrs["error.message"], span.GetName())
					errorAttrs["exception.type"] = errType
					errorAttrs["exception.message"] = message
					if stack := strings.TrimSpace(spanAttrs["exception.stacktrace"]); stack != "" {
						errorAttrs["exception.stacktrace"] = stack
					}
					errorRows = append(errorRows, map[string]any{
						"Timestamp":          nsToISO(startNs),
						"TraceId":            hex.EncodeToString(span.GetTraceId()),
						"SpanId":             hex.EncodeToString(span.GetSpanId()),
						"TraceFlags":         0,
						"SeverityText":       "ERROR",
						"SeverityNumber":     severityNumber("ERROR"),
						"ServiceName":        service,
						"Body":               message,
						"ResourceSchemaUrl":  "",
						"ResourceAttributes": map[string]string{},
						"ScopeSchemaUrl":     "",
						"ScopeName":          "",
						"ScopeVersion":       "",
						"ScopeAttributes":    map[string]string{},
						"LogAttributes":      errorAttrs,
						"EventName":          "exception",
					})
				}
			}
		}
	}
	if len(rows) == 0 {
		return nil
	}
	if err := p.insertJSONEachRow(ctx, "otel_traces", rows); err != nil {
		return err
	}
	p.rememberAttrKeys(ctx, extractAttrMaps(rows, "SpanAttributes"), "span")
	p.rememberAttrKeys(ctx, extractAttrMaps(rows, "ResourceAttributes"), "resource")
	p.logTagRuleFailure(ctx, "trace", rows)
	if len(errorRows) == 0 {
		return nil
	}
	if err := p.insertJSONEachRow(ctx, "otel_logs", errorRows); err != nil {
		return err
	}
	p.rememberAttrKeys(ctx, extractAttrMaps(errorRows, "LogAttributes"), "log")
	p.logTagRuleFailure(ctx, "error", errorRows)
	return nil
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
	if err := p.insertJSONEachRow(ctx, "otel_logs", rows); err != nil {
		return err
	}
	p.rememberAttrKeys(ctx, extractAttrMaps(rows, "LogAttributes"), "log")
	p.rememberAttrKeys(ctx, extractAttrMaps(rows, "ResourceAttributes"), "resource")
	p.rememberAttrKeys(ctx, extractAttrMaps(rows, "ScopeAttributes"), "scope")
	p.logTagRuleFailure(ctx, "log", rows)
	return nil
}

func (p *StorePipeline) logTagRuleFailure(ctx context.Context, recordType string, rows []map[string]any) {
	if err := p.applyTagRules(ctx, recordType, rows); err != nil {
		log.Printf("sobs auto-tag application failed for %s: %v", recordType, err)
	}
}

func (p *StorePipeline) applyTagRules(ctx context.Context, recordType string, rows []map[string]any) error {
	rules, err := p.loadTagRules(ctx)
	if err != nil || len(rules) == 0 || len(rows) == 0 {
		return err
	}
	version := persist.Version()
	tagRows := make([]map[string]any, 0)
	for _, row := range rows {
		service, _ := row["ServiceName"].(string)
		severity, _ := row["SeverityText"].(string)
		body, _ := row["Body"].(string)
		attrs, _ := row["LogAttributes"].(map[string]string)
		if recordType == "trace" || recordType == "ai" {
			attrs, _ = row["SpanAttributes"].(map[string]string)
		}
		if attrs == nil {
			attrs = map[string]string{}
		}
		spanName, _ := row["SpanName"].(string)
		eventType, _ := row["EventName"].(string)
		traceID, _ := row["TraceId"].(string)
		spanID, _ := row["SpanId"].(string)
		ts, _ := row["Timestamp"].(string)
		recordID := recordIDForLog(ts, service, traceID, spanID)
		if recordType == "trace" || recordType == "ai" {
			recordID = recordIDForSpan(traceID, spanID)
		}
		matchedByKey := make(map[string]string)
		for _, rule := range rules {
			if matchTagRule(rule, recordType, service, severity, body, attrs, spanName, eventType) {
				matchedByKey[rule.TagKey] = rule.TagValue
			}
		}
		for tagKey, tagValue := range matchedByKey {
			tagRows = append(tagRows, map[string]any{
				"RecordType": recordType,
				"RecordId":   recordID,
				"TagKey":     tagKey,
				"TagValue":   tagValue,
				"IsAuto":     1,
				"IsDeleted":  0,
				"Version":    version,
			})
			version++
		}
	}
	if len(tagRows) == 0 {
		return nil
	}
	return p.insertJSONEachRow(ctx, "sobs_record_tags", tagRows)
}

func (p *StorePipeline) loadTagRules(ctx context.Context) ([]tagRule, error) {
	store, err := persist.Open(ctx, p.factory)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx,
		"SELECT Id, Name, RecordTypes, MatchField, MatchOperator, MatchValue, MatchAttrKey, TagKey, TagValue, ConditionsJson FROM sobs_tag_rules FINAL WHERE IsDeleted = 0 ORDER BY Name",
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	loaded := make([]tagRule, 0)
	for rows.Next() {
		var id string
		var name string
		var recordTypes string
		var matchField string
		var matchOperator string
		var matchValue string
		var matchAttrKey string
		var tagKey string
		var tagValue string
		var conditionsJSON string
		if err := rows.Scan(&id, &name, &recordTypes, &matchField, &matchOperator, &matchValue, &matchAttrKey, &tagKey, &tagValue, &conditionsJSON); err != nil {
			return loaded, err
		}
		conditions := make([]tagRuleCondition, 0)
		if strings.TrimSpace(conditionsJSON) != "" && strings.TrimSpace(conditionsJSON) != "[]" {
			_ = json.Unmarshal([]byte(conditionsJSON), &conditions)
		}
		if len(conditions) == 0 && strings.TrimSpace(matchField) != "" {
			conditions = []tagRuleCondition{{
				MatchField:    matchField,
				MatchOperator: matchOperator,
				MatchValue:    matchValue,
				MatchAttrKey:  matchAttrKey,
			}}
		}
		rule := tagRule{
			MatchField:    matchField,
			MatchOperator: matchOperator,
			MatchValue:    matchValue,
			MatchAttrKey:  matchAttrKey,
			TagKey:        tagKey,
			TagValue:      tagValue,
			Conditions:    conditions,
		}
		for _, recordType := range strings.Split(recordTypes, ",") {
			trimmed := strings.TrimSpace(recordType)
			if trimmed != "" {
				rule.RecordTypes = append(rule.RecordTypes, trimmed)
			}
		}
		loaded = append(loaded, rule)
	}
	return loaded, nil
}

func matchTagRule(rule tagRule, recordType string, service string, severity string, body string, attrs map[string]string, spanName string, eventType string) bool {
	if len(rule.RecordTypes) > 0 {
		allowed := false
		for _, value := range rule.RecordTypes {
			if value == "all" || value == recordType {
				allowed = true
				break
			}
		}
		if !allowed {
			return false
		}
	}
	if len(rule.Conditions) > 0 {
		for _, condition := range rule.Conditions {
			if !matchSingleCondition(condition, service, severity, body, attrs, spanName, eventType) {
				return false
			}
		}
		return true
	}
	return matchSingleCondition(tagRuleCondition{
		MatchField:    rule.MatchField,
		MatchOperator: rule.MatchOperator,
		MatchValue:    rule.MatchValue,
		MatchAttrKey:  rule.MatchAttrKey,
	}, service, severity, body, attrs, spanName, eventType)
}

func matchSingleCondition(condition tagRuleCondition, service string, severity string, body string, attrs map[string]string, spanName string, eventType string) bool {
	value := ""
	switch condition.MatchField {
	case "service_name":
		value = service
	case "severity":
		value = severity
	case "body":
		value = body
	case "span_name":
		value = spanName
	case "event_type":
		value = eventType
	case "attribute":
		value = attrs[condition.MatchAttrKey]
	}
	switch condition.MatchOperator {
	case "eq":
		return value == condition.MatchValue
	case "contains":
		return strings.Contains(strings.ToLower(value), strings.ToLower(condition.MatchValue))
	case "regex":
		re, err := regexp.Compile(condition.MatchValue)
		if err != nil {
			return false
		}
		return re.MatchString(value)
	default:
		return false
	}
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

// ConsumeOpaqueJSON stages raw JSON payloads for routes that do not yet have a
// dedicated normalisation path.
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

func (p *StorePipeline) ConsumeErrorsV1(ctx context.Context, req *ErrorIngestRequest) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	ts := normalizeIngestTimestamp(req.Timestamp)
	row := map[string]any{
		"Timestamp":          ts,
		"TraceId":            req.TraceID,
		"SpanId":             req.SpanID,
		"TraceFlags":         req.TraceFlags,
		"SeverityText":       "ERROR",
		"SeverityNumber":     severityNumber("ERROR"),
		"ServiceName":        req.Service,
		"Body":               req.Message,
		"ResourceSchemaUrl":  "",
		"ResourceAttributes": map[string]string{},
		"ScopeSchemaUrl":     "",
		"ScopeName":          "",
		"ScopeVersion":       "",
		"ScopeAttributes":    map[string]string{},
		"LogAttributes":      req.Attributes,
		"EventName":          "exception",
	}
	if err := p.insertJSONEachRow(ctx, "otel_logs", []map[string]any{row}); err != nil {
		return err
	}
	p.rememberAttrKeys(ctx, extractAttrMaps([]map[string]any{row}, "LogAttributes"), "log")
	p.logTagRuleFailure(ctx, "error", []map[string]any{row})
	return nil
}

func (p *StorePipeline) ConsumeAI(ctx context.Context, req *AIIngestRequest) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	ts := normalizeIngestTimestamp(req.Timestamp)
	row := map[string]any{
		"Timestamp":          ts,
		"TraceId":            req.TraceID,
		"SpanId":             req.SpanID,
		"ParentSpanId":       "",
		"TraceState":         "",
		"SpanName":           req.SpanName,
		"SpanKind":           "CLIENT",
		"ServiceName":        req.Service,
		"ResourceAttributes": map[string]string{},
		"ScopeName":          "sobs-ai",
		"ScopeVersion":       "",
		"SpanAttributes":     req.SpanAttributes,
		"Duration":           max(0, int(req.DurationMS*1_000_000)),
		"StatusCode":         "STATUS_CODE_OK",
		"StatusMessage":      "",
		"Events":             map[string]any{"Timestamp": []string{}, "Name": []string{}, "Attributes": []map[string]string{}},
		"Links":              map[string]any{"TraceId": []string{}, "SpanId": []string{}, "TraceState": []string{}, "Attributes": []map[string]string{}},
	}
	if err := p.insertJSONEachRow(ctx, "otel_traces", []map[string]any{row}); err != nil {
		return err
	}
	p.logTagRuleFailure(ctx, "ai", []map[string]any{row})
	return nil
}

func (p *StorePipeline) ConsumeRUM(ctx context.Context, req *RUMIngestRequest) error {
	if err := p.ensureSchema(ctx); err != nil {
		return err
	}
	sessionRows := make([]map[string]any, 0, len(req.Events))
	errorRows := make([]map[string]any, 0)
	for _, sourceEvent := range req.Events {
		event := cloneJSONMap(sourceEvent)
		delete(event, "clientAuthToken")
		if stack := strings.TrimSpace(stringAny(event["stack"])); stack != "" {
			event["stack"] = maybeDemangleJSStack(stack)
		}
		remapRUMConsoleStacks(event)
		ts := normalizeIngestTimestamp(stringAny(event["timestamp"]))
		sessionID := stringAny(event["sessionId"])
		eventType := strings.TrimSpace(stringAny(event["type"]))
		if eventType == "" {
			eventType = "unknown"
		}
		url := stringAny(event["url"])
		traceID, spanID, traceFlags := extractTraceFields(event)
		attrs := stringifyAttrs(event)
		for key, value := range handleBrowserContextDelta(event) {
			attrs[key] = value
		}
		if req.ClientIP != "" {
			attrs["client.ip"] = req.ClientIP
		}
		severityText := "INFO"
		if eventType == "error" || eventType == "unhandledrejection" {
			severityText = "ERROR"
		}
		sessionRows = append(sessionRows, map[string]any{
			"Timestamp":          ts,
			"TraceId":            traceID,
			"SpanId":             spanID,
			"TraceFlags":         traceFlags,
			"SeverityText":       severityText,
			"SeverityNumber":     severityNumber(severityText),
			"ServiceName":        eventStringValueWithDefault(event, "service", "browser"),
			"Body":               persist.JSONString(event),
			"ResourceSchemaUrl":  "",
			"ResourceAttributes": map[string]string{},
			"ScopeSchemaUrl":     "",
			"ScopeName":          "browser-rum",
			"ScopeVersion":       "",
			"ScopeAttributes":    map[string]string{},
			"LogAttributes":      attrs,
			"EventName":          eventType,
		})
		if eventType != "error" && eventType != "unhandledrejection" {
			continue
		}
		errAttrs := map[string]string{
			"exception.type":    eventStringValueWithDefault(event, "errorType", "JSError"),
			"exception.message": stringAny(event["message"]),
			"url.full":          url,
			"session.id":        sessionID,
		}
		if stack := strings.TrimSpace(stringAny(event["stack"])); stack != "" {
			errAttrs["exception.stacktrace"] = stack
		}
		if errorSource := strings.TrimSpace(stringAny(event["errorSource"])); errorSource != "" {
			errAttrs["error.source"] = errorSource
		}
		if page, ok := event["page"].(map[string]any); ok {
			if title := strings.TrimSpace(stringAny(page["title"])); title != "" {
				errAttrs["browser.page.title"] = title
			}
			if viewport := strings.TrimSpace(stringAny(page["viewport"])); viewport != "" {
				errAttrs["browser.viewport"] = viewport
			}
		}
		if artifact, ok := event["artifact"].(map[string]any); ok {
			if value := strings.TrimSpace(stringAny(artifact["type"])); value != "" {
				errAttrs["artifact.type"] = value
			}
			if value := strings.TrimSpace(stringAny(artifact["id"])); value != "" {
				errAttrs["artifact.id"] = value
			}
			if value := strings.TrimSpace(stringAny(artifact["url"])); value != "" {
				errAttrs["artifact.url"] = value
			}
		}
		if replay, ok := event["replay"].(map[string]any); ok {
			if value := strings.TrimSpace(stringAny(replay["id"])); value != "" {
				errAttrs["replay.id"] = value
			}
			if value := strings.TrimSpace(stringAny(replay["url"])); value != "" {
				errAttrs["replay.url"] = value
			}
		}
		errorRows = append(errorRows, map[string]any{
			"Timestamp":          ts,
			"TraceId":            traceID,
			"SpanId":             spanID,
			"TraceFlags":         traceFlags,
			"SeverityText":       "ERROR",
			"SeverityNumber":     severityNumber("ERROR"),
			"ServiceName":        "rum",
			"Body":               stringAny(event["message"]),
			"ResourceSchemaUrl":  "",
			"ResourceAttributes": map[string]string{},
			"ScopeSchemaUrl":     "",
			"ScopeName":          "browser-rum",
			"ScopeVersion":       "",
			"ScopeAttributes":    map[string]string{},
			"LogAttributes":      errAttrs,
			"EventName":          "exception",
		})
	}
	if err := p.insertJSONEachRow(ctx, "hyperdx_sessions", sessionRows); err != nil {
		return err
	}
	if err := p.insertJSONEachRow(ctx, "otel_logs", errorRows); err != nil {
		return err
	}
	p.rememberAttrKeys(ctx, extractAttrMaps(errorRows, "LogAttributes"), "log")
	p.logTagRuleFailure(ctx, "rum", sessionRows)
	p.logTagRuleFailure(ctx, "error", errorRows)
	return nil
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
