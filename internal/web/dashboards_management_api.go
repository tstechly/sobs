package web

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/abartrim/sobs/internal/features/dashboards"
)

type dashboardCreateRequest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type chartCreateRequest struct {
	Title string         `json:"title"`
	Type  string         `json:"type"`
	Spec  map[string]any `json:"spec"`
}

type chartImportRequest struct {
	Items []dashboards.Chart `json:"items"`
}

func (s *Server) dashboardsRoot(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/dashboards" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		dashboards := s.dashboardService.List()
		ctx := map[string]any{
			"title":                 "Custom Dashboards",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "dashboards"},
			"dashboards":            dashboards,
			"show_new_form":         false,
		}
		s.renderTemplate(w, "custom_dashboards.html", ctx)
	case http.MethodPost:
		name, description, decodeErr, asJSON := decodeDashboardCreate(r)
		if decodeErr != nil {
			if asJSON {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			} else {
				http.Redirect(w, r, "/dashboards", http.StatusFound)
			}
			return
		}
		d, err := s.dashboardService.Create(name, description)
		if err != nil {
			if asJSON {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			} else {
				http.Redirect(w, r, "/dashboards", http.StatusFound)
			}
			return
		}
		if asJSON {
			writeJSON(w, http.StatusCreated, d)
			return
		}
		http.Redirect(w, r, "/dashboards/"+d.ID, http.StatusFound)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) dashboardsNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	ctx := map[string]any{
		"title":                 "Custom Dashboards",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "dashboards"},
		"dashboards":            s.dashboardService.List(),
		"show_new_form":         true,
	}
	s.renderTemplate(w, "custom_dashboards.html", ctx)
}

func (s *Server) dashboardsSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/dashboards/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	dashboardID := parts[0]

	if len(parts) == 1 {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		d, ok := s.dashboardService.Get(dashboardID)
		if !ok {
			// Redirect to the dashboards list when the requested dashboard
			// does not exist so the user lands somewhere usable.
			http.Redirect(w, r, "/dashboards", http.StatusFound)
			return
		}
		charts := s.listDashboardCharts(r, dashboardID)
		templates := s.dashboardTemplateContext()
		ctx := renderContext{
			"title":                 d.Name,
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "dashboards"},
			"dashboard": map[string]any{
				"id":          d.ID,
				"name":        d.Name,
				"description": d.Description,
			},
			"charts":    charts,
			"templates": templates,
		}
		s.renderTemplate(w, "custom_dashboard_view.html", ctx)
		return
	}

	if len(parts) == 2 && parts[1] == "delete" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.dashboardService.Delete(dashboardID) {
			if wantsJSONResponse(r) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			} else {
				http.Redirect(w, r, "/dashboards", http.StatusFound)
			}
			return
		}
		if wantsJSONResponse(r) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		} else {
			http.Redirect(w, r, "/dashboards", http.StatusFound)
		}
		return
	}

	if len(parts) == 2 && parts[1] == "charts" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := decodeChartCreateRequest(r)
		if err != nil {
			if wantsJSONResponse(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			} else {
				http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
			}
			return
		}
		c, err := s.dashboardService.AddChart(dashboardID, req.Title, req.Type, req.Spec)
		if err != nil {
			if wantsJSONResponse(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			} else {
				http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
			}
			return
		}
		if wantsJSONResponse(r) {
			writeJSON(w, http.StatusCreated, c)
		} else {
			http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
		}
		return
	}

	if len(parts) == 4 && parts[1] == "charts" && parts[3] == "edit" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		req, err := decodeChartCreateRequest(r)
		if err != nil {
			if wantsJSONResponse(r) {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			} else {
				http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
			}
			return
		}
		c, ok := s.dashboardService.EditChart(dashboardID, parts[2], req.Title, req.Type, req.Spec)
		if !ok {
			if wantsJSONResponse(r) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			} else {
				http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
			}
			return
		}
		if wantsJSONResponse(r) {
			writeJSON(w, http.StatusOK, c)
		} else {
			http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
		}
		return
	}

	if len(parts) == 4 && parts[1] == "charts" && parts[3] == "clone" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c, ok := s.dashboardService.CloneChart(dashboardID, parts[2])
		if !ok {
			if wantsJSONResponse(r) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			} else {
				http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
			}
			return
		}
		if wantsJSONResponse(r) {
			writeJSON(w, http.StatusCreated, c)
		} else {
			http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
		}
		return
	}

	if len(parts) == 4 && parts[1] == "charts" && parts[3] == "delete" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.dashboardService.DeleteChart(dashboardID, parts[2]) {
			if wantsJSONResponse(r) {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			} else {
				http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
			}
			return
		}
		if wantsJSONResponse(r) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		} else {
			http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusFound)
		}
		return
	}

	http.NotFound(w, r)
}

func (s *Server) apiDashboardsChartSubroutes(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/dashboards/")
	parts := strings.Split(path, "/")
	if len(parts) < 1 || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	dashboardID := parts[0]

	if len(parts) == 4 && parts[1] == "charts" && parts[3] == "export" {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c, ok := s.dashboardService.ExportChart(dashboardID, parts[2])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, c)
		return
	}

	if len(parts) == 3 && parts[1] == "charts" && parts[2] == "import" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req chartImportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		count := s.dashboardService.ImportCharts(dashboardID, req.Items)
		if count == 0 {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"imported": count})
		return
	}

	http.NotFound(w, r)
}

func decodeDashboardCreate(r *http.Request) (name string, description string, err error, asJSON bool) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "multipart/form-data") {
		if parseErr := r.ParseForm(); parseErr != nil {
			return "", "", parseErr, false
		}
		return strings.TrimSpace(r.Form.Get("name")), strings.TrimSpace(r.Form.Get("description")), nil, false
	}

	if strings.Contains(contentType, "application/json") || wantsJSONResponse(r) {
		asJSON = true
		var req dashboardCreateRequest
		if decodeErr := json.NewDecoder(r.Body).Decode(&req); decodeErr != nil {
			if decodeErr == io.EOF {
				return "", "", decodeErr, true
			}
			return "", "", decodeErr, true
		}
		return strings.TrimSpace(req.Name), strings.TrimSpace(req.Description), nil, true
	}
	if parseErr := r.ParseForm(); parseErr != nil {
		return "", "", parseErr, false
	}
	return strings.TrimSpace(r.Form.Get("name")), strings.TrimSpace(r.Form.Get("description")), nil, false
}

func decodeChartCreateRequest(r *http.Request) (chartCreateRequest, error) {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "multipart/form-data") {
		if err := r.ParseForm(); err != nil {
			return chartCreateRequest{}, err
		}

		title := strings.TrimSpace(r.Form.Get("title"))
		query := strings.TrimSpace(r.Form.Get("query"))
		templateID := strings.TrimSpace(r.Form.Get("template_id"))
		specJSON := strings.TrimSpace(r.Form.Get("chart_spec_json"))

		spec := map[string]any{}
		if specJSON != "" {
			if err := json.Unmarshal([]byte(specJSON), &spec); err != nil {
				return chartCreateRequest{}, err
			}
		}
		if query != "" {
			spec["query"] = query
		}
		if templateID != "" {
			spec["template_id"] = templateID
		}
		chartType := strings.TrimSpace(anyToString(spec["template_id"]))
		if chartType == "" {
			chartType = templateID
		}
		if chartType == "" {
			chartType = "line"
		}

		return chartCreateRequest{Title: title, Type: chartType, Spec: spec}, nil
	}

	if strings.Contains(contentType, "application/json") || wantsJSONResponse(r) {
		var req chartCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			return chartCreateRequest{}, err
		}
		if req.Spec == nil {
			req.Spec = map[string]any{}
		}
		return req, nil
	}
	return chartCreateRequest{}, io.EOF
}

func wantsJSONResponse(r *http.Request) bool {
	contentType := strings.ToLower(strings.TrimSpace(r.Header.Get("Content-Type")))
	accept := strings.ToLower(strings.TrimSpace(r.Header.Get("Accept")))
	if strings.Contains(contentType, "application/x-www-form-urlencoded") || strings.Contains(contentType, "multipart/form-data") {
		return false
	}
	if strings.Contains(contentType, "application/json") || strings.Contains(accept, "application/json") {
		return true
	}
	if strings.Contains(accept, "text/html") {
		return false
	}
	return true
}

func (s *Server) listDashboardCharts(r *http.Request, dashboardID string) []map[string]any {
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return []map[string]any{}
	}
	defer store.Close()

	rows, err := store.Query(r.Context(), "SELECT Id, Title, ChartType, Query, OptionsJson, Position FROM sobs_chart_configs FINAL WHERE IsDeleted = 0 AND DashboardId = ? ORDER BY Position, Id", dashboardID)
	if err != nil {
		return []map[string]any{}
	}
	defer rows.Close()

	charts := []map[string]any{}
	for rows.Next() {
		var id, title, chartType, query, optionsJSON any
		var position any
		if scanErr := rows.Scan(&id, &title, &chartType, &query, &optionsJSON, &position); scanErr != nil {
			continue
		}

		rawOptions := anyToString(optionsJSON)
		parsed := map[string]any{}
		if rawOptions != "" {
			_ = json.Unmarshal([]byte(rawOptions), &parsed)
		}
		chartSpec := map[string]any{}
		if nested, ok := parsed["chart_spec"].(map[string]any); ok {
			chartSpec = nested
		} else {
			chartSpec = map[string]any{
				"template_id": anyToString(chartType),
				"query":       anyToString(query),
			}
		}

		charts = append(charts, map[string]any{
			"id":           anyToString(id),
			"title":        anyToString(title),
			"chart_type":   anyToString(chartType),
			"query":        anyToString(query),
			"options_json": rawOptions,
			"position":     anyToInt(position),
			"chart_spec":   chartSpec,
		})
	}
	return charts
}

func (s *Server) dashboardTemplateContext() []map[string]any {
	raw := s.dashboardService.SpecTemplates()
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		id := strings.TrimSpace(anyToString(item["id"]))
		if id == "" {
			continue
		}
		name := strings.TrimSpace(anyToString(item["name"]))
		if name == "" {
			name = strings.TrimSpace(anyToString(item["label"]))
		}
		if name == "" {
			name = id
		}
		out = append(out, map[string]any{
			"id":          id,
			"name":        name,
			"description": strings.TrimSpace(anyToString(item["description"])),
			"icon":        strings.TrimSpace(anyToString(item["icon"])),
			"query_shape": strings.TrimSpace(anyToString(item["query_shape"])),
			"sample_sql":  strings.TrimSpace(anyToString(item["sample_sql"])),
			"drilldown":   item["drilldown"],
			"default_spec": map[string]any{
				"template_id": id,
				"query":       "",
			},
		})
	}
	return out
}
