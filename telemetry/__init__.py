"""
Sobs self-telemetry package.

Usage::

    from telemetry import configure_telemetry, get_tracer, get_meter, span

    # In app startup:
    configure_telemetry(app=quart_app)

    # Instrument a hot-path:
    with span("sobs.ingest.normalize", event_type="rum", event_count=3):
        normalize_event(event)
"""

from .config import telemetry_enabled
from .metrics import (
    record_dashboard_request_duration,
    record_ingest_batch_size,
    record_ingest_duration,
    record_ingest_events,
    record_rules_evaluate_duration,
    record_storage_query_duration,
    record_storage_write_duration,
)
from .setup import configure_telemetry, get_meter, get_tracer
from .spans import span, traced_view

__all__ = [
    "configure_telemetry",
    "get_meter",
    "get_tracer",
    "record_dashboard_request_duration",
    "record_ingest_batch_size",
    "record_ingest_duration",
    "record_ingest_events",
    "record_rules_evaluate_duration",
    "record_storage_query_duration",
    "record_storage_write_duration",
    "span",
    "telemetry_enabled",
    "traced_view",
]
