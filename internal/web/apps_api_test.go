package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/abartrim/sobs/internal/auth"
	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func newTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	return NewServer(cfg, auth.NewStaticProvider(), store.NewNoopStoreFactory())
}

func TestV1AppsCreateListGetPatch(t *testing.T) {
	srv := newTestServer()

	createBody := []byte(`{"name":"demo-app"}`)
	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader(createBody))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create response: %v", err)
	}
	appID, _ := created["id"].(string)
	if appID == "" {
		t.Fatal("expected app id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps/"+appID, nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	patchBody := []byte(`{"name":"demo-app-2"}`)
	patchReq := httptest.NewRequest(http.MethodPatch, "http://example.com/v1/apps/"+appID, bytes.NewReader(patchBody))
	patchRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(patchRec, patchReq)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", patchRec.Code)
	}
}

func TestV1ReleasesFlow(t *testing.T) {
	srv := newTestServer()

	createAppReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps", bytes.NewReader([]byte(`{"name":"demo-app"}`)))
	createAppRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createAppRec, createAppReq)
	if createAppRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createAppRec.Code)
	}
	var app map[string]any
	if err := json.Unmarshal(createAppRec.Body.Bytes(), &app); err != nil {
		t.Fatalf("unmarshal app: %v", err)
	}
	appID, _ := app["id"].(string)

	createReleaseReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/apps/"+appID+"/releases", bytes.NewReader([]byte(`{"version":"1.0.0"}`)))
	createReleaseRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createReleaseRec, createReleaseReq)
	if createReleaseRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createReleaseRec.Code)
	}

	var release map[string]any
	if err := json.Unmarshal(createReleaseRec.Body.Bytes(), &release); err != nil {
		t.Fatalf("unmarshal release: %v", err)
	}
	releaseID, _ := release["id"].(string)
	if releaseID == "" {
		t.Fatal("expected release id")
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/apps/"+appID+"/releases", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/releases/"+releaseID, nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getRec.Code)
	}

	artifactsReq := httptest.NewRequest(http.MethodGet, "http://example.com/v1/releases/"+releaseID+"/artifacts", nil)
	artifactsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(artifactsRec, artifactsReq)
	if artifactsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", artifactsRec.Code)
	}

	metaReq := httptest.NewRequest(http.MethodPost, "http://example.com/v1/releases/"+releaseID+"/artifacts/meta", bytes.NewReader([]byte(`{"k":"v"}`)))
	metaRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(metaRec, metaReq)
	if metaRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", metaRec.Code)
	}
}
