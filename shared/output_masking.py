from __future__ import annotations

from typing import Any


def _set_masking_settings_cache(
    *,
    cache_state: dict[str, Any],
    output_enabled: bool | None = None,
    sql_output_enabled: bool | None = None,
    loaded: bool = True,
) -> None:
    with cache_state["lock"]:
        if output_enabled is not None:
            cache_state["values"]["output_enabled"] = bool(output_enabled)
        if sql_output_enabled is not None:
            cache_state["values"]["sql_output_enabled"] = bool(sql_output_enabled)
        cache_state["values"]["loaded"] = loaded


def _get_masking_settings_flags(
    db=None,
    *,
    cache_state: dict[str, Any],
    get_db,
    get_app_setting,
    is_truthy_setting,
    masking_output_enabled_setting: str,
    masking_sql_output_enabled_setting: str,
    set_masking_settings_cache=_set_masking_settings_cache,
) -> tuple[bool, bool]:
    with cache_state["lock"]:
        if cache_state["values"]["loaded"]:
            output_enabled = bool(cache_state["values"]["output_enabled"])
            sql_output_enabled = bool(cache_state["values"]["sql_output_enabled"])
            return output_enabled, sql_output_enabled

    db_conn = db or get_db()
    output_enabled = is_truthy_setting(get_app_setting(db_conn, masking_output_enabled_setting), default=True)
    sql_output_enabled = is_truthy_setting(get_app_setting(db_conn, masking_sql_output_enabled_setting), default=True)
    set_masking_settings_cache(
        cache_state=cache_state,
        output_enabled=output_enabled,
        sql_output_enabled=sql_output_enabled,
        loaded=True,
    )
    return output_enabled, sql_output_enabled


def _mask_json_payload(value: Any, *, mask_payload_for_output_json) -> Any:
    return mask_payload_for_output_json(value, mask_sql_fields=True)


def _is_output_masking_enabled(db=None, *, get_masking_settings_flags) -> bool:
    output_enabled, _sql_output_enabled = get_masking_settings_flags(db)
    return output_enabled


def _mask_value_for_output(value: Any, db=None, *, is_output_masking_enabled, masking_module) -> Any:
    if not is_output_masking_enabled(db):
        return value
    return masking_module.mask_value(value)


def _mask_string_for_output(value: Any, db=None, *, is_output_masking_enabled, masking_module) -> str:
    if not is_output_masking_enabled(db):
        if value is None:
            return ""
        return str(value)
    return masking_module.mask_string(value)


def _mask_payload_for_output_json(
    value: Any,
    *,
    db=None,
    mask_sql_fields: bool = True,
    coerce_undefined_for_json,
    is_output_masking_enabled,
    masking_module,
    sql_output_mask_field_names: frozenset[str],
    mask_value_for_output,
) -> Any:
    safe_value = coerce_undefined_for_json(value)
    if not is_output_masking_enabled(db):
        return safe_value

    if isinstance(safe_value, dict):
        masked: dict[Any, Any] = {}
        for key, item in safe_value.items():
            key_name = masking_module.normalize_sensitive_key(key)
            if key_name in masking_module.SENSITIVE_KEYS:
                masked[key] = masking_module.MASK
                continue
            if key_name in sql_output_mask_field_names and isinstance(item, str) and not mask_sql_fields:
                masked[key] = item
                continue
            masked[key] = _mask_payload_for_output_json(
                item,
                db=db,
                mask_sql_fields=mask_sql_fields,
                coerce_undefined_for_json=coerce_undefined_for_json,
                is_output_masking_enabled=is_output_masking_enabled,
                masking_module=masking_module,
                sql_output_mask_field_names=sql_output_mask_field_names,
                mask_value_for_output=mask_value_for_output,
            )
        return masked
    if isinstance(safe_value, list):
        return [
            _mask_payload_for_output_json(
                item,
                db=db,
                mask_sql_fields=mask_sql_fields,
                coerce_undefined_for_json=coerce_undefined_for_json,
                is_output_masking_enabled=is_output_masking_enabled,
                masking_module=masking_module,
                sql_output_mask_field_names=sql_output_mask_field_names,
                mask_value_for_output=mask_value_for_output,
            )
            for item in safe_value
        ]
    if isinstance(safe_value, tuple):
        return tuple(
            _mask_payload_for_output_json(
                item,
                db=db,
                mask_sql_fields=mask_sql_fields,
                coerce_undefined_for_json=coerce_undefined_for_json,
                is_output_masking_enabled=is_output_masking_enabled,
                masking_module=masking_module,
                sql_output_mask_field_names=sql_output_mask_field_names,
                mask_value_for_output=mask_value_for_output,
            )
            for item in safe_value
        )
    return mask_value_for_output(
        value=safe_value, db=db, is_output_masking_enabled=is_output_masking_enabled, masking_module=masking_module
    )


def _is_sql_output_masking_enabled(db=None, *, get_masking_settings_flags) -> bool:
    _output_enabled, sql_output_enabled = get_masking_settings_flags(db)
    return sql_output_enabled


def _jsonify_with_optional_sql_output_mask(
    payload: Any,
    *,
    base_jsonify,
    mask_payload_for_output_json,
    is_sql_output_masking_enabled,
) -> Any:
    return base_jsonify(mask_payload_for_output_json(payload, mask_sql_fields=is_sql_output_masking_enabled()))
