from shared.rum_client_auth import (
    _normalize_origin,
    _request_origin,
    _rum_b64url_decode,
    _rum_b64url_encode,
    _rum_client_sign,
    _rum_client_token_decode,
    _rum_client_token_encode,
    _same_origin_request,
    _verify_rum_client_auth,
)


def test_rum_b64url_helpers_round_trip_and_handle_blank() -> None:
    encoded = _rum_b64url_encode(b'{"hello":"world"}')

    assert _rum_b64url_decode(encoded) == b'{"hello":"world"}'
    assert _rum_b64url_decode("") == b""


def test_normalize_origin_requires_scheme_and_host() -> None:
    assert _normalize_origin(" https://Example.COM/path?q=1 ") == "https://example.com"
    assert _normalize_origin("example.com") == ""
    assert _normalize_origin("") == ""


def test_request_origin_prefers_origin_then_referer() -> None:
    assert _request_origin({"Origin": "https://example.com/app"}) == "https://example.com"
    assert _request_origin({"Referer": "https://example.com/page?x=1"}) == "https://example.com"
    assert _request_origin({}) == ""


def test_same_origin_request_respects_forwarded_headers() -> None:
    assert (
        _same_origin_request(
            {
                "Origin": "https://public.example",
                "X-Forwarded-Host": "public.example",
                "X-Forwarded-Proto": "https",
            },
            host="internal:5000",
            scheme="http",
        )
        is True
    )
    assert _same_origin_request({"Referer": "https://example.com/page"}, host="example.com", scheme="https") is True
    assert _same_origin_request({"Origin": "https://evil.example"}, host="example.com", scheme="https") is False
    assert _same_origin_request({}, host="", scheme="https") is False


def test_rum_client_token_round_trip_and_signature_validation() -> None:
    claims = {"app": "my-app", "origin": "https://example.com", "exp": 12345}
    token = _rum_client_token_encode(claims, signing_key="secret")

    decoded, error = _rum_client_token_decode(token, signing_key="secret")

    assert error == ""
    assert decoded == claims
    assert _rum_client_sign(token.split(".", 1)[0], signing_key="secret") == token.split(".", 1)[1]


def test_rum_client_token_decode_rejects_bad_inputs() -> None:
    bad_signature = _rum_client_token_encode({"ok": True}, signing_key="secret")[:-1] + "0"
    payload_only = _rum_b64url_encode(b"[]") + "." + _rum_client_sign(_rum_b64url_encode(b"[]"), signing_key="secret")
    malformed_json = _rum_b64url_encode(b"{") + "." + _rum_client_sign(_rum_b64url_encode(b"{"), signing_key="secret")

    assert _rum_client_token_decode("missing-dot", signing_key="secret") == (None, "Invalid RUM client token format")
    assert _rum_client_token_decode(bad_signature, signing_key="secret") == (
        None,
        "Invalid RUM client token signature",
    )
    assert _rum_client_token_decode("bm90LWpzb24.invalid", signing_key="secret") == (
        None,
        "Invalid RUM client token signature",
    )
    assert _rum_client_token_decode(payload_only, signing_key="secret") == (None, "Invalid RUM client token payload")
    assert _rum_client_token_decode(malformed_json, signing_key="secret") == (None, "Invalid RUM client token payload")


def test_verify_rum_client_auth_accepts_disabled_mode() -> None:
    assert _verify_rum_client_auth([], mode="none", signing_key="", headers={}, host="", scheme="https") == (
        True,
        200,
        "",
    )


def test_verify_rum_client_auth_rejects_invalid_mode_and_missing_key() -> None:
    assert _verify_rum_client_auth([], mode="weird", signing_key="secret", headers={}, host="", scheme="https") == (
        False,
        500,
        "Invalid SOBS_RUM_CLIENT_AUTH_MODE",
    )
    assert _verify_rum_client_auth([], mode="origin", signing_key="", headers={}, host="", scheme="https") == (
        False,
        503,
        "RUM client signing key is not configured",
    )


def test_verify_rum_client_auth_rejects_missing_and_invalid_tokens() -> None:
    assert _verify_rum_client_auth(
        [],
        mode="origin",
        signing_key="secret",
        headers={},
        host="example.com",
        scheme="https",
    ) == (
        False,
        401,
        "Missing RUM client auth token",
    )

    assert _verify_rum_client_auth(
        [],
        mode="origin",
        signing_key="secret",
        headers={"X-SOBS-RUM-Token": "bad-token", "Origin": "https://example.com"},
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (False, 401, "Invalid RUM client token format")

    invalid_expiry_token = _rum_client_token_encode(
        {"origin": "https://example.com", "exp": "oops"},
        signing_key="secret",
    )
    assert _verify_rum_client_auth(
        [],
        mode="origin",
        signing_key="secret",
        headers={
            "X-SOBS-RUM-Token": invalid_expiry_token,
            "Origin": "https://example.com",
        },
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (False, 401, "Invalid RUM client token expiry")


def test_verify_rum_client_auth_rejects_expired_missing_origin_and_mismatch() -> None:
    expired = _rum_client_token_encode(
        {"origin": "https://example.com", "exp": 99},
        signing_key="secret",
    )
    missing_origin = _rum_client_token_encode({"exp": 200}, signing_key="secret")
    mismatched = _rum_client_token_encode(
        {"origin": "https://example.com", "exp": 200},
        signing_key="secret",
    )

    assert _verify_rum_client_auth(
        [],
        mode="origin",
        signing_key="secret",
        headers={"X-SOBS-RUM-Token": expired, "Origin": "https://example.com"},
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (False, 401, "RUM client token expired")
    assert _verify_rum_client_auth(
        [],
        mode="origin",
        signing_key="secret",
        headers={"X-SOBS-RUM-Token": missing_origin, "Origin": "https://example.com"},
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (False, 401, "RUM client token missing origin binding")
    assert _verify_rum_client_auth(
        [],
        mode="origin",
        signing_key="secret",
        headers={"X-SOBS-RUM-Token": mismatched},
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (False, 401, "Missing Origin/Referer for RUM client auth")
    assert _verify_rum_client_auth(
        [],
        mode="origin",
        signing_key="secret",
        headers={"X-SOBS-RUM-Token": mismatched, "Origin": "https://evil.example"},
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (False, 401, "RUM client token origin mismatch")


def test_verify_rum_client_auth_rejects_app_mismatch_and_accepts_event_token() -> None:
    token = _rum_client_token_encode(
        {"origin": "https://example.com", "app": "my-app", "exp": 200},
        signing_key="secret",
    )

    assert _verify_rum_client_auth(
        [{"appName": "other-app", "clientAuthToken": token}],
        mode="origin",
        signing_key="secret",
        headers={"Origin": "https://example.com"},
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (False, 401, "RUM client token app mismatch")
    assert _verify_rum_client_auth(
        ["ignore-me", {"appName": "my-app", "clientAuthToken": token}],
        mode="origin",
        signing_key="secret",
        headers={"Origin": "https://example.com"},
        host="example.com",
        scheme="https",
        current_time=100,
    ) == (True, 200, "")


def test_verify_rum_client_auth_uses_current_time_when_not_injected() -> None:
    token = _rum_client_token_encode(
        {"origin": "https://example.com", "app": "my-app", "exp": 4102444800},
        signing_key="secret",
    )

    assert _verify_rum_client_auth(
        [{"appName": "my-app", "clientAuthToken": token}],
        mode="origin",
        signing_key="secret",
        headers={"Origin": "https://example.com"},
        host="example.com",
        scheme="https",
    ) == (True, 200, "")
