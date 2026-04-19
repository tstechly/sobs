package mcp

import (
	"testing"
	"time"
)

func TestKeyLifecycleAndAuthentication(t *testing.T) {
	svc := NewService()
	key, raw, err := svc.CreateKey("test", "")
	if err != nil {
		t.Fatalf("create key: %v", err)
	}
	if key.ID == "" || raw == "" {
		t.Fatal("expected key metadata and raw token")
	}
	if !svc.Authenticate(raw) {
		t.Fatal("expected raw key to authenticate")
	}
	if !svc.DeleteKey(key.ID) {
		t.Fatal("expected delete to succeed")
	}
	if svc.Authenticate(raw) {
		t.Fatal("expected deleted key to fail authentication")
	}
}

func TestExpiryMetadataIgnoredAndRateLimited(t *testing.T) {
	svc := NewService()
	_, raw, err := svc.CreateKey("expired", time.Now().UTC().Add(-time.Minute).Format(time.RFC3339))
	if err != nil {
		t.Fatalf("create expired key: %v", err)
	}
	if !svc.Authenticate(raw) {
		t.Fatal("expected authentication to ignore expires_at metadata like Python does")
	}
	for i := 0; i < 60; i++ {
		if !svc.AllowRequest("127.0.0.1") {
			t.Fatalf("expected request %d to be allowed", i)
		}
	}
	if svc.AllowRequest("127.0.0.1") {
		t.Fatal("expected rate limit to reject request 61")
	}
}
