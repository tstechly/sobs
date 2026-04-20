package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

const rawSpanMaxBytes = 32 * 1024

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
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "span_id is required"})
		return
	}
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer func() { _ = store.Close() }()
	query := "SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes FROM otel_traces WHERE SpanId = ?"
	params := []any{id}
	if traceID != "" {
		query += " AND TraceId = ?"
		params = append(params, traceID)
	}
	query += " ORDER BY Timestamp DESC LIMIT 1"
	rows, err := store.Query(r.Context(), query, params...)
	withAttrs := err == nil
	if err != nil {
		query = "SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode FROM otel_traces WHERE SpanId = ?"
		params = []any{id}
		if traceID != "" {
			query += " AND TraceId = ?"
			params = append(params, traceID)
		}
		query += " ORDER BY Timestamp DESC LIMIT 1"
		rows, err = store.Query(r.Context(), query, params...)
		withAttrs = false
	}
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "span not found"})
		return
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		writeJSON(w, http.StatusNotFound, map[string]any{"error": "span not found"})
		return
	}
	var ts, tid, sid, parentSID, name, service, duration, status, attrs any
	if withAttrs {
		if err := rows.Scan(&ts, &tid, &sid, &parentSID, &name, &service, &duration, &status, &attrs); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
	} else {
		if err := rows.Scan(&ts, &tid, &sid, &parentSID, &name, &service, &duration, &status); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
			return
		}
		attrs = ""
	}
	attrMap := parseStringMap(anyToString(attrs))
	payload := map[string]any{
		"timestamp":           anyToString(ts),
		"trace_id":            anyToString(tid),
		"span_id":             anyToString(sid),
		"parent_span_id":      anyToString(parentSID),
		"name":                anyToString(name),
		"service":             anyToString(service),
		"duration_ns":         anyToInt(duration),
		"duration_ms":         roundFloat(float64(anyToInt(duration))/1000000.0, 3),
		"status_code":         normalizeTraceStatus(anyToString(status)),
		"attributes":          attrMap,
		"resource_attributes": map[string]any{},
	}
	rawBytes, _ := json.MarshalIndent(payload, "", "  ")
	truncated := false
	if len(rawBytes) > rawSpanMaxBytes {
		truncated = true
		for key, value := range attrMap {
			if len(value) > 512 {
				attrMap[key] = value[:512] + "..."
			}
		}
		payload["attributes"] = attrMap
		rawBytes, _ = json.MarshalIndent(payload, "", "  ")
	}
	writeJSON(w, http.StatusOK, map[string]any{"span": payload, "raw": string(rawBytes), "truncated": truncated})
}
