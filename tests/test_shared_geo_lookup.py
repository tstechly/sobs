from collections import OrderedDict
from threading import Lock

from shared.geo_lookup import _build_geo_dict, _geo_lookup_batch, _is_private_ip


class _FakeGeoRecord:
    def __init__(self, *, is_private: bool, country_name: str = "", country_code: str = "") -> None:
        self.is_private = is_private
        self.country_name = country_name
        self.country_code = country_code


class _FakeGeoDb:
    def __init__(self, records: dict[str, object]) -> None:
        self.records = records

    def lookup(self, ip: str) -> object:
        value = self.records[ip]
        if isinstance(value, Exception):
            raise value
        return value


def test_is_private_ip_handles_private_public_and_invalid_inputs() -> None:
    assert _is_private_ip("192.168.1.10") is True
    assert _is_private_ip("8.8.8.8") is False
    assert _is_private_ip("not-an-ip") is True


def test_build_geo_dict_returns_normalized_shape() -> None:
    assert _build_geo_dict(country="US", country_code="US", city="Austin", lat=1.2, lon=3.4) == {
        "country": "US",
        "country_code": "US",
        "city": "Austin",
        "lat": 1.2,
        "lon": 3.4,
    }


def test_geo_lookup_batch_returns_empty_when_disabled_or_no_ips() -> None:
    assert (
        _geo_lookup_batch(
            [],
            geo_enabled=True,
            geo_db=None,
            geo_cache=OrderedDict(),
            geo_cache_max=10,
        )
        == {}
    )
    assert (
        _geo_lookup_batch(
            ["8.8.8.8"],
            geo_enabled=False,
            geo_db=None,
            geo_cache=OrderedDict(),
            geo_cache_max=10,
        )
        == {}
    )


def test_geo_lookup_batch_uses_cache_and_marks_private_ips() -> None:
    cache = OrderedDict(
        {
            "1.1.1.1": _build_geo_dict(country="Australia", country_code="AU"),
            "8.8.8.8": _build_geo_dict(country="United States", country_code="US"),
        }
    )

    result = _geo_lookup_batch(
        ["192.168.1.1", "1.1.1.1"],
        geo_enabled=True,
        geo_db=None,
        geo_cache=cache,
        geo_cache_max=10,
    )

    assert result["192.168.1.1"]["country"] == "Private/Local"
    assert result["1.1.1.1"]["country_code"] == "AU"
    assert list(cache.keys())[-1] == "1.1.1.1"


def test_geo_lookup_batch_fetches_public_ips_and_evicts_oldest_cache_entry() -> None:
    cache = OrderedDict(
        {
            "1.1.1.1": _build_geo_dict(country="Australia", country_code="AU"),
        }
    )
    geo_db = _FakeGeoDb(
        {
            "8.8.8.8": _FakeGeoRecord(is_private=False, country_name="United States", country_code="US"),
        }
    )

    result = _geo_lookup_batch(
        ["8.8.8.8"],
        geo_enabled=True,
        geo_db=geo_db,
        geo_cache=cache,
        geo_cache_max=1,
        geo_cache_lock=Lock(),
    )

    assert result["8.8.8.8"] == _build_geo_dict(country="United States", country_code="US")
    assert list(cache.keys()) == ["8.8.8.8"]


def test_geo_lookup_batch_handles_private_results_and_lookup_errors() -> None:
    cache: OrderedDict[str, dict[str, object]] = OrderedDict()
    geo_db = _FakeGeoDb(
        {
            "9.9.9.9": _FakeGeoRecord(is_private=True),
            "8.8.4.4": RuntimeError("boom"),
        }
    )

    result = _geo_lookup_batch(
        ["9.9.9.9", "8.8.4.4"],
        geo_enabled=True,
        geo_db=geo_db,
        geo_cache=cache,
        geo_cache_max=10,
    )

    assert result["9.9.9.9"]["country"] == "Private/Local"
    assert "8.8.4.4" not in result
    assert "9.9.9.9" in cache
    assert "8.8.4.4" not in cache
