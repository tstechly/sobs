from __future__ import annotations

from shared.release_registry import (
    _build_seed_registry_rows,
    _parse_app_registry_seed,
    _serialize_artifact_row,
    _serialize_release_row,
)


def test_serialize_release_and_artifact_rows_parse_metadata():
    release = _serialize_release_row(
        {
            "Id": "rel-1",
            "AppId": "app-1",
            "ReleaseVersion": "1.2.3",
            "CommitSha": "abc123",
            "BuildId": "build-7",
            "Environment": "prod",
            "ReleasedAt": "2026-05-01T00:00:00Z",
            "MetadataJson": '{"channel":"stable"}',
        }
    )
    assert release == {
        "id": "rel-1",
        "appId": "app-1",
        "version": "1.2.3",
        "commitSha": "abc123",
        "buildId": "build-7",
        "environment": "prod",
        "releasedAt": "2026-05-01T00:00:00Z",
        "metadata": {"channel": "stable"},
    }

    artifact = _serialize_artifact_row(
        {
            "Id": "art-1",
            "ReleaseId": "rel-1",
            "ArtifactType": "js_sourcemap",
            "Name": "app.js.map",
            "ContentType": "application/json",
            "Size": 123,
            "StorageRef": "s3://bucket/app.js.map",
            "ChecksumSha256": "deadbeef",
            "Platform": "linux",
            "Architecture": "arm64",
            "MetadataJson": '{"minified":true}',
            "UploadedAt": "2026-05-01T00:00:00Z",
        }
    )
    assert artifact == {
        "id": "art-1",
        "releaseId": "rel-1",
        "artifactType": "js_sourcemap",
        "name": "app.js.map",
        "contentType": "application/json",
        "size": 123,
        "storageRef": "s3://bucket/app.js.map",
        "checksumSha256": "deadbeef",
        "platform": "linux",
        "architecture": "arm64",
        "metadata": {"minified": True},
        "uploadedAt": "2026-05-01T00:00:00Z",
    }


def test_parse_app_registry_seed_validates_json_shape():
    apps, error = _parse_app_registry_seed('{"apps":[{"name":"Seeded App"}]}')
    assert error is None
    assert apps == [{"name": "Seeded App"}]

    apps, error = _parse_app_registry_seed('[{"name":"Seeded App"}]')
    assert error is None
    assert apps == [{"name": "Seeded App"}]

    apps, error = _parse_app_registry_seed("{bad json")
    assert apps == []
    assert error is not None
    assert error.startswith("Failed to parse app registry seed JSON:")

    apps, error = _parse_app_registry_seed('{"apps":{}}')
    assert apps == []
    assert error == "Ignoring app registry seed: 'apps' must be an array"

    apps, error = _parse_app_registry_seed('"oops"')
    assert apps == []
    assert error == "Ignoring app registry seed: expected object with 'apps' or an array"


def test_build_seed_registry_rows_reuses_existing_ids_and_skips_invalid_entries():
    generated_ids = iter(["generated-app", "generated-release", "generated-artifact"])
    app_rows, release_rows, artifact_rows = _build_seed_registry_rows(
        [
            {
                "name": "Seeded App",
                "slug": "seeded-app",
                "ownerTeam": "platform",
                "repoUrl": "https://github.com/example/seeded",
                "defaultEnvironment": "prod",
                "enabled": False,
                "metadata": {"tier": "gold"},
                "releases": [
                    {
                        "version": "2026.04.02",
                        "commitSha": "deadbeef",
                        "environment": "prod",
                        "metadata": {"channel": "stable"},
                        "artifacts": [
                            {
                                "artifactType": "js_sourcemap",
                                "name": "main.js.map",
                                "storageRef": "s3://seeded/main.js.map",
                                "metadata": {"signed": True},
                            },
                            "ignore-artifact",
                            {"artifactType": "", "name": "skip-me"},
                        ],
                    },
                    {"version": "2026.04.03", "artifacts": {}},
                    "ignore-release",
                    {"version": "", "environment": "prod"},
                ],
            },
            {"name": "No Release List", "releases": {}},
            {"name": ""},
            "ignore-me",
        ],
        find_existing_app_id=lambda slug: "existing-app" if slug == "seeded-app" else "",
        find_existing_release_id=lambda app_id, rel_version, commit_sha, environment: (
            "existing-release"
            if (app_id, rel_version, commit_sha, environment) == ("existing-app", "2026.04.02", "deadbeef", "prod")
            else ""
        ),
        app_slug=lambda value: value.lower().replace(" ", "-"),
        parse_bool=lambda value, default: default if value is None else bool(value),
        safe_json_dumps=lambda value: str(value) if isinstance(value, str) else repr(value),
        now_iso=lambda: "2026-05-01T00:00:00Z",
        now_version=123,
        generate_id=lambda: next(generated_ids),
    )

    assert app_rows == [
        {
            "Id": "existing-app",
            "Name": "Seeded App",
            "Slug": "seeded-app",
            "OwnerTeam": "platform",
            "RepoUrl": "https://github.com/example/seeded",
            "DefaultEnvironment": "prod",
            "Enabled": 0,
            "MetadataJson": "{'tier': 'gold'}",
            "IsDeleted": 0,
            "Version": 123,
            "CreatedAt": "2026-05-01T00:00:00Z",
            "UpdatedAt": "2026-05-01T00:00:00Z",
        },
        {
            "Id": "generated-artifact",
            "Name": "No Release List",
            "Slug": "no-release-list",
            "OwnerTeam": "",
            "RepoUrl": "",
            "DefaultEnvironment": "",
            "Enabled": 1,
            "MetadataJson": "{}",
            "IsDeleted": 0,
            "Version": 123,
            "CreatedAt": "2026-05-01T00:00:00Z",
            "UpdatedAt": "2026-05-01T00:00:00Z",
        },
    ]
    assert release_rows == [
        {
            "Id": "existing-release",
            "AppId": "existing-app",
            "ReleaseVersion": "2026.04.02",
            "CommitSha": "deadbeef",
            "BuildId": "",
            "Environment": "prod",
            "ReleasedAt": "2026-05-01T00:00:00Z",
            "MetadataJson": "{'channel': 'stable'}",
            "IsDeleted": 0,
            "Version": 123,
        },
        {
            "Id": "generated-release",
            "AppId": "existing-app",
            "ReleaseVersion": "2026.04.03",
            "CommitSha": "",
            "BuildId": "",
            "Environment": "",
            "ReleasedAt": "2026-05-01T00:00:00Z",
            "MetadataJson": "{}",
            "IsDeleted": 0,
            "Version": 123,
        },
    ]
    assert artifact_rows == [
        {
            "Id": "generated-app",
            "ReleaseId": "existing-release",
            "ArtifactType": "js_sourcemap",
            "Name": "main.js.map",
            "ContentType": "",
            "Size": 0,
            "StorageRef": "s3://seeded/main.js.map",
            "ChecksumSha256": "",
            "Platform": "",
            "Architecture": "",
            "MetadataJson": "{'signed': True}",
            "UploadedAt": "2026-05-01T00:00:00Z",
            "IsDeleted": 0,
            "Version": 123,
        }
    ]
