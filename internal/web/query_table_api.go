package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

type queryRequest struct {
	Question string `json:"question"`
	SQL      string `json:"sql"`
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
	q := strings.TrimSpace(req.Question)
	if q == "" {
		q = "show recent errors"
	}
	suggested := suggestSQLForQuestion(q, s.listTableNames(r.Context()))
	writeJSON(w, http.StatusOK, map[string]any{"sql": suggested, "question": q})
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
	sqlText := strings.TrimSpace(req.SQL)
	if sqlText == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sql is required"})
		return
	}
	if !isReadOnlySQL(sqlText) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "only read-only SQL is allowed"})
		return
	}
	columns, rows, err := s.runSQL(r.Context(), sqlText)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": columns, "rows": rows, "sql": sqlText})
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
	if strings.TrimSpace(req.Prompt) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "prompt is required"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "prompt": strings.TrimSpace(req.Prompt), "spec": req.Spec})
}

func (s *Server) apiQuerySchema(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	tableNames := s.listTableNames(r.Context())
	tables := make([]map[string]any, 0, len(tableNames))
	for _, table := range tableNames {
		cols := s.listTableColumns(r.Context(), table)
		tables = append(tables, map[string]any{"name": table, "columns": cols})
	}
	writeJSON(w, http.StatusOK, map[string]any{"tables": tables})
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
	writeJSON(w, http.StatusOK, map[string]any{"items": s.listTableNames(r.Context())})
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
	writeJSON(w, http.StatusOK, map[string]any{"name": name, "columns": s.listTableColumns(r.Context(), name)})
}

func (s *Server) apiChartTypes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": []string{"line", "bar", "area", "table", "pie"}})
}

func suggestSQLForQuestion(question string, tables []string) string {
	lower := strings.ToLower(strings.TrimSpace(question))
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
	if len(tables) > 0 {
		return fmt.Sprintf("SELECT * FROM %s LIMIT 100", sanitizeIdentifier(tables[0]))
	}
	return "SELECT now64(3) AS timestamp"
}

func (s *Server) listTableNames(ctx context.Context) []string {
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return []string{"otel_logs", "otel_traces", "otel_metrics_sum"}
	}
	defer func() { _ = store.Close() }()
	out := []string{}
	rows, err := store.Query(ctx, "SELECT name FROM system.tables WHERE database = currentDatabase() ORDER BY name")
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
	return []string{"otel_logs", "otel_traces", "otel_metrics_sum"}
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

func (s *Server) runSQL(ctx context.Context, sqlText string) ([]string, [][]any, error) {
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
	trimmed := strings.TrimSpace(sqlText)
	if trimmed == "" || strings.Contains(trimmed, ";") {
		return false
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "select") || strings.HasPrefix(lower, "with") || strings.HasPrefix(lower, "show") || strings.HasPrefix(lower, "describe") || strings.HasPrefix(lower, "desc") || strings.HasPrefix(lower, "explain") {
		writeKeywords := regexp.MustCompile(`\b(insert|update|delete|drop|alter|create|truncate|optimize|backup|restore|grant|revoke)\b`)
		return !writeKeywords.MatchString(lower)
	}
	return false
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
