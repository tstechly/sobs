package web

import (
	"context"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

const (
	autoRuleCreateMax       = 200
	autoDashboardCreateMax  = 24
	seasonalMinBucketPoints = 3
)

var (
	autoRuleGTHints    = []string{"error", "latency", "duration", "timeout", "p95", "p99", "failure", "fail", "retry"}
	autoRuleLTHints    = []string{"availability", "success", "throughput", "rps", "qps"}
	seasonalStrategies = map[string]struct{}{"hour_of_day": {}, "day_of_week": {}}
)

func (s *Server) metricsRules(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		if r.URL.Path != "/metrics/rules" {
			http.NotFound(w, r)
			return
		}
		if s.renderer == nil || s.renderErr != nil {
			http.Error(w, "template error", http.StatusInternalServerError)
			return
		}
		openPanel := strings.TrimSpace(strings.ToLower(r.URL.Query().Get("open_panel")))
		if openPanel != "auto-rules" && openPanel != "auto-dashboard" {
			openPanel = ""
		}
		services := []string{}
		signals := []string{}
		sources := []string{}
		rules := []map[string]any{}
		store, err := s.storeFactory.Open(r.Context())
		if err == nil {
			defer store.Close()
			services, signals, sources = listDerivedSignalDimensions(r, store)
			for _, rule := range summaryLoadAnomalyRules(r.Context(), store) {
				rules = append(rules, summaryMetricRuleForTemplate(rule))
			}
		}
		s.renderMetricsRulesPage(w, rules, services, signals, sources, nil, []map[string]any{}, nil, []map[string]any{}, openPanel)
	case http.MethodPost:
		if err := r.ParseForm(); err != nil {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		name := strings.TrimSpace(r.Form.Get("name"))
		ruleType := strings.ToLower(strings.TrimSpace(defaultString(r.Form.Get("rule_type"), "threshold")))
		source := strings.TrimSpace(r.Form.Get("source"))
		signal := strings.TrimSpace(r.Form.Get("signal"))
		service := strings.TrimSpace(r.Form.Get("service"))
		attrFP := strings.TrimSpace(r.Form.Get("attr_fp"))
		comparator := strings.ToLower(strings.TrimSpace(defaultString(r.Form.Get("comparator"), "gt")))
		secondarySource := strings.TrimSpace(r.Form.Get("secondary_source"))
		secondarySignal := strings.TrimSpace(r.Form.Get("secondary_signal"))
		secondaryComparator := strings.ToLower(strings.TrimSpace(defaultString(r.Form.Get("secondary_comparator"), "gt")))

		if name == "" || source == "" || signal == "" {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		if ruleType != "threshold" && ruleType != "composite" {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		if comparator != "gt" && comparator != "lt" {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		if secondaryComparator != "gt" && secondaryComparator != "lt" {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}

		warningThreshold, err := strconv.ParseFloat(strings.TrimSpace(r.Form.Get("warning_threshold")), 64)
		if err != nil {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		criticalThreshold, err := strconv.ParseFloat(strings.TrimSpace(r.Form.Get("critical_threshold")), 64)
		if err != nil {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		minSampleCount := coercePositiveInt(r.Form.Get("min_sample_count"), 1, 1, 1_000_000)
		secondaryWarningThreshold := coerceFloatDefault(r.Form.Get("secondary_warning_threshold"), 0)
		secondaryCriticalThreshold := coerceFloatDefault(r.Form.Get("secondary_critical_threshold"), 0)

		if comparator == "gt" && criticalThreshold < warningThreshold {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		if comparator == "lt" && criticalThreshold > warningThreshold {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		if ruleType == "composite" {
			if secondarySource == "" || secondarySignal == "" {
				http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
				return
			}
			if secondaryComparator == "gt" && secondaryCriticalThreshold < secondaryWarningThreshold {
				http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
				return
			}
			if secondaryComparator == "lt" && secondaryCriticalThreshold > secondaryWarningThreshold {
				http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
				return
			}
		} else {
			secondarySource = ""
			secondarySignal = ""
			secondaryComparator = "gt"
			secondaryWarningThreshold = 0
			secondaryCriticalThreshold = 0
		}

		store, err := s.storeFactory.Open(r.Context())
		if err != nil {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		defer store.Close()
		if err := ensureMetricsRulesSchema(r.Context(), store); err != nil {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		_, err = store.Exec(
			r.Context(),
			"INSERT INTO sobs_anomaly_rules (Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, SecondarySignalSource, SecondarySignalName, SecondaryComparator, SecondaryWarningThreshold, SecondaryCriticalThreshold, MinSampleCount, SeasonalBucketsJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
			persist.NewID(),
			name,
			ruleType,
			source,
			signal,
			service,
			attrFP,
			comparator,
			warningThreshold,
			criticalThreshold,
			secondarySource,
			secondarySignal,
			secondaryComparator,
			secondaryWarningThreshold,
			secondaryCriticalThreshold,
			minSampleCount,
			"",
			0,
			persist.Version(),
		)
		if err != nil {
			http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func summaryMetricRuleForTemplate(rule summaryAnomalyRule) map[string]any {
	return map[string]any{
		"id":                           rule.ID,
		"name":                         rule.Name,
		"rule_type":                    rule.RuleType,
		"source":                       rule.Source,
		"signal":                       rule.Signal,
		"service":                      rule.Service,
		"attr_fp":                      rule.AttrFP,
		"comparator":                   rule.Comparator,
		"warning_threshold":            rule.WarningThreshold,
		"critical_threshold":           rule.CriticalThreshold,
		"secondary_source":             rule.SecondarySource,
		"secondary_signal":             rule.SecondarySignal,
		"secondary_comparator":         rule.SecondaryComparator,
		"secondary_warning_threshold":  rule.SecondaryWarningThreshold,
		"secondary_critical_threshold": rule.SecondaryCriticalThreshold,
		"seasonal_buckets_json":        rule.SeasonalBucketsJSON,
		"min_sample_count":             rule.MinSampleCount,
	}
}

func (s *Server) metricsRulesAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-rules", http.StatusSeeOther)
		return
	}
	action := strings.ToLower(strings.TrimSpace(defaultString(r.Form.Get("action"), "preview")))
	hours := coercePositiveInt(r.Form.Get("hours"), 24, 1, 168)
	minPoints := coercePositiveInt(r.Form.Get("min_points"), 30, 1, 5000)
	serviceFilter := strings.TrimSpace(r.Form.Get("service_filter"))
	includeAttrFP := parseBool(strings.TrimSpace(r.Form.Get("include_attr_fp")))
	mode := strings.ToLower(strings.TrimSpace(defaultString(r.Form.Get("mode"), "threshold")))
	if mode != "threshold" && mode != "seasonal" {
		mode = "threshold"
	}
	seasonalStrategy := strings.ToLower(strings.TrimSpace(defaultString(r.Form.Get("seasonal_strategy"), "hour_of_day")))
	if _, ok := seasonalStrategies[seasonalStrategy]; !ok {
		seasonalStrategy = "hour_of_day"
	}

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-rules", http.StatusSeeOther)
		return
	}
	defer store.Close()
	if err := ensureMetricsRulesSchema(r.Context(), store); err != nil {
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-rules", http.StatusSeeOther)
		return
	}
	services, signals, sources := listDerivedSignalDimensions(r, store)
	rules := []map[string]any{}
	for _, rule := range summaryLoadAnomalyRules(r.Context(), store) {
		rules = append(rules, summaryMetricRuleForTemplate(rule))
	}

	autoPreview := []map[string]any{}
	stats := autoRuleStats{}
	if mode == "seasonal" {
		autoPreview, stats, err = buildSeasonalMetricRuleCandidates(r.Context(), store, hours, minPoints, serviceFilter, includeAttrFP, seasonalStrategy)
	} else {
		autoPreview, stats, err = buildAutoMetricRuleCandidates(r.Context(), store, hours, minPoints, serviceFilter, includeAttrFP)
	}
	if err != nil && !isMissingTableError(err) {
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-rules", http.StatusSeeOther)
		return
	}
	autoSummary := map[string]any{
		"action":            action,
		"hours":             hours,
		"min_points":        minPoints,
		"service_filter":    serviceFilter,
		"include_attr_fp":   includeAttrFP,
		"mode":              mode,
		"seasonal_strategy": seasonalStrategy,
		"examined":          stats.examined,
		"existing":          stats.existing,
		"invalid":           stats.invalid,
		"candidates":        len(autoPreview),
		"create_cap":        autoRuleCreateMax,
		"capped":            len(autoPreview) > autoRuleCreateMax,
		"created":           0,
	}

	if action == "create" {
		limited := autoPreview
		if len(limited) > autoRuleCreateMax {
			limited = limited[:autoRuleCreateMax]
		}
		version := persist.Version()
		for index, candidate := range limited {
			_, err = store.Exec(
				r.Context(),
				"INSERT INTO sobs_anomaly_rules (Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, SecondarySignalSource, SecondarySignalName, SecondaryComparator, SecondaryWarningThreshold, SecondaryCriticalThreshold, MinSampleCount, SeasonalBucketsJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
				persist.NewID(),
				anyToString(candidate["name"]),
				anyToString(candidate["rule_type"]),
				anyToString(candidate["source"]),
				anyToString(candidate["signal"]),
				anyToString(candidate["service"]),
				anyToString(candidate["attr_fp"]),
				anyToString(candidate["comparator"]),
				anyToFloat(candidate["warning_threshold"]),
				anyToFloat(candidate["critical_threshold"]),
				"",
				"",
				"gt",
				0.0,
				0.0,
				anyToInt(candidate["min_sample_count"]),
				anyToString(candidate["seasonal_buckets_json"]),
				0,
				version+uint64(index),
			)
			if err != nil {
				break
			}
		}
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-rules", http.StatusSeeOther)
		return
	}

	s.renderMetricsRulesPage(w, rules, services, signals, sources, autoSummary, autoPreview, nil, []map[string]any{}, "auto-rules")
}

func (s *Server) metricsRulesDashboardAuto(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-dashboard", http.StatusSeeOther)
		return
	}
	action := strings.ToLower(strings.TrimSpace(defaultString(r.Form.Get("action"), "preview")))
	serviceFilter := strings.TrimSpace(r.Form.Get("service_filter"))
	hours := coercePositiveInt(r.Form.Get("hours"), 24, 1, 168)
	maxCharts := coercePositiveInt(r.Form.Get("max_charts"), 12, 1, autoDashboardCreateMax)
	dashboardName := strings.TrimSpace(r.Form.Get("dashboard_name"))
	if dashboardName == "" {
		dashboardName = defaultAutoDashboardName(serviceFilter)
	}

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-dashboard", http.StatusSeeOther)
		return
	}
	defer store.Close()
	if err := ensureMetricsRulesSchema(r.Context(), store); err != nil {
		http.Redirect(w, r, "/metrics/rules?open_panel=auto-dashboard", http.StatusSeeOther)
		return
	}
	services, signals, sources := listDerivedSignalDimensions(r, store)
	loadedRules := summaryLoadAnomalyRules(r.Context(), store)
	rules := make([]map[string]any, 0, len(loadedRules))
	for _, rule := range loadedRules {
		rules = append(rules, summaryMetricRuleForTemplate(rule))
	}
	autoDashboardPreview := buildAutoDashboardChartCandidates(loadedRules, serviceFilter, hours)
	autoDashboardSummary := map[string]any{
		"action":         action,
		"hours":          hours,
		"service_filter": serviceFilter,
		"max_charts":     maxCharts,
		"create_cap":     autoDashboardCreateMax,
		"dashboard_name": dashboardName,
		"rules_total":    len(loadedRules),
		"candidates":     len(autoDashboardPreview),
		"capped":         len(autoDashboardPreview) > maxCharts,
		"created":        0,
		"existing":       0,
	}

	if action == "create" {
		if len(autoDashboardPreview) == 0 {
			http.Redirect(w, r, "/metrics/rules?open_panel=auto-dashboard", http.StatusSeeOther)
			return
		}
		cappedCandidates := autoDashboardPreview
		if len(cappedCandidates) > maxCharts {
			cappedCandidates = cappedCandidates[:maxCharts]
		}
		dashboardID, err := seedDashboardIfMissing(r.Context(), store, dashboardName, "Auto-generated from active metric rules. window="+strconv.Itoa(hours)+"h, scope="+defaultString(serviceFilter, "all services")+".")
		if err != nil {
			http.Redirect(w, r, "/metrics/rules?open_panel=auto-dashboard", http.StatusSeeOther)
			return
		}
		existingCharts, err := loadDashboardCharts(r.Context(), store, dashboardID)
		if err != nil {
			http.Redirect(w, r, "/metrics/rules?open_panel=auto-dashboard", http.StatusSeeOther)
			return
		}
		existingTitles := make(map[string]struct{}, len(existingCharts))
		nextPosition := 0
		for _, chart := range existingCharts {
			existingTitles[chart.title] = struct{}{}
			if chart.position >= nextPosition {
				nextPosition = chart.position + 1
			}
		}
		version := persist.Version()
		for index, candidate := range cappedCandidates {
			title := anyToString(candidate["title"])
			if _, exists := existingTitles[title]; exists {
				continue
			}
			_, err = store.Exec(
				r.Context(),
				"INSERT INTO sobs_chart_configs (Id, DashboardId, Title, ChartType, Query, OptionsJson, Position, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)",
				persist.NewID(),
				dashboardID,
				title,
				anyToString(candidate["chart_type"]),
				anyToString(candidate["query"]),
				persist.JSONString(map[string]any{"chart_spec": buildRawChartSpec(anyToString(candidate["chart_type"]), anyToString(candidate["query"]), "")}),
				nextPosition+index,
				0,
				version+uint64(index),
			)
			if err != nil {
				break
			}
		}
		if dashboardID != "" {
			http.Redirect(w, r, "/dashboards/"+dashboardID, http.StatusSeeOther)
			return
		}
	}

	s.renderMetricsRulesPage(w, rules, services, signals, sources, nil, []map[string]any{}, autoDashboardSummary, autoDashboardPreview, "auto-dashboard")
}

func (s *Server) metricsRulesSubroutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/metrics/rules/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[1] != "delete" || parts[0] == "" {
		http.NotFound(w, r)
		return
	}
	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
		return
	}
	defer store.Close()
	if err := ensureMetricsRulesSchema(r.Context(), store); err != nil {
		http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
		return
	}
	rows, err := store.Query(r.Context(), "SELECT Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, SecondarySignalSource, SecondarySignalName, SecondaryComparator, SecondaryWarningThreshold, SecondaryCriticalThreshold, MinSampleCount FROM sobs_anomaly_rules FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", parts[0])
	if err != nil {
		http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
		return
	}
	defer rows.Close()
	if !rows.Next() {
		http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
		return
	}
	var name, ruleType, signalSource, signalName, serviceName, attrFingerprint, comparator, secondarySource, secondarySignal, secondaryComparator any
	var warningThreshold, criticalThreshold, secondaryWarningThreshold, secondaryCriticalThreshold, minSampleCount any
	if err := rows.Scan(&name, &ruleType, &signalSource, &signalName, &serviceName, &attrFingerprint, &comparator, &warningThreshold, &criticalThreshold, &secondarySource, &secondarySignal, &secondaryComparator, &secondaryWarningThreshold, &secondaryCriticalThreshold, &minSampleCount); err != nil {
		http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
		return
	}
	_, err = store.Exec(
		r.Context(),
		"INSERT INTO sobs_anomaly_rules (Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, SecondarySignalSource, SecondarySignalName, SecondaryComparator, SecondaryWarningThreshold, SecondaryCriticalThreshold, MinSampleCount, SeasonalBucketsJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		parts[0],
		anyToString(name),
		defaultString(anyToString(ruleType), "threshold"),
		anyToString(signalSource),
		anyToString(signalName),
		anyToString(serviceName),
		anyToString(attrFingerprint),
		anyToString(comparator),
		anyToFloat(warningThreshold),
		anyToFloat(criticalThreshold),
		anyToString(secondarySource),
		anyToString(secondarySignal),
		defaultString(anyToString(secondaryComparator), "gt"),
		anyToFloat(secondaryWarningThreshold),
		anyToFloat(secondaryCriticalThreshold),
		anyToInt(minSampleCount),
		"",
		1,
		persist.Version(),
	)
	http.Redirect(w, r, "/metrics/rules", http.StatusSeeOther)
}

func (s *Server) renderMetricsRulesPage(
	w http.ResponseWriter,
	rules []map[string]any,
	services []string,
	signals []string,
	sources []string,
	autoSummary map[string]any,
	autoPreview []map[string]any,
	autoDashboardSummary map[string]any,
	autoDashboardPreview []map[string]any,
	openPanel string,
) {
	ctx := map[string]any{
		"title":                  "Metrics Rules",
		"mobile_breakpoint_max":  "575.98px",
		"request":                map[string]any{"endpoint": "metrics/rules"},
		"rules":                  rules,
		"services":               services,
		"signals":                signals,
		"sources":                sources,
		"auto_summary":           autoSummary,
		"auto_preview":           autoPreview,
		"auto_dashboard_summary": autoDashboardSummary,
		"auto_dashboard_preview": autoDashboardPreview,
		"auto_open_panel":        openPanel,
		"source_label": func(source any) string {
			return strings.TrimSpace(toString(source))
		},
		"signal_label": func(_source any, signal any) string {
			return strings.TrimSpace(toString(signal))
		},
		"signal_description": func(_source any, _signal any) string {
			return ""
		},
	}
	s.renderTemplate(w, "metrics_rules.html", ctx)
}

type autoRuleStats struct {
	examined int
	existing int
	invalid  int
}

type dashboardChartRow struct {
	title    string
	position int
}

func ensureMetricsRulesSchema(ctx context.Context, store extensionpoints.ClickHouseStore) error {
	ddls := []string{
		"CREATE TABLE IF NOT EXISTS sobs_anomaly_rules (Id String, Name String, RuleType String DEFAULT 'threshold', SignalSource String, SignalName String, ServiceName String, AttrFingerprint String, Comparator String, WarningThreshold Float64, CriticalThreshold Float64, SecondarySignalSource String DEFAULT '', SecondarySignalName String DEFAULT '', SecondaryComparator String DEFAULT 'gt', SecondaryWarningThreshold Float64 DEFAULT 0, SecondaryCriticalThreshold Float64 DEFAULT 0, MinSampleCount UInt32 DEFAULT 1, SeasonalBucketsJson String DEFAULT '', IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (SignalSource, SignalName, ServiceName, AttrFingerprint, Id)",
		"CREATE TABLE IF NOT EXISTS sobs_dashboards (Id String, Name String, Description String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id",
		"CREATE TABLE IF NOT EXISTS sobs_chart_configs (Id String, DashboardId String, Title String, ChartType String, Query String, OptionsJson String, Position UInt16 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (DashboardId, Id)",
	}
	for _, ddl := range ddls {
		if _, err := store.Exec(ctx, ddl); err != nil {
			return err
		}
	}
	return nil
}

func buildAutoMetricRuleCandidates(ctx context.Context, store extensionpoints.ClickHouseStore, hours, minPoints int, serviceFilter string, includeAttrFP bool) ([]map[string]any, autoRuleStats, error) {
	whereParts := []string{"time >= now() - INTERVAL ? HOUR"}
	params := []any{hours}
	if serviceFilter != "" {
		whereParts = append(whereParts, "ServiceName = ?")
		params = append(params, serviceFilter)
	}
	whereSQL := " WHERE " + strings.Join(whereParts, " AND ")
	attrSelect := "''"
	attrGroup := ""
	if includeAttrFP {
		attrSelect = "AttrFingerprint"
		attrGroup = ", AttrFingerprint"
	}
	query := "SELECT ServiceName, SignalSource, SignalName, " + attrSelect + " AS AttrFingerprint, count() AS point_count, quantile(0.05)(toFloat64(value)) AS q05, quantile(0.20)(toFloat64(value)) AS q20, quantile(0.50)(toFloat64(value)) AS q50, quantile(0.80)(toFloat64(value)) AS q80, quantile(0.95)(toFloat64(value)) AS q95 FROM v_derived_signals_anomaly" + whereSQL + " GROUP BY ServiceName, SignalSource, SignalName" + attrGroup + " HAVING point_count >= ? ORDER BY point_count DESC"
	rows, err := store.Query(ctx, query, append(params, minPoints)...)
	if err != nil {
		return nil, autoRuleStats{}, err
	}
	defer rows.Close()
	existingSeries := make(map[string]struct{})
	for _, rule := range summaryLoadAnomalyRules(ctx, store) {
		existingSeries[rule.Source+"\x00"+rule.Signal+"\x00"+rule.Service+"\x00"+rule.AttrFP+"\x00"+defaultString(rule.RuleType, "threshold")] = struct{}{}
	}
	candidates := []map[string]any{}
	stats := autoRuleStats{}
	for rows.Next() {
		var service, source, signal, attrFP, pointCount, q05, q20, q50, q80, q95 any
		if scanErr := rows.Scan(&service, &source, &signal, &attrFP, &pointCount, &q05, &q20, &q50, &q80, &q95); scanErr != nil {
			continue
		}
		stats.examined++
		seriesKey := anyToString(source) + "\x00" + anyToString(signal) + "\x00" + anyToString(service) + "\x00" + anyToString(attrFP) + "\x00threshold"
		if _, ok := existingSeries[seriesKey]; ok {
			stats.existing++
			continue
		}
		comparator := inferAutoRuleComparator(anyToString(signal))
		warningThreshold, criticalThreshold := autoRuleThresholds(comparator, anyToFloat(q05), anyToFloat(q20), anyToFloat(q50), anyToFloat(q80), anyToFloat(q95))
		if (comparator == "gt" && criticalThreshold < warningThreshold) || (comparator == "lt" && criticalThreshold > warningThreshold) {
			stats.invalid++
			continue
		}
		candidates = append(candidates, map[string]any{
			"name":               formatAutoRuleName(anyToString(source), anyToString(signal), anyToString(service), anyToString(attrFP)),
			"rule_type":          "threshold",
			"source":             anyToString(source),
			"signal":             anyToString(signal),
			"service":            anyToString(service),
			"attr_fp":            anyToString(attrFP),
			"comparator":         comparator,
			"warning_threshold":  warningThreshold,
			"critical_threshold": criticalThreshold,
			"min_sample_count":   3,
			"point_count":        anyToInt(pointCount),
		})
	}
	return candidates, stats, rows.Err()
}

func buildSeasonalMetricRuleCandidates(ctx context.Context, store extensionpoints.ClickHouseStore, hours, minPoints int, serviceFilter string, includeAttrFP bool, strategy string) ([]map[string]any, autoRuleStats, error) {
	thresholdCandidates, stats, err := buildAutoMetricRuleCandidates(ctx, store, hours, minPoints, serviceFilter, includeAttrFP)
	if err != nil {
		return nil, stats, err
	}
	whereParts := []string{"time >= now() - INTERVAL ? HOUR"}
	params := []any{hours}
	if serviceFilter != "" {
		whereParts = append(whereParts, "ServiceName = ?")
		params = append(params, serviceFilter)
	}
	whereSQL := " WHERE " + strings.Join(whereParts, " AND ")
	attrSelect := "''"
	attrGroup := ""
	if includeAttrFP {
		attrSelect = "AttrFingerprint"
		attrGroup = ", AttrFingerprint"
	}
	bucketExpr := "toHour(time)"
	if strategy == "day_of_week" {
		bucketExpr = "toDayOfWeek(time)"
	}
	bucketQuery := "SELECT ServiceName, SignalSource, SignalName, " + attrSelect + " AS AttrFingerprint, " + bucketExpr + " AS bucket_key, count() AS point_count, quantile(0.05)(toFloat64(value)) AS q05, quantile(0.20)(toFloat64(value)) AS q20, quantile(0.50)(toFloat64(value)) AS q50, quantile(0.80)(toFloat64(value)) AS q80, quantile(0.95)(toFloat64(value)) AS q95 FROM v_derived_signals_anomaly" + whereSQL + " GROUP BY ServiceName, SignalSource, SignalName" + attrGroup + ", bucket_key HAVING point_count >= ? ORDER BY ServiceName, SignalSource, SignalName" + attrGroup + ", bucket_key"
	rows, err := store.Query(ctx, bucketQuery, append(params, seasonalMinBucketPoints)...)
	if err != nil {
		return nil, stats, err
	}
	defer rows.Close()
	bucketIndex := map[string]map[string]map[string]float64{}
	for rows.Next() {
		var service, source, signal, attrFP, bucketKey, pointCount, q05, q20, q50, q80, q95 any
		if scanErr := rows.Scan(&service, &source, &signal, &attrFP, &bucketKey, &pointCount, &q05, &q20, &q50, &q80, &q95); scanErr != nil {
			continue
		}
		seriesKey := anyToString(source) + "\x00" + anyToString(signal) + "\x00" + anyToString(service) + "\x00" + anyToString(attrFP)
		comparator := inferAutoRuleComparator(anyToString(signal))
		warningThreshold, criticalThreshold := autoRuleThresholds(comparator, anyToFloat(q05), anyToFloat(q20), anyToFloat(q50), anyToFloat(q80), anyToFloat(q95))
		if _, ok := bucketIndex[seriesKey]; !ok {
			bucketIndex[seriesKey] = map[string]map[string]float64{}
		}
		bucketIndex[seriesKey][strconv.Itoa(anyToInt(bucketKey))] = map[string]float64{"warning": warningThreshold, "critical": criticalThreshold}
	}
	if err := rows.Err(); err != nil {
		return nil, stats, err
	}
	seasonalCandidates := make([]map[string]any, 0, len(thresholdCandidates))
	for _, candidate := range thresholdCandidates {
		seriesKey := anyToString(candidate["source"]) + "\x00" + anyToString(candidate["signal"]) + "\x00" + anyToString(candidate["service"]) + "\x00" + anyToString(candidate["attr_fp"])
		buckets := bucketIndex[seriesKey]
		seasonalCandidates = append(seasonalCandidates, map[string]any{
			"name":                  candidate["name"],
			"rule_type":             "seasonal",
			"source":                candidate["source"],
			"signal":                candidate["signal"],
			"service":               candidate["service"],
			"attr_fp":               candidate["attr_fp"],
			"comparator":            candidate["comparator"],
			"warning_threshold":     candidate["warning_threshold"],
			"critical_threshold":    candidate["critical_threshold"],
			"min_sample_count":      candidate["min_sample_count"],
			"point_count":           candidate["point_count"],
			"seasonal_buckets_json": persist.JSONString(map[string]any{"strategy": strategy, "buckets": buckets}),
			"seasonal_bucket_count": len(buckets),
			"seasonal_strategy":     strategy,
		})
	}
	return seasonalCandidates, stats, nil
}

func buildAutoDashboardChartCandidates(rules []summaryAnomalyRule, serviceFilter string, hours int) []map[string]any {
	candidates := []map[string]any{}
	titleCounts := map[string]int{}
	for _, rule := range rules {
		source := strings.TrimSpace(rule.Source)
		signal := strings.TrimSpace(rule.Signal)
		if source == "" || signal == "" {
			continue
		}
		ruleService := strings.TrimSpace(rule.Service)
		if serviceFilter != "" && ruleService != "" && ruleService != serviceFilter {
			continue
		}
		whereParts := []string{"SignalSource = " + sqlLiteral(source), "SignalName = " + sqlLiteral(signal), "time >= now() - INTERVAL " + strconv.Itoa(hours) + " HOUR"}
		if ruleService != "" {
			whereParts = append(whereParts, "ServiceName = "+sqlLiteral(ruleService))
		}
		if strings.TrimSpace(rule.AttrFP) != "" {
			whereParts = append(whereParts, "AttrFingerprint = "+sqlLiteral(rule.AttrFP))
		}
		query := "SELECT time, ServiceName AS service, SignalSource AS source, SignalName AS signal, AttrFingerprint AS attr_fp, value, SampleCount AS sample_count, baseline_mean, baseline_lower, baseline_upper, anomaly_state, anomaly_score FROM v_derived_signals_anomaly WHERE " + strings.Join(whereParts, " AND ") + " ORDER BY time"
		baseTitle := strings.TrimSpace(rule.Name)
		if baseTitle == "" {
			baseTitle = source + "/" + signal
		}
		titleIndex := titleCounts[baseTitle]
		titleCounts[baseTitle] = titleIndex + 1
		title := baseTitle
		if titleIndex > 0 {
			title = baseTitle + " (" + strconv.Itoa(titleIndex+1) + ")"
		}
		candidates = append(candidates, map[string]any{
			"title":      title,
			"rule_name":  rule.Name,
			"rule_type":  rule.RuleType,
			"source":     source,
			"signal":     signal,
			"service":    ruleService,
			"attr_fp":    rule.AttrFP,
			"chart_type": "derived_signal_overlay",
			"query":      query,
		})
	}
	sort.Slice(candidates, func(i, j int) bool {
		leftService := anyToString(candidates[i]["service"])
		rightService := anyToString(candidates[j]["service"])
		if leftService != rightService {
			return leftService < rightService
		}
		leftSource := anyToString(candidates[i]["source"])
		rightSource := anyToString(candidates[j]["source"])
		if leftSource != rightSource {
			return leftSource < rightSource
		}
		leftSignal := anyToString(candidates[i]["signal"])
		rightSignal := anyToString(candidates[j]["signal"])
		if leftSignal != rightSignal {
			return leftSignal < rightSignal
		}
		return anyToString(candidates[i]["title"]) < anyToString(candidates[j]["title"])
	})
	return candidates
}

func seedDashboardIfMissing(ctx context.Context, store extensionpoints.ClickHouseStore, dashboardName, description string) (string, error) {
	rows, err := store.Query(ctx, "SELECT Id FROM sobs_dashboards FINAL WHERE IsDeleted = 0 AND Name = ? LIMIT 1", dashboardName)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	if rows.Next() {
		var dashboardID any
		if scanErr := rows.Scan(&dashboardID); scanErr == nil {
			return anyToString(dashboardID), nil
		}
	}
	dashboardID := persist.NewID()
	_, err = store.Exec(ctx, "INSERT INTO sobs_dashboards (Id, Name, Description, IsDeleted, Version) VALUES (?, ?, ?, ?, ?)", dashboardID, dashboardName, description, 0, persist.Version())
	return dashboardID, err
}

func loadDashboardCharts(ctx context.Context, store extensionpoints.ClickHouseStore, dashboardID string) ([]dashboardChartRow, error) {
	rows, err := store.Query(ctx, "SELECT Title, Position FROM sobs_chart_configs FINAL WHERE IsDeleted = 0 AND DashboardId = ? ORDER BY Position, Id", dashboardID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []dashboardChartRow{}
	for rows.Next() {
		var title, position any
		if scanErr := rows.Scan(&title, &position); scanErr != nil {
			continue
		}
		out = append(out, dashboardChartRow{title: anyToString(title), position: anyToInt(position)})
	}
	return out, rows.Err()
}

func inferAutoRuleComparator(signalName string) string {
	name := strings.ToLower(strings.TrimSpace(signalName))
	for _, token := range autoRuleLTHints {
		if strings.Contains(name, token) {
			return "lt"
		}
	}
	for _, token := range autoRuleGTHints {
		if strings.Contains(name, token) {
			return "gt"
		}
	}
	return "gt"
}

func autoRuleThresholds(comparator string, q05, q20, q50, q80, q95 float64) (float64, float64) {
	if comparator == "lt" {
		warning := q20
		critical := q05
		if critical > warning {
			critical = minFloat(warning, q50)
		}
		if critical == warning {
			if warning != 0 {
				critical = warning * 0.9
			} else {
				critical = -0.1
			}
		}
		return warning, critical
	}
	warning := q80
	critical := q95
	if critical < warning {
		critical = maxFloat(critical, q50)
		if critical < warning {
			critical = warning
		}
	}
	if critical == warning {
		if warning != 0 {
			critical = warning * 1.1
		} else {
			critical = 0.1
		}
	}
	return warning, critical
}

func formatAutoRuleName(source, signal, service, attrFP string) string {
	suffix := defaultString(service, "any")
	if strings.TrimSpace(attrFP) != "" {
		suffix += " / " + strings.TrimSpace(attrFP)
	}
	return "Auto " + source + "/" + signal + " [" + suffix + "]"
}

func defaultAutoDashboardName(serviceFilter string) string {
	if strings.TrimSpace(serviceFilter) != "" {
		return "Auto Metric Rules - " + strings.TrimSpace(serviceFilter)
	}
	return "Auto Metric Rules Dashboard"
}

func buildRawChartSpec(templateID, query, optionsJSON string) map[string]any {
	if strings.TrimSpace(optionsJSON) != "" {
		parsed := persist.ParseJSONMap(optionsJSON)
		if chartSpec, ok := parsed["chart_spec"].(map[string]any); ok && len(chartSpec) > 0 {
			return chartSpec
		}
	}
	return map[string]any{
		"template_id": templateID,
		"sql": map[string]any{
			"mode":         "raw",
			"override_sql": query,
		},
		"data": map[string]any{
			"source_view":   "v_derived_signals_anomaly",
			"service":       "",
			"signal_source": "traces",
			"signal_name":   "trace_volume",
			"metric_name":   "",
			"attr_fp":       "",
			"window_hours":  6,
			"limit":         1000,
		},
		"visual": map[string]any{
			"zoom_inside":    true,
			"zoom_slider":    false,
			"zoom_start_pct": 0,
			"zoom_end_pct":   100,
			"legend_show":    true,
			"smooth_line":    true,
			"value_color":    "",
			"role_map":       map[string]any{},
		},
	}
}

func defaultString(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func coercePositiveInt(raw string, defaultValue, minValue, maxValue int) int {
	parsed, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return defaultValue
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func coerceFloatDefault(raw string, defaultValue float64) float64 {
	parsed, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil {
		return defaultValue
	}
	return parsed
}

func sqlLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

func (s *Server) metricsAnomalyPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/anomaly" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}
	ctx := map[string]any{
		"title":                 "Metrics Anomaly Details",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "metrics/anomaly"},
		"source":                "",
		"service":               "",
		"signal":                "",
		"metric":                "",
		"attr_fp":               "",
		"from_ts":               "",
		"to_ts":                 "",
		"hours":                 24,
		"error_msg":             "",
		"rows":                  []map[string]any{},
		"total":                 0,
		"sources":               []any{},
		"services":              []any{},
		"signals":               []any{},
		"related_target":        "",
		"point_state":           "",
		"point_score":           "",
		"source_label": func(source any) string {
			return strings.TrimSpace(toString(source))
		},
		"signal_label": func(_source any, signal any) string {
			return strings.TrimSpace(toString(signal))
		},
		"signal_description": func(_source any, _signal any) string {
			return ""
		},
	}
	s.renderTemplate(w, "metrics_anomaly.html", ctx)
}

func toString(value any) string {
	if value == nil {
		return ""
	}
	if text, ok := value.(string); ok {
		return text
	}
	return ""
}

func (s *Server) apiMetricsAnomaly(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSON(w, http.StatusOK, s.metricsService.AnomalySnapshot())
}
