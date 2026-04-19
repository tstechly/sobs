package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettingsNotificationsChannelsCreate(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/channels", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestNotificationRulesLifecycleRoutes(t *testing.T) {
	srv := newTestServer()

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/rules", bytes.NewReader([]byte(`{"name":"critical-errors"}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var rule map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &rule); err != nil {
		t.Fatalf("unmarshal rule: %v", err)
	}
	id, _ := rule["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}

	toggleReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/rules/"+id+"/toggle", nil)
	toggleRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(toggleRec, toggleReq)
	if toggleRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", toggleRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/rules/"+id+"/delete", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
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
