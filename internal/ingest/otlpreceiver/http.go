package otlpreceiver

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	collogspb "go.opentelemetry.io/proto/otlp/collector/logs/v1"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type HTTPServer struct {
	receiver *Receiver
}

type OpaqueJSONConsumer interface {
	ConsumeOpaqueJSON(ctx context.Context, path string, payload any) error
}

// NewHTTPServer is retained for test convenience; production code should use
// NewHTTPServerWithPipeline to supply a real pipeline.
func NewHTTPServer() *HTTPServer {
	return NewHTTPServerWithPipeline(NewNoopPipeline())
}

func NewHTTPServerWithPipeline(pipeline Pipeline) *HTTPServer {
	return &HTTPServer{receiver: NewReceiver(pipeline)}
}

func (s *HTTPServer) Register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/traces", s.acceptTraces)
	mux.HandleFunc("/v1/metrics", s.acceptMetrics)
	mux.HandleFunc("/v1/logs", s.acceptLogs)
	mux.HandleFunc("/v1/errors", s.acceptOpaqueJSON)
	mux.HandleFunc("/v1/rum", s.acceptOpaqueJSON)
	mux.HandleFunc("/v1/ai", s.acceptOpaqueJSON)
}

func (s *HTTPServer) acceptTraces(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &coltracepb.ExportTraceServiceRequest{}
	if err := decodeProtoBody(r, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.receiver.pipeline.ConsumeTraces(r.Context(), req); err != nil {
		http.Error(w, "ingest failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *HTTPServer) acceptMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &colmetricpb.ExportMetricsServiceRequest{}
	if err := decodeProtoBody(r, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.receiver.pipeline.ConsumeMetrics(r.Context(), req); err != nil {
		http.Error(w, "ingest failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *HTTPServer) acceptLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	req := &collogspb.ExportLogsServiceRequest{}
	if err := decodeProtoBody(r, req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := s.receiver.pipeline.ConsumeLogs(r.Context(), req); err != nil {
		http.Error(w, "ingest failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
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

func decodeProtoBody(r *http.Request, message proto.Message) error {
	defer r.Body.Close()
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("invalid body")
	}
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	if contentType == "application/json" {
		if len(body) == 0 {
			return fmt.Errorf("empty body")
		}
		if err := protojson.Unmarshal(body, message); err != nil {
			return fmt.Errorf("invalid json payload")
		}
		return nil
	}
	if err := proto.Unmarshal(body, message); err != nil {
		return fmt.Errorf("invalid protobuf payload")
	}
	return nil
}
