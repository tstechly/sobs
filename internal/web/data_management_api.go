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
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		st := s.dataManagementService.GetSettings()
		dmSettings := map[string]string{
			"data_management.backup_enabled":              boolToSetting(st.BackupEnabled),
			"data_management.s3_bucket":                   st.S3Bucket,
			"data_management.s3_access_key_id":            st.S3AccessKeyID,
			"data_management.s3_region":                   st.S3Region,
			"data_management.s3_path_prefix":              st.S3PathPrefix,
			"data_management.s3_encrypt_backup":           boolToSetting(st.S3EncryptBackup),
			"data_management.backup_schedule_full":        st.BackupScheduleFull,
			"data_management.backup_schedule_incremental": st.BackupScheduleIncremental,
			"data_management.ttl_logs_days":               strconv.Itoa(st.TTLLogsDays),
			"data_management.ttl_traces_days":             strconv.Itoa(st.TTLTracesDays),
			"data_management.ttl_metrics_hours":           strconv.Itoa(st.TTLMetricsHours),
			"data_management.ttl_sessions_days":           strconv.Itoa(st.TTLSessionsDays),
			"data_management.ttl_backup_coupling_enabled": boolToSetting(st.TTLBackupCouplingEnabled),
		}
		ctx := map[string]any{
			"title":                 "Data Management Settings",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "settings/data-management"},
			"dm_settings":           dmSettings,
			"dm_secret_present": map[string]bool{
				// Sensitive fields are stored as-is (plaintext); show only
				// whether a value is present, not the value itself.
				"s3_secret_access_key":       st.S3SecretAccessKey != "",
				"backup_encryption_password": st.BackupEncryptionPassword != "",
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
			BackupEnabled:             parseBool(vals["backup_enabled"]),
			S3Bucket:                  vals["s3_bucket"],
			S3AccessKeyID:             vals["s3_access_key_id"],
			S3SecretAccessKey:         vals["s3_secret_access_key"],
			S3Region:                  vals["s3_region"],
			S3PathPrefix:              vals["s3_path_prefix"],
			S3EncryptBackup:           parseBool(vals["s3_encrypt_backup"]),
			BackupEncryptionPassword:  vals["backup_encryption_password"],
			BackupScheduleFull:        vals["backup_schedule_full"],
			BackupScheduleIncremental: vals["backup_schedule_incremental"],
			TTLLogsDays:               parseIntDefault(vals["ttl_logs_days"], 30),
			TTLTracesDays:             parseIntDefault(vals["ttl_traces_days"], 30),
			TTLMetricsHours:           parseIntDefault(vals["ttl_metrics_hours"], 168),
			TTLSessionsDays:           parseIntDefault(vals["ttl_sessions_days"], 30),
			TTLBackupCouplingEnabled:  parseBool(vals["ttl_backup_coupling_enabled"]),
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
	if r.Body != nil {
		_ = json.NewDecoder(r.Body).Decode(&req)
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
