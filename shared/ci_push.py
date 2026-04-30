"""Managed CI push API key helpers shared across SOBS modules."""

from __future__ import annotations

import hashlib
import hmac
import secrets
from datetime import datetime, timedelta, timezone
from typing import Any

_CI_PUSH_HASH_PREFIX = "scrypt:v1:"


def _normalize_ttl_days(value: Any, *, default_days: int, min_ttl_days: int, max_ttl_days: int) -> int:
    try:
        parsed = int(str(value).strip())
    except (TypeError, ValueError):
        parsed = default_days
    return max(min_ttl_days, min(max_ttl_days, parsed))


def _ci_push_expiry_iso_from_days(ttl_days: int, *, now_utc: datetime | None = None) -> str:
    current = now_utc or datetime.now(timezone.utc)
    expires = current + timedelta(days=ttl_days)
    expires = expires.replace(hour=23, minute=59, second=59, microsecond=0)
    return expires.isoformat()


def _ci_push_hash_key(secret: str) -> bytes:
    return hashlib.blake2b(secret.encode("utf-8"), person=b"sobs-ci-hash-v1", digest_size=32).digest()


def _hash_api_key(value: str, *, ci_push_hash_key: bytes) -> str:
    raw = str(value or "").strip()
    if not raw:
        return ""
    digest = hashlib.scrypt(raw.encode("utf-8"), salt=ci_push_hash_key, n=1024, r=8, p=1, dklen=32).hex()
    return _CI_PUSH_HASH_PREFIX + digest


def _generate_ci_push_api_key() -> str:
    return "sobs_ci_" + secrets.token_urlsafe(24)


def _ci_push_setting_key(app_id: str, leaf: str, *, app_key_prefix: str) -> str:
    return f"{app_key_prefix}{str(app_id or '').strip().lower()}.{leaf}"


def _ci_push_api_key_status(
    db: Any,
    app_id: str,
    *,
    load_ai_setting,
    ci_push_setting_key,
    github_token_expiry_status,
) -> dict[str, Any]:
    target_app_id = str(app_id or "").strip()
    if not target_app_id:
        return {
            "app_id": "",
            "configured": False,
            "expires_at": "",
            "rotated_at": "",
            "hash": "",
            "realtime_enabled": False,
            "expiry": {
                "state": "missing",
                "expires_at": "",
                "days_remaining": None,
                "message": "CI push API key not configured",
            },
        }

    key_hash = load_ai_setting(db, ci_push_setting_key(target_app_id, "hash"), "").strip()
    expires_at = load_ai_setting(db, ci_push_setting_key(target_app_id, "expires_at"), "").strip()
    rotated_at = load_ai_setting(db, ci_push_setting_key(target_app_id, "rotated_at"), "").strip()
    realtime_enabled = load_ai_setting(
        db, ci_push_setting_key(target_app_id, "realtime_enabled"), "false"
    ).strip().lower() in (
        "1",
        "true",
        "yes",
    )

    expiry_status = github_token_expiry_status(expires_at)
    if not key_hash:
        expiry_status = {
            "state": "missing",
            "expires_at": "",
            "days_remaining": None,
            "message": "CI push API key not configured",
        }

    return {
        "app_id": target_app_id,
        "configured": bool(key_hash),
        "expires_at": expires_at,
        "rotated_at": rotated_at,
        "hash": key_hash,
        "realtime_enabled": realtime_enabled,
        "expiry": expiry_status,
    }


def _is_valid_ci_push_api_key(
    db: Any,
    app_id: str,
    provided_key: str,
    *,
    ci_push_api_key_status,
    hash_api_key,
) -> bool:
    candidate = str(provided_key or "").strip()
    if not candidate:
        return False

    meta = ci_push_api_key_status(db, app_id)
    key_hash = str(meta.get("hash") or "")
    if not key_hash:
        return False

    expiry_state = str(((meta.get("expiry") or {}).get("state") or "")).lower()
    if expiry_state == "expired":
        return False

    if not key_hash.startswith(_CI_PUSH_HASH_PREFIX):
        return False

    candidate_hash = hash_api_key(candidate)
    return hmac.compare_digest(candidate_hash, key_hash)


def _set_ci_push_realtime_enabled(db: Any, app_id: str, enabled: bool, *, save_ai_setting, ci_push_setting_key) -> None:
    target_app_id = str(app_id or "").strip()
    if not target_app_id:
        return
    save_ai_setting(db, ci_push_setting_key(target_app_id, "realtime_enabled"), "true" if enabled else "false")


def _rotate_ci_push_api_key(
    db: Any,
    app_id: str,
    ttl_days: int,
    *,
    normalize_ttl_days,
    generate_ci_push_api_key,
    ci_push_expiry_iso_from_days,
    save_ai_setting,
    ci_push_setting_key,
    hash_api_key,
    now_iso,
) -> tuple[str, str]:
    target_app_id = str(app_id or "").strip()
    if not target_app_id:
        return "", ""
    normalized_ttl = normalize_ttl_days(ttl_days)
    plain = generate_ci_push_api_key()
    expires_at = ci_push_expiry_iso_from_days(normalized_ttl)
    save_ai_setting(db, ci_push_setting_key(target_app_id, "hash"), hash_api_key(plain))
    save_ai_setting(db, ci_push_setting_key(target_app_id, "expires_at"), expires_at)
    save_ai_setting(db, ci_push_setting_key(target_app_id, "rotated_at"), now_iso())
    return plain, expires_at


def _revoke_ci_push_api_key(db: Any, app_id: str, *, save_ai_setting, ci_push_setting_key, now_iso) -> None:
    target_app_id = str(app_id or "").strip()
    if not target_app_id:
        return
    save_ai_setting(db, ci_push_setting_key(target_app_id, "hash"), "")
    save_ai_setting(db, ci_push_setting_key(target_app_id, "expires_at"), "")
    save_ai_setting(db, ci_push_setting_key(target_app_id, "rotated_at"), now_iso())
