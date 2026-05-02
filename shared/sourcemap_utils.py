from __future__ import annotations

import os
import re
import urllib.parse
from typing import Any, Callable

_STACK_FRAME_RE = re.compile(
    r"(?P<prefix>.*?)"
    r"(?P<url>https?://[^\s\)]+|/[^\s\):]+\.js(?:\?[^\s\)]*)?)"
    r"(?::(?P<line>\d+))"
    r"(?::(?P<col>\d+))"
    r"(?P<suffix>.*)$"
)
_SOURCE_MAP_CACHE: dict[str, tuple[float, Any]] = {}


def _sourcemap_lookup_for_file(
    js_url: str,
    line: int,
    col: int,
    *,
    source_map_enable: bool,
    source_map_dir: str | None,
    source_map_cache: dict[str, tuple[float, Any]] | None = None,
) -> tuple[str, int, int, str] | None:
    if not source_map_enable or not source_map_dir:
        return None
    if not os.path.isdir(source_map_dir):
        return None

    parsed = urllib.parse.urlparse(str(js_url or ""))
    rel_path = parsed.path.lstrip("/")
    basename = os.path.basename(parsed.path)
    candidates: list[str] = []
    if rel_path:
        candidates.append(os.path.join(source_map_dir, rel_path + ".map"))
    if basename:
        candidates.append(os.path.join(source_map_dir, basename + ".map"))
        if basename.endswith(".min.js"):
            candidates.append(os.path.join(source_map_dir, basename.replace(".min.js", ".js.map")))
        if basename.endswith(".js"):
            candidates.append(os.path.join(source_map_dir, basename[:-3] + ".js.map"))

    map_path = ""
    for candidate in candidates:
        if os.path.exists(candidate):
            map_path = candidate
            break
    if not map_path:
        return None

    try:
        mtime = os.path.getmtime(map_path)
    except OSError:
        return None

    cache = _SOURCE_MAP_CACHE if source_map_cache is None else source_map_cache
    cache_entry = cache.get(map_path)
    index = None
    if cache_entry and cache_entry[0] == mtime:
        index = cache_entry[1]
    else:
        try:
            import sourcemap  # type: ignore

            with open(map_path, encoding="utf-8") as handle:
                index = sourcemap.loads(handle.read())
            cache[map_path] = (mtime, index)
        except Exception:
            return None

    try:
        token = index.lookup(max(0, line - 1), max(0, col - 1))
    except Exception:
        return None
    if not token:
        return None

    src = str(getattr(token, "src", "") or "")
    src_line = int(getattr(token, "src_line", 0) or 0)
    src_col = int(getattr(token, "src_col", 0) or 0)
    name = str(getattr(token, "name", "") or "")
    return (src, src_line + 1, src_col + 1, name)


def _maybe_demangle_js_stack(
    stack_text: str,
    *,
    source_map_enable: bool,
    sourcemap_lookup_for_file: Callable[[str, int, int], tuple[str, int, int, str] | None],
) -> str:
    text = str(stack_text or "")
    if not text or not source_map_enable:
        return text

    mapped_lines: list[str] = []
    for raw_line in text.splitlines():
        match = _STACK_FRAME_RE.match(raw_line)
        if not match:
            mapped_lines.append(raw_line)
            continue

        url = str(match.group("url") or "")
        try:
            line = int(match.group("line") or "0")
            col = int(match.group("col") or "0")
        except ValueError:
            mapped_lines.append(raw_line)
            continue

        mapped = sourcemap_lookup_for_file(url, line, col)
        if not mapped:
            mapped_lines.append(raw_line)
            continue

        src, src_line, src_col, name = mapped
        mapped_target = f"{src}:{src_line}:{src_col}" if src else f"{url}:{line}:{col}"
        if name:
            mapped_target = f"{name} ({mapped_target})"
        mapped_lines.append(f"{match.group('prefix')}[mapped] {mapped_target}{match.group('suffix')}")

    return "\n".join(mapped_lines)


def _remap_rum_console_stacks(
    event: dict[str, Any],
    *,
    maybe_demangle_js_stack: Callable[[str], str],
) -> None:
    breadcrumbs = event.get("breadcrumbs")
    if not isinstance(breadcrumbs, dict):
        return
    console_entries = breadcrumbs.get("console")
    if not isinstance(console_entries, list):
        return
    for entry in console_entries:
        if not isinstance(entry, dict):
            continue
        stack = str(entry.get("stack", ""))
        if stack:
            entry["stack"] = maybe_demangle_js_stack(stack)
