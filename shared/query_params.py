from __future__ import annotations

from datetime import datetime, timedelta
from typing import Any, Callable, Mapping


def _parse_limit(request_args: Mapping[str, Any], default: int = 200) -> int:
    try:
        return max(1, min(int(request_args.get("limit", default)), 5000))
    except (TypeError, ValueError):
        return default


def _parse_offset(request_args: Mapping[str, Any]) -> int:
    try:
        return max(0, int(request_args.get("offset", 0)))
    except (TypeError, ValueError):
        return 0


def _parse_sort(
    request_args: Mapping[str, Any],
    allowed: dict[str, str],
    default_col: str = "Timestamp",
) -> tuple[str, str, str]:
    sort_by = str(request_args.get("sort_by", default_col))
    sort_dir = str(request_args.get("sort_dir", "desc")).lower()
    if sort_by not in allowed:
        sort_by = default_col
    if sort_dir not in ("asc", "desc"):
        sort_dir = "desc"
    return sort_by, allowed[sort_by], sort_dir


def _parse_time_window_args(
    request_args: Mapping[str, Any],
    *,
    normalize_ch_timestamp: Callable[[Any], str],
) -> tuple[str, str, str]:
    from_ts_raw = str(request_args.get("from_ts", "")).strip()
    to_ts_raw = str(request_args.get("to_ts", "")).strip()
    window_s_raw = str(request_args.get("window_s", "")).strip()

    try:
        from_ts = normalize_ch_timestamp(from_ts_raw) if from_ts_raw else ""
        to_ts = normalize_ch_timestamp(to_ts_raw) if to_ts_raw else ""
        if from_ts and not to_ts and window_s_raw:
            window_s = max(1, int(window_s_raw))
            from_dt = datetime.fromisoformat(from_ts)
            to_ts = normalize_ch_timestamp(from_dt + timedelta(seconds=window_s))
        if from_ts and to_ts:
            from_dt = datetime.fromisoformat(from_ts)
            to_dt = datetime.fromisoformat(to_ts)
            if to_dt <= from_dt:
                return "", "", "Invalid time window: to_ts must be later than from_ts"
        return from_ts, to_ts, ""
    except (TypeError, ValueError):
        return "", "", "Invalid time value. Use ISO-8601, e.g. 2026-03-29T12:00:00Z"
