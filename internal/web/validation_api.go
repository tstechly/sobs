package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
)

type regexValidateRequest struct {
	Pattern string `json:"pattern"`
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
	fields := s.schemaFieldHints("otel_logs", []string{"service.name", "severity_text", "body", "trace_id", "span_id"})
	writeJSON(w, http.StatusOK, map[string]any{"fields": fields})
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
	validateFilterHandler(w, r)
}

func (s *Server) apiAIValidateFilter(w http.ResponseWriter, r *http.Request) {
	validateFilterHandler(w, r)
}

func (s *Server) apiLogsValidateRegex(w http.ResponseWriter, r *http.Request) {
	validateRegexHandler(w, r)
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
