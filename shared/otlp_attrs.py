from __future__ import annotations

import base64
from typing import Any


def _attr_list_to_dict(attr_list: list[dict[str, Any]]) -> dict[str, Any]:
    """Convert OTLP attribute list [{key, value}] to plain dict."""
    out: dict[str, Any] = {}
    for item in attr_list:
        key = item.get("key", "")
        val_obj = item.get("value", {})
        for value_type in ("stringValue", "intValue", "doubleValue", "boolValue", "bytesValue"):
            if value_type in val_obj:
                out[key] = val_obj[value_type]
                break
    return out


def _proto_any_value_to_python(value: Any) -> Any:
    """Convert OTLP AnyValue proto objects to plain Python values."""
    kind = value.WhichOneof("value")
    if kind == "string_value":
        return value.string_value
    if kind == "int_value":
        return value.int_value
    if kind == "double_value":
        return value.double_value
    if kind == "bool_value":
        return value.bool_value
    if kind == "bytes_value":
        return base64.b64encode(bytes(value.bytes_value)).decode("ascii")
    if kind == "array_value":
        return [_proto_any_value_to_python(item) for item in value.array_value.values]
    if kind == "kvlist_value":
        return {item.key: _proto_any_value_to_python(item.value) for item in value.kvlist_value.values}
    return None


def _proto_kvlist_to_dict(attributes: Any) -> dict[str, Any]:
    return {item.key: _proto_any_value_to_python(item.value) for item in attributes}
