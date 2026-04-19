package web

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abartrim/sobs/internal/auth"
	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func newParityTestServer() *Server {
	cfg := config.Default()
	cfg.TemplateRoot = "../../templates"
	cfg.EnforceAPIAuth = false
	return NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())
}

func TestUIBasicModeRequiresAuthorization(t *testing.T) {
	t.Setenv("SOBS_BASIC_AUTH_USERNAME", "user")
	t.Setenv("SOBS_BASIC_AUTH_PASSWORD", "pass")
	t.Setenv("SOBS_EXTERNAL_AUTH_URL", "")

	srv := newParityTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/logs", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if got := rec.Header().Get("WWW-Authenticate"); got != `Basic realm="SOBS"` {
		t.Fatalf("expected basic challenge, got %q", got)
	}
}

func TestUIBasicModeAcceptsValidCredentials(t *testing.T) {
	t.Setenv("SOBS_BASIC_AUTH_USERNAME", "user")
	t.Setenv("SOBS_BASIC_AUTH_PASSWORD", "pass")
	t.Setenv("SOBS_EXTERNAL_AUTH_URL", "")

	srv := newParityTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/logs", nil)
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte("user:pass")))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestUIExternalModeAcceptsSessionCookieFallback(t *testing.T) {
	authSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "Bearer sess-token" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer authSrv.Close()

	t.Setenv("SOBS_BASIC_AUTH_USERNAME", "")
	t.Setenv("SOBS_BASIC_AUTH_PASSWORD", "")
	t.Setenv("SOBS_EXTERNAL_AUTH_URL", authSrv.URL)

	srv := newParityTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/logs", nil)
	req.AddCookie(&http.Cookie{Name: "session", Value: "sess-token"})
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestV1IngestRequiresAPIKeyWhenConfigured(t *testing.T) {
	t.Setenv("SOBS_API_KEY", "abc123")

	srv := newParityTestServer()

	noKeyReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/traces", nil)
	noKeyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(noKeyRec, noKeyReq)
	if noKeyRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without key, got %d", noKeyRec.Code)
	}

	withKeyReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/traces", nil)
	withKeyReq.Header.Set("X-API-Key", "abc123")
	withKeyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(withKeyRec, withKeyReq)
	if withKeyRec.Code == http.StatusUnauthorized {
		t.Fatalf("expected non-401 with valid key, got %d", withKeyRec.Code)
	}
}

func TestRUMAssetDownloadUsesUIAuthMode(t *testing.T) {
	t.Setenv("SOBS_BASIC_AUTH_USERNAME", "user")
	t.Setenv("SOBS_BASIC_AUTH_PASSWORD", "pass")
	t.Setenv("SOBS_API_KEY", "")

	srv := newParityTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/v1/rum/assets/someid", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for asset download without UI auth, got %d", rec.Code)
	}
}
