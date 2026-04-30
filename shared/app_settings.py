"""Shared app-settings and masking-settings helpers used by SOBS."""

from __future__ import annotations

import json
import re
from typing import Any


def _get_app_setting(db, key: str, *, decrypt_secret_value, secret_setting_keys) -> str | None:
    row = db.execute(
        "SELECT Value FROM sobs_app_settings FINAL WHERE Key = ? LIMIT 1",
        (key,),
    ).fetchone()
    value = str(row[0]).strip() if row else ""
    if key in secret_setting_keys:
        value = decrypt_secret_value(value)
    return value if value else None


def _next_app_setting_updated_at(
    last_updated_at_ms: int, *, time_module, datetime_cls, timezone_obj
) -> tuple[str, int]:
    now_ms = int(time_module.time() * 1000)
    if now_ms <= last_updated_at_ms:
        now_ms = last_updated_at_ms + 1
    dt = datetime_cls.fromtimestamp(now_ms / 1000, tz=timezone_obj)
    return dt.strftime("%Y-%m-%d %H:%M:%S.%f"), now_ms


def _set_app_setting(
    db,
    key: str,
    value: str,
    *,
    encrypt_secret_value,
    secret_setting_keys,
    next_updated_at,
    insert_rows_json_each_row,
    masking_output_enabled_setting: str,
    masking_sql_output_enabled_setting: str,
    set_masking_settings_cache,
    is_truthy_setting,
) -> None:
    stored = encrypt_secret_value(value) if key in secret_setting_keys else value
    updated_at_value = next_updated_at()
    insert_rows_json_each_row(
        db,
        "sobs_app_settings",
        [{"Key": key, "Value": stored, "UpdatedAt": updated_at_value}],
    )
    if key == masking_output_enabled_setting:
        set_masking_settings_cache(output_enabled=is_truthy_setting(value, default=True))
    elif key == masking_sql_output_enabled_setting:
        set_masking_settings_cache(sql_output_enabled=is_truthy_setting(value, default=True))


def _del_app_setting(
    db,
    key: str,
    *,
    next_updated_at,
    insert_rows_json_each_row,
    masking_output_enabled_setting: str,
    masking_sql_output_enabled_setting: str,
    set_masking_settings_cache,
) -> None:
    updated_at_value = next_updated_at()
    insert_rows_json_each_row(
        db,
        "sobs_app_settings",
        [{"Key": key, "Value": "", "UpdatedAt": updated_at_value}],
    )
    if key == masking_output_enabled_setting:
        set_masking_settings_cache(output_enabled=True)
    elif key == masking_sql_output_enabled_setting:
        set_masking_settings_cache(sql_output_enabled=True)


def _load_json_string_list_setting(db, key: str, *, get_app_setting, logger) -> list[str]:
    raw = get_app_setting(db, key) or ""
    if not raw:
        return []
    try:
        values = json.loads(raw)
    except json.JSONDecodeError:
        logger.warning("Invalid JSON list in app setting %s", key)
        return []
    if not isinstance(values, list):
        return []
    result: list[str] = []
    for item in values:
        text = str(item or "").strip()
        if text:
            result.append(text)
    return result


def _save_json_string_list_setting(db, key: str, values: list[str], *, del_app_setting, set_app_setting) -> None:
    if not values:
        del_app_setting(db, key)
        return
    set_app_setting(db, key, json.dumps(values, ensure_ascii=False))


def _load_masking_custom_keys(
    db, *, load_json_string_list_setting, normalize_sensitive_key, masking_custom_keys_setting
):
    keys = [normalize_sensitive_key(value) for value in load_json_string_list_setting(db, masking_custom_keys_setting)]
    return sorted({key_name for key_name in keys if key_name})


def _save_masking_custom_keys(
    db,
    keys: list[str],
    *,
    normalize_sensitive_key,
    save_json_string_list_setting,
    masking_custom_keys_setting,
) -> None:
    normalized = sorted(
        {normalized_key for normalized_key in (normalize_sensitive_key(value) for value in keys) if normalized_key}
    )
    save_json_string_list_setting(db, masking_custom_keys_setting, normalized)


def _load_masking_custom_patterns(
    db,
    *,
    load_json_string_list_setting,
    validate_custom_masking_pattern_for_storage,
    logger,
    masking_custom_patterns_setting,
) -> list[str]:
    patterns: list[str] = []
    for value in load_json_string_list_setting(db, masking_custom_patterns_setting):
        try:
            patterns.append(validate_custom_masking_pattern_for_storage(value))
        except (ValueError, re.error):
            logger.warning("Ignoring invalid custom masking pattern from settings")
    return list(dict.fromkeys(patterns))


def _save_masking_custom_patterns(
    db,
    patterns: list[str],
    *,
    validate_custom_masking_pattern_for_storage,
    save_json_string_list_setting,
    masking_custom_patterns_setting,
) -> None:
    normalized = list(dict.fromkeys([validate_custom_masking_pattern_for_storage(value) for value in patterns]))
    save_json_string_list_setting(db, masking_custom_patterns_setting, normalized)


def _load_masking_settings(
    db,
    *,
    load_masking_custom_keys,
    load_masking_custom_patterns,
    default_sensitive_keys,
    default_sensitive_patterns,
    is_output_masking_enabled,
    is_sql_output_masking_enabled,
) -> dict[str, Any]:
    custom_keys = load_masking_custom_keys(db)
    custom_patterns = load_masking_custom_patterns(db)
    effective_keys = sorted({*default_sensitive_keys, *custom_keys})
    effective_patterns = [*default_sensitive_patterns, *custom_patterns]
    return {
        "custom_keys": custom_keys,
        "custom_patterns": custom_patterns,
        "default_keys": sorted(default_sensitive_keys),
        "default_patterns": list(default_sensitive_patterns),
        "effective_keys": effective_keys,
        "effective_patterns": effective_patterns,
        "output_masking_enabled": is_output_masking_enabled(db),
        "sql_output_masking_enabled": is_sql_output_masking_enabled(db),
    }


def _refresh_masking_runtime_rules(
    db,
    *,
    load_masking_custom_keys,
    load_masking_custom_patterns,
    last_rules_signature,
    lock,
    configure_runtime_rules,
):
    custom_keys = load_masking_custom_keys(db)
    custom_patterns = load_masking_custom_patterns(db)
    signature = (tuple(custom_keys), tuple(custom_patterns))

    with lock:
        if last_rules_signature == signature:
            return last_rules_signature
        configure_runtime_rules(
            custom_keys=custom_keys,
            custom_patterns=custom_patterns,
        )
        return signature
