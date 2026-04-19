package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestNotificationsSubscribe(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", rec.Code)
	}
}

func TestTailEndpoint(t *testing.T) {
	srv := newTestServer()
	req := httptest.NewRequest(http.MethodGet, "http://example.com/tail", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestNotificationsVAPIDLifecycleAndChannelTest(t *testing.T) {
	srv := newTestServer()

	keygenReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/vapid-keygen", nil)
	keygenRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(keygenRec, keygenReq)
	if keygenRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", keygenRec.Code)
	}

	publicReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/notifications/vapid-public-key", nil)
	publicRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(publicRec, publicReq)
	if publicRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", publicRec.Code)
	}

	subReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	subRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", subRec.Code)
	}
	var sub map[string]any
	if err := json.Unmarshal(subRec.Body.Bytes(), &sub); err != nil {
		t.Fatalf("unmarshal subscription: %v", err)
	}
	id, _ := sub["id"].(string)
	if id == "" {
		t.Fatal("expected subscription id")
	}

	testReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/channels/"+id+"/test", nil)
	testRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(testRec, testReq)
	if testRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", testRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "http://example.com/api/notifications/vapid-keys", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", deleteRec.Code)
	}
}

func TestSettingsNotificationsChannelActions(t *testing.T) {
	srv := newTestServer()

	subReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	subRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", subRec.Code)
	}
	var sub map[string]any
	if err := json.Unmarshal(subRec.Body.Bytes(), &sub); err != nil {
		t.Fatalf("unmarshal subscription: %v", err)
	}
	id, _ := sub["id"].(string)

	toggleReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/channels/"+id+"/toggle", nil)
	toggleRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(toggleRec, toggleReq)
	if toggleRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", toggleRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/channels/"+id+"/delete", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}
}

func TestNotificationsListEndpoints(t *testing.T) {
	srv := newTestServer()

	subReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/notifications/subscribe", bytes.NewReader([]byte(`{"endpoint":"https://example.com/push"}`)))
	subRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(subRec, subReq)
	if subRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", subRec.Code)
	}

	ruleReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/notifications/rules", bytes.NewReader([]byte(`{"name":"critical-errors"}`)))
	ruleRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(ruleRec, ruleReq)
	if ruleRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", ruleRec.Code)
	}

	listRulesReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/notifications/rules", nil)
	listRulesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRulesRec, listRulesReq)
	if listRulesRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRulesRec.Code)
	}

	listSubsReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/notifications/subscriptions", nil)
	listSubsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listSubsRec, listSubsReq)
	if listSubsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listSubsRec.Code)
	}
}
