package web

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/features/persist"
	"github.com/abartrim/sobs/internal/features/reports"
)

type createReportRequest struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	PageType    string         `json:"page_type"`
	Filters     map[string]any `json:"filters"`
}

type importReportsRequest struct {
	SOBSReportsExport bool             `json:"sobs_reports_export"`
	Version           string           `json:"version"`
	Reports           []reports.Report `json:"reports"`
}

const reportsExportVersion = "1"

func (s *Server) apiReports(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		pageType := strings.TrimSpace(r.URL.Query().Get("page_type"))
		items := s.reportService.List()
		if pageType != "" {
			items = s.reportService.ListByPageType(pageType)
		}
		writeJSON(w, http.StatusOK, items)
	case http.MethodPost:
		var req createReportRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		rpt, err := s.reportService.Create(reports.Report{
			Name:        strings.TrimSpace(req.Name),
			Description: strings.TrimSpace(req.Description),
			PageType:    strings.TrimSpace(req.PageType),
			Filters:     req.Filters,
		})
		if err != nil {
			message := err.Error()
			if message == "page_type is invalid" {
				message = "page_type must be one of: ai, errors, logs, metrics, rum, traces, web_traffic, work_items"
			}
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": message})
			return
		}
		writeJSON(w, http.StatusCreated, rpt)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiReportsSubroutes(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/reports/")
	if id == "" || strings.Contains(id, "/") {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.reportService.Delete(id) {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) apiReportsExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rawIDs := strings.TrimSpace(r.URL.Query().Get("ids"))
	items := s.reportService.List()
	if rawIDs != "" {
		wanted := map[string]struct{}{}
		for _, id := range strings.Split(rawIDs, ",") {
			trimmed := strings.TrimSpace(id)
			if trimmed != "" {
				wanted[trimmed] = struct{}{}
			}
		}
		filtered := make([]reports.Report, 0, len(items))
		for _, item := range items {
			if _, ok := wanted[item.ID]; ok {
				filtered = append(filtered, item)
			}
		}
		items = filtered
	}
	payload := map[string]any{
		"sobs_reports_export": true,
		"version":             reportsExportVersion,
		"exported_at":         persist.RFC3339Now(),
		"reports":             items,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="sobs_reports_export.json"`)
	writeJSON(w, http.StatusOK, payload)
}

func (s *Server) apiReportsImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req importReportsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	if !req.SOBSReportsExport {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Not a valid SOBS reports export file"})
		return
	}
	if req.Version != reportsExportVersion {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "Unsupported export version: " + req.Version})
		return
	}
	onConflict := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("on_conflict")))
	if onConflict == "" {
		onConflict = "rename"
	}
	if onConflict != "rename" && onConflict != "replace" && onConflict != "skip" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "on_conflict must be one of: rename, replace, skip"})
		return
	}
	existing := s.reportService.List()
	existingIndex := map[string]reports.Report{}
	for _, item := range existing {
		existingIndex[item.PageType+"\x00"+strings.ToLower(item.Name)] = item
	}
	imported := 0
	skipped := 0
	replaced := 0
	errorsCount := 0
	for _, item := range req.Reports {
		name := strings.TrimSpace(item.Name)
		pageType := strings.TrimSpace(item.PageType)
		if name == "" || pageType == "" {
			errorsCount++
			continue
		}
		key := pageType + "\x00" + strings.ToLower(name)
		if conflict, ok := existingIndex[key]; ok {
			switch onConflict {
			case "skip":
				skipped++
				continue
			case "replace":
				s.reportService.Delete(conflict.ID)
				delete(existingIndex, key)
				replaced++
			case "rename":
				candidate := name + " (imported)"
				suffix := 2
				for {
					candidateKey := pageType + "\x00" + strings.ToLower(candidate)
					if _, exists := existingIndex[candidateKey]; !exists {
						name = candidate
						key = candidateKey
						break
					}
					candidate = name + " (imported " + strconv.Itoa(suffix) + ")"
					suffix++
				}
			}
		}
		created, err := s.reportService.Create(reports.Report{
			Name:        name,
			Description: strings.TrimSpace(item.Description),
			PageType:    pageType,
			Filters:     item.Filters,
		})
		if err != nil {
			errorsCount++
			continue
		}
		existingIndex[key] = created
		if onConflict == "replace" {
			continue
		}
		imported++
	}
	writeJSON(w, http.StatusOK, map[string]any{"imported": imported, "skipped": skipped, "replaced": replaced, "errors": errorsCount})
}
