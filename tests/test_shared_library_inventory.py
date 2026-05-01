from __future__ import annotations

import json

from shared.library_inventory import (
    _build_github_actions_dependency_row,
    _build_release_registry_inventory_items,
    _build_scope_inventory_items,
    _build_sdk_inventory_items,
    _extract_library_versions_from_inventory,
    _github_actions_snapshot_name,
    _inventory_versions_by_package_from_inventory,
    _merge_library_inventory,
)


def test_github_actions_snapshot_name_parses_expected_archive_entries():
    assert _github_actions_snapshot_name("") is None
    assert _github_actions_snapshot_name("notes.txt") is None
    assert _github_actions_snapshot_name("nested/pip-freeze-Linux-AMD64.txt") == (
        "pip-freeze-linux-amd64",
        "linux",
        "amd64",
    )


def test_build_github_actions_dependency_row_sets_expected_metadata_and_storage_ref():
    row = _build_github_actions_dependency_row(
        record_id="row-1",
        release_id="release-1",
        owner="acme",
        repo="service",
        run_id="123",
        run_head_sha="abc123",
        artifact_id="456",
        artifact_name="sobs-release-dependency-snapshots",
        filename="nested/pip-freeze-linux-amd64.txt",
        release_version="1.2.3",
        platform="linux",
        architecture="amd64",
        raw_bytes=b"requests==2.32.3\n",
        dependencies=[{"package": "requests", "version": "2.32.3", "ecosystem": "PyPI"}],
        uploaded_at="2026-05-01 00:00:00",
        version=1234567890,
    )

    assert row["Name"] == "pip-freeze-linux-amd64"
    assert row["StorageRef"] == "github-actions://acme/service/runs/123/artifacts/456/pip-freeze-linux-amd64.txt"
    assert row["Platform"] == "linux"
    assert row["Architecture"] == "amd64"
    metadata = json.loads(row["MetadataJson"])
    assert metadata["source"] == "github_actions_artifact"
    assert metadata["repo"] == "acme/service"
    assert metadata["run_head_sha"] == "abc123"
    assert metadata["release_version"] == "1.2.3"
    assert metadata["dependencies"][0]["package"] == "requests"


def test_build_release_registry_inventory_items_uses_app_name_or_slug_and_skips_bad_dependency_shapes():
    artifact_rows = [
        {
            "ReleaseId": "release-1",
            "MetadataJson": json.dumps(
                {
                    "dependencies": [
                        {"package": "requests", "version": "2.32.3", "ecosystem": "PyPI"},
                        {"name": "urllib3", "version": "2.2.2", "ecosystem": "PyPI"},
                        "bad-entry",
                    ]
                }
            ),
        },
        {"ReleaseId": "release-2", "MetadataJson": json.dumps({"dependencies": "bad-shape"})},
    ]
    release_rows = [
        {"Id": "release-1", "AppId": "app-1", "ReleaseVersion": "2026.04.05", "Environment": "prod"},
        {"Id": "release-2", "AppId": "app-2", "ReleaseVersion": "2026.04.06", "Environment": "dev"},
    ]
    app_rows = [
        {"Id": "app-1", "Name": "Payments API", "Slug": "payments-api"},
        {"Id": "app-2", "Name": "", "Slug": "fallback-slug"},
    ]

    items = _build_release_registry_inventory_items(artifact_rows, release_rows, app_rows)

    assert items == [
        {
            "package": "requests",
            "version": "2.32.3",
            "ecosystem": "PyPI",
            "service": "Payments API",
            "source": "release_registry",
            "app_name": "Payments API",
            "release_version": "2026.04.05",
            "environment": "prod",
        },
        {
            "package": "urllib3",
            "version": "2.2.2",
            "ecosystem": "PyPI",
            "service": "Payments API",
            "source": "release_registry",
            "app_name": "Payments API",
            "release_version": "2026.04.05",
            "environment": "prod",
        },
    ]


def test_sdk_and_scope_inventory_builders_apply_ecosystem_mappers():
    sdk_items = _build_sdk_inventory_items(
        [("opentelemetry", "1.26.0", "python", "svc-a")],
        lang_to_osv_ecosystem=lambda language: {"python": "PyPI"}.get(language, ""),
    )
    assert sdk_items == [
        {
            "package": "opentelemetry",
            "version": "1.26.0",
            "ecosystem": "PyPI",
            "service": "svc-a",
            "source": "otel_sdk",
        }
    ]

    scope_items = _build_scope_inventory_items(
        [("requests", "2.32.3", "svc-b")],
        inventory_scope_ecosystem=lambda scope_name: "PyPI" if scope_name == "requests" else "",
    )
    assert scope_items == [
        {
            "package": "requests",
            "version": "2.32.3",
            "ecosystem": "PyPI",
            "service": "svc-b",
            "source": "otel_scope",
        }
    ]


def test_merge_inventory_prefers_release_registry_and_builds_derived_views():
    merged = _merge_library_inventory(
        [
            {
                "package": "requests",
                "version": "2.32.3",
                "ecosystem": "PyPI",
                "service": "checkout",
                "source": "otel_scope",
            },
            {
                "package": "requests",
                "version": "2.32.3",
                "ecosystem": "PyPI",
                "service": "checkout",
                "source": "release_registry",
                "app_name": "Checkout",
                "release_version": "1.2.3",
                "environment": "prod",
            },
            {
                "package": "urllib3",
                "version": "2.2.2",
                "ecosystem": "PyPI",
                "app_name": "Checkout",
                "source": "otel_sdk",
            },
            {
                "package": "skip-me",
                "version": "",
                "ecosystem": "PyPI",
                "service": "checkout",
                "source": "release_registry",
            },
        ]
    )

    assert merged == [
        {
            "package": "requests",
            "version": "2.32.3",
            "ecosystem": "PyPI",
            "service": "checkout",
            "source": "release_registry",
            "app_name": "Checkout",
            "release_version": "1.2.3",
            "environment": "prod",
        },
        {
            "package": "urllib3",
            "version": "2.2.2",
            "ecosystem": "PyPI",
            "service": "",
            "source": "otel_sdk",
            "app_name": "Checkout",
            "release_version": "",
            "environment": "",
        },
    ]

    extracted = _extract_library_versions_from_inventory(merged)
    assert extracted == [
        {
            "package": "requests",
            "version": "2.32.3",
            "ecosystem": "PyPI",
            "service": "checkout",
        },
        {
            "package": "urllib3",
            "version": "2.2.2",
            "ecosystem": "PyPI",
            "service": "Checkout",
        },
    ]

    assert _inventory_versions_by_package_from_inventory(merged) == {
        "PyPI::requests": {"2.32.3"},
        "PyPI::urllib3": {"2.2.2"},
    }

    assert _inventory_versions_by_package_from_inventory(
        merged + [{"package": "ignored", "version": "1.0.0", "ecosystem": ""}]
    ) == {
        "PyPI::requests": {"2.32.3"},
        "PyPI::urllib3": {"2.2.2"},
    }
