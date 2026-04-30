"""
Unit tests for the SOBS config module.

These tests cover the pure-Python helpers in ``config.py``:
environment-variable reading, path normalisation, and settings encryption.

No database or Quart application is required; all tests are synchronous.
"""

import pytest

import config as sobs_config

# ---------------------------------------------------------------------------
# _env_flag
# ---------------------------------------------------------------------------


class TestEnvFlag:
    def test_default_returned_when_not_set(self):
        assert sobs_config._env_flag("__TEST_FLAG_NOT_SET__", True) is True
        assert sobs_config._env_flag("__TEST_FLAG_NOT_SET__", False) is False

    @pytest.mark.parametrize("value", ["1", "true", "True", "TRUE", "yes", "YES", "on", "ON"])
    def test_truthy_values(self, value, monkeypatch):
        monkeypatch.setenv("__TEST_FLAG__", value)
        assert sobs_config._env_flag("__TEST_FLAG__", False) is True

    @pytest.mark.parametrize("value", ["0", "false", "no", "off", "", "  "])
    def test_falsy_values(self, value, monkeypatch):
        monkeypatch.setenv("__TEST_FLAG__", value)
        assert sobs_config._env_flag("__TEST_FLAG__", True) is False


# ---------------------------------------------------------------------------
# _normalize_base_path
# ---------------------------------------------------------------------------


class TestNormalizeBasePath:
    def test_empty_string_returns_empty(self):
        assert sobs_config._normalize_base_path("") == ""

    def test_single_slash_returns_empty(self):
        assert sobs_config._normalize_base_path("/") == ""

    def test_adds_leading_slash(self):
        assert sobs_config._normalize_base_path("foo") == "/foo"

    def test_strips_trailing_slash(self):
        assert sobs_config._normalize_base_path("/foo/") == "/foo"

    def test_collapses_double_slashes(self):
        assert sobs_config._normalize_base_path("//foo//bar//") == "/foo/bar"

    def test_normalizes_nested_path(self):
        assert sobs_config._normalize_base_path("/api/v1") == "/api/v1"


# ---------------------------------------------------------------------------
# _merge_script_name
# ---------------------------------------------------------------------------


class TestMergeScriptName:
    def test_empty_script_name_returns_empty(self):
        assert sobs_config._merge_script_name("", "/base") == ""

    def test_appends_base_path(self):
        assert sobs_config._merge_script_name("/prefix", "/base") == "/prefix/base"

    def test_no_double_append(self):
        assert sobs_config._merge_script_name("/prefix/base", "/base") == "/prefix/base"


# ---------------------------------------------------------------------------
# _read_env_or_file
# ---------------------------------------------------------------------------


class TestReadEnvOrFile:
    def test_reads_env_var(self, monkeypatch):
        monkeypatch.setenv("__TEST_RD_ENV__", "hello")
        assert sobs_config._read_env_or_file("__TEST_RD_ENV__") == "hello"

    def test_returns_empty_when_not_set(self):
        assert sobs_config._read_env_or_file("__TEST_RD_NOTSET__") == ""

    def test_reads_file_when_env_empty(self, monkeypatch, tmp_path):
        secret_file = tmp_path / "secret.txt"
        secret_file.write_text("from_file\n")
        monkeypatch.delenv("__TEST_RD_ENV__", raising=False)
        monkeypatch.setenv("__TEST_RD_FILE__", str(secret_file))
        assert sobs_config._read_env_or_file("__TEST_RD_ENV__", "__TEST_RD_FILE__") == "from_file"

    def test_env_takes_priority_over_file(self, monkeypatch, tmp_path):
        secret_file = tmp_path / "secret.txt"
        secret_file.write_text("from_file\n")
        monkeypatch.setenv("__TEST_RD_ENV__", "from_env")
        monkeypatch.setenv("__TEST_RD_FILE__", str(secret_file))
        assert sobs_config._read_env_or_file("__TEST_RD_ENV__", "__TEST_RD_FILE__") == "from_env"


# ---------------------------------------------------------------------------
# _read_file_or_env
# ---------------------------------------------------------------------------


class TestReadFileOrEnv:
    def test_reads_env_var_when_no_file_var(self, monkeypatch):
        monkeypatch.setenv("__TEST_RFE_ENV__", "env_value")
        assert sobs_config._read_file_or_env("__TEST_RFE_ENV__") == "env_value"

    def test_file_takes_priority_over_env(self, monkeypatch, tmp_path):
        secret_file = tmp_path / "secret.txt"
        secret_file.write_text("file_value\n")
        monkeypatch.setenv("__TEST_RFE_ENV__", "env_value")
        monkeypatch.setenv("__TEST_RFE_FILE__", str(secret_file))
        assert sobs_config._read_file_or_env("__TEST_RFE_ENV__", "__TEST_RFE_FILE__") == "file_value"


# ---------------------------------------------------------------------------
# _encrypt_secret_value / _decrypt_secret_value
# ---------------------------------------------------------------------------


class TestEncryptDecrypt:
    _PREFIX = "enc:v1:"

    def _patch_encryption_secret(self, monkeypatch, secret: str):
        """Patch only the module-level encryption secret."""
        monkeypatch.setattr(sobs_config, "_SETTINGS_ENCRYPTION_SECRET", secret)

    def test_no_encryption_without_secret(self, monkeypatch):
        self._patch_encryption_secret(monkeypatch, "")
        plaintext = "my_secret_value"
        assert sobs_config._encrypt_secret_value(plaintext) == plaintext

    def test_no_decryption_without_secret(self, monkeypatch):
        self._patch_encryption_secret(monkeypatch, "")
        # Must look like an encrypted value to trigger decryption path.
        fake_encrypted = self._PREFIX + "fake_token"
        assert sobs_config._decrypt_secret_value(fake_encrypted) == ""

    def test_empty_value_returned_unchanged(self, monkeypatch):
        self._patch_encryption_secret(monkeypatch, "test_key")
        assert sobs_config._encrypt_secret_value("") == ""
        assert sobs_config._decrypt_secret_value("") == ""

    def test_roundtrip(self, monkeypatch):
        self._patch_encryption_secret(monkeypatch, "test_encryption_key_32_chars!!")
        plaintext = "super_secret_password"
        encrypted = sobs_config._encrypt_secret_value(plaintext)
        assert encrypted.startswith(self._PREFIX)
        assert sobs_config._decrypt_secret_value(encrypted) == plaintext

    def test_already_encrypted_not_double_encrypted(self, monkeypatch):
        self._patch_encryption_secret(monkeypatch, "test_encryption_key_32_chars!!")
        plaintext = "value"
        encrypted = sobs_config._encrypt_secret_value(plaintext)
        double = sobs_config._encrypt_secret_value(encrypted)
        assert double == encrypted

    def test_non_encrypted_value_returned_unchanged_on_decrypt(self, monkeypatch):
        self._patch_encryption_secret(monkeypatch, "test_encryption_key_32_chars!!")
        assert sobs_config._decrypt_secret_value("plain_text") == "plain_text"


# ---------------------------------------------------------------------------
# Runtime constants
# ---------------------------------------------------------------------------


class TestConstants:
    def test_mobile_breakpoint_max_value(self):
        assert sobs_config.MOBILE_BREAKPOINT_MAX == "575.98px"

    def test_data_dir_is_string(self):
        assert isinstance(sobs_config.DATA_DIR, str)
        assert sobs_config.DATA_DIR  # non-empty

    def test_db_path_under_data_dir(self):
        assert sobs_config.DB_PATH.startswith(sobs_config.DATA_DIR)

    def test_rum_asset_dir_under_data_dir(self):
        assert sobs_config.RUM_ASSET_DIR.startswith(sobs_config.DATA_DIR)
