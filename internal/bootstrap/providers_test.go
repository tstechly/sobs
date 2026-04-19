package bootstrap

import (
	"testing"
)

func TestBuildAuthProviderDefault(t *testing.T) {
	t.Setenv("SOBS_AUTH_PROVIDER", "")
	p, err := BuildAuthProvider()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if p == nil {
		t.Fatal("expected provider")
	}
}

func TestBuildAuthProviderUnknown(t *testing.T) {
	t.Setenv("SOBS_AUTH_PROVIDER", "unknown")
	_, err := BuildAuthProvider()
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildStoreFactoryDefault(t *testing.T) {
	t.Setenv("SOBS_STORE_PROVIDER", "")
	p, err := BuildStoreFactory()
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if p == nil {
		t.Fatal("expected store factory")
	}
}

func TestBuildStoreFactoryUnknown(t *testing.T) {
	t.Setenv("SOBS_STORE_PROVIDER", "unknown")
	_, err := BuildStoreFactory()
	if err == nil {
		t.Fatal("expected error")
	}
}
