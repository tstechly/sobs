from __future__ import annotations

from typing import Any

from shared.apps_registry import (
    _app_slug,
    _find_app_by_id,
    _find_app_id_by_repo_url,
    _find_release_by_id,
    _safe_json_dumps,
    _safe_json_loads,
    _serialize_app_row,
)


class _Result:
    def __init__(self, *, one: dict[str, Any] | None = None, many: list[dict[str, Any]] | None = None):
        self._one = one
        self._many = many or []

    def fetchone(self) -> dict[str, Any] | None:
        return self._one

    def fetchall(self) -> list[dict[str, Any]]:
        return self._many


class _Db:
    def __init__(self, responses: list[_Result]):
        self._responses = responses
        self.calls: list[tuple[str, list[Any] | None]] = []

    def execute(self, query: str, params: list[Any] | None = None) -> _Result:
        self.calls.append((query, params))
        return self._responses.pop(0)


def test_safe_json_dumps_handles_strings_collections_and_invalid_values() -> None:
    assert _safe_json_dumps(None) == "{}"
    assert _safe_json_dumps("") == "{}"
    assert _safe_json_dumps('{"hello": "world"}') == '{"hello": "world"}'
    assert _safe_json_dumps("not-json") == "{}"
    assert _safe_json_dumps({"city": "Paris"}) == '{"city": "Paris"}'
    assert _safe_json_dumps([1, 2, 3]) == "[1, 2, 3]"
    assert _safe_json_dumps(9) == "{}"


def test_safe_json_loads_handles_defaults_and_type_mismatch() -> None:
    assert _safe_json_loads(None, {}) == {}
    assert _safe_json_loads("   ", {}) == {}
    assert _safe_json_loads('{"city": "Paris"}', {}) == {"city": "Paris"}
    assert _safe_json_loads("[1, 2]", []) == [1, 2]
    assert _safe_json_loads('{"city": "Paris"}', []) == []
    assert _safe_json_loads("not-json", {"fallback": True}) == {"fallback": True}


def test_app_slug_normalizes_and_truncates() -> None:
    assert _app_slug(" Checkout API ") == "checkout-api"
    assert _app_slug("***", "fallback-name") == "fallback-name"
    assert len(_app_slug("a" * 100)) == 80


def test_find_app_and_release_by_id_return_rows_when_present() -> None:
    db = _Db([_Result(one={"Id": "app-1", "Name": "Checkout"}), _Result(one={"Id": "rel-1", "Version": "1.0.0"})])

    assert _find_app_by_id(db, "app-1") == {"Id": "app-1", "Name": "Checkout"}
    assert _find_release_by_id(db, "rel-1") == {"Id": "rel-1", "Version": "1.0.0"}
    assert db.calls == [
        ("SELECT * FROM sobs_apps FINAL WHERE Id=? AND IsDeleted=0 LIMIT 1", ["app-1"]),
        ("SELECT * FROM sobs_app_releases FINAL WHERE Id=? AND IsDeleted=0 LIMIT 1", ["rel-1"]),
    ]


def test_find_app_id_by_repo_url_matches_case_insensitively() -> None:
    db = _Db(
        [
            _Result(
                many=[
                    {"Id": "app-1", "RepoUrl": "https://github.com/Octo/checkout-service"},
                    {"Id": "app-2", "RepoUrl": "https://github.com/octo/billing-service"},
                ]
            )
        ]
    )

    assert _find_app_id_by_repo_url(db, "https://github.com/octo/CHECKOUT-service") == "app-1"


def test_find_app_id_by_repo_url_rejects_blank_invalid_and_missing_matches() -> None:
    db = _Db([_Result(many=[{"Id": "app-1", "RepoUrl": "https://github.com/octo/checkout-service"}])])

    assert _find_app_id_by_repo_url(db, "") == ""
    assert _find_app_id_by_repo_url(db, "not-a-github-url") == ""
    assert _find_app_id_by_repo_url(db, "https://github.com/octo/other-service") == ""


def test_serialize_app_row_coerces_metadata_and_enabled_flag() -> None:
    assert _serialize_app_row(
        {
            "Id": "app-1",
            "Name": "Checkout",
            "Slug": "checkout",
            "OwnerTeam": "Payments",
            "RepoUrl": "https://github.com/octo/checkout",
            "DefaultEnvironment": "prod",
            "Enabled": "0",
            "MetadataJson": '{"tier": "gold"}',
            "CreatedAt": "2024-01-01",
            "UpdatedAt": "2024-01-02",
        }
    ) == {
        "id": "app-1",
        "name": "Checkout",
        "slug": "checkout",
        "ownerTeam": "Payments",
        "repoUrl": "https://github.com/octo/checkout",
        "defaultEnvironment": "prod",
        "enabled": False,
        "metadata": {"tier": "gold"},
        "createdAt": "2024-01-01",
        "updatedAt": "2024-01-02",
    }
