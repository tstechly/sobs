package web

import (
	"encoding/json"
	"fmt"
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
		if r.URL.Path != "/settings/data-management" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"dm_settings": s.dataManagementService.GetSettings()})
			return
		}
		st := s.dataManagementService.GetSettings()
		dmSettings := map[string]string{
			"data_management.backup_enabled":        boolToSetting(st.BackupEnabled),
			"data_management.s3_bucket":             st.S3Bucket,
			"data_management.ttl_logs_days":         strconv.Itoa(st.TTLLogsDays),
			"data_management.ttl_traces_days":       strconv.Itoa(st.TTLTracesDays),
			"data_management.ttl_metrics_hours":     strconv.Itoa(st.TTLMetricsHours),
			"data_management.ttl_sessions_days":     strconv.Itoa(st.TTLSessionsDays),
			"data_management.ttl_backup_coupling_enabled": "0",
			"data_management.s3_region":                  "",
			"data_management.s3_path_prefix":             "",
			"data_management.s3_access_key_id":           "",
			"data_management.s3_encrypt_backup":          "0",
			"data_management.backup_schedule_full":       "",
			"data_management.backup_schedule_incremental": "",
		}
		ctx := map[string]any{
			"title":                 "Data Management Settings",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "settings/data-management"},
			"dm_settings":           dmSettings,
			"dm_secret_present": map[string]bool{
				"s3_secret_access_key":      false,
				"backup_encryption_password": false,
			},
			"flash_msg":  "",
			"flash_type": "info",
			"db_stats": map[string]any{
				"compressed_bytes":   0,
				"uncompressed_bytes": 0,
				"compression_ratio":  nil,
				"total_rows":         nil,
				"active_queries":     nil,
				"tables":             []map[string]any{},
			},
			"fmt_bytes": func(v any) string {
				switch n := v.(type) {
				case int:
					return fmt.Sprintf("%d B", n)
				case int64:
					return fmt.Sprintf("%d B", n)
				case float64:
					return fmt.Sprintf("%.0f B", n)
				default:
					return "0 B"
				}
			},
		}
		s.renderTemplate(w, "settings_data_management.html", ctx)
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

func boolToSetting(enabled bool) string {
	if enabled {
		return "1"
	}
	return "0"
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
