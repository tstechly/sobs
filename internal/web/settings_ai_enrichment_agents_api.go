package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/features/settings"
)

func (s *Server) settingsAI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"settings": s.settingsService.AI()})
			return
		}
		s.pageTemplateHandler("/settings/ai", "settings_ai.html")(w, r)
	case http.MethodPost:
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		s.settingsService.SaveAI(vals)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": s.settingsService.AI()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) settingsEnrichment(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"settings": s.settingsService.Enrichment()})
			return
		}
		s.pageTemplateHandler("/settings/enrichment", "settings_enrichment.html")(w, r)
	case http.MethodPost:
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		geoEnabled := parseBool(vals["geo_enabled"])
		cveEnabled := parseBool(vals["cve_enabled"])
		maxReleases, _ := strconv.Atoi(strings.TrimSpace(vals["github_backfill_max_releases"]))
		s.settingsService.SaveEnrichment(geoEnabled, cveEnabled, maxReleases)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "settings": s.settingsService.Enrichment()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) settingsAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"rules": s.agentService.ListRules()})
			return
		}
		s.pageTemplateHandler("/settings/agents", "settings_agents.html")(w, r)
	case http.MethodPost:
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		actions := settings.SortedActions(map[string]bool{
			"analyze":   parseBool(vals["action_analyze"]),
			"summarize": parseBool(vals["action_summarize"]),
			"create_pr": parseBool(vals["action_create_pr"]),
		})
		rateLimit, _ := strconv.Atoi(strings.TrimSpace(vals["rate_limit_minutes"]))
		rule, err := s.agentService.CreateRule(
			strings.TrimSpace(vals["name"]),
			strings.TrimSpace(vals["description"]),
			strings.TrimSpace(vals["trigger_type"]),
			strings.TrimSpace(vals["trigger_ref_id"]),
			strings.TrimSpace(vals["trigger_state"]),
			actions,
			rateLimit,
		)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, rule)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) settingsAgentsSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/settings/agents/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "delete" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.agentService.DeleteRule(parts[0]) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": parts[0]})
}

func parseBool(v string) bool {
	s := strings.ToLower(strings.TrimSpace(v))
	return s == "1" || s == "true" || s == "yes" || s == "on"
}

func decodeStringMap(r *http.Request) (map[string]string, error) {
	if err := r.ParseForm(); err == nil && len(r.PostForm) > 0 {
		out := make(map[string]string, len(r.PostForm))
		for k, values := range r.PostForm {
			if len(values) > 0 {
				out[k] = values[0]
			}
		}
		return out, nil
	}
	var out map[string]string
	if err := json.NewDecoder(r.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}
