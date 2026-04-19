package web

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/abartrim/sobs/internal/features/datamanagement"
)

type dmBackupRunRequest struct {
	Type string `json:"type"`
}

type dmRestoreRequest struct {
	BackupName string `json:"backup_name"`
}

func (s *Server) settingsDataManagement(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"dm_settings": s.dataManagementService.GetSettings()})
			return
		}
		s.pageTemplateHandler("/settings/data-management", "settings_data_management.html")(w, r)
	case http.MethodPost:
		vals, err := decodeStringMap(r)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		st := datamanagement.Settings{
			BackupEnabled:   parseBool(vals["backup_enabled"]),
			S3Bucket:        vals["s3_bucket"],
			TTLLogsDays:     parseIntDefault(vals["ttl_logs_days"], 30),
			TTLTracesDays:   parseIntDefault(vals["ttl_traces_days"], 30),
			TTLMetricsHours: parseIntDefault(vals["ttl_metrics_hours"], 168),
			TTLSessionsDays: parseIntDefault(vals["ttl_sessions_days"], 30),
		}
		writeJSON(w, http.StatusOK, s.dataManagementService.SaveSettings(st))
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) apiDataManagementBackupList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "backups": s.dataManagementService.ListBackups()})
}

func (s *Server) apiDataManagementBackupRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req dmBackupRunRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	kind := req.Type
	if kind != "incremental" {
		kind = "full"
	}
	backup, ok, msg := s.dataManagementService.RunBackup(kind)
	if !ok {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "message": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg, "backup": backup})
}

func (s *Server) apiDataManagementRestore(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req dmRestoreRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
		return
	}
	ok, msg := s.dataManagementService.Restore(req.BackupName)
	if !ok {
		code := http.StatusBadRequest
		if msg == "Backup feature is disabled" {
			code = http.StatusForbidden
		}
		writeJSON(w, code, map[string]any{"ok": false, "message": msg})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "message": msg})
}

func parseIntDefault(raw string, def int) int {
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return def
	}
	return v
}
