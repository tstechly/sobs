package notifications

import "testing"

func TestSubscribe(t *testing.T) {
	svc := NewService()
	sub, err := svc.Subscribe("https://example.com/push")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	if sub.ID == "" {
		t.Fatal("expected id")
	}
}

func TestVAPIDLifecycle(t *testing.T) {
	svc := NewService()
	if got := svc.VAPIDPublicKey(); got != "" {
		t.Fatalf("expected empty public key, got %q", got)
	}
	pub, priv := svc.GenerateVAPIDKeys()
	if pub == "" || priv == "" {
		t.Fatal("expected keys")
	}
	if got := svc.VAPIDPublicKey(); got == "" {
		t.Fatal("expected stored public key")
	}
	svc.DeleteVAPIDKeys()
	if got := svc.VAPIDPublicKey(); got != "" {
		t.Fatalf("expected empty after delete, got %q", got)
	}
}

func TestToggleAndDeleteSubscription(t *testing.T) {
	svc := NewService()
	sub, err := svc.Subscribe("https://example.com/push")
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	toggled, ok := svc.ToggleSubscription(sub.ID)
	if !ok {
		t.Fatal("expected toggle ok")
	}
	if toggled.Enabled {
		t.Fatal("expected toggled disabled")
	}
	if !svc.DeleteSubscription(sub.ID) {
		t.Fatal("expected delete true")
	}
}

func TestRulesLifecycle(t *testing.T) {
	svc := NewService()
	r, err := svc.CreateRule("critical-errors")
	if err != nil {
		t.Fatalf("create rule: %v", err)
	}
	toggled, ok := svc.ToggleRule(r.ID)
	if !ok {
		t.Fatal("expected toggle ok")
	}
	if toggled.Enabled {
		t.Fatal("expected toggled disabled")
	}
	if !svc.DeleteRule(r.ID) {
		t.Fatal("expected delete true")
	}
	gen := svc.AutoGenerateRules()
	if len(gen) == 0 {
		t.Fatal("expected generated rules")
	}
}
