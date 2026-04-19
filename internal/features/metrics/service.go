package metrics

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Rule struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Query     string `json:"query"`
	Threshold string `json:"threshold"`
	CreatedAt string `json:"created_at"`
}

type Service struct {
	mu     sync.RWMutex
	rules  map[string]Rule
	nextID int64
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return NewStoreService(defaultstore.NewFactory())
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) ensureSchema(ctx context.Context) error {
	if s.storeFactory == nil {
		return nil
	}
	s.schemaOnce.Do(func() {
		store, err := persist.Open(ctx, s.storeFactory)
		if err != nil {
			s.schemaErr = err
			return
		}
		defer func() { _ = store.Close() }()
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_anomaly_rules (Id String, Name String, RuleType String DEFAULT 'threshold', SignalSource String, SignalName String, ServiceName String, AttrFingerprint String, Comparator String, WarningThreshold Float64, CriticalThreshold Float64, SecondarySignalSource String DEFAULT '', SecondarySignalName String DEFAULT '', SecondaryComparator String DEFAULT 'gt', SecondaryWarningThreshold Float64 DEFAULT 0, SecondaryCriticalThreshold Float64 DEFAULT 0, MinSampleCount UInt32 DEFAULT 1, SeasonalBucketsJson String DEFAULT '', IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (SignalSource, SignalName, ServiceName, AttrFingerprint, Id)")
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) ListRules() []Rule {
	return s.listRulesStoreBacked(context.Background())
}

func (s *Service) listRulesStoreBacked(ctx context.Context) []Rule {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, Name, concat(SignalSource, ':', SignalName), concat(Comparator, ' ', toString(WarningThreshold), '/', toString(CriticalThreshold)), Version FROM sobs_anomaly_rules FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Rule{}
	for rows.Next() {
		var rule Rule
		var version uint64
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Query, &rule.Threshold, &version); err != nil {
			return out
		}
		rule.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
		out = append(out, rule)
	}
	return out
}

func (s *Service) CreateRule(name, query, threshold string) (Rule, error) {
	return s.createRuleStoreBacked(context.Background(), name, query, threshold)
}

func (s *Service) createRuleStoreBacked(ctx context.Context, name, query, threshold string) (Rule, error) {
	if strings.TrimSpace(name) == "" {
		return Rule{}, errors.New("name is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Rule{}, err
	}
	id := persist.NewID()
	version := persist.Version()
	createdAt := time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
	signalSource := "metrics"
	signalName := query
	if before, after, ok := strings.Cut(query, ":"); ok {
		signalSource = strings.TrimSpace(before)
		signalName = strings.TrimSpace(after)
	}
	warning := 0.0
	critical := 0.0
	comparator := "gt"
	if parts := strings.Fields(threshold); len(parts) >= 2 {
		comparator = strings.TrimSpace(parts[0])
		if value, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
			warning = value
			critical = value
		}
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Rule{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_anomaly_rules (Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, "threshold", signalSource, signalName, "", "", comparator, warning, critical, 0, version)
	if err != nil {
		return Rule{}, err
	}
	return Rule{ID: id, Name: name, Query: query, Threshold: threshold, CreatedAt: createdAt}, nil
}

func (s *Service) DeleteRule(id string) bool {
	return s.deleteRuleStoreBacked(context.Background(), id)
}

func (s *Service) deleteRuleStoreBacked(ctx context.Context, id string) bool {
	if err := s.ensureSchema(ctx); err != nil {
		return false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold FROM sobs_anomaly_rules FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var name, ruleType, signalSource, signalName, serviceName, attrFingerprint, comparator string
	var warning, critical float64
	if err := rows.Scan(&name, &ruleType, &signalSource, &signalName, &serviceName, &attrFingerprint, &comparator, &warning, &critical); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_anomaly_rules (Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, ruleType, signalSource, signalName, serviceName, attrFingerprint, comparator, warning, critical, 1, persist.Version())
	return err == nil
}

func (s *Service) AutoRules() []Rule {
	out := make([]Rule, 0, 2)
	for _, in := range []struct {
		name string
		q    string
		thr  string
	}{{"High Error Rate", "errors_per_minute", "> 50"}, {"Latency Regression", "p95_latency_ms", "> 1000"}} {
		r, _ := s.CreateRule(in.name, in.q, in.thr)
		out = append(out, r)
	}
	return out
}

func (s *Service) AutoDashboardRules() []Rule {
	out := make([]Rule, 0, 1)
	r, _ := s.CreateRule("Dashboard Spike", "dashboard_error_spike", "> 30")
	out = append(out, r)
	return out
}

func (s *Service) AnomalySnapshot() map[string]any {
	return s.anomalySnapshotStoreBacked(context.Background())
}

func (s *Service) anomalySnapshotStoreBacked(ctx context.Context) map[string]any {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return map[string]any{"ok": false, "anomalies": []map[string]any{}}
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT SignalName, value, baseline_mean, anomaly_state FROM v_derived_signals_anomaly WHERE anomaly_state != 'normal' ORDER BY time DESC LIMIT 10")
	if err != nil {
		return map[string]any{"ok": true, "anomalies": []map[string]any{}}
	}
	defer func() { _ = rows.Close() }()
	anomalies := []map[string]any{}
	for rows.Next() {
		var metric string
		var current float64
		var baseline float64
		var severity string
		if err := rows.Scan(&metric, &current, &baseline, &severity); err != nil {
			return map[string]any{"ok": true, "anomalies": anomalies}
		}
		anomalies = append(anomalies, map[string]any{"metric": metric, "current": current, "baseline": baseline, "severity": severity})
	}
	return map[string]any{"ok": true, "anomalies": anomalies}
}
