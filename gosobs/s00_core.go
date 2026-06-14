package main

// Core infrastructure helpers shared by all ported sections.
// See CONVENTIONS.md — this file owns: logger, route table, JSON responses,
// template rendering, request-body helpers, flash messages, time formatting,
// and the shared HTTP client.

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/flosch/pongo2/v6"
)

// logger mirrors logging.getLogger("sobs").
var logger = slog.Default().With("logger", "sobs")

// ---------------------------------------------------------------------------
// Route table — sections register routes from init(); main builds the mux.
// ---------------------------------------------------------------------------

type routeEntry struct {
	Method  string
	Pattern string
	Handler http.HandlerFunc
}

var (
	registeredRoutes  []routeEntry
	registeredRouteMu sync.Mutex
	// endpointPaths maps a registered pattern to itself for url_for-style
	// lookups from templates. Flask endpoint names are not preserved in Go;
	// templates resolve url_for by literal path (see renderTemplate globals).
	endpointPaths = map[string]string{}
)

func registerRoute(method, pattern string, h http.HandlerFunc) {
	registeredRouteMu.Lock()
	defer registeredRouteMu.Unlock()
	registeredRoutes = append(registeredRoutes, routeEntry{Method: method, Pattern: pattern, Handler: h})
	endpointPaths[pattern] = pattern
}

// ---------------------------------------------------------------------------
// JSON responses
// ---------------------------------------------------------------------------

// coerceUndefinedForJson lives in s01_setup.go (port of _coerce_undefined_for_json).

func jsonResponse(w http.ResponseWriter, status int, payload any) {
	body, err := json.Marshal(coerceUndefinedForJson(payload, 0, 12))
	if err != nil {
		logger.Warn("jsonResponse marshal failed", "error", err)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error": "internal serialization error"}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// maskedJsonResponse mirrors masked_jsonify: apply input-masking rules to the
// payload when enabled, then serialize.
func maskedJsonResponse(w http.ResponseWriter, status int, payload any) {
	jsonResponse(w, status, maskJsonPayload(payload))
}

// jsonError mirrors _json_error.
func jsonError(w http.ResponseWriter, message string, statusCode int) {
	jsonResponse(w, statusCode, map[string]any{"error": message})
}

// readJsonBody decodes a JSON request body into a generic map. Mirrors
// `await request.get_json(silent=True)` returning ({}, error) on bad input.
func readJsonBody(r *http.Request) (map[string]any, error) {
	if r.Body == nil {
		return map[string]any{}, errors.New("empty body")
	}
	defer func() { _ = r.Body.Close() }()
	var out map[string]any
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil {
		return map[string]any{}, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

// ---------------------------------------------------------------------------
// Template rendering (pongo2 over the existing Jinja templates)
// ---------------------------------------------------------------------------

var (
	templateSetOnce sync.Once
	templateSet     *pongo2.TemplateSet
)

func templatesDir() string {
	// The Go binary may run from gosobs/ or the repo root; resolve robustly.
	for _, candidate := range []string{"templates", "../templates"} {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			abs, _ := filepath.Abs(candidate)
			return abs
		}
	}
	if exe, err := os.Executable(); err == nil {
		c := filepath.Join(filepath.Dir(exe), "templates")
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "templates"
}

func getTemplateSet() *pongo2.TemplateSet {
	templateSetOnce.Do(func() {
		registerJinjaFilters()
		loader, err := pongo2.NewLocalFileSystemLoader(templatesDir())
		if err != nil {
			logger.Error("template loader init failed", "error", err)
			loader, _ = pongo2.NewLocalFileSystemLoader(".")
		}
		templateSet = pongo2.NewSet("sobs", &jinjaRewritingLoader{inner: loader})
		// Global helper functions invoked by rewritten templates (the loader
		// translates Jinja constructs pongo2 cannot express into these calls).
		templateSet.Globals["getitem"] = tplGetItem // d.get('k'[, default])
		templateSet.Globals["keys"] = tplKeys       // d.keys()
		templateSet.Globals["values"] = tplValues   // d.values()
		templateSet.Globals["format"] = tplFormat   // 'fmt'|format(args)
	})
	return templateSet
}

// registerJinjaFilters registers Jinja filters that pongo2 lacks or implements
// incompatibly. Existing names are replaced so this is idempotent.
func registerJinjaFilters() {
	reg := func(name string, fn pongo2.FilterFunction) {
		if pongo2.FilterExists(name) {
			_ = pongo2.ReplaceFilter(name, fn)
		} else {
			_ = pongo2.RegisterFilter(name, fn)
		}
	}
	reg("tojson", filterToJson)
	reg("truncate", filterTruncate)
	reg("string", filterToString)
	reg("capitalize", filterCapitalize)
	reg("replace", filterReplace)
	reg("mask", filterMask)
	reg("selectattr", filterSelectattr)
	reg("round", filterRound)
	reg("min", filterMin)
	reg("max", filterMax)
	reg("trim", filterTrim)
	reg("list", filterList)
	reg("int", filterInt)
	reg("default", filterDefault)
	reg("d", filterDefault)
	// pongo2 already ships `escape`; register `e` only if it lacks the alias.
	if !pongo2.FilterExists("e") {
		_ = pongo2.RegisterFilter("e", filterEscape)
	}
}

// filterToJson mirrors Jinja's tojson: JSON-encode and mark safe (no HTML
// re-escaping). encoding/json already escapes <, >, & for embedding in HTML;
// we additionally escape the single quote like Jinja does so the output is safe
// inside single-quoted HTML attributes.
func filterToJson(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	b, err := json.Marshal(in.Interface())
	if err != nil {
		return pongo2.AsSafeValue("null"), nil
	}
	s := strings.ReplaceAll(string(b), "'", `'`)
	return pongo2.AsSafeValue(s), nil
}

// filterTruncate mirrors Jinja's truncate(length=255, killwords=False,
// end='...', leeway=5). Multi-argument calls are packed by the loader into one
// "len<sep>killwords<sep>end" string (see rewriteFilterCalls); a single integer
// argument arrives as a plain integer.
func filterTruncate(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := in.String()
	length, killwords, end := 255, false, "..."
	if param != nil {
		if param.IsString() && strings.Contains(param.String(), replaceSentinel) {
			parts := strings.Split(param.String(), replaceSentinel)
			if n, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
				length = n
			}
			if len(parts) > 1 {
				killwords = parseJinjaBool(parts[1])
			}
			if len(parts) > 2 {
				end = parts[2]
			}
		} else if !param.IsNil() {
			length = param.Integer()
		}
	}
	const leeway = 5
	runes := []rune(s)
	if len(runes) <= length+leeway {
		return pongo2.AsValue(s), nil
	}
	cut := length - len([]rune(end))
	if cut < 0 {
		cut = 0
	}
	if cut > len(runes) {
		cut = len(runes)
	}
	if killwords {
		return pongo2.AsValue(string(runes[:cut]) + end), nil
	}
	truncated := string(runes[:cut])
	if idx := strings.LastIndex(truncated, " "); idx >= 0 {
		truncated = truncated[:idx]
	}
	return pongo2.AsValue(truncated + end), nil
}

// filterSelectattr mirrors Jinja's selectattr used inside expressions:
//
//	list|selectattr('attr')                -> items where item.attr is truthy
//	list|selectattr('attr','equalto', v)   -> items where item.attr == v
//
// Multi-argument calls are packed by the loader into "attr<sep>op<sep>value".
func filterSelectattr(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	items := toAnySlice(in.Interface())
	parts := strings.Split(param.String(), replaceSentinel)
	attr := strings.TrimSpace(parts[0])
	out := make([]any, 0, len(items))
	if len(parts) >= 3 {
		want := parts[2]
		for _, it := range items {
			if fmt.Sprintf("%v", getAttr(it, attr)) == want {
				out = append(out, it)
			}
		}
	} else {
		for _, it := range items {
			if isTruthyAny(getAttr(it, attr)) {
				out = append(out, it)
			}
		}
	}
	return pongo2.AsValue(out), nil
}

// filterRound mirrors Jinja's round(precision=0) using round-half-away-from-zero
// (Jinja's default "common" method).
func filterRound(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	prec := 0
	if param != nil && !param.IsNil() {
		prec = param.Integer()
	}
	mult := math.Pow(10, float64(prec))
	return pongo2.AsValue(math.Round(in.Float()*mult) / mult), nil
}

// filterMin / filterMax mirror Jinja's min/max over an iterable.
func filterMin(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return reduceExtreme(in.Interface(), false), nil
}

func filterMax(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return reduceExtreme(in.Interface(), true), nil
}

// filterTrim mirrors Jinja's trim (strip surrounding whitespace).
func filterTrim(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(strings.TrimSpace(in.String())), nil
}

// filterList mirrors Jinja's list (materialise an iterable). pongo2's length and
// for-loop work on the underlying slice, so this is effectively a pass-through.
func filterList(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return in, nil
}

// filterInt mirrors Jinja's int (pongo2 names this filter `integer`).
func filterInt(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(in.Integer()), nil
}

// filterDefault mirrors Jinja's default/d: substitute when the value is
// undefined or falsy.
func filterDefault(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	if in == nil || in.IsNil() || !in.IsTrue() {
		return param, nil
	}
	return in, nil
}

// filterEscape mirrors Jinja's e/escape for HTML.
func filterEscape(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(html.EscapeString(in.String())), nil
}

// filterToString mirrors Jinja's string filter.
func filterToString(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(in.String()), nil
}

// filterCapitalize mirrors Jinja's capitalize: first char upper, rest lower.
func filterCapitalize(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	s := in.String()
	if s == "" {
		return pongo2.AsValue(s), nil
	}
	r := []rune(strings.ToLower(s))
	r[0] = unicode.ToUpper(r[0])
	return pongo2.AsValue(string(r)), nil
}

// filterReplace mirrors Jinja's two-arg replace; the loader encodes the operands
// as "old<replaceSentinel>new" (see rewriteJinjaForPongo2).
func filterReplace(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	p := param.String()
	oldStr, newStr := p, ""
	if idx := strings.Index(p, replaceSentinel); idx >= 0 {
		oldStr = p[:idx]
		newStr = p[idx+len(replaceSentinel):]
	}
	return pongo2.AsValue(strings.ReplaceAll(in.String(), oldStr, newStr)), nil
}

// filterMask mirrors the Python `mask` Jinja filter (_mask_value_for_output).
func filterMask(in *pongo2.Value, param *pongo2.Value) (*pongo2.Value, *pongo2.Error) {
	return pongo2.AsValue(maskStringForOutput(in.Interface(), nil)), nil
}

// jinjaTagRe matches a single Jinja delimiter block ({{ ... }} or {% ... %}).
// Rewrites are scoped to these so raw HTML/JS text is never touched.
var jinjaTagRe = regexp.MustCompile(`(?s)\{\{.*?\}\}|\{%.*?%\}`)

// urlForCallRe matches a whole url_for(...) call (single-line, no nested parens
// in args — true for all SOBS templates).
var urlForCallRe = regexp.MustCompile(`url_for\(([^)]*)\)`)

// jinjaKwargRe matches a Jinja keyword argument `name=` (not ==, >=, <=, !=).
var jinjaKwargRe = regexp.MustCompile(`([A-Za-z_]\w*)\s*=\s*`)

// jinjaInTupleRe matches `<dotted.expr> [not] in ( ... )` membership tests.
var jinjaInTupleRe = regexp.MustCompile(`([\w.]+)\s+(not\s+)?in\s+\(([^()]*)\)`)

// getFlashedCallRe matches a get_flashed_messages(...) call. The Go shim ignores
// arguments, so we drop them (pongo2 can't parse the with_categories=... kwarg).
var getFlashedCallRe = regexp.MustCompile(`get_flashed_messages\([^)]*\)`)

// flashLoopRe matches Flask's flash pair-loop. pongo2 binds only one variable
// per slice element, so we rewrite `for category, message in messages` into a
// single-variable loop over the []map shim, with field access in the body.
var flashLoopRe = regexp.MustCompile(`(?s)\{%-?\s*for\s+category\s*,\s*message\s+in\s+messages\s*-?%\}(.*?)\{%-?\s*endfor\s*-?%\}`)

// rewriteFlashLoop converts the flash pair-loop block to a pongo2-compatible
// single-variable loop. Body refs to `category`/`message` become field access.
func rewriteFlashLoop(src string) string {
	return flashLoopRe.ReplaceAllStringFunc(src, func(match string) string {
		body := flashLoopRe.FindStringSubmatch(match)[1]
		for _, old := range []string{"{{ category }}", "{{category}}"} {
			body = strings.ReplaceAll(body, old, "{{ _flash.category }}")
		}
		for _, old := range []string{"{{ message }}", "{{message}}"} {
			body = strings.ReplaceAll(body, old, "{{ _flash.message }}")
		}
		return "{% for _flash in messages %}" + body + "{% endfor %}"
	})
}

// orEmptyDictRe matches the Jinja idiom `(x or {})` used before `.get`; pongo2
// cannot parse the empty-dict literal, and getitem already treats a nil/undefined
// container as "key absent", so we drop the `or {}` fallback.
var orEmptyDictRe = regexp.MustCompile(`\(\s*([A-Za-z_][\w.]*)\s+or\s+\{\}\s*\)`)

// getCallOpenRe matches the opening of a Jinja dict `<obj>.get(` method call.
var getCallOpenRe = regexp.MustCompile(`([A-Za-z_][\w.]*(?:\[[^\]]*\])?)\.get\(`)

// keysCallRe / valuesCallRe match `<obj>.keys()` / `<obj>.values()`.
var keysCallRe = regexp.MustCompile(`([A-Za-z_][\w.]*)\.keys\(\)`)
var valuesCallRe = regexp.MustCompile(`([A-Za-z_][\w.]*)\.values\(\)`)

// formatFilterRe matches `"<fmt>"|format(args)` (printf-style). pongo2 filter
// args cannot hold expressions, so we rewrite to the format() global function.
var formatFilterRe = regexp.MustCompile(`("(?:[^"\\]|\\.)*"|'(?:[^'\\]|\\.)*')\s*\|\s*format\(([^()]*)\)`)

// filterCallOpenRe matches the opening of a parenthesised filter call `|name(`.
var filterCallOpenRe = regexp.MustCompile(`\|\s*([A-Za-z_]\w*)\s*\(`)

// notInListRe matches `<expr> not in [list]`; pongo2 has no `not in`, so we
// rewrite to `not (<expr> in [list])`.
var notInListRe = regexp.MustCompile(`([\w.\]]+)\s+not\s+in\s+(\[[^\]]*\])`)

// forIfRe matches Jinja's `{% for X in Y if COND %}` loop-with-filter; pongo2's
// for-tag has no `if` clause, so rewriteForIf hoists it into an inner {% if %}.
var forIfRe = regexp.MustCompile(`\{%-?\s*(for\s+[^%]+?\s+in\s+[^%]+?)\s+if\s+([^%]+?)\s*-?%\}`)
var forOpenRe = regexp.MustCompile(`\{%-?\s*for\b`)
var endforRe = regexp.MustCompile(`\{%-?\s*endfor\s*-?%\}`)

// fromImportRe rewrites Jinja's `{% from "x" import a, b %}` macro import into
// pongo2's `{% import "x" a, b %}` form (pongo2 has no `from` keyword).
var fromImportRe = regexp.MustCompile(`\{%-?\s*from\s+('[^']+'|"[^"]+")\s+import\s+([^%]+?)\s*-?%\}`)

// withRe / endWithRe rewrite Jinja's `{% with x = y %}…{% endwith %}` block to a
// plain `{% set %}` (pongo2 lacks a with-scope tag). The minor scope difference
// is harmless for SOBS templates.
var withRe = regexp.MustCompile(`\{%-?\s*with\s+([^%]+?)\s*-?%\}`)
var endWithRe = regexp.MustCompile(`\{%-?\s*endwith\s*-?%\}`)

// isDefinedRe / isNotDefinedRe approximate Jinja's `is defined` test (pongo2 has
// no test syntax). Undefined names resolve to nil in pongo2, so truthiness is a
// close-enough stand-in for the common `{% if x is defined %}` guards.
var isNotDefinedRe = regexp.MustCompile(`([\w.]+)\s+is\s+not\s+defined`)
var isDefinedRe = regexp.MustCompile(`([\w.]+)\s+is\s+defined`)

var pyTrueRe = regexp.MustCompile(`\bTrue\b`)
var pyFalseRe = regexp.MustCompile(`\bFalse\b`)

// replaceSentinel joins multiple Jinja filter arguments into a single pongo2
// colon-parameter string (pongo2 filters accept only one argument).
const replaceSentinel = "@@SOBSREPL@@"

// rewriteJinjaForPongo2 rewrites Jinja constructs pongo2's parser rejects.
// Most rewrites are scoped to {{ }}/{% %} blocks; structural ones (flash loop,
// for-if) operate on the whole source.
//
// PORT-NOTE: pongo2 supports neither keyword-argument call syntax, tuple
// literals, dict methods, nor Jinja's parenthesized/multi-arg filter calls. We
// translate (within tags only, except where noted):
//   - {% for x in y if c %}…{% endfor %}  -> {% for x in y %}{% if c %}…{% endif %}{% endfor %}  (whole source)
//   - url_for('static', filename='x')      -> url_for('static', 'filename', 'x')
//   - X [not] in ('a','b')                 -> [not] (X == 'a' or X == 'b')
//   - X not in ['a','b']                   -> not (X in ['a','b'])
//   - d.get('k', default)                  -> getitem(d, 'k', default)
//   - (d or {}).get('k')                   -> getitem(d, 'k')
//   - d.items()/d.keys()/d.values()        -> d / keys(d) / values(d)
//   - loop.index/first/last                -> forloop.Counter/First/Last
//   - "fmt"|format(a, b)                   -> format("fmt", a, b)
//   - |replace('a','b') / |truncate(n,k,e) -> |replace:"a@@SOBSREPL@@b" (packed)
//   - |name(arg)                           -> |name:arg
//   - Python True/False                    -> true/false
func rewriteJinjaForPongo2(src string) string {
	src = rewriteFlashLoop(src)
	src = rewriteForIf(src)
	// {% from "x" import a, b %} -> {% import "x" a, b %}.
	src = fromImportRe.ReplaceAllString(src, "{% import $1 $2 %}")
	// {% with x = y %} … {% endwith %} -> {% set x = y %} … (with removed).
	src = withRe.ReplaceAllString(src, "{% set $1 %}")
	src = endWithRe.ReplaceAllString(src, "")
	return jinjaTagRe.ReplaceAllStringFunc(src, func(tag string) string {
		// Drop get_flashed_messages(...) arguments (shim takes none).
		tag = getFlashedCallRe.ReplaceAllString(tag, "get_flashed_messages()")
		// `x is [not] defined` -> truthiness check (pongo2 has no test syntax).
		tag = isNotDefinedRe.ReplaceAllString(tag, "not ($1)")
		tag = isDefinedRe.ReplaceAllString(tag, "($1)")
		// loop.* -> forloop.* (index0 before index to avoid a partial match).
		tag = strings.ReplaceAll(tag, "loop.index0", "forloop.Counter0")
		tag = strings.ReplaceAll(tag, "loop.index", "forloop.Counter")
		tag = strings.ReplaceAll(tag, "loop.revindex0", "forloop.Revcounter0")
		tag = strings.ReplaceAll(tag, "loop.revindex", "forloop.Revcounter")
		tag = strings.ReplaceAll(tag, "loop.first", "forloop.First")
		tag = strings.ReplaceAll(tag, "loop.last", "forloop.Last")
		// dict methods: .items() iterates natively; .keys()/.values() -> globals.
		tag = keysCallRe.ReplaceAllString(tag, "keys($1)")
		tag = valuesCallRe.ReplaceAllString(tag, "values($1)")
		tag = strings.ReplaceAll(tag, ".items()", "")
		// dict .get(...) -> getitem(...).
		tag = rewriteGetCalls(tag)
		// url_for keyword args -> positional pairs.
		tag = urlForCallRe.ReplaceAllStringFunc(tag, func(match string) string {
			inner := urlForCallRe.FindStringSubmatch(match)[1]
			inner = jinjaKwargRe.ReplaceAllString(inner, "'$1', ")
			return "url_for(" + inner + ")"
		})
		// "fmt"|format(args) -> format("fmt", args) (must precede filter calls).
		tag = formatFilterRe.ReplaceAllString(tag, "format($1, $2)")
		// |name(args) -> |name:arg (single) or packed colon param (multi).
		tag = rewriteFilterCalls(tag)
		// X not in [list] -> not (X in [list]).
		tag = notInListRe.ReplaceAllString(tag, "not ($1 in $2)")
		// `X [not] in (tuple)` -> chained equality OR — but never in a for-loop.
		trimmed := strings.TrimSpace(strings.TrimPrefix(tag, "{%"))
		if !strings.HasPrefix(trimmed, "for ") {
			tag = jinjaInTupleRe.ReplaceAllStringFunc(tag, func(match string) string {
				m := jinjaInTupleRe.FindStringSubmatch(match)
				lhs := m[1]
				parts := []string{}
				for _, it := range strings.Split(m[3], ",") {
					if it = strings.TrimSpace(it); it != "" {
						parts = append(parts, lhs+" == "+it)
					}
				}
				if len(parts) == 0 {
					return match
				}
				expr := "(" + strings.Join(parts, " or ") + ")"
				if strings.TrimSpace(m[2]) == "not" {
					expr = "not " + expr
				}
				return expr
			})
		}
		// Python literals -> pongo2 literals.
		tag = pyTrueRe.ReplaceAllString(tag, "true")
		tag = pyFalseRe.ReplaceAllString(tag, "false")
		return tag
	})
}

// rewriteForIf hoists Jinja's `{% for x in y if cond %}` filter clause into an
// inner `{% if cond %}` wrapping the loop body, matching the nested-aware
// endfor. Operates on the whole source (before per-tag rewrites).
func rewriteForIf(src string) string {
	for {
		loc := forIfRe.FindStringSubmatchIndex(src)
		if loc == nil {
			break
		}
		forPart := strings.TrimSpace(src[loc[2]:loc[3]])
		cond := strings.TrimSpace(src[loc[4]:loc[5]])
		endStart, _ := findMatchingEndfor(src, loc[1])
		if endStart < 0 {
			// Could not pair an endfor — keep the loop parseable, drop the if.
			src = src[:loc[0]] + "{% " + forPart + " %}" + src[loc[1]:]
			continue
		}
		opening := "{% " + forPart + " %}{% if " + cond + " %}"
		src = src[:loc[0]] + opening + src[loc[1]:endStart] + "{% endif %}" + src[endStart:]
	}
	return src
}

// findMatchingEndfor returns the byte range of the {% endfor %} that closes the
// for-loop already opened at pos (depth starts at 1), accounting for nesting.
func findMatchingEndfor(src string, pos int) (int, int) {
	depth := 1
	i := pos
	for i < len(src) {
		fo := forOpenRe.FindStringIndex(src[i:])
		ef := endforRe.FindStringIndex(src[i:])
		if ef == nil {
			return -1, -1
		}
		if fo != nil && fo[0] < ef[0] {
			depth++
			i += fo[1]
			continue
		}
		depth--
		if depth == 0 {
			return i + ef[0], i + ef[1]
		}
		i += ef[1]
	}
	return -1, -1
}

// rewriteGetCalls converts Jinja dict `.get(...)` calls into getitem(...) global
// calls. Defaults that are dict/tuple/list literals (or None) cannot be parsed
// by pongo2 and are dropped — getitem returns nil, which iterates/renders empty.
func rewriteGetCalls(tag string) string {
	tag = orEmptyDictRe.ReplaceAllString(tag, "$1")
	for {
		loc := getCallOpenRe.FindStringSubmatchIndex(tag)
		if loc == nil {
			break
		}
		obj := tag[loc[2]:loc[3]]
		open := loc[1] - 1 // the '(' of .get(
		closeIdx := matchParen(tag, open)
		if closeIdx < 0 {
			break
		}
		args := splitArgs(tag[open+1 : closeIdx])
		var repl string
		switch {
		case len(args) == 0:
			repl = obj
		case len(args) == 1:
			repl = "getitem(" + obj + ", " + strings.TrimSpace(args[0]) + ")"
		default:
			def := strings.TrimSpace(args[1])
			if def == "None" || strings.HasPrefix(def, "{") || strings.HasPrefix(def, "(") || strings.HasPrefix(def, "[") {
				repl = "getitem(" + obj + ", " + strings.TrimSpace(args[0]) + ")"
			} else {
				repl = "getitem(" + obj + ", " + strings.TrimSpace(args[0]) + ", " + def + ")"
			}
		}
		tag = tag[:loc[0]] + repl + tag[closeIdx+1:]
	}
	return tag
}

// rewriteFilterCalls converts parenthesised filter calls `|name(args)` into
// pongo2's colon syntax. Single-argument calls become `|name:arg`; multi-argument
// calls pack the (literal) arguments into one replaceSentinel-joined string the
// corresponding filter splits back apart.
func rewriteFilterCalls(tag string) string {
	for {
		loc := filterCallOpenRe.FindStringSubmatchIndex(tag)
		if loc == nil {
			break
		}
		name := tag[loc[2]:loc[3]]
		open := loc[1] - 1 // the '(' after the filter name
		closeIdx := matchParen(tag, open)
		if closeIdx < 0 {
			break
		}
		args := splitArgs(tag[open+1 : closeIdx])
		var repl string
		switch {
		case len(args) == 0:
			repl = "|" + name
		case len(args) == 1:
			if a := strings.TrimSpace(args[0]); a == "" {
				repl = "|" + name
			} else {
				repl = "|" + name + ":" + a
			}
		default:
			vals := make([]string, len(args))
			for i, a := range args {
				vals[i] = literalValue(strings.TrimSpace(a))
			}
			repl = "|" + name + ":" + pongoQuote(strings.Join(vals, replaceSentinel))
		}
		tag = tag[:loc[0]] + repl + tag[closeIdx+1:]
	}
	return tag
}

// matchParen returns the index of the ')' that closes the '(' at openIdx,
// honouring quoted strings. Scans bytes (the structural chars are all ASCII).
func matchParen(s string, openIdx int) int {
	depth := 0
	var q byte
	for i := openIdx; i < len(s); i++ {
		c := s[i]
		if q != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == q {
				q = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			q = c
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// splitArgs splits a call's argument list on top-level commas, honouring quotes
// and (), [], {} nesting.
func splitArgs(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var args []string
	depth := 0
	var q byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		if q != 0 {
			if c == '\\' {
				i++
				continue
			}
			if c == q {
				q = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			q = c
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case ',':
			if depth == 0 {
				args = append(args, s[start:i])
				start = i + 1
			}
		}
	}
	args = append(args, s[start:])
	return args
}

// literalValue strips matching surrounding quotes from a string literal token,
// returning the raw value; non-string tokens (numbers, True/False) pass through.
func literalValue(tok string) string {
	if len(tok) >= 2 {
		if (tok[0] == '\'' && tok[len(tok)-1] == '\'') || (tok[0] == '"' && tok[len(tok)-1] == '"') {
			return tok[1 : len(tok)-1]
		}
	}
	return tok
}

// pongoQuote renders s as a double-quoted pongo2 string literal.
func pongoQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}

// ---------------------------------------------------------------------------
// Template global functions (invoked by rewritten templates) + value helpers
// ---------------------------------------------------------------------------

// tplGetItem implements Jinja dict.get: getitem(container, key[, default]).
// Returns the default (or nil) when the key is absent or the container is nil.
func tplGetItem(args ...any) any {
	if len(args) < 2 {
		return nil
	}
	var def any
	if len(args) >= 3 {
		def = args[2]
	}
	if v, ok := mapLookup(args[0], fmt.Sprintf("%v", args[1])); ok {
		return v
	}
	return def
}

// tplKeys implements dict.keys() — sorted for deterministic output.
func tplKeys(arg any) []string {
	rv := reflect.ValueOf(unwrapValue(arg))
	if rv.Kind() != reflect.Map {
		return nil
	}
	out := make([]string, 0, rv.Len())
	for _, k := range rv.MapKeys() {
		out = append(out, fmt.Sprintf("%v", k.Interface()))
	}
	sort.Strings(out)
	return out
}

// tplValues implements dict.values() — ordered by sorted key.
func tplValues(arg any) []any {
	rv := reflect.ValueOf(unwrapValue(arg))
	if rv.Kind() != reflect.Map {
		return nil
	}
	keys := rv.MapKeys()
	sort.Slice(keys, func(i, j int) bool {
		return fmt.Sprintf("%v", keys[i].Interface()) < fmt.Sprintf("%v", keys[j].Interface())
	})
	out := make([]any, 0, len(keys))
	for _, k := range keys {
		out = append(out, rv.MapIndex(k).Interface())
	}
	return out
}

// tplFormat implements Jinja's printf-style `fmt|format(args)`: format(fmt, a...).
func tplFormat(args ...any) string {
	if len(args) == 0 {
		return ""
	}
	format := fmt.Sprintf("%v", args[0])
	return fmt.Sprintf(format, coerceFormatArgs(format, args[1:])...)
}

// coerceFormatArgs coerces operands to the numeric type implied by each verb so
// e.g. `%.1f` works when pongo2 hands us an int.
func coerceFormatArgs(format string, operands []any) []any {
	out := make([]any, 0, len(operands))
	idx := 0
	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			continue
		}
		i++
		if i >= len(format) || format[i] == '%' {
			continue
		}
		for i < len(format) && strings.IndexByte("+-# 0123456789.", format[i]) >= 0 {
			i++
		}
		if i >= len(format) || idx >= len(operands) {
			break
		}
		op := operands[idx]
		idx++
		switch format[i] {
		case 'f', 'e', 'g', 'F', 'E', 'G':
			out = append(out, toFloat(op))
		case 'd', 'b', 'o', 'x', 'X', 'c':
			out = append(out, int64(toFloat(op)))
		default:
			out = append(out, op)
		}
	}
	for ; idx < len(operands); idx++ {
		out = append(out, operands[idx])
	}
	return out
}

// mapLookup fetches key from any supported map type (incl. url.Values, whose
// first value is returned) using reflection as a fallback.
func mapLookup(container any, key string) (any, bool) {
	switch m := unwrapValue(container).(type) {
	case nil:
		return nil, false
	case map[string]any:
		v, ok := m[key]
		return v, ok
	case map[string]string:
		v, ok := m[key]
		return v, ok
	case url.Values:
		if vs, ok := m[key]; ok {
			if len(vs) > 0 {
				return vs[0], true
			}
			return "", true
		}
		return nil, false
	case map[string][]string:
		if vs, ok := m[key]; ok {
			if len(vs) > 0 {
				return vs[0], true
			}
			return "", true
		}
		return nil, false
	}
	rv := reflect.ValueOf(unwrapValue(container))
	if rv.Kind() == reflect.Map {
		for _, k := range rv.MapKeys() {
			if fmt.Sprintf("%v", k.Interface()) == key {
				return rv.MapIndex(k).Interface(), true
			}
		}
	}
	return nil, false
}

// getAttr fetches a named attribute from a map item (used by selectattr).
func getAttr(item any, name string) any {
	v, _ := mapLookup(item, name)
	return v
}

// unwrapValue unwraps a *pongo2.Value (e.g. elements of a pongo2 list literal)
// to its underlying Go value, leaving other values untouched.
func unwrapValue(v any) any {
	if pv, ok := v.(*pongo2.Value); ok {
		return pv.Interface()
	}
	return v
}

// toAnySlice converts any slice/array to []any (nil for non-slices), unwrapping
// any *pongo2.Value elements produced by pongo2 list literals.
func toAnySlice(v any) []any {
	v = unwrapValue(v)
	if v == nil {
		return nil
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		out := make([]any, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			out[i] = unwrapValue(rv.Index(i).Interface())
		}
		return out
	}
	return nil
}

// isTruthyAny mirrors Jinja/Python truthiness for selectattr's existence test.
func isTruthyAny(v any) bool {
	v = unwrapValue(v)
	if v == nil {
		return false
	}
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x != ""
	case int:
		return x != 0
	case int64:
		return x != 0
	case float64:
		return x != 0
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Slice, reflect.Map, reflect.Array:
		return rv.Len() > 0
	case reflect.Ptr, reflect.Interface:
		return !rv.IsNil()
	}
	return true
}

// reduceExtreme returns the min/max element of an iterable, compared numerically.
func reduceExtreme(v any, max bool) *pongo2.Value {
	items := toAnySlice(v)
	if len(items) == 0 {
		return pongo2.AsValue(nil)
	}
	best := items[0]
	bestF := toFloat(best)
	for _, it := range items[1:] {
		f := toFloat(it)
		if (max && f > bestF) || (!max && f < bestF) {
			best, bestF = it, f
		}
	}
	return pongo2.AsValue(best)
}

// toFloat best-effort converts a value to float64.
func toFloat(v any) float64 {
	v = unwrapValue(v)
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	case int32:
		return float64(n)
	case json.Number:
		f, _ := n.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(n), 64)
		return f
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return float64(rv.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return float64(rv.Uint())
	case reflect.Float32, reflect.Float64:
		return rv.Float()
	}
	return 0
}

// parseJinjaBool parses a truncate killwords flag packed as text.
func parseJinjaBool(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "true", "1", "yes", "on":
		return true
	}
	return false
}

// jinjaRewritingLoader wraps a pongo2 loader and rewrites template source via
// rewriteJinjaForPongo2 before compilation.
type jinjaRewritingLoader struct {
	inner pongo2.TemplateLoader
}

func (l *jinjaRewritingLoader) Abs(base, name string) string {
	return l.inner.Abs(base, name)
}

func (l *jinjaRewritingLoader) Get(path string) (io.Reader, error) {
	rdr, err := l.inner.Get(path)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(rdr)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(rewriteJinjaForPongo2(string(data))), nil
}

// urlForTemplate provides a url_for shim for templates.
// PORT-NOTE: Flask url_for(endpoint, **values) resolved endpoint names; the Go
// port keeps URLs literal. Supported forms: url_for("static", filename=...) →
// /static/<filename>; any other endpoint returns "/<endpoint with _ → ->" only
// when it matches a registered pattern, else "/" + endpoint.
func urlForTemplate(endpoint string, params ...any) string {
	kv := map[string]string{}
	for i := 0; i+1 < len(params); i += 2 {
		k := fmt.Sprintf("%v", params[i])
		kv[k] = fmt.Sprintf("%v", params[i+1])
	}
	if endpoint == "static" {
		return "/static/" + strings.TrimPrefix(kv["filename"], "/")
	}
	path := "/" + strings.ReplaceAll(endpoint, "_", "-")
	if _, ok := endpointPaths[path]; ok {
		return path
	}
	if len(kv) > 0 {
		q := url.Values{}
		for k, v := range kv {
			q.Set(k, v)
		}
		return path + "?" + q.Encode()
	}
	return path
}

// renderTemplate mirrors quart.render_template + writes the response.
// Globals injected on every render mirror Quart context processors
// (injectFeatureFlags from s03) plus url_for and get_flashed_messages.
func renderTemplate(w http.ResponseWriter, r *http.Request, name string, ctx map[string]any) {
	tpl, err := getTemplateSet().FromCache(name)
	if err != nil {
		logger.Error("template load failed", "template", name, "error", err)
		http.Error(w, "template error: "+name, http.StatusInternalServerError)
		return
	}
	pctx := pongo2.Context{}
	for k, v := range injectFeatureFlags() {
		pctx[k] = v
	}
	pctx["url_for"] = urlForTemplate
	pctx["get_flashed_messages"] = func() []map[string]string { return popFlashedMessages(w, r) }
	cookies := map[string]string{}
	for _, c := range r.Cookies() {
		cookies[c.Name] = c.Value
	}
	pctx["request"] = map[string]any{
		"path":    r.URL.Path,
		"args":    r.URL.Query(),
		"cookies": cookies,
	}
	// PORT-NOTE: Flask's `config` object. The Go port exposes it as an empty map
	// so `config['KEY']|default(...)` falls through to template defaults.
	pctx["config"] = map[string]any{}
	for k, v := range ctx {
		pctx[k] = v
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tpl.ExecuteWriter(pctx, w); err != nil {
		logger.Error("template render failed", "template", name, "error", err)
	}
}

// ---------------------------------------------------------------------------
// Flash messages (cookie-based stand-in for Quart's session flash)
// ---------------------------------------------------------------------------

const flashCookieName = "sobs_flash"

type flashEntry struct {
	Message  string `json:"message"`
	Category string `json:"category"`
}

// PORT-NOTE: Quart stores flashes in the signed session cookie. The Go port
// uses an unsigned JSON cookie; flashes are display-only hints, not trusted data.
func flashMessage(w http.ResponseWriter, r *http.Request, message, category string) {
	var entries []flashEntry
	if c, err := r.Cookie(flashCookieName); err == nil {
		if raw, err := base64.URLEncoding.DecodeString(c.Value); err == nil {
			_ = json.Unmarshal(raw, &entries)
		}
	}
	entries = append(entries, flashEntry{Message: message, Category: category})
	raw, _ := json.Marshal(entries)
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    base64.URLEncoding.EncodeToString(raw),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}

func popFlashedMessages(w http.ResponseWriter, r *http.Request) []map[string]string {
	c, err := r.Cookie(flashCookieName)
	if err != nil {
		return nil
	}
	var entries []flashEntry
	if raw, err := base64.URLEncoding.DecodeString(c.Value); err == nil {
		_ = json.Unmarshal(raw, &entries)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     flashCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
	out := make([]map[string]string, 0, len(entries))
	for _, e := range entries {
		out = append(out, map[string]string{"message": e.Message, "category": e.Category})
	}
	return out
}

// ---------------------------------------------------------------------------
// Time formatting
// ---------------------------------------------------------------------------

// pyIsoFormat mirrors datetime.isoformat() for an aware UTC datetime:
// microsecond precision when non-zero, "+00:00" offset.
func pyIsoFormat(t time.Time) string {
	t = t.UTC()
	if t.Nanosecond() == 0 {
		return t.Format("2006-01-02T15:04:05") + "+00:00"
	}
	return t.Format("2006-01-02T15:04:05.000000") + "+00:00"
}

// ---------------------------------------------------------------------------
// Shared HTTP client (httpx equivalent)
// ---------------------------------------------------------------------------

var httpClient = &http.Client{Timeout: 30 * time.Second}
