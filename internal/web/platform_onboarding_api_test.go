package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestKubernetesEndpoints(t *testing.T) {
	srv := newTestServer()
	for _, tc := range []struct {
		method string
		path   string
		body   []byte
		code   int
	}{
		{http.MethodGet, "/settings/kubernetes", nil, http.StatusOK},
		{http.MethodPost, "/settings/kubernetes", []byte(`{"enabled":"true","default_namespace":"prod"}`), http.StatusOK},
		{http.MethodGet, "/kubernetes", nil, http.StatusOK},
		{http.MethodGet, "/api/kubernetes/status", nil, http.StatusOK},
	} {
		req := httptest.NewRequest(tc.method, "http://example.com"+tc.path, bytes.NewReader(tc.body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != tc.code {
			t.Fatalf("expected %d for %s, got %d", tc.code, tc.path, rec.Code)
		}
	}
}

func TestDataManagementEndpoints(t *testing.T) {
	srv := newTestServer()
	saveReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/data-management", bytes.NewReader([]byte(`{"backup_enabled":"true","s3_bucket":"b","ttl_logs_days":"7","ttl_traces_days":"7","ttl_metrics_hours":"24","ttl_sessions_days":"7"}`)))
	saveRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(saveRec, saveReq)
	if saveRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", saveRec.Code)
	}

	runReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/data-management/backup/run", bytes.NewReader([]byte(`{"type":"full"}`)))
	runRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", runRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/data-management/backup/list", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/data-management/restore", bytes.NewReader([]byte(`{"backup_name":"sobs-full-1"}`)))
	restoreRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", restoreRec.Code)
	}
}

func TestSetupWizardAndOnboardingEndpoints(t *testing.T) {
	srv := newTestServer()

	wizardReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/setup-wizard/steps?env=dev&language=python&deployment=docker", nil)
	wizardRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(wizardRec, wizardReq)
	if wizardRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", wizardRec.Code)
	}

	createRepoReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/onboarding/create-repo", bytes.NewReader([]byte(`{"name":"demo","repo_url":"https://github.com/acme/demo"}`)))
	createRepoRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRepoRec, createRepoReq)
	if createRepoRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", createRepoRec.Code)
	}

	importRepoReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/onboarding/import-repo", bytes.NewReader([]byte(`{"repo_owner":"acme","repo_name":"demo"}`)))
	importRepoRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(importRepoRec, importRepoReq)
	if importRepoRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", importRepoRec.Code)
	}

	listReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/onboarding/list-repos", bytes.NewReader([]byte(`{"owner":"acme"}`)))
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	inspectReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/onboarding/inspect-repo?app_id=1", nil)
	inspectRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(inspectRec, inspectReq)
	if inspectRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", inspectRec.Code)
	}

	issuesReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/onboarding/create-issues", bytes.NewReader([]byte(`{"app_id":"1","create_ci":true,"create_otel":true}`)))
	issuesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(issuesRec, issuesReq)
	if issuesRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", issuesRec.Code)
	}
}
