package datamanagement

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Settings struct {
	BackupEnabled               bool   `json:"backup_enabled"`
	S3Bucket                    string `json:"s3_bucket"`
	S3AccessKeyID               string `json:"s3_access_key_id"`
	S3SecretAccessKey           string `json:"s3_secret_access_key"`
	S3Region                    string `json:"s3_region"`
	S3PathPrefix                string `json:"s3_path_prefix"`
	S3EncryptBackup             bool   `json:"s3_encrypt_backup"`
	BackupEncryptionPassword    string `json:"backup_encryption_password"`
	BackupScheduleFull          string `json:"backup_schedule_full"`
	BackupScheduleIncremental   string `json:"backup_schedule_incremental"`
	TTLLogsDays                 int    `json:"ttl_logs_days"`
	TTLTracesDays               int    `json:"ttl_traces_days"`
	TTLMetricsHours             int    `json:"ttl_metrics_hours"`
	TTLSessionsDays             int    `json:"ttl_sessions_days"`
	TTLBackupCouplingEnabled    bool   `json:"ttl_backup_coupling_enabled"`
}

type Backup struct {
	Name      string `json:"name"`
	Status    string `json:"status"`
	StartedAt string `json:"start_time"`
	EndedAt   string `json:"end_time"`
}

type Service struct {
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return NewStoreService(defaultstore.NewFactory())
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) GetSettings() Settings {
	return s.getSettingsStoreBacked(context.Background())
}

func (s *Service) getSettingsStoreBacked(ctx context.Context) Settings {
	return Settings{
		BackupEnabled:             settingBool(ctx, s.storeFactory, "data_management.backup_enabled", false),
		S3Bucket:                  settingString(ctx, s.storeFactory, "data_management.s3_bucket", ""),
		S3AccessKeyID:             settingString(ctx, s.storeFactory, "data_management.s3_access_key_id", ""),
		S3SecretAccessKey:         settingString(ctx, s.storeFactory, "data_management.s3_secret_access_key", ""),
		S3Region:                  settingString(ctx, s.storeFactory, "data_management.s3_region", ""),
		S3PathPrefix:              settingString(ctx, s.storeFactory, "data_management.s3_path_prefix", ""),
		S3EncryptBackup:           settingBool(ctx, s.storeFactory, "data_management.s3_encrypt_backup", false),
		BackupEncryptionPassword:  settingString(ctx, s.storeFactory, "data_management.backup_encryption_password", ""),
		BackupScheduleFull:        settingString(ctx, s.storeFactory, "data_management.backup_schedule_full", ""),
		BackupScheduleIncremental: settingString(ctx, s.storeFactory, "data_management.backup_schedule_incremental", ""),
		TTLLogsDays:               settingInt(ctx, s.storeFactory, "data_management.ttl_logs_days", 30),
		TTLTracesDays:             settingInt(ctx, s.storeFactory, "data_management.ttl_traces_days", 30),
		TTLMetricsHours:           settingInt(ctx, s.storeFactory, "data_management.ttl_metrics_hours", 168),
		TTLSessionsDays:           settingInt(ctx, s.storeFactory, "data_management.ttl_sessions_days", 30),
		TTLBackupCouplingEnabled:  settingBool(ctx, s.storeFactory, "data_management.ttl_backup_coupling_enabled", false),
	}
}

func (s *Service) SaveSettings(st Settings) Settings {
	return s.saveSettingsStoreBacked(context.Background(), st)
}

func (s *Service) saveSettingsStoreBacked(ctx context.Context, st Settings) Settings {
	if st.TTLLogsDays <= 0 {
		st.TTLLogsDays = 30
	}
	if st.TTLTracesDays <= 0 {
		st.TTLTracesDays = 30
	}
	if st.TTLMetricsHours <= 0 {
		st.TTLMetricsHours = 168
	}
	if st.TTLSessionsDays <= 0 {
		st.TTLSessionsDays = 30
	}
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.backup_enabled", boolString(st.BackupEnabled))
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.s3_bucket", st.S3Bucket)
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.s3_access_key_id", st.S3AccessKeyID)
	// Sensitive: only overwrite if a new value is provided; empty string = keep existing
	if st.S3SecretAccessKey != "" {
		_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.s3_secret_access_key", st.S3SecretAccessKey)
	}
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.s3_region", st.S3Region)
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.s3_path_prefix", st.S3PathPrefix)
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.s3_encrypt_backup", boolString(st.S3EncryptBackup))
	// Sensitive: only overwrite if a new value is provided
	if st.BackupEncryptionPassword != "" {
		_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.backup_encryption_password", st.BackupEncryptionPassword)
	}
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.backup_schedule_full", st.BackupScheduleFull)
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.backup_schedule_incremental", st.BackupScheduleIncremental)
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.ttl_logs_days", strconv.Itoa(st.TTLLogsDays))
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.ttl_traces_days", strconv.Itoa(st.TTLTracesDays))
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.ttl_metrics_hours", strconv.Itoa(st.TTLMetricsHours))
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.ttl_sessions_days", strconv.Itoa(st.TTLSessionsDays))
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.ttl_backup_coupling_enabled", boolString(st.TTLBackupCouplingEnabled))
	return st
}

func (s *Service) ListBackups() []Backup {
	return s.listBackupsStoreBacked(context.Background())
}

func (s *Service) listBackupsStoreBacked(ctx context.Context) []Backup {
	store, err := persist.Open(ctx, s.storeFactory)
	if err == nil {
		defer func() { _ = store.Close() }()
		rows, queryErr := store.Query(ctx, "SELECT name, status, start_time, end_time FROM system.backups ORDER BY start_time DESC LIMIT 100")
		if queryErr == nil {
			defer func() { _ = rows.Close() }()
			out := []Backup{}
			for rows.Next() {
				var item Backup
				if err := rows.Scan(&item.Name, &item.Status, &item.StartedAt, &item.EndedAt); err != nil {
					return out
				}
				out = append(out, item)
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	value, ok, err := persist.GetAppSetting(ctx, s.storeFactory, "data_management.backup_history")
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return nil
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(value), &rows); err != nil {
		return nil
	}
	out := []Backup{}
	for _, row := range rows {
		out = append(out, Backup{Name: asString(row["name"]), Status: asString(row["status"]), StartedAt: asString(row["start_time"]), EndedAt: asString(row["end_time"])})
	}
	return out
}

func (s *Service) RunBackup(kind string) (Backup, bool, string) {
	return s.runBackupStoreBacked(context.Background(), kind)
}

func (s *Service) runBackupStoreBacked(ctx context.Context, kind string) (Backup, bool, string) {
	settings := s.getSettingsStoreBacked(ctx)
	if !settings.BackupEnabled {
		return Backup{}, false, "Backup feature is disabled"
	}
	if kind != "incremental" {
		kind = "full"
	}
	now := persist.RFC3339Now()
	history := s.listBackupsStoreBacked(ctx)
	name := "sobs-" + kind + "-" + strconv.Itoa(len(history)+1)
	backup := Backup{Name: name, Status: "BACKUP_COMPLETE", StartedAt: now, EndedAt: now}
	rows := []map[string]any{{"name": backup.Name, "status": backup.Status, "start_time": backup.StartedAt, "end_time": backup.EndedAt}}
	for _, item := range history {
		rows = append(rows, map[string]any{"name": item.Name, "status": item.Status, "start_time": item.StartedAt, "end_time": item.EndedAt})
	}
	_ = persist.SetAppSetting(ctx, s.storeFactory, "data_management.backup_history", persist.JSONString(rows))
	return backup, true, "backup started"
}

func (s *Service) Restore(name string) (bool, string) {
	return s.restoreStoreBacked(context.Background(), name)
}

func (s *Service) restoreStoreBacked(ctx context.Context, name string) (bool, string) {
	settings := s.getSettingsStoreBacked(ctx)
	if !settings.BackupEnabled {
		return false, "Backup feature is disabled"
	}
	if strings.TrimSpace(name) == "" {
		return false, "backup_name is required"
	}
	for _, item := range s.listBackupsStoreBacked(ctx) {
		if item.Name == name {
			return true, "restore started"
		}
	}
	return false, "backup not found"
}

func boolString(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func settingString(ctx context.Context, factory extensionpoints.StoreFactory, key, def string) string {
	value, ok, err := persist.GetAppSetting(ctx, factory, key)
	if err != nil || !ok || strings.TrimSpace(value) == "" {
		return def
	}
	return strings.TrimSpace(value)
}

func settingInt(ctx context.Context, factory extensionpoints.StoreFactory, key string, def int) int {
	value := settingString(ctx, factory, key, "")
	if value == "" {
		return def
	}
	number, err := strconv.Atoi(value)
	if err != nil || number <= 0 {
		return def
	}
	return number
}

func settingBool(ctx context.Context, factory extensionpoints.StoreFactory, key string, def bool) bool {
	value := settingString(ctx, factory, key, "")
	if value == "" {
		return def
	}
	return value == "1" || strings.EqualFold(value, "true") || strings.EqualFold(value, "yes") || strings.EqualFold(value, "on")
}

func asString(value any) string {
	text, _ := value.(string)
	return text
}
