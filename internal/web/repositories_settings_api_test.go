package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettingsRepositoriesLifecycle(t *testing.T) {
	srv := newTestServer()

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories", bytes.NewReader([]byte(`{"name":"repo1","url":"https://github.com/acme/repo1"}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}
	var repo map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &repo); err != nil {
		t.Fatalf("unmarshal repo: %v", err)
	}
	id, _ := repo["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/settings/repositories", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	validateReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories/github-token/validate", bytes.NewReader([]byte(`{"token":"ghp_123456789012345"}`)))
	validateRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(validateRec, validateReq)
	if validateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", validateRec.Code)
	}

	realtimeReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories/"+id+"/realtime-mode", bytes.NewReader([]byte(`{"enabled":true}`)))
	realtimeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(realtimeRec, realtimeReq)
	if realtimeRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", realtimeRec.Code)
	}

	rotateReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories/"+id+"/ci-ingest-key/rotate", nil)
	rotateRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rotateRec, rotateReq)
	if rotateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rotateRec.Code)
	}

	revokeReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories/"+id+"/ci-ingest-key/revoke", nil)
	revokeRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(revokeRec, revokeReq)
	if revokeRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", revokeRec.Code)
	}

	updateReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories/"+id, bytes.NewReader([]byte(`{"name":"repo2"}`)))
	updateRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(updateRec, updateReq)
	if updateRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateRec.Code)
	}

	releaseReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories/"+id+"/releases", bytes.NewReader([]byte(`{"release":"1.0.0"}`)))
	releaseRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(releaseRec, releaseReq)
	if releaseRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", releaseRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/repositories/"+id+"/delete", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}
}
