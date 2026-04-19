package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRUMAssetsAndClientToken(t *testing.T) {
	srv := newTestServer()

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum/assets", bytes.NewReader([]byte(`{"content":"asset"}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}
	var asset map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &asset); err != nil {
		t.Fatalf("unmarshal asset: %v", err)
	}
	id, _ := asset["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/rum/assets/"+id, nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	// When SOBS_RUM_CLIENT_AUTH_MODE is unset (default "none"), returns
	// {"enabled":false,...} with 200 — matching Python's behavior.
	tokenReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/rum/client-token", nil)
	tokenRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tokenRec, tokenReq)
	if tokenRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", tokenRec.Code)
	}
	var tokenResp map[string]any
	if err := json.Unmarshal(tokenRec.Body.Bytes(), &tokenResp); err != nil {
		t.Fatalf("unmarshal token resp: %v", err)
	}
	enabled, _ := tokenResp["enabled"].(bool)
	if enabled {
		t.Fatal("expected enabled=false when auth mode is none")
	}
}
