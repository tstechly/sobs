package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestQueryEndpoints(t *testing.T) {
	srv := newTestServer()

	askReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/ask", bytes.NewReader([]byte(`{"question":"show errors"}`)))
	askRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(askRec, askReq)
	if askRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", askRec.Code)
	}
	var askPayload map[string]any
	if err := json.Unmarshal(askRec.Body.Bytes(), &askPayload); err != nil {
		t.Fatalf("unmarshal ask payload: %v", err)
	}
	if ok, _ := askPayload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true in ask payload")
	}
	if _, exists := askPayload["trace_id"]; !exists {
		t.Fatalf("expected trace_id in ask payload")
	}

	runReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/run", bytes.NewReader([]byte(`{"sql":"select 1"}`)))
	runRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(runRec, runReq)
	if runRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", runRec.Code)
	}
	var runPayload map[string]any
	if err := json.Unmarshal(runRec.Body.Bytes(), &runPayload); err != nil {
		t.Fatalf("unmarshal run payload: %v", err)
	}
	if ok, _ := runPayload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true in run payload")
	}
	if _, exists := runPayload["datasets"]; !exists {
		t.Fatalf("expected datasets in run payload")
	}

	refineReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/refine-chart", bytes.NewReader([]byte(`{"prompt":"make a line chart","spec":{"type":"bar"}}`)))
	refineRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(refineRec, refineReq)
	if refineRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", refineRec.Code)
	}

	schemaReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/query/schema", nil)
	schemaRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(schemaRec, schemaReq)
	if schemaRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", schemaRec.Code)
	}

	addReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/add-to-dashboard", bytes.NewReader([]byte(`{"dashboard_id":"1"}`)))
	addRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(addRec, addReq)
	if addRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", addRec.Code)
	}
}

func TestTableExplorerAndChartTypes(t *testing.T) {
	srv := newTestServer()

	tablesReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/table-explorer/tables", nil)
	tablesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tablesRec, tablesReq)
	if tablesRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", tablesRec.Code)
	}
	var tablesPayload map[string]any
	if err := json.Unmarshal(tablesRec.Body.Bytes(), &tablesPayload); err != nil {
		t.Fatalf("unmarshal tables payload: %v", err)
	}
	if ok, _ := tablesPayload["ok"].(bool); !ok {
		t.Fatalf("expected ok=true in tables payload")
	}
	if _, exists := tablesPayload["tables"]; !exists {
		t.Fatalf("expected tables in tables payload")
	}

	tableReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/table-explorer/table/sobs_logs", nil)
	tableRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(tableRec, tableReq)
	if tableRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", tableRec.Code)
	}
	var tablePayload map[string]any
	if err := json.Unmarshal(tableRec.Body.Bytes(), &tablePayload); err != nil {
		t.Fatalf("unmarshal table payload: %v", err)
	}
	if _, exists := tablePayload["sample"]; !exists {
		t.Fatalf("expected sample in table payload")
	}
	if _, exists := tablePayload["ddl"]; !exists {
		t.Fatalf("expected ddl in table payload")
	}

	chartReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/chart-types", nil)
	chartRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(chartRec, chartReq)
	if chartRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", chartRec.Code)
	}
}
