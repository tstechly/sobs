package reports

import "testing"

func TestCreateListDelete(t *testing.T) {
	svc := NewService()
	r, err := svc.Create("daily", "select 1")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(svc.List()) != 1 {
		t.Fatal("expected one report")
	}
	if !svc.Delete(r.ID) {
		t.Fatal("expected delete true")
	}
	if len(svc.List()) != 0 {
		t.Fatal("expected zero reports")
	}
}
