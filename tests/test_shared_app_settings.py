from datetime import datetime, timezone

from shared.app_settings import (
    _del_app_setting,
    _get_app_setting,
    _load_json_string_list_setting,
    _load_masking_custom_keys,
    _load_masking_custom_patterns,
    _load_masking_settings,
    _next_app_setting_updated_at,
    _refresh_masking_runtime_rules,
    _save_json_string_list_setting,
    _save_masking_custom_keys,
    _save_masking_custom_patterns,
    _set_app_setting,
)


class _FakeResult:
    def __init__(self, row=None):
        self._row = row

    def fetchone(self):
        return self._row


class _FakeDb:
    def __init__(self, row=None):
        self.row = row
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        return _FakeResult(row=self.row)


class _FakeLogger:
    def __init__(self):
        self.messages = []

    def warning(self, message, *args):
        self.messages.append((message, args))


class _FakeTime:
    def __init__(self, value):
        self.value = value

    def time(self):
        return self.value


class _FakeLock:
    def __init__(self):
        self.entered = 0

    def __enter__(self):
        self.entered += 1
        return self

    def __exit__(self, exc_type, exc, tb):
        return False


def test_shared_app_settings_get_and_next_timestamp_cover_secret_and_monotonic_paths():
    db = _FakeDb(row=["secret-value"])
    assert (
        _get_app_setting(
            db,
            "vapid_private_key",
            decrypt_secret_value=lambda value: f"dec:{value}",
            secret_setting_keys={"vapid_private_key"},
        )
        == "dec:secret-value"
    )
    assert (
        _get_app_setting(
            _FakeDb(row=None),
            "missing",
            decrypt_secret_value=lambda value: value,
            secret_setting_keys={"vapid_private_key"},
        )
        is None
    )

    timestamp, last_ms = _next_app_setting_updated_at(
        1_700_000_000_000,
        time_module=_FakeTime(1_700_000_000.0),
        datetime_cls=datetime,
        timezone_obj=timezone.utc,
    )
    assert last_ms == 1_700_000_000_001
    assert timestamp == "2023-11-14 22:13:20.001000"


def test_shared_app_settings_set_and_delete_cover_cache_and_secret_paths():
    inserted = []
    cache_updates = []

    def insert_rows_json_each_row(db, table_name, rows):
        inserted.append((table_name, rows))

    def set_masking_settings_cache(**kwargs):
        cache_updates.append(kwargs)

    _set_app_setting(
        object(),
        "vapid_private_key",
        "plain",
        encrypt_secret_value=lambda value: f"enc:{value}",
        secret_setting_keys={"vapid_private_key"},
        next_updated_at=lambda: "2026-05-01 10:00:00.000000",
        insert_rows_json_each_row=insert_rows_json_each_row,
        masking_output_enabled_setting="masking.output_enabled",
        masking_sql_output_enabled_setting="masking.sql_output_enabled",
        set_masking_settings_cache=set_masking_settings_cache,
        is_truthy_setting=lambda value, default=True: value == "1",
    )
    _set_app_setting(
        object(),
        "masking.output_enabled",
        "0",
        encrypt_secret_value=lambda value: value,
        secret_setting_keys={"vapid_private_key"},
        next_updated_at=lambda: "2026-05-01 10:00:01.000000",
        insert_rows_json_each_row=insert_rows_json_each_row,
        masking_output_enabled_setting="masking.output_enabled",
        masking_sql_output_enabled_setting="masking.sql_output_enabled",
        set_masking_settings_cache=set_masking_settings_cache,
        is_truthy_setting=lambda value, default=True: value == "1",
    )
    _del_app_setting(
        object(),
        "masking.sql_output_enabled",
        next_updated_at=lambda: "2026-05-01 10:00:02.000000",
        insert_rows_json_each_row=insert_rows_json_each_row,
        masking_output_enabled_setting="masking.output_enabled",
        masking_sql_output_enabled_setting="masking.sql_output_enabled",
        set_masking_settings_cache=set_masking_settings_cache,
    )

    assert inserted == [
        (
            "sobs_app_settings",
            [{"Key": "vapid_private_key", "Value": "enc:plain", "UpdatedAt": "2026-05-01 10:00:00.000000"}],
        ),
        (
            "sobs_app_settings",
            [{"Key": "masking.output_enabled", "Value": "0", "UpdatedAt": "2026-05-01 10:00:01.000000"}],
        ),
        (
            "sobs_app_settings",
            [{"Key": "masking.sql_output_enabled", "Value": "", "UpdatedAt": "2026-05-01 10:00:02.000000"}],
        ),
    ]
    assert cache_updates == [
        {"output_enabled": False},
        {"sql_output_enabled": True},
    ]


def test_shared_app_settings_json_list_and_masking_helpers_cover_invalid_and_dedup_paths():
    logger = _FakeLogger()
    assert (
        _load_json_string_list_setting(
            object(),
            "key",
            get_app_setting=lambda db, key: "",
            logger=logger,
        )
        == []
    )
    assert (
        _load_json_string_list_setting(
            object(),
            "key",
            get_app_setting=lambda db, key: "{bad-json",
            logger=logger,
        )
        == []
    )
    assert logger.messages == [("Invalid JSON list in app setting %s", ("key",))]
    assert (
        _load_json_string_list_setting(
            object(),
            "key",
            get_app_setting=lambda db, key: '{"bad":true}',
            logger=logger,
        )
        == []
    )
    assert _load_json_string_list_setting(
        object(),
        "key",
        get_app_setting=lambda db, key: '[" alpha ", "", null, "beta"]',
        logger=logger,
    ) == ["alpha", "beta"]

    saved = []
    deleted = []
    _save_json_string_list_setting(
        object(),
        "setting",
        [],
        del_app_setting=lambda db, key: deleted.append(key),
        set_app_setting=lambda db, key, value: saved.append((key, value)),
    )
    _save_json_string_list_setting(
        object(),
        "setting",
        ["a", "b"],
        del_app_setting=lambda db, key: deleted.append(key),
        set_app_setting=lambda db, key, value: saved.append((key, value)),
    )
    assert deleted == ["setting"]
    assert saved == [("setting", '["a", "b"]')]

    keys_saved = []
    assert _load_masking_custom_keys(
        object(),
        load_json_string_list_setting=lambda db, key: [" Password ", "password", "", "Api-Key"],
        normalize_sensitive_key=lambda value: str(value).strip().lower().replace("-", "_"),
        masking_custom_keys_setting="masking.custom_keys",
    ) == ["api_key", "password"]
    _save_masking_custom_keys(
        object(),
        [" Password ", "password", "Api-Key"],
        normalize_sensitive_key=lambda value: str(value).strip().lower().replace("-", "_"),
        save_json_string_list_setting=lambda db, key, values: keys_saved.append((key, values)),
        masking_custom_keys_setting="masking.custom_keys",
    )
    assert keys_saved == [("masking.custom_keys", ["api_key", "password"])]

    pattern_logger = _FakeLogger()
    patterns_saved = []

    def validate_pattern(value):
        text = str(value)
        if text == "bad":
            raise ValueError("bad pattern")
        return text.upper()

    assert _load_masking_custom_patterns(
        object(),
        load_json_string_list_setting=lambda db, key: ["good", "bad", "good"],
        validate_custom_masking_pattern_for_storage=validate_pattern,
        logger=pattern_logger,
        masking_custom_patterns_setting="masking.custom_patterns",
    ) == ["GOOD"]
    assert pattern_logger.messages == [("Ignoring invalid custom masking pattern from settings", ())]
    _save_masking_custom_patterns(
        object(),
        ["alpha", "alpha", "beta"],
        validate_custom_masking_pattern_for_storage=validate_pattern,
        save_json_string_list_setting=lambda db, key, values: patterns_saved.append((key, values)),
        masking_custom_patterns_setting="masking.custom_patterns",
    )
    assert patterns_saved == [("masking.custom_patterns", ["ALPHA", "BETA"])]


def test_shared_app_settings_load_settings_and_refresh_runtime_rules_cover_merge_and_cache_paths():
    settings = _load_masking_settings(
        object(),
        load_masking_custom_keys=lambda db: ["custom_key"],
        load_masking_custom_patterns=lambda db: ["CUSTOM"],
        default_sensitive_keys={"password"},
        default_sensitive_patterns=["DEFAULT"],
        is_output_masking_enabled=lambda db: False,
        is_sql_output_masking_enabled=lambda db: True,
    )
    assert settings == {
        "custom_keys": ["custom_key"],
        "custom_patterns": ["CUSTOM"],
        "default_keys": ["password"],
        "default_patterns": ["DEFAULT"],
        "effective_keys": ["custom_key", "password"],
        "effective_patterns": ["DEFAULT", "CUSTOM"],
        "output_masking_enabled": False,
        "sql_output_masking_enabled": True,
    }

    configured = []
    lock = _FakeLock()
    signature = _refresh_masking_runtime_rules(
        object(),
        load_masking_custom_keys=lambda db: ["alpha"],
        load_masking_custom_patterns=lambda db: ["PATTERN"],
        last_rules_signature=None,
        lock=lock,
        configure_runtime_rules=lambda **kwargs: configured.append(kwargs),
    )
    assert signature == (("alpha",), ("PATTERN",))
    assert configured == [{"custom_keys": ["alpha"], "custom_patterns": ["PATTERN"]}]
    assert lock.entered == 1

    configured.clear()
    assert _refresh_masking_runtime_rules(
        object(),
        load_masking_custom_keys=lambda db: ["alpha"],
        load_masking_custom_patterns=lambda db: ["PATTERN"],
        last_rules_signature=(("alpha",), ("PATTERN",)),
        lock=lock,
        configure_runtime_rules=lambda **kwargs: configured.append(kwargs),
    ) == (("alpha",), ("PATTERN",))
    assert configured == []
