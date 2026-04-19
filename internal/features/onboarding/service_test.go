package onboarding

import "testing"

func TestRepoAndIssueFlow(t *testing.T) {
	svc := NewService()
	r, err := svc.CreateRepo("demo", "", "https://github.com/acme/demo", "", "")
	if err != nil {
		t.Fatalf("create repo: %v", err)
	}
	if r.AppID == "" {
		t.Fatal("expected app id")
	}
	if _, err := svc.ImportRepo("", "acme", "demo"); err != nil {
		t.Fatalf("import repo: %v", err)
	}
	if len(svc.ListRepos("acme")) == 0 {
		t.Fatal("expected repo list")
	}
	_, code, _ := svc.InspectRepo(r.AppID, "")
	if code != 200 {
		t.Fatal("expected inspect ok")
	}
	res, code, _ := svc.CreateIssues(r.AppID, "", true, true)
	if code != 200 || res["ok"] != true {
		t.Fatal("expected create issues ok")
	}
}
