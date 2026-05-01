from __future__ import annotations

from shared.release_enrichment import (
    _github_item_is_security_related,
    _github_ref_candidates,
    _github_version_tokens,
    _text_mentions_version_tokens,
)


def test_github_ref_candidates_handle_empty_v_prefixed_and_branch_cases():
    assert _github_ref_candidates("") == []
    assert _github_ref_candidates("1.2.3") == [
        "refs/tags/1.2.3",
        "refs/tags/v1.2.3",
        "refs/heads/1.2.3",
        "1.2.3",
    ]
    assert _github_ref_candidates("v2.0.0") == [
        "refs/tags/v2.0.0",
        "refs/heads/v2.0.0",
        "v2.0.0",
    ]


def test_github_version_tokens_adds_v_alias_when_needed():
    assert _github_version_tokens("") == set()
    assert _github_version_tokens("1.2.3") == {"1.2.3", "v1.2.3"}
    assert _github_version_tokens("V2.0.0") == {"v2.0.0"}


def test_text_mentions_version_tokens_respects_boundaries():
    tokens = {"1.2.3", "v1.2.3"}
    assert _text_mentions_version_tokens("Patch release 1.2.3 is live", tokens) is True
    assert _text_mentions_version_tokens("Shipping v1.2.3 now", tokens) is True
    assert _text_mentions_version_tokens("Shipping 11.2.34 now", tokens) is False
    assert _text_mentions_version_tokens("", tokens) is False
    assert _text_mentions_version_tokens("Patch release 1.2.3 is live", set()) is False


def test_github_item_is_security_related_checks_title_body_and_labels():
    assert _github_item_is_security_related({"title": "Security hotfix"}) is True
    assert _github_item_is_security_related({"body": "Mitigates CVE-2026-0001"}) is True
    assert _github_item_is_security_related({"labels": [{"name": "dependabot"}]}) is True
    assert _github_item_is_security_related({"labels": ["bad-label-shape"], "title": "cleanup", "body": ""}) is False
