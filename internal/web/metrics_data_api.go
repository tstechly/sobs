package web

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/flosch/pongo2/v6"
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
			"rule_state":         "",
			"rule_reason":        "",
		})
	}

	s.renderTemplate(w, "metrics.html", metricsPageContext(rows, total, limit, offset, service, selectedServices, signal, selectedSignals, source, selectedSources, attrFP, q, fromTS, toTS, hours, sortBy, sortDir, services, signals, sources, errorMsg))
}

func (s *Server) apiMetricsSummary(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	selectedServices := trimList(r.URL.Query()["service"])
	selectedSignals := trimList(r.URL.Query()["signal"])
	selectedSources := trimList(r.URL.Query()["source"])
	attrFP := strings.TrimSpace(r.URL.Query().Get("attr_fp"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
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

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "total_series": 0, "outlier_series": 0, "warning_series": 0})
		return
	}
	defer store.Close()

	whereClause, params := buildMetricsWhereClause(selectedServices, selectedSignals, selectedSources, attrFP, q, fromTS, toTS, hours)
	sql := "SELECT " +
		"count() AS total_series, " +
		"countIf(last_anomaly_state = 'outlier') AS outlier_series, " +
		"countIf(last_anomaly_state = 'warning') AS warning_series " +
		"FROM (" +
		"SELECT argMax(anomaly_state, time) AS last_anomaly_state " +
		"FROM v_derived_signals_anomaly " + whereClause + " " +
		"GROUP BY ServiceName, SignalSource, SignalName, AttrFingerprint" +
		")"

	rows, err := store.Query(r.Context(), sql, params...)
	if err != nil {
		if isMissingTableError(err) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "total_series": 0, "outlier_series": 0, "warning_series": 0})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	total := 0
	outliers := 0
	warnings := 0
	if rows.Next() {
		var totalAny, outAny, warnAny any
		if scanErr := rows.Scan(&totalAny, &outAny, &warnAny); scanErr == nil {
			total = anyToInt(totalAny)
			outliers = anyToInt(outAny)
			warnings = anyToInt(warnAny)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":             true,
		"total_series":   total,
		"outlier_series": outliers,
		"warning_series": warnings,
	})
}

func (s *Server) apiMetricsTimeseries(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	selectedServices := trimList(r.URL.Query()["service"])
	selectedSignals := trimList(r.URL.Query()["signal"])
	selectedSources := trimList(r.URL.Query()["source"])
	attrFP := strings.TrimSpace(r.URL.Query().Get("attr_fp"))
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	fromTS := strings.TrimSpace(r.URL.Query().Get("from_ts"))
	toTS := strings.TrimSpace(r.URL.Query().Get("to_ts"))
	limit := parseLimitParam(r, 500, 1, 5000)
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

	store, err := s.storeFactory.Open(r.Context())
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "rows": []any{}})
		return
	}
	defer store.Close()

	whereClause, params := buildMetricsWhereClause(selectedServices, selectedSignals, selectedSources, attrFP, q, fromTS, toTS, hours)
	sql := "SELECT time, ServiceName, SignalSource, SignalName, AttrFingerprint, value, SampleCount, baseline_mean, baseline_lower, baseline_upper, anomaly_score, anomaly_state " +
		"FROM v_derived_signals_anomaly " + whereClause + " ORDER BY time ASC LIMIT ?"
	args := append(params, limit)

	rows, err := store.Query(r.Context(), sql, args...)
	if err != nil {
		if isMissingTableError(err) {
			writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rows": []any{}})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	defer rows.Close()

	out := []map[string]any{}
	for rows.Next() {
		var ts, svc, src, sig, fp, valueAny, sampleAny, meanAny, lowAny, highAny, scoreAny, stateAny any
		if scanErr := rows.Scan(&ts, &svc, &src, &sig, &fp, &valueAny, &sampleAny, &meanAny, &lowAny, &highAny, &scoreAny, &stateAny); scanErr != nil {
			continue
		}
		out = append(out, map[string]any{
			"time":           anyToString(ts),
			"service":        anyToString(svc),
			"source":         anyToString(src),
			"signal":         anyToString(sig),
			"attr_fp":        anyToString(fp),
			"value":          valueAny,
			"sample_count":   anyToInt(sampleAny),
			"baseline_mean":  meanAny,
			"baseline_lower": lowAny,
			"baseline_upper": highAny,
			"anomaly_score":  scoreAny,
			"anomaly_state":  anyToString(stateAny),
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "rows": out, "limit": limit})
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
) pongo2.Context {
	if services == nil {
		services = []string{}
	}
	if signals == nil {
		signals = []string{}
	}
	if sources == nil {
		sources = []string{}
	}
	return pongo2.Context{
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
