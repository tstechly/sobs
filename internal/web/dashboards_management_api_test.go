package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDashboardsCRUDAndChartActions(t *testing.T) {
	srv := newTestServer()

	createDashReq := httptest.NewRequest(http.MethodPost, "http://example.com/dashboards", bytes.NewReader([]byte(`{"name":"Ops"}`)))
	createDashRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createDashRec, createDashReq)
	if createDashRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createDashRec.Code)
	}
	var dash map[string]any
	if err := json.Unmarshal(createDashRec.Body.Bytes(), &dash); err != nil {
		t.Fatalf("unmarshal dashboard: %v", err)
	}
	did, _ := dash["id"].(string)
	if did == "" {
		t.Fatal("expected dashboard id")
	}

	getDashReq := httptest.NewRequest(http.MethodGet, "http://example.com/dashboards/"+did, nil)
	getDashRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getDashRec, getDashReq)
	if getDashRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getDashRec.Code)
	}

	addChartReq := httptest.NewRequest(http.MethodPost, "http://example.com/dashboards/"+did+"/charts", bytes.NewReader([]byte(`{"title":"Latency","type":"line"}`)))
	addChartRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(addChartRec, addChartReq)
	if addChartRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", addChartRec.Code)
	}
	var chart map[string]any
	if err := json.Unmarshal(addChartRec.Body.Bytes(), &chart); err != nil {
		t.Fatalf("unmarshal chart: %v", err)
	}
	cid, _ := chart["id"].(string)
	if cid == "" {
		t.Fatal("expected chart id")
	}

	editReq := httptest.NewRequest(http.MethodPost, "http://example.com/dashboards/"+did+"/charts/"+cid+"/edit", bytes.NewReader([]byte(`{"title":"Latency P95"}`)))
	editRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(editRec, editReq)
	if editRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", editRec.Code)
	}

	cloneReq := httptest.NewRequest(http.MethodPost, "http://example.com/dashboards/"+did+"/charts/"+cid+"/clone", nil)
	cloneRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(cloneRec, cloneReq)
	if cloneRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", cloneRec.Code)
	}

	exportReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/dashboards/"+did+"/charts/"+cid+"/export", nil)
	exportRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(exportRec, exportReq)
	if exportRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", exportRec.Code)
	}

	importReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/dashboards/"+did+"/charts/import", bytes.NewReader([]byte(`{"items":[{"title":"Imported","type":"bar"}]}`)))
	importRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(importRec, importReq)
	if importRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", importRec.Code)
	}

	delChartReq := httptest.NewRequest(http.MethodPost, "http://example.com/dashboards/"+did+"/charts/"+cid+"/delete", nil)
	delChartRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delChartRec, delChartReq)
	if delChartRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", delChartRec.Code)
	}

	delDashReq := httptest.NewRequest(http.MethodPost, "http://example.com/dashboards/"+did+"/delete", nil)
	delDashRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delDashRec, delDashReq)
	if delDashRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", delDashRec.Code)
	}
}
