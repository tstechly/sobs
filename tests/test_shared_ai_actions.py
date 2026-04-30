import secrets

from shared.ai_actions import (
    _ai_action_token_secret,
    _build_client_action,
    _decode_ai_action_token,
    _encode_ai_action_token,
    _issue_ai_action_token,
    _normalize_generic_ui_action_tool_call,
    _suggest_chart_dashboard_pivot_tool,
)


def test_shared_ai_actions_token_secret_encode_decode_and_issue_cover_valid_and_invalid_paths():
    assert _ai_action_token_secret("secret") == "secret"
    assert _ai_action_token_secret("") == "sobs-dev-secret-key"

    payload = {"exp": 200, "value": "ok"}
    token = _encode_ai_action_token(payload, ai_action_token_secret="secret")
    assert (
        _decode_ai_action_token(
            token,
            ai_action_token_secret="secret",
            compare_digest=secrets.compare_digest,
            now=100,
        )
        == payload
    )
    assert (
        _decode_ai_action_token(
            "bad-token",
            ai_action_token_secret="secret",
            compare_digest=secrets.compare_digest,
            now=100,
        )
        is None
    )
    assert (
        _decode_ai_action_token(
            token + "bad",
            ai_action_token_secret="secret",
            compare_digest=secrets.compare_digest,
            now=100,
        )
        is None
    )
    expired = _encode_ai_action_token({"exp": 10, "value": "old"}, ai_action_token_secret="secret")
    assert (
        _decode_ai_action_token(
            expired,
            ai_action_token_secret="secret",
            compare_digest=secrets.compare_digest,
            now=10,
        )
        is None
    )
    non_dict = _encode_ai_action_token(["bad"], ai_action_token_secret="secret")
    assert (
        _decode_ai_action_token(
            non_dict,
            ai_action_token_secret="secret",
            compare_digest=secrets.compare_digest,
            now=1,
        )
        is None
    )
    bad_json_body = "bm90LWpzb24"
    bad_json_sig = __import__("hashlib").sha256(("secret." + bad_json_body).encode("utf-8")).hexdigest()
    assert (
        _decode_ai_action_token(
            f"{bad_json_body}.{bad_json_sig}",
            ai_action_token_secret="secret",
            compare_digest=secrets.compare_digest,
            now=1,
        )
        is None
    )

    issued_payloads = []
    issued_token = _issue_ai_action_token(
        action_id="logs.filter.apply_sql",
        target_page="/logs",
        action={"type": "apply_sql_filter"},
        requires_confirmation=False,
        chat_id="chat-1",
        turn_id="turn-1",
        now=50,
        ai_action_token_ttl_seconds=30,
        encode_ai_action_token=lambda payload: issued_payloads.append(payload) or "issued",
    )
    assert issued_token == "issued"
    assert issued_payloads == [
        {
            "v": 1,
            "iat": 50,
            "exp": 80,
            "action_id": "logs.filter.apply_sql",
            "target_page": "/logs",
            "action": {"type": "apply_sql_filter"},
            "requires_confirmation": False,
            "chat_id": "chat-1",
            "turn_id": "turn-1",
        }
    ]


def test_shared_ai_actions_build_client_action_sanitizes_nested_payloads():
    assert _build_client_action("", {}) is None
    assert _build_client_action("navigate", []) is None

    action = _build_client_action(
        "navigate",
        {
            "target_page": " /ai ",
            "deep": {"a": {"b": {"c": {"d": "too-deep"}}}},
            "items": list(range(105)),
            "long": "x" * 5000,
            "": "skip",
        },
    )
    assert action == {
        "type": "navigate",
        "target_page": "/ai",
        "deep": {"a": {"b": {"c": {"d": None}}}},
        "items": list(range(100)),
        "long": "x" * 4096,
    }

    action_with_misc = _build_client_action(
        "navigate",
        {
            "tuple_values": (1, object()),
            "nested": {" ": "skip", "ok": object()},
        },
    )
    assert action_with_misc == {
        "type": "navigate",
        "tuple_values": [1, None],
        "nested": {"ok": None},
    }


def test_shared_ai_actions_normalize_generic_ui_action_call_covers_supported_and_unsupported_paths():
    def helper_action_manifest_for_page(page):
        manifests = {
            "/": [
                {
                    "action_id": "summary.nav.ai",
                    "action_type": "navigate",
                    "implemented": True,
                    "requires_confirmation": False,
                    "label": "Open AI",
                    "arguments": {"target_page": "/ai"},
                }
            ],
            "/ai": [
                {
                    "action_id": "ai.filter.apply",
                    "action_type": "apply_form_filters",
                    "implemented": True,
                    "requires_confirmation": False,
                    "label": "Apply AI Filters",
                    "arguments": {"filter_fields": ["hours", "model"], "submit": False},
                },
                {
                    "action_id": "ai.filter.sql",
                    "action_type": "apply_sql_filter",
                    "implemented": True,
                    "requires_confirmation": False,
                    "label": "Apply SQL Filter",
                    "arguments": {},
                },
            ],
            "/dashboards": [
                {
                    "action_id": "dashboards.modal.new.open",
                    "action_type": "open_modal",
                    "implemented": True,
                    "requires_confirmation": False,
                    "label": "Open Dashboard Modal",
                    "arguments": {"modal": "new-dashboard"},
                }
            ],
        }
        return manifests.get(page, [])

    assert (
        _normalize_generic_ui_action_tool_call(
            {},
            "/ai",
            helper_action_manifest_for_page=helper_action_manifest_for_page,
            build_client_action=_build_client_action,
        )
        is None
    )
    assert (
        _normalize_generic_ui_action_tool_call(
            {"action_id": "missing", "target_page": "/ai", "arguments": {}, "notes": "Nope"},
            "/ai",
            helper_action_manifest_for_page=helper_action_manifest_for_page,
            build_client_action=_build_client_action,
        )["unsupported"]
        is True
    )

    cross_page = _normalize_generic_ui_action_tool_call(
        {"action_id": "summary.nav.ai", "target_page": "/ai", "arguments": {}, "notes": "Navigate"},
        "/",
        helper_action_manifest_for_page=helper_action_manifest_for_page,
        build_client_action=_build_client_action,
    )
    assert cross_page == {
        "tool": "propose_ui_action",
        "action_id": "summary.nav.ai",
        "summary": "Navigate",
        "requires_confirmation": True,
        "unsupported": False,
        "action": {"type": "navigate", "target_page": "/ai"},
    }

    rejected_filters = _normalize_generic_ui_action_tool_call(
        {
            "action_id": "ai.filter.apply",
            "target_page": "/ai",
            "arguments": {"filters": {"chart": "latency"}},
            "notes": "Reject unknown filters",
        },
        "/ai",
        helper_action_manifest_for_page=helper_action_manifest_for_page,
        build_client_action=_build_client_action,
    )
    assert rejected_filters == {
        "tool": "propose_ui_action",
        "action_id": "ai.filter.apply",
        "summary": "Reject unknown filters",
        "requires_confirmation": False,
        "unsupported": True,
        "action": {"type": "unsupported", "action_id": "ai.filter.apply", "target_page": "/ai"},
    }

    sql_from_nested = _normalize_generic_ui_action_tool_call(
        {
            "action_id": "ai.filter.sql",
            "target_page": "/ai",
            "arguments": {"query": {"where": "ServiceName = 'api'"}},
            "notes": "with sql SeverityText = 'ERROR'",
        },
        "/ai",
        helper_action_manifest_for_page=helper_action_manifest_for_page,
        build_client_action=_build_client_action,
    )
    assert sql_from_nested == {
        "tool": "propose_ui_action",
        "action_id": "ai.filter.sql",
        "summary": "with sql SeverityText = 'ERROR'",
        "requires_confirmation": False,
        "unsupported": False,
        "action": {
            "type": "apply_sql_filter",
            "target_page": "/ai",
            "query": {"where": "ServiceName = 'api'"},
            "sql_where": "ServiceName = 'api'",
        },
    }

    sql_from_notes = _normalize_generic_ui_action_tool_call(
        {
            "action_id": "ai.filter.sql",
            "target_page": "/ai",
            "arguments": {},
            "notes": "please continue with sql SeverityText = 'ERROR'",
        },
        "/ai",
        helper_action_manifest_for_page=helper_action_manifest_for_page,
        build_client_action=_build_client_action,
    )
    assert sql_from_notes == {
        "tool": "propose_ui_action",
        "action_id": "ai.filter.sql",
        "summary": "please continue with sql SeverityText = 'ERROR'",
        "requires_confirmation": False,
        "unsupported": False,
        "action": {
            "type": "apply_sql_filter",
            "target_page": "/ai",
            "sql_where": "SeverityText = 'ERROR'",
        },
    }

    filtered_form = _normalize_generic_ui_action_tool_call(
        {
            "action_id": "ai.filter.apply",
            "target_page": "/ai",
            "arguments": {"filters": {"hours": "1", "chart": "latency"}},
            "notes": "",
        },
        "/ai",
        helper_action_manifest_for_page=helper_action_manifest_for_page,
        build_client_action=_build_client_action,
    )
    assert filtered_form == {
        "tool": "propose_ui_action",
        "action_id": "ai.filter.apply",
        "summary": "Apply AI Filters",
        "requires_confirmation": False,
        "unsupported": False,
        "action": {
            "type": "apply_form_filters",
            "target_page": "/ai",
            "filters": {"hours": "1"},
            "filter_fields": ["hours", "model"],
            "submit": False,
        },
    }

    invalid_action = _normalize_generic_ui_action_tool_call(
        {
            "action_id": "summary.nav.ai",
            "target_page": "/ai",
            "arguments": {},
            "notes": "",
        },
        "/",
        helper_action_manifest_for_page=helper_action_manifest_for_page,
        build_client_action=lambda action_type, payload: None,
    )
    assert invalid_action == {
        "tool": "propose_ui_action",
        "action_id": "summary.nav.ai",
        "summary": "Invalid arguments for action: summary.nav.ai",
        "requires_confirmation": True,
        "unsupported": True,
        "action": {"type": "unsupported", "action_id": "summary.nav.ai", "target_page": "/ai"},
    }


def test_shared_ai_actions_suggest_chart_dashboard_pivot_tool_only_for_relevant_questions():
    normalize_calls = []

    def normalize_generic_ui_action_tool_call(args, current_page):
        normalize_calls.append((args, current_page))
        return {"action": {"type": "open_modal"}, "unsupported": False}

    assert (
        _suggest_chart_dashboard_pivot_tool(
            "",
            "/logs",
            ai_chart_request_keywords=frozenset({"chart", "graph"}),
            normalize_generic_ui_action_tool_call=normalize_generic_ui_action_tool_call,
        )
        is None
    )
    assert (
        _suggest_chart_dashboard_pivot_tool(
            "show ai latency",
            "/logs",
            ai_chart_request_keywords=frozenset({"chart", "graph"}),
            normalize_generic_ui_action_tool_call=normalize_generic_ui_action_tool_call,
        )
        is None
    )
    assert (
        _suggest_chart_dashboard_pivot_tool(
            "build an ai chart",
            "/dashboards",
            ai_chart_request_keywords=frozenset({"chart", "graph"}),
            normalize_generic_ui_action_tool_call=normalize_generic_ui_action_tool_call,
        )
        is None
    )

    suggested = _suggest_chart_dashboard_pivot_tool(
        "build an ai chart for response latency",
        "/logs",
        ai_chart_request_keywords=frozenset({"chart", "graph"}),
        normalize_generic_ui_action_tool_call=normalize_generic_ui_action_tool_call,
    )
    assert suggested == {"action": {"type": "open_modal"}, "unsupported": False}
    assert normalize_calls == [
        (
            {
                "action_id": "dashboards.modal.new.open",
                "target_page": "/dashboards",
                "arguments": {},
                "notes": "Open the new dashboard modal to create the requested chart",
            },
            "/logs",
        )
    ]

    assert (
        _suggest_chart_dashboard_pivot_tool(
            "build a chart for errors",
            "/logs",
            ai_chart_request_keywords=frozenset({"chart", "graph"}),
            normalize_generic_ui_action_tool_call=normalize_generic_ui_action_tool_call,
        )
        is None
    )
