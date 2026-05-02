from __future__ import annotations

from collections.abc import Sequence

from shared.log_regex_filters import (
    _parse_regex_filter_expression,
    _prepare_re2_filter_patterns,
    _split_regex_filter_expression_terms,
    _unescape_regex_filter_term,
    _validate_re2_pattern,
    _validate_re2_patterns,
)


class _FakeResult:
    def fetchone(self) -> dict | None:
        return {}


class _FakeDb:
    def __init__(self, responses: Sequence[object]):
        self._responses = list(responses)
        self.calls: list[tuple[str, list[object]]] = []

    def execute(self, query: str, params: list[object]) -> _FakeResult:
        self.calls.append((query, list(params)))
        response = self._responses.pop(0)
        if isinstance(response, Exception):
            raise response
        return _FakeResult()


def test_validate_re2_pattern_handles_blank_success_and_error_suffix() -> None:
    assert _validate_re2_pattern(_FakeDb([]), "") is None
    assert _validate_re2_pattern(_FakeDb([None]), "foo.*bar") is None

    error = _validate_re2_pattern(
        _FakeDb([RuntimeError("bad regex: while executing function match")]),
        "[unclosed",
    )

    assert error == "Regex error: bad regex"


def test_split_and_unescape_regex_filter_expression_terms() -> None:
    assert _split_regex_filter_expression_terms(r"foo\&&bar&&baz&&!qux\&&z") == [
        r"foo\&&bar",
        "baz",
        r"!qux\&&z",
    ]
    assert _unescape_regex_filter_term(r"foo\&&bar") == "foo&&bar"


def test_parse_regex_filter_expression_handles_empty_valid_and_invalid_inputs() -> None:
    assert _parse_regex_filter_expression("") == ([], [], None)
    assert _parse_regex_filter_expression(r"foo\&&bar&&baz&&!qux\&&z") == (
        ["foo&&bar", "baz"],
        ["qux&&z"],
        None,
    )
    assert _parse_regex_filter_expression("foo&&&&bar") == (
        [],
        [],
        "Regex error: invalid expression around '&&'",
    )
    assert _parse_regex_filter_expression("!") == (
        [],
        [],
        "Regex error: expected a pattern after '!'",
    )
    invalid = _parse_regex_filter_expression("[")
    assert invalid[0] == []
    assert invalid[1] == []
    assert invalid[2] is not None and invalid[2].startswith("Regex error: ")


def test_validate_re2_patterns_returns_first_error() -> None:
    error = _validate_re2_patterns(
        _FakeDb([None, RuntimeError("second failed")]),
        ["ok", "bad"],
    )

    assert error == "Regex error: second failed"


def test_prepare_re2_filter_patterns_short_circuits_parse_errors() -> None:
    db = _FakeDb([])

    assert _prepare_re2_filter_patterns(db, "foo&&&&bar") == (
        [],
        [],
        "Regex error: invalid expression around '&&'",
    )
    assert db.calls == []


def test_prepare_re2_filter_patterns_validates_re2_and_returns_patterns() -> None:
    assert _prepare_re2_filter_patterns(
        _FakeDb([RuntimeError("bad re2")]),
        "foo",
    ) == ([], [], "Regex error: bad re2")

    assert _prepare_re2_filter_patterns(
        _FakeDb([None, None]),
        r"foo&&!bar",
    ) == (["foo"], ["bar"], None)
