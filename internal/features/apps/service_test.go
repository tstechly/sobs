package apps

import "testing"

func TestCreateAndGetApp(t *testing.T) {
	s := NewService()
	a, err := s.CreateApp("demo")
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	got, ok := s.GetApp(a.ID)
	if !ok {
		t.Fatal("expected app to exist")
	}
	if got.Name != "demo" {
		t.Fatalf("expected name demo, got %s", got.Name)
	}
}

func TestCreateReleaseRequiresExistingApp(t *testing.T) {
	s := NewService()
	_, err := s.CreateRelease("404", "v1")
	if err == nil {
		t.Fatal("expected error")
	}
}
