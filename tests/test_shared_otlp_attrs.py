from __future__ import annotations

from opentelemetry.proto.common.v1.common_pb2 import AnyValue, ArrayValue, KeyValue, KeyValueList

from shared.otlp_attrs import _attr_list_to_dict, _proto_any_value_to_python, _proto_kvlist_to_dict


def test_attr_list_to_dict_handles_supported_otlp_json_value_wrappers() -> None:
    assert _attr_list_to_dict(
        [
            {"key": "service.name", "value": {"stringValue": "checkout"}},
            {"key": "http.status_code", "value": {"intValue": 200}},
            {"key": "latency", "value": {"doubleValue": 1.5}},
            {"key": "sampled", "value": {"boolValue": True}},
            {"key": "blob", "value": {"bytesValue": "AQI="}},
            {"key": "ignored", "value": {"arrayValue": []}},
        ]
    ) == {
        "service.name": "checkout",
        "http.status_code": 200,
        "latency": 1.5,
        "sampled": True,
        "blob": "AQI=",
    }


def test_proto_any_value_to_python_handles_all_supported_value_kinds() -> None:
    assert _proto_any_value_to_python(AnyValue(string_value="checkout")) == "checkout"
    assert _proto_any_value_to_python(AnyValue(int_value=200)) == 200
    assert _proto_any_value_to_python(AnyValue(double_value=1.5)) == 1.5
    assert _proto_any_value_to_python(AnyValue(bool_value=True)) is True
    assert _proto_any_value_to_python(AnyValue(bytes_value=b"\x01\x02")) == "AQI="
    assert _proto_any_value_to_python(
        AnyValue(array_value=ArrayValue(values=[AnyValue(string_value="alpha"), AnyValue(int_value=2)]))
    ) == ["alpha", 2]
    assert _proto_any_value_to_python(
        AnyValue(
            kvlist_value=KeyValueList(
                values=[
                    KeyValue(key="env", value=AnyValue(string_value="prod")),
                    KeyValue(key="ok", value=AnyValue(bool_value=True)),
                ]
            )
        )
    ) == {"env": "prod", "ok": True}
    assert _proto_any_value_to_python(AnyValue()) is None


def test_proto_kvlist_to_dict_converts_proto_key_values() -> None:
    assert _proto_kvlist_to_dict(
        [
            KeyValue(key="service.name", value=AnyValue(string_value="checkout")),
            KeyValue(key="http.status_code", value=AnyValue(int_value=200)),
        ]
    ) == {"service.name": "checkout", "http.status_code": 200}
