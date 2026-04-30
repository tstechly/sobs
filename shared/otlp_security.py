from __future__ import annotations

import fnmatch
import urllib.parse
from typing import Any

_SCHEME_DEFAULT_PORTS: dict[str, int] = {"http": 80, "https": 443}


def _request_is_secure_context(*, behind_tls: bool, forwarded_proto_header: str, request_scheme: str) -> bool:
    if behind_tls:
        return True
    forwarded_proto = str(forwarded_proto_header or "").split(",", 1)[0].strip().lower()
    if forwarded_proto == "https":
        return True
    return str(request_scheme or "").lower() == "https"


def _origin_allowed_for_otlp(
    origin: str,
    *,
    allowed_origins: tuple[str, ...],
    scheme_default_ports: dict[str, int] | None = None,
) -> bool:
    parsed = urllib.parse.urlparse(origin)
    scheme = (parsed.scheme or "").lower()
    host = (parsed.hostname or "").lower()
    netloc = (parsed.netloc or "").lower()
    if scheme not in {"http", "https"} or not netloc:
        return False

    with_port = f"{scheme}://{netloc}"
    candidates: list[str] = [with_port]
    default_ports = scheme_default_ports or _SCHEME_DEFAULT_PORTS
    try:
        parsed_port = parsed.port
    except ValueError:
        return False
    if parsed_port is None or parsed_port == default_ports.get(scheme):
        without_port = f"{scheme}://{host}" if host else with_port
        if without_port != with_port:
            candidates.append(without_port)
    for pattern in allowed_origins:
        lowered = pattern.lower()
        if any(fnmatch.fnmatch(candidate, lowered) for candidate in candidates):
            return True
    return False


def _path_needs_otlp_cors(path: str, *, ingest_paths: frozenset[str]) -> bool:
    if path in ingest_paths:
        return True
    if path.startswith("/v1/rum/assets/"):
        return True
    return False


def _otlp_cors_allow_methods(path: str) -> str:
    if path.startswith("/v1/rum/assets/"):
        return "GET, HEAD, OPTIONS"
    return "POST, OPTIONS"


def _append_vary_header(response: Any, value: str) -> None:
    existing = str(response.headers.get("Vary") or "")
    if not existing:
        response.headers["Vary"] = value
        return
    parts = [part.strip() for part in existing.split(",") if part.strip()]
    if value.lower() not in {part.lower() for part in parts}:
        response.headers["Vary"] = ", ".join(parts + [value])


def _apply_security_headers(
    response: Any,
    *,
    request_path: str,
    request_origin: str,
    secure_context: bool,
    allowed_origins: tuple[str, ...],
    ingest_paths: frozenset[str],
    origin_allowed_for_otlp=_origin_allowed_for_otlp,
    path_needs_otlp_cors=_path_needs_otlp_cors,
    otlp_cors_allow_methods=_otlp_cors_allow_methods,
    append_vary_header=_append_vary_header,
) -> Any:
    response.headers.setdefault("X-Content-Type-Options", "nosniff")
    response.headers.setdefault("X-Frame-Options", "SAMEORIGIN")
    response.headers.setdefault("Referrer-Policy", "strict-origin-when-cross-origin")
    response.headers.setdefault("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
    response.headers.setdefault("Content-Security-Policy", "frame-ancestors 'self'; object-src 'none'; base-uri 'self'")
    if secure_context:
        response.headers.setdefault("Strict-Transport-Security", "max-age=31536000; includeSubDomains")

    if path_needs_otlp_cors(request_path, ingest_paths=ingest_paths):
        origin = str(request_origin or "").strip()
        if origin and origin_allowed_for_otlp(origin, allowed_origins=allowed_origins):
            response.headers["Access-Control-Allow-Origin"] = origin
            append_vary_header(response, "Origin")
            response.headers.setdefault("Access-Control-Allow-Credentials", "true")
            response.headers.setdefault("Access-Control-Allow-Methods", otlp_cors_allow_methods(request_path))
            response.headers.setdefault(
                "Access-Control-Allow-Headers",
                (
                    "Content-Type, Authorization, X-API-Key, "
                    "X-SOBS-RUM-Client, X-SOBS-RUM-Signature, X-SOBS-RUM-Timestamp, "
                    "X-SOBS-Asset-Timestamp, X-SOBS-Asset-Signature"
                ),
            )
            response.headers.setdefault("Access-Control-Max-Age", "600")

    return response
