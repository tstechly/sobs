package masking

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

const (
	customKeysSetting       = "masking.custom_keys"
	customPatternsSetting   = "masking.custom_patterns"
	outputEnabledSetting    = "masking.output_enabled"
	sqlOutputEnabledSetting = "masking.sql_output_enabled"
)

// defaultSensitiveKeys mirrors Python's DEFAULT_SENSITIVE_KEYS frozenset.
var defaultSensitiveKeys = map[string]struct{}{
	"password": {}, "passwd": {}, "pwd": {}, "secret": {},
	"client_secret": {}, "api_key": {}, "api_secret": {}, "apikey": {},
	"token": {}, "access_token": {}, "refresh_token": {}, "id_token": {},
	"auth_token": {}, "bearer_token": {},
	"authorization": {}, "x-authorization": {}, "x-api-key": {},
	"private_key": {}, "private-key": {},
	"credit_card": {}, "card_number": {}, "cvv": {}, "cvc": {},
	"ssn": {}, "social_security_number": {},
	"s3_secret_access_key": {}, "backup_encryption_password": {}, "smtp_password": {},
}

// defaultSensitivePatterns mirrors Python's DEFAULT_SENSITIVE_PATTERNS list.
// Each pattern's entire match is replaced with the mask.
var defaultSensitivePatterns = []*regexp.Regexp{
	// Email addresses
	regexp.MustCompile(`(?i)\b[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}\b`),
	// JWT tokens (three base64url-encoded segments)
	regexp.MustCompile(`\beyJ[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]+\.[A-Za-z0-9\-_]*\b`),
	// Bearer token in text / HTTP headers
	regexp.MustCompile(`(?i)bearer\s+[A-Za-z0-9\-_.~+/]+=*`),
	// AWS access key IDs
	regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`),
	// US Social Security Numbers (###-##-####)
	regexp.MustCompile(`\b\d{3}-\d{2}-\d{4}\b`),
	// Visa (13 or 16 digits)
	regexp.MustCompile(`\b4[0-9]{12}(?:[0-9]{3})?\b`),
	// Mastercard
	regexp.MustCompile(`\b5[1-5][0-9]{14}\b`),
	// Amex
	regexp.MustCompile(`\b3[47][0-9]{13}\b`),
	// Discover
	regexp.MustCompile(`\b6(?:011|5[0-9]{2})[0-9]{12}\b`),
	// PEM private key blocks
	regexp.MustCompile(`(?s)-----BEGIN (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----[\s\S]+?-----END (?:RSA |EC |DSA |OPENSSH )?PRIVATE KEY-----`),
	// Generic key=value / key: value assignment in log lines
	regexp.MustCompile(`(?i)(?:password|passwd|pwd|secret|api[_\-]?key|auth[_\-]?token|access[_\-]?token)\s*[=:]\s*['"]?[A-Za-z0-9\-_.~+/!@#$%^&*]{6,}['"]?`),
	// Authorization header value
	regexp.MustCompile(`(?i)(?:Authorization|X-Api-Key|X-Auth-Token)\s*:\s*[^\r\n]+`),
}

// DefaultSensitiveKeys returns the canonical built-in sensitive key names.
func DefaultSensitiveKeys() []string {
	keys := make([]string, 0, len(defaultSensitiveKeys))
	for key := range defaultSensitiveKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// DefaultSensitivePatterns returns the canonical built-in sensitive regex patterns.
func DefaultSensitivePatterns() []string {
	patterns := make([]string, 0, len(defaultSensitivePatterns))
	for _, re := range defaultSensitivePatterns {
		if re == nil {
			continue
		}
		patterns = append(patterns, re.String())
	}
	sort.Strings(patterns)
	return patterns
}

type Service struct {
	storeFactory extensionpoints.StoreFactory
}

func NewService() *Service {
	return NewStoreService(defaultstore.NewFactory())
}

func NewStoreService(factory extensionpoints.StoreFactory) *Service {
	return &Service{storeFactory: factory}
}

func (s *Service) ListRules() map[string]any {
	return s.listRulesStoreBacked(context.Background())
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
	keys := loadStringSetSetting(context.Background(), s.storeFactory, customKeysSetting)
	keys[k] = struct{}{}
	return saveStringSetSetting(context.Background(), s.storeFactory, customKeysSetting, keys) == nil
}

func (s *Service) DeleteKey(key string) bool {
	k := strings.TrimSpace(key)
	if k == "" {
		return false
	}
	keys := loadStringSetSetting(context.Background(), s.storeFactory, customKeysSetting)
	if _, ok := keys[k]; !ok {
		return false
	}
	delete(keys, k)
	return saveStringSetSetting(context.Background(), s.storeFactory, customKeysSetting, keys) == nil
}

func (s *Service) AddPattern(pattern string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return false
	}
	patterns := loadStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting)
	patterns[p] = struct{}{}
	return saveStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting, patterns) == nil
}

func (s *Service) DeletePattern(pattern string) bool {
	p := strings.TrimSpace(pattern)
	if p == "" {
		return false
	}
	patterns := loadStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting)
	if _, ok := patterns[p]; !ok {
		return false
	}
	delete(patterns, p)
	return saveStringSetSetting(context.Background(), s.storeFactory, customPatternsSetting, patterns) == nil
}

func (s *Service) SetOutputMode(mode string) {
	m := strings.TrimSpace(mode)
	if m == "" {
		m = "mask"
	}
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, outputEnabledSetting, m)
}

func (s *Service) SetSQLOutput(mode string) {
	m := strings.TrimSpace(mode)
	if m == "" {
		m = "masked"
	}
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, sqlOutputEnabledSetting, m)
}

func (s *Service) Preview(input string) map[string]string {
	const mask = "****"
	rules := s.listRulesStoreBacked(context.Background())
	masked := input

	// Apply default sensitive patterns (regex).
	for _, re := range defaultSensitivePatterns {
		masked = re.ReplaceAllString(masked, mask)
	}

	// Apply custom user-defined patterns (compiled on the fly).
	for _, pattern := range toStringSlice(rules["patterns"]) {
		if re, err := regexp.Compile(pattern); err == nil {
			masked = re.ReplaceAllString(masked, mask)
		}
	}

	// Apply default sensitive key names: replace key=value / key: value patterns.
	for key := range defaultSensitiveKeys {
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(key) + `\s*[=:]\s*['"]?[^\s'"]{1,}['"]?`)
		masked = re.ReplaceAllString(masked, mask)
	}

	// Apply custom key names.
	for _, key := range toStringSlice(rules["keys"]) {
		if key == "" {
			continue
		}
		re := regexp.MustCompile(`(?i)` + regexp.QuoteMeta(key) + `\s*[=:]\s*['"]?[^\s'"]{1,}['"]?`)
		masked = re.ReplaceAllString(masked, mask)
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
