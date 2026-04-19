package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTableExplorerPages(t *testing.T) {
	srv := newTestServer()
	for _, p := range []string{"/table-explorer", "/table-explorer/help"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}
}

func TestReportsDeletePageAlias(t *testing.T) {
	srv := newTestServer()
	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/reports", bytes.NewReader([]byte(`{"name":"r1","query":"select 1"}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}
	var report map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &report); err != nil {
		t.Fatalf("unmarshal report: %v", err)
	}
	id, _ := report["id"].(string)
	if id == "" {
		t.Fatal("expected report id")
	}
	deleteReq := httptest.NewRequest(http.MethodPost, "http://example.com/reports/"+id+"/delete", nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}
}
