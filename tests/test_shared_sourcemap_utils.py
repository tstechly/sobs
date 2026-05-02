from __future__ import annotations

import os
import sys
import types
from pathlib import Path
from typing import Any

import shared.sourcemap_utils as sourcemap_utils
from shared.sourcemap_utils import _maybe_demangle_js_stack, _remap_rum_console_stacks, _sourcemap_lookup_for_file


class _FakeToken:
    def __init__(self, src: str, src_line: int, src_col: int, name: str):
        self.src = src
        self.src_line = src_line
        self.src_col = src_col
        self.name = name


class _FakeIndex:
    def __init__(self, token: _FakeToken | None = None, *, raises: Exception | None = None):
        self._token = token
        self._raises = raises
        self.calls: list[tuple[int, int]] = []

    def lookup(self, line: int, col: int) -> _FakeToken | None:
        self.calls.append((line, col))
        if self._raises is not None:
            raise self._raises
        return self._token


def _install_fake_sourcemap(monkeypatch, indexes: list[_FakeIndex], load_calls: list[str]) -> None:
    def _loads(payload: str) -> _FakeIndex:
        load_calls.append(payload)
        return indexes.pop(0)

    fake_module = types.SimpleNamespace(loads=_loads)
    monkeypatch.setitem(sys.modules, "sourcemap", fake_module)


def test_sourcemap_lookup_returns_none_when_disabled_or_dir_missing(tmp_path: Path) -> None:
    assert (
        _sourcemap_lookup_for_file(
            "https://cdn.example.com/assets/app.min.js",
            1,
            1,
            source_map_enable=False,
            source_map_dir=str(tmp_path),
        )
        is None
    )
    assert (
        _sourcemap_lookup_for_file(
            "https://cdn.example.com/assets/app.min.js",
            1,
            1,
            source_map_enable=True,
            source_map_dir=str(tmp_path / "missing"),
        )
        is None
    )


def test_sourcemap_lookup_reads_candidates_uses_cache_and_reloads_on_mtime_change(tmp_path: Path, monkeypatch) -> None:
    map_file = tmp_path / "assets" / "app.min.js.map"
    map_file.parent.mkdir(parents=True)
    map_file.write_text('{"version":3}', encoding="utf-8")

    first_index = _FakeIndex(_FakeToken("src/App.tsx", 9, 4, "renderApp"))
    second_index = _FakeIndex(_FakeToken("src/App.tsx", 11, 6, "renderApp"))
    load_calls: list[str] = []
    _install_fake_sourcemap(monkeypatch, [first_index, second_index], load_calls)
    cache: dict[str, tuple[float, Any]] = {}

    first = _sourcemap_lookup_for_file(
        "https://cdn.example.com/assets/app.min.js",
        1,
        123,
        source_map_enable=True,
        source_map_dir=str(tmp_path),
        source_map_cache=cache,
    )
    second = _sourcemap_lookup_for_file(
        "https://cdn.example.com/assets/app.min.js",
        1,
        123,
        source_map_enable=True,
        source_map_dir=str(tmp_path),
        source_map_cache=cache,
    )

    os.utime(map_file, None)
    third = _sourcemap_lookup_for_file(
        "https://cdn.example.com/assets/app.min.js",
        1,
        123,
        source_map_enable=True,
        source_map_dir=str(tmp_path),
        source_map_cache=cache,
    )

    assert first == ("src/App.tsx", 10, 5, "renderApp")
    assert second == ("src/App.tsx", 10, 5, "renderApp")
    assert third == ("src/App.tsx", 12, 7, "renderApp")
    assert len(load_calls) == 2
    assert first_index.calls == [(0, 122), (0, 122)]
    assert second_index.calls == [(0, 122)]


def test_sourcemap_lookup_handles_missing_map_load_failures_and_lookup_failures(tmp_path: Path, monkeypatch) -> None:
    assert (
        _sourcemap_lookup_for_file(
            "https://cdn.example.com/assets/missing.js",
            1,
            1,
            source_map_enable=True,
            source_map_dir=str(tmp_path),
        )
        is None
    )

    broken_map = tmp_path / "broken.js.map"
    broken_map.write_text("broken", encoding="utf-8")

    fake_module = types.SimpleNamespace(loads=lambda _payload: (_ for _ in ()).throw(RuntimeError("boom")))
    monkeypatch.setitem(sys.modules, "sourcemap", fake_module)
    assert (
        _sourcemap_lookup_for_file(
            "/broken.js",
            1,
            1,
            source_map_enable=True,
            source_map_dir=str(tmp_path),
        )
        is None
    )

    lookup_map = tmp_path / "lookup.js.map"
    lookup_map.write_text("ok", encoding="utf-8")
    none_index = _FakeIndex(None)
    raising_index = _FakeIndex(raises=RuntimeError("lookup failed"))
    load_calls: list[str] = []
    _install_fake_sourcemap(monkeypatch, [none_index, raising_index], load_calls)

    assert (
        _sourcemap_lookup_for_file(
            "/lookup.js",
            5,
            7,
            source_map_enable=True,
            source_map_dir=str(tmp_path),
            source_map_cache={},
        )
        is None
    )
    os.utime(lookup_map, None)
    assert (
        _sourcemap_lookup_for_file(
            "/lookup.js",
            5,
            7,
            source_map_enable=True,
            source_map_dir=str(tmp_path),
            source_map_cache={},
        )
        is None
    )
    assert len(load_calls) == 2


def test_sourcemap_lookup_handles_getmtime_failure(tmp_path: Path, monkeypatch) -> None:
    map_file = tmp_path / "mtime.js.map"
    map_file.write_text("ok", encoding="utf-8")

    monkeypatch.setattr(sourcemap_utils.os.path, "getmtime", lambda _path: (_ for _ in ()).throw(OSError("nope")))

    assert (
        _sourcemap_lookup_for_file(
            "/mtime.js",
            1,
            1,
            source_map_enable=True,
            source_map_dir=str(tmp_path),
        )
        is None
    )


def test_maybe_demangle_js_stack_preserves_plain_lines_and_maps_frames() -> None:
    stack_text = (
        "TypeError: minified failure\n"
        "  at https://cdn.example.com/assets/app.min.js:1:1234\n"
        "  at https://cdn.example.com/assets/vendor.js:2:7\n"
        "  at plain text without frame"
    )

    mapped = _maybe_demangle_js_stack(
        stack_text,
        source_map_enable=True,
        sourcemap_lookup_for_file=lambda url, line, col: (
            ("src/components/Checkout.tsx", 88, 21, "saveOrder")
            if url.endswith("app.min.js") and line == 1 and col == 1234
            else None
        ),
    )

    assert mapped == (
        "TypeError: minified failure\n"
        "  at [mapped] saveOrder (src/components/Checkout.tsx:88:21)\n"
        "  at https://cdn.example.com/assets/vendor.js:2:7\n"
        "  at plain text without frame"
    )
    assert (
        _maybe_demangle_js_stack(
            stack_text,
            source_map_enable=False,
            sourcemap_lookup_for_file=lambda *_args: None,
        )
        == stack_text
    )


def test_maybe_demangle_js_stack_handles_invalid_numeric_frame_values(monkeypatch) -> None:
    class _BadMatch:
        def group(self, name: str) -> str:
            values = {
                "prefix": "  at ",
                "url": "https://cdn.example.com/assets/app.min.js",
                "line": "1",
                "col": "abc",
                "suffix": "",
            }
            return values[name]

    class _BadRegex:
        def match(self, _raw_line: str) -> _BadMatch:
            return _BadMatch()

    monkeypatch.setattr(sourcemap_utils, "_STACK_FRAME_RE", _BadRegex())

    assert (
        sourcemap_utils._maybe_demangle_js_stack(
            "  at malformed frame",
            source_map_enable=True,
            sourcemap_lookup_for_file=lambda *_args: ("src/App.tsx", 1, 1, "render"),
        )
        == "  at malformed frame"
    )


def test_remap_rum_console_stacks_only_updates_console_entry_stacks() -> None:
    event: dict[str, Any] = {
        "breadcrumbs": {
            "console": [
                {"stack": "frame-a", "message": "keep"},
                {"message": "no stack"},
                "not-a-dict",
            ],
        }
    }

    _remap_rum_console_stacks(
        event,
        maybe_demangle_js_stack=lambda stack: f"mapped::{stack}",
    )

    assert event["breadcrumbs"]["console"][0]["stack"] == "mapped::frame-a"
    assert event["breadcrumbs"]["console"][0]["message"] == "keep"
    assert "stack" not in event["breadcrumbs"]["console"][1]
    event_with_non_list: dict[str, Any] = {"breadcrumbs": {"console": "oops"}}
    _remap_rum_console_stacks(event_with_non_list, maybe_demangle_js_stack=lambda stack: stack)
    assert event_with_non_list == {"breadcrumbs": {"console": "oops"}}
    _remap_rum_console_stacks({}, maybe_demangle_js_stack=lambda stack: stack)
