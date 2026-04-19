package agents

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/abartrim/sobs/internal/features/defaultstore"
	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Run struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Issue struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	CreatedAt string `json:"created_at"`
}

type Rule struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	TriggerType      string   `json:"trigger_type"`
	TriggerRefID     string   `json:"trigger_ref_id"`
	TriggerState     string   `json:"trigger_state"`
	Actions          []string `json:"actions"`
	RateLimitMinutes int      `json:"rate_limit_minutes"`
	Enabled          bool     `json:"enabled"`
	CreatedAt        string   `json:"created_at"`
}

type Service struct {
	mu        sync.RWMutex
	runs      map[string]Run
	issues    map[string]Issue
	rules     map[string]Rule
	nextRun   int64
	nextIssue int64
	nextRule  int64
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_agent_rules (Id String, Name String, Description String, TriggerType String, TriggerRefId String, TriggerState String, Actions String, RateLimitMinutes UInt32 DEFAULT 60, IsEnabled UInt8 DEFAULT 1, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_agent_runs (Id String, RuleId String, RuleName String, TriggerContext String, Status String, GuardDecision String, DlpResult String, Analysis String, Suggestion String, GithubIssueUrl String, ErrorMessage String, CreatedAt DateTime64(9), CompletedAt DateTime64(9), IsDismissed UInt8 DEFAULT 0, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY Id")
		}
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_github_work_items (Id String, CreatedAt DateTime64(3), CompletedAt DateTime64(3), AgentRunId String, AgentRuleId String, AgentRuleName String, AgentAction String, ServiceName String, AnomalyRuleId String, AnomalyState String, SignalSource String, SignalName String, SignalValue Float64, GithubRepo String, DedupKey String, DedupDecision String DEFAULT 'new_issue', DedupConfidence Float64 DEFAULT 0, IssueNumber UInt32 DEFAULT 0, IssueUrl String, CanonicalIssueNumber UInt32 DEFAULT 0, CanonicalIssueUrl String, RelatedIssueUrls String, OccurrenceCount UInt32 DEFAULT 1, IssueState String DEFAULT '', IssueTitle String, AnalysisSummary String, SuggestionSummary String, CopilotAssignmentRequestedAt UInt64 DEFAULT 0, CopilotAssignmentStatus String DEFAULT 'not_requested', CopilotAssignmentReason String, PrLinked UInt8 DEFAULT 0, PrNumber UInt32 DEFAULT 0, PrUrl String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (CreatedAt, AgentRunId)")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func (s *Service) ListRuns() []Run {
	if s.storeFactory != nil {
		return s.listRunsStoreBacked(context.Background())
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Run, 0, len(s.runs))
	for _, r := range s.runs {
		out = append(out, r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (s *Service) listRunsStoreBacked(ctx context.Context) []Run {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT Id, RuleName, Status, CreatedAt FROM sobs_agent_runs FINAL WHERE IsDeleted = 0 ORDER BY CreatedAt DESC LIMIT 100")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Run{}
	for rows.Next() {
		var run Run
		if err := rows.Scan(&run.ID, &run.Title, &run.Status, &run.CreatedAt); err != nil {
			return out
		}
		out = append(out, run)
	}
	return out
}

func (s *Service) CreateRun(title string) (Run, error) {
	if s.storeFactory != nil {
		return s.createRunStoreBacked(context.Background(), title)
	}
	if title == "" {
		return Run{}, errors.New("title is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextRun, 10)
	s.nextRun++
	r := Run{ID: id, Title: title, Status: "open", CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.runs[id] = r
	return r, nil
}

func (s *Service) createRunStoreBacked(ctx context.Context, title string) (Run, error) {
	if title == "" {
		return Run{}, errors.New("title is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Run{}, err
	}
	id := persist.NewID()
	createdAt := persist.RFC3339Now()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Run{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_agent_runs (Id, RuleId, RuleName, TriggerContext, Status, GuardDecision, DlpResult, Analysis, Suggestion, GithubIssueUrl, ErrorMessage, CreatedAt, CompletedAt, IsDismissed, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?), ?, ?, ?)", id, "manual", title, "{}", "open", "", "", "", "", "", "", createdAt, createdAt, 0, 0, persist.Version())
	if err != nil {
		return Run{}, err
	}
	return Run{ID: id, Title: title, Status: "open", CreatedAt: createdAt}, nil
}

func (s *Service) DismissRun(id string) (Run, bool) {
	if s.storeFactory != nil {
		return s.dismissRunStoreBacked(context.Background(), id)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.runs[id]
	if !ok {
		return Run{}, false
	}
	r.Status = "dismissed"
	s.runs[id] = r
	return r, true
}

func (s *Service) dismissRunStoreBacked(ctx context.Context, id string) (Run, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return Run{}, false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Run{}, false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT RuleName, CreatedAt FROM sobs_agent_runs FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return Run{}, false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return Run{}, false
	}
	var title string
	var createdAt string
	if err := rows.Scan(&title, &createdAt); err != nil {
		return Run{}, false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_agent_runs (Id, RuleId, RuleName, TriggerContext, Status, GuardDecision, DlpResult, Analysis, Suggestion, GithubIssueUrl, ErrorMessage, CreatedAt, CompletedAt, IsDismissed, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?), ?, ?, ?)", id, "manual", title, "{}", "dismissed", "", "", "", "", "", "", createdAt, persist.RFC3339Now(), 1, 0, persist.Version())
	if err != nil {
		return Run{}, false
	}
	return Run{ID: id, Title: title, Status: "dismissed", CreatedAt: createdAt}, true
}

func (s *Service) RaiseIssue(title, body string) (Issue, error) {
	if s.storeFactory != nil {
		return s.raiseIssueStoreBacked(context.Background(), title, body)
	}
	if title == "" {
		return Issue{}, errors.New("title is required")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextIssue, 10)
	s.nextIssue++
	iss := Issue{ID: id, Title: title, Body: body, CreatedAt: time.Now().UTC().Format(time.RFC3339)}
	s.issues[id] = iss
	return iss, nil
}

func (s *Service) raiseIssueStoreBacked(ctx context.Context, title, body string) (Issue, error) {
	if title == "" {
		return Issue{}, errors.New("title is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Issue{}, err
	}
	id := persist.NewID()
	createdAt := persist.RFC3339Now()
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Issue{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_github_work_items (Id, CreatedAt, CompletedAt, AgentRunId, AgentRuleId, AgentRuleName, AgentAction, ServiceName, AnomalyRuleId, AnomalyState, SignalSource, SignalName, SignalValue, GithubRepo, DedupKey, DedupDecision, DedupConfidence, IssueNumber, IssueUrl, CanonicalIssueNumber, CanonicalIssueUrl, RelatedIssueUrls, OccurrenceCount, IssueState, IssueTitle, AnalysisSummary, SuggestionSummary, IsDeleted, Version) VALUES (?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, createdAt, createdAt, "", "", "", "manual_issue", "", "", "", "", "", 0.0, "", strings.ToLower(strings.ReplaceAll(title, " ", "-")), "new_issue", 0.0, 0, "", 0, "", "[]", 1, "open", title, body, "", 0, persist.Version())
	if err != nil {
		return Issue{}, err
	}
	return Issue{ID: id, Title: title, Body: body, CreatedAt: createdAt}, nil
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
	rows, err := store.Query(ctx, "SELECT Id, Name, Description, TriggerType, TriggerRefId, TriggerState, Actions, RateLimitMinutes, IsEnabled, Version FROM sobs_agent_rules FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []Rule{}
	for rows.Next() {
		var rule Rule
		var actions string
		var enabled uint8
		var version uint64
		if err := rows.Scan(&rule.ID, &rule.Name, &rule.Description, &rule.TriggerType, &rule.TriggerRefID, &rule.TriggerState, &actions, &rule.RateLimitMinutes, &enabled, &version); err != nil {
			return out
		}
		rule.Actions = persist.ParseJSONStringSlice(actions)
		rule.Enabled = enabled == 1
		rule.CreatedAt = time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
		out = append(out, rule)
	}
	return out
}

func (s *Service) CreateRule(name, description, triggerType, triggerRefID, triggerState string, actions []string, rateLimitMinutes int) (Rule, error) {
	if s.storeFactory != nil {
		return s.createRuleStoreBacked(context.Background(), name, description, triggerType, triggerRefID, triggerState, actions, rateLimitMinutes)
	}
	if name == "" {
		return Rule{}, errors.New("name is required")
	}
	if triggerType == "" {
		triggerType = "manual"
	}
	if triggerState == "" {
		triggerState = "any"
	}
	if len(actions) == 0 {
		actions = []string{"analyze"}
	}
	if rateLimitMinutes < 1 {
		rateLimitMinutes = 1
	}
	if rateLimitMinutes > 10080 {
		rateLimitMinutes = 10080
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextRule, 10)
	s.nextRule++
	r := Rule{
		ID:               id,
		Name:             name,
		Description:      description,
		TriggerType:      triggerType,
		TriggerRefID:     triggerRefID,
		TriggerState:     triggerState,
		Actions:          actions,
		RateLimitMinutes: rateLimitMinutes,
		Enabled:          true,
		CreatedAt:        time.Now().UTC().Format(time.RFC3339),
	}
	s.rules[id] = r
	return r, nil
}

func (s *Service) createRuleStoreBacked(ctx context.Context, name, description, triggerType, triggerRefID, triggerState string, actions []string, rateLimitMinutes int) (Rule, error) {
	if name == "" {
		return Rule{}, errors.New("name is required")
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Rule{}, err
	}
	if triggerType == "" {
		triggerType = "manual"
	}
	if triggerState == "" {
		triggerState = "any"
	}
	if len(actions) == 0 {
		actions = []string{"analyze"}
	}
	if rateLimitMinutes < 1 {
		rateLimitMinutes = 1
	}
	id := persist.NewID()
	version := persist.Version()
	createdAt := time.Unix(0, int64(version)).UTC().Format(time.RFC3339)
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Rule{}, err
	}
	defer func() { _ = store.Close() }()
	_, err = store.Exec(ctx, "INSERT INTO sobs_agent_rules (Id, Name, Description, TriggerType, TriggerRefId, TriggerState, Actions, RateLimitMinutes, IsEnabled, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, description, triggerType, triggerRefID, triggerState, persist.JSONString(actions), rateLimitMinutes, 1, 0, version)
	if err != nil {
		return Rule{}, err
	}
	return Rule{ID: id, Name: name, Description: description, TriggerType: triggerType, TriggerRefID: triggerRefID, TriggerState: triggerState, Actions: actions, RateLimitMinutes: rateLimitMinutes, Enabled: true, CreatedAt: createdAt}, nil
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
	rows, err := store.Query(ctx, "SELECT Name, Description, TriggerType, TriggerRefId, TriggerState, Actions, RateLimitMinutes, IsEnabled FROM sobs_agent_rules FINAL WHERE IsDeleted = 0 AND Id = ? LIMIT 1", id)
	if err != nil {
		return false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return false
	}
	var name, description, triggerType, triggerRefID, triggerState, actions string
	var rateLimitMinutes uint32
	var enabled uint8
	if err := rows.Scan(&name, &description, &triggerType, &triggerRefID, &triggerState, &actions, &rateLimitMinutes, &enabled); err != nil {
		return false
	}
	_, err = store.Exec(ctx, "INSERT INTO sobs_agent_rules (Id, Name, Description, TriggerType, TriggerRefId, TriggerState, Actions, RateLimitMinutes, IsEnabled, IsDeleted, Version) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", id, name, description, triggerType, triggerRefID, triggerState, actions, rateLimitMinutes, enabled, 1, persist.Version())
	return err == nil
}
