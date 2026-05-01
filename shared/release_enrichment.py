from __future__ import annotations

import re
from typing import Any


def _github_ref_candidates(release_version: str) -> list[str]:
    version = (release_version or "").strip()
    if not version:
        return []

    candidates = [f"refs/tags/{version}"]
    if not version.startswith("v"):
        candidates.append(f"refs/tags/v{version}")
    candidates.append(f"refs/heads/{version}")
    candidates.append(version)
    return list(dict.fromkeys(candidates))


def _github_version_tokens(version: str) -> set[str]:
    normalized = str(version or "").strip().lower()
    if not normalized:
        return set()
    tokens = {normalized}
    if not normalized.startswith("v"):
        tokens.add(f"v{normalized}")
    return tokens


def _text_mentions_version_tokens(text: str, tokens: set[str]) -> bool:
    if not text or not tokens:
        return False
    lower = text.lower()
    for token in tokens:
        if re.search(rf"(^|[^0-9a-z]){re.escape(token)}([^0-9a-z]|$)", lower):
            return True
    return False


def _github_item_is_security_related(item: dict[str, Any]) -> bool:
    security_keywords = ("security", "vulnerability", "cve", "ghsa", "dependabot")
    title = str(item.get("title") or "").lower()
    body = str(item.get("body") or "").lower()
    if any(keyword in title or keyword in body for keyword in security_keywords):
        return True
    labels = item.get("labels", [])
    if isinstance(labels, list):
        for label in labels:
            if not isinstance(label, dict):
                continue
            name = str(label.get("name") or "").lower()
            if any(keyword in name for keyword in security_keywords):
                return True
    return False
