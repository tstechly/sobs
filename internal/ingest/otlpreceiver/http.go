package otlpreceiver

import (
	"bytes"
	"compress/flate"
	"compress/gzip"
	"compress/zlib"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const maxDecompressedBodyBytes = 32 * 1024 * 1024

type HTTPServer struct {
	receiver *Receiver
	tail     *TailBroker
}

type RUMIngestRequest struct {
	Events   []map[string]any `json:"events"`
	ClientIP string           `json:"clientIp"`
}

type OpaqueJSONConsumer interface {
	ConsumeOpaqueJSON(ctx context.Context, path string, payload any) error
}

type RUMConsumer interface {
	ConsumeRUM(ctx context.Context, req *RUMIngestRequest) error
}

type AIConsumer interface {
	ConsumeAI(ctx context.Context, req *AIIngestRequest) error
}

type ErrorConsumer interface {
	ConsumeErrorsV1(ctx context.Context, req *ErrorIngestRequest) error
}

// NewHTTPServer is retained for test convenience; production code should use
// NewHTTPServerWithPipeline to supply a real pipeline.
func NewHTTPServer() *HTTPServer {
	return NewHTTPServerWithPipeline(NewNoopPipeline())
}

func NewHTTPServerWithPipeline(pipeline Pipeline) *HTTPServer {
	return &HTTPServer{receiver: NewReceiver(pipeline)}
}

func NewHTTPServerWithPipelineAndTailBroker(pipeline Pipeline, tail *TailBroker) *HTTPServer {
	return &HTTPServer{receiver: NewReceiver(pipeline), tail: tail}
}

func (s *HTTPServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/traces", s.acceptTraces)
	mux.HandleFunc("/v1/metrics", s.acceptMetrics)
	mux.HandleFunc("/v1/logs", s.acceptLogs)
	mux.HandleFunc("/v1/errors", s.acceptErrors)
	mux.HandleFunc("/v1/rum", s.acceptRUM)
	mux.HandleFunc("/v1/ai", s.acceptAI)
}

func (s *HTTPServer) acceptTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &coltracepb.ExportTraceServiceRequest{}
	if err := decodeProtoBody(r, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.receiver.pipeline.ConsumeTraces(r.Context(), req); err != nil {
		var queueFull WriteQueueFullError
		if errors.As(err, &queueFull) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": queueFull.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "trace ingest write failed"})
		return
	}
	if s.tail != nil {
		for _, event := range traceTailEvents(req) {
			s.tail.Publish(event)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": countTraceSpans(req)})
}

func (s *HTTPServer) acceptMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &colmetricpb.ExportMetricsServiceRequest{}
	if err := decodeProtoBody(r, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.receiver.pipeline.ConsumeMetrics(r.Context(), req); err != nil {
		var queueFull WriteQueueFullError
		if errors.As(err, &queueFull) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": queueFull.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "metric ingest write failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": countMetricDataPoints(req)})
}

func (s *HTTPServer) acceptLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &collogspb.ExportLogsServiceRequest{}
	if err := decodeProtoBody(r, req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	if err := s.receiver.pipeline.ConsumeLogs(r.Context(), req); err != nil {
		var queueFull WriteQueueFullError
		if errors.As(err, &queueFull) {
			writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": queueFull.Error()})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "log ingest write failed"})
		return
	}
	if s.tail != nil {
		for _, event := range logTailEvents(req) {
			s.tail.Publish(event)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": countLogRecords(req)})
}

func (s *HTTPServer) acceptOpaqueJSON(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "invalid body", http.StatusBadRequest)
		return
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		http.Error(w, "empty body", http.StatusBadRequest)
		return
	}
	var payload any
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if payload == nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if consumer, ok := s.receiver.pipeline.(OpaqueJSONConsumer); ok {
		if err := consumer.ConsumeOpaqueJSON(r.Context(), r.URL.Path, payload); err != nil {
			http.Error(w, "ingest failed", http.StatusInternalServerError)
			return
		}
	}
	w.WriteHeader(http.StatusOK)
}

func (s *HTTPServer) acceptRUM(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := readEncodedRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to read request body"})
		return
	}
	events := parseRUMEventsLenient(body)
	ok, statusCode, authErr := verifyRUMClientAuth(r, events)
	if !ok {
		writeJSON(w, statusCode, map[string]any{"error": authErr})
		return
	}
	if consumer, ok := s.receiver.pipeline.(RUMConsumer); ok {
		req := &RUMIngestRequest{Events: events, ClientIP: requestClientIP(r)}
		if err := consumer.ConsumeRUM(r.Context(), req); err != nil {
			var queueFull WriteQueueFullError
			if errors.As(err, &queueFull) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": queueFull.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "rum ingest write failed"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"accepted": len(events)})
}

func (s *HTTPServer) acceptAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := readEncodedRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to read request body"})
		return
	}
	req := normalizeAIIngestRequest(parseAIJSONLenient(body))
	if consumer, ok := s.receiver.pipeline.(AIConsumer); ok {
		if err := consumer.ConsumeAI(r.Context(), req); err != nil {
			var queueFull WriteQueueFullError
			if errors.As(err, &queueFull) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": queueFull.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "ai ingest write failed"})
			return
		}
	}
	if s.tail != nil {
		s.tail.Publish(TailEvent{
			Source:     "ai",
			TS:         req.Timestamp,
			Service:    req.Service,
			Provider:   req.Provider,
			Model:      req.Model,
			Operation:  req.Operation,
			DurationMS: math.Round(req.DurationMS*10) / 10,
			TokensIn:   req.TokensIn,
			TokensOut:  req.TokensOut,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *HTTPServer) acceptErrors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	body, err := readEncodedRequestBody(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "failed to read request body"})
		return
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid json"})
		return
	}
	req := normalizeErrorIngestRequest(payload)
	if consumer, ok := s.receiver.pipeline.(ErrorConsumer); ok {
		if err := consumer.ConsumeErrorsV1(r.Context(), req); err != nil {
			var queueFull WriteQueueFullError
			if errors.As(err, &queueFull) {
				writeJSON(w, http.StatusServiceUnavailable, map[string]any{"error": queueFull.Error()})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "error ingest write failed"})
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func decodeProtoBody(r *http.Request, message proto.Message) error {
	body, err := readEncodedRequestBody(r)
	if err != nil {
		return err
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType == "application/json" {
		payload := body
		if len(payload) == 0 {
			payload = []byte("{}")
		}
		var topLevel any
		if err := json.Unmarshal(payload, &topLevel); err != nil {
			return fmt.Errorf("failed to read request body")
		}
		if _, ok := topLevel.(map[string]any); !ok {
			return fmt.Errorf("failed to parse json body")
		}
		if err := protojson.Unmarshal(payload, message); err != nil {
			return fmt.Errorf("failed to parse json body")
		}
		return nil
	}
	if err := proto.Unmarshal(body, message); err != nil {
		return fmt.Errorf("failed to parse protobuf body")
	}
	return nil
}

func readEncodedRequestBody(r *http.Request) ([]byte, error) {
	defer r.Body.Close()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read request body")
	}
	encodings := make([]string, 0)
	for _, encoding := range strings.Split(r.Header.Get("Content-Encoding"), ",") {
		encoding = strings.ToLower(strings.TrimSpace(encoding))
		if encoding != "" {
			encodings = append(encodings, encoding)
		}
	}
	data := raw
	for index := len(encodings) - 1; index >= 0; index-- {
		decoded, err := decodeBodyEncoding(data, encodings[index])
		if err != nil {
			return nil, fmt.Errorf("failed to read request body")
		}
		data = decoded
	}
	if len(data) > maxDecompressedBodyBytes {
		return nil, fmt.Errorf("failed to read request body")
	}
	return data, nil
}

func decodeBodyEncoding(raw []byte, encoding string) ([]byte, error) {
	if encoding == "" {
		return raw, nil
	}
	var reader io.ReadCloser
	switch encoding {
	case "gzip":
		gzipReader, err := gzip.NewReader(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		reader = gzipReader
	case "deflate":
		zlibReader, err := zlib.NewReader(bytes.NewReader(raw))
		if err == nil {
			reader = zlibReader
			break
		}
		reader = io.NopCloser(flate.NewReader(bytes.NewReader(raw)))
	default:
		return raw, nil
	}
	defer reader.Close()
	limited := io.LimitReader(reader, maxDecompressedBodyBytes+1)
	decoded, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(decoded) > maxDecompressedBodyBytes {
		return nil, fmt.Errorf("decompressed body too large")
	}
	return decoded, nil
}

func countLogRecords(req *collogspb.ExportLogsServiceRequest) int {
	count := 0
	for _, resourceLogs := range req.GetResourceLogs() {
		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			count += len(scopeLogs.GetLogRecords())
		}
	}
	return count
}

func countTraceSpans(req *coltracepb.ExportTraceServiceRequest) int {
	count := 0
	for _, resourceSpans := range req.GetResourceSpans() {
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			count += len(scopeSpans.GetSpans())
		}
	}
	return count
}

func countMetricDataPoints(req *colmetricpb.ExportMetricsServiceRequest) int {
	count := 0
	for _, resourceMetrics := range req.GetResourceMetrics() {
		for _, scopeMetrics := range resourceMetrics.GetScopeMetrics() {
			for _, metric := range scopeMetrics.GetMetrics() {
				count += len(metric.GetGauge().GetDataPoints())
				count += len(metric.GetSum().GetDataPoints())
				count += len(metric.GetHistogram().GetDataPoints())
			}
		}
	}
	return count
}

func logTailEvents(req *collogspb.ExportLogsServiceRequest) []TailEvent {
	out := make([]TailEvent, 0)
	for _, resourceLogs := range req.GetResourceLogs() {
		service := ""
		for _, attr := range resourceLogs.GetResource().GetAttributes() {
			if attr.GetKey() == "service.name" {
				service = anyValueToString(attr.GetValue())
				break
			}
		}
		for _, scopeLogs := range resourceLogs.GetScopeLogs() {
			for _, record := range scopeLogs.GetLogRecords() {
				out = append(out, TailEvent{
					Source:  "logs",
					TS:      nsToISO(record.GetTimeUnixNano()),
					Level:   record.GetSeverityText(),
					Service: service,
					Body:    anyValueToString(record.GetBody()),
					TraceID: fmt.Sprintf("%x", record.GetTraceId()),
				})
			}
		}
	}
	return out
}

func traceTailEvents(req *coltracepb.ExportTraceServiceRequest) []TailEvent {
	out := make([]TailEvent, 0)
	for _, resourceSpans := range req.GetResourceSpans() {
		resourceAttrs := kvListToStringMap(resourceSpans.GetResource().GetAttributes())
		service := resourceAttrs["service.name"]
		for _, scopeSpans := range resourceSpans.GetScopeSpans() {
			scopeAttrs := kvListToStringMap(scopeSpans.GetScope().GetAttributes())
			for _, span := range scopeSpans.GetSpans() {
				spanAttrs := kvListToStringMap(span.GetAttributes())
				mergedAttrs := mergeStringMaps(resourceAttrs, scopeAttrs, spanAttrs)
				traceID := hex.EncodeToString(span.GetTraceId())
				spanID := hex.EncodeToString(span.GetSpanId())
				durationMS := traceDurationMS(span.GetStartTimeUnixNano(), span.GetEndTimeUnixNano())
				status := traceEventStatus(span.GetStatus().GetCode())
				out = append(out, TailEvent{
					Source:     "traces",
					TS:         nsToISO(span.GetStartTimeUnixNano()),
					TraceID:    traceID,
					SpanID:     spanID,
					Name:       span.GetName(),
					Service:    service,
					DurationMS: durationMS,
					Status:     status,
				})
				provider := firstNonEmptyString(mergedAttrs["gen_ai.provider.name"], mergedAttrs["gen_ai.system"])
				operation := mergedAttrs["gen_ai.operation.name"]
				if provider != "" || operation != "" {
					out = append(out, TailEvent{
						Source:     "ai",
						TS:         nsToISO(span.GetStartTimeUnixNano()),
						TraceID:    traceID,
						SpanID:     spanID,
						Service:    service,
						Provider:   provider,
						Model:      mergedAttrs["gen_ai.request.model"],
						Operation:  operation,
						DurationMS: durationMS,
						Status:     status,
					})
				}
			}
		}
	}
	return out
}

func traceDurationMS(startNs uint64, endNs uint64) float64 {
	if endNs <= startNs {
		return 0
	}
	return float64(endNs-startNs) / 1_000_000.0
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
