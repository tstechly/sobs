package ai

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	sobshttp "github.com/sobs/sobs-api/internal/http"
	"github.com/sobs/sobs-api/internal/storage"
	"github.com/sobs/sobs-api/internal/stream"
)

// Handler manages AI transparency ingest.
type Handler struct {
	DB     *storage.DB
	Broker *stream.Broker
}

// Ingest handles POST /v1/ai.
func (h *Handler) Ingest(w http.ResponseWriter, r *http.Request) {
	var payload map[string]any
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		payload = map[string]any{}
	}
	ts := strOr(payload, "timestamp", time.Now().UTC().Format(time.RFC3339Nano))
	model := strOr(payload, "model", "")
	operation := strings.ToLower(strings.TrimSpace(strOr(payload, "operation", "chat")))
	if operation == "" {
		operation = "chat"
	}
	durationMs := floatOr(payload, "duration_ms", 0)
	provider := strOr(payload, "provider", "")
	service := strOr(payload, "service", "")
	spanName := strings.TrimSpace(fmt.Sprintf("%s %s", operation, model))
	tokensIn := intOr(payload, "tokens_in", 0)
	tokensOut := intOr(payload, "tokens_out", 0)

	spanAttrs := map[string]string{
		"gen_ai.operation.name":     operation,
		"gen_ai.provider.name":      provider,
		"gen_ai.request.model":      model,
		"gen_ai.usage.input_tokens":  fmt.Sprintf("%d", tokensIn),
		"gen_ai.usage.output_tokens": fmt.Sprintf("%d", tokensOut),
	}
	// Standard OTel GenAI content attributes
	for _, pair := range [][2]string{
		{"input_messages", "gen_ai.input.messages"},
		{"output_messages", "gen_ai.output.messages"},
		{"system_instructions", "gen_ai.system_instructions"},
	} {
		if v, ok := payload[pair[0]]; ok && v != nil {
			if s, ok := v.(string); ok {
				spanAttrs[pair[1]] = s
			} else {
				b, _ := json.Marshal(v)
				spanAttrs[pair[1]] = string(b)
			}
		}
	}
	// Legacy sobs fields
	if v := strOr(payload, "prompt", ""); v != "" {
		spanAttrs["sobs.gen_ai.prompt"] = v
	}
	if v := strOr(payload, "response", ""); v != "" {
		spanAttrs["sobs.gen_ai.response"] = v
	}
	if v := strOr(payload, "error_type", ""); v != "" {
		spanAttrs["error.type"] = v
	}

	durNs := int64(durationMs * 1_000_000)
	if durNs < 0 {
		durNs = 0
	}

	row := map[string]any{
		"Timestamp":          ts,
		"TraceId":            strOr(payload, "trace_id", ""),
		"SpanId":             strOr(payload, "span_id", ""),
		"ParentSpanId":       "",
		"TraceState":         "",
		"SpanName":           spanName,
		"SpanKind":           "CLIENT",
		"ServiceName":        service,
		"ResourceAttributes": map[string]string{},
		"ScopeName":          "sobs-ai",
		"ScopeVersion":       "",
		"SpanAttributes":     spanAttrs,
		"Duration":           durNs,
		"StatusCode":         "STATUS_CODE_OK",
		"StatusMessage":      "",
		"Events":             map[string]any{"Timestamp": []any{}, "Name": []any{}, "Attributes": []any{}},
		"Links":              map[string]any{"TraceId": []any{}, "SpanId": []any{}, "TraceState": []any{}, "Attributes": []any{}},
	}

	if err := h.DB.QueueWrite(func(db *storage.DB) error {
		return db.InsertJSONRows("otel_traces", []map[string]any{row})
	}); err != nil {
		if strings.Contains(err.Error(), "write queue is full") {
			sobshttp.JSONError(w, "write queue is full", http.StatusServiceUnavailable)
			return
		}
		slog.Error("ai ingest write failed", "error", err)
		sobshttp.JSONError(w, "ai ingest write failed", http.StatusInternalServerError)
		return
	}

	h.Broker.Broadcast(map[string]any{
		"source":     "ai",
		"ts":         ts,
		"service":    service,
		"provider":   provider,
		"model":      model,
		"operation":  operation,
		"duration_ms": durationMs,
		"tokens_in":  tokensIn,
		"tokens_out": tokensOut,
	})

	sobshttp.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func strOr(m map[string]any, key, fallback string) string {
	if v, ok := m[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return fallback
}

func floatOr(m map[string]any, key string, fallback float64) float64 {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return n
		case json.Number:
			f, _ := n.Float64()
			return f
		}
	}
	return fallback
}

func intOr(m map[string]any, key string, fallback int) int {
	if v, ok := m[key]; ok {
		switch n := v.(type) {
		case float64:
			return int(n)
		case json.Number:
			i, _ := n.Int64()
			return int(i)
		}
	}
	return fallback
}
