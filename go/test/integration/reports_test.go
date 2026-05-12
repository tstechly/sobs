// Package integration tests reports endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/url"
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

		t.Logf("GET /reports returned status: %d", resp.StatusCode)
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

		t.Logf("GET /api/reports returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/reports creates report", func(t *testing.T) {
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

		t.Logf("POST /api/reports returned status: %d", resp.StatusCode)
	})

	t.Run("DELETE /api/reports/<report_id> deletes report", func(t *testing.T) {
		req, err := http.NewRequest("DELETE", baseURL+"/api/reports/test-report-id", nil)
		if err != nil {
			t.Fatalf("Failed to create request: %v", err)
		}

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("DELETE /api/reports/test-report-id returned status: %d", resp.StatusCode)
	})

	t.Run("GET /api/reports/export exports reports", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/reports/export")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/reports/export returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/reports/import imports report", func(t *testing.T) {
		payload := map[string]interface{}{
			"reportJson": `{"name": "Imported Report"}`,
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/reports/import", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/reports/import returned status: %d", resp.StatusCode)
	})

	t.Run("POST /reports/<report_id>/delete deletes report (UI)", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/reports/test-report-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /reports/test-report-id/delete returned status: %d", resp.StatusCode)
	})
}
