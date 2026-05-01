from __future__ import annotations

from shared.repo_health import (
    _build_repo_health_summary,
    _build_repo_health_targets,
    _collect_release_versions_by_app,
    _collect_repo_health_version_tokens,
    _summarize_repo_health_items,
)


def test_collect_release_versions_by_app_dedupes_skips_invalid_and_caps_versions():
    versions_by_app = _collect_release_versions_by_app(
        [
            ("app-1", "1.0.0"),
            ("app-1", "1.0.0"),
            ("app-1", "1.1.0"),
            ("app-1", "1.2.0"),
            ("app-2", ""),
            ("", "1.0.0"),
            ("app-1", "1.3.0"),
        ],
        max_versions_per_app=3,
    )

    assert versions_by_app == {"app-1": ["1.0.0", "1.1.0", "1.2.0"]}


def test_build_repo_health_targets_filters_missing_repos_and_versions():
    targets = _build_repo_health_targets(
        [
            ("app-1", "Payments API", "payments-api", "https://github.com/acme/payments"),
            ("app-2", "", "no-versions", "https://github.com/acme/no-versions"),
            ("app-3", "Bad Repo", "bad-repo", "not-a-repo"),
        ],
        {"app-1": ["1.2.3"]},
        parse_github_repo_owner_name=lambda repo_url: (
            ("acme", repo_url.rsplit("/", 1)[-1]) if repo_url.startswith("https://github.com/") else ("", "")
        ),
    )

    assert targets == [
        {
            "app_name": "Payments API",
            "owner": "acme",
            "repo": "payments",
            "versions": ["1.2.3"],
        }
    ]


def test_collect_repo_health_version_tokens_ignores_blank_versions():
    tokens = _collect_repo_health_version_tokens(
        ["1.2.3", "", "v2.0.0"],
        github_version_tokens=lambda version: {version.lower(), f"tag:{version.lower()}"},
    )

    assert tokens == {"1.2.3", "tag:1.2.3", "v2.0.0", "tag:v2.0.0"}


def test_summarize_repo_health_items_filters_to_version_tokens_and_counts_security_and_prs():
    open_issues, open_prs, security_items = _summarize_repo_health_items(
        [
            {
                "title": "Patch release 1.2.3",
                "body": "security update",
                "labels": [{"name": "security"}],
            },
            {
                "title": "Release 1.2.3 rollout",
                "body": "",
                "pull_request": {"url": "https://api.github.com/pulls/1"},
                "labels": [],
            },
            {"title": "Backlog cleanup", "body": "not version related", "labels": []},
            "bad-item-shape",
        ],
        version_tokens={"1.2.3"},
        text_mentions_version_tokens=lambda text, tokens: any(token in text for token in tokens),
        github_item_is_security_related=lambda item: any(
            str(label.get("name") or "").lower() == "security"
            for label in item.get("labels", [])
            if isinstance(label, dict)
        ),
    )

    assert (open_issues, open_prs, security_items) == (1, 1, 1)


def test_build_repo_health_summary_sorts_repos_and_rolls_up_totals():
    summary = _build_repo_health_summary(
        [
            {
                "repo": "acme/beta",
                "app_name": "Beta",
                "versions": ["2.0.0"],
                "open_issues": 0,
                "open_prs": 1,
                "security_items": 0,
            },
            {
                "repo": "acme/alpha",
                "app_name": "Alpha",
                "versions": ["1.0.0"],
                "open_issues": 1,
                "open_prs": 1,
                "security_items": 1,
            },
        ],
        scanned_repos=2,
        total_repos_considered=3,
        last_synced_at="2026-05-01T00:00:00Z",
    )

    assert summary == {
        "ok": True,
        "scanned_repos": 2,
        "total_repos_considered": 3,
        "open_issues": 1,
        "open_prs": 2,
        "security_items": 1,
        "version_scoped": True,
        "last_synced_at": "2026-05-01T00:00:00Z",
        "repos": [
            {
                "repo": "acme/alpha",
                "app_name": "Alpha",
                "versions": ["1.0.0"],
                "open_issues": 1,
                "open_prs": 1,
                "security_items": 1,
            },
            {
                "repo": "acme/beta",
                "app_name": "Beta",
                "versions": ["2.0.0"],
                "open_issues": 0,
                "open_prs": 1,
                "security_items": 0,
            },
        ],
    }
