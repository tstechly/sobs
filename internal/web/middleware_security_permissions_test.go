package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/store"
)

type captureAuthProvider struct {
	permissions []string
}

func (p *captureAuthProvider) Authenticate(_ context.Context, _ *http.Request) (extensionpoints.Identity, error) {
	return extensionpoints.Identity{Subject: "test-user"}, nil
}

func (p *captureAuthProvider) Authorize(_ context.Context, _ extensionpoints.Identity, permission string) error {
	p.permissions = append(p.permissions, permission)
	return nil
}

func TestRequiredPermissionSelection(t *testing.T) {
	authProvider := &captureAuthProvider{}
	cfg := config.Default()
	srv := NewServer(cfg, authProvider, store.NewNoopStoreFactory())

	readReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/reports", nil)
	readRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", readRec.Code)
	}

	writeReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/reports", nil)
	writeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", writeRec.Code)
	}

	queryReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/run", nil)
	queryRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(queryRec, queryReq)
	if queryRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", queryRec.Code)
	}

	if len(authProvider.permissions) < 3 {
		t.Fatalf("expected at least 3 permission checks, got %d", len(authProvider.permissions))
	}
	if authProvider.permissions[0] != "reports:read" {
		t.Fatalf("expected reports:read, got %q", authProvider.permissions[0])
	}
	if authProvider.permissions[1] != "reports:write" {
		t.Fatalf("expected reports:write, got %q", authProvider.permissions[1])
	}
	if authProvider.permissions[2] != "query:write" {
		t.Fatalf("expected query:write, got %q", authProvider.permissions[2])
	}
}
