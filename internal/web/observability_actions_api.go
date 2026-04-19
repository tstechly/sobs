package web

import (
	"net/http"
	"strings"
)

func (s *Server) errorsResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/errors/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "resolve" {
		http.NotFound(w, r)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "error_id": parts[0], "state": "resolved"})
}

func (s *Server) apiTraceSpan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	id := strings.TrimPrefix(r.URL.Path, "/api/traces/span/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0})
		return
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(r.Context(), "SELECT Timestamp, ServiceName, SpanName, Duration, TraceId, StatusCode FROM otel_traces WHERE SpanId = ? ORDER BY Timestamp DESC LIMIT 1", id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0})
		return
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0})
		return
	}
	var ts string
	var service string
	var operation string
	var durationNs uint64
	var traceID string
	var statusCode string
	if err := rows.Scan(&ts, &service, &operation, &durationNs, &traceID, &statusCode); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"span_id":     id,
		"trace_id":    traceID,
		"timestamp":   ts,
		"service":     service,
		"operation":   operation,
		"duration_ms": float64(durationNs) / 1_000_000.0,
		"status":      statusCode,
	})
}
