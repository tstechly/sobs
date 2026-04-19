package mcp

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
)

const (
	apiKeyMax          = 20
	rateLimitRequests  = 60
	rateLimitWindowSec = 60
)

type Key struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Hash      string `json:"-"`
}

type Service struct {
	mu         sync.Mutex
	enabled    bool
	keys       []Key
	toolSpecs  []map[string]any
	rateWindow map[string][]time.Time
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return &Service{
		enabled: true,
		toolSpecs: []map[string]any{
			toolSpec("list_services", "List all distinct service names that have sent telemetry to SOBS."),
			toolSpec("query_otel_logs", "Query the otel_logs table with simple filters."),
			toolSpec("query_otel_traces", "Query the otel_traces table with simple filters."),
			toolSpec("query_metrics", "Query aggregated metrics data."),
			toolSpec("query_metrics_raw", "Query raw metrics samples."),
			toolSpec("get_metric_names", "List the metric names available to SOBS."),
			toolSpec("get_anomaly_rules", "List configured anomaly detection rules."),
			toolSpec("get_recent_errors", "List recent error events and spans."),
		},
		keys:       []Key{},
		rateWindow: map[string][]time.Time{},
	}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	svc := NewService()
	svc.storeFactory = factory
	return svc
}

func toolSpec(name string, description string) map[string]any {
	return map[string]any{
		"name":        name,
		"description": description,
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}, "required": []string{}},
	}
}

func (s *Service) Tools() []map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	tools := make([]map[string]any, len(s.toolSpecs))
	copy(tools, s.toolSpecs)
	return tools
}

func (s *Service) AllowRequest(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	cutoff := now.Add(-rateLimitWindowSec * time.Second)
	windows := s.rateWindow[ip]
	kept := windows[:0]
	for _, ts := range windows {
		if !ts.Before(cutoff) {
			kept = append(kept, ts)
		}
	}
	if len(kept) >= rateLimitRequests {
		s.rateWindow[ip] = kept
		return false
	}
	kept = append(kept, now)
	s.rateWindow[ip] = kept
	return true
}

func (s *Service) Enabled() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.enabled
}

func (s *Service) SetEnabled(enabled bool) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.enabled = enabled
	return s.enabled
}

func (s *Service) ListKeys() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Key, len(s.keys))
	for i, key := range s.keys {
		out[i] = safeKey(key)
	}
	return out
}

func (s *Service) ListKeysWithHash() []Key {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Key, len(s.keys))
	copy(out, s.keys)
	return out
}

func (s *Service) ReplaceKeys(keys []Key) {
	s.mu.Lock()
	defer s.mu.Unlock()
	next := make([]Key, len(keys))
	copy(next, keys)
	s.keys = next
}

func (s *Service) CreateKey(label string, expiresAt string) (Key, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.keys) >= apiKeyMax {
		return Key{}, "", fmt.Errorf("maximum of %d keys reached", apiKeyMax)
	}
	label = strings.TrimSpace(label)
	if label == "" {
		label = "API Key"
	}
	label = truncate(label, 128)
	rawKey := "smcp_" + randomHex(24)
	key := Key{
		ID:        randomHex(8),
		Label:     label,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
		ExpiresAt: strings.TrimSpace(expiresAt),
		Hash:      hashKey(rawKey),
	}
	s.keys = append(s.keys, key)
	return safeKey(key), rawKey, nil
}

func (s *Service) DeleteKey(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i, key := range s.keys {
		if key.ID != id {
			continue
		}
		s.keys = append(s.keys[:i], s.keys[i+1:]...)
		return true
	}
	return false
}

func (s *Service) Authenticate(rawKey string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(rawKey) == "" {
		return false
	}
	hash := hashKey(strings.TrimSpace(rawKey))
	now := time.Now().UTC()
	for _, key := range s.keys {
		if key.Hash != hash {
			continue
		}
		if key.ExpiresAt == "" {
			return true
		}
		expiresAt, err := time.Parse(time.RFC3339, key.ExpiresAt)
		if err != nil || expiresAt.After(now) {
			return true
		}
		return false
	}
	return false
}

func (s *Service) CallTool(name string, args map[string]any) (map[string]any, error) {
	if s.storeFactory != nil {
		return s.callToolStoreBacked(context.Background(), name, args)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	supported := map[string]func(map[string]any) map[string]any{
		"list_services": func(_ map[string]any) map[string]any {
			return map[string]any{"services": []string{}}
		},
		"query_otel_logs": func(args map[string]any) map[string]any {
			return map[string]any{"rows": []map[string]any{}, "filters": cloneMap(args)}
		},
		"query_otel_traces": func(args map[string]any) map[string]any {
			return map[string]any{"rows": []map[string]any{}, "filters": cloneMap(args)}
		},
		"query_metrics": func(args map[string]any) map[string]any {
			return map[string]any{"rows": []map[string]any{}, "filters": cloneMap(args)}
		},
		"query_metrics_raw": func(args map[string]any) map[string]any {
			return map[string]any{"rows": []map[string]any{}, "filters": cloneMap(args)}
		},
		"get_metric_names": func(_ map[string]any) map[string]any {
			return map[string]any{"metric_names": []string{}}
		},
		"get_anomaly_rules": func(_ map[string]any) map[string]any {
			return map[string]any{"rules": []map[string]any{}}
		},
		"get_recent_errors": func(args map[string]any) map[string]any {
			limit := args["limit"]
			return map[string]any{"errors": []map[string]any{}, "limit": limit}
		},
	}
	handler, ok := supported[name]
	if !ok {
		available := make([]string, 0, len(supported))
		for tool := range supported {
			available = append(available, tool)
		}
		sort.Strings(available)
		return nil, fmt.Errorf("unknown tool: %s (available: %v)", name, available)
	}
	return handler(args), nil
}

func (s *Service) callToolStoreBacked(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
	store, err := s.storeFactory.Open(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = store.Close() }()

	switch name {
	case "list_services":
		rows, err := store.Query(ctx,
			"SELECT ServiceName FROM ("+
				"SELECT ServiceName FROM otel_logs WHERE ServiceName != '' UNION DISTINCT "+
				"SELECT ServiceName FROM otel_traces WHERE ServiceName != '' UNION DISTINCT "+
				"SELECT ServiceName FROM otel_metrics WHERE ServiceName != ''"+
			") ORDER BY ServiceName LIMIT 500",
		)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		services := []string{}
		for rows.Next() {
			var service string
			if err := rows.Scan(&service); err != nil {
				return nil, err
			}
			if strings.TrimSpace(service) != "" {
				services = append(services, service)
			}
		}
		return map[string]any{"services": services}, nil

	case "get_metric_names":
		rows, err := store.Query(ctx, "SELECT DISTINCT MetricName FROM otel_metrics WHERE MetricName != '' ORDER BY MetricName LIMIT 1000")
		if err != nil {
			return map[string]any{"metric_names": []string{}}, nil
		}
		defer func() { _ = rows.Close() }()
		names := []string{}
		for rows.Next() {
			var metricName string
			if err := rows.Scan(&metricName); err != nil {
				return nil, err
			}
			names = append(names, metricName)
		}
		return map[string]any{"metric_names": names}, nil

	case "query_otel_logs":
		limit := mcpToolLimit(args, 100, 1, 500)
		service, _ := args["service"].(string)
		severity, _ := args["severity"].(string)
		search, _ := args["search"].(string)
		traceID, _ := args["trace_id"].(string)
		where := []string{"1=1"}
		params := []any{}
		if strings.TrimSpace(service) != "" {
			where = append(where, "ServiceName = ?")
			params = append(params, service)
		}
		if strings.TrimSpace(severity) != "" {
			where = append(where, "SeverityText = ?")
			params = append(params, severity)
		}
		if strings.TrimSpace(search) != "" {
			where = append(where, "positionCaseInsensitive(Body, ?) > 0")
			params = append(params, search)
		}
		if strings.TrimSpace(traceID) != "" {
			where = append(where, "TraceId = ?")
			params = append(params, traceID)
		}
		query := "SELECT Timestamp, ServiceName, SeverityText, Body, TraceId, SpanId FROM otel_logs WHERE " + strings.Join(where, " AND ") + " ORDER BY Timestamp DESC LIMIT ?"
		params = append(params, limit)
		rows, err := store.Query(ctx, query, params...)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		resultRows, err := collectRows(rows, []string{"timestamp", "service", "severity", "body", "trace_id", "span_id"})
		if err != nil {
			return nil, err
		}
		return map[string]any{"rows": resultRows, "limit": limit}, nil

	case "query_otel_traces":
		limit := mcpToolLimit(args, 100, 1, 500)
		service, _ := args["service"].(string)
		traceID, _ := args["trace_id"].(string)
		where := []string{"1=1"}
		params := []any{}
		if strings.TrimSpace(service) != "" {
			where = append(where, "ServiceName = ?")
			params = append(params, service)
		}
		if strings.TrimSpace(traceID) != "" {
			where = append(where, "TraceId = ?")
			params = append(params, traceID)
		}
		query := "SELECT Timestamp, ServiceName, TraceId, SpanId, SpanName, StatusCode, Duration FROM otel_traces WHERE " + strings.Join(where, " AND ") + " ORDER BY Timestamp DESC LIMIT ?"
		params = append(params, limit)
		rows, err := store.Query(ctx, query, params...)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		resultRows, err := collectRows(rows, []string{"timestamp", "service", "trace_id", "span_id", "span_name", "status", "duration_ns"})
		if err != nil {
			return nil, err
		}
		return map[string]any{"rows": resultRows, "limit": limit}, nil

	case "query_metrics", "query_metrics_raw":
		limit := mcpToolLimit(args, 100, 1, 500)
		service, _ := args["service"].(string)
		metricName, _ := args["metric"].(string)
		where := []string{"1=1"}
		params := []any{}
		if strings.TrimSpace(service) != "" {
			where = append(where, "ServiceName = ?")
			params = append(params, service)
		}
		if strings.TrimSpace(metricName) != "" {
			where = append(where, "MetricName = ?")
			params = append(params, metricName)
		}
		query := "SELECT TimeUnix, ServiceName, MetricName, Value, MetricType FROM otel_metrics WHERE " + strings.Join(where, " AND ") + " ORDER BY TimeUnix DESC LIMIT ?"
		params = append(params, limit)
		rows, err := store.Query(ctx, query, params...)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		resultRows, err := collectRows(rows, []string{"timestamp", "service", "metric", "value", "type"})
		if err != nil {
			return nil, err
		}
		return map[string]any{"rows": resultRows, "limit": limit}, nil

	case "get_anomaly_rules":
		rows, err := store.Query(ctx, "SELECT Id, Name, RuleType, SignalSource, SignalName, ServiceName, Comparator, WarningThreshold, CriticalThreshold FROM sobs_anomaly_rules WHERE IsDeleted = 0 ORDER BY Name LIMIT 500")
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		resultRows, err := collectRows(rows, []string{"id", "name", "type", "source", "signal", "service", "comparator", "warning_threshold", "critical_threshold"})
		if err != nil {
			return nil, err
		}
		return map[string]any{"rules": resultRows}, nil

	case "get_recent_errors":
		limit := mcpToolLimit(args, 50, 1, 500)
		rows, err := store.Query(ctx, "SELECT Timestamp, ServiceName, SeverityText, Body, TraceId, SpanId FROM otel_logs WHERE SeverityText IN ('ERROR','FATAL') ORDER BY Timestamp DESC LIMIT ?", limit)
		if err != nil {
			return nil, err
		}
		defer func() { _ = rows.Close() }()
		resultRows, err := collectRows(rows, []string{"timestamp", "service", "severity", "body", "trace_id", "span_id"})
		if err != nil {
			return nil, err
		}
		return map[string]any{"errors": resultRows, "limit": limit}, nil
	}

	return nil, fmt.Errorf("unsupported tool: %s", name)
}

func mcpToolLimit(args map[string]any, defaultValue int, minValue int, maxValue int) int {
	limit := defaultValue
	raw, ok := args["limit"]
	if !ok {
		return limit
	}
	s := fmt.Sprint(raw)
	parsed, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return limit
	}
	if parsed < minValue {
		return minValue
	}
	if parsed > maxValue {
		return maxValue
	}
	return parsed
}

func collectRows(rows extensionpoints.RowIterator, columns []string) ([]map[string]any, error) {
	out := []map[string]any{}
	for rows.Next() {
		dest := make([]any, len(columns))
		scanTargets := make([]any, len(columns))
		for i := range columns {
			scanTargets[i] = &dest[i]
		}
		if err := rows.Scan(scanTargets...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, col := range columns {
			row[col] = dest[i]
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func hashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func safeKey(key Key) Key {
	key.Hash = ""
	return key
}

func truncate(value string, maxLen int) string {
	if len(value) <= maxLen {
		return value
	}
	return value[:maxLen]
}

func randomHex(size int) string {
	buf := make([]byte, size)
	_, err := rand.Read(buf)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(buf)
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

var ErrUnauthorized = errors.New("unauthorized")
