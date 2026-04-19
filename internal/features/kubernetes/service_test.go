package kubernetes

import "testing"

func TestSettingsAndStatus(t *testing.T) {
	svc := NewService()
	st := svc.SaveSettings(true, "prod")
	if !st.Enabled || st.DefaultNamespace != "prod" {
		t.Fatal("expected saved settings")
	}
	status := svc.Status()
	if status["ok"] != true {
		t.Fatal("expected ok status")
	}
}
