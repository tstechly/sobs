package notifications

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"sync"

	"github.com/abartrim/sobs/internal/features/defaultstore"
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_notification_channels (Id String, Name String, ChannelType String, ConfigJson String, Enabled UInt8 DEFAULT 1, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_notification_rules (Id String, Name String, Enabled UInt8 DEFAULT 1, LogicOperator String DEFAULT 'any', ConditionsJson String, ChannelIds String, Severity String DEFAULT 'warning', CooldownSeconds UInt32 DEFAULT 300, LastFiredAt DateTime64(3) DEFAULT now64(3), IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) Subscribe(endpoint string) (Subscription, error) {
	return s.subscribeStoreBacked(context.Background(), endpoint)
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
	if raw, ok, err := persist.GetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.public"); err == nil && ok {
		return raw
	}
	return ""
}

func (s *Service) GenerateVAPIDKeys() (string, string) {
	pub := makeID() + makeID()
	priv := makeID() + makeID()
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.public", pub)
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.private", priv)
	return pub, priv
}

func (s *Service) DeleteVAPIDKeys() {
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.public", "")
	_ = persist.SetAppSetting(context.Background(), s.storeFactory, "notifications.vapid.private", "")
}

func (s *Service) HasSubscription(id string) bool {
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

func (s *Service) ListSubscriptions() []Subscription {
	return s.listSubscriptionsStoreBacked(context.Background())
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
	return s.toggleSubscriptionStoreBacked(context.Background(), id)
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
	return s.deleteSubscriptionStoreBacked(context.Background(), id)
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
	return s.createRuleStoreBacked(context.Background(), name)
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
	return s.toggleRuleStoreBacked(context.Background(), id)
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
