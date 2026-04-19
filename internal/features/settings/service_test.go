package settings

import "testing"

func TestSaveAIAndEnrichment(t *testing.T) {
	svc := NewService()
	svc.SaveAI(map[string]string{"provider": "openai", "model": "gpt-4.1"})
	ai := svc.AI()
	if ai["provider"] != "openai" {
		t.Fatal("expected provider")
	}
	svc.SaveEnrichment(true, false, 999)
	enrichment := svc.Enrichment()
	if enrichment["github_backfill_max_releases"] != "500" {
		t.Fatal("expected max releases clamp")
	}
	if enrichment["cve_enabled"] != "false" {
		t.Fatal("expected cve disabled")
	}
}

func TestSortedActions(t *testing.T) {
	got := SortedActions(map[string]bool{"summarize": true, "analyze": true})
	if len(got) != 2 {
		t.Fatal("expected two actions")
	}
}
