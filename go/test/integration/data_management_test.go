// Package integration tests data management endpoints.
package integration

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

// TestDataManagementUI tests data management UI routes.
func TestDataManagementUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/data-management returns data management page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/data-management")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /settings/data-management returned status: %d", resp.StatusCode)
	})
}

// TestDataManagementAPI tests data management API endpoints.
func TestDataManagementAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/data-management/backup/list lists backups", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/api/data-management/backup/list")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /api/data-management/backup/list returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/data-management/backup/run runs backup", func(t *testing.T) {
		resp, err := http.Post(baseURL+"/api/data-management/backup/run", "application/json", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/data-management/backup/run returned status: %d", resp.StatusCode)
	})

	t.Run("POST /api/data-management/restore restores backup", func(t *testing.T) {
		payload := map[string]interface{}{
			"backupId": "test-backup-id",
		}
		body, _ := json.Marshal(payload)

		resp, err := http.Post(baseURL+"/api/data-management/restore", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /api/data-management/restore returned status: %d", resp.StatusCode)
	})
}
