package ingest

import (
	"compress/flate"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sobshttp "github.com/sobs/sobs-api/internal/http"
	"github.com/sobs/sobs-api/internal/storage"
	"github.com/sobs/sobs-api/internal/stream"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	colmetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const protobufMIME = "application/x-protobuf"

// Handler manages OTLP ingest endpoints.
type Handler struct {
	DB     *storage.DB
	Broker *stream.Broker
}

// Logs handles POST /v1/logs.
func (h *Handler) Logs(w http.ResponseWriter, r *http.Request) {
	msg := &collogspb.ExportLogsServiceRequest{}
	if err := parseOTLP(r, msg); err != nil {
		sobshttp.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	events := protoLogsToRows(msg)
	if err := h.DB.QueueWrite(func(db *storage.DB) error {
		return db.InsertJSONRows("otel_logs", events)
	}); err != nil {
		if strings.Contains(err.Error(), "write queue is full") {
			sobshttp.JSONError(w, "write queue is full", http.StatusServiceUnavailable)
			return
		}
		slog.Error("log ingest write failed", "error", err)
		sobshttp.JSONError(w, "log ingest write failed", http.StatusInternalServerError)
		return
	}
	for _, e := range events {
		h.Broker.Broadcast(map[string]any{
			"source":  "logs",
			"ts":      e["Timestamp"],
			"level":   e["SeverityText"],
			"service": e["ServiceName"],
			"body":    e["Body"],
		})
	}
	sobshttp.JSON(w, http.StatusOK, map[string]any{"accepted": len(events)})
}

// Traces handles POST /v1/traces.
func (h *Handler) Traces(w http.ResponseWriter, r *http.Request) {
	msg := &coltracepb.ExportTraceServiceRequest{}
	if err := parseOTLP(r, msg); err != nil {
		sobshttp.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	spans := protoTracesToRows(msg)
	if err := h.DB.QueueWrite(func(db *storage.DB) error {
		return db.InsertJSONRows("otel_traces", spans)
	}); err != nil {
		if strings.Contains(err.Error(), "write queue is full") {
			sobshttp.JSONError(w, "write queue is full", http.StatusServiceUnavailable)
			return
		}
		slog.Error("trace ingest write failed", "error", err)
		sobshttp.JSONError(w, "trace ingest write failed", http.StatusInternalServerError)
		return
	}
	for _, e := range spans {
		attrs, _ := e["SpanAttributes"].(map[string]string)
		h.Broker.Broadcast(map[string]any{
			"source":      "traces",
			"ts":          e["Timestamp"],
			"trace_id":    e["TraceId"],
			"span_id":     e["SpanId"],
			"name":        e["SpanName"],
			"service":     e["ServiceName"],
			"duration_ms": e["Duration"],
			"status":      e["StatusCode"],
		})
		// Broadcast AI event for GenAI spans
		provider := attrs["gen_ai.provider.name"]
		if provider == "" {
			provider = attrs["gen_ai.system"]
		}
		operation := attrs["gen_ai.operation.name"]
		if provider != "" || operation != "" {
			h.Broker.Broadcast(map[string]any{
				"source":      "ai",
				"ts":          e["Timestamp"],
				"trace_id":    e["TraceId"],
				"span_id":     e["SpanId"],
				"service":     e["ServiceName"],
				"provider":    provider,
				"model":       attrs["gen_ai.request.model"],
				"operation":   operation,
				"duration_ms": e["Duration"],
				"status":      e["StatusCode"],
			})
		}
	}
	sobshttp.JSON(w, http.StatusOK, map[string]any{"accepted": len(spans)})
}

// Metrics handles POST /v1/metrics.
func (h *Handler) Metrics(w http.ResponseWriter, r *http.Request) {
	msg := &colmetricspb.ExportMetricsServiceRequest{}
	if err := parseOTLP(r, msg); err != nil {
		sobshttp.JSONError(w, err.Error(), http.StatusBadRequest)
		return
	}
	rows := protoMetricsToRows(msg)
	if len(rows) == 0 {
		sobshttp.JSON(w, http.StatusOK, map[string]any{"accepted": 0})
		return
	}
	if err := h.DB.QueueWrite(func(db *storage.DB) error {
		// Group by table
		byTable := map[string][]map[string]any{}
		for _, r := range rows {
			t, _ := r["_table"].(string)
			delete(r, "_table")
			byTable[t] = append(byTable[t], r)
		}
		for t, rs := range byTable {
			if err := db.InsertJSONRows(t, rs); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		if strings.Contains(err.Error(), "write queue is full") {
			sobshttp.JSONError(w, "write queue is full", http.StatusServiceUnavailable)
			return
		}
		slog.Error("metric ingest write failed", "error", err)
		sobshttp.JSONError(w, "metric ingest write failed", http.StatusInternalServerError)
		return
	}
	sobshttp.JSON(w, http.StatusOK, map[string]any{"accepted": len(rows)})
}

// Errors handles POST /v1/errors.
func (h *Handler) Errors(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		payload = map[string]any{}
	}
	ts := strOr(payload, "timestamp", time.Now().UTC().Format(time.RFC3339Nano))
	attrs := stringifyAttrs(asMap(payload, "attributes"))
	attrs["exception.type"] = strOr(payload, "type", "Error")
	attrs["exception.message"] = strOr(payload, "message", "")
	if v, ok := payload["stack"].(string); ok && v != "" {
		attrs["exception.stacktrace"] = v
	}
	row := map[string]any{
		"Timestamp":          ts,
		"TraceId":            strOr(payload, "trace_id", ""),
		"SpanId":             strOr(payload, "span_id", ""),
		"TraceFlags":         0,
		"SeverityText":       "ERROR",
		"SeverityNumber":     17,
		"ServiceName":        strOr(payload, "service", ""),
		"Body":               strOr(payload, "message", ""),
		"ResourceSchemaUrl":  "",
		"ResourceAttributes": map[string]string{},
		"ScopeSchemaUrl":     "",
		"ScopeName":          "",
		"ScopeVersion":       "",
		"ScopeAttributes":    map[string]string{},
		"LogAttributes":      attrs,
		"EventName":          "exception",
	}
	if err := h.DB.QueueWrite(func(db *storage.DB) error {
		return db.InsertJSONRows("otel_logs", []map[string]any{row})
	}); err != nil {
		if strings.Contains(err.Error(), "write queue is full") {
			sobshttp.JSONError(w, "write queue is full", http.StatusServiceUnavailable)
			return
		}
		slog.Error("error ingest write failed", "error", err)
		sobshttp.JSONError(w, "error ingest write failed", http.StatusInternalServerError)
		return
	}
	sobshttp.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

// --- helpers ---

func parseOTLP(r *http.Request, msg proto.Message) error {
	body, err := readBody(r)
	if err != nil {
		return fmt.Errorf("failed to read request body: %w", err)
	}
	mime := strings.ToLower(r.Header.Get("Content-Type"))
	if strings.HasPrefix(mime, protobufMIME) {
		if err := proto.Unmarshal(body, msg); err != nil {
			return fmt.Errorf("failed to parse protobuf body: %w", err)
		}
		return nil
	}
	// JSON path
	if len(body) == 0 {
		return nil
	}
	if err := protojson.Unmarshal(body, msg); err != nil {
		return fmt.Errorf("failed to parse json body: %w", err)
	}
	return nil
}

func readBody(r *http.Request) ([]byte, error) {
	var reader io.Reader = r.Body
	switch strings.ToLower(r.Header.Get("Content-Encoding")) {
	case "gzip":
		gr, err := gzip.NewReader(r.Body)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
		reader = gr
	case "deflate":
		reader = flate.NewReader(r.Body)
	}
	return io.ReadAll(reader)
}

func protoLogsToRows(msg *collogspb.ExportLogsServiceRequest) []map[string]any {
	var rows []map[string]any
	for _, rl := range msg.GetResourceLogs() {
		res := rl.GetResource()
		resAttrs := kvListToMap(res.GetAttributes())
		svcName := resAttrs["service.name"]
		for _, sl := range rl.GetScopeLogs() {
			scope := sl.GetScope()
			for _, lr := range sl.GetLogRecords() {
				ts := time.Unix(0, int64(lr.GetTimeUnixNano())).UTC().Format(time.RFC3339Nano)
				if lr.GetTimeUnixNano() == 0 {
					ts = time.Now().UTC().Format(time.RFC3339Nano)
				}
				rows = append(rows, map[string]any{
					"Timestamp":          ts,
					"TraceId":            fmt.Sprintf("%x", lr.GetTraceId()),
					"SpanId":             fmt.Sprintf("%x", lr.GetSpanId()),
					"TraceFlags":         lr.GetFlags(),
					"SeverityText":       lr.GetSeverityText(),
					"SeverityNumber":     int(lr.GetSeverityNumber()),
					"ServiceName":        svcName,
					"Body":               anyValueToString(lr.GetBody()),
					"ResourceSchemaUrl":  rl.GetSchemaUrl(),
					"ResourceAttributes": resAttrs,
					"ScopeSchemaUrl":     sl.GetSchemaUrl(),
					"ScopeName":          scope.GetName(),
					"ScopeVersion":       scope.GetVersion(),
					"ScopeAttributes":    kvListToMap(scope.GetAttributes()),
					"LogAttributes":      kvListToMap(lr.GetAttributes()),
					"EventName":          "",
				})
			}
		}
	}
	return rows
}

func protoTracesToRows(msg *coltracepb.ExportTraceServiceRequest) []map[string]any {
	var rows []map[string]any
	for _, rs := range msg.GetResourceSpans() {
		res := rs.GetResource()
		resAttrs := kvListToMap(res.GetAttributes())
		svcName := resAttrs["service.name"]
		for _, ss := range rs.GetScopeSpans() {
			scope := ss.GetScope()
			for _, span := range ss.GetSpans() {
				ts := time.Unix(0, int64(span.GetStartTimeUnixNano())).UTC().Format(time.RFC3339Nano)
				dur := int64(span.GetEndTimeUnixNano()) - int64(span.GetStartTimeUnixNano())
				if dur < 0 {
					dur = 0
				}
				rows = append(rows, map[string]any{
					"Timestamp":          ts,
					"TraceId":            fmt.Sprintf("%x", span.GetTraceId()),
					"SpanId":             fmt.Sprintf("%x", span.GetSpanId()),
					"ParentSpanId":       fmt.Sprintf("%x", span.GetParentSpanId()),
					"TraceState":         span.GetTraceState(),
					"SpanName":           span.GetName(),
					"SpanKind":           span.GetKind().String(),
					"ServiceName":        svcName,
					"ResourceAttributes": resAttrs,
					"ScopeName":          scope.GetName(),
					"ScopeVersion":       scope.GetVersion(),
					"SpanAttributes":     kvListToMap(span.GetAttributes()),
					"Duration":           dur,
					"StatusCode":         span.GetStatus().GetCode().String(),
					"StatusMessage":      span.GetStatus().GetMessage(),
					"Events":             map[string]any{"Timestamp": []any{}, "Name": []any{}, "Attributes": []any{}},
					"Links":              map[string]any{"TraceId": []any{}, "SpanId": []any{}, "TraceState": []any{}, "Attributes": []any{}},
				})
			}
		}
	}
	return rows
}

func protoMetricsToRows(msg *colmetricspb.ExportMetricsServiceRequest) []map[string]any {
	var rows []map[string]any
	for _, rm := range msg.GetResourceMetrics() {
		res := rm.GetResource()
		resAttrs := kvListToMap(res.GetAttributes())
		svcName := resAttrs["service.name"]
		for _, sm := range rm.GetScopeMetrics() {
			for _, m := range sm.GetMetrics() {
				name := m.GetName()
				desc := m.GetDescription()
				unit := m.GetUnit()
				switch d := m.GetData().(type) {
				case *metricspb.Metric_Gauge:
					for _, dp := range d.Gauge.GetDataPoints() {
						rows = append(rows, metricRow("otel_metrics_gauge", svcName, name, desc, unit, resAttrs, dp.GetAttributes(), dp.GetTimeUnixNano(), gaugeValue(dp)))
					}
				case *metricspb.Metric_Sum:
					for _, dp := range d.Sum.GetDataPoints() {
						rows = append(rows, metricRow("otel_metrics_sum", svcName, name, desc, unit, resAttrs, dp.GetAttributes(), dp.GetTimeUnixNano(), gaugeValue(dp)))
					}
				case *metricspb.Metric_Histogram:
					for _, dp := range d.Histogram.GetDataPoints() {
						rows = append(rows, metricRow("otel_metrics_histogram", svcName, name, desc, unit, resAttrs, dp.GetAttributes(), dp.GetTimeUnixNano(), dp.GetSum()))
					}
				}
			}
		}
	}
	return rows
}

type numberDataPoint interface {
	GetTimeUnixNano() uint64
	GetAttributes() []*commonpb.KeyValue
}

func gaugeValue(dp interface{ GetAsDouble() float64 }) float64 {
	return dp.GetAsDouble()
}

func metricRow(table, svcName, name, desc, unit string, resAttrs map[string]string, dpAttrs []*commonpb.KeyValue, tsNano uint64, value float64) map[string]any {
	ts := time.Unix(0, int64(tsNano)).UTC().Format(time.RFC3339Nano)
	return map[string]any{
		"_table":             table,
		"Timestamp":          ts,
		"MetricName":         name,
		"MetricDescription":  desc,
		"MetricUnit":         unit,
		"ServiceName":        svcName,
		"ResourceAttributes": resAttrs,
		"Attributes":         kvListToMap(dpAttrs),
		"Value":              value,
	}
}

func kvListToMap(kvs []*commonpb.KeyValue) map[string]string {
	m := make(map[string]string, len(kvs))
	for _, kv := range kvs {
		m[kv.GetKey()] = anyValueToString(kv.GetValue())
	}
	return m
}

func anyValueToString(v *commonpb.AnyValue) string {
	if v == nil {
		return ""
	}
	switch val := v.GetValue().(type) {
	case *commonpb.AnyValue_StringValue:
		return val.StringValue
	case *commonpb.AnyValue_IntValue:
		return fmt.Sprintf("%d", val.IntValue)
	case *commonpb.AnyValue_DoubleValue:
		return fmt.Sprintf("%g", val.DoubleValue)
	case *commonpb.AnyValue_BoolValue:
		return fmt.Sprintf("%t", val.BoolValue)
	default:
		b, _ := json.Marshal(v)
		return string(b)
	}
}

func strOr(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

func asMap(m map[string]any, key string) map[string]any {
	if v, ok := m[key]; ok {
		if mm, ok := v.(map[string]any); ok {
			return mm
		}
	}
	return map[string]any{}
}

func stringifyAttrs(m map[string]any) map[string]string {
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = fmt.Sprintf("%v", v)
	}
	return out
}
