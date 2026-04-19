package mcp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
	"golang.org/x/crypto/blake2b"
	"golang.org/x/crypto/scrypt"
)

const (
	apiKeyMax          = 20
	rateLimitRequests  = 60
	rateLimitWindowSec = 60
	mcpAPIKeysSetting  = "mcp.api_keys"
	mcpEnabledSetting  = "mcp.enabled"
)

type Key struct {
	ID        string `json:"id"`
	Label     string `json:"label"`
	CreatedAt string `json:"created_at"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Hash      string `json:"key_hash,omitempty"`
}

type Service struct {
	mu         sync.Mutex
	toolSpecs  []map[string]any
	rateWindow map[string][]time.Time
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return NewStoreService(defaultstore.NewFactory())
}

func newBaseService() *Service {
	return &Service{
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
		rateWindow: map[string][]time.Time{},
	}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	svc := newBaseService()
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
	return s.EnabledContext(context.Background())
}

func (s *Service) SetEnabled(enabled bool) bool {
	return s.SetEnabledContext(context.Background(), enabled)
}

func (s *Service) ListKeys() []Key {
	return s.ListKeysContext(context.Background())
}

func (s *Service) ListKeysWithHash() []Key {
	return s.listKeysWithHash(context.Background())
}

func (s *Service) ReplaceKeys(keys []Key) {
	_ = s.saveKeys(context.Background(), keys)
}

func (s *Service) CreateKey(label string, expiresAt string) (Key, string, error) {
	return s.CreateKeyContext(context.Background(), label, expiresAt)
	}

func (s *Service) CreateKeyContext(ctx context.Context, label string, expiresAt string) (Key, string, error) {
	keys := s.listKeysWithHash(ctx)
	if len(keys) >= apiKeyMax {
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
	keys = append(keys, key)
	if err := s.saveKeys(ctx, keys); err != nil {
		return Key{}, "", err
	}
	return safeKey(key), rawKey, nil
}

func (s *Service) DeleteKey(id string) bool {
	return s.DeleteKeyContext(context.Background(), id)
	}

func (s *Service) DeleteKeyContext(ctx context.Context, id string) bool {
	keys := s.listKeysWithHash(ctx)
	for i, key := range keys {
		if key.ID != id {
			continue
		}
		keys = append(keys[:i], keys[i+1:]...)
		return s.saveKeys(ctx, keys) == nil
	}
	return false
}

func (s *Service) Authenticate(rawKey string) bool {
	return s.AuthenticateContext(context.Background(), rawKey)
	}

func (s *Service) AuthenticateContext(ctx context.Context, rawKey string) bool {
	if strings.TrimSpace(rawKey) == "" {
		return false
	}
	hash := hashKey(strings.TrimSpace(rawKey))
	for _, key := range s.listKeysWithHash(ctx) {
		if key.Hash != hash {
			continue
		}
		return true
	}
	return false
}

func (s *Service) CallTool(name string, args map[string]any) (map[string]any, error) {
	return s.callToolStoreBacked(context.Background(), name, args)
}

func (s *Service) EnabledContext(ctx context.Context) bool {
	value, ok, err := persist.GetAppSetting(ctx, s.storeFactory, mcpEnabledSetting)
	if err != nil || !ok {
		return true
	}
	return strings.TrimSpace(value) == "1"
}

func (s *Service) SetEnabledContext(ctx context.Context, enabled bool) bool {
	value := "0"
	if enabled {
		value = "1"
	}
	if err := persist.SetAppSetting(ctx, s.storeFactory, mcpEnabledSetting, value); err != nil {
		return s.EnabledContext(ctx)
	}
	return enabled
}

func (s *Service) ListKeysContext(ctx context.Context) []Key {
	keys := s.listKeysWithHash(ctx)
	out := make([]Key, len(keys))
	for i, key := range keys {
		out[i] = safeKey(key)
	}
	return out
}

func (s *Service) listKeysWithHash(ctx context.Context) []Key {
	raw, ok, err := persist.GetAppSetting(ctx, s.storeFactory, mcpAPIKeysSetting)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return []Key{}
	}
	var keys []Key
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return []Key{}
	}
	return keys
}

func (s *Service) saveKeys(ctx context.Context, keys []Key) error {
	return persist.SetAppSetting(ctx, s.storeFactory, mcpAPIKeysSetting, persist.JSONString(keys))
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
	salt := mcpMACKey()
	derived, err := scrypt.Key([]byte(raw), salt, 1024, 8, 1, 32)
	if err != nil {
		panic(err)
	}
	return hex.EncodeToString(derived)
}

func mcpMACKey() []byte {
	secret := os.Getenv("SOBS_SECRET_KEY")
	if strings.TrimSpace(secret) == "" {
		secret = "sobs-dev-secret-key"
	}
	person := []byte("sobs-mcp-v1\x00\x00\x00\x00\x00")
	sum := blake2b.Sum256(append([]byte{}, append([]byte(secret), person...)...))
	return sum[:]
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

var ErrUnauthorized = errors.New("unauthorized")
