package web

import (
	"encoding/json"
	"net/http"
	"strings"
)

type metricsRuleRequest struct {
	Name      string `json:"name"`
	Query     string `json:"query"`
	Threshold string `json:"threshold"`
}

func (s *Server) metricsRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"rules": s.metricsService.ListRules()})
			return
		}
		s.pageTemplateHandler("/metrics/rules", "metrics_rules.html")(w, r)
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
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		writeJSON(w, http.StatusOK, s.metricsService.AnomalySnapshot())
		return
	}
	s.pageTemplateHandler("/metrics/anomaly", "metrics_anomaly.html")(w, r)
}

func (s *Server) apiMetricsAnomaly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.metricsService.AnomalySnapshot())
}
