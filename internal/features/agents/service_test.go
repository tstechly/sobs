package agents

import "testing"

func TestRunLifecycleAndIssueRaise(t *testing.T) {
	svc := NewService()
	r, err := svc.CreateRun("first run")
	if err != nil {
		t.Fatalf("create run: %v", err)
	}
	if _, ok := svc.DismissRun(r.ID); !ok {
		t.Fatal("expected dismiss ok")
	}
	if len(svc.ListRuns()) != 1 {
		t.Fatal("expected one run")
	}
	if _, err := svc.RaiseIssue("bug", "details"); err != nil {
		t.Fatalf("raise issue: %v", err)
	}
}

func TestRuleLifecycle(t *testing.T) {
	svc := NewService()
	r, err := svc.CreateRule("Investigate spike", "", "manual", "", "any", []string{"analyze"}, 60)
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if r.ID == "" {
		t.Fatal("expected rule id")
	}
	if len(svc.ListRules()) != 1 {
		t.Fatal("expected one rule")
	}
	if !svc.DeleteRule(r.ID) {
		t.Fatal("expected delete rule")
	}
}
