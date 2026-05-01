from __future__ import annotations

import json
from typing import Any

from shared.repo_health_sync import (
    _build_repo_health_persist_payload,
    _collect_github_repo_health_summary,
    _repo_health_compact_values,
)


class _FakeResponse:
    def __init__(self, status_code: int, payload: Any, *, content: bytes = b"[]") -> None:
        self.status_code = status_code
        self._payload = payload
        self.content = content

    def json(self) -> Any:
        return self._payload


class _FakeClient:
    def __init__(self, responses: dict[str, Any]) -> None:
        self.responses = responses

    async def get(self, url: str, **_kwargs: Any) -> _FakeResponse:
        response = self.responses[url]
        if isinstance(response, Exception):
            raise response
        return response


def test_repo_health_compact_values_coerces_int_fields() -> None:
    assert _repo_health_compact_values(
        {
            "scanned_repos": "2",
            "total_repos_considered": 3.0,
            "open_issues": None,
            "open_prs": "5",
            "security_items": 1,
        }
    ) == {
        "scanned_repos": 2,
        "total_repos_considered": 3,
        "open_issues": 0,
        "open_prs": 5,
        "security_items": 1,
    }


def test_build_repo_health_persist_payload_marks_changed_summary() -> None:
    payload = _build_repo_health_persist_payload(
        {
            "scanned_repos": 2,
            "total_repos_considered": 3,
            "open_issues": 4,
            "open_prs": 5,
            "security_items": 1,
            "last_synced_at": "2026-04-25T00:00:00Z",
        },
        "",
        safe_json_loads=lambda raw, default: default if not raw else json.loads(str(raw)),
    )

    assert payload["should_persist"] is True
    assert json.loads(str(payload["compact_json"])) == {
        "scanned_repos": 2,
        "total_repos_considered": 3,
        "open_issues": 4,
        "open_prs": 5,
        "security_items": 1,
        "last_synced_at": "2026-04-25T00:00:00Z",
    }


def test_build_repo_health_persist_payload_skips_unchanged_counts() -> None:
    payload = _build_repo_health_persist_payload(
        {
            "scanned_repos": 2,
            "total_repos_considered": 3,
            "open_issues": 4,
            "open_prs": 5,
            "security_items": 1,
            "last_synced_at": "2026-04-25T00:00:02Z",
        },
        '{"scanned_repos":2,"total_repos_considered":3,"open_issues":4,"open_prs":5,"security_items":1}',
        safe_json_loads=lambda raw, default: json.loads(str(raw or "{}")),
    )

    assert payload["compact"]["last_synced_at"] == "2026-04-25T00:00:02Z"


async def test_collect_github_repo_health_summary_builds_version_scoped_summary() -> None:
    client = _FakeClient(
        {
            "https://api.github.com/repos/acme/repo-one/issues": _FakeResponse(
                200,
                [
                    {"title": "Release 1.2.3 issue", "body": "security follow-up", "labels": [{"name": "security"}]},
                    {"title": "Release 1.2.3 PR", "body": "", "pull_request": {"url": "x"}, "labels": []},
                    {"title": "Backlog cleanup", "body": "not scoped", "labels": []},
                ],
            )
        }
    )

    summary = await _collect_github_repo_health_summary(
        [("1", "Repo Health App", "repo-health-app", "https://github.com/acme/repo-one")],
        [("1", "1.2.3")],
        default_github_token="ghp-default",
        client=client,
        max_repos=5,
        max_items_per_repo=50,
        load_repo_scoped_github_token=lambda owner, repo: "" if owner == "missing" else "",
        parse_github_repo_owner_name=lambda url: ("acme", "repo-one") if "repo-one" in url else ("", ""),
        github_version_tokens=lambda version: {version},
        text_mentions_version_tokens=lambda text, tokens: any(token in text for token in tokens),
        github_item_is_security_related=lambda item: any(
            str(label.get("name") or "").lower() == "security" for label in item.get("labels", [])
        ),
        github_api_headers=lambda token: {"Authorization": f"Bearer {token}"},
        now_iso=lambda: "2026-04-25T00:00:00Z",
    )

    assert summary["ok"] is True
    assert summary["scanned_repos"] == 1
    assert summary["total_repos_considered"] == 1
    assert summary["open_issues"] == 1
    assert summary["open_prs"] == 1
    assert summary["security_items"] == 1
    assert summary["repos"][0]["repo"] == "acme/repo-one"
    assert summary["repos"][0]["versions"] == ["1.2.3"]


async def test_collect_github_repo_health_summary_skips_invalid_and_failed_responses() -> None:
    client = _FakeClient(
        {
            "https://api.github.com/repos/acme/repo-one/issues": _FakeResponse(500, []),
            "https://api.github.com/repos/acme/repo-two/issues": _FakeResponse(200, {"not": "a list"}, content=b"{}"),
            "https://api.github.com/repos/acme/repo-three/issues": RuntimeError("boom"),
        }
    )

    summary = await _collect_github_repo_health_summary(
        [
            ("1", "Repo One", "repo-one", "https://github.com/acme/repo-one"),
            ("2", "Repo Two", "repo-two", "https://github.com/acme/repo-two"),
            ("3", "Repo Three", "repo-three", "https://github.com/acme/repo-three"),
            ("4", "No Versions", "repo-four", "https://github.com/acme/repo-four"),
        ],
        [("1", "1.2.3"), ("2", "2.0.0"), ("3", "3.0.0")],
        default_github_token="",
        client=client,
        max_repos=10,
        max_items_per_repo=50,
        load_repo_scoped_github_token=lambda _owner, _repo: "ghp-scoped",
        parse_github_repo_owner_name=lambda url: ("acme", str(url).rsplit("/", 1)[-1]),
        github_version_tokens=lambda version: {version},
        text_mentions_version_tokens=lambda text, tokens: any(token in text for token in tokens),
        github_item_is_security_related=lambda item: False,
        github_api_headers=lambda token: {"Authorization": f"Bearer {token}"},
        now_iso=lambda: "2026-04-25T00:00:00Z",
    )

    assert summary["ok"] is True
    assert summary["scanned_repos"] == 3
    assert summary["total_repos_considered"] == 3
    assert summary["repos"] == []
    assert summary["open_issues"] == 0
    assert summary["open_prs"] == 0
    assert summary["security_items"] == 0


async def test_collect_github_repo_health_summary_skips_targets_without_token_or_version_tokens() -> None:
    summary = await _collect_github_repo_health_summary(
        [
            ("1", "Repo One", "repo-one", "https://github.com/acme/repo-one"),
            ("2", "Repo Two", "repo-two", "https://github.com/acme/repo-two"),
        ],
        [("1", "1.2.3"), ("2", "2.0.0")],
        default_github_token="",
        client=_FakeClient({}),
        max_repos=10,
        max_items_per_repo=50,
        load_repo_scoped_github_token=lambda _owner, repo: "" if repo == "repo-one" else "ghp-scoped",
        parse_github_repo_owner_name=lambda url: ("acme", str(url).rsplit("/", 1)[-1]),
        github_version_tokens=lambda version: set() if version == "2.0.0" else {version},
        text_mentions_version_tokens=lambda text, tokens: any(token in text for token in tokens),
        github_item_is_security_related=lambda item: False,
        github_api_headers=lambda token: {"Authorization": f"Bearer {token}"},
        now_iso=lambda: "2026-04-25T00:00:00Z",
    )

    assert summary["ok"] is True
    assert summary["scanned_repos"] == 0
    assert summary["total_repos_considered"] == 2
    assert summary["repos"] == []
