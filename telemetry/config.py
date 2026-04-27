"""
Telemetry configuration helpers for Sobs.

Reads configuration from environment variables. All values have safe defaults
that result in no-op (disabled) telemetry when not explicitly set.
"""

import logging
import os

_log = logging.getLogger("sobs.telemetry")

# ---------------------------------------------------------------------------
# Public helpers
# ---------------------------------------------------------------------------


def telemetry_enabled() -> bool:
    """Return True only when Sobs telemetry is explicitly enabled and the
    standard OTEL_SDK_DISABLED override is not set."""
    if os.environ.get("OTEL_SDK_DISABLED", "").strip().lower() == "true":
        return False
    return os.environ.get("SOBS_TELEMETRY_ENABLED", "").strip().lower() == "true"


def get_exporter_type() -> str:
    """Return the configured exporter type: 'none', 'console', or 'otlp'."""
    raw = os.environ.get("SOBS_TELEMETRY_EXPORTER", "none").strip().lower()
    if raw not in ("none", "console", "otlp"):
        _log.warning("SOBS_TELEMETRY_EXPORTER has unrecognised value %r; defaulting to 'none'", raw)
        return "none"
    return raw


def get_service_name() -> str:
    return os.environ.get("SOBS_TELEMETRY_SERVICE_NAME", "sobs").strip() or "sobs"


def get_environment() -> str:
    return os.environ.get("SOBS_TELEMETRY_ENVIRONMENT", "local").strip() or "local"


def get_otlp_endpoint() -> str:
    return os.environ.get("SOBS_TELEMETRY_OTLP_ENDPOINT", "").strip()


def console_export_enabled() -> bool:
    return os.environ.get("SOBS_TELEMETRY_CONSOLE_EXPORT", "").strip().lower() == "true"


def get_sample_rate() -> float:
    """Return the configured trace sampling rate in [0.0, 1.0]."""
    raw = os.environ.get("SOBS_TELEMETRY_SAMPLE_RATE", "1.0").strip()
    try:
        rate = float(raw)
        if 0.0 <= rate <= 1.0:
            return rate
        _log.warning(
            "SOBS_TELEMETRY_SAMPLE_RATE %r is outside [0, 1]; defaulting to 1.0",
            raw,
        )
        return 1.0
    except ValueError:
        _log.warning(
            "SOBS_TELEMETRY_SAMPLE_RATE %r is not a valid float; defaulting to 1.0",
            raw,
        )
        return 1.0
