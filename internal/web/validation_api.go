package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"

	"github.com/abartrim/sobs/internal/extensionpoints"
)

type regexValidateRequest struct {
	Pattern string `json:"pattern"`
	Scope   map[string]any `json:"scope"`
}

type filterValidateRequest struct {
	Filter string `json:"filter"`
	SQL    string `json:"sql"`
}

var unsafeWherePatterns = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|truncate|alter|create|replace|rename|attach|detach|grant|revoke|system\s+stop|system\s+start|system\s+reload|kill|optimize|exchange)\b`)

func validateRegexHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req regexValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	pat := strings.TrimSpace(req.Pattern)
	if pat == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "pattern is required"})
		return
	}
	if _, err := regexp.Compile(pat); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func validateFilterHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req filterValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	flt := strings.TrimSpace(req.SQL)
	if flt == "" {
		flt = strings.TrimSpace(req.Filter)
	}
	if flt == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "normalized": "", "issues": []any{}})
		return
	}
	if len(flt) > 2048 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filter too long"})
		return
	}

	issues := structuralSQLIssues(flt)
	normalized := normalizeLogsSQLWhere(flt)
	if err := validateUserSQLWhere(normalized); err != nil {
		issues = append(issues, map[string]string{"level": "error", "message": err.Error()})
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "normalized": "", "issues": issues})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "normalized": normalized, "issues": issues})
}

func validateUserSQLWhere(sqlWhere string) error {
	if unsafeWherePatterns.MatchString(sqlWhere) {
		return fmt.Errorf("SQL filter contains a disallowed keyword. Only comparison and logical expressions are permitted in filter fields.")
	}
	return nil
}

func normalizeLogsSQLWhere(sqlWhere string) string {
	normalized := strings.ReplaceAll(strings.TrimSpace(sqlWhere), ";", "")
	hasTagRE := regexp.MustCompile(`(?i)has_tag\s*\(\s*'((?:[^']|'')+)'\s*,\s*'((?:[^']|'')*)'\s*\)`)
	normalized = hasTagRE.ReplaceAllStringFunc(normalized, func(match string) string {
		parts := hasTagRE.FindStringSubmatch(match)
		if len(parts) != 3 {
			return match
		}
		tagKey := strings.ReplaceAll(parts[1], "''", "'")
		tagValue := strings.ReplaceAll(parts[2], "''", "'")
		tagKey = strings.ReplaceAll(tagKey, "'", "''")
		tagValue = strings.ReplaceAll(tagValue, "'", "''")
		return "lower(hex(MD5(concat(ServiceName,'|',toString(Timestamp),'|',TraceId,'|',SpanId)))) IN (SELECT RecordId FROM sobs_record_tags FINAL WHERE TagKey='" + tagKey + "' AND TagValue='" + tagValue + "' AND IsDeleted=0 AND RecordType='log')"
	})
	alias := map[string]string{
		"\\blevel\\b":    "SeverityText",
		"\\bservice\\b":  "ServiceName",
		"\\btrace_id\\b": "TraceId",
		"\\bspan_id\\b":  "SpanId",
		"\\bts\\b":       "Timestamp",
		"\\bbody\\b":     "Body",
	}
	for pat, replacement := range alias {
		re := regexp.MustCompile(pat)
		normalized = re.ReplaceAllString(normalized, replacement)
	}
	return normalized
}

func structuralSQLIssues(sqlWhere string) []map[string]string {
	issues := make([]map[string]string, 0)
	quoteOpen := false
	parenDepth := 0
	for i := 0; i < len(sqlWhere); i++ {
		ch := sqlWhere[i]
		if ch == '\'' {
			if i+1 < len(sqlWhere) && sqlWhere[i+1] == '\'' {
				i++
				continue
			}
			quoteOpen = !quoteOpen
			continue
		}
		if quoteOpen {
			continue
		}
		if ch == '(' {
			parenDepth++
			continue
		}
		if ch == ')' {
			parenDepth--
			if parenDepth < 0 {
				issues = append(issues, map[string]string{"level": "error", "message": "Unexpected ')' in filter."})
				break
			}
		}
	}
	if quoteOpen {
		issues = append(issues, map[string]string{"level": "error", "message": "Unclosed single quote in filter."})
	}
	if parenDepth > 0 {
		issues = append(issues, map[string]string{"level": "error", "message": "Unclosed '(' in filter."})
	}
	if regexp.MustCompile(`(?i)\b(AND|OR|NOT|IN|LIKE|ILIKE)\s*$`).MatchString(sqlWhere) {
		issues = append(issues, map[string]string{"level": "warning", "message": "Filter ends with an operator or keyword."})
	}
	return issues
}

func (s *Server) apiLogsFieldHints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fields := []map[string]any{
		{"name": "level", "column": "SeverityText", "type": "string", "values": []string{}},
		{"name": "service", "column": "ServiceName", "type": "string", "values": []string{}},
		{"name": "body", "column": "Body", "type": "string", "values": []string{}},
		{"name": "trace_id", "column": "TraceId", "type": "string", "values": []string{}},
		{"name": "span_id", "column": "SpanId", "type": "string", "values": []string{}},
		{"name": "ts", "column": "Timestamp", "type": "datetime", "values": []string{}},
		{"name": "EventName", "column": "EventName", "type": "string", "values": []string{}},
		{"name": "ScopeName", "column": "ScopeName", "type": "string", "values": []string{}},
	}
	attrKeys, tagKeys, tagValues := s.logsAdvancedFieldHints(r)
	operators := []string{"=", "!=", "LIKE", "NOT LIKE", "ILIKE", "NOT ILIKE", "IN", "NOT IN", ">", "<", ">=", "<="}
	keywords := []string{"AND", "OR", "NOT", "IS NULL", "IS NOT NULL", "TRUE", "FALSE", "NULL"}
	functions := []map[string]any{
		{"name": "has_tag", "signature": "has_tag('key','value')", "kind": "tag"},
		{"name": "match", "signature": "match(body, 'regex')", "kind": "string"},
		{"name": "positionCaseInsensitive", "signature": "positionCaseInsensitive(body, 'needle')", "kind": "string"},
		{"name": "startsWith", "signature": "startsWith(service, 'api')", "kind": "string"},
		{"name": "endsWith", "signature": "endsWith(service, 'worker')", "kind": "string"},
		{"name": "lower", "signature": "lower(service)", "kind": "string"},
		{"name": "upper", "signature": "upper(level)", "kind": "string"},
		{"name": "toString", "signature": "toString(ts)", "kind": "cast"},
		{"name": "toDateTime", "signature": "toDateTime('2026-03-30 12:00:00')", "kind": "datetime"},
	}
	snippets := []map[string]any{
		{"label": "level='ERROR'", "insert": "level='ERROR'", "kind": "predicate"},
		{"label": "service IN ('api','worker')", "insert": "service IN ('api','worker')", "kind": "predicate"},
		{"label": "has_tag('env','prod')", "insert": "has_tag('env','prod')", "kind": "predicate"},
		{"label": "match(body, 'timeout')", "insert": "match(body, 'timeout')", "kind": "predicate"},
		{"label": "ts >= toDateTime('2026-03-30 00:00:00')", "insert": "ts >= toDateTime('2026-03-30 00:00:00')", "kind": "predicate"},
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"fields":     fields,
		"attr_keys":  attrKeys,
		"tag_keys":   tagKeys,
		"tag_values": tagValues,
		"operators":  operators,
		"keywords":   keywords,
		"functions":  functions,
		"snippets":   snippets,
	})
}

func (s *Server) apiAIFieldHints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fields := s.schemaFieldHints("otel_logs", []string{"prompt", "model", "latency_ms", "token_count", "status"})
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
}

func (s *Server) schemaFieldHints(table string, defaults []string) []string {
	set := map[string]struct{}{}
	for _, name := range s.listTableColumns(context.Background(), table) {
		norm := strings.TrimSpace(strings.ToLower(name))
		if norm != "" {
			set[norm] = struct{}{}
		}
	}
	for _, name := range defaults {
		norm := strings.TrimSpace(strings.ToLower(name))
		if norm != "" {
			set[norm] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for name := range set {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func (s *Server) apiLogsValidateFilter(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req filterValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	flt := strings.TrimSpace(req.SQL)
	if flt == "" {
		flt = strings.TrimSpace(req.Filter)
	}
	if flt == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "normalized": "", "issues": []any{}})
		return
	}
	if len(flt) > 2048 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "filter too long"})
		return
	}
	issues := structuralSQLIssues(flt)
	if hasErrorLevelIssue(issues) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "normalized": "", "issues": issues})
		return
	}
	normalized := normalizeLogsSQLWhere(flt)
	if err := validateUserSQLWhere(normalized); err != nil {
		issues = append(issues, map[string]string{"level": "error", "message": err.Error()})
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "normalized": "", "issues": issues})
		return
	}
	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()
		rows, queryErr := store.Query(r.Context(), "SELECT 1 FROM otel_logs WHERE "+normalized+" LIMIT 1")
		if queryErr != nil {
			if !isMissingTableError(queryErr) {
				issues = append(issues, map[string]string{"level": "error", "message": queryErr.Error()})
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "normalized": "", "issues": issues})
				return
			}
		} else {
			defer rows.Close()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "normalized": normalized, "issues": issues})
}

func (s *Server) apiAIValidateFilter(w http.ResponseWriter, r *http.Request) {
	validateFilterHandler(w, r)
}

func (s *Server) apiLogsValidateRegex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req regexValidateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	pattern := strings.TrimSpace(req.Pattern)
	if pattern == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sample": nil})
		return
	}
	includePatterns, excludePatterns, regexErr := prepareRegexFilterPatterns(pattern)
	if regexErr != "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": strings.TrimPrefix(regexErr, "Regex error: "), "sample": nil})
		return
	}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sample": nil})
		return
	}
	defer store.Close()
	sample, sampleErr := logsRegexBestEffortSample(r.Context(), store, req.Scope, includePatterns, excludePatterns)
	if sampleErr != nil && !isMissingTableError(sampleErr) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": sampleErr.Error(), "sample": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sample": sample})
}

func (s *Server) apiErrorsValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func (s *Server) apiTracesValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func (s *Server) apiMetricsValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func (s *Server) apiRUMValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
}

func hasErrorLevelIssue(issues []map[string]string) bool {
	for _, issue := range issues {
		if strings.EqualFold(strings.TrimSpace(issue["level"]), "error") {
			return true
		}
	}
	return false
}

func (s *Server) logsAdvancedFieldHints(r *http.Request) ([]string, []string, map[string][]string) {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return []string{}, []string{}, map[string][]string{}
	}
	defer store.Close()

	attrKeys := make([]string, 0)
	rows, err := store.Query(r.Context(), "SELECT DISTINCT AttrKey FROM sobs_log_attr_keys FINAL WHERE RecordType = 'log' AND IsDeleted = 0 ORDER BY AttrKey LIMIT 1000")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var value any
			if scanErr := rows.Scan(&value); scanErr == nil {
				if key := strings.TrimSpace(anyToString(value)); key != "" {
					attrKeys = append(attrKeys, key)
				}
			}
		}
	}

	tagKeys := make([]string, 0)
	tagValues := make(map[string][]string)
	tagKeyRows, err := store.Query(r.Context(), "SELECT DISTINCT TagKey FROM sobs_record_tags FINAL WHERE RecordType='log' AND IsDeleted=0 ORDER BY TagKey LIMIT 100")
	if err == nil {
		defer tagKeyRows.Close()
		for tagKeyRows.Next() {
			var value any
			if scanErr := tagKeyRows.Scan(&value); scanErr != nil {
				continue
			}
			tagKey := strings.TrimSpace(anyToString(value))
			if tagKey == "" {
				continue
			}
			tagKeys = append(tagKeys, tagKey)
			valueRows, valueErr := store.Query(r.Context(), "SELECT DISTINCT TagValue FROM sobs_record_tags FINAL WHERE RecordType='log' AND TagKey = ? AND IsDeleted=0 ORDER BY TagValue LIMIT 20", tagKey)
			if valueErr != nil {
				continue
			}
			values := make([]string, 0)
			for valueRows.Next() {
				var tagValue any
				if scanErr := valueRows.Scan(&tagValue); scanErr == nil {
					if v := strings.TrimSpace(anyToString(tagValue)); v != "" {
						values = append(values, v)
					}
				}
			}
			_ = valueRows.Close()
			tagValues[tagKey] = values
		}
	}

	return attrKeys, tagKeys, tagValues
}

func logsRegexBestEffortSample(ctx context.Context, store extensionpoints.ClickHouseStore, scope map[string]any, includePatterns, excludePatterns []string) (any, error) {
	includeRegexps, err := compileRegexList(includePatterns)
	if err != nil {
		return nil, err
	}
	excludeRegexps, err := compileRegexList(excludePatterns)
	if err != nil {
		return nil, err
	}
	whereParts := make([]string, 0, 5)
	params := make([]any, 0, 8)
	if service := regexScopeText(scope, "service", 200); service != "" {
		whereParts = append(whereParts, "ServiceName = ?")
		params = append(params, service)
	}
	if level := regexScopeText(scope, "level", 200); level != "" {
		whereParts = append(whereParts, "SeverityText = ?")
		params = append(params, level)
	}
	if traceID := regexScopeText(scope, "trace_id", 64); traceID != "" {
		whereParts = append(whereParts, "TraceId = ?")
		params = append(params, traceID)
	}
	fromTS := regexScopeTimestamp(scope, "from_ts")
	if fromTS != "" {
		whereParts = append(whereParts, "Timestamp >= parseDateTime64BestEffort(?)")
		params = append(params, fromTS)
	}
	toTS := regexScopeTimestamp(scope, "to_ts")
	if toTS != "" {
		whereParts = append(whereParts, "Timestamp <= parseDateTime64BestEffort(?)")
		params = append(params, toTS)
	}
	if fromTS == "" && toTS == "" {
		whereParts = append(whereParts, "Timestamp >= now() - INTERVAL 24 HOUR")
	}
	query := "SELECT Body FROM otel_logs"
	if len(whereParts) > 0 {
		query += " WHERE " + strings.Join(whereParts, " AND ")
	}
	query += " ORDER BY Timestamp DESC LIMIT 2000"
	rows, err := store.Query(ctx, query, params...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var body any
		if scanErr := rows.Scan(&body); scanErr != nil {
			continue
		}
		sample := anyToString(body)
		if matchesRegexExpression(sample, includeRegexps, excludeRegexps) {
			return truncateRegexSample(sample), nil
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return nil, nil
}

func regexScopeText(scope map[string]any, key string, maxLen int) string {
	raw := strings.TrimSpace(anyToString(scope[key]))
	if raw == "" {
		return ""
	}
	if len(raw) > maxLen {
		return raw[:maxLen]
	}
	return raw
}

func regexScopeTimestamp(scope map[string]any, key string) string {
	text := regexScopeText(scope, key, 64)
	if text == "" {
		return ""
	}
	if normalized, err := normalizeCHTimestamp(text); err == nil {
		return normalized
	}
	return ""
}

func compileRegexList(patterns []string) ([]*regexp.Regexp, error) {
	compiled := make([]*regexp.Regexp, 0, len(patterns))
	for _, pattern := range patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, re)
	}
	return compiled, nil
}

func matchesRegexExpression(sample string, includeRegexps, excludeRegexps []*regexp.Regexp) bool {
	for _, re := range includeRegexps {
		if !re.MatchString(sample) {
			return false
		}
	}
	for _, re := range excludeRegexps {
		if re.MatchString(sample) {
			return false
		}
	}
	return true
}

func truncateRegexSample(sample string) string {
	if len(sample) <= 200 {
		return sample
	}
	return sample[:197] + "..."
}

func normalizeCHTimestamp(text string) (string, error) {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return "", fmt.Errorf("empty timestamp")
	}
	return normalized, nil
}
