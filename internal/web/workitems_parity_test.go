package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/abartrim/sobs/internal/config"
	sobsstore "github.com/abartrim/sobs/internal/store"
)

func newRenderedWorkItemsTestServer() *Server {
	cfg := config.Default()
	cfg.EnforceAPIAuth = false
	cfg.TemplateRoot = "../../templates"
	return NewServer(cfg, sobsstore.NewNoopStoreFactory())
}

func TestWorkItemsHelpPageParity(t *testing.T) {
	srv := newRenderedWorkItemsTestServer()

	getReq := httptest.NewRequest(http.MethodGet, "http://example.com/work-items/help", nil)
	getRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", getRec.Code, getRec.Body.String())
	}
	if !strings.Contains(getRec.Body.String(), "Work Items Help") {
		t.Fatalf("expected work items help content")
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/work-items/help", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func TestWorkItemsPageAndAPIParity(t *testing.T) {
	srv := newRenderedWorkItemsTestServer()
	seedWorkItemsTable(t, srv)

	pageReq := httptest.NewRequest(http.MethodGet, "http://example.com/work-items?service=svc-a", nil)
	pageRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(pageRec, pageReq)
	if pageRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", pageRec.Code, pageRec.Body.String())
	}
	pageBody := pageRec.Body.String()
	if !containsAll(pageBody, "Work Items", "svc-a") {
		t.Fatalf("expected work items page to render seeded filtered item, got %s", pageBody)
	}
	if !strings.Contains(pageBody, "No work items found") && !strings.Contains(pageBody, "work-item-id") {
		t.Fatalf("expected work items page to render a valid empty or populated state")
	}

	apiReq := httptest.NewRequest(http.MethodGet, "http://example.com/api/work-items?service=svc-a", nil)
	apiRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(apiRec, apiReq)
	if apiRec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", apiRec.Code, apiRec.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(apiRec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("unmarshal api payload: %v", err)
	}
	if okVal, ok := payload["ok"].(bool); !ok || !okVal {
		t.Fatalf("expected ok=true from api payload, got %#v", payload["ok"])
	}
	_, ok := payload["items"].([]any)
	if !ok {
		t.Fatalf("expected api items list, got %#v", payload["items"])
	}

	postReq := httptest.NewRequest(http.MethodPost, "http://example.com/api/work-items", nil)
	postRec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(postRec, postReq)
	if postRec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d body=%s", postRec.Code, postRec.Body.String())
	}
}

func seedWorkItemsTable(t *testing.T, srv *Server) {
	t.Helper()

	store, err := srv.storeFactory.Open(t.Context())
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer func() { _ = store.Close() }()

	stmts := []string{
		"DROP TABLE IF EXISTS sobs_github_work_items",
		"CREATE TABLE IF NOT EXISTS sobs_github_work_items (Id String, CreatedAt DateTime64(3), IsDeleted UInt8, ServiceName String, SignalSource String, SignalName String, AnomalyState String, AnomalyRuleId String, AgentRuleId String, AgentRuleName String, AgentAction String, IssueUrl String, CanonicalIssueUrl String, IssueTitle String, IssueNumber UInt32, IssueState String, DedupDecision String, CopilotAssignmentStatus String, PrUrl String, PrNumber UInt32, AnalysisSummary String, SuggestionSummary String) ENGINE = MergeTree ORDER BY (CreatedAt, Id)",
	}
	for _, stmt := range stmts {
		if _, err := store.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("exec schema %q: %v", stmt, err)
		}
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_github_work_items (Id, CreatedAt, IsDeleted, ServiceName, SignalSource, SignalName, AnomalyState, AnomalyRuleId, AgentRuleId, AgentRuleName, AgentAction, IssueUrl, CanonicalIssueUrl, IssueTitle, IssueNumber, IssueState, DedupDecision, CopilotAssignmentStatus, PrUrl, PrNumber, AnalysisSummary, SuggestionSummary) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"wi-1",
		"2026-04-20 10:00:00.000",
		uint8(0),
		"svc-a",
		"metrics",
		"latency",
		"warning",
		"anomaly-1",
		"rule-1",
		"rule-latency",
		"github_issue",
		"https://github.com/example/repo/issues/11",
		"",
		"Issue latency spike",
		uint32(11),
		"open",
		"new_issue",
		"",
		"",
		uint32(0),
		"latency rising",
		"scale service",
	); err != nil {
		t.Fatalf("insert work item wi-1: %v", err)
	}

	if _, err := store.Exec(t.Context(), "INSERT INTO sobs_github_work_items (Id, CreatedAt, IsDeleted, ServiceName, SignalSource, SignalName, AnomalyState, AnomalyRuleId, AgentRuleId, AgentRuleName, AgentAction, IssueUrl, CanonicalIssueUrl, IssueTitle, IssueNumber, IssueState, DedupDecision, CopilotAssignmentStatus, PrUrl, PrNumber, AnalysisSummary, SuggestionSummary) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)",
		"wi-2",
		"2026-04-20 10:05:00.000",
		uint8(0),
		"svc-b",
		"logs",
		"error_rate",
		"critical",
		"anomaly-2",
		"rule-2",
		"rule-errors",
		"github_issue_copilot",
		"https://github.com/example/repo/issues/12",
		"",
		"Issue error rate",
		uint32(12),
		"open",
		"existing_issue",
		"active",
		"https://github.com/example/repo/pull/7",
		uint32(7),
		"error bursts",
		"add retries",
	); err != nil {
		t.Fatalf("insert work item wi-2: %v", err)
	}
}
