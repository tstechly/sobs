package web

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type queryRequest struct {
	Question           string `json:"question"`
	SQL                string `json:"sql"`
	Execute            *bool  `json:"execute"`
	Chart              bool   `json:"chart"`
	Stream             bool   `json:"stream"`
	PreferredChartType string `json:"preferred_chart_type"`
	ChartInstruction   string `json:"chart_instruction"`
	ThinkingLevel      string `json:"thinking_level"`
}

type refineChartRequest struct {
	Prompt string `json:"prompt"`
	Spec   any    `json:"spec"`
}

type addToDashboardRequest struct {
	DashboardID string         `json:"dashboard_id"`
	Title       string         `json:"title"`
	Type        string         `json:"type"`
	Spec        map[string]any `json:"spec"`
}

var queryAllowedTablesBuiltin = map[string]struct{}{
	"otel_logs":                     {},
	"otel_traces":                   {},
	"hyperdx_sessions":              {},
	"otel_metrics_gauge":            {},
	"otel_metrics_gauge_pinned":     {},
	"otel_metrics_sum":              {},
	"otel_metrics_sum_pinned":       {},
	"otel_metrics_histogram":        {},
	"otel_metrics_histogram_pinned": {},
	"sobs_anomaly_rules":            {},
	"sobs_raw_windows":              {},
	"otel_metrics_1m_agg":           {},
	"v_derived_signals_1m":          {},
	"v_otel_metrics_1m":             {},
	"v_otel_metrics_signal_context": {},
	"v_otel_metrics_anomaly":        {},
	"v_otel_metrics_dedup":          {},
	"v_derived_signals_anomaly":     {},
}

var querySafeIdentifier = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var queryUnsafeSQLPatterns = regexp.MustCompile(`\b(insert|update|delete|drop|truncate|alter|create|replace|rename|attach|detach|grant|revoke|system\s+stop|system\s+start|system\s+reload|kill|optimize|exchange|backup|restore)\b`)
var queryTableRefRegex = regexp.MustCompile(`\b(?:FROM|JOIN)\s+((?:[a-zA-Z_]\w*\.)*[a-zA-Z_]\w*)`)
var queryCTEAliasRegex = regexp.MustCompile(`(?:\bWITH\s+(?:RECURSIVE\s+)?|,\s*)([a-zA-Z_]\w*)\s+AS\s*\(`)
var queryArrayJoinRegex = regexp.MustCompile(`\bARRAY\s+JOIN\s+((?:[a-zA-Z_]\w*\.)*[a-zA-Z_]\w*)`)
var queryFencePrefixRegex = regexp.MustCompile("^```[a-zA-Z]*\\n?")
var queryFenceSuffixRegex = regexp.MustCompile("\\n?```$")
var querySensitiveKeys = map[string]struct{}{
	"password": {}, "passwd": {}, "pwd": {}, "secret": {},
	"client_secret": {}, "api_key": {}, "api_secret": {}, "apikey": {},
	"token": {}, "access_token": {}, "refresh_token": {}, "id_token": {},
	"auth_token": {}, "bearer_token": {}, "authorization": {}, "x_authorization": {}, "x_api_key": {},
	"private_key": {}, "private_key_pem": {}, "private-key": {},
	"credit_card": {}, "card_number": {}, "cvv": {}, "cvc": {},
	"ssn": {}, "social_security_number": {},
	"s3_secret_access_key": {}, "backup_encryption_password": {}, "smtp_password": {},
}
var querySQLOutputMaskFields = map[string]struct{}{
	"sql": {}, "query": {}, "sample_sql": {}, "override_sql": {},
}

func normalizeQuerySensitiveKey(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	normalized = strings.ReplaceAll(normalized, " ", "_")
	return normalized
}

func (s *Server) queryMaskingFlags() (bool, bool) {
	rules := s.maskingService.ListRules()
	outputMode := strings.TrimSpace(anyString(rules["output_mode"]))
	if outputMode == "" {
		outputMode = "mask"
	}
	sqlOutputMode := strings.TrimSpace(anyString(rules["sql_output"]))
	if sqlOutputMode == "" {
		sqlOutputMode = "masked"
	}
	return isMaskingOutputEnabled(outputMode), isMaskingOutputEnabled(sqlOutputMode)
}

func (s *Server) maskQueryPayload(payload any) any {
	outputEnabled, sqlOutputEnabled := s.queryMaskingFlags()
	if !outputEnabled {
		return payload
	}
	blob, err := json.Marshal(payload)
	if err != nil {
		return payload
	}
	var generic any
	if err := json.Unmarshal(blob, &generic); err != nil {
		return payload
	}
	return s.maskQueryPayloadValue(generic, "", sqlOutputEnabled)
}

func (s *Server) maskQueryPayloadValue(value any, key string, sqlOutputEnabled bool) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for k, item := range typed {
			normalized := normalizeQuerySensitiveKey(k)
			if _, sensitive := querySensitiveKeys[normalized]; sensitive {
				out[k] = "****"
				continue
			}
			if _, sqlField := querySQLOutputMaskFields[normalized]; sqlField {
				if text, ok := item.(string); ok && !sqlOutputEnabled {
					out[k] = text
					continue
				}
			}
			out[k] = s.maskQueryPayloadValue(item, k, sqlOutputEnabled)
		}
		return out
	case []any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, s.maskQueryPayloadValue(item, key, sqlOutputEnabled))
		}
		return out
	case string:
		if _, sqlField := querySQLOutputMaskFields[normalizeQuerySensitiveKey(key)]; sqlField && !sqlOutputEnabled {
			return typed
		}
		return anyString(s.maskingService.Preview(typed)["output"])
	default:
		return value
	}
}

func (s *Server) apiQueryAsk(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	cfg := s.queryAIConfig()
	if !cfg.queryEnabled() {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Query page is unavailable."})
		return
	}
	q := strings.TrimSpace(req.Question)
	if q == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "question is required"})
		return
	}
	preferredChartType := strings.TrimSpace(req.PreferredChartType)
	chartInstruction := strings.TrimSpace(req.ChartInstruction)
	thinkingLevel := normalizeThinkingLevel(firstNonEmpty(req.ThinkingLevel, cfg.ThinkingLevel))
	streamRequested := req.Stream || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")
	traceID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("query|%s|%d", q, time.Now().UnixNano()))))
	turnID := traceID[:16]
	llmStats := map[string]any{}
	if streamRequested {
		s.apiQueryAskStream(w, r, req, cfg, q, preferredChartType, chartInstruction, thinkingLevel, traceID, turnID)
		return
	}

	if allowed, reason, _ := checkGuardModelWithLLM(r.Context(), cfg, q, "/query"); !allowed {
		writeJSON(w, http.StatusForbidden, map[string]any{
			"ok":        false,
			"error":     "Request blocked by safety guard: " + reason,
			"trace_id":  traceID,
			"turn_id":   turnID,
			"sql":       "",
			"columns":   []any{},
			"rows":      []any{},
			"llm_stats": summarizeQueryLLMStats(map[string]map[string]int{}),
		})
		return
	}

	schemaStarted := time.Now()
	tableNames := s.listTableNames(r.Context())
	schemaElapsed := int(time.Since(schemaStarted).Milliseconds())
	schemaContext := buildSchemaContextForTables(s, r.Context(), tableNames)

	suggested, genStats, genErr := generateSQLWithLLM(r.Context(), cfg, q, schemaContext, preferredChartType, chartInstruction, thinkingLevel)
	if genErr != "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"ok":       false,
			"error":    genErr,
			"trace_id": traceID,
			"turn_id":  turnID,
			"sql":      "",
			"columns":  []any{},
			"rows":     []any{},
			"llm_stats": summarizeQueryLLMStats(map[string]map[string]int{
				"sql_generation": genStats,
			}),
		})
		return
	}
	stages := map[string]map[string]int{
		"sql_generation": genStats,
		"schema_build": {
			"prompt_tokens":     maxInt(1, len([]rune(schemaContext))/8),
			"completion_tokens": 0,
			"thinking_tokens":   0,
			"elapsed_ms":        schemaElapsed,
		},
	}
	doExecute := true
	if req.Execute != nil {
		doExecute = *req.Execute
	}
	columns := []string{}
	rows := [][]any{}
	fieldTypes := []map[string]any{}
	datasets := []map[string]any{}
	retryCount := 0
	chartSpec := ""
	execErr := ""
	if doExecute {
		execStarted := time.Now()
		finalSQL, finalCols, finalRows, finalErr, retries, repairStats := s.validateAndExecuteWithRepair(r.Context(), q, schemaContext, suggested, tableNames, cfg, thinkingLevel)
		retryCount = retries
		suggested = finalSQL
		execElapsed := int(time.Since(execStarted).Milliseconds())
		stages["sql_execute"] = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": execElapsed}
		if len(repairStats) > 0 {
			stages["sql_repair"] = repairStats
		}
		execErr = finalErr
		if execErr == "" {
			columns = finalCols
			rows = finalRows
			fieldTypes = inferFieldTypes(columns, rows)
			datasets = append(datasets, map[string]any{
				"name":        "main",
				"purpose":     "primary dataset",
				"sql":         suggested,
				"columns":     columns,
				"field_types": fieldTypes,
				"rows":        rows,
				"error":       "",
			})
		}
		if req.Chart && execErr == "" && len(columns) > 0 {
			namedStarted := time.Now()
			namedQueries, namedStats, _ := generateNamedQueriesWithLLM(r.Context(), cfg, q, schemaContext, suggested, preferredChartType, chartInstruction, thinkingLevel)
			if len(namedStats) == 0 {
				namedElapsed := int(time.Since(namedStarted).Milliseconds())
				namedStats = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": namedElapsed}
			}
			stages["named_query_generation"] = namedStats
			for _, nq := range namedQueries {
				nqCols, nqRows, nqErr := s.runSQL(r.Context(), nq.SQL)
				datasets = append(datasets, map[string]any{
					"name":        nq.Name,
					"purpose":     nq.Purpose,
					"sql":         nq.SQL,
					"columns":     nqCols,
					"field_types": inferFieldTypes(nqCols, nqRows),
					"rows":        nqRows,
					"error":       errorString(nqErr),
				})
			}
			sample := make([]map[string]any, 0, minInt(20, len(rows)))
			for _, row := range rows {
				if len(sample) >= 20 {
					break
				}
				record := map[string]any{}
				for i, col := range columns {
					if i < len(row) {
						record[col] = row[i]
					}
				}
				sample = append(sample, record)
			}
			var chartErr string
			chartStats := map[string]int{}
			chartSpec, chartStats, chartErr = generateChartSpecWithLLM(r.Context(), cfg, columns, sample, q, preferredChartType, chartInstruction, datasets, thinkingLevel)
			stages["chart_generation"] = chartStats
			if chartErr != "" {
				execErr = firstNonEmpty(execErr, chartErr)
			}
		}
		if len(datasets) == 0 && execErr == "" {
			datasets = append(datasets, map[string]any{
				"name":        "main",
				"purpose":     "primary dataset",
				"sql":         suggested,
				"columns":     columns,
				"field_types": fieldTypes,
				"rows":        rows,
				"error":       "",
			})
		}
	} else {
		if !isReadOnlySQL(suggested) {
			execErr = "generated SQL is not read-only"
		}
	}
	llmStats = summarizeQueryLLMStats(stages)
	writeJSON(w, http.StatusOK, s.maskQueryPayload(map[string]any{
		"ok":          true,
		"trace_id":    traceID,
		"turn_id":     turnID,
		"sql":         suggested,
		"question":    q,
		"columns":     columns,
		"rows":        rows,
		"field_types": fieldTypes,
		"datasets":    datasets,
		"retry_count": retryCount,
		"chart_spec":  chartSpec,
		"error":       execErr,
		"llm_stats":   llmStats,
	}))
}

func (s *Server) apiQueryRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req queryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	cfg := s.queryAIConfig()
	if !cfg.queryEnabled() {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Query page is unavailable."})
		return
	}
	sqlText := strings.TrimSpace(req.SQL)
	question := strings.TrimSpace(req.Question)
	preferredChartType := strings.TrimSpace(req.PreferredChartType)
	chartInstruction := strings.TrimSpace(req.ChartInstruction)
	thinkingLevel := normalizeThinkingLevel(firstNonEmpty(req.ThinkingLevel, cfg.ThinkingLevel))
	streamRequested := req.Stream || strings.Contains(strings.ToLower(r.Header.Get("Accept")), "text/event-stream")
	if sqlText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sql is required"})
		return
	}
	traceID := fmt.Sprintf("%x", md5.Sum([]byte(fmt.Sprintf("query-run|%s|%d", sqlText, time.Now().UnixNano()))))
	turnID := traceID[:16]
	if streamRequested {
		s.apiQueryRunStream(w, r, req, cfg, sqlText, question, preferredChartType, chartInstruction, thinkingLevel, traceID, turnID)
		return
	}
	if !isReadOnlySQL(sqlText) {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "only read-only SQL is allowed", "trace_id": traceID, "turn_id": turnID})
		return
	}
	if allowed, reason, _ := checkGuardModelWithLLM(r.Context(), cfg, firstNonEmpty(question, sqlText), "/query"); !allowed {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Request blocked by safety guard: " + reason, "trace_id": traceID, "turn_id": turnID})
		return
	}
	stages := map[string]map[string]int{}
	explainStarted := time.Now()
	_, _, explainErr := s.runSQL(r.Context(), "EXPLAIN "+sqlText)
	stages["sql_explain"] = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": int(time.Since(explainStarted).Milliseconds())}
	if explainErr != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": explainErr.Error(), "trace_id": traceID, "turn_id": turnID, "sql": sqlText, "columns": []any{}, "rows": []any{}})
		return
	}
	execStarted := time.Now()
	columns, rows, err := s.runSQL(r.Context(), sqlText)
	stages["sql_execute"] = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": int(time.Since(execStarted).Milliseconds())}
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error(), "trace_id": traceID, "turn_id": turnID, "sql": sqlText, "columns": []any{}, "rows": []any{}})
		return
	}
	fieldTypes := inferFieldTypes(columns, rows)
	datasets := []map[string]any{{"name": "main", "purpose": "primary dataset", "sql": sqlText, "columns": columns, "field_types": fieldTypes, "rows": rows, "error": ""}}
	chartSpec := ""
	chartErr := ""
	if req.Chart && len(columns) > 0 {
		schemaContext := buildSchemaContextForTables(s, r.Context(), s.listTableNames(r.Context()))
		namedStarted := time.Now()
		namedQueries, namedStats, _ := generateNamedQueriesWithLLM(r.Context(), cfg, firstNonEmpty(question, sqlText), schemaContext, sqlText, preferredChartType, chartInstruction, thinkingLevel)
		if len(namedStats) == 0 {
			namedStats = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": int(time.Since(namedStarted).Milliseconds())}
		}
		stages["named_query_generation"] = namedStats
		for _, nq := range namedQueries {
			nqCols, nqRows, nqErr := s.runSQL(r.Context(), nq.SQL)
			datasets = append(datasets, map[string]any{
				"name":        nq.Name,
				"purpose":     nq.Purpose,
				"sql":         nq.SQL,
				"columns":     nqCols,
				"field_types": inferFieldTypes(nqCols, nqRows),
				"rows":        nqRows,
				"error":       errorString(nqErr),
			})
		}
		sample := make([]map[string]any, 0, minInt(20, len(rows)))
		for _, row := range rows {
			if len(sample) >= 20 {
				break
			}
			record := map[string]any{}
			for i, col := range columns {
				if i < len(row) {
					record[col] = row[i]
				}
			}
			sample = append(sample, record)
		}
		var llmChartErr string
		chartStats := map[string]int{}
		chartSpec, chartStats, llmChartErr = generateChartSpecWithLLM(r.Context(), cfg, columns, sample, firstNonEmpty(question, sqlText), preferredChartType, chartInstruction, datasets, thinkingLevel)
		stages["chart_generation"] = chartStats
		if llmChartErr != "" {
			chartErr = llmChartErr
		}
	}
	writeJSON(w, http.StatusOK, s.maskQueryPayload(map[string]any{
		"ok":          true,
		"trace_id":    traceID,
		"turn_id":     turnID,
		"sql":         sqlText,
		"columns":     columns,
		"rows":        rows,
		"field_types": fieldTypes,
		"datasets":    datasets,
		"retry_count": 0,
		"chart_spec":  chartSpec,
		"error":       chartErr,
		"llm_stats":   summarizeQueryLLMStats(stages),
	}))
}

func (s *Server) apiQueryRunStream(
	w http.ResponseWriter,
	r *http.Request,
	req queryRequest,
	cfg queryAIConfig,
	sqlText string,
	question string,
	preferredChartType string,
	chartInstruction string,
	thinkingLevel string,
	traceID string,
	turnID string,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "streaming unsupported"})
		return
	}
	writeEvent := func(event string, payload map[string]any) {
		blob, _ := json.Marshal(s.maskQueryPayload(payload))
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(blob))
		flusher.Flush()
	}

	writeEvent("start", map[string]any{"trace_id": traceID, "turn_id": turnID, "ok": true})
	if !isReadOnlySQL(sqlText) {
		writeEvent("error", map[string]any{"ok": false, "error": "only read-only SQL is allowed", "trace_id": traceID, "turn_id": turnID})
		writeEvent("done", map[string]any{"ok": false})
		return
	}
	if allowed, reason, _ := checkGuardModelWithLLM(r.Context(), cfg, firstNonEmpty(question, sqlText), "/query"); !allowed {
		writeEvent("error", map[string]any{"ok": false, "error": "Request blocked by safety guard: " + reason, "trace_id": traceID, "turn_id": turnID})
		writeEvent("done", map[string]any{"ok": false})
		return
	}
	writeEvent("guard", map[string]any{"ok": true, "decision": "allowed"})

	stages := map[string]map[string]int{}
	writeEvent("stage", map[string]any{"name": "sql_explain", "ok": true})
	explainStarted := time.Now()
	_, _, explainErr := s.runSQL(r.Context(), "EXPLAIN "+sqlText)
	stages["sql_explain"] = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": int(time.Since(explainStarted).Milliseconds())}
	if explainErr != nil {
		writeEvent("error", map[string]any{"ok": false, "error": explainErr.Error(), "trace_id": traceID, "turn_id": turnID, "sql": sqlText})
		writeEvent("done", map[string]any{"ok": false})
		return
	}

	writeEvent("stage", map[string]any{"name": "sql_execute", "ok": true})
	execStarted := time.Now()
	columns, rows, err := s.runSQL(r.Context(), sqlText)
	stages["sql_execute"] = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": int(time.Since(execStarted).Milliseconds())}
	if err != nil {
		writeEvent("error", map[string]any{"ok": false, "error": err.Error(), "trace_id": traceID, "turn_id": turnID, "sql": sqlText})
		writeEvent("done", map[string]any{"ok": false})
		return
	}

	fieldTypes := inferFieldTypes(columns, rows)
	datasets := []map[string]any{{"name": "main", "purpose": "primary dataset", "sql": sqlText, "columns": columns, "field_types": fieldTypes, "rows": rows, "error": ""}}
	chartSpec := ""
	chartErr := ""
	if req.Chart && len(columns) > 0 {
		schemaContext := buildSchemaContextForTables(s, r.Context(), s.listTableNames(r.Context()))
		writeEvent("stage", map[string]any{"name": "named_query_generation", "ok": true})
		namedStarted := time.Now()
		namedQueries, namedStats, _ := generateNamedQueriesWithLLM(r.Context(), cfg, firstNonEmpty(question, sqlText), schemaContext, sqlText, preferredChartType, chartInstruction, thinkingLevel)
		if len(namedStats) == 0 {
			namedStats = map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": int(time.Since(namedStarted).Milliseconds())}
		}
		stages["named_query_generation"] = namedStats
		for _, nq := range namedQueries {
			nqCols, nqRows, nqErr := s.runSQL(r.Context(), nq.SQL)
			datasets = append(datasets, map[string]any{
				"name":        nq.Name,
				"purpose":     nq.Purpose,
				"sql":         nq.SQL,
				"columns":     nqCols,
				"field_types": inferFieldTypes(nqCols, nqRows),
				"rows":        nqRows,
				"error":       errorString(nqErr),
			})
		}
		sample := make([]map[string]any, 0, minInt(20, len(rows)))
		for _, row := range rows {
			if len(sample) >= 20 {
				break
			}
			record := map[string]any{}
			for i, col := range columns {
				if i < len(row) {
					record[col] = row[i]
				}
			}
			sample = append(sample, record)
		}
		writeEvent("stage", map[string]any{"name": "chart_generation", "ok": true})
		var llmChartErr string
		chartStats := map[string]int{}
		chartSpec, chartStats, llmChartErr = generateChartSpecWithLLM(r.Context(), cfg, columns, sample, firstNonEmpty(question, sqlText), preferredChartType, chartInstruction, datasets, thinkingLevel)
		stages["chart_generation"] = chartStats
		if llmChartErr != "" {
			chartErr = llmChartErr
		}
	}

	result := map[string]any{
		"ok":          true,
		"trace_id":    traceID,
		"turn_id":     turnID,
		"sql":         sqlText,
		"columns":     columns,
		"rows":        rows,
		"field_types": fieldTypes,
		"datasets":    datasets,
		"retry_count": 0,
		"chart_spec":  chartSpec,
		"error":       chartErr,
		"llm_stats":   summarizeQueryLLMStats(stages),
	}
	writeEvent("result", result)
	writeEvent("done", map[string]any{"ok": true})
}

func (s *Server) apiQueryAskStream(
	w http.ResponseWriter,
	r *http.Request,
	req queryRequest,
	cfg queryAIConfig,
	question string,
	preferredChartType string,
	chartInstruction string,
	thinkingLevel string,
	traceID string,
	turnID string,
) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "streaming unsupported"})
		return
	}
	writeEvent := func(event string, payload map[string]any) {
		blob, _ := json.Marshal(s.maskQueryPayload(payload))
		_, _ = fmt.Fprintf(w, "event: %s\n", event)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(blob))
		flusher.Flush()
	}

	writeEvent("start", map[string]any{"trace_id": traceID, "turn_id": turnID, "ok": true})
	allowed, reason, _ := checkGuardModelWithLLM(r.Context(), cfg, question, "/query")
	if !allowed {
		writeEvent("error", map[string]any{"ok": false, "error": "Request blocked by safety guard: " + reason, "trace_id": traceID, "turn_id": turnID})
		writeEvent("done", map[string]any{"ok": false})
		return
	}
	writeEvent("guard", map[string]any{"ok": true, "decision": "allowed"})

	tableNames := s.listTableNames(r.Context())
	schemaContext := buildSchemaContextForTables(s, r.Context(), tableNames)
	writeEvent("stage", map[string]any{"name": "schema_build", "ok": true})

	suggested, genStats, genErr := generateSQLWithLLMStream(
		r.Context(),
		cfg,
		question,
		schemaContext,
		preferredChartType,
		chartInstruction,
		thinkingLevel,
		func(delta string) {
			writeEvent("sql_delta", map[string]any{"delta": delta})
		},
	)
	if genErr != "" {
		writeEvent("error", map[string]any{"ok": false, "error": genErr, "trace_id": traceID, "turn_id": turnID, "llm_stats": summarizeQueryLLMStats(map[string]map[string]int{"sql_generation": genStats})})
		writeEvent("done", map[string]any{"ok": false})
		return
	}
	writeEvent("sql", map[string]any{"sql": suggested})

	finalSQL, cols, rows, execErr, retries, repairStats := s.validateAndExecuteWithRepair(r.Context(), question, schemaContext, suggested, tableNames, cfg, thinkingLevel)
	stages := map[string]map[string]int{"sql_generation": genStats}
	if len(repairStats) > 0 {
		stages["sql_repair"] = repairStats
	}

	fieldTypes := inferFieldTypes(cols, rows)
	datasets := []map[string]any{{"name": "main", "purpose": "primary dataset", "sql": finalSQL, "columns": cols, "field_types": fieldTypes, "rows": rows, "error": firstNonEmpty(execErr, "")}}
	chartSpec := ""
	if req.Chart && execErr == "" && len(cols) > 0 {
		namedQueries, namedStats, _ := generateNamedQueriesWithLLM(r.Context(), cfg, question, schemaContext, finalSQL, preferredChartType, chartInstruction, thinkingLevel)
		if len(namedStats) > 0 {
			stages["named_query_generation"] = namedStats
		}
		for _, nq := range namedQueries {
			nqCols, nqRows, nqErr := s.runSQL(r.Context(), nq.SQL)
			datasets = append(datasets, map[string]any{
				"name":        nq.Name,
				"purpose":     nq.Purpose,
				"sql":         nq.SQL,
				"columns":     nqCols,
				"field_types": inferFieldTypes(nqCols, nqRows),
				"rows":        nqRows,
				"error":       errorString(nqErr),
			})
		}
		sample := make([]map[string]any, 0, minInt(20, len(rows)))
		for _, row := range rows {
			if len(sample) >= 20 {
				break
			}
			record := map[string]any{}
			for i, col := range cols {
				if i < len(row) {
					record[col] = row[i]
				}
			}
			sample = append(sample, record)
		}
		chart, chartStats, chartErr := generateChartSpecWithLLM(r.Context(), cfg, cols, sample, question, preferredChartType, chartInstruction, datasets, thinkingLevel)
		if len(chartStats) > 0 {
			stages["chart_generation"] = chartStats
		}
		if chartErr == "" {
			chartSpec = chart
		} else {
			execErr = firstNonEmpty(execErr, chartErr)
		}
	}

	payload := map[string]any{
		"ok":          true,
		"trace_id":    traceID,
		"turn_id":     turnID,
		"sql":         finalSQL,
		"question":    question,
		"columns":     cols,
		"rows":        rows,
		"field_types": fieldTypes,
		"datasets":    datasets,
		"retry_count": retries,
		"chart_spec":  chartSpec,
		"error":       execErr,
		"llm_stats":   summarizeQueryLLMStats(stages),
	}
	writeEvent("result", payload)
	writeEvent("done", map[string]any{"ok": true})
}

func (s *Server) apiQueryRefineChart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req refineChartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	prompt := strings.TrimSpace(req.Prompt)
	if prompt == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	spec, ok := req.Spec.(map[string]any)
	if !ok || spec == nil {
		spec = map[string]any{}
	}
	lowerPrompt := strings.ToLower(prompt)
	if strings.Contains(lowerPrompt, "line") {
		spec["type"] = "line"
	}
	if strings.Contains(lowerPrompt, "bar") {
		spec["type"] = "bar"
	}
	if strings.Contains(lowerPrompt, "area") {
		spec["type"] = "area"
	}
	if strings.Contains(lowerPrompt, "table") {
		spec["type"] = "table"
	}
	if strings.Contains(lowerPrompt, "stack") {
		spec["stack"] = true
	}
	if strings.Contains(lowerPrompt, "sort") {
		spec["sort"] = "desc"
	}
	if strings.Contains(lowerPrompt, "limit") {
		spec["limit"] = 100
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "prompt": prompt, "spec": spec})
}

func (s *Server) apiQuerySchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tableNames := s.listTableNames(r.Context())
	schemaLines := make([]string, 0, len(tableNames))
	tables := make([]map[string]any, 0, len(tableNames))
	for _, table := range tableNames {
		colMeta := s.listTableColumnMeta(r.Context(), table)
		cols := make([]string, 0, len(colMeta))
		for _, col := range colMeta {
			name := anyToString(col["name"])
			typ := anyToString(col["type"])
			if name == "" {
				continue
			}
			if typ == "" {
				typ = "String"
			}
			cols = append(cols, name+" "+typ)
		}
		schemaLines = append(schemaLines, table+"("+strings.Join(cols, ", ")+")")
		tables = append(tables, map[string]any{"name": table, "columns": cols})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "schema": strings.Join(schemaLines, "\n"), "tables": tables})
}

func (s *Server) apiQueryAddToDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req addToDashboardRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	dashboardID := strings.TrimSpace(req.DashboardID)
	if dashboardID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "dashboard_id is required"})
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = "Query Result"
	}
	chartType := strings.TrimSpace(req.Type)
	if chartType == "" {
		chartType = "table"
	}
	spec := req.Spec
	if spec == nil {
		spec = map[string]any{}
	}
	chart, err := s.dashboardService.AddChart(dashboardID, title, chartType, spec)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"ok": true, "chart": chart})
}

func (s *Server) apiTableExplorerTables(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tableNames := s.listAllowedExistingTableNames(r.Context())
	tables := make([]map[string]any, 0, len(tableNames))
	for _, name := range tableNames {
		columns := s.listTableColumnMeta(r.Context(), name)
		tables = append(tables, map[string]any{
			"name":         name,
			"column_count": len(columns),
			"columns":      columns,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "tables": tables, "items": tableNames})
}

func (s *Server) apiTableExplorerTable(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := strings.TrimPrefix(r.URL.Path, "/api/table-explorer/table/")
	if name == "" || strings.Contains(name, "/") {
		http.NotFound(w, r)
		return
	}
	if sanitizeIdentifier(name) != name {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "invalid table name"})
		return
	}
	if !isQueryTableAllowed(name) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": fmt.Sprintf("Table '%s' is not accessible.", name)})
		return
	}
	columns := s.listTableColumnMeta(r.Context(), name)
	ddl := s.getTableDDL(r.Context(), name)
	sampleCols, sampleRows, err := s.runSQL(r.Context(), fmt.Sprintf("SELECT * FROM %s LIMIT 20", sanitizeIdentifier(name)))
	if err != nil {
		sampleCols = []string{}
		sampleRows = [][]any{}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"table":   name,
		"name":    name,
		"columns": columns,
		"ddl":     ddl,
		"sample": map[string]any{
			"columns": sampleCols,
			"rows":    sampleRows,
		},
	})
}

func (s *Server) apiChartTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	chartTypesPath := filepath.Join("static", "echarts-chart-types.json")
	chartTypesPath = filepath.Join(resolveAssetRoot("static", "echarts-chart-types.json"), "echarts-chart-types.json")
	blob, err := os.ReadFile(chartTypesPath)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Chart types catalog not found. Run: node scripts/extract-echarts-types.js"})
		return
	}
	var catalog any
	if err := json.Unmarshal(blob, &catalog); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "Failed to load chart types"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "data": catalog})
}

func suggestSQLForQuestion(question string, tables []string) string {
	return suggestSQLForQuestionWithGuidance(question, tables, "", "")
}

func suggestSQLForQuestionWithGuidance(question string, tables []string, preferredChartType string, chartInstruction string) string {
	lower := strings.ToLower(strings.TrimSpace(question))
	lowerChart := strings.ToLower(strings.TrimSpace(preferredChartType + " " + chartInstruction))
	hasTable := func(name string) bool {
		for _, table := range tables {
			if strings.EqualFold(table, name) {
				return true
			}
		}
		return false
	}
	if strings.Contains(lower, "error") && hasTable("otel_logs") {
		return "SELECT Timestamp, SeverityText, Body FROM otel_logs ORDER BY Timestamp DESC LIMIT 100"
	}
	if strings.Contains(lower, "trace") && hasTable("otel_traces") {
		return "SELECT Timestamp, TraceId, SpanId, SpanName FROM otel_traces ORDER BY Timestamp DESC LIMIT 100"
	}
	if strings.Contains(lower, "metric") && hasTable("otel_metrics_sum") {
		return "SELECT Timestamp, MetricName, Value FROM otel_metrics_sum ORDER BY Timestamp DESC LIMIT 100"
	}
	if strings.Contains(lowerChart, "sankey") || strings.Contains(lowerChart, "graph") || strings.Contains(lowerChart, "network") {
		if hasTable("otel_traces") {
			return "SELECT ServiceName AS source, SpanName AS target, count() AS value FROM otel_traces WHERE ServiceName != '' AND SpanName != '' GROUP BY ServiceName, SpanName ORDER BY value DESC LIMIT 100"
		}
	}
	if strings.Contains(lowerChart, "pie") && hasTable("otel_logs") {
		return "SELECT SeverityText AS name, count() AS value FROM otel_logs GROUP BY SeverityText ORDER BY value DESC LIMIT 20"
	}
	if strings.Contains(lowerChart, "bar") && hasTable("otel_logs") {
		return "SELECT ServiceName, count() AS error_count FROM otel_logs WHERE SeverityText IN ('ERROR','FATAL') GROUP BY ServiceName ORDER BY error_count DESC LIMIT 25"
	}
	if len(tables) > 0 {
		return fmt.Sprintf("SELECT * FROM %s LIMIT 100", sanitizeIdentifier(tables[0]))
	}
	return "SELECT now64(3) AS timestamp"
}

func buildQueryAllowedTables() map[string]struct{} {
	out := map[string]struct{}{}
	for k := range queryAllowedTablesBuiltin {
		out[k] = struct{}{}
	}
	extra := strings.TrimSpace(os.Getenv("SOBS_QUERY_ALLOWED_TABLES"))
	if extra == "" {
		return out
	}
	for _, part := range strings.Split(extra, ",") {
		name := strings.TrimSpace(part)
		if name == "" || !querySafeIdentifier.MatchString(name) {
			continue
		}
		out[strings.ToLower(name)] = struct{}{}
	}
	return out
}

func isQueryTableAllowed(table string) bool {
	allowed := buildQueryAllowedTables()
	_, ok := allowed[strings.ToLower(strings.TrimSpace(table))]
	return ok
}

func (s *Server) listAllowedExistingTableNames(ctx context.Context) []string {
	names := s.listTableNames(ctx)
	out := make([]string, 0, len(names))
	for _, name := range names {
		if isQueryTableAllowed(name) {
			out = append(out, name)
		}
	}
	return out
}

func (s *Server) listTableNames(ctx context.Context) []string {
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		fallback := []string{}
		for name := range buildQueryAllowedTables() {
			fallback = append(fallback, name)
		}
		if len(fallback) == 0 {
			return []string{"otel_logs", "otel_traces", "otel_metrics_sum"}
		}
		return fallback
	}
	defer func() { _ = store.Close() }()
	out := []string{}
	rows, err := store.Query(ctx, "SELECT name FROM system.tables WHERE database = currentDatabase() ORDER BY name")
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var name string
			if scanErr := rows.Scan(&name); scanErr == nil && strings.TrimSpace(name) != "" {
				t := strings.TrimSpace(name)
				if isQueryTableAllowed(t) {
					out = append(out, t)
				}
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	fallback := []string{}
	for name := range buildQueryAllowedTables() {
		fallback = append(fallback, name)
	}
	if len(fallback) == 0 {
		return []string{"otel_logs", "otel_traces", "otel_metrics_sum"}
	}
	return fallback
}

func (s *Server) listTableColumns(ctx context.Context, table string) []string {
	if table == "" {
		return []string{}
	}
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return []string{"id", "timestamp", "value"}
	}
	defer func() { _ = store.Close() }()
	out := []string{}
	rows, err := store.Query(ctx, "SELECT name FROM system.columns WHERE database = currentDatabase() AND table = ? ORDER BY position", table)
	if err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var name string
			if scanErr := rows.Scan(&name); scanErr == nil && strings.TrimSpace(name) != "" {
				out = append(out, strings.TrimSpace(name))
			}
		}
	}
	if len(out) > 0 {
		return out
	}
	return []string{"id", "timestamp", "value"}
}

func (s *Server) listTableColumnMeta(ctx context.Context, table string) []map[string]any {
	if table == "" {
		return []map[string]any{}
	}
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return []map[string]any{}
	}
	defer func() { _ = store.Close() }()
	out := []map[string]any{}
	rows, err := store.Query(ctx, "SELECT name, type, is_in_primary_key, is_in_sorting_key, is_in_partition_key, default_kind, comment FROM system.columns WHERE database = currentDatabase() AND table = ? ORDER BY position", table)
	if err != nil {
		return []map[string]any{}
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var name, typ, inPK, inSort, inPart, defaultKind, comment any
		if scanErr := rows.Scan(&name, &typ, &inPK, &inSort, &inPart, &defaultKind, &comment); scanErr != nil {
			continue
		}
		typeText := anyToString(typ)
		out = append(out, map[string]any{
			"name":             anyToString(name),
			"type":             typeText,
			"is_nullable":      strings.Contains(strings.ToLower(typeText), "nullable"),
			"is_primary_key":   anyToInt(inPK) > 0,
			"is_sorting_key":   anyToInt(inSort) > 0,
			"is_partition_key": anyToInt(inPart) > 0,
			"default_kind":     anyToString(defaultKind),
			"comment":          anyToString(comment),
		})
	}
	return out
}

func (s *Server) getTableDDL(ctx context.Context, table string) string {
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return ""
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT create_table_query FROM system.tables WHERE database = currentDatabase() AND name = ? LIMIT 1", table)
	if err != nil {
		return ""
	}
	defer func() { _ = rows.Close() }()
	if rows.Next() {
		var ddl any
		if scanErr := rows.Scan(&ddl); scanErr == nil {
			return anyToString(ddl)
		}
	}
	return ""
}

func (s *Server) runSQL(ctx context.Context, sqlText string) ([]string, [][]any, error) {
	sqlText = normalizeSQLText(sqlText)
	if ref := firstDisallowedTableRef(sqlText); ref != "" {
		return nil, nil, fmt.Errorf("access to table or view '%s' is not permitted", ref)
	}
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, sqlText)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = rows.Close() }()
	columnsProvider, ok := rows.(interface{ Columns() ([]string, error) })
	if !ok {
		return []string{"row"}, [][]any{}, nil
	}
	columns, err := columnsProvider.Columns()
	if err != nil {
		return nil, nil, err
	}
	maxRows := 200
	if limit := parseLimitFromSQL(sqlText); limit > 0 && limit < maxRows {
		maxRows = limit
	}
	out := make([][]any, 0, maxRows)
	for rows.Next() {
		values := make([]any, len(columns))
		args := make([]any, len(columns))
		for i := range values {
			args[i] = &values[i]
		}
		if err := rows.Scan(args...); err != nil {
			return columns, out, err
		}
		for i := range values {
			if b, ok := values[i].([]byte); ok {
				values[i] = string(b)
			}
		}
		out = append(out, values)
		if len(out) >= maxRows {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return columns, out, err
	}
	return columns, out, nil
}

func isReadOnlySQL(sqlText string) bool {
	trimmed := normalizeSQLText(sqlText)
	if trimmed == "" || strings.Contains(trimmed, ";") {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "select") || strings.HasPrefix(lower, "with") || strings.HasPrefix(lower, "show") || strings.HasPrefix(lower, "describe") || strings.HasPrefix(lower, "desc") || strings.HasPrefix(lower, "explain") {
		return !queryUnsafeSQLPatterns.MatchString(lower)
	}
	return false
}

func normalizeSQLText(sqlText string) string {
	trimmed := strings.TrimSpace(sqlText)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = queryFencePrefixRegex.ReplaceAllString(trimmed, "")
		trimmed = queryFenceSuffixRegex.ReplaceAllString(trimmed, "")
		trimmed = strings.TrimSpace(trimmed)
	}
	if strings.HasSuffix(trimmed, ";") {
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, ";"))
	}
	return trimmed
}

type namedQueryPlan struct {
	Name    string
	SQL     string
	Purpose string
}

func generateNamedQueriesForChart(question string, baseSQL string, preferredChartType string) []namedQueryPlan {
	lower := strings.ToLower(strings.TrimSpace(question + " " + preferredChartType))
	plans := []namedQueryPlan{}
	if strings.Contains(lower, "sankey") || strings.Contains(lower, "graph") || strings.Contains(lower, "network") {
		plans = append(plans,
			namedQueryPlan{Name: "nodes", Purpose: "node list", SQL: "SELECT DISTINCT ServiceName AS id, ServiceName AS label FROM otel_traces WHERE ServiceName != '' LIMIT 200"},
			namedQueryPlan{Name: "links", Purpose: "edge list", SQL: "SELECT ServiceName AS source, SpanName AS target, count() AS value FROM otel_traces WHERE ServiceName != '' AND SpanName != '' GROUP BY ServiceName, SpanName ORDER BY value DESC LIMIT 400"},
		)
	}
	if strings.Contains(lower, "timeseries") || strings.Contains(lower, "trend") || strings.Contains(lower, "line") {
		plans = append(plans, namedQueryPlan{Name: "series", Purpose: "time trend", SQL: "SELECT toStartOfMinute(Timestamp) AS ts, count() AS value FROM otel_logs GROUP BY ts ORDER BY ts DESC LIMIT 240"})
	}
	if len(plans) > 3 {
		return plans[:3]
	}
	return plans
}

func buildChartSpecJSON(question string, preferredChartType string, columns []string, rows [][]any, datasets []map[string]any) string {
	if len(columns) == 0 {
		return ""
	}
	chartType := strings.ToLower(strings.TrimSpace(preferredChartType))
	if chartType == "" {
		chartType = inferChartType(question, columns)
	}
	option := map[string]any{
		"title":   map[string]any{"text": firstNonEmpty(strings.TrimSpace(question), "Query Result")},
		"tooltip": map[string]any{"trigger": "axis"},
		"legend":  map[string]any{},
	}
	if chartType == "table" {
		return ""
	}
	if chartType == "pie" {
		pairs := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			if len(row) < 2 {
				continue
			}
			pairs = append(pairs, map[string]any{"name": fmt.Sprintf("%v", row[0]), "value": numericValue(row[1])})
		}
		option["series"] = []map[string]any{{"type": "pie", "radius": "65%", "data": pairs}}
		blob, _ := json.Marshal(option)
		return string(blob)
	}
	if chartType == "sankey" || chartType == "graph" {
		nodes := []map[string]any{}
		links := []map[string]any{}
		for _, ds := range datasets {
			name := strings.ToLower(anyToString(ds["name"]))
			dsRows, _ := ds["rows"].([][]any)
			if name == "nodes" {
				for _, r := range dsRows {
					if len(r) < 1 {
						continue
					}
					nodes = append(nodes, map[string]any{"name": fmt.Sprintf("%v", r[0])})
				}
			}
			if name == "links" {
				for _, r := range dsRows {
					if len(r) < 3 {
						continue
					}
					links = append(links, map[string]any{"source": fmt.Sprintf("%v", r[0]), "target": fmt.Sprintf("%v", r[1]), "value": numericValue(r[2])})
				}
			}
		}
		option["series"] = []map[string]any{{"type": "sankey", "data": nodes, "links": links}}
		blob, _ := json.Marshal(option)
		return string(blob)
	}
	labels := make([]any, 0, len(rows))
	values := make([]any, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		labels = append(labels, fmt.Sprintf("%v", row[0]))
		if len(row) > 1 {
			values = append(values, numericValue(row[1]))
		} else {
			values = append(values, 0)
		}
	}
	option["xAxis"] = map[string]any{"type": "category", "data": labels}
	option["yAxis"] = map[string]any{"type": "value"}
	option["series"] = []map[string]any{{"type": chartType, "data": values}}
	blob, _ := json.Marshal(option)
	return string(blob)
}

func inferChartType(question string, columns []string) string {
	lower := strings.ToLower(question)
	if strings.Contains(lower, "pie") {
		return "pie"
	}
	if strings.Contains(lower, "sankey") || strings.Contains(lower, "graph") || strings.Contains(lower, "network") {
		return "sankey"
	}
	if len(columns) > 1 {
		return "line"
	}
	return "bar"
}

func inferFieldTypes(columns []string, rows [][]any) []map[string]any {
	if len(columns) == 0 {
		return []map[string]any{}
	}
	out := make([]map[string]any, 0, len(columns))
	for i, name := range columns {
		dtype := "string"
		kind := "dimension"
		for _, row := range rows {
			if i >= len(row) {
				continue
			}
			switch row[i].(type) {
			case int, int32, int64, float32, float64, uint, uint32, uint64:
				dtype = "number"
				kind = "measure"
			}
			if dtype == "number" {
				break
			}
		}
		out = append(out, map[string]any{"name": name, "dtype": dtype, "kind": kind})
	}
	return out
}

func summarizeQueryLLMStats(stages map[string]map[string]int) map[string]any {
	summary := map[string]any{"totals": map[string]int{"prompt_tokens": 0, "completion_tokens": 0, "thinking_tokens": 0, "elapsed_ms": 0}}
	totals := summary["totals"].(map[string]int)
	keys := make([]string, 0, len(stages))
	for k := range stages {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, stageName := range keys {
		stage := stages[stageName]
		prompt := stage["prompt_tokens"]
		completion := stage["completion_tokens"]
		thinking := stage["thinking_tokens"]
		elapsed := stage["elapsed_ms"]
		summary[stageName] = map[string]int{"prompt_tokens": prompt, "completion_tokens": completion, "thinking_tokens": thinking, "elapsed_ms": elapsed}
		totals["prompt_tokens"] += prompt
		totals["completion_tokens"] += completion
		totals["thinking_tokens"] += thinking
		totals["elapsed_ms"] += elapsed
	}
	return summary
}

func queryGuardCheck(input string) (bool, string) {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return true, "allowed"
	}
	blocked := []string{"drop table", "delete from", "truncate table", "alter table", "grant ", "revoke ", "rm -rf", "exfiltrate", "credential dump"}
	for _, token := range blocked {
		if strings.Contains(lower, token) {
			return false, "Blocked by heuristic safety check"
		}
	}
	return true, "allowed"
}

func (s *Server) validateAndExecuteWithRepair(ctx context.Context, question string, schemaContext string, initialSQL string, tables []string, cfg queryAIConfig, thinkingLevel string) (string, []string, [][]any, string, int, map[string]int) {
	currentSQL := normalizeSQLText(initialSQL)
	retryCount := 0
	lastRepairStats := map[string]int{}
	if _, _, err := s.runSQL(ctx, "EXPLAIN "+currentSQL); err != nil {
		auto := autoRepairIncompleteCTESQL(currentSQL)
		if auto != "" && auto != currentSQL {
			currentSQL = auto
			retryCount++
		}
	}
	for i := 0; i < 3; i++ {
		cols, data, err := s.runSQL(ctx, currentSQL)
		if err == nil {
			return currentSQL, cols, data, "", retryCount, lastRepairStats
		}
		if i == 2 {
			return currentSQL, []string{}, [][]any{}, err.Error(), retryCount, lastRepairStats
		}
		auto := autoRepairIncompleteCTESQL(currentSQL)
		if auto != "" && auto != currentSQL {
			currentSQL = auto
			retryCount++
			continue
		}
		repaired, repairStats, repairErr := repairSQLWithLLM(ctx, cfg, question, schemaContext, currentSQL, err.Error(), i+1, thinkingLevel)
		if repairErr == "" && repaired != "" {
			currentSQL = repaired
			lastRepairStats = repairStats
			retryCount++
			continue
		}
		currentSQL = suggestSQLForQuestion(question, tables)
		retryCount++
	}
	return currentSQL, []string{}, [][]any{}, "Query execution failed", retryCount, lastRepairStats
}

func autoRepairIncompleteCTESQL(sqlText string) string {
	text := strings.TrimSpace(strings.TrimSuffix(sqlText, ";"))
	if text == "" {
		return ""
	}
	if !regexp.MustCompile(`(?i)^\s*with\b`).MatchString(text) {
		return ""
	}
	text = repairTruncatedInClauseLiterals(text)
	if strings.Count(text, "'")%2 != 0 {
		return ""
	}
	cteMatch := regexp.MustCompile(`(?i)^\s*with\s+([a-zA-Z_]\w*)\s+as\s*\(`).FindStringSubmatch(text)
	if len(cteMatch) != 2 {
		return ""
	}
	hasFinalSelect := regexp.MustCompile(`(?is)\)\s*select\b`).MatchString(text)
	openParens := strings.Count(text, "(")
	closeParens := strings.Count(text, ")")
	if hasFinalSelect && openParens <= closeParens {
		return ""
	}
	fixed := text
	if openParens > closeParens {
		fixed += strings.Repeat(")", openParens-closeParens)
	}
	if !regexp.MustCompile(`(?is)\)\s*select\b`).MatchString(fixed) {
		fixed += "\nSELECT * FROM " + cteMatch[1]
	}
	return fixed
}

func repairTruncatedInClauseLiterals(sqlText string) string {
	idx := regexp.MustCompile(`(?is)\bIN\s*\(([^)]*)$`).FindStringSubmatchIndex(sqlText)
	if len(idx) < 4 {
		return sqlText
	}
	itemsRaw := sqlText[idx[2]:idx[3]]
	if strings.TrimSpace(itemsRaw) == "" {
		return sqlText
	}
	parts := strings.Split(itemsRaw, ",")
	cleaned := make([]string, 0, len(parts))
	for _, p := range parts {
		t := strings.TrimSpace(p)
		if t == "" {
			continue
		}
		if strings.Count(t, "'")%2 != 0 {
			break
		}
		cleaned = append(cleaned, t)
	}
	if len(cleaned) == 0 {
		return sqlText
	}
	return sqlText[:idx[2]] + strings.Join(cleaned, ",") + ")"
}

func numericValue(v any) float64 {
	switch n := v.(type) {
	case int:
		return float64(n)
	case int32:
		return float64(n)
	case int64:
		return float64(n)
	case uint:
		return float64(n)
	case uint32:
		return float64(n)
	case uint64:
		return float64(n)
	case float32:
		return float64(n)
	case float64:
		return n
	default:
		f, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprintf("%v", v)), 64)
		return f
	}
}

func maxInt(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func buildSchemaContextForTables(s *Server, ctx context.Context, tables []string) string {
	parts := make([]string, 0, len(tables))
	for _, t := range tables {
		meta := s.listTableColumnMeta(ctx, t)
		cols := make([]string, 0, len(meta))
		for _, c := range meta {
			name := anyToString(c["name"])
			typ := anyToString(c["type"])
			if name == "" {
				continue
			}
			if typ == "" {
				typ = "String"
			}
			cols = append(cols, name+" "+typ)
		}
		parts = append(parts, t+"("+strings.Join(cols, ", ")+")")
	}
	return strings.Join(parts, "\n")
}

func firstDisallowedTableRef(sqlText string) string {
	text := strings.TrimSpace(sqlText)
	if text == "" {
		return ""
	}
	cteAliases := map[string]struct{}{}
	for _, m := range queryCTEAliasRegex.FindAllStringSubmatch(text, -1) {
		if len(m) == 2 {
			cteAliases[strings.ToLower(strings.TrimSpace(m[1]))] = struct{}{}
		}
	}
	arrayJoinRefs := map[string]struct{}{}
	for _, m := range queryArrayJoinRegex.FindAllStringSubmatch(text, -1) {
		if len(m) == 2 {
			arrayJoinRefs[strings.ToLower(strings.TrimSpace(m[1]))] = struct{}{}
		}
	}
	for _, m := range queryTableRefRegex.FindAllStringSubmatch(text, -1) {
		if len(m) != 2 {
			continue
		}
		ref := strings.TrimSpace(m[1])
		refLower := strings.ToLower(ref)
		if _, ok := cteAliases[refLower]; ok {
			continue
		}
		if _, ok := arrayJoinRefs[refLower]; ok {
			continue
		}
		parts := strings.Split(refLower, ".")
		dbName := "default"
		tableName := parts[len(parts)-1]
		if len(parts) > 1 {
			dbName = parts[0]
		}
		if dbName == "system" {
			continue
		}
		if dbName != "default" {
			return ref
		}
		if !isQueryTableAllowed(tableName) {
			return ref
		}
	}
	return ""
}

func sanitizeIdentifier(value string) string {
	clean := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			return r
		}
		return -1
	}, value)
	if clean == "" {
		return "otel_logs"
	}
	return clean
}

func parseLimitFromSQL(sqlText string) int {
	re := regexp.MustCompile(`(?i)\blimit\s+(\d+)\b`)
	matches := re.FindStringSubmatch(sqlText)
	if len(matches) != 2 {
		return 0
	}
	v, err := strconv.Atoi(matches[1])
	if err != nil {
		return 0
	}
	return v
}
