from __future__ import annotations

import hashlib
import hmac
import os
import re
from typing import Any


def _sanitize_rum_asset_name(value: str) -> str:
    raw = os.path.basename(str(value or "").strip())
    if not raw:
        return "asset"
    cleaned = re.sub(r"[^a-zA-Z0-9._-]+", "-", raw).strip("-._")
    return cleaned or "asset"


def _sanitize_rum_asset_type(value: str) -> str:
    raw = str(value or "").strip().lower()
    if not raw:
        return "asset"
    cleaned = re.sub(r"[^a-z0-9._-]+", "-", raw).strip("-._")
    return cleaned or "asset"


def _asset_extension(asset_name: str, content_type: str) -> str:
    _, ext = os.path.splitext(asset_name)
    if ext and re.fullmatch(r"\.[a-zA-Z0-9]{1,8}", ext):
        return ext.lstrip(".").lower()
    mapping = {
        "application/json": "json",
        "application/octet-stream": "bin",
        "text/plain": "txt",
        "image/png": "png",
        "image/jpeg": "jpg",
        "image/webp": "webp",
        "video/webm": "webm",
    }
    return mapping.get(content_type.split(";", 1)[0].strip().lower(), "bin")


def _rum_asset_signature_payload(
    method: str,
    path: str,
    timestamp: str,
    body_sha256: str,
    content_type: str,
    asset_type: str,
    asset_name: str,
) -> str:
    return "\n".join(
        [
            str(method or "").upper(),
            str(path or ""),
            str(timestamp or ""),
            str(body_sha256 or ""),
            str(content_type or "").strip().lower(),
            str(asset_type or "").strip().lower(),
            str(asset_name or ""),
        ]
    )


def _rum_asset_signature(secret: str, payload: str) -> str:
    return hmac.new(secret.encode("utf-8"), payload.encode("utf-8"), hashlib.sha256).hexdigest()


def _verify_rum_asset_signature(
    *,
    body: bytes,
    method: str,
    path: str,
    content_type: str,
    asset_type: str,
    asset_name: str,
    rum_asset_signing_key: str,
    request_headers: Any,
    now: int,
    rum_asset_sign_window_sec: int,
    compare_digest,
    rum_asset_signature_payload=_rum_asset_signature_payload,
    rum_asset_signature=_rum_asset_signature,
) -> tuple[bool, str]:
    if not rum_asset_signing_key:
        return False, "Asset upload signing key is not configured"

    timestamp = str(request_headers.get("X-SOBS-Asset-Timestamp") or "").strip()
    signature = str(request_headers.get("X-SOBS-Asset-Signature") or "").strip().lower()
    if not timestamp or not signature:
        return False, "Missing asset signature headers"

    try:
        ts = int(timestamp)
    except ValueError:
        return False, "Invalid asset signature timestamp"

    if abs(now - ts) > max(1, rum_asset_sign_window_sec):
        return False, "Asset signature timestamp outside allowed window"

    body_sha = hashlib.sha256(body).hexdigest()
    payload = rum_asset_signature_payload(
        method=method,
        path=path,
        timestamp=timestamp,
        body_sha256=body_sha,
        content_type=content_type,
        asset_type=asset_type,
        asset_name=asset_name,
    )
    expected = rum_asset_signature(rum_asset_signing_key, payload)
    if not compare_digest(signature, expected):
        return False, "Invalid asset signature"
    return True, ""
