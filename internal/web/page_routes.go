package web

import (
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/flosch/pongo2/v6"
)

func (s *Server) registerPageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/logs", s.pageLogsHandler)
	mux.HandleFunc("/errors", s.pageErrorsHandler)
	mux.HandleFunc("/traces", s.pageTracesHandler)
	mux.HandleFunc("/summary", s.summaryPage)
	mux.HandleFunc("/summary/help", s.summaryHelpPage)
	mux.HandleFunc("/logs/help", s.logsHelpPage)
	mux.HandleFunc("/errors/help", s.errorsHelpPage)
	mux.HandleFunc("/traces/help", s.tracesHelpPage)
	mux.HandleFunc("/incident", s.incidentPage)
	mux.HandleFunc("/incident/help", s.incidentHelpPage)
	mux.HandleFunc("/rum", s.rumPage)
	mux.HandleFunc("/rum/help", s.rumHelpPage)
	mux.HandleFunc("/web-traffic", s.webTrafficPage)
	mux.HandleFunc("/web-traffic/help", s.webTrafficHelpPage)
	mux.HandleFunc("/work-items", s.workItemsPage)
	mux.HandleFunc("/work-items/help", s.workItemsHelpPage)
	mux.HandleFunc("/ai", s.aiPage)
	mux.HandleFunc("/ai/help", s.aiHelpPage)
	mux.HandleFunc("/reports", s.reportsPage)
	mux.HandleFunc("/reports/help", s.reportsHelpPage)
	mux.HandleFunc("/settings", s.settingsPage)
	mux.HandleFunc("/settings/help", s.settingsHelpPage)
	mux.HandleFunc("/settings/help/ai", s.settingsAIHelpPage)
	mux.HandleFunc("/settings/help/agents", s.settingsAgentsHelpPage)
	mux.HandleFunc("/settings/help/data-management", s.settingsDataManagementHelpPage)
	mux.HandleFunc("/settings/help/enrichment", s.settingsEnrichmentHelpPage)
	mux.HandleFunc("/settings/help/kubernetes", s.settingsKubernetesHelpPage)
	mux.HandleFunc("/settings/help/masking", s.settingsMaskingHelpPage)
	mux.HandleFunc("/settings/help/notifications", s.settingsNotificationsHelpPage)
	mux.HandleFunc("/settings/help/repositories", s.settingsRepositoriesHelpPage)
	mux.HandleFunc("/settings/help/tags", s.settingsTagsHelpPage)
	mux.HandleFunc("/settings/notifications", s.settingsNotificationsPage)
	mux.HandleFunc("/query", s.queryPage)
	mux.HandleFunc("/query/help", s.queryHelpPage)
	mux.HandleFunc("/metrics/help", s.metricsHelpPage)
	mux.HandleFunc("/metrics/help/rules", s.metricsRulesHelpPage)
	mux.HandleFunc("/metrics/help/rules/auto", s.metricsRulesAutoHelpPage)
	mux.HandleFunc("/metrics/help/anomaly", s.metricsAnomalyHelpPage)
	mux.HandleFunc("/setup/help/playbooks", s.setupPlaybooksHelpPage)
	mux.HandleFunc("/dashboards/help/chart-editor", s.chartEditorHelpPage)
	mux.HandleFunc("/kubernetes/help", s.kubernetesHelpPage)
	mux.HandleFunc("/cve/help", s.cveHelpPage)
}

func (s *Server) summaryPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/summary" {
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

	stats, recentErrors, recentLogs, rumSummary, aiSummary := s.summaryData(r)

	enrichmentSettings := s.settingsService.Enrichment()
	cveEnabled := parseBool(pickSetting(enrichmentSettings, "cve_enabled", "enrichment.cve_enabled"))
	if _, ok := enrichmentSettings["cve_enabled"]; !ok {
		if _, ok := enrichmentSettings["enrichment.cve_enabled"]; !ok {
			cveEnabled = true
		}
	}

	findings := s.enrichmentService.ListFindings()
	cveOverview := map[string]any{
		"enabled":   cveEnabled,
		"total":     len(findings),
		"critical":  0,
		"high":      0,
		"medium":    0,
		"low":       0,
		"last_scan": "",
	}
	for _, f := range findings {
		switch strings.ToUpper(strings.TrimSpace(f.Severity)) {
		case "CRITICAL":
			cveOverview["critical"] = cveOverview["critical"].(int) + 1
		case "HIGH":
			cveOverview["high"] = cveOverview["high"].(int) + 1
		case "MEDIUM":
			cveOverview["medium"] = cveOverview["medium"].(int) + 1
		case "LOW":
			cveOverview["low"] = cveOverview["low"].(int) + 1
		}
		if cveOverview["last_scan"].(string) == "" && strings.TrimSpace(f.UpdatedAt) != "" {
			cveOverview["last_scan"] = f.UpdatedAt
		}
	}

	ctx := pongo2.Context{
		"title":                 "Summary",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "summary"},
		"stats":                 stats,
		"signal_health":         []any{},
		"recent_errors":         recentErrors,
		"recent_logs":           recentLogs,
		"rum_summary":           rumSummary,
		"ai_summary":            aiSummary,
		"cve_overview":          cveOverview,
	}
	s.renderTemplate(w, "summary.html", ctx)
}

func (s *Server) summaryData(r *http.Request) (map[string]any, []map[string]any, []map[string]any, []any, []any) {
	stats := map[string]any{
		"logs":     0,
		"errors":   0,
		"spans":    0,
		"rum":      0,
		"ai":       0,
		"services": []any{},
	}
	recentErrors := []map[string]any{}
	recentLogs := []map[string]any{}
	rumSummary := []any{}
	aiSummary := []any{}

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return stats, recentErrors, recentLogs, rumSummary, aiSummary
	}
	defer store.Close()

	if count, err := queryCount(r, store, "otel_logs", "", nil); err == nil {
		stats["logs"] = count
	}
	if count, err := queryCount(r, store, "otel_traces", "", nil); err == nil {
		stats["spans"] = count
	}
	if count, err := queryCount(r, store, "hyperdx_sessions", "", nil); err == nil {
		stats["rum"] = count
	}

	if rows, err := store.Query(r.Context(), "SELECT count() FROM otel_logs WHERE upper(SeverityText) IN ('ERROR','FATAL')"); err == nil {
		defer rows.Close()
		if rows.Next() {
			var c any
			if scanErr := rows.Scan(&c); scanErr == nil {
				stats["errors"] = anyToInt(c)
			}
		}
	}

	if rows, err := store.Query(r.Context(), "SELECT count() FROM otel_traces WHERE SpanAttributes['gen_ai.request.model'] != '' OR SpanAttributes['gen_ai.usage.input_tokens'] != '' OR SpanAttributes['gen_ai.usage.output_tokens'] != ''"); err == nil {
		defer rows.Close()
		if rows.Next() {
			var c any
			if scanErr := rows.Scan(&c); scanErr == nil {
				stats["ai"] = anyToInt(c)
			}
		}
	}

	services := []any{}
	if rows, err := store.Query(r.Context(), "SELECT ServiceName FROM (SELECT DISTINCT ServiceName FROM otel_logs WHERE ServiceName != '' UNION DISTINCT SELECT DISTINCT ServiceName FROM otel_traces WHERE ServiceName != '' UNION DISTINCT SELECT DISTINCT ServiceName FROM hyperdx_sessions WHERE ServiceName != '') ORDER BY ServiceName"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var svc any
			if scanErr := rows.Scan(&svc); scanErr == nil {
				if v := anyToString(svc); v != "" {
					services = append(services, v)
				}
			}
		}
	}
	stats["services"] = services

	if rows, err := store.Query(r.Context(), "SELECT Timestamp, ServiceName, Body, TraceId FROM otel_logs WHERE upper(SeverityText) IN ('ERROR','FATAL') ORDER BY Timestamp DESC LIMIT 5"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ts, svc, body, traceID any
			if scanErr := rows.Scan(&ts, &svc, &body, &traceID); scanErr != nil {
				continue
			}
			t := anyToString(ts)
			tr := anyToString(traceID)
			if tr == "" {
				tr = t + "|" + anyToString(svc)
			}
			recentErrors = append(recentErrors, map[string]any{
				"id":       tr,
				"ts":       t,
				"service":  anyToString(svc),
				"err_type": "ERROR",
				"message":  anyToString(body),
			})
		}
	}

	if rows, err := store.Query(r.Context(), "SELECT Timestamp, SeverityText, ServiceName, Body FROM otel_logs ORDER BY Timestamp DESC LIMIT 10"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var ts, level, svc, body any
			if scanErr := rows.Scan(&ts, &level, &svc, &body); scanErr != nil {
				continue
			}
			recentLogs = append(recentLogs, map[string]any{
				"ts":      anyToString(ts),
				"level":   anyToString(level),
				"service": anyToString(svc),
				"body":    anyToString(body),
			})
		}
	}

	if rows, err := store.Query(r.Context(), "SELECT EventName, count() AS cnt FROM hyperdx_sessions GROUP BY EventName ORDER BY cnt DESC"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var name, cnt any
			if scanErr := rows.Scan(&name, &cnt); scanErr != nil {
				continue
			}
			rumSummary = append(rumSummary, []any{anyToString(name), anyToInt(cnt)})
		}
	}

	if rows, err := store.Query(r.Context(), "SELECT SpanAttributes['gen_ai.request.model'] AS model, count() AS cnt, SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.input_tokens'])) AS ti, SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.output_tokens'])) AS to_ FROM otel_traces WHERE SpanAttributes['gen_ai.request.model'] != '' OR SpanAttributes['gen_ai.usage.input_tokens'] != '' OR SpanAttributes['gen_ai.usage.output_tokens'] != '' GROUP BY model ORDER BY cnt DESC"); err == nil {
		defer rows.Close()
		for rows.Next() {
			var model, cnt, ti, to any
			if scanErr := rows.Scan(&model, &cnt, &ti, &to); scanErr != nil {
				continue
			}
			aiSummary = append(aiSummary, []any{anyToString(model), anyToInt(cnt), anyToInt(ti), anyToInt(to)})
		}
	}

	return stats, recentErrors, recentLogs, rumSummary, aiSummary
}
func (s *Server) summaryHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/summary/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "summary_help.html", pongo2.Context{"title": "Summary Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "summary/help"}})
}
func (s *Server) logsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "logs_help.html", pongo2.Context{"title": "Logs Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "logs/help"}})
}
func (s *Server) errorsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/errors/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "errors_help.html", pongo2.Context{"title": "Errors Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "errors/help"}})
}
func (s *Server) tracesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/traces/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "traces_help.html", pongo2.Context{"title": "Traces Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "traces/help"}})
}
func (s *Server) incidentPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/incident" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	traceID := strings.TrimSpace(q.Get("trace_id"))
	errorID := strings.TrimSpace(q.Get("error_id"))
	rumSession := strings.TrimSpace(q.Get("rum_session"))
	rumTS := strings.TrimSpace(q.Get("rum_ts"))
	fromTS := strings.TrimSpace(q.Get("from_ts"))
	toTS := strings.TrimSpace(q.Get("to_ts"))
	service := strings.TrimSpace(q.Get("service"))

	windowMinutes := 30
	if parsed, err := strconv.Atoi(strings.TrimSpace(q.Get("window_minutes"))); err == nil {
		switch parsed {
		case 15, 30, 60, 180:
			windowMinutes = parsed
		}
	}

	ref := strings.TrimSpace(q.Get("_ref"))
	if ref == "" {
		switch {
		case traceID != "":
			ref = traceID
		case errorID != "":
			ref = errorID
		case rumSession != "":
			ref = rumSession
		}
	}

	ctx := pongo2.Context{
		"title":                    "Incident",
		"mobile_breakpoint_max":    "575.98px",
		"request":                  map[string]any{"endpoint": "incident"},
		"_ref":                     ref,
		"trace_id":                 traceID,
		"error_id":                 errorID,
		"rum_session":              rumSession,
		"rum_ts":                   rumTS,
		"from_ts":                  fromTS,
		"to_ts":                    toTS,
		"service":                  service,
		"window_minutes":           windowMinutes,
		"error_msg":                "",
		"primary_error":            nil,
		"primary_trace":            nil,
		"primary_rum":              nil,
		"existing_work_item":       nil,
		"related_errors":           []any{},
		"related_errors_truncated": false,
		"related_log_count":        0,
		"related_span_count":       0,
		"anomaly_state":            "",
		"related_rum_count":        0,
		"related_rum_sessions":     0,
		"related_rum_error_count":  0,
		"related_rum_events":       []any{},
		"mc": map[string]any{
			"health_chips":     []any{},
			"total_points":     0,
			"series":           []any{},
			"source_mode":      "none",
			"match_label":      "",
			"match_dimensions": []any{},
		},
		"raw_windows": []any{},
		"_wi_list":    []any{},
	}
	s.renderTemplate(w, "incident.html", ctx)
}
func (s *Server) incidentHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/incident/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "incident_help.html", pongo2.Context{"title": "Incident Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "incident/help"}})
}
func (s *Server) rumPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rum" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	services := []string{}
	if listed, err := s.listServicesFromLogs(r); err == nil {
		services = listed
	}

	ctx := pongo2.Context{
		"title":                 "RUM",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "rum"},
		"services":              services,
		"selected_service":      strings.TrimSpace(r.URL.Query().Get("service")),
		"from_ts":               strings.TrimSpace(r.URL.Query().Get("from_ts")),
		"to_ts":                 strings.TrimSpace(r.URL.Query().Get("to_ts")),
		"q":                     strings.TrimSpace(r.URL.Query().Get("q")),
		"rows":                  []any{},
		"total":                 0,
		"error_msg":             "",
	}
	s.renderTemplate(w, "rum.html", ctx)
}
func (s *Server) rumHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rum/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "rum_help.html", pongo2.Context{"title": "RUM Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "rum/help"}})
}
func (s *Server) webTrafficPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web-traffic" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	enrichmentSettings := s.settingsService.Enrichment()
	ctx := pongo2.Context{
		"title":                 "Web Traffic",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "web-traffic", "args": map[string]any{"from_ts": strings.TrimSpace(r.URL.Query().Get("from_ts")), "to_ts": strings.TrimSpace(r.URL.Query().Get("to_ts"))}},
		"from_ts":               strings.TrimSpace(r.URL.Query().Get("from_ts")),
		"to_ts":                 strings.TrimSpace(r.URL.Query().Get("to_ts")),
		"total":                 0,
		"geo_enabled":           parseBool(pickSetting(enrichmentSettings, "geo_enabled", "enrichment.geo_enabled")),
		"event_types":           []any{},
		"top_urls":              []any{},
		"error_msg":             "",
	}
	s.renderTemplate(w, "web_traffic.html", ctx)
}
func (s *Server) webTrafficHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web-traffic/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "web_traffic_help.html", pongo2.Context{"title": "Web Traffic Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "web-traffic/help"}})
}
func (s *Server) workItemsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/work-items" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	ctx := pongo2.Context{
		"title":                 "Work Items",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "work-items"},
		"items":                 []any{},
		"total_items":           0,
		"services":              []any{},
		"rules":                 []any{},
		"service_filter":        strings.TrimSpace(r.URL.Query().Get("service")),
		"rule_filter":           strings.TrimSpace(r.URL.Query().Get("rule_name")),
		"action_type_filter":    strings.TrimSpace(r.URL.Query().Get("action_type")),
		"status_filter":         strings.TrimSpace(r.URL.Query().Get("status")),
		"from_ts":               strings.TrimSpace(r.URL.Query().Get("from_ts")),
		"to_ts":                 strings.TrimSpace(r.URL.Query().Get("to_ts")),
	}
	s.renderTemplate(w, "work_items.html", ctx)
}
func (s *Server) workItemsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/work-items/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "work_items_help.html", pongo2.Context{"title": "Work Items Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "work-items/help"}})
}
func (s *Server) aiPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ai" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	q := r.URL.Query()
	viewMode := strings.TrimSpace(q.Get("view"))
	if viewMode != "trace" {
		viewMode = "flat"
	}
	limit := 25
	if parsed, err := strconv.Atoi(strings.TrimSpace(q.Get("limit"))); err == nil {
		switch {
		case parsed < 1:
			limit = 25
		case parsed > 200:
			limit = 200
		default:
			limit = parsed
		}
	}
	offset := 0
	if parsed, err := strconv.Atoi(strings.TrimSpace(q.Get("offset"))); err == nil && parsed >= 0 {
		offset = parsed
	}

	ctx := pongo2.Context{
		"title":                   "AI",
		"mobile_breakpoint_max":   "575.98px",
		"request":                 map[string]any{"endpoint": "ai"},
		"view_mode":               viewMode,
		"service":                 strings.TrimSpace(q.Get("service")),
		"model":                   strings.TrimSpace(q.Get("model")),
		"operation":               strings.TrimSpace(q.Get("operation")),
		"span_name":               strings.TrimSpace(q.Get("span_name")),
		"row_type":                strings.TrimSpace(q.Get("row_type")),
		"sql_where":               strings.TrimSpace(q.Get("sql")),
		"from_ts":                 strings.TrimSpace(q.Get("from_ts")),
		"to_ts":                   strings.TrimSpace(q.Get("to_ts")),
		"sort_by":                 strings.TrimSpace(q.Get("sort_by")),
		"sort_dir":                strings.TrimSpace(q.Get("sort_dir")),
		"limit":                   limit,
		"offset":                  offset,
		"total":                   0,
		"next_offset":             offset + limit,
		"services":                []any{},
		"models":                  []any{},
		"operations":              []any{},
		"span_names":              []any{},
		"selected_services":       []any{},
		"selected_models":         []any{},
		"selected_operations":     []any{},
		"selected_row_types":      []any{},
		"selected_span_names":     []any{},
		"ai_items":                []any{},
		"trace_groups":            []any{},
		"total_calls":             0,
		"total_tokens_in":         0,
		"total_tokens_out":        0,
		"total_errors":            0,
		"error_msg":               "",
		"ai_pricing_json":         "{}",
		"ai_pricing_sources_json": "[]",
	}
	s.renderTemplate(w, "ai.html", ctx)
}
func (s *Server) aiHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ai/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "ai_help.html", pongo2.Context{"title": "AI Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "ai/help"}})
}
func (s *Server) reportsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/reports" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	reportItems := s.reportService.List()
	reports := make([]map[string]any, 0, len(reportItems))
	for _, item := range reportItems {
		reports = append(reports, map[string]any{
			"id":          item.ID,
			"name":        item.Name,
			"description": "",
			"page_type":   "query",
			"filters":     map[string]any{},
		})
	}

	ctx := pongo2.Context{
		"title":                 "Reports",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "reports"},
		"reports":               reports,
	}
	s.renderTemplate(w, "reports.html", ctx)
}
func (s *Server) reportsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/reports/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "reports_help.html", pongo2.Context{"title": "Reports Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "reports/help"}})
}
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	maskingRules := s.maskingService.ListRules()
	maskingKeys := toStringSliceAny(maskingRules["keys"])
	maskingPatterns := toStringSliceAny(maskingRules["patterns"])
	aiSettings := s.settingsService.AI()
	dmSettings := s.dataManagementService.GetSettings()
	k8sSettings := s.kubernetesService.GetSettings()

	ctx := pongo2.Context{
		"title":                        "Settings",
		"mobile_breakpoint_max":        "575.98px",
		"request":                      map[string]any{"endpoint": "settings"},
		"tag_rule_count":               len(s.tagService.ListRules()),
		"anomaly_rule_count":           len(s.metricsService.ListRules()),
		"ai_configured":                isAIConfigured(aiSettings),
		"agent_rule_count":             len(s.agentService.ListRules()),
		"notification_channel_count":   len(s.notificationService.ListSubscriptions()),
		"notification_rule_count":      len(s.notificationService.ListRules()),
		"masking_custom_key_count":     len(maskingKeys),
		"masking_custom_pattern_count": len(maskingPatterns),
		"kubernetes_view_enabled":      k8sSettings.Enabled,
		"backup_enabled":               dmSettings.BackupEnabled,
		"query_allowed_tables":         s.listTableNames(r.Context()),
	}
	s.renderTemplate(w, "settings.html", ctx)
}
func (s *Server) settingsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "settings_help.html", pongo2.Context{"title": "Settings Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help"}})
}
func (s *Server) settingsAIHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/ai" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "settings_ai_help.html", pongo2.Context{"title": "Settings AI Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/ai"}})
}
func (s *Server) settingsAgentsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/agents" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "settings_agents_help.html", pongo2.Context{"title": "Settings Agents Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/agents"}})
}
func (s *Server) settingsDataManagementHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/data-management" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "data_management_help.html", pongo2.Context{"title": "Data Management Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/data-management"}})
}
func (s *Server) settingsEnrichmentHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/enrichment" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "settings_enrichment_help.html", pongo2.Context{"title": "Settings Enrichment Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/enrichment"}})
}
func (s *Server) settingsKubernetesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/kubernetes" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "kubernetes_help.html", pongo2.Context{"title": "Kubernetes Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/kubernetes"}})
}
func (s *Server) settingsMaskingHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/masking" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "masking_help.html", pongo2.Context{"title": "Masking Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/masking"}})
}
func (s *Server) settingsNotificationsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/notifications" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "settings_notifications_help.html", pongo2.Context{"title": "Notifications Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/notifications"}})
}
func (s *Server) settingsRepositoriesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/repositories" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "settings_repositories_help.html", pongo2.Context{"title": "Repositories Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/repositories"}})
}
func (s *Server) settingsTagsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/tags" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "settings_tags_help.html", pongo2.Context{"title": "Tags Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/tags"}})
}
func (s *Server) settingsNotificationsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/notifications" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	subs := s.notificationService.ListSubscriptions()
	channels := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		name := strings.TrimSpace(sub.Endpoint)
		if name == "" {
			name = "browser subscription"
		}
		channels = append(channels, map[string]any{
			"id":           sub.ID,
			"name":         name,
			"channel_type": "browser_push",
			"enabled":      sub.Enabled,
			"config": map[string]any{
				"endpoint":            sub.Endpoint,
				"mask_output_enabled": "1",
			},
		})
	}

	ruleItems := s.notificationService.ListRules()
	rules := make([]map[string]any, 0, len(ruleItems))
	for _, rule := range ruleItems {
		rules = append(rules, map[string]any{
			"id":               rule.ID,
			"name":             rule.Name,
			"enabled":          rule.Enabled,
			"logic_operator":   "any",
			"conditions":       []map[string]any{},
			"channel_ids":      []string{},
			"severity":         "warning",
			"cooldown_seconds": 300,
		})
	}

	vapidPublicKey := s.notificationService.VAPIDPublicKey()
	vapidKeySource := ""
	if strings.TrimSpace(os.Getenv("SOBS_VAPID_PRIVATE_KEY")) != "" {
		vapidKeySource = "env"
	} else if strings.TrimSpace(vapidPublicKey) != "" {
		vapidKeySource = "db"
	}

	ctx := pongo2.Context{
		"title":                 "Settings Notifications",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "settings/notifications"},
		"channel_types":         []string{"webhook", "slack", "email", "browser_push"},
		"channels":              channels,
		"rules":                 rules,
		"metric_rules":          s.metricsService.ListRules(),
		"notification_log":      []map[string]any{},
		"condition_types":       []string{"signal", "tag"},
		"signal_sources":        []string{"logs", "errors", "traces", "metrics", "rum"},
		"comparators":           []string{">", ">=", "<", "<=", "==", "!="},
		"tag_match_operators":   []string{"equals", "contains", "starts_with", "ends_with", "regex"},
		"tag_record_types":      []string{"all", "logs", "errors", "traces", "metrics", "rum"},
		"edit_rule":             nil,
		"vapid_public_key":      vapidPublicKey,
		"vapid_key_source":      vapidKeySource,
	}
	s.renderTemplate(w, "settings_notifications.html", ctx)
}
func (s *Server) queryPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/query" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	tables := s.listTableNames(r.Context())
	defaultSQL := suggestSQLForQuestion("show recent errors", tables)

	ctx := pongo2.Context{
		"title":                 "Query",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "query"},
		"tables":                tables,
		"default_sql":           defaultSQL,
		"question":              "",
		"error_msg":             "",
	}
	s.renderTemplate(w, "query.html", ctx)
}
func (s *Server) queryHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/query/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "query_help.html", pongo2.Context{"title": "Query Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "query/help"}})
}
func (s *Server) metricsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "metrics_help.html", pongo2.Context{"title": "Metrics Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help"}})
}
func (s *Server) metricsRulesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help/rules" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "metrics_rules_help.html", pongo2.Context{"title": "Metrics Rules Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help/rules"}})
}
func (s *Server) metricsRulesAutoHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help/rules/auto" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "auto_metrics_rules_help.html", pongo2.Context{"title": "Auto Metrics Rules Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help/rules/auto"}})
}
func (s *Server) metricsAnomalyHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help/anomaly" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "metrics_anomaly_help.html", pongo2.Context{"title": "Metrics Anomaly Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help/anomaly"}})
}
func (s *Server) setupPlaybooksHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/setup/help/playbooks" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "setup_playbooks_help.html", pongo2.Context{"title": "Setup Playbooks Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "setup/help/playbooks"}})
}
func (s *Server) chartEditorHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dashboards/help/chart-editor" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "chart_editor_help.html", pongo2.Context{"title": "Chart Editor Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "dashboards/help/chart-editor"}})
}
func (s *Server) kubernetesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/kubernetes/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "kubernetes_help.html", pongo2.Context{"title": "Kubernetes Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "kubernetes/help"}})
}
func (s *Server) cveHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/cve/help" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	s.renderTemplate(w, "cve_help.html", pongo2.Context{"title": "CVE Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "cve/help"}})
}

func toStringSliceAny(value any) []string {
	items, ok := value.([]string)
	if ok {
		return items
	}
	raw, ok := value.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if ok {
			out = append(out, text)
		}
	}
	return out
}

func isAIConfigured(values map[string]string) bool {
	if len(values) == 0 {
		return false
	}
	for _, key := range []string{"api_key", "base_url", "model", "endpoint"} {
		if strings.TrimSpace(values[key]) != "" {
			return true
		}
	}
	return false
}
