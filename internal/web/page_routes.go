package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/flosch/pongo2/v6"
)

const summaryAISpanCondition = "(SpanAttributes['gen_ai.provider.name'] != '' OR SpanAttributes['gen_ai.system'] != '' OR SpanAttributes['gen_ai.operation.name'] != '')"

var summarySeverityRanks = map[string]int{"normal": 0, "warning": 1, "outlier": 2}

type summaryPageData struct {
	Stats        map[string]any
	SignalHealth []map[string]any
	RecentErrors []map[string]any
	RecentLogs   []map[string]any
	RUMSummary   []any
	AISummary    []any
	CVEOverview  map[string]any
}

type summaryAnomalyRule struct {
	ID                         string
	Name                       string
	RuleType                   string
	Source                     string
	Signal                     string
	Service                    string
	AttrFP                     string
	Comparator                 string
	WarningThreshold           float64
	CriticalThreshold          float64
	SecondarySource            string
	SecondarySignal            string
	SecondaryComparator        string
	SecondaryWarningThreshold  float64
	SecondaryCriticalThreshold float64
	MinSampleCount             int
	SeasonalBucketsJSON        string
}

type summarySignalSeries struct {
	Service     string
	Source      string
	Signal      string
	AttrFP      string
	Value       float64
	SampleCount int
}

type summaryRuleEvaluation struct {
	Name  string
	State string
}

func (s *Server) registerPageRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/logs", s.pageLogsHandler)
	mux.HandleFunc("/errors", s.pageErrorsHandler)
	mux.HandleFunc("/traces", s.pageTracesHandler)
	mux.HandleFunc("/summary/help", s.summaryHelpPage)
	mux.HandleFunc("/logs/help", s.logsHelpPage)
	mux.HandleFunc("/errors/help", s.errorsHelpPage)
	mux.HandleFunc("/traces/help", s.tracesHelpPage)
	mux.HandleFunc("/incident", s.incidentPage)
	mux.HandleFunc("/incident/help", s.incidentHelpPage)
	mux.HandleFunc("/rum", s.rumPage)
	mux.HandleFunc("/rum/help", s.rumHelpPage)
	mux.HandleFunc("/web-traffic", s.webTrafficPage)
	mux.HandleFunc("/web-traffic/help", s.webTrafficHelpPage)
	mux.HandleFunc("/work-items", s.workItemsPage)
	mux.HandleFunc("/work-items/help", s.workItemsHelpPage)
	mux.HandleFunc("/ai", s.aiPage)
	mux.HandleFunc("/ai/help", s.aiHelpPage)
	mux.HandleFunc("/reports", s.reportsPage)
	mux.HandleFunc("/reports/help", s.reportsHelpPage)
	mux.HandleFunc("/settings", s.settingsPage)
	mux.HandleFunc("/settings/help", s.settingsHelpPage)
	mux.HandleFunc("/settings/help/ai", s.settingsAIHelpPage)
	mux.HandleFunc("/settings/help/agents", s.settingsAgentsHelpPage)
	mux.HandleFunc("/settings/help/data-management", s.settingsDataManagementHelpPage)
	mux.HandleFunc("/settings/help/enrichment", s.settingsEnrichmentHelpPage)
	mux.HandleFunc("/settings/help/kubernetes", s.settingsKubernetesHelpPage)
	mux.HandleFunc("/settings/help/masking", s.settingsMaskingHelpPage)
	mux.HandleFunc("/settings/help/notifications", s.settingsNotificationsHelpPage)
	mux.HandleFunc("/settings/help/repositories", s.settingsRepositoriesHelpPage)
	mux.HandleFunc("/settings/help/tags", s.settingsTagsHelpPage)
	mux.HandleFunc("/settings/notifications", s.settingsNotificationsPage)
	mux.HandleFunc("/query", s.queryPage)
	mux.HandleFunc("/query/help", s.queryHelpPage)
	mux.HandleFunc("/metrics/help", s.metricsHelpPage)
	mux.HandleFunc("/metrics/help/rules", s.metricsRulesHelpPage)
	mux.HandleFunc("/metrics/help/rules/auto", s.metricsRulesAutoHelpPage)
	mux.HandleFunc("/metrics/help/anomaly", s.metricsAnomalyHelpPage)
	mux.HandleFunc("/setup/help/playbooks", s.setupPlaybooksHelpPage)
	mux.HandleFunc("/dashboards/help/chart-editor", s.chartEditorHelpPage)
	mux.HandleFunc("/kubernetes/help", s.kubernetesHelpPage)
	mux.HandleFunc("/cve/help", s.cveHelpPage)
}

func (s *Server) summaryPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.renderErr != nil || s.renderer == nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	data := s.summaryData(r)
	ctx := pongo2.Context{
		"title":                 "Summary",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "summary"},
		"stats":                 data.Stats,
		"signal_health":         data.SignalHealth,
		"recent_errors":         data.RecentErrors,
		"recent_logs":           data.RecentLogs,
		"rum_summary":           data.RUMSummary,
		"ai_summary":            data.AISummary,
		"cve_overview":          data.CVEOverview,
	}
	s.renderTemplate(w, "summary.html", ctx)
}

func (s *Server) summaryData(r *http.Request) summaryPageData {
	data := summaryPageData{
		Stats: map[string]any{
			"logs":         0,
			"errors":       0,
			"errors_total": 0,
			"spans":        0,
			"rum":          0,
			"ai":           0,
			"services":     []any{},
		},
		SignalHealth: []map[string]any{},
		RecentErrors: []map[string]any{},
		RecentLogs:   []map[string]any{},
		RUMSummary:   []any{},
		AISummary:    []any{},
		CVEOverview: map[string]any{
			"enabled":   true,
			"last_scan": "",
			"total":     0,
			"critical":  0,
			"high":      0,
			"medium":    0,
			"low":       0,
		},
	}

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		return data
	}
	defer func() { _ = store.Close() }()

	data.Stats["logs"] = summaryActivePartRows(r.Context(), store, "otel_logs")
	data.Stats["spans"] = summaryActivePartRows(r.Context(), store, "otel_traces")
	data.Stats["rum"] = summaryActivePartRows(r.Context(), store, "hyperdx_sessions")
	data.Stats["ai"] = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM otel_traces WHERE "+summaryAISpanCondition)

	errorSourcesSQL := summaryErrorSourcesSQL()
	errorIDExpr := summaryErrorIDSQLExpr()
	unresolvedCondition := summaryUnresolvedErrorCondition()
	data.Stats["errors_total"] = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM ("+errorSourcesSQL+")")
	data.Stats["errors"] = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM ("+errorSourcesSQL+") WHERE "+unresolvedCondition)

	if rows, err := store.Query(r.Context(), "SELECT ServiceName FROM (SELECT DISTINCT ServiceName FROM otel_logs WHERE ServiceName != '' UNION DISTINCT SELECT DISTINCT ServiceName FROM otel_traces WHERE ServiceName != '' UNION DISTINCT SELECT DISTINCT ServiceName FROM hyperdx_sessions WHERE ServiceName != '') ORDER BY ServiceName"); err == nil {
		defer func() { _ = rows.Close() }()
		services := make([]any, 0)
		for rows.Next() {
			var svc any
			if scanErr := rows.Scan(&svc); scanErr == nil {
				if value := anyToString(svc); value != "" {
					services = append(services, value)
				}
			}
		}
		data.Stats["services"] = services
	}

	recentErrorsSQL := "SELECT " + errorIDExpr + " AS ErrorId, Timestamp, ServiceName, if(LogAttributes['exception.type'] != '', LogAttributes['exception.type'], 'Error') AS ErrType, if(LogAttributes['exception.message'] != '', LogAttributes['exception.message'], Body) AS Message FROM (" + errorSourcesSQL + ") WHERE Timestamp >= now() - INTERVAL 48 HOUR AND " + unresolvedCondition + " ORDER BY Timestamp DESC LIMIT 5"
	if rows, err := store.Query(r.Context(), recentErrorsSQL); err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var errorID, ts, service, errType, message any
			if scanErr := rows.Scan(&errorID, &ts, &service, &errType, &message); scanErr != nil {
				continue
			}
			data.RecentErrors = append(data.RecentErrors, map[string]any{
				"id":       anyToString(errorID),
				"ts":       anyToString(ts),
				"service":  anyToString(service),
				"err_type": anyToString(errType),
				"message":  anyToString(message),
			})
		}
	}

	if rows, err := store.Query(r.Context(), "SELECT Timestamp, SeverityText, ServiceName, Body FROM otel_logs ORDER BY Timestamp DESC LIMIT 10"); err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var ts, level, service, body any
			if scanErr := rows.Scan(&ts, &level, &service, &body); scanErr != nil {
				continue
			}
			data.RecentLogs = append(data.RecentLogs, map[string]any{
				"ts":      anyToString(ts),
				"level":   anyToString(level),
				"service": anyToString(service),
				"body":    anyToString(body),
			})
		}
	}

	if rows, err := store.Query(r.Context(), "SELECT EventName, COUNT(*) AS cnt FROM hyperdx_sessions GROUP BY EventName ORDER BY cnt DESC"); err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var eventName, count any
			if scanErr := rows.Scan(&eventName, &count); scanErr != nil {
				continue
			}
			data.RUMSummary = append(data.RUMSummary, []any{anyToString(eventName), anyToInt(count)})
		}
	}

	if rows, err := store.Query(r.Context(), "SELECT SpanAttributes['gen_ai.request.model'] AS model, COUNT(*) AS cnt, SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.input_tokens'])) AS ti, SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.output_tokens'])) AS to_ FROM otel_traces WHERE "+summaryAISpanCondition+" GROUP BY model"); err == nil {
		defer func() { _ = rows.Close() }()
		for rows.Next() {
			var model, count, tokensIn, tokensOut any
			if scanErr := rows.Scan(&model, &count, &tokensIn, &tokensOut); scanErr != nil {
				continue
			}
			data.AISummary = append(data.AISummary, []any{anyToString(model), anyToInt(count), anyToInt(tokensIn), anyToInt(tokensOut)})
		}
	}

	data.SignalHealth = summarySignalHealth(r.Context(), store)
	data.CVEOverview = summaryCVEOverview(r.Context(), store)

	return data
}

func summaryActivePartRows(ctx context.Context, store extensionpoints.ClickHouseStore, tableName string) int {
	name := sanitizeIdentifier(tableName)
	if name == "" {
		return 0
	}
	return summaryQuerySingleInt(ctx, store, fmt.Sprintf("SELECT count() FROM %s", name))
}

func summaryQuerySingleInt(ctx context.Context, store extensionpoints.ClickHouseStore, query string, args ...any) int {
	rows, err := store.Query(ctx, query, args...)
	if err != nil {
		return 0
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 0
	}
	var value any
	if err := rows.Scan(&value); err != nil {
		return 0
	}
	return anyToInt(value)
}

func queryRows(ctx context.Context, store extensionpoints.ClickHouseStore, query string, args ...any) ([]map[string]any, error) {
	rows, err := store.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	columnsProvider, ok := rows.(interface{ Columns() ([]string, error) })
	if !ok {
		return nil, fmt.Errorf("row iterator does not expose columns")
	}
	columns, err := columnsProvider.Columns()
	if err != nil {
		return nil, err
	}

	result := make([]map[string]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		scanDest := make([]any, len(columns))
		for i := range values {
			scanDest[i] = &values[i]
		}
		if scanErr := rows.Scan(scanDest...); scanErr != nil {
			continue
		}
		item := make(map[string]any, len(columns))
		for i, column := range columns {
			item[column] = values[i]
		}
		result = append(result, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func summaryAppSetting(ctx context.Context, store extensionpoints.ClickHouseStore, key string) string {
	rows, err := store.Query(ctx, "SELECT Value FROM sobs_app_settings WHERE Key = ? ORDER BY UpdatedAt DESC LIMIT 1", key)
	if err != nil {
		return ""
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return ""
	}
	var value any
	if err := rows.Scan(&value); err != nil {
		return ""
	}
	return strings.TrimSpace(anyToString(value))
}

func summaryCVEOverview(ctx context.Context, store extensionpoints.ClickHouseStore) map[string]any {
	enabledRaw := summaryAppSetting(ctx, store, "enrichment.cve_enabled")
	enabled := true
	if enabledRaw != "" {
		enabled = parseBool(enabledRaw)
	}
	overview := map[string]any{
		"enabled":   enabled,
		"last_scan": summaryAppSetting(ctx, store, "enrichment.cve_last_scan"),
		"total":     0,
		"critical":  0,
		"high":      0,
		"medium":    0,
		"low":       0,
	}
	if !enabled {
		return overview
	}
	rows, err := store.Query(ctx, "SELECT Severity, COUNT(*) AS cnt FROM sobs_cve_findings FINAL GROUP BY Severity")
	if err != nil {
		rows, err = store.Query(ctx, "SELECT Severity, COUNT(*) AS cnt FROM sobs_cve_findings GROUP BY Severity")
	}
	if err != nil {
		return overview
	}
	defer func() { _ = rows.Close() }()
	total := 0
	for rows.Next() {
		var severity, count any
		if scanErr := rows.Scan(&severity, &count); scanErr != nil {
			continue
		}
		cnt := anyToInt(count)
		total += cnt
		switch strings.ToUpper(strings.TrimSpace(anyToString(severity))) {
		case "CRITICAL":
			overview["critical"] = overview["critical"].(int) + cnt
		case "HIGH":
			overview["high"] = overview["high"].(int) + cnt
		case "MEDIUM":
			overview["medium"] = overview["medium"].(int) + cnt
		case "LOW":
			overview["low"] = overview["low"].(int) + cnt
		}
	}
	overview["total"] = total
	return overview
}

func summarySignalHealth(ctx context.Context, store extensionpoints.ClickHouseStore) []map[string]any {
	rows, err := store.Query(ctx, "SELECT ServiceName, SignalSource, SignalName, AttrFingerprint, argMax(value, time) AS value, argMax(SampleCount, time) AS SampleCount FROM v_derived_signals_anomaly WHERE time >= now() - INTERVAL 24 HOUR GROUP BY ServiceName, SignalSource, SignalName, AttrFingerprint")
	if err != nil {
		return []map[string]any{}
	}
	defer func() { _ = rows.Close() }()

	seriesRows := make([]summarySignalSeries, 0)
	for rows.Next() {
		var service, source, signal, attrFP, value, sampleCount any
		if scanErr := rows.Scan(&service, &source, &signal, &attrFP, &value, &sampleCount); scanErr != nil {
			continue
		}
		seriesRows = append(seriesRows, summarySignalSeries{
			Service:     anyToString(service),
			Source:      anyToString(source),
			Signal:      anyToString(signal),
			AttrFP:      anyToString(attrFP),
			Value:       anyToFloat(value),
			SampleCount: anyToInt(sampleCount),
		})
	}
	if len(seriesRows) == 0 {
		return []map[string]any{}
	}

	rules := summaryLoadAnomalyRules(ctx, store)
	latestLookup := make(map[string]summarySignalSeries, len(seriesRows))
	for _, row := range seriesRows {
		latestLookup[summarySignalSeriesKey(row.Service, row.AttrFP, row.Source, row.Signal)] = row
	}

	serviceWorst := map[string]int{}
	serviceCount := map[string]int{}
	for _, row := range seriesRows {
		bestEvaluation := (*summaryRuleEvaluation)(nil)
		bestSeverity := -1
		bestTypeRank := -1
		bestName := ""
		for _, rule := range rules {
			if !summaryRuleMatchesSeries(rule, row.Source, row.Signal, row.Service, row.AttrFP) {
				continue
			}
			var evaluation *summaryRuleEvaluation
			switch rule.RuleType {
			case "composite":
				evaluation = summaryEvaluateCompositeRule(ctx, store, rule, row, latestLookup)
			case "seasonal":
				evaluation = summaryEvaluateSeasonalRule(rule, row.Value, row.SampleCount)
			default:
				evaluation = summaryEvaluateThresholdRule(rule, row.Value, row.SampleCount)
			}
			if evaluation == nil {
				continue
			}
			severity := summarySeverityRanks[evaluation.State]
			typeRank := summaryRuleTypeRank(rule.RuleType)
			if severity > bestSeverity || (severity == bestSeverity && (typeRank > bestTypeRank || (typeRank == bestTypeRank && evaluation.Name > bestName))) {
				bestEvaluation = evaluation
				bestSeverity = severity
				bestTypeRank = typeRank
				bestName = evaluation.Name
			}
		}

		effectiveState := "normal"
		if bestEvaluation != nil {
			effectiveState = summaryCombineStates(effectiveState, bestEvaluation.State)
		}
		serviceWorst[row.Service] = maxInt(serviceWorst[row.Service], summarySeverityRanks[effectiveState])
		serviceCount[row.Service]++
	}

	rankToState := map[int]string{0: "normal", 1: "warning", 2: "outlier"}
	result := make([]map[string]any, 0, len(serviceWorst))
	for service, rank := range serviceWorst {
		result = append(result, map[string]any{
			"service":      service,
			"worst_state":  rankToState[rank],
			"signal_count": serviceCount[service],
		})
	}
	sort.Slice(result, func(i, j int) bool {
		leftRank := summarySeverityRanks[anyToString(result[i]["worst_state"])]
		rightRank := summarySeverityRanks[anyToString(result[j]["worst_state"])]
		if leftRank == rightRank {
			return anyToString(result[i]["service"]) < anyToString(result[j]["service"])
		}
		return leftRank > rightRank
	})
	return result
}

func summaryLoadAnomalyRules(ctx context.Context, store extensionpoints.ClickHouseStore) []summaryAnomalyRule {
	rows, err := store.Query(ctx, "SELECT Id, Name, RuleType, SignalSource, SignalName, ServiceName, AttrFingerprint, Comparator, WarningThreshold, CriticalThreshold, SecondarySignalSource, SecondarySignalName, SecondaryComparator, SecondaryWarningThreshold, SecondaryCriticalThreshold, MinSampleCount, SeasonalBucketsJson FROM sobs_anomaly_rules FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	rules := make([]summaryAnomalyRule, 0)
	for rows.Next() {
		var id, name, ruleType, source, signal, service, attrFP, comparator any
		var warning, critical, secondarySource, secondarySignal, secondaryComparator, secondaryWarning, secondaryCritical, minSampleCount, seasonalBucketsJSON any
		if scanErr := rows.Scan(&id, &name, &ruleType, &source, &signal, &service, &attrFP, &comparator, &warning, &critical, &secondarySource, &secondarySignal, &secondaryComparator, &secondaryWarning, &secondaryCritical, &minSampleCount, &seasonalBucketsJSON); scanErr != nil {
			continue
		}
		rt := anyToString(ruleType)
		if rt == "" {
			rt = "threshold"
		}
		cmp := anyToString(comparator)
		if cmp == "" {
			cmp = "gt"
		}
		secondaryCmp := anyToString(secondaryComparator)
		if secondaryCmp == "" {
			secondaryCmp = "gt"
		}
		rules = append(rules, summaryAnomalyRule{
			ID:                         anyToString(id),
			Name:                       anyToString(name),
			RuleType:                   rt,
			Source:                     anyToString(source),
			Signal:                     anyToString(signal),
			Service:                    anyToString(service),
			AttrFP:                     anyToString(attrFP),
			Comparator:                 cmp,
			WarningThreshold:           anyToFloat(warning),
			CriticalThreshold:          anyToFloat(critical),
			SecondarySource:            anyToString(secondarySource),
			SecondarySignal:            anyToString(secondarySignal),
			SecondaryComparator:        secondaryCmp,
			SecondaryWarningThreshold:  anyToFloat(secondaryWarning),
			SecondaryCriticalThreshold: anyToFloat(secondaryCritical),
			MinSampleCount:             anyToInt(minSampleCount),
			SeasonalBucketsJSON:        anyToString(seasonalBucketsJSON),
		})
	}
	return rules
}

func summaryRuleMatchesSeries(rule summaryAnomalyRule, source string, signal string, service string, attrFP string) bool {
	if rule.Source != source || rule.Signal != signal {
		return false
	}
	if rule.Service != "" && rule.Service != service {
		return false
	}
	if rule.AttrFP != "" && rule.AttrFP != attrFP {
		return false
	}
	return true
}

func summaryEvaluateThresholdRule(rule summaryAnomalyRule, value float64, sampleCount int) *summaryRuleEvaluation {
	state, ok := summaryEvaluateThresholdCondition(rule.Comparator, rule.WarningThreshold, rule.CriticalThreshold, value, sampleCount, rule.MinSampleCount)
	if !ok {
		return nil
	}
	return &summaryRuleEvaluation{Name: rule.Name, State: state}
}

func summaryEvaluateSeasonalRule(rule summaryAnomalyRule, value float64, sampleCount int) *summaryRuleEvaluation {
	state, ok := summaryEvaluateThresholdCondition(rule.Comparator, rule.WarningThreshold, rule.CriticalThreshold, value, sampleCount, rule.MinSampleCount)
	if !ok {
		return nil
	}
	return &summaryRuleEvaluation{Name: rule.Name, State: state}
}

func summaryEvaluateCompositeRule(ctx context.Context, store extensionpoints.ClickHouseStore, rule summaryAnomalyRule, row summarySignalSeries, latestLookup map[string]summarySignalSeries) *summaryRuleEvaluation {
	primaryState, ok := summaryEvaluateThresholdCondition(rule.Comparator, rule.WarningThreshold, rule.CriticalThreshold, row.Value, row.SampleCount, rule.MinSampleCount)
	if !ok {
		return nil
	}
	if rule.SecondarySource == "" || rule.SecondarySignal == "" {
		return nil
	}
	secondary, ok := latestLookup[summarySignalSeriesKey(row.Service, row.AttrFP, rule.SecondarySource, rule.SecondarySignal)]
	if !ok {
		secondary, ok = summaryLookupSecondarySeries(ctx, store, row.Service, row.AttrFP, rule.SecondarySource, rule.SecondarySignal)
		if !ok {
			return nil
		}
	}
	secondaryState, ok := summaryEvaluateThresholdCondition(rule.SecondaryComparator, rule.SecondaryWarningThreshold, rule.SecondaryCriticalThreshold, secondary.Value, secondary.SampleCount, rule.MinSampleCount)
	if !ok {
		return nil
	}
	return &summaryRuleEvaluation{Name: rule.Name, State: summaryCombineStates(primaryState, secondaryState)}
}

func summaryLookupSecondarySeries(ctx context.Context, store extensionpoints.ClickHouseStore, service string, attrFP string, source string, signal string) (summarySignalSeries, bool) {
	rows, err := store.Query(ctx, "SELECT value, SampleCount FROM v_derived_signals_anomaly WHERE ServiceName = ? AND SignalSource = ? AND SignalName = ? AND AttrFingerprint = ? ORDER BY time DESC LIMIT 1", service, source, signal, attrFP)
	if err != nil {
		return summarySignalSeries{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return summarySignalSeries{}, false
	}
	var value, sampleCount any
	if err := rows.Scan(&value, &sampleCount); err != nil {
		return summarySignalSeries{}, false
	}
	return summarySignalSeries{Service: service, Source: source, Signal: signal, AttrFP: attrFP, Value: anyToFloat(value), SampleCount: anyToInt(sampleCount)}, true
}

func summaryEvaluateThresholdCondition(comparator string, warning float64, critical float64, value float64, sampleCount int, minSampleCount int) (string, bool) {
	if sampleCount < maxInt(minSampleCount, 1) {
		return "", false
	}
	state := "normal"
	switch comparator {
	case "lt":
		if value <= critical {
			state = "outlier"
		} else if value <= warning {
			state = "warning"
		}
	default:
		if value >= critical {
			state = "outlier"
		} else if value >= warning {
			state = "warning"
		}
	}
	if state == "normal" {
		return "", false
	}
	return state, true
}

func summaryCombineStates(states ...string) string {
	bestState := "normal"
	bestRank := -1
	for _, state := range states {
		rank := summarySeverityRanks[strings.TrimSpace(state)]
		if rank > bestRank {
			bestRank = rank
			bestState = strings.TrimSpace(state)
		}
	}
	if bestState == "" {
		return "normal"
	}
	return bestState
}

func summaryRuleTypeRank(ruleType string) int {
	switch strings.TrimSpace(ruleType) {
	case "seasonal":
		return 3
	case "composite":
		return 2
	default:
		return 1
	}
}

func summarySignalSeriesKey(service string, attrFP string, source string, signal string) string {
	return service + "\x00" + attrFP + "\x00" + source + "\x00" + signal
}

func summaryErrorSourcesSQL() string {
	return "SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes FROM otel_logs WHERE EventName = 'exception' OR SeverityNumber >= 17 OR SeverityText IN ('ERROR', 'CRITICAL', 'FATAL') OR LogAttributes['exception.type'] != '' UNION ALL SELECT Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes FROM hyperdx_sessions WHERE EventName IN ('error', 'unhandledrejection', 'exception') OR SeverityNumber >= 17 OR SeverityText IN ('ERROR', 'CRITICAL', 'FATAL') OR LogAttributes['exception.type'] != ''"
}

func summaryErrorIDSQLExpr() string {
	return summaryErrorIDSQLExprForTimestamp("toString(Timestamp)")
}

func summaryUnresolvedErrorCondition() string {
	return "NOT (" + summaryErrorIDSQLExpr() + " IN (SELECT ErrorId FROM sobs_error_resolutions GROUP BY ErrorId) OR " + summaryErrorIDLocalTimeSQLExpr() + " IN (SELECT ErrorId FROM sobs_error_resolutions GROUP BY ErrorId))"
}

func summaryErrorIDLocalTimeSQLExpr() string {
	return summaryErrorIDSQLExprForTimestamp("formatDateTime(Timestamp, '%Y-%m-%d %H:%i:%s')")
}

func summaryErrorIDSQLExprForTimestamp(timestampExpr string) string {
	return "lower(hex(MD5(concat(" + timestampExpr + ", '|', ServiceName, '|', if(mapContains(LogAttributes, 'exception.type'), LogAttributes['exception.type'], 'Error'), '|', if(mapContains(LogAttributes, 'exception.message'), LogAttributes['exception.message'], Body), '|', TraceId, '|', SpanId))))"
}

func (s *Server) summaryHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/summary/help" {
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
	s.renderTemplate(w, "summary_help.html", pongo2.Context{"title": "Summary Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "summary/help"}})
}
func (s *Server) logsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/logs/help" {
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
	s.renderTemplate(w, "logs_help.html", pongo2.Context{"title": "Logs Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "logs/help"}})
}
func (s *Server) errorsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/errors/help" {
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
	s.renderTemplate(w, "errors_help.html", pongo2.Context{"title": "Errors Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "errors/help"}})
}
func (s *Server) tracesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/traces/help" {
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
	s.renderTemplate(w, "traces_help.html", pongo2.Context{"title": "Traces Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "traces/help"}})
}
func (s *Server) incidentPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/incident" {
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

	q := r.URL.Query()
	traceID := strings.TrimSpace(q.Get("trace_id"))
	errorID := strings.TrimSpace(q.Get("error_id"))
	rumSession := strings.TrimSpace(q.Get("rum_session"))
	rumTS := strings.TrimSpace(q.Get("rum_ts"))
	fromTS, toTS, timeError := parseIncidentTimeWindowArgs(r)

	windowMinutes := 30
	if parsed, err := strconv.Atoi(strings.TrimSpace(q.Get("window_minutes"))); err == nil {
		if parsed < 1 {
			parsed = 1
		}
		if parsed > 180 {
			parsed = 180
		}
		windowMinutes = parsed
	}

	errorMsg := ""
	if traceID == "" && errorID == "" && rumSession == "" {
		errorMsg = "No incident reference provided. Specify trace_id, error_id, or rum_session."
	}

	ref := strings.TrimSpace(q.Get("_ref"))
	if ref == "" {
		switch {
		case traceID != "":
			ref = traceID
		case errorID != "":
			ref = errorID
		case rumSession != "":
			ref = rumSession
		}
	}

	metricsContext := map[string]any{
		"source_mode":      "none",
		"total_points":     0,
		"series":           []any{},
		"match_mode":       "none",
		"match_label":      "no match",
		"match_dimensions": []any{},
		"health_chips":     []any{},
	}

	if traceID == "" && errorID == "" && rumSession == "" {
		s.renderTemplate(w, "incident.html", pongo2.Context{
			"title":                    "Incident",
			"mobile_breakpoint_max":    "575.98px",
			"request":                  map[string]any{"endpoint": "incident"},
			"_ref":                     ref,
			"trace_id":                 "",
			"error_id":                 "",
			"rum_session":              "",
			"rum_ts":                   "",
			"from_ts":                  "",
			"to_ts":                    "",
			"service":                  "",
			"window_minutes":           windowMinutes,
			"error_msg":                "No incident reference provided. Specify trace_id, error_id, or rum_session.",
			"time_error":               "",
			"primary_error":            nil,
			"primary_trace":            nil,
			"primary_rum":              nil,
			"existing_work_item":       nil,
			"work_item_links":          map[string]any{},
			"related_errors":           []any{},
			"related_errors_truncated": false,
			"related_log_count":        0,
			"related_span_count":       0,
			"anomaly_state":            nil,
			"related_rum_count":        0,
			"related_rum_sessions":     0,
			"related_rum_error_count":  0,
			"related_rum_events":       []any{},
			"metrics_context":          metricsContext,
			"mc":                       metricsContext,
			"raw_windows":              []any{},
			"_wi_list":                 []any{},
		})
		return
	}

	service := ""
	var primaryError map[string]any
	var primaryTrace map[string]any
	var primaryRUM map[string]any
	relatedErrors := make([]map[string]any, 0)
	relatedErrorsTruncated := false
	relatedLogCount := 0
	relatedSpanCount := 0
	relatedRUMCount := 0
	relatedRUMSessions := 0
	relatedRUMErrorCount := 0
	relatedRUMEvents := make([]map[string]any, 0)
	rawWindows := make([]map[string]any, 0)
	var anomalyState any
	workItemLinks := map[string]any{}
	var existingWorkItem any

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer func() { _ = store.Close() }()

		errorSourcesSQL := summaryErrorSourcesSQL()
		errorIDExpr := summaryErrorIDSQLExpr()
		baseErrorSQL := "SELECT " + errorIDExpr + " AS ErrorId, Timestamp, ServiceName, TraceId, SpanId, Body, LogAttributes, if(mapContains(LogAttributes, 'exception.type') AND LogAttributes['exception.type'] != '', LogAttributes['exception.type'], 'Error') AS ErrType, if(mapContains(LogAttributes, 'exception.message') AND LogAttributes['exception.message'] != '', LogAttributes['exception.message'], Body) AS Message FROM (" + errorSourcesSQL + ")"

		if errorID != "" {
			if rows, queryErr := queryRows(r.Context(), store, "SELECT * FROM ("+baseErrorSQL+") WHERE ErrorId = ? ORDER BY Timestamp DESC LIMIT 1", errorID); queryErr == nil && len(rows) > 0 {
				primaryError = buildIncidentErrorItem(rows[0])
				primaryError["resolved"] = loadResolvedErrorIDs(r.Context(), store, []string{anyToString(primaryError["id"])})[anyToString(primaryError["id"])]
			}
		}

		if traceID != "" {
			if traceRows, queryErr := queryRows(r.Context(), store, "SELECT Timestamp, TraceId, SpanId, ParentSpanId, SpanName, ServiceName, Duration, StatusCode, SpanAttributes FROM otel_traces WHERE TraceId = ? ORDER BY Timestamp ASC", traceID); queryErr == nil && len(traceRows) > 0 {
				servicesSet := map[string]struct{}{}
				startMS := parseTimestampMs(anyToString(traceRows[0]["Timestamp"]))
				endMS := startMS
				for _, row := range traceRows {
					svc := strings.TrimSpace(anyToString(row["ServiceName"]))
					if svc != "" {
						servicesSet[svc] = struct{}{}
					}
					spanStart := parseTimestampMs(anyToString(row["Timestamp"]))
					spanEnd := spanStart + roundFloat(float64(anyToInt(row["Duration"]))/1000000.0, 2)
					if spanEnd > endMS {
						endMS = spanEnd
					}
				}
				services := make([]string, 0, len(servicesSet))
				for svc := range servicesSet {
					services = append(services, svc)
				}
				sort.Strings(services)
				root := traceRows[0]
				primaryTrace = map[string]any{
					"trace_id":   traceID,
					"services":   services,
					"service":    firstSelected(services),
					"span_count": len(traceRows),
					"start_ts":   anyToString(root["Timestamp"]),
					"start_ms":   roundFloat(startMS, 0),
					"end_ms":     roundFloat(endMS, 0),
					"total_ms":   roundFloat(maxFloat(0, endMS-startMS), 2),
					"root_name":  anyToString(root["SpanName"]),
					"status":     normalizeTraceStatus(anyToString(root["StatusCode"])),
				}
			}
		}

		if rumSession != "" {
			sessionKeyExpr := "coalesce(nullIf(LogAttributes['session.id'], ''), nullIf(JSONExtractString(Body, 'sessionId'), ''), TraceId)"
			rumWhere := []string{sessionKeyExpr + " = ?"}
			rumParams := []any{rumSession}
			if rumTS != "" {
				rumWhere = append(rumWhere, "Timestamp <= parseDateTime64BestEffort(?, 9)")
				rumParams = append(rumParams, rumTS)
			}
			rumSQL := "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, ServiceName, " + sessionKeyExpr + " AS session_key FROM hyperdx_sessions WHERE " + strings.Join(rumWhere, " AND ") + " ORDER BY Timestamp DESC LIMIT 1"
			if rumRows, queryErr := queryRows(r.Context(), store, rumSQL, rumParams...); queryErr == nil && len(rumRows) > 0 {
				row := rumRows[0]
				primaryRUM = buildRUMEventItem(row["Timestamp"], row["EventName"], row["Body"], row["LogAttributes"], row["TraceId"], row["SpanId"], anyToString(row["session_key"]))
			}
		}

		eventTS := ""
		if primaryError != nil {
			service = anyToString(primaryError["service"])
			eventTS = anyToString(primaryError["ts"])
		} else if primaryTrace != nil {
			service = anyToString(primaryTrace["service"])
			eventTS = anyToString(primaryTrace["start_ts"])
		} else if primaryRUM != nil {
			service = anyToString(primaryRUM["service"])
			eventTS = anyToString(primaryRUM["ts"])
		}

		if eventTS != "" && fromTS == "" && toTS == "" && timeError == "" {
			if eventTime, ok := parseIncidentTimestamp(eventTS); ok {
				halfWindow := time.Duration(windowMinutes/2) * time.Minute
				fromTS = normalizeIncidentTimestamp(eventTime.Add(-halfWindow))
				toTS = normalizeIncidentTimestamp(eventTime.Add(halfWindow))
			}
		}

		errorWhere := make([]string, 0)
		errorParams := make([]any, 0)
		if traceID != "" {
			errorWhere = append(errorWhere, "TraceId = ?")
			errorParams = append(errorParams, traceID)
		} else if service != "" {
			errorWhere = append(errorWhere, "ServiceName = ?")
			errorParams = append(errorParams, service)
		}
		timeConds, timeParams := incidentTimeWindowConditions("Timestamp", fromTS, toTS)
		errorWhere = append(errorWhere, timeConds...)
		errorParams = append(errorParams, timeParams...)
		errorWhereSQL := ""
		if len(errorWhere) > 0 {
			errorWhereSQL = " WHERE " + strings.Join(errorWhere, " AND ")
		}
		relatedErrSQL := "SELECT * FROM (" + baseErrorSQL + ")" + errorWhereSQL + " ORDER BY Timestamp DESC LIMIT ?"
		if errRows, queryErr := queryRows(r.Context(), store, relatedErrSQL, append(errorParams, 51)...); queryErr == nil {
			relatedErrorsTruncated = len(errRows) > 50
			errRows = errRows[:minInt(len(errRows), 50)]
			ids := make([]string, 0, len(errRows))
			for _, row := range errRows {
				item := buildIncidentErrorItem(row)
				ids = append(ids, anyToString(item["id"]))
				relatedErrors = append(relatedErrors, item)
			}
			resolvedIDs := loadResolvedErrorIDs(r.Context(), store, ids)
			filtered := make([]map[string]any, 0, len(relatedErrors))
			primaryID := ""
			if primaryError != nil {
				primaryID = anyToString(primaryError["id"])
			}
			for _, item := range relatedErrors {
				id := anyToString(item["id"])
				item["resolved"] = resolvedIDs[id]
				if id == "" || id == primaryID {
					continue
				}
				filtered = append(filtered, item)
			}
			relatedErrors = filtered
		}

		logWhere := make([]string, 0)
		logParams := make([]any, 0)
		if traceID != "" {
			logWhere = append(logWhere, "TraceId = ?")
			logParams = append(logParams, traceID)
		} else if service != "" {
			logWhere = append(logWhere, "ServiceName = ?")
			logParams = append(logParams, service)
		}
		timeConds, timeParams = incidentTimeWindowConditions("Timestamp", fromTS, toTS)
		logWhere = append(logWhere, timeConds...)
		logParams = append(logParams, timeParams...)
		logWhereSQL := ""
		if len(logWhere) > 0 {
			logWhereSQL = " WHERE " + strings.Join(logWhere, " AND ")
		}
		relatedLogCount = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM otel_logs"+logWhereSQL, logParams...)

		if service != "" {
			spanWhere := []string{"ServiceName = ?"}
			spanParams := []any{service}
			timeConds, timeParams = incidentTimeWindowConditions("Timestamp", fromTS, toTS)
			spanWhere = append(spanWhere, timeConds...)
			spanParams = append(spanParams, timeParams...)
			spanWhereSQL := " WHERE " + strings.Join(spanWhere, " AND ")
			relatedSpanCount = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM otel_traces"+spanWhereSQL, spanParams...)
		}

		rumWhereParts := make([]string, 0)
		rumWhereParams := make([]any, 0)
		sessionKeyExpr := "coalesce(nullIf(LogAttributes['session.id'], ''), nullIf(JSONExtractString(Body, 'sessionId'), ''), TraceId)"
		if traceID != "" {
			rumWhereParts = append(rumWhereParts, "TraceId = ?")
			rumWhereParams = append(rumWhereParams, traceID)
		} else if service != "" {
			rumWhereParts = append(rumWhereParts, "(LogAttributes['service.name'] = ? OR LogAttributes['service'] = ?)")
			rumWhereParams = append(rumWhereParams, service, service)
		}
		timeConds, timeParams = incidentTimeWindowConditions("Timestamp", fromTS, toTS)
		rumWhereParts = append(rumWhereParts, timeConds...)
		rumWhereParams = append(rumWhereParams, timeParams...)
		rumWhereSQL := ""
		if len(rumWhereParts) > 0 {
			rumWhereSQL = " WHERE " + strings.Join(rumWhereParts, " AND ")
		}
		if summaryRows, queryErr := queryRows(r.Context(), store, "SELECT count() AS ev_count, uniq("+sessionKeyExpr+") AS session_count, countIf(EventName IN ('error', 'unhandledrejection')) AS err_count FROM hyperdx_sessions"+rumWhereSQL, rumWhereParams...); queryErr == nil && len(summaryRows) > 0 {
			relatedRUMCount = anyToInt(summaryRows[0]["ev_count"])
			relatedRUMSessions = anyToInt(summaryRows[0]["session_count"])
			relatedRUMErrorCount = anyToInt(summaryRows[0]["err_count"])
		}
		if rumRows, queryErr := queryRows(r.Context(), store, "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, ServiceName, "+sessionKeyExpr+" AS session_key FROM hyperdx_sessions"+rumWhereSQL+" ORDER BY Timestamp DESC LIMIT ?", append(rumWhereParams, 20)...); queryErr == nil {
			for _, row := range rumRows {
				relatedRUMEvents = append(relatedRUMEvents, buildRUMEventItem(row["Timestamp"], row["EventName"], row["Body"], row["LogAttributes"], row["TraceId"], row["SpanId"], anyToString(row["session_key"])))
			}
		}

		if fromTS != "" && toTS != "" {
			serviceNames := []string{}
			if service != "" {
				serviceNames = []string{service}
			}
			rawWindows = s.listIncidentRawWindows(r.Context(), store, serviceNames, fromTS, toTS, 25)
			metricsContext = s.incidentMetricsContext(r.Context(), store, serviceNames, fromTS, toTS)
		}

		if service != "" {
			if rows, queryErr := queryRows(r.Context(), store, "SELECT anomaly_state FROM v_derived_signals_anomaly WHERE ServiceName = ? AND SignalSource = 'traces' AND time >= now() - INTERVAL 48 HOUR ORDER BY time DESC LIMIT 1", service); queryErr == nil && len(rows) > 0 {
				anomalyState = anyToString(rows[0]["anomaly_state"])
			}
		}

		refIDs := make([]string, 0)
		if primaryError != nil {
			refIDs = append(refIDs, anyToString(primaryError["id"]))
		} else if errorID != "" {
			refIDs = append(refIDs, errorID)
		}
		if traceID != "" {
			refIDs = append(refIDs, traceID)
		}
		if rumSession != "" {
			refIDs = append(refIDs, rumSession)
		}
		workItemLinks = s.loadWorkItemLinksForRefIDs(r.Context(), store, refIDs)
		for _, refID := range refIDs {
			if wiRaw, ok := workItemLinks[refID]; ok {
				if wi, ok := wiRaw.(map[string]any); ok && strings.TrimSpace(anyToString(wi["issue_url"])) != "" {
					existingWorkItem = wi
					break
				}
			}
		}
	}

	ctx := pongo2.Context{
		"title":                    "Incident",
		"mobile_breakpoint_max":    "575.98px",
		"request":                  map[string]any{"endpoint": "incident"},
		"_ref":                     ref,
		"trace_id":                 traceID,
		"error_id":                 errorID,
		"rum_session":              rumSession,
		"rum_ts":                   rumTS,
		"from_ts":                  fromTS,
		"to_ts":                    toTS,
		"service":                  service,
		"window_minutes":           windowMinutes,
		"error_msg":                defaultString(timeError, errorMsg),
		"time_error":               timeError,
		"primary_error":            primaryError,
		"primary_trace":            primaryTrace,
		"primary_rum":              primaryRUM,
		"existing_work_item":       existingWorkItem,
		"work_item_links":          workItemLinks,
		"related_errors":           relatedErrors,
		"related_errors_truncated": relatedErrorsTruncated,
		"related_log_count":        relatedLogCount,
		"related_span_count":       relatedSpanCount,
		"anomaly_state":            anomalyState,
		"related_rum_count":        relatedRUMCount,
		"related_rum_sessions":     relatedRUMSessions,
		"related_rum_error_count":  relatedRUMErrorCount,
		"related_rum_events":       relatedRUMEvents,
		"metrics_context":          metricsContext,
		"mc":                       metricsContext,
		"raw_windows":              rawWindows,
		"_wi_list":                 []any{},
	}
	s.renderTemplate(w, "incident.html", ctx)
}

func parseIncidentTimeWindowArgs(r *http.Request) (string, string, string) {
	fromRaw := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toRaw := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	windowRaw := strings.TrimSpace(r.URL.Query().Get("window_s"))

	fromTS := ""
	toTS := ""
	if fromRaw != "" {
		fromTime, ok := parseIncidentTimestamp(fromRaw)
		if !ok {
			return "", "", "Invalid time value. Use ISO-8601, e.g. 2026-03-29T12:00:00Z"
		}
		fromTS = normalizeIncidentTimestamp(fromTime)
	}
	if toRaw != "" {
		toTime, ok := parseIncidentTimestamp(toRaw)
		if !ok {
			return "", "", "Invalid time value. Use ISO-8601, e.g. 2026-03-29T12:00:00Z"
		}
		toTS = normalizeIncidentTimestamp(toTime)
	}
	if fromTS != "" && toTS == "" && windowRaw != "" {
		seconds, err := strconv.Atoi(windowRaw)
		if err != nil || seconds < 1 {
			return "", "", "Invalid time value. Use ISO-8601, e.g. 2026-03-29T12:00:00Z"
		}
		fromTime, _ := parseIncidentTimestamp(fromTS)
		toTS = normalizeIncidentTimestamp(fromTime.Add(time.Duration(seconds) * time.Second))
	}
	if fromTS != "" && toTS != "" {
		fromTime, _ := parseIncidentTimestamp(fromTS)
		toTime, _ := parseIncidentTimestamp(toTS)
		if !toTime.After(fromTime) {
			return "", "", "Invalid time window: to_ts must be later than from_ts"
		}
	}
	return fromTS, toTS, ""
}

func parseIncidentTimestamp(raw string) (time.Time, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC(), true
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05.999999999", "2006-01-02 15:04:05"} {
		if parsed, err := time.ParseInLocation(layout, raw, time.UTC); err == nil {
			return parsed.UTC(), true
		}
	}
	for _, layout := range []string{"2006-01-02 15:04:05 -0700 MST", "2006-01-02 15:04:05.999999999 -0700 MST"} {
		if parsed, err := time.Parse(layout, raw); err == nil {
			return parsed.UTC(), true
		}
	}
	return time.Time{}, false
}

func normalizeIncidentTimestamp(ts time.Time) string {
	return ts.UTC().Format("2006-01-02 15:04:05")
}

func incidentTimeWindowConditions(column string, fromTS string, toTS string) ([]string, []any) {
	conditions := make([]string, 0)
	params := make([]any, 0)
	if fromTS != "" {
		conditions = append(conditions, column+" >= parseDateTime64BestEffort(?, 9)")
		params = append(params, fromTS)
	}
	if toTS != "" {
		conditions = append(conditions, column+" < parseDateTime64BestEffort(?, 9)")
		params = append(params, toTS)
	}
	return conditions, params
}

func buildIncidentErrorItem(row map[string]any) map[string]any {
	attrs := parseStringMap(anyToString(incidentRowValue(row, "LogAttributes")))
	stack := strings.TrimSpace(attrs["exception.stacktrace"])
	traceID := anyToString(incidentRowValue(row, "TraceId"))
	spanID := anyToString(incidentRowValue(row, "SpanId"))
	errType := firstNonEmpty(anyToString(incidentRowValue(row, "ErrType")), "Error")
	message := firstNonEmpty(anyToString(incidentRowValue(row, "Message")), anyToString(incidentRowValue(row, "Body")))
	service := anyToString(incidentRowValue(row, "ServiceName"))
	ts := anyToString(incidentRowValue(row, "Timestamp"))
	itemID := anyToString(incidentRowValue(row, "ErrorId", "id"))
	if itemID == "" {
		itemID = anyToString(incidentRowValue(row, "id"))
	}
	return map[string]any{
		"id":                   itemID,
		"ts":                   ts,
		"service":              service,
		"err_type":             errType,
		"message":              message,
		"message_summary":      message,
		"summary_from_json":    false,
		"message_is_json":      false,
		"message_pretty_json":  "",
		"raw_body":             anyToString(incidentRowValue(row, "Body")),
		"raw_body_is_json":     false,
		"raw_body_pretty_json": "",
		"stack":                stack,
		"stack_is_json":        false,
		"stack_pretty_json":    "",
		"trace_id":             traceID,
		"span_id":              spanID,
		"trace_ids_csv":        traceID,
		"url":                  firstNonEmpty(attrs["url.full"], attrs["url"]),
		"error_source":         attrs["error.source"],
		"page_title":           attrs["browser.page.title"],
		"viewport":             attrs["browser.viewport"],
		"artifact_type":        attrs["artifact.type"],
		"artifact_id":          attrs["artifact.id"],
		"artifact_url":         attrs["artifact.url"],
		"replay_id":            attrs["replay.id"],
		"replay_url":           attrs["replay.url"],
		"resolved":             false,
	}
}

func incidentRowValue(row map[string]any, keys ...string) any {
	if len(keys) == 0 || len(row) == 0 {
		return nil
	}
	for _, key := range keys {
		if value, ok := row[key]; ok {
			return value
		}
		needle := strings.ToLower(strings.TrimSpace(key))
		for candidate, value := range row {
			if strings.ToLower(strings.TrimSpace(candidate)) == needle {
				return value
			}
		}
	}
	return nil
}

func (s *Server) listIncidentRawWindows(ctx context.Context, store extensionpoints.ClickHouseStore, serviceNames []string, startTS string, endTS string, limit int) []map[string]any {
	if startTS == "" || endTS == "" {
		return []map[string]any{}
	}
	whereParts := []string{
		"WindowEnd >= parseDateTime64BestEffort(?, 9)",
		"WindowStart <= parseDateTime64BestEffort(?, 9)",
	}
	params := []any{startTS, endTS}
	if len(serviceNames) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(serviceNames)), ",")
		whereParts = append(whereParts, "(ServiceName = '' OR ServiceName IN ("+placeholders+"))")
		for _, svc := range serviceNames {
			params = append(params, svc)
		}
	}
	windowSQL := "SELECT Id, SignalType, SignalRef, ServiceName, Namespace, NodeName, WindowStart, WindowEnd FROM sobs_raw_windows FINAL WHERE " + strings.Join(whereParts, " AND ") + " ORDER BY WindowStart DESC LIMIT ?"
	params = append(params, maxInt(1, minInt(limit, 100)))
	rows, err := queryRows(ctx, store, windowSQL, params...)
	if err != nil || len(rows) == 0 {
		return []map[string]any{}
	}
	windowIDs := make([]string, 0, len(rows))
	for _, row := range rows {
		windowIDs = append(windowIDs, anyToString(row["Id"]))
	}
	copiedCounts := s.incidentWindowCopyCounts(ctx, store, windowIDs)
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		id := anyToString(row["Id"])
		copiedCount := copiedCounts[id]
		expectedCount := 3
		out = append(out, map[string]any{
			"id":             id,
			"signal_type":    anyToString(row["SignalType"]),
			"signal_ref":     anyToString(row["SignalRef"]),
			"service_name":   anyToString(row["ServiceName"]),
			"namespace":      anyToString(row["Namespace"]),
			"node_name":      anyToString(row["NodeName"]),
			"window_start":   anyToString(row["WindowStart"]),
			"window_end":     anyToString(row["WindowEnd"]),
			"copied_count":   copiedCount,
			"expected_count": expectedCount,
			"copy_complete":  copiedCount >= expectedCount,
		})
	}
	return out
}

func (s *Server) incidentWindowCopyCounts(ctx context.Context, store extensionpoints.ClickHouseStore, windowIDs []string) map[string]int {
	out := map[string]int{}
	if len(windowIDs) == 0 {
		return out
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(windowIDs)), ",")
	args := make([]any, 0, len(windowIDs))
	for _, id := range windowIDs {
		args = append(args, id)
	}
	query := "SELECT WindowId, countDistinct(SourceTable) AS c FROM sobs_raw_window_copy_state FINAL WHERE WindowId IN (" + placeholders + ") GROUP BY WindowId"
	rows, err := queryRows(ctx, store, query, args...)
	if err != nil {
		return out
	}
	for _, row := range rows {
		out[anyToString(row["WindowId"])] = anyToInt(row["c"])
	}
	return out
}

func (s *Server) incidentMetricsContext(ctx context.Context, store extensionpoints.ClickHouseStore, serviceNames []string, startTS string, endTS string) map[string]any {
	result := map[string]any{
		"source_mode":      "none",
		"total_points":     0,
		"series":           []any{},
		"match_mode":       "none",
		"match_label":      "no match",
		"match_dimensions": []any{},
		"health_chips":     []any{},
	}
	if startTS == "" || endTS == "" {
		return result
	}
	whereParts := []string{"TimeUnix >= parseDateTime64BestEffort(?, 9)", "TimeUnix <= parseDateTime64BestEffort(?, 9)"}
	params := []any{startTS, endTS}
	matchMode := "time_window_only"
	matchLabel := "time window only"
	matchDimensions := []any{"time_window"}
	if len(serviceNames) > 0 {
		placeholders := strings.TrimSuffix(strings.Repeat("?,", len(serviceNames)), ",")
		whereParts = append(whereParts, "ServiceName IN ("+placeholders+")")
		for _, svc := range serviceNames {
			params = append(params, svc)
		}
		matchMode = "service_exact"
		matchLabel = "service exact"
		matchDimensions = []any{"service"}
	}
	whereSQL := strings.Join(whereParts, " AND ")
	total := summaryQuerySingleInt(ctx, store, "SELECT count() FROM v_otel_metrics_dedup WHERE "+whereSQL, params...)
	if total <= 0 {
		return result
	}
	rows, err := queryRows(ctx, store, "SELECT ServiceName, MetricName, count() AS points, round(avg(Value), 4) AS avg_value, round(min(Value), 4) AS min_value, round(max(Value), 4) AS max_value FROM v_otel_metrics_dedup WHERE "+whereSQL+" GROUP BY ServiceName, MetricName ORDER BY points DESC, MetricName ASC LIMIT 12", params...)
	if err != nil {
		return result
	}
	series := make([]any, 0, len(rows))
	for _, row := range rows {
		series = append(series, map[string]any{
			"service": anyToString(row["ServiceName"]),
			"metric":  anyToString(row["MetricName"]),
			"points":  anyToInt(row["points"]),
			"avg":     anyToFloat(row["avg_value"]),
			"min":     anyToFloat(row["min_value"]),
			"max":     anyToFloat(row["max_value"]),
		})
	}
	result["source_mode"] = "mixed"
	result["total_points"] = total
	result["series"] = series
	result["match_mode"] = matchMode
	result["match_label"] = matchLabel
	result["match_dimensions"] = matchDimensions
	return result
}

func (s *Server) loadWorkItemLinksForRefIDs(ctx context.Context, store extensionpoints.ClickHouseStore, refIDs []string) map[string]any {
	refSet := make([]string, 0, len(refIDs))
	seen := map[string]bool{}
	for _, raw := range refIDs {
		id := strings.TrimSpace(raw)
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		refSet = append(refSet, id)
	}
	if len(refSet) == 0 {
		return map[string]any{}
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(refSet)), ",")
	args := make([]any, 0, len(refSet))
	for _, id := range refSet {
		args = append(args, id)
	}
	query := "SELECT AnomalyRuleId, IssueUrl, CanonicalIssueUrl, IssueNumber, IssueState FROM sobs_github_work_items FINAL WHERE IsDeleted = 0 AND IssueUrl != '' AND AnomalyRuleId IN (" + placeholders + ") ORDER BY CreatedAt DESC"
	rows, err := queryRows(ctx, store, query, args...)
	if err != nil {
		return map[string]any{}
	}
	out := map[string]any{}
	for _, row := range rows {
		ref := anyToString(row["AnomalyRuleId"])
		if ref == "" {
			continue
		}
		if _, exists := out[ref]; exists {
			continue
		}
		out[ref] = map[string]any{
			"issue_url":    defaultString(anyToString(row["IssueUrl"]), anyToString(row["CanonicalIssueUrl"])),
			"issue_number": anyToInt(row["IssueNumber"]),
			"issue_state":  anyToString(row["IssueState"]),
		}
	}
	return out
}

func (s *Server) incidentHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/incident/help" {
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
	s.renderTemplate(w, "incident_help.html", pongo2.Context{"title": "Incident Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "incident/help"}})
}
func (s *Server) rumPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rum" {
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

	viewMode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
	if viewMode != "events" {
		viewMode = "sessions"
	}
	eventType := strings.TrimSpace(r.URL.Query().Get("type"))
	errorSource := strings.TrimSpace(r.URL.Query().Get("error_source"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	limit := parseLimitParam(r, 200, 1, 10000)
	offset := parseOffsetParam(r)

	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortDir := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort_dir")))
	if sortDir != "asc" {
		sortDir = "desc"
	}
	sortCol := "Timestamp"
	if viewMode == "sessions" {
		sortCol = "severity_rank"
		switch sortBy {
		case "last_seen":
			sortCol = "last_ts"
		case "events":
			sortCol = "event_count"
		case "errors":
			sortCol = "error_count"
		case "session":
			sortCol = "session_key"
		}
	} else {
		switch sortBy {
		case "EventName":
			sortCol = "EventName"
		}
	}

	where, params := rumTimeWhereAndParams(fromTS, toTS)
	if eventType != "" {
		where = appendWhereClause(where, "EventName = ?")
		params = append(params, eventType)
	}
	if errorSource != "" {
		where = appendWhereClause(where, "LogAttributes['errorSource'] = ?")
		params = append(params, errorSource)
	}
	if q != "" {
		like := "%" + q + "%"
		where = appendWhereClause(where, "(Body ILIKE ? OR LogAttributes['url'] ILIKE ? OR LogAttributes['session.id'] ILIKE ? OR LogAttributes['errorSource'] ILIKE ? OR TraceId ILIKE ?)")
		params = append(params, like, like, like, like, like)
	}

	total := 0
	events := []map[string]any{}
	sessionGroups := []map[string]any{}
	sessionKeyExpr := "coalesce(nullIf(LogAttributes['session.id'], ''), nullIf(JSONExtractString(Body, 'sessionId'), ''), TraceId)"
	vitalsSummary := map[string]map[string]any{}
	vitalsSparklines := map[string][]map[string]any{}
	vitalsHotspot := map[string][]map[string]any{}
	errorStats := map[string]any{
		"total":        0,
		"by_type":      map[string]int{},
		"trend":        "stable",
		"recent":       0,
		"prior":        0,
		"sparkline":    []map[string]any{},
		"top_messages": []map[string]any{},
		"top_urls":     []map[string]any{},
	}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer func() { _ = store.Close() }()

		if viewMode == "sessions" {
			total = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM (SELECT "+sessionKeyExpr+" AS session_key FROM hyperdx_sessions "+where+" GROUP BY session_key)", params...)
			summarySQL := "SELECT " + sessionKeyExpr + " AS session_key, max(Timestamp) AS last_ts, count() AS event_count, countIf(EventName IN ('error','unhandledrejection')) AS error_count, countIf(EventName = 'web-vital' AND JSONExtractString(Body, 'rating') = 'poor') AS poor_vital_count, countIf(EventName = 'web-vital' AND JSONExtractString(Body, 'rating') = 'needs-improvement') AS warn_vital_count, greatest(if(countIf(EventName IN ('error','unhandledrejection')) > 0, 3, 0), if(countIf(EventName = 'web-vital' AND JSONExtractString(Body, 'rating') = 'poor') > 0, 2, 0), if(countIf(EventName = 'web-vital' AND JSONExtractString(Body, 'rating') = 'needs-improvement') > 0, 1, 0)) AS severity_rank, argMax(EventName, Timestamp) AS last_event_type, argMax(LogAttributes['url'], Timestamp) AS last_url, argMax(if(TraceId != '', TraceId, JSONExtractString(Body, 'traceId')), Timestamp) AS trace_id FROM hyperdx_sessions " + where + " GROUP BY session_key ORDER BY " + sortCol + " " + strings.ToUpper(sortDir) + " LIMIT ? OFFSET ?"
			summaryParams := append(append([]any{}, params...), limit, offset)
			rows, queryErr := queryRows(r.Context(), store, summarySQL, summaryParams...)
			if queryErr == nil {
				sessionKeys := make([]string, 0, len(rows))
				sessionGroups = make([]map[string]any, 0, len(rows))
				for _, row := range rows {
					sessionKey := strings.TrimSpace(anyToString(row["session_key"]))
					if sessionKey == "" {
						continue
					}
					sessionKeys = append(sessionKeys, sessionKey)
					sessionGroups = append(sessionGroups, map[string]any{
						"session_key":      sessionKey,
						"session_id":       truncateString(sessionKey, 8),
						"last_ts":          anyToString(row["last_ts"]),
						"event_count":      anyToInt(row["event_count"]),
						"error_count":      anyToInt(row["error_count"]),
						"poor_vital_count": anyToInt(row["poor_vital_count"]),
						"warn_vital_count": anyToInt(row["warn_vital_count"]),
						"severity_rank":    anyToInt(row["severity_rank"]),
						"last_event_type":  anyToString(row["last_event_type"]),
						"last_url":         anyToString(row["last_url"]),
						"trace_id":         anyToString(row["trace_id"]),
						"events_list":      []map[string]any{},
						"has_replay":       false,
						"has_artifact":     false,
					})
				}

				if len(sessionKeys) > 0 {
					placeholders := strings.TrimSuffix(strings.Repeat("?,", len(sessionKeys)), ",")
					detailWhere := where
					detailWhere = appendWhereClause(detailWhere, sessionKeyExpr+" IN ("+placeholders+")")
					detailParams := append(append([]any{}, params...), make([]any, 0, len(sessionKeys))...)
					for _, sessionKey := range sessionKeys {
						detailParams = append(detailParams, sessionKey)
					}
					detailSQL := "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, " + sessionKeyExpr + " AS session_key FROM hyperdx_sessions " + detailWhere + " ORDER BY Timestamp DESC"
					detailRows, detailErr := queryRows(r.Context(), store, detailSQL, detailParams...)
					if detailErr == nil {
						eventsBySession := map[string][]map[string]any{}
						for _, row := range detailRows {
							sessionKey := anyToString(row["session_key"])
							item := buildRUMEventItem(row["Timestamp"], row["EventName"], row["Body"], row["LogAttributes"], row["TraceId"], row["SpanId"], sessionKey)
							eventsBySession[sessionKey] = append(eventsBySession[sessionKey], item)
						}
						for i := range sessionGroups {
							sessionKey := anyToString(sessionGroups[i]["session_key"])
							sessionEvents := eventsBySession[sessionKey]
							sessionGroups[i]["events_list"] = sessionEvents
							sessionGroups[i]["has_replay"] = rumEventsHaveCapability(sessionEvents, "has_replay")
							sessionGroups[i]["has_artifact"] = rumEventsHaveCapability(sessionEvents, "has_artifact")
							if anyToString(sessionGroups[i]["trace_id"]) == "" {
								sessionGroups[i]["trace_id"] = firstTraceID(sessionEvents)
							}
						}
					}
				}
			}
		} else {
			total = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM hyperdx_sessions "+where, params...)
			eventSQL := "SELECT Timestamp, EventName, Body, LogAttributes, TraceId, SpanId, " + sessionKeyExpr + " AS session_key FROM hyperdx_sessions " + where + " ORDER BY " + sortCol + " " + strings.ToUpper(sortDir) + " LIMIT ? OFFSET ?"
			eventParams := append(append([]any{}, params...), limit, offset)
			rows, queryErr := queryRows(r.Context(), store, eventSQL, eventParams...)
			if queryErr == nil {
				events = make([]map[string]any, 0, len(rows))
				for _, row := range rows {
					events = append(events, buildRUMEventItem(row["Timestamp"], row["EventName"], row["Body"], row["LogAttributes"], row["TraceId"], row["SpanId"], anyToString(row["session_key"])))
				}
			}
		}

		eventTypeRows, eventTypeErr := store.Query(r.Context(), "SELECT DISTINCT EventName FROM hyperdx_sessions ORDER BY EventName")
		eventTypes := []string{}
		if eventTypeErr == nil {
			defer func() { _ = eventTypeRows.Close() }()
			for eventTypeRows.Next() {
				var v any
				if scanErr := eventTypeRows.Scan(&v); scanErr == nil {
					if s := anyToString(v); s != "" {
						eventTypes = append(eventTypes, s)
					}
				}
			}
		}

		errorSourceRows, errorSourceErr := store.Query(r.Context(), "SELECT DISTINCT LogAttributes['errorSource'] FROM hyperdx_sessions WHERE LogAttributes['errorSource'] != '' ORDER BY LogAttributes['errorSource']")
		errorSources := []string{}
		if errorSourceErr == nil {
			defer errorSourceRows.Close()
			for errorSourceRows.Next() {
				var v any
				if scanErr := errorSourceRows.Scan(&v); scanErr == nil {
					if s := anyToString(v); s != "" {
						errorSources = append(errorSources, s)
					}
				}
			}
		}

		vitalRows, vitalErr := store.Query(r.Context(), "SELECT JSONExtractString(Body, 'name') AS metric, quantileExact(0.75)(JSONExtractFloat(Body, 'value')) AS p75, count() AS cnt, countIf(JSONExtractString(Body, 'rating')='poor') AS poor_cnt, countIf(JSONExtractString(Body, 'rating')='needs-improvement') AS warn_cnt FROM hyperdx_sessions WHERE EventName='web-vital' AND Timestamp >= now() - INTERVAL 60 MINUTE GROUP BY metric")
		if vitalErr == nil {
			defer vitalRows.Close()
			for vitalRows.Next() {
				var metric, p75, cnt, poorCnt, warnCnt any
				if scanErr := vitalRows.Scan(&metric, &p75, &cnt, &poorCnt, &warnCnt); scanErr != nil {
					continue
				}
				n := anyToString(metric)
				state := "normal"
				if anyToInt(poorCnt) > 0 {
					state = "outlier"
				} else if anyToInt(warnCnt) > 0 {
					state = "warning"
				}
				vitalsSummary[n] = map[string]any{"p75": anyToString(p75), "count": anyToInt(cnt), "anomaly_state": state}
			}
		}

		sparkRows, sparkErr := store.Query(r.Context(), "SELECT JSONExtractString(Body, 'name') AS metric, toStartOfMinute(Timestamp) AS bucket, avg(JSONExtractFloat(Body, 'value')) AS avg_val FROM hyperdx_sessions WHERE EventName='web-vital' AND Timestamp >= now() - INTERVAL 60 MINUTE GROUP BY metric, bucket ORDER BY metric, bucket")
		if sparkErr == nil {
			defer sparkRows.Close()
			for sparkRows.Next() {
				var metric, bucket, avgVal any
				if scanErr := sparkRows.Scan(&metric, &bucket, &avgVal); scanErr != nil {
					continue
				}
				n := anyToString(metric)
				vitalsSparklines[n] = append(vitalsSparklines[n], map[string]any{"t": anyToString(bucket), "v": anyToString(avgVal)})
			}
		}

		hotspotRows, hotspotErr := store.Query(r.Context(), "SELECT JSONExtractString(Body, 'name') AS metric, LogAttributes['url'] AS url, count() AS total, countIf(JSONExtractString(Body, 'rating') = 'poor') AS poor_count, round(toFloat64(poor_count) / toFloat64(total), 3) AS poor_rate, round(quantileExact(0.75)(JSONExtractFloat(Body, 'value')), 1) AS p75 FROM hyperdx_sessions WHERE EventName = 'web-vital' AND Timestamp >= now() - INTERVAL 24 HOUR GROUP BY metric, url HAVING total >= 3 ORDER BY metric ASC, poor_rate DESC, total DESC LIMIT 60")
		if hotspotErr == nil {
			defer hotspotRows.Close()
			for hotspotRows.Next() {
				var metric, url, totalRows, poorCount, poorRate, p75 any
				if scanErr := hotspotRows.Scan(&metric, &url, &totalRows, &poorCount, &poorRate, &p75); scanErr != nil {
					continue
				}
				n := anyToString(metric)
				if n == "" {
					continue
				}
				vitalsHotspot[n] = append(vitalsHotspot[n], map[string]any{
					"url":        anyToString(url),
					"total":      anyToInt(totalRows),
					"poor_count": anyToInt(poorCount),
					"poor_rate":  anyToFloat(poorRate),
					"p75":        anyToFloat(p75),
				})
			}
			for metric, rows := range vitalsHotspot {
				if len(rows) > 5 {
					vitalsHotspot[metric] = rows[:5]
				}
			}
		}

		trendRows, trendErr := store.Query(r.Context(), "SELECT countIf(Timestamp >= now() - INTERVAL 30 MINUTE) AS recent, countIf(Timestamp >= now() - INTERVAL 60 MINUTE AND Timestamp < now() - INTERVAL 30 MINUTE) AS prior FROM hyperdx_sessions WHERE EventName IN ('error','unhandledrejection') AND Timestamp >= now() - INTERVAL 60 MINUTE")
		if trendErr == nil {
			defer trendRows.Close()
			if trendRows.Next() {
				var recent, prior any
				if scanErr := trendRows.Scan(&recent, &prior); scanErr == nil {
					r := anyToInt(recent)
					p := anyToInt(prior)
					errorStats["recent"] = r
					errorStats["prior"] = p
					trend := "stable"
					if p == 0 && r > 0 {
						trend = "up"
					} else if p > 0 && r > int(float64(p)*1.25) {
						trend = "up"
					} else if p > 0 && r < int(float64(p)*0.75) {
						trend = "down"
					}
					errorStats["trend"] = trend
				}
			}
		}

		typeRows, typeErr := store.Query(r.Context(), "SELECT EventName, count() AS cnt FROM hyperdx_sessions WHERE EventName IN ('error','unhandledrejection') AND Timestamp >= now() - INTERVAL 24 HOUR GROUP BY EventName")
		if typeErr == nil {
			defer typeRows.Close()
			totalErr := 0
			byType := map[string]int{}
			for typeRows.Next() {
				var name, cnt any
				if scanErr := typeRows.Scan(&name, &cnt); scanErr != nil {
					continue
				}
				c := anyToInt(cnt)
				totalErr += c
				byType[anyToString(name)] = c
			}
			errorStats["total"] = totalErr
			errorStats["by_type"] = byType
		}

		sparkErrRows, sparkErrQuery := store.Query(r.Context(), "SELECT mb, cnt FROM (SELECT toStartOfMinute(Timestamp) AS mb, count() AS cnt FROM hyperdx_sessions WHERE EventName IN ('error','unhandledrejection') AND Timestamp >= now() - INTERVAL 180 MINUTE GROUP BY mb) ORDER BY mb WITH FILL FROM toStartOfMinute(now() - INTERVAL 180 MINUTE) TO toStartOfMinute(now()) STEP toIntervalMinute(1)")
		if sparkErrQuery == nil {
			defer sparkErrRows.Close()
			errSpark := []map[string]any{}
			for sparkErrRows.Next() {
				var bucket, cnt any
				if scanErr := sparkErrRows.Scan(&bucket, &cnt); scanErr != nil {
					continue
				}
				errSpark = append(errSpark, map[string]any{"t": anyToString(bucket), "v": anyToInt(cnt)})
			}
			errorStats["sparkline"] = errSpark
		}

		topMsgRows, topMsgErr := store.Query(r.Context(), "SELECT JSONExtractString(Body, 'message') AS message, count() AS cnt FROM hyperdx_sessions WHERE EventName IN ('error','unhandledrejection') AND Timestamp >= now() - INTERVAL 24 HOUR AND JSONExtractString(Body, 'message') != '' GROUP BY message ORDER BY cnt DESC LIMIT 8")
		if topMsgErr == nil {
			defer topMsgRows.Close()
			msgs := []map[string]any{}
			for topMsgRows.Next() {
				var message, cnt any
				if scanErr := topMsgRows.Scan(&message, &cnt); scanErr != nil {
					continue
				}
				msgs = append(msgs, map[string]any{"message": anyToString(message), "count": anyToInt(cnt)})
			}
			errorStats["top_messages"] = msgs
		}

		topURLRows, topURLErr := store.Query(r.Context(), "SELECT LogAttributes['url'] AS url, count() AS cnt FROM hyperdx_sessions WHERE EventName IN ('error','unhandledrejection') AND Timestamp >= now() - INTERVAL 24 HOUR AND LogAttributes['url'] != '' GROUP BY url ORDER BY cnt DESC LIMIT 5")
		if topURLErr == nil {
			defer topURLRows.Close()
			urls := []map[string]any{}
			for topURLRows.Next() {
				var url, cnt any
				if scanErr := topURLRows.Scan(&url, &cnt); scanErr != nil {
					continue
				}
				urls = append(urls, map[string]any{"url": anyToString(url), "count": anyToInt(cnt)})
			}
			errorStats["top_urls"] = urls
		}

		ctx := pongo2.Context{
			"title":                 "RUM",
			"mobile_breakpoint_max": "575.98px",
			"request":               map[string]any{"endpoint": "rum"},
			"total":                 total,
			"limit":                 limit,
			"offset":                offset,
			"view_mode":             viewMode,
			"event_type":            eventType,
			"event_types":           eventTypes,
			"error_source":          errorSource,
			"error_sources":         errorSources,
			"events":                events,
			"session_groups":        sessionGroups,
			"vitals_summary":        vitalsSummary,
			"vitals_sparklines":     vitalsSparklines,
			"vitals_hotspot":        vitalsHotspot,
			"error_stats":           errorStats,
			"sort_by":               sortBy,
			"sort_dir":              sortDir,
			"from_ts":               fromTS,
			"to_ts":                 toTS,
			"q":                     q,
			"error_msg":             "",
		}
		s.renderTemplate(w, "rum.html", ctx)
		return
	}

	ctx := pongo2.Context{
		"title":                 "RUM",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "rum"},
		"total":                 0,
		"limit":                 limit,
		"offset":                offset,
		"view_mode":             viewMode,
		"event_type":            eventType,
		"event_types":           []string{},
		"error_source":          errorSource,
		"error_sources":         []string{},
		"events":                []map[string]any{},
		"session_groups":        []map[string]any{},
		"vitals_summary":        map[string]map[string]any{},
		"vitals_sparklines":     map[string][]map[string]any{},
		"vitals_hotspot":        map[string][]map[string]any{},
		"error_stats": map[string]any{
			"total":        0,
			"by_type":      map[string]int{},
			"trend":        "stable",
			"recent":       0,
			"prior":        0,
			"sparkline":    []map[string]any{},
			"top_messages": []map[string]any{},
			"top_urls":     []map[string]any{},
		},
		"sort_by":   sortBy,
		"sort_dir":  sortDir,
		"from_ts":   fromTS,
		"to_ts":     toTS,
		"q":         q,
		"error_msg": "",
	}
	s.renderTemplate(w, "rum.html", ctx)
}
func (s *Server) rumHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/rum/help" {
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
	s.renderTemplate(w, "rum_help.html", pongo2.Context{"title": "RUM Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "rum/help"}})
}
func (s *Server) webTrafficPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web-traffic" {
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

	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	where, params := rumTimeWhereAndParams(fromTS, toTS)

	total := 0
	topURLs := [][]any{}
	eventTypes := [][]any{}

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer store.Close()

		if count, countErr := queryCount(r, store, "hyperdx_sessions", where, params); countErr == nil {
			total = count
		}

		rows, urlErr := store.Query(r.Context(), "SELECT LogAttributes['url'] AS url, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY url HAVING url != '' ORDER BY cnt DESC LIMIT 20", params...)
		if urlErr == nil {
			defer rows.Close()
			for rows.Next() {
				var url, cnt any
				if scanErr := rows.Scan(&url, &cnt); scanErr != nil {
					continue
				}
				topURLs = append(topURLs, []any{anyToString(url), anyToInt(cnt)})
			}
		}

		eventRows, eventErr := store.Query(r.Context(), "SELECT EventName, count() AS cnt FROM hyperdx_sessions "+where+" GROUP BY EventName ORDER BY cnt DESC LIMIT 20", params...)
		if eventErr == nil {
			defer eventRows.Close()
			for eventRows.Next() {
				var eventName, cnt any
				if scanErr := eventRows.Scan(&eventName, &cnt); scanErr != nil {
					continue
				}
				eventTypes = append(eventTypes, []any{anyToString(eventName), anyToInt(cnt)})
			}
		}
	}

	enrichmentSettings := s.settingsService.Enrichment()
	ctx := pongo2.Context{
		"title":                 "Web Traffic",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "web-traffic", "args": map[string]any{"from_ts": fromTS, "to_ts": toTS}},
		"from_ts":               fromTS,
		"to_ts":                 toTS,
		"total":                 total,
		"geo_enabled":           parseBool(pickSetting(enrichmentSettings, "geo_enabled", "enrichment.geo_enabled")),
		"event_types":           eventTypes,
		"top_urls":              topURLs,
		"error_msg":             "",
	}
	s.renderTemplate(w, "web_traffic.html", ctx)
}

func rumTimeWhereAndParams(fromTS string, toTS string) (string, []any) {
	where := ""
	params := []any{}
	if fromTS != "" {
		where = appendWhereClause(where, "Timestamp >= parseDateTime64BestEffort(?, 9)")
		params = append(params, fromTS)
	}
	if toTS != "" {
		where = appendWhereClause(where, "Timestamp < parseDateTime64BestEffort(?, 9)")
		params = append(params, toTS)
	}
	return where, params
}

func appendWhereClause(where string, clause string) string {
	if strings.TrimSpace(clause) == "" {
		return where
	}
	if strings.TrimSpace(where) == "" {
		return "WHERE " + clause
	}
	return where + " AND " + clause
}

func parseJSONMap(raw string) map[string]any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]any{}
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out
	}
	return map[string]any{"message": raw}
}

func buildRUMEventItem(ts any, eventName any, body any, logAttrs any, traceID any, spanID any, sessionKey string) map[string]any {
	data := parseJSONMap(anyToString(body))
	attrs := parseStringMap(anyToString(logAttrs))
	trace := anyToString(traceID)
	if trace == "" {
		trace = anyToString(data["traceId"])
	}
	span := anyToString(spanID)
	if span == "" {
		span = anyToString(data["spanId"])
	}
	if trace != "" {
		data["traceId"] = trace
	}
	if span != "" {
		data["spanId"] = span
	}
	url := strings.TrimSpace(attrs["url"])
	if url == "" {
		url = strings.TrimSpace(attrs["url.full"])
	}
	service := anyToString(data["service"])
	hasArtifact := nestedHasIDOrURL(data["artifact"])
	hasReplay := nestedHasIDOrURL(data["replay"])
	return map[string]any{
		"ts":           anyToString(ts),
		"session_key":  sessionKey,
		"session_id":   truncateString(sessionKey, 8),
		"event_type":   anyToString(eventName),
		"url":          url,
		"data":         data,
		"trace_id":     trace,
		"span_id":      span,
		"service":      service,
		"has_artifact": hasArtifact,
		"has_replay":   hasReplay,
	}
}

func parseStringMap(raw string) map[string]string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return map[string]string{}
	}
	out := map[string]string{}
	if err := json.Unmarshal([]byte(trimmed), &out); err == nil {
		return out
	}
	return map[string]string{}
}

func nestedHasIDOrURL(value any) bool {
	item, ok := value.(map[string]any)
	if !ok {
		return false
	}
	return strings.TrimSpace(anyToString(item["id"])) != "" || strings.TrimSpace(anyToString(item["url"])) != ""
}

func rumEventsHaveCapability(events []map[string]any, key string) bool {
	for _, event := range events {
		if value, ok := event[key].(bool); ok && value {
			return true
		}
	}
	return false
}

func firstTraceID(events []map[string]any) string {
	for _, event := range events {
		if traceID := strings.TrimSpace(anyToString(event["trace_id"])); traceID != "" {
			return traceID
		}
	}
	return ""
}

func anyToFloat(v any) float64 {
	switch t := v.(type) {
	case float64:
		return t
	case float32:
		return float64(t)
	case int:
		return float64(t)
	case int8:
		return float64(t)
	case int16:
		return float64(t)
	case int32:
		return float64(t)
	case int64:
		return float64(t)
	case uint:
		return float64(t)
	case uint8:
		return float64(t)
	case uint16:
		return float64(t)
	case uint32:
		return float64(t)
	case uint64:
		return float64(t)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(t), 64)
		return f
	case []byte:
		f, _ := strconv.ParseFloat(strings.TrimSpace(string(t)), 64)
		return f
	default:
		f, _ := strconv.ParseFloat(strings.TrimSpace(fmt.Sprintf("%v", t)), 64)
		return f
	}
}

func truncateString(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if len(s) <= n {
		return s
	}
	return s[:n]
}
func (s *Server) webTrafficHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/web-traffic/help" {
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
	s.renderTemplate(w, "web_traffic_help.html", pongo2.Context{"title": "Web Traffic Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "web-traffic/help"}})
}
func (s *Server) workItemsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/work-items" {
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
	serviceFilter := strings.TrimSpace(r.URL.Query().Get("service"))
	ruleFilter := strings.TrimSpace(r.URL.Query().Get("rule_name"))
	actionTypeFilter := strings.TrimSpace(r.URL.Query().Get("action_type"))
	statusFilter := strings.TrimSpace(r.URL.Query().Get("status"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))

	items := []map[string]any{}
	totalItems := 0
	services := []map[string]any{}
	rules := []map[string]any{}
	limit := parseLimitParam(r, 100, 1, 1000)
	offset := parseOffsetParam(r)

	store, err := s.storeFactory.Open(r.Context())
	if err == nil {
		defer func() { _ = store.Close() }()

		conditions := []string{"IsDeleted = 0"}
		params := []any{}
		if serviceFilter != "" {
			conditions = append(conditions, "ServiceName = ?")
			params = append(params, serviceFilter)
		}
		if ruleFilter != "" {
			conditions = append(conditions, "AgentRuleName = ?")
			params = append(params, ruleFilter)
		}
		if actionTypeFilter != "" {
			conditions = append(conditions, "AgentAction = ?")
			params = append(params, actionTypeFilter)
		}
		if statusFilter != "" {
			conditions = append(conditions, "IssueState = ?")
			params = append(params, statusFilter)
		}
		if fromTS != "" {
			conditions = append(conditions, "CreatedAt >= parseDateTime64BestEffort(?, 9)")
			params = append(params, fromTS)
		}
		if toTS != "" {
			conditions = append(conditions, "CreatedAt <= parseDateTime64BestEffort(?, 9)")
			params = append(params, toTS)
		}

		whereSQL := ""
		if len(conditions) > 0 {
			whereSQL = " WHERE " + strings.Join(conditions, " AND ")
		}

		totalItems = summaryQuerySingleInt(r.Context(), store, "SELECT count() FROM sobs_github_work_items FINAL"+whereSQL, params...)
		rows, queryErr := queryRows(r.Context(), store, "SELECT * FROM sobs_github_work_items FINAL"+whereSQL+" ORDER BY CreatedAt DESC LIMIT ? OFFSET ?", append(params, limit, offset)...)
		if queryErr == nil {
			for _, row := range rows {
				items = append(items, serializeWorkItemRow(row))
			}
		}

		serviceRows, serviceErr := queryRows(r.Context(), store, "SELECT DISTINCT ServiceName FROM sobs_github_work_items FINAL WHERE IsDeleted = 0 AND ServiceName != '' ORDER BY ServiceName")
		if serviceErr == nil {
			for _, row := range serviceRows {
				serviceName := anyToString(incidentRowValue(row, "ServiceName"))
				if serviceName == "" {
					continue
				}
				services = append(services, map[string]any{"value": serviceName, "label": serviceName})
			}
		}

		ruleRows, ruleErr := queryRows(r.Context(), store, "SELECT DISTINCT AgentRuleName FROM sobs_github_work_items FINAL WHERE IsDeleted = 0 AND AgentRuleName != '' ORDER BY AgentRuleName")
		if ruleErr == nil {
			for _, row := range ruleRows {
				ruleName := anyToString(incidentRowValue(row, "AgentRuleName"))
				if ruleName == "" {
					continue
				}
				rules = append(rules, map[string]any{"value": ruleName, "label": ruleName})
			}
		}
	}

	ctx := pongo2.Context{
		"title":                 "Work Items",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "work-items"},
		"items":                 items,
		"total_items":           totalItems,
		"services":              services,
		"rules":                 rules,
		"service_filter":        serviceFilter,
		"rule_filter":           ruleFilter,
		"action_type_filter":    actionTypeFilter,
		"status_filter":         statusFilter,
		"from_ts":               fromTS,
		"to_ts":                 toTS,
		"time_error":            "",
	}
	s.renderTemplate(w, "work_items.html", ctx)
}
func (s *Server) workItemsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/work-items/help" {
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
	s.renderTemplate(w, "work_items_help.html", pongo2.Context{"title": "Work Items Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "work-items/help"}})
}
func (s *Server) aiPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ai" {
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

	q := r.URL.Query()
	viewMode := strings.TrimSpace(q.Get("view"))
	if viewMode != "trace" {
		viewMode = "flat"
	}
	limit := 25
	if parsed, err := strconv.Atoi(strings.TrimSpace(q.Get("limit"))); err == nil {
		switch {
		case parsed < 1:
			limit = 25
		case parsed > 200:
			limit = 200
		default:
			limit = parsed
		}
	}
	offset := 0
	if parsed, err := strconv.Atoi(strings.TrimSpace(q.Get("offset"))); err == nil && parsed >= 0 {
		offset = parsed
	}

	selectedServices := uniqueSortedStrings(q["service"])
	selectedModels := uniqueSortedStrings(q["model"])
	selectedOperations := uniqueSortedStrings(q["operation"])
	selectedSpanNames := uniqueSortedStrings(q["span_name"])
	selectedRowTypes := uniqueSortedStrings(q["row_type"])
	fromTS := strings.TrimSpace(q.Get("from_ts"))
	toTS := strings.TrimSpace(q.Get("to_ts"))

	services := []string{}
	models := []string{}
	operations := []string{}
	spanNames := []string{}
	totalCalls := 0
	totalTokensIn := 0
	totalTokensOut := 0
	totalErrors := 0
	errorMsg := ""

	pricing, pricingSources, _ := aiPricingForTemplate(buildAISettingsForTemplate(s.settingsService.AI()))

	if store, err := s.storeFactory.Open(r.Context()); err == nil {
		defer store.Close()

		baseConditions := []string{summaryAISpanCondition}
		timeConditions, timeParams := incidentTimeWindowConditions("Timestamp", fromTS, toTS)
		baseConditions = append(baseConditions, timeConditions...)
		baseWhere := "WHERE " + strings.Join(baseConditions, " AND ")

		servicesRows, servicesErr := queryRows(r.Context(), store,
			"SELECT DISTINCT ServiceName FROM otel_traces "+baseWhere+" AND ServiceName != '' ORDER BY ServiceName LIMIT 500",
			timeParams...,
		)
		if servicesErr == nil {
			for _, row := range servicesRows {
				serviceName := strings.TrimSpace(anyToString(incidentRowValue(row, "ServiceName")))
				if serviceName != "" {
					services = append(services, serviceName)
				}
			}
		} else {
			errorMsg = "Some AI metadata failed to load"
		}

		modelsRows, modelsErr := queryRows(r.Context(), store,
			"SELECT DISTINCT SpanAttributes['gen_ai.request.model'] AS model FROM otel_traces "+baseWhere+" AND SpanAttributes['gen_ai.request.model'] != '' ORDER BY model LIMIT 500",
			timeParams...,
		)
		if modelsErr == nil {
			for _, row := range modelsRows {
				modelName := strings.TrimSpace(anyToString(incidentRowValue(row, "model")))
				if modelName == "" {
					modelName = strings.TrimSpace(anyToString(incidentRowValue(row, "SpanAttributes['gen_ai.request.model']")))
				}
				if modelName == "" {
					continue
				}
				normalizedModel := strings.ToLower(modelName)
				models = append(models, modelName)
				if _, exists := pricing[normalizedModel]; !exists {
					pricing[normalizedModel] = inferAIPricingForModel(normalizedModel)
					pricingSources[normalizedModel] = "inferred"
				}
			}
		} else if errorMsg == "" {
			errorMsg = "Some AI metadata failed to load"
		}

		operationsRows, operationsErr := queryRows(r.Context(), store,
			"SELECT DISTINCT SpanAttributes['gen_ai.operation.name'] AS operation FROM otel_traces "+baseWhere+" AND SpanAttributes['gen_ai.operation.name'] != '' ORDER BY operation LIMIT 500",
			timeParams...,
		)
		if operationsErr == nil {
			for _, row := range operationsRows {
				operationName := strings.TrimSpace(anyToString(incidentRowValue(row, "operation")))
				if operationName == "" {
					operationName = strings.TrimSpace(anyToString(incidentRowValue(row, "SpanAttributes['gen_ai.operation.name']")))
				}
				if operationName != "" {
					operations = append(operations, operationName)
				}
			}
		} else if errorMsg == "" {
			errorMsg = "Some AI metadata failed to load"
		}

		spanRows, spanErr := queryRows(r.Context(), store,
			"SELECT DISTINCT SpanName FROM otel_traces "+baseWhere+" AND SpanName != '' ORDER BY SpanName LIMIT 500",
			timeParams...,
		)
		if spanErr == nil {
			for _, row := range spanRows {
				spanName := strings.TrimSpace(anyToString(incidentRowValue(row, "SpanName")))
				if spanName != "" {
					spanNames = append(spanNames, spanName)
				}
			}
		} else if errorMsg == "" {
			errorMsg = "Some AI metadata failed to load"
		}

		totalsRows, totalsErr := queryRows(r.Context(), store,
			"SELECT SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.input_tokens'])) AS tokens_in, SUM(toUInt64OrZero(SpanAttributes['gen_ai.usage.output_tokens'])) AS tokens_out, COUNT(*) AS total_calls, countIf(SpanAttributes['error.type'] != '') AS total_errors FROM otel_traces "+baseWhere,
			timeParams...,
		)
		if totalsErr == nil && len(totalsRows) > 0 {
			totalTokensIn = anyToInt(totalsRows[0]["tokens_in"])
			totalTokensOut = anyToInt(totalsRows[0]["tokens_out"])
			totalCalls = anyToInt(totalsRows[0]["total_calls"])
			totalErrors = anyToInt(totalsRows[0]["total_errors"])
		}
	}

	services = uniqueSortedStrings(services)
	models = uniqueSortedStrings(models)
	operations = uniqueSortedStrings(operations)
	spanNames = uniqueSortedStrings(spanNames)

	ctx := pongo2.Context{
		"title":                   "AI",
		"mobile_breakpoint_max":   "575.98px",
		"request":                 map[string]any{"endpoint": "ai"},
		"view_mode":               viewMode,
		"service":                 strings.TrimSpace(q.Get("service")),
		"model":                   strings.TrimSpace(q.Get("model")),
		"operation":               strings.TrimSpace(q.Get("operation")),
		"span_name":               strings.TrimSpace(q.Get("span_name")),
		"row_type":                strings.TrimSpace(q.Get("row_type")),
		"sql_where":               strings.TrimSpace(q.Get("sql")),
		"from_ts":                 fromTS,
		"to_ts":                   toTS,
		"sort_by":                 strings.TrimSpace(q.Get("sort_by")),
		"sort_dir":                strings.TrimSpace(q.Get("sort_dir")),
		"limit":                   limit,
		"offset":                  offset,
		"total":                   totalCalls,
		"next_offset":             offset + limit,
		"services":                services,
		"models":                  models,
		"operations":              operations,
		"span_names":              spanNames,
		"selected_services":       selectedServices,
		"selected_models":         selectedModels,
		"selected_operations":     selectedOperations,
		"selected_row_types":      selectedRowTypes,
		"selected_span_names":     selectedSpanNames,
		"ai_items":                []any{},
		"trace_groups":            []any{},
		"total_calls":             totalCalls,
		"total_tokens_in":         totalTokensIn,
		"total_tokens_out":        totalTokensOut,
		"total_errors":            totalErrors,
		"error_msg":               errorMsg,
		"ai_pricing_json":         pricing,
		"ai_pricing_sources_json": pricingSources,
	}
	s.renderTemplate(w, "ai.html", ctx)
}
func (s *Server) aiHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/ai/help" {
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
	s.renderTemplate(w, "ai_help.html", pongo2.Context{"title": "AI Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "ai/help"}})
}
func (s *Server) reportsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/reports" {
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

	reportItems := s.reportService.List()
	reports := make([]map[string]any, 0, len(reportItems))
	for _, item := range reportItems {
		reports = append(reports, map[string]any{
			"id":          item.ID,
			"name":        item.Name,
			"description": item.Description,
			"page_type":   item.PageType,
			"filters":     item.Filters,
		})
	}

	ctx := pongo2.Context{
		"title":                 "Reports",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "reports"},
		"reports":               reports,
	}
	s.renderTemplate(w, "reports.html", ctx)
}
func (s *Server) reportsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/reports/help" {
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
	s.renderTemplate(w, "reports_help.html", pongo2.Context{"title": "Reports Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "reports/help"}})
}
func (s *Server) settingsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings" {
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

	maskingRules := s.maskingService.ListRules()
	maskingKeys := toStringSliceAny(maskingRules["keys"])
	maskingPatterns := toStringSliceAny(maskingRules["patterns"])
	aiSettings := s.settingsService.AI()
	dmSettings := s.dataManagementService.GetSettings()
	k8sSettings := s.kubernetesService.GetSettings()

	ctx := pongo2.Context{
		"title":                        "Settings",
		"mobile_breakpoint_max":        "575.98px",
		"request":                      map[string]any{"endpoint": "settings"},
		"tag_rule_count":               len(s.tagService.ListRules()),
		"anomaly_rule_count":           len(s.metricsService.ListRules()),
		"ai_configured":                isAIConfigured(aiSettings),
		"agent_rule_count":             len(s.agentService.ListRules()),
		"notification_channel_count":   len(s.notificationService.ListSubscriptions()),
		"notification_rule_count":      len(s.notificationService.ListRules()),
		"masking_custom_key_count":     len(maskingKeys),
		"masking_custom_pattern_count": len(maskingPatterns),
		"kubernetes_view_enabled":      k8sSettings.Enabled,
		"backup_enabled":               dmSettings.BackupEnabled,
		"query_allowed_tables":         s.listTableNames(r.Context()),
	}
	s.renderTemplate(w, "settings.html", ctx)
}
func (s *Server) settingsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help" {
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
	s.renderTemplate(w, "settings_help.html", pongo2.Context{"title": "Settings Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help"}})
}
func (s *Server) settingsAIHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/ai" {
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
	s.renderTemplate(w, "settings_ai_help.html", pongo2.Context{"title": "Settings AI Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/ai"}})
}
func (s *Server) settingsAgentsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/agents" {
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
	s.renderTemplate(w, "settings_agents_help.html", pongo2.Context{"title": "Settings Agents Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/agents"}})
}
func (s *Server) settingsDataManagementHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/data-management" {
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
	s.renderTemplate(w, "data_management_help.html", pongo2.Context{"title": "Data Management Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/data-management"}})
}
func (s *Server) settingsEnrichmentHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/enrichment" {
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
	s.renderTemplate(w, "settings_enrichment_help.html", pongo2.Context{"title": "Settings Enrichment Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/enrichment"}})
}
func (s *Server) settingsKubernetesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/kubernetes" {
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
	s.renderTemplate(w, "kubernetes_help.html", pongo2.Context{"title": "Kubernetes Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/kubernetes"}})
}
func (s *Server) settingsMaskingHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/masking" {
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
	s.renderTemplate(w, "masking_help.html", pongo2.Context{"title": "Masking Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/masking"}})
}
func (s *Server) settingsNotificationsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/notifications" {
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
	s.renderTemplate(w, "settings_notifications_help.html", pongo2.Context{"title": "Notifications Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/notifications"}})
}
func (s *Server) settingsRepositoriesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/repositories" {
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
	s.renderTemplate(w, "settings_repositories_help.html", pongo2.Context{"title": "Repositories Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/repositories"}})
}
func (s *Server) settingsTagsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/help/tags" {
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
	s.renderTemplate(w, "settings_tags_help.html", pongo2.Context{"title": "Tags Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "settings/help/tags"}})
}
func (s *Server) settingsNotificationsPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/settings/notifications" {
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

	subs := s.notificationService.ListSubscriptions()
	channels := make([]map[string]any, 0, len(subs))
	for _, sub := range subs {
		name := strings.TrimSpace(sub.Endpoint)
		if name == "" {
			name = "browser subscription"
		}
		channels = append(channels, map[string]any{
			"id":           sub.ID,
			"name":         name,
			"channel_type": "browser_push",
			"enabled":      sub.Enabled,
			"config": map[string]any{
				"endpoint":            sub.Endpoint,
				"mask_output_enabled": "1",
			},
		})
	}

	ruleItems := s.notificationService.ListRules()
	rules := make([]map[string]any, 0, len(ruleItems))
	for _, rule := range ruleItems {
		rules = append(rules, map[string]any{
			"id":               rule.ID,
			"name":             rule.Name,
			"enabled":          rule.Enabled,
			"logic_operator":   "any",
			"conditions":       []map[string]any{},
			"channel_ids":      []string{},
			"severity":         "warning",
			"cooldown_seconds": 300,
		})
	}

	vapidPublicKey := s.notificationService.VAPIDPublicKey()
	vapidKeySource := ""
	if strings.TrimSpace(os.Getenv("SOBS_VAPID_PRIVATE_KEY")) != "" {
		vapidKeySource = "env"
	} else if strings.TrimSpace(vapidPublicKey) != "" {
		vapidKeySource = "db"
	}

	ctx := pongo2.Context{
		"title":                 "Settings Notifications",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "settings/notifications"},
		"channel_types":         []string{"webhook", "slack", "email", "browser_push"},
		"channels":              channels,
		"rules":                 rules,
		"metric_rules":          s.metricsService.ListRules(),
		"notification_log":      []map[string]any{},
		"condition_types":       []string{"signal", "tag"},
		"signal_sources":        []string{"logs", "errors", "traces", "metrics", "rum"},
		"comparators":           []string{">", ">=", "<", "<=", "==", "!="},
		"tag_match_operators":   []string{"equals", "contains", "starts_with", "ends_with", "regex"},
		"tag_record_types":      []string{"all", "logs", "errors", "traces", "metrics", "rum"},
		"edit_rule":             nil,
		"vapid_public_key":      vapidPublicKey,
		"vapid_key_source":      vapidKeySource,
	}
	s.renderTemplate(w, "settings_notifications.html", ctx)
}
func (s *Server) queryPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/query" {
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

	tables := s.listTableNames(r.Context())
	defaultSQL := suggestSQLForQuestion("show recent errors", tables)

	ctx := pongo2.Context{
		"title":                 "Query",
		"mobile_breakpoint_max": "575.98px",
		"request":               map[string]any{"endpoint": "query"},
		"tables":                tables,
		"default_sql":           defaultSQL,
		"question":              "",
		"error_msg":             "",
	}
	s.renderTemplate(w, "query.html", ctx)
}
func (s *Server) queryHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/query/help" {
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
	s.renderTemplate(w, "query_help.html", pongo2.Context{"title": "Query Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "query/help"}})
}
func (s *Server) metricsHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help" {
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
	s.renderTemplate(w, "metrics_help.html", pongo2.Context{"title": "Metrics Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help"}})
}
func (s *Server) metricsRulesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help/rules" {
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
	s.renderTemplate(w, "metrics_rules_help.html", pongo2.Context{"title": "Metrics Rules Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help/rules"}})
}
func (s *Server) metricsRulesAutoHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help/rules/auto" {
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
	s.renderTemplate(w, "auto_metrics_rules_help.html", pongo2.Context{"title": "Auto Metrics Rules Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help/rules/auto"}})
}
func (s *Server) metricsAnomalyHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/metrics/help/anomaly" {
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
	s.renderTemplate(w, "metrics_anomaly_help.html", pongo2.Context{"title": "Metrics Anomaly Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "metrics/help/anomaly"}})
}
func (s *Server) setupPlaybooksHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/setup/help/playbooks" {
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
	s.renderTemplate(w, "setup_playbooks_help.html", pongo2.Context{"title": "Setup Playbooks Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "setup/help/playbooks"}})
}
func (s *Server) chartEditorHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/dashboards/help/chart-editor" {
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
	s.renderTemplate(w, "chart_editor_help.html", pongo2.Context{"title": "Chart Editor Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "dashboards/help/chart-editor"}})
}
func (s *Server) kubernetesHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/kubernetes/help" {
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
	s.renderTemplate(w, "kubernetes_help.html", pongo2.Context{"title": "Kubernetes Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "kubernetes/help"}})
}
func (s *Server) cveHelpPage(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/cve/help" {
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
	s.renderTemplate(w, "cve_help.html", pongo2.Context{"title": "CVE Help", "mobile_breakpoint_max": "575.98px", "request": map[string]any{"endpoint": "cve/help"}})
}

func toStringSliceAny(value any) []string {
	items, ok := value.([]string)
	if ok {
		return items
	}
	raw, ok := value.([]any)
	if !ok {
		return []string{}
	}
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		text, ok := item.(string)
		if ok {
			out = append(out, text)
		}
	}
	return out
}

func isAIConfigured(values map[string]string) bool {
	if len(values) == 0 {
		return false
	}
	for _, key := range []string{"api_key", "base_url", "model", "endpoint"} {
		if strings.TrimSpace(values[key]) != "" {
			return true
		}
	}
	return false
}
