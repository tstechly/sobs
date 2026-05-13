package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSettingsNotificationsChannelsCreate(t *testing.T) {
	srv := newTestServer()
	body := strings.NewReader("name=test-channel&channel_type=browser_push&push_endpoint=https%3A%2F%2Fpush.notify.io%2Fabc")
	req := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/channels", body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect, got %d body=%s", rec.Code, rec.Body.String())
	}
	if loc := rec.Header().Get("Location"); loc != "/settings/notifications" {
		t.Fatalf("expected redirect to /settings/notifications, got %q", loc)
	}
}

func TestNotificationRulesLifecycleRoutes(t *testing.T) {
	srv := newTestServer()

	// Create rule directly via the service (form-based creation redirects and
	// does not return the ID in the response body, so we create it here to
	// obtain the ID for subsequent toggle / delete calls).
	rule, err := srv.notificationService.CreateRule("critical-errors")
	if err != nil {
		t.Fatalf("create rule via service: %v", err)
	}
	id := rule.ID
	if id == "" {
		t.Fatal("expected non-empty rule ID from service")
	}

	toggleReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/rules/"+id+"/toggle", nil)
	toggleRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(toggleRec, toggleReq)
	if toggleRec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", toggleRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/rules/"+id+"/delete", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", deleteRec.Code)
	}
}

func TestNotificationsRulesAutoGenerate(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/rules/auto-generate", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

