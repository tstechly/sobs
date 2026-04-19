package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSettingsMaskingLifecycle(t *testing.T) {
	srv := newTestServer()

	addKeyReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/masking/keys", bytes.NewReader([]byte(`{"key":"password"}`)))
	addKeyRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(addKeyRec, addKeyReq)
	if addKeyRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", addKeyRec.Code)
	}

	previewReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/settings/masking/preview", bytes.NewReader([]byte(`{"input":"password=abc"}`)))
	previewRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", previewRec.Code)
	}
	var preview map[string]any
	if err := json.Unmarshal(previewRec.Body.Bytes(), &preview); err != nil {
		t.Fatalf("unmarshal preview: %v", err)
	}
	if preview["output"] == preview["input"] {
		t.Fatal("expected masked output")
	}

	rulesReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/settings/masking/rules", nil)
	rulesRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rulesRec, rulesReq)
	if rulesRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rulesRec.Code)
	}

	delReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/masking/keys/delete", bytes.NewReader([]byte(`{"key":"password"}`)))
	delRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", delRec.Code)
	}
}

func TestSettingsTagsAndRecordTagAPI(t *testing.T) {
	srv := newTestServer()

	autoReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/tags/auto", nil)
	autoRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(autoRec, autoReq)
	if autoRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", autoRec.Code)
	}

	createReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/tags", bytes.NewReader([]byte(`{"name":"Rule 1","condition":"severity_text='ERROR'","tag_key":"priority","tag_value":"high"}`)))
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createRec.Code)
	}
	var rule map[string]any
	if err := json.Unmarshal(createRec.Body.Bytes(), &rule); err != nil {
		t.Fatalf("unmarshal rule: %v", err)
	}
	id, _ := rule["id"].(string)
	if id == "" {
		t.Fatal("expected id")
	}

	suggestReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/settings/tags/condition-suggestions", nil)
	suggestRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(suggestRec, suggestReq)
	if suggestRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", suggestRec.Code)
	}

	setTagReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/tags/logs/abc", bytes.NewReader([]byte(`{"tag_key":"priority","tag_value":"high"}`)))
	setTagRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(setTagRec, setTagReq)
	if setTagRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", setTagRec.Code)
	}

	getTagReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/tags/logs/abc", nil)
	getTagRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getTagRec, getTagReq)
	if getTagRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", getTagRec.Code)
	}

	delTagReq := httptest.NewRequest(http.MethodDelete, "http://example.com/api/tags/logs/abc/priority", nil)
	delTagRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delTagRec, delTagReq)
	if delTagRec.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", delTagRec.Code)
	}

	delRuleReq := httptest.NewRequest(http.MethodPost, "http://example.com/settings/tags/"+id+"/delete", nil)
	delRuleRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRuleRec, delRuleReq)
	if delRuleRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", delRuleRec.Code)
	}
}
