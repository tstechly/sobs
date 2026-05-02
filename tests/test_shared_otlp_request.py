from __future__ import annotations

import gzip
import json
import zlib
from typing import Any

import pytest

import shared.otlp_request as otlp_request
from shared.otlp_request import (
    _PROTOBUF_CONTENT_TYPE,
    _decompress_request_body,
    _decompress_with_limit,
    _parse_otlp_request,
)


class _FakeLogger:
    def __init__(self) -> None:
        self.debug_calls: list[tuple[str, tuple[Any, ...]]] = []
        self.warning_calls: list[tuple[str, tuple[Any, ...]]] = []

    def debug(self, message: str, *args: Any) -> None:
        self.debug_calls.append((message, args))

    def warning(self, message: str, *args: Any) -> None:
        self.warning_calls.append((message, args))


class _FakeRequest:
    def __init__(
        self,
        *,
        mimetype: str,
        body: bytes,
        headers: dict[str, str] | None = None,
        path: str = "/v1/logs",
        data_error: Exception | None = None,
    ) -> None:
        self.mimetype = mimetype
        self._body = body
        self.headers = headers or {}
        self.path = path
        self._data_error = data_error

    async def get_data(self) -> bytes:
        if self._data_error is not None:
            raise self._data_error
        return self._body


class _FakeProto:
    def __init__(self, *, fail: bool = False) -> None:
        self.fail = fail
        self.parsed_from_string: bytes | None = None
        self.payload: dict[str, Any] | None = None

    def ParseFromString(self, body: bytes) -> None:
        if self.fail:
            raise ValueError("bad protobuf")
        self.parsed_from_string = body


def test_decompress_request_body_supports_gzip_zlib_deflate_raw_deflate_and_chained_encodings() -> None:
    raw = b'{"resourceLogs": []}'

    gzip_body = gzip.compress(raw)
    assert _decompress_request_body(gzip_body, "gzip") == raw

    zlib_body = zlib.compress(raw)
    assert _decompress_request_body(zlib_body, "deflate") == raw

    raw_deflater = zlib.compressobj(wbits=-zlib.MAX_WBITS)
    raw_deflate_body = raw_deflater.compress(raw) + raw_deflater.flush()
    assert _decompress_request_body(raw_deflate_body, "deflate") == raw

    chained_body = zlib.compress(gzip_body)
    assert _decompress_request_body(chained_body, "gzip, deflate") == raw


def test_decompress_with_limit_and_unknown_encoding_guard_raise_when_size_cap_is_exceeded() -> None:
    compressed = zlib.compress(b"1234567890")
    with pytest.raises(ValueError, match="decompressed body exceeds 5 bytes"):
        _decompress_with_limit(compressed, wbits=zlib.MAX_WBITS, max_decompressed_body_bytes=5)

    with pytest.raises(ValueError, match="decompressed body exceeds 3 bytes"):
        _decompress_request_body(b"abcdef", "br", max_decompressed_body_bytes=3)


def test_decompress_with_limit_appends_flush_output(monkeypatch: pytest.MonkeyPatch) -> None:
    class _FakeDecompressor:
        def decompress(self, _raw: bytes, _max_length: int) -> bytes:
            return b""

        def flush(self, _max_length: int) -> bytes:
            return b"tail"

    monkeypatch.setattr(otlp_request.zlib, "decompressobj", lambda _wbits: _FakeDecompressor())

    assert _decompress_with_limit(b"compressed", wbits=zlib.MAX_WBITS) == b"tail"


def test_decompress_with_limit_rejects_flush_overflow(monkeypatch: pytest.MonkeyPatch) -> None:
    class _FakeDecompressor:
        def decompress(self, _raw: bytes, _max_length: int) -> bytes:
            return b""

        def flush(self, _max_length: int) -> bytes:
            return b"overflow"

    monkeypatch.setattr(otlp_request.zlib, "decompressobj", lambda _wbits: _FakeDecompressor())

    with pytest.raises(ValueError, match="decompressed body exceeds 3 bytes"):
        _decompress_with_limit(b"compressed", wbits=zlib.MAX_WBITS, max_decompressed_body_bytes=3)


@pytest.mark.asyncio
async def test_parse_otlp_request_parses_protobuf_body() -> None:
    logger = _FakeLogger()
    compressed = gzip.compress(b"proto-body")
    request = _FakeRequest(
        mimetype=_PROTOBUF_CONTENT_TYPE,
        body=compressed,
        headers={"Content-Encoding": "gzip"},
        path="/v1/logs",
    )

    msg, error = await _parse_otlp_request(
        _FakeProto,
        request=request,
        logger=logger,
        jsonify_func=lambda payload: payload,
    )

    assert error is None
    assert isinstance(msg, _FakeProto)
    assert msg.parsed_from_string == b"proto-body"
    assert logger.debug_calls[0][0] == "OTLP ingest: parse_path=protobuf endpoint=%s"


@pytest.mark.asyncio
async def test_parse_otlp_request_maps_protobuf_failures_to_400() -> None:
    logger = _FakeLogger()
    request = _FakeRequest(mimetype=_PROTOBUF_CONTENT_TYPE, body=b"broken", path="/v1/traces")

    msg, error = await _parse_otlp_request(
        lambda: _FakeProto(fail=True),
        request=request,
        logger=logger,
        jsonify_func=lambda payload: payload,
    )

    assert msg is None
    assert error == ({"error": "failed to parse protobuf body"}, 400)
    assert logger.warning_calls[0][0] == "OTLP protobuf parse error [%s]: %s"


@pytest.mark.asyncio
async def test_parse_otlp_request_parses_json_body() -> None:
    logger = _FakeLogger()
    request = _FakeRequest(mimetype="application/json", body=json.dumps({"resourceLogs": []}).encode())
    payloads: list[dict[str, Any]] = []

    def parse_dict(payload: dict[str, Any], msg: _FakeProto) -> _FakeProto:
        payloads.append(payload)
        msg.payload = payload
        return msg

    msg, error = await _parse_otlp_request(
        _FakeProto,
        request=request,
        logger=logger,
        jsonify_func=lambda payload: payload,
        parse_dict=parse_dict,
    )

    assert error is None
    assert isinstance(msg, _FakeProto)
    assert msg.payload == {"resourceLogs": []}
    assert payloads == [{"resourceLogs": []}]
    assert logger.debug_calls[0][0] == "OTLP ingest: parse_path=json endpoint=%s"


@pytest.mark.asyncio
async def test_parse_otlp_request_rejects_non_object_json_payload() -> None:
    logger = _FakeLogger()
    request = _FakeRequest(mimetype="application/json", body=b"[]", path="/v1/metrics")

    msg, error = await _parse_otlp_request(
        _FakeProto,
        request=request,
        logger=logger,
        jsonify_func=lambda payload: payload,
    )

    assert msg is None
    assert error == ({"error": "failed to parse json body"}, 400)
    assert logger.warning_calls[0][0] == "OTLP json parse error [%s]: top-level value is not an object"


@pytest.mark.asyncio
async def test_parse_otlp_request_maps_json_body_read_failures_to_400() -> None:
    logger = _FakeLogger()
    request = _FakeRequest(
        mimetype="application/json",
        body=b"",
        data_error=ValueError("no body"),
        path="/v1/metrics",
    )

    msg, error = await _parse_otlp_request(
        _FakeProto,
        request=request,
        logger=logger,
        jsonify_func=lambda payload: payload,
    )

    assert msg is None
    assert error == ({"error": "failed to read request body"}, 400)
    assert logger.warning_calls[0][0] == "OTLP json body read/decompress error [%s]: %s"


@pytest.mark.asyncio
async def test_parse_otlp_request_maps_json_parse_failures_to_400() -> None:
    logger = _FakeLogger()
    request = _FakeRequest(mimetype="application/json", body=b"{}", path="/v1/metrics")

    msg, error = await _parse_otlp_request(
        _FakeProto,
        request=request,
        logger=logger,
        jsonify_func=lambda payload: payload,
        parse_dict=lambda _payload, _msg: (_ for _ in ()).throw(ValueError("bad json")),
    )

    assert msg is None
    assert error == ({"error": "failed to parse json body"}, 400)
    assert logger.warning_calls[0][0] == "OTLP json parse error [%s]: %s"
