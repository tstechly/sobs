package web

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func TestWriteRoutesRequireUIAuthInBasicMode(t *testing.T) {
	t.Setenv("SOBS_BASIC_AUTH_USERNAME", "user")
	t.Setenv("SOBS_BASIC_AUTH_PASSWORD", "pass")
	t.Setenv("SOBS_EXTERNAL_AUTH_URL", "")

	cfg := config.Default()
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	subscribeBody := []byte(`{"endpoint":"https://push.notify.test.io/abc","keys":{"p256dh":"BNc...","auth":"a1b2c3"}}`)
	unauthReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader(subscribeBody))
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRec.Code)
	}

	authReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader(subscribeBody))
	authReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("user:pass")))
	authRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", authRec.Code)
	}
}

func TestReadRoutesRequireUIAuthInBasicMode(t *testing.T) {
	t.Setenv("SOBS_BASIC_AUTH_USERNAME", "user")
	t.Setenv("SOBS_BASIC_AUTH_PASSWORD", "pass")
	t.Setenv("SOBS_EXTERNAL_AUTH_URL", "")

	cfg := config.Default()
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	unauthReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/query/schema", nil)
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRec.Code)
	}

	authReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/query/schema", nil)
	authReq.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("user:pass")))
	authRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", authRec.Code)
	}
}

func TestV1APIKeyStillAppliesWhenConfiguredAndToggleDisabled(t *testing.T) {
	t.Setenv("SOBS_API_KEY", "test-key")
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	req := httptest.NewRequest(http.MethodPost, "http://example.com/v1/traces", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when API key is configured, got %d", rec.Code)
	}
}

func TestCSRFDefaultFollowsBehindTLSWhenUnset(t *testing.T) {
	t.Setenv("SOBS_CSRF_ORIGIN_CHECK", "")
	t.Setenv("SOBS_BEHIND_TLS", "1")
	t.Setenv("SOBS_BASIC_AUTH_USERNAME", "user")
	t.Setenv("SOBS_BASIC_AUTH_PASSWORD", "pass")
	t.Setenv("SOBS_EXTERNAL_AUTH_URL", "")

	cfg := config.Default()
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://push.notify.test.io/abc","keys":{"p256dh":"BNc...","auth":"a1b2c3"}}`)))
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("user:pass")))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 due to CSRF origin check, got %d", rec.Code)
	}
}
