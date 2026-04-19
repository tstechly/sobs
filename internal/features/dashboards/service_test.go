package dashboards

import "testing"

func TestList(t *testing.T) {
	svc := NewService()
	items := svc.List()
	if len(items) == 0 {
		t.Fatal("expected seeded dashboard")
	}
}

func TestDashboardAndChartLifecycle(t *testing.T) {
	svc := NewService()
	d, err := svc.Create("Ops", "Ops dashboard")
	if err != nil {
		t.Fatalf("create dashboard: %v", err)
	}
	c, err := svc.AddChart(d.ID, "Latency", "line", map[string]any{"metric": "latency"})
	if err != nil {
		t.Fatalf("add chart: %v", err)
	}
	if _, ok := svc.EditChart(d.ID, c.ID, "Latency P95", "line", nil); !ok {
		t.Fatal("expected edit chart")
	}
	if _, ok := svc.CloneChart(d.ID, c.ID); !ok {
		t.Fatal("expected clone chart")
	}
	if _, ok := svc.ExportChart(d.ID, c.ID); !ok {
		t.Fatal("expected export chart")
	}
	if !svc.DeleteChart(d.ID, c.ID) {
		t.Fatal("expected delete chart")
	}
	if !svc.Delete(d.ID) {
		t.Fatal("expected delete dashboard")
	}
}

func TestSpecHelpers(t *testing.T) {
	svc := NewService()
	if len(svc.SpecTemplates()) == 0 {
		t.Fatal("expected spec templates")
	}
	if len(svc.SpecOptions()) == 0 {
		t.Fatal("expected spec options")
	}
	spec := svc.BuildSpec("Error trend")
	ok, msg := svc.ValidateSpec(spec)
	if !ok || msg != "" {
		t.Fatalf("expected valid spec, got ok=%v msg=%q", ok, msg)
	}
	rendered := svc.RenderSpec(spec)
	if rendered["ok"] != true {
		t.Fatal("expected rendered ok")
	}
}
