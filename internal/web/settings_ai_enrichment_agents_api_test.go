package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettingsAIAndEnrichment(t *testing.T) {
	srv := newTestServer()

	aiReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/ai", bytes.NewReader([]byte(`{"provider":"openai","model":"gpt-4.1"}`)))
	aiRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(aiRec, aiReq)
	if aiRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", aiRec.Code)
	}

	enReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/enrichment", bytes.NewReader([]byte(`{"geo_enabled":"true","cve_enabled":"false","github_backfill_max_releases":"900"}`)))
	enRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(enRec, enReq)
	if enRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", enRec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(enRec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["ok"] != true {
		t.Fatal("expected ok")
	}
}

func TestSettingsAgentsCreateDelete(t *testing.T) {
	srv := newTestServer()

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/agents", bytes.NewReader([]byte(`{"name":"Rule 1","trigger_type":"manual","trigger_state":"any","action_analyze":"true","rate_limit_minutes":"30"}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}
	var rule map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &rule); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	id, _ := rule["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/agents/"+id+"/delete", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}
}
