from shared.ai_settings import _load_ai_setting, _load_all_ai_settings, _save_ai_setting


class _FakeResult:
    def __init__(self, row=None, rows=None):
        self._row = row
        self._rows = rows or []

    def fetchone(self):
        return self._row

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, row=None, rows=None):
        self.row = row
        self.rows = rows or []
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        if "LIMIT 1" in query:
            return _FakeResult(row=self.row)
        return _FakeResult(rows=self.rows)


def test_shared_ai_settings_load_returns_db_values_and_falls_back_to_env_or_default():
    db = _FakeDb(row={"Value": "encrypted-token"})
    value = _load_ai_setting(
        db,
        "ai.api_key",
        default="default-value",
        decrypt_secret_value=lambda raw: f"decrypted:{raw}",
        is_sensitive_ai_setting_key=lambda key: key == "ai.api_key",
        ai_env_overrides={"ai.api_key": ("AI_API_KEY", "AI_API_KEY_FILE")},
        read_file_or_env=lambda _env, _file: "env-value",
    )
    assert value == "decrypted:encrypted-token"

    env_db = _FakeDb(row={"Value": ""})
    value = _load_ai_setting(
        env_db,
        "ai.endpoint_url",
        default="default-value",
        decrypt_secret_value=lambda raw: raw,
        is_sensitive_ai_setting_key=lambda _key: False,
        ai_env_overrides={"ai.endpoint_url": ("AI_ENDPOINT_URL", "AI_ENDPOINT_URL_FILE")},
        read_file_or_env=lambda _env, _file: "https://env.example.com/v1",
    )
    assert value == "https://env.example.com/v1"

    default_db = _FakeDb(row=None)
    value = _load_ai_setting(
        default_db,
        "ai.model",
        default="gpt-default",
        decrypt_secret_value=lambda raw: raw,
        is_sensitive_ai_setting_key=lambda _key: False,
        ai_env_overrides={},
        read_file_or_env=lambda _env, _file: "",
    )
    assert value == "gpt-default"


def test_shared_ai_settings_save_encrypts_sensitive_values_and_inserts_versioned_row():
    inserted = []
    db = object()

    _save_ai_setting(
        db,
        "ai.api_key",
        "secret-token",
        encrypt_secret_value=lambda raw: f"enc:{raw}",
        is_sensitive_ai_setting_key=lambda key: key == "ai.api_key",
        insert_rows_json_each_row=lambda current_db, table, rows: inserted.append((current_db, table, rows)),
        now=lambda: 123.456,
    )

    assert inserted == [
        (
            db,
            "sobs_ai_settings",
            [{"Key": "ai.api_key", "Value": "enc:secret-token", "IsDeleted": 0, "Version": 123456}],
        )
    ]


def test_shared_ai_settings_load_all_uses_db_first_and_env_for_missing_values():
    db = _FakeDb(
        rows=[
            {"Key": "ai.endpoint_url", "Value": "https://db.example.com/v1"},
            {"Key": "ai.api_key", "Value": "encrypted-key"},
            {"Key": "unknown.key", "Value": "ignored"},
        ]
    )
    settings = _load_all_ai_settings(
        db,
        decrypt_secret_value=lambda raw: f"decrypted:{raw}",
        is_sensitive_ai_setting_key=lambda key: key == "ai.api_key",
        ai_setting_keys=["ai.endpoint_url", "ai.api_key", "ai.model"],
        ai_env_overrides={
            "ai.endpoint_url": ("AI_ENDPOINT_URL", "AI_ENDPOINT_URL_FILE"),
            "ai.model": ("AI_MODEL", "AI_MODEL_FILE"),
        },
        read_file_or_env=lambda env, _file: {
            "AI_ENDPOINT_URL": "https://env.example.com/v1",
            "AI_MODEL": "gpt-env",
        }.get(env, ""),
    )

    assert settings == {
        "ai.endpoint_url": "https://db.example.com/v1",
        "ai.api_key": "decrypted:encrypted-key",
        "ai.model": "gpt-env",
    }
