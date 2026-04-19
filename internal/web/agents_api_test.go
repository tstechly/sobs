package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAgentRunsAndIssueRaise(t *testing.T) {
	srv := newTestServer()

	createRunReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/agent/runs", bytes.NewReader([]byte(`{"title":"triage"}`)))
	createRunRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRunRec, createRunReq)
	if createRunRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRunRec.Code)
	}

	var run map[string]any
	if err := json.Unmarshal(createRunRec.Body.Bytes(), &run); err != nil {
		t.Fatalf("unmarshal run: %v", err)
	}
	runID, _ := run["id"].(string)
	if runID == "" {
		t.Fatal("expected run id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/agent/runs", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	dismissReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/agent/runs/"+runID+"/dismiss", nil)
	dismissRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(dismissRec, dismissReq)
	if dismissRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", dismissRec.Code)
	}

	issueReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/issues/raise", bytes.NewReader([]byte(`{"title":"bug","body":"details"}`)))
	issueRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(issueRec, issueReq)
	if issueRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", issueRec.Code)
	}
}
