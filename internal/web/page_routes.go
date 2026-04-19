package web

import (
	"net/http"
	"strings"

	"github.com/flosch/pongo2/v6"
)

var pageRouteTemplates = map[string]string{
	"/summary":                        "summary.html",
	"/logs/help":                      "logs_help.html",
	"/errors/help":                    "errors_help.html",
	"/traces/help":                    "traces_help.html",
	"/incident":                       "incident.html",
	"/incident/help":                  "incident_help.html",
	"/rum":                            "rum.html",
	"/rum/help":                       "rum_help.html",
	"/web-traffic":                    "web_traffic.html",
	"/web-traffic/help":               "web_traffic_help.html",
	"/work-items":                     "work_items.html",
	"/work-items/help":                "work_items_help.html",
	"/ai":                             "ai.html",
	"/ai/help":                        "ai_help.html",
	"/reports":                        "reports.html",
	"/reports/help":                   "reports_help.html",
	"/settings":                       "settings.html",
	"/settings/help":                  "settings_help.html",
	"/settings/help/ai":               "settings_ai_help.html",
	"/settings/help/agents":           "settings_agents_help.html",
	"/settings/help/data-management":  "data_management_help.html",
	"/settings/help/enrichment":       "settings_enrichment_help.html",
	"/settings/help/kubernetes":       "kubernetes_help.html",
	"/settings/help/masking":          "masking_help.html",
	"/settings/help/notifications":    "settings_notifications_help.html",
	"/settings/help/repositories":     "settings_repositories_help.html",
	"/settings/help/tags":             "settings_tags_help.html",
	"/settings/notifications":         "settings_notifications.html",
	"/query":                          "query.html",
	"/query/help":                     "query_help.html",
	"/summary/help":                   "summary_help.html",
	"/metrics/help":                   "metrics_help.html",
	"/metrics/help/rules":             "metrics_rules_help.html",
	"/metrics/help/rules/auto":        "auto_metrics_rules_help.html",
	"/metrics/help/anomaly":           "metrics_anomaly_help.html",
	"/setup/help/playbooks":           "setup_playbooks_help.html",
	"/dashboards/help/chart-editor":   "chart_editor_help.html",
	"/kubernetes/help":                "kubernetes_help.html",
	"/cve/help":                       "cve_help.html",
}

func (s *Server) registerPageRoutes(mux *http.ServeMux) {
	// Real handlers for data-heavy pages.
	mux.HandleFunc("/logs", s.pageLogsHandler)
	mux.HandleFunc("/errors", s.pageErrorsHandler)
	mux.HandleFunc("/traces", s.pageTracesHandler)

	// Direct template pages for non-data routes.
	for path, tpl := range pageRouteTemplates {
		mux.HandleFunc(path, s.pageTemplateHandler(path, tpl))
	}
}

func (s *Server) pageTemplateHandler(path string, templateName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != path {
			http.NotFound(w, r)
			return
		}
		if s.renderErr != nil || s.renderer == nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "page": strings.TrimPrefix(path, "/")})
			return
		}
		ctx := pongo2.Context{
			"title":                 strings.TrimPrefix(path, "/"),
			"message":               "Go runtime active.",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": strings.TrimPrefix(path, "/")},
		}
		body, err := s.renderer.Render(templateName, ctx)
		if err != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	}
}
