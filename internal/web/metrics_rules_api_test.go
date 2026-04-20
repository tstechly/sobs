package web

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestMetricsRulesCreateAndDeleteParity(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsRulesParityTables(t, srv)

	createReq := newFormRequest(
		http.MethodPost,
		"http://example.com/metrics/rules",
		url.Values{
			"name":                         {"Composite Metric Rule"},
			"rule_type":                    {"composite"},
			"source":                       {"rum_vitals"},
			"signal":                       {"LCP"},
			"service":                      {"svc-metrics"},
			"attr_fp":                      {"fp-1"},
			"comparator":                   {"gt"},
			"warning_threshold":            {"150"},
			"critical_threshold":           {"250"},
			"secondary_source":             {"logs"},
			"secondary_signal":             {"error_ratio"},
			"secondary_comparator":         {"gt"},
			"secondary_warning_threshold":  {"0.2"},
			"secondary_critical_threshold": {"0.4"},
			"min_sample_count":             {"3"},
		},
	)
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if location := createRec.Header().Get("Location"); location != "/metrics/rules" {
		t.Fatalf("expected redirect to /metrics/rules, got %q", location)
	}

	ruleID := queryRuleIDByName(t, srv, "Composite Metric Rule")
	if ruleID == "" {
		t.Fatal("expected created composite rule")
	}

	delReq := newFormRequest(http.MethodPost, "http://example.com/metrics/rules/"+ruleID+"/delete", url.Values{})
	delRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(delRec, delReq)
	if delRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", delRec.Code, delRec.Body.String())
	}
	if location := delRec.Header().Get("Location"); location != "/metrics/rules" {
		t.Fatalf("expected redirect to /metrics/rules, got %q", location)
	}
	if count := countActiveRulesByName(t, srv, "Composite Metric Rule"); count != 0 {
		t.Fatalf("expected deleted rule to be inactive, found %d rows", count)
	}
}

func TestMetricsRulesAutoPreviewAndCreateParity(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsRulesParityTables(t, srv)

	previewReq := newFormRequest(
		http.MethodPost,
		"http://example.com/metrics/rules/auto",
		url.Values{
			"action":            {"preview"},
			"hours":             {"24"},
			"min_points":        {"3"},
			"service_filter":    {"svc-auto"},
			"include_attr_fp":   {"1"},
			"mode":              {"seasonal"},
			"seasonal_strategy": {"hour_of_day"},
		},
	)
	previewRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", previewRec.Code, previewRec.Body.String())
	}
	body := previewRec.Body.String()
	if !containsAll(body, "Preview Candidates (1)", "error_ratio", "svc-auto", "seasonal") {
		t.Fatalf("expected seasonal preview content, got %s", body)
	}

	createReq := newFormRequest(
		http.MethodPost,
		"http://example.com/metrics/rules/auto",
		url.Values{
			"action":         {"create"},
			"hours":          {"24"},
			"min_points":     {"3"},
			"service_filter": {"svc-auto"},
			"mode":           {"threshold"},
		},
	)
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	if location := createRec.Header().Get("Location"); location != "/metrics/rules?open_panel=auto-rules" {
		t.Fatalf("expected redirect to auto-rules panel, got %q", location)
	}
	if count := countActiveRulesByName(t, srv, "Auto logs/error_ratio [svc-auto]"); count != 1 {
		t.Fatalf("expected one created auto rule, found %d", count)
	}
}

func TestMetricsRulesDashboardPreviewAndCreateParity(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsRulesParityTables(t, srv)

	previewReq := newFormRequest(
		http.MethodPost,
		"http://example.com/metrics/rules/dashboard/auto",
		url.Values{
			"action":         {"preview"},
			"hours":          {"24"},
			"max_charts":     {"4"},
			"service_filter": {"svc-metrics"},
			"dashboard_name": {"Rules Preview Dashboard"},
		},
	)
	previewRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(previewRec, previewReq)
	if previewRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", previewRec.Code, previewRec.Body.String())
	}
	if !containsAll(previewRec.Body.String(), "Dashboard Preview Candidates", "Slow LCP") {
		t.Fatalf("expected dashboard preview content, got %s", previewRec.Body.String())
	}

	createReq := newFormRequest(
		http.MethodPost,
		"http://example.com/metrics/rules/dashboard/auto",
		url.Values{
			"action":         {"create"},
			"hours":          {"24"},
			"max_charts":     {"4"},
			"service_filter": {"svc-metrics"},
			"dashboard_name": {"Rules Create Dashboard"},
		},
	)
	createRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303, got %d body=%s", createRec.Code, createRec.Body.String())
	}
	dashboardID := queryDashboardIDByName(t, srv, "Rules Create Dashboard")
	if dashboardID == "" {
		t.Fatal("expected created dashboard")
	}
	if location := createRec.Header().Get("Location"); location != "/dashboards/"+dashboardID {
		t.Fatalf("expected redirect to dashboard, got %q", location)
	}
	if count := countDashboardCharts(t, srv, dashboardID); count == 0 {
		t.Fatal("expected created dashboard charts")
	}
}

func TestMetricsAnomalyEndpointsRemainAvailable(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsRulesParityTables(t, srv)

	for _, path := range []string{"/metrics/rules", "/metrics/anomaly", "/api/metrics/anomaly?service=svc-otel&metric=http.server.duration"} {
		req := httptest.NewRequest(http.MethodGet, "http://example.com"+path, nil)
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d body=%s", path, rec.Code, rec.Body.String())
		}
	}
}

func seedMetricsRulesParityTables(t *testing.T, srv *Server) {
	t.Helper()
	seedMetricsPageTables(t, srv)

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	if _, err := store.Exec(t.Context(), "CREATE TABLE IF NOT EXISTS sobs_dashboards (Id String, Name String, Description String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id"); err != nil {
		t.Fatalf("create sobs_dashboards: %v", err)
	}
	if _, err := store.Exec(t.Context(), "CREATE TABLE IF NOT EXISTS sobs_chart_configs (Id String, DashboardId String, Title String, ChartType String, Query String, OptionsJson String, Position UInt16 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (DashboardId, Id)"); err != nil {
		t.Fatalf("create sobs_chart_configs: %v", err)
	}

	seriesRows := []struct {
		time        string
		service     string
		source      string
		signal      string
		attrFP      string
		value       float64
		sampleCount uint32
	}{
		{"2026-04-20 10:00:00.000", "svc-auto", "logs", "error_ratio", "fp-auto", 0.10, 10},
		{"2026-04-20 10:10:00.000", "svc-auto", "logs", "error_ratio", "fp-auto", 0.20, 12},
		{"2026-04-20 10:20:00.000", "svc-auto", "logs", "error_ratio", "fp-auto", 0.30, 11},
		{"2026-04-20 10:30:00.000", "svc-auto", "logs", "error_ratio", "fp-auto", 0.40, 13},
	}
	for _, row := range seriesRows {
		if _, err := store.Exec(t.Context(), "INSERT INTO v_derived_signals_anomaly (time, ServiceName, SignalSource, SignalName, AttrFingerprint, value, anomaly_score, anomaly_state, SampleCount) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)", row.time, row.service, row.source, row.signal, row.attrFP, row.value, 0.5, "normal", row.sampleCount); err != nil {
			t.Fatalf("insert derived signal row: %v", err)
		}
	}
}

func newFormRequest(method string, target string, values url.Values) *http.Request {
	body := strings.NewReader(values.Encode())
	req := httptest.NewRequest(method, target, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func queryRuleIDByName(t *testing.T, srv *Server, name string) string {
	t.Helper()
	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(t.Context(), "SELECT Id FROM sobs_anomaly_rules FINAL WHERE IsDeleted = 0 AND Name = ? LIMIT 1", name)
	if err != nil {
		t.Fatalf("query rule id: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return ""
	}
	var ruleID string
	if err := rows.Scan(&ruleID); err != nil {
		t.Fatalf("scan rule id: %v", err)
	}
	return ruleID
}

func countActiveRulesByName(t *testing.T, srv *Server, name string) int {
	t.Helper()
	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(t.Context(), "SELECT count() FROM sobs_anomaly_rules FINAL WHERE IsDeleted = 0 AND Name = ?", name)
	if err != nil {
		t.Fatalf("count rules: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("scan rule count: %v", err)
	}
	return count
}

func queryDashboardIDByName(t *testing.T, srv *Server, name string) string {
	t.Helper()
	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(t.Context(), "SELECT Id FROM sobs_dashboards FINAL WHERE IsDeleted = 0 AND Name = ? LIMIT 1", name)
	if err != nil {
		t.Fatalf("query dashboard id: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return ""
	}
	var dashboardID string
	if err := rows.Scan(&dashboardID); err != nil {
		t.Fatalf("scan dashboard id: %v", err)
	}
	return dashboardID
}

func countDashboardCharts(t *testing.T, srv *Server, dashboardID string) int {
	t.Helper()
	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(t.Context(), "SELECT count() FROM sobs_chart_configs FINAL WHERE IsDeleted = 0 AND DashboardId = ?", dashboardID)
	if err != nil {
		t.Fatalf("count dashboard charts: %v", err)
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		t.Fatalf("scan dashboard chart count: %v", err)
	}
	return count
}
