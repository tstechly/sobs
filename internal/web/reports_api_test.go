package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestReportsCRUDAndImportExport(t *testing.T) {
	srv := newTestServer()

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/reports", bytes.NewReader([]byte(`{"name":"r1","description":"saved logs","page_type":"logs","filters":{"q":"boom","service":"svc-logs"}}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}

	var created map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("unmarshal create: %v", err)
	}
	id, _ := created["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}
	if created["page_type"] != "logs" {
		t.Fatalf("expected logs page type, got %#v", created)
	}

	listReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/reports?page_type=logs", nil)
	listRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", listRec.Code)
	}
	var listed []map[string]any
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if len(listed) != 1 || listed[0]["name"] != "r1" {
		t.Fatalf("expected one listed report, got %#v", listed)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/reports/export", nil)
	exportRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", exportRec.Code)
	}
	if !strings.Contains(exportRec.Header().Get("Content-Disposition"), "sobs_reports_export.json") {
		t.Fatalf("expected export attachment header, got %q", exportRec.Header().Get("Content-Disposition"))
	}

	importReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/reports/import?on_conflict=rename", bytes.NewReader(exportRec.Body.Bytes()))
	importRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", importRec.Code)
	}
	var imported map[string]any
	if err := json.Unmarshal(importRec.Body.Bytes(), &imported); err != nil {
		t.Fatalf("unmarshal import: %v", err)
	}
	if imported["imported"] != float64(1) {
		t.Fatalf("expected one imported report, got %#v", imported)
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "http://example.com/api/reports/"+id, nil)
	deleteRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteRec.Code)
	}
}
