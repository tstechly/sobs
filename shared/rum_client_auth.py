from __future__ import annotations

import base64
import hashlib
import hmac
import json
import secrets
import time
import urllib.parse
from collections.abc import Mapping
from typing import Any


def _rum_b64url_encode(value: bytes) -> str:
    return base64.urlsafe_b64encode(value).decode("ascii").rstrip("=")


def _rum_b64url_decode(value: str) -> bytes:
    text = str(value or "").strip()
    if not text:
        return b""
    pad_len = (-len(text)) % 4
    return base64.urlsafe_b64decode(text + ("=" * pad_len))


def _normalize_origin(value: str) -> str:
    raw = str(value or "").strip()
    if not raw:
        return ""
    parsed = urllib.parse.urlparse(raw)
    if not parsed.scheme or not parsed.netloc:
        return ""
    return f"{parsed.scheme.lower()}://{parsed.netloc.lower()}"


def _request_origin(headers: Mapping[str, Any]) -> str:
    origin = _normalize_origin(str(headers.get("Origin", "") or ""))
    if origin:
        return origin
    referer = str(headers.get("Referer", "") or "").strip()
    parsed = urllib.parse.urlparse(referer)
    if parsed.scheme and parsed.netloc:
        return f"{parsed.scheme.lower()}://{parsed.netloc.lower()}"
    return ""


def _same_origin_request(headers: Mapping[str, Any], *, host: str, scheme: str) -> bool:
    origin = _normalize_origin(str(headers.get("Origin", "") or ""))
    referer_origin = _request_origin({"Referer": headers.get("Referer", "")})
    forwarded_host = str(headers.get("X-Forwarded-Host") or "").split(",", 1)[0].strip().lower()
    expected_host = forwarded_host or str(host or "").strip().lower()
    forwarded_proto = str(headers.get("X-Forwarded-Proto") or "").split(",", 1)[0].strip().lower()
    expected_scheme = forwarded_proto or str(scheme or "").strip().lower() or "http"
    expected_origin = f"{expected_scheme}://{expected_host}" if expected_host else ""
    if not expected_origin:
        return False
    return origin == expected_origin or referer_origin == expected_origin


def _rum_client_sign(payload: str, *, signing_key: str) -> str:
    return hmac.new(signing_key.encode("utf-8"), payload.encode("utf-8"), hashlib.sha256).hexdigest()


def _rum_client_token_encode(claims: dict[str, Any], *, signing_key: str) -> str:
    encoded_payload = _rum_b64url_encode(json.dumps(claims, separators=(",", ":"), ensure_ascii=False).encode("utf-8"))
    signature = _rum_client_sign(encoded_payload, signing_key=signing_key)
    return f"{encoded_payload}.{signature}"


def _rum_client_token_decode(token: str, *, signing_key: str) -> tuple[dict[str, Any] | None, str]:
    parts = str(token or "").strip().split(".")
    if len(parts) != 2:
        return None, "Invalid RUM client token format"
    payload_b64, signature = parts[0], parts[1].lower()
    expected = _rum_client_sign(payload_b64, signing_key=signing_key)
    if not secrets.compare_digest(signature, expected):
        return None, "Invalid RUM client token signature"
    try:
        claims = json.loads(_rum_b64url_decode(payload_b64).decode("utf-8"))
    except Exception:
        return None, "Invalid RUM client token payload"
    if not isinstance(claims, dict):
        return None, "Invalid RUM client token payload"
    return claims, ""


def _verify_rum_client_auth(
    events: list[Any],
    *,
    mode: str,
    signing_key: str,
    headers: Mapping[str, Any],
    host: str,
    scheme: str,
    current_time: int | None = None,
) -> tuple[bool, int, str]:
    resolved_mode = str(mode or "none").strip().lower()
    if resolved_mode in ("", "none", "off", "disabled"):
        return True, 200, ""

    if resolved_mode not in ("origin", "origin-session"):
        return False, 500, "Invalid SOBS_RUM_CLIENT_AUTH_MODE"

    if not signing_key:
        return False, 503, "RUM client signing key is not configured"

    token = str(headers.get("X-SOBS-RUM-Token") or "").strip()
    if not token:
        for event in events:
            if isinstance(event, dict):
                token = str(event.get("clientAuthToken", "")).strip()
                if token:
                    break
    if not token:
        return False, 401, "Missing RUM client auth token"

    claims, err = _rum_client_token_decode(token, signing_key=signing_key)
    if claims is None:
        return False, 401, err

    now = int(current_time if current_time is not None else time.time())
    try:
        exp = int(claims.get("exp", 0) or 0)
    except (TypeError, ValueError):
        return False, 401, "Invalid RUM client token expiry"
    if exp <= now:
        return False, 401, "RUM client token expired"

    bound_origin = _normalize_origin(str(claims.get("origin", "")))
    request_origin = _request_origin(headers)
    if not bound_origin:
        return False, 401, "RUM client token missing origin binding"
    if not request_origin:
        return False, 401, "Missing Origin/Referer for RUM client auth"
    if request_origin != bound_origin:
        return False, 401, "RUM client token origin mismatch"

    bound_app = str(claims.get("app", "")).strip()
    if bound_app:
        for event in events:
            if not isinstance(event, dict):
                continue
            event_app = str(event.get("appName", "")).strip()
            if event_app and event_app != bound_app:
                return False, 401, "RUM client token app mismatch"

    return True, 200, ""
