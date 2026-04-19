package web

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/abartrim/sobs/internal/features/metrics"
)

type metricsRuleRequest struct {
	Name      string `json:"name"`
	Query     string `json:"query"`
	Threshold string `json:"threshold"`
}

func (s *Server) metricsRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/metrics/rules" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		rawRules := s.metricsService.ListRules()
		rules := make([]map[string]any, 0, len(rawRules))
		for _, rule := range rawRules {
			rules = append(rules, metricRuleForTemplate(rule))
		}
		ctx := map[string]any{
			"title":                  "Metrics Rules",
			"mobile_breakpoint_max":  "575.98px",
			"request":                map[string]any{"endpoint": "metrics/rules"},
			"rules":                  rules,
			"services":               []any{},
			"signals":                []any{},
			"sources":                []any{},
			"auto_summary":           nil,
			"auto_preview":           []map[string]any{},
			"auto_dashboard_summary": nil,
			"auto_dashboard_preview": []map[string]any{},
			"auto_open_panel":        "",
			"source_label": func(source any) string {
				return strings.TrimSpace(toString(source))
			},
			"signal_label": func(_source any, signal any) string {
				return strings.TrimSpace(toString(signal))
			},
			"signal_description": func(_source any, _signal any) string {
				return ""
			},
		}
		s.renderTemplate(w, "metrics_rules.html", ctx)
	case http.MethodPost:
		var req metricsRuleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		rule, err := s.metricsService.CreateRule(req.Name, req.Query, req.Threshold)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func metricRuleForTemplate(rule metrics.Rule) map[string]any {
	query := strings.TrimSpace(rule.Query)
	source := "metrics"
	signal := query
	if before, after, ok := strings.Cut(query, ":"); ok {
		source = strings.TrimSpace(before)
		signal = strings.TrimSpace(after)
	}
	if source == "" {
		source = "metrics"
	}

	comparator := "gt"
	warning := "0"
	critical := "0"
	threshold := strings.TrimSpace(rule.Threshold)
	if parts := strings.Fields(threshold); len(parts) >= 2 {
		comparator = strings.TrimSpace(parts[0])
		if w, c, ok := strings.Cut(strings.TrimSpace(parts[1]), "/"); ok {
			warning = strings.TrimSpace(w)
			critical = strings.TrimSpace(c)
		}
	}

	return map[string]any{
		"id":                           rule.ID,
		"name":                         rule.Name,
		"rule_type":                    "threshold",
		"source":                       source,
		"signal":                       signal,
		"service":                      "",
		"attr_fp":                      "",
		"comparator":                   comparator,
		"warning_threshold":            warning,
		"critical_threshold":           critical,
		"secondary_source":             "",
		"secondary_signal":             "",
		"secondary_comparator":         "",
		"secondary_warning_threshold":  "",
		"secondary_critical_threshold": "",
		"seasonal_buckets_json":        "",
		"min_sample_count":             1,
	}
}

func (s *Server) metricsRulesAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.metricsService.AutoRules()})
}

func (s *Server) metricsRulesDashboardAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.metricsService.AutoDashboardRules()})
}

func (s *Server) metricsRulesSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/metrics/rules/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "delete" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	if !s.metricsService.DeleteRule(parts[0]) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": parts[0]})
}

func (s *Server) metricsAnomalyPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/anomaly" {
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
	ctx := map[string]any{
		"title":                 "Metrics Anomaly Details",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "metrics/anomaly"},
		"source":                "",
		"service":               "",
		"signal":                "",
		"metric":                "",
		"attr_fp":               "",
		"from_ts":               "",
		"to_ts":                 "",
		"hours":                 24,
		"error_msg":             "",
		"rows":                  []map[string]any{},
		"total":                 0,
		"sources":               []any{},
		"services":              []any{},
		"signals":               []any{},
		"related_target":        "",
		"point_state":           "",
		"point_score":           "",
		"source_label": func(source any) string {
			return strings.TrimSpace(toString(source))
		},
		"signal_label": func(_source any, signal any) string {
			return strings.TrimSpace(toString(signal))
		},
		"signal_description": func(_source any, _signal any) string {
			return ""
		},
	}
	s.renderTemplate(w, "metrics_anomaly.html", ctx)
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func (s *Server) apiMetricsAnomaly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.metricsService.AnomalySnapshot())
}
