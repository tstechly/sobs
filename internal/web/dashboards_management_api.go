package web

import (
	"encoding/json"
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
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"items": s.dashboardService.List()})
			return
		}
		s.pageTemplateHandler("/dashboards", "custom_dashboards.html")(w, r)
	case http.MethodPost:
		var req dashboardCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		d, err := s.dashboardService.Create(req.Name, req.Description)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, d)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) dashboardsNew(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"form": "new-dashboard"})
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
		d, ok := s.dashboardService.Get(dashboardID)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, d)
		return
	}

	if len(parts) == 2 && parts[1] == "delete" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.dashboardService.Delete(dashboardID) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}

	if len(parts) == 2 && parts[1] == "charts" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req chartCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		c, err := s.dashboardService.AddChart(dashboardID, req.Title, req.Type, req.Spec)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusCreated, c)
		return
	}

	if len(parts) == 4 && parts[1] == "charts" && parts[3] == "edit" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req chartCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		c, ok := s.dashboardService.EditChart(dashboardID, parts[2], req.Title, req.Type, req.Spec)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, c)
		return
	}

	if len(parts) == 4 && parts[1] == "charts" && parts[3] == "clone" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		c, ok := s.dashboardService.CloneChart(dashboardID, parts[2])
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusCreated, c)
		return
	}

	if len(parts) == 4 && parts[1] == "charts" && parts[3] == "delete" {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if !s.dashboardService.DeleteChart(dashboardID, parts[2]) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
