from datetime import datetime, timezone

from shared.query_params import _parse_limit, _parse_offset, _parse_sort, _parse_time_window_args


def _normalize(value: object) -> str:
    if isinstance(value, datetime):
        return value.astimezone(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")
    text = str(value)
    if text.endswith("Z"):
        text = text[:-1] + "+00:00"
    return datetime.fromisoformat(text).isoformat()


def test_parse_limit_clamps_and_falls_back_to_default() -> None:
    assert _parse_limit({"limit": "10"}) == 10
    assert _parse_limit({"limit": "-5"}) == 1
    assert _parse_limit({"limit": "999999"}) == 5000
    assert _parse_limit({"limit": "oops"}, default=25) == 25


def test_parse_offset_clamps_negative_and_invalid_values() -> None:
    assert _parse_offset({"offset": "15"}) == 15
    assert _parse_offset({"offset": "-8"}) == 0
    assert _parse_offset({"offset": "oops"}) == 0


def test_parse_sort_validates_column_and_direction() -> None:
    allowed = {"Timestamp": "Timestamp", "SeverityText": "SeverityText"}

    assert _parse_sort({"sort_by": "SeverityText", "sort_dir": "ASC"}, allowed) == (
        "SeverityText",
        "SeverityText",
        "asc",
    )
    assert _parse_sort({"sort_by": "bogus", "sort_dir": "sideways"}, allowed) == (
        "Timestamp",
        "Timestamp",
        "desc",
    )


def test_parse_time_window_args_supports_window_seconds_and_validation() -> None:
    from_ts, to_ts, error = _parse_time_window_args(
        {"from_ts": "2026-03-29T12:00:00Z", "window_s": "30"},
        normalize_ch_timestamp=_normalize,
    )

    assert error == ""
    assert from_ts == "2026-03-29T12:00:00+00:00"
    assert to_ts == "2026-03-29T12:00:30Z"

    assert _parse_time_window_args(
        {"from_ts": "2026-03-29T12:00:00Z", "to_ts": "2026-03-29T11:59:59Z"},
        normalize_ch_timestamp=_normalize,
    ) == ("", "", "Invalid time window: to_ts must be later than from_ts")


def test_parse_time_window_args_rejects_invalid_values() -> None:
    assert _parse_time_window_args({"from_ts": "not-a-time"}, normalize_ch_timestamp=_normalize) == (
        "",
        "",
        "Invalid time value. Use ISO-8601, e.g. 2026-03-29T12:00:00Z",
    )
    assert _parse_time_window_args({}, normalize_ch_timestamp=_normalize) == ("", "", "")
