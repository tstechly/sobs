"""Shared AI settings helpers used by SOBS."""

from __future__ import annotations

import time


def _load_ai_setting(
    db,
    key: str,
    default: str = "",
    *,
    decrypt_secret_value,
    is_sensitive_ai_setting_key,
    ai_env_overrides,
    read_file_or_env,
) -> str:
    row = db.execute(
        "SELECT Value FROM sobs_ai_settings FINAL WHERE Key=? AND IsDeleted=0 LIMIT 1",
        [key],
    ).fetchone()
    if row:
        raw_value = str(row["Value"])
        value = decrypt_secret_value(raw_value) if is_sensitive_ai_setting_key(key) else raw_value
        if value:
            return value

    env_name, env_file_name = ai_env_overrides.get(key, ("", ""))
    if env_name:
        env_fallback = read_file_or_env(env_name, env_file_name)
        if env_fallback:
            return env_fallback

    return default


def _save_ai_setting(
    db,
    key: str,
    value: str,
    *,
    encrypt_secret_value,
    is_sensitive_ai_setting_key,
    insert_rows_json_each_row,
    now=time.time,
) -> None:
    version = int(now() * 1000)
    stored_value = encrypt_secret_value(value) if is_sensitive_ai_setting_key(key) else value
    insert_rows_json_each_row(
        db,
        "sobs_ai_settings",
        [{"Key": key, "Value": stored_value, "IsDeleted": 0, "Version": version}],
    )


def _load_all_ai_settings(
    db,
    *,
    decrypt_secret_value,
    is_sensitive_ai_setting_key,
    ai_setting_keys,
    ai_env_overrides,
    read_file_or_env,
) -> dict[str, str]:
    rows = db.execute("SELECT Key, Value FROM sobs_ai_settings FINAL WHERE IsDeleted=0").fetchall()
    result = {key: "" for key in ai_setting_keys}
    for row in rows:
        key = str(row["Key"])
        if key in result:
            raw_value = str(row["Value"])
            result[key] = decrypt_secret_value(raw_value) if is_sensitive_ai_setting_key(key) else raw_value

    for key, (env_name, env_file_name) in ai_env_overrides.items():
        if result.get(key):
            continue
        env_fallback = read_file_or_env(env_name, env_file_name)
        if env_fallback:
            result[key] = env_fallback

    return result
