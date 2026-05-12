// Package integration tests reports endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestReportsUI tests reports UI routes.
func TestReportsUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /reports lists reports", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/reports")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /reports", http.StatusOK)
	})
}

// TestReportsAPI tests reports API endpoints.
func TestReportsAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/reports lists reports", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/reports")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/reports", http.StatusOK)
		assertJSONBody(t, resp, "GET /api/reports")
	})

	t.Run("POST /api/reports rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"name":        "Test Report",
			"description": "Test report description",
			"pageType":    "logs",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/reports", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/reports", http.StatusBadRequest)
	})

	t.Run("DELETE /api/reports/<report_id> returns 404 for missing report", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", baseURL+"/api/reports/test-report-id", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "DELETE /api/reports/test-report-id", http.StatusNotFound)
	})

	t.Run("GET /api/reports/export exports reports", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/reports/export")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/reports/export", http.StatusOK)
	})

	t.Run("POST /api/reports/import rejects missing fields", func(t *testing.T) {
		payload := map[string]interface{}{
			"reportJson": `{"name": "Imported Report"}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/reports/import", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /api/reports/import", http.StatusBadRequest)
	})

	t.Run("POST /reports/<report_id>/delete redirects after delete", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/reports/test-report-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /reports/test-report-id/delete", http.StatusFound)
	})
}
