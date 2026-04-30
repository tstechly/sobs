import threading

from shared.log_attr_keys import (
    _extract_attr_maps,
    _get_cached_attr_keys,
    _load_log_attr_keys_from_db,
    _prime_log_attr_key_cache,
    _remember_attr_keys,
)


class _FakeCursor:
    def __init__(self, rows):
        self.rows = rows

    def fetchall(self):
        return self.rows


class _FakeDb:
    def __init__(self, rows):
        self.rows = rows
        self.calls = []

    def execute(self, query, params=None):
        self.calls.append((query, params))
        return _FakeCursor(self.rows)


class _Logger:
    def __init__(self):
        self.messages = []

    def exception(self, message):
        self.messages.append(message)


def _cache_state():
    return {"lock": threading.Lock(), "loaded": False, "by_record_type": {"log": set(), "span": set()}}


def test_shared_log_attr_keys_load_filters_blank_values():
    db = _FakeDb([("http.route",), ("",), (" service.name ",)])
    assert _load_log_attr_keys_from_db(db, "log") == {"http.route", " service.name "}


def test_shared_log_attr_keys_prime_and_get_cached_keys_load_once_and_sort():
    db = _FakeDb([("z-key",), ("a-key",)])
    state = _cache_state()

    _prime_log_attr_key_cache(
        db,
        attr_key_record_types=("log", "span"),
        cache_state=state,
        load_log_attr_keys_from_db=lambda db, record_type: {f"{record_type}-b", f"{record_type}-a"},
    )
    _prime_log_attr_key_cache(
        db,
        attr_key_record_types=("log", "span"),
        cache_state=state,
        load_log_attr_keys_from_db=lambda db, record_type: {"should-not-run"},
    )

    assert state["loaded"] is True
    assert _get_cached_attr_keys(db, "log", attr_key_record_types=("log", "span"), cache_state=state) == [
        "log-a",
        "log-b",
    ]


def test_shared_log_attr_keys_remember_persists_new_keys_and_updates_cache():
    inserts = []
    state = {"lock": threading.Lock(), "loaded": True, "by_record_type": {"log": {"existing"}}}

    _remember_attr_keys(
        object(),
        attrs_maps=[{" existing ": 1, "http.route": 2, "service.name": 3}, {"service.name": 4, "": 5}, []],
        record_type="log",
        attr_key_record_types=("log",),
        cache_state=state,
        log_attr_keys_max=3,
        insert_rows_json_each_row=lambda db, table, rows: inserts.append((table, rows)),
        now_ms=100,
        logger=_Logger(),
    )

    assert inserts == [
        (
            "sobs_log_attr_keys",
            [
                {"RecordType": "log", "AttrKey": "http.route", "IsDeleted": 0, "Version": 100},
                {"RecordType": "log", "AttrKey": "service.name", "IsDeleted": 0, "Version": 101},
            ],
        )
    ]
    assert state["by_record_type"]["log"] == {"existing", "http.route", "service.name"}


def test_shared_log_attr_keys_remember_respects_limit_and_logs_insert_failures():
    logger = _Logger()
    limited_state = {"lock": threading.Lock(), "loaded": True, "by_record_type": {"log": {"a", "b"}}}
    _remember_attr_keys(
        object(),
        attrs_maps=[{"c": 1}],
        record_type="log",
        attr_key_record_types=("log",),
        cache_state=limited_state,
        log_attr_keys_max=2,
        insert_rows_json_each_row=lambda db, table, rows: None,
        now_ms=100,
        logger=logger,
    )
    assert limited_state["by_record_type"]["log"] == {"a", "b"}

    failing_state = {"lock": threading.Lock(), "loaded": True, "by_record_type": {"log": set()}}
    _remember_attr_keys(
        object(),
        attrs_maps=[{"service.name": 1}],
        record_type="log",
        attr_key_record_types=("log",),
        cache_state=failing_state,
        log_attr_keys_max=10,
        insert_rows_json_each_row=lambda db, table, rows: (_ for _ in ()).throw(RuntimeError("boom")),
        now_ms=100,
        logger=logger,
    )
    assert logger.messages == ["failed to persist discovered log attribute keys"]
    assert failing_state["by_record_type"]["log"] == set()


def test_shared_log_attr_keys_remember_noops_for_empty_or_duplicate_only_candidates():
    inserts = []
    state = {"lock": threading.Lock(), "loaded": True, "by_record_type": {"log": {"existing"}}}

    _remember_attr_keys(
        object(),
        attrs_maps=[],
        record_type="log",
        attr_key_record_types=("log",),
        cache_state=state,
        log_attr_keys_max=10,
        insert_rows_json_each_row=lambda db, table, rows: inserts.append((table, rows)),
        now_ms=100,
        logger=_Logger(),
    )
    _remember_attr_keys(
        object(),
        attrs_maps=[{}, {" existing ": 1}, []],
        record_type="log",
        attr_key_record_types=("log",),
        cache_state=state,
        log_attr_keys_max=10,
        insert_rows_json_each_row=lambda db, table, rows: inserts.append((table, rows)),
        now_ms=100,
        logger=_Logger(),
    )

    assert inserts == []


def test_shared_log_attr_keys_extract_attr_maps_filters_non_dict_values():
    assert _extract_attr_maps([{"Attributes": {"service.name": "api"}}, {"Attributes": []}, {}], "Attributes") == [
        {"service.name": "api"},
        {},
    ]
