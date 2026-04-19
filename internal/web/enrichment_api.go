package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

type cveDispositionRequest struct {
	Disposition string `json:"disposition"`
}

func (s *Server) enrichmentCVEPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/enrichment/cve" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		writeJSON(w, http.StatusOK, map[string]any{"findings": s.enrichmentService.ListFindings()})
		return
	}

	enrichmentSettings := s.settingsService.Enrichment()
	cveEnabled := parseBool(pickSetting(enrichmentSettings, "cve_enabled", "enrichment.cve_enabled"))
	if _, ok := enrichmentSettings["cve_enabled"]; !ok {
		if _, ok := enrichmentSettings["enrichment.cve_enabled"]; !ok {
			cveEnabled = true
		}
	}
	maxReleases := 50
	if raw := strings.TrimSpace(pickSetting(enrichmentSettings, "github_backfill_max_releases", "enrichment.github_backfill_max_releases")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			maxReleases = parsed
		}
	}

	severityFilter := strings.TrimSpace(r.URL.Query().Get("severity"))
	ecosystemFilter := strings.TrimSpace(r.URL.Query().Get("ecosystem"))
	packageFilter := strings.TrimSpace(r.URL.Query().Get("package"))
	showAll := parseBool(strings.TrimSpace(r.URL.Query().Get("show_all")))
	selectedSeverities := []string{}
	selectedEcosystems := []string{}
	if severityFilter != "" {
		selectedSeverities = []string{severityFilter}
	}
	if ecosystemFilter != "" {
		selectedEcosystems = []string{ecosystemFilter}
	}

	rawFindings := s.enrichmentService.ListFindings()
	cveFindings := make([]map[string]any, 0, len(rawFindings))
	severitySet := map[string]bool{}
	ecosystemSet := map[string]bool{}
	for _, finding := range rawFindings {
		severity := strings.TrimSpace(finding.Severity)
		if severity == "" {
			severity = "UNKNOWN"
		}
		severitySet[severity] = true
		cveFindings = append(cveFindings, map[string]any{
			"osv_id":              finding.OSVID,
			"package":             finding.Package,
			"severity":            severity,
			"disposition":         finding.Disposition,
			"disposition_expired": false,
			"disposition_note":    "",
			"published":           finding.UpdatedAt,
			"ecosystem":           "unknown",
			"version":             "",
			"service":             "",
			"cve_ids":             []string{},
			"summary":             "",
		})
		ecosystemSet["unknown"] = true
	}
	severities := []string{}
	for _, s := range []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"} {
		if severitySet[s] {
			severities = append(severities, s)
		}
	}
	if len(severities) == 0 {
		severities = []string{"CRITICAL", "HIGH", "MEDIUM", "LOW", "UNKNOWN"}
	}
	ecosystems := []string{}
	for key := range ecosystemSet {
		ecosystems = append(ecosystems, key)
	}
	if len(ecosystems) == 0 {
		ecosystems = []string{"unknown"}
	}

	ctx := map[string]any{
		"title":                      "CVE Findings",
		"mobile_breakpoint_max":      "575.98px",
		"request":                    map[string]any{"endpoint": "enrichment/cve"},
		"cve_enabled":                cveEnabled,
		"cve_findings":               cveFindings,
		"cve_last_scan":              "",
		"cve_last_backfill_cap":      maxReleases,
		"cve_last_backfill_attempted": 0,
		"cve_last_backfill_inserted":  0,
		"github_backfill_max_releases": maxReleases,
		"severity_filter":            severityFilter,
		"ecosystem_filter":           ecosystemFilter,
		"package_filter":             packageFilter,
		"show_all":                   showAll,
		"selected_severities":        selectedSeverities,
		"selected_ecosystems":        selectedEcosystems,
		"severities":                 severities,
		"ecosystems":                 ecosystems,
	}
	s.renderTemplate(w, "cve.html", ctx)
}

func (s *Server) apiWebTrafficGeo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.Geo()})
}

func (s *Server) apiWebTrafficBrowsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.Browsers()})
}

func (s *Server) apiWebTrafficOS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.OS()})
}

func (s *Server) apiWebTrafficTimezones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.Timezones()})
}

func (s *Server) apiWebTrafficLanguages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.Languages()})
}

func (s *Server) apiWebTrafficDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.Devices()})
}

func (s *Server) apiEnrichmentLibraries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.Libraries()})
}

func (s *Server) apiEnrichmentGitHubRepoHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.enrichmentService.GitHubRepoHealth())
}

func (s *Server) apiEnrichmentCVEFindings(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": s.enrichmentService.ListFindings()})
}

func (s *Server) apiEnrichmentCVEFindingsSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/enrichment/cve/findings/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] != "disposition" {
		http.NotFound(w, r)
		return
	}
	var req cveDispositionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	f, ok := s.enrichmentService.SetDisposition(parts[0], req.Disposition)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, f)
}

func (s *Server) apiEnrichmentCVEScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.enrichmentService.Scan())
}
