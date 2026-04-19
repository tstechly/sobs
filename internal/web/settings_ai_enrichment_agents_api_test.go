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

	aiPageReq := httptest.NewRequest(http.MethodGet, "http://example.com/settings/ai", nil)
	aiPageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(aiPageRec, aiPageReq)
	if aiPageRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for settings ai page, got %d", aiPageRec.Code)
	}

	enPageReq := httptest.NewRequest(http.MethodGet, "http://example.com/settings/enrichment", nil)
	enPageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(enPageRec, enPageReq)
	if enPageRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for settings enrichment page, got %d", enPageRec.Code)
	}

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

	agentsPageReq := httptest.NewRequest(http.MethodGet, "http://example.com/settings/agents", nil)
	agentsPageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(agentsPageRec, agentsPageReq)
	if agentsPageRec.Code != http.StatusOK {
		t.Fatalf("expected 200 for settings agents page, got %d", agentsPageRec.Code)
	}

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
