package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func queryMockLLMServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		if stream, _ := payload["stream"].(bool); stream {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"SELECT \"}}]}\n\n"))
			_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"1\"}}],\"usage\":{\"prompt_tokens\":8,\"completion_tokens\":2}}\n\n"))
			_, _ = w.Write([]byte("data: [DONE]\n\n"))
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "SELECT 1"}}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2, "reasoning_tokens": 0},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestQueryEndpoints(t *testing.T) {
	srv := newTestServer()
	llm := queryMockLLMServer()
	defer llm.Close()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})

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

func TestQueryAskStreaming(t *testing.T) {
	srv := newTestServer()
	llm := queryMockLLMServer()
	defer llm.Close()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/ask", bytes.NewReader([]byte(`{"question":"show errors","stream":true}`)))
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: sql_delta") {
		t.Fatalf("expected sql_delta events in stream body: %s", body)
	}
	if !strings.Contains(body, "event: result") {
		t.Fatalf("expected result event in stream body: %s", body)
	}
}

func TestQueryRunStreaming(t *testing.T) {
	srv := newTestServer()
	llm := queryMockLLMServer()
	defer llm.Close()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/run", bytes.NewReader([]byte(`{"sql":"select 1","stream":true}`)))
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: stage") {
		t.Fatalf("expected stage events in stream body: %s", body)
	}
	if !strings.Contains(body, "event: result") {
		t.Fatalf("expected result event in stream body: %s", body)
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

	tableReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/table-explorer/table/otel_logs", nil)
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

func TestQueryRunMasksSQLWhenEnabled(t *testing.T) {
	srv := newTestServer()
	llm := queryMockLLMServer()
	defer llm.Close()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})
	srv.maskingService.SetOutputMode("mask")
	srv.maskingService.SetSQLOutput("masked")

	rawSQL := "select 'password=abc123' as token"
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/run", bytes.NewReader([]byte(`{"sql":"`+rawSQL+`"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "password=abc123") {
		t.Fatalf("expected masked response body, got raw secret: %s", body)
	}
	if !strings.Contains(body, "****") {
		t.Fatalf("expected masked token in response body: %s", body)
	}
}

func TestQueryRunRespectsSQLOutputMaskingToggle(t *testing.T) {
	srv := newTestServer()
	llm := queryMockLLMServer()
	defer llm.Close()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})
	srv.maskingService.SetOutputMode("mask")
	srv.maskingService.SetSQLOutput("unmasked")

	rawSQL := "select 'password=abc123' as token"
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/run", bytes.NewReader([]byte(`{"sql":"`+rawSQL+`"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `"sql":"`+rawSQL+`"`) {
		t.Fatalf("expected sql to remain visible when sql masking is disabled: %s", body)
	}
	if strings.Contains(body, `"rows":[["password=abc123"]]`) {
		t.Fatalf("expected non-sql payload values to remain masked: %s", body)
	}
	if !strings.Contains(body, "****") {
		t.Fatalf("expected masked token in rows payload: %s", body)
	}
}

func TestQueryRunStreamMasksResultPayload(t *testing.T) {
	srv := newTestServer()
	llm := queryMockLLMServer()
	defer llm.Close()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})
	srv.maskingService.SetOutputMode("mask")
	srv.maskingService.SetSQLOutput("masked")

	rawSQL := "select 'password=abc123' as token"
	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/run", bytes.NewReader([]byte(`{"sql":"`+rawSQL+`","stream":true}`)))
	req.Header.Set("Accept", "text/event-stream")
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "password=abc123") {
		t.Fatalf("expected streamed payload to mask secret data: %s", body)
	}
	if !strings.Contains(body, "****") {
		t.Fatalf("expected masked token in streamed payload: %s", body)
	}
}

func TestQueryAskSanitizesUpstreamLLMError(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("upstream-secret-body"))
	}))
	defer llm.Close()

	srv := newTestServer()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/ask", bytes.NewReader([]byte(`{"question":"show errors"}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	errText, _ := payload["error"].(string)
	if strings.Contains(errText, "upstream-secret-body") {
		t.Fatalf("expected sanitized LLM error, got: %s", errText)
	}
	if !strings.Contains(strings.ToLower(errText), "upstream llm http") {
		t.Fatalf("expected upstream status summary in error, got: %s", errText)
	}
}

func TestQueryAskRepairsChartSpecJSON(t *testing.T) {
	llm := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.NotFound(w, r)
			return
		}
		var payload map[string]any
		_ = json.NewDecoder(r.Body).Decode(&payload)
		messages, _ := payload["messages"].([]any)
		lastText := ""
		if len(messages) > 0 {
			if last, ok := messages[len(messages)-1].(map[string]any); ok {
				if content, ok := last["content"].(string); ok {
					lastText = content
				}
			}
		}
		content := "SELECT 1"
		if strings.Contains(lastText, "Produce an ECharts option JSON object") {
			content = "{\"title\": "
		}
		if strings.Contains(lastText, "failed to parse") {
			content = `{"title":{"text":"ok"},"xAxis":{"type":"category"},"yAxis":{"type":"value"},"series":[{"type":"bar","data":[1]}]}`
		}
		resp := map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": content}}},
			"usage":   map[string]any{"prompt_tokens": 8, "completion_tokens": 2},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer llm.Close()

	srv := newTestServer()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": llm.URL, "model": "gpt-4.1-mini"})

	req := httptest.NewRequest(http.MethodPost, "http://example.com/api/query/ask", bytes.NewReader([]byte(`{"question":"show errors","chart":true}`)))
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response payload: %v", err)
	}
	errText, _ := payload["error"].(string)
	if errText != "" {
		t.Fatalf("expected chart repair to clear error, got: %s", errText)
	}
	chartSpec, _ := payload["chart_spec"].(string)
	if !strings.Contains(chartSpec, "title") {
		t.Fatalf("expected repaired chart spec, got: %s", chartSpec)
	}
}

func TestQueryAIConfigSupportsFileBackedEnv(t *testing.T) {
	srv := newTestServer()
	srv.settingsService.SaveAI(map[string]string{"endpoint_url": "", "model": "", "api_key": ""})
	tmpDir := t.TempDir()
	endpointPath := filepath.Join(tmpDir, "endpoint.txt")
	modelPath := filepath.Join(tmpDir, "model.txt")
	apiKeyPath := filepath.Join(tmpDir, "api_key.txt")
	if err := os.WriteFile(endpointPath, []byte("http://example-llm"), 0o600); err != nil {
		t.Fatalf("write endpoint file: %v", err)
	}
	if err := os.WriteFile(modelPath, []byte("gpt-test-model"), 0o600); err != nil {
		t.Fatalf("write model file: %v", err)
	}
	if err := os.WriteFile(apiKeyPath, []byte("test-api-key"), 0o600); err != nil {
		t.Fatalf("write api key file: %v", err)
	}

	t.Setenv("SOBS_AI_ENDPOINT_URL", "")
	t.Setenv("SOBS_AI_MODEL", "")
	t.Setenv("SOBS_AI_API_KEY", "")
	t.Setenv("SOBS_AI_ENDPOINT_URL_FILE", endpointPath)
	t.Setenv("SOBS_AI_MODEL_FILE", modelPath)
	t.Setenv("SOBS_AI_API_KEY_FILE", apiKeyPath)

	cfg := srv.queryAIConfig()
	if cfg.EndpointURL != "http://example-llm" {
		t.Fatalf("expected endpoint_url from file env, got: %q", cfg.EndpointURL)
	}
	if cfg.Model != "gpt-test-model" {
		t.Fatalf("expected model from file env, got: %q", cfg.Model)
	}
	if cfg.APIKey != "test-api-key" {
		t.Fatalf("expected api_key from file env, got: %q", cfg.APIKey)
	}
}
