package otlpreceiver

import (
	"encoding/json"
	"strconv"
	"strings"
)

type AIIngestRequest struct {
	Timestamp      string            `json:"timestamp"`
	Model          string            `json:"model"`
	Operation      string            `json:"operation"`
	DurationMS     float64           `json:"duration_ms"`
	Provider       string            `json:"provider"`
	Service        string            `json:"service"`
	TraceID        string            `json:"trace_id"`
	SpanID         string            `json:"span_id"`
	SpanName       string            `json:"span_name"`
	TokensIn       int               `json:"tokens_in"`
	TokensOut      int               `json:"tokens_out"`
	SpanAttributes map[string]string `json:"span_attributes"`
}

func normalizeAIIngestRequest(payload map[string]any) *AIIngestRequest {
	ts := normalizeIngestTimestamp(stringAny(payload["timestamp"]))
	model := stringAny(payload["model"])
	operation := strings.TrimSpace(strings.ToLower(stringAny(payload["operation"])))
	if operation == "" {
		operation = "chat"
	}
	durationMS := float64Any(payload["duration_ms"])
	provider := stringAny(payload["provider"])
	service := stringAny(payload["service"])
	spanName := strings.TrimSpace(operation + " " + model)
	tokensIn := intAny(payload["tokens_in"])
	tokensOut := intAny(payload["tokens_out"])
	spanAttrs := map[string]string{
		"gen_ai.operation.name":      operation,
		"gen_ai.provider.name":       provider,
		"gen_ai.request.model":       model,
		"gen_ai.usage.input_tokens":  strconv.Itoa(tokensIn),
		"gen_ai.usage.output_tokens": strconv.Itoa(tokensOut),
	}
	if raw, ok := payload["input_messages"]; ok && raw != nil {
		if text, ok := raw.(string); ok {
			spanAttrs["gen_ai.input.messages"] = text
		} else {
			spanAttrs["gen_ai.input.messages"] = persistJSONString(raw)
		}
	}
	if raw, ok := payload["output_messages"]; ok && raw != nil {
		if text, ok := raw.(string); ok {
			spanAttrs["gen_ai.output.messages"] = text
		} else {
			spanAttrs["gen_ai.output.messages"] = persistJSONString(raw)
		}
	}
	if raw, ok := payload["system_instructions"]; ok && raw != nil {
		if text, ok := raw.(string); ok {
			spanAttrs["gen_ai.system_instructions"] = text
		} else {
			spanAttrs["gen_ai.system_instructions"] = persistJSONString(raw)
		}
	}
	if prompt := stringAny(payload["prompt"]); prompt != "" {
		spanAttrs["sobs.gen_ai.prompt"] = prompt
	}
	if response := stringAny(payload["response"]); response != "" {
		spanAttrs["sobs.gen_ai.response"] = response
	}
	if errorType := stringAny(payload["error_type"]); errorType != "" {
		spanAttrs["error.type"] = errorType
	}
	return &AIIngestRequest{
		Timestamp:      ts,
		Model:          model,
		Operation:      operation,
		DurationMS:     durationMS,
		Provider:       provider,
		Service:        service,
		TraceID:        stringAny(payload["trace_id"]),
		SpanID:         stringAny(payload["span_id"]),
		SpanName:       spanName,
		TokensIn:       tokensIn,
		TokensOut:      tokensOut,
		SpanAttributes: spanAttrs,
	}
}

func cloneAIIngestRequest(req *AIIngestRequest) *AIIngestRequest {
	if req == nil {
		return &AIIngestRequest{}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return &AIIngestRequest{Timestamp: req.Timestamp, Model: req.Model, Operation: req.Operation, DurationMS: req.DurationMS, Provider: req.Provider, Service: req.Service, TraceID: req.TraceID, SpanID: req.SpanID, SpanName: req.SpanName, TokensIn: req.TokensIn, TokensOut: req.TokensOut, SpanAttributes: req.SpanAttributes}
	}
	var cloned AIIngestRequest
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return &AIIngestRequest{Timestamp: req.Timestamp, Model: req.Model, Operation: req.Operation, DurationMS: req.DurationMS, Provider: req.Provider, Service: req.Service, TraceID: req.TraceID, SpanID: req.SpanID, SpanName: req.SpanName, TokensIn: req.TokensIn, TokensOut: req.TokensOut, SpanAttributes: req.SpanAttributes}
	}
	return &cloned
}

func parseAIJSONLenient(body []byte) map[string]any {
	if strings.TrimSpace(string(body)) == "" {
		return map[string]any{}
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil || payload == nil {
		return map[string]any{}
	}
	return payload
}

func intAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		parsed, _ := typed.Int64()
		return int(parsed)
	default:
		parsed, _ := strconv.Atoi(strings.TrimSpace(stringAny(value)))
		return parsed
	}
}

func float64Any(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		parsed, _ := typed.Float64()
		return parsed
	default:
		parsed, _ := strconv.ParseFloat(strings.TrimSpace(stringAny(value)), 64)
		return parsed
	}
}