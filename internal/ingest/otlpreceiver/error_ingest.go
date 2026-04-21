package otlpreceiver

import (
	"encoding/json"
	"strings"
)

type ErrorIngestRequest struct {
	Timestamp      string            `json:"timestamp"`
	TraceID        string            `json:"trace_id"`
	SpanID         string            `json:"span_id"`
	TraceFlags     int               `json:"trace_flags"`
	Service        string            `json:"service"`
	Message        string            `json:"message"`
	ExceptionType  string            `json:"exception_type"`
	ExceptionStack string            `json:"exception_stack"`
	Attributes     map[string]string `json:"attributes"`
}

func normalizeErrorIngestRequest(payload map[string]any) *ErrorIngestRequest {
	ts := normalizeIngestTimestamp(stringAny(payload["timestamp"]))
	attrs, _ := payload["attributes"].(map[string]any)
	stringAttrs := stringifyAttrs(attrs)
	exceptionType := firstNonEmptyString(stringAny(payload["type"]), "Error")
	message := stringAny(payload["message"])
	stringAttrs["exception.type"] = exceptionType
	stringAttrs["exception.message"] = message
	stack := strings.TrimSpace(stringAny(payload["stack"]))
	if stack != "" {
		stack = maybeDemangleJSStack(stack)
		stringAttrs["exception.stacktrace"] = stack
	}
	return &ErrorIngestRequest{
		Timestamp:      ts,
		TraceID:        stringAny(payload["trace_id"]),
		SpanID:         stringAny(payload["span_id"]),
		TraceFlags:     0,
		Service:        stringAny(payload["service"]),
		Message:        message,
		ExceptionType:  exceptionType,
		ExceptionStack: stack,
		Attributes:     stringAttrs,
	}
}

func cloneErrorIngestRequest(req *ErrorIngestRequest) *ErrorIngestRequest {
	if req == nil {
		return &ErrorIngestRequest{}
	}
	raw, err := json.Marshal(req)
	if err != nil {
		return &ErrorIngestRequest{Timestamp: req.Timestamp, TraceID: req.TraceID, SpanID: req.SpanID, TraceFlags: req.TraceFlags, Service: req.Service, Message: req.Message, ExceptionType: req.ExceptionType, ExceptionStack: req.ExceptionStack, Attributes: req.Attributes}
	}
	var cloned ErrorIngestRequest
	if err := json.Unmarshal(raw, &cloned); err != nil {
		return &ErrorIngestRequest{Timestamp: req.Timestamp, TraceID: req.TraceID, SpanID: req.SpanID, TraceFlags: req.TraceFlags, Service: req.Service, Message: req.Message, ExceptionType: req.ExceptionType, ExceptionStack: req.ExceptionStack, Attributes: req.Attributes}
	}
	return &cloned
}