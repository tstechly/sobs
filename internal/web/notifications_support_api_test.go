package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNotificationsVapidPublicKey(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/notifications/vapid-public-key", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestServiceWorker(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/service-worker.js", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "self.addEventListener") {
		t.Fatalf("expected service worker javascript, got %q", rec.Body.String())
	}
}
