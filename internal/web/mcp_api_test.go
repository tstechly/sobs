package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func TestMCPToolsListAndInitialize(t *testing.T) {
	srv := newTestServer()
	toolsReq := httptest.NewRequest(http.MethodGet, "http://example.com/mcp/tools", nil)
	toolsRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(toolsRec, toolsReq)
	if toolsRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", toolsRec.Code)
	}

	initReq := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":1,"method":"initialize"}`)))
	initRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(initRec, initReq)
	if initRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", initRec.Code)
	}
}

func TestMCPKeyLifecycleAndToolCall(t *testing.T) {
	srv := newTestServer()
	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/mcp/keys", bytes.NewReader([]byte(`{"label":"agent"}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", createRec.Code)
	}
	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create key: %v", err)
	}
	rawKey, _ := created["key"].(string)
	keyID, _ := created["id"].(string)
	if rawKey == "" || keyID == "" {
		t.Fatal("expected raw key and id")
	}

	listReq := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)))
	listReq.Header.Set("X-MCP-API-Key", rawKey)
	listRec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}

	callReq := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"get_metric_names","arguments":{}}}`)))
	callReq.Header.Set("X-MCP-API-Key", rawKey)
	callRec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(callRec, callReq)
	if callRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", callRec.Code)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "http://example.com/api/mcp/keys/"+keyID, nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}
}

func TestMCPDisabledAndUnauthorized(t *testing.T) {
	srv := newTestServer()
	disableReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/mcp/enabled", bytes.NewReader([]byte(`{"enabled":false}`)))
	disableRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(disableRec, disableReq)
	if disableRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", disableRec.Code)
	}

	disabledReq := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":4,"method":"tools/list"}`)))
	disabledReq.Header.Set("X-MCP-API-Key", "bad")
	disabledRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(disabledRec, disabledReq)
	if disabledRec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", disabledRec.Code)
	}

	unauthSrv := newTestServer()
	unauthReq := httptest.NewRequest(http.MethodPost, "http://example.com/mcp", bytes.NewReader([]byte(`{"jsonrpc":"2.0","id":5,"method":"tools/list"}`)))
	unauthRec := httptest.NewRecorder()
	unauthSrv.Handler().ServeHTTP(unauthRec, unauthReq)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", unauthRec.Code)
	}
}

func TestMCPKeysPersistInChdbStore(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	chdbPath := filepath.Join(t.TempDir(), "sobs.chdb")
	factory := store.NewChdbStoreFactory(chdbPath)

	srvA := NewServer(cfg, factory)
	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/mcp/keys", bytes.NewReader([]byte(`{"label":"persisted"}`)))
	createRec := httptest.NewRecorder()
	srvA.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from create, got %d", createRec.Code)
	}

	srvB := NewServer(cfg, factory)
	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/mcp/keys", nil)
	listRec := httptest.NewRecorder()
	srvB.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200 from list, got %d", listRec.Code)
	}
	var payload struct {
		OK   bool              `json:"ok"`
		Keys []map[string]any  `json:"keys"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if !payload.OK {
		t.Fatal("expected ok=true")
	}
	if len(payload.Keys) == 0 {
		t.Fatal("expected persisted keys")
	}
}
