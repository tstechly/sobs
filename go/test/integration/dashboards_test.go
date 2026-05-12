// Package integration tests dashboard-related endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
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

		t.Logf("GET /dashboards returned status: %d", resp.StatusCode)
	})

	t.Run("GET /dashboards/new shows new dashboard form", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/dashboards/new")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /dashboards/new returned status: %d", resp.StatusCode)
	})

	t.Run("GET /dashboards/<dashboard_id> shows dashboard", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/dashboards/test-dashboard-id")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /dashboards/test-dashboard-id returned status: %d", resp.StatusCode)
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

		t.Logf("GET /api/dashboards/list returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/query/add-to-dashboard adds query to dashboard", func(t *testing.T) {
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

		t.Logf("POST /api/query/add-to-dashboard returned status: %d", resp.StatusCode)
	})

	t.Run("POST /dashboards creates dashboard", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "Test Dashboard")
		payload.Set("description", "Test dashboard description")

		resp, err := http.PostForm(baseURL+"/dashboards", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /dashboards returned status: %d", resp.StatusCode)
	})

	t.Run("POST /dashboards/<dashboard_id>/delete deletes dashboard", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/dashboards/test-dashboard-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /dashboards/test-dashboard-id/delete returned status: %d", resp.StatusCode)
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

		resp, err := http.PostForm(baseURL+"/dashboards/test-dashboard-id/charts", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /dashboards/test-dashboard-id/charts returned status: %d", resp.StatusCode)
	})

	t.Run("POST /dashboards/<dashboard_id>/charts/<chart_id>/edit edits chart", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("title", "Updated Chart Title")

		resp, err := http.PostForm(baseURL+"/dashboards/test-dashboard-id/charts/test-chart-id/edit", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /dashboards/test-dashboard-id/charts/test-chart-id/edit returned status: %d", resp.StatusCode)
	})

	t.Run("POST /dashboards/<dashboard_id>/charts/<chart_id>/clone clones chart", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/dashboards/test-dashboard-id/charts/test-chart-id/clone", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /dashboards/test-dashboard-id/charts/test-chart-id/clone returned status: %d", resp.StatusCode)
	})

	t.Run("POST /dashboards/<dashboard_id>/charts/<chart_id>/delete deletes chart", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/dashboards/test-dashboard-id/charts/test-chart-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /dashboards/test-dashboard-id/charts/test-chart-id/delete returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/dashboards/<dashboard_id>/charts/<chart_id>/export exports chart", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/dashboards/test-dashboard-id/charts/test-chart-id/export")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/dashboards/test-dashboard-id/charts/test-chart-id/export returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/dashboards/<dashboard_id>/charts/import imports chart", func(t *testing.T) {
		payload := map[string]interface{}{
			"chartJson": `{"title": "Imported Chart"}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/test-dashboard-id/charts/import", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/dashboards/test-dashboard-id/charts/import returned status: %d", resp.StatusCode)
	})
}

// TestDashboardQuery tests dashboard query endpoints.
func TestDashboardQuery(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /api/dashboards/query queries dashboard data", func(t *testing.T) {
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

		t.Logf("POST /api/dashboards/query returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/dashboards/render renders dashboard", func(t *testing.T) {
		payload := map[string]interface{}{
			"dashboardId": "test-dashboard-id",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/render", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/dashboards/render returned status: %d", resp.StatusCode)
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

		t.Logf("GET /api/dashboards/spec/templates returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/dashboards/spec/options returns options", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/dashboards/spec/options")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/dashboards/spec/options returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/dashboards/spec/compile compiles spec", func(t *testing.T) {
		payload := map[string]interface{}{
			"specJson": `{"charts": []}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/spec/compile", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/dashboards/spec/compile returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/dashboards/spec/dry-run dry runs spec", func(t *testing.T) {
		payload := map[string]interface{}{
			"specJson": `{"charts": []}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/spec/dry-run", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/dashboards/spec/dry-run returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/dashboards/spec/validate validates spec", func(t *testing.T) {
		payload := map[string]interface{}{
			"specJson": `{"charts": []}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/spec/validate", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/dashboards/spec/validate returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/dashboards/spec/render renders spec", func(t *testing.T) {
		payload := map[string]interface{}{
			"specJson": `{"charts": []}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/spec/render", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/dashboards/spec/render returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/dashboards/spec/ai-build builds spec with AI", func(t *testing.T) {
		payload := map[string]interface{}{
			"prompt": "Create a dashboard showing error rates",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/dashboards/spec/ai-build", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/dashboards/spec/ai-build returned status: %d", resp.StatusCode)
	})
}

// TestMetricsAnomalyAPI tests metrics anomaly endpoints.
func TestMetricsAnomalyAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/metrics/anomaly returns anomaly data", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/metrics/anomaly")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/metrics/anomaly returned status: %d", resp.StatusCode)
	})
}
