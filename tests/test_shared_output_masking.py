import threading

from shared.output_masking import (
    _get_masking_settings_flags,
    _is_output_masking_enabled,
    _is_sql_output_masking_enabled,
    _jsonify_with_optional_sql_output_mask,
    _mask_json_payload,
    _mask_payload_for_output_json,
    _mask_string_for_output,
    _mask_value_for_output,
    _set_masking_settings_cache,
)


class _Masking:
    MASK = "****"
    SENSITIVE_KEYS = {"password", "email", "token"}

    @staticmethod
    def normalize_sensitive_key(value):
        return str(value).strip().lower()

    @staticmethod
    def mask_value(value):
        if isinstance(value, dict):
            return {k: _Masking.mask_value(v) for k, v in value.items()}
        if isinstance(value, list):
            return [_Masking.mask_value(v) for v in value]
        if isinstance(value, tuple):
            return tuple(_Masking.mask_value(v) for v in value)
        if value is None:
            return None
        return "****"

    @staticmethod
    def mask_string(value):
        if value is None:
            return ""
        return f"masked:{value}"


def _cache_state():
    return {
        "lock": threading.Lock(),
        "values": {"loaded": False, "output_enabled": True, "sql_output_enabled": True},
    }


def test_shared_output_masking_cache_set_and_get_settings_flags_cover_cached_and_loaded_paths():
    state = _cache_state()
    _set_masking_settings_cache(cache_state=state, output_enabled=False, sql_output_enabled=True, loaded=True)
    assert _get_masking_settings_flags(
        cache_state=state,
        get_db=lambda: object(),
        get_app_setting=lambda db, key: "0",
        is_truthy_setting=lambda raw, default=False: str(raw).strip().lower() in {"1", "true", "yes", "on"},
        masking_output_enabled_setting="masking.output_enabled",
        masking_sql_output_enabled_setting="masking.sql_output_enabled",
    ) == (False, True)

    state = _cache_state()
    seen = []
    assert _get_masking_settings_flags(
        cache_state=state,
        get_db=lambda: "db",
        get_app_setting=lambda db, key: seen.append((db, key)) or ("1" if key == "masking.output_enabled" else "0"),
        is_truthy_setting=lambda raw, default=False: str(raw).strip().lower() in {"1", "true", "yes", "on"},
        masking_output_enabled_setting="masking.output_enabled",
        masking_sql_output_enabled_setting="masking.sql_output_enabled",
    ) == (True, False)
    assert seen == [("db", "masking.output_enabled"), ("db", "masking.sql_output_enabled")]


def test_shared_output_masking_mask_payload_handles_disabled_sensitive_and_sql_passthrough_paths():
    disabled = _mask_payload_for_output_json(
        {"password": "hunter2", "query": "select 1"},
        coerce_undefined_for_json=lambda value: value,
        is_output_masking_enabled=lambda db=None: False,
        masking_module=_Masking,
        sql_output_mask_field_names=frozenset({"sql", "query"}),
        mask_value_for_output=_mask_value_for_output,
    )
    assert disabled == {"password": "hunter2", "query": "select 1"}

    enabled = _mask_payload_for_output_json(
        {
            "password": "hunter2",
            "query": "select secret from users",
            "nested": [{"email": "ops@example.com"}, ("safe", {"token": "abc"})],
        },
        mask_sql_fields=False,
        coerce_undefined_for_json=lambda value: {"coerced": None} if value == "Undefined" else value,
        is_output_masking_enabled=lambda db=None: True,
        masking_module=_Masking,
        sql_output_mask_field_names=frozenset({"sql", "query"}),
        mask_value_for_output=_mask_value_for_output,
    )
    assert enabled == {
        "password": "****",
        "query": "select secret from users",
        "nested": [{"email": "****"}, ("****", {"token": "****"})],
    }


def test_shared_output_masking_mask_value_string_and_flag_helpers_cover_enabled_and_disabled_paths():
    assert _is_output_masking_enabled(db=None, get_masking_settings_flags=lambda db=None: (True, False)) is True
    assert _is_sql_output_masking_enabled(db=None, get_masking_settings_flags=lambda db=None: (True, False)) is False

    assert _mask_value_for_output(
        {"a": 1}, db=None, is_output_masking_enabled=lambda db=None: False, masking_module=_Masking
    ) == {"a": 1}
    assert (
        _mask_value_for_output(
            "secret", db=None, is_output_masking_enabled=lambda db=None: True, masking_module=_Masking
        )
        == "****"
    )
    assert (
        _mask_string_for_output(None, db=None, is_output_masking_enabled=lambda db=None: False, masking_module=_Masking)
        == ""
    )
    assert (
        _mask_string_for_output(
            "secret", db=None, is_output_masking_enabled=lambda db=None: False, masking_module=_Masking
        )
        == "secret"
    )
    assert (
        _mask_string_for_output(
            "secret", db=None, is_output_masking_enabled=lambda db=None: True, masking_module=_Masking
        )
        == "masked:secret"
    )


def test_shared_output_masking_mask_json_payload_and_jsonify_wrapper_delegate_correctly():
    assert _mask_json_payload(
        {"a": 1},
        mask_payload_for_output_json=lambda value, mask_sql_fields=True: {"wrapped": value, "sql": mask_sql_fields},
    ) == {
        "wrapped": {"a": 1},
        "sql": True,
    }

    payloads = []
    rendered = _jsonify_with_optional_sql_output_mask(
        {"query": "select 1"},
        base_jsonify=lambda payload: payloads.append(payload) or {"json": payload},
        mask_payload_for_output_json=lambda payload, mask_sql_fields=True: {"payload": payload, "sql": mask_sql_fields},
        is_sql_output_masking_enabled=lambda db=None: False,
    )
    assert rendered == {"json": {"payload": {"query": "select 1"}, "sql": False}}
    assert payloads == [{"payload": {"query": "select 1"}, "sql": False}]
