package web

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
)

const (
	defaultLogsLimit        = 200
	defaultErrorsLimit      = 100
	defaultTracesLimit      = 100
	traceDetailDefaultLimit = 100
	traceDetailMaxLimit     = 1000
	traceDetailHardCap      = 2000
	traceDetailCollapseAt   = 150
)

type traceSpanRow struct {
	Item     map[string]any
	StartMS  float64
	Duration float64
	ParentID string
	SpanID   string
}

func (s *Server) pageLogsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	selectedLevels := normalizeQueryValues(r.URL.Query()["level"], true)
	selectedServices := normalizeQueryValues(r.URL.Query()["service"], false)
	selectedEventNames := normalizeQueryValues(r.URL.Query()["event_name"], false)
	traceIDs, traceID := parseTraceFilterValues(strings.TrimSpace(r.URL.Query().Get("trace_id")), r.URL.Query()["trace_ids"])
	traceIDsCSV := strings.Join(traceIDs, ",")
	eventName := ""
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	sqlWhere := strings.TrimSpace(r.URL.Query().Get("sql"))
	runAdvancedAnalysis := strings.TrimSpace(r.URL.Query().Get("analyze")) == "1"
	statsOpen := strings.TrimSpace(r.URL.Query().Get("stats")) == "1"
	statsUpdated := strings.TrimSpace(r.URL.Query().Get("stats_updated")) == "1"
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

	services, levels, eventNames, err := s.listLogsFilterOptions(r)
	if err != nil {
		s.renderPageError(w, "logs", err)
		return
	}

	includePatterns, excludePatterns, regexErr := prepareRegexFilterPatterns(q)
	where, params, errMsg := buildLogsWhereClause(selectedLevels, selectedServices, selectedEventNames, traceIDs, traceID, fromTS, toTS, sqlWhere)
	if errMsg == "" && regexErr != "" {
		errMsg = regexErr
	}
	queryWhere := where
	queryParams := append([]any{}, params...)
	if errMsg == "" {
		queryWhere, queryParams = appendLogsRegexWhere(queryWhere, queryParams, includePatterns, excludePatterns)
	}
	if errMsg != "" {
		s.renderTemplate(w, "logs.html", renderContext{
			"title":                      "Logs",
			"mobile_breakpoint_max":      "575.98px",
			"request":                    map[string]any{"endpoint": "logs", "args": map[string]any{"stats": r.URL.Query().Get("stats"), "stats_updated": r.URL.Query().Get("stats_updated")}},
			"logs":                       []map[string]any{},
			"total":                      0,
			"limit":                      limit,
			"offset":                     offset,
			"q":                          q,
			"level":                      firstSelected(selectedLevels),
			"selected_levels":            selectedLevels,
			"service":                    firstSelected(selectedServices),
			"selected_services":          selectedServices,
			"trace_id":                   traceID,
			"trace_ids_csv":              traceIDsCSV,
			"trace_ids_count":            len(traceIDs),
			"from_ts":                    fromTS,
			"to_ts":                      toTS,
			"sql_where":                  sqlWhere,
			"services":                   services,
			"levels":                     levels,
			"event_names":                eventNames,
			"event_name":                 eventName,
			"selected_event_names":       selectedEventNames,
			"sort_by":                    sortBy,
			"sort_dir":                   strings.ToLower(sortDir),
			"stats_open":                 statsOpen,
			"stats_updated":              statsUpdated,
			"run_advanced_analysis":      runAdvancedAnalysis,
			"level_stats":                map[string]int{},
			"service_stats":              map[string]int{},
			"tag_stats":                  []map[string]any{},
			"advanced_analysis":          nil,
			"stats_generated_at_iso":     "",
			"stats_generated_at_display": "",
			"stats_generated_age_s":      0,
			"error_msg":                  errMsg,
		})
		return
	}

	rows, total, err := s.queryLogs(r, queryWhere, queryParams, orderClause, limit, offset)
	if err != nil {
		s.renderPageError(w, "logs", err)
		return
	}
	rows, tagStats := s.attachLogTags(r, rows)
	levelStats, serviceStats := s.queryLogStats(r, queryWhere, queryParams)
	statsGeneratedAtISO, statsGeneratedAtDisplay, statsGeneratedAgeS := s.queryLogStatsSnapshot(r, queryWhere, queryParams)
	advancedAnalysis := map[string]any(nil)
	if runAdvancedAnalysis {
		advancedAnalysis = s.queryAdvancedLogAnalysis(r, queryWhere, queryParams, levelStats, serviceStats)
	}

	s.renderTemplate(w, "logs.html", renderContext{
		"title":                      "Logs",
		"mobile_breakpoint_max":      "575.98px",
		"request":                    map[string]any{"endpoint": "logs", "args": map[string]any{"stats": r.URL.Query().Get("stats"), "stats_updated": r.URL.Query().Get("stats_updated")}},
		"logs":                       rows,
		"total":                      total,
		"limit":                      limit,
		"offset":                     offset,
		"q":                          q,
		"level":                      firstSelected(selectedLevels),
		"selected_levels":            selectedLevels,
		"service":                    firstSelected(selectedServices),
		"selected_services":          selectedServices,
		"trace_id":                   traceID,
		"trace_ids_csv":              traceIDsCSV,
		"trace_ids_count":            len(traceIDs),
		"from_ts":                    fromTS,
		"to_ts":                      toTS,
		"sql_where":                  sqlWhere,
		"services":                   services,
		"levels":                     levels,
		"event_names":                eventNames,
		"event_name":                 eventName,
		"selected_event_names":       selectedEventNames,
		"sort_by":                    sortBy,
		"sort_dir":                   strings.ToLower(sortDir),
		"stats_open":                 statsOpen,
		"stats_updated":              statsUpdated,
		"run_advanced_analysis":      runAdvancedAnalysis,
		"level_stats":                levelStats,
		"service_stats":              serviceStats,
		"tag_stats":                  tagStats,
		"advanced_analysis":          advancedAnalysis,
		"stats_generated_at_iso":     statsGeneratedAtISO,
		"stats_generated_at_display": statsGeneratedAtDisplay,
		"stats_generated_age_s":      statsGeneratedAgeS,
		"error_msg":                  "",
	})
}

func (s *Server) pageErrorsHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/errors" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	selectedServices := normalizeQueryValues(r.URL.Query()["service"], false)
	service := firstSelected(selectedServices)
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	groupBy := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("group_by")))
	groupedMode := r.URL.Query().Get("grouped") == "1" || groupBy == "group" || groupBy == "message" || groupBy == "fingerprint" || groupBy == "signature"
	resolved := strings.TrimSpace(r.URL.Query().Get("resolved"))
	if resolved == "" {
		resolved = "0"
	}
	limit := parseLimitParam(r, defaultErrorsLimit, 1, 10000)
	offset := parseOffsetParam(r)

	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortDir := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("sort_dir")))
	if sortDir != "ASC" {
		sortDir = "DESC"
	}
	sortCol := "Timestamp"
	if groupedMode {
		if sortBy == "" {
			sortBy = "count"
		}
		sortCol = "Count"
		switch sortBy {
		case "last_seen", "Timestamp":
			sortCol = "LastSeen"
		case "service", "ServiceName":
			sortCol = "ServiceName"
		case "count":
			sortCol = "Count"
		}
	} else {
		if sortBy == "" {
			sortBy = "Timestamp"
		}
		if sortBy == "service" {
			sortCol = "ServiceName"
		}
	}
	orderClause := fmt.Sprintf("ORDER BY %s %s", sortCol, sortDir)

	services, err := s.listServicesFromLogs(r)
	if err != nil {
		s.renderPageError(w, "errors", err)
		return
	}

	where, params, errMsg := buildErrorsWhereClause(selectedServices, fromTS, toTS, q)
	if errMsg != "" {
		s.renderTemplate(w, "errors.html", renderContext{
			"title":                 "Errors",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "errors"},
			"errors":                []map[string]any{},
			"total":                 0,
			"limit":                 limit,
			"offset":                offset,
			"service":               service,
			"selected_services":     selectedServices,
			"q":                     q,
			"from_ts":               fromTS,
			"to_ts":                 toTS,
			"resolved":              resolved,
			"services":              services,
			"sort_by":               sortBy,
			"sort_dir":              strings.ToLower(sortDir),
			"grouped":               groupedMode,
			"grouped_mode":          groupedMode,
			"group_by":              groupBy,
			"work_item_links":       map[string]any{},
			"error_msg":             errMsg,
		})
		return
	}

	var rows []map[string]any
	var total int
	var queryErr error
	if groupedMode {
		rows, total, queryErr = s.queryErrorsGrouped(r, where, params, resolved, orderClause, limit, offset)
	} else {
		rows, total, queryErr = s.queryErrors(r, where, params, resolved, orderClause, limit, offset)
	}
	if queryErr != nil {
		s.renderPageError(w, "errors", queryErr)
		return
	}

	workItemLinks := s.loadErrorWorkItemLinks(r, rows)

	s.renderTemplate(w, "errors.html", renderContext{
		"title":                 "Errors",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "errors"},
		"errors":                rows,
		"total":                 total,
		"limit":                 limit,
		"offset":                offset,
		"service":               service,
		"selected_services":     selectedServices,
		"q":                     q,
		"from_ts":               fromTS,
		"to_ts":                 toTS,
		"resolved":              resolved,
		"services":              services,
		"sort_by":               sortBy,
		"sort_dir":              strings.ToLower(sortDir),
		"grouped":               groupedMode,
		"grouped_mode":          groupedMode,
		"group_by":              groupBy,
		"work_item_links":       workItemLinks,
		"error_msg":             "",
	})
}

func (s *Server) pageTracesHandler(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/traces" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
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
	traceSpanLimit := parseLimitParamFromQuery(r, "trace_span_limit", traceDetailDefaultLimit, 1, traceDetailMaxLimit)
	traceSpanOffset := parseOffsetParamFromQuery(r, "trace_span_offset")

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

	traceDetail := map[string]any(nil)
	if traceID != "" {
		traceDetail = s.buildTraceDetail(r, traceID, traceSpanLimit, traceSpanOffset)
		rows = []map[string]any{}
		total = anyToInt(traceDetail["total_spans"])
	}

	s.renderTemplate(w, "traces.html", renderContext{
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
		"service":               firstSelected(selectedServices),
		"services":              services,
		"sort_by":               sortBy,
		"sort_dir":              strings.ToLower(sortDir),
		"trace_detail":          traceDetail,
		"work_item_links":       map[string]any{},
		"error_msg":             "",
	})
}

func parseLimitParamFromQuery(r *http.Request, key string, def, min, max int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
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

func parseOffsetParamFromQuery(r *http.Request, key string) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return 0
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func firstSelected(values []string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func buildLogsWhereClause(levels, services, eventNames, traceIDs []string, traceID, fromTS, toTS, sqlWhere string) (string, []any, string) {
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

	if len(eventNames) > 0 {
		marks := []string{}
		for _, eventName := range eventNames {
			eventName = strings.TrimSpace(eventName)
			if eventName != "" {
				marks = append(marks, "?")
				params = append(params, eventName)
			}
		}
		if len(marks) > 0 {
			conditions = append(conditions, "EventName IN ("+strings.Join(marks, ",")+")")
		}
	}

	if len(traceIDs) > 0 {
		marks := make([]string, 0, len(traceIDs))
		for _, item := range traceIDs {
			if trimmed := strings.TrimSpace(item); trimmed != "" {
				marks = append(marks, "?")
				params = append(params, strings.ToLower(trimmed))
			}
		}
		if len(marks) > 0 {
			conditions = append(conditions, "lower(TraceId) IN ("+strings.Join(marks, ",")+")")
		}
	} else if traceID != "" {
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

func (s *Server) listLogsFilterOptions(r *http.Request) ([]string, []string, []string, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, nil, nil, err
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

	eventNames := []string{}
	eventRows, err := store.Query(r.Context(), "SELECT DISTINCT EventName FROM otel_logs WHERE EventName != '' ORDER BY EventName")
	if err == nil {
		defer eventRows.Close()
		for eventRows.Next() {
			var eventName any
			if scanErr := eventRows.Scan(&eventName); scanErr == nil {
				if value := anyToString(eventName); value != "" {
					eventNames = append(eventNames, value)
				}
			}
		}
	}

	return services, levels, eventNames, nil
}

func (s *Server) listServicesFromLogs(r *http.Request) ([]string, error) {
	services, _, _, err := s.listLogsFilterOptions(r)
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

	query := fmt.Sprintf("SELECT toString(Timestamp), SeverityText, ServiceName, Body, TraceId, SpanId FROM otel_logs %s %s LIMIT %d OFFSET %d", where, orderClause, limit, offset)
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
			"tags":     []map[string]any{},
		})
	}
	return result, total, nil
}

func (s *Server) attachLogTags(r *http.Request, rows []map[string]any) ([]map[string]any, []map[string]any) {
	if len(rows) == 0 {
		return rows, []map[string]any{}
	}
	recordIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		recordID := webRecordIDForLog(anyToString(row["ts"]), anyToString(row["service"]), anyToString(row["trace_id"]), anyToString(row["span_id"]))
		row["record_id"] = recordID
		recordIDs = append(recordIDs, recordID)
	}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return rows, []map[string]any{}
	}
	defer store.Close()
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(recordIDs)), ",")
	query := "SELECT RecordId, TagKey, TagValue, IsAuto FROM sobs_record_tags FINAL WHERE RecordType='log' AND RecordId IN (" + placeholders + ") AND IsDeleted=0 ORDER BY RecordId, TagKey"
	tagRows, err := store.Query(r.Context(), query, stringSliceToAny(recordIDs)...)
	if err != nil {
		return rows, []map[string]any{}
	}
	defer tagRows.Close()
	tagsByRecordID := make(map[string][]map[string]any)
	tagStatsCount := make(map[string]int)
	tagStatsMeta := make(map[string]map[string]any)
	for tagRows.Next() {
		var recordID, tagKey, tagValue, isAuto any
		if scanErr := tagRows.Scan(&recordID, &tagKey, &tagValue, &isAuto); scanErr != nil {
			continue
		}
		entry := map[string]any{"key": anyToString(tagKey), "value": anyToString(tagValue), "is_auto": anyToInt(isAuto) != 0}
		rid := anyToString(recordID)
		tagsByRecordID[rid] = append(tagsByRecordID[rid], entry)
		statsKey := entry["key"].(string) + "\x00" + entry["value"].(string)
		tagStatsCount[statsKey]++
		tagStatsMeta[statsKey] = map[string]any{"key": entry["key"], "value": entry["value"]}
	}
	for _, row := range rows {
		if tags, ok := tagsByRecordID[anyToString(row["record_id"])]; ok {
			row["tags"] = tags
		}
	}
	tagStats := make([]map[string]any, 0, len(tagStatsCount))
	for statsKey, count := range tagStatsCount {
		item := cloneMap(tagStatsMeta[statsKey])
		item["count"] = count
		tagStats = append(tagStats, item)
	}
	sort.Slice(tagStats, func(i, j int) bool {
		leftCount := anyToInt(tagStats[i]["count"])
		rightCount := anyToInt(tagStats[j]["count"])
		if leftCount != rightCount {
			return leftCount > rightCount
		}
		leftKey := anyToString(tagStats[i]["key"])
		rightKey := anyToString(tagStats[j]["key"])
		if leftKey != rightKey {
			return leftKey < rightKey
		}
		return anyToString(tagStats[i]["value"]) < anyToString(tagStats[j]["value"])
	})
	return rows, tagStats
}

func (s *Server) queryLogStats(r *http.Request, where string, params []any) (map[string]int, map[string]int) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return map[string]int{}, map[string]int{}
	}
	defer store.Close()
	levelStats := map[string]int{}
	levelRows, err := store.Query(r.Context(), "SELECT SeverityText, count() FROM otel_logs "+where+" GROUP BY SeverityText ORDER BY count() DESC", params...)
	if err == nil {
		defer levelRows.Close()
		for levelRows.Next() {
			var level, count any
			if scanErr := levelRows.Scan(&level, &count); scanErr == nil {
				levelStats[firstNonEmpty(anyToString(level), "UNKNOWN")] = anyToInt(count)
			}
		}
	}
	serviceWhere := where
	serviceParams := append([]any{}, params...)
	if serviceWhere == "" {
		serviceWhere = "WHERE ServiceName != ''"
	} else {
		serviceWhere += " AND ServiceName != ''"
	}
	serviceStats := map[string]int{}
	serviceRows, err := store.Query(r.Context(), "SELECT ServiceName, count() FROM otel_logs "+serviceWhere+" GROUP BY ServiceName ORDER BY count() DESC LIMIT 10", serviceParams...)
	if err == nil {
		defer serviceRows.Close()
		for serviceRows.Next() {
			var service, count any
			if scanErr := serviceRows.Scan(&service, &count); scanErr == nil {
				serviceStats[anyToString(service)] = anyToInt(count)
			}
		}
	}
	return levelStats, serviceStats
}

func (s *Server) queryLogStatsSnapshot(r *http.Request, where string, params []any) (string, string, int) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return "", "", 0
	}
	defer store.Close()
	rows, err := store.Query(r.Context(), "SELECT max(Timestamp) FROM otel_logs "+where, params...)
	if err != nil {
		return "", "", 0
	}
	defer rows.Close()
	generatedAt := time.Now().UTC()
	snapshotRaw := ""
	if rows.Next() {
		var raw any
		if scanErr := rows.Scan(&raw); scanErr == nil {
			snapshotRaw = anyToString(raw)
		}
	}
	if snapshotRaw == "" {
		return "", "", 0
	}
	snapshotAt := parseTimestampTime(snapshotRaw)
	if snapshotAt.IsZero() {
		return snapshotRaw, snapshotRaw, 0
	}
	return snapshotAt.Format(time.RFC3339), snapshotAt.UTC().Format("2006-01-02 15:04:05 UTC"), max(0, int(generatedAt.Sub(snapshotAt).Seconds()))
}

func (s *Server) queryAdvancedLogAnalysis(r *http.Request, where string, params []any, levelStats, serviceStats map[string]int) map[string]any {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return map[string]any{"top_patterns": []any{}, "top_keywords": []any{}, "error_families": []any{}, "hints": []any{}}
	}
	defer store.Close()
	rows, err := store.Query(r.Context(), "SELECT Body, if(mapContains(LogAttributes, 'exception.type'), LogAttributes['exception.type'], '') AS ExceptionType FROM otel_logs "+where, params...)
	if err != nil {
		return map[string]any{"top_patterns": []any{}, "top_keywords": []any{}, "error_families": []any{}, "hints": []any{}}
	}
	defer rows.Close()
	messages := make([]string, 0)
	exceptionTypes := make([]string, 0)
	for rows.Next() {
		var body, exceptionType any
		if scanErr := rows.Scan(&body, &exceptionType); scanErr != nil {
			continue
		}
		message := anyToString(body)
		if message != "" {
			messages = append(messages, message)
		}
		if exc := strings.TrimSpace(anyToString(exceptionType)); exc != "" {
			exceptionTypes = append(exceptionTypes, exc)
		}
	}
	return computeAdvancedLogAnalysis(messages, exceptionTypes, levelStats, serviceStats)
}

func parseTraceFilterValues(traceID string, rawTraceIDs []string) ([]string, string) {
	iterParts := func(value string) []string {
		parts := strings.Split(value, ",")
		out := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				out = append(out, trimmed)
			}
		}
		return out
	}
	parsed := make([]string, 0)
	seen := make(map[string]struct{})
	for _, rawValue := range rawTraceIDs {
		for _, part := range iterParts(rawValue) {
			normalized := strings.ToLower(part)
			if _, ok := seen[normalized]; ok {
				continue
			}
			seen[normalized] = struct{}{}
			parsed = append(parsed, normalized)
		}
	}
	for _, part := range iterParts(traceID) {
		normalized := strings.ToLower(part)
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		parsed = append([]string{normalized}, parsed...)
	}
	primary := ""
	if len(parsed) > 0 {
		primary = parsed[0]
	}
	return parsed, primary
}

func normalizeQueryValues(values []string, uppercase bool) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if uppercase {
			trimmed = strings.ToUpper(trimmed)
		}
		out = append(out, trimmed)
	}
	return out
}

func splitRegexFilterExpressionTerms(expression string) []string {
	parts := make([]string, 0)
	buf := make([]rune, 0, len(expression))
	chars := []rune(expression)
	for i := 0; i < len(chars); i++ {
		if i+1 < len(chars) && chars[i] == '&' && chars[i+1] == '&' {
			backslashes := 0
			for j := i - 1; j >= 0 && chars[j] == '\\'; j-- {
				backslashes++
			}
			if backslashes%2 == 0 {
				parts = append(parts, strings.TrimSpace(string(buf)))
				buf = buf[:0]
				i++
				continue
			}
		}
		buf = append(buf, chars[i])
	}
	parts = append(parts, strings.TrimSpace(string(buf)))
	return parts
}

func prepareRegexFilterPatterns(raw string) ([]string, []string, string) {
	expression := strings.TrimSpace(raw)
	if expression == "" {
		return nil, nil, ""
	}
	parts := splitRegexFilterExpressionTerms(expression)
	if len(parts) == 0 {
		return nil, nil, "Regex error: invalid expression around '&&'"
	}
	includePatterns := make([]string, 0)
	excludePatterns := make([]string, 0)
	for _, part := range parts {
		if part == "" {
			return nil, nil, "Regex error: invalid expression around '&&'"
		}
		negate := strings.HasPrefix(part, "!")
		token := strings.TrimSpace(strings.TrimPrefix(part, "!"))
		token = strings.ReplaceAll(token, `\&&`, `&&`)
		if token == "" {
			return nil, nil, "Regex error: expected a pattern after '!'"
		}
		if _, err := regexp.Compile(token); err != nil {
			return nil, nil, "Regex error: " + err.Error()
		}
		if negate {
			excludePatterns = append(excludePatterns, token)
		} else {
			includePatterns = append(includePatterns, token)
		}
	}
	return includePatterns, excludePatterns, ""
}

func appendLogsRegexWhere(where string, params []any, includePatterns, excludePatterns []string) (string, []any) {
	conditions := make([]string, 0, len(includePatterns)+len(excludePatterns))
	updatedParams := append([]any{}, params...)
	for _, pattern := range includePatterns {
		conditions = append(conditions, "match(Body, ?)")
		updatedParams = append(updatedParams, pattern)
	}
	for _, pattern := range excludePatterns {
		conditions = append(conditions, "NOT match(Body, ?)")
		updatedParams = append(updatedParams, pattern)
	}
	if len(conditions) == 0 {
		return where, updatedParams
	}
	regexSQL := strings.Join(conditions, " AND ")
	if where == "" {
		return "WHERE " + regexSQL, updatedParams
	}
	return where + " AND " + regexSQL, updatedParams
}

func webRecordIDForLog(ts, service, traceID, spanID string) string {
	sum := md5.Sum([]byte(service + "|" + ts + "|" + traceID + "|" + spanID)) //nolint:gosec
	return hex.EncodeToString(sum[:])
}

func stringSliceToAny(values []string) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	return out
}

func parseTimestampTime(raw string) time.Time {
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC()
		}
	}
	return time.Time{}
}

func fingerprintLogMessage(message string) string {
	normalized := strings.ToLower(strings.TrimSpace(message))
	if normalized == "" {
		return "(empty message)"
	}
	replacements := []struct{ pattern, replacement string }{
		{`\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`, "<uuid>"},
		{`\b0x[0-9a-f]+\b`, "<hex>"},
		{`\b[0-9a-f]{16,}\b`, "<hash>"},
		{`\b\d{4,}\b`, "<num>"},
		{`\b\d+\b`, "<n>"},
	}
	for _, item := range replacements {
		normalized = regexp.MustCompile(item.pattern).ReplaceAllString(normalized, item.replacement)
	}
	normalized = regexp.MustCompile(`'[^']*'`).ReplaceAllString(normalized, "'<text>'")
	normalized = regexp.MustCompile(`"[^"]*"`).ReplaceAllString(normalized, `"<text>"`)
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")
	if len(normalized) > 160 {
		return normalized[:160]
	}
	return normalized
}

func computeAdvancedLogAnalysis(messages, exceptionTypes []string, levelStats, serviceStats map[string]int) map[string]any {
	if len(messages) == 0 {
		return map[string]any{"top_patterns": []any{}, "top_keywords": []any{}, "error_families": []any{}, "hints": []any{}}
	}
	fingerprintCounts := map[string]int{}
	for _, message := range messages {
		fingerprintCounts[fingerprintLogMessage(message)]++
	}
	topPatterns := topStringCounts(fingerprintCounts, "pattern", "count", 8)
	familyCounts := map[string]int{}
	for _, exceptionType := range exceptionTypes {
		if trimmed := strings.TrimSpace(exceptionType); trimmed != "" {
			familyCounts[trimmed]++
		}
	}
	familyRegex := regexp.MustCompile(`\b([A-Za-z_][A-Za-z0-9_]*(?:Error|Exception|Timeout|Refused|Unavailable|Failure))\b`)
	for _, message := range messages {
		seen := map[string]struct{}{}
		for _, match := range familyRegex.FindAllStringSubmatch(message, -1) {
			if len(match) < 2 {
				continue
			}
			family := match[1]
			if _, ok := seen[family]; ok {
				continue
			}
			seen[family] = struct{}{}
			familyCounts[family]++
		}
	}
	errorFamilies := topStringCounts(familyCounts, "family", "count", 8)
	stopWords := map[string]struct{}{"the": {}, "and": {}, "for": {}, "with": {}, "from": {}, "into": {}, "this": {}, "that": {}, "http": {}, "https": {}, "failed": {}, "error": {}, "warn": {}, "info": {}, "debug": {}, "trace": {}, "service": {}}
	keywordCounts := map[string]int{}
	tokenRegex := regexp.MustCompile(`[a-z][a-z0-9_\-]{2,}`)
	for _, message := range messages {
		for _, token := range tokenRegex.FindAllString(strings.ToLower(message), -1) {
			if _, skip := stopWords[token]; skip {
				continue
			}
			keywordCounts[token]++
		}
	}
	topKeywords := topStringCounts(keywordCounts, "keyword", "count", 10)
	hints := make([]any, 0)
	total := max(1, len(messages))
	severe := 0
	for level, count := range levelStats {
		switch strings.ToUpper(level) {
		case "ERROR", "FATAL", "CRITICAL", "ALERT", "EMERGENCY":
			severe += count
		}
	}
	severeRatio := float64(severe) / float64(total)
	if severeRatio >= 0.25 {
		hints = append(hints, fmt.Sprintf("High severe-log ratio (%.0f%%); prioritize stabilizing error paths before scaling traffic.", severeRatio*100))
	}
	if len(topPatterns) > 0 {
		topCount := anyToInt(topPatterns[0]["count"])
		if topCount >= 3 {
			hints = append(hints, fmt.Sprintf("Most frequent message pattern repeats %d times; consider deduplication/sampling and shared remediation guidance.", topCount))
		}
	}
	timeoutHits := keywordCounts["timeout"] + keywordCounts["timed"]
	if timeoutHits >= 3 {
		hints = append(hints, "Timeout-related logs are common; review dependency latency, retry budgets, and circuit breakers.")
	}
	if len(serviceStats) > 0 {
		topService := ""
		topServiceCount := 0
		for service, count := range serviceStats {
			if count > topServiceCount {
				topService = service
				topServiceCount = count
			}
		}
		if topService != "" && float64(topServiceCount)/float64(total) >= 0.6 {
			hints = append(hints, fmt.Sprintf("Most events come from %s; investigate service-level hotspots and noisy call paths.", topService))
		}
	}
	return map[string]any{"top_patterns": topPatterns, "top_keywords": topKeywords, "error_families": errorFamilies, "hints": hints}
}

func topStringCounts(counts map[string]int, valueKey, countKey string, limit int) []map[string]any {
	items := make([]map[string]any, 0, len(counts))
	for value, count := range counts {
		items = append(items, map[string]any{valueKey: value, countKey: count})
	}
	sort.Slice(items, func(i, j int) bool {
		leftCount := anyToInt(items[i][countKey])
		rightCount := anyToInt(items[j][countKey])
		if leftCount != rightCount {
			return leftCount > rightCount
		}
		return anyToString(items[i][valueKey]) < anyToString(items[j][valueKey])
	})
	if len(items) > limit {
		items = items[:limit]
	}
	return items
}

func (s *Server) queryErrors(r *http.Request, where string, params []any, resolved string, orderClause string, limit, offset int) ([]map[string]any, int, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, 0, err
	}
	defer store.Close()
	_, _ = store.Exec(r.Context(), "CREATE TABLE IF NOT EXISTS sobs_error_resolutions (ErrorId String, CreatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(CreatedAt) ORDER BY (ErrorId)")

	baseSQL, finalWhere, finalParams := buildErrorsBaseSQL(where, params, resolved)
	totalRows, err := store.Query(r.Context(), "SELECT count() FROM ("+baseSQL+finalWhere+")", finalParams...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, 0, nil
		}
		return nil, 0, err
	}
	defer totalRows.Close()
	total := 0
	if totalRows.Next() {
		var count any
		if scanErr := totalRows.Scan(&count); scanErr == nil {
			total = anyToInt(count)
		}
	}

	query := baseSQL + finalWhere + " " + orderClause + fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	rows, err := store.Query(r.Context(), query, finalParams...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, total, nil
		}
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]any{}
	ids := []string{}
	for rows.Next() {
		var errorID, ts, service, traceID, spanID, body, errType, message any
		if scanErr := rows.Scan(&errorID, &ts, &service, &traceID, &spanID, &body, &errType, &message); scanErr != nil {
			continue
		}
		itemID := anyToString(errorID)
		ids = append(ids, itemID)
		traceIDText := anyToString(traceID)
		result = append(result, map[string]any{
			"id":            itemID,
			"ts":            anyToString(ts),
			"err_type":      anyToString(errType),
			"service":       anyToString(service),
			"message":       anyToString(message),
			"raw_body":      anyToString(body),
			"trace_id":      traceIDText,
			"span_id":       anyToString(spanID),
			"trace_ids_csv": traceIDText,
			"resolved":      resolved == "1",
		})
	}
	applyResolvedState(r.Context(), store, result, ids, resolved)
	return result, total, nil
}

func (s *Server) queryErrorsGrouped(r *http.Request, where string, params []any, resolved string, orderClause string, limit, offset int) ([]map[string]any, int, error) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return nil, 0, err
	}
	defer store.Close()
	_, _ = store.Exec(r.Context(), "CREATE TABLE IF NOT EXISTS sobs_error_resolutions (ErrorId String, CreatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(CreatedAt) ORDER BY (ErrorId)")

	baseSQL, finalWhere, finalParams := buildErrorsBaseSQL(where, params, resolved)
	groupedSQL := "SELECT ServiceName, ErrType, Message, min(Timestamp) AS FirstSeen, max(Timestamp) AS LastSeen, count() AS Count, arrayStringConcat(groupUniqArray(64)(TraceId), ',') AS TraceIdsCsv, min(ErrorId) AS ErrorId, min(Body) AS Body, '' AS TraceId, '' AS SpanId FROM (" + baseSQL + finalWhere + ") GROUP BY ServiceName, ErrType, Message"

	countRows, err := store.Query(r.Context(), "SELECT count() FROM (SELECT ServiceName, ErrType, Message FROM ("+baseSQL+finalWhere+") GROUP BY ServiceName, ErrType, Message)", finalParams...)
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
	rows, err := store.Query(r.Context(), query, finalParams...)
	if err != nil {
		if isMissingTableError(err) {
			return []map[string]any{}, total, nil
		}
		return nil, 0, err
	}
	defer rows.Close()

	result := []map[string]any{}
	ids := []string{}
	for rows.Next() {
		var service, errType, message, firstSeen, lastSeen, count, traceIDsCSV, errorID, body, traceID, spanID any
		if scanErr := rows.Scan(&service, &errType, &message, &firstSeen, &lastSeen, &count, &traceIDsCSV, &errorID, &body, &traceID, &spanID); scanErr != nil {
			continue
		}
		itemID := anyToString(errorID)
		ids = append(ids, itemID)
		result = append(result, map[string]any{
			"id":            itemID,
			"ts":            anyToString(lastSeen),
			"last_seen":     anyToString(lastSeen),
			"first_seen":    anyToString(firstSeen),
			"count":         anyToInt(count),
			"err_type":      anyToString(errType),
			"service":       anyToString(service),
			"message":       anyToString(message),
			"raw_body":      anyToString(body),
			"trace_id":      anyToString(traceID),
			"span_id":       anyToString(spanID),
			"trace_ids_csv": anyToString(traceIDsCSV),
			"resolved":      resolved == "1",
		})
	}
	applyResolvedState(r.Context(), store, result, ids, resolved)
	return result, total, nil
}

func buildErrorsBaseSQL(where string, params []any, resolved string) (string, string, []any) {
	errorSourcesSQL := summaryErrorSourcesSQL()
	errorIDExpr := summaryErrorIDSQLExpr()
	baseSQL := "SELECT " + errorIDExpr + " AS ErrorId, Timestamp, ServiceName, TraceId, SpanId, Body, if(mapContains(LogAttributes, 'exception.type') AND LogAttributes['exception.type'] != '', LogAttributes['exception.type'], 'Error') AS ErrType, if(mapContains(LogAttributes, 'exception.message') AND LogAttributes['exception.message'] != '', LogAttributes['exception.message'], Body) AS Message FROM (" + errorSourcesSQL + ")"
	finalWhere := where
	if resolvedClause := buildResolvedErrorsClause(resolved); resolvedClause != "" {
		finalWhere = appendWhereClause(finalWhere, resolvedClause)
	}
	return baseSQL, finalWhere, append([]any{}, params...)
}

func buildResolvedErrorsClause(resolved string) string {
	errorIDExpr := summaryErrorIDSQLExpr()
	localIDExpr := summaryErrorIDLocalTimeSQLExpr()
	resolvedMatch := "(" + errorIDExpr + " IN (SELECT ErrorId FROM sobs_error_resolutions GROUP BY ErrorId) OR " + localIDExpr + " IN (SELECT ErrorId FROM sobs_error_resolutions GROUP BY ErrorId))"
	switch strings.TrimSpace(resolved) {
	case "1":
		return resolvedMatch
	case "0":
		return "NOT " + resolvedMatch
	default:
		return ""
	}
}

func applyResolvedState(ctx context.Context, store extensionpoints.ClickHouseStore, rows []map[string]any, ids []string, resolved string) {
	if len(rows) == 0 {
		return
	}
	if resolved == "0" || resolved == "1" {
		state := resolved == "1"
		for _, row := range rows {
			row["resolved"] = state
		}
		return
	}
	resolvedIDs := loadResolvedErrorIDs(ctx, store, ids)
	for _, row := range rows {
		row["resolved"] = resolvedIDs[anyToString(row["id"])]
	}
}

func loadResolvedErrorIDs(ctx context.Context, store extensionpoints.ClickHouseStore, ids []string) map[string]bool {
	result := map[string]bool{}
	trimmed := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			trimmed = append(trimmed, id)
		}
	}
	if len(trimmed) == 0 {
		return result
	}
	placeholders := strings.Repeat("?,", len(trimmed))
	placeholders = strings.TrimRight(placeholders, ",")
	args := make([]any, 0, len(trimmed))
	for _, id := range trimmed {
		args = append(args, id)
	}
	rows, err := store.Query(ctx, "SELECT ErrorId FROM sobs_error_resolutions WHERE ErrorId IN ("+placeholders+") GROUP BY ErrorId", args...)
	if err != nil {
		return result
	}
	defer rows.Close()
	for rows.Next() {
		var id any
		if scanErr := rows.Scan(&id); scanErr == nil {
			result[anyToString(id)] = true
		}
	}
	return result
}

func (s *Server) loadErrorWorkItemLinks(r *http.Request, rows []map[string]any) map[string]any {
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		if id := strings.TrimSpace(anyToString(row["id"])); id != "" {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return map[string]any{}
	}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return map[string]any{}
	}
	defer store.Close()
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = strings.TrimRight(placeholders, ",")
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	query := "SELECT AnomalyRuleId, IssueUrl, CanonicalIssueUrl, IssueNumber, IssueState FROM sobs_github_work_items FINAL WHERE IsDeleted = 0 AND IssueUrl != '' AND AnomalyRuleId IN (" + placeholders + ") ORDER BY CreatedAt DESC"
	resultRows, queryErr := store.Query(r.Context(), query, args...)
	if queryErr != nil {
		return map[string]any{}
	}
	defer resultRows.Close()
	links := map[string]any{}
	for resultRows.Next() {
		var refID, issueURL, canonicalURL, issueNumber, issueState any
		if scanErr := resultRows.Scan(&refID, &issueURL, &canonicalURL, &issueNumber, &issueState); scanErr != nil {
			continue
		}
		ref := anyToString(refID)
		if ref == "" {
			continue
		}
		if _, exists := links[ref]; exists {
			continue
		}
		links[ref] = map[string]any{
			"issue_url":    defaultString(anyToString(issueURL), anyToString(canonicalURL)),
			"issue_number": anyToInt(issueNumber),
			"issue_state":  anyToString(issueState),
		}
	}
	return links
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

func (s *Server) buildTraceDetail(r *http.Request, traceID string, pageLimit, pageOffset int) map[string]any {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return emptyTraceDetail(pageLimit, pageOffset)
	}
	defer store.Close()

	countRows, err := store.Query(r.Context(), "SELECT count() FROM otel_traces WHERE TraceId = ?", traceID)
	if err != nil {
		return emptyTraceDetail(pageLimit, pageOffset)
	}
	defer countRows.Close()
	totalSpans := 0
	if countRows.Next() {
		var count any
		if scanErr := countRows.Scan(&count); scanErr == nil {
			totalSpans = anyToInt(count)
		}
	}
	if totalSpans <= 0 {
		return emptyTraceDetail(pageLimit, pageOffset)
	}
	fetchLimit := totalSpans
	hardCapped := false
	if fetchLimit > traceDetailHardCap {
		fetchLimit = traceDetailHardCap
		hardCapped = true
	}

	rows, err := store.Query(r.Context(), "SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes FROM otel_traces WHERE TraceId = ? ORDER BY Timestamp ASC, SpanId ASC LIMIT ?", traceID, fetchLimit)
	withAttrs := err == nil
	if err != nil {
		rows, err = store.Query(r.Context(), "SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode FROM otel_traces WHERE TraceId = ? ORDER BY Timestamp ASC, SpanId ASC LIMIT ?", traceID, fetchLimit)
		withAttrs = false
	}
	if err != nil {
		return emptyTraceDetail(pageLimit, pageOffset)
	}
	defer rows.Close()

	allSpans := []traceSpanRow{}
	minStart := 0.0
	maxEnd := 0.0
	first := true
	for rows.Next() {
		var ts, tid, sid, parentSID, spanName, service, duration, status, attrs any
		if withAttrs {
			if scanErr := rows.Scan(&ts, &tid, &sid, &parentSID, &spanName, &service, &duration, &status, &attrs); scanErr != nil {
				continue
			}
		} else {
			if scanErr := rows.Scan(&ts, &tid, &sid, &parentSID, &spanName, &service, &duration, &status); scanErr != nil {
				continue
			}
			attrs = ""
		}
		attrMap := parseStringMap(anyToString(attrs))
		startMS := parseTimestampMs(anyToString(ts))
		durationMS := float64(anyToInt(duration)) / 1000000.0
		if durationMS < 0 {
			durationMS = 0
		}
		if first {
			minStart = startMS
			maxEnd = startMS + durationMS
			first = false
		} else {
			if startMS < minStart {
				minStart = startMS
			}
			if endMS := startMS + durationMS; endMS > maxEnd {
				maxEnd = endMS
			}
		}
		item := map[string]any{
			"ts":             anyToString(ts),
			"trace_id":       anyToString(tid),
			"span_id":        anyToString(sid),
			"parent_span_id": anyToString(parentSID),
			"name":           anyToString(spanName),
			"service":        anyToString(service),
			"duration_ms":    roundFloat(durationMS, 2),
			"status":         normalizeTraceStatus(anyToString(status)),
			"http_method":    firstNonEmpty(attrMap["http.method"], attrMap["http.request.method"]),
			"http_url":       firstNonEmpty(attrMap["http.url"], attrMap["url.full"]),
			"http_status":    firstNonEmpty(attrMap["http.status_code"], attrMap["http.response.status_code"]),
		}
		allSpans = append(allSpans, traceSpanRow{Item: item, StartMS: startMS, Duration: durationMS, ParentID: anyToString(parentSID), SpanID: anyToString(sid)})
	}
	if len(allSpans) == 0 {
		return emptyTraceDetail(pageLimit, pageOffset)
	}

	totalMS := maxFloat(maxEnd-minStart, 1)
	activeMS := computeMergedSpanCoverage(allSpans)
	coveragePct := roundFloat((activeMS/totalMS)*100.0, 2)
	spanSumMS := 0.0
	childrenByParent := map[string][]string{}
	indexByID := map[string]int{}
	for i, span := range allSpans {
		indexByID[span.SpanID] = i
		childrenByParent[span.ParentID] = append(childrenByParent[span.ParentID], span.SpanID)
		spanSumMS += span.Duration
	}
	for i := range allSpans {
		allSpans[i].Item["has_children"] = len(childrenByParent[allSpans[i].SpanID]) > 0
		allSpans[i].Item["offset_pct"] = roundFloat(((allSpans[i].StartMS-minStart)/totalMS)*100.0, 2)
		allSpans[i].Item["width_pct"] = roundFloat(maxFloat(0.5, (allSpans[i].Duration/totalMS)*100.0), 2)
	}

	orderedIDs := flattenTraceTree(allSpans, childrenByParent, indexByID)
	if pageOffset >= len(orderedIDs) && len(orderedIDs) > 0 {
		pageOffset = maxInt(0, ((len(orderedIDs)-1)/pageLimit)*pageLimit)
	}
	pageEnd := minInt(len(orderedIDs), pageOffset+pageLimit)
	spanTree := make([]map[string]any, 0, maxInt(0, pageEnd-pageOffset))
	for _, spanID := range orderedIDs[pageOffset:pageEnd] {
		idx := indexByID[spanID]
		item := cloneMap(allSpans[idx].Item)
		item["depth"] = traceDepth(idx, allSpans, indexByID)
		spanTree = append(spanTree, item)
	}

	logCounts := map[string]int{}
	logRows, logErr := store.Query(r.Context(), "SELECT SpanId, count() AS cnt FROM otel_logs WHERE TraceId = ? AND SpanId != '' GROUP BY SpanId", traceID)
	if logErr == nil {
		defer logRows.Close()
		for logRows.Next() {
			var spanID, cnt any
			if scanErr := logRows.Scan(&spanID, &cnt); scanErr == nil {
				logCounts[anyToString(spanID)] = anyToInt(cnt)
			}
		}
	}

	return map[string]any{
		"span_tree":           spanTree,
		"trace_start_ts":      anyToString(allSpans[0].Item["ts"]),
		"trace_end_ts":        anyToString(allSpans[len(allSpans)-1].Item["ts"]),
		"trace_start_ms":      roundFloat(minStart, 0),
		"trace_end_ms":        roundFloat(maxEnd, 0),
		"errors":              []map[string]any{},
		"errors_truncated":    false,
		"error_span_ids":      []string{},
		"log_counts":          logCounts,
		"anomaly_state":       "",
		"total_ms":            roundFloat(totalMS, 2),
		"active_ms":           roundFloat(activeMS, 2),
		"coverage_pct":        coveragePct,
		"span_sum_ms":         roundFloat(spanSumMS, 2),
		"timeline_segments":   buildTraceTimelineSegments(allSpans, minStart, maxEnd),
		"has_potential_gap":   false,
		"raw_windows":         []map[string]any{},
		"raw_window_segments": []map[string]any{},
		"metrics_context": map[string]any{
			"source_mode":      "none",
			"total_points":     0,
			"series":           []any{},
			"match_mode":       "none",
			"match_label":      "no match",
			"match_dimensions": []any{},
			"health_chips":     []any{},
			"metric_groups":    []any{},
			"timeseries":       map[string]any{"ticks_ms": []any{}, "by_metric": map[string]any{}},
		},
		"total_spans":        totalSpans,
		"capped_total_spans": len(orderedIDs),
		"hard_cap":           traceDetailHardCap,
		"hard_capped":        hardCapped,
		"default_collapsed":  len(orderedIDs) > traceDetailCollapseAt,
		"page_limit":         pageLimit,
		"page_offset":        pageOffset,
		"page_end":           pageEnd,
		"context_rows":       0,
		"prev_offset":        maxInt(0, pageOffset-pageLimit),
		"next_offset":        pageOffset + pageLimit,
		"has_prev_page":      pageOffset > 0,
		"has_next_page":      pageEnd < len(orderedIDs),
	}
}

func emptyTraceDetail(pageLimit, pageOffset int) map[string]any {
	return map[string]any{
		"span_tree":           []map[string]any{},
		"trace_start_ts":      "",
		"trace_end_ts":        "",
		"trace_start_ms":      0,
		"trace_end_ms":        0,
		"errors":              []map[string]any{},
		"errors_truncated":    false,
		"error_span_ids":      []string{},
		"log_counts":          map[string]int{},
		"anomaly_state":       "",
		"total_ms":            0,
		"active_ms":           0,
		"coverage_pct":        0,
		"span_sum_ms":         0,
		"timeline_segments":   []map[string]any{},
		"has_potential_gap":   false,
		"raw_windows":         []map[string]any{},
		"raw_window_segments": []map[string]any{},
		"metrics_context": map[string]any{
			"source_mode":      "none",
			"total_points":     0,
			"series":           []any{},
			"match_mode":       "none",
			"match_label":      "no match",
			"match_dimensions": []any{},
			"health_chips":     []any{},
			"metric_groups":    []any{},
			"timeseries":       map[string]any{"ticks_ms": []any{}, "by_metric": map[string]any{}},
		},
		"total_spans":        0,
		"capped_total_spans": 0,
		"hard_cap":           traceDetailHardCap,
		"hard_capped":        false,
		"default_collapsed":  false,
		"page_limit":         pageLimit,
		"page_offset":        pageOffset,
		"page_end":           0,
		"context_rows":       0,
		"prev_offset":        0,
		"next_offset":        pageOffset + pageLimit,
		"has_prev_page":      false,
		"has_next_page":      false,
	}
}

func flattenTraceTree(spans []traceSpanRow, childrenByParent map[string][]string, indexByID map[string]int) []string {
	rootIDs := []string{}
	for _, span := range spans {
		if span.ParentID == "" || !containsSpanID(indexByID, span.ParentID) {
			rootIDs = append(rootIDs, span.SpanID)
		}
	}
	ordered := []string{}
	seen := map[string]bool{}
	var walk func(string)
	walk = func(spanID string) {
		if spanID == "" || seen[spanID] {
			return
		}
		seen[spanID] = true
		ordered = append(ordered, spanID)
		children := append([]string{}, childrenByParent[spanID]...)
		sort.SliceStable(children, func(i, j int) bool {
			left := spans[indexByID[children[i]]]
			right := spans[indexByID[children[j]]]
			if left.StartMS == right.StartMS {
				return left.SpanID < right.SpanID
			}
			return left.StartMS < right.StartMS
		})
		for _, childID := range children {
			walk(childID)
		}
	}
	for _, rootID := range rootIDs {
		walk(rootID)
	}
	for _, span := range spans {
		walk(span.SpanID)
	}
	return ordered
}

func traceDepth(idx int, spans []traceSpanRow, indexByID map[string]int) int {
	depth := 0
	parentID := spans[idx].ParentID
	for parentID != "" {
		parentIdx, ok := indexByID[parentID]
		if !ok {
			break
		}
		depth++
		parentID = spans[parentIdx].ParentID
	}
	return depth
}

func containsSpanID(indexByID map[string]int, spanID string) bool {
	_, ok := indexByID[spanID]
	return ok
}

func buildTraceTimelineSegments(spans []traceSpanRow, minStart, maxEnd float64) []map[string]any {
	total := maxFloat(maxEnd-minStart, 1)
	segments := make([]map[string]any, 0, len(spans))
	for _, span := range spans {
		segments = append(segments, map[string]any{
			"kind":      "active",
			"potential": false,
			"start_pct": roundFloat(((span.StartMS-minStart)/total)*100.0, 2),
			"width_pct": roundFloat(maxFloat(0.5, (span.Duration/total)*100.0), 2),
		})
	}
	if len(segments) == 0 {
		segments = append(segments, map[string]any{"kind": "gap", "potential": false, "start_pct": 0.0, "width_pct": 100.0})
	}
	return segments
}

func computeMergedSpanCoverage(spans []traceSpanRow) float64 {
	if len(spans) == 0 {
		return 0
	}
	type interval struct{ start, end float64 }
	intervals := make([]interval, 0, len(spans))
	for _, span := range spans {
		intervals = append(intervals, interval{start: span.StartMS, end: span.StartMS + maxFloat(span.Duration, 0)})
	}
	sort.Slice(intervals, func(i, j int) bool { return intervals[i].start < intervals[j].start })
	mergedStart := intervals[0].start
	mergedEnd := intervals[0].end
	total := 0.0
	for _, iv := range intervals[1:] {
		if iv.start <= mergedEnd {
			if iv.end > mergedEnd {
				mergedEnd = iv.end
			}
			continue
		}
		total += maxFloat(0, mergedEnd-mergedStart)
		mergedStart = iv.start
		mergedEnd = iv.end
	}
	total += maxFloat(0, mergedEnd-mergedStart)
	return total
}

func parseTimestampMs(raw string) float64 {
	if raw == "" {
		return 0
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return float64(parsed.UnixMilli())
		}
	}
	return 0
}

func normalizeTraceStatus(raw string) string {
	trimmed := strings.TrimSpace(strings.ToUpper(raw))
	if trimmed == "" || trimmed == "0" {
		return "UNSET"
	}
	if trimmed == "1" {
		return "OK"
	}
	if trimmed == "2" {
		return "ERROR"
	}
	return trimmed
}

func roundFloat(value float64, decimals int) float64 {
	pow := 1.0
	for i := 0; i < decimals; i++ {
		pow *= 10
	}
	if value >= 0 {
		return float64(int(value*pow+0.5)) / pow
	}
	return float64(int(value*pow-0.5)) / pow
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func cloneMap(input map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func (s *Server) renderPageError(w http.ResponseWriter, pageName string, err error) {
	writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error(), "page": pageName})
}

func (s *Server) renderPageWithError(w http.ResponseWriter, templateName, pageName, errMsg string, services, levels []string) {
	s.renderTemplate(w, templateName, renderContext{
		"title":                 strings.Title(pageName),
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": pageName},
		"services":              services,
		"levels":                levels,
		"error_msg":             errMsg,
	})
}

func (s *Server) renderTemplate(w http.ResponseWriter, templateName string, ctx renderContext) {
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
