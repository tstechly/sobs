package apps

import "testing"

func TestCreateAndGetApp(t *testing.T) {
	s := NewService()
	item, err := s.CreateApp(CreateAppInput{Name: "demo", Slug: "demo-app", OwnerTeam: "platform"})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	got, ok := s.GetApp(item.ID)
	if !ok {
		t.Fatal("expected app to exist")
	}
	if got.Name != "demo" || got.Slug != "demo-app" || got.OwnerTeam != "platform" {
		t.Fatalf("unexpected app payload: %#v", got)
	}
}

func TestPatchAppUpdatesFields(t *testing.T) {
	s := NewService()
	created, err := s.CreateApp(CreateAppInput{Name: "demo"})
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	name := "demo-2"
	enabled := false
	updated, err := s.PatchApp(created.ID, PatchAppInput{Name: &name, Enabled: &enabled, Metadata: map[string]any{"tier": "standard"}, HasMetadata: true})
	if err != nil {
		t.Fatalf("patch app: %v", err)
	}
	if updated.Name != "demo-2" || updated.Enabled != false || updated.Metadata["tier"] != "standard" {
		t.Fatalf("unexpected updated app payload: %#v", updated)
	}
}

func TestCreateReleaseRequiresExistingApp(t *testing.T) {
	s := NewService()
	_, err := s.CreateRelease("404", CreateReleaseInput{Version: "v1"})
	if err != ErrAppNotFound {
		t.Fatalf("expected app not found, got %v", err)
	}
}