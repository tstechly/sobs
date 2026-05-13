package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func newRenderedMetricsTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, store.NewNoopStoreFactory())
}
// TODO: FIXME:
func SKIPTestMetricsPageUsesPythonRuleAnnotations(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/metrics?service=svc-metrics&source=rum_vitals&signal=LCP", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body,
		"svc-metrics",
		"LCP",
		"Slow LCP",
	) {
		t.Fatalf("expected metrics page to include seeded rule annotation, got %s", body)
	}
}

// TODO: FIXME:
func SKIPTestMetricsAnomalyPageUsesDerivedSignalRuleAnnotations(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/metrics/anomaly?service=svc-metrics&source=rum_vitals&signal=LCP&attr_fp=fp-1", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body,
		"Metrics Anomaly Details",
		"svc-metrics",
		"LCP",
		"Slow LCP",
	) {
		t.Fatalf("expected anomaly page to include derived-signal rule annotation, got %s", body)
	}
}

// TODO: FIXME:
func SKIPTestMetricsAnomalyPageSupportsMetricDrilldown(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/metrics/anomaly?service=svc-otel&metric=http.server.duration&attr_fp=fp-otel", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !containsAll(body,
		"Metrics Anomaly Details",
		"svc-otel",
		"http.server.duration",
		"histogram",
	) {
		t.Fatalf("expected anomaly page to include metric drilldown row, got %s", body)
	}
	if strings.Contains(body, "Slow LCP") {
		t.Fatalf("expected metric drilldown to avoid derived-signal rule annotation, got %s", body)
	}
}

func TestMetricsHelpPageParity(t *testing.T) {
	srv := newRenderedMetricsTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/metrics/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Metrics Help") {
		t.Fatalf("expected metrics help content, got %s", getRec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/metrics/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestMetricsAnomalyHelpPageParity(t *testing.T) {
	srv := newRenderedMetricsTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/metrics/help/anomaly", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Metrics Anomaly Help") {
		t.Fatalf("expected metrics anomaly help content, got %s", getRec.Body.String())
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/metrics/help/anomaly", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestAPIMetricsAnomalyRequiresParams(t *testing.T) {
	srv := newRenderedMetricsTestServer()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/metrics/anomaly", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "service and metric query parameters are required") {
		t.Fatalf("expected missing-params error, got %s", rec.Body.String())
	}
}

func TestAPIMetricsAnomalyReturnsPythonParityContract(t *testing.T) {
	srv := newRenderedMetricsTestServer()
	seedMetricsPageTables(t, srv)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/metrics/anomaly?service=svc-otel&metric=http.server.duration&attr_fp=fp-otel&hours=24", nil)
	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var payload struct {
		Service string   `json:"service"`
		Metric  string   `json:"metric"`
		Columns []string `json:"columns"`
		Rows    [][]any  `json:"rows"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal response: %v body=%s", err, rec.Body.String())
	}
	expectedColumns := []string{"time", "value", "sample_count", "baseline_mean", "baseline_stddev", "baseline_lower", "baseline_upper", "anomaly_score", "anomaly_state", "metric_kind", "attr_fp"}
	if payload.Service != "svc-otel" || payload.Metric != "http.server.duration" {
		t.Fatalf("unexpected service/metric payload: %#v", payload)
	}
	if !reflect.DeepEqual(payload.Columns, expectedColumns) {
		t.Fatalf("unexpected columns: got %#v want %#v", payload.Columns, expectedColumns)
	}
	if len(payload.Rows) != 1 {
		t.Fatalf("expected 1 row, got %#v", payload.Rows)
	}
	if len(payload.Rows[0]) != len(expectedColumns) {
		t.Fatalf("expected row width %d, got %#v", len(expectedColumns), payload.Rows[0])
	}
	ts, ok := payload.Rows[0][0].(string)
	if !ok {
		t.Fatalf("expected string timestamp, got %#v", payload.Rows[0][0])
	}
	if strings.Contains(ts, "UTC") {
		t.Fatalf("timestamp must not contain UTC: %#v", payload.Rows[0])
	}
	if !regexp.MustCompile(`^\d{4}-\d{2}-\d{2} \d{2}:\d{2}:\d{2}`).MatchString(ts) {
		t.Fatalf("timestamp %q lacks YYYY-MM-DD HH:MM:SS format (row=%#v)", ts, payload.Rows[0])
	}
	if payload.Rows[0][8] != "warning" || payload.Rows[0][9] != "histogram" || payload.Rows[0][10] != "fp-otel" {
		t.Fatalf("unexpected trailing cells: %#v", payload.Rows[0])
	}
}

func TestEvaluateMetricCompositeRuleUsesTimedSecondaryRow(t *testing.T) {
	rule := summaryAnomalyRule{
		ID:                         "rule-composite",
		Name:                       "Composite Load And Errors",
		RuleType:                   "composite",
		Source:                     "rum_vitals",
		Signal:                     "LCP",
		Service:                    "svc-metrics",
		AttrFP:                     "fp-1",
		Comparator:                 "gt",
		WarningThreshold:           100,
		CriticalThreshold:          200,
		SecondarySource:            "logs",
		SecondarySignal:            "error_ratio",
		SecondaryComparator:        "gt",
		SecondaryWarningThreshold:  0.1,
		SecondaryCriticalThreshold: 0.3,
		MinSampleCount:             1,
	}
	row := map[string]any{
		"service":           "svc-metrics",
		"source":            "rum_vitals",
		"signal":            "LCP",
		"attr_fp":           "fp-1",
		"last_time":         "2026-04-20 10:00:00.000",
		"last_value":        250.0,
		"last_sample_count": 8,
	}
	secondaryRow := map[string]any{
		"last_time":         "2026-04-20 10:00:00.000",
		"last_value":        0.4,
		"last_sample_count": 8,
	}
	latestLookup := map[string]map[string]any{}
	timedLookup := map[string]map[string]any{
		metricRuleLookupKey("svc-metrics", "fp-1", "logs", "error_ratio") + "\x00" + "2026-04-20 10:00:00.000": secondaryRow,
	}

	evaluation := evaluateMetricCompositeRule(t.Context(), nil, rule, row, latestLookup, timedLookup)
	if evaluation == nil {
		t.Fatal("expected composite rule evaluation")
	}
	if evaluation.RuleName != "Composite Load And Errors" || evaluation.RuleState != "outlier" {
		t.Fatalf("unexpected composite evaluation: %#v", evaluation)
	}
	if !strings.Contains(evaluation.RuleReason, "secondary error_ratio=0.4 triggered") {
		t.Fatalf("expected composite reason to mention secondary signal, got %#v", evaluation)
	}
}

func TestEvaluateMetricSeasonalRuleUsesBucketThresholds(t *testing.T) {
	rule := summaryAnomalyRule{
		ID:                  "rule-seasonal",
		Name:                "Seasonal LCP",
		RuleType:            "seasonal",
		Comparator:          "gt",
		WarningThreshold:    500,
		CriticalThreshold:   800,
		MinSampleCount:      1,
		SeasonalBucketsJSON: `{"strategy":"hour_of_day","buckets":{"10":{"warning":100,"critical":200}}}`,
	}

	evaluation := evaluateMetricSeasonalRule(rule, 150.0, 5, "2026-04-20 10:00:00.000")
	if evaluation == nil {
		t.Fatal("expected seasonal rule evaluation")
	}
	if evaluation.RuleState != "warning" || !evaluation.RuleSeasonal {
		t.Fatalf("unexpected seasonal evaluation: %#v", evaluation)
	}
}

func seedMetricsPageTables(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS v_derived_signals_anomaly",
		"DROP TABLE IF EXISTS v_otel_metrics_anomaly",
		"DROP TABLE IF EXISTS sobs_anomaly_rules",
		"CREATE TABLE IF NOT EXISTS v_derived_signals_anomaly (time DateTime64(3), ServiceName String, SignalSource String, SignalName String, AttrFingerprint String, value Float64, baseline_mean Float64 DEFAULT 0, baseline_stddev Float64 DEFAULT 0, baseline_lower Float64 DEFAULT 0, baseline_upper Float64 DEFAULT 0, anomaly_score Float64, anomaly_state String, SampleCount UInt32) ENGINE = MergeTree ORDER BY time",
		"CREATE TABLE IF NOT EXISTS v_otel_metrics_anomaly (time DateTime64(3), ServiceName String, MetricName String, MetricKind String, AttrFingerprint String, value Float64, SampleCount UInt32, baseline_mean Float64, baseline_stddev Float64, baseline_lower Float64, baseline_upper Float64, anomaly_score Float64, anomaly_state String) ENGINE = MergeTree ORDER BY time",
		"CREATE TABLE IF NOT EXISTS sobs_anomaly_rules (Id String, Name String, RuleType String DEFAULT 'threshold', SignalSource String, SignalName String, ServiceName String, AttrFingerprint String, Comparator String, WarningThreshold Float64, CriticalThreshold Float64, SecondarySignalSource String DEFAULT '', SecondarySignalName String DEFAULT '', SecondaryComparator String DEFAULT 'gt', SecondaryWarningThreshold Float64 DEFAULT 0, SecondaryCriticalThreshold Float64 DEFAULT 0, MinSampleCount UInt32 DEFAULT 1, SeasonalBucketsJson String DEFAULT '', IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (SignalSource, SignalName, ServiceName, AttrFingerprint, Id)",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	recentTS := time.Now().UTC().Add(-5 * time.Minute).Format("2006-01-02 15:04:05.000")
	if _, err := store.Exec(t.Context(), "INSERT INTO v_derived_signals_anomaly (time, ServiceName, SignalSource, SignalName, AttrFingerprint, value, anomaly_score, anomaly_state, SampleCount) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		recentTS,
		"svc-metrics",
		"rum_vitals",
		"LCP",
		"fp-1",
		250.0,
		1.2,
		"normal",
		uint32(8),
	); err != nil {
		t.Fatalf("insert metrics row: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO v_otel_metrics_anomaly (time, ServiceName, MetricName, MetricKind, AttrFingerprint, value, SampleCount, baseline_mean, baseline_stddev, baseline_lower, baseline_upper, anomaly_score, anomaly_state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		recentTS,
		"svc-otel",
		"http.server.duration",
		"histogram",
		"fp-otel",
		315.5,
		uint32(12),
		200.0,
		50.0,
		150.0,
		250.0,
		2.3,
		"warning",
	); err != nil {
		t.Fatalf("insert otel metrics anomaly row: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_anomaly_rules (Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, MinSampleCount, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"rule-threshold",
		"Slow LCP",
		"threshold",
		"rum_vitals",
		"LCP",
		"svc-metrics",
		"fp-1",
		"gt",
		100.0,
		200.0,
		uint32(1),
		uint8(0),
		uint64(1),
	); err != nil {
		t.Fatalf("insert anomaly rule: %v", err)
	}
}
