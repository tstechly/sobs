"""
Unit tests for the SOBS shared package.

Covers ``shared.serialization`` and ``shared.events`` (including
``_attr_fingerprint``).  No database or Quart application is required.
"""

import zlib

from shared.events import (
    _FINGERPRINT_SKIP_PREFIXES,
    ErrorEvent,
    LogEvent,
    MetricEvent,
    SpanEvent,
    TypedMetricEvent,
    _attr_fingerprint,
)
from shared.serialization import compress, compress_json, decompress, decompress_json

# ---------------------------------------------------------------------------
# shared.serialization
# ---------------------------------------------------------------------------


class TestSerialization:
    def test_compress_decompress_roundtrip(self):
        text = "Hello, SOBS! " * 100
        assert decompress(compress(text)) == text

    def test_compress_json_roundtrip(self):
        obj = {"key": "value", "nums": [1, 2, 3], "nested": {"a": True}}
        assert decompress_json(compress_json(obj)) == obj

    def test_compressed_smaller_than_plain(self):
        text = "a" * 1000
        assert len(compress(text)) < len(text)

    def test_decompress_none_returns_empty(self):
        assert decompress(None) == ""

    def test_decompress_empty_string_returns_empty(self):
        assert decompress("") == ""

    def test_decompress_bytes_input(self):
        text = "test data"
        raw_bytes = zlib.compress(text.encode("utf-8"), level=9)
        assert decompress(raw_bytes) == text

    def test_decompress_json_none_returns_empty_dict(self):
        assert decompress_json(None) == {}

    def test_compress_produces_ascii_string(self):
        compressed = compress("some unicode: \u00e9\u00e0\u00fc")
        assert compressed.isascii()

    def test_compress_json_handles_unicode(self):
        obj = {"greeting": "caf\u00e9"}
        result = decompress_json(compress_json(obj))
        assert result["greeting"] == "caf\u00e9"


# ---------------------------------------------------------------------------
# shared.events — dataclasses
# ---------------------------------------------------------------------------


class TestLogEvent:
    def test_construct(self):
        e = LogEvent(
            ts="2024-01-01T00:00:00Z",
            level="INFO",
            service="test-svc",
            body="hello",
            attrs={"k": "v"},
            resource_attrs={"service.name": "test-svc"},
            scope_attrs={},
            trace_id="abc123",
            span_id="def456",
        )
        assert e.level == "INFO"
        assert e.service == "test-svc"

    def test_fields_accessible(self):
        e = LogEvent(
            ts="t",
            level="WARN",
            service="s",
            body="b",
            attrs={},
            resource_attrs={},
            scope_attrs={},
            trace_id="",
            span_id="",
        )
        assert e.body == "b"


class TestSpanEvent:
    def test_construct(self):
        e = SpanEvent(
            ts="2024-01-01T00:00:00Z",
            trace_id="abc",
            span_id="123",
            parent_span_id="",
            name="GET /api",
            service="web",
            duration_ms=42.5,
            status="OK",
            attrs={},
            resource_attrs={},
            scope_attrs={},
        )
        assert e.duration_ms == 42.5
        assert e.status == "OK"


class TestErrorEvent:
    def test_construct(self):
        e = ErrorEvent(
            ts="2024-01-01",
            service="api",
            err_type="ValueError",
            message="bad value",
            stack="...",
            attrs={},
            trace_id="t",
            span_id="s",
        )
        assert e.err_type == "ValueError"


class TestMetricEvent:
    def test_construct(self):
        e = MetricEvent(ts="2024-01-01", service="metrics-svc", name="cpu.usage", attrs={"host": "h1"})
        assert e.name == "cpu.usage"


class TestTypedMetricEvent:
    def test_defaults(self):
        e = TypedMetricEvent(
            ts="t",
            service="s",
            metric_name="m",
            metric_description="d",
            metric_unit="u",
            metric_kind="gauge",
            value=1.5,
            attrs={},
            attr_fp="fp123",
        )
        assert e.is_monotonic == 0
        assert e.histogram_buckets == []
        assert e.histogram_bounds == []

    def test_histogram_fields(self):
        e = TypedMetricEvent(
            ts="t",
            service="s",
            metric_name="m",
            metric_description="d",
            metric_unit="u",
            metric_kind="histogram",
            value=0.0,
            attrs={},
            attr_fp="fp",
            histogram_count=10,
            histogram_sum=100.0,
            histogram_buckets=[2, 5, 3],
            histogram_bounds=[0.5, 1.0],
        )
        assert e.histogram_count == 10
        assert e.histogram_sum == 100.0


# ---------------------------------------------------------------------------
# _attr_fingerprint
# ---------------------------------------------------------------------------


class TestAttrFingerprint:
    def test_returns_16_hex_chars(self):
        fp = _attr_fingerprint({"key": "value"})
        assert len(fp) == 16
        assert all(c in "0123456789abcdef" for c in fp)

    def test_stable_for_same_attrs(self):
        attrs = {"b": "2", "a": "1", "c": "3"}
        assert _attr_fingerprint(attrs) == _attr_fingerprint(attrs)

    def test_order_independent(self):
        fp1 = _attr_fingerprint({"a": "1", "b": "2"})
        fp2 = _attr_fingerprint({"b": "2", "a": "1"})
        assert fp1 == fp2

    def test_skip_prefixes_excluded(self):
        fp_without = _attr_fingerprint({"env": "prod"})
        fp_with_skip = _attr_fingerprint({"env": "prod", "telemetry.sdk": "opentelemetry"})
        assert fp_without == fp_with_skip

    def test_all_skip_prefixes_excluded(self):
        for prefix in _FINGERPRINT_SKIP_PREFIXES:
            fp_base = _attr_fingerprint({"env": "staging"})
            fp_with = _attr_fingerprint({"env": "staging", f"{prefix}key": "val"})
            assert fp_base == fp_with, f"Prefix '{prefix}' was not excluded"

    def test_different_attrs_produce_different_fp(self):
        fp1 = _attr_fingerprint({"env": "prod"})
        fp2 = _attr_fingerprint({"env": "staging"})
        assert fp1 != fp2

    def test_empty_attrs(self):
        fp = _attr_fingerprint({})
        assert len(fp) == 16

    def test_max_8_pairs(self):
        # Build 10 pairs; fingerprint should be computed from the first 8 sorted pairs only.
        # Sorted order: k0, k1, k2, k3, k4, k5, k6, k7, k8, k9
        # First 8: k0..k7 — adding k8 and k9 should not change the fingerprint.
        attrs_8 = {f"k{i}": str(i) for i in range(8)}
        attrs_10 = {f"k{i}": str(i) for i in range(10)}
        # Adding k8 and k9 beyond the 8-pair cap should NOT change the fingerprint.
        assert _attr_fingerprint(attrs_8) == _attr_fingerprint(attrs_10)
        # Sanity: the result is still a valid 16-char fingerprint.
        assert len(_attr_fingerprint(attrs_10)) == 16
