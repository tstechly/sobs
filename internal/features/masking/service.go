package masking

import (
	"context"
	"sort"
	"strings"
	"sync"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

const (
	customKeysSetting       = "masking.custom_keys"
	customPatternsSetting   = "masking.custom_patterns"
	outputEnabledSetting    = "masking.output_enabled"
	sqlOutputEnabledSetting = "masking.sql_output_enabled"
)

type Service struct {
	mu         sync.RWMutex
	keys       map[string]struct{}
	patterns   map[string]struct{}
	outputMode string
	sqlOutput  string
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return &Service{
		keys:       map[string]struct{}{},
		patterns:   map[string]struct{}{},
		outputMode: "mask",
		sqlOutput:  "masked",
	}
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) ListRules() map[string]any {
	if s.storeFactory != nil {
		return s.listRulesStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]any{
		"keys":       setToSortedSlice(s.keys),
		"patterns":   setToSortedSlice(s.patterns),
		"output_mode": s.outputMode,
		"sql_output":  s.sqlOutput,
	}
}

func (s *Service) listRulesStoreBacked(ctx context.Context) map[string]any {
	keys := loadStringSetSetting(ctx, s.storeFactory, customKeysSetting)
	patterns := loadStringSetSetting(ctx, s.storeFactory, customPatternsSetting)
	outputMode := "mask"
	if value, ok, err := persist.GetAppSetting(ctx, s.storeFactory, outputEnabledSetting); err == nil && ok && strings.TrimSpace(value) != "" {
		outputMode = strings.TrimSpace(value)
	}
	sqlOutput := "masked"
	if value, ok, err := persist.GetAppSetting(ctx, s.storeFactory, sqlOutputEnabledSetting); err == nil && ok && strings.TrimSpace(value) != "" {
		sqlOutput = strings.TrimSpace(value)
	}
	return map[string]any{
		"keys":        setToSortedSlice(keys),
		"patterns":    setToSortedSlice(patterns),
		"output_mode": outputMode,
		"sql_output":  sqlOutput,
	}
}

func (s *Service) AddKey(key string) bool {
	k := strings.TrimSpace(key)
	if k == "" {
		return false
	}
	if s.storeFactory != nil {
		keys := loadStringSetSetting(context.Background(), s.storeFactory, customKeysSetting)
		keys[k] = struct{}{}
		return saveStringSetSetting(context.Background(), s.storeFactory, customKeysSetting, keys) == nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.keys[k] = struct{}{}
	return true
}

func (s *Service) DeleteKey(key string) bool {
	k := strings.TrimSpace(key)
	if k == "" {
		return false
	}
	if s.storeFactory != nil {
		keys := loadStringSetSetting(context.Background(), s.storeFactory, customKeysSetting)
		if _, ok := keys[k]; !ok {
			return false
		}
		delete(keys, k)
		return saveStringSetSetting(context.Background(), s.storeFactory, customKeysSetting, keys) == nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.keys[k]; !ok {
		return false
	}
	delete(s.keys, k)
	return true
}

func (s *Service) AddPattern(pattern string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return false
	}
	if s.storeFactory != nil {
		patterns := loadStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting)
		patterns[p] = struct{}{}
		return saveStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting, patterns) == nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.patterns[p] = struct{}{}
	return true
}

func (s *Service) DeletePattern(pattern string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return false
	}
	if s.storeFactory != nil {
		patterns := loadStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting)
		if _, ok := patterns[p]; !ok {
			return false
		}
		delete(patterns, p)
		return saveStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting, patterns) == nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.patterns[p]; !ok {
		return false
	}
	delete(s.patterns, p)
	return true
}

func (s *Service) SetOutputMode(mode string) {
	m := strings.TrimSpace(mode)
	if m == "" {
		m = "mask"
	}
	if s.storeFactory != nil {
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, outputEnabledSetting, m)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outputMode = m
}

func (s *Service) SetSQLOutput(mode string) {
	m := strings.TrimSpace(mode)
	if m == "" {
		m = "masked"
	}
	if s.storeFactory != nil {
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, sqlOutputEnabledSetting, m)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sqlOutput = m
}

func (s *Service) Preview(input string) map[string]string {
	if s.storeFactory != nil {
		rules := s.listRulesStoreBacked(context.Background())
		masked := input
		for _, key := range toStringSlice(rules["keys"]) {
			masked = strings.ReplaceAll(masked, key, "***")
		}
		for _, pattern := range toStringSlice(rules["patterns"]) {
			masked = strings.ReplaceAll(masked, pattern, "***")
		}
		return map[string]string{"input": input, "output": masked}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	masked := input
	for key := range s.keys {
		masked = strings.ReplaceAll(masked, key, "***")
	}
	for pattern := range s.patterns {
		masked = strings.ReplaceAll(masked, pattern, "***")
	}
	return map[string]string{"input": input, "output": masked}
}

func loadStringSetSetting(ctx context.Context, factory extensionpoints.StoreFactory, key string) map[string]struct{} {
	out := map[string]struct{}{}
	raw, ok, err := persist.GetAppSetting(ctx, factory, key)
	if err != nil || !ok || strings.TrimSpace(raw) == "" {
		return out
	}
	for _, item := range persist.ParseJSONStringSlice(raw) {
		item = strings.TrimSpace(item)
		if item != "" {
			out[item] = struct{}{}
		}
	}
	return out
}

func saveStringSetSetting(ctx context.Context, factory extensionpoints.StoreFactory, key string, values map[string]struct{}) error {
	return persist.SetAppSetting(ctx, factory, key, persist.JSONString(setToSortedSlice(values)))
}

func toStringSlice(value any) []string {
	items, ok := value.([]string)
	if ok {
		return items
	}
	out, ok := value.([]any)
	if !ok {
		return nil
	}
	items = make([]string, 0, len(out))
	for _, item := range out {
		if text, ok := item.(string); ok {
			items = append(items, text)
		}
	}
	return items
}

func setToSortedSlice(in map[string]struct{}) []string {
	out := make([]string, 0, len(in))
	for v := range in {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
