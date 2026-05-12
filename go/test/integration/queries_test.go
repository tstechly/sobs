// Package integration tests query and table explorer endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestQueryUI tests query UI routes.
// Note: /query and /table-explorer are documented in endpoints.txt but the server
// returns 404 (routes not registered as top-level UI pages).
func TestQueryUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /query returns 404 (route not registered)", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/query")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /query", http.StatusNotFound)
	})

	t.Run("GET /table-explorer returns 404 (route not registered)", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/table-explorer")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /table-explorer", http.StatusNotFound)
	})
}

// TestQueryAPI tests query API endpoints.
func TestQueryAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /api/query/ask requires fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"query": "show me errors from last hour",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/query/ask", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/query/ask", http.StatusBadRequest)
	})

	t.Run("POST /api/query/run requires sql", func(t *testing.T) {
		// Empty payload to assert the "sql is required" validation path.
		body, _ := json.Marshal(map[string]interface{}{})

		resp, err := http.Post(baseURL+"/api/query/run", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/query/run", http.StatusBadRequest)
		assertJSONBody(t, resp, "POST /api/query/run")
	})

	t.Run("POST /api/query/refine-chart returns 404 (route not registered)", func(t *testing.T) {
		payload := map[string]interface{}{
			"chartConfig": map[string]interface{}{"type": "line"},
			"feedback":    "make the line thicker",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/query/refine-chart", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/query/refine-chart", http.StatusNotFound)
	})

	t.Run("GET /api/query/schema returns 404 (route not registered)", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/query/schema")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/query/schema", http.StatusNotFound)
	})
}

// TestTableExplorerAPI tests table explorer API endpoints.
// Note: these endpoints are documented but the server returns 404.
func TestTableExplorerAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/table-explorer/tables returns 404 (route not registered)", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/table-explorer/tables")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/table-explorer/tables", http.StatusNotFound)
	})

	t.Run("GET /api/table-explorer/table/<name> returns 404 (route not registered)", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/table-explorer/table/otel_logs")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/table-explorer/table/otel_logs", http.StatusNotFound)
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

		assertStatusIn(t, resp, "GET /api/chart-types", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/chart-types")
	})
}
