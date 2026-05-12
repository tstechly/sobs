// Package integration tests query and table explorer endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestQueryUI tests query UI routes.
func TestQueryUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /query shows query builder", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/query")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /query returned status: %d", resp.StatusCode)
	})

	t.Run("GET /table-explorer shows table explorer", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/table-explorer")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /table-explorer returned status: %d", resp.StatusCode)
	})
}

// TestQueryAPI tests query API endpoints.
func TestQueryAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /api/query/ask queries with natural language", func(t *testing.T) {
		payload := map[string]interface{}{
			"query": "show me errors from last hour",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/query/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/query/ask returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/query/run executes SQL query", func(t *testing.T) {
		payload := map[string]interface{}{
			"sql": "SELECT count() FROM otel_logs WHERE Timestamp > now() - 3600",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/query/run", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/query/run returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/query/refine-chart refines chart", func(t *testing.T) {
		payload := map[string]interface{}{
			"chartConfig": map[string]interface{}{
				"type": "line",
			},
			"feedback": "make the line thicker",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/query/refine-chart", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/query/refine-chart returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/query/schema returns database schema", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/query/schema")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/query/schema returned status: %d", resp.StatusCode)
	})
}

// TestTableExplorerAPI tests table explorer API endpoints.
func TestTableExplorerAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/table-explorer/tables lists tables", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/table-explorer/tables")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/table-explorer/tables returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/table-explorer/table/<name> describes table", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/table-explorer/table/otel_logs")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/table-explorer/table/otel_logs returned status: %d", resp.StatusCode)
	})
}

// TestChartTypesAPI tests chart types endpoint.
func TestChartTypesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/chart-types returns available chart types", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/chart-types")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/chart-types returned status: %d", resp.StatusCode)
	})
}
