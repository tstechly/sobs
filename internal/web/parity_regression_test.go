package web

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func TestParityPageRoutesRenderHTML(t *testing.T) {
	srv := newTestServer()

	for _, path := range []string{
		"/",
		"/dashboards",
		"/settings",
		"/settings/ai",
		"/settings/enrichment",
		"/settings/repositories",
		"/settings/notifications",
		"/settings/masking",
		"/settings/tags",
		"/settings/agents",
		"/settings/data-management",
		"/settings/kubernetes",
		"/settings/mcp",
	} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d body=%q", path, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		if ct := rec.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
			t.Fatalf("expected html content type for %s, got %q", path, ct)
		}
		if strings.Contains(strings.ToLower(strings.TrimSpace(rec.Body.String())), `"ok":true`) {
			t.Fatalf("expected HTML body for %s, got JSON fallback payload", path)
		}
		if strings.Contains(strings.ToLower(strings.TrimSpace(rec.Body.String())), "template error") {
			t.Fatalf("expected rendered HTML body for %s, got template error body", path)
		}
	}
}

func TestParityPageRoutesFailLoudWithoutRenderer(t *testing.T) {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "./definitely-missing-templates"
	srv := NewServer(cfg, store.NewNoopStoreFactory())

	for _, path := range []string{"/", "/settings", "/settings/ai", "/dashboards", "/kubernetes", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 for %s when renderer unavailable, got %d", path, rec.Code)
		}
		if !strings.Contains(strings.ToLower(rec.Body.String()), "template error") {
			t.Fatalf("expected template error body for %s, got %q", path, rec.Body.String())
		}
	}
}

func TestParityV1IngestRoutesWired(t *testing.T) {
	srv := newTestServer()

	tests := []struct {
		path string
		body []byte
	}{
		{path: "/v1/logs", body: []byte(`{"resourceLogs":[]}`)},
		{path: "/v1/traces", body: []byte(`{"resourceSpans":[]}`)},
		{path: "/v1/metrics", body: []byte(`{"resourceMetrics":[]}`)},
		{path: "/v1/errors", body: []byte(`{"items":[]}`)},
		{path: "/v1/rum", body: []byte(`{"events":[]}`)},
		{path: "/v1/ai", body: []byte(`{"items":[]}`)},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+tc.path, bytes.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code == http.StatusNotFound {
			t.Fatalf("expected %s to be registered, got 404", tc.path)
		}
	}
}

func TestParitySettingsTagsTemplateContextRenders(t *testing.T) {
	srv := newTestServer()
	if srv.renderer == nil || srv.renderErr != nil {
		t.Fatalf("renderer unavailable: %v", srv.renderErr)
	}

	ctx := map[string]any{
		"title":                 "Tag Rules",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "settings/tags"},
		"rules":                 []any{},
		"edit_rule":             nil,
		"record_types":          []string{"all", "log", "trace", "error", "ai", "rum"},
		"match_fields":          []string{"severity", "service_name", "body", "trace_id", "span_id", "attribute"},
		"match_operators": []map[string]string{
			{"value": "eq", "label": "eq"},
			{"value": "contains", "label": "contains"},
			{"value": "regex", "label": "regex"},
			{"value": "startswith", "label": "startswith"},
			{"value": "endswith", "label": "endswith"},
		},
		"services":     []string{},
		"auto_summary": nil,
		"auto_preview": nil,
	}

	if _, err := srv.renderer.Render("settings_tags.html", ctx); err != nil {
		t.Fatalf("expected settings_tags.html to render, got %v", err)
	}
}

func TestParitySettingsNotificationsTemplateContextRenders(t *testing.T) {
	srv := newTestServer()
	if srv.renderer == nil || srv.renderErr != nil {
		t.Fatalf("renderer unavailable: %v", srv.renderErr)
	}

	ctx := map[string]any{
		"title":                 "Settings Notifications",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "settings/notifications"},
		"channel_types":         []string{"webhook", "slack", "email", "browser_push"},
		"channels": []map[string]any{{
			"id":           "c1",
			"name":         "ops",
			"channel_type": "webhook",
			"enabled":      true,
			"config": map[string]any{
				"url":                 "https://example.test/hook",
				"mask_output_enabled": "1",
			},
		}},
		"rules": []map[string]any{{
			"id":               "r1",
			"name":             "errors",
			"enabled":          true,
			"logic_operator":   "any",
			"conditions":       []map[string]any{},
			"channel_ids":      []string{"c1"},
			"severity":         "warning",
			"cooldown_seconds": 300,
		}},
		"metric_rules":        []any{},
		"notification_log":    []map[string]any{},
		"condition_types":     []string{"signal", "tag"},
		"signal_sources":      []string{"logs", "errors", "traces", "metrics", "rum"},
		"comparators":         []string{">", ">=", "<", "<=", "==", "!="},
		"tag_match_operators": []string{"equals", "contains", "starts_with", "ends_with", "regex"},
		"tag_record_types":    []string{"all", "logs", "errors", "traces", "metrics", "rum"},
		"edit_rule":           nil,
		"vapid_public_key":    "",
		"vapid_key_source":    "",
	}

	if _, err := srv.renderer.Render("settings_notifications.html", ctx); err != nil {
		t.Fatalf("expected settings_notifications.html to render, got %v", err)
	}
}

func TestParityRepresentativeAPIEndpointsReturnJSON(t *testing.T) {
	srv := newTestServer()

	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
		status int
	}{
		{name: "chart types", method: http.MethodGet, path: "/api/chart-types", status: http.StatusOK},
		{name: "query schema", method: http.MethodGet, path: "/api/query/schema", status: http.StatusOK},
		{name: "logs field hints", method: http.MethodGet, path: "/api/logs/field-hints", status: http.StatusOK},
		{name: "masking rules", method: http.MethodGet, path: "/api/settings/masking/rules", status: http.StatusOK},
	}

	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, "http://example.com"+tc.path, bytes.NewReader(tc.body))
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)

		if rec.Code != tc.status {
			t.Fatalf("%s: expected %d, got %d body=%q", tc.name, tc.status, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
		ct := strings.ToLower(rec.Header().Get("Content-Type"))
		if !strings.HasPrefix(ct, "application/json") {
			t.Fatalf("%s: expected application/json content type, got %q", tc.name, rec.Header().Get("Content-Type"))
		}
		body := strings.ToLower(strings.TrimSpace(rec.Body.String()))
		if strings.Contains(body, "template error") || strings.Contains(body, "<html") {
			t.Fatalf("%s: expected JSON API response, got body=%q", tc.name, strings.TrimSpace(rec.Body.String()))
		}
	}
}

func TestParityV1IngestMethodAndPayloadContracts(t *testing.T) {
	srv := newTestServer()

	for _, path := range []string{"/v1/logs", "/v1/traces", "/v1/metrics", "/v1/errors", "/v1/rum", "/v1/ai"} {
		getReq := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		getRec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(getRec, getReq)
		if getRec.Code != http.StatusMethodNotAllowed {
			t.Fatalf("%s GET: expected 405, got %d", path, getRec.Code)
		}
	}

	for _, tc := range []struct {
		path string
		body []byte
	}{
		{path: "/v1/logs", body: []byte(`{"resourceLogs":[]}`)},
		{path: "/v1/traces", body: []byte(`{"resourceSpans":[]}`)},
		{path: "/v1/metrics", body: []byte(`{"resourceMetrics":[]}`)},
		{path: "/v1/errors", body: []byte(`{"items":[]}`)},
		{path: "/v1/rum", body: []byte(`{"events":[]}`)},
		{path: "/v1/ai", body: []byte(`{"items":[]}`)},
	} {
		req := httptest.NewRequest(http.MethodPost, "http://example.com"+tc.path, bytes.NewReader(tc.body))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s POST valid payload: expected 200, got %d body=%q", tc.path, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}

	invalid := httptest.NewRequest(http.MethodPost, "http://example.com/v1/errors", bytes.NewReader([]byte(`not-json`)))
	invalid.Header.Set("Content-Type", "application/json")
	invalidRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(invalidRec, invalid)
	if invalidRec.Code != http.StatusBadRequest {
		t.Fatalf("/v1/errors invalid payload: expected 400, got %d", invalidRec.Code)
	}
}
