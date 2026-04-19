package tags

import "testing"

func TestTagsRulesAndRecordTags(t *testing.T) {
	svc := NewService()
	r, err := svc.CreateRule(RuleInput{
		Name:        "rule",
		RecordTypes: []string{"log", "error"},
		Conditions:  []Condition{{MatchField: "severity", MatchOperator: "eq", MatchValue: "ERROR"}},
		TagKey:      "priority",
		TagValue:    "high",
	})
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	if len(svc.ListRules()) != 1 {
		t.Fatal("expected one rule")
	}
	if !svc.SetRecordTag("logs", "1", "priority", "high") {
		t.Fatal("expected set tag")
	}
	tags := svc.GetRecordTags("logs", "1")
	if tags["priority"] != "high" {
		t.Fatal("expected priority tag")
	}
	if !svc.DeleteRecordTag("logs", "1", "priority") {
		t.Fatal("expected delete tag")
	}
	if !svc.DeleteRule(r.ID) {
		t.Fatal("expected delete rule")
	}
}
