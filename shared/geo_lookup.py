from __future__ import annotations

import ipaddress
from collections import OrderedDict
from contextlib import nullcontext
from typing import Any


def _is_private_ip(ip: str) -> bool:
    try:
        addr = ipaddress.ip_address(ip)
        return addr.is_private or addr.is_loopback or addr.is_link_local or addr.is_unspecified
    except ValueError:
        return True


def _build_geo_dict(
    country: str = "",
    country_code: str = "",
    city: str = "",
    lat: float = 0.0,
    lon: float = 0.0,
) -> dict[str, Any]:
    return {"country": country, "country_code": country_code, "city": city, "lat": lat, "lon": lon}


def _geo_lookup_batch(
    ips: list[str],
    *,
    geo_enabled: bool,
    geo_db: Any,
    geo_cache: OrderedDict[str, dict[str, Any]],
    geo_cache_max: int,
    geo_cache_lock: Any | None = None,
) -> dict[str, dict[str, Any]]:
    if not geo_enabled or not ips:
        return {}

    results: dict[str, dict[str, Any]] = {}
    with geo_cache_lock if geo_cache_lock is not None else nullcontext():
        uncached: list[str] = []
        for ip in ips:
            if _is_private_ip(ip):
                results[ip] = _build_geo_dict(country="Private/Local")
            elif ip in geo_cache:
                geo_cache.move_to_end(ip)
                results[ip] = geo_cache[ip]
            else:
                uncached.append(ip)

    if not uncached or geo_db is None:
        return results

    fresh: dict[str, dict[str, Any]] = {}
    for ip in uncached:
        try:
            response = geo_db.lookup(ip)
            if response and not response.is_private:
                fresh[ip] = _build_geo_dict(
                    country=response.country_name or "",
                    country_code=response.country_code or "",
                )
            else:
                fresh[ip] = _build_geo_dict(country="Private/Local")
        except Exception:
            pass

    with geo_cache_lock if geo_cache_lock is not None else nullcontext():
        while len(geo_cache) >= geo_cache_max and fresh:
            geo_cache.popitem(last=False)
        geo_cache.update(fresh)

    results.update(fresh)
    return results
