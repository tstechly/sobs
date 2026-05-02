from __future__ import annotations

import re
from typing import Any


def _validate_re2_pattern(db: Any, pattern: str) -> str | None:
    value = str(pattern or "").strip()
    if not value:
        return None
    try:
        db.execute("SELECT match('', ?)", [value]).fetchone()
    except Exception as exc:
        message = str(exc).strip()
        if ": while executing function" in message:
            message = message.split(": while executing function", 1)[0].strip()
        return f"Regex error: {message}"
    return None


def _split_regex_filter_expression_terms(expression: str) -> list[str]:
    parts: list[str] = []
    buffer: list[str] = []
    index = 0
    length = len(expression)
    while index < length:
        if index + 1 < length and expression[index] == "&" and expression[index + 1] == "&":
            backslashes = 0
            backtrack = index - 1
            while backtrack >= 0 and expression[backtrack] == "\\":
                backslashes += 1
                backtrack -= 1
            if backslashes % 2 == 0:
                parts.append("".join(buffer).strip())
                buffer = []
                index += 2
                continue
        buffer.append(expression[index])
        index += 1
    parts.append("".join(buffer).strip())
    return parts


def _unescape_regex_filter_term(term: str) -> str:
    return term.replace(r"\&&", "&&")


def _parse_regex_filter_expression(raw: str) -> tuple[list[str], list[str], str | None]:
    expression = str(raw or "").strip()
    if not expression:
        return [], [], None

    parts = _split_regex_filter_expression_terms(expression)
    if not parts or any(not part for part in parts):
        return [], [], "Regex error: invalid expression around '&&'"

    include_patterns: list[str] = []
    exclude_patterns: list[str] = []
    for part in parts:
        negate = part.startswith("!")
        token = part[1:].strip() if negate else part
        token = _unescape_regex_filter_term(token)
        if not token:
            return [], [], "Regex error: expected a pattern after '!'"
        try:
            re.compile(token, re.IGNORECASE)
        except re.error as exc:
            return [], [], f"Regex error: {exc}"
        if negate:
            exclude_patterns.append(token)
        else:
            include_patterns.append(token)

    return include_patterns, exclude_patterns, None


def _validate_re2_patterns(db: Any, patterns: list[str]) -> str | None:
    for pattern in patterns:
        re2_error = _validate_re2_pattern(db, pattern)
        if re2_error:
            return re2_error
    return None


def _prepare_re2_filter_patterns(db: Any, raw: str) -> tuple[list[str], list[str], str | None]:
    include_patterns, exclude_patterns, parse_error = _parse_regex_filter_expression(raw)
    if parse_error:
        return [], [], parse_error
    re2_error = _validate_re2_patterns(db, [*include_patterns, *exclude_patterns])
    if re2_error:
        return [], [], re2_error
    return include_patterns, exclude_patterns, None
