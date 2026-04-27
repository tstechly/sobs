"""
Metrics helpers for Sobs self-telemetry.

Provides lightweight wrappers for the counters and histograms used across
the hot-path instrumentation.  All helpers are no-op when telemetry is
disabled or when OpenTelemetry packages are not installed.
"""

from typing import Any

from .setup import get_meter

# ---------------------------------------------------------------------------
# Lazy-initialised instruments
# ---------------------------------------------------------------------------

_instruments: dict[str, Any] = {}


def _counter(name: str, unit: str = "1", description: str = ""):
    """Return (or create) a Counter instrument."""
    if name not in _instruments:
        meter = get_meter()
        _instruments[name] = meter.create_counter(name, unit=unit, description=description)
    return _instruments[name]


def _histogram(name: str, unit: str = "ms", description: str = ""):
    """Return (or create) a Histogram instrument."""
    if name not in _instruments:
        meter = get_meter()
        _instruments[name] = meter.create_histogram(name, unit=unit, description=description)
    return _instruments[name]


# ---------------------------------------------------------------------------
# Public recording helpers
# ---------------------------------------------------------------------------


def record_ingest_events(count: int, event_type: str) -> None:
    """Increment the ingest events total counter."""
    try:
        _counter(
            "sobs.ingest.events.total",
            unit="1",
            description="Total number of ingested events",
        ).add(count, {"event.type": event_type})
    except Exception:  # noqa: BLE001
        pass


def record_ingest_batch_size(size: int, event_type: str) -> None:
    """Record the size of an ingest batch."""
    try:
        _histogram(
            "sobs.ingest.batch.size",
            unit="1",
            description="Ingest batch size",
        ).record(size, {"event.type": event_type})
    except Exception:  # noqa: BLE001
        pass


def record_ingest_duration(duration_ms: float, event_type: str) -> None:
    """Record total ingest request duration."""
    try:
        _histogram(
            "sobs.ingest.duration.ms",
            unit="ms",
            description="Ingest request duration",
        ).record(duration_ms, {"event.type": event_type})
    except Exception:  # noqa: BLE001
        pass


def record_storage_write_duration(duration_ms: float, table: str) -> None:
    """Record storage write duration."""
    try:
        _histogram(
            "sobs.storage.write.duration.ms",
            unit="ms",
            description="Storage write duration",
        ).record(duration_ms, {"table": table})
    except Exception:  # noqa: BLE001
        pass


def record_storage_query_duration(duration_ms: float, query_name: str) -> None:
    """Record storage query duration."""
    try:
        _histogram(
            "sobs.storage.query.duration.ms",
            unit="ms",
            description="Storage query duration",
        ).record(duration_ms, {"query.name": query_name})
    except Exception:  # noqa: BLE001
        pass


def record_dashboard_request_duration(duration_ms: float, dashboard_name: str) -> None:
    """Record dashboard/API data assembly duration."""
    try:
        _histogram(
            "sobs.dashboard.request.duration.ms",
            unit="ms",
            description="Dashboard request duration",
        ).record(duration_ms, {"dashboard.name": dashboard_name})
    except Exception:  # noqa: BLE001
        pass


def record_rules_evaluate_duration(duration_ms: float, rule_count: int, event_count: int) -> None:
    """Record tag/rule evaluation duration."""
    try:
        _histogram(
            "sobs.rules.evaluate.duration.ms",
            unit="ms",
            description="Rule evaluation duration",
        ).record(duration_ms, {"rule.count": rule_count, "event.count": event_count})
    except Exception:  # noqa: BLE001
        pass
