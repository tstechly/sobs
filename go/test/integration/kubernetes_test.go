// Package integration tests kubernetes endpoints.
package integration

import (
	"net/http"
	"testing"
)

// TestKubernetesUI tests kubernetes UI routes.
func TestKubernetesUI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /settings/kubernetes returns k8s settings page", func(t *testing.T) {
		resp, err := http.Get(baseURL + "/settings/kubernetes")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /settings/kubernetes", http.StatusOK)
	})

	t.Run("GET /kubernetes returns 404 (route not registered)", func(t *testing.T) {
		// endpoints.txt documents this as a UI page but server returns 404.
		resp, err := http.Get(baseURL + "/kubernetes")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /kubernetes", http.StatusNotFound)
	})
}

// TestKubernetesAPI tests kubernetes API endpoints.
func TestKubernetesAPI(t *testing.T) {
	baseURL := getBaseURL()
	skipIfServerNotAvailable(t, baseURL)

	t.Run("GET /api/kubernetes/status returns 404 (route not registered)", func(t *testing.T) {
		// endpoints.txt documents this endpoint but server returns 404.
		resp, err := http.Get(baseURL + "/api/kubernetes/status")
		if err != nil {
			t.Fatalf("Failed to make request: %v", err)
		}
		defer resp.Body.Close()

		assertStatusIn(t, resp, "GET /api/kubernetes/status", http.StatusNotFound)
	})
}
