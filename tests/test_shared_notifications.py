import json

from shared.notifications import (
    _load_notification_channels,
    _load_notification_log,
    _load_notification_rules,
    _mask_channel_config,
    _normalize_notification_condition,
    _notification_channel_mask_output_enabled,
    _parse_notification_conditions_json,
)


class _FakeResult:
    def __init__(self, rows=None):
        self._rows = rows or []

    def fetchall(self):
        return self._rows


class _FakeDb:
    def __init__(self, mappings):
        self.mappings = mappings
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        for matcher, rows in self.mappings:
            if matcher in query:
                return _FakeResult(rows)
        return _FakeResult([])


def test_shared_notifications_load_channels_rules_and_log():
    db = _FakeDb(
        [
            (
                "FROM sobs_notification_channels FINAL",
                [
                    {
                        "Id": "ch-1",
                        "Name": "Webhook",
                        "ChannelType": "webhook",
                        "ConfigJson": '{"url":"https://example.com"}',
                        "Enabled": 1,
                    }
                ],
            ),
            (
                "FROM sobs_notification_rules FINAL",
                [
                    {
                        "Id": "rule-1",
                        "Name": "Rule",
                        "Enabled": 1,
                        "LogicOperator": "all",
                        "ConditionsJson": '[{"source":"logs","signal":"error_volume"}]',
                        "ChannelIds": "ch-1, ch-2",
                        "Severity": "critical",
                        "CooldownSeconds": 120,
                        "LastFiredAt": "2026-04-30 10:00:00",
                    }
                ],
            ),
            (
                "FROM sobs_notification_log ORDER BY FiredAt DESC LIMIT ?",
                [
                    {
                        "Id": "log-1",
                        "RuleId": "rule-1",
                        "RuleName": "Rule",
                        "ChannelId": "ch-1",
                        "ChannelName": "Webhook",
                        "FiredAt": "2026-04-30 10:01:00",
                        "Status": "ok",
                        "ErrorMessage": "",
                        "Summary": "summary",
                    }
                ],
            ),
        ]
    )

    channels = _load_notification_channels(db, decrypt_notification_config=lambda config: {**config, "decoded": True})
    assert channels == [
        {
            "id": "ch-1",
            "name": "Webhook",
            "channel_type": "webhook",
            "config": {"url": "https://example.com", "decoded": True},
            "enabled": True,
        }
    ]

    rules = _load_notification_rules(
        db,
        parse_notification_conditions_json=lambda raw: [{"raw": raw}],
    )
    assert rules == [
        {
            "id": "rule-1",
            "name": "Rule",
            "enabled": True,
            "logic_operator": "all",
            "conditions": [{"raw": '[{"source":"logs","signal":"error_volume"}]'}],
            "channel_ids": ["ch-1", "ch-2"],
            "severity": "critical",
            "cooldown_seconds": 120,
            "last_fired_at": "2026-04-30 10:00:00",
        }
    ]

    log_rows = _load_notification_log(db, limit=5)
    assert log_rows == [
        {
            "id": "log-1",
            "rule_id": "rule-1",
            "rule_name": "Rule",
            "channel_id": "ch-1",
            "channel_name": "Webhook",
            "fired_at": "2026-04-30 10:01:00",
            "status": "ok",
            "error_message": "",
            "summary": "summary",
        }
    ]
    assert db.calls[-1][1] == [5]


def test_shared_notifications_normalize_and_parse_conditions_cover_branches():
    def normalize(raw):
        return _normalize_notification_condition(
            raw,
            comparators=("gt", "lt", "gte", "lte", "eq"),
            tag_match_operators=("eq", "contains", "regex"),
            tag_record_types=("all", "log", "trace", "error", "ai", "rum"),
        )

    assert normalize(None) is None
    assert normalize({"type": "signal", "source": "logs", "signal": "error_volume", "service": "api"}) == {
        "type": "signal",
        "source": "logs",
        "signal": "error_volume",
        "service": "api",
        "comparator": "gt",
        "threshold": 0.0,
        "window_minutes": 5,
    }
    assert normalize(
        {
            "type": "signal",
            "source": "logs",
            "signal": "latency",
            "service": "api",
            "comparator": "bad",
            "threshold": "bad",
            "window_minutes": "bad",
        }
    ) == {
        "type": "signal",
        "source": "logs",
        "signal": "latency",
        "service": "api",
        "comparator": "gt",
        "threshold": 0.0,
        "window_minutes": 5,
    }
    assert normalize(
        {
            "type": "tag",
            "record_type": "bad",
            "tag_key": "env",
            "tag_match_operator": "bad",
            "tag_value": "prod",
            "comparator": "bad",
            "threshold": "bad",
            "window_minutes": "bad",
        }
    ) == {
        "type": "tag",
        "record_type": "all",
        "tag_key": "env",
        "tag_match_operator": "eq",
        "tag_value": "prod",
        "comparator": "gt",
        "threshold": 0.0,
        "window_minutes": 5,
    }

    assert _parse_notification_conditions_json("", normalize_notification_condition=normalize) == []
    assert _parse_notification_conditions_json("{bad-json", normalize_notification_condition=normalize) == []
    assert (
        _parse_notification_conditions_json(json.dumps({"bad": "shape"}), normalize_notification_condition=normalize)
        == []
    )
    assert _parse_notification_conditions_json(
        json.dumps([None, {"source": "logs", "signal": "error_volume", "window_minutes": 90}]),
        normalize_notification_condition=normalize,
    ) == [
        {
            "type": "signal",
            "source": "logs",
            "signal": "error_volume",
            "service": "",
            "comparator": "gt",
            "threshold": 0.0,
            "window_minutes": 60,
        }
    ]


def test_shared_notifications_masking_helpers_cover_sensitive_and_default_paths():
    masked = _mask_channel_config(
        "email",
        {"smtp_password": "secret", "auth_token": "tok", "api_key": "key", "name": "ok"},
    )
    assert masked == {
        "smtp_password": "••••••••",
        "auth_token": "••••••••",
        "api_key": "••••••••",
        "name": "ok",
    }

    def is_truthy_setting(value, default=True):
        return default if value == "" else value.lower() in {"1", "true", "yes"}

    assert _notification_channel_mask_output_enabled({}, is_truthy_setting=is_truthy_setting) is True
    assert (
        _notification_channel_mask_output_enabled(
            {"config": []},
            is_truthy_setting=is_truthy_setting,
        )
        is True
    )
    assert (
        _notification_channel_mask_output_enabled(
            {"config": {"mask_output_enabled": ""}},
            is_truthy_setting=is_truthy_setting,
        )
        is True
    )
    assert (
        _notification_channel_mask_output_enabled(
            {"config": {"mask_output_enabled": "0"}},
            is_truthy_setting=is_truthy_setting,
        )
        is False
    )
