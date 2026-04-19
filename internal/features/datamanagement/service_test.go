package datamanagement

import "testing"

func TestSettingsBackupRestore(t *testing.T) {
	svc := NewService()
	svc.SaveSettings(Settings{BackupEnabled: true, S3Bucket: "bucket", TTLLogsDays: 7, TTLTracesDays: 7, TTLMetricsHours: 24, TTLSessionsDays: 7})
	b, ok, _ := svc.RunBackup("full")
	if !ok || b.Name == "" {
		t.Fatal("expected backup")
	}
	if len(svc.ListBackups()) == 0 {
		t.Fatal("expected listed backups")
	}
	rok, _ := svc.Restore(b.Name)
	if !rok {
		t.Fatal("expected restore")
	}
}
