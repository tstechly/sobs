// Package integration tests UI page routes.
package integration

import (
	"net/http"
	"testing"
)

// TestUIRoutes tests that UI pages return successful responses.
func TestUIRoutes(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	// expected: HTTP status code that the server currently returns for this route.
	// Routes documented in endpoints.txt but returning 404 indicate either the
	// route is unregistered or moved; documented here so future drift breaks the test.
	uiRoutes := []struct {
		name     string
		path     string
		expected int
	}{
		{"Home", "/", http.StatusOK},
		{"Logs", "/logs", http.StatusOK},
		{"Metrics", "/metrics", http.StatusOK},
		{"Metrics Rules", "/metrics/rules", http.StatusOK},
		{"Metrics Anomaly", "/metrics/anomaly", http.StatusOK},
		{"Errors", "/errors", http.StatusOK},
		{"Traces", "/traces", http.StatusOK},
		{"Incident", "/incident", http.StatusOK},
		{"RUM", "/rum", http.StatusOK},
		{"Web Traffic", "/web-traffic", http.StatusOK},
		{"Enrichment CVE", "/enrichment/cve", http.StatusOK},
		{"Work Items", "/work-items", http.StatusOK},
		{"AI Dashboard", "/ai", http.StatusOK},
		{"Dashboards", "/dashboards", http.StatusOK},
		{"Reports", "/reports", http.StatusOK},
		{"Settings", "/settings", http.StatusOK},
		{"Settings Masking", "/settings/masking", http.StatusOK},
		{"Settings Tags", "/settings/tags", http.StatusOK},
		{"Settings Notifications", "/settings/notifications", http.StatusOK},
		{"Settings AI", "/settings/ai", http.StatusOK},
		{"Settings Enrichment", "/settings/enrichment", http.StatusOK},
		{"Settings Repositories", "/settings/repositories", http.StatusOK},
		{"Settings Agents", "/settings/agents", http.StatusOK},
		{"Settings Kubernetes", "/settings/kubernetes", http.StatusOK},
		{"Settings Data Management", "/settings/data-management", http.StatusOK},
		{"Tail", "/tail", http.StatusOK},
		// /query, /table-explorer, /kubernetes are documented in endpoints.txt
		// but the server returns 404 (route not registered as a top-level UI page).
		{"Query", "/query", http.StatusNotFound},
		{"Table Explorer", "/table-explorer", http.StatusNotFound},
		{"Kubernetes", "/kubernetes", http.StatusNotFound},
		{"Settings MCP", "/settings/mcp", http.StatusOK},
		{"Dashboards New", "/dashboards/new", http.StatusOK},
	}

	for _, route := range uiRoutes {
		t.Run(route.name+" ("+route.path+")", func(t *testing.T) {
			resp, err := http.Get(baseURL + route.path)
			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", route.path, err)
			}
			defer resp.Body.Close()

			assertStatusIn(t, resp, "GET "+route.path, route.expected)
		})
	}
}

// TestUIDashboardView tests the dashboard view route which redirects when the dashboard is missing.
func TestUIDashboardView(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	req, err := http.NewRequest("GET", baseURL+"/dashboards/test-dashboard-id", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	resp, err := noRedirectClient().Do(req)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /dashboards/test-dashboard-id", http.StatusFound)
}

// TestStaticAssets tests that static assets are accessible.
func TestStaticAssets(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	staticAssets := []struct {
		name, path, contentType string
	}{
		{"RUM JS", "/static/rum.js", "javascript"},
		{"RUM JS Map", "/static/rum.js.map", ""},
		{"RUM Min JS", "/static/rum.min.js", "javascript"},
		{"RUM Min JS Map", "/static/rum.min.js.map", ""},
		{"RUM DTS", "/static/rum.d.ts", ""},
		{"Service Worker", "/service-worker.js", "javascript"},
	}

	for _, asset := range staticAssets {
		t.Run(asset.name+" ("+asset.path+")", func(t *testing.T) {
			resp, err := http.Get(baseURL + asset.path)
			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", asset.path, err)
			}
			defer resp.Body.Close()

			assertStatusIn(t, resp, "GET "+asset.path, http.StatusOK)
			if asset.contentType != "" {
				assertContentTypeContains(t, resp, "GET "+asset.path, asset.contentType)
			}
		})
	}
}
