package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// apiLogsList returns paginated logs as JSON for AJAX client consumption
func (s *Server) apiLogsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	limit := parseLimitFromQueryParam(r, 200, 1, 10000)
	offset := parseOffsetFromQueryParam(r)
	level := strings.TrimSpace(r.URL.Query().Get("level"))
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	// Build WHERE clause
	conditions := []string{}
	params := []any{}

	if level != "" {
		conditions = append(conditions, "SeverityText = ?")
		params = append(params, level)
	}
	if service != "" {
		conditions = append(conditions, "ServiceName = ?")
		params = append(params, service)
	}
	if q != "" {
		conditions = append(conditions, "Body ILIKE ?")
		params = append(params, "%"+q+"%")
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get store
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer store.Close()

	// Count total
	var countSQL string
	if where == "" {
		countSQL = "SELECT count() FROM otel_logs"
	} else {
		countSQL = fmt.Sprintf("SELECT count() FROM otel_logs %s", where)
	}

	rows, err := store.Query(r.Context(), countSQL, params...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	total := 0
	if rows.Next() {
		var c uint64
		if err := rows.Scan(&c); err == nil {
			total = int(c)
		}
	}

	// Query data
	selectSQL := fmt.Sprintf(
		"SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId "+
			"FROM otel_logs %s ORDER BY Timestamp DESC LIMIT %d OFFSET %d",
		where, limit, offset,
	)

	rows, err = store.Query(r.Context(), selectSQL, params...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	logs := []map[string]any{}
	for rows.Next() {
		var ts, severity, service, body, traceID, spanID string
		if err := rows.Scan(&ts, &severity, &service, &body, &traceID, &spanID); err != nil {
			continue
		}
		logs = append(logs, map[string]any{
			"timestamp": ts,
			"level":     severity,
			"service":   service,
			"body":      body,
			"trace_id":  traceID,
			"span_id":   spanID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"logs":   logs,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// apiErrorsList returns paginated errors as JSON
func (s *Server) apiErrorsList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := parseLimitFromQueryParam(r, 100, 1, 10000)
	offset := parseOffsetFromQueryParam(r)
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))

	conditions := []string{"SeverityText IN ('ERROR', 'FATAL')"}
	params := []any{}

	if service != "" {
		conditions = append(conditions, "ServiceName = ?")
		params = append(params, service)
	}
	if q != "" {
		conditions = append(conditions, "Body ILIKE ?")
		params = append(params, "%"+q+"%")
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer store.Close()

	// Count total
	countSQL := fmt.Sprintf("SELECT count() FROM otel_logs %s", where)
	rows, err := store.Query(r.Context(), countSQL, params...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	total := 0
	if rows.Next() {
		var c uint64
		if err := rows.Scan(&c); err == nil {
			total = int(c)
		}
	}

	// Query data
	selectSQL := fmt.Sprintf(
		"SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId "+
			"FROM otel_logs %s ORDER BY Timestamp DESC LIMIT %d OFFSET %d",
		where, limit, offset,
	)

	rows, err = store.Query(r.Context(), selectSQL, params...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	errors := []map[string]any{}
	for rows.Next() {
		var ts, severity, service, body, traceID, spanID string
		if err := rows.Scan(&ts, &severity, &service, &body, &traceID, &spanID); err != nil {
			continue
		}
		errors = append(errors, map[string]any{
			"timestamp": ts,
			"severity":  severity,
			"service":   service,
			"body":      body,
			"trace_id":  traceID,
			"span_id":   spanID,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"errors": errors,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// apiTracesList returns paginated traces as JSON
func (s *Server) apiTracesList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	limit := parseLimitFromQueryParam(r, 100, 1, 10000)
	offset := parseOffsetFromQueryParam(r)
	service := strings.TrimSpace(r.URL.Query().Get("service"))
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))

	conditions := []string{}
	params := []any{}

	if service != "" {
		conditions = append(conditions, "ServiceName = ?")
		params = append(params, service)
	}
	if traceID != "" {
		conditions = append(conditions, "TraceId = ?")
		params = append(params, traceID)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer store.Close()

	// Count total
	var countSQL string
	if where == "" {
		countSQL = "SELECT count() FROM otel_traces"
	} else {
		countSQL = fmt.Sprintf("SELECT count() FROM otel_traces %s", where)
	}

	rows, err := store.Query(r.Context(), countSQL, params...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	total := 0
	if rows.Next() {
		var c uint64
		if err := rows.Scan(&c); err == nil {
			total = int(c)
		}
	}

	// Query data
	selectSQL := fmt.Sprintf(
		"SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode "+
			"FROM otel_traces %s ORDER BY Timestamp DESC LIMIT %d OFFSET %d",
		where, limit, offset,
	)

	rows, err = store.Query(r.Context(), selectSQL, params...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer rows.Close()

	traces := []map[string]any{}
	for rows.Next() {
		var ts, traceID, spanID, parentSpanID, spanName, service string
		var duration int64
		var statusCode int32
		if err := rows.Scan(&ts, &traceID, &spanID, &parentSpanID, &spanName, &service, &duration, &statusCode); err != nil {
			continue
		}
		traces = append(traces, map[string]any{
			"timestamp":      ts,
			"trace_id":       traceID,
			"span_id":        spanID,
			"parent_span_id": parentSpanID,
			"name":           spanName,
			"service":        service,
			"duration_ms":    float64(duration) / 1_000_000,
			"status":         statusCode,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"traces": traces,
		"total":  total,
		"limit":  limit,
		"offset": offset,
	})
}

// Helper functions

func parseLimitFromQueryParam(r *http.Request, def, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return def
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < min {
		return def
	}
	if val > max {
		return max
	}
	return val
}

func parseOffsetFromQueryParam(r *http.Request) int {
	raw := strings.TrimSpace(r.URL.Query().Get("offset"))
	if raw == "" {
		return 0
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 0 {
		return 0
	}
	return val
}
