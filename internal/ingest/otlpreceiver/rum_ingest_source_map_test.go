package otlpreceiver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMaybeDemangleJSStackRemapsFramesWhenSourceMapsEnabled(t *testing.T) {
	resetSourceMapCache()
	dir := t.TempDir()
	t.Setenv("SOBS_SOURCE_MAP_ENABLE", "1")
	t.Setenv("SOBS_SOURCE_MAP_DIR", dir)
	mapJSON := `{"version":3,"file":"app.min.js","sourceRoot":"/src","sources":["app.ts"],"names":[],"mappings":"AAAA"}`
	if err := os.WriteFile(filepath.Join(dir, "app.min.js.map"), []byte(mapJSON), 0o644); err != nil {
		t.Fatalf("write source map: %v", err)
	}
	stack := "TypeError: boom\n    at render (/static/app.min.js:1:1)"
	got := maybeDemangleJSStack(stack)
	if !strings.Contains(got, "[mapped] /src/app.ts:1:1") {
		t.Fatalf("expected mapped stack, got %q", got)
	}
}

func TestRemapRUMConsoleStacksUsesSourceMaps(t *testing.T) {
	resetSourceMapCache()
	dir := t.TempDir()
	t.Setenv("SOBS_SOURCE_MAP_ENABLE", "true")
	t.Setenv("SOBS_SOURCE_MAP_DIR", dir)
	mapJSON := `{"version":3,"file":"console.min.js","sourceRoot":"/src","sources":["console.ts"],"names":[],"mappings":"AAAA"}`
	if err := os.WriteFile(filepath.Join(dir, "console.min.js.map"), []byte(mapJSON), 0o644); err != nil {
		t.Fatalf("write source map: %v", err)
	}
	event := map[string]any{
		"breadcrumbs": map[string]any{
			"console": []any{
				map[string]any{"stack": "at log (/static/console.min.js:1:1)"},
			},
		},
	}
	remapRUMConsoleStacks(event)
	breadcrumbs := event["breadcrumbs"].(map[string]any)
	consoleEntries := breadcrumbs["console"].([]any)
	entry := consoleEntries[0].(map[string]any)
	if !strings.Contains(stringAny(entry["stack"]), "[mapped] /src/console.ts:1:1") {
		t.Fatalf("expected remapped console stack, got %q", stringAny(entry["stack"]))
	}
}