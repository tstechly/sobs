package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Subscription struct {
	ID        string `json:"id"`
	Endpoint  string `json:"endpoint"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

type Rule struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
}

type Service struct {
	mu        sync.RWMutex
	items     map[string]Subscription
	rules     map[string]Rule
	nextRule  int64
	vapidPub  string
	vapidPriv string
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return &Service{items: make(map[string]Subscription), rules: make(map[string]Rule), nextRule: 1}
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_notification_channels (Id String, Name String, ChannelType String, ConfigJson String, Enabled UInt8 DEFAULT 1, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_notification_rules (Id String, Name String, Enabled UInt8 DEFAULT 1, LogicOperator String DEFAULT 'any', ConditionsJson String, ChannelIds String, Severity String DEFAULT 'warning', CooldownSeconds UInt32 DEFAULT 300, LastFiredAt DateTime64(3) DEFAULT now64(3), IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) Subscribe(endpoint string) (Subscription, error) {
	if s.storeFactory != nil {
		return s.subscribeStoreBacked(context.Background(), endpoint)
	}
	if endpoint == "" {
		return Subscription{}, errors.New("endpoint is required")
	}
	id := makeID()
	sub := Subscription{ID: id, Endpoint: endpoint, Enabled: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = sub
	return sub, nil
}

func (s *Service) subscribeStoreBacked(ctx context.Context, endpoint string) (Subscription, error) {
	if endpoint == "" {
		return Subscription{}, errors.New("endpoint is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Subscription{}, err
	}
	id := persist.NewID()
	createdAt := persist.RFC3339Now()
	config := persist.JSONString(map[string]string{"endpoint": endpoint})
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Subscription{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_notification_channels (Id, Name, ChannelType, ConfigJson, Enabled, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, endpoint, "webpush", config, 1, 0, persist.Version())
	if err != nil {
		return Subscription{}, err
	}
	return Subscription{ID: id, Endpoint: endpoint, Enabled: true, CreatedAt: createdAt}, nil
}

func makeID() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func (s *Service) VAPIDPublicKey() string {
	if s.storeFactory != nil {
		if raw, ok, err := persist.GetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.public"); err == nil && ok {
			return raw
		}
		return ""
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vapidPub
}

func (s *Service) GenerateVAPIDKeys() (string, string) {
	pub := makeID() + makeID()
	priv := makeID() + makeID()
	if s.storeFactory != nil {
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.public", pub)
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.private", priv)
		return pub, priv
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vapidPub = pub
	s.vapidPriv = priv
	return pub, priv
}

func (s *Service) DeleteVAPIDKeys() {
	if s.storeFactory != nil {
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.public", "")
		_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.private", "")
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vapidPub = ""
	s.vapidPriv = ""
}

func (s *Service) HasSubscription(id string) bool {
	if s.storeFactory != nil {
		store, err := persist.Open(context.Background(), s.storeFactory)
		if err != nil {
			return false
		}
		defer func() { _ = store.Close() }()
		rows, err := store.Query(context.Background(), "SELECT 1 FROM sobs_notification_channels FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
		if err != nil {
			return false
		}
		defer func() { _ = rows.Close() }()
		return rows.Next()
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	_, ok := s.items[id]
	return ok
}

func (s *Service) ListSubscriptions() []Subscription {
	if s.storeFactory != nil {
		return s.listSubscriptionsStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Subscription, 0, len(s.items))
	for _, item := range s.items {
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) listSubscriptionsStoreBacked(ctx context.Context) []Subscription {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, ConfigJson, Enabled FROM sobs_notification_channels FINAL WHERE IsDeleted = 0 ORDER BY Id")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Subscription{}
	for rows.Next() {
		var id string
		var configJSON string
		var enabled uint8
		if err := rows.Scan(&id, &configJSON, &enabled); err != nil {
			return out
		}
		config := persist.ParseStringMap(configJSON)
		out = append(out, Subscription{ID: id, Endpoint: config["endpoint"], Enabled: enabled == 1, CreatedAt: persist.RFC3339Now()})
	}
	return out
}

func (s *Service) ToggleSubscription(id string) (Subscription, bool) {
	if s.storeFactory != nil {
		return s.toggleSubscriptionStoreBacked(context.Background(), id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	sub, ok := s.items[id]
	if !ok {
		return Subscription{}, false
	}
	sub.Enabled = !sub.Enabled
	s.items[id] = sub
	return sub, true
}

func (s *Service) toggleSubscriptionStoreBacked(ctx context.Context, id string) (Subscription, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return Subscription{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Subscription{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Name, ConfigJson, Enabled FROM sobs_notification_channels FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return Subscription{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Subscription{}, false
	}
	var name string
	var configJSON string
	var enabled uint8
	if err := rows.Scan(&name, &configJSON, &enabled); err != nil {
		return Subscription{}, false
	}
	nextEnabled := uint8(1)
	if enabled == 1 {
		nextEnabled = 0
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_notification_channels (Id, Name, ChannelType, ConfigJson, Enabled, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, name, "webpush", configJSON, nextEnabled, 0, persist.Version())
	if err != nil {
		return Subscription{}, false
	}
	config := persist.ParseStringMap(configJSON)
	return Subscription{ID: id, Endpoint: config["endpoint"], Enabled: nextEnabled == 1, CreatedAt: persist.RFC3339Now()}, true
}

func (s *Service) DeleteSubscription(id string) bool {
	if s.storeFactory != nil {
		return s.deleteSubscriptionStoreBacked(context.Background(), id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.items[id]; !ok {
		return false
	}
	delete(s.items, id)
	return true
}

func (s *Service) deleteSubscriptionStoreBacked(ctx context.Context, id string) bool {
	if err := s.ensureSchema(ctx); err != nil {
		return false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Name, ConfigJson, Enabled FROM sobs_notification_channels FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var name string
	var configJSON string
	var enabled uint8
	if err := rows.Scan(&name, &configJSON, &enabled); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_notification_channels (Id, Name, ChannelType, ConfigJson, Enabled, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?)", id, name, "webpush", configJSON, enabled, 1, persist.Version())
	return err == nil
}

func (s *Service) CreateRule(name string) (Rule, error) {
	if s.storeFactory != nil {
		return s.createRuleStoreBacked(context.Background(), name)
	}
	if name == "" {
		return Rule{}, errors.New("name is required")
	}
	id := strconv.FormatInt(s.nextRule, 10)
	s.nextRule++
	r := Rule{ID: id, Name: name, Enabled: true, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.rules[id] = r
	return r, nil
}

func (s *Service) createRuleStoreBacked(ctx context.Context, name string) (Rule, error) {
	if name == "" {
		return Rule{}, errors.New("name is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Rule{}, err
	}
	id := persist.NewID()
	createdAt := persist.RFC3339Now()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Rule{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_notification_rules (Id, Name, Enabled, LogicOperator, ConditionsJson, ChannelIds, Severity, CooldownSeconds, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, 1, "any", "[]", "[]", "warning", 300, 0, persist.Version())
	if err != nil {
		return Rule{}, err
	}
	return Rule{ID: id, Name: name, Enabled: true, CreatedAt: createdAt}, nil
}

func (s *Service) ToggleRule(id string) (Rule, bool) {
	if s.storeFactory != nil {
		return s.toggleRuleStoreBacked(context.Background(), id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.rules[id]
	if !ok {
		return Rule{}, false
	}
	r.Enabled = !r.Enabled
	s.rules[id] = r
	return r, true
}

func (s *Service) toggleRuleStoreBacked(ctx context.Context, id string) (Rule, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return Rule{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Rule{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Name, Enabled, LogicOperator, ConditionsJson, ChannelIds, Severity, CooldownSeconds FROM sobs_notification_rules FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return Rule{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Rule{}, false
	}
	var name, logicOperator, conditionsJSON, channelIDs, severity string
	var enabled uint8
	var cooldown uint32
	if err := rows.Scan(&name, &enabled, &logicOperator, &conditionsJSON, &channelIDs, &severity, &cooldown); err != nil {
		return Rule{}, false
	}
	nextEnabled := uint8(1)
	if enabled == 1 {
		nextEnabled = 0
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_notification_rules (Id, Name, Enabled, LogicOperator, ConditionsJson, ChannelIds, Severity, CooldownSeconds, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, nextEnabled, logicOperator, conditionsJSON, channelIDs, severity, cooldown, 0, persist.Version())
	if err != nil {
		return Rule{}, false
	}
	return Rule{ID: id, Name: name, Enabled: nextEnabled == 1, CreatedAt: persist.RFC3339Now()}, true
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
	rows, err := store.Query(ctx, "SELECT Name, Enabled, LogicOperator, ConditionsJson, ChannelIds, Severity, CooldownSeconds FROM sobs_notification_rules FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var name, logicOperator, conditionsJSON, channelIDs, severity string
	var enabled uint8
	var cooldown uint32
	if err := rows.Scan(&name, &enabled, &logicOperator, &conditionsJSON, &channelIDs, &severity, &cooldown); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_notification_rules (Id, Name, Enabled, LogicOperator, ConditionsJson, ChannelIds, Severity, CooldownSeconds, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, enabled, logicOperator, conditionsJSON, channelIDs, severity, cooldown, 1, persist.Version())
	return err == nil
}

func (s *Service) AutoGenerateRules() []Rule {
	out := make([]Rule, 0, 2)
	for _, n := range []string{"error-spike", "latency-regression"} {
		r, _ := s.CreateRule(n)
		out = append(out, r)
	}
	return out
}

func (s *Service) ListRules() []Rule {
	if s.storeFactory != nil {
		return s.listRulesStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Rule, 0, len(s.rules))
	for _, item := range s.rules {
		out = append(out, item)
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
	rows, err := store.Query(ctx, "SELECT Id, Name, Enabled FROM sobs_notification_rules FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Rule{}
	for rows.Next() {
		var item Rule
		var enabled uint8
		if err := rows.Scan(&item.ID, &item.Name, &enabled); err != nil {
			return out
		}
		item.Enabled = enabled == 1
		item.CreatedAt = persist.RFC3339Now()
		out = append(out, item)
	}
	return out
}
