"""
SOBS application configuration.

Centralises environment-variable reading, path normalisation, settings
encryption and core runtime constants.  This module has **no** SOBS-specific
imports beyond the Python standard library and the optional ``cryptography``
package used for Fernet-based settings encryption.

Environment variables
---------------------
The following variables are read at import time to populate the constants
exported by this module:

``SOBS_DATA_DIR``
    Root directory for all persisted data (default: ``./data``).
``SOBS_API_KEY``
    Static API key for the ingest / API endpoints.  Empty = no key required.
``SOBS_BASIC_AUTH_USERNAME`` / ``SOBS_BASIC_AUTH_PASSWORD``
    HTTP Basic Auth credentials for the web UI.  Both must be set or neither.
``SOBS_EXTERNAL_AUTH_URL``
    External auth service URL.  Mutually exclusive with Basic Auth.
``SOBS_BASE_PATH``
    Optional URL path prefix when SOBS is served behind a reverse proxy.
``SOBS_BEHIND_TLS``
    Set to ``1`` / ``true`` if SOBS sits behind a TLS-terminating proxy
    (enables secure cookies and strict CSRF checking).
``SOBS_SETTINGS_ENCRYPTION_KEY`` / ``SOBS_SETTINGS_ENCRYPTION_KEY_FILE``
    Fernet encryption key (or path to a file containing it) used to encrypt
    sensitive settings values at rest.
``SOBS_RUM_ASSET_SIGNING_KEY``
    HMAC key for RUM asset upload request signing.
``SOBS_RUM_CLIENT_AUTH_MODE``
    RUM client authentication mode: ``none``, ``origin``, or
    ``origin-session``.
``SOBS_BUILD_VERSION``
    Build / release version string injected at image-build time.
"""

from __future__ import annotations

import base64
import hashlib
import logging
import os
import re

__all__ = [
    # Utility functions
    "_env_flag",
    "_normalize_base_path",
    "_merge_script_name",
    "_read_env_or_file",
    "_read_file_or_env",
    # Encryption
    "_SETTINGS_ENCRYPTION_PREFIX",
    "_SETTINGS_ENCRYPTION_KEY_ENV",
    "_SETTINGS_ENCRYPTION_KEY_FILE_ENV",
    "_SETTINGS_ENCRYPTION_SECRET",
    "_load_settings_encryption_secret",
    "_encrypt_secret_value",
    "_decrypt_secret_value",
    # Runtime constants
    "DATA_DIR",
    "DB_PATH",
    "RUM_ASSET_DIR",
    "API_KEY",
    "BASIC_AUTH_USERNAME",
    "BASIC_AUTH_PASSWORD",
    "EXTERNAL_AUTH_URL",
    "BASE_PATH",
    "RUM_ASSET_SIGNING_KEY",
    "RUM_ASSET_SIGN_WINDOW_SEC",
    "RUM_ASSET_MAX_BYTES",
    "RUM_CLIENT_AUTH_MODE",
    "RUM_CLIENT_SIGNING_KEY",
    "RUM_CLIENT_TOKEN_TTL_SEC",
    "SOURCE_MAP_DIR",
    "SOURCE_MAP_ENABLE",
    "BUILD_VERSION",
    "MOBILE_BREAKPOINT_MAX",
    # chDB env-variable name constants
    "APP_REGISTRY_SEED_JSON_ENV",
    "APP_REGISTRY_SEED_JSON_FILE_ENV",
    "CHDB_CONFIG_FILE_ENV",
    "CHDB_EXPECT_DISK_ENV",
    "CHDB_EXPECT_POLICY_ENV",
    "CHDB_MAX_SERVER_MB_ENV",
    "CHDB_MARK_CACHE_MB_ENV",
    "CHDB_UNCOMPRESSED_CACHE_MB_ENV",
    "CHDB_MAX_THREADS_ENV",
    "CHDB_SPILL_GROUP_BY_MB_ENV",
    "CHDB_SPILL_SORT_MB_ENV",
]

_log = logging.getLogger("sobs")

# ---------------------------------------------------------------------------
# Utility: environment-variable helpers
# ---------------------------------------------------------------------------


def _env_flag(name: str, default: bool) -> bool:
    """Return *True* when the named env-var is set to a truthy value."""
    raw = os.environ.get(name)
    if raw is None:
        return default
    return str(raw).strip().lower() in {"1", "true", "yes", "on"}


def _normalize_base_path(value: str) -> str:
    """Normalize base path values to either ``''`` or ``'/segment[/sub]'``."""
    if not value:
        return ""
    normalized = re.sub(r"/+", "/", str(value).strip())
    if not normalized or normalized == "/":
        return ""
    if not normalized.startswith("/"):
        normalized = "/" + normalized
    normalized = normalized.rstrip("/")
    return normalized if normalized != "/" else ""


def _merge_script_name(script_name: str, base_path: str) -> str:
    """Append *base_path* to *script_name* exactly once."""
    if not script_name:
        return script_name or ""
    current = script_name or ""
    if current.endswith(base_path):
        return current
    if not current:
        return base_path
    return current.rstrip("/") + base_path


def _read_env_or_file(env_var: str, file_env_var: str = "") -> str:
    """Return the value of *env_var*, falling back to reading *file_env_var*.

    Priority: env-var value → file path from *file_env_var* → empty string.
    """
    value = os.environ.get(env_var, "").strip()
    if value:
        return value
    if not file_env_var:
        return ""
    file_path = os.environ.get(file_env_var, "").strip()
    if not file_path:
        return ""
    try:
        with open(file_path, encoding="utf-8") as handle:
            return handle.read().strip()
    except Exception as exc:
        _log.warning("Failed to read %s from file %s: %s", env_var, file_path, exc)
        return ""


def _read_file_or_env(env_var: str, file_env_var: str = "") -> str:
    """Return a value preferring the *file_env_var* path over *env_var*.

    Priority: file path from *file_env_var* → env-var value → empty string.
    """
    if file_env_var:
        file_path = os.environ.get(file_env_var, "").strip()
        if file_path:
            try:
                with open(file_path, encoding="utf-8") as handle:
                    file_value = handle.read().strip()
                if file_value:
                    return file_value
            except Exception as exc:
                _log.warning("Failed to read %s from file %s: %s", env_var, file_path, exc)
    return os.environ.get(env_var, "").strip()


# ---------------------------------------------------------------------------
# Settings encryption (Fernet / AES-128-CBC via cryptography package)
# ---------------------------------------------------------------------------

_SETTINGS_ENCRYPTION_PREFIX = "enc:v1:"
_SETTINGS_ENCRYPTION_KEY_ENV = "SOBS_SETTINGS_ENCRYPTION_KEY"
_SETTINGS_ENCRYPTION_KEY_FILE_ENV = "SOBS_SETTINGS_ENCRYPTION_KEY_FILE"


def _load_settings_encryption_secret() -> str:
    """Load the Fernet encryption secret from env or file (may be empty)."""
    return _read_env_or_file(_SETTINGS_ENCRYPTION_KEY_ENV, _SETTINGS_ENCRYPTION_KEY_FILE_ENV)


# Resolved once at import time; re-read on every call would be wasteful.
_SETTINGS_ENCRYPTION_SECRET: str = _load_settings_encryption_secret()


def _encrypt_secret_value(value: str) -> str:
    """Encrypt *value* with Fernet using the configured secret.

    Returns *value* unchanged when the secret is not configured or the value
    is already encrypted.
    """
    if not value or not _SETTINGS_ENCRYPTION_SECRET:
        return value
    if value.startswith(_SETTINGS_ENCRYPTION_PREFIX):
        return value
    try:
        from cryptography.fernet import Fernet

        digest = hashlib.sha256(_SETTINGS_ENCRYPTION_SECRET.encode("utf-8")).digest()
        key = base64.urlsafe_b64encode(digest)
        token = Fernet(key).encrypt(value.encode("utf-8")).decode("utf-8")
        return _SETTINGS_ENCRYPTION_PREFIX + token
    except Exception as exc:
        _log.warning("Failed to encrypt secret setting: %s", exc)
        return value


def _decrypt_secret_value(value: str) -> str:
    """Decrypt a Fernet-encrypted *value*.

    Returns an empty string when decryption fails or the key is missing.
    Returns *value* unchanged when the value is not encrypted.
    """
    if not value:
        return value
    if not value.startswith(_SETTINGS_ENCRYPTION_PREFIX):
        return value
    if not _SETTINGS_ENCRYPTION_SECRET:
        _log.warning("Encrypted setting found but no decryption key is configured")
        return ""
    token = value[len(_SETTINGS_ENCRYPTION_PREFIX):]
    try:
        from cryptography.fernet import Fernet, InvalidToken

        digest = hashlib.sha256(_SETTINGS_ENCRYPTION_SECRET.encode("utf-8")).digest()
        key = base64.urlsafe_b64encode(digest)
        return Fernet(key).decrypt(token.encode("utf-8")).decode("utf-8")
    except InvalidToken:
        _log.warning("Failed to decrypt setting value: invalid encryption key")
        return ""
    except Exception as exc:
        _log.warning("Failed to decrypt secret setting: %s", exc)
        return ""


# ---------------------------------------------------------------------------
# Runtime constants derived from environment variables
# ---------------------------------------------------------------------------

#: Root directory for all SOBS persisted data.
DATA_DIR: str = os.environ.get("SOBS_DATA_DIR", os.path.join(os.path.dirname(__file__), "data"))

#: Absolute path to the chDB database directory.
DB_PATH: str = os.path.join(DATA_DIR, "sobs.chdb")

#: Directory where RUM session-replay assets are stored on disk.
RUM_ASSET_DIR: str = os.path.join(DATA_DIR, "rum_assets")

#: Static API key for ingest/API endpoints.  Empty string = no auth required.
API_KEY: str = os.environ.get("SOBS_API_KEY", "")

#: HTTP Basic Auth username for the web UI (empty = disabled).
BASIC_AUTH_USERNAME: str = os.environ.get("SOBS_BASIC_AUTH_USERNAME", "")

#: HTTP Basic Auth password for the web UI.
BASIC_AUTH_PASSWORD: str = os.environ.get("SOBS_BASIC_AUTH_PASSWORD", "")

#: External auth service URL (empty = disabled).
EXTERNAL_AUTH_URL: str = os.environ.get("SOBS_EXTERNAL_AUTH_URL", "")

#: URL path prefix when served behind a reverse proxy (e.g. ``/sobs``).
BASE_PATH: str = _normalize_base_path(os.environ.get("SOBS_BASE_PATH", ""))

#: ``True`` when SOBS sits behind a TLS-terminating proxy.
_BEHIND_TLS: bool = _env_flag("SOBS_BEHIND_TLS", False)

#: HMAC key used to sign RUM asset upload requests.
RUM_ASSET_SIGNING_KEY: str = os.environ.get("SOBS_RUM_ASSET_SIGNING_KEY", "")

#: Maximum age (in seconds) for a valid RUM asset upload signature.
RUM_ASSET_SIGN_WINDOW_SEC: int = int(os.environ.get("SOBS_RUM_ASSET_SIGN_WINDOW_SEC", "300"))

#: Maximum size (bytes) for a single RUM asset upload.
RUM_ASSET_MAX_BYTES: int = int(os.environ.get("SOBS_RUM_ASSET_MAX_BYTES", str(8 * 1024 * 1024)))

#: RUM client authentication mode: ``none``, ``origin``, or ``origin-session``.
RUM_CLIENT_AUTH_MODE: str = os.environ.get("SOBS_RUM_CLIENT_AUTH_MODE", "none").strip().lower()

#: HMAC signing key for RUM client auth tokens.
RUM_CLIENT_SIGNING_KEY: str = os.environ.get("SOBS_RUM_CLIENT_SIGNING_KEY", "")

#: Lifetime (seconds) for issued RUM client auth tokens.
RUM_CLIENT_TOKEN_TTL_SEC: int = int(os.environ.get("SOBS_RUM_CLIENT_TOKEN_TTL_SEC", "900"))

#: Whether CSRF origin checking is enabled.
CSRF_ORIGIN_CHECK: bool = _env_flag("SOBS_CSRF_ORIGIN_CHECK", _BEHIND_TLS)

#: Directory to serve JavaScript source maps from.
SOURCE_MAP_DIR: str = os.environ.get("SOBS_SOURCE_MAP_DIR", "").strip()

#: Whether to serve JS source maps.
SOURCE_MAP_ENABLE: bool = _env_flag("SOBS_SOURCE_MAP_ENABLE", False)

#: Build / release version string (empty in dev).
BUILD_VERSION: str = os.environ.get("SOBS_BUILD_VERSION", "").strip()

#: CSS max-width token for the mobile responsive breakpoint.
MOBILE_BREAKPOINT_MAX: str = "575.98px"

# ---------------------------------------------------------------------------
# Environment variable name constants for chDB tuning
# ---------------------------------------------------------------------------

APP_REGISTRY_SEED_JSON_ENV = "SOBS_APP_REGISTRY_SEED_JSON"
APP_REGISTRY_SEED_JSON_FILE_ENV = "SOBS_APP_REGISTRY_SEED_JSON_FILE"
CHDB_CONFIG_FILE_ENV = "SOBS_CLICKHOUSE_CONFIG_FILE"
CHDB_EXPECT_DISK_ENV = "SOBS_CHDB_EXPECT_DISK"
CHDB_EXPECT_POLICY_ENV = "SOBS_CHDB_EXPECT_STORAGE_POLICY"
CHDB_MAX_SERVER_MB_ENV = "SOBS_CHDB_MAX_SERVER_MB"
CHDB_MARK_CACHE_MB_ENV = "SOBS_CHDB_MARK_CACHE_MB"
CHDB_UNCOMPRESSED_CACHE_MB_ENV = "SOBS_CHDB_UNCOMPRESSED_CACHE_MB"
CHDB_MAX_THREADS_ENV = "SOBS_CHDB_MAX_THREADS"
CHDB_SPILL_GROUP_BY_MB_ENV = "SOBS_CHDB_SPILL_GROUP_BY_MB"
CHDB_SPILL_SORT_MB_ENV = "SOBS_CHDB_SPILL_SORT_MB"
