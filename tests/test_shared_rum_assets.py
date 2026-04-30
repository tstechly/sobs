import hashlib
import hmac
import secrets

from shared.rum_assets import (
    _asset_extension,
    _rum_asset_signature,
    _rum_asset_signature_payload,
    _sanitize_rum_asset_name,
    _sanitize_rum_asset_type,
    _verify_rum_asset_signature,
)


def _signed_headers(
    *,
    secret: str,
    body: bytes,
    method: str = "POST",
    path: str = "/v1/rum/assets",
    timestamp: str = "100",
    content_type: str = "application/json",
    asset_type: str = "replay",
    asset_name: str = "events.json",
):
    payload = _rum_asset_signature_payload(
        method=method,
        path=path,
        timestamp=timestamp,
        body_sha256=hashlib.sha256(body).hexdigest(),
        content_type=content_type,
        asset_type=asset_type,
        asset_name=asset_name,
    )
    return {
        "X-SOBS-Asset-Timestamp": timestamp,
        "X-SOBS-Asset-Signature": hmac.new(secret.encode("utf-8"), payload.encode("utf-8"), hashlib.sha256)
        .hexdigest()
        .upper(),
    }


def test_shared_rum_assets_sanitize_helpers_cover_empty_and_fallback_paths():
    assert _sanitize_rum_asset_name(" ../bad path/rrweb events.json ") == "rrweb-events.json"
    assert _sanitize_rum_asset_name("...") == "asset"
    assert _sanitize_rum_asset_name("") == "asset"

    assert _sanitize_rum_asset_type(" Replay Events ") == "replay-events"
    assert _sanitize_rum_asset_type("...") == "asset"
    assert _sanitize_rum_asset_type("") == "asset"


def test_shared_rum_assets_asset_extension_uses_existing_extension_and_mime_fallbacks():
    assert _asset_extension("rrweb.json", "application/octet-stream") == "json"
    assert _asset_extension("rrweb.invalid-extension", "application/json") == "json"
    assert _asset_extension("rrweb", "text/plain; charset=utf-8") == "txt"
    assert _asset_extension("rrweb", "application/x-custom") == "bin"


def test_shared_rum_assets_signature_payload_and_signature_are_stable():
    payload = _rum_asset_signature_payload(
        method="post",
        path="/v1/rum/assets",
        timestamp="123",
        body_sha256="deadbeef",
        content_type="Application/JSON ",
        asset_type=" Replay ",
        asset_name="rrweb.json",
    )
    assert payload == "POST\n/v1/rum/assets\n123\ndeadbeef\napplication/json\nreplay\nrrweb.json"
    assert (
        _rum_asset_signature("secret", payload)
        == hmac.new(b"secret", payload.encode("utf-8"), hashlib.sha256).hexdigest()
    )


def test_shared_rum_assets_verify_rejects_missing_key_headers_and_invalid_timestamp():
    body = b'{"events":[]}'
    assert _verify_rum_asset_signature(
        body=body,
        method="POST",
        path="/v1/rum/assets",
        content_type="application/json",
        asset_type="replay",
        asset_name="events.json",
        rum_asset_signing_key="",
        request_headers={},
        now=100,
        rum_asset_sign_window_sec=300,
        compare_digest=secrets.compare_digest,
    ) == (False, "Asset upload signing key is not configured")

    assert _verify_rum_asset_signature(
        body=body,
        method="POST",
        path="/v1/rum/assets",
        content_type="application/json",
        asset_type="replay",
        asset_name="events.json",
        rum_asset_signing_key="secret",
        request_headers={},
        now=100,
        rum_asset_sign_window_sec=300,
        compare_digest=secrets.compare_digest,
    ) == (False, "Missing asset signature headers")

    assert _verify_rum_asset_signature(
        body=body,
        method="POST",
        path="/v1/rum/assets",
        content_type="application/json",
        asset_type="replay",
        asset_name="events.json",
        rum_asset_signing_key="secret",
        request_headers={"X-SOBS-Asset-Timestamp": "bad", "X-SOBS-Asset-Signature": "deadbeef"},
        now=100,
        rum_asset_sign_window_sec=300,
        compare_digest=secrets.compare_digest,
    ) == (False, "Invalid asset signature timestamp")


def test_shared_rum_assets_verify_rejects_outside_window_and_invalid_signature():
    body = b'{"events":[]}'
    headers = _signed_headers(secret="secret", body=body, timestamp="50")
    assert _verify_rum_asset_signature(
        body=body,
        method="POST",
        path="/v1/rum/assets",
        content_type="application/json",
        asset_type="replay",
        asset_name="events.json",
        rum_asset_signing_key="secret",
        request_headers=headers,
        now=100,
        rum_asset_sign_window_sec=10,
        compare_digest=secrets.compare_digest,
    ) == (False, "Asset signature timestamp outside allowed window")

    assert _verify_rum_asset_signature(
        body=body,
        method="POST",
        path="/v1/rum/assets",
        content_type="application/json",
        asset_type="replay",
        asset_name="events.json",
        rum_asset_signing_key="secret",
        request_headers={
            "X-SOBS-Asset-Timestamp": "100",
            "X-SOBS-Asset-Signature": "deadbeef",
        },
        now=100,
        rum_asset_sign_window_sec=300,
        compare_digest=secrets.compare_digest,
    ) == (False, "Invalid asset signature")


def test_shared_rum_assets_verify_accepts_valid_signature():
    body = b'{"events":[{"type":"meta"}]}'
    headers = _signed_headers(secret="secret", body=body)
    assert _verify_rum_asset_signature(
        body=body,
        method="POST",
        path="/v1/rum/assets",
        content_type="application/json",
        asset_type="replay",
        asset_name="events.json",
        rum_asset_signing_key="secret",
        request_headers=headers,
        now=100,
        rum_asset_sign_window_sec=300,
        compare_digest=secrets.compare_digest,
    ) == (True, "")
