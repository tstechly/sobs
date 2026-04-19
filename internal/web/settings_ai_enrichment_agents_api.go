package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/features/settings"
)

func (s *Server) settingsAI(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/settings/ai" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}

		raw := s.settingsService.AI()
		templateSettings := buildAISettingsForTemplate(raw)
		expiresDate, expiryStatus := githubTokenExpiryStatus(templateSettings)
		savedPricing, sources, confirmed := aiPricingForTemplate(templateSettings)

		ctx := map[string]any{
			"title":                      "AI Configuration",
			"mobile_breakpoint_max":      "575.98px",
			"request":                    map[string]any{"endpoint": "settings/ai"},
			"settings":                   templateSettings,
			"github_token_expires_date":  expiresDate,
			"github_token_expiry_status": expiryStatus,
			"github_token_validation_status": map[string]any{
				"status":            pickSetting(templateSettings, "ai.github_token_validation_status", "github_token_validation_status"),
				"last_validated_at": pickSetting(templateSettings, "ai.github_token_last_validated_at", "github_token_last_validated_at"),
			},
			"default_ai_pricing":          defaultAIPricing(),
			"saved_ai_pricing":            savedPricing,
			"ai_pricing_sources":          sources,
			"confirmed_ai_pricing_models": confirmed,
		}
		s.renderTemplate(w, "settings_ai.html", ctx)
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
		if r.URL.Path != "/settings/enrichment" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		vals := s.settingsService.Enrichment()
		maxReleases, _ := strconv.Atoi(strings.TrimSpace(vals["github_backfill_max_releases"]))
		if maxReleases < 1 {
			maxReleases = 50
		}
		if maxReleases > 500 {
			maxReleases = 500
		}
		ctx := map[string]any{
			"title":                              "Enrichment Settings",
			"mobile_breakpoint_max":              "575.98px",
			"request":                            map[string]any{"endpoint": "settings/enrichment"},
			"geo_enabled":                        parseBool(vals["geo_enabled"]),
			"cve_enabled":                        parseBool(vals["cve_enabled"]),
			"cve_last_scan":                      strings.TrimSpace(vals["cve_last_scan"]),
			"github_backfill_min_releases":       1,
			"github_backfill_max_releases_limit": 500,
			"github_backfill_max_releases":       maxReleases,
		}
		s.renderTemplate(w, "settings_enrichment.html", ctx)
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
		if r.URL.Path != "/settings/agents" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		rules := s.agentService.ListRules()
		rawRuns := s.agentService.ListRuns()
		runs := make([]map[string]any, 0, len(rawRuns))
		for _, run := range rawRuns {
			runs = append(runs, map[string]any{
				"id":               run.ID,
				"created_at":       run.CreatedAt,
				"status":           run.Status,
				"rule_name":        run.Title,
				"is_dismissed":     run.Status == "dismissed",
				"guard_decision":   "",
				"dlp_result":       "",
				"analysis":         "",
				"suggestion":       "",
				"github_issue_url": "",
				"error_message":    "",
			})
		}
		ctx := map[string]any{
			"title":                 "Agent Rules",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "settings/agents"},
			"rules":                 rules,
			"runs":                  runs,
			"trigger_types":         []string{"manual", "anomaly_rule", "tag_rule"},
			"trigger_states":        []string{"any", "warning", "critical", "firing", "resolved"},
			"anomaly_rules":         s.metricsService.ListRules(),
			"tag_rules":             s.tagService.ListRules(),
		}
		s.renderTemplate(w, "settings_agents.html", ctx)
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

func buildAISettingsForTemplate(values map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range values {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
		if !strings.HasPrefix(k, "ai.") {
			out["ai."+k] = v
		}
	}
	return out
}

func pickSetting(values map[string]string, keys ...string) string {
	for _, key := range keys {
		if val, ok := values[key]; ok && strings.TrimSpace(val) != "" {
			return strings.TrimSpace(val)
		}
	}
	return ""
}

func githubTokenExpiryStatus(values map[string]string) (string, map[string]any) {
	dateVal := pickSetting(values, "ai.github_token_expires_at", "github_token_expires_at")
	if len(dateVal) >= 10 {
		dateVal = dateVal[:10]
	}
	if dateVal == "" {
		return "", map[string]any{"state": "unknown", "message": "No expiry date set."}
	}
	parsed, err := time.Parse("2006-01-02", dateVal)
	if err != nil {
		return dateVal, map[string]any{"state": "unknown", "message": "Invalid expiry date format."}
	}
	days := int(parsed.Sub(time.Now().UTC()).Hours() / 24)
	if days < 0 {
		return dateVal, map[string]any{"state": "expired", "message": "Token is expired."}
	}
	if days <= 14 {
		return dateVal, map[string]any{"state": "warning", "message": "Token expires soon."}
	}
	return dateVal, map[string]any{"state": "healthy", "message": "Token expiry looks healthy."}
}

func defaultAIPricing() map[string]map[string]float64 {
	return map[string]map[string]float64{
		"gpt-4o":      {"in": 5.0, "out": 15.0},
		"gpt-4o-mini": {"in": 0.15, "out": 0.6},
	}
}

func aiPricingForTemplate(values map[string]string) (map[string]map[string]float64, map[string]string, []string) {
	pricing := map[string]map[string]float64{}
	sources := map[string]string{}
	for model, row := range defaultAIPricing() {
		pricing[model] = map[string]float64{"in": row["in"], "out": row["out"]}
		sources[model] = "default"
	}

	rawJSON := pickSetting(values, "ai.model_pricing", "model_pricing")
	if strings.TrimSpace(rawJSON) != "" {
		parsed := map[string]map[string]float64{}
		if err := json.Unmarshal([]byte(rawJSON), &parsed); err == nil {
			for model, row := range parsed {
				k := strings.ToLower(strings.TrimSpace(model))
				if k == "" {
					continue
				}
				pricing[k] = map[string]float64{"in": row["in"], "out": row["out"]}
				sources[k] = "custom"
			}
		}
	}

	confirmedRaw := pickSetting(values, "ai.model_pricing_confirmed", "model_pricing_confirmed")
	confirmed := []string{}
	if strings.TrimSpace(confirmedRaw) != "" {
		_ = json.Unmarshal([]byte(confirmedRaw), &confirmed)
	}
	return pricing, sources, confirmed
}
