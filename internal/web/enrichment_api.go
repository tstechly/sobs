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
		"title":                        "CVE Findings",
		"mobile_breakpoint_max":        "575.98px",
		"request":                      map[string]any{"endpoint": "enrichment/cve"},
		"cve_enabled":                  cveEnabled,
		"cve_findings":                 cveFindings,
		"cve_last_scan":                "",
		"cve_last_backfill_cap":        maxReleases,
		"cve_last_backfill_attempted":  0,
		"cve_last_backfill_inserted":   0,
		"github_backfill_max_releases": maxReleases,
		"severity_filter":              severityFilter,
		"ecosystem_filter":             ecosystemFilter,
		"package_filter":               packageFilter,
		"show_all":                     showAll,
		"selected_severities":          selectedSeverities,
		"selected_ecosystems":          selectedEcosystems,
		"severities":                   severities,
		"ecosystems":                   ecosystems,
	}
	s.renderTemplate(w, "cve.html", ctx)
}

func (s *Server) apiWebTrafficGeo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	where, params := rumTimeWhereAndParams(fromTS, toTS)

	geoEnabled := parseBool(pickSetting(s.settingsService.Enrichment(), "geo_enabled", "enrichment.geo_enabled"))

	ipDetails := []map[string]any{}
	countryCounts := []map[string]any{}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()
		rows, queryErr := store.Query(r.Context(), "SELECT LogAttributes['client.ip'] AS ip, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY ip HAVING ip != '' ORDER BY cnt DESC LIMIT 200", params...)
		if queryErr == nil {
			defer rows.Close()
			total := 0
			for rows.Next() {
				var ip, cnt any
				if scanErr := rows.Scan(&ip, &cnt); scanErr != nil {
					continue
				}
				c := anyToInt(cnt)
				total += c
				ipDetails = append(ipDetails, map[string]any{
					"ip":           anyToString(ip),
					"count":        c,
					"country":      "Unknown",
					"country_code": "",
				})
			}
			if total > 0 {
				countryCounts = append(countryCounts, map[string]any{"name": "Unknown", "value": total})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"country_counts": countryCounts,
		"ip_details":     ipDetails,
		"geo_enabled":    geoEnabled,
	})
}

func (s *Server) apiWebTrafficBrowsers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	where, params := rumTimeWhereAndParams(fromTS, toTS)
	browsers := []map[string]any{}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()
		rows, queryErr := store.Query(r.Context(), "SELECT LogAttributes['browser.context.browserName'] AS browser, LogAttributes['browser.context.browserVersion'] AS version, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY browser, version ORDER BY cnt DESC LIMIT 50", params...)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var browser, version, cnt any
				if scanErr := rows.Scan(&browser, &version, &cnt); scanErr != nil {
					continue
				}
				name := strings.TrimSpace(anyToString(browser) + " " + anyToString(version))
				if name == "" {
					name = "Unknown"
				}
				browsers = append(browsers, map[string]any{"name": name, "value": anyToInt(cnt)})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "browsers": browsers})
}

func (s *Server) apiWebTrafficOS(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	where, params := rumTimeWhereAndParams(fromTS, toTS)
	items := []map[string]any{}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()
		rows, queryErr := store.Query(r.Context(), "SELECT LogAttributes['browser.context.osName'] AS os, LogAttributes['browser.context.osVersion'] AS version, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY os, version ORDER BY cnt DESC LIMIT 50", params...)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var osName, version, cnt any
				if scanErr := rows.Scan(&osName, &version, &cnt); scanErr != nil {
					continue
				}
				name := strings.TrimSpace(anyToString(osName) + " " + anyToString(version))
				if name == "" {
					name = "Unknown"
				}
				items = append(items, map[string]any{"name": name, "value": anyToInt(cnt)})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "operating_systems": items})
}

func (s *Server) apiWebTrafficTimezones(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	where, params := rumTimeWhereAndParams(fromTS, toTS)
	items := []map[string]any{}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()
		rows, queryErr := store.Query(r.Context(), "SELECT LogAttributes['browser.context.timezone'] AS tz, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY tz HAVING tz != '' ORDER BY cnt DESC LIMIT 50", params...)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var tz, cnt any
				if scanErr := rows.Scan(&tz, &cnt); scanErr != nil {
					continue
				}
				items = append(items, map[string]any{"name": anyToString(tz), "value": anyToInt(cnt)})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "timezones": items})
}

func (s *Server) apiWebTrafficLanguages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	where, params := rumTimeWhereAndParams(fromTS, toTS)
	items := []map[string]any{}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()
		rows, queryErr := store.Query(r.Context(), "SELECT LogAttributes['browser.context.language'] AS lang, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY lang HAVING lang != '' ORDER BY cnt DESC LIMIT 50", params...)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var lang, cnt any
				if scanErr := rows.Scan(&lang, &cnt); scanErr != nil {
					continue
				}
				items = append(items, map[string]any{"name": anyToString(lang), "value": anyToInt(cnt)})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "languages": items})
}

func (s *Server) apiWebTrafficDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	where, params := rumTimeWhereAndParams(fromTS, toTS)
	items := []map[string]any{}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()
		rows, queryErr := store.Query(r.Context(), "SELECT coalesce(nullIf(LogAttributes['browser.context.deviceType'], ''), nullIf(LogAttributes['browser.context.deviceClass'], ''), 'Unknown') AS device, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY device ORDER BY cnt DESC LIMIT 50", params...)
		if queryErr == nil {
			defer rows.Close()
			for rows.Next() {
				var device, cnt any
				if scanErr := rows.Scan(&device, &cnt); scanErr != nil {
					continue
				}
				items = append(items, map[string]any{"name": anyToString(device), "value": anyToInt(cnt)})
			}
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "devices": items})
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
