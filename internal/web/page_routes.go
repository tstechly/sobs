package web

import (
	"net/http"
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

func (s *Server) renderStaticTemplatePage(w http.ResponseWriter, r *http.Request, path, templateName, endpoint, title string) {
	if r.URL.Path != path {
		http.NotFound(w, r)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": endpoint})
		return
	}
	body, err := s.renderer.Render(templateName, pongo2.Context{
		"title":                 title,
		"message":               "Go runtime active.",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": endpoint},
	})
	if err != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *Server) summaryPage(w http.ResponseWriter, r *http.Request) { s.renderStaticTemplatePage(w, r, "/summary", "summary.html", "summary", "Summary") }
func (s *Server) summaryHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/summary/help", "summary_help.html", "summary/help", "Summary Help")
}
func (s *Server) logsHelpPage(w http.ResponseWriter, r *http.Request) { s.renderStaticTemplatePage(w, r, "/logs/help", "logs_help.html", "logs/help", "Logs Help") }
func (s *Server) errorsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/errors/help", "errors_help.html", "errors/help", "Errors Help")
}
func (s *Server) tracesHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/traces/help", "traces_help.html", "traces/help", "Traces Help")
}
func (s *Server) incidentPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/incident", "incident.html", "incident", "Incident")
}
func (s *Server) incidentHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/incident/help", "incident_help.html", "incident/help", "Incident Help")
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
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "rum"})
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
	s.renderStaticTemplatePage(w, r, "/rum/help", "rum_help.html", "rum/help", "RUM Help")
}
func (s *Server) webTrafficPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/web-traffic", "web_traffic.html", "web-traffic", "Web Traffic")
}
func (s *Server) webTrafficHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/web-traffic/help", "web_traffic_help.html", "web-traffic/help", "Web Traffic Help")
}
func (s *Server) workItemsPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/work-items", "work_items.html", "work-items", "Work Items")
}
func (s *Server) workItemsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/work-items/help", "work_items_help.html", "work-items/help", "Work Items Help")
}
func (s *Server) aiPage(w http.ResponseWriter, r *http.Request) { s.renderStaticTemplatePage(w, r, "/ai", "ai.html", "ai", "AI") }
func (s *Server) aiHelpPage(w http.ResponseWriter, r *http.Request) { s.renderStaticTemplatePage(w, r, "/ai/help", "ai_help.html", "ai/help", "AI Help") }
func (s *Server) reportsPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/reports", "reports.html", "reports", "Reports")
}
func (s *Server) reportsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/reports/help", "reports_help.html", "reports/help", "Reports Help")
}
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings", "settings.html", "settings", "Settings")
}
func (s *Server) settingsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help", "settings_help.html", "settings/help", "Settings Help")
}
func (s *Server) settingsAIHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/ai", "settings_ai_help.html", "settings/help/ai", "Settings AI Help")
}
func (s *Server) settingsAgentsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/agents", "settings_agents_help.html", "settings/help/agents", "Settings Agents Help")
}
func (s *Server) settingsDataManagementHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/data-management", "data_management_help.html", "settings/help/data-management", "Data Management Help")
}
func (s *Server) settingsEnrichmentHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/enrichment", "settings_enrichment_help.html", "settings/help/enrichment", "Settings Enrichment Help")
}
func (s *Server) settingsKubernetesHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/kubernetes", "kubernetes_help.html", "settings/help/kubernetes", "Kubernetes Help")
}
func (s *Server) settingsMaskingHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/masking", "masking_help.html", "settings/help/masking", "Masking Help")
}
func (s *Server) settingsNotificationsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/notifications", "settings_notifications_help.html", "settings/help/notifications", "Notifications Help")
}
func (s *Server) settingsRepositoriesHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/repositories", "settings_repositories_help.html", "settings/help/repositories", "Repositories Help")
}
func (s *Server) settingsTagsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/help/tags", "settings_tags_help.html", "settings/help/tags", "Tags Help")
}
func (s *Server) settingsNotificationsPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/settings/notifications", "settings_notifications.html", "settings/notifications", "Settings Notifications")
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
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": "query"})
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
	s.renderStaticTemplatePage(w, r, "/query/help", "query_help.html", "query/help", "Query Help")
}
func (s *Server) metricsHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/metrics/help", "metrics_help.html", "metrics/help", "Metrics Help")
}
func (s *Server) metricsRulesHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/metrics/help/rules", "metrics_rules_help.html", "metrics/help/rules", "Metrics Rules Help")
}
func (s *Server) metricsRulesAutoHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/metrics/help/rules/auto", "auto_metrics_rules_help.html", "metrics/help/rules/auto", "Auto Metrics Rules Help")
}
func (s *Server) metricsAnomalyHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/metrics/help/anomaly", "metrics_anomaly_help.html", "metrics/help/anomaly", "Metrics Anomaly Help")
}
func (s *Server) setupPlaybooksHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/setup/help/playbooks", "setup_playbooks_help.html", "setup/help/playbooks", "Setup Playbooks Help")
}
func (s *Server) chartEditorHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/dashboards/help/chart-editor", "chart_editor_help.html", "dashboards/help/chart-editor", "Chart Editor Help")
}
func (s *Server) kubernetesHelpPage(w http.ResponseWriter, r *http.Request) {
	s.renderStaticTemplatePage(w, r, "/kubernetes/help", "kubernetes_help.html", "kubernetes/help", "Kubernetes Help")
}
func (s *Server) cveHelpPage(w http.ResponseWriter, r *http.Request) { s.renderStaticTemplatePage(w, r, "/cve/help", "cve_help.html", "cve/help", "CVE Help") }

func (s *Server) pageTemplateHandler(path string, templateName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.renderStaticTemplatePage(w, r, path, templateName, strings.TrimPrefix(path, "/"), strings.TrimPrefix(path, "/"))
	}
}
