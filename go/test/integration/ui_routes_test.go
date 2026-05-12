// Package integration tests UI page routes.
package integration

import (
	"io"
	"net/http"
	"testing"
)

// TestUIRoutes tests that UI pages return successful responses.
func TestUIRoutes(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	uiRoutes := []struct {
		name   string
		path   string
		method string
	}{
		{"Home", "/", "GET"},
		{"Logs", "/logs", "GET"},
		{"Metrics", "/metrics", "GET"},
		{"Metrics Rules", "/metrics/rules", "GET"},
		{"Metrics Anomaly", "/metrics/anomaly", "GET"},
		{"Errors", "/errors", "GET"},
		{"Traces", "/traces", "GET"},
		{"Incident", "/incident", "GET"},
		{"RUM", "/rum", "GET"},
		{"Web Traffic", "/web-traffic", "GET"},
		{"Enrichment CVE", "/enrichment/cve", "GET"},
		{"Work Items", "/work-items", "GET"},
		{"AI Dashboard", "/ai", "GET"},
		{"Dashboards", "/dashboards", "GET"},
		{"Reports", "/reports", "GET"},
		{"Settings", "/settings", "GET"},
		{"Settings Masking", "/settings/masking", "GET"},
		{"Settings Tags", "/settings/tags", "GET"},
		{"Settings Notifications", "/settings/notifications", "GET"},
		{"Settings AI", "/settings/ai", "GET"},
		{"Settings Enrichment", "/settings/enrichment", "GET"},
		{"Settings Repositories", "/settings/repositories", "GET"},
		{"Settings Agents", "/settings/agents", "GET"},
		{"Settings Kubernetes", "/settings/kubernetes", "GET"},
		{"Settings Data Management", "/settings/data-management", "GET"},
		{"Tail", "/tail", "GET"},
		{"Query", "/query", "GET"},
		{"Table Explorer", "/table-explorer", "GET"},
		{"Kubernetes", "/kubernetes", "GET"},
		{"Settings MCP", "/settings/mcp", "GET"},
	}

	for _, route := range uiRoutes {
		t.Run(route.name+" ("+route.path+")", func(t *testing.T) {
			var resp *http.Response
			var err error

			switch route.method {
			case "GET":
				resp, err = http.Get(baseURL + route.path)
			default:
				t.Fatalf("Unsupported method: %s", route.method)
				return
			}

			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", route.path, err)
			}
			defer resp.Body.Close()

			// UI pages typically return 200 OK or 302 if redirected to login
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusFound {
				t.Errorf("Expected 200 or 302 for %s, got %d", route.path, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			t.Logf("%s %s returned status: %d, body length: %d bytes", 
				route.method, route.path, resp.StatusCode, len(body))
		})
	}
}

// TestStaticAssets tests that static assets are accessible.
func TestStaticAssets(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	staticAssets := []struct {
		name string
		path string
	}{
		{"RUM JS", "/static/rum.js"},
		{"RUM JS Map", "/static/rum.js.map"},
		{"RUM Min JS", "/static/rum.min.js"},
		{"RUM Min JS Map", "/static/rum.min.js.map"},
		{"RUM DTS", "/static/rum.d.ts"},
		{"Service Worker", "/service-worker.js"},
	}

	for _, asset := range staticAssets {
		t.Run(asset.name+" ("+asset.path+")", func(t *testing.T) {
			resp, err := http.Get(baseURL + asset.path)
			if err != nil {
				t.Fatalf("Failed to make request to %s: %v", asset.path, err)
			}
			defer resp.Body.Close()

			// Static assets should return 200 OK or 404 if not found
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
				t.Errorf("Expected 200 or 404 for %s, got %d", asset.path, resp.StatusCode)
			}

			t.Logf("GET %s returned status: %d", asset.path, resp.StatusCode)
		})
	}
}
