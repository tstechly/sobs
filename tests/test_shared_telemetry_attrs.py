import json

from shared.telemetry_attrs import _hex, _map_to_dict, _stringify_attrs


def test_hex_handles_bytes_strings_and_empty_values() -> None:
    assert _hex(bytes.fromhex("aabb")) == "aabb"
    assert _hex(bytearray(b"\x01\x02")) == "0102"
    assert _hex("trace-id") == "trace-id"
    assert _hex(17) == "17"
    assert _hex("") == ""
    assert _hex(None) == ""


def test_stringify_attrs_coerces_scalar_and_complex_values() -> None:
    assert _stringify_attrs(None) == {}
    assert _stringify_attrs({}) == {}
    assert _stringify_attrs(
        {
            "text": "value",
            "int": 7,
            "float": 1.5,
            "bool": True,
            "list": ["alpha", 2],
            "dict": {"city": "Paris"},
            "none": None,
            9: "numeric-key",
        }
    ) == {
        "text": "value",
        "int": "7",
        "float": "1.5",
        "bool": "True",
        "list": '["alpha", 2]',
        "dict": '{"city": "Paris"}',
        "9": "numeric-key",
    }


def test_map_to_dict_handles_dict_json_literal_and_invalid_values() -> None:
    assert _map_to_dict({"ok": True}) == {"ok": True}
    assert _map_to_dict("") == {}
    assert _map_to_dict("   ") == {}
    assert _map_to_dict(None) == {}
    assert _map_to_dict('{"city": "Paris"}') == {"city": "Paris"}
    assert _map_to_dict(json.dumps([1, 2, 3])) == {}
    assert _map_to_dict(" {'count': 3} ") == {"count": 3}
    assert _map_to_dict("['not', 'a', 'dict']") == {}
    assert _map_to_dict("not valid") == {}
    assert _map_to_dict(42) == {}
