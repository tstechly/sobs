package rum

import "testing"

func TestCreateAndGetAsset(t *testing.T) {
	svc := NewService()
	a, err := svc.CreateAsset("hello")
	if err != nil {
		t.Fatalf("create asset: %v", err)
	}
	got, ok := svc.GetAsset(a.ID)
	if !ok {
		t.Fatal("expected asset")
	}
	if got.Content != "hello" {
		t.Fatalf("expected content hello, got %q", got.Content)
	}
}
