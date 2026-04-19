package repositories

import "testing"

func TestRepositoryLifecycle(t *testing.T) {
	svc := NewService()
	r, err := svc.Create("repo", "https://github.com/acme/repo")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, ok := svc.SetRealtime(r.ID, true); !ok {
		t.Fatal("expected realtime set")
	}
	rot, ok := svc.RotateCIIngestKey(r.ID)
	if !ok || rot.CIIngestKey == "" {
		t.Fatal("expected rotated key")
	}
	rev, ok := svc.RevokeCIIngestKey(r.ID)
	if !ok || rev.CIIngestKey != "" {
		t.Fatal("expected revoked key")
	}
	if _, ok := svc.AddRelease(r.ID, "1.0.0"); !ok {
		t.Fatal("expected release add")
	}
	if !svc.Delete(r.ID) {
		t.Fatal("expected delete")
	}
}

func TestValidateGitHubToken(t *testing.T) {
	if ValidateGitHubToken("short") {
		t.Fatal("expected short token invalid")
	}
	if !ValidateGitHubToken("ghp_123456789012345") {
		t.Fatal("expected valid token")
	}
}
