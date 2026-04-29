"""
SOBS typed event dataclasses.

Defines the normalised in-memory representations for the four OTEL signal
types handled by SOBS:

- :class:`LogEvent`          — a single OTEL log record
- :class:`SpanEvent`         — a single OTEL span (trace)
- :class:`ErrorEvent`        — an error extracted from a span or direct ingest
- :class:`MetricEvent`       — a lightweight metric event (name + attrs)
- :class:`TypedMetricEvent`  — a full metric data point with kind, value and
  histogram fields

Also exports :func:`_attr_fingerprint` and :data:`_FINGERPRINT_SKIP_PREFIXES`
which compute a stable, low-cardinality fingerprint of metric data-point
attributes (used for series identity in anomaly detection).

This module has **no** SOBS-specific imports — it depends only on the
Python standard library.
"""

from __future__ import annotations

import hashlib
from dataclasses import dataclass, field

__all__ = [
    "LogEvent",
    "SpanEvent",
    "ErrorEvent",
    "MetricEvent",
    "TypedMetricEvent",
    "_FINGERPRINT_SKIP_PREFIXES",
    "_attr_fingerprint",
]

# ---------------------------------------------------------------------------
# Event dataclasses
# ---------------------------------------------------------------------------


@dataclass
class LogEvent:
    ts: str
    level: str
    service: str
    body: str
    attrs: dict
    resource_attrs: dict
    scope_attrs: dict
    trace_id: str
    span_id: str


@dataclass
class SpanEvent:
    ts: str
    trace_id: str
    span_id: str
    parent_span_id: str
    name: str
    service: str
    duration_ms: float
    status: str
    attrs: dict
    resource_attrs: dict
    scope_attrs: dict


@dataclass
class ErrorEvent:
    ts: str
    service: str
    err_type: str
    message: str
    stack: str
    attrs: dict
    trace_id: str
    span_id: str


@dataclass
class MetricEvent:
    ts: str
    service: str
    name: str
    attrs: dict


@dataclass
class TypedMetricEvent:
    """A single OTEL metric data point with type information and value extracted."""

    ts: str
    service: str
    metric_name: str
    metric_description: str
    metric_unit: str
    metric_kind: str  # 'gauge', 'sum', or 'histogram'
    value: float
    attrs: dict  # data-point-level attributes
    attr_fp: str  # stable fingerprint for series identity
    is_monotonic: int = 0
    aggregation_temporality: int = 0
    histogram_count: int = 0
    histogram_sum: float = 0.0
    histogram_buckets: list = field(default_factory=list)
    histogram_bounds: list = field(default_factory=list)


# ---------------------------------------------------------------------------
# Attribute fingerprinting
# ---------------------------------------------------------------------------

# Attribute key prefixes excluded from the metric series fingerprint
# (high-cardinality resource attributes that do not differentiate series).
_FINGERPRINT_SKIP_PREFIXES: tuple[str, ...] = ("telemetry.", "process.", "os.", "runtime.")


def _attr_fingerprint(attrs: dict) -> str:
    """Compute a stable, low-cardinality fingerprint of data-point attributes.

    Excludes high-cardinality resource/runtime attribute prefixes and limits
    to the first 8 sorted key=value pairs to keep cardinality manageable.

    .. note::
        MD5 is used here for **non-cryptographic** cardinality reduction only
        (16-hex fingerprint).  It is not used for any security-sensitive
        purpose.
    """
    pairs = sorted(
        f"{k}={v}" for k, v in attrs.items() if not any(k.startswith(p) for p in _FINGERPRINT_SKIP_PREFIXES)
    )[:8]
    return hashlib.md5("|".join(pairs).encode()).hexdigest()[:16]  # noqa: S324
