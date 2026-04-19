package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/flosch/pongo2/v6"
)

const (
	defaultLogsLimit   = 200
	defaultErrorsLimit = 100
	defaultTracesLimit = 100
)

func (s *Server) pageLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs" {
		http.NotFound(w, r)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	selectedLevels := r.URL.Query()["level"]
	selectedServices := r.URL.Query()["service"]
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	sqlWhere := strings.TrimSpace(r.URL.Query().Get("sql"))
	limit := parseLimitParam(r, defaultLogsLimit, 1, 10000)
	offset := parseOffsetParam(r)

	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortDir := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sort_dir")))
	if sortDir != "ASC" {
		sortDir = "DESC"
	}
	sortCol := "Timestamp"
	switch sortBy {
	case "severity", "SeverityText":
		sortCol = "SeverityText"
	case "service", "ServiceName":
		sortCol = "ServiceName"
	}
	orderClause := fmt.Sprintf("ORDER BY %s %s", sortCol, sortDir)

	services, levels, err := s.listLogsFilterOptions(r)
	if err != nil {
		s.renderPageError(w, "logs", err)
		return
	}

	where, params, errMsg := buildLogsWhereClause(selectedLevels, selectedServices, traceID, fromTS, toTS, q, sqlWhere)
	if errMsg != "" {
		s.renderPageWithError(w, "logs.html", "logs", errMsg, services, levels)
		return
	}

	rows, total, err := s.queryLogs(r, where, params, orderClause, limit, offset)
	if err != nil {
		s.renderPageError(w, "logs", err)
		return
	}

	s.renderTemplate(w, "logs.html", pongo2.Context{
		"title":                 "Logs",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "logs"},
		"logs":                  rows,
		"total":                 total,
		"limit":                 limit,
		"offset":                offset,
		"q":                     q,
		"selected_levels":       selectedLevels,
		"selected_services":     selectedServices,
		"trace_id":              traceID,
		"trace_ids_count":       countTraceIDs(traceID),
		"from_ts":               fromTS,
		"to_ts":                 toTS,
		"services":              services,
		"levels":                levels,
		"sort_by":               sortBy,
		"sort_dir":              strings.ToLower(sortDir),
		"level_stats":           computeStatsFromRows(rows, "level"),
		"service_stats":         computeStatsFromRows(rows, "service"),
		"error_msg":             "",
	})
}

func (s *Server) pageErrorsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/errors" {
		http.NotFound(w, r)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	selectedServices := r.URL.Query()["service"]
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	groupBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group_by")))
	grouped := r.URL.Query().Get("grouped") == "1" || groupBy != ""
	limit := parseLimitParam(r, defaultErrorsLimit, 1, 10000)
	offset := parseOffsetParam(r)

	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortDir := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sort_dir")))
	if sortDir != "ASC" {
		sortDir = "DESC"
	}
	sortCol := "Timestamp"
	if grouped {
		sortCol = "Count"
		switch sortBy {
		case "last_seen", "Timestamp":
			sortCol = "LastSeen"
		case "service", "ServiceName":
			sortCol = "ServiceName"
		case "count":
			sortCol = "Count"
		}
	} else if sortBy == "service" {
		sortCol = "ServiceName"
	}
	orderClause := fmt.Sprintf("ORDER BY %s %s", sortCol, sortDir)

	services, err := s.listServicesFromLogs(r)
	if err != nil {
		s.renderPageError(w, "errors", err)
		return
	}

	where, params, errMsg := buildErrorsWhereClause(selectedServices, fromTS, toTS, q)
	if errMsg != "" {
		s.renderPageWithError(w, "errors.html", "errors", errMsg, services, nil)
		return
	}

	var rows []map[string]any
	var total int
	var queryErr error
	if grouped {
		rows, total, queryErr = s.queryErrorsGrouped(r, where, params, orderClause, limit, offset)
	} else {
		rows, total, queryErr = s.queryErrors(r, where, params, orderClause, limit, offset)
	}
	if queryErr != nil {
		s.renderPageError(w, "errors", queryErr)
		return
	}

	s.renderTemplate(w, "errors.html", pongo2.Context{
		"title":                 "Errors",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "errors"},
		"errors":                rows,
		"total":                 total,
		"limit":                 limit,
		"offset":                offset,
		"selected_services":     selectedServices,
		"q":                     q,
		"from_ts":               fromTS,
		"to_ts":                 toTS,
		"services":              services,
		"sort_by":               sortBy,
		"sort_dir":              strings.ToLower(sortDir),
		"grouped":               grouped,
		"group_by":              groupBy,
		"error_msg":             "",
	})
}

func (s *Server) pageTracesHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/traces" {
		http.NotFound(w, r)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	selectedServices := r.URL.Query()["service"]
	traceID := strings.TrimSpace(r.URL.Query().Get("trace_id"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	limit := parseLimitParam(r, defaultTracesLimit, 1, 10000)
	offset := parseOffsetParam(r)

	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortDir := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sort_dir")))
	if sortDir != "ASC" {
		sortDir = "DESC"
	}
	sortCol := "Timestamp"
	switch sortBy {
	case "span_name", "SpanName":
		sortCol = "SpanName"
	case "service", "ServiceName":
		sortCol = "ServiceName"
	case "duration", "Duration":
		sortCol = "Duration"
	}
	orderClause := fmt.Sprintf("ORDER BY %s %s", sortCol, sortDir)

	services, err := s.listServicesFromTraces(r)
	if err != nil {
		s.renderPageError(w, "traces", err)
		return
	}

	where, params, errMsg := buildTracesWhereClause(selectedServices, traceID, fromTS, toTS, q)
	if errMsg != "" {
		s.renderPageWithError(w, "traces.html", "traces", errMsg, services, nil)
		return
	}

	rows, total, err := s.queryTraces(r, where, params, orderClause, limit, offset)
	if err != nil {
		s.renderPageError(w, "traces", err)
		return
	}

	s.renderTemplate(w, "traces.html", pongo2.Context{
		"title":                 "Traces",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "traces"},
		"spans":                 rows,
		"total":                 total,
		"limit":                 limit,
		"offset":                offset,
		"trace_id":              traceID,
		"selected_services":     selectedServices,
		"q":                     q,
		"from_ts":               fromTS,
		"to_ts":                 toTS,
		"services":              services,
		"sort_by":               sortBy,
		"sort_dir":              strings.ToLower(sortDir),
		"error_msg":             "",
	})
}

func buildLogsWhereClause(levels, services []string, traceID, fromTS, toTS, q, sqlWhere string) (string, []any, string) {
	if sqlWhere != "" {
		safeSQL := normalizeLogsSQLWhere(sqlWhere)
		if err := validateUserSQLWhere(safeSQL); err != nil {
			return "", nil, err.Error()
		}
		where := "WHERE " + safeSQL
		params := []any{}
		if fromTS != "" {
			where += " AND Timestamp >= parseDateTime64BestEffort(?, 9)"
			params = append(params, fromTS)
		}
		if toTS != "" {
			where += " AND Timestamp < parseDateTime64BestEffort(?, 9)"
			params = append(params, toTS)
		}
		return where, params, ""
	}
	conditions := []string{}
	params := []any{}

	if len(levels) > 0 {
		marks := []string{}
		for _, level := range levels {
			level = strings.TrimSpace(level)
			if level != "" {
				marks = append(marks, "?")
				params = append(params, level)
			}
		}
		if len(marks) > 0 {
			conditions = append(conditions, "SeverityText IN ("+strings.Join(marks, ",")+")")
		}
	}

	if len(services) > 0 {
		marks := []string{}
		for _, svc := range services {
			svc = strings.TrimSpace(svc)
			if svc != "" {
				marks = append(marks, "?")
				params = append(params, svc)
			}
		}
		if len(marks) > 0 {
			conditions = append(conditions, "ServiceName IN ("+strings.Join(marks, ",")+")")
		}
	}

	if traceID != "" {
		conditions = append(conditions, "lower(TraceId) = ?")
		params = append(params, strings.ToLower(traceID))
	}
	if fromTS != "" {
		conditions = append(conditions, "Timestamp >= parseDateTime64BestEffort(?, 9)")
		params = append(params, fromTS)
	}
	if toTS != "" {
		conditions = append(conditions, "Timestamp < parseDateTime64BestEffort(?, 9)")
		params = append(params, toTS)
	}
	if q != "" {
		conditions = append(conditions, "Body ILIKE ?")
		params = append(params, "%"+q+"%")
	}

	if len(conditions) == 0 {
		return "", params, ""
	}
	return "WHERE " + strings.Join(conditions, " AND "), params, ""
}

func buildErrorsWhereClause(services []string, fromTS, toTS, q string) (string, []any, string) {
	conditions := []string{}
	params := []any{}

	if len(services) > 0 {
		marks := []string{}
		for _, svc := range services {
			svc = strings.TrimSpace(svc)
			if svc != "" {
				marks = append(marks, "?")
				params = append(params, svc)
			}
		}
		if len(marks) > 0 {
			conditions = append(conditions, "ServiceName IN ("+strings.Join(marks, ",")+")")
		}
	}
	if fromTS != "" {
		conditions = append(conditions, "Timestamp >= parseDateTime64BestEffort(?, 9)")
		params = append(params, fromTS)
	}
	if toTS != "" {
		conditions = append(conditions, "Timestamp < parseDateTime64BestEffort(?, 9)")
		params = append(params, toTS)
	}
	if q != "" {
		conditions = append(conditions, "Body ILIKE ?")
		params = append(params, "%"+q+"%")
	}

	if len(conditions) == 0 {
		return "", params, ""
	}
	return "WHERE " + strings.Join(conditions, " AND "), params, ""
}

func buildTracesWhereClause(services []string, traceID, fromTS, toTS, q string) (string, []any, string) {
	conditions := []string{}
	params := []any{}

	if len(services) > 0 {
		marks := []string{}
		for _, svc := range services {
			svc = strings.TrimSpace(svc)
			if svc != "" {
				marks = append(marks, "?")
				params = append(params, svc)
			}
		}
		if len(marks) > 0 {
			conditions = append(conditions, "ServiceName IN ("+strings.Join(marks, ",")+")")
		}
	}
	if traceID != "" {
		conditions = append(conditions, "TraceId = ?")
		params = append(params, traceID)
	}
	if fromTS != "" {
		conditions = append(conditions, "Timestamp >= parseDateTime64BestEffort(?, 9)")
		params = append(params, fromTS)
	}
	if toTS != "" {
		conditions = append(conditions, "Timestamp < parseDateTime64BestEffort(?, 9)")
		params = append(params, toTS)
	}
	if q != "" {
		conditions = append(conditions, "SpanName ILIKE ?")
		params = append(params, "%"+q+"%")
	}

	if len(conditions) == 0 {
		return "", params, ""
	}
	return "WHERE " + strings.Join(conditions, " AND "), params, ""
}

func (s *Server) listLogsFilterOptions(r *http.Request) ([]string, []string, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, nil, err
	}
	defer store.Close()

	services := []string{}
	serviceRows, err := store.Query(r.Context(), "SELECT DISTINCT ServiceName FROM otel_logs WHERE ServiceName != '' ORDER BY ServiceName")
	if err == nil {
		defer serviceRows.Close()
		for serviceRows.Next() {
			var svc any
			if scanErr := serviceRows.Scan(&svc); scanErr == nil {
				if value := anyToString(svc); value != "" {
					services = append(services, value)
				}
			}
		}
	}

	levels := []string{}
	levelRows, err := store.Query(r.Context(), "SELECT DISTINCT SeverityText FROM otel_logs WHERE SeverityText != '' ORDER BY SeverityText")
	if err == nil {
		defer levelRows.Close()
		for levelRows.Next() {
			var level any
			if scanErr := levelRows.Scan(&level); scanErr == nil {
				if value := anyToString(level); value != "" {
					levels = append(levels, value)
				}
			}
		}
	}

	return services, levels, nil
}

func (s *Server) listServicesFromLogs(r *http.Request) ([]string, error) {
	services, _, err := s.listLogsFilterOptions(r)
	return services, err
}

func (s *Server) listServicesFromTraces(r *http.Request) ([]string, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, err
	}
	defer store.Close()

	rows, err := store.Query(r.Context(), "SELECT DISTINCT ServiceName FROM otel_traces WHERE ServiceName != '' ORDER BY ServiceName")
	if err != nil {
		if isMissingTableError(err) {
			return []string{}, nil
		}
		return nil, err
	}
	defer rows.Close()

	services := []string{}
	for rows.Next() {
		var svc any
		if scanErr := rows.Scan(&svc); scanErr == nil {
			if value := anyToString(svc); value != "" {
				services = append(services, value)
			}
		}
	}
	return services, nil
}

func (s *Server) queryLogs(r *http.Request, where string, params []any, orderClause string, limit, offset int) ([]map[string]any, int, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, 0, err
	}
	defer store.Close()

	total, err := queryCount(r, store, "otel_logs", where, params)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, 0, nil
		}
		return nil, 0, err
	}

	query := fmt.Sprintf("SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId FROM otel_logs %s %s LIMIT %d OFFSET %d", where, orderClause, limit, offset)
	rows, err := store.Query(r.Context(), query, params...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, total, nil
		}
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]any{}
	for rows.Next() {
		var ts, level, service, body, traceID, spanID any
		if scanErr := rows.Scan(&ts, &level, &service, &body, &traceID, &spanID); scanErr != nil {
			continue
		}
		result = append(result, map[string]any{
			"ts":       anyToString(ts),
			"level":    anyToString(level),
			"service":  anyToString(service),
			"body":     anyToString(body),
			"trace_id": anyToString(traceID),
			"span_id":  anyToString(spanID),
		})
	}
	return result, total, nil
}

func (s *Server) queryErrors(r *http.Request, where string, params []any, orderClause string, limit, offset int) ([]map[string]any, int, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, 0, err
	}
	defer store.Close()

	finalWhere := "WHERE SeverityText IN ('ERROR','FATAL')"
	if where != "" {
		finalWhere = "WHERE SeverityText IN ('ERROR','FATAL') AND (" + strings.TrimPrefix(where, "WHERE ") + ")"
	}

	total, err := queryCount(r, store, "otel_logs", finalWhere, params)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, 0, nil
		}
		return nil, 0, err
	}

	query := fmt.Sprintf("SELECT Timestamp, SeverityText, ServiceName, Body, TraceId, SpanId FROM otel_logs %s %s LIMIT %d OFFSET %d", finalWhere, orderClause, limit, offset)
	rows, err := store.Query(r.Context(), query, params...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, total, nil
		}
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]any{}
	for rows.Next() {
		var ts, level, service, body, traceID, spanID any
		if scanErr := rows.Scan(&ts, &level, &service, &body, &traceID, &spanID); scanErr != nil {
			continue
		}
		result = append(result, map[string]any{
			"ts":       anyToString(ts),
			"err_type": anyToString(level),
			"service":  anyToString(service),
			"message":  anyToString(body),
			"trace_id": anyToString(traceID),
			"span_id":  anyToString(spanID),
		})
	}
	return result, total, nil
}

func (s *Server) queryErrorsGrouped(r *http.Request, where string, params []any, orderClause string, limit, offset int) ([]map[string]any, int, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, 0, err
	}
	defer store.Close()

	finalWhere := "WHERE SeverityText IN ('ERROR','FATAL')"
	if where != "" {
		finalWhere = "WHERE SeverityText IN ('ERROR','FATAL') AND (" + strings.TrimPrefix(where, "WHERE ") + ")"
	}

	groupedSQL := "SELECT ServiceName, SeverityText, Body, TraceId, SpanId, min(Timestamp) AS FirstSeen, max(Timestamp) AS LastSeen, count() AS Count FROM otel_logs " + finalWhere + " GROUP BY ServiceName, SeverityText, Body, TraceId, SpanId"

	countRows, err := store.Query(r.Context(), "SELECT count() FROM ("+groupedSQL+")", params...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, 0, nil
		}
		return nil, 0, err
	}
	defer countRows.Close()
	total := 0
	if countRows.Next() {
		var c any
		if scanErr := countRows.Scan(&c); scanErr == nil {
			total = anyToInt(c)
		}
	}

	query := "SELECT * FROM (" + groupedSQL + ") " + orderClause + fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	rows, err := store.Query(r.Context(), query, params...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, total, nil
		}
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]any{}
	for rows.Next() {
		var service, level, body, traceID, spanID, firstSeen, lastSeen, count any
		if scanErr := rows.Scan(&service, &level, &body, &traceID, &spanID, &firstSeen, &lastSeen, &count); scanErr != nil {
			continue
		}
		result = append(result, map[string]any{
			"ts":         anyToString(lastSeen),
			"last_seen":  anyToString(lastSeen),
			"first_seen": anyToString(firstSeen),
			"count":      anyToInt(count),
			"err_type":   anyToString(level),
			"service":    anyToString(service),
			"message":    anyToString(body),
			"raw_body":   anyToString(body),
			"trace_id":   anyToString(traceID),
			"span_id":    anyToString(spanID),
			"resolved":   false,
		})
	}
	return result, total, nil
}

func (s *Server) queryTraces(r *http.Request, where string, params []any, orderClause string, limit, offset int) ([]map[string]any, int, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, 0, err
	}
	defer store.Close()

	total, err := queryCount(r, store, "otel_traces", where, params)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, 0, nil
		}
		return nil, 0, err
	}

	query := fmt.Sprintf("SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode FROM otel_traces %s %s LIMIT %d OFFSET %d", where, orderClause, limit, offset)
	rows, err := store.Query(r.Context(), query, params...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, total, nil
		}
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]any{}
	for rows.Next() {
		var ts, traceID, spanID, parentSpanID, spanName, service, duration, status any
		if scanErr := rows.Scan(&ts, &traceID, &spanID, &parentSpanID, &spanName, &service, &duration, &status); scanErr != nil {
			continue
		}
		result = append(result, map[string]any{
			"ts":             anyToString(ts),
			"trace_id":       anyToString(traceID),
			"span_id":        anyToString(spanID),
			"parent_span_id": anyToString(parentSpanID),
			"name":           anyToString(spanName),
			"service":        anyToString(service),
			"duration_ms":    float64(anyToInt(duration)) / 1000000.0,
			"status":         anyToInt(status),
		})
	}
	return result, total, nil
}

func (s *Server) renderPageError(w http.ResponseWriter, pageName string, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "page": pageName})
}

func (s *Server) renderPageWithError(w http.ResponseWriter, templateName, pageName, errMsg string, services, levels []string) {
	s.renderTemplate(w, templateName, pongo2.Context{
		"title":                 strings.Title(pageName),
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": pageName},
		"services":              services,
		"levels":                levels,
		"error_msg":             errMsg,
	})
}

func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, ctx pongo2.Context) {
	body, err := s.renderer.Render(templateName, ctx)
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func parseLimitParam(r *http.Request, def, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return def
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < min {
		return def
	}
	if value > max {
		return max
	}
	return value
}

func parseOffsetParam(r *http.Request) int {
	raw := strings.TrimSpace(r.URL.Query().Get("offset"))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func computeStatsFromRows(rows []map[string]any, fieldName string) map[string]int {
	stats := map[string]int{}
	for _, row := range rows {
		value := anyToString(row[fieldName])
		if value != "" {
			stats[value]++
		}
	}
	return stats
}

func countTraceIDs(traceID string) int {
	if strings.TrimSpace(traceID) == "" {
		return 0
	}
	count := 0
	for _, part := range strings.Split(traceID, ",") {
		if strings.TrimSpace(part) != "" {
			count++
		}
	}
	return count
}

func queryCount(r *http.Request, store extensionpoints.ClickHouseStore, tableName, where string, params []any) (int, error) {
	if strings.TrimSpace(where) == "" {
		if tableName == "otel_logs" || tableName == "otel_traces" || tableName == "hyperdx_sessions" {
			rows, err := store.Query(r.Context(), "SELECT COALESCE(sum(rows), 0) AS c FROM system.parts WHERE active = 1 AND database = currentDatabase() AND table = ?", tableName)
			if err == nil {
				defer rows.Close()
				if rows.Next() {
					var count any
					if scanErr := rows.Scan(&count); scanErr == nil {
						return anyToInt(count), nil
					}
				}
			}
		}
	}
	query := fmt.Sprintf("SELECT count() FROM %s %s", tableName, where)
	rows, err := store.Query(r.Context(), query, params...)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	if rows.Next() {
		var count any
		if scanErr := rows.Scan(&count); scanErr == nil {
			return anyToInt(count), nil
		}
	}
	return 0, rows.Err()
}

func isMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no such table") || strings.Contains(msg, "unknown table")
}

func anyToString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}

func anyToInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case int8:
		return int(t)
	case int16:
		return int(t)
	case int32:
		return int(t)
	case int64:
		return int(t)
	case uint:
		return int(t)
	case uint8:
		return int(t)
	case uint16:
		return int(t)
	case uint32:
		return int(t)
	case uint64:
		return int(t)
	case float32:
		return int(t)
	case float64:
		return int(t)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(t))
		return i
	case []byte:
		i, _ := strconv.Atoi(strings.TrimSpace(string(t)))
		return i
	default:
		i, _ := strconv.Atoi(strings.TrimSpace(fmt.Sprintf("%v", t)))
		return i
	}
}
