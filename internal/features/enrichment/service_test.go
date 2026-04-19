package enrichment

import "testing"

func TestTrafficAndFindings(t *testing.T) {
	svc := NewService()
	// Traffic aggregations return non-nil slices (may be empty against in-memory store).
	if svc.Geo() == nil || svc.Browsers() == nil || svc.OS() == nil {
		t.Fatal("expected non-nil traffic slices")
	}
	// ListFindings returns a non-nil slice; empty is valid when no scan has run.
	if svc.ListFindings() == nil {
		t.Fatal("expected non-nil findings slice")
	}
	// Scan records a timestamp and returns ok=true.
	if svc.Scan()["ok"] != true {
		t.Fatal("expected scan ok")
	}
}
