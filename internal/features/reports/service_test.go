package reports

import "testing"

func TestCreateListDelete(t *testing.T) {
	svc := NewService()
	r, err := svc.Create(Report{Name: "daily", Description: "saved logs", PageType: "logs", Filters: map[string]any{"q": "select 1"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	items := svc.List()
	if len(items) != 1 {
		t.Fatal("expected one report")
	}
	if items[0].PageType != "logs" || items[0].Description != "saved logs" {
		t.Fatalf("expected report metadata to round-trip, got %#v", items[0])
	}
	if !svc.Delete(r.ID) {
		t.Fatal("expected delete true")
	}
	if len(svc.List()) != 0 {
		t.Fatal("expected zero reports")
	}
}
