package enrichment

import "testing"

func TestTrafficAndFindings(t *testing.T) {
	svc := NewService()
	if len(svc.Geo()) == 0 || len(svc.Browsers()) == 0 || len(svc.OS()) == 0 {
		t.Fatal("expected traffic slices")
	}
	if len(svc.ListFindings()) == 0 {
		t.Fatal("expected findings")
	}
	f, ok := svc.SetDisposition("OSV-2026-0001", "accepted-risk")
	if !ok || f.Disposition != "accepted-risk" {
		t.Fatal("expected disposition update")
	}
	if svc.Scan()["ok"] != true {
		t.Fatal("expected scan ok")
	}
}
