// Package integration tests dashboard-related endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
)

// TestDashboardsUI tests dashboard UI routes.
func TestDashboardsUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /dashboards lists dashboards", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/dashboards")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /dashboards", http.StatusOK)
		assertContentTypeContains(t, resp, "GET /dashboards", "text/html")
	})

	t.Run("GET /dashboards/new shows new dashboard form", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/dashboards/new")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /dashboards/new", http.StatusOK)
	})

	t.Run("GET /dashboards/<dashboard_id> redirects when dashboard missing", func(t *testing.T) {
		req, _ := http.NewRequest("GET", baseURL+"/dashboards/test-dashboard-id", nil)
		resp, err := noRedirectClient().Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /dashboards/test-dashboard-id", http.StatusFound)
	})
}

// TestDashboardsAPI tests dashboard API endpoints.
func TestDashboardsAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/dashboards/list lists dashboards", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/dashboards/list")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/dashboards/list", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/dashboards/list")
	})

	t.Run("POST /api/query/add-to-dashboard rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"dashboardId": "test-dashboard-id",
			"query":       "SELECT count() FROM otel_logs",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/query/add-to-dashboard", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/query/add-to-dashboard", http.StatusBadRequest)
	})

	t.Run("POST /dashboards/<dashboard_id>/delete deletes dashboard", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/dashboards/test-dashboard-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /dashboards/test-dashboard-id/delete", http.StatusFound)
	})
}

// TestDashboardCharts tests dashboard chart endpoints.
func TestDashboardCharts(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /dashboards/<dashboard_id>/charts adds chart", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("title", "Test Chart")
		payload.Set("chartType", "line")
		payload.Set("query", "SELECT time, value FROM otel_metrics_1m")

		resp, err := postFormNoRedirect(baseURL+"/dashboards/test-dashboard-id/charts", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /dashboards/test-dashboard-id/charts", http.StatusFound)
	})

	t.Run("POST /dashboards/<dashboard_id>/charts/<chart_id>/edit edits chart", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("title", "Updated Chart Title")

		resp, err := postFormNoRedirect(baseURL+"/dashboards/test-dashboard-id/charts/test-chart-id/edit", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /dashboards/test-dashboard-id/charts/test-chart-id/edit", http.StatusFound)
	})

	t.Run("POST /dashboards/<dashboard_id>/charts/<chart_id>/clone clones chart", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/dashboards/test-dashboard-id/charts/test-chart-id/clone", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /dashboards/test-dashboard-id/charts/test-chart-id/clone", http.StatusFound)
	})

	t.Run("POST /dashboards/<dashboard_id>/charts/<chart_id>/delete deletes chart", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/dashboards/test-dashboard-id/charts/test-chart-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /dashboards/test-dashboard-id/charts/test-chart-id/delete", http.StatusFound)
	})

	t.Run("GET /api/dashboards/<dashboard_id>/charts/<chart_id>/export returns 404 for missing chart", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/dashboards/test-dashboard-id/charts/test-chart-id/export")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/dashboards/test-dashboard-id/charts/test-chart-id/export", http.StatusNotFound)
	})

	t.Run("POST /api/dashboards/<dashboard_id>/charts/import returns 404 for missing dashboard", func(t *testing.T) {
		payload := map[string]interface{}{
			"chartJson": `{"title": "Imported Chart"}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/test-dashboard-id/charts/import", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/dashboards/test-dashboard-id/charts/import", http.StatusNotFound)
	})
}

// TestDashboardQuery tests dashboard query endpoints.
func TestDashboardQuery(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /api/dashboards/query rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"dashboardId": "test-dashboard-id",
			"timeRange":   "1h",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/query", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/dashboards/query", http.StatusBadRequest)
	})

	t.Run("POST /api/dashboards/render rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"dashboardId": "test-dashboard-id",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/render", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/dashboards/render", http.StatusBadRequest)
	})
}

// TestDashboardSpec tests dashboard spec endpoints.
func TestDashboardSpec(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/dashboards/spec/templates returns templates", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/dashboards/spec/templates")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/dashboards/spec/templates", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/dashboards/spec/templates")
	})

	t.Run("GET /api/dashboards/spec/options returns options", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/dashboards/spec/options")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/dashboards/spec/options", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/dashboards/spec/options")
	})

	specEndpoints := []struct {
		name, path string
	}{
		{"compile", "/api/dashboards/spec/compile"},
		{"dry-run", "/api/dashboards/spec/dry-run"},
		{"validate", "/api/dashboards/spec/validate"},
		{"render", "/api/dashboards/spec/render"},
		{"ai-build", "/api/dashboards/spec/ai-build"},
	}

	for _, ep := range specEndpoints {
		t.Run("POST "+ep.path+" rejects empty payload", func(t *testing.T) {
			body, _ := json.Marshal(map[string]interface{}{})
			resp, err := http.Post(baseURL+ep.path, "application/json", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("Failed to make request: %v", err)
			}
			defer resp.Body.Close()

			assertStatusIn(t, resp, "POST "+ep.path, http.StatusBadRequest)
		})
	}
}

// TestMetricsAnomalyAPI tests metrics anomaly endpoints.
func TestMetricsAnomalyAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/metrics/anomaly rejects missing query params", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/metrics/anomaly")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/metrics/anomaly", http.StatusBadRequest)
	})
}
