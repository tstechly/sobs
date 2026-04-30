from datetime import datetime, timezone

from shared.ci_push import (
    _ci_push_api_key_status,
    _ci_push_expiry_iso_from_days,
    _ci_push_hash_key,
    _ci_push_setting_key,
    _generate_ci_push_api_key,
    _hash_api_key,
    _is_valid_ci_push_api_key,
    _normalize_ttl_days,
    _revoke_ci_push_api_key,
    _rotate_ci_push_api_key,
    _set_ci_push_realtime_enabled,
)


def test_normalize_ttl_days_clamps_and_defaults():
    assert _normalize_ttl_days("bad", default_days=30, min_ttl_days=1, max_ttl_days=365) == 30
    assert _normalize_ttl_days("0", default_days=30, min_ttl_days=1, max_ttl_days=365) == 1
    assert _normalize_ttl_days("999", default_days=30, min_ttl_days=1, max_ttl_days=365) == 365
    assert _normalize_ttl_days("7", default_days=30, min_ttl_days=1, max_ttl_days=365) == 7


def test_ci_push_expiry_iso_from_days_sets_end_of_day_utc():
    expiry = _ci_push_expiry_iso_from_days(2, now_utc=datetime(2026, 4, 30, 10, 15, tzinfo=timezone.utc))
    assert expiry == "2026-05-02T23:59:59+00:00"


def test_ci_push_hash_key_and_hash_api_key_are_deterministic():
    key = _ci_push_hash_key("secret-value")
    hashed = _hash_api_key("plain-key", ci_push_hash_key=key)
    assert key == _ci_push_hash_key("secret-value")
    assert hashed.startswith("scrypt:v1:")
    assert hashed == _hash_api_key("plain-key", ci_push_hash_key=key)
    assert _hash_api_key("", ci_push_hash_key=key) == ""


def test_generate_ci_push_api_key_and_setting_key():
    generated = _generate_ci_push_api_key()
    assert generated.startswith("sobs_ci_")
    assert _ci_push_setting_key(" APP-ID ", "hash", app_key_prefix="ai.ci_push.app.") == "ai.ci_push.app.app-id.hash"


def test_ci_push_api_key_status_handles_missing_and_configured_cases():
    stored = {
        "ai.ci_push.app.app-id.hash": "scrypt:v1:abc",
        "ai.ci_push.app.app-id.expires_at": "2026-12-31T23:59:59+00:00",
        "ai.ci_push.app.app-id.rotated_at": "2026-04-30T12:00:00+00:00",
        "ai.ci_push.app.app-id.realtime_enabled": "true",
    }

    missing = _ci_push_api_key_status(
        object(),
        "",
        load_ai_setting=lambda _db, _key, _default: "",
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
        github_token_expiry_status=lambda expires_at: {"state": "healthy", "expires_at": expires_at},
    )
    configured = _ci_push_api_key_status(
        object(),
        "app-id",
        load_ai_setting=lambda _db, key, default: stored.get(key, default),
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
        github_token_expiry_status=lambda expires_at: {"state": "healthy", "expires_at": expires_at},
    )

    assert missing["configured"] is False
    assert missing["expiry"]["state"] == "missing"
    assert configured["configured"] is True
    assert configured["realtime_enabled"] is True
    assert configured["expiry"]["state"] == "healthy"

    unconfigured = _ci_push_api_key_status(
        object(),
        "app-id",
        load_ai_setting=lambda _db, _key, default: default,
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
        github_token_expiry_status=lambda expires_at: {"state": "healthy", "expires_at": expires_at},
    )
    assert unconfigured["configured"] is False
    assert unconfigured["expiry"]["state"] == "missing"


def test_is_valid_ci_push_api_key_checks_hash_and_expiry():
    secret_key = _ci_push_hash_key("secret-value")
    valid_hash = _hash_api_key("plain-key", ci_push_hash_key=secret_key)

    assert (
        _is_valid_ci_push_api_key(
            object(),
            "app-id",
            "plain-key",
            ci_push_api_key_status=lambda _db, _app_id: {"hash": valid_hash, "expiry": {"state": "healthy"}},
            hash_api_key=lambda candidate: _hash_api_key(candidate, ci_push_hash_key=secret_key),
        )
        is True
    )
    assert (
        _is_valid_ci_push_api_key(
            object(),
            "app-id",
            "plain-key",
            ci_push_api_key_status=lambda _db, _app_id: {"hash": valid_hash, "expiry": {"state": "expired"}},
            hash_api_key=lambda candidate: _hash_api_key(candidate, ci_push_hash_key=secret_key),
        )
        is False
    )
    assert (
        _is_valid_ci_push_api_key(
            object(),
            "app-id",
            "plain-key",
            ci_push_api_key_status=lambda _db, _app_id: {"hash": "bad-hash", "expiry": {"state": "healthy"}},
            hash_api_key=lambda candidate: _hash_api_key(candidate, ci_push_hash_key=secret_key),
        )
        is False
    )
    assert (
        _is_valid_ci_push_api_key(
            object(),
            "app-id",
            "",
            ci_push_api_key_status=lambda _db, _app_id: {"hash": valid_hash, "expiry": {"state": "healthy"}},
            hash_api_key=lambda candidate: _hash_api_key(candidate, ci_push_hash_key=secret_key),
        )
        is False
    )
    assert (
        _is_valid_ci_push_api_key(
            object(),
            "app-id",
            "plain-key",
            ci_push_api_key_status=lambda _db, _app_id: {"hash": "", "expiry": {"state": "healthy"}},
            hash_api_key=lambda candidate: _hash_api_key(candidate, ci_push_hash_key=secret_key),
        )
        is False
    )


def test_set_rotate_and_revoke_ci_push_api_key_write_expected_settings():
    saved: list[tuple[str, str]] = []

    _set_ci_push_realtime_enabled(
        object(),
        "app-id",
        True,
        save_ai_setting=lambda _db, key, value: saved.append((key, value)),
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
    )

    plain, expires_at = _rotate_ci_push_api_key(
        object(),
        "app-id",
        999,
        normalize_ttl_days=lambda ttl_days: _normalize_ttl_days(
            ttl_days, default_days=30, min_ttl_days=1, max_ttl_days=365
        ),
        generate_ci_push_api_key=lambda: "sobs_ci_generated",
        ci_push_expiry_iso_from_days=lambda ttl_days: f"expiry-{ttl_days}",
        save_ai_setting=lambda _db, key, value: saved.append((key, value)),
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
        hash_api_key=lambda value: f"hashed:{value}",
        now_iso=lambda: "2026-04-30T12:00:00+00:00",
    )

    _revoke_ci_push_api_key(
        object(),
        "app-id",
        save_ai_setting=lambda _db, key, value: saved.append((key, value)),
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
        now_iso=lambda: "2026-05-01T12:00:00+00:00",
    )

    assert plain == "sobs_ci_generated"
    assert expires_at == "expiry-365"
    assert saved == [
        ("ai.ci_push.app.app-id.realtime_enabled", "true"),
        ("ai.ci_push.app.app-id.hash", "hashed:sobs_ci_generated"),
        ("ai.ci_push.app.app-id.expires_at", "expiry-365"),
        ("ai.ci_push.app.app-id.rotated_at", "2026-04-30T12:00:00+00:00"),
        ("ai.ci_push.app.app-id.hash", ""),
        ("ai.ci_push.app.app-id.expires_at", ""),
        ("ai.ci_push.app.app-id.rotated_at", "2026-05-01T12:00:00+00:00"),
    ]


def test_ci_push_mutation_helpers_noop_for_empty_app_id():
    saved: list[tuple[str, str]] = []

    _set_ci_push_realtime_enabled(
        object(),
        "",
        True,
        save_ai_setting=lambda _db, key, value: saved.append((key, value)),
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
    )
    assert _rotate_ci_push_api_key(
        object(),
        "",
        10,
        normalize_ttl_days=lambda ttl_days: ttl_days,
        generate_ci_push_api_key=lambda: "unused",
        ci_push_expiry_iso_from_days=lambda ttl_days: "unused",
        save_ai_setting=lambda _db, key, value: saved.append((key, value)),
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
        hash_api_key=lambda value: value,
        now_iso=lambda: "unused",
    ) == ("", "")
    _revoke_ci_push_api_key(
        object(),
        "",
        save_ai_setting=lambda _db, key, value: saved.append((key, value)),
        ci_push_setting_key=lambda app_id, leaf: _ci_push_setting_key(app_id, leaf, app_key_prefix="ai.ci_push.app."),
        now_iso=lambda: "unused",
    )

    assert saved == []
