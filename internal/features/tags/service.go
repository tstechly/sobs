package tags

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

// Condition mirrors Python's per-condition dict inside ConditionsJson.
type Condition struct {
	MatchField    string `json:"match_field"`
	MatchOperator string `json:"match_operator"`
	MatchValue    string `json:"match_value"`
	MatchAttrKey  string `json:"match_attr_key,omitempty"`
}

// Rule mirrors Python's _load_tag_rules output dict.
type Rule struct {
	ID            string      `json:"id"`
	Name          string      `json:"name"`
	RecordTypes   []string    `json:"record_types"`
	MatchField    string      `json:"match_field"`
	MatchOperator string      `json:"match_operator"`
	MatchValue    string      `json:"match_value"`
	MatchAttrKey  string      `json:"match_attr_key,omitempty"`
	TagKey        string      `json:"tag_key"`
	TagValue      string      `json:"tag_value"`
	Conditions    []Condition `json:"conditions"`
	CreatedAt     string      `json:"created_at"`
}

// RuleInput is the input for creating or editing a rule, matching Python's
// create_tag_rule form fields.
type RuleInput struct {
	Name        string      `json:"name"`
	RecordTypes []string    `json:"record_types"`
	Conditions  []Condition `json:"conditions"`
	TagKey      string      `json:"tag_key"`
	TagValue    string      `json:"tag_value"`
}

type Service struct {
	mu           sync.RWMutex
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_tag_rules (Id String, Name String, RecordTypes String, MatchField String, MatchOperator String, MatchValue String, MatchAttrKey String, TagKey String, TagValue String, ConditionsJson String DEFAULT '', IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_record_tags (RecordType String, RecordId String, TagKey String, TagValue String, IsAuto UInt8 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (RecordType, RecordId, TagKey)")
		}
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
	rows, err := store.Query(ctx, "SELECT Id, Name, RecordTypes, MatchField, MatchOperator, MatchValue, MatchAttrKey, TagKey, TagValue, ConditionsJson, Version FROM sobs_tag_rules FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Rule{}
	for rows.Next() {
		var rule Rule
		var recordTypesStr, conditionsJSON string
		var version uint64
		if err := rows.Scan(&rule.ID, &rule.Name, &recordTypesStr, &rule.MatchField, &rule.MatchOperator, &rule.MatchValue, &rule.MatchAttrKey, &rule.TagKey, &rule.TagValue, &conditionsJSON, &version); err != nil {
			return out
		}
		rule.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
		// Parse RecordTypes from comma-separated string.
		for _, t := range strings.Split(recordTypesStr, ",") {
			if t = strings.TrimSpace(t); t != "" {
				rule.RecordTypes = append(rule.RecordTypes, t)
			}
		}
		if len(rule.RecordTypes) == 0 {
			rule.RecordTypes = []string{"all"}
		}
		// Parse ConditionsJson; fall back to legacy columns for old rows.
		var conditions []Condition
		if conditionsJSON != "" && conditionsJSON != "[]" {
			_ = json.Unmarshal([]byte(conditionsJSON), &conditions)
		}
		if len(conditions) == 0 && strings.TrimSpace(rule.MatchField) != "" {
			conditions = []Condition{{
				MatchField:    rule.MatchField,
				MatchOperator: rule.MatchOperator,
				MatchValue:    rule.MatchValue,
				MatchAttrKey:  rule.MatchAttrKey,
			}}
		}
		rule.Conditions = conditions
		out = append(out, rule)
	}
	return out
}

func (s *Service) CreateRule(input RuleInput) (Rule, error) {
	return s.createRuleStoreBacked(context.Background(), input)
}

func (s *Service) createRuleStoreBacked(ctx context.Context, input RuleInput) (Rule, error) {
	if strings.TrimSpace(input.Name) == "" {
		return Rule{}, errors.New("name is required")
	}
	if len(input.Conditions) == 0 {
		return Rule{}, errors.New("at least one condition is required")
	}
	if strings.TrimSpace(input.TagKey) == "" {
		return Rule{}, errors.New("tag_key is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Rule{}, err
	}

	// Normalise record types; default to "all".
	recordTypes := input.RecordTypes
	if len(recordTypes) == 0 {
		recordTypes = []string{"all"}
	}
	recordTypesStr := strings.Join(recordTypes, ",")

	// Primary (legacy) condition columns use first condition.
	primary := input.Conditions[0]

	conditionsJSON, err := json.Marshal(input.Conditions)
	if err != nil {
		return Rule{}, err
	}

	id := persist.NewID()
	version := persist.Version()
	createdAt := time.Unix(0, int64(version)).UTC().Format(time.RFC3339)

	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Rule{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx,
		"INSERT INTO sobs_tag_rules (Id, Name, RecordTypes, MatchField, MatchOperator, MatchValue, MatchAttrKey, TagKey, TagValue, ConditionsJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		id, input.Name, recordTypesStr,
		primary.MatchField, primary.MatchOperator, primary.MatchValue, primary.MatchAttrKey,
		input.TagKey, input.TagValue,
		string(conditionsJSON), 0, version,
	)
	if err != nil {
		return Rule{}, err
	}
	return Rule{
		ID:            id,
		Name:          input.Name,
		RecordTypes:   recordTypes,
		MatchField:    primary.MatchField,
		MatchOperator: primary.MatchOperator,
		MatchValue:    primary.MatchValue,
		MatchAttrKey:  primary.MatchAttrKey,
		TagKey:        input.TagKey,
		TagValue:      input.TagValue,
		Conditions:    input.Conditions,
		CreatedAt:     createdAt,
	}, nil
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
	rows, err := store.Query(ctx, "SELECT Name, RecordTypes, MatchField, MatchOperator, MatchValue, MatchAttrKey, TagKey, TagValue, ConditionsJson FROM sobs_tag_rules FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var name, recordTypes, matchField, matchOperator, matchValue, matchAttrKey, tagKey, tagValue, conditions string
	if err := rows.Scan(&name, &recordTypes, &matchField, &matchOperator, &matchValue, &matchAttrKey, &tagKey, &tagValue, &conditions); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_tag_rules (Id, Name, RecordTypes, MatchField, MatchOperator, MatchValue, MatchAttrKey, TagKey, TagValue, ConditionsJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, recordTypes, matchField, matchOperator, matchValue, matchAttrKey, tagKey, tagValue, conditions, 1, persist.Version())
	return err == nil
}

func (s *Service) AutoGenerate() []Rule {
	out := make([]Rule, 0, 2)
	for _, c := range []struct {
		name      string
		field     string
		operator  string
		value     string
		tagKey    string
		tagValue  string
	}{
		{name: "Error severity", field: "severity", operator: "eq", value: "ERROR", tagKey: "priority", tagValue: "high"},
		{name: "Latency hotspot", field: "body", operator: "contains", value: "timeout", tagKey: "hotspot", tagValue: "true"},
	} {
		r, _ := s.CreateRule(RuleInput{
			Name:        c.name,
			RecordTypes: []string{"all"},
			Conditions:  []Condition{{MatchField: c.field, MatchOperator: c.operator, MatchValue: c.value}},
			TagKey:      c.tagKey,
			TagValue:    c.tagValue,
		})
		out = append(out, r)
	}
	return out
}

func (s *Service) ConditionSuggestions() []string {
	return []string{
		"severity_text = 'ERROR'",
		"duration_ms > 1000",
		"service.name = 'api'",
	}
}

func (s *Service) recordKey(recordType, recordID string) string {
	return strings.TrimSpace(recordType) + ":" + strings.TrimSpace(recordID)
}

func (s *Service) GetRecordTags(recordType, recordID string) map[string]string {
	return s.getRecordTagsStoreBacked(context.Background(), recordType, recordID)
}

func (s *Service) getRecordTagsStoreBacked(ctx context.Context, recordType, recordID string) map[string]string {
	if err := s.ensureSchema(ctx); err != nil {
		return map[string]string{}
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return map[string]string{}
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT TagKey, TagValue FROM sobs_record_tags FINAL WHERE IsDeleted = 0 AND RecordType = ? AND RecordId = ? ORDER BY TagKey", recordType, recordID)
	if err != nil {
		return map[string]string{}
	}
	defer func() { _ = rows.Close() }()
	out := map[string]string{}
	for rows.Next() {
		var key string
		var value string
		if err := rows.Scan(&key, &value); err != nil {
			return out
		}
		out[key] = value
	}
	return out
}

func (s *Service) SetRecordTag(recordType, recordID, tagKey, tagValue string) bool {
	return s.setRecordTagStoreBacked(context.Background(), recordType, recordID, tagKey, tagValue)
}

func (s *Service) setRecordTagStoreBacked(ctx context.Context, recordType, recordID, tagKey, tagValue string) bool {
	tk := strings.TrimSpace(tagKey)
	if tk == "" {
		return false
	}
	if err := s.ensureSchema(ctx); err != nil {
		return false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_record_tags (RecordType, RecordId, TagKey, TagValue, IsAuto, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", recordType, recordID, tk, strings.TrimSpace(tagValue), 0, 0, persist.Version())
	return err == nil
}

func (s *Service) DeleteRecordTag(recordType, recordID, tagKey string) bool {
	return s.deleteRecordTagStoreBacked(context.Background(), recordType, recordID, tagKey)
}

func (s *Service) deleteRecordTagStoreBacked(ctx context.Context, recordType, recordID, tagKey string) bool {
	tk := strings.TrimSpace(tagKey)
	if tk == "" {
		return false
	}
	if err := s.ensureSchema(ctx); err != nil {
		return false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT TagValue FROM sobs_record_tags FINAL WHERE IsDeleted = 0 AND RecordType = ? AND RecordId = ? AND TagKey = ? LIMIT 1", recordType, recordID, tk)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var tagValue string
	if err := rows.Scan(&tagValue); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_record_tags (RecordType, RecordId, TagKey, TagValue, IsAuto, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", recordType, recordID, tk, tagValue, 0, 1, persist.Version())
	return err == nil
}
