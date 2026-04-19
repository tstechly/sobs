package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMetricsRulesAndAnomalyEndpoints(t *testing.T) {
	srv := newTestServer()

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/metrics/rules", bytes.NewReader([]byte(`{"name":"r1","query":"q","threshold":">1"}`)))
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

	for _, p := range []string{"/metrics/rules", "/metrics/anomaly", "/api/metrics/anomaly"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}

	for _, p := range []string{"/metrics/rules/auto", "/metrics/rules/dashboard/auto"} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}

	delReq := httptest.NewRequest(http.MethodPost, "http://example.com/metrics/rules/"+id+"/delete", nil)
	delRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", delRec.Code)
	}
}
