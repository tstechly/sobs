from __future__ import annotations

from typing import Any


def _load_log_attr_keys_from_db(db, record_type: str) -> set[str]:
    rows = db.execute(
        "SELECT DISTINCT AttrKey FROM sobs_log_attr_keys FINAL WHERE RecordType=? AND IsDeleted=0 ORDER BY AttrKey",
        [record_type],
    ).fetchall()
    return {str(row[0]) for row in rows if str(row[0]).strip()}


def _prime_log_attr_key_cache(
    db,
    *,
    attr_key_record_types: tuple[str, ...],
    cache_state: dict[str, Any],
    load_log_attr_keys_from_db=_load_log_attr_keys_from_db,
) -> None:
    with cache_state["lock"]:
        if cache_state["loaded"]:
            return
        for record_type in attr_key_record_types:
            cache_state["by_record_type"][record_type] = load_log_attr_keys_from_db(db, record_type)
        cache_state["loaded"] = True


def _get_cached_attr_keys(
    db,
    record_type: str,
    *,
    attr_key_record_types: tuple[str, ...],
    cache_state: dict[str, Any],
    prime_log_attr_key_cache=_prime_log_attr_key_cache,
) -> list[str]:
    prime_log_attr_key_cache(
        db,
        attr_key_record_types=attr_key_record_types,
        cache_state=cache_state,
        load_log_attr_keys_from_db=_load_log_attr_keys_from_db,
    )
    with cache_state["lock"]:
        keys = sorted(cache_state["by_record_type"].get(record_type, set()))
    return keys


def _remember_attr_keys(
    db,
    attrs_maps: list[dict],
    record_type: str,
    *,
    attr_key_record_types: tuple[str, ...],
    cache_state: dict[str, Any],
    log_attr_keys_max: int,
    insert_rows_json_each_row,
    now_ms: int,
    logger: Any,
    prime_log_attr_key_cache=_prime_log_attr_key_cache,
) -> None:
    if not attrs_maps:
        return
    prime_log_attr_key_cache(
        db,
        attr_key_record_types=attr_key_record_types,
        cache_state=cache_state,
        load_log_attr_keys_from_db=_load_log_attr_keys_from_db,
    )

    with cache_state["lock"]:
        existing = cache_state["by_record_type"].setdefault(record_type, set())
        if len(existing) >= log_attr_keys_max:
            return

        candidates: set[str] = set()
        for attrs in attrs_maps:
            if not isinstance(attrs, dict):
                continue
            for raw_key in attrs.keys():
                key = str(raw_key).strip()
                if not key or key in existing or key in candidates:
                    continue
                if len(existing) + len(candidates) >= log_attr_keys_max:
                    break
                candidates.add(key)

        if not candidates:
            return

        rows = [
            {
                "RecordType": record_type,
                "AttrKey": key,
                "IsDeleted": 0,
                "Version": now_ms + idx,
            }
            for idx, key in enumerate(sorted(candidates))
        ]
        try:
            insert_rows_json_each_row(db, "sobs_log_attr_keys", rows)
            existing.update(candidates)
        except Exception:
            logger.exception("failed to persist discovered log attribute keys")


def _extract_attr_maps(rows: list[dict], attr_field: str) -> list[dict]:
    maps: list[dict] = []
    for row in rows:
        raw_attrs = row.get(attr_field, {})
        if isinstance(raw_attrs, dict):
            maps.append(raw_attrs)
    return maps
