package workitems

import "testing"

func TestList(t *testing.T) {
	svc := NewService()
	items := svc.List()
	if len(items) == 0 {
		t.Fatal("expected seeded work item")
	}
}
