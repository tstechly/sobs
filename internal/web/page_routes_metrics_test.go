package web

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	"github.com/abartrim/sobs/internal/store"
)

func newRenderedMetricsTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, store.NewNoopStoreFactory())
}

func TestMetricsPageUsesPythonRuleAnnotations(t *testing.T) {
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
		ID:                 "rule-seasonal",
		Name:               "Seasonal LCP",
		RuleType:           "seasonal",
		Comparator:         "gt",
		WarningThreshold:   500,
		CriticalThreshold:  800,
		MinSampleCount:     1,
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
		"DROP TABLE IF EXISTS sobs_anomaly_rules",
		"CREATE TABLE IF NOT EXISTS v_derived_signals_anomaly (time DateTime64(3), ServiceName String, SignalSource String, SignalName String, AttrFingerprint String, value Float64, anomaly_score Float64, anomaly_state String, SampleCount UInt32) ENGINE = MergeTree ORDER BY time",
		"CREATE TABLE IF NOT EXISTS sobs_anomaly_rules (Id String, Name String, RuleType String DEFAULT 'threshold', SignalSource String, SignalName String, ServiceName String, AttrFingerprint String, Comparator String, WarningThreshold Float64, CriticalThreshold Float64, SecondarySignalSource String DEFAULT '', SecondarySignalName String DEFAULT '', SecondaryComparator String DEFAULT 'gt', SecondaryWarningThreshold Float64 DEFAULT 0, SecondaryCriticalThreshold Float64 DEFAULT 0, MinSampleCount UInt32 DEFAULT 1, SeasonalBucketsJson String DEFAULT '', IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (SignalSource, SignalName, ServiceName, AttrFingerprint, Id)",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO v_derived_signals_anomaly (time, ServiceName, SignalSource, SignalName, AttrFingerprint, value, anomaly_score, anomaly_state, SampleCount) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"2026-04-20 10:00:00.000",
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