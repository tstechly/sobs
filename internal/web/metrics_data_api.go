package web

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/extensionpoints"
)

func (s *Server) metricsPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path != "/metrics" {
		http.NotFound(w, r)
		return
	}
	if s.renderer == nil || s.renderErr != nil {
		http.Error(w, "template error", http.StatusInternalServerError)
		return
	}

	selectedServices := trimList(r.URL.Query()["service"])
	selectedSignals := trimList(r.URL.Query()["signal"])
	selectedSources := trimList(r.URL.Query()["source"])
	service := firstOrEmpty(selectedServices)
	signal := firstOrEmpty(selectedSignals)
	source := firstOrEmpty(selectedSources)
	attrFP := strings.TrimSpace(r.URL.Query().Get("attr_fp"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	limit := parseLimitParam(r, 100, 1, 1000)
	offset := parseOffsetParam(r)

	hours := 24
	if raw := strings.TrimSpace(r.URL.Query().Get("hours")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			if parsed < 1 {
				hours = 1
			} else if parsed > 168 {
				hours = 168
			} else {
				hours = parsed
			}
		}
	}

	sortBy := strings.TrimSpace(r.URL.Query().Get("sort_by"))
	sortDir := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("sort_dir")))
	if sortDir != "asc" {
		sortDir = "desc"
	}
	sortMap := map[string]string{
		"last_time":          "last_time",
		"service":            "service",
		"source":             "source",
		"signal":             "signal",
		"last_value":         "last_value",
		"last_anomaly_score": "last_anomaly_score",
		"last_anomaly_state": "last_anomaly_state",
		"last_sample_count":  "last_sample_count",
		"point_count":        "point_count",
	}
	sortCol := "last_time"
	if mapped, ok := sortMap[sortBy]; ok {
		sortCol = mapped
	} else {
		sortBy = "last_time"
	}
	orderClause := fmt.Sprintf("ORDER BY %s %s", sortCol, strings.ToUpper(sortDir))

	rows := []map[string]any{}
	total := 0
	errorMsg := ""

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		errorMsg = err.Error()
		s.renderTemplate(w, "metrics.html", metricsPageContext(rows, total, limit, offset, service, selectedServices, signal, selectedSignals, source, selectedSources, attrFP, q, fromTS, toTS, hours, sortBy, sortDir, nil, nil, nil, errorMsg))
		return
	}
	defer store.Close()

	services, signals, sources := listDerivedSignalDimensions(r, store)

	whereClause, params := buildMetricsWhereClause(selectedServices, selectedSignals, selectedSources, attrFP, q, fromTS, toTS, hours)

	groupedSQL := "SELECT " +
		"ServiceName AS service, " +
		"SignalSource AS source, " +
		"SignalName AS signal, " +
		"AttrFingerprint AS attr_fp, " +
		"max(time) AS last_time, " +
		"argMax(value, time) AS last_value, " +
		"argMax(anomaly_score, time) AS last_anomaly_score, " +
		"argMax(anomaly_state, time) AS last_anomaly_state, " +
		"argMax(SampleCount, time) AS last_sample_count, " +
		"count() AS point_count " +
		"FROM v_derived_signals_anomaly " + whereClause + " " +
		"GROUP BY ServiceName, SignalSource, SignalName, AttrFingerprint"

	countSQL := "SELECT count() FROM (" + groupedSQL + ")"
	countRows, err := store.Query(r.Context(), countSQL, params...)
	if err != nil {
		if !isMissingTableError(err) {
			errorMsg = err.Error()
		}
		s.renderTemplate(w, "metrics.html", metricsPageContext(rows, total, limit, offset, service, selectedServices, signal, selectedSignals, source, selectedSources, attrFP, q, fromTS, toTS, hours, sortBy, sortDir, services, signals, sources, errorMsg))
		return
	}
	defer countRows.Close()
	if countRows.Next() {
		var c any
		if scanErr := countRows.Scan(&c); scanErr == nil {
			total = anyToInt(c)
		}
	}

	querySQL := "SELECT * FROM (" + groupedSQL + ") " + orderClause + " LIMIT ? OFFSET ?"
	queryArgs := append(append([]any{}, params...), limit, offset)
	resultRows, err := store.Query(r.Context(), querySQL, queryArgs...)
	if err != nil {
		if !isMissingTableError(err) {
			errorMsg = err.Error()
		}
		s.renderTemplate(w, "metrics.html", metricsPageContext(rows, total, limit, offset, service, selectedServices, signal, selectedSignals, source, selectedSources, attrFP, q, fromTS, toTS, hours, sortBy, sortDir, services, signals, sources, errorMsg))
		return
	}
	defer resultRows.Close()

	for resultRows.Next() {
		var svc, src, sig, fp, lastTime, lastState any
		var lastValue, lastScore, lastSample, points any
		if scanErr := resultRows.Scan(&svc, &src, &sig, &fp, &lastTime, &lastValue, &lastScore, &lastState, &lastSample, &points); scanErr != nil {
			continue
		}
		rows = append(rows, map[string]any{
			"service":            anyToString(svc),
			"source":             anyToString(src),
			"signal":             anyToString(sig),
			"attr_fp":            anyToString(fp),
			"last_time":          anyToString(lastTime),
			"last_value":         lastValue,
			"last_anomaly_score": lastScore,
			"last_anomaly_state": anyToString(lastState),
			"last_sample_count":  anyToInt(lastSample),
			"point_count":        anyToInt(points),
			"rule_name":          "",
			"rule_state":         "normal",
			"rule_reason":        "",
			"rule_seasonal":      false,
			"effective_state":    anyToString(lastState),
		})
	}
	annotateMetricRowsWithRules(r.Context(), store, rows)

	s.renderTemplate(w, "metrics.html", metricsPageContext(rows, total, limit, offset, service, selectedServices, signal, selectedSignals, source, selectedSources, attrFP, q, fromTS, toTS, hours, sortBy, sortDir, services, signals, sources, errorMsg))
}

func metricsPageContext(
	rows []map[string]any,
	total int,
	limit int,
	offset int,
	service string,
	selectedServices []string,
	signal string,
	selectedSignals []string,
	source string,
	selectedSources []string,
	attrFP string,
	q string,
	fromTS string,
	toTS string,
	hours int,
	sortBy string,
	sortDir string,
	services []string,
	signals []string,
	sources []string,
	errorMsg string,
) renderContext {
	if services == nil {
		services = []string{}
	}
	if signals == nil {
		signals = []string{}
	}
	if sources == nil {
		sources = []string{}
	}
	return renderContext{
		"title":             "Metrics",
		"rows":              rows,
		"total":             total,
		"limit":             limit,
		"offset":            offset,
		"service":           service,
		"selected_services": selectedServices,
		"signal":            signal,
		"selected_signals":  selectedSignals,
		"source":            source,
		"selected_sources":  selectedSources,
		"attr_fp":           attrFP,
		"q":                 q,
		"from_ts":           fromTS,
		"to_ts":             toTS,
		"hours":             hours,
		"sort_by":           sortBy,
		"sort_dir":          sortDir,
		"services":          services,
		"signals":           signals,
		"sources":           sources,
		"error_msg":         errorMsg,
		"request":           map[string]any{"endpoint": "metrics"},
	}
}

func listDerivedSignalDimensions(r *http.Request, store extensionpoints.ClickHouseStore) ([]string, []string, []string) {
	services := []string{}
	serviceRows, err := store.Query(r.Context(), "SELECT DISTINCT ServiceName FROM v_derived_signals_anomaly WHERE ServiceName != '' ORDER BY ServiceName")
	if err == nil {
		defer serviceRows.Close()
		for serviceRows.Next() {
			var value any
			if scanErr := serviceRows.Scan(&value); scanErr == nil {
				t := anyToString(value)
				if t != "" {
					services = append(services, t)
				}
			}
		}
	}

	signals := []string{}
	signalRows, err := store.Query(r.Context(), "SELECT DISTINCT SignalName FROM v_derived_signals_anomaly WHERE SignalName != '' ORDER BY SignalName")
	if err == nil {
		defer signalRows.Close()
		for signalRows.Next() {
			var value any
			if scanErr := signalRows.Scan(&value); scanErr == nil {
				t := anyToString(value)
				if t != "" {
					signals = append(signals, t)
				}
			}
		}
	}

	sources := []string{}
	sourceRows, err := store.Query(r.Context(), "SELECT DISTINCT SignalSource FROM v_derived_signals_anomaly WHERE SignalSource != '' ORDER BY SignalSource")
	if err == nil {
		defer sourceRows.Close()
		for sourceRows.Next() {
			var value any
			if scanErr := sourceRows.Scan(&value); scanErr == nil {
				t := anyToString(value)
				if t != "" {
					sources = append(sources, t)
				}
			}
		}
	}

	return services, signals, sources
}

func buildMetricsWhereClause(selectedServices, selectedSignals, selectedSources []string, attrFP, q, fromTS, toTS string, hours int) (string, []any) {
	parts := []string{}
	params := []any{}

	appendInClause := func(column string, values []string) {
		vals := trimList(values)
		if len(vals) == 0 {
			return
		}
		placeholders := make([]string, 0, len(vals))
		for _, v := range vals {
			placeholders = append(placeholders, "?")
			params = append(params, v)
		}
		parts = append(parts, column+" IN ("+strings.Join(placeholders, ",")+")")
	}

	appendInClause("ServiceName", selectedServices)
	appendInClause("SignalName", selectedSignals)
	appendInClause("SignalSource", selectedSources)

	if attrFP != "" {
		parts = append(parts, "AttrFingerprint = ?")
		params = append(params, attrFP)
	}
	if fromTS != "" {
		parts = append(parts, "time >= ?")
		params = append(params, fromTS)
	}
	if toTS != "" {
		parts = append(parts, "time <= ?")
		params = append(params, toTS)
	}
	if q != "" {
		parts = append(parts, "SignalName ILIKE ?")
		params = append(params, "%"+q+"%")
	}
	if fromTS == "" && toTS == "" {
		parts = append(parts, fmt.Sprintf("time >= now() - INTERVAL %d HOUR", hours))
	}

	if len(parts) == 0 {
		return "", params
	}
	return "WHERE " + strings.Join(parts, " AND "), params
}

func trimList(values []string) []string {
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func firstOrEmpty(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

type metricRuleEvaluation struct {
	RuleID       string
	RuleName     string
	RuleState    string
	RuleReason   string
	RuleSeasonal bool
}

func annotateMetricRowsWithRules(ctx context.Context, store extensionpoints.ClickHouseStore, rows []map[string]any) {
	if len(rows) == 0 {
		return
	}
	rules := summaryLoadAnomalyRules(ctx, store)
	if len(rules) == 0 {
		for _, row := range rows {
			row["rule_name"] = ""
			row["rule_state"] = "normal"
			row["rule_reason"] = ""
			row["rule_seasonal"] = false
			row["effective_state"] = summaryCombineStates(anyToString(row["last_anomaly_state"]), anyToString(row["rule_state"]))
		}
		return
	}
	latestLookup, timedLookup := buildMetricRuleLookups(rows)
	for _, row := range rows {
		row["rule_name"] = ""
		row["rule_state"] = "normal"
		row["rule_reason"] = ""
		row["rule_seasonal"] = false
		if anyToString(row["effective_state"]) == "" {
			row["effective_state"] = anyToString(row["last_anomaly_state"])
		}

		bestSeverity := -1
		bestTypeRank := -1
		bestName := ""
		var bestMatch *metricRuleEvaluation

		rowSource := anyToString(row["source"])
		rowSignal := anyToString(row["signal"])
		rowService := anyToString(row["service"])
		rowAttrFP := anyToString(row["attr_fp"])

		for _, rule := range rules {
			if !summaryRuleMatchesSeries(rule, rowSource, rowSignal, rowService, rowAttrFP) {
				continue
			}
			var evaluation *metricRuleEvaluation
			switch rule.RuleType {
			case "composite":
				evaluation = evaluateMetricCompositeRule(ctx, store, rule, row, latestLookup, timedLookup)
			case "seasonal":
				evaluation = evaluateMetricSeasonalRule(rule, row["last_value"], row["last_sample_count"], anyToString(row["last_time"]))
			default:
				evaluation = evaluateMetricThresholdRule(rule, row["last_value"], row["last_sample_count"])
			}
			if evaluation == nil {
				continue
			}
			severity := summarySeverityRanks[evaluation.RuleState]
			typeRank := summaryRuleTypeRank(rule.RuleType)
			if severity > bestSeverity || (severity == bestSeverity && (typeRank > bestTypeRank || (typeRank == bestTypeRank && evaluation.RuleName > bestName))) {
				bestMatch = evaluation
				bestSeverity = severity
				bestTypeRank = typeRank
				bestName = evaluation.RuleName
			}
		}

		if bestMatch != nil {
			row["rule_id"] = bestMatch.RuleID
			row["rule_name"] = bestMatch.RuleName
			row["rule_state"] = bestMatch.RuleState
			row["rule_reason"] = bestMatch.RuleReason
			row["rule_seasonal"] = bestMatch.RuleSeasonal
		}
		row["effective_state"] = summaryCombineStates(anyToString(row["last_anomaly_state"]), anyToString(row["rule_state"]))
	}
}

func buildMetricRuleLookups(rows []map[string]any) (map[string]map[string]any, map[string]map[string]any) {
	latestLookup := make(map[string]map[string]any, len(rows))
	timedLookup := make(map[string]map[string]any, len(rows))
	for _, row := range rows {
		baseKey := metricRuleLookupKey(anyToString(row["service"]), anyToString(row["attr_fp"]), anyToString(row["source"]), anyToString(row["signal"]))
		latestLookup[baseKey] = row
		timedLookup[baseKey+"\x00"+anyToString(row["last_time"])] = row
	}
	return latestLookup, timedLookup
}

func metricRuleLookupKey(service, attrFP, source, signal string) string {
	return service + "\x00" + attrFP + "\x00" + source + "\x00" + signal
}

func evaluateMetricThresholdRule(rule summaryAnomalyRule, value any, sampleCount any) *metricRuleEvaluation {
	state, reason, ok := evaluateMetricThresholdCondition(rule.Name, rule.Comparator, rule.WarningThreshold, rule.CriticalThreshold, value, sampleCount, rule.MinSampleCount)
	if !ok {
		return nil
	}
	return &metricRuleEvaluation{
		RuleID:     rule.ID,
		RuleName:   rule.Name,
		RuleState:  state,
		RuleReason: reason,
		RuleSeasonal: false,
	}
}

func evaluateMetricSeasonalRule(rule summaryAnomalyRule, value any, sampleCount any, timeValue string) *metricRuleEvaluation {
	warningThreshold := rule.WarningThreshold
	criticalThreshold := rule.CriticalThreshold
	ruleSeasonal := false
	buckets := struct {
		Strategy string `json:"strategy"`
		Buckets  map[string]struct {
			Warning  float64 `json:"warning"`
			Critical float64 `json:"critical"`
		} `json:"buckets"`
	}{}
	if strings.TrimSpace(rule.SeasonalBucketsJSON) != "" && json.Unmarshal([]byte(rule.SeasonalBucketsJSON), &buckets) == nil && len(buckets.Buckets) > 0 {
		parsed := parseTimestampTime(strings.ReplaceAll(strings.TrimSpace(timeValue), "T", " "))
		if !parsed.IsZero() {
			bucketKey := strconv.Itoa(parsed.Hour())
			if strings.TrimSpace(buckets.Strategy) == "day_of_week" {
				day := int(parsed.Weekday())
				if day == 0 {
					day = 7
				}
				bucketKey = strconv.Itoa(day)
			}
			if bucket, ok := buckets.Buckets[bucketKey]; ok {
				warningThreshold = bucket.Warning
				criticalThreshold = bucket.Critical
				ruleSeasonal = true
			}
		}
	}
	state, reason, ok := evaluateMetricThresholdCondition(rule.Name, rule.Comparator, warningThreshold, criticalThreshold, value, sampleCount, rule.MinSampleCount)
	if !ok {
		return nil
	}
	return &metricRuleEvaluation{
		RuleID:       rule.ID,
		RuleName:     rule.Name,
		RuleState:    state,
		RuleReason:   reason,
		RuleSeasonal: ruleSeasonal,
	}
}

func evaluateMetricCompositeRule(ctx context.Context, store extensionpoints.ClickHouseStore, rule summaryAnomalyRule, row map[string]any, latestLookup map[string]map[string]any, timedLookup map[string]map[string]any) *metricRuleEvaluation {
	primaryState, _, ok := evaluateMetricThresholdCondition(rule.Name+" primary", rule.Comparator, rule.WarningThreshold, rule.CriticalThreshold, row["last_value"], row["last_sample_count"], rule.MinSampleCount)
	if !ok || rule.SecondarySource == "" || rule.SecondarySignal == "" {
		return nil
	}
	service := anyToString(row["service"])
	attrFP := anyToString(row["attr_fp"])
	timeValue := anyToString(row["last_time"])
	baseKey := metricRuleLookupKey(service, attrFP, rule.SecondarySource, rule.SecondarySignal)
	secondaryRow := timedLookup[baseKey+"\x00"+timeValue]
	if secondaryRow == nil {
		secondaryRow = latestLookup[baseKey]
	}
	if secondaryRow == nil {
		secondaryRow = lookupMetricSecondaryRuleRow(ctx, store, service, attrFP, rule.SecondarySource, rule.SecondarySignal, timeValue)
	}
	if secondaryRow == nil {
		return nil
	}
	secondaryState, _, ok := evaluateMetricThresholdCondition(rule.Name+" secondary", rule.SecondaryComparator, rule.SecondaryWarningThreshold, rule.SecondaryCriticalThreshold, secondaryRow["last_value"], secondaryRow["last_sample_count"], rule.MinSampleCount)
	if !ok {
		return nil
	}
	secondaryValue := secondaryRow["last_value"]
	return &metricRuleEvaluation{
		RuleID:     rule.ID,
		RuleName:   rule.Name,
		RuleState:  summaryCombineStates(primaryState, secondaryState),
		RuleReason: fmt.Sprintf("%s: primary %s=%s and secondary %s=%s triggered", rule.Name, anyToString(row["signal"]), formatMetricRuleValue(row["last_value"]), rule.SecondarySignal, formatMetricRuleValue(secondaryValue)),
	}
}

func lookupMetricSecondaryRuleRow(ctx context.Context, store extensionpoints.ClickHouseStore, service, attrFP, source, signal, timeValue string) map[string]any {
	if store == nil {
		return nil
	}
	queryByTime := "SELECT time, value, SampleCount FROM v_derived_signals_anomaly WHERE ServiceName = ? AND SignalSource = ? AND SignalName = ? AND AttrFingerprint = ? AND time = ? ORDER BY time DESC LIMIT 1"
	queryLatest := "SELECT time, value, SampleCount FROM v_derived_signals_anomaly WHERE ServiceName = ? AND SignalSource = ? AND SignalName = ? AND AttrFingerprint = ? ORDER BY time DESC LIMIT 1"
	if strings.TrimSpace(timeValue) != "" {
		if row := queryMetricSecondaryRuleRow(ctx, store, queryByTime, service, source, signal, attrFP, timeValue); row != nil {
			return row
		}
	}
	return queryMetricSecondaryRuleRow(ctx, store, queryLatest, service, source, signal, attrFP)
}

func queryMetricSecondaryRuleRow(ctx context.Context, store extensionpoints.ClickHouseStore, query string, args ...any) map[string]any {
	rows, err := store.Query(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()
	if !rows.Next() {
		return nil
	}
	var timeValue, value, sampleCount any
	if err := rows.Scan(&timeValue, &value, &sampleCount); err != nil {
		return nil
	}
	return map[string]any{
		"last_time":         anyToString(timeValue),
		"last_value":        value,
		"last_sample_count": sampleCount,
	}
}

func evaluateMetricThresholdCondition(name, comparator string, warningThreshold, criticalThreshold float64, value any, sampleCount any, minSampleCount int) (string, string, bool) {
	valueNum := anyToFloat(value)
	sampleCountNum := anyToInt(sampleCount)
	if sampleCountNum < maxInt(minSampleCount, 1) {
		return "", "", false
	}
	state := "normal"
	triggeredThreshold := 0.0
	operator := ">="
	switch comparator {
	case "lt":
		operator = "<="
		if valueNum <= criticalThreshold {
			state = "outlier"
			triggeredThreshold = criticalThreshold
		} else if valueNum <= warningThreshold {
			state = "warning"
			triggeredThreshold = warningThreshold
		}
	default:
		if valueNum >= criticalThreshold {
			state = "outlier"
			triggeredThreshold = criticalThreshold
		} else if valueNum >= warningThreshold {
			state = "warning"
			triggeredThreshold = warningThreshold
		}
	}
	if state == "normal" {
		return "", "", false
	}
	return state, fmt.Sprintf("%s: value %s %s %s", name, formatMetricRuleValue(valueNum), operator, formatMetricRuleValue(triggeredThreshold)), true
}

func formatMetricRuleValue(value any) string {
	return strconv.FormatFloat(roundFloat(anyToFloat(value), 4), 'f', -1, 64)
}
