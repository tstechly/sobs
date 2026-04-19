package metrics

import "testing"

func TestRuleLifecycleAndAnomaly(t *testing.T) {
	svc := NewService()
	r, err := svc.CreateRule("r1", "q", "> 1")
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if len(svc.ListRules()) != 1 {
		t.Fatal("expected one rule")
	}
	if len(svc.AutoRules()) == 0 || len(svc.AutoDashboardRules()) == 0 {
		t.Fatal("expected generated rules")
	}
	if !svc.DeleteRule(r.ID) {
		t.Fatal("expected delete")
	}
	if svc.AnomalySnapshot()["ok"] != true {
		t.Fatal("expected anomaly ok")
	}
}
