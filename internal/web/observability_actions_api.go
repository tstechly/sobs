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
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer func() { _ = store.Close() }()

	_, _ = store.Exec(r.Context(), "CREATE TABLE IF NOT EXISTS sobs_error_resolutions (ErrorId String, CreatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(CreatedAt) ORDER BY (ErrorId)")
	if _, err := store.Exec(r.Context(), "INSERT INTO sobs_error_resolutions (ErrorId) VALUES (?)", parts[0]); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
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
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0, "raw": map[string]any{}})
		return
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(r.Context(), "SELECT Timestamp, ServiceName, SpanName, Duration, TraceId, StatusCode FROM otel_traces WHERE SpanId = ? ORDER BY Timestamp DESC LIMIT 1", id)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0, "raw": map[string]any{}})
		return
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0, "raw": map[string]any{}})
		return
	}
	var ts string
	var service string
	var operation string
	var durationNs uint64
	var traceID string
	var statusCode string
	if err := rows.Scan(&ts, &service, &operation, &durationNs, &traceID, &statusCode); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"span_id": id, "service": "", "operation": "", "duration_ms": 0, "raw": map[string]any{}})
		return
	}
	rawAttrs := ""
	attrRows, attrErr := store.Query(r.Context(), "SELECT SpanAttributes FROM otel_traces WHERE SpanId = ? ORDER BY Timestamp DESC LIMIT 1", id)
	if attrErr == nil {
		defer func() { _ = attrRows.Close() }()
		if attrRows.Next() {
			var attrs any
			if scanErr := attrRows.Scan(&attrs); scanErr == nil {
				rawAttrs = anyToString(attrs)
				if len(rawAttrs) > 32*1024 {
					rawAttrs = rawAttrs[:32*1024]
				}
			}
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"span_id":     id,
		"trace_id":    traceID,
		"timestamp":   ts,
		"service":     service,
		"operation":   operation,
		"duration_ms": float64(durationNs) / 1_000_000.0,
		"status":      statusCode,
		"raw": map[string]any{
			"span_attributes": rawAttrs,
		},
	})
}
