package workitems

import "testing"

func TestList(t *testing.T) {
	svc := NewService()
	items := svc.List()
	if items == nil {
		t.Fatal("expected non-nil work item slice")
	}
}
