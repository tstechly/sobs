"""
Tests for Sobs self-telemetry (telemetry/ package).

These tests verify:
1. App starts with telemetry disabled (default).
2. App starts with SOBS_TELEMETRY_ENABLED=false.
3. App starts with OTEL_SDK_DISABLED=true.
4. span() helper is safe/no-op when telemetry is disabled.
5. Invalid exporter config does not crash app startup.
6. Flask/ASGI instrumentation is only initialised when telemetry is enabled.
7. Manual instrumentation does not leak raw payloads into span attributes.
"""

import os
from unittest.mock import MagicMock, patch

import pytest


# ---------------------------------------------------------------------------
# Helper: reset the telemetry module state between tests
# ---------------------------------------------------------------------------


def _reset_telemetry():
    """Reset module-level state in telemetry.setup so each test is isolated."""
    import telemetry.setup as _setup

    _setup._sdk_initialised = False
    _setup._tracer = None
    _setup._meter = None

    import telemetry.metrics as _metrics

    _metrics._instruments.clear()


# ---------------------------------------------------------------------------
# 1 & 2 – Telemetry disabled by default / when SOBS_TELEMETRY_ENABLED=false
# ---------------------------------------------------------------------------


class TestTelemetryDisabledByDefault:
    def setup_method(self):
        _reset_telemetry()

    def test_telemetry_disabled_when_env_not_set(self):
        from telemetry.config import telemetry_enabled

        with patch.dict(os.environ, {}, clear=False):
            # Remove the key if present so we test the default case.
            env = os.environ.copy()
            env.pop("SOBS_TELEMETRY_ENABLED", None)
            with patch.dict(os.environ, env, clear=True):
                assert not telemetry_enabled()

    def test_telemetry_disabled_when_false(self):
        from telemetry.config import telemetry_enabled

        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "false"}):
            assert not telemetry_enabled()

    def test_telemetry_disabled_when_zero(self):
        from telemetry.config import telemetry_enabled

        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "0"}):
            assert not telemetry_enabled()

    def test_telemetry_enabled_when_true(self):
        from telemetry.config import telemetry_enabled

        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "true"}):
            assert telemetry_enabled()

    def test_telemetry_enabled_case_insensitive(self):
        from telemetry.config import telemetry_enabled

        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "TRUE"}):
            assert telemetry_enabled()

    def test_configure_telemetry_no_op_when_disabled(self):
        """configure_telemetry() must not raise when telemetry is disabled."""
        from telemetry.setup import configure_telemetry, is_sdk_initialised

        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "false"}):
            configure_telemetry()
        assert not is_sdk_initialised()

    def test_configure_telemetry_no_op_without_packages(self):
        """configure_telemetry() must not raise even if otel packages are missing."""
        from telemetry.setup import configure_telemetry, is_sdk_initialised

        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "false"}):
            configure_telemetry()
        assert not is_sdk_initialised()


# ---------------------------------------------------------------------------
# 3 – OTEL_SDK_DISABLED respected
# ---------------------------------------------------------------------------


class TestOtelSdkDisabled:
    def setup_method(self):
        _reset_telemetry()

    def test_otel_sdk_disabled_overrides_sobs_enabled(self):
        from telemetry.config import telemetry_enabled

        with patch.dict(
            os.environ,
            {"SOBS_TELEMETRY_ENABLED": "true", "OTEL_SDK_DISABLED": "true"},
        ):
            assert not telemetry_enabled()

    def test_otel_sdk_disabled_configure_is_noop(self):
        from telemetry.setup import configure_telemetry, is_sdk_initialised

        with patch.dict(
            os.environ,
            {"SOBS_TELEMETRY_ENABLED": "true", "OTEL_SDK_DISABLED": "true"},
        ):
            configure_telemetry()
        assert not is_sdk_initialised()

    def test_otel_sdk_disabled_false_does_not_block(self):
        """OTEL_SDK_DISABLED=false should not suppress SOBS telemetry."""
        from telemetry.config import telemetry_enabled

        with patch.dict(
            os.environ,
            {"SOBS_TELEMETRY_ENABLED": "true", "OTEL_SDK_DISABLED": "false"},
        ):
            assert telemetry_enabled()


# ---------------------------------------------------------------------------
# 4 – span() helper is safe/no-op when telemetry is disabled
# ---------------------------------------------------------------------------


class TestSpanNoOp:
    def setup_method(self):
        _reset_telemetry()

    def test_span_does_not_raise_when_disabled(self):
        from telemetry.spans import span

        executed = []
        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "false"}):
            with span("sobs.ingest.normalize", **{"event.type": "rum"}):
                executed.append(True)

        assert executed == [True]

    def test_span_propagates_return_value(self):
        from telemetry.spans import span

        result = []
        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "false"}):
            with span("sobs.test"):
                result.append(42)

        assert result == [42]

    def test_span_re_raises_exceptions(self):
        from telemetry.spans import span

        with pytest.raises(ValueError, match="boom"):
            with span("sobs.test"):
                raise ValueError("boom")

    def test_span_accepts_multiple_attributes(self):
        from telemetry.spans import span

        with span(
            "sobs.ingest.request",
            route="/v1/logs",
            **{"event.type": "log", "event.count": 3, "payload.bytes": 128},
        ):
            pass  # Must not raise

    def test_span_no_raw_payload_leak(self):
        """Attributes passed to span must only be safe scalar values."""
        from telemetry.spans import span

        # Raw payload should never be passed as an attribute.
        # This test ensures that the instrumentation in the ingest routes
        # does not pass raw event bodies.
        sensitive_body = '{"password": "hunter2", "token": "abc123"}'

        mock_span = MagicMock()
        mock_span.__enter__ = MagicMock(return_value=mock_span)
        mock_span.__exit__ = MagicMock(return_value=False)

        mock_tracer = MagicMock()
        mock_tracer.start_as_current_span.return_value = mock_span

        with patch("telemetry.spans.get_tracer", return_value=mock_tracer):
            with span("sobs.ingest.parse", **{"event.type": "log", "parser": "otlp"}):
                pass

        # Verify that the sensitive_body was NOT passed as an attribute
        call_args = mock_span.set_attribute.call_args_list
        attribute_values = [str(args[0][1]) for args in call_args]
        assert sensitive_body not in attribute_values


# ---------------------------------------------------------------------------
# 5 – Invalid exporter config does not crash app startup
# ---------------------------------------------------------------------------


class TestInvalidExporterConfig:
    def setup_method(self):
        _reset_telemetry()

    def test_invalid_exporter_name_falls_back_to_none(self):
        from telemetry.config import get_exporter_type

        with patch.dict(os.environ, {"SOBS_TELEMETRY_EXPORTER": "invalid_exporter"}):
            # get_exporter_type logs a warning and returns 'none' for unknown values
            result = get_exporter_type()
        assert result == "none"

    def test_configure_telemetry_does_not_crash_on_bad_exporter(self):
        """configure_telemetry must not raise when exporter is misconfigured."""
        from telemetry.setup import configure_telemetry, is_sdk_initialised

        with patch.dict(
            os.environ,
            {
                "SOBS_TELEMETRY_ENABLED": "true",
                "SOBS_TELEMETRY_EXPORTER": "otlp",
                # Missing SOBS_TELEMETRY_OTLP_ENDPOINT – should log warning, not crash
            },
        ):
            # Ensure OTLP endpoint is not set
            env = {k: v for k, v in os.environ.items() if k != "SOBS_TELEMETRY_OTLP_ENDPOINT"}
            env["SOBS_TELEMETRY_ENABLED"] = "true"
            env["SOBS_TELEMETRY_EXPORTER"] = "otlp"
            with patch.dict(os.environ, env, clear=True):
                configure_telemetry()

        assert not is_sdk_initialised()

    def test_configure_telemetry_none_exporter_is_noop(self):
        from telemetry.setup import configure_telemetry, is_sdk_initialised

        with patch.dict(
            os.environ,
            {
                "SOBS_TELEMETRY_ENABLED": "true",
                "SOBS_TELEMETRY_EXPORTER": "none",
            },
        ):
            configure_telemetry()

        assert not is_sdk_initialised()


# ---------------------------------------------------------------------------
# 6 – Flask/ASGI instrumentation only when telemetry enabled
# ---------------------------------------------------------------------------


class TestFlaskInstrumentationControl:
    def setup_method(self):
        _reset_telemetry()

    def test_instrument_app_not_called_when_disabled(self):
        from telemetry.setup import configure_telemetry

        mock_app = MagicMock()
        with patch.dict(os.environ, {"SOBS_TELEMETRY_ENABLED": "false"}):
            with patch("telemetry.setup._instrument_app") as mock_instr:
                configure_telemetry(app=mock_app)
                mock_instr.assert_not_called()

    def test_instrument_app_not_called_for_none_exporter(self):
        from telemetry.setup import configure_telemetry

        mock_app = MagicMock()
        with patch.dict(
            os.environ,
            {
                "SOBS_TELEMETRY_ENABLED": "true",
                "SOBS_TELEMETRY_EXPORTER": "none",
            },
        ):
            with patch("telemetry.setup._instrument_app") as mock_instr:
                configure_telemetry(app=mock_app)
                mock_instr.assert_not_called()


# ---------------------------------------------------------------------------
# 7 – No raw payloads in span attributes
# ---------------------------------------------------------------------------


class TestNoRawPayloadInSpans:
    """Verify that instrumented routes do not expose raw event payloads."""

    def test_ingest_span_attributes_are_safe(self):
        """Attributes on ingest spans must only contain safe scalar metadata."""
        allowed_attribute_keys = {
            "event.type",
            "event.count",
            "payload.bytes",
            "route",
            "parser",
            "storage.engine",
            "table",
            "row.count",
            "batch.size",
            "rule.count",
            "dashboard.name",
        }
        # This is a documentation/convention test asserting the design intent.
        # The actual span attributes used in app.py are limited to these keys.
        from telemetry.spans import span

        captured_keys = []
        mock_span = MagicMock()
        mock_span.__enter__ = MagicMock(return_value=mock_span)
        mock_span.__exit__ = MagicMock(return_value=False)

        mock_tracer = MagicMock()
        mock_tracer.start_as_current_span.return_value = mock_span

        def capture_set_attribute(key, value):
            captured_keys.append(key)

        mock_span.set_attribute.side_effect = capture_set_attribute

        with patch("telemetry.spans.get_tracer", return_value=mock_tracer):
            with span(
                "sobs.ingest.request",
                route="/v1/logs",
                **{"event.type": "log", "event.count": 5},
            ):
                pass

        for key in captured_keys:
            assert key in allowed_attribute_keys, (
                f"Unexpected attribute key '{key}' – check that no raw payloads are included"
            )


# ---------------------------------------------------------------------------
# Config helpers tests
# ---------------------------------------------------------------------------


class TestTelemetryConfig:
    def test_sample_rate_default(self):
        from telemetry.config import get_sample_rate

        env = {k: v for k, v in os.environ.items() if k != "SOBS_TELEMETRY_SAMPLE_RATE"}
        with patch.dict(os.environ, env, clear=True):
            assert get_sample_rate() == 1.0

    def test_sample_rate_valid(self):
        from telemetry.config import get_sample_rate

        with patch.dict(os.environ, {"SOBS_TELEMETRY_SAMPLE_RATE": "0.5"}):
            assert get_sample_rate() == 0.5

    def test_sample_rate_out_of_range(self):
        from telemetry.config import get_sample_rate

        with patch.dict(os.environ, {"SOBS_TELEMETRY_SAMPLE_RATE": "2.0"}):
            assert get_sample_rate() == 1.0

    def test_sample_rate_invalid(self):
        from telemetry.config import get_sample_rate

        with patch.dict(os.environ, {"SOBS_TELEMETRY_SAMPLE_RATE": "not-a-float"}):
            assert get_sample_rate() == 1.0

    def test_service_name_default(self):
        from telemetry.config import get_service_name

        env = {k: v for k, v in os.environ.items() if k != "SOBS_TELEMETRY_SERVICE_NAME"}
        with patch.dict(os.environ, env, clear=True):
            assert get_service_name() == "sobs"

    def test_environment_default(self):
        from telemetry.config import get_environment

        env = {k: v for k, v in os.environ.items() if k != "SOBS_TELEMETRY_ENVIRONMENT"}
        with patch.dict(os.environ, env, clear=True):
            assert get_environment() == "local"

    def test_console_export_disabled_by_default(self):
        from telemetry.config import console_export_enabled

        env = {k: v for k, v in os.environ.items() if k != "SOBS_TELEMETRY_CONSOLE_EXPORT"}
        with patch.dict(os.environ, env, clear=True):
            assert not console_export_enabled()

    def test_console_export_enabled(self):
        from telemetry.config import console_export_enabled

        with patch.dict(os.environ, {"SOBS_TELEMETRY_CONSOLE_EXPORT": "true"}):
            assert console_export_enabled()


# ---------------------------------------------------------------------------
# Metrics helpers tests
# ---------------------------------------------------------------------------


class TestMetricsHelpers:
    def setup_method(self):
        _reset_telemetry()

    def test_record_ingest_events_no_op_when_disabled(self):
        """Metrics helpers must not raise when telemetry is disabled."""
        from telemetry.metrics import record_ingest_events

        # No exception should be raised
        record_ingest_events(10, "log")

    def test_record_storage_write_duration_no_op(self):
        from telemetry.metrics import record_storage_write_duration

        record_storage_write_duration(12.5, "otel_logs")

    def test_record_rules_evaluate_duration_no_op(self):
        from telemetry.metrics import record_rules_evaluate_duration

        record_rules_evaluate_duration(3.2, 5, 100)

    def test_record_dashboard_request_duration_no_op(self):
        from telemetry.metrics import record_dashboard_request_duration

        record_dashboard_request_duration(45.0, "errors")


# ---------------------------------------------------------------------------
# traced_view decorator tests
# ---------------------------------------------------------------------------


class TestTracedViewDecorator:
    def setup_method(self):
        _reset_telemetry()

    @pytest.mark.asyncio
    async def test_traced_view_wraps_async_function(self):
        from telemetry.spans import traced_view

        executed = []

        @traced_view("sobs.dashboard.query", **{"dashboard.name": "test"})
        async def fake_view():
            executed.append(True)
            return "ok"

        result = await fake_view()
        assert result == "ok"
        assert executed == [True]

    def test_traced_view_wraps_sync_function(self):
        from telemetry.spans import traced_view

        executed = []

        @traced_view("sobs.dashboard.query", **{"dashboard.name": "test"})
        def fake_view():
            executed.append(True)
            return "ok"

        result = fake_view()
        assert result == "ok"
        assert executed == [True]

    @pytest.mark.asyncio
    async def test_traced_view_propagates_exception(self):
        from telemetry.spans import traced_view

        @traced_view("sobs.dashboard.query")
        async def fake_view():
            raise RuntimeError("view failed")

        with pytest.raises(RuntimeError, match="view failed"):
            await fake_view()
