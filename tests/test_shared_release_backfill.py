from __future__ import annotations

import json

from shared.release_backfill import (
    GITHUB_CONTENTS_LOCKFILE_CANDIDATES,
    _build_github_backfill_targets,
    _build_github_contents_dependency_row,
)


def test_github_contents_lockfile_candidates_cover_supported_dependency_files():
    assert GITHUB_CONTENTS_LOCKFILE_CANDIDATES == [
        ("requirements.txt", "text/plain", "requirements"),
        ("package-lock.json", "application/json", "package_lock"),
        ("go.sum", "text/plain", "go_sum"),
        ("Gemfile.lock", "text/plain", "gemfile_lock"),
    ]


def test_build_github_backfill_targets_filters_existing_disabled_and_invalid_repo_rows():
    targets = _build_github_backfill_targets(
        [
            ("release-1", "app-1", "1.2.3", "abc123"),
            ("release-2", "app-2", "2.0.0", ""),
            ("release-3", "app-3", "3.0.0", "def456"),
            ("release-4", "app-4", "", "ghi789"),
            ("release-5", "app-5", "5.0.0", "jkl012"),
            ("release-6", "app-missing", "6.0.0", "mno345"),
        ],
        [
            ("app-1", "https://github.com/acme/service-one", 1),
            ("app-2", "https://github.com/acme/service-two", 1),
            ("app-3", "https://github.com/acme/service-three", 0),
            ("app-4", "https://github.com/acme/service-four", 1),
            ("app-5", "not-a-github-url", 1),
        ],
        {"release-2"},
        parse_github_repo_owner_name=lambda repo_url: (
            ("acme", repo_url.rsplit("/", 1)[-1]) if repo_url.startswith("https://github.com/") else ("", "")
        ),
    )

    assert targets == [
        {
            "release_id": "release-1",
            "release_version": "1.2.3",
            "commit_sha": "abc123",
            "owner": "acme",
            "repo": "service-one",
        }
    ]


def test_build_github_contents_dependency_row_shapes_metadata_and_storage_ref():
    row = _build_github_contents_dependency_row(
        artifact_id="artifact-1",
        release_id="release-1",
        owner="acme",
        repo="service-one",
        lockfile_path="requirements.txt",
        content_type="text/plain",
        ref="refs/tags/v1.2.3",
        raw_bytes=b"requests==2.32.3\n",
        dependencies=[{"package": "requests", "version": "2.32.3", "ecosystem": "PyPI"}],
        uploaded_at="2026-05-01 00:00:00",
        version=1234567890,
    )

    assert row["StorageRef"] == "github://acme/service-one/requirements.txt?ref=refs%2Ftags%2Fv1.2.3"
    assert row["Name"] == "requirements.txt"
    assert row["ContentType"] == "text/plain"
    assert row["Platform"] == ""
    metadata = json.loads(row["MetadataJson"])
    assert metadata == {
        "source": "github_contents_api",
        "repo": "acme/service-one",
        "ref": "refs/tags/v1.2.3",
        "path": "requirements.txt",
        "dependencies": [{"package": "requests", "version": "2.32.3", "ecosystem": "PyPI"}],
    }
