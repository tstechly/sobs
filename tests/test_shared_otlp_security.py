from quart.wrappers import Response as QuartResponse

from shared.otlp_security import (
    _append_vary_header,
    _apply_security_headers,
    _origin_allowed_for_otlp,
    _otlp_cors_allow_methods,
    _path_needs_otlp_cors,
    _request_is_secure_context,
)


def test_shared_otlp_security_request_is_secure_context_covers_all_paths():
    assert _request_is_secure_context(behind_tls=True, forwarded_proto_header="", request_scheme="http") is True
    assert (
        _request_is_secure_context(behind_tls=False, forwarded_proto_header="https, http", request_scheme="http")
        is True
    )
    assert _request_is_secure_context(behind_tls=False, forwarded_proto_header="", request_scheme="https") is True
    assert _request_is_secure_context(behind_tls=False, forwarded_proto_header="http", request_scheme="http") is False


def test_shared_otlp_security_origin_allowed_for_otlp_covers_valid_and_invalid_origins():
    allowed_origins = ("http://localhost:*", "https://example.com")

    assert _origin_allowed_for_otlp("http://localhost:3000", allowed_origins=allowed_origins) is True
    assert _origin_allowed_for_otlp("https://example.com", allowed_origins=allowed_origins) is True
    assert _origin_allowed_for_otlp("https://example.com:443", allowed_origins=allowed_origins) is True
    assert _origin_allowed_for_otlp("https://example.com:8443", allowed_origins=allowed_origins) is False
    assert _origin_allowed_for_otlp("https://example.com:abc", allowed_origins=allowed_origins) is False
    assert _origin_allowed_for_otlp("not-an-origin", allowed_origins=allowed_origins) is False
    assert _origin_allowed_for_otlp("", allowed_origins=allowed_origins) is False


def test_shared_otlp_security_path_and_method_helpers_cover_asset_and_ingest_paths():
    ingest_paths = frozenset({"/v1/logs", "/v1/rum/assets"})

    assert _path_needs_otlp_cors("/v1/logs", ingest_paths=ingest_paths) is True
    assert _path_needs_otlp_cors("/v1/rum/assets/test-id", ingest_paths=ingest_paths) is True
    assert _path_needs_otlp_cors("/v1/apps", ingest_paths=ingest_paths) is False

    assert _otlp_cors_allow_methods("/v1/rum/assets/test-id") == "GET, HEAD, OPTIONS"
    assert _otlp_cors_allow_methods("/v1/logs") == "POST, OPTIONS"


def test_shared_otlp_security_append_vary_header_deduplicates_case_insensitively():
    response = QuartResponse("")
    _append_vary_header(response, "Origin")
    _append_vary_header(response, "origin")
    assert response.headers.get("Vary") == "Origin"


def test_shared_otlp_security_apply_security_headers_adds_baseline_and_cors_headers():
    response = QuartResponse("")
    updated = _apply_security_headers(
        response,
        request_path="/v1/logs",
        request_origin="http://localhost:3000",
        secure_context=True,
        allowed_origins=("http://localhost:*",),
        ingest_paths=frozenset({"/v1/logs", "/v1/rum/assets"}),
    )

    assert updated.headers["X-Content-Type-Options"] == "nosniff"
    assert updated.headers["X-Frame-Options"] == "SAMEORIGIN"
    assert updated.headers["Strict-Transport-Security"] == "max-age=31536000; includeSubDomains"
    assert updated.headers["Access-Control-Allow-Origin"] == "http://localhost:3000"
    assert updated.headers["Access-Control-Allow-Credentials"] == "true"
    assert updated.headers["Access-Control-Allow-Methods"] == "POST, OPTIONS"
    assert updated.headers["Access-Control-Max-Age"] == "600"
    assert "X-SOBS-Asset-Signature" in updated.headers["Access-Control-Allow-Headers"]
    assert updated.headers["Vary"] == "Origin"


def test_shared_otlp_security_apply_security_headers_skips_cors_for_non_ingest_or_disallowed_origin():
    response = QuartResponse("")
    updated = _apply_security_headers(
        response,
        request_path="/v1/apps",
        request_origin="https://evil.example.com",
        secure_context=False,
        allowed_origins=("http://localhost:*",),
        ingest_paths=frozenset({"/v1/logs", "/v1/rum/assets"}),
    )

    assert "Strict-Transport-Security" not in updated.headers
    assert "Access-Control-Allow-Origin" not in updated.headers
