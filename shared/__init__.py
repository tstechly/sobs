"""
SOBS shared package.

This package provides foundational utilities and data structures shared
across all SOBS modules.

Sub-modules
-----------
- ``serialization`` — Zlib/base64 compression helpers used for storing
  large payloads inside chDB string columns.
- ``events``        — Typed dataclasses (LogEvent, SpanEvent, ErrorEvent,
  MetricEvent, TypedMetricEvent) that represent normalised OTEL signals plus
  the ``_attr_fingerprint`` cardinality-reduction helper.
"""

from __future__ import annotations

from shared.events import (
    _FINGERPRINT_SKIP_PREFIXES,
    ErrorEvent,
    LogEvent,
    MetricEvent,
    SpanEvent,
    TypedMetricEvent,
    _attr_fingerprint,
)
from shared.serialization import (
    compress,
    compress_json,
    decompress,
    decompress_json,
)

__all__ = [
    # serialization
    "compress",
    "compress_json",
    "decompress",
    "decompress_json",
    # events
    "LogEvent",
    "SpanEvent",
    "ErrorEvent",
    "MetricEvent",
    "TypedMetricEvent",
    "_FINGERPRINT_SKIP_PREFIXES",
    "_attr_fingerprint",
]
