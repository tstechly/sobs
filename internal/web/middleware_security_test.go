package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abartrim/sobs/internal/auth"
	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func TestWriteRoutesRequireAuthWhenEnabled(t *testing.T) {
	cfg := config.Default()
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	unauthReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRec.Code)
	}

	authReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	authReq.Header.Set("Authorization", "Bearer test")
	authRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", authRec.Code)
	}
}

func TestReadRoutesRequireAuthWhenEnabled(t *testing.T) {
	cfg := config.Default()
	srv := NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())

	unauthReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/query/schema", nil)
	unauthRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRec.Code)
	}

	authReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/query/schema", nil)
	authReq.Header.Set("Authorization", "Bearer test")
	authRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(authRec, authReq)
	if authRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", authRec.Code)
	}
}
