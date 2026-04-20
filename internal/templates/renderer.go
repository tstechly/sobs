package templates

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/flosch/pongo2/v6"
	minijinja "github.com/mitsuhiko/minijinja/minijinja-go/v2"
	"github.com/mitsuhiko/minijinja/minijinja-go/v2/value"
)

type Renderer struct {
	env *minijinja.Environment
}

type flaskMapObject struct {
	values map[string]any
}

var (
	templateBlockPattern      = regexp.MustCompile(`\{\{[\s\S]*?\}\}|\{%[\s\S]*?%\}`)
	urlForCallPattern         = regexp.MustCompile(`url_for\(([\s\S]*?)\)`)
	inlineIfKwargPattern      = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_]*)\s*=\s*([^,\n]+?)\s+if\s+([^,\n]+?)\s+else\s+([^,\n\)]+)`)
	stringFormatPattern       = regexp.MustCompile(`("[^"]*"|'[^']*')\.format\(([^\)]*)\)`)
	stringStartsWithPattern   = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.startswith\(`)
	stringEndsWithPattern     = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.endswith\(`)
	stringReplaceTitlePattern = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.replace\(([^\)]*)\)\.title\(\)`)
	stringReplacePattern      = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.replace\(`)
	stringTitlePattern        = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.title\(\)`)
	stringUpperPattern        = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.upper\(\)`)
	stringLowerPattern        = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.lower\(\)`)
	mapGetCallPattern         = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.get\(`)
	mapItemsCallPattern       = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.items\(\)`)
	mapKeysCallPattern        = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.keys\(\)`)
	mapValuesCallPattern      = regexp.MustCompile(`([a-zA-Z_][a-zA-Z0-9_\.]*)\.values\(\)`)
	pathParamPattern          = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)
)

func toMiniValue(v any) value.Value {
	switch typed := v.(type) {
	case map[string]any:
		return value.FromObject(&flaskMapObject{values: typed})
	case map[string]string:
		converted := make(map[string]any, len(typed))
		for k, vv := range typed {
			converted[k] = vv
		}
		return value.FromObject(&flaskMapObject{values: converted})
	default:
		return value.FromAny(v)
	}
}

func toMiniMapValue(values map[string]any) value.Value {
	mapped := make(map[string]value.Value, len(values))
	for k, raw := range values {
		mapped[k] = toMiniValue(raw)
	}
	return value.FromMap(mapped)
}

func (m *flaskMapObject) GetAttr(name string) value.Value {
	if v, ok := m.values[name]; ok {
		return toMiniValue(v)
	}
	return value.Undefined()
}

func (m *flaskMapObject) ObjectRepr() value.ObjectRepr {
	return value.ObjectReprMap
}

func (m *flaskMapObject) Keys() []string {
	keys := make([]string, 0, len(m.values))
	for k := range m.values {
		keys = append(keys, k)
	}
	return keys
}

func (m *flaskMapObject) CallMethod(_ value.State, name string, args []value.Value, _ map[string]value.Value) (value.Value, error) {
	switch name {
	case "get":
		if len(args) == 0 {
			return value.Undefined(), nil
		}
		key, _ := args[0].AsString()
		if v, ok := m.values[key]; ok {
			return toMiniValue(v), nil
		}
		if len(args) > 1 {
			return args[1], nil
		}
		return value.Undefined(), nil
	case "items":
		items := make([]value.Value, 0, len(m.values))
		for k, v := range m.values {
			pair := []value.Value{value.FromString(k), toMiniValue(v)}
			items = append(items, value.FromSlice(pair))
		}
		return value.FromSlice(items), nil
	case "keys":
		keys := make([]value.Value, 0, len(m.values))
		for k := range m.values {
			keys = append(keys, value.FromString(k))
		}
		return value.FromSlice(keys), nil
	case "values":
		vals := make([]value.Value, 0, len(m.values))
		for _, v := range m.values {
			vals = append(vals, toMiniValue(v))
		}
		return value.FromSlice(vals), nil
	default:
		return value.Undefined(), value.ErrUnknownMethod
	}
}

func normalizeContextValue(v any) any {
	switch typed := v.(type) {
	case value.Value:
		return typed
	case map[string]any:
		norm := make(map[string]any, len(typed))
		for k, vv := range typed {
			norm[k] = normalizeContextValue(vv)
		}
		return value.FromObject(&flaskMapObject{values: norm})
	case map[string]string:
		norm := make(map[string]any, len(typed))
		for k, vv := range typed {
			norm[k] = vv
		}
		return value.FromObject(&flaskMapObject{values: norm})
	case []any:
		norm := make([]any, 0, len(typed))
		for _, vv := range typed {
			norm = append(norm, normalizeContextValue(vv))
		}
		return norm
	default:
		return v
	}
}

type requestObject struct {
	endpoint string
	path     string
	args     *flaskMapObject
	cookies  *flaskMapObject
}

func (r *requestObject) GetAttr(name string) value.Value {
	switch name {
	case "endpoint":
		return value.FromString(r.endpoint)
	case "path":
		return value.FromString(r.path)
	case "args":
		if r.args == nil {
			return value.FromMap(map[string]value.Value{})
		}
		return toMiniMapValue(r.args.values)
	case "cookies":
		if r.cookies == nil {
			return value.FromMap(map[string]value.Value{})
		}
		return toMiniMapValue(r.cookies.values)
	default:
		return value.Undefined()
	}
}

func NewRenderer(templateRoot string) (*Renderer, error) {
	resolvedTemplateRoot := templateRoot
	if !filepath.IsAbs(templateRoot) {
		cwd, err := os.Getwd()
		if err == nil {
			probe := cwd
			for i := 0; i < 8; i++ {
				candidate := filepath.Join(probe, templateRoot)
				if info, statErr := os.Stat(candidate); statErr == nil && info.IsDir() {
					if baseInfo, baseErr := os.Stat(filepath.Join(candidate, "base.html")); baseErr != nil || baseInfo.IsDir() {
						next := filepath.Dir(probe)
						if next == probe {
							break
						}
						probe = next
						continue
					}
					resolvedTemplateRoot = candidate
					break
				}
				next := filepath.Dir(probe)
				if next == probe {
					break
				}
				probe = next
			}
		}
	}

	env := minijinja.NewEnvironment()
	env.SetUndefinedBehavior(minijinja.UndefinedChainable)
	env.SetLoader(func(name string) (string, error) {
		clean := filepath.Clean(name)
		path := filepath.Join(resolvedTemplateRoot, clean)
		b, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		src := string(b)
		src = applyTemplateCompatibilityShims(clean, src)
		return src, nil
	})

	// Flask-style helper used across existing Jinja templates.
	env.AddFunction("url_for", func(_ *minijinja.State, args []value.Value, kwargs map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromSafeString(""), nil
		}
		endpoint, _ := args[0].AsString()
		pathTemplate := routeForEndpoint(endpoint)

		if endpoint == "static" {
			if file, ok := kwargs["filename"]; ok {
				return value.FromSafeString("/static/" + strings.TrimPrefix(file.String(), "/")), nil
			}
			return value.FromSafeString("/static/"), nil
		}

		path, remaining := applyRoutePathParams(pathTemplate, kwargs)

		if len(remaining) == 0 {
			return value.FromSafeString(path), nil
		}

		q := url.Values{}
		for k, v := range remaining {
			if s, ok := v.AsString(); ok {
				q.Set(k, s)
			} else {
				q.Set(k, v.String())
			}
		}
		encoded := q.Encode()
		if encoded == "" {
			return value.FromSafeString(path), nil
		}
		return value.FromSafeString(path + "?" + encoded), nil
	})

	env.AddFunction("starts_with", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) < 2 {
			return value.FromBool(false), nil
		}
		s, _ := args[0].AsString()
		prefix, _ := args[1].AsString()
		return value.FromBool(strings.HasPrefix(s, prefix)), nil
	})

	env.AddFunction("ends_with", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) < 2 {
			return value.FromBool(false), nil
		}
		s, _ := args[0].AsString()
		suffix, _ := args[1].AsString()
		return value.FromBool(strings.HasSuffix(s, suffix)), nil
	})

	env.AddFunction("replace_str", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) < 3 {
			return value.FromString(""), nil
		}
		s, _ := args[0].AsString()
		oldVal, _ := args[1].AsString()
		newVal, _ := args[2].AsString()
		return value.FromString(strings.ReplaceAll(s, oldVal, newVal)), nil
	})

	env.AddFunction("title_case", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		s, _ := args[0].AsString()
		words := strings.Fields(strings.ToLower(s))
		for i, w := range words {
			if w == "" {
				continue
			}
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
		return value.FromString(strings.Join(words, " ")), nil
	})

	env.AddFunction("upper_str", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		s, _ := args[0].AsString()
		return value.FromString(strings.ToUpper(s)), nil
	})

	env.AddFunction("lower_str", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		s, _ := args[0].AsString()
		return value.FromString(strings.ToLower(s)), nil
	})

	// Preserve current behavior of the Python mask filter in templates.
	env.AddFilter("mask", func(_ minijinja.FilterState, in value.Value, _ []value.Value, _ map[string]value.Value) (value.Value, error) {
		return value.FromString(in.String()), nil
	})

	// Jinja-compatible truncate helper used in settings and list templates.
	env.AddFilter("truncate", func(_ minijinja.FilterState, in value.Value, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		limit := 80
		if len(args) > 0 {
			if f, ok := args[0].AsFloat(); ok {
				n := int(f)
				if n > 0 {
					limit = n
				}
			}
		}
		runes := []rune(in.String())
		if len(runes) <= limit {
			return value.FromString(string(runes)), nil
		}
		if limit <= 3 {
			return value.FromString(string(runes[:limit])), nil
		}
		return value.FromString(string(runes[:limit-3]) + "..."), nil
	})

	// Flask flash API shim used by base.html.
	env.AddFunction("get_flashed_messages", func(_ *minijinja.State, _ []value.Value, _ map[string]value.Value) (value.Value, error) {
		return value.FromSlice([]value.Value{}), nil
	})

	// Human-readable byte formatter used by settings_data_management.html.
	env.AddFunction("fmt_bytes", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString("0 B"), nil
		}
		n, _ := args[0].AsFloat()
		units := []string{"B", "KB", "MB", "GB", "TB"}
		idx := 0
		for n >= 1024 && idx < len(units)-1 {
			n /= 1024
			idx++
		}
		if idx == 0 {
			return value.FromString(fmt.Sprintf("%.0f %s", n, units[idx])), nil
		}
		return value.FromString(fmt.Sprintf("%.1f %s", n, units[idx])), nil
	})

	// Metrics label helpers used by metrics templates.
	env.AddFunction("source_label", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromString(""), nil
		}
		s, _ := args[0].AsString()
		return value.FromString(strings.TrimSpace(s)), nil
	})

	env.AddFunction("signal_label", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) < 2 {
			return value.FromString(""), nil
		}
		s, _ := args[1].AsString()
		return value.FromString(strings.TrimSpace(s)), nil
	})

	env.AddFunction("signal_description", func(_ *minijinja.State, _ []value.Value, _ map[string]value.Value) (value.Value, error) {
		return value.FromString(""), nil
	})

	// Python-style mapping helpers used by legacy templates and macros.
	env.AddFunction("map_get", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) < 2 {
			return value.Undefined(), nil
		}
		m, ok := args[0].AsMap()
		if !ok {
			return value.Undefined(), nil
		}
		key := args[1].String()
		if v, exists := m[key]; exists {
			return v, nil
		}
		if len(args) >= 3 {
			return args[2], nil
		}
		return value.Undefined(), nil
	})

	env.AddFunction("map_items", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromSlice([]value.Value{}), nil
		}
		m, ok := args[0].AsMap()
		if !ok {
			return value.FromSlice([]value.Value{}), nil
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]value.Value, 0, len(keys))
		for _, k := range keys {
			pairs = append(pairs, value.FromSlice([]value.Value{value.FromString(k), m[k]}))
		}
		return value.FromSlice(pairs), nil
	})

	env.AddFunction("map_keys", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromSlice([]value.Value{}), nil
		}
		m, ok := args[0].AsMap()
		if !ok {
			return value.FromSlice([]value.Value{}), nil
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		vals := make([]value.Value, 0, len(keys))
		for _, k := range keys {
			vals = append(vals, value.FromString(k))
		}
		return value.FromSlice(vals), nil
	})

	env.AddFunction("map_values", func(_ *minijinja.State, args []value.Value, _ map[string]value.Value) (value.Value, error) {
		if len(args) == 0 {
			return value.FromSlice([]value.Value{}), nil
		}
		m, ok := args[0].AsMap()
		if !ok {
			return value.FromSlice([]value.Value{}), nil
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		vals := make([]value.Value, 0, len(keys))
		for _, k := range keys {
			vals = append(vals, m[k])
		}
		return value.FromSlice(vals), nil
	})

	return &Renderer{env: env}, nil
}

func applyTemplateCompatibilityShims(name string, src string) string {
	src = templateBlockPattern.ReplaceAllStringFunc(src, func(block string) string {
		mapped := urlForCallPattern.ReplaceAllStringFunc(block, func(call string) string {
			inner := strings.TrimSuffix(strings.TrimPrefix(call, "url_for("), ")")
			inner = inlineIfKwargPattern.ReplaceAllString(inner, `$1=($3 and $2 or $4)`)
			return "url_for(" + inner + ")"
		})

		mapped = mapGetCallPattern.ReplaceAllString(mapped, `map_get($1, `)
		mapped = mapItemsCallPattern.ReplaceAllString(mapped, `map_items($1)`)
		mapped = mapKeysCallPattern.ReplaceAllString(mapped, `map_keys($1)`)
		mapped = mapValuesCallPattern.ReplaceAllString(mapped, `map_values($1)`)

		// Map Python string .format(...) method calls to Jinja filter syntax.
		mapped = stringFormatPattern.ReplaceAllString(mapped, `$1 | format($2)`)
		mapped = stringReplaceTitlePattern.ReplaceAllString(mapped, `title_case(replace_str($1, $2))`)
		mapped = stringReplacePattern.ReplaceAllString(mapped, `replace_str($1, `)
		mapped = stringTitlePattern.ReplaceAllString(mapped, `title_case($1)`)
		mapped = stringUpperPattern.ReplaceAllString(mapped, `upper_str($1)`)
		mapped = stringLowerPattern.ReplaceAllString(mapped, `lower_str($1)`)
		mapped = stringStartsWithPattern.ReplaceAllString(mapped, `starts_with($1, `)
		mapped = stringEndsWithPattern.ReplaceAllString(mapped, `ends_with($1, `)
		return mapped
	})

	return src
}

func (r *Renderer) Render(name string, context pongo2.Context) (string, error) {
	tpl, err := r.env.GetTemplate(filepath.Clean(name))
	if err != nil {
		return "", err
	}
	ctx := map[string]any(context)
	if _, ok := ctx["config"]; !ok {
		ctx["config"] = value.FromObject(&flaskMapObject{values: map[string]any{"ENABLE_FIRST_RUN_TOUR": false}})
	}
	if _, ok := ctx["request"]; !ok {
		ctx["request"] = value.FromObject(&requestObject{
			endpoint: "",
			path:     "",
			args:     &flaskMapObject{values: map[string]any{}},
			cookies:  &flaskMapObject{values: map[string]any{}},
		})
	} else if reqMap, ok := ctx["request"].(map[string]any); ok {
		endpoint := ""
		if v, exists := reqMap["endpoint"]; exists {
			if s, ok := v.(string); ok {
				endpoint = s
			}
		}
		path := ""
		if v, exists := reqMap["path"]; exists {
			if s, ok := v.(string); ok {
				path = s
			}
		}
		args := map[string]any{}
		if v, exists := reqMap["args"]; exists {
			switch typed := v.(type) {
			case map[string]string:
				for k, raw := range typed {
					args[k] = raw
				}
			case map[string]any:
				args = typed
			}
		}
		cookies := map[string]any{}
		if v, exists := reqMap["cookies"]; exists {
			switch typed := v.(type) {
			case map[string]string:
				for k, raw := range typed {
					cookies[k] = raw
				}
			case map[string]any:
				cookies = typed
			}
		}
		ctx["request"] = value.FromObject(&requestObject{
			endpoint: endpoint,
			path:     path,
			args:     &flaskMapObject{values: args},
			cookies:  &flaskMapObject{values: cookies},
		})
	}
	if _, ok := ctx["mobile_breakpoint_max"]; !ok {
		ctx["mobile_breakpoint_max"] = "575.98px"
	}
	if _, ok := ctx["sobs_version"]; !ok {
		ctx["sobs_version"] = "go-migration"
	}
	if _, ok := ctx["services"]; !ok {
		ctx["services"] = []any{}
	}
	if _, ok := ctx["sources"]; !ok {
		ctx["sources"] = []any{}
	}
	if _, ok := ctx["signals"]; !ok {
		ctx["signals"] = []any{}
	}
	if _, ok := ctx["source"]; !ok {
		ctx["source"] = ""
	}
	if _, ok := ctx["service"]; !ok {
		ctx["service"] = ""
	}
	if _, ok := ctx["signal"]; !ok {
		ctx["signal"] = ""
	}
	if _, ok := ctx["total_calls"]; !ok {
		ctx["total_calls"] = 0
	}
	if _, ok := ctx["total_tokens_in"]; !ok {
		ctx["total_tokens_in"] = 0
	}
	if _, ok := ctx["total_tokens_out"]; !ok {
		ctx["total_tokens_out"] = 0
	}
	if _, ok := ctx["total_errors"]; !ok {
		ctx["total_errors"] = 0
	}
	if _, ok := ctx["k8s_settings"]; !ok {
		ctx["k8s_settings"] = value.FromObject(&flaskMapObject{values: map[string]any{}})
	}
	if _, ok := ctx["settings"]; !ok {
		ctx["settings"] = map[string]any{}
	}
	if _, ok := ctx["rules"]; !ok {
		ctx["rules"] = []any{}
	}
	if _, ok := ctx["vitals_summary"]; !ok {
		ctx["vitals_summary"] = map[string]any{}
	}
	if _, ok := ctx["error_stats"]; !ok {
		ctx["error_stats"] = map[string]any{"total": 0}
	}
	if _, ok := ctx["db_stats"]; !ok {
		ctx["db_stats"] = map[string]any{"compressed_bytes": 0, "uncompressed_bytes": 0, "compression_ratio": 0}
	}
	if _, ok := ctx["table_stats"]; !ok {
		ctx["table_stats"] = []any{}
	}
	if _, ok := ctx["dm_settings"]; !ok {
		ctx["dm_settings"] = map[string]any{}
	}
	if _, ok := ctx["dm_secret_present"]; !ok {
		ctx["dm_secret_present"] = map[string]any{}
	}
	if _, ok := ctx["event_types"]; !ok {
		ctx["event_types"] = []any{}
	}
	if _, ok := ctx["error_sources"]; !ok {
		ctx["error_sources"] = []any{}
	}
	if _, ok := ctx["default_ai_pricing"]; !ok {
		ctx["default_ai_pricing"] = map[string]any{}
	}
	for k, v := range ctx {
		ctx[k] = normalizeContextValue(v)
	}
	return tpl.Render(ctx)
}

func routeForEndpoint(endpoint string) string {
	routes := map[string]string{
		"root":                                      "/",
		"summary":                                   "/",
		"view_summary":                              "/",
		"view_logs":                                 "/logs",
		"view_errors":                               "/errors",
		"view_traces":                               "/traces",
		"view_rum":                                  "/rum",
		"view_ai":                                   "/ai",
		"view_query":                                "/query",
		"api_query_ask":                             "/api/query/ask",
		"api_query_run":                             "/api/query/run",
		"api_query_refine_chart":                    "/api/query/refine-chart",
		"api_query_add_to_dashboard":                "/api/query/add-to-dashboard",
		"view_metrics":                              "/metrics",
		"api_chart_types":                           "/api/chart-types",
		"view_kubernetes":                           "/kubernetes",
		"view_table_explorer":                       "/table-explorer",
		"api_table_explorer_tables":                 "/api/table-explorer/tables",
		"api_table_explorer_table":                  "/api/table-explorer/table/",
		"view_reports":                              "/reports",
		"list_reports":                              "/reports",
		"api_list_reports":                          "/api/reports",
		"api_create_report":                         "/api/reports",
		"api_export_reports":                        "/api/reports/export",
		"api_import_reports":                        "/api/reports/import",
		"delete_report":                             "/reports/{report_id}/delete",
		"list_dashboards":                           "/dashboards",
		"add_chart":                                 "/dashboards/{dashboard_id}/charts",
		"edit_chart":                                "/dashboards/{dashboard_id}/charts/{chart_id}/edit",
		"remove_chart":                              "/dashboards/{dashboard_id}/charts/{chart_id}/delete",
		"clone_chart":                               "/dashboards/{dashboard_id}/charts/{chart_id}/clone",
		"import_chart":                              "/api/dashboards/{dashboard_id}/charts/import",
		"export_chart":                              "/api/dashboards/{dashboard_id}/charts/{chart_id}/export",
		"render_chart":                              "/api/dashboards/render",
		"api_dashboards_list":                       "/api/dashboards/list",
		"view_custom_dashboard":                     "/dashboards/{dashboard_id}",
		"create_dashboard":                          "/dashboards",
		"delete_dashboard":                          "/dashboards/{dashboard_id}/delete",
		"new_dashboard_form":                        "/dashboards/new",
		"ai_build_chart_spec":                       "/api/dashboards/spec/ai-build",
		"compile_chart_spec_api":                    "/api/dashboards/spec/compile",
		"dry_run_chart_spec_api":                    "/api/dashboards/spec/dry-run",
		"render_chart_spec_api":                     "/api/dashboards/spec/render",
		"validate_chart_spec_api":                   "/api/dashboards/spec/validate",
		"chart_spec_options_api":                    "/api/dashboards/spec/options",
		"view_incident":                             "/incident",
		"view_web_traffic":                          "/web-traffic",
		"view_work_items":                           "/work-items",
		"view_settings":                             "/settings",
		"view_notifications":                        "/settings/notifications",
		"view_ai_settings":                          "/settings/ai",
		"export_ai_training":                        "/api/ai/export",
		"view_settings_repositories":                "/settings/repositories",
		"view_settings_ai":                          "/settings/ai",
		"view_dm_settings":                          "/settings/data-management",
		"view_k8s_settings":                         "/settings/kubernetes",
		"view_masking_settings":                     "/settings/masking",
		"view_tag_rules":                            "/settings/tags",
		"view_enrichment_settings":                  "/settings/enrichment",
		"view_settings_agents":                      "/settings/agents",
		"view_agent_rules":                          "/settings/agents",
		"view_enrichment_cve":                       "/enrichment/cve",
		"summary_help":                              "/summary/help",
		"logs_help":                                 "/logs/help",
		"view_logs_help":                            "/logs/help",
		"errors_help":                               "/errors/help",
		"view_errors_help":                          "/errors/help",
		"traces_help":                               "/traces/help",
		"view_traces_help":                          "/traces/help",
		"rum_help":                                  "/rum/help",
		"view_rum_help":                             "/rum/help",
		"ai_help":                                   "/ai/help",
		"view_ai_help":                              "/ai/help",
		"query_help":                                "/query/help",
		"view_query_help":                           "/query/help",
		"metrics_help":                              "/metrics/help",
		"view_metrics_help":                         "/metrics/help",
		"metrics_rules_help":                        "/metrics/help/rules",
		"metrics_anomaly_help":                      "/metrics/help/anomaly",
		"auto_metrics_rules_help":                   "/metrics/help/rules/auto",
		"work_items_help":                           "/work-items/help",
		"web_traffic_help":                          "/web-traffic/help",
		"reports_help":                              "/reports/help",
		"incident_help":                             "/incident/help",
		"settings_help":                             "/settings/help",
		"settings_ai_help":                          "/settings/help/ai",
		"settings_agents_help":                      "/settings/help/agents",
		"settings_enrichment_help":                  "/settings/help/enrichment",
		"settings_kubernetes_help":                  "/settings/help/kubernetes",
		"settings_notifications_help":               "/settings/help/notifications",
		"settings_repositories_help":                "/settings/help/repositories",
		"settings_tags_help":                        "/settings/help/tags",
		"masking_help":                              "/settings/help/masking",
		"data_management_help":                      "/settings/help/data-management",
		"kubernetes_help":                           "/kubernetes/help",
		"chart_editor_help":                         "/dashboards/help/chart-editor",
		"table_explorer_help":                       "/table-explorer/help",
		"setup_playbooks_help":                      "/setup/help/playbooks",
		"cve_help":                                  "/cve/help",
		"mcp.mcp_settings_page":                     "/settings/mcp",
		"ai_helper":                                 "/api/ai/helper",
		"ai_helper_feedback":                        "/api/ai/helper/feedback",
		"ai_helper_chats":                           "/api/ai/helper/chats",
		"ai_helper_capabilities":                    "/api/ai/helper/capabilities",
		"ai_helper_execute_action":                  "/api/ai/helper/actions/execute",
		"api_ai_field_hints":                        "/api/ai/field-hints",
		"api_ai_validate_filter":                    "/api/ai/validate-filter",
		"api_logs_field_hints":                      "/api/logs/field-hints",
		"api_logs_validate_filter":                  "/api/logs/validate-filter",
		"api_logs_validate_regex":                   "/api/logs/validate-regex",
		"api_errors_validate_regex":                 "/api/errors/validate-regex",
		"api_traces_validate_regex":                 "/api/traces/validate-regex",
		"api_metrics_validate_regex":                "/api/metrics/validate-regex",
		"api_rum_validate_regex":                    "/api/rum/validate-regex",
		"api_web_traffic_geo":                       "/api/web-traffic/geo",
		"api_web_traffic_browsers":                  "/api/web-traffic/browsers",
		"api_web_traffic_os":                        "/api/web-traffic/os",
		"api_web_traffic_timezones":                 "/api/web-traffic/timezones",
		"api_web_traffic_languages":                 "/api/web-traffic/languages",
		"api_web_traffic_devices":                   "/api/web-traffic/devices",
		"api_kubernetes_status":                     "/api/kubernetes/status",
		"api_dm_backup_list":                        "/api/data-management/backup/list",
		"api_dm_backup_run":                         "/api/data-management/backup/run",
		"api_dm_restore":                            "/api/data-management/restore",
		"api_onboarding_list_repos":                 "/api/onboarding/list-repos",
		"api_onboarding_inspect_repo":               "/api/onboarding/inspect-repo",
		"api_onboarding_import_repo":                "/api/onboarding/import-repo",
		"api_onboarding_create_repo":                "/api/onboarding/create-repo",
		"api_onboarding_create_issues":              "/api/onboarding/create-issues",
		"api_setup_wizard_steps":                    "/api/setup-wizard/steps",
		"api_raw_span":                              "/api/traces/span/",
		"tail_stream":                               "/tail",
		"trigger_agent_run":                         "/api/agent/runs",
		"raise_issue_from_user_observation":         "/api/issues/raise",
		"create_agent_rule":                         "/settings/agents",
		"delete_agent_rule":                         "/settings/agents/{rule_id}/delete",
		"dismiss_agent_run":                         "/api/agent/runs/{run_id}/dismiss",
		"resolve_error":                             "/errors/{error_id}/resolve",
		"service_worker_js":                         "/service-worker.js",
		"test_notification_channel":                 "/api/notifications/check",
		"check_notifications":                       "/api/notifications/check",
		"generate_vapid_key":                        "/api/notifications/vapid-keygen",
		"delete_vapid_keys":                         "/api/notifications/vapid-keys",
		"get_vapid_public_key":                      "/api/notifications/vapid-public-key",
		"static":                                    "/static",
		"mcp.mcp_api_set_enabled":                   "/api/mcp/enabled",
		"mcp.mcp_api_create_key":                    "/api/mcp/keys",
		"mcp.mcp_api_list_keys":                     "/api/mcp/keys",
		"api_enrichment_libraries":                  "/api/enrichment/libraries",
		"api_enrichment_github_repo_health":         "/api/enrichment/github/repo-health",
		"api_cve_scan":                              "/api/enrichment/cve/scan",
		"api_cve_set_disposition":                   "/api/enrichment/cve/findings",
		"save_ai_settings":                          "/settings/ai",
		"save_enrichment_settings":                  "/settings/enrichment",
		"save_k8s_settings":                         "/settings/kubernetes",
		"save_dm_settings":                          "/settings/data-management",
		"save_settings_repository_realtime_mode":    "/settings/repositories/{app_id}/realtime-mode",
		"validate_settings_repository_github_token": "/settings/repositories/github-token/validate",
		"rotate_settings_repository_ci_ingest_key":  "/settings/repositories/{app_id}/ci-ingest-key/rotate",
		"revoke_settings_repository_ci_ingest_key":  "/settings/repositories/{app_id}/ci-ingest-key/revoke",
		"delete_settings_repository":                "/settings/repositories/{app_id}/delete",
		"update_settings_repository":                "/settings/repositories/{app_id}",
		"add_settings_repository_release":           "/settings/repositories/{app_id}/releases",
		"create_notification_channel":               "/settings/notifications/channels",
		"toggle_notification_channel":               "/settings/notifications/channels/{channel_id}/toggle",
		"delete_notification_channel":               "/settings/notifications/channels/{channel_id}/delete",
		"create_notification_rule":                  "/settings/notifications/rules",
		"toggle_notification_rule":                  "/settings/notifications/rules/{rule_id}/toggle",
		"delete_notification_rule":                  "/settings/notifications/rules/{rule_id}/delete",
		"auto_generate_notification_rules":          "/api/notifications/rules/auto-generate",
		"add_masking_key":                           "/settings/masking/keys",
		"delete_masking_key":                        "/settings/masking/keys/delete",
		"add_masking_pattern":                       "/settings/masking/patterns",
		"delete_masking_pattern":                    "/settings/masking/patterns/delete",
		"update_masking_output_setting":             "/settings/masking/output",
		"update_masking_sql_output_setting":         "/settings/masking/sql-output",
		"api_masking_preview":                       "/api/settings/masking/preview",
		"create_tag_rule":                           "/settings/tags",
		"delete_tag_rule":                           "/settings/tags/{rule_id}/delete",
		"auto_tag_rules":                            "/settings/tags/auto",
		"api_tag_rule_condition_suggestions":        "/api/settings/tags/condition-suggestions",
		"view_metrics_rules":                        "/metrics/rules",
		"view_metrics_anomaly":                      "/metrics/anomaly",
		"create_metrics_rule":                       "/metrics/rules",
		"delete_metrics_rule":                       "/metrics/rules/{rule_id}/delete",
		"auto_metrics_rules":                        "/metrics/rules/auto",
		"auto_metrics_rules_dashboard":              "/metrics/rules/dashboard/auto",
	}
	if path, ok := routes[endpoint]; ok {
		return path
	}
	if endpoint == "" {
		return "/"
	}
	return "/" + strings.TrimPrefix(strings.ReplaceAll(endpoint, "_", "/"), "/")
}

func applyRoutePathParams(pathTemplate string, kwargs map[string]value.Value) (string, map[string]value.Value) {
	if len(kwargs) == 0 || !strings.Contains(pathTemplate, "{") {
		return pathTemplate, kwargs
	}
	remaining := make(map[string]value.Value, len(kwargs))
	for k, v := range kwargs {
		remaining[k] = v
	}
	path := pathParamPattern.ReplaceAllStringFunc(pathTemplate, func(token string) string {
		match := pathParamPattern.FindStringSubmatch(token)
		if len(match) != 2 {
			return token
		}
		key := match[1]
		v, ok := remaining[key]
		if !ok {
			return token
		}
		delete(remaining, key)
		s, ok := v.AsString()
		if !ok {
			s = v.String()
		}
		return url.PathEscape(s)
	})
	return path, remaining
}
