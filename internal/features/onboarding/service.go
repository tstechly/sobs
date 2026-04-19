package onboarding

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/abartrim/sobs/internal/extensionpoints"
	"github.com/abartrim/sobs/internal/features/persist"
)

type Repo struct {
	AppID   string `json:"app_id"`
	Name    string `json:"name"`
	Slug    string `json:"slug"`
	RepoURL string `json:"repo_url"`
	Owner   string `json:"owner"`
	Repo    string `json:"repo"`
}

type Issue struct {
	Number int    `json:"issue_number"`
	Title  string `json:"issue_title"`
	State  string `json:"issue_state"`
	URL    string `json:"issue_url"`
}

type Service struct {
	mu         sync.RWMutex
	repos      map[string]Repo
	issuesByRepo map[string][]Issue
	nextAppID  int64
	nextIssue  int
	storeFactory extensionpoints.StoreFactory
	schemaOnce   sync.Once
	schemaErr    error
}

func NewService() *Service {
	return &Service{repos: map[string]Repo{}, issuesByRepo: map[string][]Issue{}, nextAppID: 1, nextIssue: 1}
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
		_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_apps (Id String, Name String, Slug String, OwnerTeam String, RepoUrl String, DefaultEnvironment String, Enabled UInt8 DEFAULT 1, MetadataJson String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0, CreatedAt DateTime64(3) DEFAULT now64(3), UpdatedAt DateTime64(3) DEFAULT now64(3)) ENGINE = ReplacingMergeTree(Version) ORDER BY (Slug, Id)")
		if err == nil {
			_, err = store.Exec(ctx, "CREATE TABLE IF NOT EXISTS sobs_github_work_items (Id String, CreatedAt DateTime64(3), CompletedAt DateTime64(3), AgentRunId String, AgentRuleId String, AgentRuleName String, AgentAction String, ServiceName String, AnomalyRuleId String, AnomalyState String, SignalSource String, SignalName String, SignalValue Float64, GithubRepo String, DedupKey String, DedupDecision String DEFAULT 'new_issue', DedupConfidence Float64 DEFAULT 0, IssueNumber UInt32 DEFAULT 0, IssueUrl String, CanonicalIssueNumber UInt32 DEFAULT 0, CanonicalIssueUrl String, RelatedIssueUrls String, OccurrenceCount UInt32 DEFAULT 1, IssueState String DEFAULT '', IssueTitle String, AnalysisSummary String, SuggestionSummary String, CopilotAssignmentRequestedAt UInt64 DEFAULT 0, CopilotAssignmentStatus String DEFAULT 'not_requested', CopilotAssignmentReason String, PrLinked UInt8 DEFAULT 0, PrNumber UInt32 DEFAULT 0, PrUrl String, IsDeleted UInt8 DEFAULT 0, Version UInt64 DEFAULT 0) ENGINE = ReplacingMergeTree(Version) ORDER BY (CreatedAt, AgentRunId)")
		}
		s.schemaErr = err
	})
	return s.schemaErr
}

func parseOwnerRepo(input string) (string, string) {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "https://github.com/")
	s = strings.TrimPrefix(s, "http://github.com/")
	s = strings.Trim(s, "/")
	parts := strings.Split(s, "/")
	if len(parts) < 2 {
		return "", ""
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
}

func slugify(name string) string {
	s := strings.ToLower(strings.TrimSpace(name))
	s = strings.ReplaceAll(s, " ", "-")
	if s == "" {
		s = "app"
	}
	return s
}

func (s *Service) CreateRepo(name, slug, repoURL, owner, repo string) (Repo, error) {
	if s.storeFactory != nil {
		return s.createRepoStoreBacked(context.Background(), name, slug, repoURL, owner, repo)
	}
	if strings.TrimSpace(name) == "" {
		return Repo{}, errors.New("App name and repository are required")
	}
	if strings.TrimSpace(repoURL) == "" && (strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "") {
		return Repo{}, errors.New("App name and repository are required")
	}
	if owner == "" || repo == "" {
		o, r := parseOwnerRepo(repoURL)
		owner, repo = o, r
	}
	if owner == "" || repo == "" {
		return Repo{}, errors.New("Enter a valid GitHub owner and repository name")
	}
	if repoURL == "" {
		repoURL = "https://github.com/" + owner + "/" + repo
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	id := strconv.FormatInt(s.nextAppID, 10)
	s.nextAppID++
	if strings.TrimSpace(slug) == "" {
		slug = slugify(name)
	}
	r := Repo{AppID: id, Name: name, Slug: slug, RepoURL: repoURL, Owner: owner, Repo: repo}
	s.repos[id] = r
	return r, nil
}

func (s *Service) createRepoStoreBacked(ctx context.Context, name, slug, repoURL, owner, repo string) (Repo, error) {
	if strings.TrimSpace(name) == "" {
		return Repo{}, errors.New("App name and repository are required")
	}
	if strings.TrimSpace(repoURL) == "" && (strings.TrimSpace(owner) == "" || strings.TrimSpace(repo) == "") {
		return Repo{}, errors.New("App name and repository are required")
	}
	if owner == "" || repo == "" {
		owner, repo = parseOwnerRepo(repoURL)
	}
	if owner == "" || repo == "" {
		return Repo{}, errors.New("Enter a valid GitHub owner and repository name")
	}
	if repoURL == "" {
		repoURL = "https://github.com/" + owner + "/" + repo
	}
	if strings.TrimSpace(slug) == "" {
		slug = slugify(name)
	}
	if err := s.ensureSchema(ctx); err != nil {
		return Repo{}, err
	}
	id := "1"
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return Repo{}, err
	}
	defer func() { _ = store.Close() }()
	nextRaw, ok, nextErr := persist.GetAppSetting(ctx, s.storeFactory, "onboarding.next_app_id")
	if nextErr == nil {
		nextID := 1
		if ok {
			if parsed, parseErr := strconv.Atoi(strings.TrimSpace(nextRaw)); parseErr == nil && parsed > 0 {
				nextID = parsed
			}
		}
		id = strconv.Itoa(nextID)
		_ = persist.SetAppSetting(ctx, s.storeFactory, "onboarding.next_app_id", strconv.Itoa(nextID+1))
	}
	now := persist.RFC3339Now()
	_, err = store.Exec(ctx, "INSERT INTO sobs_apps (Id, Name, Slug, OwnerTeam, RepoUrl, DefaultEnvironment, Enabled, MetadataJson, IsDeleted, Version, CreatedAt, UpdatedAt) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?))", id, strings.TrimSpace(name), slug, owner, repoURL, "default", 1, persist.JSONString(map[string]any{"repo_owner": owner, "repo_name": repo}), 0, persist.Version(), now, now)
	if err != nil {
		return Repo{}, err
	}
	return Repo{AppID: id, Name: strings.TrimSpace(name), Slug: slug, RepoURL: repoURL, Owner: owner, Repo: repo}, nil
}

func (s *Service) ImportRepo(repoURL, owner, repo string) (map[string]any, error) {
	if owner == "" || repo == "" {
		o, r := parseOwnerRepo(repoURL)
		owner, repo = o, r
	}
	if owner == "" || repo == "" {
		return nil, errors.New("Enter a valid GitHub owner and repository name")
	}
	full := owner + "/" + repo
	return map[string]any{"ok": true, "owner": owner, "repo": repo, "full_name": full, "repo_url": "https://github.com/" + full, "name": repo, "slug": slugify(repo), "default_branch": "main", "visibility": "public", "description": ""}, nil
}

func (s *Service) ListRepos(owner string) []map[string]any {
	if s.storeFactory != nil {
		return s.listReposStoreBacked(context.Background(), owner)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := []map[string]any{}
	for _, r := range s.repos {
		if owner != "" && !strings.EqualFold(r.Owner, owner) {
			continue
		}
		out = append(out, map[string]any{"name": r.Repo, "full_name": r.Owner + "/" + r.Repo, "repo_url": r.RepoURL, "private": false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	return out
}

func (s *Service) listReposStoreBacked(ctx context.Context, owner string) []map[string]any {
	if err := s.ensureSchema(ctx); err != nil {
		return nil
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return nil
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT RepoUrl FROM sobs_apps FINAL WHERE IsDeleted = 0 ORDER BY Name")
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	out := []map[string]any{}
	for rows.Next() {
		var repoURL string
		if err := rows.Scan(&repoURL); err != nil {
			return out
		}
		o, r := parseOwnerRepo(repoURL)
		if o == "" || r == "" {
			continue
		}
		if owner != "" && !strings.EqualFold(owner, o) {
			continue
		}
		out = append(out, map[string]any{"name": r, "full_name": o + "/" + r, "repo_url": repoURL, "private": false})
	}
	sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) < out[j]["name"].(string) })
	return out
}

func (s *Service) InspectRepo(appID, repoParam string) (map[string]any, int, string) {
	if s.storeFactory != nil {
		return s.inspectRepoStoreBacked(context.Background(), appID, repoParam)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	owner := ""
	repo := ""
	if appID != "" {
		r, ok := s.repos[appID]
		if !ok {
			return nil, 404, "App not found"
		}
		owner, repo = r.Owner, r.Repo
	} else {
		owner, repo = parseOwnerRepo(repoParam)
		if owner == "" || repo == "" {
			return nil, 400, "app_id or repo parameter required"
		}
	}
	return map[string]any{"ok": true, "owner": owner, "repo": repo, "has_github_actions": true, "sobs_ci_found": false, "sobs_otel_found": false, "copilot_available": false, "workflow_files": []string{"ci.yml"}, "error": ""}, 200, ""
}

func (s *Service) inspectRepoStoreBacked(ctx context.Context, appID, repoParam string) (map[string]any, int, string) {
	owner := ""
	repo := ""
	if appID != "" {
		var ok bool
		owner, repo, ok = s.repoForAppID(ctx, appID)
		if !ok {
			return nil, 404, "App not found"
		}
	} else {
		owner, repo = parseOwnerRepo(repoParam)
		if owner == "" || repo == "" {
			return nil, 400, "app_id or repo parameter required"
		}
	}
	return map[string]any{"ok": true, "owner": owner, "repo": repo, "has_github_actions": true, "sobs_ci_found": false, "sobs_otel_found": false, "copilot_available": false, "workflow_files": []string{"ci.yml"}, "error": ""}, 200, ""
}

func (s *Service) CreateIssues(appID, repoParam string, createCI, createOTEL bool) (map[string]any, int, string) {
	if s.storeFactory != nil {
		return s.createIssuesStoreBacked(context.Background(), appID, repoParam, createCI, createOTEL)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	owner := ""
	repo := ""
	if appID != "" {
		r, ok := s.repos[appID]
		if !ok {
			return nil, 404, "App not found"
		}
		owner, repo = r.Owner, r.Repo
	} else {
		owner, repo = parseOwnerRepo(repoParam)
	}
	if owner == "" || repo == "" {
		return nil, 400, "app_id or repo parameter required"
	}
	if !createCI && !createOTEL {
		return nil, 400, "Select at least one issue type or enable realtime support"
	}
	key := owner + "/" + repo
	issues := s.issuesByRepo[key]
	res := map[string]any{"ok": true, "ci_issue": nil, "otel_issue": nil, "realtime": nil}
	mkIssue := func(title string) Issue {
		n := s.nextIssue
		s.nextIssue++
		return Issue{Number: n, Title: title, State: "open", URL: "https://github.com/" + key + "/issues/" + strconv.Itoa(n)}
	}
	if createCI {
		iss := mkIssue("Sobs CI Metadata Setup")
		issues = append(issues, iss)
		res["ci_issue"] = iss
	}
	if createOTEL {
		iss := mkIssue("OTEL & RUM Telemetry Audit")
		issues = append(issues, iss)
		res["otel_issue"] = iss
	}
	s.issuesByRepo[key] = issues
	return res, 200, ""
}

func (s *Service) createIssuesStoreBacked(ctx context.Context, appID, repoParam string, createCI, createOTEL bool) (map[string]any, int, string) {
	owner := ""
	repo := ""
	if appID != "" {
		var ok bool
		owner, repo, ok = s.repoForAppID(ctx, appID)
		if !ok {
			return nil, 404, "App not found"
		}
	} else {
		owner, repo = parseOwnerRepo(repoParam)
	}
	if owner == "" || repo == "" {
		return nil, 400, "app_id or repo parameter required"
	}
	if !createCI && !createOTEL {
		return nil, 400, "Select at least one issue type or enable realtime support"
	}
	key := owner + "/" + repo
	res := map[string]any{"ok": true, "ci_issue": nil, "otel_issue": nil, "realtime": nil}
	nextNumber := s.nextIssueNumber(ctx, key)
	if createCI {
		iss := Issue{Number: nextNumber, Title: "Sobs CI Metadata Setup", State: "open", URL: "https://github.com/" + key + "/issues/" + strconv.Itoa(nextNumber)}
		_ = s.persistIssue(ctx, key, repo, iss, "ci")
		res["ci_issue"] = iss
		nextNumber++
	}
	if createOTEL {
		iss := Issue{Number: nextNumber, Title: "OTEL & RUM Telemetry Audit", State: "open", URL: "https://github.com/" + key + "/issues/" + strconv.Itoa(nextNumber)}
		_ = s.persistIssue(ctx, key, repo, iss, "otel")
		res["otel_issue"] = iss
	}
	return res, 200, ""
}

func (s *Service) repoForAppID(ctx context.Context, appID string) (string, string, bool) {
	if err := s.ensureSchema(ctx); err != nil {
		return "", "", false
	}
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return "", "", false
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT RepoUrl FROM sobs_apps FINAL WHERE Id = ? AND IsDeleted = 0 LIMIT 1", appID)
	if err != nil {
		return "", "", false
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return "", "", false
	}
	var repoURL string
	if err := rows.Scan(&repoURL); err != nil {
		return "", "", false
	}
	owner, repo := parseOwnerRepo(repoURL)
	return owner, repo, owner != "" && repo != ""
}

func (s *Service) nextIssueNumber(ctx context.Context, githubRepo string) int {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return 1
	}
	defer func() { _ = store.Close() }()
	rows, err := store.Query(ctx, "SELECT count() FROM sobs_github_work_items FINAL WHERE GithubRepo = ? AND IsDeleted = 0", githubRepo)
	if err != nil {
		return 1
	}
	defer func() { _ = rows.Close() }()
	if !rows.Next() {
		return 1
	}
	var count uint64
	if err := rows.Scan(&count); err != nil {
		return 1
	}
	return int(count) + 1
}

func (s *Service) persistIssue(ctx context.Context, githubRepo, serviceName string, iss Issue, issueType string) error {
	store, err := persist.Open(ctx, s.storeFactory)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	now := persist.RFC3339Now()
	_, err = store.Exec(ctx, "INSERT INTO sobs_github_work_items (Id, CreatedAt, CompletedAt, AgentRunId, AgentRuleId, AgentRuleName, AgentAction, ServiceName, AnomalyRuleId, AnomalyState, SignalSource, SignalName, SignalValue, GithubRepo, DedupKey, DedupDecision, DedupConfidence, IssueNumber, IssueUrl, CanonicalIssueNumber, CanonicalIssueUrl, RelatedIssueUrls, OccurrenceCount, IssueState, IssueTitle, AnalysisSummary, SuggestionSummary, CopilotAssignmentRequestedAt, CopilotAssignmentStatus, CopilotAssignmentReason, PrLinked, PrNumber, PrUrl, IsDeleted, Version) VALUES (?, parseDateTime64BestEffort(?), parseDateTime64BestEffort(?), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", persist.NewID(), now, now, "", "", "Onboarding Wizard", "onboarding_"+issueType, serviceName, "", "", "", "", 0.0, githubRepo, "", "new_issue", 0.0, iss.Number, iss.URL, iss.Number, iss.URL, "[]", 1, iss.State, iss.Title, "Sobs onboarding wizard issue.", "", 0, "not_requested", "", 0, 0, "", 0, persist.Version())
	return err
}
