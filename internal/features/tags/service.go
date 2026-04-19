package tags

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Rule struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Condition  string `json:"condition"`
	TagKey     string `json:"tag_key"`
	TagValue   string `json:"tag_value"`
	CreatedAt  string `json:"created_at"`
}

type Service struct {
	mu         sync.RWMutex
	rules      map[string]Rule
	recordTags map[string]map[string]string
	nextID     int64
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return &Service{rules: map[string]Rule{}, recordTags: map[string]map[string]string{}, nextID: 1}
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
	if s.storeFactory != nil {
		return s.listRulesStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Rule, 0, len(s.rules))
	for _, r := range s.rules {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
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
	rows, err := store.Query(ctx, "SELECT Id, Name, MatchValue, TagKey, TagValue, Version FROM sobs_tag_rules FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Rule{}
	for rows.Next() {
		var rule Rule
		var version uint64
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Condition, &rule.TagKey, &rule.TagValue, &version); err != nil {
			return out
		}
		rule.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
		out = append(out, rule)
	}
	return out
}

func (s *Service) CreateRule(name, condition, tagKey, tagValue string) (Rule, error) {
	if s.storeFactory != nil {
		return s.createRuleStoreBacked(context.Background(), name, condition, tagKey, tagValue)
	}
	if strings.TrimSpace(name) == "" {
		return Rule{}, errors.New("name is required")
	}
	if strings.TrimSpace(tagKey) == "" {
		return Rule{}, errors.New("tag_key is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextID, 10)
	s.nextID++
	r := Rule{ID: id, Name: strings.TrimSpace(name), Condition: strings.TrimSpace(condition), TagKey: strings.TrimSpace(tagKey), TagValue: strings.TrimSpace(tagValue), CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.rules[id] = r
	return r, nil
}

func (s *Service) createRuleStoreBacked(ctx context.Context, name, condition, tagKey, tagValue string) (Rule, error) {
	if strings.TrimSpace(name) == "" {
		return Rule{}, errors.New("name is required")
	}
	if strings.TrimSpace(tagKey) == "" {
		return Rule{}, errors.New("tag_key is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
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
	_, err = store.Exec(ctx, "INSERT INTO sobs_tag_rules (Id, Name, RecordTypes, MatchField, MatchOperator, MatchValue, MatchAttrKey, TagKey, TagValue, ConditionsJson, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, "all", "body", "contains", condition, "", tagKey, tagValue, "[]", 0, version)
	if err != nil {
		return Rule{}, err
	}
	return Rule{ID: id, Name: name, Condition: condition, TagKey: tagKey, TagValue: tagValue, CreatedAt: createdAt}, nil
}

func (s *Service) DeleteRule(id string) bool {
	if s.storeFactory != nil {
		return s.deleteRuleStoreBacked(context.Background(), id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.rules[id]; !ok {
		return false
	}
	delete(s.rules, id)
	return true
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
		condition string
		tagKey    string
		tagValue  string
	}{
		{name: "Error severity", condition: "severity_text = 'ERROR'", tagKey: "priority", tagValue: "high"},
		{name: "Latency hotspot", condition: "duration_ms > 1000", tagKey: "hotspot", tagValue: "true"},
	} {
		r, _ := s.CreateRule(c.name, c.condition, c.tagKey, c.tagValue)
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
	if s.storeFactory != nil {
		return s.getRecordTagsStoreBacked(context.Background(), recordType, recordID)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	key := s.recordKey(recordType, recordID)
	in := s.recordTags[key]
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
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
	if s.storeFactory != nil {
		return s.setRecordTagStoreBacked(context.Background(), recordType, recordID, tagKey, tagValue)
	}
	tk := strings.TrimSpace(tagKey)
	if tk == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.recordKey(recordType, recordID)
	if _, ok := s.recordTags[key]; !ok {
		s.recordTags[key] = map[string]string{}
	}
	s.recordTags[key][tk] = strings.TrimSpace(tagValue)
	return true
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
	if s.storeFactory != nil {
		return s.deleteRecordTagStoreBacked(context.Background(), recordType, recordID, tagKey)
	}
	tk := strings.TrimSpace(tagKey)
	if tk == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := s.recordKey(recordType, recordID)
	if _, ok := s.recordTags[key]; !ok {
		return false
	}
	if _, ok := s.recordTags[key][tk]; !ok {
		return false
	}
	delete(s.recordTags[key], tk)
	return true
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
