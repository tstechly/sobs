from __future__ import annotations

import json
import zlib
from collections.abc import Callable
from typing import Any

from google.protobuf.json_format import ParseDict

_PROTOBUF_CONTENT_TYPE = "application/x-protobuf"
_MAX_DECOMPRESSED_BODY_BYTES = 32 * 1024 * 1024


def _decompress_with_limit(
    raw: bytes,
    *,
    wbits: int,
    max_decompressed_body_bytes: int = _MAX_DECOMPRESSED_BODY_BYTES,
) -> bytes:
    decompressor = zlib.decompressobj(wbits)
    output_parts: list[bytes] = []
    total = 0
    chunk_size = 64 * 1024

    for start in range(0, len(raw), chunk_size):
        remaining = max_decompressed_body_bytes - total
        piece = decompressor.decompress(raw[start : start + chunk_size], remaining + 1)
        total += len(piece)
        if total > max_decompressed_body_bytes:
            raise ValueError(f"decompressed body exceeds {max_decompressed_body_bytes} bytes")
        if piece:
            output_parts.append(piece)

    remaining = max_decompressed_body_bytes - total
    tail = decompressor.flush(remaining + 1)
    total += len(tail)
    if total > max_decompressed_body_bytes:
        raise ValueError(f"decompressed body exceeds {max_decompressed_body_bytes} bytes")
    if tail:
        output_parts.append(tail)
    return b"".join(output_parts)


def _decompress_request_body(
    raw: bytes,
    content_encoding: str,
    *,
    decompress_with_limit: Callable[..., bytes] = _decompress_with_limit,
    max_decompressed_body_bytes: int = _MAX_DECOMPRESSED_BODY_BYTES,
) -> bytes:
    encodings = [encoding.strip().lower() for encoding in (content_encoding or "").split(",") if encoding.strip()]
    data = raw
    for encoding in reversed(encodings):
        if encoding == "gzip":
            data = decompress_with_limit(
                data,
                wbits=16 + zlib.MAX_WBITS,
                max_decompressed_body_bytes=max_decompressed_body_bytes,
            )
        elif encoding == "deflate":
            try:
                data = decompress_with_limit(
                    data,
                    wbits=zlib.MAX_WBITS,
                    max_decompressed_body_bytes=max_decompressed_body_bytes,
                )
            except zlib.error:
                data = decompress_with_limit(
                    data,
                    wbits=-zlib.MAX_WBITS,
                    max_decompressed_body_bytes=max_decompressed_body_bytes,
                )
        elif len(data) > max_decompressed_body_bytes:
            raise ValueError(f"decompressed body exceeds {max_decompressed_body_bytes} bytes")
    return data


async def _parse_otlp_request(
    proto_class: Callable[[], Any],
    *,
    request: Any,
    logger: Any,
    jsonify_func: Callable[[dict[str, str]], Any],
    parse_dict: Callable[[dict[str, Any], Any], Any] = ParseDict,
    json_loads: Callable[[bytes], Any] = json.loads,
    decompress_request_body: Callable[[bytes, str], bytes] = _decompress_request_body,
    protobuf_content_type: str = _PROTOBUF_CONTENT_TYPE,
) -> tuple[Any | None, tuple[Any, int] | None]:
    mimetype = (request.mimetype or "").lower()
    content_encoding = request.headers.get("Content-Encoding", "")
    msg = proto_class()
    if mimetype == protobuf_content_type:
        logger.debug("OTLP ingest: parse_path=protobuf endpoint=%s", request.path)
        try:
            raw = await request.get_data()
            body = decompress_request_body(raw, content_encoding)
            msg.ParseFromString(body)
        except Exception as exc:
            logger.warning("OTLP protobuf parse error [%s]: %s", request.path, exc)
            return None, (jsonify_func({"error": "failed to parse protobuf body"}), 400)
        return msg, None

    logger.debug("OTLP ingest: parse_path=json endpoint=%s", request.path)
    try:
        raw = await request.get_data()
        body = decompress_request_body(raw, content_encoding)
        payload = json_loads(body) if body else {}
    except Exception as exc:
        logger.warning("OTLP json body read/decompress error [%s]: %s", request.path, exc)
        return None, (jsonify_func({"error": "failed to read request body"}), 400)

    if not isinstance(payload, dict):
        logger.warning("OTLP json parse error [%s]: top-level value is not an object", request.path)
        return None, (jsonify_func({"error": "failed to parse json body"}), 400)

    try:
        parse_dict(payload, msg)
    except Exception as exc:
        logger.warning("OTLP json parse error [%s]: %s", request.path, exc)
        return None, (jsonify_func({"error": "failed to parse json body"}), 400)
    return msg, None
