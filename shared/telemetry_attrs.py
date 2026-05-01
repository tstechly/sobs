from __future__ import annotations

import ast
import json
from typing import Any


def _hex(value: Any) -> str:
    """Convert bytes or bytearray values to hex, otherwise stringify."""
    if isinstance(value, (bytes, bytearray)):
        return value.hex()
    return str(value) if value else ""


def _stringify_attrs(values: dict[Any, Any] | None) -> dict[str, str]:
    """Convert arbitrary attribute values to a string map suitable for OTel Map columns."""
    if not values:
        return {}
    out: dict[str, str] = {}
    for key, value in values.items():
        if value is None:
            continue
        if isinstance(value, (str, int, float, bool)):
            out[str(key)] = str(value)
        else:
            out[str(key)] = json.dumps(value, ensure_ascii=False)
    return out


def _map_to_dict(value: Any) -> dict[str, Any]:
    """Best-effort conversion of ClickHouse Map values to Python dicts."""
    if isinstance(value, dict):
        return value
    if not value:
        return {}
    if isinstance(value, str):
        stripped = value.strip()
        if not stripped:
            return {}
        try:
            parsed = json.loads(stripped)
            return parsed if isinstance(parsed, dict) else {}
        except json.JSONDecodeError:
            pass
        try:
            parsed = ast.literal_eval(stripped)
            return parsed if isinstance(parsed, dict) else {}
        except (ValueError, SyntaxError):
            return {}
    return {}
