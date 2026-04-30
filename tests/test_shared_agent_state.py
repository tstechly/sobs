from shared.agent_state import (
    _agent_rule_last_run_ts,
    _count_active_copilot_assignments,
    _count_copilot_assignments_last_hour,
    _count_github_issues_last_hour,
    _extract_trigger_service_name,
    _load_agent_rule,
    _load_agent_rules,
    _load_agent_runs,
    _resolve_agent_github_target,
)


class _FakeResult:
    def __init__(self, row=None, rows=None):
        self._row = row
        self._rows = rows or []

    def fetchone(self):
        return self._row

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, mappings):
        self.mappings = mappings
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        for matcher, result in self.mappings:
            if matcher in query:
                if isinstance(result, dict) and "row" in result:
                    return _FakeResult(row=result.get("row"), rows=result.get("rows", []))
                return _FakeResult(rows=result)
        return _FakeResult()


def test_shared_agent_state_load_rules_and_single_rule_map_fields():
    row = {
        "Id": "rule-1",
        "Name": "Rule 1",
        "Description": "desc",
        "TriggerType": "manual",
        "TriggerRefId": "ref-1",
        "TriggerState": "warning",
        "Actions": "analyze, github_issue ,",
        "RateLimitMinutes": 15,
        "IsEnabled": 1,
    }
    db = _FakeDb(
        [
            ("FROM sobs_agent_rules FINAL WHERE IsDeleted=0 ORDER BY Name", [row]),
            ("FROM sobs_agent_rules FINAL WHERE IsDeleted=0 AND Id=? LIMIT 1", {"row": row}),
        ]
    )

    rules = _load_agent_rules(db)
    expected_rule = {
        "id": "rule-1",
        "name": "Rule 1",
        "description": "desc",
        "trigger_type": "manual",
        "trigger_ref_id": "ref-1",
        "trigger_state": "warning",
        "actions": ["analyze", "github_issue"],
        "rate_limit_minutes": 15,
        "is_enabled": True,
    }
    assert rules == [expected_rule]
    assert _load_agent_rule(db, "rule-1") == expected_rule
    assert _load_agent_rule(_FakeDb([]), "missing") is None


def test_shared_agent_state_load_runs_and_counters_cover_zero_and_nonzero_paths():
    db = _FakeDb(
        [
            (
                "FROM sobs_agent_runs FINAL WHERE IsDeleted=0 ORDER BY CreatedAt DESC",
                [
                    {
                        "Id": "run-1",
                        "RuleId": "rule-1",
                        "RuleName": "Rule 1",
                        "TriggerContext": "{}",
                        "Status": "completed",
                        "GuardDecision": "allowed",
                        "DlpResult": "clean",
                        "Analysis": "analysis",
                        "Suggestion": "suggestion",
                        "GithubIssueUrl": "https://github.com/octo/repo/issues/1",
                        "ErrorMessage": "",
                        "CreatedAt": "2026-04-30 10:00:00",
                        "CompletedAt": "2026-04-30 10:01:00",
                        "IsDismissed": 0,
                    }
                ],
            ),
            ("max(toUnixTimestamp64Milli(CreatedAt)) AS t", {"row": {"t": 12345}}),
            ("GithubIssueUrl != ''", {"row": {"c": 4}}),
            ("CopilotAssignmentRequestedAt >= ?", {"row": {"c": 3}}),
            ("CopilotAssignmentStatus IN ('requested', 'active')", {"row": {"c": 2}}),
        ]
    )

    runs = _load_agent_runs(db, limit=5)
    assert runs == [
        {
            "id": "run-1",
            "rule_id": "rule-1",
            "rule_name": "Rule 1",
            "trigger_context": "{}",
            "status": "completed",
            "guard_decision": "allowed",
            "dlp_result": "clean",
            "analysis": "analysis",
            "suggestion": "suggestion",
            "github_issue_url": "https://github.com/octo/repo/issues/1",
            "error_message": "",
            "created_at": "2026-04-30 10:00:00",
            "completed_at": "2026-04-30 10:01:00",
            "is_dismissed": False,
        }
    ]
    assert _agent_rule_last_run_ts(db, "rule-1") == 12.345
    assert _count_github_issues_last_hour(db) == 4
    assert _count_copilot_assignments_last_hour(db, now=lambda: 10.0) == 3
    assert db.calls[-1][1] == [0]
    assert _count_active_copilot_assignments(db) == 2

    empty_db = _FakeDb(
        [
            ("max(toUnixTimestamp64Milli(CreatedAt)) AS t", {"row": {"t": 0}}),
            ("GithubIssueUrl != ''", {"row": None}),
            ("CopilotAssignmentRequestedAt >= ?", {"row": None}),
            ("CopilotAssignmentStatus IN ('requested', 'active')", {"row": None}),
        ]
    )
    assert _agent_rule_last_run_ts(empty_db, "rule-1") == 0.0
    assert _count_github_issues_last_hour(empty_db) == 0
    assert _count_copilot_assignments_last_hour(empty_db, now=lambda: 4000.0) == 0
    assert _count_active_copilot_assignments(empty_db) == 0


def test_shared_agent_state_extract_trigger_service_name_covers_sources():
    assert (
        _extract_trigger_service_name(
            {"service": "checkout"},
            safe_json_loads=lambda _value, _default: {},
        )
        == "checkout"
    )
    assert (
        _extract_trigger_service_name(
            {"extra": {"service_name": "payments"}},
            safe_json_loads=lambda _value, _default: {},
        )
        == "payments"
    )
    assert (
        _extract_trigger_service_name(
            {"extra": '{"ServiceName":"rum-ui"}'},
            safe_json_loads=lambda value, _default: {"ServiceName": "rum-ui"} if value else {},
        )
        == "rum-ui"
    )
    assert (
        _extract_trigger_service_name(
            {"extra": "[]"},
            safe_json_loads=lambda _value, _default: [],
        )
        == ""
    )


def test_shared_agent_state_resolve_github_target_covers_priority_order():
    db = _FakeDb(
        [
            (
                "FROM sobs_apps FINAL",
                {"row": {"RepoUrl": "https://github.com/octo/checkout-service"}},
            )
        ]
    )

    target = _resolve_agent_github_target(
        db,
        {"ai.github_repo": "octo/default", "ai.github_token": "default-token"},
        {"extra": {"service": "checkout"}},
        extract_trigger_service_name=lambda trigger_context: "checkout",
        parse_github_repo_owner_name=lambda value: (
            ("octo", "checkout-service") if "checkout-service" in value else ("", "")
        ),
        load_repo_scoped_github_token=lambda _db, owner, repo: f"scoped:{owner}/{repo}",
    )
    assert target == ("octo/checkout-service", "scoped:octo/checkout-service")

    fallback_target = _resolve_agent_github_target(
        _FakeDb([]),
        {"ai.github_repo": "https://github.com/acme/demo", "ai.github_token": "default-token"},
        {},
        extract_trigger_service_name=lambda _trigger_context: "",
        parse_github_repo_owner_name=lambda value: ("acme", "demo") if "demo" in value else ("", ""),
        load_repo_scoped_github_token=lambda _db, _owner, _repo: "",
    )
    assert fallback_target == ("acme/demo", "default-token")

    slash_target = _resolve_agent_github_target(
        _FakeDb([]),
        {"ai.github_repo": "acme/demo", "ai.github_token": "default-token"},
        {},
        extract_trigger_service_name=lambda _trigger_context: "",
        parse_github_repo_owner_name=lambda _value: ("", ""),
        load_repo_scoped_github_token=lambda _db, _owner, _repo: "scoped-fallback",
    )
    assert slash_target == ("acme/demo", "scoped-fallback")

    unresolved_target = _resolve_agent_github_target(
        _FakeDb([]),
        {"ai.github_repo": "not-a-repo", "ai.github_token": "default-token"},
        {},
        extract_trigger_service_name=lambda _trigger_context: "",
        parse_github_repo_owner_name=lambda _value: ("", ""),
        load_repo_scoped_github_token=lambda _db, _owner, _repo: "",
    )
    assert unresolved_target == ("not-a-repo", "default-token")

    empty_target = _resolve_agent_github_target(
        _FakeDb([]),
        {"ai.github_token": "default-token"},
        {},
        extract_trigger_service_name=lambda _trigger_context: "",
        parse_github_repo_owner_name=lambda _value: ("", ""),
        load_repo_scoped_github_token=lambda _db, _owner, _repo: "",
    )
    assert empty_target == ("", "default-token")
