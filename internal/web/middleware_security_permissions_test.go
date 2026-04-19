package web

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func TestV1AppsRequireAPIKeyWhenConfigured(t *testing.T) {
	t.Setenv("SOBS_API_KEY", "test-key")
	cfg := config.Default()
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	noKeyReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps", nil)
	noKeyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(noKeyRec, noKeyReq)
	if noKeyRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", noKeyRec.Code)
	}

	withKeyReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps", nil)
	withKeyReq.Header.Set("X-API-Key", "test-key")
	withKeyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(withKeyRec, withKeyReq)
	if withKeyRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", withKeyRec.Code)
	}
}
