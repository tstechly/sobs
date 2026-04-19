package masking

import "testing"

func TestMaskingLifecycle(t *testing.T) {
	svc := NewService()
	if !svc.AddKey("password") {
		t.Fatal("expected add key")
	}
	if !svc.AddPattern("secret") {
		t.Fatal("expected add pattern")
	}
	preview := svc.Preview("password=secret")
	if preview["output"] == preview["input"] {
		t.Fatal("expected masked output")
	}
	if !svc.DeleteKey("password") {
		t.Fatal("expected delete key")
	}
	if !svc.DeletePattern("secret") {
		t.Fatal("expected delete pattern")
	}
}
