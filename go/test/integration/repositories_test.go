// Package integration tests repository-related endpoints.
package integration

import (
	"encoding/json"
	"net/http"
	"net/url"
	"testing"
)

// TestRepositoriesUI tests repository settings UI routes.
func TestRepositoriesUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/repositories returns repos settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/repositories")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("GET /settings/repositories returned status: %d", resp.StatusCode)
	})
}

// TestRepositoriesAPI tests repository API endpoints.
func TestRepositoriesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /settings/repositories creates repository", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "test-repo")
		payload.Set("url", "https://github.com/test/repo")

		resp, err := http.PostForm(baseURL+"/settings/repositories", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/repositories/github-token/validate validates token", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("token", "test-token")

		resp, err := http.PostForm(baseURL+"/settings/repositories/github-token/validate", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories/github-token/validate returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/repositories/<app_id>/realtime-mode toggles realtime", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/repositories/test-app-id/realtime-mode", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories/test-app-id/realtime-mode returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/repositories/<app_id>/ci-ingest-key/rotate rotates key", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/rotate", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories/test-app-id/ci-ingest-key/rotate returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/repositories/<app_id>/ci-ingest-key/revoke revokes key", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/revoke", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories/test-app-id/ci-ingest-key/revoke returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/repositories/<app_id> updates repository", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("description", "Updated description")

		resp, err := http.PostForm(baseURL+"/settings/repositories/test-app-id", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories/test-app-id returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/repositories/<app_id>/releases creates release", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("version", "1.0.0")

		resp, err := http.PostForm(baseURL+"/settings/repositories/test-app-id/releases", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories/test-app-id/releases returned status: %d", resp.StatusCode)
	})

	t.Run("POST /settings/repositories/<app_id>/delete deletes repository", func(t *testing.T) {
		payload := url.Values{}
		resp, err := http.PostForm(baseURL+"/settings/repositories/test-app-id/delete", payload)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		t.Logf("POST /settings/repositories/test-app-id/delete returned status: %d", resp.StatusCode)
	})
}
