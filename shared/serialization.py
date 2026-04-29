"""
SOBS shared serialization helpers.

Provides zlib/base64 compression utilities used throughout SOBS for storing
large payloads (SQL, JSON blobs, chart specs, etc.) inside chDB string columns
and passing them in HTTP responses.

All functions are pure Python (stdlib only) and carry no SOBS-specific imports.
"""

from __future__ import annotations

import base64
import json
import zlib

__all__ = ["compress", "compress_json", "decompress", "decompress_json"]


def compress(text: str) -> str:
    """Compress *text* and return a base64-encoded string (chDB-safe)."""
    return base64.b64encode(zlib.compress(text.encode("utf-8"), level=9)).decode("ascii")


def decompress(data: str | bytes | None) -> str:
    """Decompress a base64-encoded compressed value.

    Returns an empty string for ``None`` / empty input.
    """
    if not data:
        return ""
    raw = base64.b64decode(data) if isinstance(data, str) else data
    return zlib.decompress(raw).decode("utf-8")


def compress_json(obj: object) -> str:
    """Serialise *obj* to JSON then compress the result."""
    return compress(json.dumps(obj, ensure_ascii=False))


def decompress_json(data: str | bytes | None) -> object:
    """Decompress *data* and deserialise the JSON payload.

    Returns an empty dict ``{}`` when *data* is ``None``.
    """
    if data is None:
        return {}
    return json.loads(decompress(data))
