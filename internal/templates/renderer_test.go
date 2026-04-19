package templates

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

func TestRendererRouteMapCoversTemplateURLForEndpoints(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatalf("unable to locate test file path")
	}

	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
	rendererPath := filepath.Join(repoRoot, "internal", "templates", "renderer.go")
	templatesDir := filepath.Join(repoRoot, "templates")

	rendererBytes, err := os.ReadFile(rendererPath)
	if err != nil {
		t.Fatalf("read renderer file: %v", err)
	}

	routeKeyRx := regexp.MustCompile(`"([a-zA-Z0-9_\.]+)"\s*:\s*"/`)
	routeKeys := map[string]struct{}{}
	for _, match := range routeKeyRx.FindAllStringSubmatch(string(rendererBytes), -1) {
		routeKeys[match[1]] = struct{}{}
	}

	urlForRx := regexp.MustCompile(`url_for\(\s*['\"]([^'\"]+)['\"]`)
	templateEndpoints := map[string][]string{}

	walkErr := filepath.WalkDir(templatesDir, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(path) != ".html" {
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(repoRoot, path)
		if err != nil {
			relPath = path
		}
		for _, match := range urlForRx.FindAllStringSubmatch(string(b), -1) {
			ep := strings.TrimSpace(match[1])
			if ep == "" {
				continue
			}
			templateEndpoints[ep] = append(templateEndpoints[ep], filepath.ToSlash(relPath))
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk templates: %v", walkErr)
	}

	missing := make([]string, 0)
	for ep, files := range templateEndpoints {
		if _, ok := routeKeys[ep]; ok {
			continue
		}
		missing = append(missing, ep+" ("+strings.Join(files, ", ")+")")
	}

	if len(missing) > 0 {
		t.Fatalf("renderer route map is missing url_for endpoints:\n%s", strings.Join(missing, "\n"))
	}
}
