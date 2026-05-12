// Package integration tests repository-related endpoints.
package integration

import (
	"net/http"
	"net/url"
	"strings"
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

		assertStatusIn(t, resp, "GET /settings/repositories", http.StatusOK)
	})
}

// TestRepositoriesAPI tests repository API endpoints.
// All form-encoded POSTs that succeed return 302 (redirect to settings page).
func TestRepositoriesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("POST /settings/repositories creates repository", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("name", "test-repo")
		payload.Set("url", "https://github.com/test/repo")

		resp, err := postFormNoRedirect(baseURL+"/settings/repositories", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories", http.StatusFound)
	})

	t.Run("POST /settings/repositories/github-token/validate validates token", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("token", "test-token")

		resp, err := postFormNoRedirect(baseURL+"/settings/repositories/github-token/validate", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories/github-token/validate", http.StatusFound)
	})

	t.Run("POST /settings/repositories/<app_id>/realtime-mode toggles realtime", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/repositories/test-app-id/realtime-mode", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/realtime-mode", http.StatusFound)
	})

	t.Run("POST /settings/repositories/<app_id>/ci-ingest-key/rotate rotates key", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/rotate", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/ci-ingest-key/rotate", http.StatusFound)
	})

	t.Run("POST /settings/repositories/<app_id>/ci-ingest-key/revoke revokes key", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/repositories/test-app-id/ci-ingest-key/revoke", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/ci-ingest-key/revoke", http.StatusFound)
	})

	t.Run("POST /settings/repositories/<app_id> updates repository", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("description", "Updated description")

		resp, err := postFormNoRedirect(baseURL+"/settings/repositories/test-app-id", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories/test-app-id", http.StatusFound)
	})

	t.Run("POST /settings/repositories/<app_id>/releases creates release", func(t *testing.T) {
		payload := url.Values{}
		payload.Set("version", "1.0.0")

		resp, err := postFormNoRedirect(baseURL+"/settings/repositories/test-app-id/releases", strings.NewReader(payload.Encode()))
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/releases", http.StatusFound)
	})

	t.Run("POST /settings/repositories/<app_id>/delete deletes repository", func(t *testing.T) {
		resp, err := postFormNoRedirect(baseURL+"/settings/repositories/test-app-id/delete", nil)
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "POST /settings/repositories/test-app-id/delete", http.StatusFound)
	})
}

// TestEnrichmentSettingsUI tests enrichment settings UI route.
func TestEnrichmentSettingsUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	resp, err := http.Get(baseURL + "/settings/enrichment")
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	assertStatusIn(t, resp, "GET /settings/enrichment", http.StatusOK)
}
