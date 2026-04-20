package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAIHelperEndpoints(t *testing.T) {
	srv := newTestServer()

	spanReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/span-attributes", nil)
	spanRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(spanRec, spanReq)
	if spanRec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for /api/ai/span-attributes without required params, got %d", spanRec.Code)
	}

	for _, p := range []string{
		"/api/ai/helper/capabilities",
		"/api/ai/helper/actions/manifest",
		"/api/ai/helper/chats",
		"/api/ai/export",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+p, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d", p, rec.Code)
		}
	}

	helperReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/ai/helper", bytes.NewReader([]byte(`{"messages":[{"role":"user","content":"hello"}]}`)))
	helperRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(helperRec, helperReq)
	if helperRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", helperRec.Code)
	}
	var chat map[string]any
	if err := json.Unmarshal(helperRec.Body.Bytes(), &chat); err != nil {
		t.Fatalf("unmarshal helper response: %v", err)
	}
	id, _ := chat["id"].(string)
	if id == "" {
		t.Fatal("expected chat id")
	}

	getChatReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/ai/helper/chats/"+id, nil)
	getChatRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getChatRec, getChatReq)
	if getChatRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getChatRec.Code)
	}

	feedbackReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/ai/helper/feedback", bytes.NewReader([]byte(`{"chat_id":"`+id+`","rating":"up","note":"good"}`)))
	feedbackRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(feedbackRec, feedbackReq)
	if feedbackRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", feedbackRec.Code)
	}

	execReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/ai/helper/actions/execute", bytes.NewReader([]byte(`{"action_id":"summarize","payload":{"target":"logs"}}`)))
	execRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(execRec, execReq)
	if execRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", execRec.Code)
	}
}
